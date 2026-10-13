package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Artefactos efímeros: lo que se piensa ANTES de que exista el trabajo.
//
// El primero es el interrogatorio (grill): un agente pregunta en rondas, la
// persona contesta en la aplicación o en su terminal, y al final el trabajo nace
// en un hilo que se sostiene solo. El modelo es general a propósito —`kind`— y
// cada clase nueva añade tipos de entrada y su validación en Go, no colecciones.
//
// Dos colecciones y no un JSON por artefacto: el agente escribe la ronda 3
// mientras la persona contesta la pregunta 12, y con un solo documento los dos
// escribirían la misma fila. Las entradas sólo se AÑADEN —nadie edita una
// respuesta, se da otra— así que no hay carrera, y cada una llega en vivo por el
// stream de PocketBase.
//
// Efímero quiere decir tres cosas: fuera de git, fuera del board y del calor, y
// BORRADO a los N días del último movimiento (`tuning.artifact_days`), salvo que
// alguien lo fije. Lo que vale la pena guardar pasa al hilo en el traspaso.
//
// Cuelga de una nota del inbox, que es el tema y la fila del backlog: si el
// artefacto caduca, la nota se queda. Borrar la nota sí se lleva sus artefactos.
//
// Sólo el servidor escribe: el orden (`seq`), el autor y la validación por clase
// no son cosas que un cliente pueda prometer.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		inbox, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}

		art := core.NewBaseCollection("artifacts")
		art.Fields.Add(&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: []string{"grill"}})
		art.Fields.Add(&core.TextField{Name: "title", Required: true, Max: 300})
		art.Fields.Add(&core.RelationField{
			Name: "inbox_item", CollectionId: inbox.Id, Required: true, MaxSelect: 1, CascadeDelete: true,
		})
		art.Fields.Add(&core.RelationField{Name: "creator", CollectionId: users.Id, Required: true, MaxSelect: 1})
		art.Fields.Add(&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"open", "handed_off"}})
		art.Fields.Add(&core.BoolField{Name: "pinned"})
		// Desde aquí corre el reloj de caducidad: la última entrada, o el traspaso.
		art.Fields.Add(&core.DateField{Name: "touched_at", Required: true})
		// La última vez que escribió un AGENTE: lo que dice «en vivo» en la UI.
		art.Fields.Add(&core.DateField{Name: "agent_at"})
		art.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		art.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		art.AddIndex("idx_artifacts_creator", false, "creator", "")
		art.AddIndex("idx_artifacts_inbox", false, "inbox_item", "")

		mine := `creator = @request.auth.id || ` + globalLead
		art.ListRule = types.Pointer(mine)
		art.ViewRule = types.Pointer(mine)
		art.CreateRule, art.UpdateRule, art.DeleteRule = nil, nil, nil
		if err := app.Save(art); err != nil {
			return err
		}

		ent := core.NewBaseCollection("artifact_entries")
		ent.Fields.Add(&core.RelationField{
			Name: "artifact", CollectionId: art.Id, Required: true, MaxSelect: 1, CascadeDelete: true,
		})
		ent.Fields.Add(&core.NumberField{Name: "seq", Required: true, OnlyInt: true})
		ent.Fields.Add(&core.TextField{Name: "type", Required: true, Max: 40})
		ent.Fields.Add(&core.JSONField{Name: "data", MaxSize: 256 << 10})
		ent.Fields.Add(&core.RelationField{Name: "author", CollectionId: users.Id, Required: true, MaxSelect: 1})
		// Por dónde llegó: la aplicación, o un agente desde su harness. Una
		// respuesta dada en la terminal la anota el agente, marcada así.
		ent.Fields.Add(&core.SelectField{Name: "via", Required: true, MaxSelect: 1, Values: []string{"app", "harness"}})
		ent.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		ent.AddIndex("idx_artifact_entries_seq", true, "artifact, seq", "")

		theirs := `artifact.creator = @request.auth.id || ` + globalLead
		ent.ListRule = types.Pointer(theirs)
		ent.ViewRule = types.Pointer(theirs)
		ent.CreateRule, ent.UpdateRule, ent.DeleteRule = nil, nil, nil
		if err := app.Save(ent); err != nil {
			return err
		}

		// Cuántos días vive un artefacto sin moverse. Del departamento, en la fila
		// que ya calibra el resto: la cambia el lead global.
		tun, err := app.FindCollectionByNameOrId("tuning")
		if err != nil {
			return err
		}
		tun.Fields.Add(&core.NumberField{Name: "artifact_days", OnlyInt: true})
		if err := app.Save(tun); err != nil {
			return err
		}
		rows, err := app.FindAllRecords("tuning")
		if err != nil {
			return err
		}
		for _, r := range rows {
			r.Set("artifact_days", 30)
			if err := app.Save(r); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"artifact_entries", "artifacts"} {
			if c, err := app.FindCollectionByNameOrId(name); err == nil {
				if err := app.Delete(c); err != nil {
					return err
				}
			}
		}
		if tun, err := app.FindCollectionByNameOrId("tuning"); err == nil {
			tun.Fields.RemoveByName("artifact_days")
			return app.Save(tun)
		}
		return nil
	})
}
