package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("artifacts")
		if err != nil {
			return err
		}
		c.Fields.GetByName("kind").(*core.SelectField).Values = []string{"grill", "markdown"}
		return app.Save(c)
	}, func(app core.App) error {
		// A downgrade must not leave documents with a kind the old binary cannot read.
		rows, err := app.FindAllRecords("artifacts")
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.GetString("kind") == "markdown" {
				if err := app.Delete(r); err != nil {
					return err
				}
			}
		}
		c, err := app.FindCollectionByNameOrId("artifacts")
		if err != nil {
			return err
		}
		c.Fields.GetByName("kind").(*core.SelectField).Values = []string{"grill"}
		return app.Save(c)
	})
}
