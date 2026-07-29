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
CREATE TABLE IF NOT EXISTS notification_reads (
  email           TEXT NOT NULL,      -- stable per-person key (cross-instance)
  notification_id INTEGER NOT NULL,
  read_at         TEXT NOT NULL,
  PRIMARY KEY (email, notification_id)
);
CREATE TABLE IF NOT EXISTS member_prefs (
  email          TEXT PRIMARY KEY,
  notify_enabled INTEGER NOT NULL DEFAULT 0
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

func inClause(instances []string) (string, []any) {
	ph := make([]string, len(instances))
	args := make([]any, len(instances))
	for i, inst := range instances {
		ph[i] = "?"
		args[i] = inst
	}
	return strings.Join(ph, ","), args
}

// ListNotifications returns notifications for the given instances with a per-
// person unread flag (via email). unreadOnly filters to unread; newest first.
func (s *Store) ListNotifications(email string, instances []string, unreadOnly bool, limit int) ([]domain.Notification, error) {
	if len(instances) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	in, inArgs := inClause(instances)
	q := `SELECT n.id, n.created_at, n.instance, n.bubble_id, n.bubble_name, n.kind, n.message,
	             CASE WHEN r.notification_id IS NULL THEN 1 ELSE 0 END AS unread
	      FROM notifications n
	      LEFT JOIN notification_reads r ON r.notification_id = n.id AND r.email = ?
	      WHERE n.instance IN (` + in + `)`
	if unreadOnly {
		q += ` AND r.notification_id IS NULL`
	}
	q += ` ORDER BY n.id DESC LIMIT ?`

	args := append([]any{email}, inArgs...)
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Notification
	for rows.Next() {
		var n domain.Notification
		var unread int
		if err := rows.Scan(&n.ID, &n.At, &n.Instance, &n.BubbleID, &n.BubbleName, &n.Kind, &n.Message, &unread); err != nil {
			return nil, err
		}
		n.Unread = unread == 1
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadCount counts a person's unread notifications across their instances.
func (s *Store) UnreadCount(email string, instances []string) (int, error) {
	if len(instances) == 0 {
		return 0, nil
	}
	in, inArgs := inClause(instances)
	q := `SELECT COUNT(*) FROM notifications n
	      LEFT JOIN notification_reads r ON r.notification_id = n.id AND r.email = ?
	      WHERE n.instance IN (` + in + `) AND r.notification_id IS NULL`
	args := append([]any{email}, inArgs...)
	var c int
	err := s.db.QueryRow(q, args...).Scan(&c)
	return c, err
}

// MarkRead records read receipts for the given notification ids.
func (s *Store) MarkRead(email string, ids []int64, at string) error {
	for _, id := range ids {
		if _, err := s.db.Exec(
			`INSERT OR IGNORE INTO notification_reads(email, notification_id, read_at) VALUES(?, ?, ?)`,
			email, id, at); err != nil {
			return err
		}
	}
	return nil
}

// MarkAllRead marks every notification in the person's instances as read.
func (s *Store) MarkAllRead(email string, instances []string, at string) error {
	if len(instances) == 0 {
		return nil
	}
	in, inArgs := inClause(instances)
	q := `INSERT OR IGNORE INTO notification_reads(email, notification_id, read_at)
	      SELECT ?, n.id, ? FROM notifications n WHERE n.instance IN (` + in + `)`
	args := append([]any{email, at}, inArgs...)
	_, err := s.db.Exec(q, args...)
	return err
}

// NotifyEnabled reports whether a person has opted into notifications.
func (s *Store) NotifyEnabled(email string) (bool, error) {
	var v int
	err := s.db.QueryRow(`SELECT notify_enabled FROM member_prefs WHERE email = ?`, email).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v == 1, nil
}

// SetNotifyEnabled sets a person's opt-in preference.
func (s *Store) SetNotifyEnabled(email string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO member_prefs(email, notify_enabled) VALUES(?, ?)
		 ON CONFLICT(email) DO UPDATE SET notify_enabled = excluded.notify_enabled`,
		email, v)
	return err
}
