package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Presence: one row per person, holding the last time they said they were here.
//
// Department-wide, like the objectives and the inbox and for the same reason:
// the people here work across projects, and "who is around" is not a fact about
// a workspace. Anybody signed in reads it — that is the whole point of a row of
// avatars — and nobody writes it from a client at all: the heartbeat goes
// through `POST /api/presence`, so the timestamp is the server's.
//
// The row is kept rather than deleted when somebody leaves. It is one row per
// person, it is overwritten on every beat, and a collection that empties itself
// is a collection that races with the reader deciding who is online.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		p := core.NewBaseCollection("presence")
		p.Fields.Add(&core.RelationField{
			Name: "user", CollectionId: users.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true,
		})
		p.Fields.Add(&core.DateField{Name: "at"})
		p.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		p.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		// One row per person. Two would be two answers to "is she here".
		p.AddIndex("idx_presence_user", true, "user", "")

		signedIn := `@request.auth.id != ""`
		p.ListRule = types.Pointer(signedIn)
		p.ViewRule = types.Pointer(signedIn)
		// No client writes this. Create, update and delete are nil — superuser
		// only — because the one legitimate write is the beat, and the beat is a
		// route that stamps the server's clock.
		return app.Save(p)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("presence")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
