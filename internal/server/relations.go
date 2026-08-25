package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// Plane's own relationships (docs/decisions/0006).
//
// The framework used to answer three questions inside the markdown: what kind of
// work is this (a `type` in an overlay table), where is the evidence (a `### Links`
// section somebody had to write by hand), and what does this depend on (a sentence
// in the Logbook). Plane answers all three natively — labels, links, work-item
// relations — and answering them a second time here only produced a second answer
// that could disagree.
//
// So these verbs write to Plane and write THROUGH to the mirror, which keeps the
// rule that no read path talks to Plane. The sync's per-item fill-in exists only to
// notice edges added in Plane's own UI.

// SetLabels replaces a thread's labels, by NAME.
//
// By name rather than by id because a caller who has to look up a uuid first will
// not bother, and the ids are Plane's business. A name the project does not have
// yet is CREATED: refusing would make the verb useless the first time anyone uses a
// new word, and Plane's own UI creates on the fly for exactly that reason.
//
// Labelling is classification, not production. It warms nothing — the same rule
// that governs renaming (§5.1).
func (s *Server) SetLabels(ctx context.Context, threadID string, names []string) (domain.ThreadDetail, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	cl, inst, slug, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	// Write as the acting person, so Plane attributes the change to them.
	cl = plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)

	catalogue, err := cl.ListLabels(ctx)
	if err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("list labels: %w", err)
	}
	byName := make(map[string]plane.Label, len(catalogue))
	for _, l := range catalogue {
		byName[strings.ToLower(strings.TrimSpace(l.Name))] = l
	}

	ids := make([]string, 0, len(names))
	seen := map[string]bool{}
	resolved := make([]mirror.Label, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		l, ok := byName[strings.ToLower(name)]
		if !ok {
			created, err := cl.CreateLabel(ctx, name, "")
			if err != nil {
				return domain.ThreadDetail{}, fmt.Errorf("create label %q: %w", name, err)
			}
			l = created
			byName[strings.ToLower(name)] = l
		}
		if seen[l.ID] {
			continue // the same label twice is one label
		}
		seen[l.ID] = true
		ids = append(ids, l.ID)
		resolved = append(resolved, mirror.Label{ID: l.ID, ProjectID: projID, Name: l.Name, Color: l.Color})
	}

	if err := cl.SetWorkItemLabels(ctx, wid, ids); err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("set labels: %w", err)
	}

	// Write through, so the answer this call returns already reflects the change
	// instead of waiting for a sync pass.
	if s.mirror != nil {
		if err := s.mirror.UpsertLabels(slug, resolved); err != nil {
			log.Printf("labels: mirror catalogue for %s: %v", wid, err)
		}
		if it, ok, err := s.mirror.Item(slug, wid); err == nil && ok {
			it.Labels = ids
			if err := s.mirror.UpsertItems(slug, []mirror.Item{it}, s.now()); err != nil {
				log.Printf("labels: mirror item %s: %v", wid, err)
			}
		}
	}
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("set_labels by %s: %s → %v", actor.Label(), wid, names)
	return s.ThreadDetail(ctx, full)
}

// AddLink attaches external evidence to a thread: the commit, the PR, the thing
// that shipped.
//
// This is the honest home for what a `### Links` section used to hold by hand.
// Publishing evidence IS production (§5.1), so it warms the thread — which is the
// other half of why it belongs in a verb rather than in prose: a URL pasted into a
// paragraph warms the thread only because the paragraph changed.
func (s *Server) AddLink(ctx context.Context, threadID, url, title string) (domain.ThreadDetail, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return domain.ThreadDetail{}, fmt.Errorf("%w: a link needs a url", errBadRequest)
	}
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	_, inst, slug, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)

	if _, err := cl.AddLink(ctx, wid, url, strings.TrimSpace(title)); err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("add link: %w", err)
	}
	s.refreshLinks(ctx, cl, slug, wid)
	// The link count is what the progress sweep diffs, so rebuild the snapshot to
	// let the heat land now rather than on the next tick.
	s.rebuildAndNotify(inst, full, true)

	actor, _ := domain.ActorFrom(ctx)
	log.Printf("add_link by %s: %s → %s", actor.Label(), wid, url)
	return s.ThreadDetail(ctx, full)
}

// RemoveLink detaches a link. Removing evidence is not production — nothing is
// produced by taking something away — so it warms nothing.
func (s *Server) RemoveLink(ctx context.Context, threadID, linkID string) (domain.ThreadDetail, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	_, inst, slug, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	cl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)

	if err := cl.RemoveLink(ctx, wid, linkID); err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("remove link: %w", err)
	}
	s.refreshLinks(ctx, cl, slug, wid)
	return s.ThreadDetail(ctx, full)
}

// refreshLinks re-reads one item's links and writes them through to the mirror.
// Best-effort: the write already landed in Plane, and the sync's fill-in will
// correct the mirror if this fails.
func (s *Server) refreshLinks(ctx context.Context, cl *plane.Client, slug, wid string) {
	if s.mirror == nil {
		return
	}
	ls, err := cl.ListLinks(ctx, wid)
	if err != nil {
		log.Printf("links: re-read %s: %v", wid, err)
		return
	}
	out := make([]mirror.Link, 0, len(ls))
	for _, l := range ls {
		out = append(out, mirror.Link{ID: l.ID, URL: l.URL, Title: l.Title, CreatedAt: l.CreatedAt})
	}
	if err := s.mirror.ReplaceLinks(slug, wid, out); err != nil {
		log.Printf("links: mirror %s: %v", wid, err)
	}
}

// relationTypes are the edges this surface offers. Plane understands eight; these
// are the four that answer a question the framework actually asks — what is this
// related to, what is it a duplicate of, what is blocking it. The scheduling four
// (start_before/after, finish_before/after) are read back if somebody sets them in
// Plane, and are not offered here: nothing in the model reads a date yet.
var relationTypes = map[string]bool{
	plane.RelRelatesTo: true,
	plane.RelDuplicate: true,
	plane.RelBlocking:  true,
	plane.RelBlockedBy: true,
}

// RelateThreads records a typed relationship between two threads.
//
// Relating is not production: saying two pieces of work touch each other does not
// change either of them. It warms nothing, the same as a rename or a move.
func (s *Server) RelateThreads(ctx context.Context, threadID, otherID, relType string) (domain.ThreadDetail, error) {
	relType = strings.ToLower(strings.TrimSpace(relType))
	if relType == "" {
		relType = plane.RelRelatesTo
	}
	if !relationTypes[relType] {
		return domain.ThreadDetail{}, fmt.Errorf(
			"%w: %q is not a relation — use relates_to, duplicate, blocking or blocked_by",
			errBadRequest, relType)
	}
	full, other, cl, inst, slug, wid, otherWID, err := s.relationEnds(ctx, threadID, otherID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if wid == otherWID {
		return domain.ThreadDetail{}, fmt.Errorf("%w: a thread cannot relate to itself", errBadRequest)
	}
	_ = other

	if err := cl.AddRelation(ctx, wid, relType, []string{otherWID}); err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("relate: %w", err)
	}
	// Both ends change, because Plane's pairs are symmetric: writing `blocking` on
	// one shows as `blocked_by` on the other.
	s.refreshRelations(ctx, cl, slug, wid)
	s.refreshRelations(ctx, cl, slug, otherWID)

	actor, _ := domain.ActorFrom(ctx)
	log.Printf("relate by %s: %s --%s--> %s [%s]", actor.Label(), wid, relType, otherWID, inst.Slug)
	return s.ThreadDetail(ctx, full)
}

// UnrelateThreads drops the relationship between two threads. Plane models this as
// a POST to .../relations/remove/ and does not ask which type, because a pair of
// items has at most one.
func (s *Server) UnrelateThreads(ctx context.Context, threadID, otherID string) (domain.ThreadDetail, error) {
	full, _, cl, _, slug, wid, otherWID, err := s.relationEnds(ctx, threadID, otherID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if err := cl.RemoveRelation(ctx, wid, otherWID); err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("unrelate: %w", err)
	}
	s.refreshRelations(ctx, cl, slug, wid)
	s.refreshRelations(ctx, cl, slug, otherWID)
	return s.ThreadDetail(ctx, full)
}

// relationEnds resolves both ends of a relation and returns a client authorised
// for the FIRST one's project.
//
// Both ends must live in the same project: Plane's relation endpoint is
// project-scoped, and a cross-project relation would be written into a project the
// caller may not even be a member of. Saying so is better than a confusing 404 from
// Plane.
func (s *Server) relationEnds(ctx context.Context, threadID, otherID string) (
	full, otherFull string, cl *plane.Client, inst domain.Instance, slug, wid, otherWID string, err error,
) {
	full, err = s.resolveThreadID(ctx, threadID)
	if err != nil {
		return "", "", nil, domain.Instance{}, "", "", "", err
	}
	otherFull, err = s.resolveThreadID(ctx, otherID)
	if err != nil {
		return "", "", nil, domain.Instance{}, "", "", "", err
	}
	_, inst, slug, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return "", "", nil, domain.Instance{}, "", "", "", err
	}
	_, _, otherSlug, otherProj, otherWID, err := s.interiorClient(ctx, otherFull)
	if err != nil {
		return "", "", nil, domain.Instance{}, "", "", "", err
	}
	if otherSlug != slug || otherProj != projID {
		return "", "", nil, domain.Instance{}, "", "", "", fmt.Errorf(
			"%w: both threads must be in the same workspace — Plane relates work items inside one project",
			errBadRequest)
	}
	cl = plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	return full, otherFull, cl, inst, slug, wid, otherWID, nil
}

// refreshRelations re-reads one item's relations into the mirror. Best-effort, for
// the same reason as refreshLinks.
func (s *Server) refreshRelations(ctx context.Context, cl *plane.Client, slug, wid string) {
	if s.mirror == nil {
		return
	}
	r, err := cl.ListRelations(ctx, wid)
	if err != nil {
		log.Printf("relations: re-read %s: %v", wid, err)
		return
	}
	var out []mirror.Relation
	add := func(kind string, ids []string) {
		for _, id := range ids {
			out = append(out, mirror.Relation{Type: kind, RelatedID: id})
		}
	}
	add(plane.RelBlocking, r.Blocking)
	add(plane.RelBlockedBy, r.BlockedBy)
	add(plane.RelDuplicate, r.Duplicate)
	add(plane.RelRelatesTo, r.RelatesTo)
	add("start_after", r.StartAfter)
	add("start_before", r.StartBefore)
	add("finish_after", r.FinishAfter)
	add("finish_before", r.FinishBefore)
	if err := s.mirror.ReplaceRelations(slug, wid, out); err != nil {
		log.Printf("relations: mirror %s: %v", wid, err)
	}
}

// ---- REST ----

func (s *Server) handleSetLabels(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Labels []string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, fmt.Errorf("%w: %v", errBadRequest, err))
		return
	}
	d, err := s.SetLabels(r.Context(), r.PathValue("id"), in.Labels)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleAddLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, fmt.Errorf("%w: %v", errBadRequest, err))
		return
	}
	d, err := s.AddLink(r.Context(), r.PathValue("id"), in.URL, in.Title)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleRemoveLink(w http.ResponseWriter, r *http.Request) {
	d, err := s.RemoveLink(r.Context(), r.PathValue("id"), r.PathValue("link"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleRelate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thread string `json:"thread"`
		Type   string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, fmt.Errorf("%w: %v", errBadRequest, err))
		return
	}
	d, err := s.RelateThreads(r.Context(), r.PathValue("id"), in.Thread, in.Type)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUnrelate(w http.ResponseWriter, r *http.Request) {
	d, err := s.UnrelateThreads(r.Context(), r.PathValue("id"), r.PathValue("other"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}
