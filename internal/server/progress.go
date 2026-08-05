package server

import (
	"context"
	"log"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
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
// Reads the mirror, not Plane (docs/PLANE-SYNC.md Phase 2). Note it does NOT
// reuse mirror_items.description_hash: that fingerprints the WHOLE body, so a
// Brief or title edit would register as production. Only the Logbook and DoD are
// production, so the Logbook fingerprint is recomputed here. Hashing a body is
// local CPU; the call it used to cost was the expensive part, and that is gone.
func (s *Server) progressEvidenceFromMirror(items []mirror.Item) map[string][]domain.EvidenceEvent {
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
	pulse, err := s.store.PulseFor(ids)
	if err != nil {
		log.Printf("pulse: load: %v", err)
	}

	now := s.now()
	out := make(map[string][]domain.EvidenceEvent, len(items))
	var dirty []store.ThreadProgress

	for id, cur := range observed {
		if p, ok := pulse[id]; ok && !p.LastCommentAt.IsZero() {
			out[id] = append(out[id], domain.EvidenceEvent{ThreadID: id, Kind: domain.EvComment, At: p.LastCommentAt})
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

// pulseProbeLimit bounds how many at-risk threads one tick checks for a pulse.
// The probe costs one Plane call per thread, so it is deliberately small: the
// least-recently-checked threads are picked, and the rest wait for a later tick.
const pulseProbeLimit = 20

// probePulse asks Plane for comments on the threads we are about to declare
// abandoned, and records when their discussion was last alive.
//
// Comments can't be swept cheaply — Plane has no project-wide comment feed, so
// reading them is one call per thread. But the pulse is only ever load-bearing
// in ONE place: blocking 🪦. So instead of polling every thread, this probes only
// the threads that already compute to 🪦, least-recently-checked first, capped
// per tick. Everything else records a pulse for free whenever someone opens or
// posts to the discussion.
//
// The recorded pulse reaches the board via the next snapshot refresh, which
// emits it as a non-progress evidence event.
func (s *Server) probePulse(ctx context.Context, bubbles []domain.Bubble) {
	if s.Tuning().PulseCycles <= 0 {
		return // the pulse is switched off; don't spend calls on it
	}
	type target struct{ slug, proj, wid string }
	at := map[string]target{}
	var ids []string
	for _, b := range bubbles {
		buoy := s.threadBuoyancy(b)
		for _, t := range b.Threads {
			if !t.Active || buoy[t.ID].Level != "rip" {
				continue
			}
			at[t.ID] = target{b.Instance, b.Project, t.ID}
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		return
	}

	pick, err := s.store.StalePulseCheck(ids, pulseProbeLimit)
	if err != nil {
		log.Printf("pulse: pick: %v", err)
		return
	}
	checked, found := 0, 0
	for _, id := range pick {
		tgt := at[id]
		inst, ok, err := s.instanceBySlug(tgt.slug)
		if err != nil || !ok {
			continue
		}
		cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, tgt.proj)
		cms, err := cl.ListComments(ctx, id)
		if err != nil {
			log.Printf("pulse: comments for %s: %v", id, err)
			continue
		}
		var latest time.Time
		for _, c := range cms {
			if c.CreatedAt.After(latest) {
				latest = c.CreatedAt
			}
		}
		if err := s.store.RecordPulse(id, latest, s.now()); err != nil {
			log.Printf("pulse: record %s: %v", id, err)
			continue
		}
		checked++
		if !latest.IsZero() {
			found++
		}
	}
	log.Printf("pulse: probed %d/%d at-risk thread(s), %d had a discussion", checked, len(ids), found)
}
