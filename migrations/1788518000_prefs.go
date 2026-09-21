package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Las preferencias de una persona: una fila suya, con un JSON dentro.
//
// Casi todo lo que elige quien mira vive en `localStorage` y ahí se queda: el
// orden de las columnas del kanban, qué paneles del planeador están abiertos y
// cuánto miden, el tema, el modo vim, el carril de filtros. Está escrito por qué
// (`frontend/src/lib/filters.svelte.ts`): acomodar MI vista no es acomodar la de
// los demás, y una preferencia de navegador no necesita una fila.
//
// Esto es para lo que sí tiene que seguir a la persona entre dispositivos —el
// primero, en qué modo ve el planeador: por módulos o por tareas—. Abrir el
// portátil y encontrarse el tablero en el otro modo es tener que volver a tomar
// una decisión ya tomada.
//
// Un JSON y no una columna por preferencia: cada una nueva sería otra migración
// y otro campo, y esto no es un esquema que nadie consulte ni filtre — es un
// saco que sólo lee y escribe su dueña. Si algún día hay que preguntar por el
// contenido, eso será una columna de verdad y esta seguirá siendo el saco.
//
// Nadie lee la de otro. No porque haya un secreto dentro, sino porque no hay
// ninguna razón para poder: la única pregunta legítima es «¿cómo quiero ver YO
// esto?».
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		p := core.NewBaseCollection("prefs")
		p.Fields.Add(&core.RelationField{
			Name: "user", CollectionId: users.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true,
		})
		p.Fields.Add(&core.JSONField{Name: "value", MaxSize: 64_000})
		p.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		p.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		// Una fila por persona. Dos serían dos respuestas a la misma pregunta.
		p.AddIndex("idx_prefs_user", true, "user", "")

		mine := `user = @request.auth.id`
		p.ListRule = types.Pointer(mine)
		p.ViewRule = types.Pointer(mine)
		// Al crear se comprueba el CUERPO, que es lo único que existe todavía:
		// sin esto, cualquiera podría escribirle las preferencias a otro.
		p.CreateRule = types.Pointer(`@request.body.user = @request.auth.id`)
		// Al actualizar, las dos: la fila tiene que ser mía Y tiene que seguir
		// siéndolo después, o una edición la regalaría.
		p.UpdateRule = types.Pointer(`user = @request.auth.id && @request.body.user:isset = false`)
		// Borrarla no tiene sentido: es una fila por persona que se sobreescribe.
		// Quien quiera volver al principio, guarda `{}`.
		p.DeleteRule = nil
		return app.Save(p)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("prefs")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
