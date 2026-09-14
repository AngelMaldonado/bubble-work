package bubble

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// defaultTextMax es lo que PocketBase aplica a un campo de texto sin `Max`: un
// campo "sin tope" NO es ilimitado. Si el cliente no lo supiera, un nombre de
// 5001 caracteres sería exactamente el rechazo silencioso que esto evita.
const defaultTextMax = 5000

// registerLimits publica cuánto cabe en cada campo de texto.
//
// El servidor ya lo sabe —es su esquema— y hasta ahora sólo lo decía al
// RECHAZAR: un PATCH con un 400 que la pantalla, que guarda sola al dejar de
// escribir, no tenía dónde enseñar. Así quien escribe se entera antes de
// guardar, mientras aún puede recortar.
//
// Derivado del esquema en cada petición y no copiado en el cliente: dos listas
// de topes se separan, y la que se queda atrás es la que dice «cabe» sobre algo
// que el servidor rechaza. Una migración que cambie un tope cambia esto sola.
func registerLimits(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/limits", func(e *core.RequestEvent) error {
			return e.JSON(http.StatusOK, Limits(e.App))
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}

// Limits: `colección.campo` → caracteres, de cada campo de texto de las
// colecciones propias (ni las del sistema ni las vistas, que no se escriben).
func Limits(app core.App) map[string]int {
	out := map[string]int{}
	cols, err := app.FindAllCollections(core.CollectionTypeBase, core.CollectionTypeAuth)
	if err != nil {
		return out
	}
	for _, c := range cols {
		if c.System && c.Name != "users" {
			continue
		}
		for _, f := range c.Fields {
			switch t := f.(type) {
			case *core.TextField:
				if t.Hidden || t.System || t.PrimaryKey {
					continue
				}
				max := t.Max
				if max <= 0 {
					max = defaultTextMax
				}
				out[c.Name+"."+t.Name] = max
			case *core.EditorField:
				// Éste se mide en BYTES, no en caracteres. Se publica igual: a
				// cinco megas, contar caracteres en vez de bytes sólo se equivoca
				// con texto que nadie escribe a mano.
				if t.Hidden || t.System {
					continue
				}
				out[c.Name+"."+t.Name] = int(t.CalculateMaxBodySize())
			}
		}
	}
	return out
}
