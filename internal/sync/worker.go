package sync

import (
	"context"
	"log"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

const (
	// DeltaInterval is how often the mirror pulls what changed. In steady state
	// this is ONE page per project, so it can be frequent without being costly.
	DeltaInterval = 2 * time.Minute

	// FullInterval is how often a complete reconcile runs. This is not a cost
	// knob but a CORRECTNESS one: deletes and module-membership moves are
	// invisible to the delta, so an hour is also the worst-case lag on both.
	FullInterval = time.Hour
)

// OnChange registers a callback fired after a pass that actually changed
// something. Before Phase 2 the board was rebuilt on a fixed timer because
// rebuilding meant re-fetching Plane; now it is a handful of SQLite queries, so
// it can simply happen when there is something new to show. Fixed-interval
// polling of a local file is latency for no reason.
func (s *Syncer) OnChange(fn func(slug string)) { s.onChange = fn }

// Run keeps every configured instance's mirror current until ctx is done.
//
// Everything here is background work in the Phase 0 sense: when the rate budget
// runs thin the syncer waits rather than competing with a page load. That is the
// whole reason this can run alongside the legacy fetch path during the shadow
// period without making today worse.
func (s *Syncer) Run(ctx context.Context, instances func() ([]domain.Instance, error)) {
	ctx = plane.Background(ctx)

	// First pass immediately, so a fresh deployment has a mirror to diff.
	s.pass(ctx, instances)

	t := time.NewTicker(DeltaInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.pass(ctx, instances)
		}
	}
}

// pass syncs every instance once, choosing full vs delta per instance based on
// when that instance last had a complete walk.
func (s *Syncer) pass(ctx context.Context, instances func() ([]domain.Instance, error)) {
	all, err := instances()
	if err != nil {
		log.Printf("sync: list instances: %v", err)
		return
	}
	for _, inst := range all {
		if ctx.Err() != nil {
			return
		}
		if !plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Configured() {
			continue
		}
		res, err := s.Once(ctx, inst)
		if err != nil {
			log.Printf("sync: %s: %v", inst.Slug, err)
			continue
		}
		if res.Partial {
			// Under rate pressure a pass can spend many minutes inside the Phase 0
			// waits. Say so: a syncer that is patiently yielding and a syncer that
			// is wedged look identical from the outside otherwise.
			log.Printf("sync: %s made partial progress (%d item(s)); the rest waits for budget",
				inst.Slug, res.Items)
		}
		// Only speak up when something actually moved; a quiet delta is the
		// normal case and should not fill the log.
		if res.Items > 0 || res.Comments > 0 || res.Pruned > 0 || res.Full {
			log.Printf("sync: %s", res)
		}
		// Nudge the board only when the mirror actually moved.
		if s.onChange != nil && (res.Items > 0 || res.Pruned > 0 || res.Full) {
			s.onChange(inst.Slug)
		}
	}
}

// Once syncs one instance, picking a full reconcile when the last one has aged
// out (or never happened) and a delta otherwise.
func (s *Syncer) Once(ctx context.Context, inst domain.Instance) (Result, error) {
	cur, err := s.m.Cursor(inst.Slug, resourceItems)
	if err != nil {
		return Result{Instance: inst.Slug}, err
	}
	due := cur.LastFull.IsZero() || s.now().Sub(cur.LastFull) >= FullInterval
	var res Result
	if due {
		res, err = s.reconcile(ctx, inst)
	} else {
		res, err = s.Delta(ctx, inst)
	}
	if err != nil {
		// Record the failure on the cursor so the admin surface can show that a
		// mirror is stale AND why, rather than silently serving old rows.
		cur.LastError = err.Error()
		if setErr := s.m.SetCursor(inst.Slug, resourceItems, cur); setErr != nil {
			log.Printf("sync: %s: recording error on cursor: %v", inst.Slug, setErr)
		}
	}
	return res, err
}
