package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Las etapas del departamento: en qué punto está una burbuja.
//
// No son los `states` de un workspace, y la diferencia cabe en una frase: los
// ESTADOS son de quien ejecuta —viven en un proyecto, los pone quien trabaja
// ahí— y las ETAPAS son de quien orquesta. Un lead que mira seis proyectos
// necesita un vocabulario común, y el de cada proyecto no lo es.
//
// Se siembran por horizonte porque así se planea a este nivel: a corto, a
// mediano y a largo plazo. Son un punto de partida, no un dogma — el lead las
// renombra y las reordena.
//
// Una burbuja pasa a tener DOS ciclos de vida, y son preguntas distintas:
//
//	banda (🔥😴🪦🏆)  ¿está cambiando la realidad?   la deriva el servidor
//	etapa             ¿qué decidimos que es esto?     la pone el lead
//
// Verlas juntas es el valor: «En curso» y 😴 dos ciclos seguidos es exactamente
// la fila que un lead necesita mirar — dijimos que se está haciendo y no está
// produciendo.
func init() {
	m.Register(func(app core.App) error {
		st := core.NewBaseCollection("stages")
		st.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 60})
		st.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
		// `done` marca la columna donde una burbuja se da por terminada. Es lo que
		// permite que el tablero sepa qué es el final sin depender de cómo se
		// llame la columna — «Hecho», «Entregado» o «Cerrado» son la misma cosa.
		st.Fields.Add(&core.BoolField{Name: "done"})
		st.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		st.AddIndex("idx_stages_position", false, "position", "")

		// Las LEE cualquiera que trabaje aquí: la etapa se dibuja en la tarjeta de
		// una burbuja, y una etiqueta que un miembro no puede leer se ve como un
		// hueco. Las ESCRIBE el lead global, como los objetivos: diseñar el
		// tablero es orquestar.
		signedIn := `@request.auth.id != ""`
		st.ListRule = types.Pointer(signedIn)
		st.ViewRule = types.Pointer(signedIn)
		st.CreateRule = types.Pointer(globalLead)
		st.UpdateRule = types.Pointer(globalLead)
		st.DeleteRule = types.Pointer(globalLead)
		if err := app.Save(st); err != nil {
			return err
		}

		for i, s := range []struct {
			name string
			done bool
		}{
			{"Corto plazo", false},
			{"Mediano plazo", false},
			{"Largo plazo", false},
			{"Hecho", true},
		} {
			r := core.NewRecord(st)
			r.Set("name", s.name)
			r.Set("position", i+1)
			r.Set("done", s.done)
			if err := app.Save(r); err != nil {
				return err
			}
		}

		// Y la burbuja apunta a una.
		//
		// Reemplaza a un `stage` que estaba en el esquema desde el principio como
		// Select con un solo valor —«reviewed»— y sin una línea de código que lo
		// leyera. Un campo que nadie usa no es una base sobre la que construir: es
		// un nombre ocupado.
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return err
		}
		b.Fields.RemoveByName("stage")
		b.Fields.Add(&core.RelationField{
			Name: "stage", CollectionId: st.Id, MaxSelect: 1,
		})
		return app.Save(b)
	}, func(app core.App) error {
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err == nil {
			b.Fields.RemoveByName("stage")
			if err := app.Save(b); err != nil {
				return err
			}
		}
		c, err := app.FindCollectionByNameOrId("stages")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
