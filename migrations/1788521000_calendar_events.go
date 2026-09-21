package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Lo que pasa en una fecha y NO es trabajo: la junta de los lunes, el cierre de
// mes, el día que viene el auditor.
//
// Hasta ahora el calendario sólo sabía de burbujas y de tareas con fecha, así
// que una junta semanal no tenía dónde ponerse. La salida tentadora era crear un
// hilo y darle fecha, y es justo la que hay que cerrar: un hilo se completa,
// tiene evidencia y CALIENTA. Una junta no se completa, no produce nada que
// enlazar, y un hilo que renace cada lunes emitiría `thread-created` todas las
// semanas — una burbuja 🔥 para siempre sin que nadie trabaje, que es lo
// contrario de lo que el calor dice.
//
// Por eso esto es una colección aparte y no un tipo de hilo. **No emite ningún
// evento y no calienta nada**: no está en ninguno de los hooks de
// `internal/bubble/events.go`, y eso es una decisión, no un olvido.
//
// Del DEPARTAMENTO, como los objetivos y el inbox: la gente de aquí trabaja
// cruzando proyectos, y «cuándo es la junta» no es un hecho sobre un workspace.
// Lo escribe el lead global; lo ve todo el mundo.
//
// La repetición es un MENÚ CERRADO —diaria, semanal, quincenal, mensual— y no
// una RRULE de iCalendar. Se rechaza el estándar a propósito: trae una librería
// más y mucha superficie que probar (excepciones, fin de serie, «el tercer
// jueves») por casos que todavía no existen aquí. Cuando haga falta «el tercer
// jueves de cada mes», se vuelve a esta decisión con el caso delante.
//
// Y sin fin de serie: una repetición dura hasta que alguien la borra. Un `until`
// que nadie pone es una columna vacía, y uno que alguien pone mal es una junta
// que desaparece del calendario sin que nadie sepa por qué.
func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("calendar_events")
		c.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 120})
		// Cuándo es la PRIMERA. Las siguientes se calculan; guardar una fila por
		// ocurrencia sería una tabla que crece sola y que hay que podar.
		c.Fields.Add(&core.DateField{Name: "start", Required: true})
		c.Fields.Add(&core.SelectField{
			Name:      "repeat",
			Values:    []string{"daily", "weekly", "biweekly", "monthly"},
			MaxSelect: 1,
		})
		// Qué es, para quien lo lea sin haber estado cuando se puso.
		c.Fields.Add(&core.TextField{Name: "notes", Max: 500})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		c.AddIndex("idx_calendar_events_start", false, "start", "")

		signedIn := `@request.auth.id != ""`
		c.ListRule = types.Pointer(signedIn)
		c.ViewRule = types.Pointer(signedIn)
		c.CreateRule = types.Pointer(globalLead)
		c.UpdateRule = types.Pointer(globalLead)
		c.DeleteRule = types.Pointer(globalLead)
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("calendar_events")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
