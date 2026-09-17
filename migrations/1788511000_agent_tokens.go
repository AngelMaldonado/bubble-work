package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Tokens propios del agente, no la sesión de la persona.
//
// Hasta aquí un agente entraba al MCP con el token de la sesión del navegador
// de quien copió el prompt. Eso tenía tres problemas: caducaba con la sesión
// (el MCP respondía 401 a los cinco días), no se podía revocar uno solo sin
// cerrar todas las sesiones de la persona, y nadie sabía qué agente seguía vivo.
//
// Cada fila es un token: de quién es, cómo se llama, cuándo caduca (o nunca),
// cuándo se usó por última vez y si se revocó. El token NO se guarda: sólo su
// sha256, en un campo oculto. Se enseña una vez, al crearlo, y quien lo pierda
// genera otro.
//
// Nadie escribe aquí por la API de registros: crear, extender, renombrar y
// revocar son rutas propias (`internal/bubble/tokens.go`), porque el hash lo
// calcula el servidor y un cliente que pudiera escribirlo podría fabricarse uno.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c := core.NewBaseCollection("agent_tokens")
		c.Fields.Add(&core.RelationField{
			Name: "owner", CollectionId: users.Id, Required: true, MaxSelect: 1,
			// Un usuario sólo se borra de forma lógica; si un superuser lo
			// borrara de verdad, sus tokens no tienen sentido sin él.
			CascadeDelete: true,
		})
		c.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 80})
		c.Fields.Add(&core.TextField{Name: "hash", Required: true, Max: 64, Hidden: true})
		// Lo que se enseña para reconocerlo: `bw_Ab12Cd…`.
		c.Fields.Add(&core.TextField{Name: "prefix", Max: 16})
		c.Fields.Add(&core.DateField{Name: "expires_at"})
		c.Fields.Add(&core.DateField{Name: "last_used_at"})
		c.Fields.Add(&core.DateField{Name: "revoked_at"})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		c.AddIndex("idx_agent_tokens_hash", true, "hash", "")

		// Los tuyos, y el lead global ve los de todos: es quien revoca el de
		// alguien que se fue.
		mine := `owner = @request.auth.id || @request.auth.role = 'lead'`
		c.ListRule = types.Pointer(mine)
		c.ViewRule = types.Pointer(mine)
		c.CreateRule = nil
		c.UpdateRule = nil
		c.DeleteRule = nil
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("agent_tokens")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
