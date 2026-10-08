package md

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/toc"
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

// tocMin is how many headings a document needs before it earns a table of
// contents. Below it the document fits in a glance and a TOC is noise.
const tocMin = 3

// RenderThreadHTML is RenderHTML plus a table of contents on top, for the
// documents a THREAD owns — the ones that grow until nobody can find anything
// in them.
//
// The TOC is DERIVED from the headings on every render and never written into
// the file. One an agent writes goes stale on the next edit and costs tokens on
// every write; this one cannot disagree with the document, because it is read
// from the same parse that renders it — the anchors are the ids goldmark just
// gave those headings.
//
// It arrives as a closed <details>: one line until somebody opens it. An
// always-open TOC is the same space problem as an always-open image.
func RenderThreadHTML(markdown string) string {
	if strings.TrimSpace(markdown) == "" {
		return ""
	}
	src := []byte(markdown)
	doc := gm.Parser().Parse(text.NewReader(src))

	var body bytes.Buffer
	if err := gm.Renderer().Render(&body, src, doc); err != nil {
		return ""
	}

	tree, err := toc.Inspect(doc, src, toc.MaxDepth(3), toc.Compact(true))
	if err != nil || countTOC(tree.Items) < tocMin {
		return body.String()
	}
	list := toc.RenderList(tree)
	if list == nil {
		return body.String()
	}
	var out bytes.Buffer
	out.WriteString(`<details class="doc-toc"><summary>Contenido</summary>`)
	if err := gm.Renderer().Render(&out, src, list); err != nil {
		return body.String()
	}
	out.WriteString("</details>\n")
	out.Write(body.Bytes())
	return out.String()
}

func countTOC(items toc.Items) int {
	n := 0
	for _, it := range items {
		if len(it.ID) > 0 {
			n++
		}
		n += countTOC(it.Items)
	}
	return n
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
