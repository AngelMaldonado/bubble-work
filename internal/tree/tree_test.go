package tree

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTree(t *testing.T) *Tree {
	t.Helper()
	tr, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.EnsureRepo("alpha"); err != nil {
		t.Fatal(err)
	}
	return tr
}

// The claim: a document path cannot reach outside its workspace, whatever shape
// it is written in. This is the boundary the whole store rests on.
func TestInvariant_Tree_PathsStayInsideTheWorkspace(t *testing.T) {
	tr := newTree(t)
	for _, doc := range []string{
		"../escape.md",
		"a/../../escape.md",
		"a/b/../../../../escape.md",
		"/etc/passwd.md",
		"..%2Fescape.md/../../x.md",
		"",
	} {
		if _, err := tr.Write("alpha", doc, Hash(""), "x", "a@b", "m"); err == nil {
			t.Errorf("%q was accepted; it must not be", doc)
		}
	}
	// And the repo name itself cannot climb out either.
	if err := tr.EnsureRepo("../elsewhere"); err == nil {
		t.Error("a repo above the root was accepted")
	}
}

// The claim: the layout decides what may exist and where. A tree where files can
// appear anywhere is a tree nobody can reason about — and attribution would have
// nowhere to come from.
func TestInvariant_Tree_LayoutIsEnforced(t *testing.T) {
	ok := []struct {
		doc  string
		area Area
	}{
		{"README.md", AreaDoc},
		{"threads/1-uno.md", AreaThread},
		{"threads/12-con-nombre-largo.md", AreaThread},
		{"docs/onboarding.md", AreaDoc},
		{"docs/arquitectura/decisiones.md", AreaDoc}, // docs/ anida
		{"docs/a/b/c/d/hondo.md", AreaDoc},
		{"docs/diagrama.excalidraw", AreaDoc},    // el JSON del plugin
		{"docs/diagrama.excalidraw.md", AreaDoc}, // y su forma markdown
	}
	for _, c := range ok {
		got, err := Classify(c.doc)
		if err != nil {
			t.Errorf("%q was refused: %v", c.doc, err)
			continue
		}
		if got != c.area {
			t.Errorf("%q classified as %q, want %q", c.doc, got, c.area)
		}
	}

	bad := []string{
		"notes.txt",              // no es una extensión que vivamos
		"suelto.md",              // en la raíz solo va README
		"otro/sitio.md",          // un directorio que no es del layout
		"threads/sub/anidado.md", // threads/ NO anida
		"docs/foto.png",          // los binarios quieren otra puerta
		"threads",                // un directorio no es un documento
		"../fuera.md",
	}
	for _, doc := range bad {
		if area, err := Classify(doc); err == nil {
			t.Errorf("%q was accepted as %q; it must not be", doc, area)
		}
	}
}

func TestTree_WriteReadRoundTrip(t *testing.T) {
	tr := newTree(t)
	body := "# Uno\n\n- [ ] algo\n"
	h, err := tr.Write("alpha", "threads/1-uno.md", Hash(""), body, "alice@b", "born: uno")
	if err != nil {
		t.Fatal(err)
	}
	got, gotHash, err := tr.Read("alpha", "threads/1-uno.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != body {
		t.Errorf("read back %q, wrote %q", got, body)
	}
	if gotHash != h {
		t.Errorf("hash %q != returned %q", gotHash, h)
	}
}

// A file that is not there reads as empty, so a first write and a rewrite are the
// same call to whoever is above.
func TestTree_MissingReadsAsEmpty(t *testing.T) {
	tr := newTree(t)
	got, h, err := tr.Read("alpha", "threads/nope.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" || h != Hash("") {
		t.Errorf("got %q/%q, want empty", got, h)
	}
}

// The claim: a write carrying a stale base is refused rather than silently
// overwriting. Replacing a whole file to change one line is how one writer
// destroys a paragraph they never read.
func TestInvariant_Tree_StaleWriteIsAConflict(t *testing.T) {
	tr := newTree(t)
	doc := "threads/1.md"
	h1, err := tr.Write("alpha", doc, Hash(""), "primera\n", "alice@b", "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Write("alpha", doc, h1, "segunda\n", "bob@b", "two"); err != nil {
		t.Fatal(err)
	}
	// alice still holds h1, which is now stale.
	_, err = tr.Write("alpha", doc, h1, "de alice\n", "alice@b", "three")
	if err == nil {
		t.Fatal("a stale write was accepted")
	}
	body, _, _ := tr.Read("alpha", doc)
	if body != "segunda\n" {
		t.Errorf("the conflicting write landed anyway: %q", body)
	}
}

func TestTree_NoOpWriteIsNotACommit(t *testing.T) {
	tr := newTree(t)
	doc := "threads/1.md"
	h, _ := tr.Write("alpha", doc, Hash(""), "igual\n", "a@b", "one")
	before, _ := tr.Log("alpha", doc, 20)
	if _, err := tr.Write("alpha", doc, h, "igual\n", "a@b", "two"); err != nil {
		t.Fatal(err)
	}
	after, _ := tr.Log("alpha", doc, 20)
	if len(after) != len(before) {
		t.Errorf("a no-op write recorded a commit: %d -> %d", len(before), len(after))
	}
}

// The claim: every write is one commit, authored as whoever made it. Git is the
// content history, so "who wrote this" needs no second record.
func TestInvariant_Tree_EveryWriteIsACommitByItsActor(t *testing.T) {
	tr := newTree(t)
	doc := "threads/1.md"
	h, _ := tr.Write("alpha", doc, Hash(""), "uno\n", "alice@bubble.test", "born")
	if _, err := tr.Write("alpha", doc, h, "dos\n", "bob@bubble.test", "edited"); err != nil {
		t.Fatal(err)
	}
	log, err := tr.Log("alpha", doc, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 {
		t.Fatalf("want 2 commits, got %d: %v", len(log), log)
	}
	if !strings.Contains(log[0], "bob@bubble.test") || !strings.Contains(log[0], "edited") {
		t.Errorf("newest commit is not bob's edit: %q", log[0])
	}
	if !strings.Contains(log[1], "alice@bubble.test") {
		t.Errorf("first commit is not alice's: %q", log[1])
	}
}

func TestTree_MoveKeepsHistory(t *testing.T) {
	tr := newTree(t)
	h, _ := tr.Write("alpha", "threads/1-uno.md", Hash(""), "cuerpo\n", "a@b", "born")
	_ = h
	if err := tr.Move("alpha", "threads/1-uno.md", "threads/1-dos.md", "a@b", "moved"); err != nil {
		t.Fatal(err)
	}
	if body, _, _ := tr.Read("alpha", "threads/1-dos.md"); body != "cuerpo\n" {
		t.Errorf("content did not survive the move: %q", body)
	}
	if body, _, _ := tr.Read("alpha", "threads/1-uno.md"); body != "" {
		t.Error("the old path still has content")
	}
	// --follow is what makes the history survive the rename.
	log, _ := tr.Log("alpha", "threads/1-dos.md", 20)
	if len(log) < 2 {
		t.Errorf("history did not follow the rename: %v", log)
	}
}

func TestTree_Remove(t *testing.T) {
	tr := newTree(t)
	h, _ := tr.Write("alpha", "threads/1.md", Hash(""), "x\n", "a@b", "born")
	_ = h
	if err := tr.Remove("alpha", "threads/1.md", "a@b", "deleted"); err != nil {
		t.Fatal(err)
	}
	if body, _, _ := tr.Read("alpha", "threads/1.md"); body != "" {
		t.Error("still there after remove")
	}
	// Removing again is not an error: the caller wanted it gone and it is.
	if err := tr.Remove("alpha", "threads/1.md", "a@b", "again"); err != nil {
		t.Errorf("second remove errored: %v", err)
	}
}

// The claim: two writers to the same file serialise, and exactly one of a racing
// pair wins. Without the per-path lock they interleave read-check-write and both
// believe they were first.
func TestInvariant_Tree_ConcurrentWritersSerialise(t *testing.T) {
	tr := newTree(t)
	doc := "threads/hot.md"
	base, _ := tr.Write("alpha", doc, Hash(""), "base\n", "a@b", "born")

	const writers = 8
	var wg sync.WaitGroup
	won := make([]bool, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// All of them read the same base, so exactly one may land.
			_, err := tr.Write("alpha", doc, base, "de "+string(rune('a'+i))+"\n", "a@b", "race")
			won[i] = err == nil
		}(i)
	}
	wg.Wait()

	n := 0
	for _, w := range won {
		if w {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d writers won; exactly 1 must", n)
	}
}

// Different files must not queue behind each other.
func TestTree_DifferentFilesDoNotBlock(t *testing.T) {
	tr := newTree(t)
	var wg sync.WaitGroup
	errs := make([]error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			doc := filepath.Join("threads", string(rune('a'+i))+".md")
			_, errs[i] = tr.Write("alpha", doc, Hash(""), "x\n", "a@b", "born")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d failed: %v", i, err)
		}
	}
}

// The repository is a plain git repository somebody can open with their own git.
func TestTree_IsARealGitRepo(t *testing.T) {
	tr := newTree(t)
	if _, err := tr.Write("alpha", "threads/1.md", Hash(""), "x\n", "a@b", "born"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tr.Root(), "alpha", ".git")); err != nil {
		t.Fatalf("no .git: %v", err)
	}
	out, err := run(filepath.Join(tr.Root(), "alpha"), "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("the working tree is dirty after a write: %q", out)
	}
}

// The directory IS the index: no row mirrors these files, so the listing has to be
// the walk. A file's title is its name — naming a document is the whole of naming
// it.
func TestTree_TreeListing(t *testing.T) {
	tr := newTree(t)
	for _, doc := range []string{
		"README.md",
		"threads/1-uno.md",
		"docs/onboarding.md",
		"docs/arquitectura/decisiones.md",
	} {
		if _, err := tr.Write("alpha", doc, Hash(""), "x\n", "a@b", "born"); err != nil {
			t.Fatalf("%s: %v", doc, err)
		}
	}
	entries, err := tr.Tree("alpha")
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Entry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	for _, want := range []string{"README.md", "threads", "threads/1-uno.md",
		"docs", "docs/onboarding.md", "docs/arquitectura", "docs/arquitectura/decisiones.md"} {
		if _, ok := byPath[want]; !ok {
			t.Errorf("%q missing from the tree", want)
		}
	}
	if e := byPath["docs/onboarding.md"]; e.Title != "onboarding" || e.Area != AreaDoc || e.Dir {
		t.Errorf("docs/onboarding.md = %+v", e)
	}
	if e := byPath["threads/1-uno.md"]; e.Area != AreaThread {
		t.Errorf("threads file classified as %q", e.Area)
	}
	if e := byPath["docs"]; !e.Dir {
		t.Error("docs is not reported as a directory")
	}
	// git's own directory is machinery, not content.
	for p := range byPath {
		if strings.HasPrefix(p, ".git") {
			t.Errorf("%q leaked into the tree", p)
		}
	}
}

func TestTree_Search(t *testing.T) {
	tr := newTree(t)
	write := func(doc, body string) {
		if _, err := tr.Write("alpha", doc, Hash(""), body, "a@b", "born"); err != nil {
			t.Fatalf("%s: %v", doc, err)
		}
	}
	write("threads/1-uno.md", "# Uno\n\nel presupuesto de rate limiting\n")
	write("docs/onboarding.md", "# Onboarding\n\nnada que ver\n")
	write("docs/guias/api.md", "# API\n\nhablar del PRESUPUESTO otra vez\n")

	hits, err := tr.Search("alpha", "presupuesto", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2: %+v", len(hits), hits)
	}
	// Case-insensitive, and threads come first because somebody searching is
	// usually looking for work.
	if hits[0].Path != "threads/1-uno.md" || hits[0].Area != AreaThread {
		t.Errorf("first hit is %+v, want the thread", hits[0])
	}
	if hits[0].Line != 3 || !strings.Contains(hits[0].Text, "rate limiting") {
		t.Errorf("hit does not carry its line and text: %+v", hits[0])
	}
	if hits[1].Path != "docs/guias/api.md" {
		t.Errorf("second hit is %+v, want the nested doc", hits[1])
	}
	if got, _ := tr.Search("alpha", "no-existe-en-ningun-lado", 50); len(got) != 0 {
		t.Errorf("a miss returned %d hits", len(got))
	}
	if got, _ := tr.Search("alpha", "  ", 50); len(got) != 0 {
		t.Errorf("an empty needle returned %d hits", len(got))
	}
	// The limit is a cap, not a suggestion.
	if got, _ := tr.Search("alpha", "e", 1); len(got) != 1 {
		t.Errorf("limit 1 returned %d hits", len(got))
	}
	// git's own files are not documents.
	if got, _ := tr.Search("alpha", "ref:", 50); len(got) != 0 {
		t.Errorf(".git leaked into search: %+v", got)
	}
}
