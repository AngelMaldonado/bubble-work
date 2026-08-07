package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// Service-admin surface ("God Mode"): instances, stats, calibration, kiosk
// tokens and the cross-instance views. Split out of server.go in
// docs/PLANE-SYNC.md Phase 7 — these handlers share a gate (adminOnly) and an
// audience, and nothing else in the file depends on them.

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
			AutoState: i.AutoState,
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
	// Deliberately NOT plane.Background: an admin asked for this and is watching
	// for it, so it keeps interactive priority even though it is detached.
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
	for _, i := range insts {
		b := plane.BudgetFor(i.BaseURL, i.APIKey)
		st.RateBudgets = append(st.RateBudgets, domain.RateBudget{
			Instance: i.Slug, Known: b.Known, Remaining: b.Remaining, Limit: b.Limit,
			ResetIn: b.ResetIn, Throttled: b.Throttled, Waits: b.Waits,
			Spent: b.Spent, Floor: b.Floor,
		})
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
// handleSetAutoState opts one instance in or out of writing derived levels back
// to Plane (Phase B). This is the switch that turns Bubble from a lens into
// something that edits your tracker, so it is service-admin only and audited.
func (s *Server) handleSetAutoState(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, fmt.Errorf("%w: %v", errBadRequest, err))
		return
	}
	ok, err := s.store.SetAutoState(slug, req.Enabled)
	if writeErr(w, err) {
		return
	}
	if !ok {
		writeErr(w, errNotFound)
		return
	}
	s.dropInstanceCache(slug)
	actor, _ := domain.ActorFrom(r.Context())
	log.Printf("ADMIN %s set auto-state for %s = %v", actor.Label(), slug, req.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"instance": slug, "auto_state": req.Enabled})
}

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
		ms, err := s.mirror.Members(inst.Slug)
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
		// Map iteration is random; a viewer list that reshuffles on every refresh
		// is just noise.
		sort.Slice(im.Members, func(a, b int) bool { return im.Members[a].Name < im.Members[b].Name })
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
	case errors.Is(err, errConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, errAmbig), errors.Is(err, errBadRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return true
}
