package server

import (
	"context"
	"encoding/json"
	"errors"
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

// editRegions maps the write API's field names onto the splice engine's regions.
// "brief" is the API's word for the document — the part of the page that is not
// the Logbook or the DoD — and it stays, because it is what agents already know.
var editRegions = []struct {
	name   string
	region md.Region
}{
	{"brief", md.RegionDocument},
	{"logbook", md.RegionLogbook},
	{"dod", md.RegionDoD},
}

// regionByName resolves the write API's name to a splice region. It exists
// because the two vocabularies genuinely differ — "brief" is the API's word for
// the document — and every surface has to agree on the translation.
func regionByName(name string) (md.Region, bool) {
	for _, r := range editRegions {
		if r.name == name {
			return r.region, true
		}
	}
	// Accept the engine's own names too, so an MCP caller that says "document"
	// is not mysteriously rejected.
	switch md.Region(name) {
	case md.RegionDocument, md.RegionLogbook, md.RegionDoD:
		return md.Region(name), true
	}
	return "", false
}

// UpdateThread rewrites parts of a thread's artifact page.
//
// It is REGION-SCOPED, and it SPLICES rather than re-rendering. Both matter:
//
// The regions share one Plane description (§3: one page, not a second tracker),
// so a whole-body write is the obvious shape and the wrong one — the Brief is
// the human's statement of intent, and an agent revising a plan must not be able
// to erase it. A nil field is left exactly as it was.
//
// And within a region, only the BLOCKS that actually changed are re-rendered.
// Until this, editing a Logbook re-derived the whole body from markdown, which
// silently destroyed every image and mention on the page (48 mentions and 193
// images across a live workspace). A block nobody edited now keeps its original
// bytes (docs/ARTIFACT-EDITING.md Phase 1).
func (s *Server) UpdateThread(ctx context.Context, threadID string, edit domain.ThreadEdit) (domain.ThreadDetail, error) {
	if edit.Empty() {
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
	// Read the CURRENT body rather than trusting a copy the caller loaded, so two
	// people editing different regions cannot lose each other's work.
	it, ok, err := s.mirror.Item(inst.Slug, wid)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if !ok {
		return domain.ThreadDetail{}, errNotFound
	}

	wcl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)

	// A rename is its own write: the title lives in Plane's `name`, not in the
	// description, so it does not go through the splice at all. It is also not
	// production — what the work is CALLED is not what has been done.
	if edit.Title != nil {
		title := strings.TrimSpace(*edit.Title)
		if title == "" {
			return domain.ThreadDetail{}, fmt.Errorf("%w: a thread needs a title", errBadRequest)
		}
		if title != it.Name {
			if _, err := wcl.SetWorkItemName(ctx, wid, title); err != nil {
				return domain.ThreadDetail{}, err
			}
			it.Name = title
			if err := s.mirror.UpsertItems(inst.Slug, []mirror.Item{it}, s.now()); err != nil {
				log.Printf("mirror: record rename %s: %v", wid, err)
			}
			// The board shows thread titles, so this one DOES need a rebuild even
			// though it earns no heat.
			s.rebuildAndNotify(inst, wid, true)
		}
	}

	body := it.DescriptionHTML
	production := false

	// Surgical edits first, resolved against what is CURRENTLY there. Each one
	// produces the new whole-region markdown, which then goes through the same
	// splice as any other write — so an edit still only rewrites the blocks it
	// actually touched.
	patched, err := applyRegionEdits(body, edit.Edits)
	if err != nil {
		return domain.ThreadDetail{}, err
	}

	for _, r := range editRegions {
		want := map[string]*string{
			"brief": edit.Brief, "logbook": edit.Logbook, "dod": edit.DoD,
		}[r.name]
		if p, ok := patched[r.name]; ok {
			if want != nil {
				// Naming both for one region is a contradiction, not something to
				// resolve by picking an order.
				return domain.ThreadDetail{}, fmt.Errorf(
					"%w: %s was given both a replacement and an edit — send one or the other",
					errBadRequest, r.name)
			}
			want = &p
		}
		if want == nil {
			continue
		}
		if err := checkBase(body, r.region, edit.Base[r.name]); err != nil {
			return domain.ThreadDetail{}, err
		}
		spliced, err := md.Splice(body, r.region, *want)
		if err != nil {
			return domain.ThreadDetail{}, fmt.Errorf("%w: %v", errBadRequest, err)
		}
		if spliced != body {
			body = spliced
			// Only the Logbook and the DoD are production (§5.1); a Brief edit is
			// not, and progressEvidenceFromMirror agrees by fingerprinting only
			// those two.
			production = production || r.region != md.RegionDocument
		}
	}

	if body == it.DescriptionHTML {
		// Every region was submitted unchanged. Writing would cost a Plane call
		// and, for a Logbook, would stamp production for work nobody did — which
		// is exactly what autosave must not do.
		return s.ThreadDetail(ctx, full)
	}

	// Written with the caller's own key, so Plane attributes the edit to the
	// person (or the human an agent is impersonating), exactly like a comment.
	// writeBody also puts it in the mirror, rather than waiting for a delta pass
	// to rediscover a change the server itself just made.
	if err := s.writeBody(ctx, wcl, inst, it, body); err != nil {
		return domain.ThreadDetail{}, err
	}
	// A Logbook change is production (§5.1). Rebuilding the snapshot is what
	// turns it into heat and pushes it to every open board.
	s.rebuildAndNotify(inst, wid, production)

	return s.ThreadDetail(ctx, full)
}

// writeBody pushes a new description to Plane and records it locally.
//
// Shared by every path that rewrites a body — editing a region, ticking a todo,
// deleting a section — so the mirror write-through can never be forgotten by one
// of them. Not optimistic: Plane accepted the write and returned the row, so
// this IS what Plane holds.
func (s *Server) writeBody(ctx context.Context, cl *plane.Client, inst domain.Instance, it mirror.Item, html string) error {
	updatedAt, err := cl.SetWorkItemBody(ctx, it.ID, html)
	if err != nil {
		return err
	}
	it.DescriptionHTML = html
	it.DescriptionHash = mirror.HashBody(html)
	if !updatedAt.IsZero() {
		it.UpdatedAt = updatedAt
	}
	if err := s.mirror.UpsertItems(inst.Slug, []mirror.Item{it}, s.now()); err != nil {
		log.Printf("mirror: record body write %s: %v", it.ID, err)
	}
	return nil
}

// applyRegionEdits turns find-and-replace edits into new region markdown.
//
// Everything is resolved before ANY write: a set that fails half way through
// leaves nothing applied, because a partly-applied patch is worse than a
// refused one — the caller cannot tell which half landed.
func applyRegionEdits(descriptionHTML string, edits []domain.RegionEdit) (map[string]string, error) {
	if len(edits) == 0 {
		return nil, nil
	}
	byRegion := map[string][]md.Edit{}
	order := []string{}
	for _, e := range edits {
		name := strings.TrimSpace(strings.ToLower(e.Region))
		if name == "" {
			name = "logbook" // where an agent almost always means
		}
		if _, ok := regionByName(name); !ok {
			return nil, fmt.Errorf("%w: unknown region %q", errBadRequest, e.Region)
		}
		if _, seen := byRegion[name]; !seen {
			order = append(order, name)
		}
		byRegion[name] = append(byRegion[name], md.Edit{Old: e.Old, New: e.New, All: e.All})
	}

	out := map[string]string{}
	for _, name := range order {
		region, _ := regionByName(name)
		current, found := md.RegionMarkdown(descriptionHTML, region)
		if !found {
			return nil, fmt.Errorf("%w: this thread has no %s to edit", errNotFound, name)
		}
		next, err := md.ApplyEdits(current, byRegion[name])
		if err != nil {
			// A miss or an ambiguity is the caller quoting something that is not
			// there, which is a bad request rather than a server failure — and
			// the message already says which text and why.
			return nil, fmt.Errorf("%w: %s: %v", errBadRequest, name, err)
		}
		out[name] = next
	}
	return out, nil
}

// checkBase enforces optimistic concurrency for one region. An absent base means
// the caller did not read the page first — legitimate for CLI and MCP writes —
// and is accepted rather than guessed at.
func checkBase(descriptionHTML string, region md.Region, base string) error {
	if base == "" {
		return nil
	}
	current, _ := md.RegionMarkdown(descriptionHTML, region)
	if md.Hash(current) != base {
		return fmt.Errorf("%w: the %s changed while you were editing it", errConflict, region)
	}
	return nil
}

// ToggleTodo ticks or un-ticks one checklist item, which is the smallest real
// piece of evidence a thread can produce.
//
// text guards index. An index alone is a question the caller cannot answer —
// the list it counted may have been re-ordered by anyone since it rendered — and
// ticking the wrong box is worse than refusing, because it is silent AND it
// manufactures evidence of production. This is the same failure the mirror work
// kept running into: a local copy confidently answering a question it does not
// actually know (docs/PLANE-SYNC.md).
func (s *Server) ToggleTodo(ctx context.Context, threadID string, region md.Region, index int, text string, done bool) (domain.ThreadDetail, error) {
	switch region {
	case md.RegionDocument, md.RegionLogbook, md.RegionDoD:
	default:
		return domain.ThreadDetail{}, fmt.Errorf("%w: unknown region %q", errBadRequest, region)
	}
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	_, inst, _, _, wid, err := s.interiorClient(ctx, full)
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

	current, found := md.RegionMarkdown(it.DescriptionHTML, region)
	if !found {
		return domain.ThreadDetail{}, fmt.Errorf("%w: this thread has no %s", errNotFound, region)
	}
	next, err := md.ToggleTodo(current, index, text, done)
	switch {
	case errors.Is(err, md.ErrTodoMoved):
		return domain.ThreadDetail{}, fmt.Errorf("%w: %v", errConflict, err)
	case errors.Is(err, md.ErrNoSuchTodo):
		return domain.ThreadDetail{}, fmt.Errorf("%w: %v", errBadRequest, err)
	case err != nil:
		return domain.ThreadDetail{}, err
	}
	if next == current {
		return s.ThreadDetail(ctx, full) // already in that state
	}
	return s.UpdateThread(ctx, full, domain.ThreadEdit{
		Brief:   pick(region == md.RegionDocument, next),
		Logbook: pick(region == md.RegionLogbook, next),
		DoD:     pick(region == md.RegionDoD, next),
	})
}

func pick(when bool, v string) *string {
	if !when {
		return nil
	}
	return &v
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
	html := md.RenderPlaneHTML(body)

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
	s.rebuildAndNotify(inst, wid, true)

	return s.ThreadDetail(ctx, full)
}

// rebuildAndNotify recomputes the instance snapshot from the mirror and nudges
// every open client. Cheap since PLANE-SYNC Phase 2 — a handful of SQLite
// queries and no network — which is what makes it reasonable on a write path.
//
// production says whether the write could have changed the board. Only a Logbook
// or DoD edit can; a Brief edit cannot, and autosave means those arrive every
// few seconds while somebody types. Skipping the rebuild for them still
// repaints the open thread, which is the part anyone would notice.
func (s *Server) rebuildAndNotify(inst domain.Instance, threadID string, production bool) {
	if production {
		if _, err := s.refreshInstance(context.Background(), inst); err != nil {
			log.Printf("rebuild after artifact write: %v", err)
			return
		}
	}
	s.broadcastThread(threadID)
}

func (s *Server) handleUpdateThread(w http.ResponseWriter, r *http.Request) {
	var in domain.ThreadEdit
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	d, err := s.UpdateThread(r.Context(), r.PathValue("id"), in)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleToggleTodo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Region string `json:"region"`
		Index  int    `json:"index"`
		Text   string `json:"text"`
		Done   bool   `json:"done"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	region, ok := regionByName(in.Region)
	if in.Region == "" {
		region, ok = md.RegionLogbook, true
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown region " + in.Region})
		return
	}
	d, err := s.ToggleTodo(r.Context(), r.PathValue("id"), region, in.Index, in.Text, in.Done)
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
