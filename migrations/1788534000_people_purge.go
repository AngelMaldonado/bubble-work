package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Purga de datos personales: a los 30 días de borrada, a una persona se le
// quitan sus datos personales (tokens, preferencias, presencia, Mi espacio,
// avatar, membresías y equipos) y queda su cuenta como firma de lo que hizo.
// `purged_at` dice cuándo: restaurarla después la devuelve sin nada de eso.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.DateField{Name: "purged_at"})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return nil
		}
		c.Fields.RemoveByName("purged_at")
		return app.Save(c)
	})
}
