package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Personal writing is not a thread, workspace wiki or evidence. Global leads
// have no exception to ownership. Protected files enforce the same ViewRule.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		ws, err := app.FindCollectionByNameOrId("workspaces")
		if err != nil {
			return err
		}
		mine := `@request.auth.id != '' && owner = @request.auth.id`
		project := `(workspace = '' || @request.auth.role = 'lead' || workspace.memberships_via_workspace.user ?= @request.auth.id)`
		add := func(c *core.Collection) {
			c.Fields.Add(&core.RelationField{Name: "owner", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true})
			c.Fields.Add(&core.RelationField{Name: "workspace", CollectionId: ws.Id, MaxSelect: 1})
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
			c.ListRule = types.Pointer(mine)
			c.ViewRule = types.Pointer(mine)
			c.CreateRule = types.Pointer(mine + " && " + project)
			c.UpdateRule = types.Pointer(mine + ` && @request.body.owner:isset = false && ` + project)
			c.DeleteRule = types.Pointer(mine)
			c.AddIndex("idx_"+c.Name+"_owner", false, "owner", "")
		}
		notes := core.NewBaseCollection("personal_notes")
		add(notes)
		notes.Fields.Add(&core.TextField{Name: "title", Required: true, Max: 150})
		notes.Fields.Add(&core.TextField{Name: "body", Max: 128 * 1024})
		notes.Fields.Add(&core.TextField{Name: "color", Max: 7, Pattern: `^#[0-9a-fA-F]{6}$`})
		if err := app.Save(notes); err != nil {
			return err
		}
		assets := core.NewBaseCollection("personal_assets")
		add(assets)
		assets.Fields.Add(&core.RelationField{Name: "note", CollectionId: notes.Id, MaxSelect: 1, CascadeDelete: true})
		assets.Fields.Add(&core.FileField{Name: "file", Required: true, MaxSelect: 1, MaxSize: 20 * 1024 * 1024, Protected: true})
		assets.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 200})
		assets.CreateRule = types.Pointer(mine + " && " + project + ` && (note = '' || note.owner = @request.auth.id)`)
		assets.UpdateRule = types.Pointer(mine + ` && @request.body.owner:isset = false && ` + project + ` && (note = '' || note.owner = @request.auth.id)`)
		return app.Save(assets)
	}, func(app core.App) error {
		for _, name := range []string{"personal_assets", "personal_notes"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err = app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}
