package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// The planner belongs to the DEPARTMENT, not to a workspace.
//
// 5a filed objectives and inbox items under a workspace, which was the wrong
// scope and the model already said so: the strategic layer is the department
// head, who "sees every workspace and administers it without being a member of
// any". The objectives that surface when anybody describes this out loud —
// clients, profitability, ISO 9001, technical debt — are not a project's. The
// projects are what HANGS from them, and an objective that lives inside one of
// them cannot be what the others are measured against.
//
// The inbox has the same shape for a different reason: a note arrives before
// anybody knows what it belongs to. That is the whole point of an inbox, and
// asking which project it is for at the door is asking the question that
// triaging exists to answer.
//
// So both lose `workspace`, and with it the per-workspace rules. What replaces
// them is the split the rest of the model already uses: everybody signed in
// READS the department's plan, and the global lead is the one who shapes it.
func init() {
	m.Register(upPlannerDepartment, downPlannerDepartment)
}

// Anybody who works here. Not `wsMember`: there is no workspace left to be a
// member of, and the plan of the department is not a secret from the people
// executing it.
const signedIn = `@request.auth.id != ""`

func upPlannerDepartment(app core.App) error {
	objectives, err := app.FindCollectionByNameOrId("objectives")
	if err != nil {
		return err
	}
	objectives.Fields.RemoveByName("workspace")
	// The name was unique per workspace; now it is unique, full stop. Two
	// objectives called the same thing are two people meaning one thing.
	objectives.Indexes = types.JSONArray[string]{}
	objectives.AddIndex("idx_objectives_name", true, "name", "")
	objectives.ListRule = types.Pointer(signedIn)
	objectives.ViewRule = types.Pointer(signedIn)
	objectives.CreateRule = types.Pointer(globalLead)
	objectives.UpdateRule = types.Pointer(globalLead)
	objectives.DeleteRule = types.Pointer(globalLead)
	if err := app.Save(objectives); err != nil {
		return err
	}

	inbox, err := app.FindCollectionByNameOrId("inbox_items")
	if err != nil {
		return err
	}
	inbox.Fields.RemoveByName("workspace")
	inbox.Indexes = types.JSONArray[string]{}
	inbox.AddIndex("idx_inbox_captured_by", false, "captured_by", "")
	inbox.ListRule = types.Pointer(signedIn)
	inbox.ViewRule = types.Pointer(signedIn)
	inbox.CreateRule = types.Pointer(signedIn)
	// Whoever wrote it, or the person whose job triaging is.
	inbox.UpdateRule = types.Pointer(globalLead + ` || captured_by = @request.auth.id`)
	inbox.DeleteRule = types.Pointer(globalLead + ` || captured_by = @request.auth.id`)
	return app.Save(inbox)
}

func downPlannerDepartment(app core.App) error {
	ws, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return err
	}
	back := func(name string) error {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return nil
		}
		c.Fields.Add(&core.RelationField{
			Name: "workspace", CollectionId: ws.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true,
		})
		return app.Save(c)
	}
	if err := back("objectives"); err != nil {
		return err
	}
	return back("inbox_items")
}
