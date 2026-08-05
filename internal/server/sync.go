package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// Admin surface for the Plane mirror (docs/PLANE-SYNC.md Phase 1). Admin
// capability, so REST + CLI + web only — no MCP, matching autostate and tuning.

// syncInstance resolves the {slug} path value to a configured instance, writing
// the error response itself when it cannot.
func (s *Server) syncInstance(w http.ResponseWriter, r *http.Request) (domain.Instance, bool) {
	if s.syncer == nil {
		http.Error(w, "mirror is not available on this server", http.StatusServiceUnavailable)
		return domain.Instance{}, false
	}
	slug := r.PathValue("slug")
	all, err := s.store.ListInstances()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return domain.Instance{}, false
	}
	for _, i := range all {
		if i.Slug == slug {
			return i, true
		}
	}
	http.Error(w, "unknown instance: "+slug, http.StatusNotFound)
	return domain.Instance{}, false
}

// handleSyncStatus reports the mirror's census and cursor for one instance —
// cheap, no Plane calls.
func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	inst, ok := s.syncInstance(w, r)
	if !ok {
		return
	}
	counts, err := s.mirror.Counts(inst.Slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cur, err := s.mirror.Cursor(inst.Slug, "items")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, domain.SyncStatus{
		Instance:  inst.Slug,
		Projects:  counts.Projects,
		Modules:   counts.Modules,
		Items:     counts.Items,
		States:    counts.States,
		Members:   counts.Members,
		Comments:  counts.Comments,
		Watermark: rfc3339(cur.Watermark),
		LastFull:  rfc3339(cur.LastFull),
		LastOK:    rfc3339(cur.LastOK),
		LastError: cur.LastError,
	})
}

// handleSyncDiff compares the mirror against a live fetch. POST rather than GET
// because it is expensive and has a real cost in rate budget — it should not be
// something a browser can trigger by prefetching a link.
func (s *Server) handleSyncDiff(w http.ResponseWriter, r *http.Request) {
	inst, ok := s.syncInstance(w, r)
	if !ok {
		return
	}
	rep, err := s.syncer.Diff(r.Context(), inst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	out := domain.SyncDiff{
		Instance: rep.Instance, Projects: rep.Projects, Modules: rep.Modules,
		Items: rep.Items, Clean: rep.Clean(), TookMS: rep.Took.Milliseconds(),
		Watermark: rfc3339(rep.Watermark), LastFull: rfc3339(rep.LastFull),
		LastOK: rfc3339(rep.LastOK), LastError: rep.LastError,
	}
	for _, f := range rep.Findings {
		out.Findings = append(out.Findings, domain.SyncFinding{
			Kind: f.Kind, Scope: f.Scope, ID: f.ID, Label: f.Label,
			Field: f.Field, Plane: f.Plane, Local: f.Local, Text: f.String(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSyncRebuild drops an instance's mirror and rebuilds it from Plane.
//
// This exists to keep the central claim honest: the mirror is a PROJECTION, and
// deleting it must cost a backfill and nothing else. If this ever loses
// something, that something was in the wrong table — which is exactly why the
// outbox lives with the overlay rather than here (Phase 5).
func (s *Server) handleSyncRebuild(w http.ResponseWriter, r *http.Request) {
	inst, ok := s.syncInstance(w, r)
	if !ok {
		return
	}
	if err := s.mirror.Reset(inst.Slug); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res, err := s.syncer.Backfill(r.Context(), inst)
	if err != nil {
		// The mirror is now empty and the rebuild failed. Say so plainly: the
		// board will be thin until a later pass succeeds.
		http.Error(w, "mirror cleared but the rebuild failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, domain.SyncResult{
		Instance: res.Instance, Full: res.Full, Projects: res.Projects,
		Modules: res.Modules, Items: res.Items, Comments: res.Comments,
		Pruned: res.Pruned, TookMS: res.Took.Milliseconds(),
		Watermark: rfc3339(res.Watermark),
		Partial:   res.Partial, Errors: res.Errors,
	})
}

// handleSyncBackfill forces a complete walk of one instance.
func (s *Server) handleSyncBackfill(w http.ResponseWriter, r *http.Request) {
	inst, ok := s.syncInstance(w, r)
	if !ok {
		return
	}
	res, err := s.syncer.Backfill(r.Context(), inst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, domain.SyncResult{
		Instance: res.Instance, Full: res.Full, Projects: res.Projects,
		Modules: res.Modules, Items: res.Items, Comments: res.Comments,
		Pruned: res.Pruned, TookMS: res.Took.Milliseconds(),
		Watermark: rfc3339(res.Watermark),
		Partial:   res.Partial, Errors: res.Errors,
	})
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ---- outbox (docs/PLANE-SYNC.md Phase 5) ----

// handleAdminOutbox lists queued and abandoned writes. An abandoned entry is the
// point of this surface: it is a write that never reached Plane, and it must be
// visible rather than quietly gone.
func (s *Server) handleAdminOutbox(w http.ResponseWriter, r *http.Request) {
	es, err := s.store.ListOutbox(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pending, abandoned, err := s.store.CountOutbox()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := domain.OutboxView{Pending: pending, Abandoned: abandoned}
	for _, e := range es {
		out.Entries = append(out.Entries, domain.OutboxItem{
			ID: e.ID, Instance: e.Instance, Kind: e.Kind, TargetID: e.TargetID,
			Author: e.AuthorEmail, Status: e.Status, Attempts: e.Attempts,
			FieldLock: e.FieldLock, LastError: e.LastError,
			CreatedAt: rfc3339(e.CreatedAt), NextAt: rfc3339(e.NextAt),
			Summary: outboxSummary(e),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDropOutbox clears one entry (an admin unsticking a queue).
func (s *Server) handleDropOutbox(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	ok, err := s.store.DiscardOutbox(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "no such entry", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// outboxSummary renders an entry as one human-readable line, so every surface
// describes a queued write the same way.
func outboxSummary(e store.OutboxEntry) string {
	switch e.Kind {
	case store.OutComment:
		body := strings.Join(strings.Fields(e.Body()), " ")
		if len(body) > 60 {
			body = body[:57] + "…"
		}
		return fmt.Sprintf("unsent comment by %s: %q", e.AuthorEmail, body)
	case store.OutState:
		return fmt.Sprintf("move %s to state %s", short(e.TargetID), short(e.StateID()))
	}
	return e.Kind
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
