package server

import (
	"context"
	"log"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// The outbox drainer (docs/journal/PLANE-SYNC.md Phase 5).
//
// It drains ONLY the kinds that can be performed without a person present: auto-
// state moves, and published artifact bodies (docs/decisions/0001). Both use the
// instance key the server already holds.
//
// Comment drafts are deliberately not here: they carry no credential, because
// storing every user's Plane key to replay later would be a worse problem than the
// one it solves. Their author re-sends them.
//
// A body is different from a comment in the way that matters — the markdown is
// already safe in thread_docs, so what is queued is a PUBLICATION to retry, not
// words that could be lost. That is why it may drain unattended.

const (
	// drainInterval is how often the queue is swept. Frequent is fine — an empty
	// queue costs one indexed SELECT and no Plane traffic at all.
	drainInterval = 60 * time.Second

	// drainBatch bounds how many entries one sweep sends, so a queue that built
	// up during a Plane outage drains steadily instead of as a thundering herd
	// the moment Plane returns.
	drainBatch = 10

	// outboxMaxAttempts is when an entry is abandoned. It stops being retried
	// and stops locking its field — Plane's own value becomes truth again — but
	// the row stays visible, because silently dropping a write is the one thing
	// an outbox must never do.
	outboxMaxAttempts = 8
)

// outboxBackoff spaces retries out, capped so a long-dead Plane is still
// retried about hourly rather than drifting into never.
func outboxBackoff(attempts int) time.Duration {
	d := time.Minute << attempts // 1m, 2m, 4m, ...
	if d > time.Hour {
		d = time.Hour
	}
	return d
}

// RunOutbox drains the queue until ctx is done.
func (s *Server) RunOutbox(ctx context.Context) {
	// Background priority: a retry of something that already failed must never
	// take the allowance a person's page load needs (Phase 0).
	ctx = plane.Background(ctx)
	t := time.NewTicker(drainInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.drainOutbox(ctx)
		}
	}
}

// drainOutbox sends one batch of due entries, per kind.
func (s *Server) drainOutbox(ctx context.Context) {
	s.drainKind(ctx, store.OutState, func(ctx context.Context, cl *plane.Client, e store.OutboxEntry) error {
		if err := cl.SetWorkItemState(ctx, e.TargetID, e.StateID()); err != nil {
			return err
		}
		if err := s.store.RecordAutoState(e.TargetID, e.StateID(), s.now()); err != nil {
			log.Printf("outbox: record provenance %s: %v", e.TargetID, err)
		}
		return nil
	})
	// Publishing a body (docs/decisions/0001). Recording the published hash is not
	// bookkeeping: it is what stops the next sync from reading our own publication
	// as somebody editing in Plane.
	s.drainKind(ctx, store.OutDoc, func(ctx context.Context, cl *plane.Client, e store.OutboxEntry) error {
		html := e.HTML()
		if _, err := cl.SetWorkItemBody(ctx, e.TargetID, html); err != nil {
			return err
		}
		return s.store.SetPublished(e.TargetID, mirror.HashBody(html), s.now())
	})
}

// drainKind sweeps one kind of queued write. The shape is identical for every kind
// — resolve the instance, send, back off or complete — so only the send differs.
func (s *Server) drainKind(ctx context.Context, kind string, send func(context.Context, *plane.Client, store.OutboxEntry) error) {
	now := s.now()
	due, err := s.store.DueOutbox(kind, now, drainBatch)
	if err != nil {
		log.Printf("outbox: load due %s: %v", kind, err)
		return
	}
	if len(due) == 0 {
		return
	}
	insts, err := s.store.ListInstances()
	if err != nil {
		log.Printf("outbox: list instances: %v", err)
		return
	}
	byslug := make(map[string]domain.Instance, len(insts))
	for _, i := range insts {
		byslug[i.Slug] = i
	}

	sent, failed := 0, 0
	for _, e := range due {
		inst, ok := byslug[e.Instance]
		if !ok {
			// The instance was removed while this sat in the queue. There is
			// nowhere to send it and never will be, so it goes.
			log.Printf("outbox: dropping %d — instance %s no longer exists", e.ID, e.Instance)
			if _, derr := s.store.DiscardOutbox(e.ID); derr != nil {
				log.Printf("outbox: discard %d: %v", e.ID, derr)
			}
			continue
		}
		projID := s.projectOf(inst.Slug, e.TargetID)
		cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, projID)
		if err := send(ctx, cl, e); err != nil {
			failed++
			next := now.Add(outboxBackoff(e.Attempts))
			if ferr := s.store.FailOutbox(e.ID, next, outboxMaxAttempts, err.Error()); ferr != nil {
				log.Printf("outbox: record failure %d: %v", e.ID, ferr)
			}
			continue
		}
		sent++
		if err := s.store.CompleteOutbox(e.ID); err != nil {
			log.Printf("outbox: complete %d: %v", e.ID, err)
		}
	}
	if sent > 0 || failed > 0 {
		log.Printf("outbox: drained %d %s, %d still failing", sent, kind, failed)
	}
}

// projectOf finds the project a mirrored work item belongs to, since a state
// write needs a project-scoped client and the queue only stores the item id.
func (s *Server) projectOf(slug, itemID string) string {
	if s.mirror == nil {
		return ""
	}
	it, ok, err := s.mirror.Item(slug, itemID)
	if err != nil || !ok {
		return ""
	}
	return it.ProjectID
}
