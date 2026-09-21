package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Los MÓDULOS también se ordenan.
//
// `threads.sequence` ordena las piezas; esto ordena los cuerpos de trabajo. Son
// **dos líneas independientes**, a propósito y no por descuido: un módulo en el
// paso 2 puede tener hilos en el paso 5 de la suya, porque responden a dos
// preguntas distintas —«¿qué cuerpo de trabajo atacamos antes?» y «¿qué pieza
// se hace antes?»— y atarlas obligaría a que mover una moviera la otra.
//
// Lo que NO cambia, y es lo que hay que leer antes de tocar esto: **`next`
// sigue siendo de hilos**. La tool que un agente pregunta para saber qué le
// toca contesta con piezas ejecutables, porque es lo que se puede empezar y
// terminar. Un módulo no se «hace»; se planea y se cierra. Si algún día `next`
// tiene que hablar de módulos, será una decisión propia y no una consecuencia
// de que exista esta columna.
//
// Cómo se recorre: «ahora» es el primer paso cuya burbuja sigue ABIERTA.
// Cerrarla avanza la línea, y cerrar ya es una decisión con fecha y frase — la
// alternativa, derivarlo de que no le queden hilos abiertos, haría que un
// módulo recién creado contara como terminado desde que nace y que añadirle un
// hilo lo hiciera retroceder.
//
// La escribe el lead global, como la de los hilos y como el resto de la capa
// estratégica: el orden de ejecución del departamento lo decide quien orquesta.
// La línea cruza proyectos, así que dejársela al lead de cada proyecto sería
// dos personas reordenando la misma línea sin saberlo.
func init() {
	m.Register(func(app core.App) error {
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return err
		}
		if b.Fields.GetByName("sequence") == nil {
			b.Fields.Add(&core.NumberField{Name: "sequence", OnlyInt: true})
		}
		// La misma guarda que `1788513000_thread_priority_sequence.go`: quien no
		// es lead global puede seguir escribiendo todo lo demás de una burbuja,
		// pero no su paso. `:isset` distingue «no lo mando» de «lo mando vacío».
		guard := ` && (@request.body.sequence:isset = false || @request.auth.role = 'lead')`
		if b.CreateRule != nil && *b.CreateRule != "" {
			b.CreateRule = types.Pointer("(" + *b.CreateRule + ")" + guard)
		}
		if b.UpdateRule != nil && *b.UpdateRule != "" {
			b.UpdateRule = types.Pointer("(" + *b.UpdateRule + ")" + guard)
		}
		return app.Save(b)
	}, func(app core.App) error {
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return nil
		}
		b.Fields.RemoveByName("sequence")
		return app.Save(b)
	})
}
