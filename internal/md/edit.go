// Package md is the markdown engine: rendering, checkboxes and surgical edits.
//
// What it deliberately does NOT do is have opinions about how a document is
// shaped. There is no linter, no template, no required section — how work gets
// written down is the writer's to decide, and every team thinks about its
// artifacts differently. Guidance belongs in whatever a team writes for itself,
// never in this package.
//
// Everything here speaks MARKDOWN. v0's other half spoke Plane's ProseMirror HTML
// — regions spliced into a `description_html`, mention and image components,
// fidelity checks between the two — and all of it went with Plane.
package md

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// taskRe is a real markdown checkbox, and the only thing this package treats as
// a task.
var taskRe = regexp.MustCompile(`^\s*[-*]\s+\[([ xX])\]\s+(.*)$`)

// Edit replaces one exact run of text within a region.

type Edit struct {
	// Old must appear EXACTLY once, unless All. Empty means append.
	Old string
	New string
	// All replaces every occurrence, for a caller that means it.
	All bool
}

// ApplyEdits runs edits against a region's markdown, in order.
//
// In order matters: a later edit sees the result of an earlier one, so a caller
// can rename something and then edit the renamed line. It also means a failure
// part-way leaves NOTHING applied — the whole set is rejected, because half a
// patch is worse than none.
func ApplyEdits(markdown string, edits []Edit) (string, error) {
	out := markdown
	for i, e := range edits {
		// An empty Old is an append, which is the one case where there is
		// nothing to match and nothing to get wrong.
		if e.Old == "" {
			if strings.TrimSpace(e.New) == "" {
				continue
			}
			if strings.TrimSpace(out) == "" {
				out = e.New
			} else {
				out = strings.TrimRight(out, "\n") + "\n\n" + e.New
			}
			continue
		}
		n := strings.Count(out, e.Old)
		switch {
		case n == 0:
			return "", fmt.Errorf("edit %d: %w: %q", i+1, ErrEditNotFound, clipEdit(e.Old))
		case n > 1 && !e.All:
			return "", fmt.Errorf("edit %d: %w (%d times): %q", i+1, ErrEditAmbiguous, n, clipEdit(e.Old))
		}
		if e.All {
			out = strings.ReplaceAll(out, e.Old, e.New)
		} else {
			out = strings.Replace(out, e.Old, e.New, 1)
		}
	}
	return out, nil
}

func clipEdit(s string) string {
	const max = 80
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// ToggleTodo ticks or unticks the checkbox at index, counting real `- [ ]` boxes
// ANYWHERE in the document.
//
// Only real boxes. v0 had a fallback that treated any plain bullet as a task when
// a section had no boxes, which was its Logbook convention leaking into the
// engine — a prose bullet is a sentence, and deciding otherwise is the server
// having an opinion about how somebody writes.
//
// wantText, when given, must still match the item's text: an index alone is a
// position, and positions move while somebody is reading. Quoting what you meant
// to tick is what turns a stale index into an error instead of the wrong box.
func ToggleTodo(doc string, index int, wantText string, done bool) (string, error) {
	lines := strings.Split(doc, "\n")

	var at []int
	for i, ln := range lines {
		if taskRe.MatchString(ln) {
			at = append(at, i)
		}
	}
	if index < 0 || index >= len(at) {
		return "", ErrNoSuchTodo
	}

	i := at[index]
	line := lines[i]
	text := ""
	if m := taskRe.FindStringSubmatch(line); m != nil {
		text = strings.TrimSpace(m[2])
	}
	if strings.TrimSpace(wantText) != "" && !sameTodoText(text, wantText) {
		return "", ErrTodoMoved
	}

	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	marker := "-"
	if trimmed != "" {
		marker = trimmed[:1]
	}
	box := "[ ]"
	if done {
		box = "[x]"
	}
	lines[i] = indent + marker + " " + box + " " + text
	return strings.Join(lines, "\n"), nil
}

// sameTodoText compares two todo texts the way a person would: spacing is
// cosmetic, everything else is the item.
func sameTodoText(a, b string) bool {
	return strings.EqualFold(
		strings.Join(strings.Fields(a), " "),
		strings.Join(strings.Fields(b), " "))
}

// ErrEditNotFound reports that the quoted text is not in the region.
var ErrEditNotFound = errors.New("that text is not in this section")

// ErrEditAmbiguous reports that the quoted text appears more than once.
var ErrEditAmbiguous = errors.New("that text appears more than once — quote more of it")

// ErrTodoMoved reports that the item at the given index is not the one the
// caller meant to tick.
var ErrTodoMoved = errors.New("that todo is no longer at that position")

// ErrNoSuchTodo reports an index past the end of a section's checklist.
var ErrNoSuchTodo = errors.New("no todo at that position")

// Window devuelve un trozo de un documento, por líneas y contando desde 1.
//
// Existe porque leer para editar no debería costar el documento entero. El
// `Read` de un harness sobre disco trae 2000 líneas por defecto y deja pedir el
// resto; aquí un documento de 38 KB —los hay, y son los importantes— son diez
// mil tokens cada vez que un agente quiere corregir una línea, y otros diez mil
// si tiene que releer tras un conflicto.
//
// Cuenta las líneas del ORIGINAL, no del trozo: lo que devuelve `total` es lo
// que hace falta para saber que falta algo, y una cuenta sobre el recorte diría
// siempre que está completo.
//
// `from` fuera de rango no es un error: se ajusta. Pedir la línea 900 de un
// documento de 500 es una pregunta razonable de alguien que no sabía cuánto
// medía, y contestarla con un fallo le obliga a una lectura más para averiguar
// lo que la respuesta ya podía decirle.
func Window(markdown string, from, count int) (text string, first, last, total int) {
	lines := strings.Split(markdown, "\n")
	// Un archivo que termina en salto no tiene una última línea vacía: la tiene
	// el `Split`, y contarla haría que todo documento midiera uno de más.
	if n := len(lines); n > 1 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	// Y un documento vacío no tiene UNA línea vacía: no tiene ninguna. Decir
	// «1 de 1» sería inventarse una para que la cuenta cuadre.
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	total = len(lines)
	if total == 0 {
		return "", 0, 0, 0
	}
	if from < 1 {
		from = 1
	}
	if from > total {
		from = total
	}
	if count < 1 {
		count = total
	}
	first = from
	last = from + count - 1
	if last > total {
		last = total
	}
	return strings.Join(lines[first-1:last], "\n"), first, last, total
}
