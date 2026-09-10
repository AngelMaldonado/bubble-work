package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Dónde está el código de un proyecto.
//
// Una fila por repositorio y no una lista dentro del workspace: son varios,
// entran y salen, y cada uno quiere su propio nombre — que en una cadena
// separada por comas es exactamente el momento en que alguien inventa un
// formato. Además, una fila se puede mirar, borrar y contar.
//
// Esto NO es el repositorio del workspace. Ese es `repo_path`, el árbol de
// markdown que este servidor escribe, y no se elige ni se enlaza. Estos son los
// repositorios donde vive el CÓDIGO, que este servidor no toca: un enlace, y la
// honestidad de decir que la evidencia vive ahí afuera.
//
// Como todo lo del workspace: lo ven sus miembros, lo escribe su lead.
func init() {
	m.Register(func(app core.App) error {
		ws, err := app.FindCollectionByNameOrId("workspaces")
		if err != nil {
			return err
		}
		r := core.NewBaseCollection("workspace_repos")
		r.Fields.Add(&core.RelationField{
			Name: "workspace", CollectionId: ws.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true,
		})
		r.Fields.Add(&core.URLField{Name: "url", Required: true})
		// Vacío significa "el que diga la URL": `owner/repo` es como se llama un
		// repositorio en voz alta, y pedirlo dos veces es pedirlo de más.
		r.Fields.Add(&core.TextField{Name: "name", Max: 120})
		r.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		r.AddIndex("idx_workspace_repos_ws", false, "workspace", "")
		r.AddIndex("idx_workspace_repos_url", true, "workspace, url", "")

		member := `workspace.memberships_via_workspace.user ?= @request.auth.id`
		lead := member + ` && workspace.memberships_via_workspace.role ?= 'lead'`
		r.ListRule = types.Pointer(orLead(member))
		r.ViewRule = types.Pointer(orLead(member))
		r.CreateRule = types.Pointer(orLead(lead))
		r.UpdateRule = types.Pointer(orLead(lead))
		r.DeleteRule = types.Pointer(orLead(lead))
		return app.Save(r)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("workspace_repos")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
