// Package sync fills the mirror from Plane. It is the WRITE side of
// docs/PLANE-SYNC.md: the only place that decides what to fetch and when.
//
// Two passes, deliberately different in cost:
//
//   - Backfill  — a complete walk. Expensive, run once per instance and then
//     periodically to catch what a delta structurally cannot see (deletes,
//     module membership changes).
//   - Delta     — order_by=-updated_at with an early stop at the watermark.
//     ONE page in steady state. This is what runs on the tick.
package sync

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

const (
	// resourceItems tracks the work-item watermark; resourceStructure tracks the
	// last complete walk of projects/modules/states/members.
	resourceItems = "items"
	// resourceStructure tracks the last refresh of the project/module lists.
	resourceStructure = "structure"
	// resourceProject prefixes a PER-PROJECT cursor, which is what makes a full
	// walk resumable across passes when the rate budget runs out mid-way.
	resourceProject = "project:"

	// commentFetchPerPass bounds how many items ONE PASS pulls comments for,
	// across every project — not per project. Comments are the only per-item
	// call left in the sync, so this is the single biggest cost lever: applied
	// per project it would let an hourly reconcile of N projects spend N × the
	// cap, forever. A burst of commenting should spread over passes instead.
	//
	// Note the cost is paid per ITEM, not per comment: an item with no comments
	// still costs a call to discover that. So this bounds calls, not rows.
	commentFetchPerPass = 25
)

// Syncer keeps one server's mirror current.
type Syncer struct {
	m            *mirror.Mirror
	now          func() time.Time
	onChange     func(slug string) // see OnChange
	onReconciled func(slug string) // see OnReconciled
	locks        LockFunc          // see SetLocks
}

// LockFunc reports which mirror columns must not be overwritten for which work
// items, because a write to them is queued and has not reached Plane yet.
type LockFunc func(instance string, ids []string) (map[string]map[string]bool, error)

// SetLocks installs the conflict shield (docs/PLANE-SYNC.md Phase 5).
//
// Plane remains the system of record, so a sync normally overwrites the mirror
// wholesale. The one exception is a field with a write still in flight: applying
// Plane's older value there would make the board visibly revert a change the
// user already made, then flip back when the write lands. The lock lifts as soon
// as the write succeeds or is abandoned, and Plane is truth again.
func (s *Syncer) SetLocks(fn LockFunc) { s.locks = fn }

// New builds a Syncer over a mirror.
func New(m *mirror.Mirror) *Syncer {
	return &Syncer{m: m, now: time.Now}
}

// SetClock overrides the clock (tests).
func (s *Syncer) SetClock(fn func() time.Time) { s.now = fn }

// Result reports what one pass did — the numbers sync-diff and the admin
// surfaces print.
type Result struct {
	Instance  string
	Full      bool
	Projects  int
	Modules   int
	Items     int
	Comments  int
	Pruned    int
	Watermark time.Time
	// Partial means at least one project could not be walked completely,
	// usually a 429. The rows that DID arrive are kept; prunes and the watermark
	// are held back, because "I could not see it" must never be mistaken for
	// "it is gone".
	Partial bool
	Errors  []string
	// Skipped counts projects already walked cleanly within this reconcile
	// window — resumption working, not an error.
	Skipped int
	Took    time.Duration
}

func (r Result) String() string {
	kind := "delta"
	if r.Full {
		kind = "full"
	}
	s := fmt.Sprintf("%s %s: %d projects, %d modules, %d items, %d comments, %d pruned in %s",
		r.Instance, kind, r.Projects, r.Modules, r.Items, r.Comments, r.Pruned,
		r.Took.Round(time.Millisecond))
	if r.Skipped > 0 {
		s += fmt.Sprintf(" [%d project(s) already fresh]", r.Skipped)
	}
	if r.Partial {
		s += fmt.Sprintf(" (PARTIAL: %d project(s) incomplete)", len(r.Errors))
	}
	return s
}

// Backfill walks an instance completely and replaces what it finds. Safe to
// re-run: every write is an upsert, and prunes only happen after a COMPLETE walk
// of a project (a partial walk must never be read as "these rows are gone").
//
// Explicitly requested, so it re-walks EVERY project — an admin asking for a
// backfill means "go and look", not "look at whatever has aged out".
func (s *Syncer) Backfill(ctx context.Context, inst domain.Instance) (Result, error) {
	return s.run(ctx, inst, true, false)
}

// Delta applies only what changed since the watermark. Falls back to a full walk
// when there is no watermark yet, so a fresh instance needs no special casing.
func (s *Syncer) Delta(ctx context.Context, inst domain.Instance) (Result, error) {
	return s.run(ctx, inst, false, false)
}

// reconcile is the AUTOMATIC full walk. Unlike Backfill it resumes: projects
// already walked cleanly inside the current window are skipped, so a pass that
// runs out of rate budget makes forward progress instead of restarting.
func (s *Syncer) reconcile(ctx context.Context, inst domain.Instance) (Result, error) {
	return s.run(ctx, inst, true, true)
}

func (s *Syncer) run(ctx context.Context, inst domain.Instance, full, resume bool) (Result, error) {
	start := s.now()
	res := Result{Instance: inst.Slug, Full: full}

	// Every sync pass is bulk work, INCLUDING an admin-triggered backfill. It is
	// tempting to give an explicitly-requested walk interactive priority, but a
	// walk that spends the whole minute's allowance is exactly what starves the
	// board — and unlike a page load it can afford to wait. Yielding also lets a
	// rate-limited pass finish slowly instead of failing fast.
	ctx = plane.Background(ctx)

	base := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "")
	if !base.Configured() {
		return res, fmt.Errorf("instance %s is not configured for Plane", inst.Slug)
	}

	cur, err := s.m.Cursor(inst.Slug, resourceItems)
	if err != nil {
		return res, fmt.Errorf("read cursor: %w", err)
	}
	// No watermark means nothing has ever been mirrored — a delta would have no
	// floor to stop at, so it IS a full walk. Say so rather than pretending.
	if cur.Watermark.IsZero() {
		full = true
		res.Full = true
	}

	// Structure (the project and module LISTS) is re-read on its own cadence, not
	// on every delta — see StructureInterval. A full pass always refreshes it.
	structCur, err := s.m.Cursor(inst.Slug, resourceStructure)
	if err != nil {
		return res, fmt.Errorf("read structure cursor: %w", err)
	}
	freshStructure := full || structCur.LastOK.IsZero() ||
		s.now().Sub(structCur.LastOK) >= StructureInterval

	// ---- structure: projects, and per project modules + states ----
	projects, err := s.projects(ctx, base, inst, freshStructure)
	if err != nil {
		// Losing the project list is usually a 429, and it used to abort the
		// whole pass — which is the worst possible response, because the mirror
		// ALREADY KNOWS the projects. Fall back to them and carry on degraded;
		// a new project simply waits for a pass that can read the list.
		known, kerr := s.m.Projects(inst.Slug)
		if kerr != nil || len(known) == 0 {
			return res, err
		}
		log.Printf("sync %s: list projects failed (%v); continuing with %d mirrored project(s)",
			inst.Slug, err, len(known))
		projects = known
		res.Partial = true
		res.Errors = append(res.Errors, fmt.Sprintf("projects: %v", err))
	}
	res.Projects = len(projects)

	if full {
		if ms, err := base.Members(ctx); err != nil {
			log.Printf("sync %s: members: %v", inst.Slug, err)
		} else {
			mm := make([]mirror.Member, 0, len(ms))
			for _, m := range ms {
				mm = append(mm, mirror.Member{ID: m.ID, Email: m.Email, DisplayName: m.DisplayName, Role: m.Role})
			}
			if err := s.m.UpsertMembers(inst.Slug, mm); err != nil {
				return res, fmt.Errorf("write members: %w", err)
			}
		}
	}

	if full {
		// A full walk of a busy workspace can sit inside the Phase 0 rate waits
		// for a long time. Announce the start so a slow pass is legible as
		// progress rather than mistaken for a hang.
		log.Printf("sync %s: full walk of %d project(s) starting", inst.Slug, len(projects))
	}

	// One comment budget for the WHOLE pass, shared across projects.
	commentBudget := commentFetchPerPass

	// newest tracks the high-water mark actually OBSERVED. Deliberately not
	// time.Now(): an item written while we were paging would otherwise fall
	// between the last page and the clock, and be skipped forever.
	newest := cur.Watermark

	for _, p := range projects {
		// A full walk is RESUMABLE per project. Without this, a pass that runs out
		// of rate budget halfway restarts from project one next time and — because
		// a partial pass cannot advance the watermark — never finishes at all.
		// Under sustained pressure that is self-reinforcing, which is exactly what
		// the first live run did.
		if full && resume {
			pc, err := s.m.Cursor(inst.Slug, resourceProject+p.ID)
			if err != nil {
				return res, err
			}
			if !pc.LastFull.IsZero() && s.now().Sub(pc.LastFull) < FullInterval {
				res.Skipped++
				continue
			}
		}

		cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, p.ID)

		// One project failing — nearly always a 429 — must not throw away the
		// whole pass. Keep what arrived, mark the pass partial, and let the next
		// pass finish the job. The invariant that makes this safe is below: a
		// project that did not walk cleanly is never pruned.
		fail := func(stage string, err error) {
			res.Partial = true
			res.Errors = append(res.Errors, fmt.Sprintf("%s (%s): %v", short(p.ID), stage, err))
			log.Printf("sync %s: project %s: %s: %v", inst.Slug, short(p.ID), stage, err)
		}

		mods, err := s.modules(ctx, cl, inst, p.ID, full, freshStructure)
		if err != nil {
			fail("modules", err)
			continue
		}
		res.Modules += len(mods)

		if full {
			if err := s.states(ctx, cl, inst, p.ID); err != nil {
				// States are not fatal to the items walk, but a project whose
				// states are unknown must not be pruned on this pass.
				fail("states", err)
				continue
			}
			// Cycles define the heat window (§3.6), but they are OPTIONAL: a
			// project with the cycles feature off has no such endpoint, and heat
			// correctly falls back to the rolling window. Blocking the project on
			// this would leave every cycle-less project permanently unclean, so
			// it stays best-effort exactly as the pre-mirror code had it.
			if err := s.cycles(ctx, cl, inst, p.ID); err != nil {
				log.Printf("sync %s: project %s: cycles: %v (heat falls back to the rolling window)",
					inst.Slug, short(p.ID), err)
			}
		}

		since := cur.Watermark
		if full {
			since = time.Time{}
		}
		rows, err := cl.ListItemsSince(ctx, since)
		if err != nil {
			fail("items", err)
			continue
		}
		// Snapshot the PRIOR body hashes before upserting. The comment rule turns
		// on "did the body change", and the upsert below destroys the evidence —
		// asking afterwards always answers "unchanged".
		prevHash := make(map[string]string, len(rows))
		if !full {
			for _, r := range rows {
				prev, ok, err := s.m.Item(inst.Slug, r.ID)
				if err != nil {
					return res, err // a local sqlite failure IS fatal
				}
				if ok {
					prevHash[r.ID] = prev.DescriptionHash
				}
			}
		}

		// Fields with a queued write are held back — see SetLocks.
		var locked map[string]map[string]bool
		if s.locks != nil && len(rows) > 0 {
			ids := make([]string, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			if lk, lerr := s.locks(inst.Slug, ids); lerr != nil {
				log.Printf("sync %s: reading write locks: %v", inst.Slug, lerr)
			} else {
				locked = lk
			}
		}

		items := make([]mirror.Item, 0, len(rows))
		for _, r := range rows {
			if r.UpdatedAt.After(newest) {
				newest = r.UpdatedAt
			}
			it := toItem(r, p.ID)
			if locked[r.ID]["state"] {
				// Keep what the mirror already says about state; everything else
				// on the row still applies.
				if prev, ok, err := s.m.Item(inst.Slug, r.ID); err == nil && ok {
					it.StateID, it.StateName, it.StateGroup = prev.StateID, prev.StateName, prev.StateGroup
				}
			}
			items = append(items, it)
		}
		if len(items) > 0 {
			if err := s.m.UpsertItems(inst.Slug, items, s.now()); err != nil {
				return res, fmt.Errorf("write items: %w", err)
			}
		}
		res.Items += len(items)

		// Prunes ride ONLY on a complete walk. rows is every item in the project
		// here, so anything mirrored and absent is genuinely deleted.
		if full {
			keep := make([]string, 0, len(rows))
			for _, r := range rows {
				keep = append(keep, r.ID)
			}
			n, err := s.m.PruneItems(inst.Slug, p.ID, keep)
			if err != nil {
				return res, fmt.Errorf("prune items: %w", err)
			}
			res.Pruned += n
			modKeep := make([]string, 0, len(mods))
			for _, mo := range mods {
				modKeep = append(modKeep, mo.ID)
			}
			n, err = s.m.PruneModules(inst.Slug, p.ID, modKeep)
			if err != nil {
				return res, fmt.Errorf("prune modules: %w", err)
			}
			res.Pruned += n
		}

		// Comments: fetched only for items the delta actually named, and only
		// those whose body did NOT change — a body edit explains the updated_at
		// on its own, so re-reading its comments would be wasted budget.
		// Comments are a FILL-IN, not structure. A comment fetch that fails or
		// runs out of budget must not mark the project unclean: the board and
		// sync-diff do not read comments at all, and the pulse only needs
		// accuracy for dying threads (THREAD-LIFECYCLE.md). Blocking a project's
		// cursor on them would stall the whole reconcile behind the cheapest-to-
		// defer work.
		n, err := s.comments(ctx, cl, inst, rows, prevHash, full, &commentBudget)
		if err != nil {
			log.Printf("sync %s: project %s: comments: %v", inst.Slug, short(p.ID), err)
		}
		res.Comments += n

		// This project walked cleanly — record it so a later pass can skip it and
		// spend its budget on the projects still outstanding.
		if full {
			if err := s.m.SetCursor(inst.Slug, resourceProject+p.ID,
				mirror.Cursor{LastFull: s.now(), LastOK: s.now()}); err != nil {
				return res, fmt.Errorf("write project cursor: %w", err)
			}
		}
	}

	// A partial pass must not advance the watermark: the newest thing we managed
	// to read says nothing about the projects we could not reach, and moving the
	// mark would skip their changes permanently. Items already written stay —
	// they are correct, just incomplete.
	if res.Partial {
		res.Watermark = cur.Watermark
		cur.LastError = strings.Join(res.Errors, "; ")
		if err := s.m.SetCursor(inst.Slug, resourceItems, cur); err != nil {
			return res, fmt.Errorf("write cursor: %w", err)
		}
		res.Took = s.now().Sub(start)
		// Not an error: partial progress is the designed outcome under rate
		// pressure, and the caller can see Partial/Errors.
		return res, nil
	}

	// Only stamp the structure cursor when the lists were actually re-read AND
	// the pass completed cleanly; stamping it after a partial pass would skip the
	// next refresh on the strength of data we failed to fetch.
	if freshStructure {
		if err := s.m.SetCursor(inst.Slug, resourceStructure,
			mirror.Cursor{LastOK: s.now(), LastFull: structCur.LastFull}); err != nil {
			return res, fmt.Errorf("write structure cursor: %w", err)
		}
	}

	cur.Watermark = newest
	cur.LastOK = s.now()
	cur.LastError = ""
	if full {
		cur.LastFull = s.now()
	}
	if err := s.m.SetCursor(inst.Slug, resourceItems, cur); err != nil {
		return res, fmt.Errorf("write cursor: %w", err)
	}
	res.Watermark = newest
	res.Took = s.now().Sub(start)
	return res, nil
}

// projects resolves the instance's projects (respecting a pinned one) and
// mirrors them.
func (s *Syncer) projects(ctx context.Context, base *plane.Client, inst domain.Instance, refresh bool) ([]mirror.Project, error) {
	if !refresh {
		// Serve the list we already have. A genuinely new project waits for the
		// next structure refresh, which is the point of the cadence.
		if known, err := s.m.Projects(inst.Slug); err == nil && len(known) > 0 {
			return known, nil
		}
	}
	if inst.Project != "" {
		// A pinned instance still needs a name for the board, so take it from
		// the mirror if we have it and don't spend a call re-deriving it.
		ps, _ := s.m.Projects(inst.Slug)
		for _, p := range ps {
			if p.ID == inst.Project {
				return []mirror.Project{p}, nil
			}
		}
		p := mirror.Project{ID: inst.Project}
		if err := s.m.UpsertProjects(inst.Slug, []mirror.Project{p}, s.now()); err != nil {
			return nil, err
		}
		return []mirror.Project{p}, nil
	}
	ps, err := base.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]mirror.Project, 0, len(ps))
	for _, p := range ps {
		out = append(out, mirror.Project{ID: p.ID, Name: p.Name, Identifier: p.Identifier})
	}
	if err := s.m.UpsertProjects(inst.Slug, out, s.now()); err != nil {
		return nil, fmt.Errorf("write projects: %w", err)
	}
	return out, nil
}

// modules mirrors a project's modules, and on a full pass their membership too.
//
// Membership is refreshed only on a full pass BY DESIGN: moving an item between
// modules may not bump the item's updated_at, so a delta cannot be trusted to
// notice it. That makes the reconcile interval a correctness knob, which is
// recorded in PLANE-SYNC.md rather than hidden here.
func (s *Syncer) modules(ctx context.Context, cl *plane.Client, inst domain.Instance, projID string, full, refresh bool) ([]mirror.Module, error) {
	if !refresh {
		if known, err := s.m.Modules(inst.Slug, projID); err == nil {
			return known, nil
		}
	}
	ms, err := cl.ListModules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list modules for project %s: %w", projID, err)
	}
	out := make([]mirror.Module, 0, len(ms))
	for _, m := range ms {
		out = append(out, mirror.Module{ID: m.ID, ProjectID: projID, Name: m.Name})
	}
	if err := s.m.UpsertModules(inst.Slug, out, s.now()); err != nil {
		return nil, fmt.Errorf("write modules: %w", err)
	}
	if !full {
		return out, nil
	}
	for _, m := range out {
		ids, err := cl.ModuleItemIDs(ctx, m.ID)
		if err != nil {
			return nil, fmt.Errorf("module %s membership: %w", m.ID, err)
		}
		if err := s.m.SetModuleItems(inst.Slug, m.ID, ids); err != nil {
			return nil, fmt.Errorf("write module membership: %w", err)
		}
	}
	return out, nil
}

func (s *Syncer) states(ctx context.Context, cl *plane.Client, inst domain.Instance, projID string) error {
	ss, err := cl.ListStates(ctx)
	if err != nil {
		return fmt.Errorf("list states for project %s: %w", projID, err)
	}
	out := make([]mirror.State, 0, len(ss))
	for _, st := range ss {
		out = append(out, mirror.State{
			ID: st.ID, ProjectID: projID, Name: st.Name, Group: st.Group, Default: st.Default,
		})
	}
	if err := s.m.UpsertStates(inst.Slug, out); err != nil {
		return fmt.Errorf("write states: %w", err)
	}
	return nil
}

func (s *Syncer) cycles(ctx context.Context, cl *plane.Client, inst domain.Instance, projID string) error {
	cs, err := cl.ListCycles(ctx)
	if err != nil {
		return fmt.Errorf("list cycles for project %s: %w", projID, err)
	}
	out := make([]mirror.Cycle, 0, len(cs))
	for _, c := range cs {
		out = append(out, mirror.Cycle{
			ID: c.ID, ProjectID: projID, Name: c.Name,
			StartDate: c.StartDate, EndDate: c.EndDate,
		})
	}
	if err := s.m.UpsertCycles(inst.Slug, out); err != nil {
		return fmt.Errorf("write cycles: %w", err)
	}
	return nil
}

// comments fetches comments for the items worth fetching them for.
//
// The rule comes straight from the 2026-08-04 probe: a comment bumps its parent
// item's updated_at, so a commented item shows up in the delta. If the body hash
// ALSO changed, the bump is explained by the edit and we skip — the comment, if
// any, will still be there next time the item surfaces for another reason, and
// the pulse only needs accuracy for dying threads (THREAD-LIFECYCLE.md).
//
// prevHash holds the body hashes from BEFORE this pass wrote its items; reading
// them from the mirror here would always compare a row against itself.
func (s *Syncer) comments(ctx context.Context, cl *plane.Client, inst domain.Instance, rows []plane.ItemRow, prevHash map[string]string, full bool, budget *int) (int, error) {
	if budget != nil && *budget <= 0 {
		return 0, nil
	}
	type cand struct {
		id string
		at time.Time
	}
	var want []cand
	for _, r := range rows {
		if full {
			want = append(want, cand{r.ID, r.UpdatedAt})
			continue
		}
		// Unseen item, or an update whose body did not change → the bump came
		// from something else, plausibly a comment.
		prev, seen := prevHash[r.ID]
		if !seen || prev == mirror.HashBody(r.DescriptionHTML) {
			want = append(want, cand{r.ID, r.UpdatedAt})
		}
	}
	// Newest first, so a truncated pass covers what matters most.
	sort.Slice(want, func(i, j int) bool { return want[i].at.After(want[j].at) })
	if budget != nil && len(want) > *budget {
		// Say what was dropped. A silent cap reads as "covered everything".
		log.Printf("sync %s: %d item(s) want comments, budget allows %d this pass; the rest wait",
			inst.Slug, len(want), *budget)
		want = want[:*budget]
	}
	n := 0
	for _, c := range want {
		if budget != nil {
			*budget--
		}
		cs, err := cl.ListComments(ctx, c.id)
		if err != nil {
			// One unreadable item must not abort the whole pass.
			log.Printf("sync %s: comments for %s: %v", inst.Slug, c.id, err)
			continue
		}
		out := make([]mirror.Comment, 0, len(cs))
		for _, cm := range cs {
			out = append(out, mirror.Comment{
				ID: cm.ID, ItemID: c.id, ActorID: cm.ActorID, HTML: cm.HTML, CreatedAt: cm.CreatedAt,
			})
		}
		if err := s.m.ReplaceComments(inst.Slug, c.id, out); err != nil {
			return n, fmt.Errorf("write comments for %s: %w", c.id, err)
		}
		n += len(out)
	}
	return n, nil
}

func toItem(r plane.ItemRow, projID string) mirror.Item {
	if r.ProjectID != "" {
		projID = r.ProjectID
	}
	return mirror.Item{
		ID: r.ID, ProjectID: projID, Seq: r.Sequence, Name: r.Name,
		StateID: r.StateID, StateName: r.StateName, StateGroup: r.StateGroup,
		Priority: r.Priority, ParentID: r.Parent, Assignees: r.Assignees,
		DescriptionHTML: r.DescriptionHTML, DescriptionHash: mirror.HashBody(r.DescriptionHTML),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, CompletedAt: r.CompletedAt,
	}
}
