package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Un hilo dice de qué equipo es (E1, diseño del hilo #3 de Burbujas, §2.1).
//
// Hoy siempre tiene proyecto y el equipo es el de su proyecto: el servidor lo
// DERIVA al validar y lo mantiene al mover el proyecto de equipo; lo que mande
// el cliente se pisa. Es la base para que un hilo exista sin proyecto (una
// captura del inbox de su equipo): entonces `team` será lo único que diga a
// quién pertenece, y por eso no se deduce nunca de una ruta.
//
// Ni las reglas de la colección ni ningún permiso leen todavía este campo: el
// acceso sigue colgando del proyecto. Abrir el caso «sin proyecto» es otro
// paso, con sus pruebas de privacidad.
func init() {
	m.Register(func(app core.App) error {
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		teams, err := app.FindCollectionByNameOrId("teams")
		if err != nil {
			return err
		}
		threads.Fields.Add(&core.RelationField{Name: "team", CollectionId: teams.Id, MaxSelect: 1})
		threads.AddIndex("idx_threads_team", false, "team", "")
		if err := app.Save(threads); err != nil {
			return err
		}
		_, err = app.DB().NewQuery(`UPDATE threads
			SET team = COALESCE((SELECT team FROM workspaces WHERE workspaces.id = threads.workspace), '')
			WHERE workspace != ''`).Execute()
		return err
	}, func(app core.App) error {
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.RemoveIndex("idx_threads_team")
		threads.Fields.RemoveByName("team")
		return app.Save(threads)
	})
}
