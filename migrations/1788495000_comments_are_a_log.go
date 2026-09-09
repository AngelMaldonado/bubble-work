package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Lo dicho, dicho: los comentarios se escriben y no se reescriben.
//
// Hasta ahora el autor podía editar y borrar el suyo, que es lo razonable en un
// chat. Un thread no es un chat: es el registro de una conversación sobre
// trabajo, y su valor está justo en que se pueda leer después para entender por
// qué se decidió algo. Un hilo donde las frases cambian o desaparecen deja de
// servir para eso — y lo peor no es la frase borrada, es que las respuestas que
// quedan pasan a contestar a algo que ya no está.
//
// Así que es un log al que se agrega: `create` sigue siendo de cualquier miembro
// del thread, y `update` y `delete` no son de nadie. Nil, no "sólo el lead":
// dárselo al lead convertiría el registro en algo que se puede maquillar desde
// arriba, que es la versión peor del mismo problema.
//
// Corregir lo dicho es decir otra cosa. El comentario equivocado se queda, y
// debajo va el que lo corrige — que es también como funciona fuera de una
// pantalla.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("comments")
		if err != nil {
			return err
		}
		c.UpdateRule = nil
		c.DeleteRule = nil
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("comments")
		if err != nil {
			return err
		}
		mine := `thread.workspace.memberships_via_workspace.user ?= @request.auth.id` +
			` && author = @request.auth.id`
		c.UpdateRule = &mine
		c.DeleteRule = &mine
		return app.Save(c)
	})
}
