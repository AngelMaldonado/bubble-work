package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// fakePlane is a minimal stand-in for the endpoints the syncer uses. It records
// every path so tests can assert on CALL COUNT — the point of the whole refactor
// is spending fewer calls, so "did the delta actually stay cheap" is a first
// class assertion here.
type fakePlane struct {
	srv      *httptest.Server
	items    []map[string]any // newest-first is enforced by the handler
	modules  []map[string]any
	member   []map[string]any
	states   []map[string]any
	comments map[string][]map[string]any
	modItems map[string][]string
	calls    int64
	paths    []string
	fail429  string // when set, any path containing this 429s
}

func newFakePlane(t *testing.T) *fakePlane {
	t.Helper()
	f := &fakePlane{
		comments: map[string][]map[string]any{},
		modItems: map[string][]string{},
		member:   []map[string]any{{"id": "u1", "email": "a@b.c", "display_name": "A", "role": 20}},
		states: []map[string]any{
			{"id": "s-start", "name": "En Progreso", "group": "started", "default": false},
		},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&f.calls, 1)
		f.paths = append(f.paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		if f.fail429 != "" && strings.Contains(p, f.fail429) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		switch {
		case strings.HasSuffix(p, "/projects/"):
			page(w, []map[string]any{{"id": "p1", "name": "Proj", "identifier": "PRJ"}})
		case strings.HasSuffix(p, "/modules/"):
			page(w, f.modules)
		case strings.HasSuffix(p, "/module-issues/"):
			id := segAfter(p, "modules")
			var out []map[string]any
			for _, it := range f.modItems[id] {
				out = append(out, map[string]any{"id": it})
			}
			page(w, out)
		case strings.HasSuffix(p, "/states/"):
			page(w, f.states)
		case strings.HasSuffix(p, "/members/"):
			json.NewEncoder(w).Encode(f.member)
		case strings.HasSuffix(p, "/comments/"):
			id := segAfter(p, "work-items")
			page(w, f.comments[id])
		case strings.HasSuffix(p, "/work-items/"):
			// Always newest-first; the client early-stops at its watermark.
			sorted := append([]map[string]any(nil), f.items...)
			sort.Slice(sorted, func(i, j int) bool {
				return fmt.Sprint(sorted[i]["updated_at"]) > fmt.Sprint(sorted[j]["updated_at"])
			})
			page(w, sorted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func page(w http.ResponseWriter, results any) {
	if results == nil {
		results = []map[string]any{}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"results": results, "next_page_results": false, "next_cursor": "",
	})
}

func segAfter(path, key string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, s := range parts {
		if s == key && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (f *fakePlane) callCount() int { return int(atomic.LoadInt64(&f.calls)) }
func (f *fakePlane) reset()         { atomic.StoreInt64(&f.calls, 0); f.paths = nil }

func (f *fakePlane) addItem(id string, seq int, name, body string, updated time.Time) {
	f.items = append(f.items, map[string]any{
		"id": id, "project": "p1", "name": name, "description_html": body,
		"sequence_id": seq, "priority": "none", "parent": nil, "assignees": []string{"u1"},
		"created_at": updated.Format(time.RFC3339Nano),
		"updated_at": updated.Format(time.RFC3339Nano),
		"state":      map[string]any{"id": "s-start", "name": "En Progreso", "group": "started"},
	})
}

func (f *fakePlane) touchItem(id string, updated time.Time, body string) {
	for _, it := range f.items {
		if it["id"] == id {
			it["updated_at"] = updated.Format(time.RFC3339Nano)
			if body != "" {
				it["description_html"] = body
			}
			return
		}
	}
}

func newTestSyncer(t *testing.T) (*Syncer, *mirror.Mirror) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m, err := mirror.New(st.DB())
	if err != nil {
		t.Fatal(err)
	}
	return New(m), m
}

func inst(f *fakePlane) domain.Instance {
	return domain.Instance{Slug: "cuby", BaseURL: f.srv.URL, APIKey: "k", Workspace: "ws"}
}

func TestBackfillThenDeltaIsCheap(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.modules = []map[string]any{{"id": "m1", "name": "Bubble One"}}
	f.modItems["m1"] = []string{"i1", "i2"}
	f.addItem("i1", 1, "One", "<p>a</p>", base)
	f.addItem("i2", 2, "Two", "<p>b</p>", base.Add(time.Minute))

	s, m := newTestSyncer(t)
	res, err := s.Backfill(context.Background(), inst(f))
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.Items != 2 || res.Modules != 1 {
		t.Errorf("backfill = %+v, want 2 items / 1 module", res)
	}
	if !res.Full {
		t.Error("backfill did not report itself as a full pass")
	}
	if c, _ := m.Counts("cuby"); c.Items != 2 || c.Members != 1 || c.States != 1 {
		t.Errorf("mirror census = %+v", c)
	}

	// A delta with nothing changed must find nothing AND stay cheap. This is the
	// core economic claim of the whole refactor, so it is asserted numerically.
	f.reset()
	res, err = s.Delta(context.Background(), inst(f))
	if err != nil {
		t.Fatalf("delta: %v", err)
	}
	if res.Items != 0 {
		t.Errorf("quiet delta applied %d items, want 0", res.Items)
	}
	if n := f.callCount(); n > 3 {
		t.Errorf("quiet delta spent %d calls (%v); want <= 3", n, f.paths)
	}
}

// The watermark must come from the newest item OBSERVED, never from the clock —
// otherwise an item written while we were paging falls in the gap forever.
func TestWatermarkComesFromDataNotClock(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.addItem("i1", 1, "One", "<p>a</p>", base)

	s, m := newTestSyncer(t)
	// A clock far ahead of the data would silently swallow anything in between.
	s.SetClock(func() time.Time { return base.Add(72 * time.Hour) })
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	cur, err := m.Cursor("cuby", "items")
	if err != nil {
		t.Fatal(err)
	}
	if !cur.Watermark.Equal(base) {
		t.Errorf("watermark = %v, want the newest item's updated_at %v", cur.Watermark, base)
	}
}

func TestDeltaPicksUpAnEdit(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.addItem("i1", 1, "One", "<p>a</p>", base)

	s, m := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	f.touchItem("i1", base.Add(time.Hour), "<p>EDITED</p>")

	res, err := s.Delta(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if res.Items != 1 {
		t.Errorf("delta applied %d items, want 1", res.Items)
	}
	got, _, _ := m.Item("cuby", "i1")
	if !strings.Contains(got.DescriptionHTML, "EDITED") {
		t.Errorf("body not updated: %q", got.DescriptionHTML)
	}
	if got.DescriptionHash != mirror.HashBody("<p>EDITED</p>") {
		t.Error("description_hash did not follow the body")
	}
}

// A comment bumps its parent's updated_at with no body change (probed
// 2026-08-04). That is how the syncer knows to re-read comments at all.
func TestCommentBumpFetchesComments(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.addItem("i1", 1, "One", "<p>a</p>", base)

	s, m := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}

	f.comments["i1"] = []map[string]any{
		{"id": "c1", "actor": "u1", "comment_html": "<p>ping</p>",
			"created_at": base.Add(time.Hour).Format(time.RFC3339Nano)},
	}
	f.touchItem("i1", base.Add(time.Hour), "") // body unchanged

	if _, err := s.Delta(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	last, err := m.LastCommentAt("cuby", "i1")
	if err != nil {
		t.Fatal(err)
	}
	if !last.Equal(base.Add(time.Hour)) {
		t.Errorf("pulse = %v, want the comment's time %v", last, base.Add(time.Hour))
	}
}

// A body edit explains the updated_at bump by itself, so re-reading that item's
// comments would be wasted budget.
func TestBodyEditSkipsCommentFetch(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.addItem("i1", 1, "One", "<p>a</p>", base)

	s, _ := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	f.touchItem("i1", base.Add(time.Hour), "<p>a very different body</p>")
	f.reset()
	if _, err := s.Delta(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	for _, p := range f.paths {
		if strings.HasSuffix(p, "/comments/") {
			t.Errorf("delta fetched comments after a pure body edit: %v", f.paths)
			break
		}
	}
}

// Deletes are structurally invisible to a delta, so they must survive until a
// full reconcile — and then actually go.
func TestDeleteOnlyLandsOnFullReconcile(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.addItem("i1", 1, "One", "<p>a</p>", base)
	f.addItem("i2", 2, "Two", "<p>b</p>", base)

	s, m := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	f.items = f.items[:1] // i2 deleted in Plane

	if _, err := s.Delta(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := m.Item("cuby", "i2"); !ok {
		t.Error("a delta pruned a deleted item; it cannot know it is gone")
	}
	res, err := s.Backfill(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if res.Pruned != 1 {
		t.Errorf("full reconcile pruned %d, want 1", res.Pruned)
	}
	if _, ok, _ := m.Item("cuby", "i2"); ok {
		t.Error("full reconcile did not remove the deleted item")
	}
}

func TestDiffCleanAfterBackfillAndDirtyAfterDrift(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.modules = []map[string]any{{"id": "m1", "name": "Bubble One"}}
	f.modItems["m1"] = []string{"i1"}
	f.addItem("i1", 1, "One", "<p>a</p>", base)

	s, m := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	rep, err := s.Diff(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("diff dirty right after a backfill: %v", rep.Findings)
	}

	// Corrupt the mirror behind the syncer's back — the diff must catch it, or
	// it is not a gate at all.
	it, _, _ := m.Item("cuby", "i1")
	it.Name = "WRONG"
	if err := m.UpsertItems("cuby", []mirror.Item{it}, time.Now()); err != nil {
		t.Fatal(err)
	}
	rep, err = s.Diff(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean() {
		t.Fatal("diff reported clean against a corrupted mirror")
	}
	found := false
	for _, fd := range rep.Findings {
		if fd.Field == "name" && fd.Local == "WRONG" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings did not name the corrupted field: %v", rep.Findings)
	}
}

func TestDiffCatchesMembershipDrift(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.modules = []map[string]any{{"id": "m1", "name": "Bubble One"}}
	f.modItems["m1"] = []string{"i1"}
	f.addItem("i1", 1, "One", "<p>a</p>", base)
	f.addItem("i2", 2, "Two", "<p>b</p>", base)

	s, _ := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	// i2 joins the module in Plane. Membership moves may not bump updated_at, so
	// this is exactly the drift a delta cannot see — the diff must.
	f.modItems["m1"] = []string{"i1", "i2"}

	rep, err := s.Diff(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean() {
		t.Fatal("diff missed a module membership change")
	}
	if rep.Findings[0].Scope != "membership" {
		t.Errorf("finding scope = %q, want membership", rep.Findings[0].Scope)
	}
}

// A rate-limited project must not poison the whole pass, and — the part that
// actually matters — must not let a prune mistake "I could not see it" for
// "it is gone".
func TestPartialPassKeepsDataAndHoldsTheWatermark(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.modules = []map[string]any{{"id": "m1", "name": "Bubble One"}}
	f.modItems["m1"] = []string{"i1"}
	f.addItem("i1", 1, "One", "<p>a</p>", base)
	f.addItem("i2", 2, "Two", "<p>b</p>", base)

	s, m := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	before, err := m.Cursor("cuby", "items")
	if err != nil {
		t.Fatal(err)
	}

	// Now Plane starts 429ing the item list, as it did on the first live run.
	f.fail429 = "/work-items/"
	res, err := s.Backfill(context.Background(), inst(f))
	if err != nil {
		t.Fatalf("a rate-limited pass should degrade, not fail: %v", err)
	}
	if !res.Partial {
		t.Error("pass did not report itself partial")
	}
	if len(res.Errors) == 0 {
		t.Error("partial pass recorded no reason")
	}
	// The invariant: nothing was pruned, and both items survive.
	if res.Pruned != 0 {
		t.Errorf("pruned %d rows on a pass that could not see the project", res.Pruned)
	}
	for _, id := range []string{"i1", "i2"} {
		if _, ok, _ := m.Item("cuby", id); !ok {
			t.Errorf("%s was lost during a partial pass", id)
		}
	}
	after, err := m.Cursor("cuby", "items")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Watermark.Equal(before.Watermark) {
		t.Errorf("watermark moved on a partial pass: %v -> %v", before.Watermark, after.Watermark)
	}
	if after.LastError == "" {
		t.Error("cursor does not record why the mirror may be stale")
	}
}

// A full walk must be RESUMABLE. Without this, a pass that runs out of budget
// halfway restarts from the beginning next time and — because a partial pass
// cannot advance the watermark — never finishes at all. That is self-reinforcing
// under sustained rate pressure, and it is what the first live run hit.
func TestFullWalkResumesInsteadOfRestarting(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.modules = []map[string]any{{"id": "m1", "name": "Bubble One"}}
	f.modItems["m1"] = []string{"i1"}
	f.addItem("i1", 1, "One", "<p>a</p>", base)

	s, m := newTestSyncer(t)
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	// The project walked cleanly, so it now carries its own cursor.
	pc, err := m.Cursor("cuby", resourceProject+"p1")
	if err != nil {
		t.Fatal(err)
	}
	if pc.LastFull.IsZero() {
		t.Fatal("a clean project walk left no per-project cursor to resume from")
	}

	// A second AUTOMATIC full pass inside the window must SKIP it rather than
	// spend the budget re-walking what is already fresh. (An explicit Backfill
	// deliberately does not skip — "go and look" means go and look.)
	f.reset()
	res, err := s.reconcile(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 {
		t.Errorf("skipped %d project(s), want 1 — resumption did not engage", res.Skipped)
	}
	for _, p := range f.paths {
		if strings.Contains(p, "/modules/") || strings.Contains(p, "/work-items/") {
			t.Errorf("re-walked a project that was already fresh: %v", f.paths)
			break
		}
	}

	// Once the window has passed, it walks again.
	s.SetClock(func() time.Time { return time.Now().Add(FullInterval + time.Minute) })
	res, err = s.reconcile(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 0 {
		t.Errorf("skipped %d after the reconcile window elapsed, want 0", res.Skipped)
	}

	// An explicit backfill must never skip, whatever the cursors say.
	f.reset()
	res, err = s.Backfill(context.Background(), inst(f))
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 0 {
		t.Errorf("an explicit backfill skipped %d project(s); it must always re-walk", res.Skipped)
	}
}

// The comment budget is per PASS, not per project. Applied per project it would
// let an hourly reconcile of N projects spend N × the cap forever — comments are
// the only per-item call left in the sync, so this is the biggest cost lever.
func TestCommentBudgetIsSharedAcrossProjects(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	// More items than the per-pass budget, so the cap must bite.
	for i := 0; i < commentFetchPerPass+10; i++ {
		f.addItem(fmt.Sprintf("i%02d", i), i, fmt.Sprintf("Item %d", i), "<p>b</p>", base)
	}

	s, _ := newTestSyncer(t)
	f.reset()
	if _, err := s.Backfill(context.Background(), inst(f)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, p := range f.paths {
		if strings.HasSuffix(p, "/comments/") {
			calls++
		}
	}
	if calls > commentFetchPerPass {
		t.Errorf("spent %d comment calls in one pass, budget is %d", calls, commentFetchPerPass)
	}
	if calls == 0 {
		t.Error("spent no comment calls at all; the budget should allow some")
	}
}

// A comment fetch that fails must not make the project unclean. Comments are a
// fill-in — the board and sync-diff do not read them — so blocking a project's
// cursor on them would stall the reconcile behind the most deferrable work.
func TestCommentFailureDoesNotBlockTheProject(t *testing.T) {
	f := newFakePlane(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	f.modules = []map[string]any{{"id": "m1", "name": "Bubble One"}}
	f.modItems["m1"] = []string{"i1"}
	f.addItem("i1", 1, "One", "<p>a</p>", base)
	f.fail429 = "/comments/"

	s, m := newTestSyncer(t)
	res, err := s.Backfill(context.Background(), inst(f))
	if err != nil {
		t.Fatalf("a comment failure should not fail the pass: %v", err)
	}
	if res.Partial {
		t.Error("a comment failure marked the pass partial; comments are a fill-in")
	}
	if res.Items != 1 {
		t.Errorf("items = %d, want 1 — structure should still have landed", res.Items)
	}
	// The project walked cleanly, so it carries a cursor and the watermark moved.
	pc, err := m.Cursor("cuby", resourceProject+"p1")
	if err != nil {
		t.Fatal(err)
	}
	if pc.LastFull.IsZero() {
		t.Error("project cursor not set; a comment failure blocked an otherwise clean walk")
	}
	cur, _ := m.Cursor("cuby", resourceItems)
	if cur.Watermark.IsZero() {
		t.Error("watermark held back by a comment failure alone")
	}
}
