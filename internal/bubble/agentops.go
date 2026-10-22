package bubble

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Lo que un agente puede hacer además de escribir documentos: el inbox de
// ideas, Mi espacio, los pines, repositorios e inventario, personas, cambios
// del departamento, y los campos de un hilo que no son su nombre.
//
// Mismas reglas que la aplicación: `app.Save` no conoce las reglas de las
// colecciones, así que cada función dice qué regla refleja y la comprueba aquí.
// La ADMINISTRACIÓN (personas, roles, tokens, membresías, canales, acceso al
// inventario, prompts) se queda en la aplicación, a mano, a propósito.

// ---- personas ------------------------------------------------------------------

// Person es alguien del departamento, como lo necesita un agente para asignar o
// poner responsables: su id, su nombre y su rol. Sin correo: no hace falta para
// nada de eso y no tiene por qué viajar.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
	Me   bool   `json:"me,omitempty"`
}

func personName(u *core.Record) string {
	for _, k := range []string{"display_name", "name"} {
		if n := strings.TrimSpace(u.GetString(k)); n != "" {
			return n
		}
	}
	return "(sin nombre)"
}

// People lista a las personas activas. Refleja `users.ListRule`: cualquiera con
// sesión. Con `project`, sólo quienes ven ese proyecto (a quién tiene sentido
// asignar o poner de responsable); con `team`, quienes están en ese equipo.
func People(app core.App, auth *core.Record, project, team string) ([]Person, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	wsID := ""
	if project != "" {
		ws, _, err := WorkspaceFor(app, auth, project)
		if err != nil {
			return nil, err
		}
		wsID = ws.Id
	}
	teamID := ""
	if team != "" {
		id, err := ResolveTeam(app, auth, team)
		if err != nil {
			return nil, err
		}
		teamID = id
	}
	rows, err := app.FindRecordsByFilter("users", "deleted_at = ''", "display_name", 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Person, 0, len(rows))
	for _, u := range rows {
		if wsID != "" && !canSee(app, u, wsID) {
			continue
		}
		if teamID != "" && !InTeam(app, u, teamID) {
			continue
		}
		out = append(out, Person{ID: u.Id, Name: personName(u), Role: u.GetString("role"), Me: u.Id == auth.Id})
	}
	return out, nil
}

// joinAssigned: asignar es invitar. Quien queda de responsable de un módulo o
// asignado a un hilo y no ve el proyecto se suma a él como miembro (ve ESE
// proyecto, nada más del equipo), siempre que quien asigna pueda invitar: un
// lead del proyecto o de su equipo. Si no puede, se rechaza: asignar no es una
// puerta para saltarse quién invita. Devuelve a quién sumó.
func joinAssigned(app core.App, auth *core.Record, ids []string, wsID string) ([]string, error) {
	joined := []string{}
	var col *core.Collection
	for _, id := range ids {
		u, err := app.FindRecordById("users", id)
		if err != nil || canSee(app, u, wsID) {
			continue
		}
		if IsSuperuser(auth) || !leadsWorkspace(app, auth, wsID) {
			return nil, fmt.Errorf("%s no está en este proyecto, y sólo un lead del proyecto o de su equipo puede invitarlo: pídele que lo invite", personName(u))
		}
		if col == nil {
			if col, err = app.FindCollectionByNameOrId("memberships"); err != nil {
				return nil, err
			}
		}
		m := core.NewRecord(col)
		m.Set("workspace", wsID)
		m.Set("user", id)
		m.Set("role", "member")
		if err := app.Save(m); err != nil {
			return nil, err
		}
		joined = append(joined, personName(u))
	}
	return joined, nil
}

// assignedOnRequest hace lo mismo por la API REST (la interfaz): un campo de
// personas (`owners` de un módulo, `assignees` de un hilo) que trae a alguien
// que no ve el proyecto lo suma, en la misma transacción que la escritura.
func assignedOnRequest(field string) func(e *core.RecordRequestEvent) error {
	return func(e *core.RecordRequestEvent) error {
		if !isPersonAuth(e.Auth) {
			return e.Next()
		}
		before := map[string]bool{}
		if !e.Record.IsNew() {
			for _, id := range e.Record.Original().GetStringSlice(field) {
				before[id] = true
			}
		}
		fresh := []string{}
		for _, id := range e.Record.GetStringSlice(field) {
			if !before[id] {
				fresh = append(fresh, id)
			}
		}
		if len(fresh) == 0 {
			return e.Next()
		}
		return e.App.RunInTransaction(func(tx core.App) error {
			defer swapApp(e, tx)()
			if _, err := joinAssigned(tx, e.Auth, fresh, e.Record.GetString("workspace")); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.Next()
		})
	}
}

func peopleNamed(app core.App, ids []string) []Person {
	out := []Person{}
	for _, id := range ids {
		if u, err := app.FindRecordById("users", id); err == nil {
			out = append(out, Person{ID: u.Id, Name: personName(u), Role: u.GetString("role")})
		}
	}
	return out
}

// activePeople comprueba que cada id sea una persona activa.
func activePeople(app core.App, ids []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		u, err := app.FindRecordById("users", id)
		if err != nil || u.GetString("deleted_at") != "" {
			return nil, fmt.Errorf("no hay ninguna persona activa con id %q (people_list las dice)", id)
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// ---- inbox: ideas, no trabajo ---------------------------------------------------

// Idea es una nota del inbox. Es una IDEA: algo que alguien capturó y nadie ha
// decidido. No es un hilo ni un módulo, y no se ejecuta hasta que una persona
// decide volverla trabajo.
type Idea struct {
	ID         string   `json:"id"`
	Team       string   `json:"team"`
	TeamName   string   `json:"team_name"`
	Note       string   `json:"note"`
	Body       string   `json:"body,omitempty"`
	Source     string   `json:"source"`
	From       string   `json:"from,omitempty"`
	CapturedBy string   `json:"captured_by"`
	Created    string   `json:"created"`
	Thread     string   `json:"thread,omitempty"`
	Bubble     string   `json:"bubble,omitempty"`
	Decided    bool     `json:"decided"`
	Files      []string `json:"files,omitempty"`
}

func ideaOf(app core.App, r *core.Record, full bool) Idea {
	src := r.GetString("source")
	if src == "" {
		src = "app"
	}
	by := ""
	if u, err := app.FindRecordById("users", r.GetString("captured_by")); err == nil {
		by = personName(u)
	}
	team := ""
	if t, err := app.FindRecordById("teams", r.GetString("team")); err == nil {
		team = t.GetString("name")
	}
	v := Idea{
		ID: r.Id, Team: r.GetString("team"), TeamName: team, Note: r.GetString("note"), Source: src, From: r.GetString("source_from"),
		CapturedBy: by, Created: r.GetDateTime("created").String(),
		Thread: r.GetString("thread"), Bubble: r.GetString("bubble"),
	}
	v.Decided = v.Thread != "" || v.Bubble != ""
	if full {
		v.Body = r.GetString("body")
		v.Files = r.GetStringSlice("files")
	}
	return v
}

// ideaFor refleja `inbox_items.ViewRule`: quien la capturó, o un lead de su
// equipo. Lo que entra por el WhatsApp de un equipo lo captura quien encendió
// ese canal, y es de ese equipo.
func ideaFor(app core.App, auth *core.Record, id string) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	r, err := app.FindRecordById("inbox_items", id)
	if err != nil || (r.GetString("captured_by") != auth.Id && !LeadsTeam(app, auth, r.GetString("team"))) {
		return nil, ErrDenied
	}
	return r, nil
}

// Ideas lista el inbox que la persona puede ver: lo suyo, y lo de los equipos
// que lidera. `team` limita a un equipo; `source` filtra (`whatsapp`, `app`);
// `decided` incluye las que ya se volvieron trabajo.
func Ideas(app core.App, auth *core.Record, team, source string, decided bool) ([]Idea, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	params := dbx.Params{"me": auth.Id}
	scope := []string{"captured_by = {:me}"}
	for i, t := range LedTeams(app, auth) {
		k := fmt.Sprintf("t%d", i)
		params[k] = t
		scope = append(scope, "team = {:"+k+"}")
	}
	where := []string{"(" + strings.Join(scope, " || ") + ")"}
	if team != "" {
		id, err := ResolveTeam(app, auth, team)
		if err != nil {
			return nil, err
		}
		params["team"] = id
		where = append(where, "team = {:team}")
	}
	switch source {
	case "":
	case "app":
		where = append(where, "source = ''")
	default:
		where = append(where, "source = {:src}")
		params["src"] = source
	}
	if !decided {
		where = append(where, "thread = '' && bubble = ''")
	}
	rows, err := app.FindRecordsByFilter("inbox_items", strings.Join(where, " && "), "-created", 200, 0, params)
	if err != nil {
		return nil, err
	}
	out := make([]Idea, 0, len(rows))
	for _, r := range rows {
		out = append(out, ideaOf(app, r, false))
	}
	return out, nil
}

// IdeaRead trae una idea completa: su texto, su cuerpo y sus archivos.
func IdeaRead(app core.App, auth *core.Record, id string) (Idea, error) {
	r, err := ideaFor(app, auth, id)
	if err != nil {
		return Idea{}, err
	}
	return ideaOf(app, r, true), nil
}

// IdeaCapture anota una idea en el inbox de un equipo del que la persona es
// parte (vacío: su equipo por defecto). Refleja `inbox_items.CreateRule` y los
// hooks que firman `captured_by` y completan `team`.
func IdeaCapture(app core.App, auth *core.Record, team, note, body string) (Idea, error) {
	if !isPersonAuth(auth) {
		return Idea{}, ErrNotAPerson
	}
	teamID, err := ResolveTeam(app, auth, team)
	if err != nil {
		return Idea{}, err
	}
	if !InTeam(app, auth, teamID) {
		return Idea{}, fmt.Errorf("una idea va al inbox de un equipo del que eres parte")
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return Idea{}, fmt.Errorf("una idea necesita un texto (`note`)")
	}
	col, err := app.FindCollectionByNameOrId("inbox_items")
	if err != nil {
		return Idea{}, err
	}
	r := core.NewRecord(col)
	r.Set("note", note)
	r.Set("body", body)
	r.Set("captured_by", auth.Id)
	r.Set("team", teamID)
	if err := app.Save(r); err != nil {
		return Idea{}, err
	}
	return ideaOf(app, r, true), nil
}

// IdeaUpdate corrige el texto de una idea. Refleja `inbox_items.UpdateRule`.
func IdeaUpdate(app core.App, auth *core.Record, id string, note, body *string) (Idea, error) {
	r, err := ideaFor(app, auth, id)
	if err != nil {
		return Idea{}, err
	}
	if note != nil {
		if strings.TrimSpace(*note) == "" {
			return Idea{}, fmt.Errorf("una idea necesita un texto (`note`)")
		}
		r.Set("note", strings.TrimSpace(*note))
	}
	if body != nil {
		r.Set("body", *body)
	}
	if err := app.Save(r); err != nil {
		return Idea{}, err
	}
	return ideaOf(app, r, true), nil
}

// IdeaDecide anota a qué trabajo fue una idea, como hace el triaje de la
// aplicación: la nota no se borra, apunta al hilo o al módulo. Vacío las deja
// sin decidir otra vez.
func IdeaDecide(app core.App, auth *core.Record, id, thread, bubble string) (Idea, error) {
	r, err := ideaFor(app, auth, id)
	if err != nil {
		return Idea{}, err
	}
	// Una idea se vuelve trabajo en SU equipo: el contexto en el que nació.
	same := func(wsTeam string) error {
		if wsTeam != r.GetString("team") {
			return fmt.Errorf("esa idea es de otro equipo: se vuelve trabajo en un proyecto de su propio equipo")
		}
		return nil
	}
	if thread != "" {
		_, ws, _, err := ThreadFor(app, auth, thread)
		if err != nil {
			return Idea{}, err
		}
		if err := same(ws.GetString("team")); err != nil {
			return Idea{}, err
		}
	}
	if bubble != "" {
		b, err := app.FindRecordById("bubbles", bubble)
		if err != nil {
			return Idea{}, ErrDenied
		}
		ws, _, err := WorkspaceFor(app, auth, b.GetString("workspace"))
		if err != nil {
			return Idea{}, err
		}
		if err := same(ws.GetString("team")); err != nil {
			return Idea{}, err
		}
	}
	r.Set("thread", thread)
	r.Set("bubble", bubble)
	if err := app.Save(r); err != nil {
		return Idea{}, err
	}
	return ideaOf(app, r, true), nil
}

// IdeaDelete descarta una idea. Refleja `inbox_items.DeleteRule`.
func IdeaDelete(app core.App, auth *core.Record, id string) error {
	r, err := ideaFor(app, auth, id)
	if err != nil {
		return err
	}
	return app.Delete(r)
}

// ---- Mi espacio ----------------------------------------------------------------

// Los 24 colores de las notas, con su nombre, los mismos de
// `frontend/src/lib/projectColors.svelte.ts`.
var noteColors = map[string]string{
	"fresa": "#ffb8b8", "coral": "#ffc9c2", "mandarina": "#ffd0a8", "durazno": "#ffdfc4",
	"arena": "#efe3d0", "mostaza": "#f7e3a1", "limón": "#fff0b3", "crema": "#fff7dc",
	"lima": "#e4f2b8", "pistacho": "#d4ecb0", "salvia": "#cfe3cf", "menta": "#bcebd9",
	"agua": "#b8eee8", "cielo": "#bcecf5", "celeste": "#cfe6ff", "azul": "#c9dafc",
	"pervinca": "#d3d4fb", "lavanda": "#dfd3ff", "lila": "#f1d4f5", "orquídea": "#f6c9ea",
	"rosa": "#fbd1df", "chicle": "#ffc2d8", "niebla": "#e3e6ec", "piedra": "#d6d9e0",
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// NoteColorNames es la lista de nombres que acepta `color`, para las
// descripciones de las tools.
func NoteColorNames() string {
	names := make([]string, 0, len(noteColors))
	for k := range noteColors {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func noteColor(c string) (string, error) {
	c = strings.ToLower(strings.TrimSpace(c))
	if c == "" || hexColor.MatchString(c) {
		return c, nil
	}
	if hex, ok := noteColors[c]; ok {
		return hex, nil
	}
	return "", fmt.Errorf("color %q: usa un nombre de la paleta (%s) o un #rrggbb", c, NoteColorNames())
}

// SpaceNote es una nota de Mi espacio: el lienzo privado de cada persona.
type SpaceNote struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"body,omitempty"`
	Color   string `json:"color,omitempty"`
	Project string `json:"project,omitempty"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	W       int    `json:"w"`
	H       int    `json:"h"`
}

func spaceOf(r *core.Record) SpaceNote {
	return SpaceNote{
		ID: r.Id, Title: r.GetString("title"), Body: r.GetString("body"),
		Color: r.GetString("color"), Project: r.GetString("workspace"),
		X: r.GetInt("x"), Y: r.GetInt("y"), W: r.GetInt("w"), H: r.GetInt("h"),
	}
}

// SpaceEdit son los campos que se pueden cambiar de una nota; nil = igual.
type SpaceEdit struct {
	Title, Body, Color, Project *string
	X, Y, W, H                  *int
}

// noteFor refleja las reglas de `personal_notes`: SÓLO su dueño, ni siquiera
// el lead global.
func noteFor(app core.App, auth *core.Record, id string) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	r, err := app.FindRecordById("personal_notes", id)
	if err != nil || r.GetString("owner") != auth.Id {
		return nil, ErrDenied
	}
	return r, nil
}

func applySpace(app core.App, auth *core.Record, r *core.Record, in SpaceEdit) error {
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return fmt.Errorf("una nota necesita un título")
		}
		if len([]rune(t)) > 150 {
			return fmt.Errorf("el título pasa de 150 caracteres")
		}
		r.Set("title", t)
	}
	if in.Body != nil {
		r.Set("body", *in.Body)
	}
	if in.Color != nil {
		c, err := noteColor(*in.Color)
		if err != nil {
			return err
		}
		r.Set("color", c)
	}
	if in.Project != nil {
		ws := ""
		if *in.Project != "" {
			// Refleja la regla: sólo un proyecto que la persona ve.
			w, _, err := WorkspaceFor(app, auth, *in.Project)
			if err != nil {
				return err
			}
			ws = w.Id
		}
		r.Set("workspace", ws)
	}
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	if in.X != nil {
		r.Set("x", clamp(*in.X, -1000000, 1000000))
	}
	if in.Y != nil {
		r.Set("y", clamp(*in.Y, -1000000, 1000000))
	}
	if in.W != nil {
		r.Set("w", clamp(*in.W, 180, 1200))
	}
	if in.H != nil {
		h := *in.H
		if h != 0 {
			h = clamp(h, 140, 1600)
		}
		r.Set("h", h)
	}
	return nil
}

// Space lista las notas de Mi espacio de quien llama.
func Space(app core.App, auth *core.Record) ([]SpaceNote, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	rows, err := app.FindRecordsByFilter("personal_notes", "owner = {:me}", "-updated", 0, 0, dbx.Params{"me": auth.Id})
	if err != nil {
		return nil, err
	}
	out := make([]SpaceNote, 0, len(rows))
	for _, r := range rows {
		out = append(out, spaceOf(r))
	}
	return out, nil
}

// SpaceCreate crea una nota. Sin posición, queda a la derecha de las que ya hay.
func SpaceCreate(app core.App, auth *core.Record, in SpaceEdit) (SpaceNote, error) {
	if !isPersonAuth(auth) {
		return SpaceNote{}, ErrNotAPerson
	}
	if in.Title == nil {
		return SpaceNote{}, fmt.Errorf("una nota necesita un título")
	}
	col, err := app.FindCollectionByNameOrId("personal_notes")
	if err != nil {
		return SpaceNote{}, err
	}
	r := core.NewRecord(col)
	r.Set("owner", auth.Id)
	r.Set("w", 240)
	if in.X == nil || in.Y == nil {
		x, y := -120, -110
		if mine, err := Space(app, auth); err == nil && len(mine) > 0 {
			x, y = mine[0].X, mine[0].Y
			for _, n := range mine {
				w := n.W
				if w == 0 {
					w = 240
				}
				if n.X+w+40 > x {
					x = n.X + w + 40
				}
				if n.Y < y {
					y = n.Y
				}
			}
		}
		if in.X == nil {
			in.X = &x
		}
		if in.Y == nil {
			in.Y = &y
		}
	}
	if err := applySpace(app, auth, r, in); err != nil {
		return SpaceNote{}, err
	}
	if err := app.Save(r); err != nil {
		return SpaceNote{}, err
	}
	return spaceOf(r), nil
}

// SpaceUpdate cambia una nota.
func SpaceUpdate(app core.App, auth *core.Record, id string, in SpaceEdit) (SpaceNote, error) {
	r, err := noteFor(app, auth, id)
	if err != nil {
		return SpaceNote{}, err
	}
	if err := applySpace(app, auth, r, in); err != nil {
		return SpaceNote{}, err
	}
	if err := app.Save(r); err != nil {
		return SpaceNote{}, err
	}
	return spaceOf(r), nil
}

// SpaceMove es dónde queda una nota al acomodar varias.
type SpaceMove struct {
	Note string `json:"note"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    *int   `json:"w,omitempty"`
	H    *int   `json:"h,omitempty"`
}

// SpaceArrange acomoda varias notas de una vez: todas o ninguna.
func SpaceArrange(app core.App, auth *core.Record, moves []SpaceMove) ([]SpaceNote, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	if len(moves) == 0 {
		return nil, fmt.Errorf("`notes` está vacío: no hay nada que acomodar")
	}
	out := []SpaceNote{}
	err := app.RunInTransaction(func(tx core.App) error {
		for _, m := range moves {
			r, err := noteFor(tx, auth, m.Note)
			if err != nil {
				return fmt.Errorf("nota %q: %w", m.Note, err)
			}
			x, y := m.X, m.Y
			if err := applySpace(tx, auth, r, SpaceEdit{X: &x, Y: &y, W: m.W, H: m.H}); err != nil {
				return err
			}
			if err := tx.Save(r); err != nil {
				return err
			}
			out = append(out, spaceOf(r))
		}
		return nil
	})
	return out, err
}

// SpaceDelete borra una nota (y lo que tenía adjunto, en cascada).
func SpaceDelete(app core.App, auth *core.Record, id string) error {
	r, err := noteFor(app, auth, id)
	if err != nil {
		return err
	}
	return app.Delete(r)
}

// ---- pines de hilo ---------------------------------------------------------------

// Los pines son atajos PERSONALES a hilos, guardados en `prefs.value.thread_pins`
// de cada quien. Refleja `prefs`: sólo el dueño.

func prefsOf(app core.App, auth *core.Record) (*core.Record, map[string]any, error) {
	r, err := app.FindFirstRecordByData("prefs", "user", auth.Id)
	if err != nil {
		col, err := app.FindCollectionByNameOrId("prefs")
		if err != nil {
			return nil, nil, err
		}
		r = core.NewRecord(col)
		r.Set("user", auth.Id)
	}
	value := map[string]any{}
	_ = r.UnmarshalJSONField("value", &value)
	return r, value, nil
}

func pinIDs(value map[string]any) []string {
	out := []string{}
	if list, ok := value["thread_pins"].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok && len(s) == 15 {
				out = append(out, s)
			}
		}
	}
	return out
}

// Pin es un hilo fijado, con lo que hace falta para reconocerlo.
type Pin struct {
	Thread  string `json:"thread"`
	Name    string `json:"name"`
	Project string `json:"project"`
}

// Pins lista los pines de quien llama, en su orden. Los de hilos que ya no
// alcanza (o que se borraron) no salen.
func Pins(app core.App, auth *core.Record) ([]Pin, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	_, value, err := prefsOf(app, auth)
	if err != nil {
		return nil, err
	}
	out := []Pin{}
	for _, id := range pinIDs(value) {
		th, ws, _, err := ThreadFor(app, auth, id)
		if err != nil {
			continue
		}
		out = append(out, Pin{Thread: th.Id, Name: th.GetString("name"), Project: ws.GetString("slug")})
	}
	return out, nil
}

// SetPin fija o suelta un hilo.
func SetPin(app core.App, auth *core.Record, thread string, pinned bool) ([]Pin, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	if pinned {
		if _, _, _, err := ThreadFor(app, auth, thread); err != nil {
			return nil, err
		}
	}
	r, value, err := prefsOf(app, auth)
	if err != nil {
		return nil, err
	}
	ids := []any{}
	for _, id := range pinIDs(value) {
		if id != thread {
			ids = append(ids, id)
		}
	}
	if pinned {
		ids = append(ids, thread)
	}
	value["thread_pins"] = ids
	r.Set("value", value)
	if err := app.Save(r); err != nil {
		return nil, err
	}
	return Pins(app, auth)
}

// ---- repositorios -------------------------------------------------------------

// leadsWorkspace refleja la regla de escritura de `workspace_repos`: un lead
// del equipo del proyecto, o un lead de ese proyecto.
func leadsWorkspace(app core.App, auth *core.Record, wsID string) bool {
	if LeadsWorkspaceTeam(app, auth, wsID) {
		return true
	}
	if !isPersonAuth(auth) {
		return false
	}
	var n int
	err := app.DB().NewQuery(
		`SELECT COUNT(*) FROM memberships WHERE workspace = {:ws} AND user = {:u} AND role = 'lead'`).
		Bind(dbx.Params{"ws": wsID, "u": auth.Id}).Row(&n)
	return err == nil && n > 0
}

// RepoLink enlaza un repositorio a un proyecto.
func RepoLink(app core.App, auth *core.Record, ws *core.Record, raw, name string) (Repo, error) {
	if !leadsWorkspace(app, auth, ws.Id) {
		return Repo{}, fmt.Errorf("sólo un lead del proyecto (o el lead global) enlaza repositorios")
	}
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || strings.Trim(u.Path, "/") == "" {
		return Repo{}, fmt.Errorf("%q no es la URL de un repositorio", raw)
	}
	col, err := app.FindCollectionByNameOrId("workspace_repos")
	if err != nil {
		return Repo{}, err
	}
	r := core.NewRecord(col)
	r.Set("workspace", ws.Id)
	r.Set("url", raw)
	r.Set("name", strings.TrimSpace(name))
	if err := app.Save(r); err != nil {
		return Repo{}, err
	}
	return Repo{ID: r.Id, URL: raw, Name: r.GetString("name")}, nil
}

// RepoUnlink quita un repositorio enlazado.
func RepoUnlink(app core.App, auth *core.Record, id string) error {
	r, err := app.FindRecordById("workspace_repos", id)
	if err != nil {
		return ErrDenied
	}
	if _, _, err := WorkspaceFor(app, auth, r.GetString("workspace")); err != nil {
		return err
	}
	if !leadsWorkspace(app, auth, r.GetString("workspace")) {
		return fmt.Errorf("sólo un lead del proyecto (o el lead global) quita repositorios")
	}
	return app.Delete(r)
}

// ---- inventario ------------------------------------------------------------------

// Escribir el inventario refleja `inventory_groups/items` Create/Update/Delete:
// sólo el lead global. Quién lo ve (`inventory_access`) es administración y se
// queda en la aplicación.

// InvEdit son los campos de un grupo o una cosa; nil = igual.
type InvEdit struct {
	Group, Name, Provider, URL, Vault, Notes, Renews, Cost *string
}

func invLead(auth *core.Record) error {
	if !isGlobalLead(auth) {
		return fmt.Errorf("sólo el lead global escribe el inventario")
	}
	return nil
}

func optURL(field, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("`%s` tiene que ser un enlace http(s), no %q", field, v)
	}
	return v, nil
}

func applyInv(app core.App, r *core.Record, in InvEdit, item bool) error {
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return fmt.Errorf("necesita un nombre")
		}
		r.Set("name", strings.TrimSpace(*in.Name))
	}
	if !item {
		if in.Notes != nil {
			r.Set("note", *in.Notes)
		}
		return nil
	}
	if in.Group != nil {
		if _, err := app.FindRecordById("inventory_groups", *in.Group); err != nil {
			return fmt.Errorf("no hay ningún grupo %q (inventory_list los dice)", *in.Group)
		}
		r.Set("group", *in.Group)
	}
	if in.Provider != nil {
		r.Set("provider", strings.TrimSpace(*in.Provider))
	}
	if in.Cost != nil {
		r.Set("cost", strings.TrimSpace(*in.Cost))
	}
	if in.Notes != nil {
		r.Set("notes", *in.Notes)
	}
	for field, v := range map[string]*string{"url": in.URL, "vault": in.Vault} {
		if v == nil {
			continue
		}
		ok, err := optURL(field, *v)
		if err != nil {
			return err
		}
		r.Set(field, ok)
	}
	if in.Renews != nil {
		d := strings.TrimSpace(*in.Renews)
		if d == "" {
			r.Set("renews_at", "")
		} else {
			if !dayOnly.MatchString(d) {
				return fmt.Errorf("una fecha es un día: 2026-09-15, no %q", d)
			}
			r.Set("renews_at", d+" 00:00:00.000Z")
		}
	}
	return nil
}

// InvGroupSave crea (id vacío) o cambia un grupo del inventario.
func InvGroupSave(app core.App, auth *core.Record, id string, in InvEdit) (*core.Record, error) {
	if err := invLead(auth); err != nil {
		return nil, err
	}
	var r *core.Record
	if id == "" {
		col, err := app.FindCollectionByNameOrId("inventory_groups")
		if err != nil {
			return nil, err
		}
		r = core.NewRecord(col)
		if in.Name == nil {
			return nil, fmt.Errorf("un grupo necesita un nombre")
		}
	} else {
		found, err := app.FindRecordById("inventory_groups", id)
		if err != nil {
			return nil, ErrDenied
		}
		r = found
	}
	if err := applyInv(app, r, in, false); err != nil {
		return nil, err
	}
	return r, app.Save(r)
}

// InvGroupDelete borra un grupo VACÍO. Borrarlo con cosas dentro se las llevaría
// en cascada, y eso no se hace sin mirar: primero se mueven o se borran.
func InvGroupDelete(app core.App, auth *core.Record, id string) error {
	if err := invLead(auth); err != nil {
		return err
	}
	r, err := app.FindRecordById("inventory_groups", id)
	if err != nil {
		return ErrDenied
	}
	items, _ := app.FindAllRecords("inventory_items", dbx.HashExp{"group": id})
	if len(items) > 0 {
		return fmt.Errorf("el grupo tiene %d cosas: muévelas o bórralas primero", len(items))
	}
	return app.Delete(r)
}

// InvItemSave crea (id vacío) o cambia una cosa del inventario.
func InvItemSave(app core.App, auth *core.Record, id string, in InvEdit) (*core.Record, error) {
	if err := invLead(auth); err != nil {
		return nil, err
	}
	var r *core.Record
	if id == "" {
		col, err := app.FindCollectionByNameOrId("inventory_items")
		if err != nil {
			return nil, err
		}
		r = core.NewRecord(col)
		if in.Name == nil || in.Group == nil {
			return nil, fmt.Errorf("una cosa del inventario necesita `name` y `group`")
		}
	} else {
		found, err := app.FindRecordById("inventory_items", id)
		if err != nil {
			return nil, ErrDenied
		}
		r = found
	}
	if err := applyInv(app, r, in, true); err != nil {
		return nil, err
	}
	return r, app.Save(r)
}

// InvItemDelete borra una cosa del inventario.
func InvItemDelete(app core.App, auth *core.Record, id string) error {
	if err := invLead(auth); err != nil {
		return err
	}
	r, err := app.FindRecordById("inventory_items", id)
	if err != nil {
		return ErrDenied
	}
	return app.Delete(r)
}

// ---- el hilo, completo -------------------------------------------------------------

// ThreadInfo es la ficha de un hilo: todo lo que no es su documento.
type ThreadInfo struct {
	ID        string      `json:"id"`
	Seq       int         `json:"seq"`
	Name      string      `json:"name"`
	Project   string      `json:"project"`
	Team      TeamRef     `json:"team"`
	Bubble    string      `json:"bubble,omitempty"`
	BubbleNm  string      `json:"bubble_name,omitempty"`
	State     string      `json:"state"`
	Group     string      `json:"state_group"`
	States    []StateView `json:"states"`
	Assignees []Person    `json:"assignees"`
	Labels    []string    `json:"labels"`
	// Los tags del frontmatter de su documento: se cambian editándolo.
	Tags      []string       `json:"tags"`
	Due       string         `json:"due_date,omitempty"`
	Priority  string         `json:"priority,omitempty"`
	Sequence  int            `json:"sequence,omitempty"`
	Relations []RelationView `json:"relations"`
	Evidence  []Evidence     `json:"evidence"`
	DocPath   string         `json:"doc_path"`
	Pinned    bool           `json:"pinned"`
}

// StateView es un estado posible del proyecto; `group` dice qué significa
// (backlog, unstarted, started, completed, cancelled).
type StateView struct {
	Name  string `json:"name"`
	Group string `json:"group"`
}

type RelationView struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Thread string `json:"thread"`
	Name   string `json:"name"`
}

type Evidence struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// ThreadGet arma la ficha de un hilo que la persona alcanza.
func ThreadGet(app core.App, auth *core.Record, id string) (ThreadInfo, error) {
	th, ws, _, err := ThreadFor(app, auth, id)
	if err != nil {
		return ThreadInfo{}, err
	}
	v := ThreadInfo{
		ID: th.Id, Seq: th.GetInt("seq"), Name: th.GetString("name"), Project: ws.GetString("slug"),
		Team:   TeamRefOf(app, ws.GetString("team")),
		Bubble: th.GetString("bubble"), Priority: th.GetString("priority"), Sequence: th.GetInt("sequence"),
		DocPath: th.GetString("doc_path"), Assignees: peopleNamed(app, th.GetStringSlice("assignees")),
		Labels: []string{}, Tags: append([]string{}, threadTags(th)...), Relations: []RelationView{}, Evidence: []Evidence{}, States: []StateView{},
	}
	if b, err := app.FindRecordById("bubbles", v.Bubble); err == nil {
		v.BubbleNm = b.GetString("name")
	}
	if st, err := app.FindRecordById("states", th.GetString("state")); err == nil {
		v.State, v.Group = st.GetString("name"), st.GetString("group")
	}
	if states, err := app.FindRecordsByFilter("states", "workspace = {:ws}", "position", 0, 0, dbx.Params{"ws": ws.Id}); err == nil {
		for _, s := range states {
			v.States = append(v.States, StateView{Name: s.GetString("name"), Group: s.GetString("group")})
		}
	}
	for _, l := range th.GetStringSlice("labels") {
		if r, err := app.FindRecordById("labels", l); err == nil {
			v.Labels = append(v.Labels, r.GetString("name"))
		}
	}
	if d := th.GetDateTime("due_date"); !d.IsZero() {
		v.Due = d.String()[:10]
	}
	if rels, err := app.FindRecordsByFilter("thread_relations", "thread = {:t} || related = {:t}", "created", 0, 0, dbx.Params{"t": th.Id}); err == nil {
		for _, r := range rels {
			other, kind := r.GetString("related"), r.GetString("type")
			if other == th.Id {
				// La misma fila leída desde el otro lado.
				other = r.GetString("thread")
				switch kind {
				case "blocking":
					kind = "blocked_by"
				case "blocked_by":
					kind = "blocking"
				}
			}
			name := ""
			if o, err := app.FindRecordById("threads", other); err == nil {
				name = o.GetString("name")
			}
			v.Relations = append(v.Relations, RelationView{ID: r.Id, Type: kind, Thread: other, Name: name})
		}
	}
	if links, err := app.FindRecordsByFilter("thread_links", "thread = {:t}", "created", 0, 0, dbx.Params{"t": th.Id}); err == nil {
		for _, l := range links {
			v.Evidence = append(v.Evidence, Evidence{URL: l.GetString("url"), Title: l.GetString("title")})
		}
	}
	if _, value, err := prefsOf(app, auth); err == nil {
		for _, p := range pinIDs(value) {
			if p == th.Id {
				v.Pinned = true
			}
		}
	}
	return v, nil
}

// setThreadMore aplica lo que ThreadEdit trae además de nombre, módulo y
// prioridad. Refleja `threads.UpdateRule`: cualquier miembro.
func setThreadMore(app core.App, auth *core.Record, th, ws *core.Record, in ThreadEdit) (completed bool, err error) {
	if in.Assignees != nil {
		ids, err := activePeople(app, *in.Assignees)
		if err != nil {
			return false, err
		}
		if _, err := joinAssigned(app, auth, ids, ws.Id); err != nil {
			return false, err
		}
		th.Set("assignees", ids)
	}
	if in.Due != nil {
		d := strings.TrimSpace(*in.Due)
		if d == "" {
			th.Set("due_date", "")
		} else {
			if !dayOnly.MatchString(d) {
				return false, fmt.Errorf("una fecha es un día: 2026-09-15, no %q", d)
			}
			th.Set("due_date", d+" 00:00:00.000Z")
		}
	}
	if in.State != nil {
		want := strings.TrimSpace(*in.State)
		states, err := app.FindRecordsByFilter("states", "workspace = {:ws}", "position", 0, 0, dbx.Params{"ws": ws.Id})
		if err != nil {
			return false, err
		}
		var hit *core.Record
		names := []string{}
		for _, s := range states {
			names = append(names, s.GetString("name"))
			if s.Id == want || strings.EqualFold(s.GetString("name"), want) {
				hit = s
			}
		}
		if hit == nil {
			return false, fmt.Errorf("el proyecto no tiene el estado %q; tiene: %s", want, strings.Join(names, ", "))
		}
		was := completedState(app, th.GetString("state"))
		th.Set("state", hit.Id)
		completed = !was && hit.GetString("group") == "completed"
	}
	if in.Labels != nil {
		ids := []string{}
		for _, name := range *in.Labels {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			l, err := app.FindFirstRecordByFilter("labels", "workspace = {:ws} && name = {:n}", dbx.Params{"ws": ws.Id, "n": name})
			if err != nil {
				// Refleja `labels.CreateRule`: cualquier miembro crea una etiqueta.
				col, err := app.FindCollectionByNameOrId("labels")
				if err != nil {
					return false, err
				}
				l = core.NewRecord(col)
				l.Set("workspace", ws.Id)
				l.Set("name", name)
				if err := app.Save(l); err != nil {
					return false, err
				}
			}
			ids = append(ids, l.Id)
		}
		th.Set("labels", ids)
	}
	return completed, nil
}

// ThreadRelate anota una relación entre dos hilos que la persona alcanza.
// Refleja `thread_relations.CreateRule`.
func ThreadRelate(app core.App, auth *core.Record, thread, related, kind string) (RelationView, error) {
	switch kind {
	case "relates_to", "duplicate", "blocking", "blocked_by":
	default:
		return RelationView{}, fmt.Errorf("una relación es relates_to, duplicate, blocking o blocked_by, no %q", kind)
	}
	if thread == related {
		return RelationView{}, fmt.Errorf("un hilo no se relaciona consigo mismo")
	}
	if _, _, _, err := ThreadFor(app, auth, thread); err != nil {
		return RelationView{}, err
	}
	other, _, _, err := ThreadFor(app, auth, related)
	if err != nil {
		return RelationView{}, err
	}
	if dup, err := app.FindFirstRecordByFilter("thread_relations",
		"(thread = {:a} && related = {:b}) || (thread = {:b} && related = {:a})", dbx.Params{"a": thread, "b": related}); err == nil {
		return RelationView{}, fmt.Errorf("esos dos hilos ya están relacionados (%s, id %s); quítala primero para cambiarla", dup.GetString("type"), dup.Id)
	}
	col, err := app.FindCollectionByNameOrId("thread_relations")
	if err != nil {
		return RelationView{}, err
	}
	r := core.NewRecord(col)
	r.Set("thread", thread)
	r.Set("related", related)
	r.Set("type", kind)
	if err := app.Save(r); err != nil {
		return RelationView{}, err
	}
	return RelationView{ID: r.Id, Type: kind, Thread: related, Name: other.GetString("name")}, nil
}

// ThreadUnrelate quita una relación. Refleja `thread_relations.DeleteRule`.
func ThreadUnrelate(app core.App, auth *core.Record, id string) error {
	r, err := app.FindRecordById("thread_relations", id)
	if err != nil {
		return ErrDenied
	}
	if _, _, _, err := ThreadFor(app, auth, r.GetString("thread")); err != nil {
		return err
	}
	return app.Delete(r)
}

// ---- secuencia -----------------------------------------------------------------------

// SetSequence pone un hilo o un módulo en la secuencia de su equipo.
// Refleja `threads.UpdateRule`: `sequence` lo escribe un lead del equipo.
func SetSequence(app core.App, auth *core.Record, kind, id string, n int) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	if n < 0 {
		return nil, fmt.Errorf("la secuencia es 0 (fuera) o un paso positivo")
	}
	var r *core.Record
	switch kind {
	case "thread":
		th, _, _, err := ThreadFor(app, auth, id)
		if err != nil {
			return nil, err
		}
		r = th
	case "bubble":
		b, err := app.FindRecordById("bubbles", id)
		if err != nil {
			return nil, ErrDenied
		}
		if _, _, err := WorkspaceFor(app, auth, b.GetString("workspace")); err != nil {
			return nil, err
		}
		r = b
	default:
		return nil, fmt.Errorf("da un `thread` o un `bubble`")
	}
	if !LeadsWorkspaceTeam(app, auth, r.GetString("workspace")) {
		return nil, fmt.Errorf("sólo un lead del equipo ordena su secuencia")
	}
	r.Set("sequence", n)
	return r, app.Save(r)
}

// ---- cambios del departamento ----------------------------------------------------------

// Change es algo que pasó: un evento del registro, con nombres.
type Change struct {
	At      string         `json:"at"`
	Kind    string         `json:"kind"`
	Warms   bool           `json:"warms"`
	Project string         `json:"project"`
	Team    string         `json:"team"`
	Thread  string         `json:"thread,omitempty"`
	Name    string         `json:"name,omitempty"`
	Actor   string         `json:"actor,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

// Changes es lo que pasó en los proyectos que la persona ve desde `since`.
// Refleja `events.ListRule`: el lead global, o los miembros de cada proyecto.
func Changes(app core.App, auth *core.Record, since, project string, limit int) ([]Change, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	from := time.Now().UTC().Add(-24 * time.Hour)
	switch s := strings.TrimSpace(since); {
	case s == "":
	case s == "cycle":
		if t, err := time.Parse(time.RFC3339, sinceOf(app, "cycle")); err == nil {
			from = t
		}
	case dayOnly.MatchString(s):
		t, _ := time.Parse("2006-01-02", s)
		from = t
	default:
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("`since` es un día (2026-10-12), una fecha RFC 3339 o «cycle», no %q", s)
		}
		from = t.UTC()
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	wss := []*core.Record{}
	if project != "" {
		ws, _, err := WorkspaceFor(app, auth, project)
		if err != nil {
			return nil, err
		}
		wss = append(wss, ws)
	} else {
		all, err := app.FindAllRecords("workspaces")
		if err != nil {
			return nil, err
		}
		for _, w := range all {
			if canSee(app, auth, w.Id) {
				wss = append(wss, w)
			}
		}
	}
	out := []Change{}
	if len(wss) == 0 {
		return out, nil
	}
	slug := map[string]string{}
	teamName := map[string]string{}
	rows := []*core.Record{}
	at := from.Format("2006-01-02 15:04:05.000Z")
	for _, w := range wss {
		slug[w.Id] = w.GetString("slug")
		teamName[w.Id] = TeamRefOf(app, w.GetString("team")).Name
		part, err := app.FindRecordsByFilter("events", "at >= {:from} && workspace = {:ws}", "-at", limit, 0,
			dbx.Params{"from": at, "ws": w.Id})
		if err != nil {
			return nil, err
		}
		rows = append(rows, part...)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].GetDateTime("at").After(rows[j].GetDateTime("at")) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	names := map[string]string{}
	for _, e := range rows {
		c := Change{
			At: e.GetDateTime("at").String(), Kind: e.GetString("kind"), Project: slug[e.GetString("workspace")],
			Team: teamName[e.GetString("workspace")],
		}
		switch c.Kind {
		case EvDocumentChanged, EvThreadCreated, EvThreadCompleted, EvLinkAdded:
			c.Warms = true
		}
		if e.GetString("target_type") == "thread" {
			c.Thread = e.GetString("target")
			if _, ok := names[c.Thread]; !ok {
				if th, err := app.FindRecordById("threads", c.Thread); err == nil {
					names[c.Thread] = th.GetString("name")
				}
			}
			c.Name = names[c.Thread]
		}
		if u, err := app.FindRecordById("users", e.GetString("actor")); err == nil {
			c.Actor = personName(u)
		}
		meta := map[string]any{}
		if e.UnmarshalJSONField("meta", &meta) == nil && len(meta) > 0 {
			c.Meta = meta
		}
		out = append(out, c)
	}
	return out, nil
}

// ---- artefactos ------------------------------------------------------------------------

// ArtifactsOf lista los artefactos de quien llama (o, con `all`, también los
// de los equipos que lidera), los abiertos primero. La misma lista que
// `GET /api/artifacts`.
func ArtifactsOf(app core.App, auth *core.Record, all bool) ([]ArtifactView, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	filter := "creator = {:c}"
	params := dbx.Params{"c": auth.Id}
	if all {
		led := LedTeams(app, auth)
		if len(led) == 0 {
			return nil, fmt.Errorf("sólo un lead de equipo lee los artefactos de su equipo")
		}
		scope := []string{filter}
		for i, t := range led {
			k := fmt.Sprintf("t%d", i)
			params[k] = t
			scope = append(scope, "inbox_item.team = {:"+k+"}")
		}
		filter = strings.Join(scope, " || ")
	}
	rows, err := app.FindRecordsByFilter("artifacts", filter, "-touched_at", 0, 0, params)
	if err != nil {
		return nil, err
	}
	list := make([]ArtifactView, 0, len(rows))
	for _, r := range rows {
		entries, _ := loadEntries(app, r.Id)
		list = append(list, viewOf(app, r, entries, false))
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Status == "open" && list[j].Status != "open"
	})
	return list, nil
}

// InvGroup es un grupo del inventario.
type InvGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

// InventoryGroups lista los grupos, también los vacíos, con la misma regla
// de lectura que el inventario.
func InventoryGroups(app core.App, auth *core.Record) ([]InvGroup, error) {
	if !SeesInventory(app, auth) {
		return nil, ErrDenied
	}
	rows, err := app.FindRecordsByFilter("inventory_groups", "", "position", 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]InvGroup, 0, len(rows))
	for _, g := range rows {
		out = append(out, InvGroup{ID: g.Id, Name: g.GetString("name"), Note: g.GetString("note")})
	}
	return out, nil
}
