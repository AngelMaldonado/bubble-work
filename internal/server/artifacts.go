package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// Editing a thread's artifacts (docs/MCP-ACCESS.md steps 1-2).
//
// Until this existed the MCP surface could create a thread and comment on it and
// change nothing else — which, since THREAD-LIFECYCLE.md treats a Logbook edit as
// the primary evidence of production, made an agent structurally incapable of
// warming a thread it was working on.

// UpdateThread rewrites parts of a thread's artifact page.
//
// It is SECTION-SCOPED on purpose. The Brief and the Logbook share one Plane
// description (§3: one page, not a second tracker), so a whole-body write is the
// obvious shape and the wrong one: the Brief is the human's statement of intent,
// and an agent revising a plan should not be able to erase it. A nil field is
// left exactly as it was.
func (s *Server) UpdateThread(ctx context.Context, threadID string, brief, logbook *string) (domain.ThreadDetail, error) {
	if brief == nil && logbook == nil {
		return domain.ThreadDetail{}, fmt.Errorf("%w: nothing to update", errBadRequest)
	}
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	_, inst, _, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if s.mirror == nil {
		return domain.ThreadDetail{}, fmt.Errorf("mirror unavailable")
	}
	it, ok, err := s.mirror.Item(inst.Slug, wid)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if !ok {
		return domain.ThreadDetail{}, errNotFound
	}

	body := md.FromHTML(it.DescriptionHTML)
	if brief != nil {
		body = md.ReplaceSection(body, "Brief", *brief)
	}
	if logbook != nil {
		body = md.ReplaceSection(body, "Logbook", *logbook)
	}
	html := md.RenderHTML(body)

	// Written with the caller's own key, so Plane attributes the edit to the
	// person (or the human an agent is impersonating), exactly like a comment.
	wcl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	updatedAt, err := wcl.SetWorkItemBody(ctx, wid, html)
	if err != nil {
		return domain.ThreadDetail{}, err
	}

	// Write it into the mirror NOW rather than waiting for a delta pass to
	// rediscover our own write. Not optimistic: Plane accepted it, so this is
	// what Plane holds. Without this the board would be up to a delta interval
	// behind a change the server itself just made.
	it.DescriptionHTML = html
	it.DescriptionHash = mirror.HashBody(html)
	if !updatedAt.IsZero() {
		it.UpdatedAt = updatedAt
	}
	if err := s.mirror.UpsertItems(inst.Slug, []mirror.Item{it}, s.now()); err != nil {
		log.Printf("mirror: record thread update %s: %v", wid, err)
	}
	// A Logbook change is production (§5.1). Rebuilding the snapshot is what
	// turns it into heat and pushes it to every open board.
	s.rebuildAndNotify(inst, wid)

	return s.ThreadDetail(ctx, full)
}

// AddRevision hangs a revision artifact off a thread. A revision IS a
// sub-work-item (INTERIOR-PLAN.md), and landing one is the other kind of
// production evidence besides a Logbook edit.
func (s *Server) AddRevision(ctx context.Context, threadID, title, body string) (domain.ThreadDetail, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return domain.ThreadDetail{}, fmt.Errorf("%w: a revision needs a title", errBadRequest)
	}
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	_, inst, _, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if s.mirror == nil {
		return domain.ThreadDetail{}, fmt.Errorf("mirror unavailable")
	}

	// "rev:" is the prefix the interior strips when labelling revisions, so
	// naming it here keeps the convention in one place.
	name := title
	if !strings.HasPrefix(strings.ToLower(name), "rev:") {
		name = "rev: " + name
	}
	html := md.RenderHTML(body)

	wcl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	state, err := wcl.DefaultState(ctx)
	if err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("resolving a state for the revision: %w", err)
	}
	childID, err := wcl.CreateChildWorkItem(ctx, name, html, state, wid)
	if err != nil {
		return domain.ThreadDetail{}, err
	}

	// Same reasoning as UpdateThread: put it in the mirror now. A revision that
	// took a delta interval to appear would make "add a revision and watch it
	// land" not work, which is the whole point.
	now := s.now()
	if err := s.mirror.UpsertItems(inst.Slug, []mirror.Item{{
		ID: childID, ProjectID: projID, Name: name, ParentID: wid,
		DescriptionHTML: html, DescriptionHash: mirror.HashBody(html),
		CreatedAt: now, UpdatedAt: now,
	}}, now); err != nil {
		log.Printf("mirror: record revision %s: %v", childID, err)
	}
	s.rebuildAndNotify(inst, wid)

	return s.ThreadDetail(ctx, full)
}

// rebuildAndNotify recomputes the instance snapshot from the mirror and nudges
// every open client. Cheap since Phase 2 — a handful of SQLite queries and no
// network — which is what makes it reasonable to do on a write path at all.
func (s *Server) rebuildAndNotify(inst domain.Instance, threadID string) {
	if _, err := s.refreshInstance(context.Background(), inst); err != nil {
		log.Printf("rebuild after artifact write: %v", err)
		return
	}
	s.broadcastThread(threadID)
}

func (s *Server) handleUpdateThread(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Brief   *string `json:"brief,omitempty"`
		Logbook *string `json:"logbook,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	d, err := s.UpdateThread(r.Context(), r.PathValue("id"), in.Brief, in.Logbook)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleAddRevision(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	d, err := s.AddRevision(r.Context(), r.PathValue("id"), in.Title, in.Body)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, d)
}
