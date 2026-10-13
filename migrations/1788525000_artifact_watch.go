package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/security"
)

// La llave de vigilancia de un artefacto.
//
// Un agente que espera a que la persona conteste corre un script que pregunta
// «¿ya contestaron?». Ese script no puede usar el token del agente: vive en la
// configuración MCP del harness, el agente no lo ve, y además sólo abre `/mcp`.
// Así que cada artefacto lleva su propia llave, que sólo sirve para esa pregunta
// —no lee contenido ni escribe nada— y muere con él.
//
// Oculta: la API de registros no la devuelve nunca. Viaja sólo dentro del
// comando que reciben quien lo abrió (y el lead) por las puertas del servidor.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("artifacts")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.TextField{Name: "watch_key", Hidden: true, Max: 64})
		if err := app.Save(c); err != nil {
			return err
		}
		rows, err := app.FindAllRecords("artifacts")
		if err != nil {
			return err
		}
		for _, r := range rows {
			r.Set("watch_key", security.RandomString(40))
			if err := app.Save(r); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("artifacts")
		if err != nil {
			return nil
		}
		c.Fields.RemoveByName("watch_key")
		return app.Save(c)
	})
}
