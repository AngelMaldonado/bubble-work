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
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// Editing a thread's artifacts (docs/journal/MCP-ACCESS.md steps 1-2).
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

// docRegionTail is the stored region holding what follows the last section. It is
// not editable and not part of editRegions on purpose: it exists so the document
// store is a COMPLETE copy of the page (docs/decisions/0001), not so anyone can
// write to it by name.
const docRegionTail = "tail"

// storedBody rebuilds a thread's markdown from the document store, and reports
// whether the store had it. This is the read path: when it returns true, showing
// the thread parses no Plane HTML at all.
func (s *Server) storedBody(wid string) (map[string]store.ThreadDoc, string, bool) {
	docs, err := s.store.ThreadDocs(wid)
	if err != nil {
		log.Printf("docs: read %s: %v", wid, err)
		return nil, "", false
	}
	if len(docs) == 0 {
		return nil, "", false // never edited since adoption: fall back to the mirror
	}
	body := md.AssembleBody(
		docs[string(md.RegionDocument)].Markdown,
		docs[string(md.RegionLogbook)].Markdown,
		docs[string(md.RegionDoD)].Markdown,
		docs[docRegionTail].Markdown,
	)
	if strings.TrimSpace(body) == "" {
		return nil, "", false
	}
	return docs, body, true
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
// bytes (docs/journal/ARTIFACT-EDITING.md Phase 1).
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

	// Named sections, resolved against the DOCUMENT region. A section is not a
	// region of its own — it is a heading inside the document — so this produces
	// the document's new markdown and then takes the ordinary region path, which
	// means the splice still rewrites only the blocks that actually changed.
	if len(edit.Sections) > 0 {
		if edit.Brief != nil {
			return domain.ThreadDetail{}, fmt.Errorf(
				"%w: the document was given both a replacement and a section edit — send one or the other",
				errBadRequest)
		}
		if _, ok := patched["brief"]; ok {
			return domain.ThreadDetail{}, fmt.Errorf(
				"%w: the document was given both a quoted edit and a section edit — send one or the other",
				errBadRequest)
		}
		doc, _ := md.RegionMarkdown(body, md.RegionDocument)
		next, err := applySectionEdits(doc, edit.Sections)
		if err != nil {
			return domain.ThreadDetail{}, err
		}
		if patched == nil {
			patched = map[string]string{}
		}
		patched["brief"] = next
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
		if err := s.checkBase(wid, body, r.region, edit.Base[r.name]); err != nil {
			return domain.ThreadDetail{}, err
		}
		spliced, err := md.Splice(body, r.region, *want)
		if err != nil {
			return domain.ThreadDetail{}, fmt.Errorf("%w: %v", errBadRequest, err)
		}
		if spliced != body {
			body = spliced
			// ANY changed region is production (docs/decisions/0004), the Brief
			// included: writing is the work here. progressEvidenceFromMirror agrees
			// by fingerprinting the whole document rather than only the plan.
			production = true
		}
	}

	// The house standard (spec §3.1, §3.2), checked on the RESULT rather than on
	// what was sent: an edit changes part of a page, and whether the page still
	// holds together is a property of the whole thing.
	//
	// Refused by default on every surface. §9.3 makes an agent deliberately
	// indistinguishable from the person it acts for, so there is no "is this an
	// agent" to branch on — strict is the default and the web editor opts out,
	// because autosave that stops mid-sentence is its own kind of broken.
	var warnings []md.Finding
	if body != it.DescriptionHTML {
		// Only what this write INTRODUCED. A page that was already messy is not
		// this caller's fault, and refusing their one-line fix over it would make
		// the tool unusable on everything that already exists.
		broke := md.NewFindings(md.Lint(md.FromHTML(it.DescriptionHTML)), md.Lint(md.FromHTML(body)))
		// Only REFUSALS can fail a write (docs/decisions/0003). The advisory rules —
		// a Brief getting long, a Definition of Done reading like a task list — are
		// reported next to a write that succeeded; refusing on craft would teach
		// people to pass lenient on everything, which costs the real rules too.
		if refusals := md.Refusals(broke); len(refusals) > 0 && !edit.Lenient {
			return domain.ThreadDetail{}, fmt.Errorf("%w: %v", errBadRequest, md.LintError(refusals))
		}
		warnings = broke
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

	d, err := s.ThreadDetail(ctx, full)
	if err != nil {
		return d, err
	}
	d.Warnings = warnings
	return d, nil
}

// writeBody pushes a new description to Plane and records it locally.
//
// Shared by every path that rewrites a body — editing a region, ticking a todo,
// deleting a section — so the mirror write-through can never be forgotten by one
// of them. Not optimistic: Plane accepted the write and returned the row, so
// this IS what Plane holds.
func (s *Server) writeBody(ctx context.Context, cl *plane.Client, inst domain.Instance, it mirror.Item, html string) error {
	// The document goes into the RECORD first, and Plane is published to second
	// (docs/decisions/0001). That order is the whole point of the inversion: the
	// write is safe before the network is involved, so Plane being down costs a
	// retry rather than the edit.
	s.recordDocs(ctx, it.ID, html)

	it.DescriptionHTML = html
	it.DescriptionHash = mirror.HashBody(html)

	updatedAt, err := cl.SetWorkItemBody(ctx, it.ID, html)
	if err != nil {
		// Optimistic: queue the publication and carry on. The mirror keeps our
		// version — reads come from the store anyway, and the field lock stops an
		// incoming sync from reverting it while the entry is pending, which is the
		// same shield a queued state move already uses.
		if _, derr := s.store.DiscardPendingDocs(inst.Slug, it.ID); derr != nil {
			log.Printf("outbox: superseding queued publications for %s: %v", it.ID, derr)
		}
		if _, qerr := s.store.Enqueue(store.OutboxEntry{
			Instance: inst.Slug, Kind: store.OutDoc, TargetID: it.ID,
			Payload: map[string]string{"html": html}, FieldLock: "description",
			CreatedAt: s.now(), LastError: err.Error(),
		}); qerr != nil {
			// Nothing queued and nothing published: the markdown is stored, but
			// Plane will not converge on its own. That is worth failing the write
			// over, because the caller can retry.
			return fmt.Errorf("publish to Plane failed (%v) and could not be queued: %w", err, qerr)
		}
		log.Printf("docs: publication of %s queued — Plane said: %v", it.ID, err)
		if merr := s.mirror.UpsertItems(inst.Slug, []mirror.Item{it}, s.now()); merr != nil {
			log.Printf("mirror: record queued body %s: %v", it.ID, merr)
		}
		return nil
	}

	if err := s.store.SetPublished(it.ID, it.DescriptionHash, s.now()); err != nil {
		log.Printf("docs: record publish of %s: %v", it.ID, err)
	}
	if !updatedAt.IsZero() {
		it.UpdatedAt = updatedAt
	}
	if err := s.mirror.UpsertItems(inst.Slug, []mirror.Item{it}, s.now()); err != nil {
		log.Printf("mirror: record body write %s: %v", it.ID, err)
	}
	return nil
}

// recordDocs stores the markdown of every region of a body (docs/decisions/0001).
//
// It hangs off writeBody deliberately: that is the single place any artifact write
// reaches Plane — an edit, a section, a ticked todo, a deleted region — so one hook
// keeps the document store complete without every caller having to remember.
//
// Nothing READS these rows yet. Accumulating the record first, and switching reads
// over separately, is what makes the change reversible: until the read path moves,
// a wrong row is invisible rather than damaging.
func (s *Server) recordDocs(ctx context.Context, wid, html string) {
	actor, _ := domain.ActorFrom(ctx)
	by := actor.Label()
	now := s.now()

	docs := make([]store.ThreadDoc, 0, len(editRegions)+1)
	for _, r := range editRegions {
		m, ok := md.RegionMarkdown(html, r.region)
		if !ok {
			// An absent region is stored as empty, which PutThreadDocs treats as a
			// removal. A Logbook that was deleted must not linger in the record.
			m = ""
		}
		docs = append(docs, store.ThreadDoc{
			ThreadID: wid, Region: string(r.region), Markdown: m,
			Hash: md.Hash(m), UpdatedAt: now, UpdatedBy: by,
		})
	}
	// Whatever the author wrote AFTER the last section belongs to no region, so it
	// has to be captured separately or the record would be a lossy copy of the page.
	tail, _ := md.TailMarkdown(html)
	docs = append(docs, store.ThreadDoc{
		ThreadID: wid, Region: docRegionTail, Markdown: tail,
		Hash: md.Hash(tail), UpdatedAt: now, UpdatedBy: by,
	})
	if err := s.store.PutThreadDocs(docs); err != nil {
		log.Printf("docs: record %s: %v", wid, err)
	}
}

// explainNoTodo turns "no todo at that position" into something a caller can act on.
func (s *Server) explainNoTodo(it mirror.Item, region md.Region, index, have int) error {
	body := md.FromHTML(it.DescriptionHTML)
	if dup := md.Refusals(md.Lint(body)); len(dup) > 0 {
		for _, f := range dup {
			if f.Rule == "duplicate-section" {
				return fmt.Errorf("no todo at index %d: the %s region is unreachable because this page "+
					"has duplicate section headings — %s", index, region, f.Message)
			}
		}
	}
	if have <= 0 {
		what := fmt.Sprintf("the %s has no checklist items at all", region)
		if have < 0 {
			what = fmt.Sprintf("this thread has no %s section", region)
		}
		// Where the items actually are. A caller off by one region gets an answer
		// instead of a dead end.
		var elsewhere []string
		for _, other := range []md.Region{md.RegionLogbook, md.RegionDoD, md.RegionDocument} {
			if other == region {
				continue
			}
			if m, ok := md.RegionMarkdown(it.DescriptionHTML, other); ok {
				if n := len(md.ParseTodos(m)); n > 0 {
					elsewhere = append(elsewhere, fmt.Sprintf("%s has %d", other, n))
				}
			}
		}
		hint := ""
		if len(elsewhere) > 0 {
			hint = fmt.Sprintf(" The %s — did you mean one of those? read_thread reports each todo's region.",
				strings.Join(elsewhere, ", the "))
		}
		return fmt.Errorf("no todo at index %d: %s.%s", index, what, hint)
	}
	return fmt.Errorf("no todo at index %d: the %s has %d (valid indices are 0..%d, counted within that "+
		"region only — read_thread reports them per region)", index, region, have, have-1)
}

// closestLine names the line in a region that most resembles what a caller quoted,
// so a failed exact edit points at the text instead of just refusing.
//
// Deliberately crude — token overlap, not an edit distance. The job is to answer
// "did you mean this one?", and a caller who quoted from the rendered artifact
// instead of from regions[] needs to SEE the difference, not a similarity score.
func closestLine(region, want string) string {
	norm := func(s string) []string { return strings.Fields(strings.ToLower(s)) }
	wantTok := norm(want)
	if len(wantTok) == 0 {
		return ""
	}
	inWant := make(map[string]bool, len(wantTok))
	for _, w := range wantTok {
		inWant[w] = true
	}
	best, bestScore := "", 0.0
	for _, ln := range strings.Split(region, "\n") {
		toks := norm(ln)
		if len(toks) == 0 {
			continue
		}
		hits := 0
		for _, tk := range toks {
			if inWant[tk] {
				hits++
			}
		}
		if score := float64(hits) / float64(len(wantTok)); score > bestScore {
			best, bestScore = strings.TrimSpace(ln), score
		}
	}
	if bestScore < 0.4 {
		return ""
	}
	return best
}

// applyRegionEdits turns find-and-replace edits into new region markdown.
//
// Everything is resolved before ANY write: a set that fails half way through
// leaves nothing applied, because a partly-applied patch is worse than a
// refused one — the caller cannot tell which half landed.
// reservedSections are the headings that ARE regions. Writing them as sections
// would append a second "## Logbook" inside the document rather than touching
// the real one, so they are refused with a pointer at the right field.
var reservedSections = map[string]string{
	"logbook":            "logbook",
	"bitácora":           "logbook",
	"bitacora":           "logbook",
	"definition of done": "dod",
	"dod":                "dod",
}

// applySectionEdits folds named-section writes into the document's markdown.
func applySectionEdits(doc string, edits []domain.SectionEdit) (string, error) {
	for _, e := range edits {
		title := strings.TrimSpace(e.Title)
		if title == "" {
			return "", fmt.Errorf("%w: a section edit needs a title", errBadRequest)
		}
		if field, bad := reservedSections[strings.ToLower(title)]; bad {
			return "", fmt.Errorf(
				"%w: %q is a region of its own — send it as %q instead, so it is recorded as production",
				errBadRequest, title, field)
		}
		if e.Delete {
			_, rest, found := md.ExtractSection(doc, title)
			if !found {
				return "", fmt.Errorf("%w: there is no section called %q", errBadRequest, title)
			}
			doc = rest
			continue
		}
		if e.Markdown == nil {
			return "", fmt.Errorf(
				"%w: section %q was given neither content nor delete", errBadRequest, title)
		}
		doc = md.ReplaceSection(doc, title, *e.Markdown)
	}
	return strings.TrimSpace(doc), nil
}

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
			// there, which is a bad request rather than a server failure.
			//
			// The common cause is not a typo: it is text copied from the RENDERED
			// artifact rather than from regions[].markdown, which is the only text a
			// splice diffs against. So the message names the closest line it can find
			// and says where to copy from — an agent recovers from an error that
			// carries the fix, and gives up on one that does not.
			hint := ""
			for _, e := range byRegion[name] {
				if near := closestLine(current, e.Old); near != "" {
					hint = fmt.Sprintf(" The closest line in the %s is: %q. Quote from "+
						"regions[%q].markdown, not from artifacts[].markdown.", name, near, name)
					break
				}
			}
			return nil, fmt.Errorf("%w: %s: %v%s", errBadRequest, name, err, hint)
		}
		out[name] = next
	}
	return out, nil
}

// checkBase enforces optimistic concurrency for one region. An absent base means
// the caller did not read the page first — legitimate for CLI and MCP writes —
// and is accepted rather than guessed at.
// It compares against the DOCUMENT STORE when the thread has one, because that is
// what the read served and therefore what the editor's base was computed from
// (docs/decisions/0001). Comparing against Plane's HTML for a stored thread would
// invent conflicts whenever the two representations differ by a byte.
func (s *Server) checkBase(wid, descriptionHTML string, region md.Region, base string) error {
	if base == "" {
		return nil
	}
	if docs, _, ok := s.storedBody(wid); ok {
		if d, has := docs[string(region)]; has {
			if d.Hash != base {
				return fmt.Errorf("%w: the %s changed while you were editing it", errConflict, region)
			}
			return nil
		}
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
// actually know (docs/journal/PLANE-SYNC.md).
func (s *Server) ToggleTodo(ctx context.Context, threadID string, region md.Region, index int, text string, done bool) (domain.ThreadDetail, error) {
	return s.ToggleTodos(ctx, threadID, []domain.TodoToggle{
		{Region: string(region), Index: index, Text: text, Done: done},
	})
}

// ToggleTodos ticks or un-ticks SEVERAL items in one operation.
//
// One call, one Plane write. Ticking five items used to be five calls, each with its
// own read, splice and write — five round trips for the caller and five writes against
// a 60-per-minute budget, for a change nobody would think of as five changes.
//
// It is all-or-nothing. A set that fails half way through leaves the caller unable to
// say which half landed, which is the same reasoning applyRegionEdits already follows.
// Indices stay valid across the batch because toggling rewrites a line in place — it
// never reorders or removes one.
func (s *Server) ToggleTodos(ctx context.Context, threadID string, items []domain.TodoToggle) (domain.ThreadDetail, error) {
	if len(items) == 0 {
		return domain.ThreadDetail{}, fmt.Errorf("%w: no items to toggle", errBadRequest)
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

	// Resolve every item against the region text as it accumulates, so two toggles in
	// the same region both land.
	next := map[md.Region]string{}
	for _, item := range items {
		region := md.Region(strings.TrimSpace(strings.ToLower(item.Region)))
		if region == "brief" { // the API's older word for the document
			region = md.RegionDocument
		}
		if region == "" {
			// A thread whose plan lives under its author's own headings has no
			// Logbook to default to (docs/decisions/0005), and defaulting there
			// anyway is how "no such todo" gets returned for an item that is
			// plainly on the page.
			region = md.RegionLogbook
			if _, found := md.RegionMarkdown(it.DescriptionHTML, md.RegionLogbook); !found {
				region = md.RegionDocument
			}
		}
		switch region {
		case md.RegionDocument, md.RegionLogbook, md.RegionDoD:
		default:
			return domain.ThreadDetail{}, fmt.Errorf("%w: unknown region %q", errBadRequest, item.Region)
		}

		current, ok := next[region]
		if !ok {
			var found bool
			current, found = md.RegionMarkdown(it.DescriptionHTML, region)
			if !found {
				return domain.ThreadDetail{}, fmt.Errorf("%w: %v",
					errNotFound, s.explainNoTodo(it, region, item.Index, -1))
			}
		}
		// In the document a bullet is prose, so only real checkboxes are addressable
		// there — the same rule ParseChecklist reads it by, or an index would mean
		// two different things at the two ends of the call.
		strict := region == md.RegionDocument
		out, err := md.ToggleTodoIn(current, item.Index, item.Text, item.Done, strict)
		switch {
		case errors.Is(err, md.ErrTodoMoved):
			return domain.ThreadDetail{}, fmt.Errorf("%w: %v — read the thread again and quote the item you meant",
				errConflict, err)
		case errors.Is(err, md.ErrNoSuchTodo):
			have := len(md.ParseTodos(current))
			if strict {
				have = len(md.ParseChecklist(current))
			}
			return domain.ThreadDetail{}, fmt.Errorf("%w: %v",
				errBadRequest, s.explainNoTodo(it, region, item.Index, have))
		case err != nil:
			return domain.ThreadDetail{}, err
		}
		next[region] = out
	}

	edit := domain.ThreadEdit{}
	changed := false
	for region, out := range next {
		before, _ := md.RegionMarkdown(it.DescriptionHTML, region)
		if out == before {
			continue // already in that state
		}
		changed = true
		switch region {
		case md.RegionDocument:
			edit.Brief = &out
		case md.RegionLogbook:
			edit.Logbook = &out
		case md.RegionDoD:
			edit.DoD = &out
		}
	}
	if !changed {
		return s.ThreadDetail(ctx, full)
	}
	return s.UpdateThread(ctx, full, edit)
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
// production says whether the write could have changed the board. Since
// docs/decisions/0004 every body edit can, the Brief included, so this is now true
// for any changed region.
//
// The cost that used to justify the exception is real and has not gone away:
// autosave means Brief edits arrive every few seconds while somebody types, and each
// one rebuilds the instance snapshot. It is a local rebuild (SQLite only, no Plane
// traffic) and singleflight coalesces concurrent ones, so it is affordable at
// current sizes — but debouncing it is the obvious next move if a typing session
// ever shows up in a profile. Skipping a rebuild would only DELAY heat, never lose
// it: the fingerprint diff compares against what is stored, so whenever the next
// rebuild runs it still fires.
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
	// One item at the top level, or several under `items` — ticking five things is not
	// five changes, and it should not be five writes to Plane either.
	var in struct {
		Region string              `json:"region"`
		Index  int                 `json:"index"`
		Text   string              `json:"text"`
		Done   bool                `json:"done"`
		Items  []domain.TodoToggle `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	items := in.Items
	if len(items) == 0 {
		items = []domain.TodoToggle{{Region: in.Region, Index: in.Index, Text: in.Text, Done: in.Done}}
	}
	d, err := s.ToggleTodos(r.Context(), r.PathValue("id"), items)
	if writeErr(w, err) {
		return
	}
	// The web repaints from the full interior; the compact confirmation rides along
	// for callers that only want to know what landed.
	writeJSON(w, http.StatusOK, struct {
		domain.ThreadDetail
		Confirmed domain.TodoResult `json:"confirmed"`
	}{d, domain.NewTodoResult(d, items)})
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
