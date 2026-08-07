package md

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Block-level splicing (docs/ARTIFACT-EDITING.md Phase 1).
//
// A whole-body write is the obvious shape and the wrong one: reading a body
// costs a FromHTML and writing it costs a RenderHTML, so every construct the
// pair does not carry — today mentions and images — is destroyed even in the
// parts nobody edited. Phase 0 got that pair to 99.68% per block, which is very
// good and still not "never".
//
// So writes do not re-render a body. They diff the submitted markdown against
// the markdown derived from it, block by block, and a block whose markdown did
// not change keeps its ORIGINAL BYTES — never re-parsed, never re-rendered.
// Blast radius is what you actually typed in.

// Block is one top-level element of a body: its byte range in the source, the
// original bytes, and the markdown they render to.
type Block struct {
	Start, End int
	HTML       string
	Markdown   string
}

// Region names an editable part of a thread page.
type Region string

const (
	// RegionDocument is everything that is not the Logbook or the Definition of
	// Done. On the 92-of-96 threads that have no Logbook it is the whole body.
	RegionDocument Region = "document"
	RegionLogbook  Region = "logbook"
	RegionDoD      Region = "dod"
)

// sectionTitles are the headings that delimit each region. Matching is on the
// heading TEXT, case-insensitively, the same way ExtractSection works.
var sectionTitles = map[Region][]string{
	RegionLogbook: {"logbook"},
	RegionDoD:     {"definition of done", "dod"},
}

// voidElements never nest, so the tokenizer must not count them as opening a
// level — otherwise a <br> would swallow the rest of the document into one block.
var voidElements = map[string]struct{}{
	"area": {}, "base": {}, "br": {}, "col": {}, "embed": {}, "hr": {},
	"img": {}, "input": {}, "link": {}, "meta": {}, "source": {}, "track": {}, "wbr": {},
}

// Blocks splits a body into its top-level elements.
//
// It works on the raw bytes with a tokenizer rather than on a parsed tree,
// because the whole point is to hand back ORIGINAL bytes: html.Parse plus
// re-serialisation normalises attribute order and quoting, so a "byte-identical"
// splice built that way would not be.
func Blocks(descriptionHTML string) []Block {
	bs := topLevel(descriptionHTML, 0)

	// A body we wrote ourselves may be wrapped in a single container. Descend
	// into it so its children are the blocks; the wrapper's own tags stay in the
	// prefix and suffix a splice preserves.
	for len(bs) == 1 {
		inner, base, ok := unwrap(descriptionHTML, bs[0])
		if !ok {
			break
		}
		next := topLevel(inner, base)
		if len(next) == 0 {
			break
		}
		bs = next
	}

	for i := range bs {
		bs[i].HTML = descriptionHTML[bs[i].Start:bs[i].End]
		bs[i].Markdown = FromHTML(bs[i].HTML)
	}
	return bs
}

// topLevel returns the byte ranges of the depth-zero elements of doc, offset by
// base so the ranges index into the original document.
func topLevel(doc string, base int) []Block {
	z := html.NewTokenizer(strings.NewReader(doc))
	off, depth := 0, 0
	start := -1
	var out []Block
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		tokenStart := off
		off += len(z.Raw())

		switch tt {
		case html.StartTagToken:
			name, _ := z.TagName()
			if _, void := voidElements[string(name)]; void {
				// The tokenizer reports "<img …>" as a start tag, not a
				// self-closing one, and no end tag ever follows. Left to the
				// generic path it would open a block that never closed and
				// swallow everything after it.
				if depth == 0 && start < 0 {
					out = append(out, Block{Start: base + tokenStart, End: base + off})
				}
				continue
			}
			if depth == 0 && start < 0 {
				start = tokenStart
			}
			depth++
		case html.SelfClosingTagToken:
			if depth == 0 && start < 0 {
				out = append(out, Block{Start: base + tokenStart, End: base + off})
			}
		case html.EndTagToken:
			if depth > 0 {
				depth--
			}
			if depth == 0 && start >= 0 {
				out = append(out, Block{Start: base + start, End: base + off})
				start = -1
			}
		}
	}
	return out
}

// unwrap reports the inner content of a lone container block, so a body wrapped
// in one <div> is not treated as a single unsplittable block.
func unwrap(doc string, b Block) (inner string, base int, ok bool) {
	raw := doc[b.Start:b.End]
	name, attrsEnd := tagName(raw)
	switch name {
	case "div", "section", "article", "main", "body":
	default:
		return "", 0, false
	}
	closing := "</" + name
	last := strings.LastIndex(raw, closing)
	if attrsEnd < 0 || last < attrsEnd {
		return "", 0, false
	}
	return raw[attrsEnd:last], b.Start + attrsEnd, true
}

// tagName returns a start tag's name and the offset just past its ">".
func tagName(raw string) (string, int) {
	if len(raw) < 2 || raw[0] != '<' {
		return "", -1
	}
	end := strings.IndexByte(raw, '>')
	if end < 0 {
		return "", -1
	}
	name := raw[1:end]
	if i := strings.IndexAny(name, " \t\n/"); i >= 0 {
		name = name[:i]
	}
	return strings.ToLower(name), end + 1
}

// RegionMarkdown returns the markdown a region currently holds — what an editor
// loads — plus whether the region exists at all.
func RegionMarkdown(descriptionHTML string, region Region) (string, bool) {
	bs := Blocks(descriptionHTML)
	lo, hi, found := regionRange(bs, region)
	if !found {
		return "", false
	}
	parts := make([]string, 0, hi-lo)
	for _, b := range bs[lo:hi] {
		if strings.TrimSpace(b.Markdown) != "" {
			parts = append(parts, b.Markdown)
		}
	}
	return strings.Join(parts, "\n\n"), true
}

// regionRange returns the CONTENT block range of a region, [lo,hi). For a
// section it excludes the heading block: the heading is the section's marker,
// not its content, and editing a Logbook must not be able to rename it.
func regionRange(bs []Block, region Region) (lo, hi int, found bool) {
	if region == RegionDocument {
		// Everything before the first special section. Blocks AFTER the Logbook
		// or DoD are left alone — preserved, but not editable in this pass.
		end := len(bs)
		for _, r := range []Region{RegionLogbook, RegionDoD} {
			if h, ok := sectionHeading(bs, r); ok && h < end {
				end = h
			}
		}
		return 0, end, true
	}
	h, ok := sectionHeading(bs, region)
	if !ok {
		return 0, 0, false
	}
	level, _, _ := headingOf(bs[h].Markdown)
	hi = len(bs)
	for j := h + 1; j < len(bs); j++ {
		if l, _, isHead := headingOf(bs[j].Markdown); isHead && l <= level {
			hi = j
			break
		}
	}
	return h + 1, hi, true
}

// sectionHeading finds the block index of a region's heading.
func sectionHeading(bs []Block, region Region) (int, bool) {
	want := sectionTitles[region]
	for i, b := range bs {
		_, title, ok := headingOf(b.Markdown)
		if !ok {
			continue
		}
		for _, w := range want {
			if strings.EqualFold(title, w) {
				return i, true
			}
		}
	}
	return 0, false
}

// headingOf reports whether a block's markdown is a single heading, and at what
// level. Reading the derived markdown rather than the HTML keeps one definition
// of "this is a heading" shared with everything else in this package.
func headingOf(markdown string) (level int, title string, ok bool) {
	s := strings.TrimSpace(markdown)
	if s == "" || s[0] != '#' || strings.Contains(s, "\n") {
		return 0, "", false
	}
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n > 6 || n >= len(s) || s[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(s[n:]), true
}

// Splice replaces one region of a body with new markdown, reusing the original
// bytes of every block whose markdown did not change.
//
// The body it splices into must be the CURRENT one — the caller reads it from
// the mirror at write time rather than trusting a copy the client loaded, so two
// people editing different regions cannot lose each other's work.
func Splice(descriptionHTML string, region Region, markdown string) (string, error) {
	if region != RegionDocument && region != RegionLogbook && region != RegionDoD {
		return "", fmt.Errorf("unknown region %q", region)
	}
	bs := Blocks(descriptionHTML)
	lo, hi, found := regionRange(bs, region)
	if !found {
		// A thread born without a Logbook gains one rather than silently
		// swallowing the update (§3.2 allows a small thread to have none).
		return appendSection(descriptionHTML, region, markdown)
	}

	// Plane's editor leaves empty paragraphs behind as spacing. They carry no
	// markdown, so the editor never shows them and the submitted text cannot
	// mention them — but they are still the author's layout, and a diff that
	// only saw content would delete every one of them on the first save.
	// They ride along instead, anchored to the block they follow.
	lead, content, trail := partitionSpacers(bs, lo, hi)

	want := splitMarkdownBlocks(markdown)
	have := make([]string, 0, len(content))
	for _, i := range content {
		have = append(have, strings.TrimSpace(bs[i].Markdown))
	}

	reuse := matchBlocks(have, want)
	unchanged := len(have) == len(want)
	if unchanged {
		for j, k := range reuse {
			if k != j {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		// Nothing moved. Returning the input verbatim is not an optimisation —
		// it is the guarantee that saving an untouched region is a no-op on
		// Plane's bytes, and so cannot generate heat or a diff.
		return descriptionHTML, nil
	}

	sep := regionSeparator(descriptionHTML, bs, lo, hi)
	parts := make([]string, 0, len(want)+len(lead))
	for _, i := range lead {
		parts = append(parts, bs[i].HTML)
	}
	for j, m := range want {
		if k := reuse[j]; k >= 0 {
			parts = append(parts, bs[content[k]].HTML)
			for _, i := range trail[k] {
				parts = append(parts, bs[i].HTML)
			}
			continue
		}
		if rendered := strings.TrimSpace(RenderPlaneHTML(m)); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	inner := strings.Join(parts, sep)

	// An empty content range still has a well-defined insertion point: just
	// after the section heading, or at the very start for an empty document.
	from, to := insertionPoint(bs, lo, hi)
	return descriptionHTML[:from] + inner + descriptionHTML[to:], nil
}

// partitionSpacers separates a region's blocks into the ones the editor can see
// and the empty ones it cannot. lead holds spacers before any content; content
// holds the visible blocks; trail[k] holds the spacers that followed content[k].
func partitionSpacers(bs []Block, lo, hi int) (lead, content []int, trail map[int][]int) {
	trail = map[int][]int{}
	for i := lo; i < hi; i++ {
		if strings.TrimSpace(bs[i].Markdown) != "" {
			content = append(content, i)
			continue
		}
		if len(content) == 0 {
			lead = append(lead, i)
		} else {
			k := len(content) - 1
			trail[k] = append(trail[k], i)
		}
	}
	return lead, content, trail
}

// insertionPoint returns the byte range a region's content occupies. When the
// region holds no blocks it collapses to a single position.
func insertionPoint(bs []Block, lo, hi int) (from, to int) {
	if lo < hi {
		return bs[lo].Start, bs[hi-1].End
	}
	if lo > 0 && lo-1 < len(bs) {
		return bs[lo-1].End, bs[lo-1].End // just after the heading
	}
	return 0, 0
}

// regionSeparator picks the whitespace to put between blocks, copying whatever
// the document already uses — Plane writes none, goldmark writes a newline — so
// a splice does not restyle the parts of the body it is rebuilding.
func regionSeparator(doc string, bs []Block, lo, hi int) string {
	for i := lo; i+1 < hi; i++ {
		return doc[bs[i].End:bs[i+1].Start]
	}
	for i := 0; i+1 < len(bs); i++ {
		return doc[bs[i].End:bs[i+1].Start]
	}
	return ""
}

// appendSection adds a region that does not exist yet, at the end of the body.
func appendSection(doc string, region Region, markdown string) (string, error) {
	titles := sectionTitles[region]
	if len(titles) == 0 {
		return "", fmt.Errorf("cannot create region %q", region)
	}
	title := "Logbook"
	if region == RegionDoD {
		title = "Definition of Done"
	}
	body := strings.TrimSpace(RenderPlaneHTML(markdown))
	out := strings.TrimRight(doc, " \n\t")
	return out + "<h2>" + title + "</h2>" + body, nil
}

// splitMarkdownBlocks breaks edited markdown back into blocks on blank lines,
// which is the inverse of how Blocks derives them. A fenced code block is
// atomic: the blank lines inside one are content, not boundaries.
func splitMarkdownBlocks(markdown string) []string {
	s := strings.ReplaceAll(markdown, "\r\n", "\n")
	var out []string
	var cur []string
	fenced := false

	flush := func() {
		joined := strings.Trim(strings.Join(cur, "\n"), "\n")
		if strings.TrimSpace(joined) != "" {
			out = append(out, joined)
		}
		cur = cur[:0]
	}
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			cur = append(cur, ln)
			if fenced {
				flush()
			}
			fenced = !fenced
			continue
		}
		// A TRULY empty line separates blocks; a whitespace-only one does not.
		// The distinction matters because two <br>s in a row — pressing enter
		// twice in Plane's editor — render as "  \n  \n", whose middle line is
		// two spaces. Treating that as a boundary split one paragraph into
		// several blocks that then matched nothing and were re-rendered on
		// every save. Blocks are joined with a bare "\n\n", so the separator
		// this has to recognise is always exactly empty.
		if !fenced && ln == "" {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return out
}

// RemoveRegion deletes a region outright — for a section, its heading goes with
// it, which is what distinguishes deleting a Logbook from emptying one.
//
// The document region keeps nothing to delete but its blocks: it has no heading
// of its own, being defined as "everything before the first special section".
func RemoveRegion(descriptionHTML string, region Region) (string, error) {
	if region != RegionDocument && region != RegionLogbook && region != RegionDoD {
		return "", fmt.Errorf("unknown region %q", region)
	}
	bs := Blocks(descriptionHTML)
	lo, hi, found := regionRange(bs, region)
	if !found {
		return descriptionHTML, nil // nothing there is already the desired state
	}
	if region != RegionDocument {
		lo-- // take the heading with it
	}
	if lo >= hi || lo < 0 {
		return descriptionHTML, nil
	}
	return strings.TrimSpace(descriptionHTML[:bs[lo].Start] + descriptionHTML[bs[hi-1].End:]), nil
}

// Hash fingerprints a region's markdown so an editor can prove it is writing
// over what it read. Exact, not whitespace-normalised: this answers "did this
// change under me", where LogbookFingerprint answers "did the plan change" and
// deliberately forgives a reflow.
func Hash(markdown string) string {
	sum := sha256.Sum256([]byte(markdown))
	return hex.EncodeToString(sum[:8])
}

// ErrTodoMoved reports that the item at the given index is not the one the
// caller meant to tick.
var ErrTodoMoved = errors.New("that todo is no longer at that position")

// ErrNoSuchTodo reports an index past the end of a section's checklist.
var ErrNoSuchTodo = errors.New("no todo at that position")

// ToggleTodo flips the checked state of the nth checklist item in a section.
//
// wantText GUARDS the index. An index alone is a question the caller cannot
// actually answer — the list it counted may have been re-ordered by anyone since
// it was rendered — and ticking the wrong box is worse than refusing, because it
// is silent and it manufactures evidence of production. Pass "" only when the
// caller genuinely has no text to check against.
//
// Items are counted exactly the way ParseTodos counts them, so an index taken
// from a rendered Logbook means the same thing here. A plain bullet becomes a
// checkbox: ticking one is a real edit somebody asked for, and the alternative
// is a checkbox in the UI that refuses to work.
func ToggleTodo(section string, index int, wantText string, done bool) (string, error) {
	lines := strings.Split(section, "\n")

	var at []int
	for i, ln := range lines {
		if taskRe.MatchString(ln) {
			at = append(at, i)
		}
	}
	if len(at) == 0 {
		for i, ln := range lines {
			if bulletRe.MatchString(ln) {
				at = append(at, i)
			}
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
	} else if m := bulletRe.FindStringSubmatch(line); m != nil {
		text = strings.TrimSpace(m[1])
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

// matchBlocks aligns the submitted blocks against the ones already there and
// returns, for each submitted block, the index of the existing block it reuses
// (or -1 when it is new). A longest-common-subsequence alignment is what makes
// inserting a paragraph at the top cost one rendered block rather than
// re-rendering everything below it.
func matchBlocks(have, want []string) []int {
	n, m := len(have), len(want)
	out := make([]int, m)
	for j := range out {
		out[j] = -1
	}
	if n == 0 || m == 0 {
		return out
	}

	// lcs[i][j] = length of the longest common subsequence of have[i:], want[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if have[i] == want[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case have[i] == want[j]:
			out[j] = i
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}
