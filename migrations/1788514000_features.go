package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Funciones que el departamento enciende o apaga.
//
// Una fila, como `tuning`, y por la misma razón: activar algo es un UPDATE, no
// un despliegue. No va DENTRO de `tuning` porque aquello es la calibración de la
// flotabilidad —números que cambian lo que flota— y esto es qué partes del
// producto existen para este departamento.
//
// La primera es la secuencia de ejecución. Nace APAGADA: cambia cómo se decide
// qué se hace primero, y eso lo decide el lead global, no una migración. Apagada,
// los datos (`threads.sequence`) se quedan: volver a encenderla trae la línea
// como estaba.
func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("features")
		c.Fields.Add(&core.BoolField{Name: "sequence"})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		signedIn := `@request.auth.id != ""`
		c.ListRule = types.Pointer(signedIn)
		c.ViewRule = types.Pointer(signedIn)
		c.UpdateRule = types.Pointer(globalLead)
		c.CreateRule = nil
		c.DeleteRule = nil
		if err := app.Save(c); err != nil {
			return err
		}
		row := core.NewRecord(c)
		row.Set("sequence", false)
		return app.Save(row)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("features")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
