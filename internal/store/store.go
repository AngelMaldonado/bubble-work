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
  closed    INTEGER NOT NULL DEFAULT 0,
  stage     TEXT NOT NULL DEFAULT ''  -- explicit stage overlay: '' | 'reviewed' (§ web-ui)
);
CREATE TABLE IF NOT EXISTS plane_instances (
  slug           TEXT PRIMARY KEY,  -- "ayetec", "cuby"
  name           TEXT,
  base_url       TEXT NOT NULL,     -- https://plane.ayetec.space
  api_key        TEXT NOT NULL,
  workspace      TEXT NOT NULL,     -- workspace slug
  project        TEXT NOT NULL,     -- pinned project id, or '' for the whole workspace
  webhook_secret TEXT               -- HMAC secret for inbound Plane webhooks (§6)
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
CREATE TABLE IF NOT EXISTS bubble_snapshots (
  slug       TEXT PRIMARY KEY,   -- instance slug
  bubbles    TEXT NOT NULL,      -- JSON-encoded []domain.Bubble (materialized read model)
  updated_at TEXT NOT NULL       -- RFC3339 timestamp of the fetch
);
CREATE TABLE IF NOT EXISTS comment_reads (
  instance    TEXT NOT NULL,     -- instance slug (Plane has no comment reactions API)
  comment_id  TEXT NOT NULL,     -- Plane comment id
  reader_id   TEXT NOT NULL,     -- Plane user id of the reader
  reader_name TEXT NOT NULL,     -- display name at read time
  read_at     TEXT NOT NULL,     -- RFC3339
  PRIMARY KEY (instance, comment_id, reader_id)
);
CREATE TABLE IF NOT EXISTS server_settings (
  key   TEXT PRIMARY KEY,        -- e.g. "tuning" (the buoyancy calibration)
  value TEXT NOT NULL            -- opaque JSON, owned by the caller
);
CREATE TABLE IF NOT EXISTS kiosk_tokens (
  token      TEXT PRIMARY KEY,   -- server-issued read-only display credential (§9 Phase 9)
  instance   TEXT NOT NULL,      -- the instance slug this token may view
  name       TEXT NOT NULL,      -- human label (e.g. "lobby screen")
  created_at TEXT NOT NULL
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
	Stage   string // "" | "reviewed"
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
	// Best-effort migrations for pre-existing DBs (errors on already-migrated
	// DBs are expected and ignored).
	_, _ = db.Exec(`ALTER TABLE plane_instances ADD COLUMN webhook_secret TEXT`)
	_, _ = db.Exec(`ALTER TABLE bubble_contracts ADD COLUMN stage TEXT NOT NULL DEFAULT ''`)
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// GetContract returns a bubble's overlay, or ok=false when none is set yet.
func (s *Store) GetContract(bubbleID string) (Contract, bool, error) {
	var c Contract
	var closed int
	err := s.db.QueryRow(
		`SELECT COALESCE(outcome,''), COALESCE(owner,''), COALESCE(closure,''), closed, COALESCE(stage,'')
		   FROM bubble_contracts WHERE bubble_id = ?`, bubbleID,
	).Scan(&c.Outcome, &c.Owner, &c.Closure, &closed, &c.Stage)
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
		`INSERT INTO bubble_contracts(bubble_id, outcome, owner, closure, closed, stage)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(bubble_id) DO UPDATE SET
		   outcome = excluded.outcome, owner = excluded.owner,
		   closure = excluded.closure, closed = excluded.closed, stage = excluded.stage`,
		bubbleID, c.Outcome, c.Owner, c.Closure, closed, c.Stage,
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

// SetStage sets just the explicit stage overlay (” | 'reviewed').
func (s *Store) SetStage(bubbleID, stage string) error {
	_, err := s.db.Exec(
		`INSERT INTO bubble_contracts(bubble_id, stage) VALUES(?, ?)
		 ON CONFLICT(bubble_id) DO UPDATE SET stage = excluded.stage`,
		bubbleID, stage,
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
		if err := rows.Scan(&i.Slug, &i.Name, &i.BaseURL, &i.APIKey, &i.Workspace, &i.Project, &i.WebhookSecret); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ListInstances returns every registered instance (admin view; includes keys).
func (s *Store) ListInstances() ([]domain.Instance, error) {
	rows, err := s.db.Query(
		`SELECT slug, COALESCE(name,''), base_url, api_key, workspace, project, COALESCE(webhook_secret,'')
		 FROM plane_instances ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	return scanInstances(rows)
}

// SetWebhookSecret stores the HMAC secret for an instance's inbound webhooks.
func (s *Store) SetWebhookSecret(slug, secret string) (bool, error) {
	res, err := s.db.Exec(`UPDATE plane_instances SET webhook_secret = ? WHERE slug = ?`, secret, slug)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
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

// GetSetting reads a server-wide setting. The value is opaque JSON to the store
// — the server owns its shape. ok is false when the key has never been written.
func (s *Store) GetSetting(key string) (value string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT value FROM server_settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSetting upserts a server-wide setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO server_settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Snapshot is a persisted materialized read model for one instance (F2). bubbles
// is opaque JSON to the store — the server owns its shape.
type Snapshot struct {
	Slug      string
	Bubbles   string
	UpdatedAt string
}

// SaveSnapshot upserts an instance's materialized bubble snapshot so a restart
// can serve the last-known board instantly, before the background refresher runs.
func (s *Store) SaveSnapshot(slug, bubblesJSON, updatedAt string) error {
	_, err := s.db.Exec(
		`INSERT INTO bubble_snapshots(slug, bubbles, updated_at) VALUES(?, ?, ?)
		 ON CONFLICT(slug) DO UPDATE SET bubbles = excluded.bubbles, updated_at = excluded.updated_at`,
		slug, bubblesJSON, updatedAt)
	return err
}

// LoadSnapshots returns every persisted instance snapshot (for boot warm-up).
func (s *Store) LoadSnapshots() ([]Snapshot, error) {
	rows, err := s.db.Query(`SELECT slug, bubbles, updated_at FROM bubble_snapshots`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		if err := rows.Scan(&sn.Slug, &sn.Bubbles, &sn.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// Reader is one person who has read a comment (a 👀 read-receipt).
type Reader struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MarkCommentsRead records readerID as having read each comment (idempotent).
// Plane exposes no comment-reaction API, so read state is a server overlay.
func (s *Store) MarkCommentsRead(instance, readerID, readerName, at string, commentIDs []string) error {
	if len(commentIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO comment_reads (instance, comment_id, reader_id, reader_name, read_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(instance, comment_id, reader_id) DO UPDATE SET reader_name = excluded.reader_name`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, cid := range commentIDs {
		if _, err := stmt.Exec(instance, cid, readerID, readerName, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CommentReaders returns, per comment id, the people who have read it.
func (s *Store) CommentReaders(instance string, commentIDs []string) (map[string][]Reader, error) {
	out := map[string][]Reader{}
	if len(commentIDs) == 0 {
		return out, nil
	}
	q := `SELECT comment_id, reader_id, reader_name FROM comment_reads WHERE instance = ? AND comment_id IN (` +
		placeholders(len(commentIDs)) + `) ORDER BY read_at`
	args := make([]any, 0, len(commentIDs)+1)
	args = append(args, instance)
	for _, c := range commentIDs {
		args = append(args, c)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid string
		var r Reader
		if err := rows.Scan(&cid, &r.ID, &r.Name); err != nil {
			return nil, err
		}
		out[cid] = append(out[cid], r)
	}
	return out, rows.Err()
}

// KioskToken is a read-only display credential bound to one instance.
type KioskToken struct {
	Token     string `json:"token"`
	Instance  string `json:"instance"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// AddKioskToken stores a new kiosk display token.
func (s *Store) AddKioskToken(k KioskToken) error {
	_, err := s.db.Exec(
		`INSERT INTO kiosk_tokens (token, instance, name, created_at) VALUES (?, ?, ?, ?)`,
		k.Token, k.Instance, k.Name, k.CreatedAt)
	return err
}

// LookupKioskToken returns the token's binding, if it exists.
func (s *Store) LookupKioskToken(token string) (KioskToken, bool, error) {
	var k KioskToken
	err := s.db.QueryRow(
		`SELECT token, instance, name, created_at FROM kiosk_tokens WHERE token = ?`, token).
		Scan(&k.Token, &k.Instance, &k.Name, &k.CreatedAt)
	if err == sql.ErrNoRows {
		return KioskToken{}, false, nil
	}
	if err != nil {
		return KioskToken{}, false, err
	}
	return k, true, nil
}

// ListKioskTokens returns all kiosk tokens (admin view).
func (s *Store) ListKioskTokens() ([]KioskToken, error) {
	rows, err := s.db.Query(`SELECT token, instance, name, created_at FROM kiosk_tokens ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KioskToken
	for rows.Next() {
		var k KioskToken
		if err := rows.Scan(&k.Token, &k.Instance, &k.Name, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RemoveKioskToken revokes a token; reports whether one was deleted.
func (s *Store) RemoveKioskToken(token string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM kiosk_tokens WHERE token = ?`, token)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// placeholders returns "?, ?, ..." for an IN clause of n items.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}
