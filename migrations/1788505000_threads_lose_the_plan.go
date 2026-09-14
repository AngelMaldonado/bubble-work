package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El thread se queda con la ejecución, y suelta el plan.
//
// Fuera `objective`, `impact` y `urgency`, y con ellos la vista
// `thread_priority`. Ya están arriba: la migración anterior los subió a la
// burbuja y ésta quita el sitio donde podían volver a escribirse.
//
// Dejarlos «por si acaso» habría sido peor que quitarlos: dos sitios donde
// escribir la misma respuesta es la definición de un dato que se contradice, y
// el que nadie lee es siempre el que alguien acaba creyendo.
//
// Lo que un thread conserva es lo suyo: su nombre, de qué burbuja es, su estado
// en el flujo del proyecto, quién lo tiene, cuándo vence y su documento. El
// operador se organiza como quiera dentro de eso.
//
// Ésta es la que hace irreversible el cambio: `down` devuelve las columnas
// vacías, no lo que decían.
func init() {
	m.Register(func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("thread_priority"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		t, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		for _, f := range []string{"objective", "impact", "urgency"} {
			t.Fields.RemoveByName(f)
		}
		return app.Save(t)
	}, func(app core.App) error {
		t, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return nil
		}
		obj, err := app.FindCollectionByNameOrId("objectives")
		if err != nil {
			return err
		}
		t.Fields.Add(&core.RelationField{Name: "objective", CollectionId: obj.Id, MaxSelect: 1})
		t.Fields.Add(&core.SelectField{
			Name: "impact", Values: []string{"high", "mid", "low"}, MaxSelect: 1,
		})
		t.Fields.Add(&core.SelectField{
			Name: "urgency", Values: []string{"high", "mid", "low"}, MaxSelect: 1,
		})
		// Las columnas vuelven vacías: lo que decían se subió a las burbujas y
		// allí perdió de cuál thread venía. Una vuelta atrás que rellenara esto
		// estaría inventando.
		return app.Save(t)
	})
}
