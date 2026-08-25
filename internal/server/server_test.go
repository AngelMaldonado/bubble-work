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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/heat"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
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
		case strings.HasSuffix(p, "/relations/remove/"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(p, "/relations/") && r.Method == http.MethodPost:
			io.WriteString(w, `[]`)
		case strings.HasSuffix(p, "/relations/"):
			io.WriteString(w, `{"relates_to":["rev-1"],"blocking":[],"blocked_by":[]}`)
		case strings.HasSuffix(p, "/labels/") && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":"lab-new","name":"infra","color":"#123456"}`)
		case strings.HasSuffix(p, "/labels/"):
			io.WriteString(w, `{"results":[{"id":"lab-1","name":"bug","color":"#ff0000"}]}`)
		case strings.HasSuffix(p, "/links/") && r.Method == http.MethodPost:
			io.WriteString(w, `{"id":"lnk-1","url":"https://example.test/pr/1","title":"PR #1","created_at":"2026-08-07T12:00:00Z"}`)
		case strings.HasSuffix(p, "/links/"):
			io.WriteString(w, `{"results":[{"id":"lnk-1","url":"https://example.test/pr/1","title":"PR #1","created_at":"2026-08-07T12:00:00Z"}]}`)
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
// Since docs/journal/PLANE-SYNC.md Phase 2 the board is BUILT from sqlite rather than
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
	body := `{"bubble_id":"ws:p2:m2","name":"New thread",
	          "brief":"### Context\nwhy\n\n## Definition of Done\n- [ ] tests pass",
	          "logbook":"- [ ] phase 1"}`

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

// Creating a thread asks for a name and nothing else (docs/decisions/0005). The old
// policy gate — Brief, a checkbox Definition of Done, a Logbook — is gone: what it
// wanted is still SAID, in the result's message, and none of it refuses.
func TestCreateThreadAsksOnlyForAName(t *testing.T) {
	ts, key := authedServer(t)
	url := ts.URL + "/api/threads/birth"

	if code, _ := do(t, http.MethodPost, url, "", `{}`); code != http.StatusUnauthorized {
		t.Fatalf("unauth birth: want 401, got %d", code)
	}
	if code, body := do(t, http.MethodPost, url, key, `{"bubble_id":"ws:p1:m1"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("a nameless thread: want 422, got %d (%s)", code, body)
	}

	// A name alone. No Brief, no Definition of Done, no Logbook.
	code, body := do(t, http.MethodPost, url, key, `{"bubble_id":"ws:p1:m1","name":"Just a name"}`)
	if code != http.StatusOK {
		t.Fatalf("a name-only thread: want 200, got %d (%s)", code, body)
	}
	if !strings.Contains(string(body), "no document yet") {
		t.Errorf("the result should say what is missing without refusing it: %s", body)
	}

	// A free-form document, in the author's own shape: no reserved headings at all.
	free := `{"bubble_id":"ws:p1:m1","name":"Free form",
	          "body":"## Lo que quiero\n\nque el import no truene\n\n## Pasos\n\n- [ ] leer el CSV"}`
	code, body = do(t, http.MethodPost, url, key, free)
	if code != http.StatusOK {
		t.Fatalf("a free-form thread: want 200, got %d (%s)", code, body)
	}
	if !strings.Contains(string(body), "no Definition of Done") {
		t.Errorf("a missing finish line should be mentioned: %s", body)
	}

	// A prose Definition of Done — the payload the old gate refused twice over.
	proseDoD := `{"bubble_id":"ws:p1:m1","name":"T",
	              "brief":"### Context\nwhy\n\n## Definition of Done\nit works and it is measured",
	              "logbook":"- [ ] one"}`
	if code, body := do(t, http.MethodPost, url, key, proseDoD); code != http.StatusOK {
		t.Fatalf("a prose DoD: want 200, got %d (%s)", code, body)
	}

	// And the sectioned shape still works exactly as it did.
	ok := `{"bubble_id":"ws:p1:m1","name":"T",
	        "brief":"### Context\nwhy\n\n## Definition of Done\n- [ ] tests pass",
	        "logbook":"- [ ] one"}`
	if code, out := do(t, http.MethodPost, url, key, ok); code != http.StatusOK {
		t.Fatalf("sectioned birth: want 200, got %d (%s)", code, out)
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
	// (docs/journal/PLANE-SYNC.md Phase 3). Reading the discussion no longer "records"
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
// (docs/journal/PLANE-SYNC.md).
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

// TestColdAuthCostsOneCall is the Phase 4 acceptance test (docs/journal/PLANE-SYNC.md).
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
// (docs/journal/PLANE-SYNC.md). What a person actually loses when a post fails is their
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
// introduced (docs/journal/PLANE-SYNC.md Phase 7): reads come from a local mirror, so a
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
// surface (docs/journal/MCP-ACCESS.md): an agent edits a Logbook, that counts as
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

// A Plane mention rendered to nothing at all until docs/journal/ARTIFACT-EDITING.md
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

// The Phase 3 write path (docs/journal/ARTIFACT-EDITING.md): splicing, optimistic
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
// (docs/journal/PLANE-SYNC.md Phase 2): CreateBubble dropped the instance cache, which
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

// What a write can still be refused for, since docs/decisions/0005: structural
// damage, and nothing else. A duplicated reserved heading truncates the first
// section, so every region-scoped write afterwards addresses nothing — that is
// machinery, not taste. Shape advice (two H1s, a missing finish line) rides back as
// warnings on a write that landed.
//
// The rule is server-side, so no client can bypass it — including MCP, where §9.3
// makes an agent deliberately indistinguishable from the person it acts for.
func TestMarkdownStandardIsEnforcedOnWrites(t *testing.T) {
	var mu sync.Mutex
	body := `<h2>Brief</h2><p>why this exists.</p>` +
		`<h2>Definition of Done</h2><ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="false"><div><p>it works</p></div></li></ul>`

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
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":"First thread","description_html":%q,"created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],"next_page_results":false}`, cur)
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

	// Structural damage is still refused: a second `## Logbook` leaves the canonical
	// region empty and makes every region-scoped write address nothing.
	bad, _ := json.Marshal(domain.ThreadEdit{
		Brief: strPtr("# One\n\n## Logbook\n\n- [ ] a\n\n## Logbook\n\n- [ ] b"),
	})
	code, resp := do(t, http.MethodPatch, ts.URL+thread, key, string(bad))
	if code != http.StatusBadRequest {
		t.Fatalf("a duplicated section heading was accepted: %d %s", code, resp)
	}
	if !strings.Contains(string(resp), "ONE of each") {
		t.Errorf("the refusal should say what to change: %s", resp)
	}
	mu.Lock()
	untouched := body
	mu.Unlock()
	if strings.Contains(untouched, "- [ ] b") {
		t.Error("a refused write still reached Plane")
	}

	// Shape is somebody's business, not ours: two H1s land, and are mentioned.
	loose, _ := json.Marshal(domain.ThreadEdit{
		Brief: strPtr("# One\n\nwhy this exists.\n\n# Two\n\nmore"),
	})
	code, resp = do(t, http.MethodPatch, ts.URL+thread, key, string(loose))
	if code != http.StatusOK {
		t.Fatalf("a page with two H1s was refused: %d %s", code, resp)
	}
	var d domain.ThreadDetail
	json.Unmarshal(resp, &d)
	if len(d.Warnings) != 1 || d.Warnings[0].Rule != "one-h1" {
		t.Errorf("a write should report what it let through: %+v", d.Warnings)
	}

	// And now that the page ALREADY has two H1s, an unrelated edit is not
	// punished for somebody else's mess.
	ok, _ := json.Marshal(domain.ThreadEdit{
		Edits: []domain.RegionEdit{{Region: "brief", Old: "why this exists.", New: "why this really exists."}},
	})
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, string(ok)); code != http.StatusOK {
		t.Errorf("an unrelated edit was blocked by a pre-existing violation: %d %s", code, b)
	}
}

// A plan written under somebody's own headings is a plan (docs/decisions/0005): its
// checkboxes are addressable as region `document`, and ticking one is the same
// evidence a Logbook tick is.
func TestDocumentCheckboxesAreTickable(t *testing.T) {
	var mu sync.Mutex
	body := `<h1>Importar CSV</h1><p>- una observación en prosa</p>` +
		`<h2>Pasos</h2><ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="false"><div><p>leer el archivo</p></div></li>` +
		`<li data-type="taskItem" data-checked="false"><div><p>validar</p></div></li></ul>`

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
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"Importar CSV"}]}`)
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
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":"Importar CSV","description_html":%q,"created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],"next_page_results":false}`, cur)
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

	// read_thread reports them, with the address toggle_todo takes.
	var d domain.ThreadDetail
	_, rb := do(t, http.MethodGet, ts.URL+thread, key, "")
	if err := json.Unmarshal(rb, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Artifacts) != 1 {
		t.Fatalf("want one document artifact, got %d", len(d.Artifacts))
	}
	todos := d.Artifacts[0].Todos
	if len(todos) != 2 {
		t.Fatalf("the document's checkboxes were not reported (a prose bullet must not count): %+v", todos)
	}
	if todos[1].Region != "document" || todos[1].Index != 1 {
		t.Errorf("want region document index 1, got %+v", todos[1])
	}

	// Tick one, addressed the way it was reported.
	tick, _ := json.Marshal(map[string]any{
		"region": "document", "index": 1, "text": "validar", "done": true,
	})
	code, tb := do(t, http.MethodPost, ts.URL+thread+"/todo", key, string(tick))
	if code != http.StatusOK {
		t.Fatalf("tick a document todo: %d %s", code, tb)
	}
	mu.Lock()
	after := body
	mu.Unlock()
	if !strings.Contains(after, `data-checked="true"`) {
		t.Errorf("the tick did not reach Plane:\n%s", after)
	}
	if strings.Count(after, `data-checked="true"`) != 1 {
		t.Errorf("more than one box moved:\n%s", after)
	}
}

// Plane's own relationships (docs/decisions/0006). The framework stopped keeping a
// thread "type" of its own and stopped parsing a `### Links` section out of prose;
// these three verbs are where those questions live now — and only ONE of them is
// production, because only one of them changes anything outside the tracker.
func TestPlaneRelationshipsReplaceTheParsedFields(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
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
	const thread = "/api/threads/ws:p1:wi-1"

	// 1. Labels: by NAME, replacing the set. The known one resolves, the unknown one
	//    is created rather than refused.
	code, rb := do(t, http.MethodPost, ts.URL+thread+"/labels", key, `{"labels":["bug","infra"]}`)
	if code != http.StatusOK {
		t.Fatalf("set labels: %d %s", code, rb)
	}
	var d domain.ThreadDetail
	if err := json.Unmarshal(rb, &d); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, l := range d.Labels {
		names = append(names, l.Name)
	}
	if len(names) != 2 || names[0] != "bug" || names[1] != "infra" {
		t.Errorf("labels = %v, want [bug infra]", names)
	}

	// 2. Links: external evidence, and the answer carries it back.
	code, lb := do(t, http.MethodPost, ts.URL+thread+"/links", key,
		`{"url":"https://example.test/pr/1","title":"PR #1"}`)
	if code != http.StatusOK {
		t.Fatalf("add link: %d %s", code, lb)
	}
	if err := json.Unmarshal(lb, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Links) != 1 || d.Links[0].URL != "https://example.test/pr/1" {
		t.Fatalf("links = %+v, want the PR", d.Links)
	}
	// Publishing evidence is production, so the link count is what the sweep diffs.
	if p, err := st.ThreadProgressFor([]string{"wi-1"}); err != nil {
		t.Fatal(err)
	} else if p["wi-1"].Links != 1 {
		t.Errorf("the link was not recorded as progress: %+v", p["wi-1"])
	}

	// 3. Relations: typed, and refused when the type is not one Plane knows.
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/relations", key,
		`{"thread":"ws:p1:wi-2","type":"vaguely_about"}`); code == http.StatusOK {
		t.Errorf("an invented relation type was accepted: %s", b)
	}
	code, rel := do(t, http.MethodPost, ts.URL+thread+"/relations", key,
		`{"thread":"ws:p1:wi-2","type":"blocking"}`)
	if code != http.StatusOK {
		t.Fatalf("relate: %d %s", code, rel)
	}
	// A thread cannot relate to itself.
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/relations", key,
		`{"thread":"ws:p1:wi-1"}`); code == http.StatusOK {
		t.Errorf("a thread was allowed to relate to itself: %s", b)
	}
}

// A Workspace is the outermost container, and until now it was the one thing you
// could create and never revise: no rename, no delete. Deleting one is also the
// most destructive call on the surface — it takes every bubble and thread inside
// — so what it reports has to be true before Plane is touched, not after.
func TestWorkspaceRenameAndDelete(t *testing.T) {
	var mu sync.Mutex
	name, gone := "Sandbox", false

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		curName, dead := name, gone
		mu.Unlock()
		switch {
		case r.Method == http.MethodDelete && strings.HasSuffix(p, "/projects/p1/"):
			mu.Lock()
			gone = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPatch && strings.HasSuffix(p, "/projects/p1/"):
			var in struct {
				Name string `json:"name"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			name = in.Name
			mu.Unlock()
			io.WriteString(w, `{"id":"p1"}`)
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/projects/p1/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner"}]`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			if dead {
				io.WriteString(w, `{"results":[],"next_page_results":false}`)
				return
			}
			fmt.Fprintf(w, `{"results":[{"id":"p1","name":%q,"identifier":"SB"}],"next_page_results":false}`, curName)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			if dead {
				io.WriteString(w, `{"results":[]}`)
				return
			}
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.Contains(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread"}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			if dead {
				io.WriteString(w, `{"results":[],"next_page_results":false}`)
				return
			}
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread","description_html":"<p>x</p>",
			  "created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],
			  "next_page_results":false}`)
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
	const key = "plane_personal_key"

	// 1. Rename reaches Plane and the board tells the truth immediately — the
	// project list is only re-read on the ten-minute structure cadence, so
	// without the write-through the old name would linger.
	code, rb := do(t, http.MethodPatch, ts.URL+"/api/workspaces/ws:p1", key, `{"name":"Renamed"}`)
	if code != http.StatusOK {
		t.Fatalf("rename: %d %s", code, rb)
	}
	mu.Lock()
	planeName := name
	mu.Unlock()
	if planeName != "Renamed" {
		t.Errorf("Plane still calls it %q", planeName)
	}
	var board []domain.BubbleView
	_, bb := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	json.Unmarshal(bb, &board)
	if len(board) == 0 {
		t.Fatal("the board lost its bubble to a rename")
	}
	if board[0].ProjectName != "Renamed" {
		t.Errorf("board still shows %q", board[0].ProjectName)
	}

	// An empty name is not a rename.
	if code, _ := do(t, http.MethodPatch, ts.URL+"/api/workspaces/ws:p1", key, `{"name":"  "}`); code != http.StatusBadRequest {
		t.Errorf("blank rename: want 400, got %d", code)
	}

	// 2. Delete reports what it destroyed, and destroys it everywhere.
	code, rb = do(t, http.MethodDelete, ts.URL+"/api/workspaces/ws:p1", key, "")
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, rb)
	}
	var out map[string]int
	json.Unmarshal(rb, &out)
	if out["deleted_bubbles"] != 1 || out["deleted_threads"] != 1 {
		t.Errorf("counted before deleting: want 1 bubble + 1 thread, got %v", out)
	}
	mu.Lock()
	dead := gone
	mu.Unlock()
	if !dead {
		t.Error("Plane was never asked to delete the project")
	}
	board = nil
	_, bb = do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	json.Unmarshal(bb, &board)
	if len(board) != 0 {
		t.Errorf("a deleted workspace is still on the board: %v", board)
	}
	if _, ok, _ := st.GetContract("ws:p1:m1"); ok {
		t.Error("a deleted workspace left a bubble contract behind")
	}
	// And it is gone, not merely invisible: a second delete has nothing to find.
	if code, _ := do(t, http.MethodDelete, ts.URL+"/api/workspaces/ws:p1", key, ""); code == http.StatusOK {
		t.Error("deleting a workspace twice succeeded twice")
	}
}

// An empty workspace is invisible everywhere the board is the source: buildInstance
// skips a project with no modules, so a workspace nothing has been put in yet
// cannot be inferred from bubbles. That made a freshly created one unreachable —
// you could not even put its first bubble in, because every picker was derived
// from the board.
func TestWorkspacesListsEmptyOnes(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/projects/p1/members/"),
			strings.HasSuffix(p, "/projects/empty/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner"}]`)
		case strings.HasSuffix(p, "/projects/secret/members/"):
			io.WriteString(w, `[{"id":"u9","email":"someone@else","display_name":"Else"}]`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Has bubbles","identifier":"HB"},
			                              {"id":"empty","name":"Brand new","identifier":"BN"},
			                              {"id":"secret","name":"Not yours","identifier":"NY"}],
			                   "next_page_results":false}`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.Contains(p, "/projects/p1/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.Contains(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread"}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/projects/p1/work-items/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"First thread","description_html":"<p>x</p>",
			  "created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z","state":{"id":"s1","group":"unstarted"}}],
			  "next_page_results":false}`)
		default:
			io.WriteString(w, `{"results":[],"next_page_results":false}`)
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
	const key = "plane_personal_key"

	// The board only knows the project that has a bubble in it — that is the bug.
	var board []domain.BubbleView
	_, bb := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	json.Unmarshal(bb, &board)
	for _, b := range board {
		if b.Project == "empty" {
			t.Fatal("the board should not know an empty workspace; the fixture is wrong")
		}
	}

	var ws []domain.Workspace
	code, wb := do(t, http.MethodGet, ts.URL+"/api/workspaces", key, "")
	if code != http.StatusOK {
		t.Fatalf("list workspaces: %d %s", code, wb)
	}
	json.Unmarshal(wb, &ws)
	got := map[string]bool{}
	for _, w := range ws {
		got[w.ID] = true
		if w.Instance != "ws" {
			t.Errorf("%s came back without its instance", w.ID)
		}
	}
	if !got["empty"] {
		t.Error("an empty workspace is still invisible — that is the whole point of this list")
	}
	if !got["p1"] {
		t.Error("the workspace that does have bubbles went missing")
	}
	// Membership is still the boundary: this list must not become a way to see
	// projects the board would have hidden.
	if got["secret"] {
		t.Error("a project the caller is not a member of leaked into the workspace list")
	}
}

// Project pages are the standing documentation a workspace accumulates — specs,
// references, decision records. They are not threads: no buoyancy, no heat, no
// place on the board. What has to hold is that they round-trip through markdown
// the same way a thread's artifacts do, and that a write cannot silently land on
// top of somebody else's.
func TestProjectPages(t *testing.T) {
	var mu sync.Mutex
	body := `<h1>Spec</h1><p>The <strong>contract</strong>.</p>`
	title := "Product spec"
	locked := false
	deleted := false

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		curBody, curTitle, curLocked, gone := body, title, locked, deleted
		mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/pages/pg-1/"):
			switch r.Method {
			case http.MethodDelete:
				mu.Lock()
				deleted = true
				mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
			case http.MethodPatch:
				var in struct {
					Name            *string `json:"name"`
					DescriptionHTML *string `json:"description_html"`
				}
				json.NewDecoder(r.Body).Decode(&in)
				mu.Lock()
				if in.Name != nil {
					title = *in.Name
				}
				if in.DescriptionHTML != nil {
					body = *in.DescriptionHTML
				}
				mu.Unlock()
				io.WriteString(w, `{"id":"pg-1"}`)
			default:
				fmt.Fprintf(w, `{"id":"pg-1","name":%q,"description_html":%q,"is_locked":%v,
				  "created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T11:00:00Z"}`,
					curTitle, curBody, curLocked)
			}
		case strings.HasSuffix(p, "/pages/"):
			if r.Method == http.MethodPost {
				var in struct {
					Name string `json:"name"`
				}
				json.NewDecoder(r.Body).Decode(&in)
				fmt.Fprintf(w, `{"id":"pg-new","name":%q,"created_at":"2026-08-07T12:00:00Z",
				  "updated_at":"2026-08-07T12:00:00Z"}`, in.Name)
				return
			}
			// The list endpoint returns metadata only — no description_html —
			// which is exactly why reading a body is its own request.
			if gone {
				io.WriteString(w, `{"results":[],"next_page_results":false}`)
				return
			}
			fmt.Fprintf(w, `{"results":[
			  {"id":"pg-1","name":%q,"updated_at":"2026-08-07T11:00:00Z","archived_at":null},
			  {"id":"pg-old","name":"Retired","updated_at":"2026-08-06T09:00:00Z","archived_at":"2026-08-06T10:00:00Z"},
			  {"id":"pg-2","name":"Decisions","updated_at":"2026-08-07T13:00:00Z","archived_at":null}
			],"next_page_results":false}`, curTitle)
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/projects/p1/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner"}]`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Sandbox","identifier":"SB"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		default:
			io.WriteString(w, `{"results":[],"next_page_results":false}`)
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
	const key = "plane_personal_key"

	// 1. Listing skips archived pages and puts the most recently touched first.
	var env domain.PageList
	code, lb := do(t, http.MethodGet, ts.URL+"/api/workspaces/ws:p1/pages", key, "")
	if code != http.StatusOK {
		t.Fatalf("list pages: %d %s", code, lb)
	}
	json.Unmarshal(lb, &env)
	list := env.Pages
	if len(list) != 2 {
		t.Fatalf("want 2 live pages (the archived one dropped), got %d: %v", len(list), list)
	}
	if list[0].Title != "Decisions" {
		t.Errorf("newest change should sort first, got %q", list[0].Title)
	}
	if list[1].ID != "ws:p1:pg-1" {
		t.Errorf("page ids are namespaced like everything else, got %q", list[1].ID)
	}
	// This Plane answers the pages endpoint, so it is the record and every page
	// reports so — the flag is what the UI reads to decide whether to explain
	// itself, and it must not cry wolf on a Plane that works.
	if !env.PlaneHoldsPages {
		t.Error("a Plane that serves /pages/ should be reported as holding them")
	}
	for _, p := range list {
		if p.Storage != domain.PageInPlane {
			t.Errorf("page %s should be marked as living in Plane, got %q", p.ID, p.Storage)
		}
	}

	// 2. Reading converts Plane's editor HTML to markdown.
	var d domain.PageDetail
	code, db := do(t, http.MethodGet, ts.URL+"/api/pages/ws:p1:pg-1", key, "")
	if code != http.StatusOK {
		t.Fatalf("read page: %d %s", code, db)
	}
	json.Unmarshal(db, &d)
	if !strings.Contains(d.Markdown, "# Spec") || !strings.Contains(d.Markdown, "**contract**") {
		t.Errorf("body did not come back as markdown: %q", d.Markdown)
	}
	if d.Hash == "" {
		t.Error("no hash to write back against")
	}

	// 3. A stale base hash is refused rather than overwriting somebody else.
	stale := `{"markdown":"# Spec\n\nmine","base_hash":"not-what-you-read"}`
	if code, _ := do(t, http.MethodPatch, ts.URL+"/api/pages/ws:p1:pg-1", key, stale); code != http.StatusConflict {
		t.Errorf("stale write: want 409, got %d", code)
	}

	// 4. A page is somebody's document: two H1s is a note, not a refusal
	// (docs/decisions/0005). The one thing still refused is a duplicated reserved
	// heading, which breaks region-scoped writes.
	loose := `{"title":"Loose","markdown":"# One\n\n# Two\n"}`
	if code, bb := do(t, http.MethodPost, ts.URL+"/api/workspaces/ws:p1/pages", key, loose); code != http.StatusOK {
		t.Errorf("two H1s in a new page: want 200, got %d %s", code, bb)
	}
	dup := `{"title":"Dup","markdown":"## Logbook\n\n- [ ] a\n\n## Logbook\n\n- [ ] b\n"}`
	if code, bb := do(t, http.MethodPost, ts.URL+"/api/workspaces/ws:p1/pages", key, dup); code != http.StatusBadRequest {
		t.Errorf("a duplicated section heading in a new page: want 400, got %d %s", code, bb)
	}

	// 5. A good write lands, and the round trip preserves the markdown.
	good := `{"markdown":"# Spec\n\n## Contract\n\nIt must hold.\n"}`
	code, gb := do(t, http.MethodPatch, ts.URL+"/api/pages/ws:p1:pg-1", key, good)
	if code != http.StatusOK {
		t.Fatalf("update page: %d %s", code, gb)
	}
	code, db = do(t, http.MethodGet, ts.URL+"/api/pages/ws:p1:pg-1", key, "")
	json.Unmarshal(db, &d)
	if !strings.Contains(d.Markdown, "## Contract") || !strings.Contains(d.Markdown, "It must hold.") {
		t.Errorf("the write did not survive the round trip: %q", d.Markdown)
	}

	// 6. A page Plane has locked is read-only here too, rather than failing
	// somewhere deep inside Plane with an opaque message.
	mu.Lock()
	locked = true
	mu.Unlock()
	code, rb := do(t, http.MethodPatch, ts.URL+"/api/pages/ws:p1:pg-1", key, `{"title":"nope"}`)
	if code != http.StatusBadRequest || !strings.Contains(string(rb), "locked") {
		t.Errorf("locked page: want 400 saying so, got %d %s", code, rb)
	}
	mu.Lock()
	locked = false
	mu.Unlock()

	// 7. Delete reaches Plane.
	if code, _ := do(t, http.MethodDelete, ts.URL+"/api/pages/ws:p1:pg-1", key, ""); code != http.StatusOK {
		t.Errorf("delete page: %d", code)
	}
	mu.Lock()
	killed := deleted
	mu.Unlock()
	if !killed {
		t.Error("Plane was never asked to delete the page")
	}
}

// A thread page is ONE document — ParseThread deliberately does not split it on
// H1, because Plane uses H1 and H2 as ordinary content headings. So a "new work
// artifact" inside a thread is a new `## ` section, and until sections were
// addressable there was no way to say that: you could replace the whole document
// or quote a fragment, neither of which is "add a section".
//
// The H1 the standard refuses is not an obstacle to this. It is the reason for
// it: the table of contents starts at H2, so a section added with `#` would be
// invisible in the navigation that exists to find it.
func TestNamedSections(t *testing.T) {
	var mu sync.Mutex
	body := `<h1>Brief: Ship it</h1><p data-id="p1">The intent.</p>` +
		`<h2>Logbook</h2><ul data-type="taskList">` +
		`<li data-type="taskItem" data-checked="false"><div><p>do it</p></div></li></ul>`

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		cur := body
		mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/projects/p1/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner"}]`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Sandbox","identifier":"SB"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.Contains(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"Ship it"}]}`)
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
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":"Ship it","description_html":%q,
			  "created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z",
			  "state":{"id":"s1","group":"unstarted"}}],"next_page_results":false}`, cur)
		default:
			io.WriteString(w, `{"results":[],"next_page_results":false}`)
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
	const key = "plane_personal_key"
	const thread = "/api/threads/ws:p1:wi-1"

	read := func() string {
		mu.Lock()
		defer mu.Unlock()
		return body
	}

	// 1. A section that does not exist is CREATED, as an H2, and the Brief and
	// the Logbook are untouched around it.
	add := `{"sections":[{"title":"Diseño","markdown":"Lo decidido va aquí."}]}`
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, add); code != http.StatusOK {
		t.Fatalf("add section: %d %s", code, b)
	}
	after := read()
	if !strings.Contains(after, "Dise") || !strings.Contains(after, "Lo decidido va aqu") {
		t.Fatalf("the section did not land:\n%s", after)
	}
	if !strings.Contains(after, `<h2`) {
		t.Error("a new section must be an H2 — the TOC starts there and cannot see an H1")
	}
	if !strings.Contains(after, "The intent.") || !strings.Contains(after, "do it") {
		t.Errorf("adding a section disturbed the Brief or the Logbook:\n%s", after)
	}
	// The document keeps ONE H1: that is the rule the standard enforces, and a
	// section write must not be a way around it.
	if n := strings.Count(md.FromHTML(after), "\n# ") + strings.Count(md.FromHTML(after), "# Brief"); n > 1 {
		t.Errorf("a section write introduced a second H1:\n%s", md.FromHTML(after))
	}

	// 2. Writing it again REPLACES its content, rather than appending a twin.
	again := `{"sections":[{"title":"Diseño","markdown":"Reescrito."}]}`
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, again); code != http.StatusOK {
		t.Fatalf("rewrite section: %d %s", code, b)
	}
	after = read()
	if strings.Contains(after, "Lo decidido va aqu") {
		t.Error("rewriting a section left the old content behind")
	}
	if got := strings.Count(md.FromHTML(after), "## Dise"); got != 1 {
		t.Errorf("want exactly one heading for the section, got %d:\n%s", got, md.FromHTML(after))
	}

	// 3. A region is not a section. Writing "Logbook" through this door would
	// append a second one inside the document instead of touching the real one.
	res := `{"sections":[{"title":"Logbook","markdown":"nope"}]}`
	code, rb := do(t, http.MethodPatch, ts.URL+thread, key, res)
	if code != http.StatusBadRequest || !strings.Contains(string(rb), "logbook") {
		t.Errorf("reserved section: want 400 pointing at the field, got %d %s", code, rb)
	}

	// 4. Naming the document and a section inside it at once is a contradiction.
	both := `{"brief":"# Brief: Ship it\n\nwhole","sections":[{"title":"Diseño","markdown":"part"}]}`
	if code, _ := do(t, http.MethodPatch, ts.URL+thread, key, both); code != http.StatusBadRequest {
		t.Errorf("document + section: want 400, got %d", code)
	}

	// 5. Delete takes the heading with it.
	del := `{"sections":[{"title":"Diseño","delete":true}]}`
	if code, b := do(t, http.MethodPatch, ts.URL+thread, key, del); code != http.StatusOK {
		t.Fatalf("delete section: %d %s", code, b)
	}
	after = read()
	if strings.Contains(after, "Reescrito") || strings.Contains(md.FromHTML(after), "## Dise") {
		t.Errorf("the section survived its own deletion:\n%s", after)
	}
	if !strings.Contains(after, "The intent.") || !strings.Contains(after, "do it") {
		t.Errorf("deleting a section disturbed the rest of the page:\n%s", after)
	}

	// 6. Deleting one that was never there is an error, not a silent success.
	if code, _ := do(t, http.MethodPatch, ts.URL+thread, key, del); code != http.StatusBadRequest {
		t.Error("deleting a missing section should say so")
	}
}

// A renamed bubble must read back renamed IMMEDIATELY, everywhere.
//
// This is the same trap CreateBubble fell into (see above): modules are re-read
// from Plane on the TEN MINUTE structure cadence, so a rename that only reached
// Plane would leave every local surface — the board, list_bubbles, an agent
// looking the bubble up by name — showing the old name for minutes, with no way
// to tell it had already changed. The mirror write-through is what closes that
// window, and this pins it: NO sync pass runs between the rename and the read.
//
// It also pins what a rename must NOT do: the §4 contract and the derived level
// belong to the work, not to the label on it.
func TestRenamingABubbleIsVisibleImmediatelyAndKeepsItsContract(t *testing.T) {
	var mu sync.Mutex
	renamed := "" // what Plane was actually asked to store

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/modules/m1/"):
			b, _ := io.ReadAll(r.Body)
			var in map[string]any
			json.Unmarshal(b, &in)
			mu.Lock()
			renamed, _ = in["name"].(string)
			mu.Unlock()
			io.WriteString(w, `{"id":"m1","name":"Onboarding, take two"}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Onboarding"}]}`)
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

	const id = "ws:p1:m1"
	if code, b := do(t, http.MethodPost, ts.URL+"/api/bubbles/"+id+"/contract", key,
		`{"outcome":"signups convert 20% better","owner":"Owner"}`); code != http.StatusOK {
		t.Fatalf("set contract: %d %s", code, b)
	}

	code, rb := do(t, http.MethodPatch, ts.URL+"/api/bubbles/"+id, key, `{"name":"Onboarding, take two"}`)
	if code != http.StatusOK {
		t.Fatalf("rename: %d %s", code, rb)
	}
	var got domain.NewBubble
	if err := json.Unmarshal(rb, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Onboarding, take two" || got.ID != id {
		t.Errorf("the rename reported the wrong bubble: %+v", got)
	}

	// Plane is the system of record, so it has to have been told.
	mu.Lock()
	sent := renamed
	mu.Unlock()
	if sent != "Onboarding, take two" {
		t.Errorf("Plane was not asked to store the new name, got %q", sent)
	}

	// NO sync pass has run. The board must show the new name anyway.
	var board []domain.BubbleView
	_, bb := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	if err := json.Unmarshal(bb, &board); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range board {
		if b.ID != id {
			continue
		}
		found = true
		if b.Name != "Onboarding, take two" {
			t.Errorf("the board still shows the old name: %q", b.Name)
		}
		// The name is the handle; the contract is the work. Renaming touches one.
		if b.Outcome != "signups convert 20% better" {
			t.Errorf("the rename damaged the §4 outcome: %q", b.Outcome)
		}
		if b.Owner != "Owner" {
			t.Errorf("the rename damaged the §4 owner: %q", b.Owner)
		}
	}
	if !found {
		t.Fatal("the bubble left the board when it was renamed")
	}

	// An empty name is a slip, not a rename, and it must not reach Plane.
	if code, _ := do(t, http.MethodPatch, ts.URL+"/api/bubbles/"+id, key, `{"name":"   "}`); code != http.StatusBadRequest {
		t.Errorf("renaming to blank was allowed: %d", code)
	}
}

// A Plane with no pages API must not cost a workspace its documentation.
//
// Plane Community serves project pages only on its INTERNAL, session-authenticated
// API; the public API an API key can reach has no pages route at any current
// version, so no upgrade fixes it (docs/journal/PAGES-CAPABILITY.md). On such an instance
// this server becomes the record for pages, and the whole point is that every
// operation still works — otherwise the §2.5 tier simply does not exist there.
//
// The fake is the part that matters: it reproduces Plane's real behaviour, where
// ANY unrouted URL answers 404 with `{"error": "Page not found."}`. That body is
// why the capability cannot be read off a status code, and why the probe needs a
// control request.
func TestPagesFallBackToTheServerWhenPlaneHasNoPagesAPI(t *testing.T) {
	var mu sync.Mutex
	probes := 0 // how many times we asked Plane about pages at all

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Sandbox","identifier":"SB"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.Contains(p, "/pages"):
			// Community: no route. Note this catches the capability probe's control
			// path too, which is the point — both answers are byte-identical.
			mu.Lock()
			probes++
			mu.Unlock()
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error": "Page not found."}`)
		default:
			io.WriteString(w, `{"results":[],"next_page_results":false}`)
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
	const key = "plane_personal_key"
	const ws = "/api/workspaces/ws:p1/pages"

	// 1. Listing succeeds and SAYS Plane is not holding them. The old behaviour
	//    was an error here, which left the reading room empty and blamed a Plane
	//    upgrade that would not have helped.
	var env domain.PageList
	code, lb := do(t, http.MethodGet, ts.URL+ws, key, "")
	if code != http.StatusOK {
		t.Fatalf("listing pages on a Community Plane should work, got %d %s", code, lb)
	}
	json.Unmarshal(lb, &env)
	if env.PlaneHoldsPages {
		t.Error("a Plane with no pages route must not be reported as holding pages")
	}
	if len(env.Pages) != 0 {
		t.Errorf("nothing has been written yet, got %v", env.Pages)
	}

	// 2. Creating works, and lands here rather than nowhere.
	mk, _ := json.Marshal(map[string]string{
		"title": "Deployment contract", "markdown": "# Deployment contract\n\nOne binary.\n",
	})
	code, cb := do(t, http.MethodPost, ts.URL+ws, key, string(mk))
	if code != http.StatusOK {
		t.Fatalf("create page: %d %s", code, cb)
	}
	var made domain.PageDetail
	json.Unmarshal(cb, &made)
	if made.Storage != domain.PageInLocal {
		t.Errorf("a page created here should say so, got storage %q", made.Storage)
	}
	// The id has to announce its backend, so a read never has to guess or probe.
	if !strings.Contains(made.ID, ":loc_") {
		t.Errorf("a locally-held page needs a distinguishable id, got %q", made.ID)
	}

	// 3. It reads back, with a body and a hash — the same contract as a Plane page.
	var got domain.PageDetail
	code, rb := do(t, http.MethodGet, ts.URL+"/api/pages/"+made.ID, key, "")
	if code != http.StatusOK {
		t.Fatalf("read page: %d %s", code, rb)
	}
	json.Unmarshal(rb, &got)
	if !strings.Contains(got.Markdown, "One binary.") {
		t.Errorf("the body did not survive the round trip: %q", got.Markdown)
	}
	if got.Hash == "" || got.HTML == "" {
		t.Error("a locally-held page still owes the reader rendered HTML and a hash")
	}

	// 4. The stale-write guard is a property of editing a document, not of Plane.
	stale, _ := json.Marshal(map[string]string{"markdown": "# Nope\n", "base_hash": "notthehash"})
	if code, _ := do(t, http.MethodPatch, ts.URL+"/api/pages/"+made.ID, key, string(stale)); code != http.StatusConflict {
		t.Errorf("a stale base_hash should still be a 409, got %d", code)
	}

	// 5. A real edit sticks.
	upd, _ := json.Marshal(map[string]string{
		"title": "Deployment contract v2", "base_hash": got.Hash,
		"markdown": "# Deployment contract\n\nTwo modes.\n",
	})
	code, ub := do(t, http.MethodPatch, ts.URL+"/api/pages/"+made.ID, key, string(upd))
	if code != http.StatusOK {
		t.Fatalf("update page: %d %s", code, ub)
	}
	var after domain.PageDetail
	json.Unmarshal(ub, &after)
	if after.Title != "Deployment contract v2" || !strings.Contains(after.Markdown, "Two modes.") {
		t.Errorf("the edit did not land: %+v", after)
	}

	// 6. And it is in the list, which still reports where these live.
	code, lb2 := do(t, http.MethodGet, ts.URL+ws, key, "")
	if code != http.StatusOK {
		t.Fatalf("re-list: %d %s", code, lb2)
	}
	env = domain.PageList{}
	json.Unmarshal(lb2, &env)
	if len(env.Pages) != 1 || env.Pages[0].Title != "Deployment contract v2" {
		t.Fatalf("the page is missing from the workspace's list: %+v", env)
	}
	if env.PlaneHoldsPages {
		t.Error("the capability verdict should not have flipped")
	}

	// 7. The verdict is CACHED. Six operations have run; Plane must not have been
	//    asked about pages again, because the answer is a property of the
	//    deployment and every ask costs a request against 60/min.
	mu.Lock()
	spent := probes
	mu.Unlock()
	if spent > 2 {
		t.Errorf("the capability should be probed once (2 requests: pages + control), spent %d", spent)
	}

	// 8. Deleting works and is honest about having nothing left.
	if code, _ := do(t, http.MethodDelete, ts.URL+"/api/pages/"+made.ID, key, ""); code != http.StatusOK {
		t.Error("deleting a locally-held page should work")
	}
	if code, _ := do(t, http.MethodDelete, ts.URL+"/api/pages/"+made.ID, key, ""); code != http.StatusNotFound {
		t.Error("deleting it twice should be a 404, not a silent success")
	}
}

// A 404 from a real handler must NOT be read as "this Plane has no pages API".
//
// The two are indistinguishable by status, which is the whole difficulty: Plane
// answers an unrouted URL and a missing object with the same code. So the probe
// compares the body against a control request to a sibling path that cannot be
// routed. When they differ, a real handler answered — the route exists, and the
// problem is this project. Getting this backwards would silently start a second,
// local copy of a workspace's documentation on an instance that can hold it.
func TestARealPagesHandler404IsNotMistakenForAMissingAPI(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/p1/"):
			// Pages is switched OFF for this project, which is why its handler 404s.
			io.WriteString(w, `{"id":"p1","name":"Sandbox","identifier":"SB","page_view":false}`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Sandbox","identifier":"SB"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/pages/"):
			// A real DRF handler's 404 — note the different body.
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"detail":"Not found."}`)
		case strings.Contains(p, "/pages-capability-probe/"):
			// The catch-all, as Plane spells it.
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error": "Page not found."}`)
		default:
			io.WriteString(w, `{"results":[],"next_page_results":false}`)
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

	code, body := do(t, http.MethodGet, ts.URL+"/api/workspaces/ws:p1/pages", "plane_personal_key", "")
	if code == http.StatusOK {
		t.Fatalf("this should NOT have fallen back to local storage: %s", body)
	}
	// And the message has to name the actual fix, which is a checkbox in Plane.
	if !strings.Contains(string(body), "switched off") {
		t.Errorf("the error should say pages are switched off for the project, got %s", body)
	}
	// The capability must be remembered as SUPPORTED, or the next create would
	// write locally and split the workspace's documentation in two.
	sup, _, known, err := st.Capability("ws", "pages")
	if err != nil {
		t.Fatal(err)
	}
	if known && !sup {
		t.Error("a project-level 404 was cached as 'this Plane cannot hold pages'")
	}
}

// Pages nest, and a deleted parent must not take its children with it.
//
// Plane models pages with a parent and draws them as a tree in its own UI, so the
// hierarchy is read from it rather than invented here — and a locally-held page
// carries the same field so both kinds form ONE tree. Two things can go wrong
// quietly, which is why they are pinned: a parent from another workspace produces
// a page that exists and hangs off nothing a tree can draw, and deleting a parent
// leaves children pointing at an id that is gone. Either way the document is
// still in the database and absent from the only UI that can reach it.
func TestPagesNestAndSurviveTheirParent(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Sandbox","identifier":"SB"}],"next_page_results":false}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"s1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.Contains(p, "/pages"):
			// A Community Plane: no pages route, so these are held here.
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error": "Page not found."}`)
		default:
			io.WriteString(w, `{"results":[],"next_page_results":false}`)
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
	const key = "plane_personal_key"
	const ws = "/api/workspaces/ws:p1/pages"

	mk := func(title, parent string) domain.PageDetail {
		t.Helper()
		body, _ := json.Marshal(domain.CreatePageRequest{
			Title: title, Markdown: "# " + title + "\n", Parent: parent,
		})
		code, rb := do(t, http.MethodPost, ts.URL+ws, key, string(body))
		if code != http.StatusOK {
			t.Fatalf("create %q: %d %s", title, code, rb)
		}
		var d domain.PageDetail
		if err := json.Unmarshal(rb, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}

	root := mk("Arquitectura", "")
	if root.Parent != "" {
		t.Errorf("a page created with no parent is at the root, got %q", root.Parent)
	}
	kidA := mk("Decisiones", root.ID)
	kidB := mk("Contrato de API", root.ID)
	if kidA.Parent != root.ID || kidB.Parent != root.ID {
		t.Fatalf("children did not record their parent: %q %q", kidA.Parent, kidB.Parent)
	}

	// The parent survives a round trip through the list, which is what the tree
	// is actually built from.
	byID := func() map[string]domain.Page {
		t.Helper()
		var env domain.PageList
		code, lb := do(t, http.MethodGet, ts.URL+ws, key, "")
		if code != http.StatusOK {
			t.Fatalf("list: %d %s", code, lb)
		}
		if err := json.Unmarshal(lb, &env); err != nil {
			t.Fatal(err)
		}
		m := map[string]domain.Page{}
		for _, p := range env.Pages {
			m[p.ID] = p
		}
		return m
	}
	if got := byID()[kidA.ID].Parent; got != root.ID {
		t.Errorf("the list lost the hierarchy: child's parent is %q, want %q", got, root.ID)
	}

	// A parent in another workspace is refused. Accepting it would produce a page
	// that no tree can draw — present, and invisible.
	stray, _ := json.Marshal(domain.CreatePageRequest{
		Title: "Huérfana", Parent: "other:proj:loc_whatever",
	})
	if code, rb := do(t, http.MethodPost, ts.URL+ws, key, string(stray)); code != http.StatusBadRequest {
		t.Errorf("a cross-workspace parent should be refused, got %d %s", code, rb)
	}
	// So is one that simply does not exist.
	ghost, _ := json.Marshal(domain.CreatePageRequest{
		Title: "Fantasma", Parent: "ws:p1:loc_nothinghere",
	})
	if code, _ := do(t, http.MethodPost, ts.URL+ws, key, string(ghost)); code == http.StatusOK {
		t.Error("a parent that does not exist should be refused")
	}

	// Deleting the parent PROMOTES its children rather than orphaning them.
	if code, rb := do(t, http.MethodDelete, ts.URL+"/api/pages/"+root.ID, key, ""); code != http.StatusOK {
		t.Fatalf("delete parent: %d %s", code, rb)
	}
	after := byID()
	if len(after) != 2 {
		t.Fatalf("both children should have survived their parent, got %d pages", len(after))
	}
	for _, id := range []string{kidA.ID, kidB.ID} {
		p, ok := after[id]
		if !ok {
			t.Errorf("child %s vanished with its parent", id)
			continue
		}
		if p.Parent != "" {
			t.Errorf("child %s still points at its deleted parent (%q)", p.Title, p.Parent)
		}
	}
}

// Finishing a thread, which until now you had to leave Bubble Work to do.
//
// 🏆 is derived from Plane's state (heat.ClassifyThread, threadLevel), and
// autostate refuses to write it — autoTarget returns "" for done. So this pins
// the whole verb: the target state is resolved by GROUP rather than by a localized
// name, the write reaches Plane, the mirror reflects it with no sync pass, and the
// derived level actually flips to 🏆.
//
// The Definition of Done no longer gates it (docs/decisions/0005). It is REPORTED:
// finishing with items outstanding lands, and says what was left.
func TestCompletingAThreadReportsItsDoD(t *testing.T) {
	var mu sync.Mutex
	// One unticked DoD item, and one in the Logbook — which must NOT block, since
	// the plan can carry items that outlive the thread.
	body := `<h1>Ship the thing</h1><p>why.</p>` +
		`<h2>Logbook</h2>` +
		`<ul data-type="taskList"><li data-type="taskItem" data-checked="false"><div><p>monitor for a week</p></div></li></ul>` +
		`<h2>Definition of Done</h2>` +
		`<ul data-type="taskList"><li data-type="taskItem" data-checked="false"><div><p>tests pass</p></div></li></ul>`
	stateID, stateGroup := "s1", "unstarted"
	moves := 0

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		mu.Lock()
		cur, sid, grp := body, stateID, stateGroup
		mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			// Deliberately NOT called "Done": names are project-configured and
			// localized, so anything matching on the name would fail here.
			io.WriteString(w, `{"results":[
			  {"id":"s1","name":"Por hacer","group":"unstarted","default":true},
			  {"id":"s2","name":"Haciendo","group":"started"},
			  {"id":"s3","name":"Entregado","group":"completed","default":true}
			]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		case strings.HasSuffix(p, "/module-issues/"):
			io.WriteString(w, `{"results":[{"id":"wi-1","name":"Ship the thing"}]}`)
		case r.Method == http.MethodPatch && strings.Contains(p, "/work-items/"):
			var in struct {
				State           *string `json:"state"`
				DescriptionHTML *string `json:"description_html"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			if in.State != nil {
				moves++
				stateID = *in.State
				switch stateID {
				case "s3":
					stateGroup = "completed"
				case "s2":
					stateGroup = "started"
				default:
					stateGroup = "unstarted"
				}
			}
			if in.DescriptionHTML != nil {
				body = *in.DescriptionHTML
			}
			mu.Unlock()
			io.WriteString(w, `{"updated_at":"2026-08-07T12:00:00Z"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			fmt.Fprintf(w, `{"results":[{"id":"wi-1","name":"Ship the thing","description_html":%q,`+
				`"created_at":"2026-08-07T10:00:00Z","updated_at":"2026-08-07T10:00:00Z",`+
				`"state":{"id":%q,"group":%q}}],"next_page_results":false}`, cur, sid, grp)
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

	level := func() string {
		t.Helper()
		var d domain.ThreadDetail
		_, rb := do(t, http.MethodGet, ts.URL+thread, key, "")
		if err := json.Unmarshal(rb, &d); err != nil {
			t.Fatal(err)
		}
		return d.Level
	}

	// 1. An unmet DoD does not refuse — it is named in the answer, so the person
	//    finishing can see what they closed it over.
	code, rb := do(t, http.MethodPost, ts.URL+thread+"/complete", key, `{}`)
	if code != http.StatusOK {
		t.Fatalf("an unmet DoD must not refuse the completion, got %d %s", code, rb)
	}
	var early domain.ThreadDetail
	if err := json.Unmarshal(rb, &early); err != nil {
		t.Fatal(err)
	}
	if len(early.UnmetDoD) != 1 || early.UnmetDoD[0] != "tests pass" {
		t.Errorf("the answer should name the outstanding item, got %v", early.UnmetDoD)
	}
	if early.Level != "done" {
		t.Errorf("it should have finished, got level %q", early.Level)
	}

	// Put it back to work so the rest of the verb can be pinned from the start.
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/reopen", key, ""); code != http.StatusOK {
		t.Fatalf("reopen: %d %s", code, b)
	}

	// 2. Tick the DoD item. This is the ordinary todo path, not a special case.
	tick, _ := json.Marshal(map[string]any{
		"region": "dod", "index": 0, "text": "tests pass", "done": true,
	})
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/todo", key, string(tick)); code != http.StatusOK {
		t.Fatalf("tick the DoD item: %d %s", code, b)
	}

	// 3. Now it completes — and lands in the `completed` GROUP, whatever the
	//    project calls that column.
	code, cb := do(t, http.MethodPost, ts.URL+thread+"/complete", key, `{}`)
	if code != http.StatusOK {
		t.Fatalf("complete: %d %s", code, cb)
	}
	var done domain.ThreadDetail
	if err := json.Unmarshal(cb, &done); err != nil {
		t.Fatal(err)
	}
	if done.StateGroup != "completed" {
		t.Errorf("the thread should be in the completed group, got %q (%q)", done.StateGroup, done.State)
	}
	if done.State != "Entregado" {
		t.Errorf("resolution is by group, so it should have picked the project's own column: %q", done.State)
	}
	// The whole point: the DERIVED level flips, with no sync pass in between.
	if done.Level != "done" {
		t.Errorf("a completed thread should read 🏆, got %q", done.Level)
	}
	if got := level(); got != "done" {
		t.Errorf("a re-read should still say done, got %q — the mirror did not take the write", got)
	}

	// 4. Idempotent: completing it again is a no-op, not a second write.
	mu.Lock()
	before := moves
	mu.Unlock()
	if code, _ := do(t, http.MethodPost, ts.URL+thread+"/complete", key, `{}`); code != http.StatusOK {
		t.Error("completing an already-finished thread should be a no-op, not an error")
	}
	mu.Lock()
	after := moves
	mu.Unlock()
	if after != before {
		t.Errorf("completing twice wrote to Plane twice (%d → %d)", before, after)
	}

	// 5. Reopening puts it back to work, and is never gated by a checklist.
	code, ob := do(t, http.MethodPost, ts.URL+thread+"/reopen", key, "")
	if code != http.StatusOK {
		t.Fatalf("reopen: %d %s", code, ob)
	}
	var back domain.ThreadDetail
	if err := json.Unmarshal(ob, &back); err != nil {
		t.Fatal(err)
	}
	if back.StateGroup != "started" {
		t.Errorf("a reopened thread belongs in started, got %q", back.StateGroup)
	}
	if back.Level == "done" {
		t.Error("a reopened thread must not still read 🏆")
	}

	// 6. force is accepted and means nothing now — there is no check to skip. A
	//    caller written against the old gate keeps working.
	untick, _ := json.Marshal(map[string]any{
		"region": "dod", "index": 0, "text": "tests pass", "done": false,
	})
	if code, b := do(t, http.MethodPost, ts.URL+thread+"/todo", key, string(untick)); code != http.StatusOK {
		t.Fatalf("untick: %d %s", code, b)
	}
	code, fb := do(t, http.MethodPost, ts.URL+thread+"/complete", key, `{"force":true}`)
	if code != http.StatusOK {
		t.Fatalf("force: %d %s", code, fb)
	}
	var forced domain.ThreadDetail
	json.Unmarshal(fb, &forced)
	if forced.Level != "done" {
		t.Errorf("a forced completion should still land, got level %q", forced.Level)
	}
	if len(forced.UnmetDoD) != 1 {
		t.Errorf("it should still report what was outstanding, got %v", forced.UnmetDoD)
	}
}

// What unmetDoD reports, which is now the whole of what a Definition of Done does
// at completion time: it describes, it does not block.
func TestAThreadWithNoDoDCanStillFinish(t *testing.T) {
	if unmet := unmetDoD(`<h1>Small</h1><p>Just did it.</p>`); len(unmet) != 0 {
		t.Errorf("no DoD is not an unmet DoD, got %v", unmet)
	}
	// An empty DoD section is the same: a heading with prose under it is a
	// paragraph-shaped promise, not a checklist with nothing ticked.
	if unmet := unmetDoD(`<h2>Definition of Done</h2><p>It works.</p>`); len(unmet) != 0 {
		t.Errorf("a prose DoD has no items to be outstanding, got %v", unmet)
	}
	// But a checklist with an open box is.
	html := `<h2>Definition of Done</h2>` +
		`<ul data-type="taskList"><li data-type="taskItem" data-checked="true"><div><p>a</p></div></li>` +
		`<li data-type="taskItem" data-checked="false"><div><p>b</p></div></li></ul>`
	unmet := unmetDoD(html)
	if len(unmet) != 1 || !strings.Contains(unmet[0], "b") {
		t.Errorf("want the one open item, got %v", unmet)
	}
}

// Birth is production, so a thread is 🔥 from its first moment and the bubble that
// gained it rises with it (docs/decisions/0002).
//
// This is the bug the decision exists to fix: birth's only evidence used to be an
// EvThreadCreated appended to the in-memory cache, which (a) died with the cache
// and (b) is deliberately not production anyway. The progress sweep then baselined
// the thread SILENTLY — correctly, since it cannot know when a body it is seeing
// for the first time was last touched — so a freshly born thread read 😴 and its
// bubble stayed asleep while someone was actively starting work in it.
func TestInvariant_Threads_BirthIsProduction(t *testing.T) {
	st := openStore(t)
	// A STATEFUL fake, unlike the shared one: it lists the work item it was asked
	// to create. That matters here because the reconcile is a complete walk and
	// prunes what Plane does not list — with a stateless fake the newly born thread
	// vanishes on the next sweep, which is exactly the pass this test needs to
	// survive.
	var created []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/users/me"):
			io.WriteString(w, `{"id":"u1","email":"owner@x","display_name":"Owner"}`)
		case strings.HasSuffix(p, "/members/"):
			io.WriteString(w, `[{"id":"u1","email":"owner@x","display_name":"Owner","role":20}]`)
		case strings.HasSuffix(p, "/states/"):
			io.WriteString(w, `{"results":[{"id":"state-1","name":"Todo","group":"unstarted","default":true}]}`)
		case strings.HasSuffix(p, "/work-items/") && r.Method == http.MethodPost:
			created = append(created, "new-wid-123")
			io.WriteString(w, `{"id":"new-wid-123"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/work-items/"):
			items := make([]string, 0, len(created))
			for _, id := range created {
				items = append(items, `{"id":"`+id+`","name":"Ship the thing","description_html":"<h2>Brief</h2><p>x</p><h2>Logbook</h2><ul><li data-checked=\'false\'>first step</li></ul>","created_at":"2026-08-15T12:00:00Z","updated_at":"2026-08-15T12:00:00Z","state":{"id":"state-1","name":"Todo","group":"unstarted"}}`)
			}
			io.WriteString(w, `{"results":[`+strings.Join(items, ",")+`],"next_page_results":false}`)
		case strings.Contains(p, "/modules/m1/") && strings.HasSuffix(p, "/module-issues/"):
			items := make([]string, 0, len(created))
			for _, id := range created {
				items = append(items, `{"id":"`+id+`","name":"Ship the thing","created_at":"2026-08-15T12:00:00Z","completed_at":null,"sequence_id":9,"sort_order":9000,"assignees":["u1"],"state":"state-1"}`)
			}
			io.WriteString(w, `{"results":[`+strings.Join(items, ",")+`]}`)
		case strings.HasSuffix(p, "/projects/"):
			io.WriteString(w, `{"results":[{"id":"p1","name":"Proj One"}]}`)
		case strings.HasSuffix(p, "/modules/"):
			io.WriteString(w, `{"results":[{"id":"m1","name":"Bubble A"}]}`)
		default:
			io.WriteString(w, `{"results":[]}`)
		}
	}))
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

	body := `{"instance":"ws","bubble_id":"ws:p1:m1","name":"Ship the thing",
	          "brief":"## Context\nit is broken\n\n## Outcome\nit works\n\n## Definition of Done\n- [ ] it works",
	          "logbook":"**Owner:** me · **State:** building · **Next:** start\n\n- [ ] first step"}`
	if code, out := do(t, http.MethodPost, ts.URL+"/api/threads/birth", key, body); code != http.StatusOK {
		t.Fatalf("birth: %d %s", code, out)
	}

	// The birth is DURABLE, not a cache artifact: it survives the caches being
	// dropped, which is what the sweep does on every pass.
	prog, err := st.ThreadProgressFor([]string{"new-wid-123"})
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	born := prog["new-wid-123"]
	if born.BornAt.IsZero() {
		t.Fatalf("birth was not recorded: %+v", born)
	}
	// Seeded with the logbook AS BORN, so the first sweep reports the birth rather
	// than inventing a "logbook updated" for a plan nobody has edited yet.
	if born.LogbookHash == "" {
		t.Error("born row has no logbook fingerprint, so the next sweep will report a phantom edit")
	}

	sweep(t, srv)

	// A sweep saves progress for every thread it looked at, and it has no idea when
	// anything was born. The one timestamp nothing can reconstruct must survive it.
	prog, _ = st.ThreadProgressFor([]string{"new-wid-123"})
	if prog["new-wid-123"].BornAt.IsZero() {
		t.Fatal("the sweep cleared born_at")
	}

	code, out := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:new-wid-123", key, "")
	if code != http.StatusOK {
		t.Fatalf("thread detail: %d %s", code, out)
	}
	var d struct {
		Level  string `json:"level"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Level != "in_progress" {
		t.Fatalf("a just-born thread reads %q (%s), want in_progress", d.Level, d.Reason)
	}

	// And the reported symptom: the BUBBLE has to rise too. It does so through the
	// roll-up rather than a rule of its own — the bubble's band is the band of its
	// hottest unfinished thread, so nothing here is bubble-specific.
	code, out = do(t, http.MethodGet, ts.URL+"/api/bubbles", key, "")
	if code != http.StatusOK {
		t.Fatalf("bubbles: %d %s", code, out)
	}
	var bubbles []struct {
		ID           string         `json:"id"`
		Level        string         `json:"level"`
		ThreadLevels map[string]int `json:"thread_levels"`
	}
	if err := json.Unmarshal(out, &bubbles); err != nil {
		t.Fatalf("decode bubbles: %v", err)
	}
	var found bool
	for _, b := range bubbles {
		if b.ID != "ws:p1:m1" {
			continue
		}
		found = true
		if b.Level != "in_progress" {
			t.Errorf("the bubble that gained the thread reads %q, want in_progress (threads: %v)",
				b.Level, b.ThreadLevels)
		}
		if b.ThreadLevels["in_progress"] != 1 {
			t.Errorf("thread_levels = %v, want one in_progress", b.ThreadLevels)
		}
	}
	if !found {
		t.Fatalf("bubble ws:p1:m1 missing from %d bubbles", len(bubbles))
	}
}

// The other half of the asymmetry: a work item that merely APPEARED in Plane earns
// nothing. Nobody wrote a Brief for it, so it is a draft waiting to be born, and
// the newborn grace keeps it 😴 rather than calling it abandoned.
func TestInvariant_Threads_CreatedIsNotBorn(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	win := heat.Window{CurStart: now.Add(-week), PrevStart: now.Add(-2 * week), Length: week, Decay: week}
	tun := domain.DefaultTuning()

	fresh := domain.Thread{Active: true, Owner: "me", CreatedAt: now.Add(-time.Hour)}

	created := []domain.EvidenceEvent{{Kind: domain.EvThreadCreated, At: fresh.CreatedAt}}
	if got := threadLevel(fresh, created, domain.Dormant, win, tun, now); got != "zzzz" {
		t.Errorf("created-only: want zzzz, got %s", got)
	}
	borne := []domain.EvidenceEvent{{Kind: domain.EvThreadBorn, At: fresh.CreatedAt}}
	if got := threadLevel(fresh, borne, domain.Hot, win, tun, now); got != "in_progress" {
		t.Errorf("born: want in_progress, got %s", got)
	}
	// And a birth ages like any other evidence — it is not a permanent 🔥.
	old := domain.Thread{Active: true, Owner: "me", CreatedAt: now.AddDate(0, -6, 0)}
	stale := []domain.EvidenceEvent{{Kind: domain.EvThreadBorn, At: old.CreatedAt}}
	if got := threadLevel(old, stale, domain.Dormant, win, tun, now); got != "zzzz" {
		t.Errorf("a birth six months ago: want zzzz, got %s", got)
	}
}

// Export writes the work to disk as plain markdown (docs/decisions/0001). It is
// built BEFORE the overlay becomes the record for bodies, because a backup story
// that arrives after the thing it protects is not a backup story.
func TestInvariant_Storage_ExportWritesTheWork(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	// Pinned to ONE project on purpose: the shared fake serves the same work items
	// for every project, so a whole-workspace instance would attribute them to
	// whichever project was walked last and prove nothing about the layout.
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)

	// A locally-held page: on an instance whose Plane cannot hold pages this is the
	// only copy in existence, so an export that skipped it would be worthless.
	if err := st.PutLocalPage(store.LocalPage{
		ID: "loc_abc", Instance: "ws", Project: "p1", Title: "Architecture / notes",
		Body: "# Architecture\n\nIt is one binary.", Author: "owner@x",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put page: %v", err)
	}

	inst, _, err := srv.instanceBySlug("ws")
	if err != nil {
		t.Fatalf("instance: %v", err)
	}
	dir := t.TempDir()
	res, err := srv.Export(context.Background(), inst, dir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if res.Threads == 0 || res.Files == 0 || res.Pages != 1 {
		t.Fatalf("nothing much exported: %+v", res)
	}

	// The project directory is named after the project, falling back to its id when
	// the name was never mirrored — which is what the fake leaves us with, so the
	// assertions below find it rather than hard-coding either.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("want one project directory, got %v (%v)", entries, err)
	}
	proj := filepath.Join(dir, entries[0].Name())

	// The fixture's thread, under its bubble, with its Logbook — and the DoD is
	// written even though it is parsed out of the body separately.
	thread := filepath.Join(proj, "Bubble A", "First thread")
	if _, err := os.Stat(filepath.Join(thread, "BRIEF.md")); err != nil {
		t.Errorf("no BRIEF.md under %s: %v", thread, err)
	}
	logbook, err := os.ReadFile(filepath.Join(thread, "LOGBOOK.md"))
	if err != nil {
		t.Fatalf("no LOGBOOK.md: %v", err)
	}
	if !strings.Contains(string(logbook), "scaffold") {
		t.Errorf("logbook lost its todos:\n%s", logbook)
	}
	// A revision is a sub-work-item, so it is written inside its parent.
	if _, err := os.Stat(filepath.Join(thread, "revisions", "rev first pass.md")); err != nil {
		t.Errorf("revision not exported: %v", err)
	}
	// The page title had a slash in it. Plane names are arbitrary text, so a name
	// that escapes its directory is the failure mode to prove impossible.
	if _, err := os.Stat(filepath.Join(proj, "pages", "Architecture notes.md")); err != nil {
		t.Errorf("page not exported under a safe name: %v", err)
	}

	// A relative path would mean something different depending on how the server
	// was started, so it is refused rather than guessed at.
	if _, err := srv.Export(context.Background(), inst, "relative/path"); err == nil {
		t.Error("a relative export path should be refused")
	}
}

// Every artifact write also lands in the document store (docs/decisions/0001).
//
// Nothing reads those rows yet — that is the next step — so this is what makes the
// change safe to land on its own: the record accumulates while Plane is still
// authoritative, and a wrong row is invisible rather than damaging.
func TestInvariant_Documents_WritesAccumulateInTheStore(t *testing.T) {
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

	// Editing the Logbook through the ordinary write path.
	body := `{"logbook":"- [x] scaffold\n- [ ] wire it up\n- [ ] measure"}`
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key, body); code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, out)
	}

	docs, err := st.ThreadDocs("wi-1")
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	lb, ok := docs["logbook"]
	if !ok {
		t.Fatalf("no logbook row after an edit: %v", docs)
	}
	if !strings.Contains(lb.Markdown, "measure") {
		t.Errorf("stored logbook is not what was written:\n%s", lb.Markdown)
	}
	// The hash is the optimistic-write base, so it has to be the hash OF the stored
	// markdown — an editor sends it back to prove it is writing over what it read.
	if lb.Hash != md.Hash(lb.Markdown) {
		t.Errorf("hash %q does not match its markdown", lb.Hash)
	}
	if lb.UpdatedBy == "" {
		t.Error("stored document has no author")
	}
	// The document region rode along, because the write path stores every region
	// rather than only the one that changed: a partial record is a record that
	// cannot be read from.
	if _, ok := docs["document"]; !ok {
		t.Errorf("only the edited region was stored: %v", docs)
	}
	// And what we published is remembered, which is what stops a later sync from
	// mistaking our own write for someone editing in Plane.
	pub, err := st.Published("wi-1")
	if err != nil || pub.PublishedHash == "" {
		t.Fatalf("publish state not recorded: %+v (%v)", pub, err)
	}

	// A deliberate deletion takes the document with it. This is the ONE path
	// allowed to remove these rows — a Plane walk must never do it, because for
	// these rows there is no upstream copy to recover from.
	if code, out := do(t, http.MethodDelete, ts.URL+"/api/threads/ws:p1:wi-1", key,
		`{"confirm":"First thread"}`); code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, out)
	}
	if docs, _ := st.ThreadDocs("wi-1"); len(docs) != 0 {
		t.Errorf("documents survived a deliberate delete: %v", docs)
	}
	if pub, _ := st.Published("wi-1"); pub.PublishedHash != "" {
		t.Error("publish state survived a deliberate delete")
	}
}

// planeWithBodyFailure is the shared fake plus a switch that makes body writes
// fail, so the optimistic publish path can be exercised without unplugging
// anything. It also records the last body Plane actually received.
type planeWithBodyFailure struct {
	*httptest.Server
	failing  bool
	lastBody string
	writes   int
}

func fakePlaneFailingBodies(t *testing.T) *planeWithBodyFailure {
	t.Helper()
	f := &planeWithBodyFailure{}
	inner := fakePlane()
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/work-items/") {
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if html, ok := payload["description_html"].(string); ok {
				f.writes++
				if f.failing {
					w.WriteHeader(http.StatusInternalServerError)
					io.WriteString(w, `{"error":"plane is having a day"}`)
					return
				}
				f.lastBody = html
			}
			io.WriteString(w, `{"updated_at":"2026-08-15T12:00:00Z"}`)
			return
		}
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { f.Close(); inner.Close() })
	return f
}

// Publishing must not feed itself (docs/decisions/0001). Writing a body bumps
// Plane's updated_at, so the next sync sees a changed row — and if "changed" were
// the test for a Plane-side edit, the server would read its own publication as
// someone else's work and publish again, forever. The guard is comparing against
// the hash we PUBLISHED, never against "did this row change".
func TestInvariant_Documents_PublishDoesNotLoop(t *testing.T) {
	st := openStore(t)
	fake := fakePlaneFailingBodies(t)
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

	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key,
		`{"logbook":"- [x] scaffold\n- [ ] publish once"}`); code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, out)
	}
	writesAfterEdit := fake.writes

	// What we published is exactly what the mirror now holds: nothing to reconcile.
	pub, err := st.Published("wi-1")
	if err != nil {
		t.Fatalf("published: %v", err)
	}
	it, ok, err := srv.mirror.Item("ws", "wi-1")
	if err != nil || !ok {
		t.Fatalf("mirror item: %v", err)
	}
	if pub.PublishedHash != it.DescriptionHash {
		t.Fatalf("published hash %q != mirrored hash %q — a sync would read our own write as a Plane edit",
			pub.PublishedHash, it.DescriptionHash)
	}

	// Several sync passes must not turn into several publications.
	for i := 0; i < 3; i++ {
		sweep(t, srv)
		srv.drainOutbox(context.Background())
	}
	if fake.writes != writesAfterEdit {
		t.Errorf("syncing published again: %d writes after the edit, %d now", writesAfterEdit, fake.writes)
	}
	if entries, _ := st.ListOutbox(50); len(entries) != 0 {
		t.Errorf("a successful publish left something queued: %+v", entries)
	}
}

// Once a thread's document is stored, showing it must not depend on Plane's HTML
// at all (docs/decisions/0001). Proven by corrupting the mirrored body: if the read
// still returns what was written, it cannot have been read from there.
func TestInvariant_Documents_ReadsDoNotUsePlaneHTML(t *testing.T) {
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

	// wi-2 has no revisions, so nothing on this read path touches the mirror's HTML.
	const wid = "ws:p1:wi-2"
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/"+wid, key,
		`{"brief":"# Second thread\n\nThe real intent.","logbook":"- [ ] the real plan"}`); code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, out)
	}

	// Now poison Plane's copy. In production this would be a fidelity loss or a
	// half-rendered body; here it is unmistakable.
	it, _, err := srv.mirror.Item("ws", "wi-2")
	if err != nil {
		t.Fatalf("mirror item: %v", err)
	}
	it.DescriptionHTML = "<p>CORRUPTED — this must not be shown</p>"
	it.DescriptionHash = "poison"
	if err := srv.mirror.UpsertItems("ws", []mirror.Item{it}, time.Now()); err != nil {
		t.Fatalf("poison mirror: %v", err)
	}

	code, out := do(t, http.MethodGet, ts.URL+"/api/threads/"+wid, key, "")
	if code != http.StatusOK {
		t.Fatalf("thread detail: %d %s", code, out)
	}
	if strings.Contains(string(out), "CORRUPTED") {
		t.Fatal("the read served Plane's HTML instead of the stored document")
	}
	for _, want := range []string{"The real intent.", "the real plan"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the read lost %q from the stored document", want)
		}
	}
}

// Plane being down costs a retry, not the edit (docs/decisions/0001). The markdown
// is recorded before the network is involved, the publication is queued, and the
// field lock stops a sync pass from reverting what has not been sent yet.
func TestInvariant_Documents_PlaneDownDoesNotLoseTheEdit(t *testing.T) {
	st := openStore(t)
	fake := fakePlaneFailingBodies(t)
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

	fake.failing = true
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key,
		`{"logbook":"- [ ] written while Plane was down"}`); code != http.StatusOK {
		t.Fatalf("the edit should still succeed: %d %s", code, out)
	}

	docs, _ := st.ThreadDocs("wi-1")
	if !strings.Contains(docs["logbook"].Markdown, "while Plane was down") {
		t.Fatalf("the edit did not reach the record: %v", docs["logbook"].Markdown)
	}
	entries, _ := st.ListOutbox(50)
	if len(entries) != 1 || entries[0].Kind != store.OutDoc || entries[0].FieldLock != "description" {
		t.Fatalf("expected one queued publication holding a description lock, got %+v", entries)
	}

	// A sync pass now sees Plane's OLDER body. The lock is what stops it reverting
	// the mirror, which is what the board would otherwise show.
	sweep(t, srv)
	code, out := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	if code != http.StatusOK {
		t.Fatalf("detail: %d %s", code, out)
	}
	if !strings.Contains(string(out), "while Plane was down") {
		t.Error("a sync pass reverted an edit that was only waiting to be published")
	}

	// Editing again supersedes the queued publication rather than stacking a second
	// one: the intermediate body was never Plane's and nobody is waiting to see it.
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key,
		`{"logbook":"- [ ] written twice while Plane was down"}`); code != http.StatusOK {
		t.Fatalf("second patch: %d %s", code, out)
	}
	if entries, _ := st.ListOutbox(50); len(entries) != 1 {
		t.Errorf("queued publications stacked instead of superseding: %+v", entries)
	}

	// Plane comes back. The queue drains, and what Plane receives is the LATEST
	// body — the one anybody reading Bubble Work has been seeing all along.
	fake.failing = false
	srv.drainOutbox(context.Background())
	if !strings.Contains(fake.lastBody, "written twice while Plane was down") {
		t.Errorf("Plane did not receive the latest body: %q", fake.lastBody)
	}
	if entries, _ := st.ListOutbox(50); len(entries) != 0 {
		t.Errorf("the queue did not drain: %+v", entries)
	}
	pub, _ := st.Published("wi-1")
	it, _, _ := srv.mirror.Item("ws", "wi-1")
	if pub.PublishedHash != it.DescriptionHash {
		t.Error("after draining, the published hash does not match the mirror — the next sync would see a phantom edit")
	}
}

// Plane stays a writable surface: an edit made there is IMPORTED, not overwritten
// (docs/decisions/0001). And importing must not bounce — the imported state is by
// definition what Plane already has, so the next pass must find nothing to do.
func TestInvariant_Documents_PlaneEditWinsAndDoesNotBounce(t *testing.T) {
	st := openStore(t)
	fake := fakePlaneFailingBodies(t)
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

	// Write here first, so the thread is stored AND published — until we have
	// published something there is nothing to compare an incoming body against.
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key,
		`{"logbook":"- [ ] written in bubble work"}`); code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, out)
	}

	// Now somebody edits the work item in Plane's own editor. The sync hook is what
	// notices, so feed it the way a pass would.
	planeEdited := `<h2>Brief</h2><p>Do it.</p><h2>Logbook</h2>` +
		`<ul><li>edited in plane</li></ul>`
	srv.importPlaneEdits("ws", map[string]string{"wi-1": planeEdited})

	docs, _ := st.ThreadDocs("wi-1")
	if !strings.Contains(docs["logbook"].Markdown, "edited in plane") {
		t.Fatalf("Plane's edit did not win: %q", docs["logbook"].Markdown)
	}
	if docs["logbook"].UpdatedBy != importedBy {
		t.Errorf("imported region is not marked as coming from Plane: %q", docs["logbook"].UpdatedBy)
	}
	// Winning must not mean the other version is gone.
	prev, replacedAt, err := st.ReplacedDocs("wi-1")
	if err != nil {
		t.Fatalf("replaced docs: %v", err)
	}
	if !strings.Contains(prev["logbook"].Markdown, "written in bubble work") {
		t.Errorf("the replaced version was not kept: %+v", prev)
	}
	if replacedAt.IsZero() {
		t.Error("no replacement timestamp")
	}

	// A second pass with the SAME body is not another edit.
	before, _ := st.Published("wi-1")
	srv.importPlaneEdits("ws", map[string]string{"wi-1": planeEdited})
	after, _ := st.Published("wi-1")
	if before.PublishedHash != after.PublishedHash {
		t.Error("the same body imported twice — the loop guard is not holding")
	}
	if again, _, _ := st.ReplacedDocs("wi-1"); !strings.Contains(again["logbook"].Markdown, "written in bubble work") {
		t.Error("a no-op pass overwrote the undo copy")
	}

	// The reader is told where the body came from.
	code, out := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	if code != http.StatusOK {
		t.Fatalf("detail: %d %s", code, out)
	}
	var d struct {
		FromPlane bool `json:"from_plane"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !d.FromPlane {
		t.Error("the thread does not say its body was written in Plane")
	}

	// And the arbitration rule: with OUR publication still queued, Plane's older
	// body is not an edit to adopt.
	fake.failing = true
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key,
		`{"logbook":"- [ ] ours, queued"}`); code != http.StatusOK {
		t.Fatalf("patch while Plane is down: %d %s", code, out)
	}
	srv.importPlaneEdits("ws", map[string]string{"wi-1": planeEdited})
	docs, _ = st.ThreadDocs("wi-1")
	if !strings.Contains(docs["logbook"].Markdown, "ours, queued") {
		t.Errorf("a queued publication lost to an import it should have shielded: %q", docs["logbook"].Markdown)
	}
}

// Adoption takes an instance's existing bodies into the document store, once
// (docs/decisions/0001), so threads that predate the inversion stop depending on
// the fallback read path.
func TestInvariant_Documents_AdoptIsIdempotent(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	if err := st.AddInstance(domain.Instance{
		Slug: "ws", BaseURL: fake.URL, APIKey: "admin-key", Workspace: "w", Project: "p1",
	}); err != nil {
		t.Fatalf("add instance: %v", err)
	}
	srv := New(st, time.Hour)
	warmMirror(t, srv)
	inst, _, err := srv.instanceBySlug("ws")
	if err != nil {
		t.Fatalf("instance: %v", err)
	}

	res, err := srv.Adopt(context.Background(), inst)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if res.Adopted == 0 {
		t.Fatalf("adopted nothing: %+v", res)
	}
	docs, _ := st.ThreadDocs("wi-1")
	if !strings.Contains(docs["logbook"].Markdown, "scaffold") {
		t.Errorf("the adopted logbook is wrong: %q", docs["logbook"].Markdown)
	}
	// An adopted body is NOT marked as "written in Plane": every adopted body
	// technically came from Plane, so flagging them all would make that signal
	// meaningless on the threads where it should mean something.
	if docs["logbook"].UpdatedBy != adoptedBy {
		t.Errorf("adopted region marked %q", docs["logbook"].UpdatedBy)
	}
	// Plane already holds exactly this body, so it counts as published — otherwise
	// the first sync after adoption would read every thread as edited in Plane.
	pub, _ := st.Published("wi-1")
	it, _, _ := srv.mirror.Item("ws", "wi-1")
	if pub.PublishedHash != it.DescriptionHash {
		t.Error("adoption did not record the published state")
	}
	srv.importPlaneEdits("ws", map[string]string{"wi-1": it.DescriptionHTML})
	if again, _, _ := st.ReplacedDocs("wi-1"); len(again) != 0 {
		t.Error("the first sync after adoption imported an edit that never happened")
	}

	// Re-running must not clobber writing done since the last run.
	if err := st.PutThreadDocs([]store.ThreadDoc{{
		ThreadID: "wi-1", Region: "logbook", Markdown: "- [ ] written after adoption",
		Hash: md.Hash("- [ ] written after adoption"), UpdatedAt: time.Now(), UpdatedBy: "me",
	}}); err != nil {
		t.Fatalf("write after adoption: %v", err)
	}
	second, err := srv.Adopt(context.Background(), inst)
	if err != nil {
		t.Fatalf("second adopt: %v", err)
	}
	if second.Adopted != 0 || second.Skipped == 0 {
		t.Errorf("a second adoption did work it should have skipped: %+v", second)
	}
	docs, _ = st.ThreadDocs("wi-1")
	if !strings.Contains(docs["logbook"].Markdown, "written after adoption") {
		t.Error("re-adopting overwrote work done since the first run")
	}
}

// The document is written VERBATIM (docs/decisions/0006). The server used to add a
// `## Brief` heading and read Context / Outcome / Symptom out of what followed; now
// it knows nothing about the prose except where the Logbook and the Definition of
// Done are, because those two are addressable regions.
func TestInvariant_Artifacts_DocumentIsWrittenVerbatim(t *testing.T) {
	ts, key := authedServer(t)

	body := `{"instance":"ws","bubble_id":"ws:p1:m1","name":"Importar CSV",
	          "body":"# Importar CSV\n\n## Lo que quiero\n\nque no truene\n\n## Pasos\n\n- [ ] leer el archivo"}`
	code, out := do(t, http.MethodPost, ts.URL+"/api/threads/birth", key, body)
	if code != http.StatusOK {
		t.Fatalf("create: %d %s", code, out)
	}

	code, out = do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:new-wid-123", key, "")
	if code != http.StatusOK {
		t.Fatalf("detail: %d %s", code, out)
	}
	var d domain.ThreadDetail
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(d.Artifacts) != 1 {
		t.Fatalf("want one document artifact, got %d", len(d.Artifacts))
	}
	got := d.Artifacts[0].Markdown
	for _, want := range []string{"## Lo que quiero", "## Pasos", "- [ ] leer el archivo"} {
		if !strings.Contains(got, want) {
			t.Errorf("the author's own section did not survive (%q):\n%s", want, got)
		}
	}
	if strings.Contains(got, "## Brief") {
		t.Errorf("the server invented a heading nobody wrote:\n%s", got)
	}
	// A `type` a caller still sends is ignored rather than refused: the field is
	// gone, and rejecting an unknown key would break every script that predates it.
	typed := `{"instance":"ws","bubble_id":"ws:p1:m1","name":"y","type":"epic","body":"y"}`
	if code, out := do(t, http.MethodPost, ts.URL+"/api/threads/birth", key, typed); code != http.StatusOK {
		t.Errorf("a stale `type` field was not ignored: %d %s", code, out)
	}
}

// Any edit to the document is production (docs/decisions/0004) — the Brief
// included. Writing is the work here, so sharpening why something matters counts as
// much as ticking a box. What differs is the LABEL, not whether it warms.
func TestInvariant_Heat_AnyBodyEditIsProduction(t *testing.T) {
	st := openStore(t)
	fake := fakePlane()
	t.Cleanup(fake.Close)
	// Pinned to one project: the shared fake serves the same work items for every
	// project, so on a whole-workspace instance the mirror attributes them to
	// whichever project was walked last, and the progress diff for a bubble in the
	// OTHER project then sees none of them.
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

	// Baseline. The progress diff runs when the board is BUILT, not when the mirror
	// is filled, so a read is what records the fingerprints — and the first sighting
	// is deliberately silent about evidence.
	board := func() {
		if code, out := do(t, http.MethodGet, ts.URL+"/api/bubbles", key, ""); code != http.StatusOK {
			t.Fatalf("bubbles: %d %s", code, out)
		}
	}
	board()
	before, _ := st.ThreadProgressFor([]string{"wi-1"})
	if before["wi-1"].BodyHash == "" {
		t.Fatal("building the board did not fingerprint the whole body")
	}

	// Edit ONLY the document region — no todo ticked, no plan touched.
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key,
		`{"brief":"# First thread\n\nSharper statement of why this matters."}`); code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, out)
	}
	// Deliberately NOT re-syncing: the shared fake always serves the same canned
	// body, so a backfill would replace the mirror with the pre-edit version and this
	// would be testing the fixture rather than the rule. The write already wrote
	// through to the mirror, exactly as it does in production.
	board()

	after, _ := st.ThreadProgressFor([]string{"wi-1"})
	got := after["wi-1"]
	if got.BodyHash == before["wi-1"].BodyHash {
		t.Fatal("the body fingerprint did not move after a Brief edit")
	}
	if got.LogbookAt.IsZero() {
		t.Fatal("a Brief edit produced no evidence timestamp")
	}
	// Labelled as a document change, not as a plan change: the plan did not move.
	if got.LogbookKind != domain.EvBodyUpdated {
		t.Errorf("kind = %q, want %q", got.LogbookKind, domain.EvBodyUpdated)
	}
	if got.LogbookHash != before["wi-1"].LogbookHash {
		t.Error("the plan fingerprint moved on a Brief-only edit")
	}
	// And it is progress, so the thread reads 🔥.
	code, out := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:wi-1", key, "")
	if code != http.StatusOK {
		t.Fatalf("detail: %d %s", code, out)
	}
	var d struct {
		Level  string `json:"level"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Level != "in_progress" {
		t.Errorf("after a Brief edit the thread reads %q (%s), want in_progress", d.Level, d.Reason)
	}
}

// A comment still never WARMS anything (§5.2 stands), but a bubble people are
// actively discussing is not a grave (docs/decisions/0004). The floor is on the
// BAND only — lifecycle, score and ordering are untouched, because presence must not
// outrank output.
func TestInvariant_Heat_DiscussionKeepsABubbleOutOfTheGrave(t *testing.T) {
	srv := New(openStore(t), time.Hour)
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	srv.now = func() time.Time { return now }
	tun := domain.DefaultTuning()

	// An old, assigned, never-productive thread: 🪦 at both grains.
	abandoned := domain.Thread{ID: "t1", Active: true, Owner: "me", CreatedAt: now.AddDate(0, -6, 0)}
	bubble := domain.Bubble{
		ID: "ws:p1:m1", Name: "Cold", Owner: "me",
		Threads:  []domain.Thread{abandoned},
		Evidence: []domain.EvidenceEvent{{ThreadID: "t1", Kind: domain.EvThreadCreated, At: abandoned.CreatedAt}},
	}
	_, level, _ := srv.bubbleHeat(bubble, tun, now)
	if level != "rip" {
		t.Fatalf("baseline: want rip, got %s", level)
	}

	// Somebody commented yesterday.
	talked := bubble
	talked.Evidence = append(append([]domain.EvidenceEvent{}, bubble.Evidence...),
		domain.EvidenceEvent{ThreadID: "t1", Kind: domain.EvComment, At: now.AddDate(0, 0, -1)})

	res, level, _ := srv.bubbleHeat(talked, tun, now)
	if level != "zzzz" {
		t.Fatalf("a discussed bubble reads %s, want zzzz", level)
	}
	// The band floored; the temperature did NOT rise.
	if res.Lifecycle != domain.Dormant {
		t.Errorf("a comment changed the bubble's lifecycle to %s — presence must not warm", res.Lifecycle)
	}
	if res.Score != 0 {
		t.Errorf("a comment gave the bubble a score of %.3f — it must not affect ordering", res.Score)
	}
	// Stale chatter does not save it.
	stale := bubble
	stale.Evidence = append(append([]domain.EvidenceEvent{}, bubble.Evidence...),
		domain.EvidenceEvent{ThreadID: "t1", Kind: domain.EvComment, At: now.AddDate(0, -3, 0)})
	if _, level, _ := srv.bubbleHeat(stale, tun, now); level != "rip" {
		t.Errorf("months-old chatter kept a bubble alive: %s", level)
	}
	// And it can be switched off entirely, like the thread-grain pulse.
	off := tun
	off.PulseCycles = 0
	if _, level, _ := srv.bubbleHeat(talked, off, now); level != "rip" {
		t.Errorf("pulse_cycles=0 should disable the floor, got %s", level)
	}
}

// Birth owns the section headings, so a caller that includes them must not end up
// with two. This is the regression test for the failure a real agent session hit: the
// duplicate does not merely look untidy — the FIRST section's range stops at the
// second heading, so the canonical region is EMPTY, and every region-scoped operation
// then addresses nothing.
func TestInvariant_Artifacts_BirthDoesNotDuplicateHeadings(t *testing.T) {
	ts, key := authedServer(t)

	// Exactly what the agent sent: its own `## Brief` and `## Logbook` headings.
	body := `{"instance":"ws","bubble_id":"ws:p1:m1","name":"Reasignación","type":"feature",
	          "brief":"## Brief\n\n## Context\nwhy now\n\n## Outcome\nit works\n\n## Definition of Done\n- [ ] measured",
	          "logbook":"## Logbook\n\n### Fase 1\n- [ ] uno\n- [ ] dos"}`
	if code, out := do(t, http.MethodPost, ts.URL+"/api/threads/birth", key, body); code != http.StatusOK {
		t.Fatalf("birth: %d %s", code, out)
	}

	code, out := do(t, http.MethodGet, ts.URL+"/api/threads/ws:p1:new-wid-123", key, "")
	if code != http.StatusOK {
		t.Fatalf("detail: %d %s", code, out)
	}
	var d struct {
		Logbook *struct {
			Markdown string `json:"markdown"`
			Todos    []struct {
				Text   string `json:"text"`
				Region string `json:"region"`
				Index  int    `json:"index"`
			} `json:"todos"`
		} `json:"logbook"`
		Artifacts []struct {
			Markdown string `json:"markdown"`
		} `json:"artifacts"`
		Regions map[string]struct {
			Markdown string `json:"markdown"`
		} `json:"regions"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Logbook == nil {
		t.Fatal("no logbook")
	}
	// The plan is IN the logbook region, not stranded in the document.
	if len(d.Logbook.Todos) != 2 {
		t.Fatalf("logbook has %d todo(s); the region was truncated by a duplicate heading: %q",
			len(d.Logbook.Todos), d.Logbook.Markdown)
	}
	if got := d.Regions["logbook"].Markdown; !strings.Contains(got, "uno") {
		t.Errorf("the writable logbook region does not hold the plan: %q", got)
	}
	if len(d.Artifacts) > 0 && strings.Contains(d.Artifacts[0].Markdown, "Fase 1") {
		t.Error("the plan leaked into the document region — the Logbook heading was duplicated")
	}
	// Each todo carries the address toggle_todo takes.
	for i, td := range d.Logbook.Todos {
		if td.Region != "logbook" || td.Index != i {
			t.Errorf("todo %d addressed as %s[%d]", i, td.Region, td.Index)
		}
	}

	// And ticking one by that address works — the operation that used to answer
	// "no todo at that position".
	tick := `{"region":"logbook","index":1,"text":"dos","done":true}`
	if code, out := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:new-wid-123/todo", key, tick); code != http.StatusOK {
		t.Fatalf("toggle_todo: %d %s", code, out)
	}
}

// An index that does not exist is almost never a miscount: it is the caller
// addressing the wrong region. The refusal has to say so, because "no todo at that
// position" sent a real agent looking in the right place for the wrong reason.
func TestInvariant_Artifacts_NoSuchTodoExplainsItself(t *testing.T) {
	ts, key := authedServer(t)

	// wi-1's Logbook has two items; the DoD has none.
	code, out := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/todo", key,
		`{"region":"dod","index":0,"done":true}`)
	if code == http.StatusOK {
		t.Fatal("ticking a nonexistent DoD item succeeded")
	}
	msg := string(out)
	if !strings.Contains(msg, "no dod section") {
		t.Errorf("the error does not say what is missing: %s", msg)
	}
	if !strings.Contains(msg, "logbook has 2") {
		t.Errorf("the error does not point at the region that DOES have items: %s", msg)
	}

	// An out-of-range index in a region that has items says how many there are.
	code, out = do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/todo", key,
		`{"region":"logbook","index":9,"done":true}`)
	if code == http.StatusOK {
		t.Fatal("ticking index 9 of a two-item list succeeded")
	}
	if msg := string(out); !strings.Contains(msg, "valid indices") {
		t.Errorf("the error does not say what the valid indices are: %s", msg)
	}
}

// Ticking several items is ONE change: one call, one Plane write, all-or-nothing.
// It used to be one call and one write per item — five round trips and five writes
// against a 60-per-minute budget for something nobody thinks of as five changes.
func TestInvariant_Artifacts_TodosToggleInOneWrite(t *testing.T) {
	st := openStore(t)
	fake := fakePlaneFailingBodies(t)
	// Pinned: the shared fake serves the same items for every project, so on a
	// whole-workspace instance the progress diff for a bubble in one project sees
	// items the mirror attributed to the other — and the level would read cold.
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

	// A page with a plan and a finish line.
	setup := `{"logbook":"- [ ] uno\n- [ ] dos\n- [ ] tres","dod":"- [ ] measured\n- [ ] reviewed"}`
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key, setup); code != http.StatusOK {
		t.Fatalf("setup: %d %s", code, out)
	}
	writesBefore := fake.writes

	batch := `{"items":[
	  {"region":"logbook","index":0,"text":"uno","done":true},
	  {"region":"logbook","index":2,"text":"tres","done":true},
	  {"region":"dod","index":0,"text":"measured","done":true}
	]}`
	code, out := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/todo", key, batch)
	if code != http.StatusOK {
		t.Fatalf("batch toggle: %d %s", code, out)
	}
	if n := fake.writes - writesBefore; n != 1 {
		t.Errorf("three toggles cost %d Plane write(s), want 1", n)
	}

	// The confirmation states the PERSISTED state, so nobody has to re-read.
	var res struct {
		Confirmed domain.TodoResult `json:"confirmed"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	c := res.Confirmed
	if len(c.Applied) != 3 {
		t.Fatalf("confirmed %d item(s), want 3: %+v", len(c.Applied), c.Applied)
	}
	for _, a := range c.Applied {
		if !a.Done {
			t.Errorf("%q reads as not done in the confirmation", a.Text)
		}
	}
	if c.LogbookDone != 2 || c.LogbookOpen != 1 {
		t.Errorf("logbook counts: %d done / %d open, want 2/1", c.LogbookDone, c.LogbookOpen)
	}
	if len(c.Unmet) != 1 || c.Unmet[0] != "reviewed" {
		t.Errorf("unmet = %v, want just [reviewed]", c.Unmet)
	}
	if c.Level != "in_progress" {
		t.Errorf("level after ticking todos = %q; a ticked todo is production", c.Level)
	}

	// All-or-nothing: one bad item leaves the whole batch unapplied.
	before, _ := st.ThreadDocs("wi-1")
	bad := `{"items":[
	  {"region":"logbook","index":1,"text":"dos","done":true},
	  {"region":"logbook","index":9,"text":"nope","done":true}
	]}`
	if code, _ := do(t, http.MethodPost, ts.URL+"/api/threads/ws:p1:wi-1/todo", key, bad); code == http.StatusOK {
		t.Fatal("a batch with an impossible index succeeded")
	}
	after, _ := st.ThreadDocs("wi-1")
	if before["logbook"].Hash != after["logbook"].Hash {
		t.Error("a refused batch applied part of itself")
	}
}

// One call answers the framework's questions for a whole bubble — and reports what is
// MISSING, which is the finding nobody gets by reading threads one at a time.
func TestInvariant_Threads_AuditAnswersTheWholeBubble(t *testing.T) {
	ts, key := authedServer(t)

	// wi-1 carries a Logbook but no Definition of Done (the fixture's shape).
	code, out := do(t, http.MethodGet, ts.URL+"/api/bubbles/ws:p1:m1/audit", key, "")
	if code != http.StatusOK {
		t.Fatalf("audit: %d %s", code, out)
	}
	var a domain.BubbleAudit
	if err := json.Unmarshal(out, &a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(a.Threads) == 0 {
		t.Fatal("the audit found no threads")
	}
	if a.Counts == nil {
		t.Error("no per-level counts")
	}
	var seen bool
	for _, th := range a.Threads {
		if th.Seq != 1 {
			continue
		}
		seen = true
		if th.Level == "" {
			t.Error("a thread has no derived level")
		}
		// The fixture has no DoD, so the audit must say so rather than staying quiet.
		if th.HasDoD {
			t.Error("the fixture thread should have no Definition of Done")
		}
		if th.Missing == "" {
			t.Error("a thread with no Definition of Done was reported as intact")
		}
	}
	if !seen {
		t.Fatal("thread #1 missing from the audit")
	}
	if a.NeedsRepair == 0 {
		t.Error("NeedsRepair is 0 even though a thread has no finish line")
	}

	// The declared next action is surfaced, which is what makes the audit answer
	// "what happens next here?" without opening anything.
	next := `{"logbook":"**Owner:** me · **State:** building · **Next:** measure it\n\n- [ ] uno",
	          "dod":"- [ ] measured"}`
	if code, out := do(t, http.MethodPatch, ts.URL+"/api/threads/ws:p1:wi-1", key, next); code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, out)
	}
	code, out = do(t, http.MethodGet, ts.URL+"/api/bubbles/ws:p1:m1/audit", key, "")
	if code != http.StatusOK {
		t.Fatalf("audit: %d %s", code, out)
	}
	if err := json.Unmarshal(out, &a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, th := range a.Threads {
		if th.Seq == 1 {
			if th.Next != "measure it" {
				t.Errorf("next = %q, want %q", th.Next, "measure it")
			}
			if th.DoDTotal != 1 || th.DoDDone != 0 {
				t.Errorf("DoD progress = %d/%d, want 0/1", th.DoDDone, th.DoDTotal)
			}
		}
	}
}
