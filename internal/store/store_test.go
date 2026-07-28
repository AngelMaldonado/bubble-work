package store

import (
	"path/filepath"
	"testing"

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
