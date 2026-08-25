package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// Project pages: reference material that outlives a thread.
//
// A Brief says why one piece of work exists and a Logbook says how it is going.
// Neither is the place for the standing documentation a body of work accumulates
// — the product spec, the API contract, the decision record everybody keeps
// re-deriving. That belongs to the WORKSPACE, and Plane already has somewhere to
// put it: project pages, carrying description_html in the same editor shape as a
// work item, so internal/md reads and writes them unchanged.
//
// Pages earn NO heat. Heat is evidence that a body of work moved (AGENTS.md);
// documentation exists to be read, and writing some is not the same as producing
// the outcome a bubble is contracted to reach. Nothing here touches the board.
//
// These read LIVE from Plane rather than from the mirror. The mirror exists
// because the board is rebuilt every cycle for every bubble, which would burn
// the 60 req/min budget; pages are opened deliberately, one at a time, by
// someone who wants to read one. Mirroring them would add a table and a sync
// worker to save a request that is only spent when a human asks for it.

// pageParts splits a namespaced page id and authorizes the workspace it is in.
func (s *Server) pageParts(ctx context.Context, id string) (domain.Instance, string, string, error) {
	slug, projID, pageID := cut3(id)
	if slug == "" || projID == "" || pageID == "" {
		return domain.Instance{}, "", "", fmt.Errorf(
			"%w: expected slug:project:page, got %q", errBadRequest, id)
	}
	inst, proj, err := s.wsParts(ctx, slug+":"+projID)
	if err != nil {
		return domain.Instance{}, "", "", err
	}
	return inst, proj, pageID, nil
}

func (s *Server) pageClient(ctx context.Context, inst domain.Instance, projID string) *plane.Client {
	return plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
}

// asPage converts one Plane page to ours. slugProj is "<instance>:<project>".
func asPage(p plane.Page, slug, projID string) domain.Page {
	return domain.Page{
		ID:        slug + ":" + projID + ":" + p.ID,
		Title:     p.Name,
		Instance:  slug,
		Project:   projID,
		Locked:    p.IsLocked,
		Archived:  p.Archived(),
		UpdatedAt: parseTime(p.UpdatedAt),
		CreatedAt: parseTime(p.CreatedAt),
		Storage:   domain.PageInPlane,
		Parent:    nsPageID(slug, projID, p.ParentID),
	}
}

// nsPageID namespaces a raw Plane page id, and leaves "" alone — a root page has
// no parent, and "slug:project:" would be a broken reference rather than none.
func nsPageID(slug, projID, raw string) string {
	if raw == "" {
		return ""
	}
	return slug + ":" + projID + ":" + raw
}

// ---- where a page lives ----
//
// Plane Community serves project pages ONLY on its internal, session-authenticated
// API; the public API an API key can reach has no pages route at all, at any
// current version (docs/journal/PAGES-CAPABILITY.md). Nothing about that is going to be
// fixed by an upgrade, so a workspace on such an instance would have nowhere to
// keep the standing documentation §2.5 says it needs.
//
// So the server becomes the record for pages there. This is a deliberate, narrow
// exception to "Plane is the system of record": the alternative is not "Plane
// holds it", it is "nobody does".

// localPagePrefix marks a page id this server owns. It is part of the id rather
// than a lookup so that every read knows which backend to ask before it asks
// anything — including a read for an id that has since been deleted.
const localPagePrefix = "loc_"

// localPage reports whether a raw page id belongs to this server.
func localPage(pageID string) bool { return strings.HasPrefix(pageID, localPagePrefix) }

// newLocalPageID mints an id for a page this server will hold. Deliberately not a
// Plane-shaped uuid: an id that looks like Plane's invites the assumption that
// Plane has it.
func newLocalPageID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return localPagePrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// capabilityRecheck is how long a "no" is trusted. A "yes" is permanent — an API
// surface does not disappear — but a "no" can be undone by upgrading Plane or by
// moving to an edition that has it, and nobody should have to restart the server
// to be believed.
const capabilityRecheck = 12 * time.Hour

// planeHoldsPages answers whether this instance's Plane can be the record for
// pages, from cache when possible.
//
// It NEVER fails the caller: a probe that cannot get an answer (Plane unreachable,
// credential rejected) is reported as "Plane holds pages", so the normal Plane
// path runs and produces its own real error. Guessing "local" there would be
// worse than an error — it would quietly start a second, divergent copy of a
// workspace's documentation because Plane happened to be down.
func (s *Server) planeHoldsPages(ctx context.Context, inst domain.Instance, projID string) bool {
	if sup, at, known, err := s.store.Capability(inst.Slug, "pages"); err == nil && known {
		if sup || s.now().Sub(at) < capabilityRecheck {
			return sup
		}
	}
	cl := s.pageClient(ctx, inst, projID)
	ok, err := cl.PagesAPIAvailable(ctx, projID)
	if err != nil {
		log.Printf("pages capability %s: probe inconclusive: %v", inst.Slug, err)
		return true
	}
	if err := s.store.SetCapability(inst.Slug, "pages", ok, s.now()); err != nil {
		log.Printf("pages capability %s: could not cache: %v", inst.Slug, err)
	}
	if !ok {
		log.Printf("pages capability %s: this Plane has no pages route on its public API — "+
			"pages for it are recorded locally", inst.Slug)
	}
	return ok
}

// asLocalPage converts a stored page to the wire shape.
func asLocalPage(p store.LocalPage) domain.Page {
	return domain.Page{
		ID:        p.ID,
		Title:     p.Title,
		Instance:  p.Instance,
		Project:   p.Project,
		Locked:    p.Locked,
		Archived:  p.Archived,
		UpdatedAt: p.UpdatedAt,
		CreatedAt: p.CreatedAt,
		Storage:   domain.PageInLocal,
		Parent:    p.Parent,
	}
}

// localPageDetail renders a stored page the same way a Plane one is rendered, so
// no reader has to care which it got.
func localPageDetail(p store.LocalPage) domain.PageDetail {
	return domain.PageDetail{
		Page:     asLocalPage(p),
		Markdown: p.Body,
		HTML:     md.RenderHTML(p.Body),
		Hash:     md.Hash(p.Body),
	}
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Pages lists a workspace's pages, newest change first — a document you touched
// this morning is the one you are most likely looking for.
//
// Archived pages are left out: Plane archives a page to get it out of the way,
// and repeating it here would undo that.
// Locally-held pages are ALWAYS included, even on an instance whose Plane does
// serve pages. Pages written while it could not must not disappear the day it is
// upgraded, and their ids say who holds them, so both kinds coexist in one list
// with no migration.
func (s *Server) Pages(ctx context.Context, workspaceID string) (domain.PageList, error) {
	inst, projID, err := s.wsParts(ctx, workspaceID)
	if err != nil {
		return domain.PageList{}, err
	}

	out := []domain.Page{}
	locals, err := s.store.LocalPages(inst.Slug, projID)
	if err != nil {
		return domain.PageList{}, fmt.Errorf("read local pages: %w", err)
	}
	for _, p := range locals {
		if p.Archived {
			continue
		}
		out = append(out, asLocalPage(p))
	}

	inPlane := s.planeHoldsPages(ctx, inst, projID)
	if inPlane {
		cl := s.pageClient(ctx, inst, projID)
		ps, err := cl.ListPages(ctx, projID)
		if err != nil {
			return domain.PageList{}, s.explainPageFailure(ctx, cl, projID, err)
		}
		for _, p := range ps {
			if p.Archived() {
				continue
			}
			out = append(out, asPage(p, inst.Slug, projID))
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return domain.PageList{Pages: out, PlaneHoldsPages: inPlane}, nil
}

// explainPageFailure turns Plane's 404 into something actionable.
//
// It is only reached once the capability probe has established that the pages
// route DOES exist here, so a 404 now means the project — not the surface. The
// remaining question is whether Pages is switched off for it, which hides the
// feature and answers 404 rather than saying so.
//
// It used to also claim "this Plane is too old, the endpoint arrived in a later
// release", inferred from the same 404. That was wrong twice over: the 404 is
// Plane's answer to ANY unrouted URL, so it carried no such information, and the
// endpoint is missing from Plane Community at every current version rather than
// waiting in a newer one. It sent people to do an upgrade that changes nothing.
func (s *Server) explainPageFailure(ctx context.Context, cl *plane.Client, projID string, err error) error {
	var api *plane.APIError
	if !errors.As(err, &api) || api.Status != http.StatusNotFound {
		return fmt.Errorf("list pages: %w", err)
	}
	p, perr := cl.GetProject(ctx, projID)
	if perr == nil && !p.PageView {
		return fmt.Errorf(
			"%w: pages are switched off for %q in Plane — turn Pages on in that project's "+
				"settings and this works immediately", errBadRequest, p.Name)
	}
	return fmt.Errorf("list pages: %w", err)
}

// Page returns one page with its body as markdown, from whichever system holds
// it — the id says which, so this costs no probe and no extra request.
func (s *Server) Page(ctx context.Context, pageID string) (domain.PageDetail, error) {
	inst, projID, pid, err := s.pageParts(ctx, pageID)
	if err != nil {
		return domain.PageDetail{}, err
	}
	if localPage(pid) {
		p, ok, err := s.store.LocalPage(pageID)
		if err != nil {
			return domain.PageDetail{}, fmt.Errorf("read page: %w", err)
		}
		if !ok {
			return domain.PageDetail{}, errNotFound
		}
		return localPageDetail(p), nil
	}
	p, err := s.pageClient(ctx, inst, projID).GetPage(ctx, projID, pid)
	if err != nil {
		return domain.PageDetail{}, fmt.Errorf("read page: %w", err)
	}
	body := md.FromHTML(p.Description)
	return domain.PageDetail{
		Page:     asPage(p, inst.Slug, projID),
		Markdown: body,
		HTML:     md.RenderHTML(body),
		Hash:     md.Hash(body),
	}, nil
}

// CreatePage adds a page to a workspace.
func (s *Server) CreatePage(ctx context.Context, req domain.CreatePageRequest) (domain.PageDetail, error) {
	inst, projID, err := s.wsParts(ctx, req.Workspace)
	if err != nil {
		return domain.PageDetail{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return domain.PageDetail{}, fmt.Errorf("%w: a page needs a title", errBadRequest)
	}
	// The standard applies to a document too — an H1 per page, headings that
	// step one level at a time (spec §3.1). Judged on what this write contains,
	// since there is nothing here it could have inherited.
	// Refusals only: the advisory rules describe a thread's Brief and Logbook, and a
	// page is neither (docs/decisions/0003).
	if bad := md.Refusals(md.Lint(req.Markdown)); len(bad) > 0 {
		return domain.PageDetail{}, fmt.Errorf("%w: %v", errBadRequest, md.LintError(bad))
	}
	// A parent has to be a real page in THIS workspace. Left unchecked, a
	// mistyped or cross-workspace id would produce a page that exists but hangs
	// off nothing the tree can render, so it would simply not appear.
	parent := strings.TrimSpace(req.Parent)
	if parent != "" {
		if !strings.HasPrefix(parent, inst.Slug+":"+projID+":") {
			return domain.PageDetail{}, fmt.Errorf(
				"%w: a page's parent must be in the same workspace, got %q", errBadRequest, parent)
		}
		if _, err := s.Page(ctx, parent); err != nil {
			return domain.PageDetail{}, fmt.Errorf("parent page: %w", err)
		}
	}

	actorNow, _ := domain.ActorFrom(ctx)
	if !s.planeHoldsPages(ctx, inst, projID) {
		id, err := newLocalPageID()
		if err != nil {
			return domain.PageDetail{}, err
		}
		now := s.now()
		lp := store.LocalPage{
			ID: inst.Slug + ":" + projID + ":" + id, Instance: inst.Slug, Project: projID,
			Title: title, Body: req.Markdown, CreatedAt: now, UpdatedAt: now,
			Author: actorNow.Label(), Parent: parent,
		}
		if err := s.store.PutLocalPage(lp); err != nil {
			return domain.PageDetail{}, fmt.Errorf("create page: %w", err)
		}
		log.Printf("create_page by %s: %s (local — this Plane has no pages API)",
			actorNow.Label(), lp.ID)
		return localPageDetail(lp), nil
	}

	var html string
	if strings.TrimSpace(req.Markdown) != "" {
		html = md.RenderPlaneHTML(req.Markdown)
	}
	// Plane wants its own raw id, not our namespaced one.
	_, _, parentRaw := cut3(parent)
	p, err := s.pageClient(ctx, inst, projID).CreatePage(ctx, projID, title, html, parentRaw)
	if err != nil {
		return domain.PageDetail{}, fmt.Errorf("create page: %w", err)
	}
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("create_page by %s: %s in %s:%s", actor.Label(), p.ID, inst.Slug, projID)
	return domain.PageDetail{
		Page:     asPage(p, inst.Slug, projID),
		Markdown: req.Markdown,
		HTML:     md.RenderHTML(req.Markdown),
		Hash:     md.Hash(req.Markdown),
	}, nil
}

// UpdatePage rewrites a page's title, its body, or both.
func (s *Server) UpdatePage(ctx context.Context, pageID string, edit domain.PageEdit) (domain.PageDetail, error) {
	inst, projID, pid, err := s.pageParts(ctx, pageID)
	if err != nil {
		return domain.PageDetail{}, err
	}
	if localPage(pid) {
		return s.updateLocalPage(ctx, pageID, edit)
	}
	cl := s.pageClient(ctx, inst, projID)
	cur, err := cl.GetPage(ctx, projID, pid)
	if err != nil {
		return domain.PageDetail{}, fmt.Errorf("read page: %w", err)
	}
	// Plane locks a page to stop it being edited. Honour that rather than
	// discovering it as an opaque error from the write.
	if cur.IsLocked {
		return domain.PageDetail{}, fmt.Errorf("%w: this page is locked in Plane", errBadRequest)
	}

	body := md.FromHTML(cur.Description)
	if edit.BaseHash != "" && edit.BaseHash != md.Hash(body) {
		return domain.PageDetail{}, fmt.Errorf(
			"%w: this page changed since you read it — read it again and reapply", errConflict)
	}

	patch := map[string]any{}
	if edit.Title != nil {
		title := strings.TrimSpace(*edit.Title)
		if title == "" {
			return domain.PageDetail{}, fmt.Errorf("%w: a page needs a title", errBadRequest)
		}
		patch["name"] = title
	}
	if edit.Markdown != nil {
		next := *edit.Markdown
		// Same rule as a thread write: judged on what this write INTRODUCED, so
		// an unrelated fix is not blocked by a page somebody else left messy.
		if broke := md.Refusals(md.NewFindings(md.Lint(body), md.Lint(next))); len(broke) > 0 {
			return domain.PageDetail{}, fmt.Errorf("%w: %v", errBadRequest, md.LintError(broke))
		}
		patch["description_html"] = md.RenderPlaneHTML(next)
		body = next
	}
	if len(patch) == 0 {
		return s.Page(ctx, pageID)
	}
	if err := cl.UpdatePage(ctx, projID, pid, patch); err != nil {
		return domain.PageDetail{}, fmt.Errorf("update page: %w", err)
	}

	actor, _ := domain.ActorFrom(ctx)
	log.Printf("update_page by %s: %s", actor.Label(), pageID)

	out := asPage(cur, inst.Slug, projID)
	if edit.Title != nil {
		out.Title = strings.TrimSpace(*edit.Title)
	}
	out.UpdatedAt = s.now()
	return domain.PageDetail{
		Page: out, Markdown: body, HTML: md.RenderHTML(body), Hash: md.Hash(body),
	}, nil
}

// updateLocalPage applies a page edit to the copy this server holds. Every rule
// the Plane path enforces applies here — the lock, the 409 on a stale base hash,
// the lint judged on what this write introduced — because they are properties of
// editing a document, not of Plane.
func (s *Server) updateLocalPage(ctx context.Context, pageID string, edit domain.PageEdit) (domain.PageDetail, error) {
	cur, ok, err := s.store.LocalPage(pageID)
	if err != nil {
		return domain.PageDetail{}, fmt.Errorf("read page: %w", err)
	}
	if !ok {
		return domain.PageDetail{}, errNotFound
	}
	if cur.Locked {
		return domain.PageDetail{}, fmt.Errorf("%w: this page is locked", errBadRequest)
	}
	if edit.BaseHash != "" && edit.BaseHash != md.Hash(cur.Body) {
		return domain.PageDetail{}, fmt.Errorf(
			"%w: this page changed since you read it — read it again and reapply", errConflict)
	}

	next := cur
	if edit.Title != nil {
		title := strings.TrimSpace(*edit.Title)
		if title == "" {
			return domain.PageDetail{}, fmt.Errorf("%w: a page needs a title", errBadRequest)
		}
		next.Title = title
	}
	if edit.Markdown != nil {
		if broke := md.Refusals(md.NewFindings(md.Lint(cur.Body), md.Lint(*edit.Markdown))); len(broke) > 0 {
			return domain.PageDetail{}, fmt.Errorf("%w: %v", errBadRequest, md.LintError(broke))
		}
		next.Body = *edit.Markdown
	}
	if next.Title == cur.Title && next.Body == cur.Body {
		return localPageDetail(cur), nil
	}
	next.UpdatedAt = s.now()
	if err := s.store.PutLocalPage(next); err != nil {
		return domain.PageDetail{}, fmt.Errorf("update page: %w", err)
	}
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("update_page by %s: %s (local)", actor.Label(), pageID)
	return localPageDetail(next), nil
}

// DeletePage removes a page from wherever it lives. Irreversible either way: Plane
// keeps no copy, and neither do we.
func (s *Server) DeletePage(ctx context.Context, pageID string) error {
	inst, projID, pid, err := s.pageParts(ctx, pageID)
	if err != nil {
		return err
	}
	if localPage(pid) {
		// Read it first, for its parent: its children move up to take its place.
		cur, ok, err := s.store.LocalPage(pageID)
		if err != nil {
			return fmt.Errorf("delete page: %w", err)
		}
		if !ok {
			return errNotFound
		}
		gone, err := s.store.DeleteLocalPage(pageID)
		if err != nil {
			return fmt.Errorf("delete page: %w", err)
		}
		if !gone {
			return errNotFound
		}
		// Deleting a page must not take its children with it, silently or at all.
		if err := s.store.PromoteLocalPageChildren(pageID, cur.Parent); err != nil {
			log.Printf("overlay: promote children of %s: %v", pageID, err)
		}
		actor, _ := domain.ActorFrom(ctx)
		log.Printf("delete_page by %s: %s (local)", actor.Label(), pageID)
		return nil
	}
	if err := s.pageClient(ctx, inst, projID).DeletePage(ctx, projID, pid); err != nil {
		return fmt.Errorf("delete page: %w", err)
	}
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("delete_page by %s: %s", actor.Label(), pageID)
	return nil
}

// ---- REST ----

func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Pages(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) handleCreatePage(w http.ResponseWriter, r *http.Request) {
	var in domain.CreatePageRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	in.Workspace = r.PathValue("id")
	d, err := s.CreatePage(r.Context(), in)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	d, err := s.Page(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpdatePage(w http.ResponseWriter, r *http.Request) {
	var in domain.PageEdit
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	d, err := s.UpdatePage(r.Context(), r.PathValue("id"), in)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeletePage(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, s.DeletePage(r.Context(), r.PathValue("id"))) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
