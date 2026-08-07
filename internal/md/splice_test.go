package md

import (
	"strings"
	"testing"
)

// Splicing (docs/ARTIFACT-EDITING.md Phase 1). The property that matters is not
// "the output is valid HTML" but "the bytes I did not edit are the bytes that
// were there" — so most of these assert on identity, not on shape.

// A Plane body, in the shape Plane actually emits: no wrapper, no whitespace
// between blocks, ProseMirror attributes on everything, and the two constructs
// the markdown bridge cannot yet carry.
const planeBody = `<h1 class="editor-heading-block" data-id="h1">Rework the intake form</h1>` +
	`<p class="editor-paragraph-block" data-id="p1">The current form loses half its submissions.</p>` +
	`<p class="editor-paragraph-block" data-id="p2"><mention-component id="m1" entity_identifier="u1" entity_name="user_mention"></mention-component> owns this.</p>` +
	`<image-component data-id="i1" src="7fce8f23-4354-4e9a-9313-415d5e4c9ea3" width="269px"></image-component>` +
	`<h2 class="editor-heading-block" data-id="h2">Logbook</h2>` +
	`<ul class="not-prose" data-id="ul1" data-type="taskList">` +
	`<li data-id="l1" data-checked="true" data-type="taskItem"><label><input type="checkbox" checked="checked"><span></span></label><div><p class="editor-paragraph-block">measure the drop-off</p></div></li>` +
	`<li data-id="l2" data-checked="false" data-type="taskItem"><label><input type="checkbox"><span></span></label><div><p class="editor-paragraph-block">rewrite the validation</p></div></li>` +
	`</ul>` +
	`<h2 class="editor-heading-block" data-id="h3">Definition of Done</h2>` +
	`<p class="editor-paragraph-block" data-id="p3">Submissions stop being lost.</p>`

func TestBlocksTileTheBody(t *testing.T) {
	bs := Blocks(planeBody)
	if len(bs) != 8 {
		t.Fatalf("want 8 top-level blocks, got %d", len(bs))
	}
	// Every byte must belong to a block or to whitespace between blocks: a
	// splice that silently dropped the gaps would corrupt the document.
	prev := 0
	var rebuilt strings.Builder
	for _, b := range bs {
		if gap := planeBody[prev:b.Start]; strings.TrimSpace(gap) != "" {
			t.Fatalf("content outside a block at %d: %q", prev, gap)
		}
		rebuilt.WriteString(planeBody[b.Start:b.End])
		prev = b.End
	}
	if strings.TrimSpace(planeBody[prev:]) != "" {
		t.Fatalf("trailing content outside a block: %q", planeBody[prev:])
	}
	if rebuilt.String() != planeBody {
		t.Error("blocks do not reassemble into the original body")
	}
}

func TestRegionMarkdownExcludesItsHeading(t *testing.T) {
	got, ok := RegionMarkdown(planeBody, RegionLogbook)
	if !ok {
		t.Fatal("logbook not found")
	}
	want := "- [x] measure the drop-off\n- [ ] rewrite the validation"
	if got != want {
		t.Errorf("logbook markdown:\n  want %q\n  got  %q", want, got)
	}
	if strings.Contains(got, "Logbook") {
		t.Error("the heading leaked into the editable content")
	}

	// The document region is everything before the first special section.
	doc, ok := RegionMarkdown(planeBody, RegionDocument)
	if !ok {
		t.Fatal("document not found")
	}
	if strings.Contains(doc, "measure the drop-off") {
		t.Error("the logbook leaked into the document region")
	}
	if !strings.Contains(doc, "Rework the intake form") {
		t.Error("the document region lost its heading")
	}

	if dod, ok := RegionMarkdown(planeBody, RegionDoD); !ok || dod != "Submissions stop being lost." {
		t.Errorf("dod: ok=%v got %q", ok, dod)
	}
}

// Saving a region you did not change must not touch Plane's bytes at all —
// otherwise autosave would rewrite the body on focus and blur, generating heat
// for work nobody did.
func TestSplicingUnchangedMarkdownIsAByteLevelNoop(t *testing.T) {
	for _, region := range []Region{RegionDocument, RegionLogbook, RegionDoD} {
		current, ok := RegionMarkdown(planeBody, region)
		if !ok {
			t.Fatalf("%s not found", region)
		}
		out, err := Splice(planeBody, region, current)
		if err != nil {
			t.Fatalf("%s: %v", region, err)
		}
		if out != planeBody {
			t.Errorf("%s: splicing back the same markdown changed the body", region)
		}
	}
}

// The whole reason for splicing: editing the Logbook must not disturb a mention
// or an image sitting in the Brief, even though the markdown bridge cannot
// represent either of them.
func TestEditingOneRegionLeavesTheRestByteIdentical(t *testing.T) {
	out, err := Splice(planeBody, RegionLogbook,
		"- [x] measure the drop-off\n- [x] rewrite the validation\n- [ ] ship it")
	if err != nil {
		t.Fatal(err)
	}
	for _, must := range []string{
		`<mention-component id="m1" entity_identifier="u1" entity_name="user_mention"></mention-component>`,
		`<image-component data-id="i1" src="7fce8f23-4354-4e9a-9313-415d5e4c9ea3" width="269px"></image-component>`,
		`<h1 class="editor-heading-block" data-id="h1">Rework the intake form</h1>`,
		`<p class="editor-paragraph-block" data-id="p1">The current form loses half its submissions.</p>`,
		`<h2 class="editor-heading-block" data-id="h3">Definition of Done</h2>`,
		`<p class="editor-paragraph-block" data-id="p3">Submissions stop being lost.</p>`,
	} {
		if !strings.Contains(out, must) {
			t.Errorf("a Logbook edit disturbed bytes it had no business touching:\n  lost %s", must)
		}
	}
	if !strings.Contains(out, "ship it") {
		t.Error("the edit itself did not land")
	}
	// And the Logbook now reads back as what was submitted.
	got, _ := RegionMarkdown(out, RegionLogbook)
	if got != "- [x] measure the drop-off\n- [x] rewrite the validation\n- [ ] ship it" {
		t.Errorf("logbook after edit: %q", got)
	}
}

// An untouched block keeps its ORIGINAL bytes even when its neighbours change —
// this is what caps the blast radius at "the blocks you typed in".
func TestUntouchedBlocksAreReusedNotRerendered(t *testing.T) {
	doc, _ := RegionMarkdown(planeBody, RegionDocument)
	edited := strings.Replace(doc,
		"The current form loses half its submissions.",
		"The current form loses MOST of its submissions.", 1)
	if edited == doc {
		t.Fatal("test setup: nothing was edited")
	}
	out, err := Splice(planeBody, RegionDocument, edited)
	if err != nil {
		t.Fatal(err)
	}
	// The edited paragraph is re-rendered and so loses its ProseMirror attrs...
	if !strings.Contains(out, "MOST of its submissions") {
		t.Fatal("the edit did not land")
	}
	// ...but its neighbours in the same region keep theirs, verbatim.
	for _, must := range []string{
		`<h1 class="editor-heading-block" data-id="h1">Rework the intake form</h1>`,
		`<p class="editor-paragraph-block" data-id="p2"><mention-component id="m1" entity_identifier="u1" entity_name="user_mention"></mention-component> owns this.</p>`,
		`<image-component data-id="i1" src="7fce8f23-4354-4e9a-9313-415d5e4c9ea3" width="269px"></image-component>`,
	} {
		if !strings.Contains(out, must) {
			t.Errorf("neighbouring block was re-rendered instead of reused:\n  lost %s", must)
		}
	}
}

func TestSpliceInsertDeleteReorder(t *testing.T) {
	base, _ := RegionMarkdown(planeBody, RegionLogbook)

	// insert at the top — LCS must reuse both existing items rather than
	// re-rendering the list because everything shifted down.
	out, err := Splice(planeBody, RegionLogbook, "- [ ] scope it\n\n"+base)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := RegionMarkdown(out, RegionLogbook); !strings.HasPrefix(got, "- [ ] scope it") {
		t.Errorf("insert at top: %q", got)
	}

	// delete everything
	out, err = Splice(planeBody, RegionLogbook, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := RegionMarkdown(out, RegionLogbook); !ok || strings.TrimSpace(got) != "" {
		t.Errorf("delete all: ok=%v got %q", ok, got)
	}
	// The section heading survives an emptied Logbook, and so does the DoD.
	if !strings.Contains(out, ">Logbook</h2>") {
		t.Error("emptying the logbook removed its heading")
	}
	if !strings.Contains(out, "Submissions stop being lost.") {
		t.Error("emptying the logbook damaged the Definition of Done")
	}
}

// A thread born without a Logbook gains one instead of the update vanishing.
func TestSpliceCreatesAMissingSection(t *testing.T) {
	body := `<p class="editor-paragraph-block" data-id="p1">Just a brief, no plan yet.</p>`
	out, err := Splice(body, RegionLogbook, "- [ ] write the plan")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, body) {
		t.Error("creating a Logbook disturbed the existing body")
	}
	got, ok := RegionMarkdown(out, RegionLogbook)
	if !ok || got != "- [ ] write the plan" {
		t.Errorf("created logbook: ok=%v got %q", ok, got)
	}
}

// Content after the special sections is preserved even though this pass cannot
// edit it — the failure to avoid is silently swallowing it.
func TestContentAfterTheLogbookSurvives(t *testing.T) {
	body := planeBody + `<p class="editor-paragraph-block" data-id="tail">A postscript.</p>`
	out, err := Splice(body, RegionLogbook, "- [ ] only item")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<p class="editor-paragraph-block" data-id="tail">A postscript.</p>`) {
		t.Error("trailing content was swallowed by the splice")
	}
}

// Bodies we wrote ourselves may arrive wrapped in a single container; the
// wrapper must not make the whole document one unsplittable block.
func TestSpliceThroughAWrapperDiv(t *testing.T) {
	body := `<div><h2>Logbook</h2><ul><li>alpha</li><li>beta</li></ul></div>`
	bs := Blocks(body)
	if len(bs) != 2 {
		t.Fatalf("wrapper not unwrapped: got %d blocks", len(bs))
	}
	out, err := Splice(body, RegionLogbook, "- alpha\n- beta\n- gamma")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "<div><h2>Logbook</h2>") || !strings.HasSuffix(out, "</div>") {
		t.Errorf("wrapper was not preserved: %s", out)
	}
	if got, _ := RegionMarkdown(out, RegionLogbook); got != "- alpha\n- beta\n- gamma" {
		t.Errorf("got %q", got)
	}
}

func TestSpliceRejectsAnUnknownRegion(t *testing.T) {
	if _, err := Splice(planeBody, Region("brief"), "x"); err == nil {
		t.Error("want an error for an unknown region")
	}
}

// Plane leaves empty paragraphs behind as spacing. They carry no markdown, so
// the editor cannot show them and the submitted text cannot mention them — but
// they are the author's layout, and a diff that only saw content deleted every
// one of them on the first save. That was 37 of 96 real bodies.
func TestEmptySpacerParagraphsSurviveASave(t *testing.T) {
	body := `<p class="editor-paragraph-block" data-id="p1">First.</p>` +
		`<p class="editor-paragraph-block" data-id="gap"></p>` +
		`<p class="editor-paragraph-block" data-id="p2">Second.</p>`
	current, _ := RegionMarkdown(body, RegionDocument)
	if strings.Contains(current, "gap") {
		t.Fatal("test setup: the spacer should be invisible to the editor")
	}
	out, err := Splice(body, RegionDocument, current)
	if err != nil {
		t.Fatal(err)
	}
	if out != body {
		t.Errorf("an untouched save dropped the spacer:\n  want %s\n  got  %s", body, out)
	}
	// And it stays put when a neighbour is edited.
	out, err = Splice(body, RegionDocument, strings.Replace(current, "Second.", "Third.", 1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<p class="editor-paragraph-block" data-id="gap"></p>`) {
		t.Errorf("editing a neighbour deleted the spacer: %s", out)
	}
}

// The tokenizer reports "<img …>" as a start tag with no end tag ever following.
// Treated generically it opened a block that never closed and swallowed the rest
// of the document into it.
func TestVoidElementIsItsOwnBlock(t *testing.T) {
	body := `<p data-id="a">Before.</p><img data-id="i" src="https://example.com/x.png" alt="x"><p data-id="b">After.</p>`
	bs := Blocks(body)
	if len(bs) != 3 {
		t.Fatalf("want 3 blocks, got %d: %+v", len(bs), bs)
	}
	if bs[1].HTML != `<img data-id="i" src="https://example.com/x.png" alt="x">` {
		t.Errorf("img block: %q", bs[1].HTML)
	}
	current, _ := RegionMarkdown(body, RegionDocument)
	out, err := Splice(body, RegionDocument, current)
	if err != nil {
		t.Fatal(err)
	}
	if out != body {
		t.Errorf("void element broke the splice:\n  want %s\n  got  %s", body, out)
	}
}

// Two <br>s in a row — pressing enter twice in Plane's editor — render as
// "  \n  \n", whose middle line is two spaces. Splitting blocks on any blank-ish
// line tore that paragraph apart, so it matched nothing and was re-rendered on
// every save.
func TestDoubleBreakDoesNotSplitAParagraph(t *testing.T) {
	body := `<p class="editor-paragraph-block" data-id="p">one<br><br>two</p>`
	if got := len(Blocks(body)); got != 1 {
		t.Fatalf("want 1 block, got %d", got)
	}
	current, _ := RegionMarkdown(body, RegionDocument)
	if got := len(splitMarkdownBlocks(current)); got != 1 {
		t.Errorf("a double break split the paragraph into %d blocks: %q", got, current)
	}
	out, err := Splice(body, RegionDocument, current)
	if err != nil {
		t.Fatal(err)
	}
	if out != body {
		t.Errorf("double break broke the splice:\n  want %s\n  got  %s", body, out)
	}
}

func TestSplitMarkdownBlocksKeepsFencesWhole(t *testing.T) {
	got := splitMarkdownBlocks("intro\n\n```go\nx := 1\n\ny := 2\n```\n\nafter")
	if len(got) != 3 {
		t.Fatalf("want 3 blocks, got %d: %q", len(got), got)
	}
	if !strings.Contains(got[1], "y := 2") {
		t.Errorf("a blank line split a fenced block: %q", got[1])
	}
}
