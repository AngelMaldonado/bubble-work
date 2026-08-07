package md

import (
	"strings"
	"testing"
)

// Plane's node vocabulary (docs/ARTIFACT-EDITING.md Phase 2).

// A mention carries no text of its own, so before Phase 2 it rendered to nothing
// and any write deleted it — 48 of them across the live corpus.
func TestMentionSurvivesAWrite(t *testing.T) {
	body := `<p class="editor-paragraph-block"><mention-component id="node-1" entity_identifier="a9bb2e5c-9109-4790-b931-40f1d2ea6ae6" entity_name="user_mention"></mention-component> owns this.</p>`

	got := FromHTML(body)
	if !strings.Contains(got, MentionScheme+"a9bb2e5c-9109-4790-b931-40f1d2ea6ae6") {
		t.Fatalf("mention did not reach markdown: %q", got)
	}

	written := RenderPlaneHTML(got)
	if !strings.Contains(written, `<mention-component`) {
		t.Errorf("mention did not survive the write: %s", written)
	}
	if !strings.Contains(written, `entity_identifier="a9bb2e5c-9109-4790-b931-40f1d2ea6ae6"`) {
		t.Errorf("mention lost the person it points at: %s", written)
	}
	if back := FromHTML(written); back != got {
		t.Errorf("mention does not round-trip:\n  want %q\n  got  %q", got, back)
	}
}

// An image became <img src="plane-asset:…">, which Plane cannot render — so a
// write broke every one of the 193 in the corpus.
func TestImageSurvivesAWrite(t *testing.T) {
	body := `<image-component data-id="i1" src="7fce8f23-4354-4e9a-9313-415d5e4c9ea3" width="269px"></image-component>`

	got := FromHTML(body)
	if got != "![]("+AssetScheme+"7fce8f23-4354-4e9a-9313-415d5e4c9ea3)" {
		t.Fatalf("image markdown: %q", got)
	}
	written := RenderPlaneHTML(got)
	if !strings.Contains(written, `<image-component src="7fce8f23-4354-4e9a-9313-415d5e4c9ea3">`) {
		t.Errorf("image was not written back as a Plane node: %s", written)
	}
	if strings.Contains(written, "<img") {
		t.Errorf("image was written as an <img> Plane cannot render: %s", written)
	}
	if back := FromHTML(written); back != got {
		t.Errorf("image does not round-trip:\n  want %q\n  got  %q", got, back)
	}
}

// A real external URL is an ordinary image and must stay one.
func TestExternalImageIsNotRewritten(t *testing.T) {
	written := RenderPlaneHTML("![alt](https://example.com/x.png)")
	if !strings.Contains(written, `<img`) || strings.Contains(written, "image-component") {
		t.Errorf("external image was rewritten: %s", written)
	}
}

// The Logbook is where heat comes from. goldmark writes "- [x]" as a bare
// <input> inside a plain <ul>, which is not the shape Plane's editor produces —
// and a checkbox Plane drops is a todo nobody can tick.
func TestTaskListIsWrittenInPlanesShape(t *testing.T) {
	written := RenderPlaneHTML("- [x] shipped it\n- [ ] still open")

	for _, must := range []string{
		`data-type="taskList"`,
		`data-type="taskItem"`,
		`data-checked="true"`,
		`data-checked="false"`,
		`<label>`,
		`<input type="checkbox" checked="checked"/>`,
	} {
		if !strings.Contains(written, must) {
			t.Errorf("missing %s in:\n%s", must, written)
		}
	}
	// And it still reads back as the same todos, with their state.
	back := FromHTML(written)
	if back != "- [x] shipped it\n- [ ] still open" {
		t.Errorf("task list does not round-trip: %q", back)
	}
	if n := CountDone("## Logbook\n\n" + back); n != 1 {
		t.Errorf("CountDone after a write: want 1, got %d", n)
	}
}

// goldmark separates a checkbox from its text with one space, which the Plane
// shape has to drop. Trimming every text node instead of just that one ate the
// space after inline markup: "**CREANDO** NO" became "**CREANDO**NO".
func TestTaskItemKeepsSpacesAfterInlineMarkup(t *testing.T) {
	src := "- [x] SE VAN **CREANDO** NO CONFORME SE VAN ACEPTANDO"
	if back := FromHTML(RenderPlaneHTML(src)); back != src {
		t.Errorf("task item lost a space:\n  want %q\n  got  %q", src, back)
	}
}

func TestNestedTaskListSurvives(t *testing.T) {
	src := "- [x] parent\n  - [ ] child"
	back := FromHTML(RenderPlaneHTML(src))
	if back != src {
		t.Errorf("nested task list:\n  want %q\n  got  %q", back, src)
	}
}

// A plain bullet list must NOT become a task list — that would invent todos, and
// ParseTodos already treats bare bullets as open items.
func TestPlainListIsNotTurnedIntoATaskList(t *testing.T) {
	written := RenderPlaneHTML("- one\n- two")
	if strings.Contains(written, "taskList") || strings.Contains(written, "checkbox") {
		t.Errorf("a plain list grew checkboxes: %s", written)
	}
}

// Everything the write renderer touches has to survive splicing too, since a
// spliced block is rendered with exactly this path.
func TestSplicedBlockKeepsPlaneNodes(t *testing.T) {
	body := `<p data-id="a">Intro.</p>` +
		`<p data-id="b"><mention-component entity_identifier="u1" entity_name="user_mention"></mention-component> please look.</p>`
	current, _ := RegionMarkdown(body, RegionDocument)
	edited := strings.Replace(current, "Intro.", "Introduction.", 1)

	out, err := Splice(body, RegionDocument, edited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Introduction.") {
		t.Fatal("the edit did not land")
	}
	// Untouched, so it keeps its original bytes.
	if !strings.Contains(out, `<p data-id="b"><mention-component entity_identifier="u1" entity_name="user_mention"></mention-component> please look.</p>`) {
		t.Errorf("mention block was not reused verbatim: %s", out)
	}
}
