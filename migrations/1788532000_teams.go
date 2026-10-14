package migrations

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Equipos. Un equipo agrupa proyectos y tiene sus propios leads; es la
// frontera del CONTENIDO. El rol `lead` de la persona sigue siendo el de
// administración (personas, tokens, prompts, inventario) y además hace lead
// implícito de todo equipo que NO sea personal. Un equipo personal sólo lo ve
// quien está en él: ni siquiera otro lead global.
//
// Lo que antes decía «el lead global ve todo» ahora dice «el lead del equipo
// de este proyecto ve todo lo de ese proyecto». La membresía de proyecto no
// cambia: invitar a alguien a un proyecto de otro equipo le enseña ese
// proyecto y nada más.
//
// Todo lo que existe pasa al equipo «TI»: nadie pierde nada al migrar.
//
// Rechazado: un campo `private` en el proyecto. Un equipo personal es el mismo
// caso, y los equipos además separan objetivos, secuencia, calendario, inbox y
// canal de WhatsApp de cada contexto.

const leadRole = "@request.auth.role = 'lead'"

// teamLead: el lead global (si el equipo no es personal) o una fila de lead en
// el equipo. `p` es la ruta hasta el registro que tiene `team` ("", "workspace.").
func teamLead(p string) string {
	t := p + "team."
	return fmt.Sprintf("((%s && %spersonal = false) || (%steam_members_via_team.user ?= @request.auth.id && %steam_members_via_team.role ?= 'lead'))", leadRole, t, t, t)
}

// teamMember: lo mismo, con cualquier fila del equipo.
func teamMember(p string) string {
	t := p + "team."
	return fmt.Sprintf("((%s && %spersonal = false) || %steam_members_via_team.user ?= @request.auth.id)", leadRole, t, t)
}

// Colecciones cuyo acceso cuelga de un proyecto. Donde la regla decía «lead
// global», ahora dice «lead del equipo del proyecto».
var wsScoped = []string{"workspaces", "memberships", "states", "labels", "bubbles", "threads",
	"thread_links", "thread_relations", "comments", "events", "thread_evidence", "bubble_priority",
	"workspace_repos", "personal_notes", "personal_assets"}

var memberships = regexp.MustCompile(`([a-z_.]*)memberships_via_workspace`)

// wsPath es la ruta al proyecto que la regla ya usa: "" en `workspaces`,
// "workspace." o "thread.workspace." en lo demás.
func wsPath(rule string) string {
	if mm := memberships.FindStringSubmatch(rule); mm != nil {
		return mm[1]
	}
	return ""
}

func rewrite(rule *string, path string) *string {
	if rule == nil {
		return nil
	}
	return types.Pointer(strings.ReplaceAll(*rule, leadRole, teamLead(path)))
}

func unwrite(rule *string, path string) *string {
	if rule == nil {
		return nil
	}
	return types.Pointer(strings.ReplaceAll(*rule, teamLead(path), leadRole))
}

func personalName(u *core.Record) string {
	for _, k := range []string{"display_name", "name"} {
		if n := strings.TrimSpace(u.GetString(k)); n != "" {
			return n
		}
	}
	return strings.SplitN(u.GetString("email"), "@", 2)[0]
}

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		teams := core.NewBaseCollection("teams")
		teams.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 80})
		teams.Fields.Add(&core.BoolField{Name: "personal"})
		teams.Fields.Add(&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1})
		teams.Fields.Add(&core.NumberField{Name: "position", OnlyInt: true})
		teams.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		teams.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		teams.AddIndex("idx_teams_name", true, "name", "")
		if err := app.Save(teams); err != nil {
			return err
		}

		members := core.NewBaseCollection("team_members")
		members.Fields.Add(&core.RelationField{Name: "team", CollectionId: teams.Id, Required: true, MaxSelect: 1, CascadeDelete: true})
		members.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true})
		members.Fields.Add(&core.SelectField{Name: "role", Required: true, MaxSelect: 1, Values: []string{"lead", "member"}})
		members.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		members.AddIndex("idx_team_members_pair", true, "team, user", "")
		members.AddIndex("idx_team_members_user", false, "user", "")
		if err := app.Save(members); err != nil {
			return err
		}
		// Quién ve un equipo: el lead global (si no es personal) y quien está en
		// él. Escribir es del servidor (rutas `/api/teams`), que valida.
		see := fmt.Sprintf("(%s && personal = false) || team_members_via_team.user ?= @request.auth.id", leadRole)
		teams.ListRule, teams.ViewRule = types.Pointer(see), types.Pointer(see)
		if err := app.Save(teams); err != nil {
			return err
		}
		seeMembers := fmt.Sprintf("user = @request.auth.id || (%s && team.personal = false) || team.team_members_via_team.user ?= @request.auth.id", leadRole)
		members.ListRule, members.ViewRule = types.Pointer(seeMembers), types.Pointer(seeMembers)
		if err := app.Save(members); err != nil {
			return err
		}

		// El equipo de todo lo que ya existe.
		ti := core.NewRecord(teams)
		ti.Set("name", "TI")
		ti.Set("position", 0)
		if err := app.Save(ti); err != nil {
			return err
		}
		people, err := app.FindAllRecords("users")
		if err != nil {
			return err
		}
		for i, u := range people {
			if u.GetString("deleted_at") != "" {
				continue
			}
			row := core.NewRecord(members)
			row.Set("team", ti.Id)
			row.Set("user", u.Id)
			role := "member"
			if u.GetString("role") == "lead" {
				role = "lead"
			}
			row.Set("role", role)
			if err := app.Save(row); err != nil {
				return err
			}
			// Cada lead global nace con su contexto privado. (El servidor hace lo
			// mismo con un lead nuevo o con alguien que pasa a serlo.)
			if role != "lead" {
				continue
			}
			name := "Personal · " + personalName(u)
			if n, _ := app.CountRecords("teams", dbx.HashExp{"name": name}); n > 0 {
				name = fmt.Sprintf("%s (%s)", name, strings.SplitN(u.GetString("email"), "@", 2)[0])
			}
			own := core.NewRecord(teams)
			own.Set("name", name)
			own.Set("personal", true)
			own.Set("owner", u.Id)
			own.Set("position", i+1)
			if err := app.Save(own); err != nil {
				return err
			}
			lead := core.NewRecord(members)
			lead.Set("team", own.Id)
			lead.Set("user", u.Id)
			lead.Set("role", "lead")
			if err := app.Save(lead); err != nil {
				return err
			}
		}

		// `team` en lo que es de un equipo, relleno con TI.
		for _, name := range []string{"workspaces", "objectives", "calendar_events", "inbox_items", "channels"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.Fields.Add(&core.RelationField{Name: "team", CollectionId: teams.Id, MaxSelect: 1})
			if err := app.Save(c); err != nil {
				return err
			}
			if _, err := app.DB().NewQuery("UPDATE {{" + name + "}} SET [[team]] = {:t} WHERE [[team]] = '' OR [[team]] IS NULL").
				Bind(map[string]any{"t": ti.Id}).Execute(); err != nil {
				return err
			}
			c, _ = app.FindCollectionByNameOrId(name)
			f := c.Fields.GetByName("team").(*core.RelationField)
			f.Required = true
			switch name {
			case "objectives":
				c.RemoveIndex("idx_objectives_name")
				c.AddIndex("idx_objectives_team_name", true, "team, name", "")
			case "inbox_items":
				c.RemoveIndex("idx_inbox_source_ref")
				c.AddIndex("idx_inbox_source_ref", true, "team, source, source_ref", "source_ref != ''")
				c.AddIndex("idx_inbox_team", false, "team", "")
			case "channels":
				c.RemoveIndex("idx_channels_kind")
				c.AddIndex("idx_channels_team_kind", true, "team, kind", "")
			case "workspaces":
				c.AddIndex("idx_workspaces_team", false, "team", "")
			}
			if err := app.Save(c); err != nil {
				return err
			}
		}

		// Reglas de lo que cuelga de un proyecto.
		for _, name := range wsScoped {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			path := ""
			for _, r := range []*string{c.ListRule, c.ViewRule, c.CreateRule, c.UpdateRule, c.DeleteRule} {
				if r != nil && strings.Contains(*r, "memberships_via_workspace") {
					path = wsPath(*r)
					break
				}
			}
			c.ListRule = rewrite(c.ListRule, path)
			c.ViewRule = rewrite(c.ViewRule, path)
			c.CreateRule = rewrite(c.CreateRule, path)
			c.UpdateRule = rewrite(c.UpdateRule, path)
			c.DeleteRule = rewrite(c.DeleteRule, path)
			if name == "workspaces" {
				// Crear un proyecto: en un equipo del que eres parte. Vacío lo
				// completa el servidor con tu equipo por defecto.
				c.CreateRule = types.Pointer(`@request.auth.id != "" && (team = "" || ` + teamMember("") + `)`)
			}
			if err := app.Save(c); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}

		set := func(name string, l, v, cr, u, d *string) error {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.ListRule, c.ViewRule, c.CreateRule, c.UpdateRule, c.DeleteRule = l, v, cr, u, d
			return app.Save(c)
		}
		lead := types.Pointer(teamLead(""))
		// Crear sin equipo: el servidor pone el equipo por defecto y comprueba
		// que quien crea lo lidera.
		leadOrDefault := types.Pointer(`@request.auth.id != "" && (team = "" || ` + teamLead("") + `)`)
		if err := set("objectives", lead, lead, leadOrDefault, lead, lead); err != nil {
			return err
		}
		member := types.Pointer(teamMember(""))
		if err := set("calendar_events", member, member, leadOrDefault, lead, lead); err != nil {
			return err
		}
		// Una idea: la ve quien la capturó y los leads de su equipo.
		idea := types.Pointer("captured_by = @request.auth.id || " + teamLead(""))
		if err := set("inbox_items", idea, idea,
			types.Pointer(`@request.auth.id != "" && (team = "" || `+teamMember("")+`)`), idea, idea); err != nil {
			return err
		}
		art := types.Pointer("creator = @request.auth.id || " + teamLead("inbox_item."))
		if err := set("artifacts", art, art, nil, nil, nil); err != nil {
			return err
		}
		entry := types.Pointer("artifact.creator = @request.auth.id || " + teamLead("artifact.inbox_item."))
		return set("artifact_entries", entry, entry, nil, nil, nil)
	}, func(app core.App) error {
		for _, name := range wsScoped {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			path := ""
			for _, r := range []*string{c.ListRule, c.ViewRule, c.UpdateRule, c.DeleteRule} {
				if r != nil && strings.Contains(*r, "memberships_via_workspace") {
					path = wsPath(*r)
					break
				}
			}
			c.ListRule = unwrite(c.ListRule, path)
			c.ViewRule = unwrite(c.ViewRule, path)
			c.CreateRule = unwrite(c.CreateRule, path)
			c.UpdateRule = unwrite(c.UpdateRule, path)
			c.DeleteRule = unwrite(c.DeleteRule, path)
			if name == "workspaces" {
				c.CreateRule = types.Pointer(leadRole + ` || (@request.auth.id != "")`)
			}
			if err := app.Save(c); err != nil {
				return err
			}
		}
		lead := types.Pointer(leadRole)
		signed := types.Pointer(`@request.auth.id != ""`)
		restore := map[string][5]*string{
			"objectives":       {lead, lead, lead, lead, lead},
			"calendar_events":  {signed, signed, lead, lead, lead},
			"inbox_items":      {types.Pointer(leadRole + " || captured_by = @request.auth.id"), types.Pointer(leadRole + " || captured_by = @request.auth.id"), signed, types.Pointer(leadRole + " || captured_by = @request.auth.id"), types.Pointer(leadRole + " || captured_by = @request.auth.id")},
			"artifacts":        {types.Pointer("creator = @request.auth.id || " + leadRole), types.Pointer("creator = @request.auth.id || " + leadRole), nil, nil, nil},
			"artifact_entries": {types.Pointer("artifact.creator = @request.auth.id || " + leadRole), types.Pointer("artifact.creator = @request.auth.id || " + leadRole), nil, nil, nil},
		}
		for name, r := range restore {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.ListRule, c.ViewRule, c.CreateRule, c.UpdateRule, c.DeleteRule = r[0], r[1], r[2], r[3], r[4]
			if err := app.Save(c); err != nil {
				return err
			}
		}
		for _, name := range []string{"workspaces", "objectives", "calendar_events", "inbox_items", "channels"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			switch name {
			case "objectives":
				c.RemoveIndex("idx_objectives_team_name")
				c.AddIndex("idx_objectives_name", true, "name", "")
			case "inbox_items":
				c.RemoveIndex("idx_inbox_source_ref")
				c.RemoveIndex("idx_inbox_team")
				c.AddIndex("idx_inbox_source_ref", true, "source, source_ref", "source_ref != ''")
			case "channels":
				c.RemoveIndex("idx_channels_team_kind")
				c.AddIndex("idx_channels_kind", true, "kind", "")
			case "workspaces":
				c.RemoveIndex("idx_workspaces_team")
			}
			c.Fields.RemoveByName("team")
			if err := app.Save(c); err != nil {
				return err
			}
		}
		for _, name := range []string{"team_members", "teams"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}
