package server

import (
	"fmt"
	"log"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// The board, built from the local mirror instead of from Plane
// (docs/PLANE-SYNC.md Phase 2). This replaces fetchInstance, which made one
// ListModules call per project plus one ListModuleWorkItems per module on every
// refresh — roughly 32 Plane calls per tick, competing with human page loads for
// the same 60/min.
//
// Nothing about heat, buoyancy or the contract changes. They stop being computed
// *during a fetch* and start being computed *over the mirror*; the inputs are
// identical, which is precisely what `bubble admin sync-diff` verifies.

// buildInstance assembles an instance's bubbles from the mirror. No Plane calls,
// no network, no error path that depends on an upstream being reachable — the
// only failures possible here are local SQLite ones.
func (s *Server) buildInstance(inst domain.Instance) ([]domain.Bubble, error) {
	if s.mirror == nil {
		return nil, fmt.Errorf("mirror unavailable")
	}
	projects, err := s.mirror.Projects(inst.Slug)
	if err != nil {
		return nil, fmt.Errorf("mirror projects: %w", err)
	}
	// A pinned instance shows only its project; the board must not widen just
	// because the mirror happens to hold more.
	if inst.Project != "" {
		kept := projects[:0]
		for _, p := range projects {
			if p.ID == inst.Project {
				kept = append(kept, p)
			}
		}
		projects = kept
	}

	names, err := s.mirrorNames(inst.Slug)
	if err != nil {
		return nil, err
	}

	now := s.now()
	var out []domain.Bubble
	for _, p := range projects {
		mods, err := s.mirror.Modules(inst.Slug, p.ID)
		if err != nil {
			return nil, fmt.Errorf("mirror modules: %w", err)
		}
		if len(mods) == 0 {
			continue
		}

		pc := projectCtx{
			slug: inst.Slug, projID: p.ID, projName: p.Name, names: names,
		}
		pc.cycleStart, pc.cyclePrevStart = s.mirrorCycleWindow(inst.Slug, p.ID, now)

		// The progress diff PERSISTS (it records when each Logbook last changed),
		// so it runs once per project here rather than per module — one writer,
		// never inside a fan-out. It reads the mirror now instead of spending a
		// ListProjectItems call per project per tick.
		items, err := s.mirror.Items(inst.Slug, p.ID)
		if err != nil {
			return nil, fmt.Errorf("mirror items: %w", err)
		}
		pc.progress = s.progressEvidenceFromMirror(items)

		for _, m := range mods {
			mi, err := s.mirror.ModuleItems(inst.Slug, m.ID)
			if err != nil {
				return nil, fmt.Errorf("mirror module items: %w", err)
			}
			out = append(out, s.buildBubbleFromMirror(pc, m, mi))
		}
	}
	return out, nil
}

// buildBubbleFromMirror is buildBubble over mirror rows. The evidence rules are
// unchanged: birth and completion come from the item's own timestamps, and the
// produced-since evidence comes from the persisted progress diff (§5.1).
func (s *Server) buildBubbleFromMirror(pc projectCtx, m mirror.Module, items []mirror.Item) domain.Bubble {
	id := pc.slug + ":" + pc.projID + ":" + m.ID
	b := domain.Bubble{
		ID: id, Name: m.Name, Instance: pc.slug, Project: pc.projID, ProjectName: pc.projName,
		CycleStart: pc.cycleStart, CyclePrevStart: pc.cyclePrevStart,
	}
	if c, ok, _ := s.store.GetContract(id); ok {
		b.Outcome, b.Owner, b.Closure, b.Closed, b.Stage = c.Outcome, c.Owner, c.Closure, c.Closed, c.Stage
	}
	for _, it := range items {
		owner := ""
		if len(it.Assignees) > 0 {
			owner = pc.names[it.Assignees[0]]
		}
		b.Threads = append(b.Threads, domain.Thread{
			ID: it.ID, Name: it.Name, Active: it.CompletedAt == nil,
			Seq: it.Seq, Owner: owner, Parent: it.ParentID,
			// The state group rides along on the item because the sync asks for
			// expand=state, so there is no id→state join to get stale.
			State: it.StateName, StateGroup: it.StateGroup,
			CreatedAt: it.CreatedAt, CompletedAt: it.CompletedAt,
		})
		if !it.CreatedAt.IsZero() {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: domain.EvThreadCreated, At: it.CreatedAt})
		}
		if it.CompletedAt != nil {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{ThreadID: it.ID, Kind: domain.EvThreadCompleted, At: *it.CompletedAt})
		}
		b.Evidence = append(b.Evidence, pc.progress[it.ID]...)
	}
	return b
}

// mirrorNames is memberNames served from the mirror: assignee id → display name.
func (s *Server) mirrorNames(slug string) (map[string]string, error) {
	ms, err := s.mirror.Members(slug)
	if err != nil {
		return nil, fmt.Errorf("mirror members: %w", err)
	}
	names := make(map[string]string, len(ms))
	for id, m := range ms {
		if m.DisplayName != "" {
			names[id] = m.DisplayName
		} else {
			names[id] = m.Email
		}
	}
	return names, nil
}

// mirrorCycleWindow is projectCycleWindow over mirrored cycles. Zero times mean
// "no active cycle", and heat falls back to the rolling window (§3.6) — the same
// contract as before, including for projects with cycles switched off.
func (s *Server) mirrorCycleWindow(slug, projID string, now time.Time) (curStart, prevStart time.Time) {
	cs, err := s.mirror.Cycles(slug, projID)
	if err != nil {
		log.Printf("mirror cycles %s/%s: %v", slug, projID, err)
		return time.Time{}, time.Time{}
	}
	if len(cs) == 0 {
		return time.Time{}, time.Time{}
	}
	// Reuse plane.Cycle's date parsing rather than reimplementing it: Plane emits
	// RFC3339 or bare YYYY-MM-DD, and end dates are inclusive-end-of-day. Two
	// parsers for one format is how the board and Plane start disagreeing.
	pcs := make([]plane.Cycle, 0, len(cs))
	for _, c := range cs {
		pcs = append(pcs, plane.Cycle{ID: c.ID, Name: c.Name, StartDate: c.StartDate, EndDate: c.EndDate})
	}
	return cycleWindow(pcs, now)
}
