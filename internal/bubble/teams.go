package bubble

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// Equipos: la frontera del CONTENIDO.
//
// Un equipo agrupa proyectos y tiene sus leads. El rol `lead` de una persona
// es de ADMINISTRACIÓN (personas, tokens, prompts, inventario) y además la hace
// lead implícita de todo equipo que no sea personal. Un equipo personal sólo lo
// ve quien está en él: ni otro lead global. Invitar a alguien a un proyecto
// (membresía de proyecto) le enseña ese proyecto y nada más del equipo.
//
// Las mismas preguntas que hacen las reglas de las colecciones
// (migrations/1788532000_teams.go), hechas aquí para las puertas que no pasan
// por ellas: MCP, rutas propias y `app.Save`.

// TeamView es un equipo como lo ve quien pregunta.
type TeamView struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Personal bool         `json:"personal"`
	Owner    string       `json:"owner,omitempty"`
	Role     string       `json:"role"`               // lead | member: el tuyo
	Implicit bool         `json:"implicit,omitempty"` // lead por ser lead global
	Members  []TeamMember `json:"members,omitempty"`
}

// TeamMember es una fila de un equipo. `global` dice que es lead global: lo es
// de todo equipo compartido aunque su fila diga otra cosa.
type TeamMember struct {
	User   string `json:"user"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Global bool   `json:"global,omitempty"`
}

func teamRow(app core.App, teamID, userID string) *core.Record {
	if teamID == "" || userID == "" {
		return nil
	}
	r, err := app.FindFirstRecordByFilter("team_members", "team = {:t} && user = {:u}",
		dbx.Params{"t": teamID, "u": userID})
	if err != nil {
		return nil
	}
	return r
}

// LeadsTeam: el lead global si el equipo no es personal, o una fila de lead.
func LeadsTeam(app core.App, auth *core.Record, teamID string) bool {
	if IsSuperuser(auth) {
		return true
	}
	if !isPersonAuth(auth) || teamID == "" {
		return false
	}
	t, err := app.FindRecordById("teams", teamID)
	if err != nil {
		return false
	}
	if isGlobalLead(auth) && !t.GetBool("personal") {
		return true
	}
	r := teamRow(app, teamID, auth.Id)
	return r != nil && r.GetString("role") == "lead"
}

// InTeam: lead del equipo, o cualquier fila en él.
func InTeam(app core.App, auth *core.Record, teamID string) bool {
	return LeadsTeam(app, auth, teamID) || (isPersonAuth(auth) && teamRow(app, teamID, auth.Id) != nil)
}

// TeamRef es el contexto de algo, como lo lee un agente: qué equipo y si es
// personal (privado).
type TeamRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Personal bool   `json:"personal"`
}

// TeamRefOf describe un equipo por su id; vacío si no existe.
func TeamRefOf(app core.App, teamID string) TeamRef {
	t, err := app.FindRecordById("teams", teamID)
	if err != nil {
		return TeamRef{ID: teamID}
	}
	return TeamRef{ID: t.Id, Name: t.GetString("name"), Personal: t.GetBool("personal")}
}

// WorkspaceTeam es el contexto de un proyecto.
func WorkspaceTeam(app core.App, wsID string) TeamRef {
	return TeamRefOf(app, teamOfWorkspace(app, wsID))
}

// teamOfWorkspace es el equipo de un proyecto.
func teamOfWorkspace(app core.App, wsID string) string {
	ws, err := app.FindRecordById("workspaces", wsID)
	if err != nil {
		return ""
	}
	return ws.GetString("team")
}

// LeadsWorkspaceTeam: ¿lideras el equipo de este proyecto?
func LeadsWorkspaceTeam(app core.App, auth *core.Record, wsID string) bool {
	return LeadsTeam(app, auth, teamOfWorkspace(app, wsID))
}

// TeamsOf son los equipos que la persona ve, con su papel en cada uno: los
// compartidos primero, los personales al final.
func TeamsOf(app core.App, auth *core.Record) ([]TeamView, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	rows, err := app.FindRecordsByFilter("teams", "", "position,name", 0, 0)
	if err != nil {
		return nil, err
	}
	out := []TeamView{}
	for _, t := range rows {
		v := TeamView{ID: t.Id, Name: t.GetString("name"), Personal: t.GetBool("personal"), Owner: t.GetString("owner")}
		if r := teamRow(app, t.Id, auth.Id); r != nil {
			v.Role = r.GetString("role")
		}
		if v.Role != "lead" && isGlobalLead(auth) && !v.Personal {
			v.Role, v.Implicit = "lead", true
		}
		if v.Role == "" {
			continue
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return !out[i].Personal && out[j].Personal })
	return out, nil
}

// LedTeams son los ids de los equipos que la persona lidera.
func LedTeams(app core.App, auth *core.Record) []string {
	ts, _ := TeamsOf(app, auth)
	out := []string{}
	for _, t := range ts {
		if t.Role == "lead" {
			out = append(out, t.ID)
		}
	}
	return out
}

// DefaultTeam es a dónde va lo que no dice equipo: el que la persona eligió en
// la aplicación (`prefs.team`), o su primer equipo compartido, o el personal.
func DefaultTeam(app core.App, auth *core.Record) string {
	if !isPersonAuth(auth) {
		return ""
	}
	if _, value, err := prefsOf(app, auth); err == nil {
		if t, ok := value["team"].(string); ok && t != "" && InTeam(app, auth, t) {
			return t
		}
	}
	ts, _ := TeamsOf(app, auth)
	for _, t := range ts {
		if !t.Implicit || len(ts) == 1 {
			return t.ID
		}
	}
	if len(ts) > 0 {
		return ts[0].ID
	}
	return ""
}

// ResolveTeam acepta el id o el nombre de un equipo que la persona ve; vacío
// es su equipo por defecto.
func ResolveTeam(app core.App, auth *core.Record, idOrName string) (string, error) {
	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		if t := DefaultTeam(app, auth); t != "" {
			return t, nil
		}
		return "", fmt.Errorf("no estás en ningún equipo: pide a un lead que te agregue")
	}
	ts, err := TeamsOf(app, auth)
	if err != nil {
		return "", err
	}
	names := []string{}
	for _, t := range ts {
		if t.ID == idOrName || strings.EqualFold(t.Name, idOrName) {
			return t.ID, nil
		}
		names = append(names, t.Name)
	}
	return "", fmt.Errorf("no ves ningún equipo %q; los tuyos: %s", idOrName, strings.Join(names, ", "))
}

// TeamMembersOf es quién está en un equipo (las filas explícitas).
func TeamMembersOf(app core.App, teamID string) []TeamMember { return teamMembersOf(app, teamID) }

func teamMembersOf(app core.App, teamID string) []TeamMember {
	rows, _ := app.FindRecordsByFilter("team_members", "team = {:t}", "created", 0, 0, dbx.Params{"t": teamID})
	out := []TeamMember{}
	for _, r := range rows {
		name, global := "", false
		if u, err := app.FindRecordById("users", r.GetString("user")); err == nil {
			name, global = personName(u), u.GetString("role") == "lead"
		}
		out = append(out, TeamMember{User: r.GetString("user"), Name: name, Role: r.GetString("role"), Global: global})
	}
	return out
}

// ---- hooks ---------------------------------------------------------------------

// Un proyecto nace en un equipo del que eres parte; vacío, en el tuyo por
// defecto. Moverlo de equipo es de un lead global que lidere los dos.
func workspaceTeamOnCreate(e *core.RecordRequestEvent) error {
	if !isPersonAuth(e.Auth) {
		return e.Next()
	}
	t := e.Record.GetString("team")
	if t == "" {
		t = DefaultTeam(e.App, e.Auth)
		e.Record.Set("team", t)
	}
	if t == "" || !InTeam(e.App, e.Auth, t) {
		return e.BadRequestError("un proyecto nace en un equipo del que eres parte", nil)
	}
	return e.Next()
}

func workspaceTeamOnUpdate(e *core.RecordRequestEvent) error {
	if !isPersonAuth(e.Auth) {
		return e.Next()
	}
	was := e.Record.Original().GetString("team")
	now := e.Record.GetString("team")
	if was != now {
		if !isGlobalLead(e.Auth) || !LeadsTeam(e.App, e.Auth, was) || !LeadsTeam(e.App, e.Auth, now) {
			return e.ForbiddenError("mover un proyecto de equipo es de un lead global que lidere los dos equipos", nil)
		}
	}
	return e.Next()
}

// Una idea va a un equipo del que eres parte; vacía, al tuyo por defecto.
func ideaTeamOnCreate(e *core.RecordRequestEvent) error {
	if !isPersonAuth(e.Auth) {
		return e.Next()
	}
	t := e.Record.GetString("team")
	if t == "" {
		t = DefaultTeam(e.App, e.Auth)
		e.Record.Set("team", t)
	}
	if t == "" || !InTeam(e.App, e.Auth, t) {
		return e.BadRequestError("una idea va al inbox de un equipo del que eres parte", nil)
	}
	return e.Next()
}

// Un objetivo o una fecha del calendario son de un equipo y los pone un lead
// de ese equipo; sin equipo, el de por defecto.
func ledTeamOnCreate(what string) func(e *core.RecordRequestEvent) error {
	return func(e *core.RecordRequestEvent) error {
		if !isPersonAuth(e.Auth) {
			return e.Next()
		}
		t := e.Record.GetString("team")
		if t == "" {
			t = DefaultTeam(e.App, e.Auth)
			e.Record.Set("team", t)
		}
		if t == "" || !LeadsTeam(e.App, e.Auth, t) {
			return e.ForbiddenError(what+" lo pone un lead del equipo", nil)
		}
		return e.Next()
	}
}

// ---- rutas -----------------------------------------------------------------------

// Un módulo cuelga de un objetivo de SU equipo: el de su proyecto.
func bubbleObjectiveSameTeam(e *core.RecordRequestEvent) error {
	if o := e.Record.GetString("objective"); o != "" {
		obj, err := e.App.FindRecordById("objectives", o)
		if err != nil || obj.GetString("team") != teamOfWorkspace(e.App, e.Record.GetString("workspace")) {
			return e.BadRequestError("un módulo cuelga de un objetivo del equipo de su proyecto", nil)
		}
	}
	return e.Next()
}

// Una persona nueva llega al primer equipo compartido (el de siempre, TI)
// como miembro: sin equipo no podría ni capturar una idea. A nivel de
// registro, no de petición, para que también lo haga `bubble person`.
func joinDefaultTeam(e *core.RecordEvent) error {
	if err := e.Next(); err != nil {
		return err
	}
	if e.Record.GetString("deleted_at") != "" {
		return nil
	}
	t, err := e.App.FindFirstRecordByFilter("teams", "personal = false", dbx.Params{})
	if err != nil {
		return nil
	}
	rows, _ := e.App.FindRecordsByFilter("teams", "personal = false", "position,created", 1, 0)
	if len(rows) > 0 {
		t = rows[0]
	}
	if teamRow(e.App, t.Id, e.Record.Id) != nil {
		return nil
	}
	mc, err := e.App.FindCollectionByNameOrId("team_members")
	if err != nil {
		return nil
	}
	row := core.NewRecord(mc)
	row.Set("team", t.Id)
	row.Set("user", e.Record.Id)
	role := "member"
	if e.Record.GetString("role") == "lead" {
		role = "lead"
	}
	row.Set("role", role)
	if err := e.App.Save(row); err != nil {
		e.App.Logger().Error("no se pudo sumar a la persona a su equipo", "user", e.Record.Id, "error", err)
	}
	ensurePersonal(e.App, e.Record)
	return nil
}

// ensurePersonal: todo lead global tiene su equipo personal, su contexto
// privado. Se crea al darlo de alta como lead o al ascenderlo; quitarle el rol
// no se lo quita (sigue siendo suyo).
func ensurePersonal(app core.App, u *core.Record) {
	if u.GetString("role") != "lead" || u.GetString("deleted_at") != "" {
		return
	}
	if _, err := app.FindFirstRecordByFilter("teams", "personal = true && owner = {:o}", dbx.Params{"o": u.Id}); err == nil {
		return
	}
	if _, err := CreateTeam(app, u, "", true); err != nil {
		app.Logger().Error("no se pudo crear el equipo personal", "user", u.Id, "error", err)
	}
}

func personalOnPromote(e *core.RecordEvent) error {
	if err := e.Next(); err != nil {
		return err
	}
	ensurePersonal(e.App, e.Record)
	return nil
}

func registerTeams(app core.App) {
	app.OnRecordAfterCreateSuccess("users").BindFunc(joinDefaultTeam)
	app.OnRecordAfterUpdateSuccess("users").BindFunc(personalOnPromote)
	// Asignar es invitar (ver joinAssigned).
	app.OnRecordCreateRequest("bubbles").BindFunc(assignedOnRequest("owners"))
	app.OnRecordUpdateRequest("bubbles").BindFunc(assignedOnRequest("owners"))
	app.OnRecordCreateRequest("threads").BindFunc(assignedOnRequest("assignees"))
	app.OnRecordUpdateRequest("threads").BindFunc(assignedOnRequest("assignees"))
	app.OnRecordCreateRequest("bubbles").BindFunc(bubbleObjectiveSameTeam)
	app.OnRecordUpdateRequest("bubbles").BindFunc(bubbleObjectiveSameTeam)
	app.OnRecordCreateRequest("workspaces").BindFunc(workspaceTeamOnCreate)
	app.OnRecordUpdateRequest("workspaces").BindFunc(workspaceTeamOnUpdate)
	app.OnRecordCreateRequest("inbox_items").BindFunc(ideaTeamOnCreate)
	app.OnRecordCreateRequest("objectives").BindFunc(ledTeamOnCreate("un objetivo"))
	app.OnRecordCreateRequest("calendar_events").BindFunc(ledTeamOnCreate("una fecha del calendario"))

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		person := apis.RequireAuth("users")

		se.Router.GET("/api/teams", func(e *core.RequestEvent) error {
			ts, err := TeamsOf(e.App, e.Auth)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			for i := range ts {
				ts[i].Members = teamMembersOf(e.App, ts[i].ID)
			}
			return e.JSON(http.StatusOK, map[string]any{"teams": ts, "default": DefaultTeam(e.App, e.Auth)})
		}).Bind(person)

		// Un equipo compartido lo crea un admin (lead global).
		se.Router.POST("/api/teams", func(e *core.RequestEvent) error {
			if !isGlobalLead(e.Auth) {
				return e.ForbiddenError("un equipo lo crea un lead global", nil)
			}
			var body struct {
				Name string `json:"name"`
			}
			if err := e.BindBody(&body); err != nil || strings.TrimSpace(body.Name) == "" {
				return e.BadRequestError("un equipo necesita un nombre", err)
			}
			t, err := CreateTeam(e.App, e.Auth, body.Name, false)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"id": t.Id, "name": t.GetString("name")})
		}).Bind(person)

		// Mi equipo personal: uno por lead global.
		se.Router.POST("/api/teams/personal", func(e *core.RequestEvent) error {
			if !isGlobalLead(e.Auth) {
				return e.ForbiddenError("un equipo personal es de un lead global", nil)
			}
			var body struct {
				Name string `json:"name"`
			}
			_ = e.BindBody(&body)
			t, err := CreateTeam(e.App, e.Auth, body.Name, true)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"id": t.Id, "name": t.GetString("name")})
		}).Bind(person)

		se.Router.PATCH("/api/teams/{id}", func(e *core.RequestEvent) error {
			id := e.Request.PathValue("id")
			if !LeadsTeam(e.App, e.Auth, id) {
				return e.NotFoundError("", nil)
			}
			var body struct {
				Name string `json:"name"`
			}
			if err := e.BindBody(&body); err != nil || strings.TrimSpace(body.Name) == "" {
				return e.BadRequestError("un equipo necesita un nombre", err)
			}
			t, err := e.App.FindRecordById("teams", id)
			if err != nil {
				return e.NotFoundError("", err)
			}
			t.Set("name", strings.TrimSpace(body.Name))
			if err := e.App.Save(t); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"id": t.Id, "name": t.GetString("name")})
		}).Bind(person)

		se.Router.DELETE("/api/teams/{id}", func(e *core.RequestEvent) error {
			id := e.Request.PathValue("id")
			t, err := e.App.FindRecordById("teams", id)
			if err != nil || !LeadsTeam(e.App, e.Auth, id) {
				return e.NotFoundError("", nil)
			}
			if !t.GetBool("personal") && !isGlobalLead(e.Auth) {
				return e.ForbiddenError("un equipo compartido lo borra un lead global", nil)
			}
			var n int
			_ = e.App.DB().NewQuery("SELECT COUNT(*) FROM workspaces WHERE team = {:t}").Bind(dbx.Params{"t": id}).Row(&n)
			if n > 0 {
				return e.BadRequestError(fmt.Sprintf("el equipo tiene %d proyectos: muévelos o bórralos primero", n), nil)
			}
			if err := e.App.Delete(t); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.NoContent(http.StatusNoContent)
		}).Bind(person)

		// Quién está en un equipo. Lo administra un lead del equipo; en uno
		// personal no entra nadie más: a otra persona se la invita a un
		// PROYECTO, no al equipo.
		se.Router.POST("/api/teams/{id}/members", func(e *core.RequestEvent) error {
			id := e.Request.PathValue("id")
			var body struct {
				User string `json:"user"`
				Role string `json:"role"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.BadRequestError("no se pudo leer", err)
			}
			if err := SetTeamMember(e.App, e.Auth, id, body.User, body.Role); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"members": teamMembersOf(e.App, id)})
		}).Bind(person)

		se.Router.DELETE("/api/teams/{id}/members/{user}", func(e *core.RequestEvent) error {
			id := e.Request.PathValue("id")
			if err := SetTeamMember(e.App, e.Auth, id, e.Request.PathValue("user"), ""); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"members": teamMembersOf(e.App, id)})
		}).Bind(person)
		return se.Next()
	})
}

// CreateTeam crea un equipo. Uno personal queda a nombre de quien lo crea, que
// es su lead; sólo puede haber uno por persona.
func CreateTeam(app core.App, auth *core.Record, name string, personal bool) (*core.Record, error) {
	name = strings.TrimSpace(name)
	if personal {
		if _, err := app.FindFirstRecordByFilter("teams", "personal = true && owner = {:o}", dbx.Params{"o": auth.Id}); err == nil {
			return nil, fmt.Errorf("ya tienes un equipo personal")
		}
		if name == "" {
			name = "Personal · " + personName(auth)
			if n, _ := app.CountRecords("teams", dbx.HashExp{"name": name}); n > 0 {
				name = fmt.Sprintf("%s (%s)", name, strings.SplitN(auth.GetString("email"), "@", 2)[0])
			}
		}
	}
	if name == "" {
		return nil, fmt.Errorf("un equipo necesita un nombre")
	}
	col, err := app.FindCollectionByNameOrId("teams")
	if err != nil {
		return nil, err
	}
	t := core.NewRecord(col)
	t.Set("name", name)
	t.Set("personal", personal)
	if personal {
		t.Set("owner", auth.Id)
	}
	var max int
	_ = app.DB().NewQuery("SELECT COALESCE(MAX(position),0) FROM teams").Row(&max)
	t.Set("position", max+1)
	err = app.RunInTransaction(func(tx core.App) error {
		if err := tx.Save(t); err != nil {
			return err
		}
		if !personal {
			return nil
		}
		mc, err := tx.FindCollectionByNameOrId("team_members")
		if err != nil {
			return err
		}
		row := core.NewRecord(mc)
		row.Set("team", t.Id)
		row.Set("user", auth.Id)
		row.Set("role", "lead")
		return tx.Save(row)
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// SetTeamMember pone (role lead|member) o quita (role vacío) a alguien de un
// equipo. Lo hace un lead del equipo. En uno personal no entra nadie más.
func SetTeamMember(app core.App, auth *core.Record, teamID, user, role string) error {
	t, err := app.FindRecordById("teams", teamID)
	if err != nil || !LeadsTeam(app, auth, teamID) {
		return ErrDenied
	}
	if t.GetBool("personal") {
		return fmt.Errorf("a un equipo personal no entra nadie más: invita a la persona a un PROYECTO del equipo")
	}
	row := teamRow(app, teamID, user)
	if role == "" {
		if row == nil {
			return nil
		}
		return app.Delete(row)
	}
	if role != "lead" && role != "member" {
		return fmt.Errorf("el papel en un equipo es lead o member, no %q", role)
	}
	if _, err := activePeople(app, []string{user}); err != nil {
		return err
	}
	if row == nil {
		mc, err := app.FindCollectionByNameOrId("team_members")
		if err != nil {
			return err
		}
		row = core.NewRecord(mc)
		row.Set("team", teamID)
		row.Set("user", user)
	}
	row.Set("role", role)
	return app.Save(row)
}
