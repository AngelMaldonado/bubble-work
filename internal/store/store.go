// Package store is the server's authoritative overlay: the Bubble contract
// (§4), member registry (§9.3) and anything Plane can't express. It uses SQLite
// (pure-Go modernc driver) so the server stays a single static binary (§9.6).
package store

import (
	"database/sql"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

const schema = `
CREATE TABLE IF NOT EXISTS bubble_contracts (
  bubble_id TEXT PRIMARY KEY,      -- "<instance-slug>:<project-id>:<module-id>"
  outcome   TEXT,
  owner     TEXT,
  closure   TEXT,
  closed    INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS plane_instances (
  slug      TEXT PRIMARY KEY,      -- "ayetec", "cuby"
  name      TEXT,
  base_url  TEXT NOT NULL,         -- https://plane.ayetec.space
  api_key   TEXT NOT NULL,
  workspace TEXT NOT NULL,         -- workspace slug
  project   TEXT NOT NULL          -- pinned project id, or '' for the whole workspace
);
CREATE TABLE IF NOT EXISTS bubble_state (
  bubble_id  TEXT PRIMARY KEY,     -- last-known lifecycle, for transition detection
  lifecycle  TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS notifications (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at  TEXT NOT NULL,
  instance    TEXT NOT NULL,
  bubble_id   TEXT NOT NULL,
  bubble_name TEXT NOT NULL,
  kind        TEXT NOT NULL,
  message     TEXT NOT NULL
);
`

// Store wraps the SQLite connection.
type Store struct{ db *sql.DB }

// Contract is a bubble's §4 overlay, owned by the server.
type Contract struct {
	Outcome string
	Owner   string
	Closure string
	Closed  bool
}

// Open opens (and migrates) the SQLite database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// GetContract returns a bubble's overlay, or ok=false when none is set yet.
func (s *Store) GetContract(bubbleID string) (Contract, bool, error) {
	var c Contract
	var closed int
	err := s.db.QueryRow(
		`SELECT COALESCE(outcome,''), COALESCE(owner,''), COALESCE(closure,''), closed
		   FROM bubble_contracts WHERE bubble_id = ?`, bubbleID,
	).Scan(&c.Outcome, &c.Owner, &c.Closure, &closed)
	if err == sql.ErrNoRows {
		return Contract{}, false, nil
	}
	if err != nil {
		return Contract{}, false, err
	}
	c.Closed = closed == 1
	return c, true, nil
}

// SetContract upserts a bubble's §4 contract.
func (s *Store) SetContract(bubbleID string, c Contract) error {
	closed := 0
	if c.Closed {
		closed = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO bubble_contracts(bubble_id, outcome, owner, closure, closed)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(bubble_id) DO UPDATE SET
		   outcome = excluded.outcome, owner = excluded.owner,
		   closure = excluded.closure, closed = excluded.closed`,
		bubbleID, c.Outcome, c.Owner, c.Closure, closed,
	)
	return err
}

// SetClosed flips just the closed flag (used by close_bubble, §5.3).
func (s *Store) SetClosed(bubbleID string, closed bool) error {
	ci := 0
	if closed {
		ci = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO bubble_contracts(bubble_id, closed) VALUES(?, ?)
		 ON CONFLICT(bubble_id) DO UPDATE SET closed = excluded.closed`,
		bubbleID, ci,
	)
	return err
}

// ---- Plane instances (§9.7 federation) ----

// AddInstance registers or updates a Plane instance (upsert by slug).
func (s *Store) AddInstance(i domain.Instance) error {
	_, err := s.db.Exec(
		`INSERT INTO plane_instances(slug, name, base_url, api_key, workspace, project)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(slug) DO UPDATE SET
		   name=excluded.name, base_url=excluded.base_url, api_key=excluded.api_key,
		   workspace=excluded.workspace, project=excluded.project`,
		i.Slug, i.Name, i.BaseURL, i.APIKey, i.Workspace, i.Project,
	)
	return err
}

func scanInstances(rows *sql.Rows) ([]domain.Instance, error) {
	defer rows.Close()
	var out []domain.Instance
	for rows.Next() {
		var i domain.Instance
		if err := rows.Scan(&i.Slug, &i.Name, &i.BaseURL, &i.APIKey, &i.Workspace, &i.Project); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ListInstances returns every registered instance (admin view; includes keys).
func (s *Store) ListInstances() ([]domain.Instance, error) {
	rows, err := s.db.Query(
		`SELECT slug, COALESCE(name,''), base_url, api_key, workspace, project
		 FROM plane_instances ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	return scanInstances(rows)
}

// InstanceExists reports whether a slug is registered.
func (s *Store) InstanceExists(slug string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM plane_instances WHERE slug = ?`, slug).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// RemoveInstance deletes an instance.
func (s *Store) RemoveInstance(slug string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM plane_instances WHERE slug = ?`, slug)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---- Lifecycle tracking & notifications (§5 push) ----

// GetLifecycle returns a bubble's last-recorded lifecycle, ok=false if none.
func (s *Store) GetLifecycle(bubbleID string) (string, bool, error) {
	var lc string
	err := s.db.QueryRow(`SELECT lifecycle FROM bubble_state WHERE bubble_id = ?`, bubbleID).Scan(&lc)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return lc, true, nil
}

// SetLifecycle records a bubble's current lifecycle (upsert).
func (s *Store) SetLifecycle(bubbleID, lifecycle, updatedAt string) error {
	_, err := s.db.Exec(
		`INSERT INTO bubble_state(bubble_id, lifecycle, updated_at) VALUES(?, ?, ?)
		 ON CONFLICT(bubble_id) DO UPDATE SET lifecycle=excluded.lifecycle, updated_at=excluded.updated_at`,
		bubbleID, lifecycle, updatedAt)
	return err
}

// AddNotification appends a notification.
func (s *Store) AddNotification(n domain.Notification) error {
	_, err := s.db.Exec(
		`INSERT INTO notifications(created_at, instance, bubble_id, bubble_name, kind, message)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		n.At, n.Instance, n.BubbleID, n.BubbleName, n.Kind, n.Message)
	return err
}

// ListNotifications returns recent notifications for the given instances,
// newest first. An empty instance list returns nothing.
func (s *Store) ListNotifications(instances []string, limit int) ([]domain.Notification, error) {
	if len(instances) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	ph := make([]string, len(instances))
	args := make([]any, 0, len(instances)+1)
	for i, inst := range instances {
		ph[i] = "?"
		args = append(args, inst)
	}
	args = append(args, limit)
	q := `SELECT created_at, instance, bubble_id, bubble_name, kind, message
	      FROM notifications WHERE instance IN (` + strings.Join(ph, ",") + `)
	      ORDER BY id DESC LIMIT ?`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(&n.At, &n.Instance, &n.BubbleID, &n.BubbleName, &n.Kind, &n.Message); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
