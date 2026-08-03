// Package server is the Bubble Work brain: the single interface clients use, the
// policy engine that makes the framework's rules unbypassable (§9.4), and the
// place heat is derived. It serves both a REST API (humans via CLI) and MCP
// (agents) — two front doors, one API.
package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/heat"
	"github.com/AngelMaldonado/bubble-work/internal/mcpapi"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
	"github.com/AngelMaldonado/bubble-work/web"
)

var (
	errNotFound = errors.New("bubble not found")
	errUnauth   = errors.New("unauthorized")
	errForbid   = errors.New("not authorized for this instance")
	errAmbig    = errors.New("ambiguous id")
	// errBadRequest is a client input error (empty/invalid payload) → 400.
	errBadRequest = errors.New("bad request")
	// errUpstream is a transient Plane failure during resolution. It must map to
	// 5xx (not 401) so callers retry instead of signing out, and it is never
	// cached — see resolveUncached.
	errUpstream = errors.New("upstream plane error")
)

const (
	// authTTL caches a resolved identity to avoid hitting Plane every request.
	authTTL = 5 * time.Minute
	// snapshotRefresh is how often the background refresher rebuilds each
	// instance's bubble snapshot. Reads are served from the snapshot and never
	// fetch Plane on the request path, so this cadence — not request latency —
	// governs freshness. Kept gentle: a full fan-out trips self-hosted Plane's
	// rate limit, and webhooks cover real-time changes.
	snapshotRefresh = 90 * time.Second
	// membersTTL caches an instance's member id→name map. Members change rarely,
	// and this call otherwise runs on every timeline/thread open (~0.5s each).
	membersTTL = 10 * time.Minute
	// commentsTTL caches a work item's comments so repeat chat opens don't each
	// hit Plane's paged comments endpoint. Kept short since posting invalidates it
	// and read-receipts are attached fresh regardless.
	commentsTTL = 30 * time.Second
	// detailTTL caches a thread's rendered interior. Longer than comments since a
	// thread body changes far less often than its discussion.
	detailTTL = 60 * time.Second
	// statesTTL caches a project's workflow states (id → group/name). A project's
	// states are configuration, not data — they change on the order of never.
	statesTTL = 30 * time.Minute
	// kioskTokenPrefix marks a server-issued read-only display credential so
	// resolve() can shortcut it before attempting Plane authentication (§9 Phase 9).
	kioskTokenPrefix = "kiosk_"
	// fetchConcurrency bounds parallel Plane calls per instance fetch. Kept low
	// so a full fetch (modules + work-items + cycles) doesn't burst past Plane's
	// rate limit; get() also retries 429s with backoff.
	fetchConcurrency = 2
)

// Server holds the overlay store and the cycle pulse. Plane clients are built
// per-request from the acting identity's authorized instances (federation).
type Server struct {
	store *store.Store
	now   func() time.Time

	// tuning is the live calibration of the buoyancy model (both grains).
	// Persisted in the store and editable by a service admin at runtime; every
	// derived read goes through Tuning(), so a change takes effect immediately.
	tuningMu sync.RWMutex
	tuning   domain.Tuning

	mu    sync.Mutex
	cache map[string]cachedActor

	// snapshot holds each instance's fully-materialized bubbles, rebuilt by the
	// background refresher. Reads serve from here and never fetch Plane on the
	// request path. sf coalesces concurrent refreshes of the same instance.
	bubblesMu    sync.Mutex
	bubblesCache map[string]cachedBubbles
	sf           singleflight.Group

	// subs are live SSE listeners (F4). broadcast() nudges each when the snapshot
	// changes so connected boards update without polling.
	subsMu sync.Mutex
	subs   map[chan struct{}]struct{}

	// membersCache caches each instance's member id→name map (see memberNames).
	membersMu    sync.Mutex
	membersCache map[string]cachedMembers

	// commentsCache holds each work item's rendered comments (WITHOUT 👀 readers,
	// which are attached fresh from SQLite per request). It shortcuts the live,
	// paged Plane fetch on repeat chat opens; sf coalesces concurrent misses.
	commentsMu    sync.Mutex
	commentsCache map[string]cachedComments

	// detailCache holds each work item's fully-rendered interior (artifacts,
	// logbook, revisions). Revisions require paging the whole project (~1.7s), so
	// without this every thread open pays that cost. Viewer-agnostic.
	detailMu    sync.Mutex
	detailCache map[string]cachedDetail

	// statesCache holds each project's workflow states by id, so a thread's real
	// Plane state can be shown without a fetch per timeline/thread open
	// (THREAD-LIFECYCLE.md). Keyed "slug:project".
	statesMu    sync.Mutex
	statesCache map[string]cachedStates

	adminToken  string          // godmode break-glass credential (from env)
	adminEmails map[string]bool // Plane emails granted service-admin
	startedAt   time.Time
}

// SetAdmin configures godmode: a break-glass token and/or a set of admin emails
// that get the ServiceAdmin flag (§ admin).
func (s *Server) SetAdmin(token string, emails []string) {
	s.adminToken = token
	s.adminEmails = make(map[string]bool, len(emails))
	for _, e := range emails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			s.adminEmails[e] = true
		}
	}
}

// godmodeActor is the identity behind the admin token: cross-org, all instances.
func (s *Server) godmodeActor() domain.Actor {
	var slugs []string
	if insts, err := s.store.ListInstances(); err == nil {
		for _, i := range insts {
			slugs = append(slugs, i.Slug)
		}
	}
	return domain.Actor{
		ID: "service-admin", Name: "service-admin", Kind: "admin",
		Admin: true, ServiceAdmin: true, Instances: slugs,
	}
}

type cachedActor struct {
	actor domain.Actor
	exp   time.Time
}

type cachedBubbles struct {
	bubbles   []domain.Bubble
	updatedAt time.Time
	partial   bool
}

type cachedMembers struct {
	names map[string]string
	exp   time.Time
}

// cachedComments holds a work item's rendered comments (readerless — 👀 receipts
// are attached per request so they stay live).
type cachedComments struct {
	comments []domain.Comment
	exp      time.Time
}

// cachedDetail holds a work item's rendered interior (viewer-agnostic).
type cachedDetail struct {
	detail domain.ThreadDetail
	exp    time.Time
}

// cachedStates holds a project's workflow states keyed by state id.
type cachedStates struct {
	byID map[string]plane.State
	exp  time.Time
}

// tuningKey is where the buoyancy calibration lives in the settings table.
const tuningKey = "tuning"

// New builds a server. With no instances configured (or none authorized for the
// caller) it degrades gracefully to an empty world. Any persisted snapshot is
// loaded so the board serves the last-known state instantly after a restart.
//
// cycle seeds the default pulse from config; a persisted admin calibration
// (God Mode) overrides it.
func New(st *store.Store, cycle time.Duration) *Server {
	tun := domain.DefaultTuning()
	if cycle > 0 {
		tun.CycleHours = cycle.Hours()
	}
	s := &Server{
		store:         st,
		tuning:        tun,
		now:           time.Now,
		cache:         map[string]cachedActor{},
		bubblesCache:  map[string]cachedBubbles{},
		subs:          map[chan struct{}]struct{}{},
		membersCache:  map[string]cachedMembers{},
		commentsCache: map[string]cachedComments{},
		detailCache:   map[string]cachedDetail{},
		statesCache:   map[string]cachedStates{},
		adminEmails:   map[string]bool{},
		startedAt:     time.Now(),
	}
	s.loadTuning()
	s.loadPersistedSnapshots()
	return s
}

// Tuning returns the live buoyancy calibration.
func (s *Server) Tuning() domain.Tuning {
	s.tuningMu.RLock()
	defer s.tuningMu.RUnlock()
	return s.tuning
}

// SetTuning validates, persists and applies a new calibration. Everything is
// derived at read time, so the board reflects it on the very next read — no
// cache flush, no recompute. Subscribers are nudged so open screens repaint.
func (s *Server) SetTuning(t domain.Tuning) (domain.Tuning, error) {
	t = t.Sanitize()
	blob, err := json.Marshal(t)
	if err != nil {
		return domain.Tuning{}, err
	}
	if err := s.store.SetSetting(tuningKey, string(blob)); err != nil {
		return domain.Tuning{}, err
	}
	s.tuningMu.Lock()
	s.tuning = t
	s.tuningMu.Unlock()
	s.broadcast()
	return t, nil
}

// loadTuning restores the persisted calibration over the config-seeded default.
// A corrupt row is logged and ignored rather than failing the boot.
func (s *Server) loadTuning() {
	raw, ok, err := s.store.GetSetting(tuningKey)
	if err != nil {
		log.Printf("tuning: load: %v", err)
		return
	}
	if !ok {
		return
	}
	// Start from the current default so a field added after the row was written
	// keeps its default instead of decoding to a zero.
	t := s.tuning
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		log.Printf("tuning: stored calibration is unreadable, using defaults: %v", err)
		return
	}
	s.tuning = t.Sanitize()
}

// subscribe registers an SSE listener; unsubscribe removes it. broadcast nudges
// every listener non-blockingly (a full buffer means an update is already
// pending, so the drop is harmless).
func (s *Server) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch
}

func (s *Server) unsubscribe(ch chan struct{}) {
	s.subsMu.Lock()
	delete(s.subs, ch)
	s.subsMu.Unlock()
}

func (s *Server) broadcast() {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// loadPersistedSnapshots warms the in-memory snapshot from sqlite at boot (F2),
// so the first reads after a restart/redeploy return the last-known board
// without waiting for the background refresher's first pass.
func (s *Server) loadPersistedSnapshots() {
	rows, err := s.store.LoadSnapshots()
	if err != nil {
		log.Printf("snapshot load: %v", err)
		return
	}
	s.bubblesMu.Lock()
	defer s.bubblesMu.Unlock()
	for _, r := range rows {
		var bs []domain.Bubble
		if err := json.Unmarshal([]byte(r.Bubbles), &bs); err != nil {
			log.Printf("snapshot load %s: %v", r.Slug, err)
			continue
		}
		t, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		s.bubblesCache[r.Slug] = cachedBubbles{bubbles: bs, updatedAt: t}
	}
	if n := len(s.bubblesCache); n > 0 {
		log.Printf("loaded %d persisted instance snapshot(s)", n)
	}
}

// persistSnapshot writes an instance's snapshot to sqlite (best-effort).
func (s *Server) persistSnapshot(slug string, bs []domain.Bubble) {
	data, err := json.Marshal(bs)
	if err != nil {
		log.Printf("snapshot marshal %s: %v", slug, err)
		return
	}
	if err := s.store.SaveSnapshot(slug, string(data), s.now().Format(time.RFC3339)); err != nil {
		log.Printf("snapshot save %s: %v", slug, err)
	}
}

// resolve turns a bearer credential into an Actor. The credential is always a
// Plane API key: a human presents their own; an agent presents the key of the
// human it impersonates. The server verifies it via /users/me and scopes it via
// members/ across every instance using its own admin keys (§9.3). Cached (authTTL).
func (s *Server) resolve(ctx context.Context, cred string) (domain.Actor, error) {
	if cred == "" {
		return domain.Actor{}, errUnauth
	}
	// Break-glass admin token → godmode (no Plane round-trip, not cached).
	if s.adminToken != "" && hmac.Equal([]byte(cred), []byte(s.adminToken)) {
		return s.godmodeActor(), nil
	}
	// Kiosk display token → a read-only actor scoped to one instance. Checked
	// before the Plane round-trip: a kiosk token is not a Plane key, so it would
	// otherwise fail /users/me. Cheap SQLite lookup, not cached.
	if strings.HasPrefix(cred, kioskTokenPrefix) {
		if k, ok, err := s.store.LookupKioskToken(cred); err != nil {
			return domain.Actor{}, fmt.Errorf("%w: kiosk lookup: %v", errUpstream, err)
		} else if ok {
			name := k.Name
			if name == "" {
				name = "kiosk"
			}
			return domain.Actor{
				ID: "kiosk:" + k.Instance, Name: name, Kind: "kiosk",
				ReadOnly: true, Instances: []string{k.Instance},
			}, nil
		}
		return domain.Actor{}, errUnauth // looks like a kiosk token but unknown/revoked
	}
	now := s.now()

	s.mu.Lock()
	if c, ok := s.cache[cred]; ok && now.Before(c.exp) {
		s.mu.Unlock()
		return c.actor, nil
	}
	s.mu.Unlock()

	actor, err := s.resolveUncached(ctx, cred)
	if err != nil {
		return domain.Actor{}, err
	}
	s.mu.Lock()
	s.cache[cred] = cachedActor{actor: actor, exp: now.Add(authTTL)}
	s.mu.Unlock()
	return actor, nil
}

func (s *Server) resolveUncached(ctx context.Context, cred string) (domain.Actor, error) {
	// Identify the user behind the Plane key via /users/me on whichever instance
	// accepts it. (An agent presents the key of the human it impersonates, so it
	// resolves to that human — indistinguishable, by design.)
	instances, err := s.store.ListInstances()
	if err != nil {
		return domain.Actor{}, err
	}
	var email, name, uid string
	var meTransient error // a non-auth failure (429/5xx/timeout) while identifying
	for _, inst := range instances {
		u, err := plane.New(inst.BaseURL, cred, inst.Workspace, "").Me(ctx)
		if err == nil && u.Email != "" {
			email, name, uid = u.Email, u.DisplayName, u.ID
			meTransient = nil
			break
		}
		if err != nil && !plane.IsAuthError(err) {
			meTransient = err // remember: could not reach Plane, not "key rejected"
		}
	}
	if email == "" {
		// A transient upstream failure must NOT look like "key rejected" — that
		// would 401 the caller (and sign a browser out). Surface it as upstream.
		if meTransient != nil {
			return domain.Actor{}, fmt.Errorf("%w: identifying via Plane: %v", errUpstream, meTransient)
		}
		return domain.Actor{}, errUnauth
	}

	// Scope + role: using the server's OWN admin key per instance, find every
	// instance whose member list contains this email.
	var scope []string
	admin := false
	var memberErr error // a failed membership fetch (transient or misconfig)
	for _, inst := range instances {
		members, err := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Members(ctx)
		if err != nil {
			memberErr = err // can't confirm membership here — remember it
			continue
		}
		for _, mm := range members {
			if strings.EqualFold(mm.Email, email) {
				scope = append(scope, inst.Slug)
				if mm.Role >= plane.RoleAdmin {
					admin = true
				}
				break
			}
		}
	}
	// An empty scope caused by a fetch error is NOT authoritative — do not return
	// (and let resolve cache) a scopeless actor for authTTL, or a single blip
	// blacks out every bubble for minutes. Fail so the caller simply retries.
	if len(scope) == 0 && memberErr != nil {
		return domain.Actor{}, fmt.Errorf("%w: scoping via Plane members: %v", errUpstream, memberErr)
	}
	if name == "" {
		name = email
	}
	return domain.Actor{
		ID: uid, Name: name, Kind: "human", Email: email, Admin: admin,
		ServiceAdmin: s.adminEmails[strings.ToLower(email)],
		Instances:    scope,
	}, nil
}

// Handler assembles the REST + MCP routes onto one mux. Every route is behind
// member authentication (§9.3): each request resolves to a Member or is refused.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/whoami", s.restAuth(s.handleWhoami))
	mux.HandleFunc("POST /api/workspaces", s.restAuth(s.handleCreateWorkspace))
	mux.HandleFunc("POST /api/bubbles", s.restAuth(s.handleCreateBubble))
	mux.HandleFunc("GET /api/bubbles", s.restAuth(s.handleBubbles))
	mux.HandleFunc("GET /api/stream", s.restAuth(s.handleStream))
	mux.HandleFunc("GET /api/bubbles/{id}/heat", s.restAuth(s.handleHeat))
	mux.HandleFunc("GET /api/bubbles/{id}/threads", s.restAuth(s.handleTimeline))
	mux.HandleFunc("POST /api/threads/birth", s.restAuth(s.handleBirth))
	mux.HandleFunc("GET /api/threads/{id}", s.restAuth(s.handleThreadDetail))
	mux.HandleFunc("GET /api/threads/{id}/comments", s.restAuth(s.handleThreadComments))
	mux.HandleFunc("POST /api/threads/{id}/comments", s.restAuth(s.handlePostComment))
	mux.HandleFunc("POST /api/threads/{id}/comments/read", s.restAuth(s.handleMarkCommentsRead))
	mux.HandleFunc("POST /api/bubbles/{id}/contract", s.restAuth(s.handleSetContract))
	mux.HandleFunc("POST /api/bubbles/{id}/close", s.restAuth(s.handleClose))
	mux.HandleFunc("POST /api/bubbles/{id}/reopen", s.restAuth(s.handleReopen))
	mux.HandleFunc("POST /api/bubbles/{id}/review", s.restAuth(s.handleReview))
	mux.HandleFunc("POST /api/bubbles/{id}/unreview", s.restAuth(s.handleUnreview))
	mux.HandleFunc("GET /api/threads", s.restAuth(s.handleThreads))
	mux.HandleFunc("GET /api/notifications", s.restAuth(s.handleNotifications))
	mux.HandleFunc("POST /api/notifications/read", s.restAuth(s.handleMarkRead))
	mux.HandleFunc("POST /api/notifications/prefs", s.restAuth(s.handlePrefs))
	mux.HandleFunc("POST /api/tick", s.restAuth(s.handleTick))

	// Service-admin (godmode) — cross-org ops, gated on ServiceAdmin (§ admin).
	mux.HandleFunc("GET /api/admin/instances", s.adminOnly(s.handleAdminInstances))
	mux.HandleFunc("GET /api/admin/bubbles", s.adminOnly(s.handleAdminBubbles))
	mux.HandleFunc("GET /api/admin/stats", s.adminOnly(s.handleAdminStats))
	mux.HandleFunc("POST /api/admin/refresh", s.adminOnly(s.handleAdminRefresh))
	mux.HandleFunc("POST /api/admin/tick", s.adminOnly(s.handleTick))
	mux.HandleFunc("GET /api/admin/members", s.adminOnly(s.handleAdminMembers))
	mux.HandleFunc("GET /api/admin/tuning", s.adminOnly(s.handleGetTuning))
	mux.HandleFunc("PUT /api/admin/tuning", s.adminOnly(s.handleSetTuning))
	mux.HandleFunc("GET /api/admin/kiosk", s.adminOnly(s.handleListKiosk))
	mux.HandleFunc("POST /api/admin/kiosk", s.adminOnly(s.handleCreateKiosk))
	mux.HandleFunc("DELETE /api/admin/kiosk/{token}", s.adminOnly(s.handleRevokeKiosk))

	// Our own MCP front door (§9.5), behind the same member credential using the
	// SDK's bearer middleware; the verified member reaches tool handlers via
	// req.Extra.TokenInfo.
	// Health check (unauthenticated) — for the services platform's health_url.
	mux.HandleFunc("GET /health", s.handleHealth)

	// Plane webhooks authenticate via HMAC signature, not a member token, so
	// they are NOT behind restAuth.
	mux.HandleFunc("POST /webhooks/plane/{slug}", s.handlePlaneWebhook)

	mcp := auth.RequireBearerToken(
		s.verifyToken,
		&auth.RequireBearerTokenOptions{AllowMissingExpiration: true},
	)(mcpapi.Handler(s))
	mux.Handle("/mcp", mcp)
	mux.Handle("/mcp/", mcp)

	// The browser UI (§ web-ui) — embedded SPA served at "/" with a fallback to
	// index.html for client-side routes. Least-specific pattern, so every /api,
	// /mcp, /webhooks and /health route above still wins.
	mux.Handle("/", spaHandler())
	return mux
}

// spaHandler serves the embedded web bundle: real files (assets, index.html) are
// served directly; any other path falls back to index.html so the SPA can route.
func spaHandler() http.HandlerFunc {
	dist := web.Dist()
	fileServer := http.FileServerFS(dist)
	return func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "" {
			clean = "index.html"
		}
		if _, err := fs.Stat(dist, clean); err != nil {
			// unknown path (or a directory) → hand the SPA its entry point
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			clean = "index.html"
		}
		// Vite fingerprints asset filenames, so they're safe to cache forever.
		// The HTML entry MUST NOT be cached, or the browser keeps loading old
		// hashed bundles after a rebuild (stale UI). embed.FS has a zero modtime,
		// so without this browsers heuristically cache index.html.
		if strings.HasPrefix(clean, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	}
}

// bearer extracts a token from the Authorization: Bearer <token> header.
func bearer(r *http.Request) string {
	if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// restAuth authenticates a REST request, attaches the Actor to the context, and
// refuses anonymous or unresolvable callers (§9.3, §9.4).
func (s *Server) restAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, err := s.resolve(r.Context(), bearer(r))
		if err != nil {
			// Transient upstream failure → 502 so the client retries; a real auth
			// failure → 401. Never 401 on a Plane blip (it would sign browsers out).
			if errors.Is(err, errUpstream) {
				writeJSON(w, http.StatusBadGateway, map[string]string{
					"error": "temporarily unable to reach Plane — retrying",
				})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized — present your Plane API key (humans) or agent token (agents) as a Bearer credential",
			})
			return
		}
		// A read-only actor (kiosk display) may only read: reject any mutation.
		if actor.ReadOnly && r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "read-only kiosk credential — this view cannot make changes",
			})
			return
		}
		log.Printf("%s → %s %s", actor.Label(), r.Method, r.URL.Path)
		ctx := domain.WithCred(domain.WithActor(r.Context(), actor), bearer(r))
		next(w, r.WithContext(ctx))
	}
}

// verifyToken is the MCP bearer verifier: it resolves the credential to an Actor
// carried into tool handlers as req.Extra.TokenInfo (§9.3).
func (s *Server) verifyToken(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
	actor, err := s.resolve(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("credential rejected: %w", auth.ErrInvalidToken)
	}
	// Kiosk display tokens are read-only and must never reach MCP tools.
	if actor.ReadOnly {
		return nil, fmt.Errorf("read-only credential cannot use MCP: %w", auth.ErrInvalidToken)
	}
	return &auth.TokenInfo{
		UserID: actor.ID,
		Extra:  map[string]any{"actor": actor, "cred": token},
	}, nil
}

// handleWhoami returns the resolved identity — useful to confirm Plane
// pass-through auth and see which instances/role Plane granted you.
func (s *Server) handleWhoami(w http.ResponseWriter, r *http.Request) {
	actor, _ := domain.ActorFrom(r.Context())
	writeJSON(w, http.StatusOK, actor)
}

// ---- Backend implementation (shared by REST + MCP) ----

// bubbleLevel maps a bubble to its UI band (§ web-ui): explicit stage/closed win,
// otherwise heat decides in-progress ↔ zzzz ↔ rip.
func bubbleLevel(b domain.Bubble, lc domain.Lifecycle, tun domain.Tuning) string {
	switch {
	case b.Closed:
		return "done"
	case b.Stage == "reviewed":
		return "reviewed"
	}
	switch lc {
	case domain.Hot, domain.Warm:
		return "in_progress"
	case domain.Cooling:
		return "zzzz"
	case domain.Dormant:
		if len(b.Evidence) == 0 || (tun.BubbleRipNeedsOwner && b.Owner == "") {
			return "rip" // never got going / abandoned
		}
		return "zzzz" // had a life, went quiet
	default:
		return "zzzz"
	}
}

// threadLevel maps a thread to the same UI bands as a bubble
// (THREAD-LIFECYCLE.md Phase A). Evidence decides the band; Plane's state group
// only short-circuits the two TERMINAL columns, which are statements of fact
// rather than status theatre. A thread being dragged into "In Progress" is
// motion, not evidence, so it does not by itself make the thread 🔥 — and a
// thread sitting in Backlog while its todos get ticked genuinely is 🔥.
func threadLevel(t domain.Thread, ev []domain.EvidenceEvent, lc domain.Lifecycle, w heat.Window, tun domain.Tuning, now time.Time) string {
	if tun.ThreadTerminalStateWins {
		switch t.StateGroup {
		case "completed":
			return "done"
		case "cancelled":
			return "rip"
		}
	}
	switch lc {
	case domain.Closed:
		return "done"
	case domain.Hot, domain.Warm:
		return "in_progress"
	case domain.Cooling:
		return "zzzz"
	case domain.Dormant:
		produced := false
		for _, e := range ev {
			if e.Progress() {
				produced = true
				break
			}
		}
		if produced && !(tun.ThreadRipNeedsOwner && t.Owner == "") {
			return "zzzz" // had a life, went quiet
		}
		// Nothing produced yet (or nobody to produce it). A thread still inside its
		// grace period is simply waiting its turn, not abandoned.
		if heat.Newborn(t, w, tun, now) {
			return "zzzz"
		}
		return "rip" // never got going / abandoned
	default:
		return "zzzz"
	}
}

// threadBuoyancy classifies every thread in a bubble against the bubble's own
// heat window, keyed by raw work-item id (THREAD-LIFECYCLE.md Phase A). This is
// the single place per-thread lifecycle is derived; the timeline, the interior
// and the bubble roll-up all read it.
func (s *Server) threadBuoyancy(b domain.Bubble) map[string]domain.Buoyancy {
	now, tun := s.now(), s.Tuning()
	win := heat.WindowFor(b, tun, now)
	byThread := heat.Attribute(b.Evidence)
	out := make(map[string]domain.Buoyancy, len(b.Threads))
	for _, t := range b.Threads {
		ev := byThread[t.ID]
		r := heat.ClassifyThread(t, ev, win, tun, now)
		out[t.ID] = domain.Buoyancy{
			Lifecycle: r.Lifecycle,
			Level:     threadLevel(t, ev, r.Lifecycle, win, tun, now),
			Score:     r.Score,
			Reason:    r.Reason,
		}
	}
	return out
}

// toViews classifies bubbles into derived views, hottest first (buoyancy).
func (s *Server) toViews(bubbles []domain.Bubble) []domain.BubbleView {
	now, tun := s.now(), s.Tuning()
	out := make([]domain.BubbleView, 0, len(bubbles))
	for _, b := range bubbles {
		r := heat.Classify(b, tun, now)
		out = append(out, domain.BubbleView{
			ID: b.ID, Name: b.Name, Instance: b.Instance,
			Project: b.Project, ProjectName: b.ProjectName,
			Lifecycle: r.Lifecycle, Level: bubbleLevel(b, r.Lifecycle, tun),
			Score: r.Score, Reason: r.Reason,
			Outcome: b.Outcome, Owner: b.Owner, Members: bubbleMembers(b),
			Threads: len(b.Threads), ThreadLevels: levelCounts(s.threadBuoyancy(b)),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// levelCounts rolls per-thread levels up into a histogram for the bubble view
// ("2 threads burning, 1 asleep"). Nil for a bubble with no threads so the JSON
// omits it entirely.
func levelCounts(bs map[string]domain.Buoyancy) map[string]int {
	if len(bs) == 0 {
		return nil
	}
	out := make(map[string]int, len(bs))
	for _, b := range bs {
		out[b.Level]++
	}
	return out
}

// bubbleMembers is the set of people with a stake in a bubble — every thread's
// assignee plus the contract owner — powering the per-assignee view scope (§9).
func bubbleMembers(b domain.Bubble) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	add(b.Owner)
	for _, t := range b.Threads {
		add(t.Owner)
	}
	sort.Strings(out)
	return out
}

// Bubbles returns the caller's bubbles as derived views.
func (s *Server) Bubbles(ctx context.Context) ([]domain.BubbleView, error) {
	bubbles, err := s.collect(ctx)
	if err != nil {
		return nil, err
	}
	return s.toViews(bubbles), nil
}

// AllBubbles returns bubbles across EVERY instance (service-admin, bypasses scope).
func (s *Server) AllBubbles(ctx context.Context) []domain.BubbleView {
	return s.toViews(s.collectAll(ctx))
}

// flushCaches clears the identity and bubble caches (service-admin refresh).
func (s *Server) flushCaches() {
	s.mu.Lock()
	s.cache = map[string]cachedActor{}
	s.mu.Unlock()
	s.bubblesMu.Lock()
	s.bubblesCache = map[string]cachedBubbles{}
	s.bubblesMu.Unlock()
}

// adminOnly wraps a handler behind auth + the ServiceAdmin flag, and audit-logs.
func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return s.restAuth(func(w http.ResponseWriter, r *http.Request) {
		actor, _ := domain.ActorFrom(r.Context())
		if !actor.ServiceAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "service admin only"})
			return
		}
		log.Printf("ADMIN %s → %s %s", actor.Label(), r.Method, r.URL.Path)
		next(w, r)
	})
}

// Heat returns a single bubble's derived view, accepting a short or full id.
func (s *Server) Heat(ctx context.Context, id string) (domain.BubbleView, error) {
	vs, err := s.Bubbles(ctx)
	if err != nil {
		return domain.BubbleView{}, err
	}
	return matchBubble(vs, id)
}

// matchBubble resolves a query against the caller's bubbles. A query containing
// ':' is treated as a full namespaced id (exact match); otherwise it matches the
// module segment (the short id) by exact value or unique prefix — git-style.
func matchBubble(vs []domain.BubbleView, q string) (domain.BubbleView, error) {
	if strings.Contains(q, ":") {
		for _, v := range vs {
			if v.ID == q {
				return v, nil
			}
		}
		return domain.BubbleView{}, errNotFound
	}
	var hits []domain.BubbleView
	for _, v := range vs {
		mod := v.ID[strings.LastIndex(v.ID, ":")+1:]
		if mod == q {
			return v, nil // exact short id wins outright
		}
		if strings.HasPrefix(mod, q) {
			hits = append(hits, v)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return domain.BubbleView{}, errNotFound
	default:
		return domain.BubbleView{}, fmt.Errorf("%w: %q matches %d bubbles — use more characters", errAmbig, q, len(hits))
	}
}

// resolveID turns a short/full query into a full namespaced bubble id, scoped to
// the caller's bubbles.
func (s *Server) resolveID(ctx context.Context, q string) (string, error) {
	vs, err := s.Bubbles(ctx)
	if err != nil {
		return "", err
	}
	v, err := matchBubble(vs, q)
	if err != nil {
		return "", err
	}
	return v.ID, nil
}

// BirthThread is the policy gate (§3): no thread is born without a Brief that
// carries a Definition of Done and a Logbook (unless it's a small thread).
func (s *Server) BirthThread(ctx context.Context, req domain.BirthRequest) (domain.BirthResult, error) {
	actor, _ := domain.ActorFrom(ctx)
	if strings.TrimSpace(req.Brief) == "" {
		return domain.BirthResult{}, fmt.Errorf("birth rejected: a BRIEF is required (§3.1)")
	}
	if !strings.Contains(strings.ToLower(req.Brief), "definition of done") {
		return domain.BirthResult{}, fmt.Errorf("birth rejected: the BRIEF must include a Definition of Done section (§3.1)")
	}
	if strings.TrimSpace(req.Logbook) == "" && !req.SmallThread {
		return domain.BirthResult{}, fmt.Errorf("birth rejected: a LOGBOOK is required unless small_thread=true (§3.2)")
	}
	if strings.TrimSpace(req.Name) == "" {
		return domain.BirthResult{}, fmt.Errorf("birth rejected: a thread name is required")
	}

	// Resolve the target bubble and split its namespaced id.
	id, err := s.resolveID(ctx, req.BubbleID)
	if err != nil {
		return domain.BirthResult{}, err
	}
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return domain.BirthResult{}, fmt.Errorf("malformed bubble id %q", id)
	}
	slug, projectID, moduleID := parts[0], parts[1], parts[2]
	inst, ok, err := s.instanceBySlug(slug)
	if err != nil {
		return domain.BirthResult{}, err
	}
	if !ok {
		return domain.BirthResult{}, errNotFound
	}

	// Write AS the acting person (impersonation): use their presented Plane key
	// so Plane attributes the work item to them. Fall back to the instance admin
	// key only if no caller credential is available.
	writeKey := inst.APIKey
	if cred, ok := domain.CredFrom(ctx); ok && cred != "" {
		writeKey = cred
	}
	cl := plane.New(inst.BaseURL, writeKey, inst.Workspace, projectID)

	state, err := cl.DefaultState(ctx)
	if err != nil {
		return domain.BirthResult{}, fmt.Errorf("resolve state: %w", err)
	}
	wid, err := cl.CreateWorkItem(ctx, req.Name, briefLogbookHTML(req.Brief, req.Logbook), state)
	if err != nil {
		return domain.BirthResult{}, fmt.Errorf("create work item: %w", err)
	}
	if err := cl.AddIssuesToModule(ctx, moduleID, []string{wid}); err != nil {
		log.Printf("birth_thread: created %s but link to module failed: %v", wid, err)
	}

	// Reflect the new thread in the cache without a refetch.
	s.patchCachedBubble(slug, id, func(b *domain.Bubble) {
		b.Threads = append(b.Threads, domain.Thread{ID: wid, Name: req.Name, Active: true})
		b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: wid, Kind: domain.EvThreadCreated, At: s.now()})
	})
	log.Printf("birth_thread by %s: bubble=%s -> work item %s", actor.Label(), id, wid)
	return domain.BirthResult{ThreadID: wid, Created: true, Message: fmt.Sprintf("created thread %q in %s", req.Name, id)}, nil
}

// writeKey returns the Plane key to write with: the caller's own (impersonation)
// if present, else the instance admin key.
func (s *Server) writeKey(ctx context.Context, inst domain.Instance) string {
	if cred, ok := domain.CredFrom(ctx); ok && cred != "" {
		return cred
	}
	return inst.APIKey
}

// deriveIdentifier makes a Plane project identifier from a name (uppercase
// alphanumerics, up to 5 chars).
func deriveIdentifier(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() >= 5 {
				break
			}
		}
	}
	if b.Len() == 0 {
		return "BW"
	}
	return b.String()
}

// CreateWorkspace creates a Plane project (our Workspace) with modules enabled
// (§3.5), attributed to the caller and scoped to an instance they can see.
func (s *Server) CreateWorkspace(ctx context.Context, req domain.CreateWorkspaceRequest) (domain.Workspace, error) {
	actor, _ := domain.ActorFrom(ctx)
	if !actor.CanSee(req.Instance) {
		return domain.Workspace{}, errForbid
	}
	inst, ok, err := s.instanceBySlug(req.Instance)
	if err != nil {
		return domain.Workspace{}, err
	}
	if !ok {
		return domain.Workspace{}, errNotFound
	}
	if strings.TrimSpace(req.Name) == "" {
		return domain.Workspace{}, fmt.Errorf("workspace name is required")
	}
	ident := strings.TrimSpace(req.Identifier)
	if ident == "" {
		ident = deriveIdentifier(req.Name)
	}
	features := map[string]bool{
		"module_view":      true,          // bubbles need modules
		"cycle_view":       !req.NoCycles, // heat cadence (§1) — on by default
		"page_view":        !req.NoPages,  // docs — on by default
		"issue_views_view": req.Views,
		"intake_view":      req.Intake,
	}
	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, "")
	p, err := cl.CreateProject(ctx, req.Name, ident, features)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create project: %w", err)
	}
	s.dropInstanceCache(req.Instance)
	log.Printf("create_workspace by %s: %s (%s) in %s", actor.Label(), p.ID, ident, req.Instance)
	return domain.Workspace{ID: p.ID, Name: p.Name, Identifier: p.Identifier, Instance: req.Instance}, nil
}

// CreateBubble creates a Plane module (a Bubble) in a project (§3.5).
func (s *Server) CreateBubble(ctx context.Context, req domain.CreateBubbleRequest) (domain.NewBubble, error) {
	actor, _ := domain.ActorFrom(ctx)
	if !actor.CanSee(req.Instance) {
		return domain.NewBubble{}, errForbid
	}
	inst, ok, err := s.instanceBySlug(req.Instance)
	if err != nil {
		return domain.NewBubble{}, err
	}
	if !ok {
		return domain.NewBubble{}, errNotFound
	}
	if strings.TrimSpace(req.Project) == "" || strings.TrimSpace(req.Name) == "" {
		return domain.NewBubble{}, fmt.Errorf("instance, project and name are required")
	}
	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, req.Project)
	m, err := cl.CreateModule(ctx, req.Name)
	if err != nil {
		return domain.NewBubble{}, fmt.Errorf("create module: %w", err)
	}
	id := req.Instance + ":" + req.Project + ":" + m.ID
	// Optional §4 contract at creation. Write straight to the store by id — the
	// new bubble isn't in the snapshot yet, so resolveID-based SetContract can't
	// find it. Best-effort: a failed contract write shouldn't undo the bubble.
	outcome, owner := strings.TrimSpace(req.Outcome), strings.TrimSpace(req.Owner)
	if outcome != "" || owner != "" {
		if err := s.store.SetContract(id, store.Contract{Outcome: outcome, Owner: owner}); err != nil {
			log.Printf("create_bubble: contract set failed for %s: %v", id, err)
		}
	}
	s.dropInstanceCache(req.Instance)
	log.Printf("create_bubble by %s: %s", actor.Label(), id)
	return domain.NewBubble{ID: id, Name: m.Name}, nil
}

// instanceBySlug looks up a registered instance by slug.
func (s *Server) instanceBySlug(slug string) (domain.Instance, bool, error) {
	all, err := s.store.ListInstances()
	if err != nil {
		return domain.Instance{}, false, err
	}
	for _, i := range all {
		if i.Slug == slug {
			return i, true, nil
		}
	}
	return domain.Instance{}, false, nil
}

// briefLogbookHTML renders the two birth artifacts into one description page (§7.1).
func briefLogbookHTML(brief, logbook string) string {
	esc := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		return strings.ReplaceAll(s, "\n", "<br/>")
	}
	var b strings.Builder
	b.WriteString("<h2>Brief</h2><p>")
	b.WriteString(esc(brief))
	b.WriteString("</p>")
	if strings.TrimSpace(logbook) != "" {
		b.WriteString("<h2>Logbook</h2><p>")
		b.WriteString(esc(logbook))
		b.WriteString("</p>")
	}
	return b.String()
}

// authorizeBubble confirms the actor may act on a namespaced bubble id and
// returns the instance slug for cache invalidation.
func (s *Server) authorizeBubble(ctx context.Context, id string) (slug string, err error) {
	actor, _ := domain.ActorFrom(ctx)
	slug, _, _ = strings.Cut(id, ":")
	if slug != "" && !actor.CanSee(slug) {
		return "", errForbid
	}
	return slug, nil
}

// dropInstanceCache evicts an instance's cached bubbles so the next read
// refetches from Plane. Used by webhooks, where we don't know exactly what
// changed, only that something did.
func (s *Server) dropInstanceCache(slug string) {
	s.bubblesMu.Lock()
	delete(s.bubblesCache, slug)
	s.bubblesMu.Unlock()
}

// handlePlaneWebhook receives a Plane webhook for one instance, verifies its
// HMAC-SHA256 signature, and drops that instance's cache so `ls` reflects the
// change near-instantly (§6). Real-time freshness; heat is still derived.
func (s *Server) handlePlaneWebhook(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	inst, ok, err := s.instanceBySlug(slug)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	if !ok || inst.WebhookSecret == "" {
		http.Error(w, "unknown webhook", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	mac := hmac.New(sha256.New, []byte(inst.WebhookSecret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	got := r.Header.Get("X-Plane-Signature")
	if !hmac.Equal([]byte(want), []byte(got)) {
		http.Error(w, "bad signature", http.StatusForbidden)
		return
	}
	// Rebuild this instance's snapshot in the background so the change shows up
	// without a blocking read (singleflight coalesces with the periodic refresh).
	go s.refreshInstance(context.Background(), inst)
	log.Printf("webhook: %s refresh triggered from Plane", slug)
	w.WriteHeader(http.StatusOK)
}

// patchCachedBubble applies an in-place update to a cached bubble so a just-
// written contract/closure reflects immediately WITHOUT a Plane refetch (those
// are server-owned overlay fields). If the instance isn't cached, the next read
// fetches fresh and merges the overlay anyway — so this is a pure optimization
// that also keeps writes from hammering Plane's rate limit.
func (s *Server) patchCachedBubble(slug, id string, apply func(*domain.Bubble)) {
	s.bubblesMu.Lock()
	defer s.bubblesMu.Unlock()
	c, ok := s.bubblesCache[slug]
	if !ok {
		return
	}
	for i := range c.bubbles {
		if c.bubbles[i].ID == id {
			apply(&c.bubbles[i])
			s.broadcast() // reflect the write to live SSE listeners immediately (F4)
			return
		}
	}
}

// SetContract applies a partial §4 contract update to a bubble (short or full id).
func (s *Server) SetContract(ctx context.Context, q string, in domain.ContractInput) (domain.Contract, error) {
	id, err := s.resolveID(ctx, q)
	if err != nil {
		return domain.Contract{}, err
	}
	slug, err := s.authorizeBubble(ctx, id)
	if err != nil {
		return domain.Contract{}, err
	}
	cur, _, err := s.store.GetContract(id)
	if err != nil {
		return domain.Contract{}, err
	}
	if in.Outcome != nil {
		cur.Outcome = *in.Outcome
	}
	if in.Owner != nil {
		cur.Owner = *in.Owner
	}
	if in.Closure != nil {
		cur.Closure = *in.Closure
	}
	if err := s.store.SetContract(id, cur); err != nil {
		return domain.Contract{}, err
	}
	s.patchCachedBubble(slug, id, func(b *domain.Bubble) {
		b.Outcome, b.Owner, b.Closure = cur.Outcome, cur.Owner, cur.Closure
	})
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("set_contract by %s: bubble=%s", actor.Label(), id)
	return domain.Contract{Outcome: cur.Outcome, Owner: cur.Owner, Closure: cur.Closure, Closed: cur.Closed}, nil
}

// setClosed opens/closes a bubble in the overlay (§5.3), attributed and scoped;
// accepts a short or full id.
func (s *Server) setClosed(ctx context.Context, q string, closed bool) error {
	id, err := s.resolveID(ctx, q)
	if err != nil {
		return err
	}
	slug, err := s.authorizeBubble(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.SetClosed(id, closed); err != nil {
		return err
	}
	s.patchCachedBubble(slug, id, func(b *domain.Bubble) { b.Closed = closed })
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("set_closed(%v) by %s: bubble=%s", closed, actor.Label(), id)
	return nil
}

// CloseBubble closes a bubble (used by the MCP close_bubble tool).
func (s *Server) CloseBubble(ctx context.Context, id string) error {
	return s.setClosed(ctx, id, true)
}

// setStage sets a bubble's explicit stage overlay (” | 'reviewed'), scoped and
// cache-patched; accepts a short or full id.
func (s *Server) setStage(ctx context.Context, q, stage string) error {
	id, err := s.resolveID(ctx, q)
	if err != nil {
		return err
	}
	slug, err := s.authorizeBubble(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.SetStage(id, stage); err != nil {
		return err
	}
	s.patchCachedBubble(slug, id, func(b *domain.Bubble) { b.Stage = stage })
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("set_stage(%q) by %s: bubble=%s", stage, actor.Label(), id)
	return nil
}

// SearchThreads returns the caller's threads (tasks) matching q, for the ⌘K
// omnibar (case-insensitive substring; empty q returns all, capped).
func (s *Server) SearchThreads(ctx context.Context, q string) ([]domain.ThreadHit, error) {
	bubbles, err := s.collect(ctx)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]domain.ThreadHit, 0, 32)
	for _, b := range bubbles {
		for _, t := range b.Threads {
			if q != "" && !strings.Contains(strings.ToLower(t.Name), q) {
				continue
			}
			out = append(out, domain.ThreadHit{
				ID: t.ID, Name: t.Name, BubbleID: b.ID, BubbleName: b.Name,
				Instance: b.Instance, Open: t.Active,
			})
			if len(out) >= 100 {
				return out, nil
			}
		}
	}
	return out, nil
}

// collect federates over the acting identity's authorized instances, reading
// each from a short-lived cache (or fetching it concurrently on a miss). One
// unreachable instance is logged and skipped — it never blanks the whole view.
func (s *Server) collect(ctx context.Context) ([]domain.Bubble, error) {
	actor, ok := domain.ActorFrom(ctx)
	if !ok {
		return nil, nil // auth middleware makes this unreachable in practice
	}
	all, err := s.store.ListInstances()
	if err != nil {
		return nil, err
	}

	var out []domain.Bubble
	for _, inst := range all {
		if !actor.CanSee(inst.Slug) || !plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Configured() {
			continue
		}
		bs, err := s.instanceBubbles(ctx, inst)
		if err != nil {
			log.Printf("instance %s: %v", inst.Slug, err)
			continue
		}
		out = append(out, bs...)
	}
	return out, nil
}

// instanceBubbles serves an instance's bubbles from the in-memory snapshot,
// never fetching Plane on the request path. On a cold snapshot (first ever read,
// before the background refresher has populated it) it fetches once so the very
// first load isn't empty; thereafter the refresher keeps it warm.
func (s *Server) instanceBubbles(ctx context.Context, inst domain.Instance) ([]domain.Bubble, error) {
	s.bubblesMu.Lock()
	c, ok := s.bubblesCache[inst.Slug]
	s.bubblesMu.Unlock()
	if ok {
		return c.bubbles, nil
	}
	return s.refreshInstance(ctx, inst)
}

// refreshInstance fetches an instance from Plane and atomically updates its
// snapshot, coalescing concurrent refreshes of the same instance (background
// loop + cold reads + webhooks). It keeps the last good snapshot on failure and
// never lets a rate-limited partial shrink a fuller one — so reads never see a
// half-populated or empty board once warm.
func (s *Server) refreshInstance(ctx context.Context, inst domain.Instance) ([]domain.Bubble, error) {
	v, err, _ := s.sf.Do(inst.Slug, func() (any, error) {
		bs, partial, err := s.fetchInstance(ctx, inst)
		if err != nil {
			// Total failure (e.g. list-projects 429'd) — keep the prior snapshot
			// rather than blanking the board.
			s.bubblesMu.Lock()
			prev, ok := s.bubblesCache[inst.Slug]
			s.bubblesMu.Unlock()
			if ok {
				return prev.bubbles, nil
			}
			return nil, err
		}
		s.bubblesMu.Lock()
		prev, had := s.bubblesCache[inst.Slug]
		// A partial must not replace a fuller snapshot — hold the better one.
		if partial && had && len(prev.bubbles) > len(bs) {
			s.bubblesMu.Unlock()
			return prev.bubbles, nil
		}
		s.bubblesCache[inst.Slug] = cachedBubbles{bubbles: bs, updatedAt: s.now(), partial: partial}
		s.bubblesMu.Unlock()
		// Persist outside the lock so restarts serve the last-known board (F2).
		s.persistSnapshot(inst.Slug, bs)
		s.broadcast() // nudge live SSE listeners (F4)
		return bs, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]domain.Bubble), nil
}

// RefreshAll rebuilds every configured instance's snapshot. Used at startup and
// on each background tick; one unreachable instance is logged and skipped.
func (s *Server) RefreshAll(ctx context.Context) {
	all, err := s.store.ListInstances()
	if err != nil {
		log.Printf("refresh: list instances: %v", err)
		return
	}
	for _, inst := range all {
		if !plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Configured() {
			continue
		}
		if _, err := s.refreshInstance(ctx, inst); err != nil {
			log.Printf("refresh: instance %s: %v", inst.Slug, err)
		}
	}
}

// RunRefresher warms the snapshot immediately, then rebuilds it every
// snapshotRefresh until ctx is done. This — not request latency — is what keeps
// the board fresh.
func (s *Server) RunRefresher(ctx context.Context) {
	s.RefreshAll(ctx)
	t := time.NewTicker(snapshotRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RefreshAll(ctx)
		}
	}
}

// fetchInstance pulls an instance's bubbles from Plane. It discovers projects
// (unless one is pinned) and fans out across modules concurrently. Heat evidence
// comes from work-item timestamps in the list response — no per-item activity
// call — so this stays within Plane's rate limits.
func (s *Server) fetchInstance(ctx context.Context, inst domain.Instance) ([]domain.Bubble, bool, error) {
	base := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "")
	projects := []string{inst.Project}
	projName := map[string]string{}
	if inst.Project == "" {
		ps, err := base.ListProjects(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("list projects: %w", err)
		}
		projects = projects[:0]
		for _, p := range ps {
			projects = append(projects, p.ID)
			projName[p.ID] = p.Name
		}
	}

	// Resolve assignee names once (cached) so the snapshot's threads carry owners
	// for the timeline — served straight from memory, no per-open Plane fetch.
	names := s.memberNames(ctx, inst)

	var (
		mu     sync.Mutex
		out    []domain.Bubble
		failed int // module/project fetches that errored (→ partial result)
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fetchConcurrency)

	now := s.now()
	for _, projID := range projects {
		projID := projID
		cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, projID)
		mods, err := cl.ListModules(gctx)
		if err != nil {
			log.Printf("instance %s project %s: list modules: %v", inst.Slug, projID, err)
			mu.Lock()
			failed++
			mu.Unlock()
			continue
		}
		// Align heat to this project's active Plane cycle (§3.6); zero → rolling.
		cycleStart, cyclePrev := s.projectCycleWindow(gctx, cl, now)
		// Resolve state uuid → group/name once per project (cached, ~never changes)
		// so each thread carries its real Plane state (THREAD-LIFECYCLE.md).
		states := s.projectStates(gctx, cl, inst.Slug, projID)
		for _, m := range mods {
			m := m
			g.Go(func() error {
				items, err := cl.ListModuleWorkItems(gctx, m.ID)
				if err != nil {
					log.Printf("instance %s module %s: list work items: %v", inst.Slug, m.ID, err)
					mu.Lock()
					failed++
					mu.Unlock()
					return nil // one bad module shouldn't fail the whole fetch
				}
				b := s.buildBubble(inst.Slug, projID, projName[projID], m, items, names, states)
				b.CycleStart, b.CyclePrevStart = cycleStart, cyclePrev
				mu.Lock()
				out = append(out, b)
				mu.Unlock()
				return nil
			})
		}
	}
	if err := g.Wait(); err != nil {
		return nil, false, err
	}
	return out, failed > 0, nil
}

// projectCycleWindow fetches a project's cycles and returns the active cycle's
// window (§3.6). Best-effort: on any error or when cycles are off/absent it
// returns zero times, so heat falls back to the rolling window.
func (s *Server) projectCycleWindow(ctx context.Context, cl *plane.Client, now time.Time) (curStart, prevStart time.Time) {
	cycles, err := cl.ListCycles(ctx)
	if err != nil || len(cycles) == 0 {
		return time.Time{}, time.Time{}
	}
	return cycleWindow(cycles, now)
}

// cycleWindow picks the cycle containing `now` and the one immediately before it.
// Zero times mean "no active cycle" → the caller falls back to the rolling
// window. Pure, so it is unit-tested directly.
func cycleWindow(cycles []plane.Cycle, now time.Time) (curStart, prevStart time.Time) {
	var active *plane.Cycle
	for i := range cycles {
		st, ok1 := cycles[i].Start()
		en, ok2 := cycles[i].End()
		if ok1 && ok2 && !now.Before(st) && !now.After(en) {
			active = &cycles[i]
			break
		}
	}
	if active == nil {
		return time.Time{}, time.Time{}
	}
	curStart, _ = active.Start()

	// Previous cycle: the one whose end is latest among those ending at/before the
	// active cycle's start.
	var prevEnd time.Time
	for i := range cycles {
		en, ok := cycles[i].End()
		if !ok || en.After(curStart) {
			continue
		}
		if st, ok2 := cycles[i].Start(); ok2 && en.After(prevEnd) {
			prevEnd, prevStart = en, st
		}
	}
	if prevStart.IsZero() {
		// No prior cycle recorded — assume one active-length cycle before.
		if ae, ok := active.End(); ok {
			prevStart = curStart.Add(-ae.Sub(curStart))
		}
	}
	return curStart, prevStart
}

// buildBubble assembles a bubble from a module + its work items, deriving heat
// evidence from each item's created (thread born) and completed (todo done)
// timestamps (§5.1), then merging the server-owned contract overlay (§4).
func (s *Server) buildBubble(slug, projID, projName string, m plane.Module, items []plane.WorkItem, names map[string]string, states map[string]plane.State) domain.Bubble {
	// Namespaced id carries the project so writes/close can route.
	id := slug + ":" + projID + ":" + m.ID
	b := domain.Bubble{ID: id, Name: m.Name, Instance: slug, Project: projID, ProjectName: projName}
	if c, ok, _ := s.store.GetContract(id); ok {
		b.Outcome, b.Owner, b.Closure, b.Closed, b.Stage = c.Outcome, c.Owner, c.Closure, c.Closed, c.Stage
	}
	for _, it := range items {
		owner := ""
		if len(it.Assignees) > 0 {
			owner = names[it.Assignees[0]]
		}
		st := states[it.StateID]
		b.Threads = append(b.Threads, domain.Thread{
			ID: it.ID, Name: it.Name, Active: it.Active,
			Seq: it.Sequence, Owner: owner, Parent: it.Parent,
			State: st.Name, StateGroup: st.Group,
			CreatedAt: it.CreatedAt, CompletedAt: it.CompletedAt,
		})
		if !it.CreatedAt.IsZero() {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: domain.EvThreadCreated, At: it.CreatedAt})
		}
		if it.CompletedAt != nil {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: domain.EvCompletedTodo, At: *it.CompletedAt})
		}
	}
	return b
}

// ---- Scheduler & notifications (§5 push) ----

// collectAll federates over EVERY configured instance (not actor-scoped), using
// the stored admin keys — for the server-side tick sweep.
func (s *Server) collectAll(ctx context.Context) []domain.Bubble {
	all, err := s.store.ListInstances()
	if err != nil {
		log.Printf("tick: list instances: %v", err)
		return nil
	}
	var out []domain.Bubble
	for _, inst := range all {
		if !plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Configured() {
			continue
		}
		bs, err := s.instanceBubbles(ctx, inst)
		if err != nil {
			log.Printf("tick: instance %s: %v", inst.Slug, err)
			continue
		}
		out = append(out, bs...)
	}
	return out
}

// coolingNotice decides whether a lifecycle transition warrants a notification.
// Only downward moves into Cooling or Dormant are surfaced — the buoyancy model
// alerts on things sinking, not on good news.
func coolingNotice(prev, cur domain.Lifecycle) (kind string, notify bool) {
	if cur == prev {
		return "", false
	}
	switch cur {
	case domain.Cooling, domain.Dormant:
		return string(cur), true
	default:
		return "", false
	}
}

func notifyMessage(b domain.Bubble, cur domain.Lifecycle) string {
	switch cur {
	case domain.Dormant:
		return fmt.Sprintf("🧊 %q (%s) went dormant — revive it with real work, or close it (§8).", b.Name, b.Instance)
	default:
		return fmt.Sprintf("💧 %q (%s) is cooling — no meaningful output recently.", b.Name, b.Instance)
	}
}

// Tick recomputes every bubble's temperature, records a notification for each
// downward transition, and returns how many were created. First sighting of a
// bubble is baselined silently (no notification flood on first run).
func (s *Server) Tick(ctx context.Context) (int, error) {
	now, tun := s.now(), s.Tuning()
	stamp := now.Format(time.RFC3339)
	n := 0
	for _, b := range s.collectAll(ctx) {
		cur := heat.Classify(b, tun, now).Lifecycle
		prev, had, err := s.store.GetLifecycle(b.ID)
		if err != nil {
			log.Printf("tick: get lifecycle %s: %v", b.ID, err)
			continue
		}
		if had && string(cur) != prev {
			if kind, ok := coolingNotice(domain.Lifecycle(prev), cur); ok {
				msg := notifyMessage(b, cur)
				if err := s.store.AddNotification(domain.Notification{
					At: stamp, Instance: b.Instance, BubbleID: b.ID, BubbleName: b.Name, Kind: kind, Message: msg,
				}); err == nil {
					log.Printf("notify: %s", msg)
					n++
				}
			}
		}
		if !had || string(cur) != prev {
			_ = s.store.SetLifecycle(b.ID, string(cur), stamp)
		}
	}
	return n, nil
}

// RunTicker runs Tick immediately and then every interval until ctx is done.
func (s *Server) RunTicker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := s.Tick(ctx); err != nil {
			log.Printf("tick: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---- HTTP handlers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleBubbles(w http.ResponseWriter, r *http.Request) {
	vs, err := s.Bubbles(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

// handleStream is a Server-Sent Events stream of the caller's bubbles (F4). It
// pushes the current board on connect and whenever the snapshot changes, so the
// UI doesn't have to poll. A heartbeat keeps intermediaries from timing out.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // stop reverse proxies buffering the stream

	sub := s.subscribe()
	defer s.unsubscribe(sub)

	send := func() bool {
		vs, err := s.Bubbles(r.Context())
		if err != nil {
			return true // transient upstream — keep the connection, retry next nudge
		}
		data, err := json.Marshal(vs)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: bubbles\ndata: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() { // initial board on connect
		return
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub:
			if !send() {
				return
			}
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleHeat(w http.ResponseWriter, r *http.Request) {
	v, err := s.Heat(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleBirth(w http.ResponseWriter, r *http.Request) {
	var req domain.BirthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	res, err := s.BirthThread(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ws, err := s.CreateWorkspace(r.Context(), req)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

func (s *Server) handleCreateBubble(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateBubbleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	b, err := s.CreateBubble(r.Context(), req)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleSetContract(w http.ResponseWriter, r *http.Request) {
	var in domain.ContractInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c, err := s.SetContract(r.Context(), r.PathValue("id"), in)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, s.setClosed(r.Context(), r.PathValue("id"), true)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "closed": true})
}

func (s *Server) handleReopen(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, s.setClosed(r.Context(), r.PathValue("id"), false)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "closed": false})
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, s.setStage(r.Context(), r.PathValue("id"), "reviewed")) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stage": "reviewed"})
}

func (s *Server) handleUnreview(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, s.setStage(r.Context(), r.PathValue("id"), "")) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stage": ""})
}

func (s *Server) handleThreads(w http.ResponseWriter, r *http.Request) {
	hits, err := s.SearchThreads(r.Context(), r.URL.Query().Get("q"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, hits)
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	actor, _ := domain.ActorFrom(r.Context())
	enabled, err := s.store.NotifyEnabled(actor.Email)
	if writeErr(w, err) {
		return
	}
	inbox := domain.Inbox{Enabled: enabled}
	if enabled {
		unreadOnly := r.URL.Query().Get("unread") == "1"
		ns, err := s.store.ListNotifications(actor.Email, actor.Instances, unreadOnly, 50)
		if writeErr(w, err) {
			return
		}
		count, err := s.store.UnreadCount(actor.Email, actor.Instances)
		if writeErr(w, err) {
			return
		}
		inbox.Notifications, inbox.UnreadCount = ns, count
	}
	writeJSON(w, http.StatusOK, inbox)
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	actor, _ := domain.ActorFrom(r.Context())
	var req struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	stamp := s.now().Format(time.RFC3339)
	var err error
	if req.All {
		err = s.store.MarkAllRead(actor.Email, actor.Instances, stamp)
	} else {
		err = s.store.MarkRead(actor.Email, req.IDs, stamp)
	}
	if writeErr(w, err) {
		return
	}
	count, _ := s.store.UnreadCount(actor.Email, actor.Instances)
	writeJSON(w, http.StatusOK, map[string]int{"unread_count": count})
}

func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	actor, _ := domain.ActorFrom(r.Context())
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if writeErr(w, s.store.SetNotifyEnabled(actor.Email, req.Enabled)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enabled})
}

// handleHealth reports liveness plus the build revision/time (from the embedded
// VCS stamp), so a stale server is easy to spot.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	info := map[string]string{"status": "ok"}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				info["revision"] = kv.Value
			case "vcs.time":
				info["built"] = kv.Value
			}
		}
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleTick(w http.ResponseWriter, r *http.Request) {
	n, err := s.Tick(r.Context())
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"new_notifications": n})
}

func (s *Server) handleAdminInstances(w http.ResponseWriter, r *http.Request) {
	insts, err := s.store.ListInstances()
	if writeErr(w, err) {
		return
	}
	out := make([]domain.AdminInstance, 0, len(insts))
	s.bubblesMu.Lock()
	for _, i := range insts {
		_, cached := s.bubblesCache[i.Slug]
		out = append(out, domain.AdminInstance{
			Slug: i.Slug, Name: i.Name, BaseURL: i.BaseURL, Workspace: i.Workspace,
			Project: i.Project, HasWebhook: i.WebhookSecret != "", Cached: cached,
		})
	}
	s.bubblesMu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminBubbles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.AllBubbles(r.Context()))
}

func (s *Server) handleAdminRefresh(w http.ResponseWriter, r *http.Request) {
	s.flushCaches()
	// Re-warm the snapshot in the background so the next read isn't a cold fetch.
	go s.RefreshAll(context.Background())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	insts, _ := s.store.ListInstances()
	s.bubblesMu.Lock()
	cachedInst := len(s.bubblesCache)
	s.bubblesMu.Unlock()
	s.mu.Lock()
	cachedIdents := len(s.cache)
	s.mu.Unlock()
	st := domain.AdminStats{
		Instances: len(insts), CachedInstances: cachedInst, CachedIdents: cachedIdents,
		StartedAt: s.startedAt.Format(time.RFC3339),
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				st.Revision = kv.Value
			case "vcs.time":
				st.Built = kv.Value
			}
		}
	}
	writeJSON(w, http.StatusOK, st)
}

// handleAdminMembers returns each instance's Plane members (read-only). Bubble
// doesn't own membership — Plane does — so this is a viewer, not a manager. An
// instance whose member fetch fails is included with an error rather than
// failing the whole call.
// handleGetTuning returns the live buoyancy calibration, alongside the defaults
// so a client can show what "stock" looks like and offer a reset.
func (s *Server) handleGetTuning(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.TuningView{
		Tuning:   s.Tuning(),
		Defaults: domain.DefaultTuning(),
		Fields:   domain.TuningFields(),
	})
}

// handleSetTuning replaces the calibration. The body is decoded ONTO the current
// values, so a partial edit only changes the fields it names.
func (s *Server) handleSetTuning(w http.ResponseWriter, r *http.Request) {
	t := s.Tuning()
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeErr(w, fmt.Errorf("%w: %v", errBadRequest, err))
		return
	}
	applied, err := s.SetTuning(t)
	if writeErr(w, err) {
		return
	}
	actor, _ := domain.ActorFrom(r.Context())
	log.Printf("tuning updated by %s: %+v", actor.Label(), applied)
	writeJSON(w, http.StatusOK, domain.TuningView{
		Tuning:   applied,
		Defaults: domain.DefaultTuning(),
		Fields:   domain.TuningFields(),
	})
}

func (s *Server) handleAdminMembers(w http.ResponseWriter, r *http.Request) {
	insts, err := s.store.ListInstances()
	if writeErr(w, err) {
		return
	}
	out := make([]domain.InstanceMembers, 0, len(insts))
	for _, inst := range insts {
		im := domain.InstanceMembers{Instance: inst.Slug, Name: inst.Name, Members: []domain.Member{}}
		ms, err := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Members(r.Context())
		if err != nil {
			im.Error = err.Error()
			out = append(out, im)
			continue
		}
		for _, m := range ms {
			name := m.DisplayName
			if name == "" {
				name = m.Email
			}
			im.Members = append(im.Members, domain.Member{
				ID: m.ID, Name: name, Email: m.Email, Role: m.Role, Admin: m.Role >= plane.RoleAdmin,
			})
		}
		out = append(out, im)
	}
	writeJSON(w, http.StatusOK, out)
}

// newKioskToken mints a random, URL-safe read-only display credential.
func newKioskToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return kioskTokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) handleListKiosk(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.store.ListKioskTokens()
	if writeErr(w, err) {
		return
	}
	if tokens == nil {
		tokens = []store.KioskToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) handleCreateKiosk(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Instance string `json:"instance"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	in.Instance = strings.TrimSpace(in.Instance)
	if in.Instance == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "instance is required"})
		return
	}
	if ok, err := s.store.InstanceExists(in.Instance); writeErr(w, err) {
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such instance: " + in.Instance})
		return
	}
	token, err := newKioskToken()
	if writeErr(w, err) {
		return
	}
	k := store.KioskToken{
		Token: token, Instance: in.Instance, Name: strings.TrimSpace(in.Name),
		CreatedAt: s.now().Format(time.RFC3339),
	}
	if writeErr(w, s.store.AddKioskToken(k)) {
		return
	}
	actor, _ := domain.ActorFrom(r.Context())
	log.Printf("kiosk token minted for %s by %s", in.Instance, actor.Label())
	writeJSON(w, http.StatusCreated, k)
}

func (s *Server) handleRevokeKiosk(w http.ResponseWriter, r *http.Request) {
	ok, err := s.store.RemoveKioskToken(r.PathValue("token"))
	if writeErr(w, err) {
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such kiosk token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// writeErr maps a backend error to an HTTP response; returns true if it wrote one.
func writeErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errForbid):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, errNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, errAmbig), errors.Is(err, errBadRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return true
}
