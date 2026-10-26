package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Roadmap: el calendario como un camino. Una función que el departamento
// enciende (`features.roadmap`), apagada al nacer: es una vista nueva, y no
// cambia ningún dato — apagarla sólo la quita de la columna.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("features")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.BoolField{Name: "roadmap"})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("features")
		if err != nil {
			return nil
		}
		c.Fields.RemoveByName("roadmap")
		return app.Save(c)
	})
}
