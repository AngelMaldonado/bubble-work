package migrations

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// The strategic layer: one person who sees every workspace and administers them,
// alongside the per-workspace membership the operative layer runs on.
//
// Two different things now share the word "lead", so keep them apart when reading:
//
//	users.role = 'lead'         GLOBAL — the department head. Sees everything.
//	memberships.role = 'lead'   PER WORKSPACE — whoever founded it. Invites people.
//
// This also opens the invite path: `memberships` was superuser-only, which made
// every workspace effectively single-player.
const globalLead = `@request.auth.role = 'lead'`

// orLead lets the global lead through in addition to whoever the rule already let
// through.
//
// The original rule is PARENTHESISED rather than concatenated, because a rule like
// `wsLead` is two conditions joined by `&&` and gluing `A || B && C` makes the
// meaning depend on which operator binds tighter. Explicit beats remembering.
func orLead(rule string) string {
	return globalLead + " || (" + rule + ")"
}

func init() {
	m.Register(upGlobalLead, downGlobalLead)
}

// ruledCollections are every collection whose existing rules gain the global
// bypass. `memberships` is absent because its rules are written fresh below, and
// `users` because its rules are about a person rather than a workspace.
var ruledCollections = []string{
	"workspaces", "states", "labels", "bubbles", "threads",
	"thread_links", "thread_relations", "comments",
}

func upGlobalLead(app core.App) error {
	// ---- the global role ----
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.Fields.Add(&core.SelectField{
		Name:      "role",
		Values:    []string{"lead", "member"},
		MaxSelect: 1,
		Required:  true,
	})

	// Anyone signed in may LIST people. Assigning a thread, reading who wrote a
	// comment and — the reason this changed — inviting somebody all need a roster,
	// and PocketBase's default here is owner-only, which made the invite path
	// unusable: a workspace lead could not see the person they wanted to add.
	//
	// What it costs is the whole department's names and emails being visible to the
	// whole department. At this size that is what a staff list is anyway.
	signedIn := `@request.auth.id != ""`
	users.ListRule = types.Pointer(signedIn)
	users.ViewRule = types.Pointer(signedIn)

	// You may edit yourself, but you may NOT hand yourself the global role: without
	// `role:isset = false` anybody promotes themselves by editing their own
	// profile. Only a superuser or an existing global lead sets it.
	users.UpdateRule = types.Pointer(
		orLead(`id = @request.auth.id && @request.body.role:isset = false`))

	if err := app.Save(users); err != nil {
		return err
	}
	// Existing people become plain members. Nobody gains anything from this
	// migration running.
	if _, err := app.DB().NewQuery(
		`UPDATE users SET role = 'member' WHERE role = '' OR role IS NULL`,
	).Execute(); err != nil {
		return err
	}

	// ---- the bypass, applied to what is already there ----
	//
	// Wrapping the CURRENT rule rather than restating it: the rules stay defined in
	// the migration that introduced them, and this one says only what it adds.
	for _, name := range ruledCollections {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		for _, r := range []**string{&c.ListRule, &c.ViewRule, &c.CreateRule, &c.UpdateRule, &c.DeleteRule} {
			if *r == nil || strings.HasPrefix(**r, globalLead) {
				continue // nil means superusers only, and that stays superusers only
			}
			*r = types.Pointer(orLead(**r))
		}
		if err := app.Save(c); err != nil {
			return err
		}
	}

	// ---- the invite path ----
	ms, err := app.FindCollectionByNameOrId("memberships")
	if err != nil {
		return err
	}
	// A lead of THIS workspace, or the global lead. Same shape as `wsLead` in the
	// structure migration, reached from a membership row through its own workspace.
	msLead := orLead(`workspace.memberships_via_workspace.user ?= @request.auth.id` +
		` && workspace.memberships_via_workspace.role ?= 'lead'`)
	ms.CreateRule = types.Pointer(msLead)
	ms.UpdateRule = types.Pointer(msLead)
	ms.DeleteRule = types.Pointer(msLead)
	if ms.ListRule != nil && !strings.HasPrefix(*ms.ListRule, globalLead) {
		ms.ListRule = types.Pointer(orLead(*ms.ListRule))
		ms.ViewRule = types.Pointer(orLead(*ms.ViewRule))
	}
	return app.Save(ms)
}

func downGlobalLead(app core.App) error {
	prefix := globalLead + " || ("
	unwrap := func(r **string) {
		if *r == nil || !strings.HasPrefix(**r, prefix) {
			return
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(**r, prefix), ")")
		*r = types.Pointer(inner)
	}

	for _, name := range ruledCollections {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		for _, r := range []**string{&c.ListRule, &c.ViewRule, &c.CreateRule, &c.UpdateRule, &c.DeleteRule} {
			unwrap(r)
		}
		if err := app.Save(c); err != nil {
			return err
		}
	}

	ms, err := app.FindCollectionByNameOrId("memberships")
	if err == nil {
		unwrap(&ms.ListRule)
		unwrap(&ms.ViewRule)
		ms.CreateRule, ms.UpdateRule, ms.DeleteRule = nil, nil, nil
		if err := app.Save(ms); err != nil {
			return err
		}
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.Fields.RemoveByName("role")
	owner := fmt.Sprintf("id = %s", "@request.auth.id")
	users.ListRule = types.Pointer(owner)
	users.ViewRule = types.Pointer(owner)
	users.UpdateRule = types.Pointer(owner)
	return app.Save(users)
}
