// Package mirror is the local projection of Plane: a SQLite copy of the work
// items, modules, states, members and comments the server reads (see
// docs/journal/PLANE-SYNC.md). It is the READ side — nothing here talks to Plane, and
// nothing here decides what to fetch. internal/sync fills it; internal/server
// queries it.
//
// The mirror is REBUILDABLE. Deleting every mirror_* table must cost a backfill
// and nothing else — never data. That is what keeps AGENTS.md honest: Plane
// remains the system of record, this is just a fast local copy of it.
package mirror

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// schema is applied on New. Every table is keyed by (instance, id) so one
// database holds every federated Plane instance without collision.
//
// "group" is a SQL keyword, hence state_group. Timestamps are RFC3339 TEXT to
// match the rest of the store rather than inventing a second convention.
const schema = `
CREATE TABLE IF NOT EXISTS mirror_projects (
  instance   TEXT NOT NULL,
  id         TEXT NOT NULL,
  name       TEXT NOT NULL,
  identifier TEXT NOT NULL DEFAULT '',
  synced_at  TEXT NOT NULL,
  PRIMARY KEY (instance, id)
);
CREATE TABLE IF NOT EXISTS mirror_modules (
  instance   TEXT NOT NULL,
  id         TEXT NOT NULL,
  project_id TEXT NOT NULL,
  name       TEXT NOT NULL,
  synced_at  TEXT NOT NULL,
  PRIMARY KEY (instance, id)
);
CREATE TABLE IF NOT EXISTS mirror_module_items (
  instance  TEXT NOT NULL,
  module_id TEXT NOT NULL,
  item_id   TEXT NOT NULL,
  PRIMARY KEY (instance, module_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_mirror_module_items_item
  ON mirror_module_items(instance, item_id);
CREATE TABLE IF NOT EXISTS mirror_states (
  instance    TEXT NOT NULL,
  id          TEXT NOT NULL,
  project_id  TEXT NOT NULL,
  name        TEXT NOT NULL,        -- localized; display only, never matched on
  state_group TEXT NOT NULL,        -- backlog|unstarted|started|completed|cancelled
  is_default  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (instance, id)
);
CREATE TABLE IF NOT EXISTS mirror_members (
  instance     TEXT NOT NULL,
  id           TEXT NOT NULL,
  email        TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  role         INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (instance, id)
);
-- Who may see WHICH PROJECT. Plane scopes membership per project, and a private
-- project's members are a subset of the workspace's, so scoping the board by
-- workspace alone would show every project to everyone who can log in.
CREATE TABLE IF NOT EXISTS mirror_project_members (
  instance   TEXT NOT NULL,
  project_id TEXT NOT NULL,
  member_id  TEXT NOT NULL,
  PRIMARY KEY (instance, project_id, member_id)
);
-- One row per project whose membership we have actually READ. Distinct from the
-- membership itself: a project with no rows in mirror_project_members might have
-- no members we know of, or might never have been fetched, and an authorization
-- boundary cannot afford to guess which.
CREATE TABLE IF NOT EXISTS mirror_project_member_sync (
  instance   TEXT NOT NULL,
  project_id TEXT NOT NULL,
  synced_at  TEXT NOT NULL,
  PRIMARY KEY (instance, project_id)
);
CREATE TABLE IF NOT EXISTS mirror_items (
  instance         TEXT NOT NULL,
  id               TEXT NOT NULL,
  project_id       TEXT NOT NULL,
  seq              INTEGER NOT NULL DEFAULT 0,
  name             TEXT NOT NULL DEFAULT '',
  state_id         TEXT NOT NULL DEFAULT '',
  state_name       TEXT NOT NULL DEFAULT '',
  state_group      TEXT NOT NULL DEFAULT '',
  priority         TEXT NOT NULL DEFAULT '',
  parent_id        TEXT NOT NULL DEFAULT '',
  assignees_json   TEXT NOT NULL DEFAULT '[]',
  -- Plane's own labels on the item, as ids. They ride along in the same work-item
  -- payload the delta already reads, so they cost nothing extra
  -- (docs/decisions/0006).
  labels_json      TEXT NOT NULL DEFAULT '[]',
  description_html TEXT NOT NULL DEFAULT '',
  -- lets the syncer tell a body edit from a comment without a second call, and
  -- lets the progress diff run without re-reading bodies (PLANE-SYNC.md)
  description_hash TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL DEFAULT '',
  updated_at       TEXT NOT NULL DEFAULT '',
  completed_at     TEXT NOT NULL DEFAULT '',
  synced_at        TEXT NOT NULL,
  PRIMARY KEY (instance, id)
);
CREATE INDEX IF NOT EXISTS idx_mirror_items_project ON mirror_items(instance, project_id);
CREATE INDEX IF NOT EXISTS idx_mirror_items_parent  ON mirror_items(instance, parent_id);
CREATE TABLE IF NOT EXISTS mirror_cycles (
  instance   TEXT NOT NULL,
  id         TEXT NOT NULL,
  project_id TEXT NOT NULL,
  name       TEXT NOT NULL DEFAULT '',
  -- stored VERBATIM: Plane emits either RFC3339 or a bare YYYY-MM-DD, and draft
  -- cycles emit null. plane.Cycle already parses both leniently, so the mirror
  -- keeps the raw text rather than becoming a second date parser that could
  -- disagree with the first.
  start_date TEXT NOT NULL DEFAULT '',
  end_date   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (instance, id)
);
CREATE TABLE IF NOT EXISTS mirror_comments (
  instance     TEXT NOT NULL,
  id           TEXT NOT NULL,
  item_id      TEXT NOT NULL,
  actor_id     TEXT NOT NULL DEFAULT '',
  comment_html TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (instance, id)
);
CREATE INDEX IF NOT EXISTS idx_mirror_comments_item ON mirror_comments(instance, item_id);
-- The project's label catalogue: what each label id on an item actually says.
-- Project-scoped and cheap, so it rides the slow structure cadence.
CREATE TABLE IF NOT EXISTS mirror_labels (
  instance   TEXT NOT NULL,
  id         TEXT NOT NULL,
  project_id TEXT NOT NULL,
  name       TEXT NOT NULL DEFAULT '',
  color      TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (instance, id)
);
CREATE INDEX IF NOT EXISTS idx_mirror_labels_project ON mirror_labels(instance, project_id);
-- External evidence hung off a work item: a commit, a PR, a published deliverable.
-- Unlike labels these need a call PER ITEM, so they are filled in on a budget the
-- same way comments are (docs/journal/PLANE-SYNC.md).
CREATE TABLE IF NOT EXISTS mirror_item_links (
  instance   TEXT NOT NULL,
  id         TEXT NOT NULL,
  item_id    TEXT NOT NULL,
  url        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (instance, id)
);
CREATE INDEX IF NOT EXISTS idx_mirror_links_item ON mirror_item_links(instance, item_id);
-- Typed relationships between work items (relates_to, duplicate, blocking,
-- blocked_by, …). Also a per-item call, on the same budget.
CREATE TABLE IF NOT EXISTS mirror_item_relations (
  instance      TEXT NOT NULL,
  item_id       TEXT NOT NULL,
  relation_type TEXT NOT NULL,
  related_id    TEXT NOT NULL,
  PRIMARY KEY (instance, item_id, relation_type, related_id)
);
CREATE INDEX IF NOT EXISTS idx_mirror_relations_item ON mirror_item_relations(instance, item_id);
CREATE TABLE IF NOT EXISTS sync_cursors (
  instance    TEXT NOT NULL,
  resource    TEXT NOT NULL,        -- "items" | "structure"
  watermark   TEXT NOT NULL DEFAULT '',  -- newest updated_at applied
  last_full   TEXT NOT NULL DEFAULT '',  -- last complete reconcile
  last_ok     TEXT NOT NULL DEFAULT '',  -- last successful pass of any kind
  last_error  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (instance, resource)
);
`

// Mirror reads and writes the local projection. It borrows the store's handle so
// both live in one file and one pool (see store.DB).
type Mirror struct{ db *sql.DB }

// New applies the mirror schema and returns a handle.
func New(db *sql.DB) (*Mirror, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("mirror schema: %w", err)
	}
	// Best-effort migration for a mirror that predates the column (an error on an
	// already-migrated DB is expected and ignored). A row that keeps the default
	// simply carries no labels until the next pass reads the item again — the
	// mirror is a rebuildable projection, so this is the cheapest safe path.
	_, _ = db.Exec(`ALTER TABLE mirror_items ADD COLUMN labels_json TEXT NOT NULL DEFAULT '[]'`)
	return &Mirror{db: db}, nil
}

// ---- row types (storage shapes, not DTOs) ----

// Item is one mirrored work item.
type Item struct {
	ID              string
	ProjectID       string
	Seq             int
	Name            string
	StateID         string
	StateName       string
	StateGroup      string
	Priority        string
	ParentID        string
	Assignees       []string
	Labels          []string
	DescriptionHTML string
	DescriptionHash string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
}

// Label is one of a project's labels, mirrored so a thread can say what KIND of
// work it is without us keeping a second answer (docs/decisions/0006).
type Label struct {
	ID        string
	ProjectID string
	Name      string
	Color     string
}

// Link is one external URL attached to a work item.
type Link struct {
	ID        string
	URL       string
	Title     string
	CreatedAt time.Time
}

// Relation is one typed edge from a work item to another.
type Relation struct {
	Type      string // relates_to | duplicate | blocking | blocked_by | …
	RelatedID string
}

// Module is one mirrored module (a Bubble).
type Module struct {
	ID        string
	ProjectID string
	Name      string
}

// Project is one mirrored project (a Workspace).
type Project struct {
	ID         string
	Name       string
	Identifier string
}

// State is one mirrored workflow state.
type State struct {
	ID        string
	ProjectID string
	Name      string
	Group     string
	Default   bool
}

// Member is one mirrored workspace member.
type Member struct {
	ID          string
	Email       string
	DisplayName string
	Role        int
}

// Cycle is a project's repeating pulse (§3.6), the window heat is measured
// against. Dates stay as raw strings — see the schema note.
type Cycle struct {
	ID        string
	ProjectID string
	Name      string
	StartDate string
	EndDate   string
}

// Comment is one mirrored work-item comment.
type Comment struct {
	ID        string
	ItemID    string
	ActorID   string
	HTML      string
	CreatedAt time.Time
}

// Cursor is a resource's sync position for one instance.
type Cursor struct {
	Watermark time.Time
	LastFull  time.Time
	LastOK    time.Time
	LastError string
}

// HashBody fingerprints a work-item body. Whitespace-normalized so a reflow or
// re-indent is not mistaken for an edit — the same discipline as
// md.LogbookFingerprint, applied to the whole description.
func HashBody(html string) string {
	if strings.TrimSpace(html) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(html), " ")))
	return hex.EncodeToString(sum[:])[:16]
}

// ---- time helpers: RFC3339 TEXT in, time.Time out ----

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func tsp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return ts(*t)
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseTSP(s string) *time.Time {
	if t := parseTS(s); !t.IsZero() {
		return &t
	}
	return nil
}

func encodeIDs(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeIDs(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
