package md

import (
	"errors"
	"strings"
	"testing"
)

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

func TestToggleTodoForgivesReflowedText(t *testing.T) {
	if _, err := ToggleTodo("- [ ] ship   it", 0, "ship it", true); err != nil {
		t.Errorf("a reflow was treated as a different todo: %v", err)
	}
}

func TestToggleTodoKeepsIndentationAndMarker(t *testing.T) {
	got, err := ToggleTodo("* [ ] top\n  * [ ] nested", 1, "nested", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "* [ ] top\n  * [x] nested" {
		t.Errorf("toggle reformatted the list: %q", got)
	}
}

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

func TestCountDone(t *testing.T) {
	body := `# Brief
Do the thing.

## Logbook
- [x] scaffold
- [ ] wire it up
- [x] ship

## Definition of Done
- [x] tests pass
- [ ] reviewed
`
	if got := CountDone(body); got != 3 {
		t.Fatalf("want 3 ticked items, got %d", got)
	}
	// Somebody's own shape: no Logbook, no DoD, a checklist under their own heading.
	free := "# Importar CSV\n\n## Pasos\n- [x] leer el archivo\n- [ ] validar\n\n## Notas\n- [x] avisar al equipo\n"
	if got := CountDone(free); got != 2 {
		t.Fatalf("checkboxes outside the Logbook did not count: %d", got)
	}
	// Plain bullets are not checklist items, so they never count as done.
	if got := CountDone("## Logbook\n- just a note\n- another"); got != 0 {
		t.Fatalf("bullets counted as done: %d", got)
	}
	// A document with nothing ticked has produced nothing.
	if got := CountDone("# Brief\nNo checklist here."); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

// The claim: only a real `- [ ]` box is a task. v0 fell back to treating any
// plain bullet as one when a section had no boxes — its Logbook convention
// leaking into the engine. A prose bullet is a sentence.
func TestInvariant_MD_OnlyRealBoxesAreTasks(t *testing.T) {
	doc := "# Notas\n\n- una idea suelta\n- otra idea\n"
	if got := ParseChecklist(doc); len(got) != 0 {
		t.Errorf("plain bullets parsed as %d todos, want 0: %+v", len(got), got)
	}
	if _, err := ToggleTodo(doc, 0, "", true); !errors.Is(err, ErrNoSuchTodo) {
		t.Errorf("ticking a plain bullet gave %v, want ErrNoSuchTodo", err)
	}
	mixed := doc + "\n- [ ] esto sí\n"
	if got := ParseChecklist(mixed); len(got) != 1 || got[0].Text != "esto sí" {
		t.Errorf("got %+v, want exactly the one real box", got)
	}
}

// Checkboxes count ANYWHERE, not only under a blessed heading.
func TestMD_BoxesCountAnywhere(t *testing.T) {
	doc := "- [x] arriba de todo\n\n## Lo que sea\n\n- [ ] a\n- [x] b\n\n### Hondo\n\n- [x] c\n"
	if n := CountDone(doc); n != 3 {
		t.Errorf("CountDone = %d, want 3", n)
	}
	if n := len(ParseChecklist(doc)); n != 4 {
		t.Errorf("ParseChecklist found %d, want 4", n)
	}
}

func TestMD_RenderHTML(t *testing.T) {
	out := RenderHTML("# T\n\n- [ ] a\n\n```go\nx := 1\n```\n")
	for _, want := range []string{"<h1", "checkbox", "<code"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered HTML has no %q:\n%s", want, out)
		}
	}
}

func TestWindow(t *testing.T) {
	doc := "uno\ndos\ntres\ncuatro\ncinco\n"

	// Lo normal: un trozo, y la cuenta del ORIGINAL para saber que falta algo.
	got, first, last, total := Window(doc, 2, 2)
	if got != "dos\ntres" || first != 2 || last != 3 || total != 5 {
		t.Errorf("Window(2,2) = %q [%d-%d de %d]", got, first, last, total)
	}

	// El salto final no es una línea: contarla haría que todo documento midiera
	// uno de más, y «5 de 6» es una cuenta que nadie puede comprobar.
	if _, _, _, n := Window(doc, 1, 0); n != 5 {
		t.Errorf("total = %d, quería 5 — el salto final no es una línea", n)
	}

	// Pedir más de lo que hay devuelve lo que hay, no un error: es la pregunta
	// razonable de quien no sabía cuánto medía.
	got, first, last, _ = Window(doc, 4, 99)
	if got != "cuatro\ncinco" || first != 4 || last != 5 {
		t.Errorf("Window(4,99) = %q [%d-%d]", got, first, last)
	}
	if _, f, _, _ := Window(doc, 900, 3); f != 5 {
		t.Errorf("una línea más allá del final se ajusta al final, y dio %d", f)
	}
	if _, f, l, _ := Window(doc, -3, 2); f != 1 || l != 2 {
		t.Errorf("desde antes del principio empieza en 1, y dio %d-%d", f, l)
	}

	// Un documento vacío no tiene líneas, y decir «1 de 1» sería inventarse una.
	if _, _, _, n := Window("", 1, 10); n != 0 {
		t.Errorf("el vacío mide %d líneas", n)
	}
}
