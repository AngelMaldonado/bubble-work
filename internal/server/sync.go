package server

import (
	"net/http"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
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
