// Package store is the server's authoritative overlay: the Bubble contract
// (§4), member registry (§9.3) and anything Plane can't express. It uses SQLite
// (pure-Go modernc driver) so the server stays a single static binary (§9.6).
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
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
  -- fingerprint of the WHOLE document, so an edit anywhere counts as production
  -- (docs/decisions/0004). Kept alongside logbook_hash because the difference is
  -- what labels the evidence: a plan change or a document change.
  body_hash    TEXT NOT NULL DEFAULT '',
  logbook_kind TEXT NOT NULL DEFAULT '',  -- what the last change was (an Ev* kind)
  done_todos   INTEGER NOT NULL,  -- ticked items last time we looked (tells a tick from a re-plan)
  revisions    INTEGER NOT NULL,  -- sub-work-items (revision artifacts) last time we looked
  -- Plane links on the item: publishing external evidence is production
  -- (docs/decisions/0006), and like revisions it is counted rather than hashed.
  links        INTEGER NOT NULL DEFAULT 0,
  links_at     TEXT NOT NULL DEFAULT '',
  logbook_at   TEXT NOT NULL,     -- RFC3339 of the last observed Logbook CHANGE ('' = never)
  revisions_at TEXT NOT NULL,     -- RFC3339 of the last observed revision ADDED ('' = never)
  -- When the thread was BORN here: it passed the birth rule, so a Brief with a
  -- Definition of Done and a seeded Logbook were written. That is production
  -- (docs/decisions/0002), and it has to be durable — the cache-only event it
  -- replaced died with the cache, which is why a new thread read 😴.
  -- Empty for a work item that merely appeared in Plane: created is not born.
  born_at      TEXT NOT NULL DEFAULT ''
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
-- The artifact bodies themselves, as markdown (docs/decisions/0001).
--
-- This is the RECORD, not a projection: Plane holds a rendered copy and the
-- mirror can be rebuilt from Plane, but nothing upstream can reconstruct these
-- rows. They live here with the outbox and local_pages for that reason, and they
-- are pruned only on deliberate deletion — never as a side effect of what a Plane
-- walk did or did not see.
--
-- One row per region, and there are exactly three (the splice engine's
-- document/logbook/dod). Named ## sections are edits INSIDE the document region,
-- not regions of their own.
CREATE TABLE IF NOT EXISTS thread_docs (
  thread_id  TEXT NOT NULL,          -- Plane work-item id (bare, like thread_progress)
  region     TEXT NOT NULL,          -- 'document' | 'logbook' | 'dod'
  markdown   TEXT NOT NULL,          -- canonical content
  hash       TEXT NOT NULL,          -- md.Hash(markdown): the optimistic-write base
  updated_at TEXT NOT NULL,          -- RFC3339
  updated_by TEXT NOT NULL DEFAULT '', -- the Actor, or 'plane' for an imported edit
  PRIMARY KEY (thread_id, region)
);
-- What we last PUBLISHED to Plane, per thread rather than per region: the body is
-- one HTML document however many regions it is authored in. Comparing Plane's
-- current hash against this is how a Plane-side edit is detected without
-- mistaking our own publish for one.
CREATE TABLE IF NOT EXISTS thread_publish (
  thread_id      TEXT PRIMARY KEY,
  published_hash TEXT NOT NULL,      -- mirror.HashBody of the HTML we sent
  published_at   TEXT NOT NULL       -- RFC3339
);
-- The version an import REPLACED (docs/decisions/0001). One row per region,
-- overwritten by the next import: this is an undo, not a history. It exists because
-- a Plane-side edit wins, and winning must not mean the other version is gone.
CREATE TABLE IF NOT EXISTS thread_docs_prev (
  thread_id  TEXT NOT NULL,
  region     TEXT NOT NULL,
  markdown   TEXT NOT NULL,
  hash       TEXT NOT NULL,
  updated_at TEXT NOT NULL,          -- when the version being replaced was written
  updated_by TEXT NOT NULL DEFAULT '',
  replaced_at TEXT NOT NULL,         -- when the import overwrote it
  PRIMARY KEY (thread_id, region)
);
CREATE TABLE IF NOT EXISTS server_settings (
  key   TEXT PRIMARY KEY,        -- e.g. "tuning" (the buoyancy calibration)
  value TEXT NOT NULL            -- opaque JSON, owned by the caller
);
-- The outbox lives HERE, with the overlay, and deliberately NOT in the mirror.
-- The mirror is a rebuildable projection: mirror.Reset and sync-backfill drop
-- its tables on purpose. This holds writes that have NOT reached Plane, so
-- losing it loses real work (docs/journal/PLANE-SYNC.md Phase 5).
CREATE TABLE IF NOT EXISTS outbox (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  instance     TEXT NOT NULL,
  kind         TEXT NOT NULL,            -- 'state' (auto-drains) | 'comment' (a draft)
  target_id    TEXT NOT NULL,            -- Plane work-item id
  payload      TEXT NOT NULL,            -- JSON, shape depends on kind
  -- whose draft this is. Comments carry no credential (that would put every
  -- user's Plane key at rest), so a draft can only be re-sent by its author,
  -- with their live key. This scopes it to them.
  author_email TEXT NOT NULL DEFAULT '',
  -- the mirror column an incoming sync must not overwrite while this is pending,
  -- so a sync cannot revert a write that has not landed yet.
  field_lock   TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'pending',  -- pending | abandoned
  attempts     INTEGER NOT NULL DEFAULT 0,
  next_at      TEXT NOT NULL DEFAULT '',         -- RFC3339; backoff for auto-drain
  created_at   TEXT NOT NULL,
  last_error   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox(status, instance);
CREATE INDEX IF NOT EXISTS idx_outbox_target  ON outbox(instance, target_id, status);

CREATE TABLE IF NOT EXISTS kiosk_tokens (
  token      TEXT PRIMARY KEY,   -- server-issued read-only display credential (§9 Phase 9)
  instance   TEXT NOT NULL,      -- the instance slug this token may view
  name       TEXT NOT NULL,      -- human label (e.g. "lobby screen")
  created_at TEXT NOT NULL
);

-- Pages this server is the record for, because the instance's Plane cannot be
-- (docs/journal/PAGES-CAPABILITY.md). Plane Community exposes pages only on its INTERNAL
-- session-authenticated API, so on those deployments there is nowhere upstream to
-- put a spec — and a workspace with no home for its standing documentation is
-- worse than one whose documentation lives here.
--
-- Like the outbox, this belongs to the OVERLAY and not the mirror: the mirror is
-- a projection Plane can rebuild, and these rows are the only copy in existence.
-- Dropping them loses real work.
CREATE TABLE IF NOT EXISTS local_pages (
  id         TEXT PRIMARY KEY,   -- "<instance>:<project>:loc_<uuid>" — the prefix
                                 -- is what tells a read which backend owns it
  instance   TEXT NOT NULL,
  project    TEXT NOT NULL,
  title      TEXT NOT NULL,
  body       TEXT NOT NULL,      -- markdown, the same shape the editor writes
  locked     INTEGER NOT NULL DEFAULT 0,
  archived   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  author     TEXT NOT NULL DEFAULT '',  -- who created it, for the audit trail
  parent     TEXT NOT NULL DEFAULT ''   -- namespaced id of the page above it, '' at the root
);
CREATE INDEX IF NOT EXISTS idx_local_pages_project ON local_pages(instance, project);

-- Whether an instance's Plane serves project pages on its PUBLIC API. Cached
-- because it is a property of the deployment, not of a request, and probing it
-- costs a request against a 60 req/min budget. Re-probed when it says "no", in
-- case the instance is upgraded; a "yes" cannot become false.
CREATE TABLE IF NOT EXISTS instance_capabilities (
  slug       TEXT NOT NULL,
  capability TEXT NOT NULL,       -- 'pages'
  supported  INTEGER NOT NULL,
  checked_at TEXT NOT NULL,
  PRIMARY KEY (slug, capability)
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
// readers (docs/journal/PLANE-SYNC.md Phase 0). This was inert while the store held only
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
	// Pages nest, the way Plane's own do, so a locally-held one needs somewhere to
	// say what it sits under.
	_, _ = db.Exec(`ALTER TABLE local_pages ADD COLUMN parent TEXT NOT NULL DEFAULT ''`)
	// Being born is production (docs/decisions/0002). Threads that predate this
	// column keep an empty born_at and are deliberately NOT reheated — synthesising
	// a birth from created_at would make the board lie about old work.
	_, _ = db.Exec(`ALTER TABLE thread_progress ADD COLUMN born_at TEXT NOT NULL DEFAULT ''`)
	// Any edit to the body is production now, not only the plan (docs/decisions/0004).
	// An empty body_hash on an existing row baselines silently on the next sweep, so
	// nobody's old prose edit is retro-actively announced as new work.
	_, _ = db.Exec(`ALTER TABLE thread_progress ADD COLUMN body_hash TEXT NOT NULL DEFAULT ''`)
	// Publishing external evidence is production (docs/decisions/0006). An existing
	// row starts at zero links and baselines on the next sweep, so links that were
	// already there are not announced as new work.
	_, _ = db.Exec(`ALTER TABLE thread_progress ADD COLUMN links INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE thread_progress ADD COLUMN links_at TEXT NOT NULL DEFAULT ''`)
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle so the Plane mirror can keep its tables in
// the SAME file and connection pool (docs/journal/PLANE-SYNC.md Phase 1). Two pools over
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
	BodyHash    string    // md.BodyFingerprint of the whole document
	LogbookKind string    // the domain.Ev* kind of the last change
	DoneTodos   int       // ticked items, so a tick can be told from a re-plan
	Revisions   int       // sub-work-items
	Links       int       // Plane links on the item (docs/decisions/0006)
	LogbookAt   time.Time // when the Logbook last changed
	RevisionsAt time.Time // when a revision was last added
	LinksAt     time.Time // when a link was last added
	BornAt      time.Time // when the thread passed the birth rule here (zero = not born here)
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
			`SELECT thread_id, logbook_hash, body_hash, logbook_kind, done_todos, revisions, links, logbook_at, revisions_at, links_at, born_at
			 FROM thread_progress WHERE thread_id IN (`+placeholders(len(part))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p ThreadProgress
			var logAt, revAt, linksAt, bornAt string
			if err := rows.Scan(&p.ThreadID, &p.LogbookHash, &p.BodyHash, &p.LogbookKind,
				&p.DoneTodos, &p.Revisions, &p.Links, &logAt, &revAt, &linksAt, &bornAt); err != nil {
				rows.Close()
				return nil, err
			}
			p.LogbookAt, _ = time.Parse(time.RFC3339, logAt)
			p.RevisionsAt, _ = time.Parse(time.RFC3339, revAt)
			p.LinksAt, _ = time.Parse(time.RFC3339, linksAt)
			p.BornAt, _ = time.Parse(time.RFC3339, bornAt)
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
	// born_at is only ever SET, never cleared: an empty incoming value leaves the
	// stored one alone. The sweep that writes most of these rows has no idea when a
	// thread was born, so without this clause a routine progress save would erase
	// the one timestamp nothing can reconstruct.
	stmt, err := tx.Prepare(
		`INSERT INTO thread_progress(thread_id, logbook_hash, body_hash, logbook_kind, done_todos, revisions, links, logbook_at, revisions_at, links_at, born_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(thread_id) DO UPDATE SET
		   logbook_hash = excluded.logbook_hash, body_hash = excluded.body_hash,
		   logbook_kind = excluded.logbook_kind,
		   done_todos = excluded.done_todos, revisions = excluded.revisions,
		   links = excluded.links,
		   logbook_at = excluded.logbook_at, revisions_at = excluded.revisions_at,
		   links_at = excluded.links_at,
		   born_at = CASE WHEN excluded.born_at <> '' THEN excluded.born_at ELSE thread_progress.born_at END`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range ps {
		if _, err := stmt.Exec(p.ThreadID, p.LogbookHash, p.BodyHash, p.LogbookKind,
			p.DoneTodos, p.Revisions, p.Links,
			stamp(p.LogbookAt), stamp(p.RevisionsAt), stamp(p.LinksAt), stamp(p.BornAt)); err != nil {
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

// LocalPage is a page this server is the record for. Body is markdown, since
// that is what the editor writes and what internal/md renders from — there is no
// Plane round trip to convert for.
type LocalPage struct {
	ID        string
	Instance  string
	Project   string
	Title     string
	Body      string
	Locked    bool
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	Author    string
	// Parent is the namespaced id this page nests under, '' at the root. Mirrors
	// what Plane models with parent_id, so the two kinds form ONE tree.
	Parent string
}

// PutLocalPage inserts or replaces a locally-held page.
func (s *Store) PutLocalPage(p LocalPage) error {
	_, err := s.db.Exec(`
		INSERT INTO local_pages
		  (id, instance, project, title, body, locked, archived, created_at, updated_at, author, parent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  title = excluded.title, body = excluded.body, locked = excluded.locked,
		  archived = excluded.archived, updated_at = excluded.updated_at,
		  parent = excluded.parent`,
		p.ID, p.Instance, p.Project, p.Title, p.Body, b2i(p.Locked), b2i(p.Archived),
		p.CreatedAt.UTC().Format(time.RFC3339), p.UpdatedAt.UTC().Format(time.RFC3339),
		p.Author, p.Parent)
	return err
}

// LocalPages returns one project's locally-held pages, newest change first.
func (s *Store) LocalPages(instance, project string) ([]LocalPage, error) {
	rows, err := s.db.Query(`
		SELECT id, instance, project, title, body, locked, archived, created_at, updated_at, author, parent
		FROM local_pages WHERE instance = ? AND project = ?
		ORDER BY updated_at DESC`, instance, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LocalPage
	for rows.Next() {
		p, err := scanLocalPage(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LocalPage returns one locally-held page.
func (s *Store) LocalPage(id string) (LocalPage, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, instance, project, title, body, locked, archived, created_at, updated_at, author, parent
		FROM local_pages WHERE id = ?`, id)
	p, err := scanLocalPage(row.Scan)
	if err == sql.ErrNoRows {
		return LocalPage{}, false, nil
	}
	if err != nil {
		return LocalPage{}, false, err
	}
	return p, true, nil
}

// DeleteLocalPage removes a locally-held page; reports whether one went.
func (s *Store) DeleteLocalPage(id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM local_pages WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// PromoteLocalPageChildren re-parents a deleted page's children onto whatever it
// sat under. Without it they would point at an id that no longer exists, and a
// tree rendered from its roots would simply not show them — the document would
// still be in the database and gone from the only UI that can reach it.
func (s *Store) PromoteLocalPageChildren(parentID, newParent string) error {
	_, err := s.db.Exec(`UPDATE local_pages SET parent = ? WHERE parent = ?`, newParent, parentID)
	return err
}

// ForgetLocalPages drops every locally-held page in a project. Called when the
// workspace itself is deleted — the pages belonged to it, and nothing else will
// ever ask for them again.
func (s *Store) ForgetLocalPages(instance, project string) error {
	_, err := s.db.Exec(`DELETE FROM local_pages WHERE instance = ? AND project = ?`, instance, project)
	return err
}

// scanLocalPage reads one row in the column order every query above uses.
func scanLocalPage(scan func(...any) error) (LocalPage, error) {
	var p LocalPage
	var locked, archived int
	var created, updated string
	if err := scan(&p.ID, &p.Instance, &p.Project, &p.Title, &p.Body,
		&locked, &archived, &created, &updated, &p.Author, &p.Parent); err != nil {
		return LocalPage{}, err
	}
	p.Locked, p.Archived = locked != 0, archived != 0
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return p, nil
}

// Capability reports a cached instance capability verdict, and when it was taken.
func (s *Store) Capability(slug, capability string) (supported bool, checkedAt time.Time, known bool, err error) {
	var sup int
	var at string
	e := s.db.QueryRow(
		`SELECT supported, checked_at FROM instance_capabilities WHERE slug = ? AND capability = ?`,
		slug, capability).Scan(&sup, &at)
	if e == sql.ErrNoRows {
		return false, time.Time{}, false, nil
	}
	if e != nil {
		return false, time.Time{}, false, e
	}
	t, _ := time.Parse(time.RFC3339, at)
	return sup != 0, t, true, nil
}

// SetCapability records what a probe found.
func (s *Store) SetCapability(slug, capability string, supported bool, at time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO instance_capabilities (slug, capability, supported, checked_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(slug, capability) DO UPDATE SET
		  supported = excluded.supported, checked_at = excluded.checked_at`,
		slug, capability, b2i(supported), at.UTC().Format(time.RFC3339))
	return err
}

// b2i encodes a bool for SQLite, which has no boolean type.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// placeholders returns "?, ?, ..." for an IN clause of n items.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}

// Prune deletes overlay rows about work items that no longer exist, and returns
// how many went (docs/journal/PLANE-SYNC.md Phase 7).
//
// thread_progress, thread_pulse and thread_autostate are keyed by Plane work-item
// id and were only ever inserted into. A deleted work item left its rows behind
// forever — a slow leak, and worse, a resurrection hazard: recreating an id would
// inherit a stranger's progress timestamps and be born warm.
//
// live is every work-item id the mirror currently knows about, so the caller
// MUST pass a complete set. An empty set is treated as "the mirror is not ready"
// and prunes nothing, because deleting the entire overlay on the strength of a
// failed sync would be catastrophic and silent.
//
// thread_docs and thread_publish are deliberately NOT in this list, and must never
// be added to it (docs/decisions/0001). Those rows are the only copy of the
// writing: for the tables below, a wrong prune costs a timestamp the next sweep
// re-derives, and for those it costs the work. They are cleared by ForgetThreads —
// deliberate deletion — and by nothing else.
func (s *Store) Prune(live []string) (int, error) {
	if len(live) == 0 {
		return 0, nil
	}
	placeholders := "?" + strings.Repeat(",?", len(live)-1)
	args := make([]any, 0, len(live))
	for _, id := range live {
		args = append(args, id)
	}
	total := 0
	for _, table := range []string{"thread_progress", "thread_pulse", "thread_autostate"} {
		res, err := s.db.Exec(
			`DELETE FROM `+table+` WHERE thread_id NOT IN (`+placeholders+`)`, args...)
		if err != nil {
			return total, fmt.Errorf("prune %s: %w", table, err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

// ForgetBubble drops a bubble's overlay: its contract, closure, stage and
// derived lifecycle. Called when a bubble is deleted from Plane, so a module id
// that Plane later reuses cannot inherit a dead bubble's outcome and owner.
func (s *Store) ForgetBubble(bubbleID string) error {
	for _, q := range []string{
		`DELETE FROM bubble_contracts WHERE bubble_id = ?`,
		`DELETE FROM bubble_state WHERE bubble_id = ?`,
	} {
		if _, err := s.db.Exec(q, bubbleID); err != nil {
			return err
		}
	}
	return nil
}

// ForgetThreads drops the overlay for deleted threads: their progress baseline,
// their comment pulse and any pending auto-state write.
//
// The progress baseline matters most. It is what "has this thread produced
// anything since we last looked" is measured against, so a stale row for a
// recreated id would compare new work against a dead thread's history.
func (s *Store) ForgetThreads(threadIDs []string) error {
	for _, id := range threadIDs {
		for _, q := range []string{
			`DELETE FROM thread_progress WHERE thread_id = ?`,
			`DELETE FROM thread_pulse WHERE thread_id = ?`,
			`DELETE FROM thread_autostate WHERE thread_id = ?`,
			// The document and what we published of it. Safe HERE and only here:
			// this function is called when someone deliberately deletes a thread or
			// its whole workspace, never from a sync pass (see Prune).
			`DELETE FROM thread_docs WHERE thread_id = ?`,
			`DELETE FROM thread_docs_prev WHERE thread_id = ?`,
			`DELETE FROM thread_publish WHERE thread_id = ?`,
		} {
			if _, err := s.db.Exec(q, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- artifact documents (docs/decisions/0001) ----

// ThreadDoc is one region of a thread's page, as markdown. This is the record:
// Plane's description_html is a rendering of it, and the mirror is a projection of
// that rendering. Neither can reconstruct this row.
type ThreadDoc struct {
	ThreadID  string
	Region    string // 'document' | 'logbook' | 'dod'
	Markdown  string
	Hash      string // md.Hash(Markdown) — the optimistic-write `base`
	UpdatedAt time.Time
	UpdatedBy string // an Actor label, or "plane" for an edit imported from Plane
}

// ThreadDocs returns a thread's regions keyed by region name. An empty map means
// the thread has no stored document yet, which is the signal to fall back to the
// mirrored body rather than to show an empty page.
func (s *Store) ThreadDocs(threadID string) (map[string]ThreadDoc, error) {
	rows, err := s.db.Query(`
		SELECT thread_id, region, markdown, hash, updated_at, updated_by
		FROM thread_docs WHERE thread_id = ?`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ThreadDoc{}
	for rows.Next() {
		var d ThreadDoc
		var at string
		if err := rows.Scan(&d.ThreadID, &d.Region, &d.Markdown, &d.Hash, &at, &d.UpdatedBy); err != nil {
			return nil, err
		}
		d.UpdatedAt, _ = time.Parse(time.RFC3339, at)
		out[d.Region] = d
	}
	return out, rows.Err()
}

// PutThreadDocs upserts a thread's regions in one transaction. Regions absent from
// the batch are left alone; a region whose markdown is empty is REMOVED, because an
// empty region and a missing one are the same thing to every reader and keeping the
// row would make an empty Logbook look like a written one.
func (s *Store) PutThreadDocs(docs []ThreadDoc) error {
	if len(docs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	put, err := tx.Prepare(`
		INSERT INTO thread_docs(thread_id, region, markdown, hash, updated_at, updated_by)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(thread_id, region) DO UPDATE SET
		  markdown = excluded.markdown, hash = excluded.hash,
		  updated_at = excluded.updated_at, updated_by = excluded.updated_by`)
	if err != nil {
		return err
	}
	defer put.Close()
	del, err := tx.Prepare(`DELETE FROM thread_docs WHERE thread_id = ? AND region = ?`)
	if err != nil {
		return err
	}
	defer del.Close()
	for _, d := range docs {
		if strings.TrimSpace(d.Markdown) == "" {
			if _, err := del.Exec(d.ThreadID, d.Region); err != nil {
				return err
			}
			continue
		}
		if _, err := put.Exec(d.ThreadID, d.Region, d.Markdown, d.Hash,
			stamp(d.UpdatedAt), d.UpdatedBy); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PublishState is what we last sent to Plane for one thread.
type PublishState struct {
	PublishedHash string
	PublishedAt   time.Time
}

// SetPublished records the hash of the HTML we just published.
func (s *Store) SetPublished(threadID, hash string, at time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO thread_publish(thread_id, published_hash, published_at)
		VALUES(?, ?, ?)
		ON CONFLICT(thread_id) DO UPDATE SET
		  published_hash = excluded.published_hash, published_at = excluded.published_at`,
		threadID, hash, stamp(at))
	return err
}

// Published returns what was last published for a thread. A zero value means we
// have never published it, which is why divergence detection must treat an empty
// hash as "no opinion" rather than as a mismatch.
func (s *Store) Published(threadID string) (PublishState, error) {
	row := s.db.QueryRow(`SELECT published_hash, published_at FROM thread_publish WHERE thread_id = ?`, threadID)
	var p PublishState
	var at string
	switch err := row.Scan(&p.PublishedHash, &at); err {
	case nil:
		p.PublishedAt, _ = time.Parse(time.RFC3339, at)
		return p, nil
	case sql.ErrNoRows:
		return PublishState{}, nil
	default:
		return PublishState{}, err
	}
}

// ImportedDocs replaces a thread's stored regions with a version imported from
// Plane, keeping what it replaced (docs/decisions/0001).
//
// Plane stays a writable surface, so an edit made there wins — but "wins" must not
// mean the other version is gone, which is the same rule the outbox holds in the
// other direction: nothing disappears without being recorded somewhere.
func (s *Store) ImportedDocs(threadID string, docs []ThreadDoc, at time.Time) error {
	prev, err := s.ThreadDocs(threadID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM thread_docs_prev WHERE thread_id = ?`, threadID); err != nil {
		return err
	}
	keep, err := tx.Prepare(`
		INSERT INTO thread_docs_prev(thread_id, region, markdown, hash, updated_at, updated_by, replaced_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer keep.Close()
	for _, d := range prev {
		if _, err := keep.Exec(d.ThreadID, d.Region, d.Markdown, d.Hash,
			stamp(d.UpdatedAt), d.UpdatedBy, stamp(at)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.PutThreadDocs(docs)
}

// ReplacedDocs returns the version an import overwrote, keyed by region, plus when
// it was replaced. Empty when nothing has ever been imported for this thread.
func (s *Store) ReplacedDocs(threadID string) (map[string]ThreadDoc, time.Time, error) {
	rows, err := s.db.Query(`
		SELECT region, markdown, hash, updated_at, updated_by, replaced_at
		FROM thread_docs_prev WHERE thread_id = ?`, threadID)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	out := map[string]ThreadDoc{}
	var replaced time.Time
	for rows.Next() {
		d := ThreadDoc{ThreadID: threadID}
		var at, rep string
		if err := rows.Scan(&d.Region, &d.Markdown, &d.Hash, &at, &d.UpdatedBy, &rep); err != nil {
			return nil, time.Time{}, err
		}
		d.UpdatedAt, _ = time.Parse(time.RFC3339, at)
		if r, err := time.Parse(time.RFC3339, rep); err == nil && r.After(replaced) {
			replaced = r
		}
		out[d.Region] = d
	}
	return out, replaced, rows.Err()
}

func anySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
