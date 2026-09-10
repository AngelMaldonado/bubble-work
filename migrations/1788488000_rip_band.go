package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Warm and Cooling are gone, and the grave is a band.
//
// The bands were Hot → Warm → Cooling → Dormant → Closed. Warm and Cooling were
// a gradient nobody acted on: "produced last cycle" and "produced neither
// cycle" lead to the same morning. What is left is Hot → Dormant → Rip →
// Closed, which is what v0 shipped and what people actually read.
//
// Nothing stores a lifecycle — heat is a pure function of evidence, so there is
// no column of stale bands to migrate. The only thing on disk that mentioned a
// band was the tuning flag, and it now names the band it produces.
func init() {
	m.Register(func(app core.App) error {
		tun, err := app.FindCollectionByNameOrId("tuning")
		if err != nil {
			return err
		}
		f := tun.Fields.GetByName("ownerless_is_dormant")
		if f == nil {
			return nil
		}
		// Renaming in place keeps the value: a workspace that had the rule
		// switched off keeps it switched off.
		f.SetName("ownerless_is_rip")
		return app.Save(tun)
	}, func(app core.App) error {
		tun, err := app.FindCollectionByNameOrId("tuning")
		if err != nil {
			return err
		}
		f := tun.Fields.GetByName("ownerless_is_rip")
		if f == nil {
			return nil
		}
		f.SetName("ownerless_is_dormant")
		return app.Save(tun)
	})
}
