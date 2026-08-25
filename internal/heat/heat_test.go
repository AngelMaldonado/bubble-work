package heat

import (
	"math"
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

// tuning returns the default calibration with a given cycle length, so the
// tests read as "default model, short cycle".
func tuning(cycle time.Duration) domain.Tuning {
	t := domain.DefaultTuning()
	t.CycleHours = cycle.Hours()
	return t
}

func TestClassify(t *testing.T) {
	tun := tuning(time.Hour)
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	// evidence at `mins` minutes before now.
	at := func(mins int) time.Time { return now.Add(-time.Duration(mins) * time.Minute) }
	bubble := func(owner string, active bool, evMins ...int) domain.Bubble {
		b := domain.Bubble{Owner: owner, Threads: []domain.Thread{{Active: active}}}
		for _, m := range evMins {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{At: at(m)})
		}
		return b
	}

	cases := []struct {
		name string
		b    domain.Bubble
		want domain.Lifecycle
	}{
		{"hot: output this cycle", bubble("me", true, 10), domain.Hot},
		{"warm: last cycle + active", bubble("me", true, 90), domain.Warm},
		{"cooling: last cycle but no active threads", bubble("me", false, 90), domain.Cooling},
		{"dormant: silent 2+ cycles", bubble("me", true, 200), domain.Dormant},
		{"dormant: no owner", bubble("", false, 90), domain.Dormant},
		{"dormant: no evidence at all", bubble("me", true), domain.Dormant},
		{"closed wins", domain.Bubble{Closed: true, Owner: "me", Evidence: []domain.EvidenceEvent{{At: at(1)}}}, domain.Closed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.b, tun, now)
			if got.Lifecycle != c.want {
				t.Fatalf("want %s, got %s (%s)", c.want, got.Lifecycle, got.Reason)
			}
		})
	}
}

// A thread is classified from ITS OWN evidence against the bubble's window, so
// two threads in one bubble can sit at different temperatures
// (THREAD-LIFECYCLE.md Phase A).
func TestClassifyThread(t *testing.T) {
	tun := tuning(time.Hour)
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	at := func(mins int) time.Time { return now.Add(-time.Duration(mins) * time.Minute) }

	hot := domain.Thread{ID: "a", Active: true, Owner: "me"}
	cold := domain.Thread{ID: "b", Active: true, Owner: "me"}
	shipped := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)
	done := domain.Thread{ID: "c", Active: false, Owner: "me", CompletedAt: &shipped}

	b := domain.Bubble{
		Owner:   "me",
		Threads: []domain.Thread{hot, cold, done},
		Evidence: []domain.EvidenceEvent{
			{ThreadID: "a", Kind: domain.EvCompletedTodo, At: at(5)},   // this cycle
			{ThreadID: "b", Kind: domain.EvCompletedTodo, At: at(300)}, // long gone
			{ThreadID: "c", Kind: domain.EvCompletedTodo, At: shipped},
		},
	}
	win := WindowFor(b, tun, now)
	byThread := Attribute(b.Evidence)

	// The bubble as a whole is Hot — but only one of its threads is.
	if r := Classify(b, tun, now); r.Lifecycle != domain.Hot {
		t.Fatalf("bubble: want Hot, got %s", r.Lifecycle)
	}
	if r := ClassifyThread(hot, byThread["a"], win, tun, now); r.Lifecycle != domain.Hot {
		t.Fatalf("thread a: want Hot, got %s", r.Lifecycle)
	}
	if r := ClassifyThread(cold, byThread["b"], win, tun, now); r.Lifecycle != domain.Dormant {
		t.Fatalf("thread b: want Dormant, got %s", r.Lifecycle)
	}
	if r := ClassifyThread(done, byThread["c"], win, tun, now); r.Lifecycle != domain.Closed {
		t.Fatalf("thread c: want Closed, got %s", r.Lifecycle)
	}
	// Birth does not heat a thread: a work item created minutes ago that has
	// produced nothing is NOT 🔥 — otherwise every new Backlog item would read as
	// in-progress for a whole cycle just for existing.
	newborn := domain.Thread{ID: "e", Active: true, Owner: "me", CreatedAt: at(2)}
	birthOnly := []domain.EvidenceEvent{{ThreadID: "e", Kind: domain.EvThreadCreated, At: at(2)}}
	if r := ClassifyThread(newborn, birthOnly, win, tun, now); r.Lifecycle != domain.Dormant {
		t.Fatalf("newborn thread: want Dormant, got %s (%s)", r.Lifecycle, r.Reason)
	} else if r.Reason != "born recently; nothing produced yet" {
		t.Errorf("newborn reason = %q", r.Reason)
	}
	// The bubble that gained it, however, IS warmed by the birth (§5.1).
	nb := domain.Bubble{Owner: "me", Threads: []domain.Thread{newborn}, Evidence: birthOnly}
	if r := Classify(nb, tun, now); r.Lifecycle != domain.Hot {
		t.Fatalf("bubble gaining a thread: want Hot, got %s", r.Lifecycle)
	}

	// An unassigned open thread has nobody accountable → Dormant, like a bubble.
	orphan := domain.Thread{ID: "d", Active: true}
	if r := ClassifyThread(orphan, []domain.EvidenceEvent{{At: at(5)}}, win, tun, now); r.Lifecycle != domain.Hot {
		t.Fatalf("orphan with fresh output should still be Hot, got %s", r.Lifecycle)
	}
	if r := ClassifyThread(orphan, nil, win, tun, now); r.Lifecycle != domain.Dormant {
		t.Fatalf("orphan with no output: want Dormant, got %s", r.Lifecycle)
	}
}

// TestInvariant_Heat_EveryBranchOfClassify pins the shared rule itself — the one
// `classify` both grains delegate to. Its five outcomes were only ever reached
// through Classify/ClassifyThread, so a branch could change meaning without any
// test naming what it decided. Each case here isolates ONE branch, including the
// ordering that matters most: output outranks paperwork, so something actively
// producing stays Hot even with nobody accountable.
func TestInvariant_Heat_EveryBranchOfClassify(t *testing.T) {
	tun := tuning(time.Hour) // window: current = last 1h, previous = 1h before that
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	at := func(mins int) time.Time { return now.Add(-time.Duration(mins) * time.Minute) }

	// A bubble carries the evidence; owner and an open thread are varied per case.
	bubble := func(owner string, active bool, evMins ...int) domain.Bubble {
		b := domain.Bubble{Owner: owner, Threads: []domain.Thread{{Active: active}}}
		for _, m := range evMins {
			b.Evidence = append(b.Evidence, domain.EvidenceEvent{Kind: domain.EvCompletedTodo, At: at(m)})
		}
		return b
	}

	cases := []struct {
		name string
		b    domain.Bubble
		want domain.Lifecycle
		code string
	}{
		{"inCurrent", bubble("me", true, 10), domain.Hot, ReasonHotCurrent},
		{"inCurrent wins over older evidence", bubble("me", true, 10, 200), domain.Hot, ReasonHotCurrent},
		{"inPrevious and open", bubble("me", true, 90), domain.Warm, ReasonWarmPrevious},
		{"inPrevious but nothing open", bubble("me", false, 90), domain.Cooling, ReasonCooling},
		{"never produced", bubble("me", true), domain.Dormant, ReasonDormantNever},
		{"silent past the threshold", bubble("me", true, 200), domain.Dormant, ReasonDormantSilent},
		{"ownerless once quiet", bubble("", false, 90), domain.Dormant, ReasonDormantOwnerless},
		// The ordering rule: ownerless does NOT sink something still producing.
		{"ownerless but producing", bubble("", true, 10), domain.Hot, ReasonHotCurrent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.b, tun, now)
			if got.Lifecycle != c.want {
				t.Fatalf("lifecycle: want %s, got %s (%s)", c.want, got.Lifecycle, got.Reason)
			}
			// The code is the contract with translating clients; the English
			// sentence is for logs and may be reworded.
			if got.Code != c.code {
				t.Errorf("code: want %q, got %q", c.code, got.Code)
			}
		})
	}

	// dormant_silent carries the threshold so a client can say "silent for N+
	// cycles" in its own language.
	if r := Classify(bubble("me", true, 200), tun, now); r.Args["cycles"] != "2" {
		t.Errorf("dormant_silent args = %v, want cycles=2", r.Args)
	}

	// OwnerlessIsDormant off: nobody accountable stops mattering.
	lenient := tun
	lenient.OwnerlessIsDormant = false
	if r := Classify(bubble("", false, 90), lenient, now); r.Lifecycle != domain.Cooling {
		t.Errorf("with ownerless_is_dormant off: want Cooling, got %s", r.Lifecycle)
	}
}

// TestInvariant_Heat_ScoreIsExponentialDecay pins the actual curve, not just the
// ordering TestScoreDecay checks. The numbers are what make two bubbles in the
// same band sort sensibly, so a changed base or window is a behaviour change even
// though every lifecycle stays the same.
func TestInvariant_Heat_ScoreIsExponentialDecay(t *testing.T) {
	tun := tuning(time.Hour) // decay_cycles = 1, so the decay window is 1h
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	score := func(ago time.Duration) float64 {
		b := domain.Bubble{Owner: "me", Threads: []domain.Thread{{Active: true}},
			Evidence: []domain.EvidenceEvent{{Kind: domain.EvCompletedTodo, At: now.Add(-ago)}}}
		return Classify(b, tun, now).Score
	}

	cases := []struct {
		name string
		ago  time.Duration
		want float64
	}{
		{"just now", 0, 1.0},
		{"one decay window", time.Hour, 0.368},      // e^-1
		{"two decay windows", 2 * time.Hour, 0.135}, // e^-2
	}
	for _, c := range cases {
		if got := score(c.ago); math.Abs(got-c.want) > 0.005 {
			t.Errorf("%s: score = %.3f, want ≈%.3f", c.name, got, c.want)
		}
	}

	// No evidence scores zero — it must never sort above something that produced.
	empty := Classify(domain.Bubble{Owner: "me"}, tun, now)
	if empty.Score != 0 {
		t.Errorf("no evidence: score = %.3f, want 0", empty.Score)
	}
	// A closed bubble scores zero however fresh its last evidence was: it has
	// stopped taking attention.
	closed := Classify(domain.Bubble{Closed: true, Owner: "me",
		Evidence: []domain.EvidenceEvent{{Kind: domain.EvCompletedTodo, At: now}}}, tun, now)
	if closed.Score != 0 {
		t.Errorf("closed: score = %.3f, want 0", closed.Score)
	}
}

// Attribute drops bubble-level evidence (no ThreadID) and buckets the rest.
func TestAttribute(t *testing.T) {
	now := time.Now()
	got := Attribute([]domain.EvidenceEvent{
		{ThreadID: "a", At: now},
		{ThreadID: "a", At: now},
		{ThreadID: "b", At: now},
		{At: now}, // bubble-level: belongs to no thread
	})
	if len(got) != 2 || len(got["a"]) != 2 || len(got["b"]) != 1 {
		t.Fatalf("unexpected attribution: %v", got)
	}
}

// Fresh evidence must score higher than stale evidence (buoyancy ordering).
func TestScoreDecay(t *testing.T) {
	tun := tuning(time.Hour)
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	fresh := Classify(domain.Bubble{Owner: "me", Evidence: []domain.EvidenceEvent{{At: now.Add(-1 * time.Minute)}}}, tun, now)
	stale := Classify(domain.Bubble{Owner: "me", Evidence: []domain.EvidenceEvent{{At: now.Add(-50 * time.Minute)}}}, tun, now)
	if !(fresh.Score > stale.Score) {
		t.Fatalf("fresh (%.3f) should outscore stale (%.3f)", fresh.Score, stale.Score)
	}
}

// TestClassifyCycleAware: when the bubble carries a Plane cycle window, recency
// is measured against those real boundaries, not the rolling `cycle` argument.
func TestClassifyCycleAware(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	curStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)   // active cycle began 9 days ago
	prevStart := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) // previous cycle

	base := func(evAt time.Time) domain.Bubble {
		return domain.Bubble{
			Owner:          "me",
			Threads:        []domain.Thread{{Active: true}},
			Evidence:       []domain.EvidenceEvent{{At: evAt}},
			CycleStart:     curStart,
			CyclePrevStart: prevStart,
		}
	}

	// The rolling cycle is deliberately tiny (1h) — if it were used instead of the
	// window, everything would read as ancient. Cycle-aware must win.
	tun := tuning(time.Hour)

	// Evidence inside the active cycle → Hot, even though it's 3 days old
	// (far older than the 1h rolling window).
	if r := Classify(base(now.Add(-72*time.Hour)), tun, now); r.Lifecycle != domain.Hot {
		t.Fatalf("in-cycle evidence: want Hot, got %s", r.Lifecycle)
	}
	// Evidence in the previous cycle (with active threads) → Warm.
	if r := Classify(base(time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)), tun, now); r.Lifecycle != domain.Warm {
		t.Fatalf("prev-cycle evidence: want Warm, got %s", r.Lifecycle)
	}
	// Evidence before the previous cycle → Dormant (silent 2+ cycles).
	if r := Classify(base(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)), tun, now); r.Lifecycle != domain.Dormant {
		t.Fatalf("stale evidence: want Dormant, got %s", r.Lifecycle)
	}
}

// A comment is PRESENCE, not production: it must never warm anything, at either
// grain, but it does keep a thread out of the grave (THREAD-LIFECYCLE.md).
func TestPulseNeverWarms(t *testing.T) {
	tun := tuning(time.Hour)
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-5 * time.Minute)

	thread := domain.Thread{ID: "a", Active: true, Owner: "me", CreatedAt: now.AddDate(0, -6, 0)}
	chatter := []domain.EvidenceEvent{{ThreadID: "a", Kind: domain.EvComment, At: fresh}}

	b := domain.Bubble{Owner: "me", Threads: []domain.Thread{thread}, Evidence: chatter}
	// The bubble stays cold: a busy comment thread is not output.
	if r := Classify(b, tun, now); r.Lifecycle != domain.Dormant {
		t.Fatalf("comments warmed a bubble: %s", r.Lifecycle)
	}
	win := WindowFor(b, tun, now)
	if r := ClassifyThread(thread, chatter, win, tun, now); r.Lifecycle != domain.Dormant {
		t.Fatalf("comments warmed a thread: %s", r.Lifecycle)
	}
	// But the pulse is detectable, which is what blocks the grave.
	if !HasPulse(chatter, win, tun, now) {
		t.Error("a comment 5 minutes ago should register as a pulse")
	}
	// Old chatter is no pulse at all.
	stale := []domain.EvidenceEvent{{ThreadID: "a", Kind: domain.EvComment, At: now.Add(-300 * time.Minute)}}
	if HasPulse(stale, win, tun, now) {
		t.Error("a 5-hour-old comment should not register against a 1h pulse window")
	}
	// And it can be switched off entirely.
	off := tun
	off.PulseCycles = 0
	if HasPulse(chatter, win, off, now) {
		t.Error("pulse_cycles=0 should disable the pulse")
	}
	// Real progress still warms normally, so the pulse filter isn't over-eager.
	work := append(chatter, domain.EvidenceEvent{ThreadID: "a", Kind: domain.EvLogbookUpdated, At: fresh})
	if r := ClassifyThread(thread, work, win, tun, now); r.Lifecycle != domain.Hot {
		t.Fatalf("progress alongside chatter should be Hot, got %s", r.Lifecycle)
	}
}
