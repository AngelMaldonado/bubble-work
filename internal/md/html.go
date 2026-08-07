// Package md converts Plane's stored rich text (description_html, produced by a
// ProseMirror/TipTap editor) into GFM Markdown, and splits a thread's body into
// the artifacts, TOC, and logbook the interior view renders (INTERIOR-PLAN.md,
// Phase 8). Plane is the system of record; this package only reads.
package md

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// FromHTML converts a Plane description_html fragment to GFM Markdown. It is
// tolerant of the ProseMirror quirks Plane emits — most importantly task lists
// (the logbook), which become GFM checkboxes with their checked state preserved.
func FromHTML(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return ""
	}
	c := &htmlConv{}
	root := findBody(doc)
	if root == nil {
		root = doc
	}
	c.blockChildren(root)
	return normalizeBlanks(c.sb.String())
}

type htmlConv struct {
	sb strings.Builder
}

func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if b := findBody(ch); b != nil {
			return b
		}
	}
	return nil
}

// blockChildren renders each child of n as block-level content.
func (c *htmlConv) blockChildren(n *html.Node) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.block(ch)
	}
}

func (c *htmlConv) block(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		if t := strings.TrimSpace(collapseWS(n.Data)); t != "" {
			c.sb.WriteString(t)
			c.sb.WriteString("\n\n")
		}
	case html.ElementNode:
		// Plane embeds images as a custom <image-component src="<asset-id>">.
		if n.Data == "image-component" {
			if src := attr(n, "src"); src != "" {
				c.sb.WriteString("![](" + assetRef(src) + ")\n\n")
			}
			return
		}
		switch n.DataAtom {
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			level := int(n.Data[1] - '0')
			c.sb.WriteString(strings.Repeat("#", level))
			c.sb.WriteByte(' ')
			c.sb.WriteString(strings.TrimSpace(c.inline(n)))
			c.sb.WriteString("\n\n")
		case atom.P:
			if t := strings.TrimSpace(c.inline(n)); t != "" {
				c.sb.WriteString(t)
				c.sb.WriteString("\n\n")
			}
		case atom.Ul, atom.Ol:
			c.list(n, n.DataAtom == atom.Ol, "")
			c.sb.WriteByte('\n')
		case atom.Blockquote:
			var inner htmlConv
			inner.blockChildren(n)
			for _, line := range strings.Split(strings.TrimRight(inner.sb.String(), "\n"), "\n") {
				c.sb.WriteString("> ")
				c.sb.WriteString(line)
				c.sb.WriteByte('\n')
			}
			c.sb.WriteByte('\n')
		case atom.Pre:
			// preserve the code language (e.g. language-mermaid) so mermaid blocks
			// are detected and other languages can be highlighted later.
			lang := ""
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				if ch.Type == html.ElementNode && ch.DataAtom == atom.Code {
					for _, cls := range strings.Fields(attr(ch, "class")) {
						if l, ok := strings.CutPrefix(cls, "language-"); ok {
							lang = l
						}
					}
				}
			}
			code := strings.TrimRight(textContent(n), "\n")
			c.sb.WriteString("```" + lang + "\n")
			c.sb.WriteString(code)
			c.sb.WriteString("\n```\n\n")
		case atom.Hr:
			c.sb.WriteString("---\n\n")
		case atom.Img:
			// a block-level <img> (direct child, not inside a <p>)
			if src := attr(n, "src"); src != "" {
				c.sb.WriteString("![" + attr(n, "alt") + "](" + src + ")\n\n")
			}
		case atom.Table:
			c.table(n)
		case atom.Div, atom.Section, atom.Article, atom.Main, atom.Header, atom.Footer:
			c.blockChildren(n) // unwrap containers
		default:
			// Unknown element: if it wraps block content, recurse; otherwise treat
			// it as an inline run and emit it as a paragraph.
			if hasBlockChild(n) {
				c.blockChildren(n)
			} else if t := strings.TrimSpace(c.inline(n)); t != "" {
				c.sb.WriteString(t)
				c.sb.WriteString("\n\n")
			}
		}
	}
}

// hardBreak is the markdown for a <br>: two spaces then a newline.
const hardBreak = "  \n"

// inlineBuf assembles inline renderings under one rule the round trip depends on:
// a hard break OWNS the whitespace on both sides of it.
//
// RenderHTML emits "  \n" as "<br>\n". Read back, the source newline AFTER the
// <br> collapses to a leading space, and any spaces BEFORE it are re-emitted on
// top of the "  " the break already carries. Left alone, "a   \nb" returns as
// "a  \n b", then as "a  \nb" — a body that rewrites itself every time it is
// saved, which is exactly what an editor must not do.
type inlineBuf struct {
	b          []byte
	afterBreak bool
}

func (ib *inlineBuf) add(s string) {
	if s == "" {
		return
	}
	if s == hardBreak {
		ib.b = bytes.TrimRight(ib.b, " \t")
		ib.b = append(ib.b, s...)
		ib.afterBreak = true
		return
	}
	// After a break, or against whitespace already in the buffer, drop the
	// leading run. collapseWS only sees one text node at a time, so two runs
	// meeting at an inline boundary — "ser: " + <span> + " 1 se" — survive as a
	// double space that HTML never rendered and that collapses on the next read.
	if ib.afterBreak || endsWithSpace(ib.b) {
		if s = strings.TrimLeft(s, " \t"); s == "" {
			return
		}
	}
	ib.b = append(ib.b, s...)
	ib.afterBreak = strings.HasSuffix(s, "\n")
}

func endsWithSpace(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	return b[len(b)-1] == ' ' || b[len(b)-1] == '\t'
}

func (ib *inlineBuf) String() string { return string(ib.b) }

// inline renders the inline children of n to a single string.
func (c *htmlConv) inline(n *html.Node) string {
	var ib inlineBuf
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		ib.add(c.inlineNode(ch))
	}
	return ib.String()
}

func (c *htmlConv) inlineNode(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return collapseWS(n.Data)
	case html.ElementNode:
		if n.Data == "image-component" {
			if src := attr(n, "src"); src != "" {
				return "![](" + assetRef(src) + ")"
			}
			return ""
		}
		switch n.DataAtom {
		case atom.Strong, atom.B:
			return wrap("**", strings.TrimSpace(c.inline(n)))
		case atom.Em, atom.I:
			return wrap("*", strings.TrimSpace(c.inline(n)))
		case atom.Del, atom.S:
			return wrap("~~", strings.TrimSpace(c.inline(n)))
		case atom.Code:
			return wrap("`", c.inline(n))
		case atom.A:
			txt := c.inline(n)
			if href := attr(n, "href"); href != "" {
				return "[" + txt + "](" + href + ")"
			}
			return txt
		case atom.Br:
			return "  \n"
		case atom.Img:
			return "![" + attr(n, "alt") + "](" + attr(n, "src") + ")"
		case atom.Input:
			return "" // task-list checkbox is handled by list()
		default:
			t := c.inline(n)
			// ProseMirror mentions render as a span/custom tag with data-type.
			if attr(n, "data-type") == "mention" && t != "" && !strings.HasPrefix(t, "@") {
				return "@" + t
			}
			return t
		}
	}
	return ""
}

// wrap applies a markdown delimiter only when there is inner content.
func wrap(delim, inner string) string {
	if inner == "" {
		return ""
	}
	return delim + inner + delim
}

// list renders a <ul>/<ol>. indent is the prefix every item of this list carries;
// a nested list is indented to its parent item's CONTENT column, which is what
// GFM requires — not a fixed two spaces. Under "- " that is the same thing, but
// under "3. " two spaces is one short of nesting, so the child list was being
// flattened into its parent and renumbered on the way through.
func (c *htmlConv) list(n *html.Node, ordered bool, indent string) {
	idx := listStart(n, ordered)
	for li := n.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.DataAtom != atom.Li {
			continue
		}
		checked, isTask := taskState(li)
		// bullet is the list marker alone; marker includes the checkbox, which is
		// content rather than part of the marker and so does not shift the column
		// a nested list has to reach.
		var marker, bullet string
		switch {
		case isTask && checked:
			marker, bullet = "- [x] ", "- "
		case isTask:
			marker, bullet = "- [ ] ", "- "
		case ordered:
			marker = strconv.Itoa(idx) + ". "
			bullet = marker
			idx++
		default:
			marker, bullet = "- ", "- "
		}
		c.sb.WriteString(indent)
		c.sb.WriteString(marker)
		c.sb.WriteString(strings.TrimSpace(c.liText(li)))
		c.sb.WriteByte('\n')
		// Nested lists render one level deeper.
		child := indent + strings.Repeat(" ", len(bullet))
		for ch := li.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && (ch.DataAtom == atom.Ul || ch.DataAtom == atom.Ol) {
				c.list(ch, ch.DataAtom == atom.Ol, child)
			}
		}
	}
}

// listStart reads an <ol start="N">. goldmark emits one whenever a list does not
// begin at 1, so honouring it is what keeps "3. 4. 5." from resetting to "1.".
func listStart(n *html.Node, ordered bool) int {
	if !ordered {
		return 1
	}
	if s := attr(n, "start"); s != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && v > 0 {
			return v
		}
	}
	return 1
}

// liText gathers a list item's inline text, skipping nested lists (rendered
// separately) and unwrapping the p/div/label wrappers ProseMirror nests inside.
//
// It assembles through inlineBuf for the same reason inline does: a task item
// whose text carries a hard break is common (Plane's editor makes one on every
// shift-enter), and joining the pieces raw reintroduced the leading space the
// paragraph path had just been taught to drop.
func (c *htmlConv) liText(li *html.Node) string {
	var ib inlineBuf
	for ch := li.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && (ch.DataAtom == atom.Ul || ch.DataAtom == atom.Ol) {
			continue
		}
		if ch.Type == html.ElementNode &&
			(ch.DataAtom == atom.P || ch.DataAtom == atom.Div || ch.DataAtom == atom.Label || ch.DataAtom == atom.Span) {
			ib.add(c.inline(ch))
		} else {
			ib.add(c.inlineNode(ch))
		}
	}
	return ib.String()
}

// taskState reports whether a list item is a task item and, if so, its checked
// state. It understands both the data-checked/data-type attributes and a nested
// <input type="checkbox"> — the two shapes Plane/TipTap emit.
func taskState(li *html.Node) (checked, isTask bool) {
	if dc := attr(li, "data-checked"); dc != "" {
		return dc == "true", true
	}
	found, chk := false, false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Input &&
			strings.EqualFold(attr(n, "type"), "checkbox") {
			found = true
			if hasAttr(n, "checked") {
				chk = true
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(li)
	if found {
		return chk, true
	}
	if attr(li, "data-type") == "taskItem" {
		return false, true
	}
	return false, false
}

func (c *htmlConv) table(n *html.Node) {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(nd *html.Node) {
		if nd.Type == html.ElementNode && nd.DataAtom == atom.Tr {
			var cells []string
			for cell := nd.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type == html.ElementNode && (cell.DataAtom == atom.Td || cell.DataAtom == atom.Th) {
					cells = append(cells, strings.TrimSpace(collapseWS(textContent(cell))))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for ch := nd.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	writeRow := func(cells []string) {
		c.sb.WriteByte('|')
		for i := 0; i < cols; i++ {
			v := ""
			if i < len(cells) {
				v = cells[i]
			}
			c.sb.WriteByte(' ')
			c.sb.WriteString(v)
			c.sb.WriteString(" |")
		}
		c.sb.WriteByte('\n')
	}
	writeRow(rows[0])
	c.sb.WriteByte('|')
	for i := 0; i < cols; i++ {
		c.sb.WriteString(" --- |")
	}
	c.sb.WriteByte('\n')
	for _, r := range rows[1:] {
		writeRow(r)
	}
	c.sb.WriteByte('\n')
}

// ---- small helpers ----

// AssetScheme marks a Plane asset id inside an image src so the server can
// rewrite it to a real (proxied) URL once it knows the instance and project.
const AssetScheme = "plane-asset:"

// assetRef wraps a Plane image src: absolute URLs pass through; bare asset ids
// get the plane-asset: marker for later server-side rewriting.
func assetRef(src string) string {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src
	}
	return AssetScheme + src
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(nd *html.Node) {
		if nd.Type == html.TextNode {
			b.WriteString(nd.Data)
		}
		for ch := nd.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return b.String()
}

func hasBlockChild(n *html.Node) bool {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode {
			continue
		}
		switch ch.DataAtom {
		case atom.P, atom.Div, atom.Ul, atom.Ol, atom.H1, atom.H2, atom.H3,
			atom.H4, atom.H5, atom.H6, atom.Table, atom.Blockquote, atom.Pre, atom.Hr:
			return true
		}
	}
	return false
}

var wsRe = regexp.MustCompile(`\s+`)

// collapseWS turns any run of whitespace (including the newlines and indentation
// ProseMirror pretty-prints between tags) into a single space.
func collapseWS(s string) string {
	return wsRe.ReplaceAllString(s, " ")
}

var blankRe = regexp.MustCompile(`\n{3,}`)

// normalizeBlanks trims the result and collapses 3+ blank lines to a single one.
func normalizeBlanks(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = blankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
