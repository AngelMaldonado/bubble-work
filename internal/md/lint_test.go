package md

import (
	"strings"
	"testing"
)

// The house standard (README.md §3.1, §3.2).
func TestLint(t *testing.T) {
	// The reported case: a teammate wrote several level-1 headings.
	f := Lint("# One\n\nprose\n\n# Two\n\nmore")
	if len(f) != 1 || f[0].Rule != "one-h1" {
		t.Fatalf("multiple H1s not caught: %+v", f)
	}
	if f[0].N != 2 || !strings.Contains(f[0].Message, "table of contents") {
		t.Errorf("the finding should count them and say why it matters: %+v", f[0])
	}

	// One H1 with sections under it is the ordinary shape, not a violation.
	if f := Lint("# A thread\n\n## Problem\n\nprose"); len(f) != 0 {
		t.Errorf("an ordinary document was flagged: %+v", f)
	}
	// So is a page that starts at H2, which is what the Plane binding writes.
	if f := Lint("## Brief\n\nwhy\n\n## Definition of Done\n\n- [ ] done"); len(f) != 0 {
		t.Errorf("the Plane binding's own shape was flagged: %+v", f)
	}

	// A heading that skips a level renders a child with no parent in the TOC.
	if f := Lint("## A\n\n#### B"); len(f) != 1 || f[0].Rule != "heading-skip" {
		t.Errorf("skipped level not caught: %+v", f)
	}

	// "# not a heading" inside a fence is a shell comment. Flagging it would
	// train people to ignore the linter.
	if f := Lint("# Title\n\n```sh\n# install\n# build\n```"); len(f) != 0 {
		t.Errorf("comments inside a code fence were read as headings: %+v", f)
	}
}

// Structure rules fire only where the structure exists — most real threads are
// free-form documents that never claimed to be a Brief — and since
// docs/decisions/0005 they only ever WARN. The framework does not own anybody's
// format: the only refusal left is structural damage (a duplicated section
// heading), because that one breaks region-scoped writes rather than taste.
func TestLintOnlyDemandsStructureThatIsClaimed(t *testing.T) {
	if f := Refusals(Lint("just some notes\n\n## Findings\n\nprose")); len(f) != 0 {
		t.Errorf("a free-form document was held to the Brief template: %+v", f)
	}

	// A Definition of Done nested under somebody's own heading is still found.
	if f := Refusals(Lint("## Contexto\n\nwhy\n\n### Definition of Done\n\n- [ ] it works")); len(f) != 0 {
		t.Errorf("a nested DoD was mishandled: %+v", f)
	}

	// A document with no Definition of Done is not remarked on at all: since
	// docs/decisions/0006 nothing on the page claims one is owed.
	if f := Lint("## Lo que quiero\n\nque no truene"); len(f) != 0 {
		t.Errorf("somebody's own section was linted: %+v", f)
	}
	// A Logbook with no todo is said, and does not block.
	if f := Refusals(Lint("## Logbook\n\njust prose, no todos")); len(f) != 0 {
		t.Errorf("a Logbook without a todo was refused: %+v", f)
	}
	if !warns(Lint("## Logbook\n\njust prose, no todos"), "logbook-todo") {
		t.Errorf("a Logbook without a todo was not even mentioned")
	}
	if f := Lint("## Logbook\n\n- [ ] do the thing\n\n**Next:** ship it"); len(f) != 0 {
		t.Errorf("a Logbook with a todo was flagged: %+v", f)
	}

	// Two H1s are a warning now: they are invisible to the table of contents,
	// which is worth saying and not worth refusing a write over.
	if f := Refusals(Lint("# One\n\n# Two")); len(f) != 0 {
		t.Errorf("a page with two H1s was refused: %+v", f)
	}

	// The one refusal left: a duplicated reserved heading truncates the first
	// section, so every region-scoped write addresses nothing.
	dup := Refusals(Lint("## Logbook\n\n- [ ] a\n\n## Logbook\n\n- [ ] b"))
	if len(dup) != 1 || dup[0].Rule != "duplicate-section" {
		t.Errorf("a duplicated section heading is the one thing that must refuse: %+v", dup)
	}
}

func warns(fs []Finding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule && !f.Refuses() {
			return true
		}
	}
	return false
}

// The advisory rules (docs/decisions/0003) must SAY things without blocking
// anything: a linter that refuses on craft is one people learn to route around.
func TestInvariant_Artifacts_AdvisoryRulesNeverRefuse(t *testing.T) {
	// A Definition of Done written as a task list — the failure the DoD gate cares
	// about, since it reads these items to decide whether a thread may finish.
	body := "## Definition of Done\n\n- [ ] implement the export command\n\n" +
		"## Logbook\n\n- [ ] step one"
	f := Lint(body)
	var warned, refused []string
	for _, x := range f {
		if x.Refuses() {
			refused = append(refused, x.Rule)
		} else {
			warned = append(warned, x.Rule)
		}
	}
	if len(refused) != 0 {
		t.Errorf("advisory findings refused a write: %v", refused)
	}
	has := func(rule string) bool {
		for _, w := range warned {
			if w == rule {
				return true
			}
		}
		return false
	}
	if !has("dod-is-tasks") {
		t.Errorf("a DoD item opening with a task verb was not noticed: %v", warned)
	}
	if !has("logbook-next") {
		t.Errorf("a Logbook with no Next: was not noticed: %v", warned)
	}
	// And LintError speaks only for refusals, so warnings cannot fail a write.
	if err := LintError(f); err != nil {
		t.Errorf("warnings produced a refusal: %v", err)
	}
	// A DoD phrased as facts is not flagged.
	ok := "## Definition of Done\n\n- [ ] `bubble admin export` round-trips a thread\n\n" +
		"## Logbook\n\n**Next:** start\n\n- [ ] step one"
	for _, x := range Lint(ok) {
		if x.Rule == "dod-is-tasks" || x.Rule == "logbook-next" {
			t.Errorf("a well-formed page was flagged: %s — %s", x.Rule, x.Message)
		}
	}
}

// A write is judged on what it DID, not on what it inherited. Otherwise a
// one-line fix is refused because somebody else left the page messy.
func TestNewFindingsJudgesTheWriteNotThePage(t *testing.T) {
	messy := Lint("# One\n\n# Two")
	stillMessy := Lint("# One\n\n# Two\n\nan unrelated paragraph")
	if got := NewFindings(messy, stillMessy); len(got) != 0 {
		t.Errorf("an unrelated edit was blocked by a pre-existing violation: %+v", got)
	}

	// Making it worse IS this write's fault, even though the rule already fired.
	worse := Lint("# One\n\n# Two\n\n# Three")
	if got := NewFindings(messy, worse); len(got) != 1 || got[0].N != 3 {
		t.Errorf("adding a third H1 to a page with two was not caught: %+v", got)
	}

	// Breaking something that was fine is caught.
	clean := Lint("# One\n\n## Two")
	if got := NewFindings(clean, messy); len(got) != 1 || got[0].Rule != "one-h1" {
		t.Errorf("a write that introduced a second H1 was not caught: %+v", got)
	}

	// And fixing it is never refused.
	if got := NewFindings(messy, clean); len(got) != 0 {
		t.Errorf("a write that FIXED the page was refused: %+v", got)
	}
}
