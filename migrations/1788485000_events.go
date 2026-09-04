package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Phase 2: the evidence log.
//
// This is the whole reason the record moved. v0 INFERRED evidence — it diffed
// hashes and counters on a sync pass, because the writes happened in Plane — so
// ten edits in a cycle were one event, authorship never reached the event, and a
// change reverted between two passes never happened. With Bubble owning the write
// path, a production event is observed AS IT OCCURS.
//
// Append-only, and never written by a client: every row here is the server saying
// what it just did.
func init() {
	m.Register(func(app core.App) error {
		ws, err := app.FindCollectionByNameOrId("workspaces")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		ev := core.NewBaseCollection("events")
		ev.Fields.Add(&core.RelationField{
			Name: "workspace", CollectionId: ws.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true,
		})
		ev.Fields.Add(&core.SelectField{
			Name: "target_type", Values: []string{"thread", "workspace"},
			MaxSelect: 1, Required: true,
		})
		// Plain text, NOT a relation, so deleting a thread does not delete the
		// record that its work happened. The evidence outlives the row it was about.
		ev.Fields.Add(&core.TextField{Name: "target", Max: 40})
		ev.Fields.Add(&core.SelectField{
			Name: "kind",
			Values: []string{
				"document-changed", // any write under threads/ — see below
				"thread-created",
				"thread-completed",
				"link-added",
				"doc-changed", // a wiki page: recorded, never warms
				"comment",     // pulse
			},
			MaxSelect: 1, Required: true,
		})
		ev.Fields.Add(&core.RelationField{
			Name: "actor", CollectionId: users.Id, MaxSelect: 1,
		})
		// Explicit rather than an autodate, so a backfill can write history at the
		// time it actually happened.
		ev.Fields.Add(&core.DateField{Name: "at", Required: true})
		ev.Fields.Add(&core.JSONField{Name: "meta", MaxSize: 4000})

		ev.AddIndex("idx_events_workspace_at", false, "workspace, at", "")
		ev.AddIndex("idx_events_target", false, "target, at", "")

		seen := orLead(`workspace.memberships_via_workspace.user ?= @request.auth.id`)
		ev.ListRule = types.Pointer(seen)
		ev.ViewRule = types.Pointer(seen)
		// Nobody writes evidence from a client. A row here is the server's account
		// of what it did; a client that could add one could manufacture heat, and
		// the one rule the model has is that activity is not evidence.
		ev.CreateRule = nil
		ev.UpdateRule = nil
		ev.DeleteRule = nil

		return app.Save(ev)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("events")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
