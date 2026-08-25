package md

import (
	"errors"
	"strings"
	"testing"
)

// Splicing (docs/journal/ARTIFACT-EDITING.md Phase 1). The property that matters is not
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

// An index alone is a question the caller cannot answer: the list may have been
// re-ordered since it was rendered. Ticking the wrong box is worse than
// refusing, because it is silent AND it manufactures evidence of production.
func TestToggleTodoRefusesWhenTheItemMoved(t *testing.T) {
	section := "- [ ] wire it up\n- [ ] ship it"

	if _, err := ToggleTodo(section, 1, "wire it up", true); !errors.Is(err, ErrTodoMoved) {
		t.Errorf("a mismatched text was accepted: %v", err)
	}
	if _, err := ToggleTodo(section, 7, "ship it", true); !errors.Is(err, ErrNoSuchTodo) {
		t.Errorf("an out-of-range index was accepted: %v", err)
	}

	got, err := ToggleTodo(section, 1, "ship it", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "- [ ] wire it up\n- [x] ship it" {
		t.Errorf("toggle: %q", got)
	}
	// Un-ticking is the same operation in reverse.
	back, err := ToggleTodo(got, 1, "ship it", false)
	if err != nil || back != section {
		t.Errorf("un-tick: %q (%v)", back, err)
	}
}

// Spacing is cosmetic, so a reflowed item is still the same item.
func TestToggleTodoForgivesReflowedText(t *testing.T) {
	if _, err := ToggleTodo("- [ ] ship   it", 0, "ship it", true); err != nil {
		t.Errorf("a reflow was treated as a different todo: %v", err)
	}
}

// Indentation and the bullet character are the author's, not ours.
func TestToggleTodoKeepsIndentationAndMarker(t *testing.T) {
	got, err := ToggleTodo("* [ ] top\n  * [ ] nested", 1, "nested", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "* [ ] top\n  * [x] nested" {
		t.Errorf("toggle reformatted the list: %q", got)
	}
}

// ParseTodos falls back to plain bullets when a section has no checkboxes, so
// the UI renders checkboxes for them. Ticking one has to work, or the fallback
// produces a control that refuses to do anything.
func TestToggleTodoUpgradesAPlainBullet(t *testing.T) {
	got, err := ToggleTodo("- just a bullet", 0, "just a bullet", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "- [x] just a bullet" {
		t.Errorf("toggle: %q", got)
	}
}

func TestHashChangesWithTheMarkdown(t *testing.T) {
	if Hash("a") == Hash("b") {
		t.Error("different markdown hashed the same")
	}
	if Hash("a") != Hash("a") {
		t.Error("hash is not stable")
	}
	// Exact, not normalised: this answers "did it change under me", where
	// LogbookFingerprint answers "did the plan change" and forgives a reflow.
	if Hash("- a") == Hash("-  a") {
		t.Error("hash forgave a whitespace change it should have caught")
	}
}

// Todos are not confined to the Logbook. 59 of 96 real bodies keep them in the
// document with no Logbook section at all, so a toggle that only understood
// logbook/dod rendered a checkbox that did nothing.
func TestToggleTodoInTheDocumentRegion(t *testing.T) {
	body := `<p data-id="p1">Findings:</p>` +
		`<ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="false"><div><p>login is slow</p></div></li>` +
		`<li data-type="taskItem" data-checked="false"><div><p>error text is in English</p></div></li>` +
		`</ul>`

	current, ok := RegionMarkdown(body, RegionDocument)
	if !ok {
		t.Fatal("document region not found")
	}
	next, err := ToggleTodo(current, 1, "error text is in English", true)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Splice(body, RegionDocument, next)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := RegionMarkdown(out, RegionDocument)
	if !strings.Contains(got, "- [x] error text is in English") {
		t.Errorf("the todo was not ticked: %q", got)
	}
	if !strings.Contains(got, "- [ ] login is slow") {
		t.Errorf("the wrong todo moved: %q", got)
	}
	// The prose above the list is untouched, verbatim.
	if !strings.Contains(out, `<p data-id="p1">Findings:</p>`) {
		t.Errorf("ticking a todo disturbed the paragraph above it:\n%s", out)
	}
}

// Replacing a whole region to change one line is how sections get paraphrased,
// truncated, or appended to twice. An edit says WHICH TEXT changes.
func TestApplyEdits(t *testing.T) {
	const src = "- [ ] measure the drop-off\n- [ ] rewrite the validation\n- [ ] ship it"

	got, err := ApplyEdits(src, []Edit{{Old: "- [ ] ship it", New: "- [x] ship it"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "- [ ] measure the drop-off\n- [ ] rewrite the validation\n- [x] ship it" {
		t.Errorf("one line changed, the rest should be untouched: %q", got)
	}

	// Edits see each other, so a rename then an edit of the renamed line works.
	got, err = ApplyEdits(src, []Edit{
		{Old: "ship it", New: "ship the thing"},
		{Old: "- [ ] ship the thing", New: "- [x] ship the thing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "- [x] ship the thing") {
		t.Errorf("edits do not compose: %q", got)
	}

	// An empty Old appends — the one case with nothing to match.
	got, err = ApplyEdits(src, []Edit{{New: "- [ ] and one more"}})
	if err != nil || !strings.HasSuffix(got, "\n\n- [ ] and one more") {
		t.Errorf("append: %q (%v)", got, err)
	}

	// An empty New deletes.
	got, err = ApplyEdits(src, []Edit{{Old: "\n- [ ] ship it", New: ""}})
	if err != nil || strings.Contains(got, "ship it") {
		t.Errorf("delete: %q (%v)", got, err)
	}
}

// The guards. Both refusals exist because the alternative is silently editing
// something the caller did not mean — the same reasoning as toggle_todo's text.
func TestApplyEditsRefusesRatherThanGuesses(t *testing.T) {
	const src = "- [ ] review the diff\n- [ ] review the docs"

	// Quoting something that is not there means the caller is looking at a stale
	// copy. Writing anything at that point would be a guess.
	_, err := ApplyEdits(src, []Edit{{Old: "- [ ] review the tests", New: "x"}})
	if !errors.Is(err, ErrEditNotFound) {
		t.Errorf("a missing match was accepted: %v", err)
	}

	// "review the" matches twice. Picking the first is a question the caller
	// cannot know it answered wrongly.
	_, err = ApplyEdits(src, []Edit{{Old: "review the", New: "re-review the"}})
	if !errors.Is(err, ErrEditAmbiguous) {
		t.Errorf("an ambiguous match was accepted: %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "2 times") {
		t.Errorf("the refusal should say how many it found: %v", err)
	}
	// ...unless the caller says it means all of them.
	got, err := ApplyEdits(src, []Edit{{Old: "review the", New: "re-review the", All: true}})
	if err != nil || strings.Count(got, "re-review the") != 2 {
		t.Errorf("All: %q (%v)", got, err)
	}

	// A failure part-way applies NOTHING: half a patch is worse than none,
	// because the caller cannot tell which half landed.
	_, err = ApplyEdits(src, []Edit{
		{Old: "review the diff", New: "review the patch"},
		{Old: "not in here at all", New: "x"},
	})
	if err == nil {
		t.Fatal("a set with a bad edit was accepted")
	}
	if !strings.Contains(err.Error(), "edit 2") {
		t.Errorf("the refusal should say WHICH edit failed: %v", err)
	}
}

// An edit still goes through the splice, so untouched blocks keep their bytes.
func TestEditThenSpliceKeepsUntouchedBytes(t *testing.T) {
	current, _ := RegionMarkdown(planeBody, RegionLogbook)
	next, err := ApplyEdits(current, []Edit{
		{Old: "- [ ] rewrite the validation", New: "- [x] rewrite the validation"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := Splice(planeBody, RegionLogbook, next)
	if err != nil {
		t.Fatal(err)
	}
	for _, must := range []string{
		`<mention-component id="m1" entity_identifier="u1" entity_name="user_mention"></mention-component>`,
		`<image-component data-id="i1" src="7fce8f23-4354-4e9a-9313-415d5e4c9ea3" width="269px"></image-component>`,
		`<p class="editor-paragraph-block" data-id="p3">Submissions stop being lost.</p>`,
	} {
		if !strings.Contains(out, must) {
			t.Errorf("a one-line edit disturbed the rest of the page:\n  lost %s", must)
		}
	}
	if got, _ := RegionMarkdown(out, RegionLogbook); !strings.Contains(got, "- [x] rewrite the validation") {
		t.Errorf("the edit did not land: %q", got)
	}
}

// A page whose author kept writing after the Definition of Done has content that
// belongs to no region: the document region stops at the first special heading, and
// a section region stops at the next heading of its level. Storing only the three
// regions would drop it (docs/decisions/0001), so it is captured as the tail and
// comes back in the assembled body.
func TestTailAndAssembleRoundTrip(t *testing.T) {
	html := `<h1>Ship it</h1><p>why</p>` +
		`<h2>Logbook</h2><ul><li>step</li></ul>` +
		`<h2>Definition of Done</h2><ul><li>done</li></ul>` +
		`<h2>Notes</h2><p>a thought after the DoD</p>`

	doc, ok := RegionMarkdown(html, RegionDocument)
	if !ok {
		t.Fatal("no document region")
	}
	lb, _ := RegionMarkdown(html, RegionLogbook)
	dod, _ := RegionMarkdown(html, RegionDoD)

	tail, ok := TailMarkdown(html)
	if !ok {
		t.Fatalf("the trailing section was not captured")
	}
	if !strings.Contains(tail, "a thought after the DoD") {
		t.Errorf("tail = %q", tail)
	}
	// Nothing is in two places: the tail must not repeat the DoD's own items.
	if strings.Contains(tail, "done") {
		t.Errorf("tail overlaps the DoD: %q", tail)
	}

	body := AssembleBody(doc, lb, dod, tail)
	for _, want := range []string{"Ship it", "why", "## Logbook", "step",
		"## Definition of Done", "done", "Notes", "a thought after the DoD"} {
		if !strings.Contains(body, want) {
			t.Errorf("assembled body lost %q:\n%s", want, body)
		}
	}
	// And the headings are back where ParseThread looks for them.
	arts, logbook := ParseThread(body, "Ship it")
	if logbook == nil {
		t.Fatal("assembled body has no logbook")
	}
	if len(logbook.DoD) != 1 || logbook.DoD[0].Text != "done" {
		t.Errorf("DoD did not survive assembly: %+v", logbook.DoD)
	}
	if len(arts) == 0 || !strings.Contains(arts[0].Markdown, "why") {
		t.Errorf("document did not survive assembly: %+v", arts)
	}

	// A page with no special sections has no tail — everything is the document.
	if _, ok := TailMarkdown(`<h1>Plain</h1><p>just prose</p>`); ok {
		t.Error("a page with no sections should have no tail")
	}
}
