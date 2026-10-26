package bubble

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// El equipo de un hilo lo DERIVA el servidor (E1). Con proyecto, es el del
// proyecto, y lo que mande el cliente se pisa: dejarlo elegir permitiría que un
// hilo dijera pertenecer a un equipo que no es el de su proyecto, y cuando las
// reglas lo lean eso sería una puerta.
//
// Va en validación (no en el hook de la petición) para cubrir también
// `app.Save`: MCP, canales y migraciones.
func registerThreadTeam(app core.App) {
	app.OnRecordValidate("threads").BindFunc(func(e *core.RecordEvent) error {
		if ws := e.Record.GetString("workspace"); ws != "" {
			w, err := e.App.FindRecordById("workspaces", ws)
			if err != nil {
				return err
			}
			e.Record.Set("team", w.GetString("team"))
		}
		return e.Next()
	})

	// Mover un proyecto de equipo arrastra a sus hilos, en la misma
	// transacción: si no se pueden actualizar, el proyecto no se mueve.
	app.OnRecordUpdate("workspaces").BindFunc(func(e *core.RecordEvent) error {
		now := e.Record.GetString("team")
		if now == e.Record.Original().GetString("team") {
			return e.Next()
		}
		return e.App.RunInTransaction(func(tx core.App) error {
			e.App = tx
			if err := e.Next(); err != nil {
				return err
			}
			_, err := tx.DB().NewQuery(`UPDATE threads SET team = {:t} WHERE workspace = {:w}`).
				Bind(dbx.Params{"t": now, "w": e.Record.Id}).Execute()
			return err
		})
	})
}
