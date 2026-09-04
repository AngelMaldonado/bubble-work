package heat

import (
	"math"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

// One-week cycles, dormant after 2, decay over 2, one cycle of grace.
var tun = Tuning{CycleHours: 168, DormantCycles: 2, DecayCycles: 2, GraceCycles: 1, OwnerlessIsDormant: true}

func ago(d time.Duration) time.Time { return now.Add(-d) }

const (
	day  = 24 * time.Hour
	week = 7 * day
)

func TestClassify_TheLadder(t *testing.T) {
	cases := []struct {
		name string
		ev   Evidence
		want Lifecycle
		code string
	}{
		{"output today", Evidence{LastWarmAt: ago(day), WarmCount: 1}, Hot, ReasonHotCurrent},
		{"output last cycle", Evidence{LastWarmAt: ago(10 * day), WarmCount: 1}, Warm, ReasonWarmPrevious},
		{"quiet two cycles", Evidence{LastWarmAt: ago(3 * week), WarmCount: 1}, Dormant, ReasonDormantSilent},
		{"never produced", Evidence{CreatedAt: ago(8 * week)}, Dormant, ReasonDormantNever},
		{"completed", Evidence{Completed: true, LastWarmAt: ago(day)}, Closed, ReasonClosed},
		{"born a moment ago", Evidence{CreatedAt: ago(time.Hour)}, Cooling, ReasonNewborn},
	}
	for _, c := range cases {
		got := Classify(c.ev, tun, now)
		if got.Lifecycle != c.want || got.Code != c.code {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, got.Lifecycle, got.Code, c.want, c.code)
		}
	}
}

// The claim, and the reason RollUp is ordered the way it is: a bubble still
// producing stays Hot with nobody accountable. Ownerlessness sinks what has
// ALREADY gone quiet — output outranks paperwork.
//
// It is a BUBBLE rule. A thread has assignees; the contract is what needs an
// owner, so this is the only grain where it can fire at all.
func TestInvariant_Heat_OutputOutranksPaperwork(t *testing.T) {
	producing := Classify(Evidence{LastWarmAt: ago(day), WarmCount: 3}, tun, now)
	if got := RollUp([]Result{producing}, false, tun); got.Lifecycle != Hot {
		t.Errorf("ownerless but producing = %s, want hot", got.Lifecycle)
	}
	cooling := Classify(Evidence{LastWarmAt: ago(3 * week), WarmCount: 3}, tun, now)
	if got := RollUp([]Result{cooling}, false, tun); got.Code != ReasonDormantOwnerless {
		t.Errorf("ownerless and quiet = %s/%s, want dormant/ownerless", got.Lifecycle, got.Code)
	}
	if got := RollUp([]Result{cooling}, true, tun); got.Code != ReasonDormantSilent {
		t.Errorf("with an owner = %s/%s, want dormant/silent", got.Lifecycle, got.Code)
	}
	off := tun
	off.OwnerlessIsDormant = false
	if got := RollUp([]Result{cooling}, false, off); got.Code != ReasonDormantSilent {
		t.Errorf("with the knob off = %s/%s, want dormant/silent", got.Lifecycle, got.Code)
	}
}

// The curve v0 documented: fresh ≈ 1.0, one decay window ≈ 0.37, two ≈ 0.14.
func TestInvariant_Heat_TheDecayCurve(t *testing.T) {
	decay := time.Duration(tun.DecayCycles) * week
	for _, c := range []struct {
		age  time.Duration
		want float64
	}{{0, 1.0}, {decay, 0.3679}, {2 * decay, 0.1353}} {
		got := Classify(Evidence{LastWarmAt: ago(c.age), WarmCount: 1}, tun, now).Score
		if math.Abs(got-c.want) > 0.001 {
			t.Errorf("age %v: score %.4f, want %.4f", c.age, got, c.want)
		}
	}
	if got := Classify(Evidence{}, tun, now).Score; got != 0 {
		t.Errorf("no evidence scored %v, want 0", got)
	}
}

// Time is a PARAMETER. The same evidence cools as `now` moves, with nothing
// stored and no job running.
func TestInvariant_Heat_TimeIsTheOnlyThingThatChanges(t *testing.T) {
	ev := Evidence{LastWarmAt: now, WarmCount: 1}
	want := []Lifecycle{Hot, Hot, Warm, Dormant}
	for i, at := range []time.Time{now, now.Add(6 * day), now.Add(10 * day), now.Add(4 * week)} {
		if got := Classify(ev, tun, at).Lifecycle; got != want[i] {
			t.Errorf("at +%v: %s, want %s", at.Sub(now), got, want[i])
		}
	}
}

// Recalibrating changes every verdict at once, because nothing was stored.
func TestHeat_RecalibrationIsImmediate(t *testing.T) {
	ev := Evidence{LastWarmAt: ago(10 * day), WarmCount: 1}
	if got := Classify(ev, tun, now).Lifecycle; got != Warm {
		t.Fatalf("with weekly cycles: %s, want warm", got)
	}
	monthly := tun
	monthly.CycleHours = 24 * 30
	if got := Classify(ev, monthly, now).Lifecycle; got != Hot {
		t.Errorf("with monthly cycles: %s, want hot", got)
	}
}

// A comment holds a thread out of the grave and never warms it.
func TestInvariant_Heat_PulseNeverWarms(t *testing.T) {
	commented := Evidence{LastAnyAt: ago(day), CreatedAt: ago(8 * week)}
	if got := Classify(commented, tun, now); got.Lifecycle != Dormant {
		t.Errorf("a comment warmed it to %s", got.Lifecycle)
	}
	if !HasPulse(commented, tun, now) {
		t.Error("a recent comment is not registering as a pulse")
	}
	silent := Evidence{LastAnyAt: ago(6 * week), CreatedAt: ago(8 * week)}
	if HasPulse(silent, tun, now) {
		t.Error("an old comment still counts as a pulse")
	}
}

// A bubble is as hot as its hottest UNFINISHED thread — not the union of its
// threads' evidence, which counts every birth as the bubble's own output and makes
// a pile of untouched work read hot.
func TestInvariant_Heat_BubbleIsItsHottestOpenThread(t *testing.T) {
	hot := Classify(Evidence{LastWarmAt: ago(day), WarmCount: 1}, tun, now)
	cold := Classify(Evidence{LastWarmAt: ago(4 * week), WarmCount: 1}, tun, now)
	done := Classify(Evidence{Completed: true}, tun, now)

	if got := RollUp([]Result{cold, hot, done}, true, tun).Lifecycle; got != Hot {
		t.Errorf("roll-up = %s, want hot", got)
	}
	// A bubble whose only hot thread is FINISHED is not hot.
	hotDone := Classify(Evidence{Completed: true, LastWarmAt: ago(time.Hour)}, tun, now)
	if got := RollUp([]Result{cold, hotDone}, true, tun).Lifecycle; got != Dormant {
		t.Errorf("with only a finished hot thread: %s, want dormant", got)
	}
	if got := RollUp(nil, true, tun).Lifecycle; got != Dormant {
		t.Errorf("an empty bubble = %s, want dormant", got)
	}
}
