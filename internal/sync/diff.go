package sync

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// Diff is the Phase 1 acceptance gate: proof the mirror agrees with Plane before
// anything is allowed to READ from the mirror.
//
// It compares the SUBSTRATE — the module list, module membership, and the item
// fields the board and interior are built from — rather than the derived board.
// That is deliberate. The board is a pure function of this substrate, so equal
// substrate means an equal board by construction, and a mismatch points at the
// sync bug directly instead of at some downstream symptom. Diffing rendered
// bubbles would conflate "the mirror is wrong" with "the board builder changed".
//
// This runs the LIVE path, so it costs the very calls Phase 1 exists to remove.
// It is a shadow-period tool, not something to leave running.

// Finding is one disagreement.
type Finding struct {
	Kind  string `json:"kind"`  // missing-in-mirror | missing-in-plane | field
	Scope string `json:"scope"` // module | membership | item
	ID    string `json:"id"`
	Label string `json:"label"`
	Field string `json:"field,omitempty"`
	Plane string `json:"plane,omitempty"`
	Local string `json:"local,omitempty"`
}

func (f Finding) String() string {
	switch f.Kind {
	case "missing-in-mirror":
		return fmt.Sprintf("%s %s (%s) is in Plane but not mirrored", f.Scope, f.Label, short(f.ID))
	case "missing-in-plane":
		return fmt.Sprintf("%s %s (%s) is mirrored but gone from Plane", f.Scope, f.Label, short(f.ID))
	default:
		return fmt.Sprintf("%s %s (%s): %s plane=%q mirror=%q",
			f.Scope, f.Label, short(f.ID), f.Field, f.Plane, f.Local)
	}
}

// DiffReport is the outcome of one comparison.
type DiffReport struct {
	Instance  string        `json:"instance"`
	Projects  int           `json:"projects"`
	Modules   int           `json:"modules"`
	Items     int           `json:"items"`
	Findings  []Finding     `json:"findings"`
	Watermark time.Time     `json:"watermark"`
	LastFull  time.Time     `json:"last_full"`
	LastOK    time.Time     `json:"last_ok"`
	LastError string        `json:"last_error,omitempty"`
	Took      time.Duration `json:"took"`
}

// Clean reports whether the mirror matched Plane exactly.
func (r DiffReport) Clean() bool { return len(r.Findings) == 0 }

// Diff compares an instance's mirror against a live fetch.
func (s *Syncer) Diff(ctx context.Context, inst domain.Instance) (DiffReport, error) {
	start := s.now()
	rep := DiffReport{Instance: inst.Slug}

	// Same reasoning as run(): this walks Plane completely, so it yields rather
	// than competing with page loads. It is slower under rate pressure and that
	// is the correct trade — a verification tool must not degrade the thing it
	// is verifying.
	ctx = plane.Background(ctx)

	base := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "")
	if !base.Configured() {
		return rep, fmt.Errorf("instance %s is not configured for Plane", inst.Slug)
	}

	if cur, err := s.m.Cursor(inst.Slug, resourceItems); err == nil {
		rep.Watermark, rep.LastFull, rep.LastOK, rep.LastError =
			cur.Watermark, cur.LastFull, cur.LastOK, cur.LastError
	}

	projects := []string{inst.Project}
	if inst.Project == "" {
		ps, err := base.ListProjects(ctx)
		if err != nil {
			return rep, fmt.Errorf("list projects: %w", err)
		}
		projects = projects[:0]
		for _, p := range ps {
			projects = append(projects, p.ID)
		}
	}
	rep.Projects = len(projects)

	for _, projID := range projects {
		cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, projID)

		// ---- modules ----
		liveMods, err := cl.ListModules(ctx)
		if err != nil {
			return rep, fmt.Errorf("list modules: %w", err)
		}
		localMods, err := s.m.Modules(inst.Slug, projID)
		if err != nil {
			return rep, err
		}
		rep.Modules += len(liveMods)

		localModByID := map[string]mirror.Module{}
		for _, m := range localMods {
			localModByID[m.ID] = m
		}
		for _, lm := range liveMods {
			mm, ok := localModByID[lm.ID]
			if !ok {
				rep.Findings = append(rep.Findings, Finding{
					Kind: "missing-in-mirror", Scope: "module", ID: lm.ID, Label: lm.Name})
				continue
			}
			if mm.Name != lm.Name {
				rep.Findings = append(rep.Findings, Finding{
					Kind: "field", Scope: "module", ID: lm.ID, Label: lm.Name,
					Field: "name", Plane: lm.Name, Local: mm.Name})
			}
			delete(localModByID, lm.ID)
		}
		for id, mm := range localModByID {
			rep.Findings = append(rep.Findings, Finding{
				Kind: "missing-in-plane", Scope: "module", ID: id, Label: mm.Name})
		}

		// ---- module membership ----
		for _, lm := range liveMods {
			liveIDs, err := cl.ModuleItemIDs(ctx, lm.ID)
			if err != nil {
				return rep, fmt.Errorf("module %s membership: %w", lm.ID, err)
			}
			localItems, err := s.m.ModuleItems(inst.Slug, lm.ID)
			if err != nil {
				return rep, err
			}
			localIDs := make([]string, 0, len(localItems))
			for _, it := range localItems {
				localIDs = append(localIDs, it.ID)
			}
			if a, b := sortedCopy(liveIDs), sortedCopy(localIDs); !equalStrings(a, b) {
				rep.Findings = append(rep.Findings, Finding{
					Kind: "field", Scope: "membership", ID: lm.ID, Label: lm.Name,
					Field: "items", Plane: summarize(a), Local: summarize(b)})
			}
		}

		// ---- items ----
		liveRows, err := cl.ListItemsSince(ctx, time.Time{})
		if err != nil {
			return rep, fmt.Errorf("list items: %w", err)
		}
		rep.Items += len(liveRows)
		localItems, err := s.m.Items(inst.Slug, projID)
		if err != nil {
			return rep, err
		}
		localByID := map[string]mirror.Item{}
		for _, it := range localItems {
			localByID[it.ID] = it
		}
		for _, lr := range liveRows {
			li, ok := localByID[lr.ID]
			label := fmt.Sprintf("#%d %s", lr.Sequence, lr.Name)
			if !ok {
				rep.Findings = append(rep.Findings, Finding{
					Kind: "missing-in-mirror", Scope: "item", ID: lr.ID, Label: label})
				continue
			}
			rep.Findings = append(rep.Findings, compareItem(lr, li, label)...)
			delete(localByID, lr.ID)
		}
		for id, li := range localByID {
			rep.Findings = append(rep.Findings, Finding{
				Kind: "missing-in-plane", Scope: "item", ID: id,
				Label: fmt.Sprintf("#%d %s", li.Seq, li.Name)})
		}
	}

	rep.Took = s.now().Sub(start)
	return rep, nil
}

// compareItem checks the fields the board and the interior actually read. Adding
// a field to the mirror means adding it here, or sync-diff will pass while the
// mirror quietly drifts.
func compareItem(p plane.ItemRow, l mirror.Item, label string) []Finding {
	var out []Finding
	add := func(field, pv, lv string) {
		if pv != lv {
			out = append(out, Finding{
				Kind: "field", Scope: "item", ID: p.ID, Label: label,
				Field: field, Plane: pv, Local: lv})
		}
	}
	add("name", p.Name, l.Name)
	add("seq", fmt.Sprint(p.Sequence), fmt.Sprint(l.Seq))
	// Group, never name — Plane state names are localized ("En Progreso").
	add("state_group", p.StateGroup, l.StateGroup)
	add("priority", p.Priority, l.Priority)
	add("parent", p.Parent, l.ParentID)
	add("assignees", summarize(sortedCopy(p.Assignees)), summarize(sortedCopy(l.Assignees)))
	add("created_at", tsOf(p.CreatedAt), tsOf(l.CreatedAt))
	add("completed_at", tspOf(p.CompletedAt), tspOf(l.CompletedAt))
	// The body is compared by hash, not by text: a 14k Logbook in a diff line is
	// unreadable, and the hash is what the syncer keys its decisions on anyway.
	add("body", mirror.HashBody(p.DescriptionHTML), l.DescriptionHash)
	return out
}

func tsOf(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func tspOf(t *time.Time) string {
	if t == nil {
		return ""
	}
	return tsOf(*t)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// summarize renders an id set compactly — full ids would make a finding
// unreadable, and the count is usually the tell.
func summarize(ids []string) string {
	if len(ids) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, short(id))
	}
	return fmt.Sprintf("%d [%s]", len(ids), strings.Join(parts, " "))
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
