// Package bubble holds the server-side rules that a collection rule cannot state.
//
// PocketBase's API rules are filters: they answer "may this request touch this
// row". Three kinds of thing cannot be said that way and live here instead —
// something that has to HAPPEN as a consequence of a write, a field the client
// must not be trusted to set, and an integrity rule that spans two rows.
package bubble

import (
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/tree"
)

// Register binds Bubble's hooks and routes to the app.
func Register(app core.App, t *tree.Tree) {
	app.OnRecordCreateRequest("workspaces").BindFunc(foundingMembership)

	app.OnRecordCreateRequest("users").BindFunc(nameable)

	app.OnRecordCreateRequest("threads").BindFunc(assignSeq)
	// Only the bubble. An objective belongs to the DEPARTMENT and not to any
	// workspace, so a thread from any project counting towards one is the point
	// rather than a leak — which is why this guard, briefly written for both,
	// covers one again.
	app.OnRecordCreateRequest("threads").BindFunc(relationInSameWorkspace("bubble", "bubbles"))
	app.OnRecordUpdateRequest("threads").BindFunc(relationInSameWorkspace("bubble", "bubbles"))

	app.OnRecordCreateRequest("comments").BindFunc(stampAuthor("author"))
	app.OnRecordCreateRequest("thread_links").BindFunc(stampAuthor("added_by"))
	app.OnRecordCreateRequest("inbox_items").BindFunc(stampAuthor("captured_by"))

	app.OnRecordUpdateRequest("memberships").BindFunc(keepALead(false))
	app.OnRecordDeleteRequest("memberships").BindFunc(keepALead(true))

	registerDocuments(app, t)
	registerEvidence(app)
	registerBoard(app)
	registerPresence(app)
	registerAssets(app, t)
}

// getenv is here rather than inline so the one place that reads the environment is
// easy to find.
func getenv(k string) string { return os.Getenv(k) }

// keepALead refuses the write that would leave a workspace with no lead.
//
// A lead can demote themselves or delete their own membership, and there is no
// rule that can stop the LAST one doing it: a filter answers "may you touch this
// row", never "how many sibling rows would be left". Without this a workspace
// reaches a state where nobody can invite, rename or close it, and only a
// superuser can repair it from the dashboard.
//
// Checked BEFORE the write rather than after, so nothing has to be rolled back:
// the count plus the intent is enough to know the answer.
//
// Cascade deletes do not come through here — deleting a workspace takes its
// memberships with it through the model layer, not the request layer, which is
// right: a workspace with no lead is a problem, a workspace that no longer exists
// is not.
// `removing` says whether the row is going away entirely. It matters because the
// intent check below reads the request BODY, and a delete has none — an earlier
// version shared one function for both and let every delete through, including
// the last lead's.
func keepALead(removing bool) func(*core.RecordRequestEvent) error {
	return func(e *core.RecordRequestEvent) error {
		return keepALeadFn(e, removing)
	}
}

func keepALeadFn(e *core.RecordRequestEvent, removing bool) error {
	current, err := e.App.FindRecordById("memberships", e.Record.Id)
	if err != nil {
		return e.Next() // creating, or already gone: not our case
	}
	if current.GetString("role") != "lead" {
		return e.Next() // demoting or removing a member changes nothing
	}

	if !removing {
		// An update that keeps the row a lead, or does not mention the role at all,
		// takes nothing away. Read the intent from the request rather than from
		// e.Record, which still holds the stored values.
		info, err := e.RequestInfo()
		if err != nil || info == nil {
			return e.Next()
		}
		role, ok := info.Body["role"]
		if !ok || role == "lead" {
			return e.Next()
		}
	}

	var leads int
	err = e.App.DB().
		NewQuery("SELECT COUNT(*) FROM memberships WHERE workspace = {:ws} AND role = 'lead'").
		Bind(map[string]any{"ws": current.GetString("workspace")}).
		Row(&leads)
	if err != nil {
		return fmt.Errorf("count leads: %w", err)
	}
	if leads <= 1 {
		return fmt.Errorf("this is the workspace's last lead — promote somebody else first")
	}
	return e.Next()
}

// foundingMembership makes a workspace's creator its lead, in the same
// transaction that creates the workspace.
//
// The workspace rules are `memberships_via_workspace.user ?= @request.auth.id`,
// so a workspace with no membership rows is invisible to everyone — including the
// person who just created it. Two separate writes would leave that state reachable
// whenever the second one failed: an orphan nobody can see and only a superuser
// can find. So both land together or neither does.
func foundingMembership(e *core.RecordRequestEvent) error {
	// A superuser creating from the dashboard is not a `users` record, and the
	// membership relation points at `users`. Let the create through and leave the
	// workspace memberless — see PLAN.md; the dashboard is the one place that can
	// then fix it.
	if !isPerson(e) {
		return e.Next()
	}

	return e.App.RunInTransaction(func(txApp core.App) error {
		defer swapApp(e, txApp)()

		if err := e.Next(); err != nil {
			return err
		}

		memberships, err := txApp.FindCollectionByNameOrId("memberships")
		if err != nil {
			return err
		}
		m := core.NewRecord(memberships)
		m.Set("workspace", e.Record.Id)
		m.Set("user", e.Auth.Id)
		m.Set("role", "lead")
		if err := txApp.Save(m); err != nil {
			return err
		}
		return foundingStates(txApp, e.Record.Id)
	})
}

// foundingStates gives a new workspace a workflow to work with.
//
// A workspace with no states is a workspace where nothing can be COMPLETED:
// completion is a thread reaching a state whose group is `completed`, so
// without one the verb has nowhere to point and "Terminar" answers "this
// workspace has no state for that" — which is true, and useless as a first
// experience.
//
// Three, because three is the argument every team actually has: not started,
// doing, done. They are ordinary rows — rename them, add to them, delete them;
// the only thing that matters to the model is which GROUP each one is in.
func foundingStates(app core.App, workspace string) error {
	states, err := app.FindCollectionByNameOrId("states")
	if err != nil {
		return err
	}
	for i, s := range []struct {
		name, group string
		isDefault   bool
	}{
		{"Por hacer", "backlog", true},
		{"En curso", "started", false},
		{"Hecho", "completed", false},
	} {
		r := core.NewRecord(states)
		r.Set("workspace", workspace)
		r.Set("name", s.name)
		r.Set("group", s.group)
		r.Set("position", i+1)
		r.Set("is_default", s.isDefault)
		if err := app.Save(r); err != nil {
			return err
		}
	}
	return nil
}

// assignSeq gives a new thread the next number in its workspace.
//
// The short number is what people say out loud, so it has to be per-workspace and
// small rather than a uuid.
//
// Deliberately NOT wrapped in RunInTransaction. Wrapping `e.Next()` puts the whole
// request inside a transaction — and `e.Next()` is what WRITES THE RESPONSE, so a
// rollback after it returns produces an HTTP 200 carrying a record id that does not
// exist. That was observed here, not theorised: a create answered 200 with `seq: 3`
// and left no row. One field on one record needs no transaction of its own; the
// save has one already.
//
// The race that leaves is two creates reading the same MAX. The unique index on
// (workspace, seq) refuses the second rather than letting two threads quietly share
// a number, which is the right failure: loud, and retryable.
func assignSeq(e *core.RecordRequestEvent) error {
	var next int
	err := e.App.DB().
		NewQuery("SELECT COALESCE(MAX(seq), 0) + 1 FROM threads WHERE workspace = {:ws}").
		Bind(map[string]any{"ws": e.Record.GetString("workspace")}).
		Row(&next)
	if err != nil {
		return fmt.Errorf("next seq: %w", err)
	}
	e.Record.Set("seq", next)
	return e.Next()
}

// relationInSameWorkspace refuses a thread pointing at a row that belongs to some
// other workspace.
//
// Nothing in a collection rule can compare two rows like this, and without it the
// workspace boundary has a hole exactly one relation wide: a member of A could
// file their thread into a bubble in B and it would show up on B's board.
//
// Written as a factory because it was, for one migration, also applied to the
// objective — until the objective stopped belonging to a workspace at all.
func relationInSameWorkspace(field, collection string) func(*core.RecordRequestEvent) error {
	return func(e *core.RecordRequestEvent) error {
		id := e.Record.GetString(field)
		if id == "" {
			return e.Next() // a thread outside any bubble, or under no objective
		}
		r, err := e.App.FindRecordById(collection, id)
		if err != nil {
			return fmt.Errorf("%s %q not found", field, id)
		}
		if r.GetString("workspace") != e.Record.GetString("workspace") {
			return fmt.Errorf("%s %q belongs to another workspace", field, id)
		}
		return e.Next()
	}
}

// stampAuthor overwrites an authorship field with whoever is actually making the
// request.
//
// The collection rule can only say "you may edit a comment whose author is you",
// which protects updates and says nothing about creates — so without this a member
// could post a comment signed as somebody else. Authorship is not an input.
func stampAuthor(field string) func(*core.RecordRequestEvent) error {
	return func(e *core.RecordRequestEvent) error {
		if isPerson(e) {
			e.Record.Set(field, e.Auth.Id)
		}
		return e.Next()
	}
}

// isPerson reports whether the request is authenticated as a member of `users`.
// A superuser is not: it is the operator of the box, not somebody in the model.
func isPerson(e *core.RecordRequestEvent) bool {
	return e.Auth != nil && e.Auth.Collection().Name == "users"
}

// swapApp points the event at a transactional app so e.Next() — the actual save —
// runs inside the transaction rather than beside it, and returns the undo.
func swapApp(e *core.RecordRequestEvent, txApp core.App) func() {
	outer := e.App
	e.App = txApp
	return func() { e.App = outer }
}

// nameable makes a new person's email visible to their colleagues.
//
// PocketBase hides `email` from everybody but the record's owner unless
// `emailVisibility` is set, and that default is written for a public app where
// the other accounts are strangers. Here they are the department: anybody signed
// in may already LIST people — that is what makes an invitation possible — so
// the only thing the default achieved was a roster of rows reading "sin
// nombre", because `display_name` is optional and an account made from the
// dashboard rarely has one.
//
// The name still wins wherever both exist. This is the fallback that makes a
// person identifiable at all.
func nameable(e *core.RecordRequestEvent) error {
	e.Record.Set("emailVisibility", true)
	return e.Next()
}
