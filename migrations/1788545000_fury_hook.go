package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El webhook de los semáforos de la oficina (hilo #6, historia 3): una fila con
// la URL a la que Bubble avisa del nivel de furia de cada persona, el secreto
// con el que firma cada aviso (HMAC-SHA256) y cómo fue el último envío.
//
// El secreto se guarda en claro —firmar lo necesita— en un campo oculto, y la
// colección no tiene reglas: sólo la leen y escriben las rutas
// `/api/fury/hook`, del lead global, que enseñan el secreto una sola vez.
func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("fury_hook")
		c.Fields.Add(&core.URLField{Name: "url"})
		c.Fields.Add(&core.TextField{Name: "secret", Hidden: true, Max: 128})
		c.Fields.Add(&core.TextField{Name: "last_status", Max: 300})
		c.Fields.Add(&core.DateField{Name: "last_at"})
		c.Fields.Add(&core.NumberField{Name: "last_sent", OnlyInt: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("fury_hook")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
