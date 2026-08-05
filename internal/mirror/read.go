package mirror

import (
	"database/sql"
	"time"
)

// Reads never touch Plane. Phase 1 uses these only for sync-diff; Phase 2 points
// the board at them and Phase 3 the thread interior.

const itemCols = `id, project_id, seq, name, state_id, state_name, state_group,
	priority, parent_id, assignees_json, description_html, description_hash,
	created_at, updated_at, completed_at`

func scanItem(rows *sql.Rows) (Item, error) {
	var it Item
	var assignees, created, updated, completed string
	err := rows.Scan(&it.ID, &it.ProjectID, &it.Seq, &it.Name, &it.StateID,
		&it.StateName, &it.StateGroup, &it.Priority, &it.ParentID, &assignees,
		&it.DescriptionHTML, &it.DescriptionHash, &created, &updated, &completed)
	if err != nil {
		return Item{}, err
	}
	it.Assignees = decodeIDs(assignees)
	it.CreatedAt, it.UpdatedAt, it.CompletedAt = parseTS(created), parseTS(updated), parseTSP(completed)
	return it, nil
}

// Items returns every mirrored work item for a project.
func (m *Mirror) Items(instance, projectID string) ([]Item, error) {
	rows, err := m.db.Query(
		`SELECT `+itemCols+` FROM mirror_items WHERE instance = ? AND project_id = ?
		 ORDER BY seq`, instance, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ModuleItems returns the work items belonging to a module — the board's unit of
// work. Ordered by Plane's own sequence so the timeline reads the same as there.
func (m *Mirror) ModuleItems(instance, moduleID string) ([]Item, error) {
	rows, err := m.db.Query(
		`SELECT `+itemCols+` FROM mirror_items i
		 JOIN mirror_module_items mi
		   ON mi.instance = i.instance AND mi.item_id = i.id
		 WHERE i.instance = ? AND mi.module_id = ?
		 ORDER BY i.seq`, instance, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Item returns one work item.
func (m *Mirror) Item(instance, id string) (Item, bool, error) {
	rows, err := m.db.Query(
		`SELECT `+itemCols+` FROM mirror_items WHERE instance = ? AND id = ?`, instance, id)
	if err != nil {
		return Item{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return Item{}, false, rows.Err()
	}
	it, err := scanItem(rows)
	return it, err == nil, err
}

// Children returns a work item's sub-items (revision artifacts).
func (m *Mirror) Children(instance, parentID string) ([]Item, error) {
	rows, err := m.db.Query(
		`SELECT `+itemCols+` FROM mirror_items
		 WHERE instance = ? AND parent_id = ? ORDER BY seq`, instance, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Projects returns the mirrored projects for an instance.
func (m *Mirror) Projects(instance string) ([]Project, error) {
	rows, err := m.db.Query(
		`SELECT id, name, identifier FROM mirror_projects WHERE instance = ? ORDER BY name`, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Identifier); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Modules returns the mirrored modules (bubbles) for a project.
func (m *Mirror) Modules(instance, projectID string) ([]Module, error) {
	rows, err := m.db.Query(
		`SELECT id, project_id, name FROM mirror_modules
		 WHERE instance = ? AND project_id = ? ORDER BY name`, instance, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Module
	for rows.Next() {
		var mo Module
		if err := rows.Scan(&mo.ID, &mo.ProjectID, &mo.Name); err != nil {
			return nil, err
		}
		out = append(out, mo)
	}
	return out, rows.Err()
}

// States returns a project's workflow states keyed by state id.
func (m *Mirror) States(instance, projectID string) (map[string]State, error) {
	rows, err := m.db.Query(
		`SELECT id, project_id, name, state_group, is_default FROM mirror_states
		 WHERE instance = ? AND project_id = ?`, instance, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]State{}
	for rows.Next() {
		var s State
		var def int
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Group, &def); err != nil {
			return nil, err
		}
		s.Default = def == 1
		out[s.ID] = s
	}
	return out, rows.Err()
}

// Members returns an instance's member registry keyed by Plane user id. This is
// what removes the per-auth "Members on every instance" storm in Phase 4.
func (m *Mirror) Members(instance string) (map[string]Member, error) {
	rows, err := m.db.Query(
		`SELECT id, email, display_name, role FROM mirror_members WHERE instance = ?`, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Member{}
	for rows.Next() {
		var mm Member
		if err := rows.Scan(&mm.ID, &mm.Email, &mm.DisplayName, &mm.Role); err != nil {
			return nil, err
		}
		out[mm.ID] = mm
	}
	return out, rows.Err()
}

// Cycles returns a project's cycles.
func (m *Mirror) Cycles(instance, projectID string) ([]Cycle, error) {
	rows, err := m.db.Query(
		`SELECT id, project_id, name, start_date, end_date FROM mirror_cycles
		 WHERE instance = ? AND project_id = ?`, instance, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cycle
	for rows.Next() {
		var c Cycle
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Name, &c.StartDate, &c.EndDate); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Comments returns a work item's comments, oldest first (as Plane paginates).
func (m *Mirror) Comments(instance, itemID string) ([]Comment, error) {
	rows, err := m.db.Query(
		`SELECT id, item_id, actor_id, comment_html, created_at FROM mirror_comments
		 WHERE instance = ? AND item_id = ? ORDER BY created_at`, instance, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		var created string
		if err := rows.Scan(&c.ID, &c.ItemID, &c.ActorID, &c.HTML, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTS(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

// LastCommentAt is the thread pulse, answered locally. This single query is what
// retires probePulse and its 20-calls-per-tick budget (PLANE-SYNC.md Phase 3).
func (m *Mirror) LastCommentAt(instance, itemID string) (time.Time, error) {
	var s sql.NullString
	err := m.db.QueryRow(
		`SELECT max(created_at) FROM mirror_comments WHERE instance = ? AND item_id = ?`,
		instance, itemID).Scan(&s)
	if err != nil || !s.Valid {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return parseTS(s.String), nil
}

// AllItemIDs returns every mirrored work-item id for an instance — the live set
// the overlay retention prunes against.
func (m *Mirror) AllItemIDs(instance string) ([]string, error) {
	rows, err := m.db.Query(`SELECT id FROM mirror_items WHERE instance = ?`, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Counts is a coarse census, for sync-diff and the admin surface.
type Counts struct {
	Projects int
	Modules  int
	Items    int
	States   int
	Members  int
	Comments int
	Cycles   int
}

// Counts reports how much is mirrored for an instance.
func (m *Mirror) Counts(instance string) (Counts, error) {
	var c Counts
	for _, q := range []struct {
		table string
		into  *int
	}{
		{"mirror_projects", &c.Projects},
		{"mirror_modules", &c.Modules},
		{"mirror_items", &c.Items},
		{"mirror_states", &c.States},
		{"mirror_members", &c.Members},
		{"mirror_comments", &c.Comments},
		{"mirror_cycles", &c.Cycles},
	} {
		if err := m.db.QueryRow(
			`SELECT count(*) FROM `+q.table+` WHERE instance = ?`, instance).Scan(q.into); err != nil {
			return Counts{}, err
		}
	}
	return c, nil
}
