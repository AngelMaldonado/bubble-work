package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Canales: por dónde entra al inbox lo que no se escribe en la app.
//
// `channels` es una fila por tipo de canal (whatsapp, …): si está activo, su
// configuración no secreta (qué grupo escuchar) y a nombre de quién se capturan
// las notas — `inbox_items.captured_by` exige una persona, y quien conecta el
// canal es quien responde por lo que entra. Las credenciales de un canal NO van
// aquí: WhatsApp guarda su sesión en su propio archivo, porque son llaves.
//
// Sin reglas de la API de registros: sólo el servidor la lee y la escribe, por
// las rutas de `internal/channels`, que exigen al lead global.
//
// En `inbox_items`, de dónde vino una nota: el canal (`source`), el id del
// mensaje en ese canal (`source_ref`, único, para que un mensaje que llega dos
// veces —una reconexión, un reintento— entre una sola vez) y quién lo mandó
// allí (`source_from`), que no tiene por qué ser una persona de la cuenta.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c := core.NewBaseCollection("channels")
		c.Fields.Add(&core.TextField{Name: "kind", Required: true, Max: 40, Pattern: `^[a-z0-9-]+$`})
		c.Fields.Add(&core.BoolField{Name: "enabled"})
		c.Fields.Add(&core.JSONField{Name: "config", MaxSize: 64 << 10})
		c.Fields.Add(&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		c.AddIndex("idx_channels_kind", true, "kind", "")
		if err := app.Save(c); err != nil {
			return err
		}

		inbox, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}
		inbox.Fields.Add(&core.TextField{Name: "source", Max: 40})
		inbox.Fields.Add(&core.TextField{Name: "source_ref", Max: 200})
		inbox.Fields.Add(&core.TextField{Name: "source_from", Max: 200})
		inbox.AddIndex("idx_inbox_source_ref", true, "source, source_ref", "source_ref != ''")
		return app.Save(inbox)
	}, func(app core.App) error {
		inbox, err := app.FindCollectionByNameOrId("inbox_items")
		if err == nil {
			inbox.RemoveIndex("idx_inbox_source_ref")
			for _, f := range []string{"source", "source_ref", "source_from"} {
				inbox.Fields.RemoveByName(f)
			}
			if err := app.Save(inbox); err != nil {
				return err
			}
		}
		if c, err := app.FindCollectionByNameOrId("channels"); err == nil {
			return app.Delete(c)
		}
		return nil
	})
}
