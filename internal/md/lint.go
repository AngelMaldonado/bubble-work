package md

import (
	"fmt"
	"strings"
)

// What the linter still has an opinion about.
//
// It used to hold a house standard for a Brief: five sections, one H1, a required
// Definition of Done. That standard is gone (docs/decisions/0006) — the document's
// shape is its author's, and nothing here reads Context / Outcome / Symptom any
// more. What survives is about the two sections the REGION model owns, plus a few
// notes worth saying out loud.
//
// Rules that govern a STRUCTURE only fire when that structure is there, and even
// then all but one of them advise rather than refuse.

// Severity separates the rules that REFUSE a write from the ones that merely say
// something (docs/decisions/0003, docs/decisions/0005).
//
// Every rule here used to refuse. Now exactly ONE does: two headings with the same
// reserved name, which is not a matter of taste but of machinery — the second
// truncates the first, so the region ends up empty and every region-scoped write
// addresses nothing.
//
// Everything else warns. The framework does not own anybody's format
// (docs/decisions/0005): a page with two H1s, a Logbook with no checkbox, a Brief
// with no Definition of Done are all legible documents somebody meant to write, and
// a linter that refuses on shape is a linter people route around — or, worse, one
// that makes the tool unusable on the work that already exists.
type Severity string

const (
	SevRefuse Severity = "refuse"
	SevWarn   Severity = "warn"
)

// Finding is one violation of the standard.
type Finding struct {
	Rule string `json:"rule"`
	// Message says what to change, not merely what is wrong.
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	// N counts the offending things where a count is what changes — two H1s
	// becoming three is a NEW violation even though the rule already fired.
	N int `json:"n,omitempty"`
	// Severity is empty on findings written before it existed, which reads as
	// SevRefuse — the safe direction for an old rule, and every rule that predates
	// this field did refuse.
	Severity Severity `json:"severity,omitempty"`
}

// Refuses reports whether a finding should block the write.
func (f Finding) Refuses() bool { return f.Severity == "" || f.Severity == SevRefuse }

// Refusals filters findings down to the blocking ones.
func Refusals(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Refuses() {
			out = append(out, f)
		}
	}
	return out
}

func (f Finding) Error() string { return f.Message }

// Lint checks a thread page's markdown against the standard.
func Lint(body string) []Finding {
	var out []Finding
	lines := strings.Split(body, "\n")

	// Headings, in order, skipping fenced code — "# not a heading" inside a
	// shell example is a comment, and flagging it would train people to ignore
	// the linter.
	type head struct {
		level int
		line  int
	}
	var heads []head
	fenced := false
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if m := headingRe.FindStringSubmatch(ln); m != nil {
			heads = append(heads, head{level: len(m[1]), line: i + 1})
		}
	}

	var h1s []int
	for _, h := range heads {
		if h.level == 1 {
			h1s = append(h1s, h.line)
		}
	}
	if len(h1s) > 1 {
		out = append(out, Finding{
			Rule:     "one-h1",
			Severity: SevWarn,
			Line:     h1s[1],
			N:        len(h1s),
			Message: fmt.Sprintf(
				"%d level-1 headings (lines %s). A page reads best with one title and `## ` sections: "+
					"extra H1s are invisible to the table of contents, which starts at H2.",
				len(h1s), joinInts(h1s)),
		})
	}

	for i := 1; i < len(heads); i++ {
		if jump := heads[i].level - heads[i-1].level; jump > 1 {
			out = append(out, Finding{
				Rule:     "heading-skip",
				Severity: SevWarn,
				Line:     heads[i].line,
				Message: fmt.Sprintf(
					"line %d jumps from H%d to H%d. Headings step one level at a time, or the table of contents "+
						"renders a child with no parent.",
					heads[i].line, heads[i-1].level, heads[i].level),
			})
			break // one is enough to make the point; a cascade is noise
		}
	}

	// Structure rules, only where the structure exists.
	if logbook, _, hasLog := ExtractSection(body, "logbook"); hasLog {
		if len(ParseTodos(logbook)) == 0 {
			out = append(out, Finding{
				Rule:     "logbook-todo",
				Severity: SevWarn,
				Message: "the Logbook has no actionable todo. A `- [ ] ` item is what the next person " +
					"picks up — and ticking one is evidence of production, so a plan with none can only " +
					"warm this thread by being rewritten.",
			})
		}
		// The status line (docs/decisions/0003). `Next:` is what the operating loop
		// already demands and had nowhere to live, so its absence is worth saying —
		// and only saying.
		if !strings.Contains(strings.ToLower(logbook), "next:") {
			out = append(out, Finding{
				Rule:     "logbook-next",
				Severity: SevWarn,
				Message: "the Logbook does not say what happens next. A status line — " +
					"`**Owner:** … · **State:** … · **Next:** <one concrete action>` — is the first " +
					"thing anyone picking this up again reads.",
			})
		}
		if n := countHeadings(logbook); n > 5 {
			out = append(out, Finding{
				Rule: "logbook-phases", Severity: SevWarn, N: n,
				Message: fmt.Sprintf("the Logbook has %d phases. Past about five, a plan is usually "+
					"several threads wearing one Brief.", n),
			})
		}
	}
	out = append(out, duplicateSections(body)...)
	out = append(out, dodFindings(body)...)
	return out
}

// dodFindings are the notes worth making about a Definition of Done. All of them
// WARN: how somebody phrases their finish line is a matter of craft, and a linter
// that refuses on craft is one people learn to bypass.
//
// They fire on the DoD wherever it is, with no Brief required — since
// docs/decisions/0006 there is no Brief to require.
func dodFindings(body string) []Finding {
	dod, _, hasDoD := ExtractSection(body, "definition of done", "dod")
	if !hasDoD {
		return nil
	}
	var out []Finding
	items := ParseTodos(dod)
	switch {
	case len(items) == 0:
		out = append(out, Finding{
			Rule: "dod-checkboxes", Severity: SevWarn,
			Message: "the Definition of Done has no checkboxes. Its open items are what gets " +
				"reported when the thread finishes, so a DoD written as prose reports nothing.",
		})
	case len(items) > 10:
		out = append(out, Finding{
			Rule: "dod-size", Severity: SevWarn, N: len(items),
			Message: fmt.Sprintf("the Definition of Done has %d items. Past ten it is usually a plan, "+
				"which belongs in the Logbook — the DoD is the finish line.", len(items)),
		})
	}
	// Facts, not tasks. These items answer "is this finished?" for somebody who was
	// not there; a list of things TO DO answers a different question.
	for _, item := range items {
		if verb := taskVerb(item.Text); verb != "" {
			out = append(out, Finding{
				Rule: "dod-is-tasks", Severity: SevWarn,
				Message: fmt.Sprintf("a Definition of Done item starts with %q, which describes work rather "+
					"than a finish line. Each item should be verifiable by someone else without asking you: "+
					"not \"implement the export\" but \"`bubble admin export` round-trips a thread identically\".", verb),
			})
			break // one is enough to make the point
		}
	}
	return out
}

// taskVerbs are the openings that mean "work to do" rather than "true when done",
// in both languages this repository is written in.
var taskVerbs = []string{
	"implement", "add", "write", "create", "build", "fix", "make", "update", "refactor",
	"implementar", "agregar", "añadir", "escribir", "crear", "hacer", "arreglar", "actualizar",
}

func taskVerb(text string) string {
	first := strings.ToLower(strings.TrimSpace(text))
	if i := strings.IndexAny(first, " \t"); i > 0 {
		first = first[:i]
	}
	for _, v := range taskVerbs {
		if first == v {
			return v
		}
	}
	return ""
}

func countHeadings(section string) int {
	n := 0
	fenced := false
	for _, ln := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			fenced = !fenced
			continue
		}
		if !fenced && headingRe.MatchString(ln) {
			n++
		}
	}
	return n
}

// NewFindings reports the violations a write INTRODUCES.
//
// Refusing on the state of the whole page would block a one-line fix because
// somebody else left the page messy — punishing whoever touches a thread next
// rather than whoever broke it, and making the tool unusable on the work that
// already exists. A write is judged on what it did, not on what it inherited.
func NewFindings(before, after []Finding) []Finding {
	had := make(map[string]int, len(before))
	for _, f := range before {
		if f.N > had[f.Rule] {
			had[f.Rule] = f.N
		}
		if _, seen := had[f.Rule]; !seen {
			had[f.Rule] = 0
		}
	}
	var out []Finding
	for _, f := range after {
		prev, existed := had[f.Rule]
		switch {
		case !existed:
			out = append(out, f) // the write broke something that was fine
		case f.N > prev:
			out = append(out, f) // it was already broken, and this made it worse
		}
	}
	return out
}

// LintError turns findings into one refusal a caller can act on. Warnings are
// filtered out: they are reported alongside a write that succeeded, never as the
// reason one failed.
func LintError(findings []Finding) error {
	findings = Refusals(findings)
	if len(findings) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(findings))
	for _, f := range findings {
		msgs = append(msgs, f.Message)
	}
	return fmt.Errorf("this write does not meet the markdown standard: %s", strings.Join(msgs, " "))
}

func joinInts(ns []int) string {
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		parts = append(parts, fmt.Sprint(n))
	}
	return strings.Join(parts, ", ")
}

// reservedTitles are the section names the region model owns. Two of any of them
// on one page is not a style problem — it is a broken document.
var reservedTitles = map[string]string{
	"logbook":            "Logbook",
	"definition of done": "Definition of Done",
	"dod":                "Definition of Done",
}

// duplicateSections reports section names that appear more than once at top level.
//
// This is the failure that cost a real MCP session an afternoon: creating a thread
// wraps a caller's Logbook in its own `## Logbook`, so a caller that included the
// heading ended up with two. And a duplicate does something
// worse than look untidy — the FIRST section's range ends at the second heading, so
// the canonical region becomes EMPTY while the content sits in the document region.
// Every region-scoped operation then addresses nothing: toggle_todo answers "no todo
// at that position", exact edits cannot find text that is plainly visible in the
// rendered artifact, and the reader shows a Logbook that disagrees with the page.
//
// So this refuses. It is the one lint rule about structure that is not a matter of
// taste.
func duplicateSections(body string) []Finding {
	seen := map[string][]int{}
	fenced := false
	for i, ln := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		m := headingRe.FindStringSubmatch(ln)
		if m == nil || len(m[1]) > 2 { // H1/H2 only: a `### Fase 1` twice is fine
			continue
		}
		if canonical, ok := reservedTitles[strings.ToLower(strings.TrimSpace(m[2]))]; ok {
			seen[canonical] = append(seen[canonical], i+1)
		}
	}
	var out []Finding
	for _, canonical := range []string{"Logbook", "Definition of Done"} {
		lines := seen[canonical]
		if len(lines) < 2 {
			continue
		}
		out = append(out, Finding{
			Rule: "duplicate-section", Severity: SevRefuse, Line: lines[1], N: len(lines),
			Message: fmt.Sprintf(
				"%d %q headings (lines %s). A thread page has ONE of each: the second one "+
					"truncates the first, so the region ends up empty and every region-scoped "+
					"write addresses nothing. Keep one and merge the content.",
				len(lines), canonical, joinInts(lines)),
		})
	}
	return out
}

// StripLeadingHeading removes a leading heading whose title matches one of titles,
// so a caller may include `## Logbook` in the text they send for the Logbook without
// producing two of them. Anything below the heading is kept exactly as written.
//
// Only a LEADING heading, and only a matching one: a Logbook that mentions the word
// further down is left alone.
func StripLeadingHeading(markdown string, titles ...string) string {
	want := make(map[string]bool, len(titles))
	for _, t := range titles {
		want[strings.ToLower(strings.TrimSpace(t))] = true
	}
	lines := strings.Split(markdown, "\n")
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue // leading blank lines are not content
		}
		m := headingRe.FindStringSubmatch(ln)
		if m == nil || !want[strings.ToLower(strings.TrimSpace(m[2]))] {
			return markdown // the first real line is not a heading we own
		}
		return strings.TrimLeft(strings.Join(lines[i+1:], "\n"), "\n")
	}
	return markdown
}
