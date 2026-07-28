package heat

import (
	"testing"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

func TestClassify(t *testing.T) {
	cycle := time.Hour
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
			got := Classify(c.b, cycle, now)
			if got.Lifecycle != c.want {
				t.Fatalf("want %s, got %s (%s)", c.want, got.Lifecycle, got.Reason)
			}
		})
	}
}

// Fresh evidence must score higher than stale evidence (buoyancy ordering).
func TestScoreDecay(t *testing.T) {
	cycle := time.Hour
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	fresh := Classify(domain.Bubble{Owner: "me", Evidence: []domain.EvidenceEvent{{At: now.Add(-1 * time.Minute)}}}, cycle, now)
	stale := Classify(domain.Bubble{Owner: "me", Evidence: []domain.EvidenceEvent{{At: now.Add(-50 * time.Minute)}}}, cycle, now)
	if !(fresh.Score > stale.Score) {
		t.Fatalf("fresh (%.3f) should outscore stale (%.3f)", fresh.Score, stale.Score)
	}
}
