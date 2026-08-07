package md

import (
	"strings"
	"testing"
)

// Round-trip fidelity (docs/ARTIFACT-EDITING.md Phase 0).
//
// These bodies are synthetic on purpose: the real corpus is client work and does
// not belong in the repo. What they DO reproduce is the element census measured
// across it — p, span, div, taskList/taskItem, image-component, mention-component,
// blockquote, table, ol, ul, code, s, br and h1..h6 — including the ProseMirror
// wrapper shapes (class="editor-paragraph-block", data-id) Plane actually emits.

var corpus = map[string]string{
	"headings": `<h1 class="editor-heading-block" data-id="a">Title</h1>` +
		`<h2 class="editor-heading-block" data-id="b">Section</h2>` +
		`<h3>Sub</h3><h4>Deeper</h4><h5>Deeper still</h5><h6>Deepest</h6>` +
		`<p class="editor-paragraph-block" data-id="c">Prose under the heading.</p>`,

	"inline marks": `<p>Plain, <strong>bold</strong>, <em>italic</em>, <s>struck</s>, ` +
		`<code>literal</code> and <a href="https://example.com/x">a link</a>.</p>`,

	// The defect Phase 0 fixes: RenderHTML writes "<br>\n", and the source newline
	// after it used to come back as a leading space on the next line.
	"hard breaks": `<p class="editor-paragraph-block">line one<br>line two<br>line three</p>`,

	"bullets": `<ul><li><p>first</p></li><li><p>second</p></li></ul>`,

	"nested bullets": `<ul><li><p>parent</p><ul><li><p>child</p></li></ul></li><li><p>sibling</p></li></ul>`,

	// The other Phase 0 defect: a nested list under "3. " needs three columns of
	// indent, not two, or GFM flattens it into the parent and renumbers.
	"nested ordered": `<ol><li><p>one</p></li><li><p>two</p>` +
		`<ol><li><p>two point one</p></li><li><p>two point two</p></li></ol></li>` +
		`<li><p>three</p></li></ol>`,

	"ordered starting late": `<ol start="3"><li><p>third</p></li><li><p>fourth</p></li></ol>`,

	"task list, plane shape": `<ul data-type="taskList">` +
		`<li class="relative" data-id="1" data-checked="true" data-type="taskItem">` +
		`<label><input type="checkbox" checked="checked"><span></span></label>` +
		`<div><p class="editor-paragraph-block">shipped the sync worker</p></div></li>` +
		`<li class="relative" data-id="2" data-checked="false" data-type="taskItem">` +
		`<label><input type="checkbox"><span></span></label>` +
		`<div><p class="editor-paragraph-block">still open</p></div></li></ul>`,

	"blockquote": `<blockquote data-id="q"><p class="editor-paragraph-block">quoted line</p>` +
		`<p class="editor-paragraph-block">second quoted line</p></blockquote>`,

	"table": `<table><tbody><tr><th>Area</th><th>Verdict</th></tr>` +
		`<tr><td>Headings</td><td>fine</td></tr><tr><td>Lists</td><td>fine</td></tr></tbody></table>`,

	"code block": "<pre><code class=\"language-go\">x := 1\ny := 2\n</code></pre>",

	"mermaid": "<pre><code class=\"language-mermaid\">graph TD\nA-->B\n</code></pre>",

	"logbook page": `<h2>Brief</h2><p>The problem, stated once.</p>` +
		`<h2>Logbook</h2><ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="true"><div><p>phase one</p></div></li>` +
		`<li data-type="taskItem" data-checked="false"><div><p>phase two</p></div></li></ul>` +
		`<h2>Definition of Done</h2><ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="false"><div><p>it works</p></div></li></ul>`,

	"mixed document": `<h1>Report</h1><p>Intro with <strong>emphasis</strong>.</p>` +
		`<ul><li><p>a point</p></li></ul><blockquote><p>an aside</p></blockquote>` +
		`<h2>Detail</h2><ol><li><p>step</p></li></ol><hr/>`,
}

// TestRoundTripCorpus is the Phase 0 gate: reading a body and writing it back
// must be a no-op. Anything that drifts here is content a save silently rewrites.
func TestRoundTripCorpus(t *testing.T) {
	for name, body := range corpus {
		t.Run(name, func(t *testing.T) {
			f := Check(body)
			if !f.Stable {
				t.Errorf("round trip is not stable at line %d\n  was: %q\n  now: %q",
					f.Line, f.Before, f.After)
			}
		})
	}
}

// The two defects Phase 0 fixes, pinned individually so a regression names itself
// rather than showing up as a corpus entry that drifted.
func TestHardBreakSurvivesRoundTrip(t *testing.T) {
	const src = "line one  \nline two  \nline three"
	if got := FromHTML(RenderHTML(src)); got != src {
		t.Errorf("hard breaks gained whitespace:\n  want %q\n  got  %q", src, got)
	}
}

func TestNestedOrderedListKeepsItsNumbering(t *testing.T) {
	html := corpus["nested ordered"]
	md := FromHTML(html)
	// The child list must be indented to the parent's content column (three
	// spaces under "2. "), which is what keeps GFM from flattening it.
	if !strings.Contains(md, "\n   1. two point one") {
		t.Errorf("nested ordered list is not indented to the content column:\n%s", md)
	}
	if !strings.Contains(md, "\n3. three") {
		t.Errorf("parent numbering did not survive:\n%s", md)
	}
}

func TestOrderedListStartIsHonoured(t *testing.T) {
	if got := FromHTML(`<ol start="3"><li>third</li><li>fourth</li></ol>`); got != "3. third\n4. fourth" {
		t.Errorf("ol start ignored: got %q", got)
	}
}

// Check reports the nodes a write would DESTROY, measured rather than assumed —
// a body can round-trip "stably" and still lose every mention in it, because
// FromHTML used to drop them before the comparison ever happened. Since Phase 2
// taught the bridge Plane's vocabulary, the answer must be none.
func TestCheckReportsNoNodesDestroyed(t *testing.T) {
	body := `<p><mention-component id="m1" entity_identifier="u1" entity_name="user_mention"></mention-component> hi</p>` +
		`<p><image-component src="asset-1"></image-component></p>` +
		`<p><image-component src="asset-2"></image-component></p>`
	f := Check(body)
	if f.Mentions != 0 {
		t.Errorf("a write would delete %d mention(s)", f.Mentions)
	}
	if f.Assets != 0 {
		t.Errorf("a write would break %d image(s)", f.Assets)
	}
	if !f.Stable {
		t.Errorf("mentions/images do not round-trip: line %d\n  was %q\n  now %q",
			f.Line, f.Before, f.After)
	}
}
