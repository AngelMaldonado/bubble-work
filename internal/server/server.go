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

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/heat"
	"github.com/AngelMaldonado/bubble-work/internal/mcpapi"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

var (
	errNotFound = errors.New("bubble not found")
	errUnauth   = errors.New("unauthorized")
)

// authTTL is how long a resolved identity is cached, to avoid hitting Plane on
// every request.
const authTTL = 5 * time.Minute

// Server holds the overlay store and the cycle pulse. Plane clients are built
// per-request from the acting identity's authorized instances (federation).
type Server struct {
	store *store.Store
	cycle time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]cachedActor
}

type cachedActor struct {
	actor domain.Actor
	exp   time.Time
}

// New builds a server. With no instances configured (or none authorized for the
// caller) it degrades gracefully to an empty world.
func New(st *store.Store, cycle time.Duration) *Server {
	return &Server{store: st, cycle: cycle, now: time.Now, cache: map[string]cachedActor{}}
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
	mux.HandleFunc("POST /api/bubbles/{id}/close", s.restAuth(s.handleClose))

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
		next(w, r.WithContext(domain.WithActor(r.Context(), actor)))
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
		Extra:  map[string]any{"actor": actor},
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

// Heat returns a single bubble's derived view.
func (s *Server) Heat(ctx context.Context, id string) (domain.BubbleView, error) {
	vs, err := s.Bubbles(ctx)
	if err != nil {
		return domain.BubbleView{}, err
	}
	for _, v := range vs {
		if v.ID == id {
			return v, nil
		}
	}
	return domain.BubbleView{}, errNotFound
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
	// TODO: POST the work item to Plane (Brief+Logbook → work-item description
	// page, §7.1) once the write mapping is wired. For now we enforce the gate.
	log.Printf("birth_thread by %s: bubble=%s name=%q", actor.Label(), req.BubbleID, req.Name)
	return domain.BirthResult{
		Created: false,
		Message: fmt.Sprintf("birth artifacts valid — ready to create the work item in Plane (actor: %s)", actor.Label()),
	}, nil
}

// CloseBubble records a bubble's closure in the overlay (§5.3), attributed to
// the acting identity and refused if they aren't scoped to the bubble's instance.
func (s *Server) CloseBubble(ctx context.Context, id string) error {
	actor, _ := domain.ActorFrom(ctx)
	if slug, _, ok := strings.Cut(id, ":"); ok && !actor.CanSee(slug) {
		return fmt.Errorf("not authorized for instance %q", slug)
	}
	log.Printf("close_bubble by %s: bubble=%s", actor.Label(), id)
	return s.store.SetClosed(id, true)
}

// collect federates over the acting member's authorized instances, pulling
// bubbles from each Plane and merging the server-owned contract overlay. One
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
		if !actor.CanSee(inst.Slug) {
			continue
		}
		if !plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Configured() {
			continue
		}
		// A pinned project scopes to one; an empty project means the whole
		// workspace, so discover every project in it.
		projects := []string{inst.Project}
		if inst.Project == "" {
			disc := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "")
			ps, err := disc.ListProjects(ctx)
			if err != nil {
				log.Printf("instance %s: list projects: %v", inst.Slug, err)
				continue
			}
			projects = projects[:0]
			for _, p := range ps {
				projects = append(projects, p.ID)
			}
		}

		for _, projID := range projects {
			cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, projID)
			mods, err := cl.ListModules(ctx)
			if err != nil {
				log.Printf("instance %s project %s: list modules: %v", inst.Slug, projID, err)
				continue
			}
			for _, m := range mods {
				// Namespaced id carries the project so writes/close can route.
				id := inst.Slug + ":" + projID + ":" + m.ID
				b := domain.Bubble{ID: id, Name: m.Name, Instance: inst.Slug}
				if c, ok, _ := s.store.GetContract(id); ok {
					b.Outcome, b.Owner, b.Closure, b.Closed = c.Outcome, c.Owner, c.Closure, c.Closed
				}
				items, _ := cl.ListModuleWorkItems(ctx, m.ID)
				for _, it := range items {
					b.Threads = append(b.Threads, domain.Thread{ID: it.ID, Name: it.Name, Active: !it.Completed})
					acts, _ := cl.ListActivities(ctx, it.ID)
					for _, a := range acts {
						if kind, ok := plane.Meaningful(a); ok {
							b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: kind, At: a.At})
						}
					}
				}
				out = append(out, b)
			}
		}
	}
	return out, nil
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
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
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

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	if err := s.CloseBubble(r.Context(), r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
