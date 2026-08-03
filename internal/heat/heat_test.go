package heat

import (
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
