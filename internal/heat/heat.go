// Package heat derives a bubble's temperature as a PURE function of its
// evidence, the cycle length, and the current time (§0, §5). Nothing is stored
// and nothing decays in the background — time passing changes the inputs, and
// the output changes for free.
//
// The same classifier runs at two grains: a whole bubble (Classify) and a single
// thread measured against its bubble's window (ClassifyThread) — see
// THREAD-LIFECYCLE.md.
package heat

import (
	"math"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

// Result is the derived temperature of a bubble or thread.
type Result struct {
	Lifecycle domain.Lifecycle
	Score     float64 // 0..1, higher = hotter = floats higher
	Reason    string
}

// Window is the resolved heat window recency is measured against: the current
// cycle's start, the previous cycle's start, and the length score decays over.
type Window struct {
	CurStart  time.Time
	PrevStart time.Time
	Decay     time.Duration
}

// WindowFor resolves a bubble's heat window: the Plane cycle boundaries when the
// project has an active cycle (§3.6), otherwise a rolling window of length
// `cycle` ending now. A bubble's threads are classified against the SAME window,
// so thread and bubble temperature stay commensurable.
func WindowFor(b domain.Bubble, cycle time.Duration, now time.Time) Window {
	w := Window{CurStart: now.Add(-cycle), PrevStart: now.Add(-2 * cycle), Decay: cycle}
	if b.CycleStart.IsZero() {
		return w
	}
	w.CurStart = b.CycleStart
	if b.CyclePrevStart.IsZero() {
		w.PrevStart = w.CurStart.Add(-cycle)
		return w
	}
	w.PrevStart = b.CyclePrevStart
	if d := w.CurStart.Sub(w.PrevStart); d > 0 {
		w.Decay = d // score decays over the real cycle length
	}
	return w
}

// Classify computes a bubble's lifecycle and buoyancy score.
//
// The cycle is the repeating pulse against which recency is measured (§1). A
// bubble is Hot if it produced meaningful output in the current cycle, Warm if
// it did last cycle and still has active threads, Cooling if it has gone quiet,
// and Dormant if it has been silent for two or more cycles or has no owner.
func Classify(b domain.Bubble, cycle time.Duration, now time.Time) Result {
	if b.Closed {
		return Result{domain.Closed, 0, "outcome reached or explicitly abandoned"}
	}
	active := false
	for _, t := range b.Threads {
		if t.Active {
			active = true
			break
		}
	}
	return classify(b.Evidence, WindowFor(b, cycle, now), b.Owner != "", active, now)
}

// ClassifyThread computes ONE thread's lifecycle from its own evidence stream,
// measured against its bubble's window (THREAD-LIFECYCLE.md). A completed thread
// is Closed; otherwise the bubble rules apply at the work-item grain, with the
// thread's assignee standing in for the bubble's owner and its own openness
// standing in for "active threads remain".
//
// One rule differs from the bubble grain: a thread's own BIRTH does not heat it.
// A new work item is a real output for the bubble that gained it, but the item
// itself has produced nothing by existing — otherwise every freshly created
// thread would read 🔥 for a whole cycle while sitting untouched in Backlog.
func ClassifyThread(t domain.Thread, ev []domain.EvidenceEvent, w Window, now time.Time) Result {
	if !t.Active || t.CompletedAt != nil {
		return Result{domain.Closed, 0, "thread completed"}
	}
	progress := make([]domain.EvidenceEvent, 0, len(ev))
	for _, e := range ev {
		if e.Progress() {
			progress = append(progress, e)
		}
	}
	r := classify(progress, w, t.Owner != "", t.Active, now)
	switch {
	case r.Lifecycle == domain.Warm:
		r.Reason = "progress last cycle; still open" // the bubble wording doesn't fit a thread
	case len(progress) == 0 && !t.CreatedAt.IsZero() && t.CreatedAt.After(w.CurStart):
		r.Reason = "born this cycle; nothing produced yet"
	}
	return r
}

// Attribute buckets a bubble's evidence by the thread that produced it, so each
// thread can be classified from its own stream. Events with no ThreadID (bubble-
// level evidence) are dropped — they belong to no single thread.
func Attribute(evidence []domain.EvidenceEvent) map[string][]domain.EvidenceEvent {
	out := make(map[string][]domain.EvidenceEvent, len(evidence))
	for _, e := range evidence {
		if e.ThreadID == "" {
			continue
		}
		out[e.ThreadID] = append(out[e.ThreadID], e)
	}
	return out
}

// classify is the shared rule, applied identically to a bubble and to a thread.
// hasOwner is "someone is accountable"; active is "work remains open".
func classify(evidence []domain.EvidenceEvent, w Window, hasOwner, active bool, now time.Time) Result {
	var latest time.Time
	inCurrent, inPrevious := false, false
	for _, e := range evidence {
		if e.At.After(latest) {
			latest = e.At
		}
		switch {
		case e.At.After(w.CurStart):
			inCurrent = true
		case e.At.After(w.PrevStart):
			inPrevious = true
		}
	}

	// Buoyancy score: exponential decay of the most-recent evidence over one
	// cycle. Fresh output ≈ 1.0; a cycle-old bubble ≈ 0.37; two cycles ≈ 0.14.
	score := 0.0
	if !latest.IsZero() {
		score = math.Exp(-now.Sub(latest).Seconds() / w.Decay.Seconds())
	}

	switch {
	case inCurrent:
		return Result{domain.Hot, score, "meaningful output in the current cycle"}
	case inPrevious && active:
		return Result{domain.Warm, score, "output last cycle; active threads remain"}
	case latest.IsZero() || latest.Before(w.PrevStart) || !hasOwner:
		return Result{domain.Dormant, score, dormantReason(latest, hasOwner)}
	default:
		return Result{domain.Cooling, score, "no meaningful output this cycle or last"}
	}
}

func dormantReason(latest time.Time, hasOwner bool) string {
	switch {
	case !hasOwner:
		return "no active owner"
	case latest.IsZero():
		return "no meaningful output ever recorded"
	default:
		return "silent for two or more cycles"
	}
}
