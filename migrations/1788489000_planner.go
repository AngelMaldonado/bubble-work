package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Phase 5: the two things the planner needs that did not exist, and nothing else.
//
// What it deliberately does NOT add is a card. A planner with its own cards is a
// second inventory of work beside the threads, and two lists of the same work
// disagree by Thursday. The kanban's columns are the `states` a workspace
// already defines, and what moves across them is a THREAD — the model's one
// executable unit. The calendar reads `threads.due_date`, which has been there
// since phase 1.
//
// So two collections:
//
//   - `objectives` — the strategic layer. A thread points at one, which is how a
//     lead sees whether a quarter's stated outcome has anything moving under it.
//     Configuration of the workspace, so a lead shapes it and everyone reads it.
//
//   - `inbox_items` — what has been CAPTURED and is not yet work. This is the
//     collection that earns its place: a note is not a thread, has no outcome and
//     no evidence, and forcing it to be one is how a backlog fills with rows
//     nobody meant to commit to. Triaging is what turns one into a thread, and
//     the item keeps a pointer to what it became.
func init() {
	m.Register(upPlanner, downPlanner)
}

func upPlanner(app core.App) error {
	ws, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return err
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	threads, err := app.FindCollectionByNameOrId("threads")
	if err != nil {
		return err
	}

	stamps := func(c *core.Collection) {
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	}
	workspaceField := func() *core.RelationField {
		return &core.RelationField{
			Name: "workspace", CollectionId: ws.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true,
		}
	}

	// ---- objectives: what the work is FOR ----
	//
	// `outcome` is the same word a bubble uses, and means the same thing: what is
	// true when this is done. An objective without one is a heading.
	objectives := core.NewBaseCollection("objectives")
	objectives.Fields.Add(workspaceField())
	objectives.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 160})
	objectives.Fields.Add(&core.TextField{Name: "outcome", Max: 500})
	// When it is meant to be true by. Not a deadline on the threads under it:
	// those carry their own, and a date that cascades is a date nobody set.
	objectives.Fields.Add(&core.DateField{Name: "due_date"})
	objectives.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
	// Closed rather than deleted, so what was attempted stays legible.
	objectives.Fields.Add(&core.DateField{Name: "closed_at"})
	stamps(objectives)
	objectives.AddIndex("idx_objectives_workspace_name", true, "workspace, name", "")
	// `orLead` on every rule, like every collection that came before: the GLOBAL
	// lead — the department head — sees and administers every workspace without
	// being a member of any. A collection added later that forgets this is a
	// collection the strategic layer cannot see, which for objectives would be
	// funny if it were not exactly backwards.
	objectives.ListRule = types.Pointer(orLead(wsMember))
	objectives.ViewRule = types.Pointer(orLead(wsMember))
	// Shaping it is the lead's — the same reasoning as `states`, and the opposite
	// of `labels`, which are created in passing while working.
	objectives.CreateRule = types.Pointer(orLead(wsLead))
	objectives.UpdateRule = types.Pointer(orLead(wsLead))
	objectives.DeleteRule = types.Pointer(orLead(wsLead))
	if err := app.Save(objectives); err != nil {
		return err
	}

	// ---- threads point at an objective ----
	//
	// Optional, and NOT cascading: closing an objective must not delete the work
	// that was done under it. The relation is the thread's, because a thread knows
	// what it is for and an objective should not have to hold a list.
	threads.Fields.Add(&core.RelationField{
		Name: "objective", CollectionId: objectives.Id, MaxSelect: 1,
	})
	if err := app.Save(threads); err != nil {
		return err
	}

	// ---- inbox_items: captured, not yet work ----
	inbox := core.NewBaseCollection("inbox_items")
	inbox.Fields.Add(workspaceField())
	inbox.Fields.Add(&core.TextField{Name: "note", Required: true, Max: 2000})
	// Who wrote it. Forced server-side on create, like a comment's author: it is
	// not an input, and a rule that says "you may edit your own" protects nothing
	// if the create can claim to be somebody else.
	inbox.Fields.Add(&core.RelationField{
		Name: "captured_by", CollectionId: users.Id, Required: true, MaxSelect: 1,
	})
	// What it BECAME. Set when the item is triaged; a triaged item stays in the
	// collection because "we already decided about this" is worth being able to
	// see, and because deleting it loses the only record that it was ever raised.
	inbox.Fields.Add(&core.RelationField{
		Name: "thread", CollectionId: threads.Id, MaxSelect: 1,
	})
	stamps(inbox)
	inbox.AddIndex("idx_inbox_workspace", false, "workspace", "")
	inbox.ListRule = types.Pointer(orLead(wsMember))
	inbox.ViewRule = types.Pointer(orLead(wsMember))
	// Anybody in the workspace may capture. That is the whole point of an inbox:
	// a door that needs permission is a door people route around.
	inbox.CreateRule = types.Pointer(orLead(wsMember))
	// Editing or throwing away a note belongs to whoever wrote it, or to a lead —
	// triaging is the lead's job, and a note nobody can file is a note that stays
	// forever.
	ownerOrLead := orLead("(" + wsMember + " && captured_by = @request.auth.id) || (" + wsLead + ")")
	inbox.UpdateRule = types.Pointer(ownerOrLead)
	inbox.DeleteRule = types.Pointer(ownerOrLead)
	return app.Save(inbox)
}

func downPlanner(app core.App) error {
	threads, err := app.FindCollectionByNameOrId("threads")
	if err == nil {
		if f := threads.Fields.GetByName("objective"); f != nil {
			threads.Fields.RemoveByName("objective")
			if err := app.Save(threads); err != nil {
				return err
			}
		}
	}
	for _, name := range []string{"inbox_items", "objectives"} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		if err := app.Delete(c); err != nil {
			return err
		}
	}
	return nil
}
