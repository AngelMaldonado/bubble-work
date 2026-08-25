package md

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// gm is the shared Markdown→HTML renderer — the same engine (goldmark GFM) the
// mds tool uses, with auto heading IDs so the web view can anchor a TOC/minimap.
// goldmark escapes raw HTML, so rendered output is safe to inject. Convert is
// safe for concurrent use.
var gm = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
)

// RenderHTML converts GFM Markdown to safe HTML for the web interior view.
func RenderHTML(markdown string) string {
	if strings.TrimSpace(markdown) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := gm.Convert([]byte(markdown), &buf); err != nil {
		return ""
	}
	return buf.String()
}

// Artifact is one "file" of a thread's work: a top-level H1 section of the body
// (or the whole body when there are no H1s), with a TOC built from its H2+.
// HTML is the goldmark-rendered body for the web view; CLI/MCP use Markdown.
type Artifact struct {
	// ID is the Plane work-item id for artifacts that ARE a work item — today
	// that is revisions, which is what makes them renamable. Empty for the
	// thread's own body, whose title is the thread's.
	ID       string     `json:"id,omitempty"`
	Title    string     `json:"title"`
	TOC      []TOCEntry `json:"toc"`
	Markdown string     `json:"markdown"`
	HTML     string     `json:"html"`
	// Todos are the checklist items written in the document itself, addressed the
	// same way the Logbook's are (region + index) so toggle_todo can tick them.
	//
	// A document's checkboxes are somebody's plan written in their own shape
	// (docs/decisions/0005). Only real `- [ ]` boxes count here: a plain bullet in
	// prose is a sentence, and the Logbook's bullet-as-todo fallback would turn a
	// paragraph of bullets into a task list nobody wrote.
	Todos []Todo `json:"todos,omitempty"`
}

// TOCEntry is one heading (level 2..6) inside an artifact.
type TOCEntry struct {
	Level int    `json:"level"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// Todo is one logbook / DoD checklist item.
//
// Region and Index are the item's ADDRESS: they are exactly what toggle_todo takes.
// They exist because a reader used to have to infer them, and inferring them wrongly
// is silent — an index counted across the whole page toggles a different item, or
// none. Indices are per region and start at 0.
type Todo struct {
	Text   string `json:"text"`
	Done   bool   `json:"done"`
	Region string `json:"region,omitempty"` // "document" | "logbook" | "dod" — pass this to toggle_todo
	Index  int    `json:"index"`            // position WITHIN that region
}

// address stamps a parsed list with the region that owns it, so every todo carries
// the two values a caller needs to act on it.
func address(todos []Todo, region Region) []Todo {
	for i := range todos {
		todos[i].Region, todos[i].Index = string(region), i
	}
	return todos
}

// Logbook is the thread's unique task section: the raw markdown, its parsed
// todos, the Definition of Done items (merged in per INTERIOR-PLAN.md), and
// whether the work is phased (has phase subheadings) vs a flat task list.
type Logbook struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`               // goldmark-rendered logbook (web view)
	Todos    []Todo `json:"todos"`              // parsed items (CLI/MCP)
	DoD      []Todo `json:"dod,omitempty"`      // parsed DoD items (CLI/MCP)
	DoDHTML  string `json:"dod_html,omitempty"` // goldmark-rendered DoD (web view)
	Phased   bool   `json:"phased"`
}

var (
	headingRe = regexp.MustCompile(`(?m)^(#{1,6})\s+(.*)$`)
	h1Re      = regexp.MustCompile(`^#\s+(.*)$`)
	taskRe    = regexp.MustCompile(`^\s*[-*]\s+\[([ xX])\]\s+(.*)$`)
	bulletRe  = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	slugRe    = regexp.MustCompile(`[^a-z0-9]+`)
)

// ParseThread turns a thread's full body markdown into its interior pieces: the
// work-artifact files, and the Logbook (with DoD folded in), if present. The
// Logbook and Definition-of-Done sections are pulled out of the body so they do
// not also appear as work artifacts. threadName titles the implicit artifact
// when the body has no H1 (or has a preamble before the first H1).
func ParseThread(body, threadName string) (artifacts []Artifact, logbook *Logbook) {
	body = strings.TrimSpace(body)

	logMD, rest, hasLog := ExtractSection(body, "logbook")
	dodMD, rest, hasDoD := ExtractSection(rest, "definition of done", "dod")

	// Render the whole remaining body as ONE document. Plane uses H1/H2 as
	// ordinary in-content headings, so splitting on H1 would fragment real work
	// items into spurious "files". The minimap navigates the headings instead.
	if rest = strings.TrimSpace(rest); rest != "" {
		title := strings.TrimSpace(threadName)
		if title == "" {
			title = "Document"
		}
		art := makeArtifact(title, rest)
		art.Todos = address(ParseChecklist(rest), RegionDocument)
		artifacts = []Artifact{art}
	}

	if hasLog || hasDoD {
		logbook = &Logbook{
			Markdown: logMD,
			HTML:     RenderHTML(logMD),
			Todos:    address(ParseTodos(logMD), RegionLogbook),
			DoD:      address(ParseTodos(dodMD), RegionDoD),
			DoDHTML:  RenderHTML(dodMD),
			Phased:   headingRe.MatchString(logMD),
		}
	}
	return artifacts, logbook
}

// BodyFingerprint is a stable hash of a thread's WHOLE document — every region,
// not just the plan (docs/decisions/0004).
//
// Whitespace is collapsed the same way LogbookFingerprint collapses it, so a reflow
// or a re-indent is not a change. That normalization is the only thing standing
// between "any edit counts" and "Plane re-serialising a body counts".
//
// Both fingerprints are kept, and the difference is what LABELS the evidence: a
// change inside the Logbook or the Definition of Done is a plan change
// (logbook-updated), a change anywhere else is a document change (body-updated).
// Both are production; knowing which is which is worth two hashes.
func BodyFingerprint(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var norm []string
	for _, ln := range strings.Split(body, "\n") {
		if ln = strings.Join(strings.Fields(ln), " "); ln != "" {
			norm = append(norm, ln)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(norm, "\n")))
	return hex.EncodeToString(sum[:8])
}

// LogbookFingerprint returns a stable hash of a thread's Logbook and Definition
// of Done sections — the plan, not the prose around it. Whitespace is normalized
// first, so a reflow or a trailing space is not a change; anything else (a todo
// ticked or unticked, an item added, a phase reworded) is.
//
// Returns "" when the thread has no Logbook at all, which reads naturally as
// "no plan yet" and becomes a change the moment one is written.
func LogbookFingerprint(body string) string {
	logMD, rest, hasLog := ExtractSection(strings.TrimSpace(body), "logbook")
	dodMD, _, hasDoD := ExtractSection(rest, "definition of done", "dod")
	if !hasLog && !hasDoD {
		return ""
	}
	var norm []string
	for _, section := range []string{logMD, dodMD} {
		for _, ln := range strings.Split(section, "\n") {
			// Collapse every run of whitespace: indentation and spacing are cosmetic,
			// and cosmetic edits must not generate heat (AGENTS.md).
			if ln = strings.Join(strings.Fields(ln), " "); ln != "" {
				norm = append(norm, ln)
			}
		}
		norm = append(norm, "\x00") // keep the section boundary significant
	}
	sum := sha256.Sum256([]byte(strings.Join(norm, "\n")))
	return hex.EncodeToString(sum[:8])
}

// CountDone reports how many checklist items in a thread body are ticked,
// ANYWHERE in the document — the Logbook and the Definition of Done have no
// monopoly on a plan (docs/decisions/0005). Somebody who writes their todos under
// their own headings produces the same evidence as somebody who uses ours.
//
// Checkbox items only: the bullet-as-todo fallback ParseTodos applies inside a
// Logbook would read a paragraph of prose bullets as a task list, and a task list
// nobody wrote can never be ticked, so it would only ever depress the count.
//
// Deliberately cheap — it scans lines and counts, rendering no HTML — because the
// snapshot refresher runs it over every work item in a project on every sweep,
// only to notice when the number goes up (THREAD-LIFECYCLE.md Phase C).
func CountDone(body string) int {
	n := 0
	for _, t := range ParseChecklist(body) {
		if t.Done {
			n++
		}
	}
	return n
}

// ExtractSection finds the first heading whose (trimmed, lower-cased) text
// matches any of titles and returns that section's body (everything after the
// heading line up to the next heading of the same or higher level), the input
// with that section removed, and whether it was found.
func ExtractSection(md string, titles ...string) (section, rest string, found bool) {
	want := make(map[string]bool, len(titles))
	for _, t := range titles {
		want[strings.ToLower(strings.TrimSpace(t))] = true
	}
	lines := strings.Split(md, "\n")

	start, level := -1, 0
	for i, ln := range lines {
		if m := headingRe.FindStringSubmatch(ln); m != nil {
			if want[strings.ToLower(strings.TrimSpace(m[2]))] {
				start, level = i, len(m[1])
				break
			}
		}
	}
	if start < 0 {
		return "", md, false
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if m := headingRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= level {
			end = i
			break
		}
	}

	section = strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
	remaining := append(append([]string{}, lines[:start]...), lines[end:]...)
	rest = strings.TrimSpace(strings.Join(remaining, "\n"))
	return section, rest, true
}

// ReplaceSection swaps the CONTENT of a section, leaving its heading, its
// position in the document and every other section untouched. When the section
// is absent it is appended with an H2 heading, so a thread born without a
// Logbook gains one rather than silently swallowing the update.
//
// This exists so an agent can rewrite a Logbook without being able to touch the
// Brief. The two live in one Plane description by design (§3), so a whole-body
// write is the natural shape and the wrong one: the Brief is the human's
// statement of intent, and nothing that edits a plan should be able to erase it.
func ReplaceSection(md, title, body string) string {
	lines := strings.Split(md, "\n")
	want := strings.ToLower(strings.TrimSpace(title))

	start, level := -1, 0
	for i, ln := range lines {
		if m := headingRe.FindStringSubmatch(ln); m != nil {
			if strings.ToLower(strings.TrimSpace(m[2])) == want {
				start, level = i, len(m[1])
				break
			}
		}
	}
	body = strings.TrimSpace(body)
	if start < 0 {
		out := strings.TrimSpace(md)
		if out != "" {
			out += "\n\n"
		}
		return out + "## " + title + "\n\n" + body
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if m := headingRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= level {
			end = i
			break
		}
	}
	out := append([]string{}, lines[:start+1]...) // keep the heading exactly as written
	out = append(out, "", body, "")
	out = append(out, lines[end:]...)
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

// Split breaks body into artifacts on top-level H1 headings. Any content before
// the first H1 (or the whole body when there are none) becomes one artifact
// titled fallbackTitle (defaulting to "Brief").
func Split(body, fallbackTitle string) []Artifact {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	if fallbackTitle == "" {
		fallbackTitle = "Brief"
	}
	lines := strings.Split(body, "\n")

	var idx []int
	var titles []string
	for i, ln := range lines {
		if m := h1Re.FindStringSubmatch(ln); m != nil {
			idx = append(idx, i)
			titles = append(titles, strings.TrimSpace(m[1]))
		}
	}
	if len(idx) == 0 {
		return []Artifact{makeArtifact(fallbackTitle, body)}
	}

	var arts []Artifact
	if pre := strings.TrimSpace(strings.Join(lines[:idx[0]], "\n")); pre != "" {
		arts = append(arts, makeArtifact(fallbackTitle, pre))
	}
	for k := range idx {
		start := idx[k] + 1
		end := len(lines)
		if k+1 < len(idx) {
			end = idx[k+1]
		}
		arts = append(arts, makeArtifact(titles[k], strings.Join(lines[start:end], "\n")))
	}
	return arts
}

// ParseTodos extracts checklist items from a section. GFM checkboxes
// (`- [ ]` / `- [x]`) win; if a section has none, plain bullets become open
// todos so a simple task list still surfaces.
func ParseTodos(section string) []Todo {
	section = strings.TrimSpace(section)
	if section == "" {
		return nil
	}
	lines := strings.Split(section, "\n")

	var out []Todo
	for _, ln := range lines {
		if m := taskRe.FindStringSubmatch(ln); m != nil {
			out = append(out, Todo{Text: strings.TrimSpace(m[2]), Done: strings.EqualFold(m[1], "x")})
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, ln := range lines {
		if m := bulletRe.FindStringSubmatch(ln); m != nil {
			out = append(out, Todo{Text: strings.TrimSpace(m[1])})
		}
	}
	return out
}

// ParseChecklist extracts ONLY real GFM checkboxes (`- [ ]` / `- [x]`), with no
// bullet fallback. It is what reads a free-form document, where a bullet is prose
// rather than a task.
func ParseChecklist(section string) []Todo {
	var out []Todo
	for _, ln := range strings.Split(section, "\n") {
		if m := taskRe.FindStringSubmatch(ln); m != nil {
			out = append(out, Todo{Text: strings.TrimSpace(m[2]), Done: strings.EqualFold(m[1], "x")})
		}
	}
	return out
}

// NewArtifact builds a single artifact (title + body) with a TOC derived from
// the body's H2+ headings — used for free-form content like revision notes.
func NewArtifact(title, body string) Artifact { return makeArtifact(title, body) }

func makeArtifact(title, body string) Artifact {
	body = strings.TrimSpace(body)
	return Artifact{Title: title, Markdown: body, TOC: toc(body), HTML: RenderHTML(body)}
}

func toc(body string) []TOCEntry {
	var out []TOCEntry
	for _, m := range headingRe.FindAllStringSubmatch(body, -1) {
		level := len(m[1])
		if level < 2 { // H1s are artifact titles, not TOC entries
			continue
		}
		title := strings.TrimSpace(m[2])
		out = append(out, TOCEntry{Level: level, Title: title, Slug: slugify(title)})
	}
	return out
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// nextRe finds the `Next:` label ANYWHERE on a line, and captures the rest of it.
//
// Anywhere, because the status line the framework prescribes puts three labels on one
// line — `**Owner:** … · **State:** … · **Next:** …` — so a pattern anchored at the
// start of the line would miss the exact shape everybody is told to write. (It did,
// until a test wrote that expectation down and made it obvious.)
//
// The label must still be introduced by the start of the line or by a separator, so
// prose like "- [ ] do the next thing: ..." is not mistaken for a declaration.
var nextRe = regexp.MustCompile(`(?i)(?:^|[·|—;]|\*\*|__)\s*(?:\*\*|__)?next(?:\*\*|__)?\s*:\s*(.+)$`)

// NextAction returns the single next concrete action a Logbook declares, or "".
//
// docs/decisions/0003 made this a CONVENTION rather than a field, on the grounds that
// parsing markdown people already write is cheap and migrating a field into prose is
// not. This is that parse, and it is what lets an audit answer "what happens next
// here?" for a whole bubble without opening every thread.
func NextAction(logbook string) string {
	for _, ln := range strings.Split(logbook, "\n") {
		if m := nextRe.FindStringSubmatch(strings.TrimSpace(ln)); m != nil {
			// Strip trailing emphasis the author may have wrapped the value in.
			return strings.TrimSpace(strings.Trim(strings.TrimSpace(m[1]), "*_"))
		}
	}
	return ""
}
