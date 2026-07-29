package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// fakePlane emulates the minimal Plane REST surface: /users/me (identifies any
// key as owner@x), /members/ (owner@x is an admin), and a workspace with two
// projects each holding one module.
func fakePlane() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"state-1","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/work-items/") && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":"new-wid-123"}`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Proj One"},{"id":"p2","name":"Proj Two"}]}`)
		case strings.HasSuffix(p, "/modules/") && strings.Contains(p, "/projects/p1/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m2","name":"Bubble B"}]}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// authedServer registers a whole-workspace instance backed by a fake Plane and
// returns the test server plus a usable Plane key (any string works).
func authedServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	ts := httptest.NewServer(New(st, time.Hour).Handler())
	t.Cleanup(ts.Close)
	return ts, "plane_personal_key"
}

func do(t *testing.T, method, url, token, body string) (int, []byte) {
	t.Helper()
	var r *http.Request
	var err error
	if body == "" {
		r, err = http.NewRequest(method, url, nil)
	} else {
		r, err = http.NewRequest(method, url, strings.NewReader(body))
	}
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestAuthRequired(t *testing.T) {
	// A store with no instances can't validate any key.
	ts := httptest.NewServer(New(openStore(t), time.Hour).Handler())
	defer ts.Close()

	if code, _ := do(t, http.MethodGet, ts.URL+"/api/bubbles", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", code)
	}
	if code, _ := do(t, http.MethodGet, ts.URL+"/api/bubbles", "some_key", ""); code != http.StatusUnauthorized {
		t.Fatalf("unverifiable key: want 401, got %d", code)
	}
}

// TestPlanePassthroughAuth proves a caller authenticates with a Plane API key:
// identity, admin role, and instance scope are all derived from Plane.
func TestPlanePassthroughAuth(t *testing.T) {
	ts, key := authedServer(t)

	code, body := do(t, http.MethodGet, ts.URL+"/api/whoami", key, "")
	if code != http.StatusOK {
		t.Fatalf("whoami: want 200, got %d", code)
	}
	var who domain.Actor
	if err := json.Unmarshal(body, &who); err != nil {
		t.Fatalf("decode whoami: %v", err)
	}
	if who.Kind != "human" || who.Email != "owner@x" || !who.Admin {
		t.Fatalf("whoami: want human owner@x admin, got %+v", who)
	}
	if len(who.Instances) != 1 || who.Instances[0] != "ws" {
		t.Fatalf("whoami: want instances [ws], got %v", who.Instances)
	}
}

// TestFederationWholeWorkspace: a project-less instance auto-discovers every
// project and namespaces the bubbles.
func TestFederationWholeWorkspace(t *testing.T) {
	ts, key := authedServer(t)

	code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	if code != http.StatusOK {
		t.Fatalf("bubbles: want 200, got %d", code)
	}
	var views []domain.BubbleView
	if err := json.Unmarshal(body, &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("want 2 bubbles across 2 projects, got %d: %+v", len(views), views)
	}
	names := map[string]bool{}
	for _, v := range views {
		names[v.Name] = true
		if !strings.HasPrefix(v.ID, "ws:") {
			t.Fatalf("bubble id not namespaced: %q", v.ID)
		}
	}
	if !names["Bubble A"] || !names["Bubble B"] {
		t.Fatalf("expected bubbles from both projects, got %v", names)
	}
}

func TestBirthCreatesWorkItem(t *testing.T) {
	ts, key := authedServer(t)
	body := `{"bubble_id":"ws:p1:m1","name":"New thread","brief":"why. Definition of Done: tests pass","logbook":"phase 1"}`

	code, resp := do(t, http.MethodPost, ts.URL+"/api/threads/birth", key, body)
	if code != http.StatusOK {
		t.Fatalf("birth: want 200, got %d (%s)", code, resp)
	}
	var r domain.BirthResult
	json.Unmarshal(resp, &r)
	if !r.Created || r.ThreadID != "new-wid-123" {
		t.Fatalf("unexpected birth result: %+v", r)
	}

	// the new thread shows up on the bubble (cache patched, no refetch)
	_, lb := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	var vs []domain.BubbleView
	json.Unmarshal(lb, &vs)
	for _, v := range vs {
		if v.ID == "ws:p1:m1" {
			if v.Threads != 1 {
				t.Fatalf("want 1 thread after birth, got %d", v.Threads)
			}
			return
		}
	}
	t.Fatal("bubble ws:p1:m1 not found after birth")
}

func TestContractAndClose(t *testing.T) {
	ts, key := authedServer(t)
	id := "ws:p1:m1" // "Bubble A" from the fake

	find := func() domain.BubbleView {
		_, body := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
		var vs []domain.BubbleView
		json.Unmarshal(body, &vs)
		for _, v := range vs {
			if v.ID == id {
				return v
			}
		}
		t.Fatalf("bubble %s not found", id)
		return domain.BubbleView{}
	}

	// set contract
	code, body := do(t, http.MethodPost, ts.URL+"/api/bubbles/"+id+"/contract", key, `{"owner":"angel","outcome":"ship it"}`)
	if code != http.StatusOK {
		t.Fatalf("set contract: want 200, got %d", code)
	}
	var c domain.Contract
	json.Unmarshal(body, &c)
	if c.Owner != "angel" || c.Outcome != "ship it" {
		t.Fatalf("unexpected contract: %+v", c)
	}
	if v := find(); v.Owner != "angel" {
		t.Fatalf("owner not reflected in ls: %+v", v)
	}

	// close → lifecycle closed
	if code, _ := do(t, http.MethodPost, ts.URL+"/api/bubbles/"+id+"/close", key, ""); code != http.StatusOK {
		t.Fatalf("close: want 200, got %d", code)
	}
	if v := find(); v.Lifecycle != domain.Closed {
		t.Fatalf("want closed after close, got %s", v.Lifecycle)
	}

	// reopen → not closed
	if code, _ := do(t, http.MethodPost, ts.URL+"/api/bubbles/"+id+"/reopen", key, ""); code != http.StatusOK {
		t.Fatalf("reopen: want 200, got %d", code)
	}
	if v := find(); v.Lifecycle == domain.Closed {
		t.Fatalf("want not-closed after reopen, got %s", v.Lifecycle)
	}

	// a bubble in an instance the caller can't see doesn't resolve → 404
	if code, _ := do(t, http.MethodPost, ts.URL+"/api/bubbles/other:p:m/contract", key, `{"owner":"x"}`); code != http.StatusNotFound {
		t.Fatalf("cross-instance contract: want 404, got %d", code)
	}
}

// TestTickTransition drives a real sweep: a bubble that is hot at the first tick
// (fresh work item) goes dormant once enough time passes, and the second tick
// records exactly one notification.
func TestTickTransition(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"P"}]}`)
		case strings.HasSuffix(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"w1","name":"W","created_at":"2026-01-01T12:00:00Z"}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	defer fake.Close()

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{Slug: "ws", BaseURL: fake.URL, APIKey: "k", Workspace: "w", Project: ""}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour) // 1h cycle
	ctx := context.Background()

	// Tick 1: 10 min after creation → bubble is hot → baseline, no notification.
	srv.now = func() time.Time { return created.Add(10 * time.Minute) }
	if n, err := srv.Tick(ctx); err != nil || n != 0 {
		t.Fatalf("tick1: want 0 notifications, got %d (err %v)", n, err)
	}

	// Tick 2: 3h later → latest evidence is >2 cycles old → dormant → 1 notification.
	srv.now = func() time.Time { return created.Add(3 * time.Hour) }
	if n, err := srv.Tick(ctx); err != nil || n != 1 {
		t.Fatalf("tick2: want 1 notification, got %d (err %v)", n, err)
	}

	ns, _ := st.ListNotifications("", []string{"ws"}, false, 10)
	if len(ns) != 1 || ns[0].Kind != "dormant" {
		t.Fatalf("want 1 dormant notification, got %+v", ns)
	}

	// Tick 3: no further change → no new notification.
	if n, _ := srv.Tick(ctx); n != 0 {
		t.Fatalf("tick3: want 0 (no transition), got %d", n)
	}
}

func TestCoolingNotice(t *testing.T) {
	cases := []struct {
		prev, cur domain.Lifecycle
		notify    bool
	}{
		{domain.Hot, domain.Cooling, true},
		{domain.Warm, domain.Dormant, true},
		{domain.Cooling, domain.Dormant, true},
		{domain.Cooling, domain.Cooling, false}, // no transition
		{domain.Dormant, domain.Hot, false},     // warming up — good news, no alert
		{domain.Hot, domain.Warm, false},        // still not cold enough
		{domain.Cooling, domain.Closed, false},  // closed isn't a cooling alert
	}
	for _, c := range cases {
		if _, got := coolingNotice(c.prev, c.cur); got != c.notify {
			t.Fatalf("%s->%s: want notify=%v, got %v", c.prev, c.cur, c.notify, got)
		}
	}
}

func TestMatchBubble(t *testing.T) {
	vs := []domain.BubbleView{
		{ID: "ws:p1:aaaa1111"},
		{ID: "ws:p2:bbbb2222"},
		{ID: "ws:p1:aaaa9999"},
	}
	check := func(q, wantID string) {
		t.Helper()
		v, err := matchBubble(vs, q)
		if err != nil || v.ID != wantID {
			t.Fatalf("match %q: want %s, got %q err=%v", q, wantID, v.ID, err)
		}
	}
	check("bbbb2222", "ws:p2:bbbb2222")       // exact short id
	check("bbbb", "ws:p2:bbbb2222")           // unique prefix
	check("ws:p1:aaaa1111", "ws:p1:aaaa1111") // full namespaced id

	if _, err := matchBubble(vs, "aaaa"); !errors.Is(err, errAmbig) {
		t.Fatalf("prefix 'aaaa' should be ambiguous, got %v", err)
	}
	if _, err := matchBubble(vs, "zzzz"); !errors.Is(err, errNotFound) {
		t.Fatalf("prefix 'zzzz' should be not-found, got %v", err)
	}
}

func TestBirthPolicyGate(t *testing.T) {
	ts, key := authedServer(t)
	url := ts.URL + "/api/threads/birth"

	if code, _ := do(t, http.MethodPost, url, "", `{}`); code != http.StatusUnauthorized {
		t.Fatalf("unauth birth: want 401, got %d", code)
	}
	if code, _ := do(t, http.MethodPost, url, key, `{"brief":"do it","logbook":"phase 1"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("no DoD: want 422, got %d", code)
	}
	if code, _ := do(t, http.MethodPost, url, key, `{"bubble_id":"ws:p1:m1","name":"T","brief":"why. Definition of Done: tests pass","logbook":"phase 1"}`); code != http.StatusOK {
		t.Fatalf("valid birth: want 200, got %d", code)
	}
}
