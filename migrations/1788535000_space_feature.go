package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Mi espacio (los post-its personales) como una función del departamento
// (`features.space`), ENCENDIDA al nacer: ya existía y la usan. Apagarla la quita
// de la columna; las notas se quedan donde estaban.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("features")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.BoolField{Name: "space"})
		if err := app.Save(c); err != nil {
			return err
		}
		rows, err := app.FindAllRecords("features")
		if err != nil {
			return err
		}
		for _, r := range rows {
			r.Set("space", true)
			if err := app.Save(r); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("features")
		if err != nil {
			return nil
		}
		c.Fields.RemoveByName("space")
		return app.Save(c)
	})
}
