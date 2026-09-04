// Package bubble holds the server-side rules that a collection rule cannot state.
//
// PocketBase's API rules are filters: they answer "may this request touch this
// row". Anything that has to HAPPEN as a consequence of a write lives here.
package bubble

import (
	"github.com/pocketbase/pocketbase/core"
)

// Register binds Bubble's hooks to the app.
func Register(app core.App) {
	app.OnRecordCreateRequest("workspaces").BindFunc(foundingMembership)
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
	// workspace memberless — see the note in PLAN.md; the dashboard is the one
	// place that can then fix it.
	if e.Auth == nil || e.Auth.Collection().Name != "users" {
		return e.Next()
	}

	return e.App.RunInTransaction(func(txApp core.App) error {
		// Swap the event's app for the transactional one so e.Next() — the actual
		// save — runs inside this transaction rather than beside it.
		outer := e.App
		e.App = txApp
		defer func() { e.App = outer }()

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
		return txApp.Save(m)
	})
}
