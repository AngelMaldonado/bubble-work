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
