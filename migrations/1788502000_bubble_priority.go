package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// La prioridad y el objetivo pasan a ser de la BURBUJA.
//
// Estaban en el thread, y era un nivel demasiado bajo para las dos cosas. Un
// objetivo dice PARA QUÉ sirve un cuerpo de trabajo, y el cuerpo de trabajo es
// la burbuja: colgarlo de cada thread obligaba a repetir la misma respuesta en
// cada pieza y dejaba que dos piezas de lo mismo se contradijeran. La prioridad
// es lo mismo por otro lado: quien la decide es quien orquesta, no quien
// ejecuta, y el operador ya tiene su propia forma de ordenarse el día.
//
// La tabla no cambia —impacto × urgencia, la misma de siempre— sólo cambia de
// dueño. Y sigue sin guardarse: una vista, porque un veredicto almacenado es uno
// que alguien puede escribir en contra del mapa, y eso convierte el mapa en
// decoración.
func init() {
	m.Register(func(app core.App) error {
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return err
		}
		obj, err := app.FindCollectionByNameOrId("objectives")
		if err != nil {
			return err
		}
		// UNO, no varios. Una burbuja con dos objetivos son dos burbujas: su
		// outcome deja de ser uno solo. Ampliar de uno a varios más adelante es
		// trivial; al revés hay que elegir cuál se tira.
		b.Fields.Add(&core.RelationField{
			Name: "objective", CollectionId: obj.Id, MaxSelect: 1,
		})
		b.Fields.Add(&core.SelectField{
			Name: "impact", Values: []string{"high", "mid", "low"}, MaxSelect: 1,
		})
		b.Fields.Add(&core.SelectField{
			Name: "urgency", Values: []string{"high", "mid", "low"}, MaxSelect: 1,
		})
		if err := app.Save(b); err != nil {
			return err
		}

		// La misma vista que la de threads, con el sujeto cambiado. Sobre una sola
		// línea y con CAST, por lo mismo que la otra: PocketBase deriva los campos
		// parseando este SELECT, y sin el cast la columna calculada llega tipada
		// como json y `priority = 'P1'` acaba filtrando por JSON_EXTRACT.
		pri := core.NewViewCollection("bubble_priority")
		pri.ViewQuery = "SELECT b.id AS id, b.workspace AS workspace, b.name AS name," +
			" b.objective AS objective, b.impact AS impact, b.urgency AS urgency," +
			" CAST(CASE" +
			"   WHEN b.impact = '' OR b.urgency = '' THEN ''" +
			"   WHEN b.impact = 'high' AND b.urgency = 'high' THEN 'P1'" +
			"   WHEN b.impact = 'high' AND b.urgency = 'mid'  THEN 'P2'" +
			"   WHEN b.impact = 'high' AND b.urgency = 'low'  THEN 'P3'" +
			"   WHEN b.impact = 'mid'  AND b.urgency = 'high' THEN 'P2'" +
			"   WHEN b.impact = 'mid'  AND b.urgency = 'mid'  THEN 'P2'" +
			"   WHEN b.impact = 'mid'  AND b.urgency = 'low'  THEN 'P3'" +
			"   WHEN b.impact = 'low'  AND b.urgency = 'high' THEN 'P3'" +
			"   WHEN b.impact = 'low'  AND b.urgency = 'mid'  THEN 'P3'" +
			"   ELSE 'P4'" +
			" END AS TEXT) AS priority" +
			" FROM bubbles b"
		seen := orLead(`workspace.memberships_via_workspace.user ?= @request.auth.id`)
		pri.ListRule = types.Pointer(seen)
		pri.ViewRule = types.Pointer(seen)
		return app.Save(pri)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("bubble_priority"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return nil
		}
		for _, f := range []string{"objective", "impact", "urgency"} {
			b.Fields.RemoveByName(f)
		}
		return app.Save(b)
	})
}
