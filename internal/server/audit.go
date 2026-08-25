package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// Auditing a bubble in one call.
//
// The framework asks the same four questions of every thread — is it born, what is
// its finish line, how far along is it, what happens next — and answering them used
// to cost one read per thread plus the timeline. A caller with a ten-thread bubble
// made eleven calls to learn what the model already knows.
//
// Everything here is local: the mirror for bodies and structure, the overlay for
// evidence and types, the pure heat function for levels. It spends no Plane calls, so
// it is as cheap to run on every thread as on one.
//
// It deliberately reports what is MISSING as prominently as what is done. A bubble's
// real state is usually "three threads fine, one never got a Definition of Done", and
// that last clause is the one nobody finds by reading threads one at a time.
//
// Missing is an OBSERVATION, not a violation (docs/decisions/0005). Nothing here was
// ever required of the author; what the audit says is that a thread with no finish
// line is harder to finish, which is worth knowing across ten threads at once.
func (s *Server) AuditBubble(ctx context.Context, bubbleID string) (domain.BubbleAudit, error) {
	full, err := s.resolveID(ctx, bubbleID)
	if err != nil {
		return domain.BubbleAudit{}, err
	}
	bubbles, err := s.collect(ctx)
	if err != nil {
		return domain.BubbleAudit{}, err
	}
	var b *domain.Bubble
	for i := range bubbles {
		if bubbles[i].ID == full {
			b = &bubbles[i]
			break
		}
	}
	if b == nil {
		return domain.BubbleAudit{}, errNotFound
	}
	slug, projID, _ := cut3(b.ID)

	now, tun := s.now(), s.Tuning()
	res, level, buoy := s.bubbleHeat(*b, tun, now)

	out := domain.BubbleAudit{
		BubbleID: b.ID, Name: b.Name, Instance: b.Instance,
		ProjectName: b.ProjectName, Level: level, Lifecycle: string(res.Lifecycle),
		Reason: res.Reason, Outcome: b.Outcome, Owner: b.Owner, Closure: b.Closure,
		Closed: b.Closed, Counts: map[string]int{},
	}

	// Latest progress per thread, so the audit can say when each one last moved.
	latest := map[string]time.Time{}
	for _, e := range b.Evidence {
		if e.ThreadID == "" || !e.Progress() {
			continue
		}
		if e.At.After(latest[e.ThreadID]) {
			latest[e.ThreadID] = e.At
		}
	}

	for _, th := range b.Threads {
		if th.Parent != "" {
			continue // a revision is part of its parent, not a thread of the bubble
		}
		ta := domain.ThreadAudit{
			ID: slug + ":" + projID + ":" + th.ID, Seq: th.Seq, Title: th.Name,
			Owner: th.Owner, Level: buoy[th.ID].Level, Reason: buoy[th.ID].Reason,
			State: th.State, StateGroup: th.StateGroup,
		}
		if at, ok := latest[th.ID]; ok {
			t := at
			ta.LastProgressAt = &t
		}

		// The body, read the way every other read path reads it: the document store
		// when it has one, the mirrored HTML otherwise.
		body := ""
		if _, stored, ok := s.storedBody(th.ID); ok {
			body = stored
		} else if it, ok, err := s.mirror.Item(slug, th.ID); err == nil && ok {
			body = md.FromHTML(it.DescriptionHTML)
		}

		// The two addressable sections come out first; the DOCUMENT is whatever is
		// left, whatever headings its author used. This is the same order ParseThread
		// uses, for the same reason.
		logbook, rest, hasLog := md.ExtractSection(body, "logbook")
		dod, rest2, hasDoD := md.ExtractSection(rest, "definition of done", "dod")
		doc := strings.TrimSpace(rest2)

		ta.HasBrief = doc != ""
		ta.HasDoD = hasDoD
		ta.HasLogbook = hasLog
		ta.Next = md.NextAction(logbook)

		for _, td := range md.ParseTodos(dod) {
			ta.DoDTotal++
			if td.Done {
				ta.DoDDone++
			} else {
				ta.Unmet = append(ta.Unmet, td.Text)
			}
		}
		for _, td := range md.ParseTodos(logbook) {
			ta.TodosTotal++
			if td.Done {
				ta.TodosDone++
			}
		}
		// Checkboxes written in the document count too: a plan under somebody's own
		// headings is a plan (docs/decisions/0005). Checkbox-only there, because a
		// prose bullet is a sentence rather than a task.
		for _, td := range md.ParseChecklist(doc) {
			ta.TodosTotal++
			if td.Done {
				ta.TodosDone++
			}
		}

		// The one question worth asking of every thread at once: could somebody else
		// tell whether this is finished, and could they pick it up? Neither is
		// required — this names what would make the thread legible, in the order
		// that matters.
		switch {
		case !ta.HasBrief:
			ta.Missing = "a document — this thread is a title and nothing else"
		case !ta.HasDoD || ta.DoDTotal == 0:
			ta.Missing = "a finish line — no Definition of Done items to check against"
		case ta.TodosTotal == 0 && ta.Level != "done":
			ta.Missing = "checklist items — nothing here can be ticked off"
		case ta.Next == "" && ta.Level != "done":
			ta.Missing = "a **Next:** line saying the single next action"
		}
		if ta.Missing != "" {
			out.NeedsRepair++
		}
		out.Counts[ta.Level]++
		out.Threads = append(out.Threads, ta)
	}
	return out, nil
}

func (s *Server) handleAuditBubble(w http.ResponseWriter, r *http.Request) {
	a, err := s.AuditBubble(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a)
}
