// Package heat derives a temperature from evidence.
//
// It is a PURE function of the evidence, the calibration and the current time.
// Nothing is stored and nothing decays in a background job — time passing changes
// the inputs, so the output changes for free. Two consequences worth stating: there
// is no cooling job that can fall behind, and there is no stored level that can be
// wrong.
//
// `now` is a PARAMETER, not `time.Now()` read inside. That is what lets the curve
// be pinned at a fixed instant in a test, and what keeps "what did this look like
// last Tuesday" answerable at all.
package heat

import (
	"fmt"
	"math"
	"time"
)

// Lifecycle is the band something sits in.
type Lifecycle string

// Four bands, not five. There were Warm and Cooling between Hot and Dormant,
// and they were a gradient nobody acted on: "produced last cycle" and "produced
// neither cycle" both mean the same thing to a reader deciding what to do this
// morning. What is left is the set v0 shipped and people used — producing,
// quiet, abandoned, finished — where every band names a different ACTION.
//
// Rip is a band and not a decoration on Dormant. Quiet with somebody
// accountable is a person to ask; quiet with nobody accountable is a decision
// to make. Same silence, different problem, so it gets its own place to sit.
const (
	Hot     Lifecycle = "hot"
	Dormant Lifecycle = "dormant"
	Rip     Lifecycle = "rip"
	Closed  Lifecycle = "closed"
)

// Reason codes. Reason carries the English sentence for logs; Code and Args say
// the same thing structurally, so a client renders it in its own language. The
// heat model has no business knowing what language anyone reads.
const (
	ReasonHotCurrent    = "hot_current"
	ReasonRipOwnerless  = "rip_ownerless"
	ReasonDormantNever  = "dormant_never"
	ReasonDormantSilent = "dormant_silent"
	ReasonNewborn       = "newborn"
	ReasonClosed        = "closed"
)

// Tuning is the calibration, which lives in a row rather than in this file.
type Tuning struct {
	CycleHours     float64
	DormantCycles  float64
	DecayCycles    float64
	GraceCycles    float64
	OwnerlessIsRip bool
}

// Cycle is the resolved window length.
func (t Tuning) Cycle() time.Duration {
	if t.CycleHours <= 0 {
		return 168 * time.Hour
	}
	return time.Duration(t.CycleHours * float64(time.Hour))
}

// Evidence is what a thread's log adds up to — the shape the aggregation view
// returns. One timestamp for the newest thing that WARMS, one for the newest
// thing of any kind (a comment is presence, not production), and a count.
type Evidence struct {
	LastWarmAt time.Time
	LastAnyAt  time.Time
	WarmCount  int
	CreatedAt  time.Time
	Completed  bool
}

// Result is the derived temperature.
type Result struct {
	Lifecycle Lifecycle         `json:"lifecycle"`
	Score     float64           `json:"score"` // 0..1 — higher floats higher
	Reason    string            `json:"reason"`
	Code      string            `json:"code"`
	Args      map[string]string `json:"args,omitempty"`
}

// Window is what recency is measured against.
type Window struct {
	CurStart  time.Time
	PrevStart time.Time
	Length    time.Duration
	Decay     time.Duration
}

// WindowFor resolves the rolling window ending now.
func WindowFor(tun Tuning, now time.Time) Window {
	length := tun.Cycle()
	dormant := tun.DormantCycles
	if dormant < 1 {
		dormant = 2
	}
	decay := tun.DecayCycles
	if decay <= 0 {
		decay = 2
	}
	cur := now.Add(-length)
	return Window{
		CurStart:  cur,
		PrevStart: cur.Add(-time.Duration(float64(length) * (dormant - 1))),
		Length:    length,
		Decay:     time.Duration(float64(length) * decay),
	}
}

// Classify turns one thread's evidence into a band.
//
// Ownership is not consulted here, and that is deliberate. A thread has assignees;
// a BUBBLE has an owner, because the contract is what needs somebody accountable.
// The rule lives in RollUp, which is the only grain where it can fire.
func Classify(ev Evidence, tun Tuning, now time.Time) Result {
	if ev.Completed {
		return Result{Closed, 0, "the thread is complete", ReasonClosed, nil}
	}
	w := WindowFor(tun, now)

	score := 0.0
	if !ev.LastWarmAt.IsZero() {
		score = math.Exp(-now.Sub(ev.LastWarmAt).Seconds() / w.Decay.Seconds())
	}

	switch {
	// Anything that produced inside the dormancy window is producing. The window
	// is what DormantCycles configures, so the boundary stays tunable — it just
	// no longer has a band sitting on top of it.
	case ev.LastWarmAt.After(w.PrevStart):
		return Result{Hot, score, "meaningful output within the cycle window", ReasonHotCurrent, nil}

	// A thread born a moment ago with nothing on it is NEW, not abandoned. It
	// still reads quiet — it IS quiet — but the reason is what keeps grace from
	// calling it a grave.
	case ev.WarmCount == 0 && newborn(ev.CreatedAt, w, tun, now):
		return Result{Dormant, score, "born recently; nothing produced yet", ReasonNewborn, nil}

	default:
		r := Result{Lifecycle: Dormant, Score: score}
		r.Reason, r.Code, r.Args = dormantReason(ev.LastWarmAt, tun)
		return r
	}
}

// HasPulse reports whether somebody has commented recently enough to keep a
// thread out of the grave. A pulse NEVER warms anything — it only says a human is
// still paying attention, so calling the work abandoned would be wrong.
func HasPulse(ev Evidence, tun Tuning, now time.Time) bool {
	if ev.LastAnyAt.IsZero() {
		return false
	}
	return ev.LastAnyAt.After(WindowFor(tun, now).PrevStart)
}

// RollUp gives a bubble the band of its HOTTEST unfinished thread, and then puts
// it in the grave if nobody is accountable and it has already gone quiet.
//
// Not the union of its threads' evidence, which is what v0 did and what v0 found
// misleading: the union counts every thread's birth as the bubble's own output, so
// a bubble full of untouched new work reads Hot. The roll-up reports what its
// threads are actually doing.
//
// Ownerlessness is applied HERE and only here. The order is the model: a bubble
// still producing stays Hot with nobody named — output outranks paperwork — and
// the missing owner only buries what had already stopped.
func RollUp(threads []Result, hasOwner bool, tun Tuning) Result {
	rank := map[Lifecycle]int{Hot: 3, Dormant: 2, Rip: 1, Closed: 0}
	best := Result{Lifecycle: Dormant, Code: ReasonDormantNever,
		Reason: "nothing here has produced anything"}
	seen := false
	for _, t := range threads {
		if t.Lifecycle == Closed {
			continue // a finished thread says nothing about whether the bubble is alive
		}
		if !seen || rank[t.Lifecycle] > rank[best.Lifecycle] ||
			(rank[t.Lifecycle] == rank[best.Lifecycle] && t.Score > best.Score) {
			best, seen = t, true
		}
	}
	// Grace is what a newborn bubble is for: nothing has been produced yet
	// because there has been no time to produce it, and burying it on its first
	// morning teaches people to ignore the band.
	if tun.OwnerlessIsRip && !hasOwner &&
		best.Lifecycle != Hot && best.Code != ReasonNewborn {
		best.Lifecycle = Rip
		best.Reason = "quiet, and nobody accountable"
		best.Code = ReasonRipOwnerless
		best.Args = nil
	}
	return best
}

func newborn(created time.Time, w Window, tun Tuning, now time.Time) bool {
	if tun.GraceCycles <= 0 || created.IsZero() {
		return false
	}
	return created.After(now.Add(-time.Duration(float64(w.Length) * tun.GraceCycles)))
}

func dormantReason(latest time.Time, tun Tuning) (string, string, map[string]string) {
	if latest.IsZero() {
		return "nothing has ever been produced here", ReasonDormantNever, nil
	}
	n := tun.DormantCycles
	if n < 1 {
		n = 2
	}
	return fmt.Sprintf("silent for %g cycles or more", n), ReasonDormantSilent,
		map[string]string{"cycles": fmt.Sprintf("%g", n)}
}
