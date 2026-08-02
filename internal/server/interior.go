package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// interiorClient parses a namespaced id (slug:project:objectid), authorizes the
// actor for that instance (read = workspace membership), and returns a Plane
// client pinned to the project plus the id parts.
func (s *Server) interiorClient(ctx context.Context, nsID string) (cl *plane.Client, inst domain.Instance, slug, projID, objID string, err error) {
	parts := strings.SplitN(nsID, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, domain.Instance{}, "", "", "", errNotFound
	}
	slug, projID, objID = parts[0], parts[1], parts[2]

	actor, _ := domain.ActorFrom(ctx)
	if !actor.CanSee(slug) {
		return nil, domain.Instance{}, "", "", "", errForbid
	}
	inst, ok, err := s.instanceBySlug(slug)
	if err != nil {
		return nil, domain.Instance{}, "", "", "", err
	}
	if !ok {
		return nil, domain.Instance{}, "", "", "", errNotFound
	}
	cl = plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, projID)
	return cl, inst, slug, projID, objID, nil
}

// memberNames maps an instance's member ids to display names for resolving
// assignees and comment authors. Best-effort: an error yields an empty map so
// the interior still renders (just without names).
func (s *Server) memberNames(ctx context.Context, inst domain.Instance) map[string]string {
	names := map[string]string{}
	members, err := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Members(ctx)
	if err != nil {
		return names
	}
	for _, m := range members {
		if m.DisplayName != "" {
			names[m.ID] = m.DisplayName
		} else {
			names[m.ID] = m.Email
		}
	}
	return names
}

// Timeline returns a bubble's threads newest-first (INTERIOR-PLAN.md Phase 10).
// bubbleID may be a short id (module segment) or a full namespaced id — it is
// resolved the same way as `heat`.
func (s *Server) Timeline(ctx context.Context, bubbleID string) ([]domain.ThreadNode, error) {
	full, err := s.resolveID(ctx, bubbleID)
	if err != nil {
		return nil, err
	}
	cl, inst, slug, projID, moduleID, err := s.interiorClient(ctx, full)
	if err != nil {
		return nil, err
	}
	items, err := cl.ListModuleWorkItems(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	names := s.memberNames(ctx, inst)

	out := make([]domain.ThreadNode, 0, len(items))
	for _, it := range items {
		owner := ""
		if len(it.Assignees) > 0 {
			owner = names[it.Assignees[0]]
		}
		out = append(out, domain.ThreadNode{
			ID:          slug + ":" + projID + ":" + it.ID,
			Seq:         it.Sequence,
			Title:       it.Name,
			Active:      it.Active,
			Owner:       owner,
			Parent:      it.Parent,
			CreatedAt:   it.CreatedAt,
			CompletedAt: it.CompletedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ThreadDetail returns a thread's full interior (INTERIOR-PLAN.md Phase 11).
// threadID may be a full namespaced id (slug:project:workitem) or a bare
// work-item id/prefix, resolved against the caller's threads.
func (s *Server) ThreadDetail(ctx context.Context, threadID string) (domain.ThreadDetail, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	cl, inst, _, _, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	wi, err := cl.GetWorkItem(ctx, wid)
	if err != nil {
		return domain.ThreadDetail{}, err
	}

	bodyMD := md.FromHTML(wi.DescriptionHTML)
	arts, log := md.ParseThread(bodyMD, wi.Name)
	kind := "simple"
	if log != nil && log.Phased {
		kind = "phased"
	}

	names := s.memberNames(ctx, inst)
	var assignees []string
	for _, a := range wi.Assignees {
		if n := names[a]; n != "" {
			assignees = append(assignees, n)
		}
	}

	return domain.ThreadDetail{
		ID:          full,
		Seq:         wi.Sequence,
		Title:       wi.Name,
		Kind:        kind,
		Active:      wi.CompletedAt == nil,
		Priority:    wi.Priority,
		Assignees:   assignees,
		Artifacts:   arts,
		Logbook:     log,
		Revisions:   s.revisions(ctx, cl, wid),
		CreatedAt:   wi.CreatedAt,
		CompletedAt: wi.CompletedAt,
	}, nil
}

// resolveThreadID normalizes a thread query to a full namespaced id
// (slug:project:workitem). A query already carrying the two colons is returned
// as-is; a bare work-item id (or unique prefix) is located among the caller's
// threads so `bubble thread <wid>` works like `heat`.
func (s *Server) resolveThreadID(ctx context.Context, q string) (string, error) {
	if strings.Count(q, ":") >= 2 {
		return q, nil
	}
	bubbles, err := s.collect(ctx)
	if err != nil {
		return "", err
	}
	var hits []string
	for _, b := range bubbles {
		slug, projID, _ := cut3(b.ID)
		for _, t := range b.Threads {
			ns := slug + ":" + projID + ":" + t.ID
			if t.ID == q {
				return ns, nil // exact wid wins outright
			}
			if strings.HasPrefix(t.ID, q) {
				hits = append(hits, ns)
			}
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return "", errNotFound
	default:
		return "", fmt.Errorf("%w: %q matches %d threads — use more characters", errAmbig, q, len(hits))
	}
}

// cut3 splits a namespaced bubble id (slug:project:module) into its parts.
func cut3(id string) (slug, proj, obj string) {
	parts := strings.SplitN(id, ":", 3)
	for i := range parts {
		switch i {
		case 0:
			slug = parts[0]
		case 1:
			proj = parts[1]
		case 2:
			obj = parts[2]
		}
	}
	return slug, proj, obj
}

// revisions collects the work items related via relates_to whose title is
// prefixed "rev:" and renders each as a free-form artifact (INTERIOR-PLAN.md).
// Best-effort: relation/fetch errors are skipped, never fatal.
func (s *Server) revisions(ctx context.Context, cl *plane.Client, wid string) []md.Artifact {
	rel, err := cl.ListRelations(ctx, wid)
	if err != nil {
		return nil
	}
	out := make([]md.Artifact, 0, len(rel.RelatesTo))
	for _, rid := range rel.RelatesTo {
		wi, err := cl.GetWorkItem(ctx, rid)
		if err != nil {
			continue
		}
		title := strings.TrimSpace(wi.Name)
		if !strings.HasPrefix(strings.ToLower(title), "rev:") {
			continue
		}
		label := strings.TrimSpace(title[len("rev:"):])
		if label == "" {
			label = title
		}
		out = append(out, md.NewArtifact(label, md.FromHTML(wi.DescriptionHTML)))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ThreadComments returns a thread's comment feed oldest-first (Phase 12).
func (s *Server) ThreadComments(ctx context.Context, threadID string) ([]domain.Comment, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return nil, err
	}
	cl, inst, _, _, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return nil, err
	}
	cms, err := cl.ListComments(ctx, wid)
	if err != nil {
		return nil, err
	}
	names := s.memberNames(ctx, inst)

	out := make([]domain.Comment, 0, len(cms))
	for _, cm := range cms {
		out = append(out, domain.Comment{
			ID:        cm.ID,
			Author:    names[cm.ActorID],
			Markdown:  md.FromHTML(cm.HTML),
			CreatedAt: cm.CreatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ---- HTTP handlers ----

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.Timeline(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleThreadDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.ThreadDetail(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleThreadComments(w http.ResponseWriter, r *http.Request) {
	c, err := s.ThreadComments(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, c)
}
