package md

import "strings"

// Round-trip fidelity (docs/ARTIFACT-EDITING.md Phase 0).
//
// Bodies live in Plane as description_html. Reading one costs a FromHTML, and
// writing it back costs a RenderHTML — so anything the pair does not preserve is
// silently destroyed the moment a thread is edited, whether by a person in the
// web interior or by an agent through MCP.
//
// The honest way to know how much survives is to measure it against real bodies
// rather than reason about it, which is what Check exists for: `bubble sync
// fidelity` runs it over an instance's whole mirror and reports the numbers.

// Check reports what a read→write round trip would do to one body.
type Fidelity struct {
	// Stable is true when reading the body and writing it back is a no-op —
	// FromHTML(RenderHTML(FromHTML(html))) == FromHTML(html).
	Stable bool
	// Mentions counts <mention-component> nodes. FromHTML drops them today, so
	// every one of these is deleted by a write. Phase 2 drives this to zero.
	Mentions int
	// Assets counts <image-component> nodes. RenderHTML turns them into
	// <img src="plane-asset:…">, which Plane cannot render — so a write breaks
	// every one. Phase 2 drives this to zero too.
	Assets int
	// SpliceClean is true when every region of the body splices its own current
	// markdown back to the IDENTICAL BYTES. This is the gate that matters: it is
	// what makes saving an untouched region a no-op on Plane, and what keeps an
	// edit to one region from disturbing another.
	SpliceClean bool
	// Line, Before and After locate the first difference, empty when Stable.
	Line   int
	Before string
	After  string
}

// Check round-trips one description_html and reports what changed.
func Check(descriptionHTML string) Fidelity {
	f := Fidelity{
		Stable:      true,
		SpliceClean: spliceIsNoop(descriptionHTML),
		Mentions:    strings.Count(descriptionHTML, "<mention-component"),
		Assets:      strings.Count(descriptionHTML, "<image-component"),
	}
	before := FromHTML(descriptionHTML)
	after := FromHTML(RenderHTML(before))
	if before == after {
		return f
	}
	f.Stable = false

	// Report the first differing line rather than the whole document: these
	// bodies run to thousands of words and the useful signal is which construct
	// drifted, not how much prose surrounds it.
	a := strings.Split(before, "\n")
	b := strings.Split(after, "\n")
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := "", ""
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			f.Line, f.Before, f.After = i+1, clip(x), clip(y)
			break
		}
	}
	return f
}

// spliceIsNoop reports whether every region of a body, spliced back with the
// markdown it currently holds, returns the body unchanged byte for byte.
func spliceIsNoop(descriptionHTML string) bool {
	for _, r := range []Region{RegionDocument, RegionLogbook, RegionDoD} {
		current, found := RegionMarkdown(descriptionHTML, r)
		if !found {
			continue
		}
		out, err := Splice(descriptionHTML, r, current)
		if err != nil || out != descriptionHTML {
			return false
		}
	}
	return true
}

func clip(s string) string {
	const max = 160
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
