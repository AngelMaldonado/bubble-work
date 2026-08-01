// Package heat derives a bubble's temperature as a PURE function of its
// evidence, the cycle length, and the current time (§0, §5). Nothing is stored
// and nothing decays in the background — time passing changes the inputs, and
// the output changes for free.
package heat

import (
	"math"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

// Result is the derived temperature of a bubble.
type Result struct {
	Lifecycle domain.Lifecycle
	Score     float64 // 0..1, higher = hotter = floats higher
	Reason    string
}

// Classify computes a bubble's lifecycle and buoyancy score.
//
// The cycle is the repeating pulse against which recency is measured (§1). A
// bubble is Hot if it produced meaningful output in the current cycle, Warm if
// it did last cycle and still has active threads, Cooling if it has gone quiet,
// and Dormant if it has been silent for two or more cycles or has no owner.
//
// When the bubble carries a Plane cycle window (§3.6), recency is measured
// against those real sprint boundaries; otherwise it falls back to a rolling
// window of length `cycle` ending now.
func Classify(b domain.Bubble, cycle time.Duration, now time.Time) Result {
	if b.Closed {
		return Result{domain.Closed, 0, "outcome reached or explicitly abandoned"}
	}

	// Resolve the heat window: Plane cycle when present, rolling otherwise.
	curStart := now.Add(-cycle)
	prevStart := now.Add(-2 * cycle)
	decay := cycle
	if !b.CycleStart.IsZero() {
		curStart = b.CycleStart
		if !b.CyclePrevStart.IsZero() {
			prevStart = b.CyclePrevStart
			if d := curStart.Sub(prevStart); d > 0 {
				decay = d // score decays over the real cycle length
			}
		} else {
			prevStart = curStart.Add(-cycle)
		}
	}

	var latest time.Time
	inCurrent, inPrevious := false, false
	for _, e := range b.Evidence {
		if e.At.After(latest) {
			latest = e.At
		}
		switch {
		case e.At.After(curStart):
			inCurrent = true
		case e.At.After(prevStart):
			inPrevious = true
		}
	}

	hasOwner := b.Owner != ""
	active := false
	for _, t := range b.Threads {
		if t.Active {
			active = true
			break
		}
	}

	// Buoyancy score: exponential decay of the most-recent evidence over one
	// cycle. Fresh output ≈ 1.0; a cycle-old bubble ≈ 0.37; two cycles ≈ 0.14.
	score := 0.0
	if !latest.IsZero() {
		score = math.Exp(-now.Sub(latest).Seconds() / decay.Seconds())
	}

	switch {
	case inCurrent:
		return Result{domain.Hot, score, "meaningful output in the current cycle"}
	case inPrevious && active:
		return Result{domain.Warm, score, "output last cycle; active threads remain"}
	case latest.IsZero() || latest.Before(prevStart) || !hasOwner:
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
