package bubble

import (
	"reflect"
	"testing"
)

func TestValidateMarkdownPublication(t *testing.T) {
	valid := Entry{Type: "document", Data: map[string]any{"filename": "propuesta.md", "markdown": "# Hola\n- [ ] ejemplo", "base": float64(0)}}
	if err := validateMarkdown(valid, nil); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		key   string
		value any
	}{
		{"path", "filename", "docs/a.md"}, {"backslash", "filename", "docs\\a.md"}, {"extension", "filename", "a.txt"}, {"empty name", "filename", ".md"}, {"uppercase empty name", "filename", ".MD"}, {"control in name", "filename", "a\v.md"},
		{"missing text", "markdown", nil}, {"oversize", "markdown", string(make([]byte, (128<<10)+1))},
		{"missing base", "base", nil}, {"string base", "base", "0"}, {"fractional base", "base", 0.5}, {"negative base", "base", -1}, {"future base", "base", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := valid
			e.Data = map[string]any{}
			for k, v := range valid.Data {
				e.Data[k] = v
			}
			e.Data[c.key] = c.value
			if err := validateMarkdown(e, nil); err == nil {
				t.Fatal("accepted invalid publication")
			}
		})
	}
	wrong := valid
	wrong.Type = "answer"
	if validateMarkdown(wrong, nil) == nil {
		t.Fatal("accepted web answer")
	}
	previous := valid
	previous.Seq = 4
	if validateMarkdown(valid, []Entry{previous}) == nil {
		t.Fatal("accepted stale base")
	}
	valid.Data["base"] = 4
	if err := validateMarkdown(valid, []Entry{previous}); err != nil {
		t.Fatal(err)
	}
}

func TestLatestMarkdownAndWebHTML(t *testing.T) {
	es := []Entry{{Type: "document", Seq: 1, Data: map[string]any{"filename": "a.md", "markdown": "old"}}, {Type: "document", Seq: 2, Data: map[string]any{"filename": "b.md", "markdown": "**new**"}}, {Type: "handoff", Seq: 3}}
	doc := latestDocument(es)
	if doc == nil || doc.Seq != 2 || doc.Markdown != "**new**" {
		t.Fatalf("document: %#v", doc)
	}
	view := withHTML(ArtifactView{Kind: "markdown", Document: doc})
	if view.Document.HTML != "<p><strong>new</strong></p>\n" || doc.HTML != "" {
		t.Fatalf("web render: %#v; mutated source: %#v", view.Document, doc)
	}
}

func TestParticipantBoard(t *testing.T) {
	b := Board{Workspaces: []BoardWorkspace{{ID: "a"}, {ID: "b"}}, Bubbles: []BubbleHeat{{ID: "owned", Workspace: "a", Owners: []string{"me"}}, {ID: "assigned", Workspace: "a", Owners: []string{"other"}}, {ID: "hidden", Workspace: "b", Owners: []string{"other"}}}, Threads: []ThreadHeat{
		{ID: "owned-child", Workspace: "a", Bubble: "owned", Assignees: []string{"other"}},
		{ID: "assigned-child", Workspace: "a", Bubble: "assigned", Assignees: []string{"me"}},
		{ID: "sibling", Workspace: "a", Bubble: "assigned", Assignees: []string{"other"}},
		{ID: "standalone", Workspace: "a", Assignees: []string{"me"}},
		{ID: "hidden-child", Workspace: "b", Bubble: "hidden", Assignees: []string{"other"}},
	}}
	out := participantBoard(b, "me")
	var bs, ts, ws []string
	for _, x := range out.Bubbles {
		bs = append(bs, x.ID)
	}
	for _, x := range out.Threads {
		ts = append(ts, x.ID)
	}
	for _, x := range out.Workspaces {
		ws = append(ws, x.ID)
	}
	if !reflect.DeepEqual(bs, []string{"owned", "assigned"}) || !reflect.DeepEqual(ts, []string{"owned-child", "assigned-child", "standalone"}) || !reflect.DeepEqual(ws, []string{"a"}) {
		t.Fatalf("scope: %v %v %v", bs, ts, ws)
	}
	if len(b.Bubbles) != 3 || len(b.Threads) != 5 {
		t.Fatal("mutated source board")
	}
	empty := participantBoard(b, "nobody")
	if empty.Bubbles == nil || empty.Threads == nil || empty.Workspaces == nil || len(empty.Bubbles) != 0 || len(empty.Threads) != 0 || len(empty.Workspaces) != 0 {
		t.Fatalf("empty: %#v", empty)
	}
}
