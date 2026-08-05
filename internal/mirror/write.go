package mirror

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Every upsert here is idempotent: re-applying the same row is a no-op, so a
// retried sync pass, a webhook replay and a full reconcile can all overlap
// without corrupting anything. Batches run in ONE transaction — partly for
// speed, mostly so a read never observes half a page.

// UpsertItems writes work items. now stamps synced_at.
func (m *Mirror) UpsertItems(instance string, items []Item, now time.Time) error {
	return m.batch(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
			INSERT INTO mirror_items(instance, id, project_id, seq, name, state_id, state_name,
			  state_group, priority, parent_id, assignees_json, description_html,
			  description_hash, created_at, updated_at, completed_at, synced_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  project_id = excluded.project_id, seq = excluded.seq, name = excluded.name,
			  state_id = excluded.state_id, state_name = excluded.state_name,
			  state_group = excluded.state_group, priority = excluded.priority,
			  parent_id = excluded.parent_id, assignees_json = excluded.assignees_json,
			  description_html = excluded.description_html,
			  description_hash = excluded.description_hash,
			  created_at = excluded.created_at, updated_at = excluded.updated_at,
			  completed_at = excluded.completed_at, synced_at = excluded.synced_at`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, it := range items {
			if _, err := st.Exec(instance, it.ID, it.ProjectID, it.Seq, it.Name,
				it.StateID, it.StateName, it.StateGroup, it.Priority, it.ParentID,
				encodeIDs(it.Assignees), it.DescriptionHTML, it.DescriptionHash,
				ts(it.CreatedAt), ts(it.UpdatedAt), tsp(it.CompletedAt), ts(now)); err != nil {
				return fmt.Errorf("upsert item %s: %w", it.ID, err)
			}
		}
		return nil
	})
}

// UpsertProjects writes projects.
func (m *Mirror) UpsertProjects(instance string, ps []Project, now time.Time) error {
	return m.batch(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
			INSERT INTO mirror_projects(instance, id, name, identifier, synced_at)
			VALUES(?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  name = excluded.name, identifier = excluded.identifier,
			  synced_at = excluded.synced_at`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, p := range ps {
			if _, err := st.Exec(instance, p.ID, p.Name, p.Identifier, ts(now)); err != nil {
				return fmt.Errorf("upsert project %s: %w", p.ID, err)
			}
		}
		return nil
	})
}

// UpsertModules writes modules (bubbles).
func (m *Mirror) UpsertModules(instance string, ms []Module, now time.Time) error {
	return m.batch(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
			INSERT INTO mirror_modules(instance, id, project_id, name, synced_at)
			VALUES(?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  project_id = excluded.project_id, name = excluded.name,
			  synced_at = excluded.synced_at`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, mo := range ms {
			if _, err := st.Exec(instance, mo.ID, mo.ProjectID, mo.Name, ts(now)); err != nil {
				return fmt.Errorf("upsert module %s: %w", mo.ID, err)
			}
		}
		return nil
	})
}

// SetModuleItems replaces a module's membership wholesale. Membership is a SET,
// not a stream of events — Plane offers no "removed from module" signal, so the
// only way to notice a removal is to replace the whole edge list.
func (m *Mirror) SetModuleItems(instance, moduleID string, itemIDs []string) error {
	return m.batch(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`DELETE FROM mirror_module_items WHERE instance = ? AND module_id = ?`,
			instance, moduleID); err != nil {
			return err
		}
		st, err := tx.Prepare(
			`INSERT OR IGNORE INTO mirror_module_items(instance, module_id, item_id) VALUES(?,?,?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, id := range itemIDs {
			if _, err := st.Exec(instance, moduleID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertStates writes a project's workflow states.
func (m *Mirror) UpsertStates(instance string, ss []State) error {
	return m.batch(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
			INSERT INTO mirror_states(instance, id, project_id, name, state_group, is_default)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  project_id = excluded.project_id, name = excluded.name,
			  state_group = excluded.state_group, is_default = excluded.is_default`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, s := range ss {
			def := 0
			if s.Default {
				def = 1
			}
			if _, err := st.Exec(instance, s.ID, s.ProjectID, s.Name, s.Group, def); err != nil {
				return fmt.Errorf("upsert state %s: %w", s.ID, err)
			}
		}
		return nil
	})
}

// UpsertMembers writes the workspace member registry.
func (m *Mirror) UpsertMembers(instance string, ms []Member) error {
	return m.batch(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
			INSERT INTO mirror_members(instance, id, email, display_name, role)
			VALUES(?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  email = excluded.email, display_name = excluded.display_name,
			  role = excluded.role`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, mm := range ms {
			if _, err := st.Exec(instance, mm.ID, mm.Email, mm.DisplayName, mm.Role); err != nil {
				return fmt.Errorf("upsert member %s: %w", mm.ID, err)
			}
		}
		return nil
	})
}

// UpsertCycles writes a project's cycles.
func (m *Mirror) UpsertCycles(instance string, cs []Cycle) error {
	return m.batch(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
			INSERT INTO mirror_cycles(instance, id, project_id, name, start_date, end_date)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  project_id = excluded.project_id, name = excluded.name,
			  start_date = excluded.start_date, end_date = excluded.end_date`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, c := range cs {
			if _, err := st.Exec(instance, c.ID, c.ProjectID, c.Name, c.StartDate, c.EndDate); err != nil {
				return fmt.Errorf("upsert cycle %s: %w", c.ID, err)
			}
		}
		return nil
	})
}

// ReplaceComments swaps a work item's comment set. Like module membership this
// is a replace rather than a merge: a DELETED comment has no signal of its own,
// so re-stating the whole set is the only way it disappears here too.
func (m *Mirror) ReplaceComments(instance, itemID string, cs []Comment) error {
	return m.batch(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`DELETE FROM mirror_comments WHERE instance = ? AND item_id = ?`,
			instance, itemID); err != nil {
			return err
		}
		// Upsert, not a plain INSERT: every write in this package is idempotent,
		// and a bare INSERT breaks that promise the moment the same comment id
		// arrives under a second item — a retried pass then fails on a UNIQUE
		// violation instead of being a no-op.
		st, err := tx.Prepare(`
			INSERT INTO mirror_comments(instance, id, item_id, actor_id, comment_html, created_at)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  item_id = excluded.item_id, actor_id = excluded.actor_id,
			  comment_html = excluded.comment_html, created_at = excluded.created_at`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, c := range cs {
			if _, err := st.Exec(instance, c.ID, itemID, c.ActorID, c.HTML, ts(c.CreatedAt)); err != nil {
				return fmt.Errorf("insert comment %s: %w", c.ID, err)
			}
		}
		return nil
	})
}

// PruneItems removes mirrored items for a project that are no longer in Plane.
// Deletes are invisible to a delta sync (an absent item reports nothing), so
// they are only ever caught here, during a full reconcile. Returns how many rows
// went — a non-zero count during steady state is worth a log line.
func (m *Mirror) PruneItems(instance, projectID string, keep []string) (int, error) {
	return m.prune(`mirror_items`, `project_id`, instance, projectID, keep)
}

// PruneModules is PruneItems for modules: a bubble deleted in Plane must stop
// appearing on the board.
func (m *Mirror) PruneModules(instance, projectID string, keep []string) (int, error) {
	return m.prune(`mirror_modules`, `project_id`, instance, projectID, keep)
}

func (m *Mirror) prune(table, scopeCol, instance, scope string, keep []string) (int, error) {
	// An empty keep set means the project genuinely has nothing. That is a real
	// state (a project can be emptied), so it is honored rather than treated as
	// "the fetch probably failed" — the caller only reaches a prune after a
	// COMPLETE walk, which is where that judgment belongs.
	q := `DELETE FROM ` + table + ` WHERE instance = ? AND ` + scopeCol + ` = ?`
	args := []any{instance, scope}
	if len(keep) > 0 {
		q += ` AND id NOT IN (?` + strings.Repeat(`,?`, len(keep)-1) + `)`
		for _, k := range keep {
			args = append(args, k)
		}
	}
	res, err := m.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---- cursors ----

// Cursor reads a resource's sync position.
func (m *Mirror) Cursor(instance, resource string) (Cursor, error) {
	var c Cursor
	var wm, full, ok, lastErr string
	err := m.db.QueryRow(
		`SELECT watermark, last_full, last_ok, last_error FROM sync_cursors
		 WHERE instance = ? AND resource = ?`, instance, resource).
		Scan(&wm, &full, &ok, &lastErr)
	if err == sql.ErrNoRows {
		return Cursor{}, nil
	}
	if err != nil {
		return Cursor{}, err
	}
	c.Watermark, c.LastFull, c.LastOK, c.LastError = parseTS(wm), parseTS(full), parseTS(ok), lastErr
	return c, nil
}

// SetCursor writes a resource's sync position.
func (m *Mirror) SetCursor(instance, resource string, c Cursor) error {
	_, err := m.db.Exec(`
		INSERT INTO sync_cursors(instance, resource, watermark, last_full, last_ok, last_error)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(instance, resource) DO UPDATE SET
		  watermark = excluded.watermark, last_full = excluded.last_full,
		  last_ok = excluded.last_ok, last_error = excluded.last_error`,
		instance, resource, ts(c.Watermark), ts(c.LastFull), ts(c.LastOK), c.LastError)
	return err
}

// Reset drops everything mirrored for one instance. The mirror is a projection,
// so this is always safe: the next pass rebuilds it. Cursors go too, or the
// rebuild would resume from a watermark describing data that is no longer there.
func (m *Mirror) Reset(instance string) error {
	return m.batch(func(tx *sql.Tx) error {
		for _, t := range []string{
			"mirror_items", "mirror_modules", "mirror_module_items", "mirror_projects",
			"mirror_states", "mirror_members", "mirror_comments", "mirror_cycles",
			"sync_cursors",
		} {
			if _, err := tx.Exec(`DELETE FROM `+t+` WHERE instance = ?`, instance); err != nil {
				return fmt.Errorf("reset %s: %w", t, err)
			}
		}
		return nil
	})
}

// batch runs fn in a transaction, rolling back on any error.
func (m *Mirror) batch(fn func(*sql.Tx) error) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
