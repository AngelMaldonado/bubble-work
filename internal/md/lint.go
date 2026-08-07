package md

import (
	"fmt"
	"strings"
)

// The house markdown standard (docs/bubble-work-spec.md §3.1, §3.2).
//
// §3.1 gives the shape of a Brief:
//
//	# Brief: <thread name>
//	## Problem / opportunity
//	## Intended outcome
//	## Context & constraints
//	## Definition of Done      <- required
//	## Links to source material
//
// One H1, everything else H2. The code already assumes it — birth writes
// <h2>Brief</h2>, ReplaceSection appends with "## ", and toc() skips H1
// outright because "H1s are artifact titles, not TOC entries". So a page with
// several H1s does not merely look wrong: those sections cannot appear in the
// TOC or the minimap at all.
//
// Rules that govern a STRUCTURE only fire when that structure is there. Most
// real threads are free-form documents with no Brief and no Logbook, and
// demanding a Definition of Done from a page that never claimed to be a Brief
// would make the tool unusable on the work that already exists.

// Finding is one violation of the standard.
type Finding struct {
	Rule string `json:"rule"`
	// Message says what to change, not merely what is wrong.
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	// N counts the offending things where a count is what changes — two H1s
	// becoming three is a NEW violation even though the rule already fired.
	N int `json:"n,omitempty"`
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
			Rule: "one-h1",
			Line: h1s[1],
			N:    len(h1s),
			Message: fmt.Sprintf(
				"%d level-1 headings (lines %s). A thread page has one title; its sections are `## ` (spec §3.1). "+
					"Extra H1s are also invisible to the table of contents, which starts at H2.",
				len(h1s), joinInts(h1s)),
		})
	}

	for i := 1; i < len(heads); i++ {
		if jump := heads[i].level - heads[i-1].level; jump > 1 {
			out = append(out, Finding{
				Rule: "heading-skip",
				Line: heads[i].line,
				Message: fmt.Sprintf(
					"line %d jumps from H%d to H%d. Headings step one level at a time, or the table of contents "+
						"renders a child with no parent.",
					heads[i].line, heads[i-1].level, heads[i].level),
			})
			break // one is enough to make the point; a cascade is noise
		}
	}

	// Structure rules, only where the structure exists.
	if _, _, hasBrief := ExtractSection(body, "brief"); hasBrief {
		// Anywhere on the page: §3.1 nests the Definition of Done INSIDE the
		// Brief, so looking only outside it would flag the very shape the
		// template prescribes.
		if _, _, hasDoD := ExtractSection(body, "definition of done", "dod"); !hasDoD {
			out = append(out, Finding{
				Rule: "dod-required",
				Message: "this page has a Brief but no Definition of Done. §3.1 marks it required — " +
					"a thread with no agreed finish line cannot be finished. Add a `## Definition of Done` section.",
			})
		}
	}
	if logbook, _, hasLog := ExtractSection(body, "logbook"); hasLog {
		if len(ParseTodos(logbook)) == 0 {
			out = append(out, Finding{
				Rule: "logbook-todo",
				Message: "the Logbook has no actionable todo. §3.2: a seeded logbook carries at least one, " +
					"plus a current owner and state. Add a `- [ ] ` item.",
			})
		}
	}
	return out
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

// LintError turns findings into one refusal a caller can act on.
func LintError(findings []Finding) error {
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
