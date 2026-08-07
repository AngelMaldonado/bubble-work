package md

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Plane's own node vocabulary (docs/ARTIFACT-EDITING.md Phase 2).
//
// Plane stores rich text as description_html produced by a ProseMirror/TipTap
// editor, and three of its constructs have no GFM spelling: images are
// <image-component>, mentions are <mention-component>, and checkboxes are a
// <ul data-type="taskList"> rather than a plain list. Reading them was already
// lossy — mentions vanished outright — and WRITING markdown back produced HTML
// Plane could not render.
//
// So markdown carries them as markers, and RenderPlaneHTML maps the markers back
// onto Plane's nodes. RenderHTML is left alone: it renders for OUR web view,
// where an <image-component> means nothing and the server rewrites the markers
// for display instead.

// MentionScheme marks a Plane user id inside a link target, the way AssetScheme
// marks an asset id inside an image src.
const MentionScheme = "plane-mention:"

// mentionLabel is what a mention reads as when nobody has resolved the id to a
// display name. The label is decoration: only the link target survives a write,
// so a surface that knows the person's name is free to substitute it.
const mentionLabel = "@mention"

// RenderPlaneHTML converts GFM Markdown to the HTML Plane stores.
//
// Unlike RenderHTML this is a WRITE path, so it re-serialises through the HTML
// parser: the output is new bytes either way, and there is nothing to preserve.
// (Splicing is what preserves bytes, and it only ever calls this for the blocks
// that actually changed.)
func RenderPlaneHTML(markdown string) string {
	raw := RenderHTML(markdown)
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return raw
	}
	body := findBody(doc)
	if body == nil {
		return raw
	}
	planeify(body)

	var buf bytes.Buffer
	for ch := body.FirstChild; ch != nil; ch = ch.NextSibling {
		if err := html.Render(&buf, ch); err != nil {
			return raw
		}
	}
	return strings.TrimSpace(buf.String())
}

// planeify rewrites a rendered tree in place. Nodes are collected first and
// mutated after, because replacing a node while walking its parent's sibling
// chain loses the rest of the chain.
func planeify(root *html.Node) {
	var imgs, links, lists []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Img:
				if strings.HasPrefix(attr(n, "src"), AssetScheme) {
					imgs = append(imgs, n)
				}
			case atom.A:
				if strings.HasPrefix(attr(n, "href"), MentionScheme) {
					links = append(links, n)
				}
			case atom.Ul, atom.Ol:
				if hasTaskItem(n) {
					lists = append(lists, n)
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)

	for _, n := range imgs {
		id := strings.TrimPrefix(attr(n, "src"), AssetScheme)
		replace(n, element("image-component", "src", id))
	}
	for _, n := range links {
		id := strings.TrimPrefix(attr(n, "href"), MentionScheme)
		replace(n, element("mention-component",
			"entity_identifier", id, "entity_name", "user_mention"))
	}
	for _, n := range lists {
		taskList(n)
	}
}

// hasTaskItem reports whether a list holds GFM checkbox items, which is how
// goldmark renders "- [x]" — a bare <input type=checkbox> inside the <li>.
func hasTaskItem(list *html.Node) bool {
	for li := list.FirstChild; li != nil; li = li.NextSibling {
		if li.Type == html.ElementNode && li.DataAtom == atom.Li && findCheckbox(li) != nil {
			return true
		}
	}
	return false
}

// findCheckbox returns the <li>'s own checkbox input, ignoring any belonging to
// a nested list.
func findCheckbox(li *html.Node) *html.Node {
	for ch := li.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && ch.DataAtom == atom.Input &&
			strings.EqualFold(attr(ch, "type"), "checkbox") {
			return ch
		}
	}
	return nil
}

// taskList rewrites goldmark's checkbox list into the shape Plane's own editor
// writes, so a Logbook we save is a Logbook Plane can render and edit. This is
// the one transform that is not merely cosmetic: the Logbook is where heat comes
// from, and a checkbox Plane drops is a todo that stops counting.
func taskList(list *html.Node) {
	setAttr(list, "data-type", "taskList")
	for li := list.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.DataAtom != atom.Li {
			continue
		}
		box := findCheckbox(li)
		if box == nil {
			continue
		}
		checked := hasAttr(box, "checked")
		setAttr(li, "data-type", "taskItem")
		setAttr(li, "data-checked", boolAttr(checked))

		// Detach everything, then rebuild: label + checkbox, the content in a
		// <div><p>, and any nested list after it as a sibling — which is where
		// TipTap expects to find one.
		var content, nested []*html.Node
		for ch := li.FirstChild; ch != nil; {
			next := ch.NextSibling
			li.RemoveChild(ch)
			switch {
			case ch == box:
				// dropped; the rebuilt label carries its own input
			case ch.Type == html.ElementNode && (ch.DataAtom == atom.Ul || ch.DataAtom == atom.Ol):
				nested = append(nested, ch)
			default:
				content = append(content, ch)
			}
			ch = next
		}

		label := element("label")
		input := element("input", "type", "checkbox")
		if checked {
			setAttr(input, "checked", "checked")
		}
		label.AppendChild(input)
		label.AppendChild(element("span"))
		li.AppendChild(label)

		wrapper := element("div")
		para := element("p")
		first := true
		for _, c := range content {
			// goldmark separates the checkbox from the text with a single space.
			// Only THAT one goes: trimming every text node would eat the space
			// after inline markup, turning "**CREANDO** NO" into "**CREANDO**NO".
			if first && c.Type == html.TextNode {
				c.Data = strings.TrimLeft(c.Data, " ")
				if c.Data == "" {
					continue
				}
			}
			para.AppendChild(c)
			first = false
		}
		wrapper.AppendChild(para)
		li.AppendChild(wrapper)
		for _, c := range nested {
			li.AppendChild(c)
		}
	}
}

func boolAttr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// element builds a node from alternating key/value attribute pairs.
func element(tag string, kv ...string) *html.Node {
	n := &html.Node{Type: html.ElementNode, Data: tag, DataAtom: atom.Lookup([]byte(tag))}
	for i := 0; i+1 < len(kv); i += 2 {
		n.Attr = append(n.Attr, html.Attribute{Key: kv[i], Val: kv[i+1]})
	}
	return n
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func replace(old, new *html.Node) {
	if old.Parent == nil {
		return
	}
	old.Parent.InsertBefore(new, old)
	old.Parent.RemoveChild(old)
}
