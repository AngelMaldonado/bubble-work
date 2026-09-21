package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Las columnas del tablero de TAREAS, y en qué columna está cada una.
//
// El planeador se puede ver de dos maneras: por módulos —las burbujas, en
// `stages`, que es lo que había— o por tareas. Un tablero de tareas necesita sus
// propias columnas, y ninguna de las que ya existen sirve:
//
//   - `stages` son del departamento y ya significan otra cosa: dónde está un
//     MÓDULO en el plan. Una burbuja en «Corto plazo» con sus hilos repartidos
//     por «Largo plazo» es un estado que nadie sabe leer.
//   - `states` existen y son por workspace, pero el planeador es del
//     departamento entero: con columnas por proyecto, un tablero que cruza
//     proyectos no tendría ninguna columna en común.
//
// Así que una colección nueva, calcada de `stages` en forma y en permisos: del
// departamento, la lee cualquiera que trabaje aquí, la escribe el lead global.
//
// NACE VACÍA, a propósito. Qué columnas tiene un tablero de tareas lo decide
// quien orquesta, no una migración — «Por hacer / En curso / Hecha» es una
// opinión, y sembrarla es imponerla. Mientras no haya ninguna, todas las tareas
// caen en la columna sintética «Sin planear», que el tablero de burbujas ya
// dibuja con el id vacío y que no vive en ninguna tabla.
//
// Lo que NO se añade: un `rank` para ordenar a mano dentro de la columna. Se
// quitó de los hilos a propósito (`1788504000_rank_moves_up.go`) y no hace falta
// aquí: una columna de tareas se ordena por la prioridad que el lead ya les pone
// y por la secuencia de ejecución, las dos cosas que ya dicen qué va primero.
// Si algún día el orden derivado no basta, eso será su propia decisión.
func init() {
	m.Register(func(app core.App) error {
		ts := core.NewBaseCollection("thread_stages")
		ts.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 60})
		ts.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
		// Como en `stages`: marca dónde una tarea se da por terminada, para que
		// el tablero sepa cuál es el final sin depender de cómo se llame.
		ts.Fields.Add(&core.BoolField{Name: "done"})
		ts.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		ts.AddIndex("idx_thread_stages_position", false, "position", "")

		signedIn := `@request.auth.id != ""`
		ts.ListRule = types.Pointer(signedIn)
		ts.ViewRule = types.Pointer(signedIn)
		ts.CreateRule = types.Pointer(globalLead)
		ts.UpdateRule = types.Pointer(globalLead)
		ts.DeleteRule = types.Pointer(globalLead)
		if err := app.Save(ts); err != nil {
			return err
		}

		th, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		// Sin `CascadeDelete`: borrar una columna NO puede llevarse las tareas que
		// había dentro. Vuelven a «Sin planear», que es lo que pasa con las
		// burbujas cuando se borra su etapa.
		th.Fields.Add(&core.RelationField{
			Name: "stage", CollectionId: ts.Id, MaxSelect: 1,
		})
		// Mover una tarea de columna es operar, no orquestar: lo hace quien
		// trabaja en el proyecto, con la regla que el hilo ya tiene. La
		// prioridad y la secuencia siguen siendo del lead global, y eso lo
		// guarda `1788513000_thread_priority_sequence.go` sin tocar esto.
		return app.Save(th)
	}, func(app core.App) error {
		if th, err := app.FindCollectionByNameOrId("threads"); err == nil {
			th.Fields.RemoveByName("stage")
			if err := app.Save(th); err != nil {
				return err
			}
		}
		c, err := app.FindCollectionByNameOrId("thread_stages")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
