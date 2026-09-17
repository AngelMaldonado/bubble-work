package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// La forma de trabajar del departamento, escrita por el lead y leída por los
// agentes: un AGENTS.md para quien planea y otro para quien opera.
//
// Hasta aquí lo único que un agente se instalaba era `prompts/house-rules.md`,
// texto del binario: cambiar cómo trabaja el equipo pedía un commit y un
// despliegue, y lo decidía quien programa en vez de quien dirige. Ahora esa
// sección es breve y le OBLIGA al agente a leer, al empezar cada sesión, el
// documento de su rol aquí (`agents_md` por MCP).
//
// Una fila por VERSIÓN, no una fila por rol que se sobrescribe: cada guardado
// del lead crea una fila nueva y la vigente es la más reciente. Nadie edita ni
// borra una versión — lo que un agente leyó un martes tiene que poder leerse
// después, y eso es la trazabilidad que pide ISO 9001, gratis.
//
// Aquí y no en el árbol de markdown de un workspace: el documento es del
// DEPARTAMENTO, como los objetivos, y un workspace es la frontera de un proyecto.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c := core.NewBaseCollection("agents_md")
		c.Fields.Add(&core.SelectField{
			Name: "role", Required: true, MaxSelect: 1,
			Values: []string{"planner", "operators"},
		})
		// Mismo tope que una nota: un campo de texto sin Max en PocketBase es
		// 5000, y una forma de trabajo con tablas ya pasa de eso.
		c.Fields.Add(&core.TextField{Name: "content", Max: roomyText})
		// Sin borrado en cascada: a una persona se la borra de forma lógica, y si
		// un superuser la borrara de verdad, la historia no debe irse con ella.
		c.Fields.Add(&core.RelationField{
			Name: "author", CollectionId: users.Id, Required: true, MaxSelect: 1,
		})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.AddIndex("idx_agents_md_role_created", false, "role, created", "")

		signedIn := `@request.auth.id != ""`
		c.ListRule = types.Pointer(signedIn)
		c.ViewRule = types.Pointer(signedIn)
		// Sólo el lead global, y firmada por quien la guarda: sin la segunda
		// condición cualquiera con el rol podría atribuirle una versión a otro.
		c.CreateRule = types.Pointer(globalLead + ` && @request.body.author = @request.auth.id`)
		c.UpdateRule = nil
		c.DeleteRule = nil
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("agents_md")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
