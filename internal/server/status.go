package server

import (
	"net/http"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	planesync "github.com/AngelMaldonado/bubble-work/internal/sync"
)

// Degraded mode (docs/journal/PLANE-SYNC.md Phase 7).
//
// Every read now comes from a local copy of Plane, which is the whole point —
// but it introduces a failure this codebase did not previously have: if the sync
// stops, the board keeps rendering, confidently, from data that is quietly
// getting older. A board that is behind is fine. A board that is behind and says
// nothing is not.
//
// This is the counterweight. It is deliberately cheap (two indexed queries and
// no Plane traffic) so a client can ask often.

// staleAfter is how far behind the mirror may fall before the board says so.
// Three delta intervals: one missed pass is ordinary — a rate-limit yield will
// do it — but three in a row means something is actually wrong.
var staleAfter = 3 * planesync.DeltaInterval

// Status reports whether what the caller is looking at can be trusted to be
// current, and whether any of their own writes are still unsent.
func (s *Server) Status(actor domain.Actor) domain.ServiceStatus {
	out := domain.ServiceStatus{}
	if s.mirror == nil {
		out.Stale = true
		out.Reason = "the local mirror is unavailable; the board may be out of date"
		return out
	}
	now := s.now()
	insts, err := s.store.ListInstances()
	if err != nil {
		out.Stale = true
		out.Reason = err.Error()
		return out
	}
	for _, inst := range insts {
		// Only speak about instances this person can actually see; another
		// workspace's sync problem is not their business.
		if !actor.ServiceAdmin && !actor.CanSee(inst.Slug) {
			continue
		}
		cur, err := s.mirror.Cursor(inst.Slug, "items")
		if err != nil {
			continue
		}
		is := domain.InstanceStatus{Instance: inst.Slug, LastError: cur.LastError}
		if !cur.LastOK.IsZero() {
			is.LastOK = cur.LastOK.UTC().Format(time.RFC3339)
			is.BehindSeconds = int(now.Sub(cur.LastOK).Seconds())
		}
		// Never synced at all is its own kind of stale, and a worse one.
		is.Stale = cur.LastOK.IsZero() || now.Sub(cur.LastOK) > staleAfter
		if is.Stale {
			out.Stale = true
		}
		out.Instances = append(out.Instances, is)
	}
	if out.Stale {
		out.Reason = "Plane sync is behind — the board may be showing older data"
	}

	// Their own unsent words, so the count is a thing they can act on rather
	// than a global number they cannot.
	if actor.Email != "" {
		if n, err := s.store.CountDraftsBy(actor.Email); err == nil {
			out.UnsentDrafts = n
		}
	}
	return out
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	actor, _ := domain.ActorFrom(r.Context())
	writeJSON(w, http.StatusOK, s.Status(actor))
}
