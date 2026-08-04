package plane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newBudgetSrv serves a work-item-ish payload and reports a rate budget that
// counts down from `start`, the way Plane does.
func newBudgetSrv(t *testing.T, start int, resetIn time.Duration) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		rem := start - int(n)
		if rem < 0 {
			rem = 0
		}
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprint(rem))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(resetIn).Unix()))
		if rem == 0 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestBudgetLearnsFromHeaders(t *testing.T) {
	srv, _ := newBudgetSrv(t, 60, time.Minute)
	c := New(srv.URL, "k-learn", "ws", "")

	var out map[string]any
	if err := c.get(context.Background(), "/x", &out); err != nil {
		t.Fatalf("get: %v", err)
	}

	b := BudgetFor(srv.URL, "k-learn")
	if !b.Known {
		t.Fatal("budget still unknown after a response carrying the headers")
	}
	if b.Remaining != 59 {
		t.Errorf("remaining = %d, want 59", b.Remaining)
	}
	if b.ResetIn <= 0 || b.ResetIn > 61 {
		t.Errorf("reset_in = %d, want a value inside the window", b.ResetIn)
	}
	if b.Spent != 1 {
		t.Errorf("spent = %d, want 1", b.Spent)
	}
}

// The whole point of Phase 0: when the allowance runs thin, background work
// stands aside and a human's call still goes through.
func TestBackgroundYieldsInteractiveDoesNot(t *testing.T) {
	// Start just above the floor so a single call drops us to it.
	srv, hits := newBudgetSrv(t, backgroundFloor+1, 30*time.Second)
	c := New(srv.URL, "k-lane", "ws", "")

	var out map[string]any
	if err := c.get(context.Background(), "/warm", &out); err != nil {
		t.Fatalf("warm-up: %v", err)
	}
	if got := BudgetFor(srv.URL, "k-lane").Remaining; got != backgroundFloor {
		t.Fatalf("remaining = %d, want %d (the floor)", got, backgroundFloor)
	}
	before := atomic.LoadInt64(hits)

	// Background: must NOT reach the server before the window resets. A short
	// deadline stands in for "the reset is far away".
	bg, cancel := context.WithTimeout(bgctx(), 150*time.Millisecond)
	defer cancel()
	err := c.get(bg, "/deferred", &out)
	if err == nil {
		t.Error("background call went through at the floor; it should have yielded")
	}
	if got := atomic.LoadInt64(hits) - before; got != 0 {
		t.Errorf("background made %d request(s) at the floor, want 0", got)
	}

	// Interactive: must go through regardless.
	if err := c.get(context.Background(), "/urgent", &out); err != nil {
		t.Fatalf("interactive call was blocked at the floor: %v", err)
	}
	if got := atomic.LoadInt64(hits) - before; got != 1 {
		t.Errorf("interactive made %d request(s), want 1", got)
	}
	if w := BudgetFor(srv.URL, "k-lane").Waits; w == 0 {
		t.Error("no yield recorded, so the wait was not attributed to the floor")
	}
}

// A 429 with a known reset should wait for the window rather than retry into a
// wall — retrying sooner is guaranteed to fail and burns more allowance.
func TestBackoffPrefersResetOver429(t *testing.T) {
	b := &Budget{known: true, remaining: 0, reset: time.Now().Add(3 * time.Second)}
	err := &APIError{Status: http.StatusTooManyRequests, Path: "GET /x"}

	got := backoffFor(err, b, 1)
	if got < 2*time.Second || got > 4*time.Second {
		t.Errorf("backoff = %v, want ~3s (wait for the reset)", got)
	}

	// Retry-After still wins when Plane sends one.
	err2 := &APIError{Status: http.StatusTooManyRequests, retryAfter: 900 * time.Millisecond}
	if got := backoffFor(err2, b, 1); got != 900*time.Millisecond {
		t.Errorf("backoff = %v, want Retry-After to win", got)
	}

	// A non-429 falls back to exponential.
	if got := backoffFor(&APIError{Status: 500}, b, 1); got != 400*time.Millisecond {
		t.Errorf("backoff = %v, want the 400ms exponential step", got)
	}
}

// A Plane build that omits the headers must leave the policy inert rather than
// blocking every background call forever.
func TestNoHeadersMeansNoThrottling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "k-bare", "ws", "")

	var out map[string]any
	for i := 0; i < 5; i++ {
		if err := c.get(bgctx(), "/x", &out); err != nil {
			t.Fatalf("call %d blocked with no rate headers present: %v", i, err)
		}
	}
	if b := BudgetFor(srv.URL, "k-bare"); b.Known {
		t.Error("budget claims to be known despite no headers ever arriving")
	}
}

// The limit is per API key, so two keys must not share a budget.
func TestBudgetIsPerKey(t *testing.T) {
	srv, _ := newBudgetSrv(t, 60, time.Minute)
	var out map[string]any
	a := New(srv.URL, "k-a", "ws", "")
	b := New(srv.URL, "k-b", "ws", "")

	for i := 0; i < 3; i++ {
		if err := a.get(context.Background(), "/x", &out); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.get(context.Background(), "/x", &out); err != nil {
		t.Fatal(err)
	}
	if got := BudgetFor(srv.URL, "k-a").Spent; got != 3 {
		t.Errorf("key a spent = %d, want 3", got)
	}
	if got := BudgetFor(srv.URL, "k-b").Spent; got != 1 {
		t.Errorf("key b spent = %d, want 1 (budgets leaked across keys)", got)
	}
}

// bgctx is a context marked as deferrable background work.
func bgctx() context.Context { return Background(context.Background()) }
