package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
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
func (s *Server) Pages(ctx context.Context, workspaceID string) ([]domain.Page, error) {
	inst, projID, err := s.wsParts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	ps, err := s.pageClient(ctx, inst, projID).ListPages(ctx, projID)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	out := make([]domain.Page, 0, len(ps))
	for _, p := range ps {
		if p.Archived() {
			continue
		}
		out = append(out, asPage(p, inst.Slug, projID))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Page returns one page with its body as markdown.
func (s *Server) Page(ctx context.Context, pageID string) (domain.PageDetail, error) {
	inst, projID, pid, err := s.pageParts(ctx, pageID)
	if err != nil {
		return domain.PageDetail{}, err
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
	if bad := md.Lint(req.Markdown); len(bad) > 0 {
		return domain.PageDetail{}, fmt.Errorf("%w: %v", errBadRequest, md.LintError(bad))
	}
	var html string
	if strings.TrimSpace(req.Markdown) != "" {
		html = md.RenderPlaneHTML(req.Markdown)
	}
	p, err := s.pageClient(ctx, inst, projID).CreatePage(ctx, projID, title, html)
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
		if broke := md.NewFindings(md.Lint(body), md.Lint(next)); len(broke) > 0 {
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

// DeletePage removes a page from Plane. Irreversible, like every other delete
// here — Plane is the system of record and keeps no copy.
func (s *Server) DeletePage(ctx context.Context, pageID string) error {
	inst, projID, pid, err := s.pageParts(ctx, pageID)
	if err != nil {
		return err
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
