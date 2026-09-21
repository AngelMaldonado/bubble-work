package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Una tarea vuelve a tener fecha propia.
//
// `1788508000_dates_move_up.go` quitó `threads.due_date` y subió los valores a
// las burbujas. El argumento era bueno y sigue siéndolo a medias: lo que el lead
// pone en el calendario es cuándo tiene que estar un CUERPO de trabajo, y eso es
// de la burbuja. Lo que no se sostuvo es la otra mitad — que repartir ese plazo
// entre las piezas no necesita un campo.
//
// Sí lo necesita, y por algo que sólo se ve al usarlo: sin fecha por pieza no se
// puede decir «esto vence el jueves» de nada más pequeño que un módulo. Con
// cinco tareas dentro de una burbuja que vence el 30, las cinco parecen vencer
// el 30, y la que en realidad bloquea a las otras el martes no se distingue. El
// plazo de la burbuja sigue siendo el contrato; la fecha de la tarea es cómo se
// reparte, y repartirlo en la cabeza de quien ejecuta es exactamente lo que se
// pierde cuando esa persona no está.
//
// Lo que se RECHAZA, para que no se vuelva a intentar: derivar la fecha de la
// burbuja de sus tareas. Eso es lo que hacía el traspaso de aquella migración y
// es lo que lo volvía confuso — dos sitios diciendo el mismo plazo, uno de ellos
// calculado, y ninguna forma de decir «la burbuja entera vence el 30 aunque
// ninguna pieza suelta lo haga». Las dos fechas son independientes a propósito:
// una es el compromiso, la otra el reparto.
//
// No se recuperan los valores viejos: `1788508000` los subió y su `down` ya
// decía que no los devuelve. Lo que hay en las burbujas se queda donde está, y
// las tareas nacen sin fecha, que es la verdad — nadie ha repartido nada
// todavía.
func init() {
	m.Register(func(app core.App) error {
		th, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		if th.Fields.GetByName("due_date") != nil {
			return nil
		}
		th.Fields.Add(&core.DateField{Name: "due_date"})
		return app.Save(th)
	}, func(app core.App) error {
		th, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return nil
		}
		th.Fields.RemoveByName("due_date")
		return app.Save(th)
	})
}
