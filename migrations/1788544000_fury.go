package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Furia de IA (hilo #6 de Burbujas).
//
//   - `features.fury`: el interruptor. Nace APAGADO: con él apagado nadie la
//     calcula ni la ve. Es del departamento, no de un equipo: las reglas son
//     del lead global e iguales para todos.
//   - `fury_rules`: una fila con el catálogo de reglas en JSON (umbrales y
//     niveles). Sin fila, valen las de serie (`fury.Defaults`).
//
// Sin reglas de API en `fury_rules`: se lee y se escribe por `/api/fury/rules`,
// que valida el catálogo y que sólo deja escribir al lead global. La furia en
// sí no se guarda en ningún sitio: se calcula.
func init() {
	m.Register(func(app core.App) error {
		f, err := app.FindCollectionByNameOrId("features")
		if err != nil {
			return err
		}
		f.Fields.Add(&core.BoolField{Name: "fury"})
		if err := app.Save(f); err != nil {
			return err
		}
		c := core.NewBaseCollection("fury_rules")
		c.Fields.Add(&core.JSONField{Name: "rules", MaxSize: 20000})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		return app.Save(c)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("fury_rules"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		f, err := app.FindCollectionByNameOrId("features")
		if err != nil {
			return nil
		}
		f.Fields.RemoveByName("fury")
		return app.Save(f)
	})
}
