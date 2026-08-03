// Package md converts Plane's stored rich text (description_html, produced by a
// ProseMirror/TipTap editor) into GFM Markdown, and splits a thread's body into
// the artifacts, TOC, and logbook the interior view renders (INTERIOR-PLAN.md,
// Phase 8). Plane is the system of record; this package only reads.
package md

import (
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
			c.list(n, n.DataAtom == atom.Ol, 0)
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

// inline renders the inline children of n to a single string.
func (c *htmlConv) inline(n *html.Node) string {
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		b.WriteString(c.inlineNode(ch))
	}
	return b.String()
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

func (c *htmlConv) list(n *html.Node, ordered bool, depth int) {
	idx := 1
	indent := strings.Repeat("  ", depth)
	for li := n.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.DataAtom != atom.Li {
			continue
		}
		checked, isTask := taskState(li)
		var marker string
		switch {
		case isTask && checked:
			marker = "- [x] "
		case isTask:
			marker = "- [ ] "
		case ordered:
			marker = strconv.Itoa(idx) + ". "
		default:
			marker = "- "
		}
		idx++
		c.sb.WriteString(indent)
		c.sb.WriteString(marker)
		c.sb.WriteString(strings.TrimSpace(c.liText(li)))
		c.sb.WriteByte('\n')
		// Nested lists render one level deeper.
		for ch := li.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && (ch.DataAtom == atom.Ul || ch.DataAtom == atom.Ol) {
				c.list(ch, ch.DataAtom == atom.Ol, depth+1)
			}
		}
	}
}

// liText gathers a list item's inline text, skipping nested lists (rendered
// separately) and unwrapping the p/div/label wrappers ProseMirror nests inside.
func (c *htmlConv) liText(li *html.Node) string {
	var b strings.Builder
	for ch := li.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && (ch.DataAtom == atom.Ul || ch.DataAtom == atom.Ol) {
			continue
		}
		if ch.Type == html.ElementNode &&
			(ch.DataAtom == atom.P || ch.DataAtom == atom.Div || ch.DataAtom == atom.Label || ch.DataAtom == atom.Span) {
			b.WriteString(c.inline(ch))
		} else {
			b.WriteString(c.inlineNode(ch))
		}
	}
	return b.String()
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
