package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Una persona puede tener cara.
//
// Hasta ahora una persona era su inicial en un círculo de color. «Mi usuario»,
// en ajustes, deja poner una foto. Un archivo del propio registro y no un asset
// de git: no pertenece a ningún proyecto, y el repositorio de un proyecto no es
// sitio para la cara de alguien.
//
// Pequeña a propósito —2 MB, sólo imágenes de mapa de bits— con miniaturas que
// PocketBase genera al pedirlas. SVG no: podría ejecutar código en nuestro
// origen, la misma razón por la que `assets/` lo sirve con una política que no
// permite nada.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if users.Fields.GetByName("avatar") != nil {
			return nil
		}
		users.Fields.Add(&core.FileField{
			Name:      "avatar",
			MaxSelect: 1,
			MaxSize:   2 << 20,
			MimeTypes: []string{"image/png", "image/jpeg", "image/webp", "image/gif"},
			Thumbs:    []string{"64x64", "160x160"},
		})
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.RemoveByName("avatar")
		return app.Save(users)
	})
}
