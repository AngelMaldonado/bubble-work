package mirror

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/store"
)

func openTestMirror(t *testing.T) *Mirror {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	m, err := New(st.DB())
	if err != nil {
		t.Fatalf("mirror open: %v", err)
	}
	return m
}

func item(id string, seq int, name string) Item {
	return Item{
		ID: id, ProjectID: "p1", Seq: seq, Name: name,
		StateGroup: "started", Assignees: []string{"u1"},
		DescriptionHTML: "<p>body " + id + "</p>",
		DescriptionHash: HashBody("<p>body " + id + "</p>"),
		CreatedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestItemRoundTrip(t *testing.T) {
	m := openTestMirror(t)
	now := time.Now()
	done := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	in := item("i1", 7, "Wire up auth")
	in.CompletedAt = &done
	in.Priority = "high"
	in.ParentID = "parent-1"
	in.Assignees = []string{"u1", "u2"}

	if err := m.UpsertItems("cuby", []Item{in}, now); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, ok, err := m.Item("cuby", "i1")
	if err != nil || !ok {
		t.Fatalf("read back: ok=%v err=%v", ok, err)
	}
	if got.Name != in.Name || got.Seq != in.Seq || got.Priority != in.Priority {
		t.Errorf("scalars lost: %+v", got)
	}
	if got.ParentID != "parent-1" {
		t.Errorf("parent = %q, want parent-1", got.ParentID)
	}
	if len(got.Assignees) != 2 || got.Assignees[0] != "u1" || got.Assignees[1] != "u2" {
		t.Errorf("assignees = %v, want [u1 u2]", got.Assignees)
	}
	if !got.CreatedAt.Equal(in.CreatedAt) || !got.UpdatedAt.Equal(in.UpdatedAt) {
		t.Errorf("timestamps drifted: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(done) {
		t.Errorf("completed_at = %v, want %v", got.CompletedAt, done)
	}
	if got.DescriptionHTML != in.DescriptionHTML {
		t.Error("body did not survive the round trip")
	}
}

// A nil CompletedAt must come back nil, not as the zero time — the difference is
// "still open" vs "finished at year zero", and heat reads it.
func TestOpenItemHasNoCompletedAt(t *testing.T) {
	m := openTestMirror(t)
	if err := m.UpsertItems("cuby", []Item{item("i1", 1, "open")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _, err := m.Item("cuby", "i1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil for an open item", got.CompletedAt)
	}
}

func TestUpsertIsIdempotent(t *testing.T) {
	m := openTestMirror(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := m.UpsertItems("cuby", []Item{item("i1", 1, "same")}, now); err != nil {
			t.Fatal(err)
		}
	}
	c, err := m.Counts("cuby")
	if err != nil {
		t.Fatal(err)
	}
	if c.Items != 1 {
		t.Errorf("items = %d after 3 identical upserts, want 1", c.Items)
	}
}

// One database holds every federated instance; a slug must scope everything.
func TestInstancesAreIsolated(t *testing.T) {
	m := openTestMirror(t)
	now := time.Now()
	if err := m.UpsertItems("cuby", []Item{item("shared-id", 1, "cuby's")}, now); err != nil {
		t.Fatal(err)
	}
	if err := m.UpsertItems("ayetec", []Item{item("shared-id", 2, "ayetec's")}, now); err != nil {
		t.Fatal(err)
	}
	a, _, _ := m.Item("cuby", "shared-id")
	b, _, _ := m.Item("ayetec", "shared-id")
	if a.Name != "cuby's" || b.Name != "ayetec's" {
		t.Errorf("instances collided: %q / %q", a.Name, b.Name)
	}
	if err := m.Reset("cuby"); err != nil {
		t.Fatal(err)
	}
	if c, _ := m.Counts("cuby"); c.Items != 0 {
		t.Errorf("cuby survived its own reset: %d items", c.Items)
	}
	if c, _ := m.Counts("ayetec"); c.Items != 1 {
		t.Errorf("resetting cuby took ayetec with it: %d items", c.Items)
	}
}

// Module membership is a SET, not a stream: Plane gives no "removed" signal, so
// re-stating the set is the only way a removal lands.
func TestSetModuleItemsReplaces(t *testing.T) {
	m := openTestMirror(t)
	now := time.Now()
	items := []Item{item("i1", 1, "a"), item("i2", 2, "b"), item("i3", 3, "c")}
	if err := m.UpsertItems("cuby", items, now); err != nil {
		t.Fatal(err)
	}
	if err := m.SetModuleItems("cuby", "mod1", []string{"i1", "i2", "i3"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.ModuleItems("cuby", "mod1"); len(got) != 3 {
		t.Fatalf("membership = %d, want 3", len(got))
	}
	// i2 moved out of the module in Plane.
	if err := m.SetModuleItems("cuby", "mod1", []string{"i1", "i3"}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.ModuleItems("cuby", "mod1")
	if len(got) != 2 {
		t.Fatalf("membership = %d after removal, want 2", len(got))
	}
	for _, it := range got {
		if it.ID == "i2" {
			t.Error("i2 is still in the module after being removed from the set")
		}
	}
	// The item itself must survive — it left the module, it was not deleted.
	if _, ok, _ := m.Item("cuby", "i2"); !ok {
		t.Error("removing i2 from a module deleted the work item")
	}
}

func TestPruneKeepsWhatPlaneStillHas(t *testing.T) {
	m := openTestMirror(t)
	now := time.Now()
	if err := m.UpsertItems("cuby",
		[]Item{item("i1", 1, "a"), item("i2", 2, "b"), item("i3", 3, "c")}, now); err != nil {
		t.Fatal(err)
	}
	n, err := m.PruneItems("cuby", "p1", []string{"i1", "i3"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}
	if _, ok, _ := m.Item("cuby", "i2"); ok {
		t.Error("i2 survived a prune that did not keep it")
	}
	if _, ok, _ := m.Item("cuby", "i1"); !ok {
		t.Error("prune removed an item it was told to keep")
	}
}

// An emptied project is a real state, so an empty keep-set must clear the
// project rather than being treated as a failed fetch.
func TestPruneWithEmptyKeepClearsProject(t *testing.T) {
	m := openTestMirror(t)
	if err := m.UpsertItems("cuby", []Item{item("i1", 1, "a")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	n, err := m.PruneItems("cuby", "p1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}
}

func TestCommentsAndPulse(t *testing.T) {
	m := openTestMirror(t)
	t0 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	cs := []Comment{
		{ID: "c1", ActorID: "u1", HTML: "<p>first</p>", CreatedAt: t0},
		{ID: "c2", ActorID: "u2", HTML: "<p>second</p>", CreatedAt: t0.Add(time.Hour)},
	}
	if err := m.ReplaceComments("cuby", "i1", cs); err != nil {
		t.Fatal(err)
	}
	got, err := m.Comments("cuby", "i1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "c1" {
		t.Fatalf("comments = %+v, want c1 then c2", got)
	}
	// This one query is what retires the 20-calls-per-tick pulse probe.
	last, err := m.LastCommentAt("cuby", "i1")
	if err != nil {
		t.Fatal(err)
	}
	if !last.Equal(t0.Add(time.Hour)) {
		t.Errorf("pulse = %v, want %v", last, t0.Add(time.Hour))
	}
	// A deleted comment has no signal of its own, so a replace is how it goes.
	if err := m.ReplaceComments("cuby", "i1", cs[:1]); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Comments("cuby", "i1"); len(got) != 1 {
		t.Errorf("comments = %d after replace, want 1", len(got))
	}
	if last, _ := m.LastCommentAt("cuby", "i1"); !last.Equal(t0) {
		t.Errorf("pulse did not roll back with the deleted comment: %v", last)
	}
}

func TestPulseOfSilentThreadIsZero(t *testing.T) {
	m := openTestMirror(t)
	last, err := m.LastCommentAt("cuby", "never-commented")
	if err != nil {
		t.Fatalf("err = %v, want a zero time and no error", err)
	}
	if !last.IsZero() {
		t.Errorf("pulse = %v, want zero", last)
	}
}

// Whitespace-only differences are reflow, not an edit — the same discipline as
// md.LogbookFingerprint, or every re-render would read as progress.
func TestHashBodyIgnoresReflow(t *testing.T) {
	a := HashBody("<p>hello   world</p>\n<p>next</p>")
	b := HashBody("<p>hello world</p>\n\n   <p>next</p>")
	if a != b {
		t.Errorf("reflow changed the hash: %s vs %s", a, b)
	}
	if HashBody("") != "" {
		t.Error("an empty body should hash to the empty string, not a digest")
	}
	if HashBody("<p>x</p>") == HashBody("<p>y</p>") {
		t.Error("different bodies collided")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	m := openTestMirror(t)
	if c, err := m.Cursor("cuby", "items"); err != nil || !c.Watermark.IsZero() {
		t.Fatalf("fresh cursor = %+v, %v; want zero", c, err)
	}
	wm := time.Date(2026, 8, 4, 13, 30, 0, 0, time.UTC)
	in := Cursor{Watermark: wm, LastFull: wm.Add(-time.Hour), LastOK: wm, LastError: "boom"}
	if err := m.SetCursor("cuby", "items", in); err != nil {
		t.Fatal(err)
	}
	got, err := m.Cursor("cuby", "items")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Watermark.Equal(wm) || !got.LastFull.Equal(in.LastFull) || got.LastError != "boom" {
		t.Errorf("cursor = %+v, want %+v", got, in)
	}
}

func TestChildrenAndStates(t *testing.T) {
	m := openTestMirror(t)
	now := time.Now()
	parent := item("p", 1, "parent")
	kid := item("k", 2, "rev: first pass")
	kid.ParentID = "p"
	if err := m.UpsertItems("cuby", []Item{parent, kid}, now); err != nil {
		t.Fatal(err)
	}
	kids, err := m.Children("cuby", "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 1 || kids[0].ID != "k" {
		t.Errorf("children = %+v, want just k", kids)
	}

	if err := m.UpsertStates("cuby", []State{
		{ID: "s1", ProjectID: "p1", Name: "En Progreso", Group: "started", Default: true},
	}); err != nil {
		t.Fatal(err)
	}
	states, err := m.States("cuby", "p1")
	if err != nil {
		t.Fatal(err)
	}
	// The name is localized; only the group is safe to match on.
	if states["s1"].Group != "started" || !states["s1"].Default {
		t.Errorf("state = %+v", states["s1"])
	}
}
