package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Un hilo vuelve a tener prioridad, y entra en la secuencia del departamento.
//
// PRIORIDAD. Había subido a la burbuja (`lift_to_bubbles`) con el argumento de
// que la decide quien orquesta. Vuelve al hilo, SEPARADA de la de su burbuja —ni
// la hereda ni la pisa—: la burbuja dice cuánto importa un cuerpo de trabajo, el
// hilo cuánto importa esa pieza. Y en el hilo se GUARDA (P1…P4), no se deriva
// del mapa: el mapa sirve para decidir un cuerpo de trabajo, y para una pieza
// basta con elegir la letra.
//
// SECUENCIA. El orden en que se ejecuta no es de un proyecto ni de un hilo: es
// una decisión del lead para todo el departamento. `sequence` es el PASO del hilo
// en esa línea (10, 20, 30… con huecos para insertar sin renumerar); dos hilos
// con el mismo número van en paralelo, y vacío es «no está en la secuencia».
// «Ahora» no se guarda: es el primer paso con algo abierto, así que terminar un
// hilo recorre la línea sin que nadie mueva un puntero.
//
// Las dos las escribe sólo el lead global: la regla de actualización de un hilo
// deja a cualquier miembro tocar su nombre o su estado, y estas dos quedan fuera.
func init() {
	m.Register(func(app core.App) error {
		th, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		if th.Fields.GetByName("priority") == nil {
			th.Fields.Add(&core.SelectField{Name: "priority", MaxSelect: 1, Values: []string{"P1", "P2", "P3", "P4"}})
		}
		if th.Fields.GetByName("sequence") == nil {
			th.Fields.Add(&core.NumberField{Name: "sequence", OnlyInt: true})
		}
		guard := ` && (@request.body.priority:isset = false || @request.auth.role = 'lead')` +
			` && (@request.body.sequence:isset = false || @request.auth.role = 'lead')`
		if th.CreateRule != nil {
			th.CreateRule = types.Pointer(`(` + *th.CreateRule + `)` + guard)
		}
		if th.UpdateRule != nil {
			th.UpdateRule = types.Pointer(`(` + *th.UpdateRule + `)` + guard)
		}
		return app.Save(th)
	}, func(app core.App) error {
		th, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		th.Fields.RemoveByName("priority")
		th.Fields.RemoveByName("sequence")
		member := `@request.auth.role = 'lead' || (workspace.memberships_via_workspace.user ?= @request.auth.id)`
		th.CreateRule = types.Pointer(member)
		th.UpdateRule = types.Pointer(member)
		return app.Save(th)
	})
}
