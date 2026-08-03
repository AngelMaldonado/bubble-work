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
// Plane exposes no "the logbook changed" event, so we detect production by
// DIFFING: each sweep fingerprints every thread's Logbook (+ DoD) and counts its
// revision sub-items, compares against what it saw last time, and records the
// moment something changed. The stored timestamp is the durable part; heat is
// derived from it on every read, so one edit keeps a thread warm for a whole
// cycle rather than for a single refresh.
//
// ANY change to the Logbook counts, not just a todo being ticked. The Logbook is
// the plan (§ working protocol: "update the Logbook when the plan materially
// changes"), so re-phasing it, adding a todo, or striking one is a recorded
// decision — changed reality, not motion. We still track the ticked count, but
// only to label the evidence: a tick is `completed-todo`, anything else is
// `logbook-updated`. Both warm the thread identically.
//
// Two rules matter:
//
//   - A thread we are seeing for the FIRST time is baselined silently. We have no
//     idea when its logbook was last touched, and stamping it "now" would
//     fabricate heat for work that may be a year old (§5.1).
//   - Prose OUTSIDE the Logbook — the Brief, the title — is not progress. Only
//     the plan and the revisions are diffed. Whitespace is normalized first, so a
//     reflow isn't a change.
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
		body := md.FromHTML(it.DescriptionHTML)
		ids = append(ids, it.ID)
		observed[it.ID] = store.ThreadProgress{
			ThreadID:    it.ID,
			LogbookHash: md.LogbookFingerprint(body),
			DoneTodos:   md.CountDone(body),
			Revisions:   revisions[it.ID],
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
		cur.LogbookAt, cur.RevisionsAt = was.LogbookAt, was.RevisionsAt
		cur.LogbookKind = was.LogbookKind
		if cur.LogbookHash != was.LogbookHash {
			cur.LogbookAt = now
			cur.LogbookKind = domain.EvLogbookUpdated
			if cur.DoneTodos > was.DoneTodos {
				cur.LogbookKind = domain.EvCompletedTodo // a tick, specifically
			}
		}
		if cur.Revisions > was.Revisions {
			cur.RevisionsAt = now
		}
		if cur.LogbookHash != was.LogbookHash || cur.DoneTodos != was.DoneTodos ||
			cur.Revisions != was.Revisions || !cur.RevisionsAt.Equal(was.RevisionsAt) {
			dirty = append(dirty, cur)
		}
		if !cur.LogbookAt.IsZero() {
			kind := cur.LogbookKind
			if kind == "" {
				kind = domain.EvLogbookUpdated
			}
			out[id] = append(out[id], domain.EvidenceEvent{ThreadID: id, Kind: kind, At: cur.LogbookAt})
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
