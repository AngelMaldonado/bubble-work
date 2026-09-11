package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El orden de una columna del planeador, cuando alguien lo decide a mano.
//
// Por defecto una columna se ordena sola: por la prioridad que el servidor
// deriva de impacto × urgencia. Eso es lo que dice el modelo —el orden se
// deriva, no se administra— y sigue siendo el valor por defecto.
//
// Pero derivado no siempre es lo que una persona sabe. «Esto va primero porque
// el cliente llama el martes» no cabe en impacto × urgencia, y obligar a
// falsear la urgencia para colocar una tarjeta es peor que dejar mover la
// tarjeta: corrompe el dato con el que se calcula todo lo demás.
//
// Así que `rank` es una excepción DECLARADA, no un segundo sistema: cero
// significa «ninguno, ordéname tú», y una columna donde alguien arrastró pasa a
// llevar rangos en todas sus tarjetas. Se ve cuál es cuál, y se puede volver.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.NumberField{Name: "rank", OnlyInt: true})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return nil
		}
		c.Fields.RemoveByName("rank")
		return app.Save(c)
	})
}
