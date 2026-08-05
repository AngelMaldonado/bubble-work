package server

import (
	"log"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
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
// Reads the mirror, not Plane (docs/PLANE-SYNC.md Phase 2). Note it does NOT
// reuse mirror_items.description_hash: that fingerprints the WHOLE body, so a
// Brief or title edit would register as production. Only the Logbook and DoD are
// production, so the Logbook fingerprint is recomputed here. Hashing a body is
// local CPU; the call it used to cost was the expensive part, and that is gone.
func (s *Server) progressEvidenceFromMirror(slug string, items []mirror.Item) map[string][]domain.EvidenceEvent {
	if len(items) == 0 {
		return nil
	}

	// A revision artifact is a sub-work-item, so a thread's revision count is how
	// many items name it as parent.
	revisions := make(map[string]int, len(items))
	for _, it := range items {
		if it.ParentID != "" {
			revisions[it.ParentID]++
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
	// Pulse rides along in the evidence stream as a non-progress event: the
	// classifier ignores it, threadLevel uses it to block the grave.
	//
	// It comes from the mirrored comments now — a SELECT, where it used to be a
	// Plane call per at-risk thread, capped at 20 a tick and never able to cover
	// everything (docs/PLANE-SYNC.md Phase 3).
	//
	// The stored pulse is still consulted, and the LATER of the two wins. They
	// answer subtly different questions: an item with no mirrored comments might
	// have none, or might simply not have had its comments fetched yet — the
	// comment budget fills those in over several passes. Taking the max means a
	// thread is never wrongly declared abandoned because the mirror had not got
	// to it, while a mirrored comment still corrects a stale stored pulse.
	pulse, err := s.store.PulseFor(ids)
	if err != nil {
		log.Printf("pulse: load: %v", err)
	}
	lastComment := make(map[string]time.Time, len(ids))
	if s.mirror != nil {
		for _, id := range ids {
			if at, err := s.mirror.LastCommentAt(slug, id); err == nil && !at.IsZero() {
				lastComment[id] = at
			}
		}
	}

	now := s.now()
	out := make(map[string][]domain.EvidenceEvent, len(items))
	var dirty []store.ThreadProgress

	for id, cur := range observed {
		at := pulse[id].LastCommentAt
		if m := lastComment[id]; m.After(at) {
			at = m
		}
		if !at.IsZero() {
			out[id] = append(out[id], domain.EvidenceEvent{ThreadID: id, Kind: domain.EvComment, At: at})
		}
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
