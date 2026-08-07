// Package heat derives a bubble's temperature as a PURE function of its
// evidence, the tuning, and the current time (§0, §5). Nothing is stored and
// nothing decays in the background — time passing changes the inputs, and the
// output changes for free.
//
// The same classifier runs at two grains: a whole bubble (Classify) and a single
// thread measured against its bubble's window (ClassifyThread) — see
// THREAD-LIFECYCLE.md. Every threshold it uses lives in domain.Tuning, which a
// service admin can calibrate at runtime.
package heat

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

// Reason codes. Reason carries the English sentence — the CLI, MCP and logs read
// it and are unaffected by any of this — while Code and Args say the same thing
// structurally so a client can render it in its own language.
//
// Deliberately NOT a translation layer in here: the heat model has no business
// knowing what language anyone reads. It states what it concluded; presentation
// is the client's problem.
const (
	ReasonHotCurrent       = "hot_current"       // output in the current cycle
	ReasonWarmPrevious     = "warm_previous"     // output last cycle, threads still open
	ReasonCooling          = "cooling"           // nothing this cycle or last
	ReasonDormantOwnerless = "dormant_ownerless" // nobody accountable
	ReasonDormantNever     = "dormant_never"     // nothing ever produced
	ReasonDormantSilent    = "dormant_silent"    // silent for {cycles}+ cycles
	ReasonThreadWarmOpen   = "thread_warm_open"  // thread wording for Warm
	ReasonThreadNewborn    = "thread_newborn"    // born recently, nothing yet
	ReasonClosedBubble     = "closed_bubble"     // outcome reached or abandoned
	ReasonClosedThread     = "closed_thread"     // the work item is complete
)

// Result is the derived temperature of a bubble or thread.
type Result struct {
	Lifecycle domain.Lifecycle
	Score     float64 // 0..1, higher = hotter = floats higher
	Reason    string  // English, for the CLI and logs
	Code      string  // stable key for clients that translate (see above)
	Args      map[string]string
}

// Window is the resolved heat window recency is measured against: the current
// cycle's start, the start of the oldest cycle that still counts as "recent",
// the resolved cycle length, and the length the score decays over.
type Window struct {
	CurStart  time.Time
	PrevStart time.Time
	Length    time.Duration // one cycle
	Decay     time.Duration // Length × Tuning.DecayCycles
}

// WindowFor resolves a bubble's heat window: the Plane cycle boundaries when the
// project has an active cycle (§3.6), otherwise a rolling window of one cycle
// ending now. A bubble's threads are classified against the SAME window, so
// thread and bubble temperature stay commensurable.
func WindowFor(b domain.Bubble, tun domain.Tuning, now time.Time) Window {
	length := tun.Cycle()
	curStart := now.Add(-length)
	var prevStart time.Time

	if !b.CycleStart.IsZero() {
		curStart = b.CycleStart
		if !b.CyclePrevStart.IsZero() {
			if d := curStart.Sub(b.CyclePrevStart); d > 0 {
				length = d // the project's real cycle length
			}
			if tun.DormantCycles == 2 {
				prevStart = b.CyclePrevStart // exact Plane boundary at the default
			}
		}
	}
	if prevStart.IsZero() {
		prevStart = curStart.Add(-time.Duration(float64(length) * (tun.DormantCycles - 1)))
	}
	return Window{
		CurStart:  curStart,
		PrevStart: prevStart,
		Length:    length,
		Decay:     time.Duration(float64(length) * tun.DecayCycles),
	}
}

// Classify computes a bubble's lifecycle and buoyancy score.
//
// The cycle is the repeating pulse against which recency is measured (§1). A
// bubble is Hot if it produced meaningful output in the current cycle, Warm if
// it did last cycle and still has active threads, Cooling if it has gone quiet,
// and Dormant once it has been silent for Tuning.DormantCycles cycles or has no
// owner.
func Classify(b domain.Bubble, tun domain.Tuning, now time.Time) Result {
	if b.Closed {
		return Result{domain.Closed, 0, "outcome reached or explicitly abandoned", ReasonClosedBubble, nil}
	}
	active := false
	for _, t := range b.Threads {
		if t.Active {
			active = true
			break
		}
	}
	ownerless := tun.OwnerlessIsDormant && b.Owner == ""
	// Pulse (comments) is presence, not output — it must not warm a bubble either.
	return classify(without(b.Evidence, pulseOnly), WindowFor(b, tun, now), ownerless, active, tun, now)
}

// ClassifyThread computes ONE thread's lifecycle from its own evidence stream,
// measured against its bubble's window (THREAD-LIFECYCLE.md). A completed thread
// is Closed; otherwise the bubble rules apply at the work-item grain, with the
// thread's assignee standing in for the bubble's owner and its own openness
// standing in for "active threads remain".
//
// One rule differs from the bubble grain: unless Tuning.ThreadBirthHeats is on,
// a thread's own BIRTH does not heat it. A new work item is a real output for
// the bubble that gained it, but the item itself has produced nothing by
// existing — otherwise every freshly created thread would read 🔥 for a whole
// cycle while sitting untouched in Backlog.
func ClassifyThread(t domain.Thread, ev []domain.EvidenceEvent, w Window, tun domain.Tuning, now time.Time) Result {
	if !t.Active || t.CompletedAt != nil {
		return Result{domain.Closed, 0, "thread completed", ReasonClosedThread, nil}
	}
	// Only production heats a thread. Comments are stripped always (presence), and
	// the thread's own birth unless the calibration says otherwise.
	keep := func(e domain.EvidenceEvent) bool { return e.Progress() }
	if tun.ThreadBirthHeats {
		keep = func(e domain.EvidenceEvent) bool { return !e.Pulse() }
	}
	progress := make([]domain.EvidenceEvent, 0, len(ev))
	for _, e := range ev {
		if keep(e) {
			progress = append(progress, e)
		}
	}
	ownerless := tun.OwnerlessIsDormant && t.Owner == ""
	r := classify(progress, w, ownerless, t.Active, tun, now)
	switch {
	case r.Lifecycle == domain.Warm:
		// the bubble wording doesn't fit a thread
		r.Reason, r.Code = "progress last cycle; still open", ReasonThreadWarmOpen
	case len(progress) == 0 && Newborn(t, w, tun, now):
		r.Reason, r.Code = "born recently; nothing produced yet", ReasonThreadNewborn
	}
	return r
}

// Newborn reports whether a thread is still inside its grace period — born less
// than Tuning.ThreadGraceCycles cycles ago. A newborn that hasn't produced
// anything yet is waiting its turn (😴), not abandoned (🪦).
func Newborn(t domain.Thread, w Window, tun domain.Tuning, now time.Time) bool {
	if tun.ThreadGraceCycles <= 0 || t.CreatedAt.IsZero() {
		return false
	}
	return t.CreatedAt.After(now.Add(-time.Duration(float64(w.Length) * tun.ThreadGraceCycles)))
}

// HasPulse reports whether someone has commented on a thread recently enough to
// keep it out of the grave. A pulse never warms anything — it only says a human
// is still paying attention, so declaring the thread abandoned would be wrong
// (THREAD-LIFECYCLE.md).
func HasPulse(ev []domain.EvidenceEvent, w Window, tun domain.Tuning, now time.Time) bool {
	if tun.PulseCycles <= 0 {
		return false
	}
	cutoff := now.Add(-time.Duration(float64(w.Length) * tun.PulseCycles))
	for _, e := range ev {
		if e.Pulse() && e.At.After(cutoff) {
			return true
		}
	}
	return false
}

func pulseOnly(e domain.EvidenceEvent) bool { return e.Pulse() }

// without returns the events that do NOT match drop, sharing the input when
// nothing matches (the common case: most threads have no comments).
func without(ev []domain.EvidenceEvent, drop func(domain.EvidenceEvent) bool) []domain.EvidenceEvent {
	n := 0
	for _, e := range ev {
		if drop(e) {
			n++
		}
	}
	if n == 0 {
		return ev
	}
	out := make([]domain.EvidenceEvent, 0, len(ev)-n)
	for _, e := range ev {
		if !drop(e) {
			out = append(out, e)
		}
	}
	return out
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
// ownerless means "nobody is accountable AND that should sink it"; active means
// "work remains open".
func classify(evidence []domain.EvidenceEvent, w Window, ownerless, active bool, tun domain.Tuning, now time.Time) Result {
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

	// Buoyancy score: exponential decay of the most-recent evidence over the decay
	// window. Fresh output ≈ 1.0; one decay-window old ≈ 0.37; two ≈ 0.14.
	score := 0.0
	if !latest.IsZero() {
		score = math.Exp(-now.Sub(latest).Seconds() / w.Decay.Seconds())
	}

	// Note the ordering: something actively producing stays Hot/Warm even with
	// nobody named as owner. Ownerlessness sinks what has already gone quiet.
	switch {
	case inCurrent:
		return Result{domain.Hot, score, "meaningful output in the current cycle", ReasonHotCurrent, nil}
	case inPrevious && active:
		return Result{domain.Warm, score, "output last cycle; active threads remain", ReasonWarmPrevious, nil}
	case latest.IsZero() || latest.Before(w.PrevStart) || ownerless:
		r := Result{Lifecycle: domain.Dormant, Score: score}
		r.Reason, r.Code, r.Args = dormantReason(latest, ownerless, tun.DormantCycles)
		return r
	default:
		return Result{domain.Cooling, score, "no meaningful output this cycle or last", ReasonCooling, nil}
	}
}

func dormantReason(latest time.Time, ownerless bool, cycles float64) (text, code string, args map[string]string) {
	switch {
	case ownerless:
		return "no active owner", ReasonDormantOwnerless, nil
	case latest.IsZero():
		return "no meaningful output ever recorded", ReasonDormantNever, nil
	default:
		n := strconv.FormatFloat(cycles, 'g', -1, 64)
		return fmt.Sprintf("silent for %s+ cycles", n), ReasonDormantSilent, map[string]string{"cycles": n}
	}
}
