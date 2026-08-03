package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestInstanceCRUD(t *testing.T) {
	st := openTestStore(t)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.AddInstance(domain.Instance{Slug: "ayetec", BaseURL: "https://plane.ayetec.space", APIKey: "k1", Workspace: "w1", Project: ""}))
	must(st.AddInstance(domain.Instance{Slug: "cuby", BaseURL: "https://plane.cuby.work", APIKey: "k2", Workspace: "w2", Project: "p2"}))

	is, err := st.ListInstances()
	must(err)
	if len(is) != 2 {
		t.Fatalf("want 2 instances, got %d", len(is))
	}

	if ok, _ := st.InstanceExists("ayetec"); !ok {
		t.Fatal("ayetec should exist")
	}
	if ok, _ := st.InstanceExists("nope"); ok {
		t.Fatal("nope should not exist")
	}

	// Upsert updates in place rather than duplicating.
	must(st.AddInstance(domain.Instance{Slug: "cuby", BaseURL: "https://plane.cuby.work", APIKey: "k2b", Workspace: "w2", Project: "p2"}))
	is, _ = st.ListInstances()
	if len(is) != 2 {
		t.Fatalf("upsert should not add a row, got %d", len(is))
	}

	ok, err := st.RemoveInstance("ayetec")
	must(err)
	if !ok {
		t.Fatal("remove should report deletion")
	}
	is, _ = st.ListInstances()
	if len(is) != 1 || is[0].Slug != "cuby" {
		t.Fatalf("after remove want [cuby], got %+v", is)
	}
}

func TestNotifications(t *testing.T) {
	st := openTestStore(t)

	// lifecycle tracking
	if _, ok, _ := st.GetLifecycle("ayetec:p:m"); ok {
		t.Fatal("no lifecycle expected yet")
	}
	if err := st.SetLifecycle("ayetec:p:m", "hot", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	lc, ok, _ := st.GetLifecycle("ayetec:p:m")
	if !ok || lc != "hot" {
		t.Fatalf("want hot, got %q ok=%v", lc, ok)
	}

	// notifications, scoped by instance
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.AddNotification(domain.Notification{At: "t1", Instance: "ayetec", BubbleID: "ayetec:p:m", BubbleName: "A", Kind: "cooling", Message: "a cooling"}))
	must(st.AddNotification(domain.Notification{At: "t2", Instance: "cuby", BubbleID: "cuby:p:m", BubbleName: "B", Kind: "dormant", Message: "b dormant"}))

	const me = "me@x"
	// scoped by instance
	ns, err := st.ListNotifications(me, []string{"ayetec"}, false, 50)
	must(err)
	if len(ns) != 1 || ns[0].Instance != "ayetec" || !ns[0].Unread {
		t.Fatalf("scoped list: want 1 unread ayetec, got %+v", ns)
	}
	both, _ := st.ListNotifications(me, []string{"ayetec", "cuby"}, false, 50)
	if len(both) != 2 {
		t.Fatalf("want 2 across both, got %d", len(both))
	}
	if none, _ := st.ListNotifications(me, nil, false, 50); len(none) != 0 {
		t.Fatalf("no instances should return none, got %d", len(none))
	}

	// unread count + read receipts (per person)
	if c, _ := st.UnreadCount(me, []string{"ayetec", "cuby"}); c != 2 {
		t.Fatalf("want 2 unread, got %d", c)
	}
	must(st.MarkRead(me, []int64{both[0].ID}, "now"))
	if c, _ := st.UnreadCount(me, []string{"ayetec", "cuby"}); c != 1 {
		t.Fatalf("after read one: want 1 unread, got %d", c)
	}
	// another person is unaffected
	if c, _ := st.UnreadCount("other@x", []string{"ayetec", "cuby"}); c != 2 {
		t.Fatalf("other person: want 2 unread, got %d", c)
	}
	// unread-only filter
	if un, _ := st.ListNotifications(me, []string{"ayetec", "cuby"}, true, 50); len(un) != 1 {
		t.Fatalf("unread-only: want 1, got %d", len(un))
	}
	must(st.MarkAllRead(me, []string{"ayetec", "cuby"}, "now"))
	if c, _ := st.UnreadCount(me, []string{"ayetec", "cuby"}); c != 0 {
		t.Fatalf("after mark all: want 0 unread, got %d", c)
	}

	// prefs (opt-in), keyed by email, default off
	if on, _ := st.NotifyEnabled(me); on {
		t.Fatal("default should be opted out")
	}
	must(st.SetNotifyEnabled(me, true))
	if on, _ := st.NotifyEnabled(me); !on {
		t.Fatal("should be opted in after enable")
	}
}

func TestContractOverlay(t *testing.T) {
	st := openTestStore(t)
	const id = "ayetec:p1:m1"

	if _, ok, _ := st.GetContract(id); ok {
		t.Fatal("no contract expected yet")
	}
	if err := st.SetContract(id, Contract{Outcome: "ship it", Owner: "me", Closure: "merged"}); err != nil {
		t.Fatal(err)
	}
	c, ok, err := st.GetContract(id)
	if err != nil || !ok {
		t.Fatalf("get contract: ok=%v err=%v", ok, err)
	}
	if c.Outcome != "ship it" || c.Owner != "me" || c.Closed {
		t.Fatalf("unexpected contract: %+v", c)
	}
	if err := st.SetClosed(id, true); err != nil {
		t.Fatal(err)
	}
	c, _, _ = st.GetContract(id)
	if !c.Closed || c.Outcome != "ship it" {
		t.Fatalf("close should preserve fields: %+v", c)
	}
}

func TestSnapshotPersistence(t *testing.T) {
	st := openTestStore(t)

	if rows, err := st.LoadSnapshots(); err != nil || len(rows) != 0 {
		t.Fatalf("fresh store: want no snapshots, got %v err=%v", rows, err)
	}

	if err := st.SaveSnapshot("ayetec", `[{"ID":"a"}]`, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("save: %v", err)
	}
	// upsert replaces, does not duplicate
	if err := st.SaveSnapshot("ayetec", `[{"ID":"a"},{"ID":"b"}]`, "2026-08-02T01:00:00Z"); err != nil {
		t.Fatalf("resave: %v", err)
	}
	if err := st.SaveSnapshot("cuby", `[]`, "2026-08-02T02:00:00Z"); err != nil {
		t.Fatalf("save2: %v", err)
	}

	rows, err := st.LoadSnapshots()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 snapshots, got %d", len(rows))
	}
	got := map[string]Snapshot{}
	for _, r := range rows {
		got[r.Slug] = r
	}
	if got["ayetec"].Bubbles != `[{"ID":"a"},{"ID":"b"}]` || got["ayetec"].UpdatedAt != "2026-08-02T01:00:00Z" {
		t.Fatalf("ayetec not upserted: %+v", got["ayetec"])
	}
}

func TestCommentReads(t *testing.T) {
	st := openTestStore(t)
	at := "2026-08-02T00:00:00Z"

	// two readers mark comment c1; one marks c2.
	if err := st.MarkCommentsRead("ws", "u1", "Ana", at, []string{"c1"}); err != nil {
		t.Fatalf("mark u1: %v", err)
	}
	if err := st.MarkCommentsRead("ws", "u2", "Bo", at, []string{"c1", "c2"}); err != nil {
		t.Fatalf("mark u2: %v", err)
	}
	// idempotent: re-marking the same (comment, reader) doesn't duplicate.
	if err := st.MarkCommentsRead("ws", "u1", "Ana", at, []string{"c1"}); err != nil {
		t.Fatalf("remark u1: %v", err)
	}

	byID, err := st.CommentReaders("ws", []string{"c1", "c2"})
	if err != nil {
		t.Fatalf("readers: %v", err)
	}
	if len(byID["c1"]) != 2 {
		t.Errorf("c1 want 2 readers, got %+v", byID["c1"])
	}
	if len(byID["c2"]) != 1 || byID["c2"][0].Name != "Bo" {
		t.Errorf("c2 readers wrong: %+v", byID["c2"])
	}
	// instance is scoped.
	other, _ := st.CommentReaders("other", []string{"c1"})
	if len(other) != 0 {
		t.Errorf("instance scope leaked: %+v", other)
	}
}

func TestKioskTokens(t *testing.T) {
	st := openTestStore(t)

	if _, ok, err := st.LookupKioskToken("nope"); err != nil || ok {
		t.Fatalf("empty lookup: ok=%v err=%v", ok, err)
	}
	k := KioskToken{Token: "kiosk_abc", Instance: "ws", Name: "lobby", CreatedAt: "2026-08-02T00:00:00Z"}
	if err := st.AddKioskToken(k); err != nil {
		t.Fatalf("add: %v", err)
	}
	got, ok, err := st.LookupKioskToken("kiosk_abc")
	if err != nil || !ok || got.Instance != "ws" || got.Name != "lobby" {
		t.Fatalf("lookup: %+v ok=%v err=%v", got, ok, err)
	}
	if list, _ := st.ListKioskTokens(); len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	if removed, _ := st.RemoveKioskToken("kiosk_abc"); !removed {
		t.Error("expected removal")
	}
	if _, ok, _ := st.LookupKioskToken("kiosk_abc"); ok {
		t.Error("token should be gone after revoke")
	}
}

// Thread progress round-trips, and an unseen thread is simply absent (which is
// how the refresher knows to baseline it silently).
func TestThreadProgress(t *testing.T) {
	st := openTestStore(t)
	at := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)

	if err := st.SaveThreadProgress([]ThreadProgress{
		{ThreadID: "wi-1", DoneTodos: 2, Revisions: 1, TodosAt: at},
		{ThreadID: "wi-2", DoneTodos: 0, Revisions: 0}, // baselined, never moved
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := st.ThreadProgressFor([]string{"wi-1", "wi-2", "wi-missing"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if p := got["wi-1"]; p.DoneTodos != 2 || p.Revisions != 1 || !p.TodosAt.Equal(at) {
		t.Errorf("wi-1 round-trip wrong: %+v", p)
	}
	// The zero time survives as "never observed going up".
	if p := got["wi-2"]; !p.TodosAt.IsZero() || !p.RevisionsAt.IsZero() {
		t.Errorf("wi-2 should carry zero stamps: %+v", p)
	}

	// Saving again upserts rather than duplicating.
	later := at.Add(time.Hour)
	if err := st.SaveThreadProgress([]ThreadProgress{
		{ThreadID: "wi-1", DoneTodos: 3, Revisions: 1, TodosAt: later},
	}); err != nil {
		t.Fatalf("resave: %v", err)
	}
	got, _ = st.ThreadProgressFor([]string{"wi-1"})
	if p := got["wi-1"]; p.DoneTodos != 3 || !p.TodosAt.Equal(later) {
		t.Errorf("upsert wrong: %+v", p)
	}
}
