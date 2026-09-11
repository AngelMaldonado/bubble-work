package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Una nota capturada podía crecer en la pantalla y no en ninguna parte.
//
// El panel de una nota ofrece un campo de markdown —«¿qué pidió exactamente?
// ¿a quién afecta?»— porque una captura suele llegar como media frase y
// entenderla es lo que la vuelve triable. Pero `inbox_items` sólo tenía `note`,
// así que lo escrito ahí no tenía dónde guardarse: se veía mientras el panel
// estaba abierto y desaparecía al recargar.
//
// Un campo en la base y no un archivo en el árbol, que es donde vive el resto
// de la escritura de este producto: un archivo pertenece al repositorio de un
// workspace, y una nota del inbox no tiene workspace todavía — no tenerlo es
// exactamente lo que la hace una nota y no trabajo.
//
// Y sigue sin calentar nada. Escribir aquí no es evidencia: lo que cambia la
// realidad es el thread en que esto se convierta.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.TextField{Name: "body", Max: 20000})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return nil
		}
		c.Fields.RemoveByName("body")
		return app.Save(c)
	})
}
