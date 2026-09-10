package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// The plan is read by the person whose plan it is.
//
// 5a opened `objectives` and `inbox_items` to everybody signed in, with a real
// argument: an objective the projects cannot see is not something to measure
// them against. What that argument missed is that the objectives of a
// department are also its strategy — margins, a client that is slipping, debt
// somebody has decided to stop paying down — and the screen where they are
// written was made the lead's for the same reason. A screen that is one
// person's and an API that is everybody's is not a decision, it is a gap.
//
// So the read follows the screen: the global lead — the department head — and
// nobody else.
//
// `inbox_items` keeps ONE opening, and it is not a compromise: whoever captured
// a note may read it back. Capturing stays open to everybody, and a note you
// write and can never see again is a note that stops being written — which
// would empty the inbox rather than protect it.
func init() {
	m.Register(func(app core.App) error {
		obj, err := app.FindCollectionByNameOrId("objectives")
		if err != nil {
			return err
		}
		obj.ListRule = types.Pointer(globalLead)
		obj.ViewRule = types.Pointer(globalLead)
		if err := app.Save(obj); err != nil {
			return err
		}

		inbox, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}
		mineOrLead := globalLead + ` || captured_by = @request.auth.id`
		inbox.ListRule = types.Pointer(mineOrLead)
		inbox.ViewRule = types.Pointer(mineOrLead)
		return app.Save(inbox)
	}, func(app core.App) error {
		obj, err := app.FindCollectionByNameOrId("objectives")
		if err != nil {
			return err
		}
		obj.ListRule = types.Pointer(signedIn)
		obj.ViewRule = types.Pointer(signedIn)
		if err := app.Save(obj); err != nil {
			return err
		}
		inbox, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}
		inbox.ListRule = types.Pointer(signedIn)
		inbox.ViewRule = types.Pointer(signedIn)
		return app.Save(inbox)
	})
}
