package plane

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Plane's rate limit is per API KEY, not per connection — and this package hands
// out a fresh Client per call site (plane.New(...) is called ad-hoc all over the
// server). So the budget cannot live on a Client; it lives in a process-wide
// registry keyed by the credential, and every Client built for that key shares
// it. See docs/journal/PLANE-SYNC.md Phase 0.
//
// Plane reports the state of that limit on EVERY response:
//
//	x-ratelimit-remaining: 16
//	x-ratelimit-reset:     1785849381   (unix seconds)
//
// Before this, we ignored both and discovered the ceiling by being 429'd. Now we
// budget against it, and — the point of the exercise — we protect a slice of it
// for work a human is waiting on.

// backgroundFloor is how much of the allowance is reserved for interactive work.
// Once the remaining budget drops to this, BACKGROUND callers wait for the reset
// window; interactive callers always pass. Plane's default ceiling is 60/min, so
// this keeps roughly a quarter of it for humans.
const backgroundFloor = 15

// maxLaneWait bounds how long a background caller will sit waiting for a reset,
// so a bogus or far-future reset header cannot stall the sync worker forever.
const maxLaneWait = 65 * time.Second

// lane distinguishes work a human is waiting on from work that can be deferred.
// It rides on the context so the ~25 client methods need no signature change:
// the sync worker marks its context Background once, and everything it does
// inherits the lower priority.
type lane int

const (
	laneInteractive lane = iota // a human (or an agent acting for one) is waiting
	laneBackground              // refresher, sync worker, outbox drain
)

type laneKey struct{}

// Background marks ctx as deferrable work, which yields to interactive callers
// when the remaining rate budget gets thin.
func Background(ctx context.Context) context.Context {
	return context.WithValue(ctx, laneKey{}, laneBackground)
}

func laneOf(ctx context.Context) lane {
	if l, ok := ctx.Value(laneKey{}).(lane); ok {
		return l
	}
	return laneInteractive // unmarked callers are assumed to be user-facing
}

// Budget is one API key's live view of its rate limit.
type Budget struct {
	mu        sync.Mutex
	known     bool      // have we ever seen a rate-limit header?
	remaining int       // best estimate right now
	limit     int       // highest remaining ever seen ≈ the ceiling
	reset     time.Time // when remaining refills
	throttled int       // 429s observed
	waits     int       // times a background caller yielded
	spent     int       // requests issued since process start
}

var (
	budgetsMu sync.Mutex
	budgets   = map[string]*Budget{}
)

// budgetFor returns the shared Budget for a credential, creating it on first use.
// Keyed by base URL + key: the same key against two Plane deployments is two
// independent limits.
func budgetFor(baseURL, apiKey string) *Budget {
	if apiKey == "" {
		return nil
	}
	budgetsMu.Lock()
	defer budgetsMu.Unlock()
	k := baseURL + "\x00" + apiKey
	b, ok := budgets[k]
	if !ok {
		b = &Budget{remaining: -1}
		budgets[k] = b
	}
	return b
}

// budget is the Budget this client's requests draw from.
func (c *Client) budget() *Budget { return budgetFor(c.BaseURL, c.APIKey) }

// wait applies the reservation policy before a request goes out. Interactive
// callers are never delayed. A background caller yields until the reset when the
// remaining allowance has fallen to the floor, which is what stops the refresher
// from spending the last of the budget a human's page load needs.
func (b *Budget) wait(ctx context.Context, ln lane) error {
	if b == nil || ln == laneInteractive {
		return nil
	}
	b.mu.Lock()
	if !b.known || b.remaining > backgroundFloor {
		b.mu.Unlock()
		return nil
	}
	d := time.Until(b.reset)
	if d <= 0 {
		// The window has already rolled over; the next response will tell us the
		// real figure. Proceed rather than guess.
		b.mu.Unlock()
		return nil
	}
	if d > maxLaneWait {
		d = maxLaneWait
	}
	b.waits++
	b.mu.Unlock()

	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// begin records that a request is going out, decrementing the local estimate so
// that concurrent in-flight requests do not all act on the same stale figure.
// observe corrects it from the authoritative header when the response lands.
func (b *Budget) begin() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.spent++
	if b.known && b.remaining > 0 {
		b.remaining--
	}
	b.mu.Unlock()
}

// observe snaps the budget to what Plane actually reported.
func (b *Budget) observe(h http.Header, status int) {
	if b == nil {
		return
	}
	rem, okRem := atoiHeader(h, "X-RateLimit-Remaining")
	rst, okRst := atoiHeader(h, "X-RateLimit-Reset")

	b.mu.Lock()
	defer b.mu.Unlock()
	if status == http.StatusTooManyRequests {
		b.throttled++
	}
	if okRem {
		b.known = true
		b.remaining = rem
		// Plane does not send the ceiling, so infer it: the largest remaining we
		// have ever seen is the top of the window.
		if rem > b.limit {
			b.limit = rem
		}
	}
	if okRst {
		b.reset = time.Unix(int64(rst), 0)
	}
}

// resetIn reports how long until the window rolls over (0 when unknown/past).
func (b *Budget) resetIn() time.Duration {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reset.IsZero() {
		return 0
	}
	if d := time.Until(b.reset); d > 0 {
		return d
	}
	return 0
}

func atoiHeader(h http.Header, name string) (int, bool) {
	v := strings.TrimSpace(h.Get(name))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// BudgetStat is a read-only view of one key's rate budget, for /api/admin/stats.
type BudgetStat struct {
	Instance  string `json:"instance"`
	Known     bool   `json:"known"`     // false until a response carried the headers
	Remaining int    `json:"remaining"` // -1 when unknown
	Limit     int    `json:"limit"`     // inferred ceiling (largest remaining seen)
	ResetIn   int    `json:"reset_in"`  // seconds until the window rolls over
	Throttled int    `json:"throttled"` // 429s observed since start
	Waits     int    `json:"waits"`     // times background work yielded to the floor
	Spent     int    `json:"spent"`     // requests issued since start
	Floor     int    `json:"floor"`     // the reserved interactive slice
}

// BudgetFor reports the live budget for a credential. Returns Known=false when
// nothing has been observed yet (no calls made, or a Plane build that omits the
// headers — in which case the reservation policy is simply inert).
func BudgetFor(baseURL, apiKey string) BudgetStat {
	b := budgetFor(baseURL, apiKey)
	if b == nil {
		return BudgetStat{Remaining: -1, Floor: backgroundFloor}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := BudgetStat{
		Known: b.known, Remaining: b.remaining, Limit: b.limit,
		Throttled: b.throttled, Waits: b.waits, Spent: b.spent,
		Floor: backgroundFloor,
	}
	if !b.reset.IsZero() {
		if d := time.Until(b.reset); d > 0 {
			st.ResetIn = int(d.Seconds() + 0.5)
		}
	}
	return st
}
