package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// Re-homing a thread (docs/ARTIFACT-EDITING.md).
//
// birth_thread put a thread in a bubble and nothing could ever change its mind,
// which made the first guess permanent — an odd property for a system whose
// whole point is that a bubble's outcome can stop justifying the work.
//
// A bubble is a Plane MODULE, and module membership is just a link: removing it
// leaves the work item, its Brief, its Logbook, its comments, its history and
// its id completely untouched. So a move is genuinely a move, not a copy and not
// a rebuild.

// MoveThread re-homes a thread into another bubble.
//
// It is a MOVE, so it leaves every other bubble too. Plane lets a work item
// belong to several modules at once and the board shows it in each, so removing
// only the one you named would quietly turn this into a copy.
func (s *Server) MoveThread(ctx context.Context, threadID, bubbleID string) (domain.ThreadDetail, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	_, inst, slug, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if s.mirror == nil {
		return domain.ThreadDetail{}, fmt.Errorf("mirror unavailable")
	}

	// The target is authorized in its own right: being able to see a thread says
	// nothing about being allowed to file it somewhere else.
	target, err := s.resolveID(ctx, bubbleID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if _, err := s.authorizeBubble(ctx, target); err != nil {
		return domain.ThreadDetail{}, err
	}
	tSlug, tProj, moduleID := cut3(target)
	if moduleID == "" {
		return domain.ThreadDetail{}, fmt.Errorf("%w: malformed bubble id %q", errBadRequest, bubbleID)
	}
	// A module can only hold work items of its own project, so this is Plane's
	// limit rather than ours — and saying so beats a confusing 400 from Plane.
	if tSlug != slug || tProj != projID {
		return domain.ThreadDetail{}, fmt.Errorf(
			"%w: a thread can only move between bubbles in the same workspace — %s lives in %s:%s",
			errBadRequest, full, slug, projID)
	}

	from, err := s.mirror.ModulesForItem(slug, wid)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	already := false
	var leaving []string
	for _, m := range from {
		if m == moduleID {
			already = true
			continue
		}
		leaving = append(leaving, m)
	}
	if already && len(leaving) == 0 {
		return s.ThreadDetail(ctx, full) // already exactly where it was asked to go
	}

	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)

	// Link BEFORE unlinking. If the second half fails the thread is in two
	// bubbles, which is visible and fixable; the other order would leave it in
	// none, which looks like the thread vanished.
	if !already {
		if err := cl.AddIssuesToModule(ctx, moduleID, []string{wid}); err != nil {
			return domain.ThreadDetail{}, fmt.Errorf("adding to the target bubble: %w", err)
		}
	}
	for _, m := range leaving {
		if err := cl.RemoveIssueFromModule(ctx, m, wid); err != nil {
			// Partial: it IS in the target now, so report rather than pretend.
			return domain.ThreadDetail{}, fmt.Errorf(
				"moved into the target bubble, but could not remove it from %s: %w", m, err)
		}
	}

	// Write through, for the same reason every other create and delete does: the
	// module lists are re-read on the ten-minute structure cadence, so without
	// this the board would show the thread in its old bubble until then.
	s.remember(slug, moduleID, wid, true)
	for _, m := range leaving {
		s.remember(slug, m, wid, false)
	}

	// Not production — filing work somewhere else is not evidence that anything
	// got done. But both bubbles change what they contain, and a bubble's band
	// is its hottest unfinished thread, so the board does have to be rebuilt.
	s.rebuildAndNotify(inst, wid, true)

	actor, _ := domain.ActorFrom(ctx)
	log.Printf("move_thread by %s: %s -> %s (left %d bubble(s))", actor.Label(), full, target, len(leaving))
	return s.ThreadDetail(ctx, full)
}

// remember adds or removes one item from a mirrored module's membership.
func (s *Server) remember(slug, moduleID, itemID string, member bool) {
	items, err := s.mirror.ModuleItems(slug, moduleID)
	if err != nil {
		log.Printf("mirror: read bubble %s: %v", moduleID, err)
		return
	}
	ids := make([]string, 0, len(items)+1)
	for _, it := range items {
		if it.ID != itemID {
			ids = append(ids, it.ID)
		}
	}
	if member {
		ids = append(ids, itemID)
	}
	if err := s.mirror.SetModuleItems(slug, moduleID, ids); err != nil {
		log.Printf("mirror: record membership %s: %v", moduleID, err)
	}
}

func (s *Server) handleMoveThread(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BubbleID string `json:"bubble_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	d, err := s.MoveThread(r.Context(), r.PathValue("id"), in.BubbleID)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ---- workspaces ----
//
// A Workspace is the boundary for a body of work and maps to a Plane PROJECT
// (AGENTS.md vocabulary — not a Plane "workspace"). Creating one already
// existed; renaming and deleting did not, so the outermost container was the
// one thing you could make and never revise.

// Workspaces lists the workspaces the caller can see.
//
// The board could never answer this. It is assembled from BUBBLES, and
// buildInstance skips a project with no modules outright — so a workspace with
// nothing in it yet does not exist as far as the board is concerned. Anything
// deriving its list of workspaces from the board therefore cannot show a
// freshly created one, which also means you cannot put the first bubble in it.
// This reads the mirror's projects directly instead.
func (s *Server) Workspaces(ctx context.Context) ([]domain.Workspace, error) {
	if s.mirror == nil {
		return nil, fmt.Errorf("mirror unavailable")
	}
	actor, _ := domain.ActorFrom(ctx)
	insts, err := s.Instances()
	if err != nil {
		return nil, err
	}
	out := []domain.Workspace{}
	for _, inst := range insts {
		if !actor.CanSee(inst.Slug) {
			continue
		}
		ps, err := s.mirror.Projects(inst.Slug)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			// A pinned instance shows only its project, exactly as the board does.
			if inst.Project != "" && p.ID != inst.Project {
				continue
			}
			if !actor.CanSeeProject(inst.Slug, p.ID) {
				continue
			}
			out = append(out, domain.Workspace{
				ID: p.ID, Name: p.Name, Identifier: p.Identifier, Instance: inst.Slug,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Instance != out[j].Instance {
			return out[i].Instance < out[j].Instance
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	ws, err := s.Workspaces(r.Context())
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

// wsParts splits a namespaced workspace id and authorizes it.
func (s *Server) wsParts(ctx context.Context, id string) (domain.Instance, string, error) {
	slug, projID, _ := cut3(id + ":")
	if slug == "" || projID == "" {
		return domain.Instance{}, "", fmt.Errorf("%w: expected slug:project, got %q", errBadRequest, id)
	}
	actor, _ := domain.ActorFrom(ctx)
	// Project membership, the same gate the board and the interior use.
	if !actor.CanSeeProject(slug, projID) {
		return domain.Instance{}, "", errForbid
	}
	inst, ok, err := s.instanceBySlug(slug)
	if err != nil {
		return domain.Instance{}, "", err
	}
	if !ok {
		return domain.Instance{}, "", errNotFound
	}
	return inst, projID, nil
}

// RenameWorkspace retitles a workspace. Like a thread's title it is not
// production — what a body of work is called is not what has been done.
func (s *Server) RenameWorkspace(ctx context.Context, id, name string) (domain.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Workspace{}, fmt.Errorf("%w: a workspace needs a name", errBadRequest)
	}
	inst, projID, err := s.wsParts(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	if err := cl.SetProjectName(ctx, projID, name); err != nil {
		return domain.Workspace{}, fmt.Errorf("rename project: %w", err)
	}
	if s.mirror != nil {
		if ps, err := s.mirror.Projects(inst.Slug); err == nil {
			for _, p := range ps {
				if p.ID == projID {
					p.Name = name
					if err := s.mirror.UpsertProjects(inst.Slug, []mirror.Project{p}, s.now()); err != nil {
						log.Printf("mirror: record workspace rename %s: %v", projID, err)
					}
					break
				}
			}
		}
	}
	s.dropInstanceCache(inst.Slug)
	if _, err := s.refreshInstance(context.Background(), inst); err != nil {
		log.Printf("rebuild after workspace rename: %v", err)
	}
	s.broadcastThread("")
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("rename_workspace by %s: %s -> %q", actor.Label(), id, name)
	return domain.Workspace{ID: projID, Name: name, Instance: inst.Slug}, nil
}

// RenameBubble retitles a bubble. Not production either: §4 says a bubble is its
// OUTCOME, and the name is only the handle you grab it by — so renaming earns no
// heat and leaves the contract, the stage and every thread inside untouched.
//
// The write goes through to the mirror for the same reason CreateBubble's does:
// modules are re-read on the ten-minute structure cadence, so a board rebuilt
// from an un-updated mirror would keep showing the old name for minutes after
// Plane accepted the new one — the local copy being confidently wrong.
func (s *Server) RenameBubble(ctx context.Context, q, name string) (domain.NewBubble, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.NewBubble{}, fmt.Errorf("%w: a bubble needs a name", errBadRequest)
	}
	id, err := s.resolveID(ctx, q)
	if err != nil {
		return domain.NewBubble{}, err
	}
	slug, err := s.authorizeBubble(ctx, id)
	if err != nil {
		return domain.NewBubble{}, err
	}
	inst, ok, err := s.instanceBySlug(slug)
	if err != nil {
		return domain.NewBubble{}, err
	}
	if !ok {
		return domain.NewBubble{}, errNotFound
	}
	_, projID, moduleID := cut3(id)
	if projID == "" || moduleID == "" {
		return domain.NewBubble{}, fmt.Errorf("%w: malformed bubble id %q", errBadRequest, id)
	}

	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	if err := cl.SetModuleName(ctx, moduleID, name); err != nil {
		return domain.NewBubble{}, fmt.Errorf("rename module: %w", err)
	}

	if s.mirror != nil {
		if ms, err := s.mirror.Modules(inst.Slug, projID); err == nil {
			for _, m := range ms {
				if m.ID == moduleID {
					m.Name = name
					if err := s.mirror.UpsertModules(inst.Slug, []mirror.Module{m}, s.now()); err != nil {
						log.Printf("mirror: record bubble rename %s: %v", id, err)
					}
					break
				}
			}
		}
	}

	s.dropInstanceCache(slug)
	if _, err := s.refreshInstance(context.Background(), inst); err != nil {
		log.Printf("rebuild after bubble rename: %v", err)
	}
	s.broadcastThread("")
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("rename_bubble by %s: %s -> %q", actor.Label(), id, name)
	return domain.NewBubble{ID: id, Name: name}, nil
}

// DeleteWorkspace removes a workspace from Plane, and with it EVERY bubble,
// thread, artifact and comment inside. Nothing about this is recoverable, which
// is why the caller has to say how much it is destroying and mean it.
func (s *Server) DeleteWorkspace(ctx context.Context, id string) (bubbles, threads int, err error) {
	inst, projID, err := s.wsParts(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	if s.mirror == nil {
		return 0, 0, fmt.Errorf("mirror unavailable")
	}
	// Counted BEFORE, so the caller is told what it cost.
	if ms, err := s.mirror.Modules(inst.Slug, projID); err == nil {
		bubbles = len(ms)
	}
	if its, err := s.mirror.Items(inst.Slug, projID); err == nil {
		threads = len(its)
	}

	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	if err := cl.DeleteProject(ctx, projID); err != nil {
		return 0, 0, fmt.Errorf("delete project: %w", err)
	}

	// The overlay keys on bubble ids, which only exist while the modules do —
	// so it has to be cleared before the mirror forgets them.
	if ms, err := s.mirror.Modules(inst.Slug, projID); err == nil {
		for _, m := range ms {
			if err := s.store.ForgetBubble(inst.Slug + ":" + projID + ":" + m.ID); err != nil {
				log.Printf("overlay: forget bubble %s: %v", m.ID, err)
			}
		}
	}
	if its, err := s.mirror.Items(inst.Slug, projID); err == nil {
		ids := make([]string, 0, len(its))
		for _, it := range its {
			ids = append(ids, it.ID)
		}
		if err := s.store.ForgetThreads(ids); err != nil {
			log.Printf("overlay: forget threads: %v", err)
		}
	}
	if err := s.mirror.DeleteProject(inst.Slug, projID); err != nil {
		log.Printf("mirror: forget workspace %s: %v", projID, err)
	}

	s.dropInstanceCache(inst.Slug)
	if _, err := s.refreshInstance(context.Background(), inst); err != nil {
		log.Printf("rebuild after delete_workspace: %v", err)
	}
	s.broadcastThread("")
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("delete_workspace by %s: %s (%d bubble(s), %d thread(s))", actor.Label(), id, bubbles, threads)
	return bubbles, threads, nil
}

func (s *Server) handleRenameWorkspace(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	ws, err := s.RenameWorkspace(r.Context(), r.PathValue("id"), in.Name)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

func (s *Server) handleRenameBubble(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	b, err := s.RenameBubble(r.Context(), r.PathValue("id"), in.Name)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	bubbles, threads, err := s.DeleteWorkspace(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted_bubbles": bubbles, "deleted_threads": threads})
}
