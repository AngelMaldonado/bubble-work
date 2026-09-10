package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Qué pieza representa a cada cosa del inventario.
//
// Se guarda el NOMBRE del dibujo —`computer`, `key`, `servidor`— y no la imagen:
// la imagen ya está en el disco de todos, y guardarla por fila sería una copia
// por grupo de algo que no cambia. En un item, vacío significa «la de mi grupo»:
// dentro de Dominios todo es un dominio hasta que alguien diga otra cosa.
//
// Va en su propia migración y no en la del inventario, aunque nació el mismo
// día, por la razón que lo hizo evidente: la otra ya había corrido, y editar una
// migración aplicada no la vuelve a ejecutar. El campo no existía, PocketBase
// descartaba el valor sin decir nada, y la galería caía en su dibujo por defecto
// como si nadie hubiera elegido — que es exactamente lo que parecía desde el
// otro lado de la pantalla.
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"inventory_groups", "inventory_items"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.Fields.Add(&core.TextField{Name: "art", Max: 60})
			if err := app.Save(c); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"inventory_groups", "inventory_items"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			c.Fields.RemoveByName("art")
			if err := app.Save(c); err != nil {
				return err
			}
		}
		return nil
	})
}
