package plane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// This file is what the mirror syncs FROM (docs/PLANE-SYNC.md Phase 1). It is
// deliberately separate from the request-path helpers above: those answer "what
// does this screen need", these answer "what changed".

// ItemRow is a work item as the mirror stores it — every field the board and the
// interior read, from ONE list page. Probed 2026-08-04: the list carries the
// full, untruncated description_html (a 14k body came back byte-identical to
// GET), so there is no per-item fetch behind this.
type ItemRow struct {
	ID              string
	ProjectID       string
	Name            string
	DescriptionHTML string
	Sequence        int
	Priority        string
	StateID         string
	StateName       string // localized — display only, never matched on
	StateGroup      string // backlog|unstarted|started|completed|cancelled
	Parent          string
	Assignees       []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
}

// itemFields is everything the mirror stores. state comes back as an object
// rather than a bare uuid because of expand=state, which removes the ListStates
// join from the read path (verified to compose with fields= on 2026-08-04).
const itemFields = "id,project,name,description_html,sequence_id,priority,state,parent,assignees,created_at,updated_at,completed_at"

// errStopPaging unwinds getPaged early once we reach items older than the
// watermark. Never escapes this file.
var errStopPaging = errors.New("stop paging")

// ListItemsSince pages the project's work items newest-first and stops at the
// first item not newer than `since`. That is the whole delta strategy: Plane has
// no updated_at__gte filter, but order_by=-updated_at plus an early stop is
// equivalent and costs ONE page in steady state.
//
// Pass a zero `since` for a full walk (the Phase 1 backfill).
//
// Note the ordering contract: callers must treat the newest UpdatedAt they see
// as the next watermark, NOT time.Now() — an item written while we paged would
// otherwise be skipped forever.
func (c *Client) ListItemsSince(ctx context.Context, since time.Time) ([]ItemRow, error) {
	q := url.Values{}
	q.Set("order_by", "-updated_at")
	q.Set("expand", "state")
	q.Set("fields", itemFields)

	var out []ItemRow
	err := c.getPaged(ctx, c.projectBase()+"/work-items/?"+q.Encode(), func(raw json.RawMessage) error {
		var page []struct {
			ID              string     `json:"id"`
			Project         string     `json:"project"`
			Name            string     `json:"name"`
			DescriptionHTML string     `json:"description_html"`
			SequenceID      int        `json:"sequence_id"`
			Priority        string     `json:"priority"`
			Parent          *string    `json:"parent"`
			Assignees       []string   `json:"assignees"`
			CreatedAt       time.Time  `json:"created_at"`
			UpdatedAt       time.Time  `json:"updated_at"`
			CompletedAt     *time.Time `json:"completed_at"`
			State           struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Group string `json:"group"`
			} `json:"state"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		for _, it := range page {
			// Newest-first, so the first item at or before the watermark means
			// everything after it is older too.
			if !since.IsZero() && !it.UpdatedAt.After(since) {
				return errStopPaging
			}
			r := ItemRow{
				ID: it.ID, ProjectID: it.Project, Name: it.Name,
				DescriptionHTML: it.DescriptionHTML, Sequence: it.SequenceID,
				Priority: it.Priority, StateID: it.State.ID, StateName: it.State.Name,
				StateGroup: it.State.Group, Assignees: it.Assignees,
				CreatedAt: it.CreatedAt, UpdatedAt: it.UpdatedAt, CompletedAt: it.CompletedAt,
			}
			if it.Parent != nil {
				r.Parent = *it.Parent
			}
			out = append(out, r)
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopPaging) {
		return nil, err
	}
	return out, nil
}

// ModuleItemIDs returns just the work-item ids belonging to a module — the
// bubble-membership edge. Slimmed to one field because that is genuinely all
// the mirror needs; the items themselves arrive via ListItemsSince.
func (c *Client) ModuleItemIDs(ctx context.Context, moduleID string) ([]string, error) {
	var out []string
	err := c.getPaged(ctx, c.projectBase()+"/modules/"+moduleID+"/module-issues/?fields=id", func(raw json.RawMessage) error {
		var page []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		for _, it := range page {
			out = append(out, it.ID)
		}
		return nil
	})
	return out, err
}

// ProjectRow is a project as the mirror stores it.
type ProjectRow struct {
	ID         string
	Name       string
	Identifier string
}

// ModuleRow is a module (a Bubble) as the mirror stores it.
type ModuleRow struct {
	ID        string
	ProjectID string
	Name      string
}

// String makes a failed sync legible in logs without a fmt.Sprintf at each site.
func (r ItemRow) String() string {
	return fmt.Sprintf("item %s (#%d %q)", r.ID, r.Sequence, r.Name)
}
