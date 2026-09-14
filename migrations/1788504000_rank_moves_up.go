package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El orden a mano también sube: lo que se coloca en el tablero son burbujas.
//
// `threads.rank` se añadió cuando las columnas del planeador eran objetivos y
// las tarjetas eran threads. Ahora las tarjetas son burbujas, así que el rango
// que un lead pone arrastrando pertenece a la burbuja y el de los threads no lo
// lee nadie.
//
// Una migración nueva y no un retoque de la anterior: una migración aplicada es
// historia, no un borrador — PocketBase no la vuelve a correr, así que editarla
// deja a cada base en un estado distinto según cuándo se actualizó.
func init() {
	m.Register(func(app core.App) error {
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return err
		}
		b.Fields.Add(&core.NumberField{Name: "rank", OnlyInt: true})
		if err := app.Save(b); err != nil {
			return err
		}
		t, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		t.Fields.RemoveByName("rank")
		return app.Save(t)
	}, func(app core.App) error {
		if t, err := app.FindCollectionByNameOrId("threads"); err == nil {
			t.Fields.Add(&core.NumberField{Name: "rank", OnlyInt: true})
			if err := app.Save(t); err != nil {
				return err
			}
		}
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return nil
		}
		b.Fields.RemoveByName("rank")
		return app.Save(b)
	})
}
