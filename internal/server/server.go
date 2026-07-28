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
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/heat"
	"github.com/AngelMaldonado/bubble-work/internal/mcpapi"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

var errNotFound = errors.New("bubble not found")

// Server holds the overlay store, the Plane client, and the cycle pulse.
type Server struct {
	store *store.Store
	plane *plane.Client
	cycle time.Duration
	now   func() time.Time
}

// New builds a server. A nil/unconfigured plane client degrades gracefully to
// an empty world so the server still boots for local development.
func New(st *store.Store, pl *plane.Client, cycle time.Duration) *Server {
	return &Server{store: st, plane: pl, cycle: cycle, now: time.Now}
}

// Handler assembles the REST + MCP routes onto one mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/bubbles", s.handleBubbles)
	mux.HandleFunc("GET /api/bubbles/{id}/heat", s.handleHeat)
	mux.HandleFunc("POST /api/threads/birth", s.handleBirth)
	mux.HandleFunc("POST /api/bubbles/{id}/close", s.handleClose)

	mcp := mcpapi.Handler(s) // our own MCP front door (§9.5), not Plane's
	mux.Handle("/mcp", mcp)
	mux.Handle("/mcp/", mcp)
	return mux
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
			ID: b.ID, Name: b.Name,
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
	return domain.BirthResult{
		Created: false,
		Message: "birth artifacts valid — ready to create the work item in Plane",
	}, nil
}

// CloseBubble records a bubble's closure in the overlay (§5.3).
func (s *Server) CloseBubble(ctx context.Context, id string) error {
	return s.store.SetClosed(id, true)
}

// collect pulls bubbles from Plane and merges the server-owned contract overlay.
func (s *Server) collect(ctx context.Context) ([]domain.Bubble, error) {
	if !s.plane.Configured() {
		return nil, nil
	}
	mods, err := s.plane.ListModules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Bubble, 0, len(mods))
	for _, m := range mods {
		b := domain.Bubble{ID: m.ID, Name: m.Name}
		if c, ok, _ := s.store.GetContract(m.ID); ok {
			b.Outcome, b.Owner, b.Closure, b.Closed = c.Outcome, c.Owner, c.Closure, c.Closed
		}
		items, _ := s.plane.ListModuleWorkItems(ctx, m.ID)
		for _, it := range items {
			b.Threads = append(b.Threads, domain.Thread{ID: it.ID, Name: it.Name, Active: !it.Completed})
			acts, _ := s.plane.ListActivities(ctx, it.ID)
			for _, a := range acts {
				if kind, ok := plane.Meaningful(a); ok {
					b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: kind, At: a.At})
				}
			}
		}
		out = append(out, b)
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
