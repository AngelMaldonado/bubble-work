package md

import (
	"strings"
	"testing"
)

// The house standard (docs/bubble-work-spec.md §3.1, §3.2).
func TestLint(t *testing.T) {
	// The reported case: a teammate wrote several level-1 headings.
	f := Lint("# One\n\nprose\n\n# Two\n\nmore")
	if len(f) != 1 || f[0].Rule != "one-h1" {
		t.Fatalf("multiple H1s not caught: %+v", f)
	}
	if f[0].N != 2 || !strings.Contains(f[0].Message, "§3.1") {
		t.Errorf("the finding should count them and cite the standard: %+v", f[0])
	}

	// One H1 is the template, not a violation.
	if f := Lint("# Brief: thing\n\n## Problem\n\nprose"); len(f) != 0 {
		t.Errorf("the §3.1 shape was flagged: %+v", f)
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
// free-form documents that never claimed to be a Brief.
func TestLintOnlyDemandsStructureThatIsClaimed(t *testing.T) {
	if f := Lint("just some notes\n\n## Findings\n\nprose"); len(f) != 0 {
		t.Errorf("a free-form document was held to the Brief template: %+v", f)
	}

	// §3.1 nests the Definition of Done INSIDE the Brief, so the template's own
	// shape must not be flagged.
	if f := Lint("## Brief\n\nwhy\n\n### Definition of Done\n\n- [ ] it works"); len(f) != 0 {
		t.Errorf("a DoD nested inside the Brief was not seen: %+v", f)
	}

	f := Lint("## Brief\n\nwhy this exists")
	if len(f) != 1 || f[0].Rule != "dod-required" {
		t.Errorf("a Brief without a Definition of Done: %+v", f)
	}

	f = Lint("## Logbook\n\njust prose, no todos")
	if len(f) != 1 || f[0].Rule != "logbook-todo" {
		t.Errorf("a Logbook without a todo: %+v", f)
	}
	if f := Lint("## Logbook\n\n- [ ] do the thing"); len(f) != 0 {
		t.Errorf("a Logbook with a todo was flagged: %+v", f)
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
