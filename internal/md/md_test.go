package md

import (
	"reflect"
	"strings"
	"testing"
)

func TestFromHTML_Basics(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"heading+para", `<h1>Title</h1><p>Hello world</p>`, "# Title\n\nHello world"},
		{"h2h3", `<h2>A</h2><h3>B</h3>`, "## A\n\n### B"},
		{"bold+italic", `<p>a <strong>b</strong> and <em>c</em></p>`, "a **b** and *c*"},
		{"inline code", `<p>use <code>go build</code></p>`, "use `go build`"},
		{"link", `<p><a href="https://x.dev">site</a></p>`, "[site](https://x.dev)"},
		{"strikethrough", `<p><s>gone</s></p>`, "~~gone~~"},
		{"hr", `<p>a</p><hr/><p>b</p>`, "a\n\n---\n\nb"},
		{"blockquote", `<blockquote><p>quoted</p></blockquote>`, "> quoted"},
		{"empty", ``, ""},
		{"whitespace collapse", "<p>a\n   b\tc</p>", "a b c"},
		{"block image", `<p>x</p><img src="https://e.com/a.png" alt="pic">`, "x\n\n![pic](https://e.com/a.png)"},
		{"inline image", `<p>see <img src="https://e.com/b.png" alt="b"></p>`, "see ![b](https://e.com/b.png)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromHTML(tc.in); got != tc.want {
				t.Errorf("FromHTML(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFromHTML_Lists(t *testing.T) {
	got := FromHTML(`<ul><li>one</li><li>two</li></ul>`)
	if got != "- one\n- two" {
		t.Errorf("ul: got %q", got)
	}
	got = FromHTML(`<ol><li>first</li><li>second</li></ol>`)
	if got != "1. first\n2. second" {
		t.Errorf("ol: got %q", got)
	}
	got = FromHTML(`<ul><li>a<ul><li>a1</li></ul></li><li>b</li></ul>`)
	if got != "- a\n  - a1\n- b" {
		t.Errorf("nested: got %q", got)
	}
}

// Task lists are the logbook — checkbox state must survive both shapes Plane emits.
func TestFromHTML_TaskLists(t *testing.T) {
	// Shape A: data-checked attribute (TipTap default).
	a := `<ul data-type="taskList">
	  <li data-type="taskItem" data-checked="true"><div><p>done item</p></div></li>
	  <li data-type="taskItem" data-checked="false"><div><p>open item</p></div></li>
	</ul>`
	if got := FromHTML(a); got != "- [x] done item\n- [ ] open item" {
		t.Errorf("data-checked task list: got %q", got)
	}

	// Shape B: nested <input type=checkbox>.
	b := `<ul>
	  <li><label><input type="checkbox" checked="checked"><span></span></label><div>shipped</div></li>
	  <li><label><input type="checkbox"><span></span></label><div>todo</div></li>
	</ul>`
	if got := FromHTML(b); got != "- [x] shipped\n- [ ] todo" {
		t.Errorf("input task list: got %q", got)
	}
}

func TestFromHTML_CodeBlock(t *testing.T) {
	got := FromHTML("<pre><code>line1\nline2</code></pre>")
	want := "```\nline1\nline2\n```"
	if got != want {
		t.Errorf("code block:\n got %q\nwant %q", got, want)
	}
}

func TestFromHTML_Table(t *testing.T) {
	got := FromHTML(`<table><tbody>
	  <tr><th>H1</th><th>H2</th></tr>
	  <tr><td>a</td><td>b</td></tr>
	</tbody></table>`)
	want := "| H1 | H2 |\n| --- | --- |\n| a | b |"
	if got != want {
		t.Errorf("table:\n got %q\nwant %q", got, want)
	}
}

func TestExtractSection(t *testing.T) {
	body := "# Brief\n\nThe problem.\n\n## Logbook\n\n- [x] a\n- [ ] b\n\n## Notes\n\nmore"
	section, rest, found := ExtractSection(body, "logbook")
	if !found {
		t.Fatal("logbook not found")
	}
	if section != "- [x] a\n- [ ] b" {
		t.Errorf("section = %q", section)
	}
	// The Logbook heading+body is removed; Brief and Notes remain.
	if strings.Contains(rest, "Logbook") || !strings.Contains(rest, "Notes") || !strings.Contains(rest, "Brief") {
		t.Errorf("rest = %q", rest)
	}
}

func TestExtractSection_Missing(t *testing.T) {
	_, rest, found := ExtractSection("# Only\n\ntext", "logbook")
	if found {
		t.Error("should not find logbook")
	}
	if rest != "# Only\n\ntext" {
		t.Errorf("rest mutated: %q", rest)
	}
}

func TestSplit(t *testing.T) {
	// No H1 → single fallback-titled artifact.
	arts := Split("just a paragraph", "MyThread")
	if len(arts) != 1 || arts[0].Title != "MyThread" {
		t.Fatalf("fallback: %+v", arts)
	}

	// Multiple H1 with preamble → preamble + one per H1.
	body := "intro line\n\n# File A\n\n## Sub\n\ntext\n\n# File B\n\nbody b"
	arts = Split(body, "Brief")
	if len(arts) != 3 {
		t.Fatalf("want 3 artifacts, got %d: %+v", len(arts), arts)
	}
	if arts[0].Title != "Brief" || arts[1].Title != "File A" || arts[2].Title != "File B" {
		t.Errorf("titles: %q %q %q", arts[0].Title, arts[1].Title, arts[2].Title)
	}
	if strings.Contains(arts[1].Markdown, "# File A") {
		t.Error("artifact body should not include its own H1 heading")
	}
	// TOC of File A picks up the H2.
	wantTOC := []TOCEntry{{Level: 2, Title: "Sub", Slug: "sub"}}
	if !reflect.DeepEqual(arts[1].TOC, wantTOC) {
		t.Errorf("toc = %+v", arts[1].TOC)
	}
}

func TestParseTodos(t *testing.T) {
	got := ParseTodos("- [x] done\n- [ ] open\n- [X] also done")
	want := []Todo{{"done", true}, {"open", false}, {"also done", true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("checkbox todos = %+v", got)
	}
	// Plain bullets (no checkboxes) become open todos.
	got = ParseTodos("- first\n- second")
	want = []Todo{{"first", false}, {"second", false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bullet todos = %+v", got)
	}
	if ParseTodos("") != nil {
		t.Error("empty should be nil")
	}
}

func TestParseThread(t *testing.T) {
	body := "# Brief\n\nWhat and why.\n\n## Logbook\n\n### Phase 1\n\n- [x] scaffold\n- [ ] wire\n\n## Definition of Done\n\n- [ ] tests pass"
	arts, log := ParseThread(body, "Thread")
	if log == nil {
		t.Fatal("expected a logbook")
	}
	if !log.Phased {
		t.Error("logbook with a phase subheading should be phased")
	}
	if len(log.Todos) != 2 || log.Todos[0].Text != "scaffold" || !log.Todos[0].Done {
		t.Errorf("todos = %+v", log.Todos)
	}
	if len(log.DoD) != 1 || log.DoD[0].Text != "tests pass" {
		t.Errorf("dod = %+v", log.DoD)
	}
	// Logbook and DoD are pulled out of the artifacts.
	for _, a := range arts {
		if strings.Contains(a.Markdown, "scaffold") || strings.Contains(a.Markdown, "tests pass") {
			t.Errorf("artifact %q still contains logbook/dod content", a.Title)
		}
	}
	// The Brief survives as an artifact.
	if len(arts) == 0 || !strings.Contains(arts[0].Markdown, "What and why") {
		t.Errorf("brief artifact missing: %+v", arts)
	}
}

func TestParseThread_SimpleNoPhases(t *testing.T) {
	body := "Do the thing.\n\n## Logbook\n\n- [ ] step one\n- [ ] step two"
	_, log := ParseThread(body, "Thread")
	if log == nil || log.Phased {
		t.Errorf("flat task list should be simple (not phased): %+v", log)
	}
}

// A realistic Plane birth page (as briefLogbookHTML would emit) round-trips.
func TestFromHTML_BirthPage(t *testing.T) {
	html := `<h2>Brief</h2><p>Ship the thing.</p><h2>Logbook</h2><p>plan</p>`
	got := FromHTML(html)
	want := "## Brief\n\nShip the thing.\n\n## Logbook\n\nplan"
	if got != want {
		t.Errorf("birth page:\n got %q\nwant %q", got, want)
	}
}

// CountDone counts ticked items across the Logbook AND the Definition of Done —
// it is what the refresher diffs to notice a thread produced something.
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
	// Plain bullets are not checklist items, so they never count as done.
	if got := CountDone("## Logbook\n- just a note\n- another"); got != 0 {
		t.Fatalf("bullets counted as done: %d", got)
	}
	// A thread with no logbook at all has produced nothing.
	if got := CountDone("# Brief\nNo logbook here."); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}
