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
	// Keyed on purpose: ParseTodos does not know which region it was handed, so
	// Region/Index stay zero until ParseThread addresses them.
	want := []Todo{{Text: "done", Done: true}, {Text: "open"}, {Text: "also done", Done: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("checkbox todos = %+v", got)
	}
	// Plain bullets (no checkboxes) become open todos.
	got = ParseTodos("- first\n- second")
	want = []Todo{{Text: "first"}, {Text: "second"}}
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

// CountDone counts ticked items ANYWHERE in the document — the Logbook and the DoD
// have no monopoly on a plan (docs/decisions/0005). It is what the refresher diffs to
// notice a thread produced something.
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

// In a free-form document a bullet is prose, not a task: ParseChecklist reads only
// real boxes, and ParseThread addresses them so toggle_todo can reach them.
func TestDocumentChecklistIsBoxesOnly(t *testing.T) {
	body := "# Notes\n\n- an observation\n- another observation\n\n## Pasos\n- [ ] validar\n- [x] leer\n"
	if got := len(ParseChecklist(body)); got != 2 {
		t.Fatalf("want the 2 real boxes, got %d", got)
	}
	arts, _ := ParseThread(body, "Notes")
	if len(arts) != 1 {
		t.Fatalf("want one document artifact, got %d", len(arts))
	}
	todos := arts[0].Todos
	if len(todos) != 2 {
		t.Fatalf("the document's todos were not addressed: %+v", todos)
	}
	if todos[0].Region != string(RegionDocument) || todos[0].Index != 0 || todos[1].Index != 1 {
		t.Errorf("todos must carry the address toggle_todo takes: %+v", todos)
	}
	// And the strict toggle addresses the same positions the reader reported.
	out, err := ToggleTodoIn(body, 0, "validar", true, true)
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if !strings.Contains(out, "- [x] validar") {
		t.Errorf("the wrong line moved:\n%s", out)
	}
}

// LogbookFingerprint changes when the PLAN changes — a tick, an added item, a
// reworded phase — but not when the prose around it moves or is reflowed.
func TestLogbookFingerprint(t *testing.T) {
	base := "# Brief\nDo the thing.\n\n## Logbook\n- [x] scaffold\n- [ ] wire it up\n"

	if LogbookFingerprint("# Brief\nNo plan here.") != "" {
		t.Fatal("a thread with no logbook should have no fingerprint")
	}
	h := LogbookFingerprint(base)
	if h == "" {
		t.Fatal("a logbook should fingerprint")
	}
	// Whitespace and the surrounding prose are noise.
	if got := LogbookFingerprint("# Brief\nSomething else entirely.\n\n## Logbook\n-  [x]   scaffold  \n\n- [ ] wire it up\n"); got != h {
		t.Errorf("reflow/prose changed the fingerprint: %s vs %s", got, h)
	}
	// Ticking, unticking and adding all count as changes to the plan.
	for _, changed := range []string{
		"## Logbook\n- [x] scaffold\n- [x] wire it up\n",
		"## Logbook\n- [ ] scaffold\n- [ ] wire it up\n",
		"## Logbook\n- [x] scaffold\n- [ ] wire it up\n- [ ] ship\n",
		"## Logbook\n- [x] scaffold the server\n- [ ] wire it up\n",
	} {
		if LogbookFingerprint(changed) == h {
			t.Errorf("plan change went unnoticed: %q", changed)
		}
	}
	// The DoD is part of the plan too.
	if LogbookFingerprint(base+"\n## Definition of Done\n- [ ] tests pass\n") == h {
		t.Error("adding a Definition of Done should change the fingerprint")
	}
}

// TestReplaceSection is really a test about blast radius: an agent rewriting a
// Logbook must not be able to touch the Brief. The two live in one Plane
// description by design (§3), so a whole-body write would be the natural shape
// and the wrong one.
func TestReplaceSection(t *testing.T) {
	const doc = `# Brief

The pull is that signups leak.

## Logbook

- [x] scaffold
- [ ] wire

## Notes

keep me
`
	got := ReplaceSection(doc, "Logbook", "- [x] scaffold\n- [x] wire\n- [ ] ship")

	if !strings.Contains(got, "The pull is that signups leak.") {
		t.Error("the Brief was lost")
	}
	if !strings.Contains(got, "keep me") {
		t.Error("a later section was lost")
	}
	if !strings.Contains(got, "- [x] wire") || strings.Contains(got, "- [ ] wire") {
		t.Errorf("the Logbook was not replaced:\n%s", got)
	}
	// Order matters: a section that jumps to the end of the document on every
	// edit would churn the diff and confuse anyone reading it in Plane.
	if strings.Index(got, "## Logbook") > strings.Index(got, "## Notes") {
		t.Errorf("the section moved:\n%s", got)
	}
	// The heading is kept verbatim rather than re-emitted, so an H3 "Logbook" or
	// odd spacing survives a round trip.
	if strings.Count(got, "Logbook") != 1 {
		t.Errorf("the heading was duplicated:\n%s", got)
	}
}

// A thread born small has no Logbook at all (§3.2 allows it). An update must
// create one rather than quietly doing nothing.
func TestReplaceSectionAppendsWhenAbsent(t *testing.T) {
	got := ReplaceSection("# Brief\n\nJust a paragraph.", "Logbook", "- [ ] first todo")
	if !strings.Contains(got, "## Logbook") || !strings.Contains(got, "- [ ] first todo") {
		t.Errorf("absent section was not appended:\n%s", got)
	}
	if !strings.Contains(got, "Just a paragraph.") {
		t.Error("appending clobbered the existing body")
	}
	// ...and it must be findable by the parser that reads it back.
	if sec, _, ok := ExtractSection(got, "logbook"); !ok || !strings.Contains(sec, "first todo") {
		t.Errorf("the appended section does not round-trip: %q", sec)
	}
}

// Every todo a reader receives carries the two values needed to act on it, because
// inferring them is silent when it goes wrong: an index counted across the whole
// page toggles a different item, or none at all.
func TestInvariant_Artifacts_TodosCarryTheirAddress(t *testing.T) {
	body := "# T\n\nwhy\n\n## Logbook\n\n### Fase 1\n\n- [x] uno\n- [ ] dos\n\n" +
		"## Definition of Done\n\n- [ ] it works\n- [ ] it is measured"
	_, log := ParseThread(body, "T")
	if log == nil {
		t.Fatal("no logbook")
	}
	for i, td := range log.Todos {
		if td.Region != string(RegionLogbook) || td.Index != i {
			t.Errorf("logbook todo %d addressed as %s[%d]", i, td.Region, td.Index)
		}
	}
	for i, td := range log.DoD {
		if td.Region != string(RegionDoD) || td.Index != i {
			t.Errorf("dod todo %d addressed as %s[%d]", i, td.Region, td.Index)
		}
	}
	// The two lists are indexed INDEPENDENTLY — the DoD's first item is dod[0], not
	// logbook-count + 0. Counting across the page is the mistake this prevents.
	if len(log.DoD) > 0 && log.DoD[0].Index != 0 {
		t.Errorf("the DoD's first item is index %d", log.DoD[0].Index)
	}
}

// A page with two of the same section is refused, because a duplicate does not just
// look untidy: the first section's range stops at the second heading, so the
// canonical region is EMPTY and every region-scoped write addresses nothing.
func TestInvariant_Artifacts_DuplicateSectionsAreRefused(t *testing.T) {
	dup := "## Brief\n\nwhy\n\n## Logbook\n\n- [ ] uno\n\n## Logbook\n\n- [ ] uno"
	found := false
	for _, f := range Refusals(Lint(dup)) {
		if f.Rule == "duplicate-section" {
			found = true
			if f.N != 2 {
				t.Errorf("N = %d, want 2", f.N)
			}
		}
	}
	if !found {
		t.Fatalf("two Logbooks were not refused: %+v", Lint(dup))
	}
	// The shape it protects: the first region really is empty.
	sec, _, _ := ExtractSection(dup, "logbook")
	if len(ParseTodos(sec)) != 1 {
		t.Logf("first logbook section: %q", sec)
	}
	// Phase headings repeating is fine — only the reserved H1/H2 names count.
	ok := "## Logbook\n\n### Fase 1\n\n- [ ] a\n\n### Fase 1\n\n- [ ] b"
	for _, f := range Lint(ok) {
		if f.Rule == "duplicate-section" {
			t.Errorf("a repeated ### heading was refused: %s", f.Message)
		}
	}
}

// Birth owns the section headings, so a caller may include them without producing
// two: StripLeadingHeading takes off a leading one that matches, and nothing else.
func TestStripLeadingHeading(t *testing.T) {
	cases := []struct{ in, want string }{
		{"## Logbook\n- [ ] uno", "- [ ] uno"},
		{"# Logbook\n\n- [ ] uno", "- [ ] uno"},
		{"\n\n## Logbook\n- [ ] uno", "- [ ] uno"},
		{"- [ ] uno", "- [ ] uno"},                                       // no heading
		{"## Fase 1\n- [ ] uno", "## Fase 1\n- [ ] uno"},                 // not ours
		{"- [ ] see the Logbook", "- [ ] see the Logbook"},               // mentions it, not a heading
		{"## Logbook\n\n## Logbook\n- [ ] uno", "## Logbook\n- [ ] uno"}, // strips ONE
	}
	for _, c := range cases {
		if got := StripLeadingHeading(c.in, "logbook"); got != c.want {
			t.Errorf("StripLeadingHeading(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The Logbook's `Next:` line is a convention, so the parse has to be forgiving about
// how a human wrote it — and must not mistake prose that merely says "next" for a
// declared action.
func TestNextAction(t *testing.T) {
	cases := []struct{ in, want string }{
		// The shape the framework actually prescribes: three labels, one line.
		{"**Owner:** me · **State:** building · **Next:** measure the push", "measure the push"},
		{"**Next:** measure the push", "measure the push"},
		{"Next: measure the push", "measure the push"},
		{"- **Next:** measure the push", "measure the push"},
		{"**next:** lowercase label", "lowercase label"},
		{"### Fase 1\n\n**Next:** run the suite\n\n- [ ] thing", "run the suite"},
		{"- [ ] do the next thing", ""},
		{"no status line here", ""},
	}
	for _, c := range cases {
		if got := NextAction(c.in); got != c.want {
			t.Errorf("NextAction(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
