package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// De qué mensajes se hizo una nota, pieza por pieza.
//
// Una nota de un canal puede venir de varios mensajes: un álbum de WhatsApp son
// un mensaje «álbum» y una foto por mensaje. `source_ref` es el del conjunto;
// `source_parts` dice qué mensaje trajo cada archivo — [{ref, file}] —, que es
// lo que permite que borrar UNA foto en el chat quite esa foto de la nota y no
// la nota entera.
func init() {
	m.Register(func(app core.App) error {
		inbox, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}
		if inbox.Fields.GetByName("source_parts") == nil {
			inbox.Fields.Add(&core.JSONField{Name: "source_parts", MaxSize: 64 << 10})
		}
		return app.Save(inbox)
	}, func(app core.App) error {
		inbox, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}
		inbox.Fields.RemoveByName("source_parts")
		return app.Save(inbox)
	})
}
