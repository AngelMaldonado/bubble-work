package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// The people who were already here get a name too.
//
// `nameable` sets `emailVisibility` on every account created from now on; this
// is the same statement about the ones that exist. Without it the roster of a
// workspace founded before this change still reads "sin nombre" for anybody who
// never filled in a display name — which is most accounts, because the field is
// optional and the dashboard does not ask for it.
func init() {
	m.Register(func(app core.App) error {
		rows, err := app.FindAllRecords("users")
		if err != nil {
			return err
		}
		for _, r := range rows {
			if !r.GetBool("emailVisibility") {
				r.Set("emailVisibility", true)
				if err := app.Save(r); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(app core.App) error {
		// Down does nothing on purpose: hiding the addresses again would not
		// restore a state anybody was in, it would only break the roster in the
		// other direction.
		return nil
	})
}
