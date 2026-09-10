package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// El inventario: dónde vive lo que hace funcionar todo esto.
//
// VPS, dominios, servicios contratados, licencias. No es trabajo y no calienta
// nada —documentar un servidor no es evidencia de que la realidad cambió— pero
// es lo primero que alguien busca a las tres de la mañana, y hoy vive en la
// cabeza de una persona o en un chat que nadie encuentra.
//
// Tres colecciones, y la tercera es la que decide quién mira:
//
//	· `inventory_access` — VER el inventario se asigna, y se asigna con una
//	  FILA. Es como se dice pertenecer en todo el resto de este modelo: una
//	  membresía es una fila, y esto es la misma idea aplicada a un armario en
//	  vez de a un proyecto. Un campo booleano en `users` habría sido un tercer
//	  eje sobre el rol, invisible desde el lado del inventario y difícil de
//	  auditar; una fila se ve, se quita, y dice quién la dio.
//	· `inventory_groups` — la galería: VPS, dominios, lo que el departamento
//	  decida. Los grupos son datos y no un `select` en el código, porque la
//	  lista de qué clases de cosas se contratan cambia sin que nadie recompile.
//	· `inventory_items` — lo de adentro, con su proveedor, su fecha de
//	  renovación y sus notas.
//
// NUNCA credenciales. Hay un campo para el enlace a la bóveda: el inventario
// dice DÓNDE está la contraseña, no cuál es. Un inventario que guarda secretos
// es una brecha con buen diseño, y la primera persona que pegue una ahí lo hará
// porque el campo existía.
func init() {
	m.Register(upInventory, downInventory)
}

// stamped da a una colección sus dos fechas. Local a este archivo: el `stamps`
// de la fase 1 es una función dentro de su migración, y llamarla desde aquí
// ataría dos migraciones que deben poder leerse por separado.
func stamped(c *core.Collection) {
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
}

func upInventory(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}

	access := core.NewBaseCollection("inventory_access")
	access.Fields.Add(&core.RelationField{
		Name: "user", CollectionId: users.Id,
		Required: true, MaxSelect: 1, CascadeDelete: true,
	})
	access.Fields.Add(&core.RelationField{
		Name: "granted_by", CollectionId: users.Id, MaxSelect: 1,
	})
	access.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	access.AddIndex("idx_inventory_access_user", true, "user", "")
	// Ves TU propia fila, para que la aplicación pueda preguntarse "¿me toca?"
	// sin pedir permiso para averiguar si tiene permiso. El lead global ve
	// todas, porque repartirlas es su trabajo.
	access.ListRule = types.Pointer(`user = @request.auth.id || ` + globalLead)
	access.ViewRule = types.Pointer(`user = @request.auth.id || ` + globalLead)
	access.CreateRule = types.Pointer(globalLead)
	access.UpdateRule = types.Pointer(globalLead)
	access.DeleteRule = types.Pointer(globalLead)
	if err := app.Save(access); err != nil {
		return err
	}

	// Quien tenga su fila, o el lead global. Escrito una vez y usado en las dos
	// colecciones: dos copias de una condición de acceso son dos respuestas a
	// la misma pregunta, y la que se olvida de actualizar es la que abre.
	//
	// El `@request.auth.id != ""` no es cinturón y tirantes: sin él, con CERO
	// filas de acceso la comparación es vacío contra vacío y da verdadero — así
	// que el inventario se veía entero desde una sesión anónima, y justo mientras
	// está vacío, que es cuando nadie lo estaría mirando para notarlo. Un armario
	// que se abre solo hasta que alguien le pone la primera llave.
	sees := globalLead +
		` || (@request.auth.id != "" && @collection.inventory_access.user ?= @request.auth.id)`

	groups := core.NewBaseCollection("inventory_groups")
	groups.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 80})
	groups.Fields.Add(&core.TextField{Name: "note", Max: 300})
	groups.Fields.Add(&core.FileField{
		Name: "image", MaxSelect: 1, MaxSize: 3 << 20,
		MimeTypes: []string{"image/png", "image/jpeg", "image/webp", "image/svg+xml", "image/avif"},
	})
	groups.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
	stamped(groups)
	groups.AddIndex("idx_inventory_groups_name", true, "name", "")
	groups.ListRule = types.Pointer(sees)
	groups.ViewRule = types.Pointer(sees)
	groups.CreateRule = types.Pointer(globalLead)
	groups.UpdateRule = types.Pointer(globalLead)
	groups.DeleteRule = types.Pointer(globalLead)
	if err := app.Save(groups); err != nil {
		return err
	}

	items := core.NewBaseCollection("inventory_items")
	items.Fields.Add(&core.RelationField{
		Name: "group", CollectionId: groups.Id,
		Required: true, MaxSelect: 1, CascadeDelete: true,
	})
	items.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 120})
	items.Fields.Add(&core.TextField{Name: "provider", Max: 120})
	items.Fields.Add(&core.URLField{Name: "url"})
	// El enlace a la bóveda, no la contraseña. Ver arriba.
	items.Fields.Add(&core.URLField{Name: "vault"})
	// Markdown, y lo renderiza el mismo servidor que todo lo demás.
	items.Fields.Add(&core.TextField{Name: "notes", Max: 5000})
	// Lo que hace ganarse la pantalla: un dominio que expira es el clásico
	// "nadie se dio cuenta".
	items.Fields.Add(&core.DateField{Name: "renews_at"})
	items.Fields.Add(&core.TextField{Name: "cost", Max: 60})
	items.Fields.Add(&core.FileField{
		Name: "image", MaxSelect: 1, MaxSize: 3 << 20,
		MimeTypes: []string{"image/png", "image/jpeg", "image/webp", "image/svg+xml", "image/avif"},
	})
	items.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
	stamped(items)
	items.AddIndex("idx_inventory_items_group", false, "group", "")
	items.ListRule = types.Pointer(sees)
	items.ViewRule = types.Pointer(sees)
	items.CreateRule = types.Pointer(globalLead)
	items.UpdateRule = types.Pointer(globalLead)
	items.DeleteRule = types.Pointer(globalLead)
	return app.Save(items)
}

func downInventory(app core.App) error {
	// En orden inverso: los items apuntan a los grupos.
	for _, name := range []string{"inventory_items", "inventory_groups", "inventory_access"} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		if err := app.Delete(c); err != nil {
			return err
		}
	}
	return nil
}
