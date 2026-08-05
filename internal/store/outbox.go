package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The outbox: writes that have not reached Plane (docs/PLANE-SYNC.md Phase 5).
//
// It holds two kinds, and they behave differently on purpose:
//
//   - OutState — an auto-state move. It is performed with the INSTANCE key,
//     which the server already stores, so it can be drained by a worker with
//     backoff and no human present.
//   - OutComment — a comment draft. Comments are posted with the CALLER's own
//     Plane key so the author in Plane is the real person; queueing a credential
//     to replay later would put every user's key at rest in this database. So a
//     draft carries only the text, and is re-sent by its author from the UI with
//     their live key. Nothing drains it automatically, by design.

const (
	OutState   = "state"   // payload: {"state_id": "..."}
	OutComment = "comment" // payload: {"body": "..."} — a draft, never auto-drained

	OutPending   = "pending"
	OutAbandoned = "abandoned"
)

// OutboxEntry is one queued write.
type OutboxEntry struct {
	ID          int64
	Instance    string
	Kind        string
	TargetID    string
	Payload     map[string]string
	AuthorEmail string
	FieldLock   string
	Status      string
	Attempts    int
	NextAt      time.Time
	CreatedAt   time.Time
	LastError   string
}

// Body returns a comment draft's text.
func (e OutboxEntry) Body() string { return e.Payload["body"] }

// StateID returns a queued state move's target state.
func (e OutboxEntry) StateID() string { return e.Payload["state_id"] }

// Enqueue records a write that did not reach Plane. Returns the new row id.
func (s *Store) Enqueue(e OutboxEntry) (int64, error) {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return 0, err
	}
	if e.Status == "" {
		e.Status = OutPending
	}
	res, err := s.db.Exec(`
		INSERT INTO outbox(instance, kind, target_id, payload, author_email,
		  field_lock, status, attempts, next_at, created_at, last_error)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		e.Instance, e.Kind, e.TargetID, string(payload), strings.ToLower(e.AuthorEmail),
		e.FieldLock, e.Status, e.Attempts, tstr(e.NextAt), tstr(e.CreatedAt), e.LastError)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const outboxCols = `id, instance, kind, target_id, payload, author_email,
	field_lock, status, attempts, next_at, created_at, last_error`

func scanOutbox(rows *sql.Rows) (OutboxEntry, error) {
	var e OutboxEntry
	var payload, next, created string
	if err := rows.Scan(&e.ID, &e.Instance, &e.Kind, &e.TargetID, &payload,
		&e.AuthorEmail, &e.FieldLock, &e.Status, &e.Attempts, &next, &created,
		&e.LastError); err != nil {
		return OutboxEntry{}, err
	}
	_ = json.Unmarshal([]byte(payload), &e.Payload)
	e.NextAt, e.CreatedAt = tparse(next), tparse(created)
	return e, nil
}

// DueOutbox returns pending entries of the given kind whose backoff has elapsed,
// oldest first so a queue drains in the order it was written.
func (s *Store) DueOutbox(kind string, now time.Time, limit int) ([]OutboxEntry, error) {
	rows, err := s.db.Query(`
		SELECT `+outboxCols+` FROM outbox
		WHERE status = ? AND kind = ? AND (next_at = '' OR next_at <= ?)
		ORDER BY id LIMIT ?`, OutPending, kind, tstr(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOutbox(rows)
}

// DraftsFor returns one person's pending comment drafts on a work item.
// Scoped by author because a draft can only ever be re-sent by its author.
func (s *Store) DraftsFor(instance, targetID, authorEmail string) ([]OutboxEntry, error) {
	rows, err := s.db.Query(`
		SELECT `+outboxCols+` FROM outbox
		WHERE instance = ? AND target_id = ? AND kind = ? AND status = ?
		  AND author_email = ?
		ORDER BY id`, instance, targetID, OutComment, OutPending, strings.ToLower(authorEmail))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOutbox(rows)
}

// Draft loads one entry by id, scoped to its author so nobody can re-send or
// discard someone else's words.
func (s *Store) Draft(id int64, authorEmail string) (OutboxEntry, bool, error) {
	rows, err := s.db.Query(`
		SELECT `+outboxCols+` FROM outbox WHERE id = ? AND author_email = ?`,
		id, strings.ToLower(authorEmail))
	if err != nil {
		return OutboxEntry{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return OutboxEntry{}, false, rows.Err()
	}
	e, err := scanOutbox(rows)
	return e, err == nil, err
}

// ListOutbox returns every entry for the admin surface, newest first.
func (s *Store) ListOutbox(limit int) ([]OutboxEntry, error) {
	rows, err := s.db.Query(`SELECT `+outboxCols+` FROM outbox ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOutbox(rows)
}

// LockedFields reports, per work item, which mirror columns have a pending write
// and must therefore not be overwritten by an incoming sync. This is what stops
// a sync pass reverting a change that has not landed in Plane yet.
func (s *Store) LockedFields(instance string, ids []string) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	q := `SELECT target_id, field_lock FROM outbox
	      WHERE instance = ? AND status = ? AND field_lock <> ''
	        AND target_id IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
	args := []any{instance, OutPending}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var target, field string
		if err := rows.Scan(&target, &field); err != nil {
			return nil, err
		}
		if out[target] == nil {
			out[target] = map[string]bool{}
		}
		out[target][field] = true
	}
	return out, rows.Err()
}

// CompleteOutbox removes an entry that landed. Done rows are deleted rather than
// marked: the queue is a worklist, not an audit log, and a table that only grows
// is a slow leak (the mirror's retention problem, avoided here by construction).
func (s *Store) CompleteOutbox(id int64) error {
	_, err := s.db.Exec(`DELETE FROM outbox WHERE id = ?`, id)
	return err
}

// FailOutbox records an attempt that did not land. Past maxAttempts the entry is
// abandoned: it stops being retried and stops locking its field, so Plane's own
// value becomes truth again — but the row STAYS, visible on the admin surface,
// because silently dropping someone's write is the one thing an outbox must
// never do.
func (s *Store) FailOutbox(id int64, next time.Time, maxAttempts int, reason string) error {
	_, err := s.db.Exec(`
		UPDATE outbox
		SET attempts = attempts + 1,
		    next_at = ?,
		    last_error = ?,
		    status = CASE WHEN attempts + 1 >= ? THEN ? ELSE status END,
		    field_lock = CASE WHEN attempts + 1 >= ? THEN '' ELSE field_lock END
		WHERE id = ?`,
		tstr(next), reason, maxAttempts, OutAbandoned, maxAttempts, id)
	return err
}

// DiscardOutbox drops an entry outright (a user discarding their own draft, or
// an admin clearing a stuck row).
func (s *Store) DiscardOutbox(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM outbox WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountOutbox reports pending and abandoned totals for the admin surface.
func (s *Store) CountOutbox() (pending, abandoned int, err error) {
	err = s.db.QueryRow(`
		SELECT
		  sum(CASE WHEN status = ? THEN 1 ELSE 0 END),
		  sum(CASE WHEN status = ? THEN 1 ELSE 0 END)
		FROM outbox`, OutPending, OutAbandoned).Scan(&nullInt{&pending}, &nullInt{&abandoned})
	return pending, abandoned, err
}

func collectOutbox(rows *sql.Rows) ([]OutboxEntry, error) {
	var out []OutboxEntry
	for rows.Next() {
		e, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// nullInt scans a possibly-NULL aggregate into an int (sum() over no rows).
type nullInt struct{ p *int }

func (n *nullInt) Scan(v any) error {
	if v == nil {
		*n.p = 0
		return nil
	}
	switch t := v.(type) {
	case int64:
		*n.p = int(t)
	default:
		return fmt.Errorf("outbox count: unexpected %T", v)
	}
	return nil
}

func tstr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func tparse(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
