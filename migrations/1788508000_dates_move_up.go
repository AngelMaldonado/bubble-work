package migrations

import (
	"log"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// La fecha sube de thread a burbuja, como subieron el objetivo y la prioridad.
//
// Al mover el plan a las burbujas las fechas se quedaron abajo, con un argumento:
// vencer es de la ejecución. No se sostuvo. Lo que el lead pone en el calendario
// es cuándo tiene que estar un CUERPO de trabajo —«la facturación, para el 30»—,
// que es lo mismo que decide al ponerlo en corto o en largo plazo. Con la fecha
// en cada thread, la tarjeta del plan no tenía fecha, el calendario enseñaba
// piezas en vez de lo planeado, y cada operador que abría un thread nuevo tenía
// que inventarse de nuevo el plazo que ya estaba decidido. Cómo reparte ese
// plazo entre sus piezas es cosa de quien ejecuta, y no necesita un campo.
//
// La regla del traspaso elige no inventar, como la del objetivo:
//
//   - La fecha MÁS TARDÍA entre los threads ABIERTOS de la burbuja: el cuerpo de
//     trabajo no está hecho hasta que lo está su última pieza. Una fecha de un
//     thread cerrado es historia, no plazo — sólo cuenta si no queda ninguna
//     abierta.
//   - Si las fechas de una burbuja no coincidían, se anota en el log.
//
// Después se quita el campo de los threads. No se deshace: `down` devuelve el
// campo vacío y lo dice.
func init() {
	m.Register(func(app core.App) error {
		bubbles, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return err
		}
		if bubbles.Fields.GetByName("due_date") == nil {
			bubbles.Fields.Add(&core.DateField{Name: "due_date"})
			if err := app.Save(bubbles); err != nil {
				return err
			}
		}

		rows, err := app.FindAllRecords("bubbles")
		if err != nil {
			return err
		}
		var lifted, split int
		for _, b := range rows {
			threads, err := app.FindAllRecords("threads", dbx.HashExp{"bubble": b.Id})
			if err != nil {
				return err
			}
			var open, any types.DateTime
			days := map[string]bool{}
			for _, t := range threads {
				due := t.GetDateTime("due_date")
				if due.IsZero() {
					continue
				}
				days[due.Time().Format("2006-01-02")] = true
				if due.Time().After(any.Time()) {
					any = due
				}
				if t.GetDateTime("completed_at").IsZero() && due.Time().After(open.Time()) {
					open = due
				}
			}
			pick := open
			if pick.IsZero() {
				pick = any
			}
			if pick.IsZero() {
				continue
			}
			if len(days) > 1 {
				split++
				log.Printf("migración: la burbuja %q tenía %d fechas distintas en sus threads; se queda con %s",
					b.GetString("name"), len(days), pick.Time().Format("2006-01-02"))
			}
			b.Set("due_date", pick)
			if err := app.Save(b); err != nil {
				return err
			}
			lifted++
		}
		log.Printf("migración: %d burbujas heredaron fecha de sus threads (%d con fechas distintas)", lifted, split)

		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.Fields.RemoveByName("due_date")
		return app.Save(threads)
	}, func(app core.App) error {
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		if threads.Fields.GetByName("due_date") == nil {
			threads.Fields.Add(&core.DateField{Name: "due_date"})
			if err := app.Save(threads); err != nil {
				return err
			}
		}
		bubbles, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return err
		}
		bubbles.Fields.RemoveByName("due_date")
		if err := app.Save(bubbles); err != nil {
			return err
		}
		log.Print("migración: los threads vuelven a tener campo de fecha, vacío. " +
			"La fecha que tenía cada uno NO se restaura: al subirla se perdió de cuál venía.")
		return nil
	})
}
