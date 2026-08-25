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
// ANY change to the DOCUMENT counts (docs/decisions/0004), not only the plan and
// not only a todo being ticked. Writing is the work here: re-phasing a plan,
// sharpening a Brief, or striking a todo are all changed reality rather than
// motion. Two fingerprints are kept so the evidence can be LABELLED — a tick is
// `completed-todo`, a plan change is `logbook-updated`, anything else on the page is
// `body-updated` — but all three warm the thread identically.
//
// Two rules matter:
//
//   - A thread we are seeing for the FIRST time is baselined silently. We have no
//     idea when its logbook was last touched, and stamping it "now" would
//     fabricate heat for work that may be a year old (§5.1).
//   - The TITLE is still not progress: it lives in Plane's `name`, not in the
//     document, and what work is called is not what has been done. Whitespace is
//     normalized first, so a reflow or a re-indent is not a change.
//
// A store failure is logged and yields no evidence — threads simply don't warm,
// which is the safe direction.
// Reads the mirror, not Plane (docs/journal/PLANE-SYNC.md Phase 2). Note it still
// does NOT reuse mirror_items.description_hash even though any edit now counts:
// that hash is over Plane's HTML, so Plane re-serialising a body would register as
// production. The fingerprints here are over the MARKDOWN, with whitespace
// collapsed. Hashing a body is local CPU; the call it used to cost is gone.
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

	// Links are external evidence published against a thread (docs/decisions/0006),
	// counted the same way revisions are: the count going UP is the event.
	ids0 := make([]string, 0, len(items))
	for _, it := range items {
		ids0 = append(ids0, it.ID)
	}
	links := map[string]int{}
	if s.mirror != nil {
		if n, err := s.mirror.LinkCount(slug, ids0); err == nil {
			links = n
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
			BodyHash:    md.BodyFingerprint(body),
			DoneTodos:   md.CountDone(body),
			Revisions:   revisions[it.ID],
			Links:       links[it.ID],
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
	// everything (docs/journal/PLANE-SYNC.md Phase 3).
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
		// Birth is production (docs/decisions/0002), and it is recorded by
		// BirthThread rather than detected here — there is nothing in a body that
		// says "this passed the birth rule". Emitted BEFORE the baseline check, so a
		// thread born a moment ago reports its birth even on the pass that first
		// sees it. A work item that merely appeared in Plane has no born_at and
		// earns nothing, which is the whole asymmetry the decision describes.
		if !was.BornAt.IsZero() {
			out[id] = append(out[id], domain.EvidenceEvent{ThreadID: id, Kind: domain.EvThreadBorn, At: was.BornAt})
		}
		if !seen {
			dirty = append(dirty, cur) // baseline, no evidence
			continue
		}
		cur.BornAt = was.BornAt // carry it forward; the sweep must never clear a birth
		cur.LogbookAt, cur.RevisionsAt, cur.LinksAt = was.LogbookAt, was.RevisionsAt, was.LinksAt
		cur.LogbookKind = was.LogbookKind
		// ANY change to the document is production (docs/decisions/0004), not only a
		// change to the plan. Which part moved decides the LABEL, not whether it
		// counts: a ticked todo, then the plan, then anything else on the page.
		//
		// A row carrying no body_hash yet has never been compared on this rule, so it
		// baselines silently rather than announcing an old prose edit as new work.
		bodyMoved := was.BodyHash != "" && cur.BodyHash != was.BodyHash
		planMoved := cur.LogbookHash != was.LogbookHash
		if planMoved || bodyMoved {
			cur.LogbookAt = now
			switch {
			case cur.DoneTodos > was.DoneTodos:
				cur.LogbookKind = domain.EvCompletedTodo // a tick, specifically
			case planMoved:
				cur.LogbookKind = domain.EvLogbookUpdated
			default:
				cur.LogbookKind = domain.EvBodyUpdated
			}
		}
		if cur.Revisions > was.Revisions {
			cur.RevisionsAt = now
		}
		// A link REMOVED is not production, so only an increase stamps the clock.
		if cur.Links > was.Links {
			cur.LinksAt = now
		}
		if planMoved || cur.BodyHash != was.BodyHash || cur.DoneTodos != was.DoneTodos ||
			cur.Revisions != was.Revisions || !cur.RevisionsAt.Equal(was.RevisionsAt) ||
			cur.Links != was.Links || !cur.LinksAt.Equal(was.LinksAt) {
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
		if !cur.LinksAt.IsZero() {
			out[id] = append(out[id], domain.EvidenceEvent{ThreadID: id, Kind: domain.EvLinkAdded, At: cur.LinksAt})
		}
	}

	if err := s.store.SaveThreadProgress(dirty); err != nil {
		log.Printf("progress: save: %v", err)
	}
	return out
}
