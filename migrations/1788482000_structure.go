package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Phase 1a: the structure tier under a workspace — bubbles, threads, and the
// things Plane used to own for us (states, labels, relations, links, comments).
//
// Every collection here is insecure until its rule says which workspace it belongs
// to. Nothing supplies that boundary any more, so each rule is written by hand and
// proved against a running server rather than reasoned about.
func init() {
	m.Register(upStructure, downStructure)
}

// Rule fragments. `workspace.` is a hop through the relation field into
// `workspaces`, and from there the same back-relation phase 0 uses. Two hops for
// anything hanging off a thread; PocketBase allows six.
const (
	wsMember = `workspace.memberships_via_workspace.user ?= @request.auth.id`
	wsLead   = `workspace.memberships_via_workspace.user ?= @request.auth.id && workspace.memberships_via_workspace.role ?= 'lead'`

	threadMember = `thread.workspace.memberships_via_workspace.user ?= @request.auth.id`
)

func upStructure(app core.App) error {
	ws, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		return err
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}

	wsField := func(cascade bool) *core.RelationField {
		return &core.RelationField{
			Name: "workspace", CollectionId: ws.Id,
			Required: true, MaxSelect: 1, CascadeDelete: cascade,
		}
	}
	stamps := func(c *core.Collection) {
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	}

	// ---- states: the workflow, which Plane used to define ----
	//
	// `group` is what code matches on; `name` is what a person reads and may be in
	// any language. v0 learned that the hard way against a localised Plane.
	states := core.NewBaseCollection("states")
	states.Fields.Add(wsField(true))
	states.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 60})
	states.Fields.Add(&core.SelectField{
		Name:      "group",
		Values:    []string{"backlog", "unstarted", "started", "completed", "cancelled"},
		MaxSelect: 1, Required: true,
	})
	states.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
	states.Fields.Add(&core.BoolField{Name: "is_default"})
	stamps(states)
	states.AddIndex("idx_states_workspace_name", true, "workspace, name", "")
	// Configuration of the workspace: everyone reads it, a lead shapes it.
	states.ListRule = types.Pointer(wsMember)
	states.ViewRule = types.Pointer(wsMember)
	states.CreateRule = types.Pointer(wsLead)
	states.UpdateRule = types.Pointer(wsLead)
	states.DeleteRule = types.Pointer(wsLead)
	if err := app.Save(states); err != nil {
		return err
	}

	// ---- labels: what KIND of work a thread is ----
	labels := core.NewBaseCollection("labels")
	labels.Fields.Add(wsField(true))
	labels.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 60})
	labels.Fields.Add(&core.TextField{Name: "color", Max: 20})
	stamps(labels)
	labels.AddIndex("idx_labels_workspace_name", true, "workspace, name", "")
	labels.ListRule = types.Pointer(wsMember)
	labels.ViewRule = types.Pointer(wsMember)
	// Unlike states, a label is created in passing while classifying work — a verb
	// that needs a lead is a verb nobody uses.
	labels.CreateRule = types.Pointer(wsMember)
	labels.UpdateRule = types.Pointer(wsMember)
	labels.DeleteRule = types.Pointer(wsLead)
	if err := app.Save(labels); err != nil {
		return err
	}

	// ---- bubbles: the unit of attention ----
	bubbles := core.NewBaseCollection("bubbles")
	bubbles.Fields.Add(wsField(true))
	bubbles.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 120})
	bubbles.Fields.Add(&core.TextField{Name: "outcome", Max: 500})
	bubbles.Fields.Add(&core.RelationField{
		Name: "owner", CollectionId: users.Id, MaxSelect: 1,
	})
	// How it ended, in the words of whoever closed it. `closed_at` set rather than
	// a boolean: closing is an event with a time, and the time is what the model
	// measures against.
	bubbles.Fields.Add(&core.TextField{Name: "closure", Max: 500})
	bubbles.Fields.Add(&core.DateField{Name: "closed_at"})
	bubbles.Fields.Add(&core.SelectField{
		Name: "stage", Values: []string{"reviewed"}, MaxSelect: 1,
	})
	stamps(bubbles)
	bubbles.AddIndex("idx_bubbles_workspace", false, "workspace", "")
	bubbles.ListRule = types.Pointer(wsMember)
	bubbles.ViewRule = types.Pointer(wsMember)
	bubbles.CreateRule = types.Pointer(wsMember)
	bubbles.UpdateRule = types.Pointer(wsMember)
	// Lead only, because a bubble is where threads live and deleting one is the
	// most destructive thing a member could do by accident.
	bubbles.DeleteRule = types.Pointer(wsLead)
	if err := app.Save(bubbles); err != nil {
		return err
	}

	// ---- threads: one executable unit of work ----
	threads := core.NewBaseCollection("threads")
	threads.Fields.Add(wsField(true))
	// NOT required, and NOT cascade-deleted. A thread can exist outside any bubble
	// (that is what a move passes through), and deleting a bubble must never
	// destroy the writing inside its threads — the one thing this system holds that
	// nothing else does.
	threads.Fields.Add(&core.RelationField{
		Name: "bubble", CollectionId: bubbles.Id, MaxSelect: 1,
	})
	threads.Fields.Add(&core.NumberField{Name: "seq", OnlyInt: true})
	threads.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 200})
	threads.Fields.Add(&core.RelationField{
		Name: "state", CollectionId: states.Id, MaxSelect: 1,
	})
	threads.Fields.Add(&core.RelationField{
		Name: "assignees", CollectionId: users.Id, MaxSelect: 20,
	})
	threads.Fields.Add(&core.RelationField{
		Name: "labels", CollectionId: labels.Id, MaxSelect: 20,
	})
	stamps(threads)
	threads.Fields.Add(&core.DateField{Name: "completed_at"})
	threads.Fields.Add(&core.DateField{Name: "due_date"})
	// The two inputs to the determinant map. `priority` is deliberately NOT a
	// column: a stored verdict is one somebody can write inconsistently with the
	// map, which turns the map into decoration. Phase 2 derives it.
	threads.Fields.Add(&core.SelectField{
		Name: "impact", Values: []string{"high", "mid", "low"}, MaxSelect: 1,
	})
	threads.Fields.Add(&core.SelectField{
		Name: "urgency", Values: []string{"high", "mid", "low"}, MaxSelect: 1,
	})
	threads.AddIndex("idx_threads_workspace", false, "workspace", "")
	threads.AddIndex("idx_threads_bubble", false, "bubble", "")
	// The short number people say out loud. Unique per workspace so a racing pair
	// of creates fails loudly instead of quietly sharing one.
	threads.AddIndex("idx_threads_workspace_seq", true, "workspace, seq", "")
	threads.ListRule = types.Pointer(wsMember)
	threads.ViewRule = types.Pointer(wsMember)
	threads.CreateRule = types.Pointer(wsMember)
	threads.UpdateRule = types.Pointer(wsMember)
	threads.DeleteRule = types.Pointer(wsMember)
	if err := app.Save(threads); err != nil {
		return err
	}

	// A thread's parent, for revisions and sub-work. Added after the save because a
	// relation to the collection being created needs its id to exist.
	threads.Fields.Add(&core.RelationField{
		Name: "parent", CollectionId: threads.Id, MaxSelect: 1,
	})
	if err := app.Save(threads); err != nil {
		return err
	}

	thField := &core.RelationField{
		Name: "thread", CollectionId: threads.Id,
		Required: true, MaxSelect: 1, CascadeDelete: true,
	}

	// ---- thread_links: where the evidence lives ----
	//
	// Landing one is production: it is proof that reality changed outside this
	// tool. Phase 2 turns that into an event.
	links := core.NewBaseCollection("thread_links")
	links.Fields.Add(thField)
	links.Fields.Add(&core.URLField{Name: "url", Required: true})
	links.Fields.Add(&core.TextField{Name: "title", Max: 200})
	links.Fields.Add(&core.RelationField{Name: "added_by", CollectionId: users.Id, MaxSelect: 1})
	links.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	links.AddIndex("idx_links_thread", false, "thread", "")
	links.ListRule = types.Pointer(threadMember)
	links.ViewRule = types.Pointer(threadMember)
	links.CreateRule = types.Pointer(threadMember)
	links.UpdateRule = types.Pointer(threadMember)
	links.DeleteRule = types.Pointer(threadMember)
	if err := app.Save(links); err != nil {
		return err
	}

	// ---- thread_relations: what this depends on ----
	//
	// A blocker described in a sentence is a blocker no other surface can see.
	rels := core.NewBaseCollection("thread_relations")
	rels.Fields.Add(thField)
	rels.Fields.Add(&core.SelectField{
		Name:      "type",
		Values:    []string{"relates_to", "duplicate", "blocking", "blocked_by"},
		MaxSelect: 1, Required: true,
	})
	rels.Fields.Add(&core.RelationField{
		Name: "related", CollectionId: threads.Id,
		Required: true, MaxSelect: 1, CascadeDelete: true,
	})
	rels.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	rels.AddIndex("idx_relations_unique", true, "thread, type, related", "")
	rels.AddIndex("idx_relations_thread", false, "thread", "")
	rels.ListRule = types.Pointer(threadMember)
	rels.ViewRule = types.Pointer(threadMember)
	rels.CreateRule = types.Pointer(threadMember)
	rels.UpdateRule = types.Pointer(threadMember)
	rels.DeleteRule = types.Pointer(threadMember)
	if err := app.Save(rels); err != nil {
		return err
	}

	// ---- comments: pulse, not production ----
	//
	// A recent comment stops a bubble being called a grave; it never warms one.
	// That distinction is phase 2's, but it is why this is its own collection and
	// not a field on the thread.
	comments := core.NewBaseCollection("comments")
	comments.Fields.Add(thField)
	comments.Fields.Add(&core.RelationField{
		Name: "author", CollectionId: users.Id,
		Required: true, MaxSelect: 1,
	})
	comments.Fields.Add(&core.EditorField{Name: "body", Required: true})
	stamps(comments)
	comments.AddIndex("idx_comments_thread", false, "thread", "")
	comments.ListRule = types.Pointer(threadMember)
	comments.ViewRule = types.Pointer(threadMember)
	comments.CreateRule = types.Pointer(threadMember)
	// Editing somebody else's words is not a thing a member gets to do, even a
	// lead. The author owns what they said.
	mine := threadMember + ` && author = @request.auth.id`
	comments.UpdateRule = types.Pointer(mine)
	comments.DeleteRule = types.Pointer(mine)
	return app.Save(comments)
}

func downStructure(app core.App) error {
	// Reverse dependency order.
	for _, name := range []string{
		"comments", "thread_relations", "thread_links", "threads", "bubbles",
		"labels", "states",
	} {
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
