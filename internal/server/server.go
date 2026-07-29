// Package server is the Bubble Work brain: the single interface clients use, the
// policy engine that makes the framework's rules unbypassable (§9.4), and the
// place heat is derived. It serves both a REST API (humans via CLI) and MCP
// (agents) — two front doors, one API.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/sync/errgroup"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/heat"
	"github.com/AngelMaldonado/bubble-work/internal/mcpapi"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

var (
	errNotFound = errors.New("bubble not found")
	errUnauth   = errors.New("unauthorized")
	errForbid   = errors.New("not authorized for this instance")
	errAmbig    = errors.New("ambiguous id")
)

const (
	// authTTL caches a resolved identity to avoid hitting Plane every request.
	authTTL = 5 * time.Minute
	// bubblesTTL caches an instance's fetched bubbles so repeated `ls` is instant.
	bubblesTTL = 60 * time.Second
	// fetchConcurrency bounds parallel Plane calls per instance fetch.
	fetchConcurrency = 8
)

// Server holds the overlay store and the cycle pulse. Plane clients are built
// per-request from the acting identity's authorized instances (federation).
type Server struct {
	store *store.Store
	cycle time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]cachedActor

	bubblesMu    sync.Mutex
	bubblesCache map[string]cachedBubbles
}

type cachedActor struct {
	actor domain.Actor
	exp   time.Time
}

type cachedBubbles struct {
	bubbles []domain.Bubble
	exp     time.Time
}

// New builds a server. With no instances configured (or none authorized for the
// caller) it degrades gracefully to an empty world.
func New(st *store.Store, cycle time.Duration) *Server {
	return &Server{
		store:        st,
		cycle:        cycle,
		now:          time.Now,
		cache:        map[string]cachedActor{},
		bubblesCache: map[string]cachedBubbles{},
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
	for _, inst := range instances {
		u, err := plane.New(inst.BaseURL, cred, inst.Workspace, "").Me(ctx)
		if err == nil && u.Email != "" {
			email, name, uid = u.Email, u.DisplayName, u.ID
			break
		}
	}
	if email == "" {
		return domain.Actor{}, errUnauth
	}

	// Scope + role: using the server's OWN admin key per instance, find every
	// instance whose member list contains this email.
	var scope []string
	admin := false
	for _, inst := range instances {
		members, err := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Members(ctx)
		if err != nil {
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
	if name == "" {
		name = email
	}
	return domain.Actor{ID: uid, Name: name, Kind: "human", Email: email, Admin: admin, Instances: scope}, nil
}

// Handler assembles the REST + MCP routes onto one mux. Every route is behind
// member authentication (§9.3): each request resolves to a Member or is refused.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/whoami", s.restAuth(s.handleWhoami))
	mux.HandleFunc("GET /api/bubbles", s.restAuth(s.handleBubbles))
	mux.HandleFunc("GET /api/bubbles/{id}/heat", s.restAuth(s.handleHeat))
	mux.HandleFunc("POST /api/threads/birth", s.restAuth(s.handleBirth))
	mux.HandleFunc("POST /api/bubbles/{id}/contract", s.restAuth(s.handleSetContract))
	mux.HandleFunc("POST /api/bubbles/{id}/close", s.restAuth(s.handleClose))
	mux.HandleFunc("POST /api/bubbles/{id}/reopen", s.restAuth(s.handleReopen))

	// Our own MCP front door (§9.5), behind the same member credential using the
	// SDK's bearer middleware; the verified member reaches tool handlers via
	// req.Extra.TokenInfo.
	mcp := auth.RequireBearerToken(
		s.verifyToken,
		&auth.RequireBearerTokenOptions{AllowMissingExpiration: true},
	)(mcpapi.Handler(s))
	mux.Handle("/mcp", mcp)
	mux.Handle("/mcp/", mcp)
	return mux
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
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized — present your Plane API key (humans) or agent token (agents) as a Bearer credential",
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

// Bubbles returns every bubble as a derived view, hottest first (buoyancy).
func (s *Server) Bubbles(ctx context.Context) ([]domain.BubbleView, error) {
	bubbles, err := s.collect(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]domain.BubbleView, 0, len(bubbles))
	for _, b := range bubbles {
		r := heat.Classify(b, s.cycle, now)
		out = append(out, domain.BubbleView{
			ID: b.ID, Name: b.Name, Instance: b.Instance,
			Lifecycle: r.Lifecycle, Score: r.Score, Reason: r.Reason,
			Outcome: b.Outcome, Owner: b.Owner, Threads: len(b.Threads),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
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
		b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: wid, Kind: "thread-created", At: s.now()})
	})
	log.Printf("birth_thread by %s: bubble=%s -> work item %s", actor.Label(), id, wid)
	return domain.BirthResult{ThreadID: wid, Created: true, Message: fmt.Sprintf("created thread %q in %s", req.Name, id)}, nil
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

// instanceBubbles returns an instance's bubbles from cache, or fetches them.
func (s *Server) instanceBubbles(ctx context.Context, inst domain.Instance) ([]domain.Bubble, error) {
	now := s.now()
	s.bubblesMu.Lock()
	if c, ok := s.bubblesCache[inst.Slug]; ok && now.Before(c.exp) {
		s.bubblesMu.Unlock()
		return c.bubbles, nil
	}
	s.bubblesMu.Unlock()

	bs, err := s.fetchInstance(ctx, inst)
	if err != nil {
		return nil, err
	}
	s.bubblesMu.Lock()
	s.bubblesCache[inst.Slug] = cachedBubbles{bubbles: bs, exp: now.Add(bubblesTTL)}
	s.bubblesMu.Unlock()
	return bs, nil
}

// fetchInstance pulls an instance's bubbles from Plane. It discovers projects
// (unless one is pinned) and fans out across modules concurrently. Heat evidence
// comes from work-item timestamps in the list response — no per-item activity
// call — so this stays within Plane's rate limits.
func (s *Server) fetchInstance(ctx context.Context, inst domain.Instance) ([]domain.Bubble, error) {
	base := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "")
	projects := []string{inst.Project}
	if inst.Project == "" {
		ps, err := base.ListProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		projects = projects[:0]
		for _, p := range ps {
			projects = append(projects, p.ID)
		}
	}

	var (
		mu  sync.Mutex
		out []domain.Bubble
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fetchConcurrency)

	for _, projID := range projects {
		projID := projID
		cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, projID)
		mods, err := cl.ListModules(gctx)
		if err != nil {
			log.Printf("instance %s project %s: list modules: %v", inst.Slug, projID, err)
			continue
		}
		for _, m := range mods {
			m := m
			g.Go(func() error {
				items, err := cl.ListModuleWorkItems(gctx, m.ID)
				if err != nil {
					log.Printf("instance %s module %s: list work items: %v", inst.Slug, m.ID, err)
					return nil // one bad module shouldn't fail the whole fetch
				}
				b := s.buildBubble(inst.Slug, projID, m, items)
				mu.Lock()
				out = append(out, b)
				mu.Unlock()
				return nil
			})
		}
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// buildBubble assembles a bubble from a module + its work items, deriving heat
// evidence from each item's created (thread born) and completed (todo done)
// timestamps (§5.1), then merging the server-owned contract overlay (§4).
func (s *Server) buildBubble(slug, projID string, m plane.Module, items []plane.WorkItem) domain.Bubble {
	// Namespaced id carries the project so writes/close can route.
	id := slug + ":" + projID + ":" + m.ID
	b := domain.Bubble{ID: id, Name: m.Name, Instance: slug}
	if c, ok, _ := s.store.GetContract(id); ok {
		b.Outcome, b.Owner, b.Closure, b.Closed = c.Outcome, c.Owner, c.Closure, c.Closed
	}
	for _, it := range items {
		b.Threads = append(b.Threads, domain.Thread{ID: it.ID, Name: it.Name, Active: it.Active})
		if !it.CreatedAt.IsZero() {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: "thread-created", At: it.CreatedAt})
		}
		if it.CompletedAt != nil {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: "completed-todo", At: *it.CompletedAt})
		}
	}
	return b
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

// writeErr maps a backend error to an HTTP response; returns true if it wrote one.
func writeErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errForbid):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, errNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, errAmbig):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return true
}
