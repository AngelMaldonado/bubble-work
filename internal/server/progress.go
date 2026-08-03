package server

import (
	"context"
	"log"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// progressEvidence turns a project's work items into per-thread progress
// evidence (THREAD-LIFECYCLE.md Phase C).
//
// Plane exposes no "a todo got ticked" event, so we detect production by
// DIFFING: each sweep counts what a thread has produced — ticked logbook/DoD
// items and revision sub-items — against what it had last time, and the moment a
// counter goes up is recorded. The stored timestamp is the durable part; heat is
// derived from it on every read, so one tick keeps a thread warm for a whole
// cycle rather than for a single refresh.
//
// Two rules matter:
//
//   - A thread we are seeing for the FIRST time is baselined silently. We have no
//     idea when those todos were ticked, and stamping them "now" would fabricate
//     heat for work that may be a year old (§5.1: evidence, never motion).
//   - Counters going DOWN (a todo unticked, a revision deleted) is not evidence
//     of anything. The counters follow, the timestamps do not move.
//
// A store failure is logged and yields no evidence — threads simply don't warm,
// which is the safe direction.
func (s *Server) progressEvidence(items []plane.WorkItemDetail) map[string][]domain.EvidenceEvent {
	if len(items) == 0 {
		return nil
	}

	// A revision artifact is a sub-work-item, so a thread's revision count is how
	// many items name it as parent.
	revisions := make(map[string]int, len(items))
	for _, it := range items {
		if it.Parent != "" {
			revisions[it.Parent]++
		}
	}

	ids := make([]string, 0, len(items))
	observed := make(map[string]store.ThreadProgress, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
		observed[it.ID] = store.ThreadProgress{
			ThreadID:  it.ID,
			DoneTodos: md.CountDone(md.FromHTML(it.DescriptionHTML)),
			Revisions: revisions[it.ID],
		}
	}

	prev, err := s.store.ThreadProgressFor(ids)
	if err != nil {
		log.Printf("progress: load: %v", err)
		return nil
	}

	now := s.now()
	out := make(map[string][]domain.EvidenceEvent, len(items))
	var dirty []store.ThreadProgress

	for id, cur := range observed {
		was, seen := prev[id]
		if !seen {
			dirty = append(dirty, cur) // baseline, no evidence
			continue
		}
		cur.TodosAt, cur.RevisionsAt = was.TodosAt, was.RevisionsAt
		if cur.DoneTodos > was.DoneTodos {
			cur.TodosAt = now
		}
		if cur.Revisions > was.Revisions {
			cur.RevisionsAt = now
		}
		if cur.DoneTodos != was.DoneTodos || cur.Revisions != was.Revisions ||
			!cur.TodosAt.Equal(was.TodosAt) || !cur.RevisionsAt.Equal(was.RevisionsAt) {
			dirty = append(dirty, cur)
		}
		if !cur.TodosAt.IsZero() {
			out[id] = append(out[id], domain.EvidenceEvent{ThreadID: id, Kind: domain.EvCompletedTodo, At: cur.TodosAt})
		}
		if !cur.RevisionsAt.IsZero() {
			out[id] = append(out[id], domain.EvidenceEvent{ThreadID: id, Kind: domain.EvRevisionAdded, At: cur.RevisionsAt})
		}
	}

	if err := s.store.SaveThreadProgress(dirty); err != nil {
		log.Printf("progress: save: %v", err)
	}
	return out
}

// projectItems pages a project's work items for the progress diff. Best-effort:
// on failure the sweep continues without new evidence rather than dropping the
// whole project from the board.
func (s *Server) projectItems(ctx context.Context, cl *plane.Client, slug, projID string) []plane.WorkItemDetail {
	items, err := cl.ListProjectItems(ctx)
	if err != nil {
		log.Printf("instance %s project %s: list work items for progress: %v", slug, projID, err)
		return nil
	}
	return items
}
