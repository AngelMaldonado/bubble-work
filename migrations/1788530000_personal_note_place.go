package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Where a sticky note sits on the owner's infinite canvas. Server-side, not
// localStorage, so arranging the board follows the person between devices.
// Privacy is unchanged: the same owner-only rules guard these columns.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("personal_notes")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.NumberField{Name: "x", OnlyInt: true, Min: types.Pointer(-1_000_000.0), Max: types.Pointer(1_000_000.0)})
		c.Fields.Add(&core.NumberField{Name: "y", OnlyInt: true, Min: types.Pointer(-1_000_000.0), Max: types.Pointer(1_000_000.0)})
		c.Fields.Add(&core.NumberField{Name: "w", OnlyInt: true, Min: types.Pointer(0.0), Max: types.Pointer(640.0)})
		if err := app.Save(c); err != nil {
			return err
		}
		// Notes written before the canvas existed get a readable grid instead
		// of a pile at the origin.
		rows, err := app.FindAllRecords("personal_notes")
		if err != nil {
			return err
		}
		for i, r := range rows {
			r.Set("x", (i%4)*280)
			r.Set("y", (i/4)*300)
			r.Set("w", 240)
			if err := app.Save(r); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("personal_notes")
		if err != nil {
			return err
		}
		for _, n := range []string{"x", "y", "w"} {
			c.Fields.RemoveByName(n)
		}
		return app.Save(c)
	})
}
