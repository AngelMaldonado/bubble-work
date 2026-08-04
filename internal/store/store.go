// Package store is the server's authoritative overlay: the Bubble contract
// (§4), member registry (§9.3) and anything Plane can't express. It uses SQLite
// (pure-Go modernc driver) so the server stays a single static binary (§9.6).
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
  webhook_secret TEXT,              -- HMAC secret for inbound Plane webhooks (§6)
  auto_state     INTEGER NOT NULL DEFAULT 0  -- write derived levels back to Plane? (Phase B, off)
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
CREATE TABLE IF NOT EXISTS thread_progress (
  thread_id    TEXT PRIMARY KEY,  -- Plane work-item id
  logbook_hash TEXT NOT NULL DEFAULT '',  -- fingerprint of the Logbook + DoD text
  logbook_kind TEXT NOT NULL DEFAULT '',  -- what the last change was (an Ev* kind)
  done_todos   INTEGER NOT NULL,  -- ticked items last time we looked (tells a tick from a re-plan)
  revisions    INTEGER NOT NULL,  -- sub-work-items (revision artifacts) last time we looked
  logbook_at   TEXT NOT NULL,     -- RFC3339 of the last observed Logbook CHANGE ('' = never)
  revisions_at TEXT NOT NULL      -- RFC3339 of the last observed revision ADDED ('' = never)
);
CREATE TABLE IF NOT EXISTS thread_autostate (
  thread_id  TEXT PRIMARY KEY,  -- Plane work-item id
  state_id   TEXT NOT NULL,     -- the state WE last wrote (provenance, Phase B)
  written_at TEXT NOT NULL,     -- RFC3339
  handed_off INTEGER NOT NULL DEFAULT 0  -- 1 = a human overrode us; never touch it again
);
CREATE TABLE IF NOT EXISTS thread_pulse (
  thread_id       TEXT PRIMARY KEY,  -- Plane work-item id
  last_comment_at TEXT NOT NULL,     -- RFC3339 of the newest comment we have seen
  checked_at      TEXT NOT NULL      -- RFC3339 of the last time we looked (probe budgeting)
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

// dsn turns a filesystem path into a modernc DSN carrying the pragmas this
// server needs. See Open for why each one is here.
func dsn(path string) string {
	// A caller that already speaks URI knows better than we do.
	if strings.HasPrefix(path, "file:") {
		return path
	}
	// Relative paths would produce an opaque "file:t.db" URI; make it absolute so
	// the driver always sees file:///... (best-effort — a failure here just means
	// we hand the driver what the caller gave us).
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	q := url.Values{}
	for _, p := range []string{
		// WAL: readers do not block the writer and the writer does not block
		// readers. Without it the default rollback journal makes any concurrent
		// read/write pair collide as SQLITE_BUSY.
		"journal_mode(WAL)",
		// SQLite still permits only ONE writer at a time; busy_timeout makes the
		// loser wait for the lock instead of failing instantly.
		"busy_timeout(5000)",
		// With WAL, NORMAL syncs at checkpoints rather than every commit. The
		// exposure is losing the last commits on an OS crash (not on a process
		// crash), which for a rebuildable mirror is a fine trade.
		"synchronous(NORMAL)",
	} {
		q.Add("_pragma", p)
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return u.String()
}

// Open opens (and migrates) the SQLite database at path.
//
// The connection is tuned for one background writer alongside many concurrent
// readers (docs/PLANE-SYNC.md Phase 0). This was inert while the store held only
// the small, rare overlay writes; it stops being inert the moment the sync
// worker bulk-upserts the mirror while every board read queries the same file.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	// Reads stay concurrent (that is the point of WAL); writes serialize on the
	// file lock and wait out busy_timeout. The cap is here to bound file handles
	// and memory, not to serialize.
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	// journal_mode is a property of the DATABASE, not the connection, so a
	// pre-existing file only converts on this first successful call. Verify
	// rather than assume: silently staying in rollback-journal mode is exactly
	// the failure this is meant to prevent, and it would only surface later as
	// intermittent SQLITE_BUSY under load.
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		db.Close()
		return nil, fmt.Errorf("read journal_mode: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		db.Close()
		return nil, fmt.Errorf("sqlite journal_mode is %q, want WAL", mode)
	}
	// Best-effort migrations for pre-existing DBs (errors on already-migrated
	// DBs are expected and ignored).
	_, _ = db.Exec(`ALTER TABLE plane_instances ADD COLUMN webhook_secret TEXT`)
	_, _ = db.Exec(`ALTER TABLE plane_instances ADD COLUMN auto_state INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE bubble_contracts ADD COLUMN stage TEXT NOT NULL DEFAULT ''`)
	// thread_progress once keyed progress off the ticked-todo count alone; it now
	// fingerprints the whole Logbook, so any plan change counts (THREAD-LIFECYCLE.md).
	_, _ = db.Exec(`ALTER TABLE thread_progress ADD COLUMN logbook_hash TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE thread_progress ADD COLUMN logbook_kind TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE thread_progress RENAME COLUMN todos_at TO logbook_at`)
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle so the Plane mirror can keep its tables in
// the SAME file and connection pool (docs/PLANE-SYNC.md Phase 1). Two pools over
// one SQLite file would reintroduce exactly the lock contention WAL is here to
// remove, so the mirror borrows this rather than opening its own.
//
// The overlay tables above and the mirror_* tables are different things: this is
// the server's own state, those are a rebuildable projection of Plane.
func (s *Store) DB() *sql.DB { return s.db }

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
		var auto int
		if err := rows.Scan(&i.Slug, &i.Name, &i.BaseURL, &i.APIKey, &i.Workspace, &i.Project, &i.WebhookSecret, &auto); err != nil {
			return nil, err
		}
		i.AutoState = auto == 1
		out = append(out, i)
	}
	return out, rows.Err()
}

// ListInstances returns every registered instance (admin view; includes keys).
func (s *Store) ListInstances() ([]domain.Instance, error) {
	rows, err := s.db.Query(
		`SELECT slug, COALESCE(name,''), base_url, api_key, workspace, project, COALESCE(webhook_secret,''),
		        COALESCE(auto_state, 0)
		 FROM plane_instances ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	return scanInstances(rows)
}

// SetAutoState turns Plane state write-back on or off for one instance
// (THREAD-LIFECYCLE.md Phase B). Off by default; nothing writes to Plane until
// an operator opts that instance in.
func (s *Store) SetAutoState(slug string, enabled bool) (bool, error) {
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE plane_instances SET auto_state = ? WHERE slug = ?`, v, slug)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
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

// ThreadProgress is what a thread had produced the last time the refresher
// looked, plus WHEN it last changed. The timestamps are the durable part: heat
// is derived from them on every read, so one edit keeps warming the thread long
// after the refresh that noticed it. A zero time means "never observed
// changing" — including a thread we have only ever seen once.
type ThreadProgress struct {
	ThreadID    string
	LogbookHash string    // md.LogbookFingerprint of the Logbook + DoD
	LogbookKind string    // the domain.Ev* kind of the last change
	DoneTodos   int       // ticked items, so a tick can be told from a re-plan
	Revisions   int       // sub-work-items
	LogbookAt   time.Time // when the Logbook last changed
	RevisionsAt time.Time // when a revision was last added
}

// ThreadProgressFor loads the last-observed progress for the given work items.
// Missing ids are simply absent from the map (first sighting).
func (s *Store) ThreadProgressFor(ids []string) (map[string]ThreadProgress, error) {
	out := make(map[string]ThreadProgress, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	// Chunked so a large project stays well under SQLite's variable limit.
	const chunk = 400
	for start := 0; start < len(ids); start += chunk {
		end := min(start+chunk, len(ids))
		part := ids[start:end]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := s.db.Query(
			`SELECT thread_id, logbook_hash, logbook_kind, done_todos, revisions, logbook_at, revisions_at
			 FROM thread_progress WHERE thread_id IN (`+placeholders(len(part))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p ThreadProgress
			var logAt, revAt string
			if err := rows.Scan(&p.ThreadID, &p.LogbookHash, &p.LogbookKind,
				&p.DoneTodos, &p.Revisions, &logAt, &revAt); err != nil {
				rows.Close()
				return nil, err
			}
			p.LogbookAt, _ = time.Parse(time.RFC3339, logAt)
			p.RevisionsAt, _ = time.Parse(time.RFC3339, revAt)
			out[p.ThreadID] = p
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SaveThreadProgress upserts the observed progress for a batch of threads.
func (s *Store) SaveThreadProgress(ps []ThreadProgress) error {
	if len(ps) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`INSERT INTO thread_progress(thread_id, logbook_hash, logbook_kind, done_todos, revisions, logbook_at, revisions_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(thread_id) DO UPDATE SET
		   logbook_hash = excluded.logbook_hash, logbook_kind = excluded.logbook_kind,
		   done_todos = excluded.done_todos, revisions = excluded.revisions,
		   logbook_at = excluded.logbook_at, revisions_at = excluded.revisions_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range ps {
		if _, err := stmt.Exec(p.ThreadID, p.LogbookHash, p.LogbookKind, p.DoneTodos, p.Revisions,
			stamp(p.LogbookAt), stamp(p.RevisionsAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// stamp formats a time for storage; the zero time is stored as an empty string.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// AutoState records the Plane state WE last wrote for a thread, so the sweep can
// tell its own handiwork from a human's. HandedOff latches: once someone moves a
// card we auto-managed, we back off from that thread permanently
// (THREAD-LIFECYCLE.md Phase B).
type AutoState struct {
	ThreadID  string
	StateID   string
	WrittenAt time.Time
	HandedOff bool
}

// AutoStateFor loads what we last wrote for the given work items.
func (s *Store) AutoStateFor(ids []string) (map[string]AutoState, error) {
	out := make(map[string]AutoState, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const chunk = 400
	for start := 0; start < len(ids); start += chunk {
		end := min(start+chunk, len(ids))
		part := ids[start:end]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := s.db.Query(
			`SELECT thread_id, state_id, written_at, handed_off FROM thread_autostate
			 WHERE thread_id IN (`+placeholders(len(part))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var a AutoState
			var at string
			var handed int
			if err := rows.Scan(&a.ThreadID, &a.StateID, &at, &handed); err != nil {
				rows.Close()
				return nil, err
			}
			a.WrittenAt, _ = time.Parse(time.RFC3339, at)
			a.HandedOff = handed == 1
			out[a.ThreadID] = a
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RecordAutoState remembers the state we just wrote for a thread.
func (s *Store) RecordAutoState(threadID, stateID string, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO thread_autostate(thread_id, state_id, written_at, handed_off) VALUES(?, ?, ?, 0)
		 ON CONFLICT(thread_id) DO UPDATE SET
		   state_id = excluded.state_id, written_at = excluded.written_at`,
		threadID, stateID, stamp(at))
	return err
}

// HandOffAutoState latches a thread as human-owned: we noticed its state is no
// longer what we left it as, so we stop auto-managing it for good.
func (s *Store) HandOffAutoState(threadID string) error {
	_, err := s.db.Exec(
		`INSERT INTO thread_autostate(thread_id, state_id, written_at, handed_off) VALUES(?, '', '', 1)
		 ON CONFLICT(thread_id) DO UPDATE SET handed_off = 1`, threadID)
	return err
}

// Pulse is the presence signal for a thread: when someone last commented, and
// when we last checked. Comments never warm a thread — the timestamp only keeps
// it out of the grave (THREAD-LIFECYCLE.md).
type Pulse struct {
	ThreadID      string
	LastCommentAt time.Time
	CheckedAt     time.Time
}

// PulseFor loads the recorded pulse for the given work items.
func (s *Store) PulseFor(ids []string) (map[string]Pulse, error) {
	out := make(map[string]Pulse, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const chunk = 400
	for start := 0; start < len(ids); start += chunk {
		end := min(start+chunk, len(ids))
		part := ids[start:end]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := s.db.Query(
			`SELECT thread_id, last_comment_at, checked_at FROM thread_pulse
			 WHERE thread_id IN (`+placeholders(len(part))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p Pulse
			var last, checked string
			if err := rows.Scan(&p.ThreadID, &last, &checked); err != nil {
				rows.Close()
				return nil, err
			}
			p.LastCommentAt, _ = time.Parse(time.RFC3339, last)
			p.CheckedAt, _ = time.Parse(time.RFC3339, checked)
			out[p.ThreadID] = p
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RecordPulse notes that we looked at a thread's comments, and when the newest
// one was. lastComment never moves backwards: a deleted comment doesn't erase
// the fact that someone WAS paying attention.
func (s *Store) RecordPulse(threadID string, lastComment, checkedAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO thread_pulse(thread_id, last_comment_at, checked_at) VALUES(?, ?, ?)
		 ON CONFLICT(thread_id) DO UPDATE SET
		   last_comment_at = MAX(thread_pulse.last_comment_at, excluded.last_comment_at),
		   checked_at = excluded.checked_at`,
		threadID, stamp(lastComment), stamp(checkedAt))
	return err
}

// StalePulseCheck returns which of the given threads we have checked least
// recently (never-checked first), capped at limit — the probe budget.
func (s *Store) StalePulseCheck(ids []string, limit int) ([]string, error) {
	if len(ids) == 0 || limit <= 0 {
		return nil, nil
	}
	checked, err := s.PulseFor(ids)
	if err != nil {
		return nil, err
	}
	ordered := append([]string(nil), ids...)
	sort.Slice(ordered, func(i, j int) bool {
		return checked[ordered[i]].CheckedAt.Before(checked[ordered[j]].CheckedAt)
	})
	return ordered[:min(limit, len(ordered))], nil
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
