package migrations

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// What each thread's evidence adds up to.
//
// The split follows the plan: the VIEW aggregates and GO decides. Aggregation over
// `events` — which will be the largest table — is what SQL is for; the ladder
// (Hot/Warm/Cooling/Dormant), its ordering rules and its reason codes are what
// `internal/heat`'s tests pin, and they port unchanged.
//
// The score is not here either. It could be — exp() and friends exist in
// modernc.org/sqlite, and a view joining the `tuning` ROW recalibrates with an
// UPDATE rather than a migration. What argues against it is that `unixepoch()`
// makes time an AMBIENT input: a pure function takes `now` as an argument, which
// is why its curve can be pinned at a fixed instant in a test and why asking what
// something looked like last Tuesday stays possible.
func init() {
	m.Register(func(app core.App) error {
		warming := []string{"document-changed", "thread-created", "thread-completed", "link-added"}
		quoted := "'" + strings.Join(warming, "','") + "'"

		ev := core.NewViewCollection("thread_evidence")
		// One line, and every computed column CAST. PocketBase parses this SELECT
		// list itself to derive the view's fields: anything it cannot split comes
		// back as `invalid identifier parts`, and anything uncast is typed `json`,
		// which quotes the value and pushes filters through JSON_EXTRACT.
		ev.ViewQuery = "SELECT t.id AS id, t.workspace AS workspace, t.bubble AS bubble," +
			" t.seq AS seq, t.name AS name, t.state AS state, t.created AS created," +
			" CAST((SELECT MAX(e.at) FROM events e WHERE e.target = t.id AND e.kind IN (" + quoted + ")) AS TEXT) AS last_warm_at," +
			" CAST((SELECT MAX(e.at) FROM events e WHERE e.target = t.id) AS TEXT) AS last_any_at," +
			" CAST((SELECT COUNT(*) FROM events e WHERE e.target = t.id AND e.kind IN (" + quoted + ")) AS INT) AS warm_count" +
			" FROM threads t"

		seen := orLead(`workspace.memberships_via_workspace.user ?= @request.auth.id`)
		ev.ListRule = types.Pointer(seen)
		ev.ViewRule = types.Pointer(seen)
		return app.Save(ev)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("thread_evidence")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
