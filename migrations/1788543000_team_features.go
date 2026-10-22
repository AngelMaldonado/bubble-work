package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Funciones por equipo.
//
// `features` sigue siendo una fila global: lo que el lead global enciende para
// todos. Esto la matiza por equipo, porque la secuencia y el roadmap ya son de
// cada equipo y un equipo que no los usa no tiene por qué verlos.
//
// Una fila por equipo, y cada función con tres estados: vacío (hereda el
// global), `on` u `off`. Un select y no un booleano porque un booleano no sabe
// decir «hereda». Rechazado: columnas en `teams`, por lo mismo.
//
// Mi espacio no está aquí: es de cada persona, no de un equipo.
//
// Sin reglas: nadie la lee ni la escribe por la API de registros. Viaja dentro
// de `GET /api/teams` —que ya decide quién ve qué equipo— y se escribe por
// `POST /api/teams/{id}/features`, que valida que quien pide lidere el equipo.
func init() {
	m.Register(func(app core.App) error {
		teams, err := app.FindCollectionByNameOrId("teams")
		if err != nil {
			return err
		}
		c := core.NewBaseCollection("team_features")
		c.Fields.Add(&core.RelationField{Name: "team", CollectionId: teams.Id, Required: true, MaxSelect: 1, CascadeDelete: true})
		for _, f := range []string{"sequence", "roadmap"} {
			c.Fields.Add(&core.SelectField{Name: f, MaxSelect: 1, Values: []string{"on", "off"}})
		}
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		c.AddIndex("idx_team_features_team", true, "team", "")
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("team_features")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
