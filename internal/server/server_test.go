package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/heat"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
	planesync "github.com/AngelMaldonado/bubble-work/internal/sync"
)

// fakePlane emulates the minimal Plane REST surface: /users/me (identifies any
// key as owner@x), /members/ (owner@x is an admin), and a workspace with two
// projects each holding one module.
func fakePlane() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"id":"newproj","name":"BW Sandbox","identifier":"BWSBX"}`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/projects/"):
			io.WriteString(w, `{}`)
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"id":"newmod","name":"First bubble"}`)
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			// Names are project-configured and localized on purpose — the server must
			// match on `group`, never on `name` (THREAD-LIFECYCLE.md).
			io.WriteString(w, `{"results":[
				{"id":"state-1","name":"Todo","group":"unstarted","default":true},
				{"id":"state-2","name":"En Progreso","group":"started"},
				{"id":"state-3","name":"Finalizado","group":"completed"}
			]}`)
		case strings.HasSuffix(p, "/work-items/") && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":"new-wid-123"}`)
		case strings.Contains(p, "/modules/m1/") && strings.HasSuffix(p, "/module-issues/"):
			io.WriteString(w, `{"results":[
				{"id":"wi-1","name":"First thread","created_at":"2026-01-01T10:00:00Z","completed_at":null,"sequence_id":1,"sort_order":1000,"assignees":["u1"],"state":"state-2"},
				{"id":"wi-2","name":"Second thread","created_at":"2026-02-01T10:00:00Z","completed_at":"2026-03-01T00:00:00Z","sequence_id":2,"sort_order":2000,"assignees":[],"state":"state-3"}
			]}`)
		case strings.HasSuffix(p, "/relations/"):
			io.WriteString(w, `{"relates_to":["rev-1"],"blocking":[],"blocked_by":[]}`)
		case strings.HasSuffix(p, "/comments/") && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":"c2","actor":"u1","comment_html":"<p>shipping it</p>","created_at":"2026-04-02T00:00:00Z"}`)
		case strings.HasSuffix(p, "/comments/"):
			io.WriteString(w, `{"results":[{"id":"c1","actor":"u1","comment_html":"<p>looks good</p>","created_at":"2026-04-01T00:00:00Z"}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			// The project-wide item list. Real Plane returns EVERY item here —
			// threads and their revision sub-items alike — and callers filter
			// client-side (ListChildren by parent, the mirror not at all). `state`
			// arrives as an object because the sync asks for expand=state.
			io.WriteString(w, `{"results":[
				{"id":"wi-1","name":"First thread","description_html":"<h1>Brief</h1><p>Do it.</p><h2>Logbook</h2><ul><li data-checked='true'>scaffold</li><li data-checked='false'>wire</li></ul>","sequence_id":1,"sort_order":1000,"priority":"high","assignees":["u1"],"created_at":"2026-01-01T10:00:00Z","updated_at":"2026-01-01T10:00:00Z","completed_at":null,"state":{"id":"state-2","name":"En Progreso","group":"started"}},
				{"id":"wi-2","name":"Second thread","description_html":"","sequence_id":2,"sort_order":2000,"assignees":[],"created_at":"2026-02-01T10:00:00Z","updated_at":"2026-03-01T00:00:00Z","completed_at":"2026-03-01T00:00:00Z","state":{"id":"state-3","name":"Finalizado","group":"completed"}},
				{"id":"rev-1","name":"rev: first pass","description_html":"<h2>Findings</h2><p>looks solid</p>","parent":"wi-1","created_at":"2026-01-05T10:00:00Z","updated_at":"2026-01-05T10:00:00Z","state":{"id":"state-2","name":"En Progreso","group":"started"}}
			],"next_page_results":false}`)
		case r.Method == http.MethodGet && strings.Contains(p, "/work-items/"):
			io.WriteString(w, `{"id":"wi-1","name":"First thread","description_html":"<h1>Brief</h1><p>Do it.</p><h2>Logbook</h2><ul><li data-checked='true'>scaffold</li><li data-checked='false'>wire</li></ul>","sequence_id":1,"priority":"high","assignees":["u1"],"state":"state-2","created_at":"2026-01-01T10:00:00Z"}`)
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

// warmMirror fills the mirror from the fake Plane.
//
// Since docs/PLANE-SYNC.md Phase 2 the board is BUILT from sqlite rather than
// fetched from Plane, so a test that asserts on board contents has to sync
// first — the same thing the real server does at boot. Backfill rather than
// Delta on purpose: the fakes mostly omit updated_at, so a delta would early-stop
// at the watermark and see nothing.
func warmMirror(t *testing.T, srv *Server) {
	t.Helper()
	insts, err := srv.Instances()
	if err != nil {
		t.Fatalf("instances: %v", err)
	}
	for _, i := range insts {
		if _, err := srv.Syncer().Backfill(context.Background(), i); err != nil {
			t.Fatalf("warm mirror %s: %v", i.Slug, err)
		}
	}
}

// sweep is one production cycle at test scale: the syncer pulls, then the
// snapshot is rebuilt from what it pulled.
func sweep(t *testing.T, srv *Server) {
	t.Helper()
	warmMirror(t, srv)
	srv.flushCaches()
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
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
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

// F4: the SSE stream pushes the board on connect as an `event: bubbles` frame.
func TestStreamPushesInitialBoard(t *testing.T) {
	ts, key := authedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/stream", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	buf := make([]byte, 4096)
	var acc strings.Builder
	for !strings.Contains(acc.String(), "\n\n") {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			acc.WriteString(string(buf[:n]))
		}
		if err != nil {
			break
		}
	}
	if got := acc.String(); !strings.Contains(got, "event: bubbles") || !strings.Contains(got, "data:") {
		t.Fatalf("no initial bubbles frame: %q", got)
	}
}

// F2: a persisted snapshot warms the in-memory board at boot, so reads serve the
// last-known state immediately after a restart (before any Plane fetch).
func TestSnapshotWarmsOnBoot(t *testing.T) {
	st := openStore(t)
	if err := st.SaveSnapshot("ws", `[{"ID":"ws:p:m","Name":"Persisted"}]`, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	srv.bubblesMu.Lock()
	c, ok := srv.bubblesCache["ws"]
	srv.bubblesMu.Unlock()
	if !ok || len(c.bubbles) != 1 || c.bubbles[0].Name != "Persisted" {
		t.Fatalf("boot did not warm from persisted snapshot: %+v", c)
	}
}

func TestInteriorEndpoints(t *testing.T) {
	ts, key := authedServer(t)

	// Timeline: newest thread first, owner resolved, ids namespaced.
	code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles/ws:p1:m1/threads", key, "")
	if code != http.StatusOK {
		t.Fatalf("timeline status %d: %s", code, body)
	}
	var nodes []domain.ThreadNode
	if err := json.Unmarshal(body, &nodes); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d: %s", len(nodes), body)
	}
	if nodes[0].ID != "ws:p1:wi-2" { // Feb is newer than Jan
		t.Errorf("timeline not newest-first: %+v", nodes)
	}
	if nodes[1].ID != "ws:p1:wi-1" || nodes[1].Owner != "Owner" {
		t.Errorf("owner not resolved / wrong node: %+v", nodes[1])
	}
	if nodes[1].Active != true || nodes[0].Active != false {
		t.Errorf("active flags wrong: %+v", nodes)
	}

	// Thread detail: artifacts, logbook todos, revisions, kind.
	code, body = do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	if code != http.StatusOK {
		t.Fatalf("detail status %d: %s", code, body)
	}
	var d domain.ThreadDetail
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if d.Kind != "simple" {
		t.Errorf("kind = %q, want simple", d.Kind)
	}
	if d.Logbook == nil || len(d.Logbook.Todos) != 2 || !d.Logbook.Todos[0].Done || d.Logbook.Todos[1].Done {
		t.Errorf("logbook todos wrong: %+v", d.Logbook)
	}
	// The whole body renders as one document titled after the thread; the "Brief"
	// H1 stays inline (no fragmenting on content headings).
	if len(d.Artifacts) != 1 || d.Artifacts[0].Title != "First thread" ||
		!strings.Contains(d.Artifacts[0].Markdown, "# Brief") {
		t.Errorf("artifacts wrong: %+v", d.Artifacts)
	}
	if len(d.Revisions) != 1 || d.Revisions[0].Title != "first pass" {
		t.Errorf("revisions wrong: %+v", d.Revisions)
	}
	if len(d.Assignees) != 1 || d.Assignees[0] != "Owner" {
		t.Errorf("assignees wrong: %+v", d.Assignees)
	}

	// Comments: rendered to markdown, author resolved.
	code, body = do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1/comments", key, "")
	if code != http.StatusOK {
		t.Fatalf("comments status %d: %s", code, body)
	}
	var cs []domain.Comment
	if err := json.Unmarshal(body, &cs); err != nil {
		t.Fatalf("decode comments: %v", err)
	}
	if len(cs) != 1 || cs[0].Author != "Owner" || cs[0].Markdown != "looks good" {
		t.Errorf("comments wrong: %+v", cs)
	}

	// Posting a comment: written as the caller (actor u1 == me → mine), 201.
	code, body = do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/comments", key, `{"body":"shipping it"}`)
	if code != http.StatusCreated {
		t.Fatalf("post comment status %d: %s", code, body)
	}
	var pc domain.Comment
	if err := json.Unmarshal(body, &pc); err != nil {
		t.Fatalf("decode posted comment: %v", err)
	}
	if pc.Markdown != "shipping it" || pc.Author != "Owner" || !pc.Mine {
		t.Errorf("posted comment wrong: %+v", pc)
	}
	if pc.HTML == "" {
		t.Error("posted comment should carry rendered HTML")
	}

	// Empty body is a 400, not a Plane round-trip.
	code, body = do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/comments", key, `{"body":"   "}`)
	if code != http.StatusBadRequest {
		t.Errorf("empty comment should be 400, got %d: %s", code, body)
	}
}

// Threads carry their own derived lifecycle and their real Plane state
// (THREAD-LIFECYCLE.md Phase A). The fixture's threads were born in early 2026,
// so both are long past dormant: wi-1 has an owner but never produced anything
// (🪦), wi-2 completed (🏆).
func TestThreadBuoyancy(t *testing.T) {
	ts, key := authedServer(t)

	code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles/ws:p1:m1/threads", key, "")
	if code != http.StatusOK {
		t.Fatalf("timeline status %d: %s", code, body)
	}
	var nodes []domain.ThreadNode
	if err := json.Unmarshal(body, &nodes); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	byID := map[string]domain.ThreadNode{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	born := byID["ws:p1:wi-1"]
	if born.Level != "rip" || born.Lifecycle != domain.Dormant {
		t.Errorf("wi-1: want rip/dormant, got %s/%s (%s)", born.Level, born.Lifecycle, born.Reason)
	}
	// Plane state is passed through verbatim; only the group is machine-readable.
	if born.State != "En Progreso" || born.StateGroup != "started" {
		t.Errorf("wi-1 plane state wrong: %q / %q", born.State, born.StateGroup)
	}

	shipped := byID["ws:p1:wi-2"]
	if shipped.Level != "done" || shipped.Lifecycle != domain.Closed {
		t.Errorf("wi-2: want done/closed, got %s/%s", shipped.Level, shipped.Lifecycle)
	}
	if shipped.StateGroup != "completed" {
		t.Errorf("wi-2 state group = %q, want completed", shipped.StateGroup)
	}

	// The same levels roll up onto the bubble view.
	code, body = do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	if code != http.StatusOK {
		t.Fatalf("bubbles status %d: %s", code, body)
	}
	var views []domain.BubbleView
	if err := json.Unmarshal(body, &views); err != nil {
		t.Fatalf("decode bubbles: %v", err)
	}
	var found bool
	for _, v := range views {
		if v.ID != "ws:p1:m1" {
			continue
		}
		found = true
		if v.ThreadLevels["rip"] != 1 || v.ThreadLevels["done"] != 1 {
			t.Errorf("thread level roll-up wrong: %v", v.ThreadLevels)
		}
	}
	if !found {
		t.Fatalf("bubble ws:p1:m1 missing from board: %s", body)
	}

	// The interior carries the same derived level for the open thread.
	code, body = do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	if code != http.StatusOK {
		t.Fatalf("detail status %d: %s", code, body)
	}
	var d domain.ThreadDetail
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if d.Level != "rip" || d.StateGroup != "started" {
		t.Errorf("detail buoyancy/state wrong: %s / %s", d.Level, d.StateGroup)
	}
}

// threadLevel is pure, so the interesting combinations are worth pinning down
// directly: evidence decides the band, and Plane's state group only short-
// circuits the two terminal columns (THREAD-LIFECYCLE.md).
func TestThreadLevel(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	tun := domain.DefaultTuning()
	week := 7 * 24 * time.Hour
	win := heat.Window{CurStart: now.Add(-week), PrevStart: now.Add(-2 * week), Length: week, Decay: week}
	born := func(daysAgo int) time.Time { return now.AddDate(0, 0, -daysAgo) }
	birth := []domain.EvidenceEvent{{Kind: domain.EvThreadCreated, At: born(2)}}
	work := []domain.EvidenceEvent{{Kind: domain.EvCompletedTodo, At: born(1)}}

	cases := []struct {
		name string
		t    domain.Thread
		ev   []domain.EvidenceEvent
		lc   domain.Lifecycle
		want string
	}{
		{"fresh backlog item is parked, not burning",
			domain.Thread{Active: true, Owner: "me", StateGroup: "backlog", CreatedAt: born(2)}, birth, domain.Dormant, "zzzz"},
		{"backlog item with real progress is burning",
			domain.Thread{Active: true, Owner: "me", StateGroup: "backlog", CreatedAt: born(2)}, work, domain.Hot, "in_progress"},
		{"old thread that never produced is abandoned",
			domain.Thread{Active: true, Owner: "me", StateGroup: "unstarted", CreatedAt: born(90)}, birth, domain.Dormant, "rip"},
		{"thread that produced then went quiet is asleep",
			domain.Thread{Active: true, Owner: "me", StateGroup: "started", CreatedAt: born(90)}, work, domain.Dormant, "zzzz"},
		{"production resurrects a cancelled thread",
			domain.Thread{Active: true, Owner: "me", StateGroup: "cancelled", CreatedAt: born(1)}, work, domain.Hot, "in_progress"},
		{"a quiet cancelled thread stays a grave",
			domain.Thread{Active: true, Owner: "me", StateGroup: "cancelled", CreatedAt: born(90)}, birth, domain.Dormant, "rip"},
		{"completed in Plane is done regardless of heat",
			domain.Thread{Active: true, Owner: "me", StateGroup: "completed", CreatedAt: born(1)}, work, domain.Hot, "done"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := threadLevel(c.t, c.ev, c.lc, win, tun, now); got != c.want {
				t.Fatalf("want %s, got %s", c.want, got)
			}
		})
	}
}

// A caller cannot read the interior of an instance they aren't a member of.
func TestInteriorForbidsUnseenInstance(t *testing.T) {
	ts, key := authedServer(t)
	code, _ := do(t, http.MethodGet, ts.URL+"/api/threads/other:p1:wi-1", key, "")
	if code != http.StatusForbidden {
		t.Errorf("cross-instance read status = %d, want 403", code)
	}
}

func TestLevelsReviewAndSearch(t *testing.T) {
	// fake with one work item (old timestamp → dormant), so we can walk the bands.
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
			io.WriteString(w, `{"results":[{"id":"w1","name":"Design signup","created_at":"2026-01-01T12:00:00Z"}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			// item DATA now comes from here; /module-issues/ only supplies membership
			io.WriteString(w, `{"results":[{"id":"w1","name":"Design signup","created_at":"2026-01-01T12:00:00Z","updated_at":"2026-01-01T12:00:00Z"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Onboarding"}]}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	defer fake.Close()
	st := openStore(t)
	if err := st.AddInstance(domain.Instance{Slug: "ws", BaseURL: fake.URL, APIKey: "k", Workspace: "w", Project: ""}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	const id = "ws:p1:m1"
	key := "k"

	level := func() string {
		_, body := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
		var vs []domain.BubbleView
		json.Unmarshal(body, &vs)
		for _, v := range vs {
			if v.ID == id {
				return v.Level
			}
		}
		t.Fatal("bubble not found")
		return ""
	}

	// dormant + no owner → rip
	if l := level(); l != "rip" {
		t.Fatalf("initial: want rip, got %q", l)
	}
	// Naming an owner does NOT rescue the band: with the roll-up on, a bubble is
	// its threads, and paperwork is not work. (bubble_rip_needs_owner only
	// applies in union mode.)
	do(t, http.MethodPost, ts.URL+"/api/bubbles/"+id+"/contract", key, `{"owner":"angel"}`)
	if l := level(); l != "rip" {
		t.Fatalf("an owner should not change the roll-up band, got %q", l)
	}
	// review → reviewed
	if code, _ := do(t, http.MethodPost, ts.URL+"/api/bubbles/"+id+"/review", key, ""); code != http.StatusOK {
		t.Fatalf("review: want 200, got %d", code)
	}
	if l := level(); l != "reviewed" {
		t.Fatalf("after review: want reviewed, got %q", l)
	}
	// close → done (wins over reviewed)
	do(t, http.MethodPost, ts.URL+"/api/bubbles/"+id+"/close", key, "")
	if l := level(); l != "done" {
		t.Fatalf("after close: want done, got %q", l)
	}

	// thread search
	_, body := do(t, http.MethodGet, ts.URL+"/api/threads?q=sign", key, "")
	var hits []domain.ThreadHit
	json.Unmarshal(body, &hits)
	if len(hits) != 1 || hits[0].Name != "Design signup" || hits[0].BubbleID != id {
		t.Fatalf("search: want 1 hit for Design signup, got %+v", hits)
	}
}

func TestServiceAdmin(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{Slug: "ws", BaseURL: fake.URL, APIKey: "k", Workspace: "w", Project: ""}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv) // scoping reads the mirrored member registry (Phase 4)
	srv.SetAdmin("root-secret", []string{"boss@x"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// a normal member (any key → owner@x, not admin) is refused
	if code, _ := do(t, http.MethodGet, ts.URL+"/api/admin/stats", "member-key", ""); code != http.StatusForbidden {
		t.Fatalf("normal member on admin route: want 403, got %d", code)
	}
	// the admin token gets in
	code, body := do(t, http.MethodGet, ts.URL+"/api/admin/stats", "root-secret", "")
	if code != http.StatusOK {
		t.Fatalf("admin token: want 200, got %d", code)
	}
	var stats domain.AdminStats
	json.Unmarshal(body, &stats)
	if stats.Instances != 1 {
		t.Fatalf("stats: want 1 instance, got %d", stats.Instances)
	}
	// admin sees all instances' bubbles
	if code, _ := do(t, http.MethodGet, ts.URL+"/api/admin/bubbles", "root-secret", ""); code != http.StatusOK {
		t.Fatalf("admin bubbles: want 200, got %d", code)
	}
	// no token → 401 (auth), not 403
	if code, _ := do(t, http.MethodGet, ts.URL+"/api/admin/stats", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token on admin route: want 401, got %d", code)
	}
}

func TestCreateWorkspaceAndBubble(t *testing.T) {
	ts, key := authedServer(t)

	// create workspace (project)
	code, body := do(t, http.MethodPost, ts.URL+"/api/workspaces", key, `{"instance":"ws","name":"BW Sandbox","identifier":"BWSBX"}`)
	if code != http.StatusCreated {
		t.Fatalf("create workspace: want 201, got %d (%s)", code, body)
	}
	var w domain.Workspace
	json.Unmarshal(body, &w)
	if w.ID != "newproj" || w.Identifier != "BWSBX" || w.Instance != "ws" {
		t.Fatalf("unexpected workspace: %+v", w)
	}

	// create bubble (module) in it
	code, body = do(t, http.MethodPost, ts.URL+"/api/bubbles", key, `{"instance":"ws","project":"newproj","name":"First bubble"}`)
	if code != http.StatusCreated {
		t.Fatalf("create bubble: want 201, got %d (%s)", code, body)
	}
	var b domain.NewBubble
	json.Unmarshal(body, &b)
	if b.ID != "ws:newproj:newmod" {
		t.Fatalf("want namespaced id ws:newproj:newmod, got %q", b.ID)
	}

	// scope: an instance you can't see is refused
	if code, _ := do(t, http.MethodPost, ts.URL+"/api/workspaces", key, `{"instance":"other","name":"x"}`); code != http.StatusForbidden {
		t.Fatalf("cross-instance create: want 403, got %d", code)
	}
}

func TestBirthCreatesWorkItem(t *testing.T) {
	ts, key := authedServer(t)
	// Use m2 (empty baseline); m1 carries the interior fixture's threads.
	body := `{"bubble_id":"ws:p2:m2","name":"New thread","brief":"why. Definition of Done: tests pass","logbook":"phase 1"}`

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
		if v.ID == "ws:p2:m2" {
			if v.Threads != 1 {
				t.Fatalf("want 1 thread after birth, got %d", v.Threads)
			}
			return
		}
	}
	t.Fatal("bubble ws:p2:m2 not found after birth")
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
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			io.WriteString(w, `{"results":[{"id":"w1","name":"W","created_at":"2026-01-01T12:00:00Z","updated_at":"2026-01-01T12:00:00Z"}],"next_page_results":false}`)
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
	warmMirror(t, srv)
	// Union mode: this exercises the classic "a bubble cools and we say so" path,
	// where a thread's BIRTH warms its bubble. Under the roll-up the same bubble
	// is dormant from the start (an untouched work item has produced nothing), so
	// there is no transition to report — see TestBubbleRollup.
	if tun := srv.Tuning(); true {
		tun.BubbleLevelRollup = false
		if _, err := srv.SetTuning(tun); err != nil {
			t.Fatalf("tuning: %v", err)
		}
	}
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

func TestPlaneWebhook(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{Slug: "ws", BaseURL: fake.URL, APIKey: "k", Workspace: "w", Project: ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetWebhookSecret("ws", "topsecret"); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	// Seed a stale one-bubble snapshot so we can observe the webhook rebuilding
	// it (the mirror holds two bubbles). Since Phase 2 a webhook triggers a
	// snapshot REBUILD from the mirror rather than a fetch from Plane; making it
	// apply the event payload directly is Phase 6.
	srv.bubblesCache["ws"] = cachedBubbles{
		bubbles:   []domain.Bubble{{ID: "ws:stale:x", Name: "stale"}},
		updatedAt: srv.now(),
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"event":"issue","action":"created"}`
	sign := func(secret string) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte(body))
		return hex.EncodeToString(m.Sum(nil))
	}
	post := func(slug, sig string) int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/webhooks/plane/"+slug, strings.NewReader(body))
		req.Header.Set("X-Plane-Signature", sig)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := post("ws", "deadbeef"); code != http.StatusForbidden {
		t.Fatalf("bad signature: want 403, got %d", code)
	}
	if code := post("nope", sign("topsecret")); code != http.StatusNotFound {
		t.Fatalf("unknown instance: want 404, got %d", code)
	}
	if code := post("ws", sign("topsecret")); code != http.StatusOK {
		t.Fatalf("valid webhook: want 200, got %d", code)
	}
	// the valid webhook triggers a background rebuild, replacing the stale seed
	// with the mirror's two bubbles.
	deadline := time.Now().Add(2 * time.Second)
	for {
		srv.bubblesMu.Lock()
		c, ok := srv.bubblesCache["ws"]
		srv.bubblesMu.Unlock()
		if ok && len(c.bubbles) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("webhook did not refresh the snapshot from Plane")
		}
		time.Sleep(20 * time.Millisecond)
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

// TestTransientMembersFailIsRetryable guards the "bubbles suddenly vanished"
// bug: a transient failure of the Plane members fetch must surface as a
// retryable 502 (never 401, which would sign a browser out) and must NOT be
// cached as an authoritative empty scope — once Plane recovers, the next
// request must see full scope again.
func TestTransientMembersFailIsRetryable(t *testing.T) {
	st := openStore(t)
	var failMembers atomic.Bool
	failMembers.Store(true)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			if failMembers.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, `{"error":"boom"}`)
				return
			}
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"P1"}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"B"}]}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// During the outage the mirror never learned the members, so scoping cannot
	// be authoritative: retryable 502, not 401, and above all not a 200 with an
	// empty scope (which would look like "you belong nowhere").
	if code, _ := do(t, "GET", ts.URL+"/api/whoami", "k", ""); code != http.StatusBadGateway {
		t.Fatalf("during members outage want 502, got %d", code)
	}
	// Recover Plane, and let the sync pick the members up.
	failMembers.Store(false)
	warmMirror(t, srv)
	code, body := do(t, "GET", ts.URL+"/api/whoami", "k", "")
	if code != http.StatusOK {
		t.Fatalf("after recovery want 200, got %d: %s", code, body)
	}
	if !strings.Contains(string(body), `"instances":["ws"]`) {
		t.Fatalf("want scope restored to [ws], got %s", body)
	}
}

// TestCycleWindow verifies the active-cycle picker (§3.6): it selects the cycle
// containing `now` and the one immediately before it, and returns zero when now
// falls outside every cycle (→ rolling-window fallback).
func TestCycleWindow(t *testing.T) {
	cycles := []plane.Cycle{
		{Name: "S1", StartDate: "2026-06-01", EndDate: "2026-06-14"},
		{Name: "S2", StartDate: "2026-06-15", EndDate: "2026-06-28"},
		{Name: "S3", StartDate: "2026-06-29", EndDate: "2026-07-12"},
	}
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC) // inside S3

	cur, prev := cycleWindow(cycles, now)
	if !cur.Equal(time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("curStart: want 2026-06-29, got %s", cur)
	}
	if !prev.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("prevStart: want 2026-06-15 (S2 start), got %s", prev)
	}

	// now outside all cycles → zero window (fallback to rolling).
	cur, prev = cycleWindow(cycles, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if !cur.IsZero() || !prev.IsZero() {
		t.Fatalf("out-of-range: want zero window, got cur=%s prev=%s", cur, prev)
	}
}

func TestKioskCredential(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	if err := st.AddKioskToken(store.KioskToken{
		Token: "kiosk_test", Instance: "ws", Name: "lobby", CreatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("add kiosk: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const kiosk = "kiosk_test"

	// whoami: a read-only kiosk actor scoped to its instance.
	code, body := do(t, http.MethodGet, ts.URL+"/api/whoami", kiosk, "")
	if code != http.StatusOK {
		t.Fatalf("whoami status %d: %s", code, body)
	}
	var who domain.Actor
	if err := json.Unmarshal(body, &who); err != nil {
		t.Fatalf("decode actor: %v", err)
	}
	if !who.ReadOnly || who.Kind != "kiosk" || len(who.Instances) != 1 || who.Instances[0] != "ws" {
		t.Fatalf("kiosk actor wrong: %+v", who)
	}

	// reads are allowed.
	if code, body = do(t, http.MethodGet, ts.URL+"/api/bubbles", kiosk, ""); code != http.StatusOK {
		t.Fatalf("kiosk read bubbles: %d %s", code, body)
	}

	// writes are rejected with 403 (before reaching any handler).
	code, body = do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/comments", kiosk, `{"body":"hi"}`)
	if code != http.StatusForbidden {
		t.Errorf("kiosk write should be 403, got %d: %s", code, body)
	}

	// a kiosk-looking token that isn't registered is rejected.
	if code, _ = do(t, http.MethodGet, ts.URL+"/api/whoami", "kiosk_bogus", ""); code != http.StatusUnauthorized {
		t.Errorf("unknown kiosk token should be 401, got %d", code)
	}
}

// The buoyancy thresholds are calibration, not constants: a service admin can
// retune them at runtime and every derived read reflects it immediately.
func TestAdminTuning(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	srv.SetAdmin("", []string{"owner@x"})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	code, body := do(t, http.MethodGet, ts.URL+"/api/admin/tuning", key, "")
	if code != http.StatusOK {
		t.Fatalf("get tuning status %d: %s", code, body)
	}
	var v domain.TuningView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode tuning: %v", err)
	}
	if v.Tuning.CycleHours != 1 { // seeded from New(st, time.Hour)
		t.Errorf("cycle seeded from config: got %v", v.Tuning.CycleHours)
	}
	if v.Defaults.CycleHours != 168 || len(v.Fields) == 0 {
		t.Errorf("defaults/schema missing: %+v", v)
	}

	// The fixture's evidence is months old, so against a 1h cycle the bubble has
	// long gone dormant with no owner → 🪦.
	if lv := bubbleLevelFor(t, ts, key, "ws:p1:m1"); lv != "rip" {
		t.Fatalf("before retune: want rip, got %s", lv)
	}

	// Stretch the cycle to a year: the thread is now well inside its newborn grace
	// period, so it stops reading as abandoned and the bubble follows it up a
	// band on the very next read. No recompute, no cache flush — all derived.
	code, body = do(t, http.MethodPut, ts.URL+"/api/admin/tuning", key, `{"cycle_hours": 8760}`)
	if code != http.StatusOK {
		t.Fatalf("put tuning status %d: %s", code, body)
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode applied tuning: %v", err)
	}
	// A partial body patches only the keys it names.
	if v.Tuning.CycleHours != 8760 || v.Tuning.DormantCycles != 2 || !v.Tuning.ThreadTerminalStateWins {
		t.Fatalf("partial patch clobbered other knobs: %+v", v.Tuning)
	}
	if lv := bubbleLevelFor(t, ts, key, "ws:p1:m1"); lv != "zzzz" {
		t.Fatalf("after retune: want zzzz, got %s", lv)
	}

	// Out-of-range values are clamped, never applied raw.
	code, body = do(t, http.MethodPut, ts.URL+"/api/admin/tuning", key, `{"decay_cycles": 0, "cycle_hours": -5}`)
	if code != http.StatusOK {
		t.Fatalf("put clamp status %d: %s", code, body)
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode clamped: %v", err)
	}
	if v.Tuning.DecayCycles != 0.1 || v.Tuning.CycleHours != 1 {
		t.Errorf("values not clamped: %+v", v.Tuning)
	}

	// It survives a restart: the calibration is persisted, not in-memory.
	if got := New(st, time.Hour).Tuning().DecayCycles; got != 0.1 {
		t.Errorf("tuning did not persist across restart: %v", got)
	}

	// A garbage body is a 400, and leaves the calibration alone.
	if code, _ = do(t, http.MethodPut, ts.URL+"/api/admin/tuning", key, `not json`); code != http.StatusBadRequest {
		t.Errorf("bad tuning body: want 400, got %d", code)
	}
}

// bubbleLevelFor reads one bubble's band off the board.
func bubbleLevelFor(t *testing.T, ts *httptest.Server, key, id string) string {
	t.Helper()
	code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	if code != http.StatusOK {
		t.Fatalf("bubbles status %d: %s", code, body)
	}
	var vs []domain.BubbleView
	if err := json.Unmarshal(body, &vs); err != nil {
		t.Fatalf("decode bubbles: %v", err)
	}
	for _, v := range vs {
		if v.ID == id {
			return v.Level
		}
	}
	t.Fatalf("bubble %s not on the board: %s", id, body)
	return ""
}

func TestAdminMembers(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)                    // the member registry is mirrored now (Phase 4)
	srv.SetAdmin("", []string{"owner@x"}) // the fake /users/me email → service admin
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	code, body := do(t, http.MethodGet, ts.URL+"/api/admin/members", "plane_personal_key", "")
	if code != http.StatusOK {
		t.Fatalf("members status %d: %s", code, body)
	}
	var ims []domain.InstanceMembers
	if err := json.Unmarshal(body, &ims); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(ims) != 1 || ims[0].Instance != "ws" || len(ims[0].Members) != 1 {
		t.Fatalf("members wrong: %+v", ims)
	}
	if m := ims[0].Members[0]; m.Email != "owner@x" || !m.Admin {
		t.Errorf("member mapping wrong: %+v", m)
	}
}

// TestProgressEvidence is Phase C end-to-end: Plane has no "a todo got ticked"
// event, so production is detected by diffing each sweep. The first sighting
// baselines silently (we don't know when old todos were ticked), an increase
// warms the thread, and — crucially — the warmth persists across later sweeps
// that see no change, because the moment of the increase is what's stored.
func TestProgressEvidence(t *testing.T) {
	var doneTodos atomic.Int32 // ticked items in wi-1's logbook, live-editable
	var revisions atomic.Int32 // revision sub-items hanging off wi-1

	body := func() string {
		var b strings.Builder
		b.WriteString("<h1>Brief</h1><p>Do it.</p><h2>Logbook</h2><ul>")
		for i := 0; i < 3; i++ {
			if int32(i) < doneTodos.Load() {
				b.WriteString("<li data-checked='true'>step</li>")
			} else {
				b.WriteString("<li data-checked='false'>step</li>")
			}
		}
		b.WriteString("</ul>")
		return b.String()
	}

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Backlog","group":"backlog","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread","created_at":"2026-01-01T10:00:00Z","completed_at":null,"sequence_id":1,"assignees":["u1"],"state":"s1"}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			items := []string{fmt.Sprintf(`{"id":"wi-1","name":"First thread","description_html":%q,"created_at":"2026-01-01T10:00:00Z"}`, body())}
			for i := 0; i < int(revisions.Load()); i++ {
				items = append(items, fmt.Sprintf(`{"id":"rev-%d","name":"rev: pass","parent":"wi-1"}`, i))
			}
			fmt.Fprintf(w, `{"results":[%s],"next_page_results":false}`, strings.Join(items, ","))
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	level := func() string {
		code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles/ws:p1:m1/threads", key, "")
		if code != http.StatusOK {
			t.Fatalf("timeline status %d: %s", code, body)
		}
		var nodes []domain.ThreadNode
		if err := json.Unmarshal(body, &nodes); err != nil {
			t.Fatalf("decode timeline: %v", err)
		}
		if len(nodes) != 1 {
			t.Fatalf("want 1 thread, got %d", len(nodes))
		}
		return nodes[0].Level
	}

	// First sweep: the thread already has one ticked todo, but we've never seen it
	// before. Baseline silently — stamping it "now" would fabricate heat for work
	// that could be a year old. Born in January against a 1h cycle → 🪦.
	doneTodos.Store(1)
	if lv := level(); lv != "rip" {
		t.Fatalf("first sighting should baseline silently, got %s", lv)
	}

	// Someone ticks another one. The next sweep notices and stamps it, labelling
	// the change as a tick rather than a re-plan.
	doneTodos.Store(2)
	sweep(t, srv)
	if lv := level(); lv != "in_progress" {
		t.Fatalf("after a todo was ticked: want in_progress, got %s", lv)
	}
	if prog, err := st.ThreadProgressFor([]string{"wi-1"}); err != nil {
		t.Fatalf("load progress: %v", err)
	} else if p := prog["wi-1"]; p.LogbookKind != domain.EvCompletedTodo {
		t.Errorf("a tick should be labelled completed-todo, got %+v", p)
	}

	// A later sweep sees no change at all — the thread must STAY warm, because
	// heat is derived from when the tick happened, not from noticing it.
	sweep(t, srv)
	if lv := level(); lv != "in_progress" {
		t.Fatalf("warmth must survive a no-change sweep, got %s", lv)
	}

	// Any logbook change is progress — the Logbook is the plan, so revising it
	// (here, striking an item) is a recorded decision, not motion.
	doneTodos.Store(0)
	sweep(t, srv)
	if lv := level(); lv != "in_progress" {
		t.Fatalf("re-planning is progress too, got %s", lv)
	}
	prog, err := st.ThreadProgressFor([]string{"wi-1"})
	if err != nil {
		t.Fatalf("load progress: %v", err)
	}
	// ...and the kind distinguishes a tick from a re-plan, even though both warm.
	if p := prog["wi-1"]; p.LogbookKind != domain.EvLogbookUpdated || p.LogbookAt.IsZero() {
		t.Errorf("want a logbook-updated stamp, got %+v", p)
	}

	// A revision artifact landing is progress too, counted from the sub-items
	// that name the thread as parent.
	revisions.Store(1)
	sweep(t, srv)
	if lv := level(); lv != "in_progress" {
		t.Fatalf("after a revision landed: want in_progress, got %s", lv)
	}
	prog, _ = st.ThreadProgressFor([]string{"wi-1"})
	if p := prog["wi-1"]; p.Revisions != 1 || p.RevisionsAt.IsZero() {
		t.Errorf("revision not counted/stamped: %+v", p)
	}
}

// A recent comment blocks the grave without warming the thread: it stays 😴
// rather than 🪦, and never reaches 🔥 (THREAD-LIFECYCLE.md).
func TestCommentPulseBlocksTheGrave(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	win := heat.Window{CurStart: now.Add(-week), PrevStart: now.Add(-2 * week), Length: week, Decay: week}
	tun := domain.DefaultTuning()

	// Old, assigned, never produced anything → normally abandoned.
	abandoned := domain.Thread{Active: true, Owner: "me", CreatedAt: now.AddDate(0, -6, 0)}
	birth := []domain.EvidenceEvent{{Kind: domain.EvThreadCreated, At: abandoned.CreatedAt}}

	if got := threadLevel(abandoned, birth, domain.Dormant, win, tun, now); got != "rip" {
		t.Fatalf("baseline: want rip, got %s", got)
	}

	// Someone commented yesterday — still being discussed, so not a grave.
	chattered := append(birth, domain.EvidenceEvent{Kind: domain.EvComment, At: now.AddDate(0, 0, -1)})
	if got := threadLevel(abandoned, chattered, domain.Dormant, win, tun, now); got != "zzzz" {
		t.Fatalf("a recent comment should hold it at zzzz, got %s", got)
	}

	// Chatter from months ago doesn't save it.
	stale := append(birth, domain.EvidenceEvent{Kind: domain.EvComment, At: now.AddDate(0, -3, 0)})
	if got := threadLevel(abandoned, stale, domain.Dormant, win, tun, now); got != "rip" {
		t.Fatalf("stale chatter should not block the grave, got %s", got)
	}

	// With the pulse switched off, discussion carries no weight at all.
	off := tun
	off.PulseCycles = 0
	if got := threadLevel(abandoned, chattered, domain.Dormant, win, off, now); got != "rip" {
		t.Fatalf("pulse_cycles=0: want rip, got %s", got)
	}

	// And a comment can never talk a CANCELLED thread out of the grave — only
	// production undoes a cancellation.
	cancelled := abandoned
	cancelled.StateGroup = "cancelled"
	if got := threadLevel(cancelled, chattered, domain.Dormant, win, tun, now); got != "rip" {
		t.Fatalf("chatter must not revive a cancelled thread, got %s", got)
	}
	if got := threadLevel(cancelled, chattered, domain.Hot, win, tun, now); got != "in_progress" {
		t.Fatalf("production must revive a cancelled thread, got %s", got)
	}
}

// Reading or posting a discussion records the pulse for free — no extra Plane
// traffic, and no dependence on the tick probe having run.
func TestPulseRecordedFromComments(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	// The pulse now comes from the MIRRORED comments — a local SELECT, where it
	// used to be a Plane call per at-risk thread capped at 20 a tick
	// (docs/PLANE-SYNC.md Phase 3). Reading the discussion no longer "records"
	// anything, because reading the mirror observes nothing new about Plane.
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC) // the fixture's only comment
	at, err := srv.mirror.LastCommentAt("ws", "wi-1")
	if err != nil {
		t.Fatalf("mirror pulse: %v", err)
	}
	if !at.Equal(want) {
		t.Fatalf("pulse from the mirror = %v, want %v", at, want)
	}

	// Opening the discussion still works and still shows the comment.
	if code, body := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1/comments", key, ""); code != http.StatusOK {
		t.Fatalf("comments status %d: %s", code, body)
	}

	// Posting one is the strongest pulse: someone is here now. The created row
	// goes straight into the mirror rather than waiting for a sync pass, so the
	// discussion — and the pulse — reflect it immediately.
	if code, body := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/comments", key, `{"body":"still on it"}`); code != http.StatusCreated {
		t.Fatalf("post status %d: %s", code, body)
	}
	at, err = srv.mirror.LastCommentAt("ws", "wi-1")
	if err != nil {
		t.Fatalf("mirror pulse: %v", err)
	}
	if !at.After(want) {
		t.Fatalf("posting should advance the pulse, got %v", at)
	}
	// The stored pulse is still written on a post and still consulted as a
	// fallback: an item whose comments the sync has not reached yet must not be
	// mistaken for one that has none.
	if p, _ := st.PulseFor([]string{"wi-1"}); !p["wi-1"].LastCommentAt.After(want) {
		t.Errorf("posting should also advance the stored fallback pulse: %+v", p["wi-1"])
	}
}

// Phase B: the sweep reflects derived levels back onto Plane — but only for
// instances that opted in, and never over a human's own edit.
func TestAutoStateWriteBack(t *testing.T) {
	var (
		mu      sync.Mutex
		state   = "state-1" // wi-1's current state in the fake ("Todo", unstarted)
		patched []string    // state ids we were asked to write, in order
	)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[
				{"id":"state-1","name":"Todo","group":"unstarted","default":true},
				{"id":"state-2","name":"En Progreso","group":"started"},
				{"id":"state-3","name":"Cancelado","group":"cancelled"}
			]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/module-issues/"):
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":"Old thread","created_at":"2026-01-01T10:00:00Z","completed_at":null,"sequence_id":1,"assignees":["u1"],"state":%q}]}`, state)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			// item data + its CURRENT state, so a re-sync reflects our own write back
			grp := map[string]string{"state-1": "unstarted", "state-2": "started", "state-3": "cancelled"}[state]
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":"Old thread","created_at":"2026-01-01T10:00:00Z","updated_at":"2026-01-01T10:00:00Z","completed_at":null,"sequence_id":1,"assignees":["u1"],"state":{"id":%q,"group":%q}}],"next_page_results":false}`, state, grp)
		case r.Method == http.MethodPatch && strings.Contains(p, "/work-items/"):
			var body struct {
				State string `json:"state"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			patched = append(patched, body.State)
			state = body.State // Plane now reflects our write
			io.WriteString(w, `{}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)

	writes := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), patched...)
	}

	// Off by default: Bubble is a lens. The thread is long dormant and never
	// produced anything (🪦), but nothing is written.
	if _, err := srv.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if got := writes(); len(got) != 0 {
		t.Fatalf("auto-state must be opt-in, but wrote %v", got)
	}

	// Opt in. Now the 🪦 thread is moved to the cancelled group — by GROUP, not by
	// name, so the localized "Cancelado" is found correctly.
	if ok, err := st.SetAutoState("ws", true); err != nil || !ok {
		t.Fatalf("enable: %v %v", ok, err)
	}
	srv.flushCaches()
	if _, err := srv.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	got := writes()
	if len(got) != 1 || got[0] != "state-3" {
		t.Fatalf("want one write to the cancelled state, got %v", got)
	}

	// It records what it wrote, and doesn't write again on the next sweep — the
	// card is already where it belongs.
	prov, err := st.AutoStateFor([]string{"wi-1"})
	if err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if p := prov["wi-1"]; p.StateID != "state-3" || p.HandedOff {
		t.Fatalf("provenance wrong: %+v", p)
	}
	srv.flushCaches()
	srv.Tick(context.Background())
	if got := writes(); len(got) != 1 {
		t.Fatalf("a settled card should not be rewritten, got %v", got)
	}

	// A human drags it somewhere else. We must notice, hand the thread off, and
	// never touch it again — even though our rules still say 🪦.
	mu.Lock()
	state = "state-2" // moved to "En Progreso" by hand
	mu.Unlock()
	srv.flushCaches()
	srv.Tick(context.Background())
	if got := writes(); len(got) != 1 {
		t.Fatalf("we must not fight a human edit, but wrote %v", got)
	}
	if prov, _ := st.AutoStateFor([]string{"wi-1"}); !prov["wi-1"].HandedOff {
		t.Fatal("the thread should be latched as human-owned")
	}
	// ...and the hand-off is permanent, not just for this sweep.
	srv.flushCaches()
	srv.Tick(context.Background())
	if got := writes(); len(got) != 1 {
		t.Fatalf("hand-off must latch, but wrote %v", got)
	}
}

// The level → Plane group mapping, including what we deliberately never touch.
func TestAutoTarget(t *testing.T) {
	for level, want := range map[string]string{
		"in_progress": "started",
		"zzzz":        "backlog",
		"rip":         "cancelled",
		"done":        "", // finishing is Plane's business
		"reviewed":    "",
		"":            "",
	} {
		if got := autoTarget(level); got != want {
			t.Errorf("autoTarget(%q) = %q, want %q", level, got, want)
		}
	}
}

// A bubble IS its threads: it sits in the band of its hottest unfinished one.
// The union mode it replaces counts a thread's BIRTH as bubble output, so a
// bubble full of untouched new work items reads 🔥 there and 😴 here.
func TestBubbleRollup(t *testing.T) {
	srv := New(openStore(t), time.Hour)
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	srv.now = func() time.Time { return now }
	tun := srv.Tuning()

	old := now.AddDate(0, -6, 0)
	thread := func(id string, opts ...func(*domain.Thread)) domain.Thread {
		th := domain.Thread{ID: id, Active: true, Owner: "me", CreatedAt: old}
		for _, o := range opts {
			o(&th)
		}
		return th
	}
	done := func(th *domain.Thread) { th.Active = false; c := old; th.CompletedAt = &c }
	fresh := func(th *domain.Thread) { th.CreatedAt = now.Add(-time.Minute) } // inside grace

	birth := func(ids ...string) []domain.EvidenceEvent {
		var ev []domain.EvidenceEvent
		for _, id := range ids {
			ev = append(ev, domain.EvidenceEvent{ThreadID: id, Kind: domain.EvThreadCreated, At: old})
		}
		return ev
	}

	level := func(b domain.Bubble, tn domain.Tuning) string {
		_, lv, _ := srv.bubbleHeat(b, tn, now)
		return lv
	}

	// One thread waiting its turn (😴), one abandoned (🪦) → the hottest wins.
	mixed := domain.Bubble{
		Threads:  []domain.Thread{thread("a", fresh), thread("b")},
		Evidence: birth("a", "b"),
	}
	if got := level(mixed, tun); got != "zzzz" {
		t.Errorf("hottest thread should set the band: got %s", got)
	}

	// All abandoned → the bubble is too.
	dead := domain.Bubble{Threads: []domain.Thread{thread("a"), thread("b")}, Evidence: birth("a", "b")}
	if got := level(dead, tun); got != "rip" {
		t.Errorf("a bubble of graves is a grave: got %s", got)
	}

	// Every thread finished, but nobody closed the bubble. NOT 🏆 — that band
	// means "closed", and closing is a human decision (§4).
	finished := domain.Bubble{Threads: []domain.Thread{thread("a", done)}, Evidence: birth("a")}
	res, lv, _ := srv.bubbleHeat(finished, tun, now)
	if lv != "zzzz" || !strings.Contains(res.Reason, "close or redefine") {
		t.Errorf("finished bubble: got %s / %q", lv, res.Reason)
	}

	// A bubble with no threads at all never got going.
	if got := level(domain.Bubble{}, tun); got != "rip" {
		t.Errorf("empty bubble: got %s", got)
	}

	// Explicit human declarations still outrank anything derived.
	closed := dead
	closed.Closed = true
	if got := level(closed, tun); got != "done" {
		t.Errorf("closed must win: got %s", got)
	}
	reviewed := dead
	reviewed.Stage = "reviewed"
	if got := level(reviewed, tun); got != "reviewed" {
		t.Errorf("reviewed must win: got %s", got)
	}

	// The headline divergence, and the reason the roll-up is on by default: a
	// bubble whose work items were all created moments ago but touched by nobody.
	// The union counts those births as bubble output and calls it 🔥; the roll-up
	// asks the threads, and every one of them is still waiting its turn.
	justBorn := domain.Bubble{
		Owner:   "me",
		Threads: []domain.Thread{thread("a", fresh), thread("b", fresh)},
		Evidence: []domain.EvidenceEvent{
			{ThreadID: "a", Kind: domain.EvThreadCreated, At: now.Add(-time.Minute)},
			{ThreadID: "b", Kind: domain.EvThreadCreated, At: now.Add(-time.Minute)},
		},
	}
	union := tun
	union.BubbleLevelRollup = false
	if got := level(justBorn, union); got != "in_progress" {
		t.Errorf("union mode: birthing threads warms the bubble, got %s", got)
	}
	if got := level(justBorn, tun); got != "zzzz" {
		t.Errorf("roll-up: nothing has been produced yet, want zzzz, got %s", got)
	}
}

// TestBoardMakesNoPlaneCalls is the Phase 2+3 acceptance test
// (docs/PLANE-SYNC.md).
//
// The whole point of the mirror is that READING stops touching Plane. Asserting
// that in prose is worthless — this counts actual HTTP requests to the fake and
// requires the count to be EXACTLY zero across every read path: the board, the
// timeline, search, heat, a thread's interior and its comments.
func TestBoardMakesNoPlaneCalls(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	var paths []string
	inner := fakePlane()
	t.Cleanup(inner.Close)
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		req, _ := http.NewRequest(r.Method, inner.URL+r.URL.RequestURI(), r.Body)
		req.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	t.Cleanup(counting.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: counting.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv) // the sync worker's calls are expected and not counted below
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Resolve identity once so the auth cache is warm. Identity is deliberately
	// OUT of scope here: /users/me is the actual credential check and the member
	// scoping still calls Plane, both until Phase 4. Flushing the whole cache in
	// the loop below would re-resolve every iteration and this test would be
	// measuring auth rather than the board.
	if code, body := do(t, http.MethodGet, ts.URL+"/api/whoami", "plane_personal_key", ""); code != http.StatusOK {
		t.Fatalf("whoami: %d %s", code, body)
	}

	calls.Store(0) // ← everything from here must be served from sqlite
	mu.Lock()
	paths = nil
	mu.Unlock()

	for i := 0; i < 5; i++ {
		// Drop only the SNAPSHOT, so each iteration rebuilds the board from
		// scratch. A cold rebuild is the expensive path and it must still be free.
		srv.bubblesMu.Lock()
		srv.bubblesCache = map[string]cachedBubbles{}
		srv.bubblesMu.Unlock()
		if code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles", "plane_personal_key", ""); code != http.StatusOK {
			t.Fatalf("bubbles: %d %s", code, body)
		}
		if code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles/ws:p1:m1/threads", "plane_personal_key", ""); code != http.StatusOK {
			t.Fatalf("timeline: %d %s", code, body)
		}
		if code, body := do(t, http.MethodGet, ts.URL+"/api/threads?q=thread", "plane_personal_key", ""); code != http.StatusOK {
			t.Fatalf("search: %d %s", code, body)
		}
		if code, body := do(t, http.MethodGet, ts.URL+"/api/bubbles/ws:p1:m1/heat", "plane_personal_key", ""); code != http.StatusOK {
			t.Fatalf("heat: %d %s", code, body)
		}
		// Phase 3: the thread interior too. This was the expensive one — four
		// concurrent Plane calls, one of which paged the WHOLE project to find
		// revisions, cached for 60s because it took over a second.
		if code, body := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", "plane_personal_key", ""); code != http.StatusOK {
			t.Fatalf("thread detail: %d %s", code, body)
		}
		if code, body := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1/comments", "plane_personal_key", ""); code != http.StatusOK {
			t.Fatalf("comments: %d %s", code, body)
		}
	}

	if n := calls.Load(); n != 0 {
		mu.Lock()
		defer mu.Unlock()
		t.Errorf("the board made %d Plane call(s); Phase 2 requires zero:\n  %s",
			n, strings.Join(paths, "\n  "))
	}
}

// TestColdAuthCostsOneCall is the Phase 4 acceptance test (docs/PLANE-SYNC.md).
//
// Scoping used to call Members on EVERY configured instance, so a cold auth cost
// up to 2N Plane calls and had to be cached for 5 minutes to be affordable. Only
// /users/me is a Plane call now — it is the actual credential check and must
// stay live, because serving identity from the mirror would let a revoked key
// keep working for as long as the mirror remembered the person.
func TestColdAuthCostsOneCall(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	inner := fakePlane()
	t.Cleanup(inner.Close)
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		req, _ := http.NewRequest(r.Method, inner.URL+r.URL.RequestURI(), r.Body)
		req.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	t.Cleanup(counting.Close)

	st := openStore(t)
	// THREE instances against the same fake: under the old scoping this alone
	// would have cost up to six calls per cold auth.
	for _, slug := range []string{"a", "b", "c"} {
		if err := st.AddInstance(domain.Instance{
			Slug: slug, BaseURL: counting.URL, APIKey: "admin-key", Workspace: "w", Project: "",
		}); err != nil {
			t.Fatal(err)
		}
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	mu.Lock()
	paths = nil
	mu.Unlock()

	if code, body := do(t, http.MethodGet, ts.URL+"/api/whoami", "plane_personal_key", ""); code != http.StatusOK {
		t.Fatalf("whoami: %d %s", code, body)
	}

	mu.Lock()
	got := append([]string(nil), paths...)
	mu.Unlock()
	if len(got) != 1 || !strings.HasSuffix(got[0], "/users/me") {
		t.Errorf("cold auth spent %d call(s) %v; want exactly one /users/me", len(got), got)
	}

	// Scope is NOT cached — it is a mirror lookup, so a membership change lands
	// immediately rather than lagging by authTTL. A second request re-scopes for
	// free and must not re-identify.
	mu.Lock()
	paths = nil
	mu.Unlock()
	if code, _ := do(t, http.MethodGet, ts.URL+"/api/whoami", "plane_personal_key", ""); code != http.StatusOK {
		t.Fatal("second whoami failed")
	}
	mu.Lock()
	n := len(paths)
	mu.Unlock()
	if n != 0 {
		t.Errorf("a warm auth spent %d Plane call(s); identity should still be cached", n)
	}
}

// TestCommentSurvivesPlaneOutage is the Phase 5 acceptance test
// (docs/PLANE-SYNC.md). What a person actually loses when a post fails is their
// typing, so that is what must survive — without storing their credential to
// replay later, and without the comment turning up in Plane authored by a
// service account.
func TestCommentSurvivesPlaneOutage(t *testing.T) {
	var down atomic.Bool
	var posted atomic.Int32
	inner := fakePlane()
	t.Cleanup(inner.Close)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments/") {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments/") {
			posted.Add(1)
		}
		req, _ := http.NewRequest(r.Method, inner.URL+r.URL.RequestURI(), r.Body)
		req.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	// Plane is down. The post must NOT 500 — it parks the words and says so.
	down.Store(true)
	code, body := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/comments", key, `{"body":"important thought"}`)
	if code != http.StatusAccepted {
		t.Fatalf("during outage want 202 Accepted, got %d: %s", code, body)
	}
	var draft domain.Comment
	if err := json.Unmarshal(body, &draft); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !draft.Pending || draft.DraftID == 0 || draft.Markdown != "important thought" {
		t.Fatalf("draft wrong: %+v", draft)
	}

	// It appears in the thread where it was typed, marked unsent.
	_, body = do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1/comments", key, "")
	var cs []domain.Comment
	if err := json.Unmarshal(body, &cs); err != nil {
		t.Fatalf("decode comments: %v", err)
	}
	found := false
	for _, c := range cs {
		if c.Pending && c.Markdown == "important thought" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the draft is not in the thread: %+v", cs)
	}

	// Nothing was sent to Plane, and no credential was stored to send it later.
	if n := posted.Load(); n != 0 {
		t.Errorf("posted %d comment(s) to Plane during the outage", n)
	}
	entries, err := st.ListOutbox(10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("outbox = %d entries (%v), want 1", len(entries), err)
	}
	for k, v := range entries[0].Payload {
		if k != "body" {
			t.Errorf("draft payload carries %q=%q; it must hold only the text", k, v)
		}
	}
	if strings.Contains(strings.ToLower(entries[0].Payload["body"]), "plane_personal_key") {
		t.Error("the caller's credential leaked into the draft")
	}

	// Plane comes back; the author re-sends with their live key.
	down.Store(false)
	code, body = do(t, http.MethodPost,
		fmt.Sprintf("%s/api/threads/ws:p1:wi-1/drafts/%d/retry", ts.URL, draft.DraftID), key, "")
	if code != http.StatusCreated {
		t.Fatalf("retry want 201, got %d: %s", code, body)
	}
	if n := posted.Load(); n != 1 {
		t.Errorf("retry posted %d time(s), want 1", n)
	}
	// ...and the draft is gone, not duplicated alongside the real comment.
	if entries, _ := st.ListOutbox(10); len(entries) != 0 {
		t.Errorf("draft survived a successful retry: %+v", entries)
	}
}

// A draft belongs to its author and nobody else: it carries no credential, so
// showing or re-sending it for another person is showing them words that nobody
// present can post.
func TestDraftsAreAuthorScoped(t *testing.T) {
	st := openStore(t)
	id, err := st.Enqueue(store.OutboxEntry{
		Instance: "ws", Kind: store.OutComment, TargetID: "wi-1",
		Payload: map[string]string{"body": "mine"}, AuthorEmail: "someone@else",
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ds, err := st.DraftsFor("ws", "wi-1", "owner@x"); err != nil || len(ds) != 0 {
		t.Errorf("another person's draft is visible: %+v (%v)", ds, err)
	}
	if _, ok, _ := st.Draft(id, "owner@x"); ok {
		t.Error("another person's draft is addressable by id")
	}
	if ds, _ := st.DraftsFor("ws", "wi-1", "SOMEONE@ELSE"); len(ds) != 1 {
		t.Errorf("the author cannot see their own draft (case-insensitively): %+v", ds)
	}
}

// A queued state write must not be reverted by a sync pass that still sees
// Plane's older value — the board would visibly flip back, then forward again
// when the write lands.
func TestQueuedWriteIsShieldedFromSync(t *testing.T) {
	st := openStore(t)
	if _, err := st.Enqueue(store.OutboxEntry{
		Instance: "ws", Kind: store.OutState, TargetID: "wi-1",
		Payload: map[string]string{"state_id": "state-3"}, FieldLock: "state",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	locked, err := st.LockedFields("ws", []string{"wi-1", "wi-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !locked["wi-1"]["state"] {
		t.Error("a pending state write does not lock the state field")
	}
	if locked["wi-2"]["state"] {
		t.Error("an unrelated item is locked")
	}

	// Once abandoned the lock lifts: Plane's own value becomes truth again.
	es, _ := st.ListOutbox(1)
	for i := 0; i < outboxMaxAttempts; i++ {
		if err := st.FailOutbox(es[0].ID, time.Now(), outboxMaxAttempts, "nope"); err != nil {
			t.Fatal(err)
		}
	}
	locked, _ = st.LockedFields("ws", []string{"wi-1"})
	if locked["wi-1"]["state"] {
		t.Error("an abandoned write still locks its field")
	}
	// ...but the row stays visible. Silently dropping a write is the one thing
	// an outbox must never do.
	after, _ := st.ListOutbox(10)
	if len(after) != 1 || after[0].Status != store.OutAbandoned {
		t.Errorf("abandoned entry should remain visible: %+v", after)
	}
}

// TestDegradedModeReportsStaleness covers the failure this whole refactor
// introduced (docs/PLANE-SYNC.md Phase 7): reads come from a local mirror, so a
// stopped sync leaves the board rendering confidently from ageing data. Being
// behind is fine. Being behind silently is not.
func TestDegradedModeReportsStaleness(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	actor := domain.Actor{Email: "owner@x", Instances: []string{"ws"}}

	// Never synced is stale, and is the worse kind — there is no data at all.
	got := srv.Status(actor)
	if !got.Stale {
		t.Error("a mirror that has never synced reports healthy")
	}

	warmMirror(t, srv)
	if got := srv.Status(actor); got.Stale {
		t.Errorf("a freshly synced mirror reports stale: %+v", got)
	}

	// One missed pass is ordinary — a rate-limit yield will do it — so the
	// threshold must not cry wolf at the first one.
	srv.now = func() time.Time { return time.Now().Add(planesync.DeltaInterval + time.Second) }
	if got := srv.Status(actor); got.Stale {
		t.Error("one missed sync interval should not raise the alarm")
	}

	// Three in a row means something is actually wrong.
	srv.now = func() time.Time { return time.Now().Add(4 * planesync.DeltaInterval) }
	got = srv.Status(actor)
	if !got.Stale {
		t.Fatal("the board did not report itself stale after several missed passes")
	}
	if got.Reason == "" || len(got.Instances) != 1 || !got.Instances[0].Stale {
		t.Errorf("stale status is not actionable: %+v", got)
	}
	if got.Instances[0].BehindSeconds < int(3*planesync.DeltaInterval/time.Second) {
		t.Errorf("behind_seconds understates the lag: %+v", got.Instances[0])
	}
}

// A person is told about their own unsent words, and only their own: a global
// queue depth is not something they can act on.
func TestStatusCountsOnlyYourOwnDrafts(t *testing.T) {
	st := openStore(t)
	srv := New(st, time.Hour)
	for _, who := range []string{"owner@x", "owner@x", "someone@else"} {
		if _, err := st.Enqueue(store.OutboxEntry{
			Instance: "ws", Kind: store.OutComment, TargetID: "wi-1",
			Payload: map[string]string{"body": "hi"}, AuthorEmail: who, CreatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := srv.Status(domain.Actor{Email: "owner@x"}).UnsentDrafts; n != 2 {
		t.Errorf("unsent drafts = %d, want 2 (mine only)", n)
	}
	if n := srv.Status(domain.Actor{Email: "someone@else"}).UnsentDrafts; n != 1 {
		t.Errorf("unsent drafts = %d, want 1", n)
	}
}

// Overlay rows are keyed by Plane work-item id and were only ever inserted into,
// so a deleted item left them behind forever — a slow leak, and a resurrection
// hazard if an id were reused.
func TestPruneDropsOverlayForDeletedItems(t *testing.T) {
	st := openStore(t)
	now := time.Now()
	for _, id := range []string{"wi-1", "wi-2", "gone"} {
		if err := st.SaveThreadProgress([]store.ThreadProgress{{
			ThreadID: id, LogbookHash: "h", DoneTodos: 1, LogbookAt: now,
		}}); err != nil {
			t.Fatal(err)
		}
		if err := st.RecordPulse(id, now, now); err != nil {
			t.Fatal(err)
		}
	}
	// An EMPTY live set must prune nothing: it means the mirror is not ready,
	// and deleting the whole overlay on the strength of a failed sync would be
	// catastrophic and silent.
	if n, err := st.Prune(nil); err != nil || n != 0 {
		t.Fatalf("an empty live set pruned %d rows (%v); it must prune none", n, err)
	}
	n, err := st.Prune([]string{"wi-1", "wi-2"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 { // one thread_progress + one thread_pulse row for "gone"
		t.Errorf("pruned %d rows, want 2", n)
	}
	if p, _ := st.ThreadProgressFor([]string{"gone"}); len(p) != 0 {
		t.Error("a deleted item kept its progress row")
	}
	if p, _ := st.ThreadProgressFor([]string{"wi-1"}); len(p) != 1 {
		t.Error("prune removed a live item's progress")
	}
}

// TestUpdateThreadIsEvidenceAndImmediate covers the whole point of the MCP
// surface (docs/MCP-ACCESS.md): an agent edits a Logbook, that counts as
// production, and the change is visible WITHOUT waiting for a sync pass.
func TestUpdateThreadIsEvidenceAndImmediate(t *testing.T) {
	var mu sync.Mutex
	body := "<h1>Brief</h1><p>The pull is real.</p><h2>Logbook</h2><ul><li data-checked='false'>wire</li></ul>"
	var patched int

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		cur := body
		mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread"}]}`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/work-items/"):
			var in struct {
				DescriptionHTML string `json:"description_html"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			body, patched = in.DescriptionHTML, patched+1
			mu.Unlock()
			io.WriteString(w, `{"updated_at":"2026-08-06T12:00:00Z"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":"First thread","description_html":%q,"created_at":"2026-08-06T10:00:00Z","updated_at":"2026-08-06T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],"next_page_results":false}`, cur)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	// Build the board once BEFORE editing, so the progress diff has a baseline.
	// A thread's first sighting is baselined silently — we cannot know when its
	// logbook was last touched, and stamping it "now" would fabricate heat for
	// work that might be a year old (THREAD-LIFECYCLE.md). In production the
	// refresher establishes that baseline long before anyone edits anything;
	// only a test can arrive with the edit and the first sighting at once.
	if code, b := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, ""); code != http.StatusOK {
		t.Fatalf("board: %d %s", code, b)
	}

	// Tick the todo through the artifact surface.
	logbook := "- [x] wire\n- [ ] ship"
	payload, _ := json.Marshal(map[string]string{"logbook": logbook})
	code, resp := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key, string(payload))
	if code != http.StatusOK {
		t.Fatalf("update: %d %s", code, resp)
	}

	// The Brief SURVIVED. This is the invariant that makes it safe to let an
	// agent write here at all.
	mu.Lock()
	sent := body
	mu.Unlock()
	if !strings.Contains(sent, "The pull is real.") {
		t.Errorf("the Brief was destroyed by a Logbook edit:\n%s", sent)
	}
	if !strings.Contains(sent, "ship") {
		t.Errorf("the Logbook was not written:\n%s", sent)
	}

	// Visible IMMEDIATELY — no sync pass ran between the write and this read.
	var d domain.ThreadDetail
	_, rb := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	if err := json.Unmarshal(rb, &d); err != nil {
		t.Fatal(err)
	}
	if d.Logbook == nil || !strings.Contains(d.Logbook.Markdown, "ship") {
		t.Errorf("the edit is not visible without a sync pass: %+v", d.Logbook)
	}

	// ...and it registered as production, so the thread is warm rather than
	// sitting in the grave it was born into.
	if _, err := srv.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, rb = do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	json.Unmarshal(rb, &d)
	if d.Level != "in_progress" {
		t.Errorf("a Logbook edit did not warm the thread: level=%s reason=%s", d.Level, d.Reason)
	}
}

// A Plane mention rendered to nothing at all until docs/ARTIFACT-EDITING.md
// Phase 2, so every mention in every body was invisible in the interior. The id
// resolves against the mirrored members; one we do not know must still show that
// somebody was mentioned rather than vanishing again.
func TestRewriteMentionsNamesThePerson(t *testing.T) {
	names := map[string]string{"u1": "Angel Maldonado"}
	in := md.RenderHTML("Hey [@mention](" + md.MentionScheme + "u1), and [@mention](" + md.MentionScheme + "u2) too.")

	got := rewriteMentions(in, names)
	if !strings.Contains(got, `<span class="plane-mention">@Angel Maldonado</span>`) {
		t.Errorf("known member was not named: %s", got)
	}
	if !strings.Contains(got, `<span class="plane-mention">@someone</span>`) {
		t.Errorf("unknown member vanished instead of degrading: %s", got)
	}
	if strings.Contains(got, md.MentionScheme) {
		t.Errorf("a raw mention marker leaked into the view: %s", got)
	}
	// A body with no mentions is returned untouched.
	plain := md.RenderHTML("nothing to see")
	if rewriteMentions(plain, names) != plain {
		t.Error("a body without mentions was rewritten")
	}
}

// The Phase 3 write path (docs/ARTIFACT-EDITING.md): splicing, optimistic
// concurrency, and a todo toggle that refuses rather than ticking the wrong box.
// One fake Plane, four guarantees, because they all turn on the same write.
func TestArtifactWritesAreSplicedGuardedAndIdempotent(t *testing.T) {
	var mu sync.Mutex
	const mention = `<mention-component id="n1" entity_identifier="u1" entity_name="user_mention"></mention-component>`
	const image = `<image-component data-id="i1" src="asset-7" width="269px"></image-component>`
	body := `<h1 class="editor-heading-block" data-id="h1">First thread</h1>` +
		`<p class="editor-paragraph-block" data-id="p1">The pull is real.</p>` +
		`<p class="editor-paragraph-block" data-id="p2">` + mention + ` owns this.</p>` +
		image +
		// A todo living in the DOCUMENT, with no Logbook above it — the shape 59
		// of 96 real bodies have.
		`<ul data-type="taskList"><li data-type="taskItem" data-checked="false">` +
		`<div><p>read the report</p></div></li></ul>` +
		`<h2 class="editor-heading-block" data-id="h2">Logbook</h2>` +
		`<ul data-type="taskList"><li data-type="taskItem" data-checked="false"><div><p>wire it up</p></div></li></ul>` +
		`<h2 class="editor-heading-block" data-id="h3">Definition of Done</h2>` +
		`<p class="editor-paragraph-block" data-id="p3">It works.</p>`
	patched := 0
	name := "First thread"

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		cur, curName := body, name
		mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread"}]}`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/work-items/"):
			var in struct {
				DescriptionHTML *string `json:"description_html"`
				Name            *string `json:"name"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			if in.DescriptionHTML != nil {
				body, patched = *in.DescriptionHTML, patched+1
			}
			if in.Name != nil {
				name = *in.Name
			}
			mu.Unlock()
			io.WriteString(w, `{"updated_at":"2026-08-07T12:00:00Z"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":%q,"description_html":%q,"created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],"next_page_results":false}`, curName, cur)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"
	const thread = "/api/threads/ws:p1:wi-1"

	read := func() domain.ThreadDetail {
		t.Helper()
		var d domain.ThreadDetail
		code, rb := do(t, http.MethodGet, ts.URL+thread, key, "")
		if code != http.StatusOK {
			t.Fatalf("read: %d %s", code, rb)
		}
		if err := json.Unmarshal(rb, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	sent := func() string {
		mu.Lock()
		defer mu.Unlock()
		return body
	}
	writes := func() int {
		mu.Lock()
		defer mu.Unlock()
		return patched
	}

	if code, b := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, ""); code != http.StatusOK {
		t.Fatalf("board: %d %s", code, b)
	}

	d := read()
	if d.Regions == nil {
		t.Fatal("the read model exposes no editable regions")
	}
	for _, r := range []string{"brief", "logbook", "dod"} {
		if d.Regions[r].Hash == "" {
			t.Errorf("region %q has no hash to write against", r)
		}
	}

	// 1. Saving a region you did not change costs NOTHING. Autosave fires on
	//    focus and blur; if that wrote, it would burn rate budget and, for a
	//    Logbook, stamp production for work nobody did.
	before := writes()
	payload, _ := json.Marshal(domain.ThreadEdit{
		Logbook: strPtr(d.Regions["logbook"].Markdown),
		Base:    map[string]string{"logbook": d.Regions["logbook"].Hash},
	})
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, string(payload)); code != http.StatusOK {
		t.Fatalf("no-op save: %d %s", code, b)
	}
	if writes() != before {
		t.Errorf("saving an unchanged region wrote to Plane %d time(s)", writes()-before)
	}

	// 2. A real Logbook edit lands, and the mention and image elsewhere on the
	//    page SURVIVE it. Before splicing, this write destroyed both.
	payload, _ = json.Marshal(domain.ThreadEdit{
		Logbook: strPtr("- [x] wire it up\n- [ ] ship it"),
		Base:    map[string]string{"logbook": d.Regions["logbook"].Hash},
	})
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, string(payload)); code != http.StatusOK {
		t.Fatalf("logbook edit: %d %s", code, b)
	}
	for _, must := range []string{mention, image, "The pull is real.", "It works."} {
		if !strings.Contains(sent(), must) {
			t.Errorf("a Logbook edit destroyed bytes it had no business touching:\n  lost %s", must)
		}
	}
	if !strings.Contains(sent(), "ship it") {
		t.Errorf("the edit did not land:\n%s", sent())
	}

	// 3. The base hash we just used is now stale, so re-using it must 409
	//    rather than silently overwriting whatever changed.
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, string(payload)); code != http.StatusConflict {
		t.Errorf("a stale base hash was accepted: %d %s", code, b)
	}

	// 4. A todo toggle whose text no longer matches is REFUSED, and writes
	//    nothing. Ticking the wrong box is worse than failing: it is silent, and
	//    it manufactures evidence of production.
	before = writes()
	bad, _ := json.Marshal(map[string]any{
		"region": "logbook", "index": 1, "text": "something else entirely", "done": true,
	})
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/todo", key, string(bad)); code != http.StatusConflict {
		t.Errorf("toggling a moved todo was accepted: %d %s", code, b)
	}
	if writes() != before {
		t.Error("a refused toggle still wrote to Plane")
	}

	// ...and the same toggle with the right text works.
	good, _ := json.Marshal(map[string]any{
		"region": "logbook", "index": 1, "text": "ship it", "done": true,
	})
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/todo", key, string(good)); code != http.StatusOK {
		t.Fatalf("toggle: %d %s", code, b)
	}
	if d := read(); d.Logbook == nil || !strings.Contains(d.Logbook.Markdown, "[x] ship it") {
		t.Errorf("the todo was not ticked: %+v", d.Logbook)
	}

	// 4b. The API says "brief"; the splice engine says "document". A toggle that
	//     cast the string instead of translating it silently rejected every todo
	//     living outside a Logbook — which is most of them.
	brief, _ := json.Marshal(map[string]any{
		"region": "brief", "index": 0, "text": "read the report", "done": true,
	})
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/todo", key, string(brief)); code != http.StatusOK {
		t.Fatalf("a todo in the document region was refused: %d %s", code, b)
	}
	if !strings.Contains(sent(), "read the report") || !strings.Contains(sent(), `data-checked="true"`) {
		t.Errorf("the document todo was not ticked:\n%s", sent())
	}
	if !strings.Contains(sent(), mention) {
		t.Error("ticking a document todo destroyed the mention")
	}

	// 4c. A title is the work item's Plane NAME, not part of the body, so it
	//     takes its own path and must not disturb the description at all.
	beforeBody := sent()
	ren, _ := json.Marshal(domain.ThreadEdit{Title: strPtr("Rework the intake form")})
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, string(ren)); code != http.StatusOK {
		t.Fatalf("rename: %d %s", code, b)
	}
	mu.Lock()
	gotName := name
	mu.Unlock()
	if gotName != "Rework the intake form" {
		t.Errorf("the thread was not renamed: %q", gotName)
	}
	if sent() != beforeBody {
		t.Error("a rename rewrote the description")
	}
	if d := read(); d.Title != "Rework the intake form" {
		t.Errorf("the new title is not visible without a sync pass: %q", d.Title)
	}
	// An empty title is a slip, not a rename.
	empty, _ := json.Marshal(domain.ThreadEdit{Title: strPtr("   ")})
	if code, _ := do(t, http.MethodPatch, ts.URL+thread, key, string(empty)); code != http.StatusBadRequest {
		t.Errorf("an empty title was accepted: %d", code)
	}

	// 4d. A surgical EDIT: quote one line, change it, leave the rest alone. This
	//     is what stops a caller from having to reproduce a whole section — the
	//     failure mode being that it paraphrases or appends to a stale copy.
	before4d := read().Regions["logbook"].Markdown
	edits, _ := json.Marshal(map[string]any{
		"edits": []map[string]any{
			{"region": "logbook", "old": "wire it up", "new": "wire it up properly"},
		},
	})
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, string(edits)); code != http.StatusOK {
		t.Fatalf("edit: %d %s", code, b)
	}
	if !strings.Contains(sent(), "read the report") || !strings.Contains(sent(), mention) {
		t.Error("an edit disturbed content it did not name")
	}
	after4d := read().Regions["logbook"].Markdown
	if !strings.Contains(after4d, "wire it up properly") {
		t.Errorf("the edit did not land: %q", after4d)
	}
	// Everything the edit did not quote is still exactly as it was.
	if want := strings.Replace(before4d, "wire it up", "wire it up properly", 1); after4d != want {
		t.Errorf("an edit changed more than it quoted:\n  want %q\n  got  %q", want, after4d)
	}

	// Quoting something that is not there is refused, and writes nothing.
	beforeMiss := writes()
	miss, _ := json.Marshal(map[string]any{
		"edits": []map[string]any{{"region": "logbook", "old": "- [ ] never existed", "new": "x"}},
	})
	if code, body := do(t, http.MethodPatch, ts.URL+thread, key, string(miss)); code != http.StatusBadRequest {
		t.Errorf("a stale quote was accepted: %d %s", code, body)
	}
	if writes() != beforeMiss {
		t.Error("a refused edit still wrote to Plane")
	}

	// 5. The Definition of Done is writable in its own right.
	dod, _ := json.Marshal(domain.ThreadEdit{DoD: strPtr("- [ ] it works\n- [ ] somebody said so")})
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, string(dod)); code != http.StatusOK {
		t.Fatalf("dod edit: %d %s", code, b)
	}
	if !strings.Contains(sent(), "somebody said so") {
		t.Errorf("the DoD edit did not land:\n%s", sent())
	}
	if !strings.Contains(sent(), mention) {
		t.Error("a DoD edit destroyed the mention")
	}
}

func strPtr(s string) *string { return &s }

// A revision IS its own work item, so ticking a todo inside one must write to
// the REVISION, not to the thread that owns it. Before revisions carried an id
// they were not addressable at all, and the click went to the parent's document.
func TestRevisionsAreAddressableAndTickable(t *testing.T) {
	var mu sync.Mutex
	parent := `<p data-id="p1">The parent brief.</p>`
	child := `<ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="false"><div><p>re-run the numbers</p></div></li></ul>`

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		curParent, curChild := parent, child
		mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread"}]}`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/work-items/rev-1/"):
			var in struct {
				DescriptionHTML *string `json:"description_html"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			if in.DescriptionHTML != nil {
				child = *in.DescriptionHTML
			}
			mu.Unlock()
			io.WriteString(w, `{"updated_at":"2026-08-07T12:00:00Z"}`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/work-items/"):
			t.Error("a revision's todo was written to the PARENT thread")
			io.WriteString(w, `{"updated_at":"2026-08-07T12:00:00Z"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			fmt.Fprintf(w, `{"results":[
			  {"id":"wi-1","name":"First thread","description_html":%q,"created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}},
			  {"id":"rev-1","name":"rev: findings","parent":"wi-1","description_html":%q,"created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}
			],"next_page_results":false}`, curParent, curChild)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	var d domain.ThreadDetail
	_, rb := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	if err := json.Unmarshal(rb, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Revisions) != 1 {
		t.Fatalf("want 1 revision, got %d", len(d.Revisions))
	}
	// The id is what makes it addressable — without it the click had nowhere to go.
	if d.Revisions[0].ID != "ws:p1:rev-1" {
		t.Fatalf("revision id: %q", d.Revisions[0].ID)
	}

	body, _ := json.Marshal(map[string]any{
		"region": "brief", "index": 0, "text": "re-run the numbers", "done": true,
	})
	if code, b := do(t, http.MethodPost, ts.URL+"/api/threads/"+d.Revisions[0].ID+"/todo", key, string(body)); code != http.StatusOK {
		t.Fatalf("toggle in a revision: %d %s", code, b)
	}
	mu.Lock()
	got := child
	mu.Unlock()
	if !strings.Contains(got, `data-checked="true"`) {
		t.Errorf("the revision's todo was not ticked:\n%s", got)
	}
	if !strings.Contains(got, "re-run the numbers") {
		t.Errorf("the revision's body was damaged:\n%s", got)
	}
}

// A bubble created through MCP or the API must be visible IMMEDIATELY.
//
// It regressed when the board moved off Plane and onto the mirror
// (docs/PLANE-SYNC.md Phase 2): CreateBubble dropped the instance cache, which
// used to force a Plane refetch and afterwards only forced a rebuild from a
// mirror that had never heard of the new module. Modules are re-read on the
// TEN MINUTE structure cadence, so the bubble existed in Plane and no surface
// could see it — not the board, not list_bubbles, not an agent looking for its
// id.
func TestNewBubblesAndThreadsAreVisibleImmediately(t *testing.T) {
	var mu sync.Mutex
	modules := `{"results":[]}`

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		mods := modules
		mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/modules/"):
			// Plane accepts it. The mirror will not learn of it for ten minutes.
			io.WriteString(w, `{"id":"m-new","name":"Fresh bubble"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/work-items/"):
			io.WriteString(w, `{"id":"wi-new","name":"First thread"}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, mods)
		case strings.Contains(p, "/module-issues/"):
			io.WriteString(w, `{"results":[]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			io.WriteString(w, `{"results":[],"next_page_results":false}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	mk, _ := json.Marshal(map[string]string{
		"instance": "ws", "project": "p1", "name": "Fresh bubble", "outcome": "it ships",
	})
	code, rb := do(t, http.MethodPost, ts.URL+"/api/bubbles", key, string(mk))
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, rb)
	}
	var made domain.NewBubble
	if err := json.Unmarshal(rb, &made); err != nil {
		t.Fatal(err)
	}

	// NO sync pass has run. The board must show it anyway.
	var board []domain.BubbleView
	_, bb := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	if err := json.Unmarshal(bb, &board); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range board {
		if b.ID == made.ID {
			found = true
			if b.Name != "Fresh bubble" {
				t.Errorf("bubble name: %q", b.Name)
			}
		}
	}
	if !found {
		t.Fatalf("a bubble created a moment ago is invisible to every surface: %s not in %d bubbles",
			made.ID, len(board))
	}

	// ...and a thread born into it is visible without a sync pass too, attached
	// to the right bubble rather than orphaned.
	birth, _ := json.Marshal(map[string]any{
		"instance": "ws", "bubble_id": made.ID, "name": "First thread",
		"brief": "why\n\n## Definition of Done\n- [ ] it works", "logbook": "- [ ] start",
	})
	if code, b := do(t, http.MethodPost, ts.URL+"/api/threads/birth", key, string(birth)); code >= 300 {
		t.Fatalf("birth: %d %s", code, b)
	}
	_, tb := do(t, http.MethodGet, ts.URL+"/api/bubbles/"+made.ID+"/threads", key, "")
	var threads []domain.ThreadNode
	if err := json.Unmarshal(tb, &threads); err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].Title != "First thread" {
		t.Errorf("a thread born a moment ago is not in its bubble: %+v", threads)
	}
}

// Deleting is irreversible and it deletes from PLANE, so the things worth
// pinning are what it takes with it and what it leaves alone.
func TestDeleteRemovesFromPlaneAndEverywhereElse(t *testing.T) {
	var mu sync.Mutex
	deleted := map[string]bool{}
	body := `<p data-id="p1">The brief.</p>` +
		`<h2>Logbook</h2><ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="false"><div><p>do it</p></div></li></ul>`

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		cur, gone := body, map[string]bool{}
		for k, v := range deleted {
			gone[k] = v
		}
		mu.Unlock()
		switch {
		case r.Method == http.MethodDelete:
			mu.Lock()
			for _, id := range []string{"m1", "wi-1", "rev-1"} {
				if strings.Contains(p, "/"+id+"/") {
					deleted[id] = true
				}
			}
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			if gone["m1"] {
				io.WriteString(w, `{"results":[]}`)
				return
			}
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.Contains(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread"}]}`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/work-items/"):
			var in struct {
				DescriptionHTML *string `json:"description_html"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			if in.DescriptionHTML != nil {
				body = *in.DescriptionHTML
			}
			mu.Unlock()
			io.WriteString(w, `{"updated_at":"2026-08-07T12:00:00Z"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			fmt.Fprintf(w, `{"results":[
			  {"id":"wi-1","name":"First thread","description_html":%q,"created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}},
			  {"id":"rev-1","name":"rev: findings","parent":"wi-1","description_html":"<p>notes</p>","created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}
			],"next_page_results":false}`, cur)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"
	const thread = "/api/threads/ws:p1:wi-1"

	// 1. Deleting an ARTIFACT takes the section and its heading, and nothing else.
	if code, b := do(t, http.MethodDelete, ts.URL+thread+"/regions/logbook", key, ""); code != http.StatusOK {
		t.Fatalf("delete region: %d %s", code, b)
	}
	mu.Lock()
	after := body
	mu.Unlock()
	if strings.Contains(after, "Logbook") || strings.Contains(after, "do it") {
		t.Errorf("the Logbook survived its own deletion:\n%s", after)
	}
	if !strings.Contains(after, `<p data-id="p1">The brief.</p>`) {
		t.Errorf("deleting the Logbook disturbed the Brief:\n%s", after)
	}

	// 2. Deleting a THREAD takes its revisions with it, in Plane.
	code, rb := do(t, http.MethodDelete, ts.URL+thread, key, "")
	if code != http.StatusOK {
		t.Fatalf("delete thread: %d %s", code, rb)
	}
	var out map[string]int
	json.Unmarshal(rb, &out)
	if out["deleted_revisions"] != 1 {
		t.Errorf("revisions taken down with the thread: want 1, got %d", out["deleted_revisions"])
	}
	mu.Lock()
	killedThread, killedRev := deleted["wi-1"], deleted["rev-1"]
	mu.Unlock()
	if !killedThread || !killedRev {
		t.Errorf("Plane was not asked to delete both: thread=%v revision=%v", killedThread, killedRev)
	}
	// The overlay must not keep a baseline for a thread that no longer exists.
	if p, err := st.ThreadProgressFor([]string{"wi-1"}); err == nil {
		if _, stale := p["wi-1"]; stale {
			t.Error("a deleted thread left its progress baseline behind")
		}
	}

	// 3. Deleting a BUBBLE reports what it unbubbled, and leaves the board.
	code, rb = do(t, http.MethodDelete, ts.URL+"/api/bubbles/ws:p1:m1", key, "")
	if code != http.StatusOK {
		t.Fatalf("delete bubble: %d %s", code, rb)
	}
	mu.Lock()
	killedModule := deleted["m1"]
	mu.Unlock()
	if !killedModule {
		t.Error("Plane was not asked to delete the module")
	}
	var board []domain.BubbleView
	_, bb := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	json.Unmarshal(bb, &board)
	for _, b := range board {
		if b.ID == "ws:p1:m1" {
			t.Error("a deleted bubble is still on the board")
		}
	}
	if _, ok, _ := st.GetContract("ws:p1:m1"); ok {
		t.Error("a deleted bubble left its contract behind")
	}
}

// Plane scopes membership PER PROJECT — on a real workspace every project can be
// private with its own member list — so scoping the board by WORKSPACE alone
// showed every project's work to anyone who could log in.
//
// This is the boundary, so it is checked from both sides: what the board offers,
// and what happens when somebody addresses a bubble or a thread by id anyway.
func TestProjectMembershipIsTheBoundary(t *testing.T) {
	// p1 has both people; p2 has only the owner. Both are private, which is what
	// the real workspace looked like when this was found.
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			// Whoever presents a key is "the outsider" unless it is the admin key.
			if r.Header.Get("X-API-Key") == "admin-key" {
				io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
			} else {
				io.WriteString(w, `{"id":"u2","email":"outsider@x","display_name":"Outsider"}`)
			}
		case strings.HasSuffix(p, "/projects/p1/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner"},
			                   {"id":"u2","email":"outsider@x","display_name":"Outsider"}]`)
		case strings.HasSuffix(p, "/projects/p2/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner"}]`)
		case strings.HasSuffix(p, "/members/"):
			// The WORKSPACE has both — which is exactly why it is the wrong list
			// to authorize with.
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20},
			                   {"id":"u2","email":"outsider@x","display_name":"Outsider","role":15}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Shared","identifier":"SH"},
			                              {"id":"p2","name":"Private","identifier":"PV"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.Contains(p, "/p1/") && strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Shared bubble"}]}`)
		case strings.Contains(p, "/p2/") && strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m2","name":"Private bubble"}]}`)
		case strings.Contains(p, "/p1/") && strings.Contains(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"Shared thread"}]}`)
		case strings.Contains(p, "/p2/") && strings.Contains(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-2","name":"Private thread"}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			id, name := "wi-1", "Shared thread"
			if strings.Contains(p, "/p2/") {
				id, name = "wi-2", "Private thread"
			}
			fmt.Fprintf(w, `{"results":[{"id":%q,"name":%q,"description_html":"<p>secret</p>","created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],"next_page_results":false}`, id, name)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	const outsider = "outsider_plane_key" // a workspace member, NOT a member of p2

	var board []domain.BubbleView
	code, rb := do(t, http.MethodGet, ts.URL+"/api/bubbles", outsider, "")
	if code != http.StatusOK {
		t.Fatalf("board: %d %s", code, rb)
	}
	if err := json.Unmarshal(rb, &board); err != nil {
		t.Fatal(err)
	}
	for _, b := range board {
		if strings.Contains(b.ID, ":p2:") {
			t.Errorf("a private project's bubble was on the board of a non-member: %s (%s)", b.ID, b.Name)
		}
	}
	// ...and the one they DO belong to is still there, or this is just a blackout.
	shared := false
	for _, b := range board {
		if strings.Contains(b.ID, ":p1:") {
			shared = true
		}
	}
	if !shared {
		t.Fatal("scoping hid a project the person IS a member of")
	}

	// Hiding it from the board is no protection if you can still address it by
	// id. Either refusal is correct here: 403 says "exists, not yours" and 404
	// says nothing at all — and where the id is resolved against the caller's
	// own board, 404 is the better answer, because 403 would confirm that a
	// bubble by that id exists.
	refused := func(t *testing.T, what string, code int, body string) {
		t.Helper()
		if code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("%s by a non-member: want 403 or 404, got %d %s", what, code, body)
		}
		if strings.Contains(body, "secret") || strings.Contains(body, "Private thread") {
			t.Errorf("%s leaked content of a project the caller is not in: %s", what, body)
		}
	}
	for _, path := range []string{
		"/api/bubbles/ws:p2:m2/threads",
		"/api/threads/ws:p2:wi-2",
		"/api/threads/ws:p2:wi-2/comments",
	} {
		code, body := do(t, http.MethodGet, ts.URL+path, outsider, "")
		refused(t, "GET "+path, code, string(body))
	}
	// Writes too — reading is not the only way to learn what is in there.
	edit, _ := json.Marshal(domain.ThreadEdit{Logbook: strPtr("- [ ] mine now")})
	code, body := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p2:wi-2", outsider, string(edit))
	refused(t, "PATCH thread", code, string(body))
	code, body = do(t, http.MethodDelete, ts.URL+"/api/bubbles/ws:p2:m2", outsider, "")
	refused(t, "DELETE bubble", code, string(body))
}

// A move must be a MOVE. Plane lets a work item belong to several modules at
// once and the board shows it in each, so removing only the bubble you named
// would quietly make this a copy — the thread would appear twice.
func TestMoveThreadLeavesTheOldBubble(t *testing.T) {
	var mu sync.Mutex
	// membership as Plane sees it
	inModule := map[string][]string{"m1": {"wi-1"}, "m2": {}}

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/projects/p1/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner"}]`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Kiosko"},{"id":"m2","name":"POS"}]}`)

		case r.Method == http.MethodDelete && strings.Contains(p, "/module-issues/"):
			mod := segment(p, "modules")
			item := lastSegment(p)
			mu.Lock()
			kept := inModule[mod][:0]
			for _, id := range inModule[mod] {
				if id != item {
					kept = append(kept, id)
				}
			}
			inModule[mod] = kept
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPost && strings.HasSuffix(p, "/module-issues/"):
			var in struct {
				Issues []string `json:"issues"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			mod := segment(p, "modules")
			mu.Lock()
			inModule[mod] = append(inModule[mod], in.Issues...)
			mu.Unlock()
			io.WriteString(w, `{}`)

		case strings.Contains(p, "/module-issues/"):
			mu.Lock()
			ids := append([]string(nil), inModule[segment(p, "modules")]...)
			mu.Unlock()
			out := make([]string, 0, len(ids))
			for _, id := range ids {
				out = append(out, fmt.Sprintf(`{"id":%q,"name":"First thread"}`, id))
			}
			fmt.Fprintf(w, `{"results":[%s]}`, strings.Join(out, ","))

		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread","description_html":"<p>body</p>","created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],"next_page_results":false}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(fake.Close)

	st := openStore(t)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	const key = "plane_personal_key"

	body, _ := json.Marshal(map[string]string{"bubble_id": "ws:p1:m2"})
	if code, b := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/move", key, string(body)); code != http.StatusOK {
		t.Fatalf("move: %d %s", code, b)
	}

	mu.Lock()
	old, new_ := append([]string(nil), inModule["m1"]...), append([]string(nil), inModule["m2"]...)
	mu.Unlock()
	if len(old) != 0 {
		t.Errorf("the thread is still in its old bubble — that is a copy, not a move: %v", old)
	}
	if len(new_) != 1 || new_[0] != "wi-1" {
		t.Errorf("the thread did not arrive in the target bubble: %v", new_)
	}

	// Visible on the board immediately — module lists are only re-read on the
	// ten-minute structure cadence, so this depends on the write-through.
	var board []domain.BubbleView
	_, bb := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	json.Unmarshal(bb, &board)
	for _, b := range board {
		if b.ID == "ws:p1:m1" && b.Threads != 0 {
			t.Errorf("the old bubble still lists %d thread(s) without a sync pass", b.Threads)
		}
		if b.ID == "ws:p1:m2" && b.Threads != 1 {
			t.Errorf("the new bubble lists %d thread(s), want 1", b.Threads)
		}
	}

	// Moving it where it already is changes nothing rather than erroring.
	if code, b := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/move", key, string(body)); code != http.StatusOK {
		t.Errorf("a no-op move failed: %d %s", code, b)
	}
}

func segment(path, after string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == after && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func lastSegment(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
