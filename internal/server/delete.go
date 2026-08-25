package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// Deleting things (docs/journal/ARTIFACT-EDITING.md).
//
// These are IRREVERSIBLE and they delete from Plane, which is the system of
// record. §5.3 says a bubble should normally die by being CLOSED — that keeps
// the record of what was done, and heat is evidence that work happened. This is
// for the things that should never have existed: a mistyped bubble, a test
// thread, a revision attached to the wrong parent.
//
// Every one of them writes through to the mirror and the overlay itself, for
// the same reason creates do: the sync worker is a reconciler, not the delivery
// mechanism, and a projection that still lists a deleted thread is the local
// copy being confidently wrong.

// DeleteBubble removes a bubble from Plane. Its threads SURVIVE — a Plane module
// is a grouping, not a container — and are left belonging to no bubble, which
// also means they leave the board, since the board is built from modules.
func (s *Server) DeleteBubble(ctx context.Context, bubbleID string) (int, error) {
	id, err := s.resolveID(ctx, bubbleID)
	if err != nil {
		return 0, err
	}
	slug, err := s.authorizeBubble(ctx, id)
	if err != nil {
		return 0, err
	}
	inst, ok, err := s.instanceBySlug(slug)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errNotFound
	}
	_, projID, moduleID := cut3(id)
	if projID == "" || moduleID == "" {
		return 0, fmt.Errorf("%w: malformed bubble id %q", errBadRequest, id)
	}

	// Counted BEFORE the delete, so the caller can be told what it cost.
	orphaned := 0
	if s.mirror != nil {
		if items, err := s.mirror.ModuleItems(slug, moduleID); err == nil {
			orphaned = len(items)
		}
	}

	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	if err := cl.DeleteModule(ctx, moduleID); err != nil {
		return 0, fmt.Errorf("delete module: %w", err)
	}

	if s.mirror != nil {
		if err := s.mirror.DeleteModule(slug, moduleID); err != nil {
			log.Printf("mirror: forget bubble %s: %v", id, err)
		}
	}
	if err := s.store.ForgetBubble(id); err != nil {
		log.Printf("overlay: forget bubble %s: %v", id, err)
	}

	s.dropInstanceCache(slug)
	if _, err := s.refreshInstance(context.Background(), inst); err != nil {
		log.Printf("rebuild after delete_bubble: %v", err)
	}
	s.broadcastThread("")

	actor, _ := domain.ActorFrom(ctx)
	log.Printf("delete_bubble by %s: %s (%d thread(s) unbubbled)", actor.Label(), id, orphaned)
	return orphaned, nil
}

// DeleteThread removes a thread from Plane, taking its Brief, Logbook, comments
// and revisions with it. A revision is itself a work item, so the same call
// deletes one.
//
// Children are deleted EXPLICITLY and first, rather than trusting Plane to
// cascade: whether it orphans or removes them is its business, and leaving that
// undecided would leave sub-items belonging to a parent that no longer exists.
func (s *Server) DeleteThread(ctx context.Context, threadID string) (int, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return 0, err
	}
	_, inst, slug, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return 0, err
	}
	if s.mirror == nil {
		return 0, fmt.Errorf("mirror unavailable")
	}
	if _, ok, err := s.mirror.Item(slug, wid); err != nil {
		return 0, err
	} else if !ok {
		return 0, errNotFound
	}

	gone := []string{wid}
	if kids, err := s.mirror.Children(slug, wid); err == nil {
		for _, k := range kids {
			gone = append(gone, k.ID)
		}
	}

	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	// Children first: deleting the parent while a child still points at it is
	// the ordering that can leave Plane inconsistent.
	for i := len(gone) - 1; i >= 0; i-- {
		if err := cl.DeleteWorkItem(ctx, gone[i]); err != nil {
			return 0, fmt.Errorf("delete work item %s: %w", gone[i], err)
		}
	}

	if err := s.mirror.DeleteItems(slug, gone); err != nil {
		log.Printf("mirror: forget threads %v: %v", gone, err)
	}
	if err := s.store.ForgetThreads(gone); err != nil {
		log.Printf("overlay: forget threads %v: %v", gone, err)
	}

	s.dropInstanceCache(slug)
	if _, err := s.refreshInstance(context.Background(), inst); err != nil {
		log.Printf("rebuild after delete_thread: %v", err)
	}
	s.broadcastThread("")

	actor, _ := domain.ActorFrom(ctx)
	log.Printf("delete_thread by %s: %s (with %d revision(s))", actor.Label(), full, len(gone)-1)
	return len(gone) - 1, nil
}

// DeleteRegion removes one artifact from a thread's page — the Logbook, the
// Definition of Done, or the document — heading and all.
//
// Unlike the other two this is not a Plane deletion: the thread lives on, minus
// a section. It goes through the same splice as any edit, so everything it does
// not touch keeps its original bytes.
func (s *Server) DeleteRegion(ctx context.Context, threadID string, region md.Region) (domain.ThreadDetail, error) {
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

	next, err := md.RemoveRegion(it.DescriptionHTML, region)
	if err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("%w: %v", errBadRequest, err)
	}
	if next == it.DescriptionHTML {
		return s.ThreadDetail(ctx, full) // already absent
	}

	wcl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	if err := s.writeBody(ctx, wcl, inst, it, next); err != nil {
		return domain.ThreadDetail{}, err
	}
	// Removing a Logbook is a change to the plan, so it rebuilds the board the
	// same way writing one does.
	s.rebuildAndNotify(inst, wid, region != md.RegionDocument)

	actor, _ := domain.ActorFrom(ctx)
	log.Printf("delete_region by %s: %s %s", actor.Label(), full, region)
	return s.ThreadDetail(ctx, full)
}

// ---- HTTP ----

func (s *Server) handleDeleteBubble(w http.ResponseWriter, r *http.Request) {
	n, err := s.DeleteBubble(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unbubbled_threads": n})
}

func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	n, err := s.DeleteThread(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted_revisions": n})
}

func (s *Server) handleDeleteRegion(w http.ResponseWriter, r *http.Request) {
	region, ok := regionByName(r.PathValue("region"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown region " + r.PathValue("region")})
		return
	}
	d, err := s.DeleteRegion(r.Context(), r.PathValue("id"), region)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}
