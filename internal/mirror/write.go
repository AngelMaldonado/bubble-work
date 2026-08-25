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
			  state_group, priority, parent_id, assignees_json, labels_json, description_html,
			  description_hash, created_at, updated_at, completed_at, synced_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  project_id = excluded.project_id, seq = excluded.seq, name = excluded.name,
			  state_id = excluded.state_id, state_name = excluded.state_name,
			  state_group = excluded.state_group, priority = excluded.priority,
			  parent_id = excluded.parent_id, assignees_json = excluded.assignees_json,
			  labels_json = excluded.labels_json,
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
				encodeIDs(it.Assignees), encodeIDs(it.Labels), it.DescriptionHTML, it.DescriptionHash,
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

// UpsertComment writes ONE comment without disturbing the item's others.
//
// ReplaceComments is the sync's tool: it re-states a whole set, which is the
// only way a deletion lands. This is for the write path — when the server itself
// posts a comment it already holds the created row, so the discussion can show
// it immediately instead of waiting for the next sync pass to discover it.
func (m *Mirror) UpsertComment(instance, itemID string, c Comment) error {
	_, err := m.db.Exec(`
		INSERT INTO mirror_comments(instance, id, item_id, actor_id, comment_html, created_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(instance, id) DO UPDATE SET
		  item_id = excluded.item_id, actor_id = excluded.actor_id,
		  comment_html = excluded.comment_html, created_at = excluded.created_at`,
		instance, c.ID, itemID, c.ActorID, c.HTML, ts(c.CreatedAt))
	return err
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

// DeleteModule drops a bubble's rows: the module itself and every membership
// row pointing at it. The ITEMS survive — a module is a grouping, not a
// container, and Plane keeps them too (docs/journal/ARTIFACT-EDITING.md).
func (m *Mirror) DeleteModule(instance, moduleID string) error {
	return m.batch(func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM mirror_module_items WHERE instance = ? AND module_id = ?`,
			`DELETE FROM mirror_modules WHERE instance = ? AND id = ?`,
		} {
			if _, err := tx.Exec(q, instance, moduleID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteItems drops work items and everything that hangs off them: their
// comments and their membership of any bubble.
//
// The mirror is a projection, so this is not the source of truth — but leaving
// the rows behind would keep a deleted thread on the board until a full
// reconcile pruned it, which is the same "the local copy is confidently wrong"
// failure the write-through fixes exist to avoid.
func (m *Mirror) DeleteItems(instance string, itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return m.batch(func(tx *sql.Tx) error {
		for _, id := range itemIDs {
			for _, q := range []string{
				`DELETE FROM mirror_comments WHERE instance = ? AND item_id = ?`,
				`DELETE FROM mirror_module_items WHERE instance = ? AND item_id = ?`,
				`DELETE FROM mirror_items WHERE instance = ? AND id = ?`,
			} {
				if _, err := tx.Exec(q, instance, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// SetProjectMembers replaces one project's membership. Replace rather than
// merge: somebody REMOVED from a project has to stop seeing it, and a merge
// would never take anyone away.
func (m *Mirror) SetProjectMembers(instance, projectID string, memberIDs []string) error {
	return m.batch(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`DELETE FROM mirror_project_members WHERE instance = ? AND project_id = ?`,
			instance, projectID); err != nil {
			return err
		}
		st, err := tx.Prepare(
			`INSERT OR IGNORE INTO mirror_project_members(instance, project_id, member_id) VALUES(?,?,?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, id := range memberIDs {
			if _, err := st.Exec(instance, projectID, id); err != nil {
				return err
			}
		}
		// Record that we READ it, separately from what we read. A project whose
		// membership genuinely came back empty is a different fact from one we
		// have never fetched.
		_, err = tx.Exec(
			`INSERT INTO mirror_project_member_sync(instance, project_id, synced_at) VALUES(?,?,?)
			 ON CONFLICT(instance, project_id) DO UPDATE SET synced_at = excluded.synced_at`,
			instance, projectID, time.Now().UTC().Format(time.RFC3339))
		return err
	})
}

// DeleteProject drops a workspace and everything mirrored beneath it.
//
// Every table that keys on the project, plus the items — which key on the
// instance rather than the project, so they have to be found through the
// project first or they would be orphaned rows that still answer queries.
func (m *Mirror) DeleteProject(instance, projectID string) error {
	items, err := m.Items(instance, projectID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	if err := m.DeleteItems(instance, ids); err != nil {
		return err
	}
	return m.batch(func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM mirror_module_items WHERE instance = ? AND module_id IN
			   (SELECT id FROM mirror_modules WHERE instance = ? AND project_id = ?)`,
		} {
			if _, err := tx.Exec(q, instance, instance, projectID); err != nil {
				return err
			}
		}
		for _, q := range []string{
			`DELETE FROM mirror_modules WHERE instance = ? AND project_id = ?`,
			`DELETE FROM mirror_states WHERE instance = ? AND project_id = ?`,
			`DELETE FROM mirror_cycles WHERE instance = ? AND project_id = ?`,
			`DELETE FROM mirror_project_members WHERE instance = ? AND project_id = ?`,
			`DELETE FROM mirror_project_member_sync WHERE instance = ? AND project_id = ?`,
			`DELETE FROM mirror_projects WHERE instance = ? AND id = ?`,
		} {
			if _, err := tx.Exec(q, instance, projectID); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertLabels writes a project's label catalogue.
func (m *Mirror) UpsertLabels(instance string, ls []Label) error {
	return m.batch(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
			INSERT INTO mirror_labels(instance, id, project_id, name, color)
			VALUES(?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  project_id = excluded.project_id, name = excluded.name, color = excluded.color`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, l := range ls {
			if _, err := st.Exec(instance, l.ID, l.ProjectID, l.Name, l.Color); err != nil {
				return fmt.Errorf("upsert label %s: %w", l.ID, err)
			}
		}
		return nil
	})
}

// ReplaceLinks makes the mirror agree with Plane about one item's links. Wholesale,
// because a link that vanished in Plane must vanish here too — a stale "evidence
// published" would keep a thread warm on a URL nobody can open.
func (m *Mirror) ReplaceLinks(instance, itemID string, ls []Link) error {
	return m.batch(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`DELETE FROM mirror_item_links WHERE instance = ? AND item_id = ?`,
			instance, itemID); err != nil {
			return err
		}
		st, err := tx.Prepare(`
			INSERT INTO mirror_item_links(instance, id, item_id, url, title, created_at)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(instance, id) DO UPDATE SET
			  item_id = excluded.item_id, url = excluded.url, title = excluded.title,
			  created_at = excluded.created_at`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, l := range ls {
			if _, err := st.Exec(instance, l.ID, itemID, l.URL, l.Title, ts(l.CreatedAt)); err != nil {
				return fmt.Errorf("insert link %s: %w", l.ID, err)
			}
		}
		return nil
	})
}

// ReplaceRelations makes the mirror agree with Plane about one item's relations.
func (m *Mirror) ReplaceRelations(instance, itemID string, rs []Relation) error {
	return m.batch(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`DELETE FROM mirror_item_relations WHERE instance = ? AND item_id = ?`,
			instance, itemID); err != nil {
			return err
		}
		st, err := tx.Prepare(`
			INSERT INTO mirror_item_relations(instance, item_id, relation_type, related_id)
			VALUES(?,?,?,?)
			ON CONFLICT(instance, item_id, relation_type, related_id) DO NOTHING`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, r := range rs {
			if _, err := st.Exec(instance, itemID, r.Type, r.RelatedID); err != nil {
				return fmt.Errorf("insert relation %s->%s: %w", itemID, r.RelatedID, err)
			}
		}
		return nil
	})
}
