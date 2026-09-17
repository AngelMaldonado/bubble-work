package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Borrar a una persona es marcarla, no quitar su fila.
//
// Borrarla de verdad no se puede y no se debe: un comentario exige autor, y lo
// que alguien escribió, comentó o enlazó es historia del departamento. PocketBase
// ya lo rechazaba («record is part of a required relation reference»).
//
// `deleted_at` con fecha es «borrada»:
//
//   - no entra: `authRule` la rechaza al iniciar sesión y al refrescar, y al
//     borrarla se rota su `tokenKey`, así que ninguna sesión abierta sobrevive;
//   - sus tokens de agente dejan de valer (lo comprueba el middleware de /mcp);
//   - sus membresías se CONSERVAN, así que restaurarla —vaciar la fecha— lo deja
//     todo como estaba;
//   - lo que escribió se queda, firmado con su nombre.
//
// Nadie marca `deleted_at` editando un usuario: lo hacen las rutas de
// `internal/bubble/people.go`, que son las que impiden borrarse a uno mismo o
// dejar el departamento sin lead global. Y el borrado duro por la API se cierra
// del todo: la regla por defecto de PocketBase dejaba a cada persona borrarse a
// sí misma.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if users.Fields.GetByName("deleted_at") == nil {
			users.Fields.Add(&core.DateField{Name: "deleted_at"})
		}
		users.AuthRule = types.Pointer(`deleted_at = ""`)
		users.DeleteRule = nil
		users.UpdateRule = types.Pointer(
			`(` + orLead(`id = @request.auth.id && @request.body.role:isset = false`) + `)` +
				` && @request.body.deleted_at:isset = false`)
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.RemoveByName("deleted_at")
		users.AuthRule = types.Pointer("")
		users.UpdateRule = types.Pointer(
			orLead(`id = @request.auth.id && @request.body.role:isset = false`))
		return app.Save(users)
	})
}
