package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Lo que se le dice a la IA, editable desde la aplicación.
//
// Los prompts del binario (`guide`, `house_rules`, `connect`, `grill`) y la
// descripción de cada tool MCP eran texto embebido: cambiarlos pedía un commit
// y un despliegue. Ahora el lead los edita en «Prompts», como ya editaba los
// AGENTS.md, y lo embebido queda como el ORIGINAL al que se puede volver.
//
// Una fila por versión, como `agents_md`: nadie edita ni borra una, la vigente
// es la más reciente, y `reset` es una versión que dice «vuelve al original».
// `base` es la huella del original cuando se guardó: si el binario cambia ese
// texto después, la pantalla avisa de que la versión editada se quedó atrás.
//
// Los AGENTS.md siguen en `agents_md`; esto es todo lo demás.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c := core.NewBaseCollection("prompt_versions")
		c.Fields.Add(&core.TextField{Name: "key", Required: true, Max: 120})
		c.Fields.Add(&core.TextField{Name: "content", Max: roomyText})
		c.Fields.Add(&core.BoolField{Name: "reset"})
		c.Fields.Add(&core.TextField{Name: "base", Max: 64})
		c.Fields.Add(&core.RelationField{Name: "author", CollectionId: users.Id, Required: true, MaxSelect: 1})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.AddIndex("idx_prompt_versions_key_created", false, "key, created", "")
		signedIn := `@request.auth.id != ""`
		c.ListRule = types.Pointer(signedIn)
		c.ViewRule = types.Pointer(signedIn)
		// Sólo por la ruta del servidor, que sabe cuál es el original y sella la
		// huella. Una fila escrita a mano no la llevaría.
		c.CreateRule, c.UpdateRule, c.DeleteRule = nil, nil, nil
		return app.Save(c)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("prompt_versions"); err == nil {
			return app.Delete(c)
		}
		return nil
	})
}
