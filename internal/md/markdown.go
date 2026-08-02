package md

import (
	"regexp"
	"strings"
)

// Artifact is one "file" of a thread's work: a top-level H1 section of the body
// (or the whole body when there are no H1s), with a TOC built from its H2+.
type Artifact struct {
	Title    string     `json:"title"`
	TOC      []TOCEntry `json:"toc"`
	Markdown string     `json:"markdown"`
}

// TOCEntry is one heading (level 2..6) inside an artifact.
type TOCEntry struct {
	Level int    `json:"level"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// Todo is one logbook / DoD checklist item.
type Todo struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Logbook is the thread's unique task section: the raw markdown, its parsed
// todos, the Definition of Done items (merged in per INTERIOR-PLAN.md), and
// whether the work is phased (has phase subheadings) vs a flat task list.
type Logbook struct {
	Markdown string `json:"markdown"`
	Todos    []Todo `json:"todos"`
	DoD      []Todo `json:"dod,omitempty"`
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

	artifacts = Split(rest, threadName)

	if hasLog || hasDoD {
		logbook = &Logbook{
			Markdown: logMD,
			Todos:    ParseTodos(logMD),
			DoD:      ParseTodos(dodMD),
			Phased:   headingRe.MatchString(logMD),
		}
	}
	return artifacts, logbook
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

// NewArtifact builds a single artifact (title + body) with a TOC derived from
// the body's H2+ headings — used for free-form content like revision notes.
func NewArtifact(title, body string) Artifact { return makeArtifact(title, body) }

func makeArtifact(title, body string) Artifact {
	return Artifact{Title: title, Markdown: strings.TrimSpace(body), TOC: toc(body)}
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
