package migrations

import (
	"log"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Lo que los threads sabían, subido a su burbuja.
//
// Es el paso de no retorno de este cambio: después de esto, el objetivo y la
// prioridad viven arriba, y `down` no puede devolverlos a cada thread sin
// inventarse cuál era de cuál. Por eso va en su propia migración y por eso su
// vuelta atrás lo dice en voz alta en lugar de fingir.
//
// Dos reglas, y las dos eligen no inventar:
//
//   - OBJETIVO: el más repetido entre los threads de la burbuja. Si dos threads
//     de la misma burbuja apuntaban a objetivos distintos, alguien tenía una
//     burbuja que servía a dos cosas — se queda con la mayoría y se anota en el
//     log, que es donde una persona puede mirarlo.
//   - PRIORIDAD: el MÁXIMO de sus threads, por eje. Una burbuja con un thread de
//     impacto alto es una burbuja de impacto alto: bajar la nota sería decir que
//     lo urgente de dentro no cuenta.
//
// Las burbujas sin threads, o cuyos threads no tenían nada, se quedan sin nada.
// Un valor por defecto aquí sería una prioridad que nadie decidió.
func init() {
	m.Register(func(app core.App) error {
		bubbles, err := app.FindAllRecords("bubbles")
		if err != nil {
			return err
		}
		rank := map[string]int{"low": 1, "mid": 2, "high": 3}
		names := map[int]string{1: "low", 2: "mid", 3: "high"}

		var lifted, split int
		for _, b := range bubbles {
			threads, err := app.FindAllRecords("threads", dbx.HashExp{"bubble": b.Id})
			if err != nil {
				return err
			}
			if len(threads) == 0 {
				continue
			}

			votes := map[string]int{}
			var impact, urgency int
			for _, t := range threads {
				if o := t.GetString("objective"); o != "" {
					votes[o]++
				}
				if v := rank[t.GetString("impact")]; v > impact {
					impact = v
				}
				if v := rank[t.GetString("urgency")]; v > urgency {
					urgency = v
				}
			}

			best, most := "", 0
			for o, n := range votes {
				if n > most {
					best, most = o, n
				}
			}
			if len(votes) > 1 {
				split++
				log.Printf("migración: la burbuja %q servía a %d objetivos distintos; se queda con el de %d de sus %d threads",
					b.GetString("name"), len(votes), most, len(threads))
			}

			if best == "" && impact == 0 && urgency == 0 {
				continue
			}
			if best != "" {
				b.Set("objective", best)
			}
			if impact > 0 {
				b.Set("impact", names[impact])
			}
			if urgency > 0 {
				b.Set("urgency", names[urgency])
			}
			if err := app.Save(b); err != nil {
				return err
			}
			lifted++
		}
		log.Printf("migración: %d burbujas heredaron objetivo o prioridad de sus threads (%d con objetivos en conflicto)",
			lifted, split)
		return nil
	}, func(app core.App) error {
		// Deshacer esto sería repartir un objetivo y una prioridad de vuelta entre
		// threads que ya no dicen cuál era suyo. Se limpia lo que se escribió y se
		// dice que lo demás no vuelve: una vuelta atrás que miente es peor que una
		// que no existe.
		bubbles, err := app.FindAllRecords("bubbles")
		if err != nil {
			return nil
		}
		for _, b := range bubbles {
			b.Set("objective", "")
			b.Set("impact", "")
			b.Set("urgency", "")
			if err := app.Save(b); err != nil {
				return err
			}
		}
		log.Print("migración: las burbujas quedaron sin objetivo ni prioridad. " +
			"Lo que tenían los threads NO se restaura: al subirlo se perdió de cuál venía cada cosa.")
		return nil
	})
}
