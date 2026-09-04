// Package migrations holds Bubble's schema, one versioned file at a time.
//
// v0 applied `CREATE TABLE IF NOT EXISTS` on every open plus a list of
// `ALTER TABLE`s whose errors were discarded, with no schema-version row. That
// works only while every change is additive: a migration that has to REWRITE rows
// has nowhere to record that it ran, and re-running it on every open is not the
// same thing as running it once. These files are the answer — each runs exactly
// once, in name order, and says how to undo itself.
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Rule fragments, written once because getting them wrong is a security bug
// rather than a typo.
//
// `memberships_via_workspace` is a BACK-relation: it resolves to the membership
// rows whose `workspace` field points at THIS workspace. That scoping is the whole
// reason it is safe — the join is emitted as
// `LEFT JOIN memberships ... ON memberships.workspace = workspaces.id`, so the
// conditions cannot match a membership of some other workspace.
//
// `?=` is "any of": at least one joined membership satisfies it. Plain `=` would
// mean EVERY membership of the workspace must satisfy it, which for `isMember`
// would let exactly one person in and lock out everyone else the moment a second
// joined.
const (
	isMember = `memberships_via_workspace.user ?= @request.auth.id`
	isLead   = `memberships_via_workspace.user ?= @request.auth.id && memberships_via_workspace.role ?= 'lead'`
)

func init() {
	m.Register(up, down)
}

func up(app core.App) error {
	// ---- users: PocketBase already ships this auth collection ----
	//
	// It is not created here, only extended. Password, OAuth2, OTP, tokens and the
	// owner-scoped default rules come with it — that is the identity work v0 never
	// got to, and the reason its MCP had to borrow a Plane API key as its
	// credential.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	// Only people are identified here, and only people publish. An agent acts with
	// a person's credential and therefore IS that person for every rule in the
	// system — which is v0's identity model unchanged, and the reason there is no
	// `kind` column distinguishing a human principal from a non-human one. Nothing
	// downstream has to ask.
	users.Fields.Add(&core.TextField{
		Name: "display_name",
		Max:  100,
	})
	if err := app.Save(users); err != nil {
		return err
	}

	// ---- workspaces: the boundary for a body of work ----
	ws := core.NewBaseCollection("workspaces")
	ws.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 100})
	ws.Fields.Add(&core.TextField{
		Name:     "slug",
		Required: true,
		Max:      60,
		Pattern:  `^[a-z0-9]+(-[a-z0-9]+)*$`,
	})
	ws.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	ws.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	ws.AddIndex("idx_workspaces_slug", true, "slug", "")

	// Saved WITHOUT its rules, because they name a back-relation into a collection
	// that does not exist yet — PocketBase validates a rule when the collection is
	// saved, and refuses one it cannot resolve:
	//
	//	failed to load back relation field "memberships_via_workspace" collection
	//
	// So the order is: both collections first, then the rules. A rule cannot be
	// written before the thing it reads.
	if err := app.Save(ws); err != nil {
		return err
	}

	// ---- memberships: who may see what ----
	//
	// This is what every rule in the system reads. v0 derived authorization from a
	// local mirror of Plane's project membership, which meant "who can see this"
	// was only as fresh as the last sync pass. Here it is a row.
	ms := core.NewBaseCollection("memberships")
	ms.Fields.Add(&core.RelationField{
		Name:          "workspace",
		CollectionId:  ws.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	})
	ms.Fields.Add(&core.RelationField{
		Name:          "user",
		CollectionId:  users.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	})
	ms.Fields.Add(&core.SelectField{
		Name:      "role",
		Values:    []string{"lead", "member"},
		MaxSelect: 1,
		Required:  true,
	})
	ms.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	ms.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	// One membership per person per workspace. Two rows would mean two roles, and
	// nothing says which wins.
	ms.AddIndex("idx_memberships_workspace_user", true, "workspace, user", "")

	// You see the roster of a workspace you belong to. The path reads: from this
	// membership, to its workspace, to that workspace's memberships, is one of them
	// mine.
	seesRoster := `workspace.memberships_via_workspace.user ?= @request.auth.id`
	ms.ListRule = types.Pointer(seesRoster)
	ms.ViewRule = types.Pointer(seesRoster)
	// Left nil — superusers only. Nothing in phase 0 adds a member from a client:
	// the founding membership is minted server-side, and inviting somebody is the
	// planner's job. A rule nobody has exercised is not a rule worth shipping on an
	// authorization boundary.
	ms.CreateRule = nil
	ms.UpdateRule = nil
	ms.DeleteRule = nil

	if err := app.Save(ms); err != nil {
		return err
	}

	// Now that `memberships` exists, the back-relation resolves and the workspace
	// rules can be applied.
	ws.ListRule = types.Pointer(isMember)
	ws.ViewRule = types.Pointer(isMember)
	// Anyone signed in may found a workspace; the hook in internal/bubble makes
	// them its lead in the same transaction. Without that, a workspace would be
	// born with no members and be invisible to everyone including its author.
	ws.CreateRule = types.Pointer(`@request.auth.id != ""`)
	ws.UpdateRule = types.Pointer(isLead)
	ws.DeleteRule = types.Pointer(isLead)

	return app.Save(ws)
}

func down(app core.App) error {
	// Reverse order: memberships holds relations into workspaces and users.
	for _, name := range []string{"memberships", "workspaces"} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue // already gone
		}
		if err := app.Delete(c); err != nil {
			return err
		}
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.Fields.RemoveByName("display_name")
	return app.Save(users)
}
