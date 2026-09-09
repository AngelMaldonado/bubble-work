package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Accountability is a LIST, and a thread row can say how old it is.
//
// A bubble had one `owner`, and one owner is how real work gets described wrong:
// the bubbles people actually run are shared — two people carry a client, a lead
// and whoever is doing the work carry a migration — and naming one of them makes
// the other invisible on the board. It matters more here than in most products
// because ownership is not decoration: `RollUp` buries a quiet bubble with
// NOBODY accountable in the 🪦 band, on the grounds that there is no one to ask.
// That test is "is anybody accountable", and a list answers it exactly as well
// as a single relation does — an empty list is the same silence.
//
// So `owner` becomes `owners`, plural in the schema because it is plural in the
// model. The field is added, the old value copied into it, and the old field
// removed, in that order: dropping first and adding second is how a rename loses
// every value it was supposed to keep.
//
// The evidence view gains `assignees` in the same pass, for the other half of
// the same question. A thread row in the bubble's drawer says which band it is
// in and how long it has been there; who has it was missing because the view
// never selected it, and the drawer had nothing to draw.
func init() {
	m.Register(upBubbleOwners, downBubbleOwners)
}

func upBubbleOwners(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	bubbles, err := app.FindCollectionByNameOrId("bubbles")
	if err != nil {
		return err
	}
	bubbles.Fields.Add(&core.RelationField{
		Name: "owners", CollectionId: users.Id, MaxSelect: 20,
	})
	if err := app.Save(bubbles); err != nil {
		return err
	}

	// Carry what was there. A bubble with no owner gets an empty list, which is
	// the same statement it was already making.
	rows, err := app.FindAllRecords("bubbles")
	if err != nil {
		return err
	}
	for _, r := range rows {
		if one := r.GetString("owner"); one != "" {
			r.Set("owners", []string{one})
			if err := app.Save(r); err != nil {
				return err
			}
		}
	}

	bubbles.Fields.RemoveByName("owner")
	if err := app.Save(bubbles); err != nil {
		return err
	}

	return evidenceViewWithAssignees(app)
}

// evidenceViewWithAssignees re-declares the view with one more column. The whole
// SELECT is written out rather than patched, because the view IS the string:
// PocketBase parses this list to derive the fields, and a half-edited query is a
// collection with fields nobody meant.
func evidenceViewWithAssignees(app core.App) error {
	ev, err := app.FindCollectionByNameOrId("thread_evidence")
	if err != nil {
		return err
	}
	ev.ViewQuery = evidenceSelect(true)
	return app.Save(ev)
}

func downBubbleOwners(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	bubbles, err := app.FindCollectionByNameOrId("bubbles")
	if err != nil {
		return err
	}
	bubbles.Fields.Add(&core.RelationField{
		Name: "owner", CollectionId: users.Id, MaxSelect: 1,
	})
	if err := app.Save(bubbles); err != nil {
		return err
	}
	rows, err := app.FindAllRecords("bubbles")
	if err != nil {
		return err
	}
	for _, r := range rows {
		// The first one, because a single field cannot hold the rest. Going back
		// is lossy and says so here rather than pretending otherwise.
		if many := r.GetStringSlice("owners"); len(many) > 0 {
			r.Set("owner", many[0])
			if err := app.Save(r); err != nil {
				return err
			}
		}
	}
	bubbles.Fields.RemoveByName("owners")
	if err := app.Save(bubbles); err != nil {
		return err
	}

	ev, err := app.FindCollectionByNameOrId("thread_evidence")
	if err != nil {
		return err
	}
	ev.ViewQuery = evidenceSelect(false)
	return app.Save(ev)
}
