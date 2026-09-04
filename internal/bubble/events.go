package bubble

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Evidence kinds.
//
// One kind covers every change to a thread's document. v0 had three —
// `body-updated`, `logbook-updated`, `completed-todo` — because it was inferring
// what had happened from hashes and counters, and the shape of the change was the
// only clue it had about the KIND of work. Owning the write path removes the need
// to guess, and with it the need to distinguish: an edit to a thread is
// production, whatever part of it moved.
//
// What is NOT collapsed is the evidence that is not a file write. A link landing
// and a thread completing say something a document edit does not, and folding them
// in would lose signal quietly.
const (
	EvDocumentChanged = "document-changed" // any write under threads/
	EvThreadCreated   = "thread-created"   // somebody defined a piece of work
	EvThreadCompleted = "thread-completed" // it reached a completed state
	EvLinkAdded       = "link-added"       // external evidence was published

	// A wiki page is recorded and never warms anything. A glossary somebody
	// tidied did not move the work; if it turns out that is where the work
	// actually happens, the rows are already here to reconsider it without a
	// backfill.
	EvDocChanged = "doc-changed"

	// Pulse, not progress: it holds 😴 and blocks 🪦, and never wakes anything.
	EvComment = "comment"
)

// warms says whether a kind counts towards buoyancy.
func warms(kind string) bool {
	switch kind {
	case EvDocumentChanged, EvThreadCreated, EvThreadCompleted, EvLinkAdded:
		return true
	}
	return false
}

// record appends one evidence row.
//
// Best effort by design: evidence is the account of a write that already
// succeeded, so failing to record it must not undo the work. It is logged loudly
// instead, because silence here is how heat starts lying.
func record(app core.App, workspace, targetType, target, kind, actor string, meta map[string]any) {
	col, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		app.Logger().Error("no events collection", "err", err)
		return
	}
	r := core.NewRecord(col)
	r.Set("workspace", workspace)
	r.Set("target_type", targetType)
	r.Set("target", target)
	r.Set("kind", kind)
	r.Set("at", types_Now())
	if actor != "" {
		r.Set("actor", actor)
	}
	if meta != nil {
		r.Set("meta", meta)
	}
	if err := app.Save(r); err != nil {
		app.Logger().Error("could not record evidence",
			"kind", kind, "target", target, "err", err)
	}
}

func types_Now() string { return time.Now().UTC().Format("2006-01-02 15:04:05.000Z") }

// registerEvidence records the production that is not a file write.
//
// A document edit is the common case and is recorded where the write happens; so
// is a thread being created, because that hook already has the request. These are
// what is left: a link landing, a comment, and a thread finishing.
//
// Links and comments need no request context — the server already stamped who
// wrote them, so the row itself says who to credit.
func registerEvidence(app core.App) {
	app.OnRecordAfterCreateSuccess("thread_links").BindFunc(func(e *core.RecordEvent) error {
		th, err := e.App.FindRecordById("threads", e.Record.GetString("thread"))
		if err == nil {
			record(e.App, th.GetString("workspace"), "thread", th.Id,
				EvLinkAdded, e.Record.GetString("added_by"),
				map[string]any{"url": e.Record.GetString("url")})
		}
		return e.Next()
	})

	app.OnRecordAfterCreateSuccess("comments").BindFunc(func(e *core.RecordEvent) error {
		th, err := e.App.FindRecordById("threads", e.Record.GetString("thread"))
		if err == nil {
			record(e.App, th.GetString("workspace"), "thread", th.Id,
				EvComment, e.Record.GetString("author"), nil)
		}
		return e.Next()
	})

	// Completion is a STATE reaching the completed group, not a field somebody
	// sets. Recorded on the transition only: saving a thread that is already
	// finished is not finishing it again.
	app.OnRecordUpdateRequest("threads").BindFunc(func(e *core.RecordRequestEvent) error {
		was := completedState(e.App, currentState(e.App, e.Record.Id))
		if err := e.Next(); err != nil {
			return err
		}
		if !was && completedState(e.App, e.Record.GetString("state")) {
			actor := ""
			if isPersonAuth(e.Auth) {
				actor = e.Auth.Id
			}
			record(e.App, e.Record.GetString("workspace"), "thread", e.Record.Id,
				EvThreadCompleted, actor, map[string]any{"name": e.Record.GetString("name")})
		}
		return nil
	})
}

func currentState(app core.App, threadID string) string {
	th, err := app.FindRecordById("threads", threadID)
	if err != nil {
		return ""
	}
	return th.GetString("state")
}

func completedState(app core.App, stateID string) bool {
	if stateID == "" {
		return false
	}
	st, err := app.FindRecordById("states", stateID)
	if err != nil {
		return false
	}
	return st.GetString("group") == "completed"
}
