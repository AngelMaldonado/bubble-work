package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// Finishing a thread (docs/THREAD-LIFECYCLE.md).
//
// 🏆 is derived from Plane: a work item in a `completed` state, or one Plane says
// is no longer active. Nothing in Bubble Work could put it there — autostate
// refuses on purpose (autoTarget returns "" for done, "hands off") — so the one
// lifecycle transition the framework asks you to make was the one you had to
// leave the framework to make.
//
// That is backwards for the part of the model that cares most. A thread carries a
// **Definition of Done** stating exactly when it is finished, and its todos are
// how you verify it; you could tick the last box and still have to go find the
// card in Plane. An agent that had just satisfied the DoD had no way to say so at
// all.
//
// So completing is a first-class verb here, with one rule attached: the DoD is
// the authority on "done", not the todo count and not the person's mood. A thread
// whose Definition of Done still has unticked items is refused, and the refusal
// names them. That is the closing counterpart of the §3 birth rule — the same
// artifact gates both ends of a thread's life.
//
// Note what is NOT here: no overlay flag, no second source of truth. This writes
// Plane's state and lets the ordinary derivation report it, so a thread completed
// from Plane's own UI and one completed here are indistinguishable afterwards —
// which is the point of Plane being the system of record.

// UnmetDoD is returned when a thread is asked to finish with its Definition of
// Done unmet. It carries the outstanding items so the caller can show them
// instead of a bare refusal.
type UnmetDoD struct{ Items []string }

func (e *UnmetDoD) Error() string {
	if len(e.Items) == 1 {
		return fmt.Sprintf("%s: the Definition of Done is not met yet — 1 item outstanding: %q",
			errBadRequest, e.Items[0])
	}
	return fmt.Sprintf("%s: the Definition of Done is not met yet — %d items outstanding, starting with %q",
		errBadRequest, len(e.Items), e.Items[0])
}

// Unwrap makes it a bad request, so every surface maps it to 400 without knowing
// about this type.
func (e *UnmetDoD) Unwrap() error { return errBadRequest }

// CompleteThread moves a thread into its project's completed state, which is what
// makes it 🏆 everywhere.
//
// force skips the Definition of Done check. It exists because a DoD can be
// genuinely wrong — the work turned out to be something else, or a checklist item
// describes a thing that will never happen — and refusing forever would just
// teach people to keep threads open. It is a deliberate override, logged as one,
// never the default on any surface.
func (s *Server) CompleteThread(ctx context.Context, threadID string, force bool) (domain.ThreadDetail, error) {
	return s.moveThreadState(ctx, threadID, "completed", force)
}

// ReopenThread moves a finished thread back into `started`, for the mis-click and
// for work that turned out not to be done.
//
// A completing verb without its inverse is a trap: the only way back would be
// Plane's UI, which is exactly the trip this set out to remove.
func (s *Server) ReopenThread(ctx context.Context, threadID string) (domain.ThreadDetail, error) {
	return s.moveThreadState(ctx, threadID, "started", true)
}

// moveThreadState is the one path both verbs share: resolve the target state by
// GROUP, write it to Plane, and write through to the mirror so every surface
// reflects it now rather than after the next sync.
func (s *Server) moveThreadState(ctx context.Context, threadID, group string, force bool) (domain.ThreadDetail, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	cl, inst, slug, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if s.mirror == nil {
		return domain.ThreadDetail{}, fmt.Errorf("mirror unavailable")
	}
	it, ok, err := s.mirror.Item(slug, wid)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if !ok {
		return domain.ThreadDetail{}, errNotFound
	}

	// The DoD gate, on the way in only: reopening something is never blocked by a
	// checklist, and force is the explicit override.
	if group == "completed" && !force {
		if unmet := unmetDoD(it.DescriptionHTML); len(unmet) > 0 {
			return domain.ThreadDetail{}, &UnmetDoD{Items: unmet}
		}
	}

	// Resolved by GROUP, never by name: state names are project-configured and
	// localized, so "Done" is not a thing to match on. A project with no state in
	// the group cannot be served — and saying so is better than moving the card
	// somewhere arbitrary.
	states, err := s.mirrorStates(slug, projID)
	if err != nil || len(states) == 0 {
		return domain.ThreadDetail{}, fmt.Errorf(
			"%w: this project's states are not mirrored yet — try again in a moment", errBadRequest)
	}
	target, found := defaultStateOf(states, group)
	if !found {
		return domain.ThreadDetail{}, fmt.Errorf(
			"%w: this project has no %s state in Plane, so a thread cannot be moved there",
			errBadRequest, group)
	}
	if it.StateID == target.ID {
		return s.ThreadDetail(ctx, full) // already there; not an error, just nothing to do
	}

	queued := false
	if err := cl.SetWorkItemState(ctx, wid, target.ID); err != nil {
		// Queue rather than fail. The write is small, idempotent and worth
		// retrying, and the field lock stops an incoming sync reverting the mirror
		// to Plane's older state while it is still in flight
		// (docs/PLANE-SYNC.md Phase 5).
		if _, qerr := s.store.Enqueue(store.OutboxEntry{
			Instance: slug, Kind: store.OutState, TargetID: wid,
			Payload:   map[string]string{"state_id": target.ID},
			FieldLock: "state", CreatedAt: s.now(), LastError: err.Error(),
		}); qerr != nil {
			return domain.ThreadDetail{}, fmt.Errorf("set state: %w", err)
		}
		queued = true
	}

	// A human moved this card. If autostate is managing the instance it must back
	// off for good rather than argue with the declaration on its next pass — the
	// rule already stated for a move made in Plane's UI, applied to a move made
	// through here.
	if err := s.store.HandOffAutoState(wid); err != nil {
		log.Printf("autostate: hand-off %s: %v", wid, err)
	}

	it.StateID = target.ID
	it.StateName = target.Name
	it.StateGroup = target.Group
	// CompletedAt is what heat.ClassifyThread reads first, so it has to move with
	// the state or the thread would sit in a Done column still reading 🔥.
	if group == "completed" {
		now := s.now()
		it.CompletedAt = &now
	} else {
		it.CompletedAt = nil
	}
	if err := s.mirror.UpsertItems(slug, []mirror.Item{it}, s.now()); err != nil {
		log.Printf("mirror: record state move %s: %v", wid, err)
	}

	// The board's bands are derived from thread levels, so this one needs a real
	// rebuild — unlike a rename, it changes where the bubble floats.
	s.dropInstanceCache(slug)
	if _, err := s.refreshInstance(context.Background(), inst); err != nil {
		log.Printf("rebuild after state move: %v", err)
	}
	s.broadcastThread(wid)

	actor, _ := domain.ActorFrom(ctx)
	verb := "complete_thread"
	if group != "completed" {
		verb = "reopen_thread"
	}
	extra := ""
	if force {
		extra = " (forced past the Definition of Done)"
	}
	if queued {
		extra += " (queued — Plane did not accept it yet)"
	}
	log.Printf("%s by %s: %s → %q [%s]%s", verb, actor.Label(), wid, target.Name, group, extra)

	return s.ThreadDetail(ctx, full)
}

// unmetDoD returns the unticked Definition of Done items in a thread body.
//
// Read from the DoD section only. The Logbook's own todos are the plan, and a
// plan can legitimately carry items that outlive the thread — "monitor for a
// week" — whereas the DoD is the promise about when this is finished. Blocking on
// the Logbook would make the gate unpassable in practice, which is how a rule
// gets routed around instead of followed.
func unmetDoD(descriptionHTML string) []string {
	body := md.FromHTML(descriptionHTML)
	dod, _, found := md.ExtractSection(body, "definition of done", "dod")
	if !found {
		// No DoD at all is not a gate. A thread born small is allowed to close on
		// a paragraph (AGENTS.md), and refusing here would punish exactly the
		// threads the rule exempts.
		return nil
	}
	var out []string
	for _, t := range md.ParseTodos(dod) {
		if !t.Done {
			out = append(out, t.Text)
		}
	}
	return out
}

// ---- REST ----

func (s *Server) handleCompleteThread(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Force bool `json:"force"`
	}
	// A body is optional: POST with nothing means "complete it, respecting the DoD".
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	d, err := s.CompleteThread(r.Context(), r.PathValue("id"), in.Force)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleReopenThread(w http.ResponseWriter, r *http.Request) {
	d, err := s.ReopenThread(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}
