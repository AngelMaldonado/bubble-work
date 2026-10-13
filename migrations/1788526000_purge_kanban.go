package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El kanban se quita entero.
//
// Probado en Cuby con un equipo de cuatro: las burbujas y los hilos salían más
// rápido de lo que un lead los colocaba en columnas, y el plan se pasó a una
// junta semanal. Lo que queda del planeador es la secuencia (si está
// encendida), el calendario y lo capturado; la vista larga será la de los
// objetivos (grill 2026-10-08, Q1).
//
// Se van las dos tablas de columnas (`stages` de módulos, `thread_stages` de
// tareas), el campo que colocaba cada fila en una (`stage`) y el orden a mano
// dentro de una columna (`bubbles.rank`). Los campos primero: una relación
// apuntando a una colección que ya no existe no se puede guardar.
//
// `down` devuelve el esquema, no lo que decía: en qué columna estaba cada cosa
// no vuelve.
func init() {
	m.Register(func(app core.App) error {
		for coll, fields := range map[string][]string{
			"bubbles": {"stage", "rank"},
			"threads": {"stage"},
		} {
			c, err := app.FindCollectionByNameOrId(coll)
			if err != nil {
				return err
			}
			for _, f := range fields {
				c.Fields.RemoveByName(f)
			}
			if err := app.Save(c); err != nil {
				return err
			}
		}
		for _, name := range []string{"thread_stages", "stages"} {
			if c, err := app.FindCollectionByNameOrId(name); err == nil {
				if err := app.Delete(c); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"stages", "thread_stages"} {
			if _, err := app.FindCollectionByNameOrId(name); err == nil {
				continue
			}
			c := core.NewBaseCollection(name)
			c.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 60})
			c.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
			c.Fields.Add(&core.BoolField{Name: "done"})
			if err := app.Save(c); err != nil {
				return err
			}
		}
		st, _ := app.FindCollectionByNameOrId("stages")
		ts, _ := app.FindCollectionByNameOrId("thread_stages")
		if b, err := app.FindCollectionByNameOrId("bubbles"); err == nil {
			b.Fields.Add(&core.RelationField{Name: "stage", CollectionId: st.Id, MaxSelect: 1})
			b.Fields.Add(&core.NumberField{Name: "rank", OnlyInt: true})
			if err := app.Save(b); err != nil {
				return err
			}
		}
		if t, err := app.FindCollectionByNameOrId("threads"); err == nil {
			t.Fields.Add(&core.RelationField{Name: "stage", CollectionId: ts.Id, MaxSelect: 1})
			return app.Save(t)
		}
		return nil
	})
}
