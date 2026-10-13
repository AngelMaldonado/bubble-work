package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// A sticky note is resized in both directions. `h` is the height the owner
// chose (0 = as tall as its text), and the width cap rises from 640 to 1200 so
// a note can hold a real list. Same owner-only rules as every other column.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("personal_notes")
		if err != nil {
			return err
		}
		if w, ok := c.Fields.GetByName("w").(*core.NumberField); ok {
			w.Max = types.Pointer(1200.0)
		}
		c.Fields.Add(&core.NumberField{Name: "h", OnlyInt: true, Min: types.Pointer(0.0), Max: types.Pointer(1600.0)})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("personal_notes")
		if err != nil {
			return err
		}
		if _, err := app.DB().NewQuery("UPDATE personal_notes SET w = 640 WHERE w > 640").Execute(); err != nil {
			return err
		}
		c.Fields.RemoveByName("h")
		if w, ok := c.Fields.GetByName("w").(*core.NumberField); ok {
			w.Max = types.Pointer(640.0)
		}
		return app.Save(c)
	})
}
