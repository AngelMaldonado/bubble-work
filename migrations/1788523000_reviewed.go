package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Revisado: lo que ya se miró y se da por cerrado del todo.
//
// Hasta ahora había dos estados y un salto entre ellos: terminado —un hilo en un
// estado del grupo `completed`, un módulo cerrado— y borrado. Terminar es del
// que ejecuta y pasa muchas veces al día; borrar se lleva el trabajo por
// delante. Entre los dos faltaba el gesto de quien mira lo terminado y dice
// «sí, esto está bien»: la columna de lo hecho crecía sin que nadie pudiera
// marcar por dónde iba.
//
// Con esto, la columna derecha de la secuencia deja de ser el historial entero
// y pasa a ser **lo hecho que falta revisar**, que es una cola de trabajo de
// verdad: se vacía. Lo revisado sale de ahí y sigue donde ha estado siempre —en
// el kanban, en el board de su proyecto, en el documento—, así que no se
// esconde nada, sólo deja de pedir atención.
//
// Sólo una fecha, sin `reviewed_by`. Quién lo marcó ya está en `events`, que es
// donde vive quién hizo qué, y una segunda copia del autor es una segunda copia
// que puede discrepar. La pregunta que esta columna hace es «¿está revisado?»,
// no «¿quién lo revisó?».
//
// Sin guarda de rol: lo marca quien puede editar la fila. Revisar lo hecho es
// operar, no orquestar — y una cola que sólo una persona puede vaciar es una
// cola que no se vacía.
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"threads", "bubbles"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if c.Fields.GetByName("reviewed_at") == nil {
				c.Fields.Add(&core.DateField{Name: "reviewed_at"})
			}
			if err := app.Save(c); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"threads", "bubbles"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			c.Fields.RemoveByName("reviewed_at")
			if err := app.Save(c); err != nil {
				return err
			}
		}
		return nil
	})
}
