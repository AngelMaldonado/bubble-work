package bubble

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/AngelMaldonado/bubble-work/internal/heat"
)

// The board: what floats, and why.
//
// Both axes are computed here and stored nowhere. Heat says how ALIVE something
// is (evidence × time); priority says how much it MATTERS (impact × urgency).
// They are independent on purpose — folded together, an untouched P1 keeps
// floating and stops looking like a problem, which is the most useful signal the
// system can produce.
func registerBoard(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/workspaces/{id}/board", func(e *core.RequestEvent) error {
			ws, _, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			b, err := BoardFor(e.App, ws)
			if err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, b)
		}).Bind(apis.RequireAuth())

		// Todos: las burbujas de cada workspace que esta persona alcanza, en un
		// solo board.
		//
		// Agregado AQUI y no en el navegador. Pedir un board por workspace y
		// concatenarlos serian N respuestas calculadas en N instantes distintos
		// —el calor es funcion del tiempo, asi que dos filas se compararian
		// contra dos "ahora"— y una segunda implementacion de "que flota". Aqui
		// hay un solo reloj y una sola calibracion, que es lo que hace
		// comparables dos burbujas de proyectos distintos.
		//
		// La frontera la aplica el servidor, como en todas partes: quien no es
		// lead global ve exactamente los workspaces de los que es miembro, y un
		// board vacio es la respuesta correcta para quien no es de ninguno.
		//
		// `?scope=shared` es «Todos los equipos»: la vista del lead global que
		// junta los equipos COMPARTIDOS. Deja fuera todo proyecto de un equipo
		// personal —también el suyo, y el de otro al que lo invitaron—, porque
		// esa vista se enseña en una pantalla compartida y lo personal no se
		// mezcla ahí. El cliente ya filtra; esto es para que filtrar mal en el
		// navegador no baste para colarlo.
		se.Router.GET("/api/board", func(e *core.RequestEvent) error {
			var b Board
			var err error
			switch scope := e.Request.URL.Query().Get("scope"); scope {
			case "":
				b, err = AllBoards(e)
			case "shared":
				b, err = SharedBoards(e)
			default:
				return e.BadRequestError("scope es vacío o shared", nil)
			}
			if err != nil {
				return err
			}
			return e.JSON(http.StatusOK, b)
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}

// ThreadHeat is one thread as the board reports it: both axes, side by side and
// never mixed.
type ThreadHeat struct {
	ID  string `json:"id"`
	Seq int    `json:"seq"`
	// Which workspace this row came from. Redundant on a board that is ONE
	// workspace, and the whole point on the board that is all of them: a row
	// that cannot say where it lives cannot be opened, because the address is
	// built from a slug and a seq.
	Workspace string      `json:"workspace"`
	Name      string      `json:"name"`
	Bubble    string      `json:"bubble,omitempty"`
	Heat      heat.Result `json:"heat"`
	Pulse     bool        `json:"pulse"`
	// Who has it, and when anything last happened to it. Both are for the row a
	// bubble's drawer draws — "🔥 produciendo · ana · hace 2 d" — which is the
	// half of "should I open this?" the title cannot answer. The timestamp is
	// sent raw: how long ago that reads in words is a question about a language,
	// and the server does not have one.
	Assignees []string `json:"assignees,omitempty"`
	At        string   `json:"at,omitempty"`
	// Los tags del frontmatter de su documento, en el orden escrito: la vista
	// de blobs agrupa por ellos. Clasificación, no evidencia.
	Tags []string `json:"tags,omitempty"`
	// Cuánto tiene escrito: palabras de su Markdown (documento y páginas) y
	// archivos (páginas e imágenes citadas). Para medir en Blobs; no es calor.
	Words int `json:"words,omitempty"`
	Files int `json:"files,omitempty"`
	// La prioridad de ESTA pieza (P1…P4), separada de la de su burbuja: la
	// escribe el lead, no se deriva.
	Priority string `json:"priority,omitempty"`
	// Su paso en la secuencia del departamento; 0 es «no está en ella». Dos
	// hilos con el mismo paso van en paralelo.
	Sequence int `json:"sequence,omitempty"`
	// Su lugar en la línea VIVA de todo el departamento: 1 es «ahora», 2 el
	// siguiente… Cero si no está en la secuencia o ya terminó. Se calcula contra
	// todos los proyectos a la vez —una línea es una—, así que un cajón que sólo
	// ve una burbuja puede decir igual en qué paso va cada hilo.
	Step int `json:"step,omitempty"`
	// Cuándo vence ESTA pieza. Independiente del plazo de su burbuja: aquélla
	// es el compromiso, ésta es cómo se reparte entre las piezas.
	Due string `json:"due_date,omitempty"`
}

// day is a stored date as the ten characters a calendar reads. Empty stays
// empty: a zero time printed in full would read as a deadline in year one.
func day(t types.DateTime) string {
	if t.IsZero() {
		return ""
	}
	return t.String()[:10]
}

// BubbleHeat is one bubble, banded by its hottest OPEN thread.
type BubbleHeat struct {
	DueDate   string `json:"due_date,omitempty"`
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	// Su paso en la línea de MÓDULOS, que es otra que la de los hilos: aquélla
	// ordena piezas y ésta cuerpos de trabajo. Cero es «no está en ella».
	Sequence int `json:"sequence,omitempty"`
	// Accountability is a list. The 🪦 band asks whether ANYBODY is accountable,
	// and an empty list answers that exactly as a missing name did.
	Owners  []string `json:"owners,omitempty"`
	Outcome string   `json:"outcome,omitempty"`
	// Para qué sirve, y cuánto importa. Las dos subieron del thread a la
	// burbuja: un objetivo describe un cuerpo de trabajo, y la prioridad la
	// decide quien orquesta. `Priority` se deriva —impacto × urgencia— y por eso
	// llega junto a la banda y nunca mezclada con ella: una dice si la realidad
	// está cambiando, la otra cuánto importa que cambie.
	Objective string `json:"objective,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Closed    bool   `json:"closed"`
	// How it ended, in the words of whoever closed it. Carried on the board so
	// a closed bubble can say why without a second fetch — and so reopening one
	// knows what sentence it is clearing.
	Closure string `json:"closure,omitempty"`
	// When this bubble last produced anything, across its threads. What the
	// cycle bar in the drawer is measured against: the window is the tuning's,
	// and how much of it is spent is this.
	WarmAt string      `json:"warm_at,omitempty"`
	Heat   heat.Result `json:"heat"`
}

// Board is the whole answer, including the calibration it was computed against —
// so a reader can tell a cold thread from a short cycle.
type Board struct {
	Workspace string      `json:"workspace"`
	At        string      `json:"at"`
	Tuning    heat.Tuning `json:"tuning"`
	// Which workspaces this board was composed of — ALWAYS, including the board
	// that is one workspace and lists just itself.
	//
	// Not omitted when empty, and not left out of the single-workspace board:
	// a row carries an id, an id is not a name anybody can read, and a client
	// that has to tell "field absent" from "empty list" is a client with two
	// shapes to handle. Somebody who belongs to no workspace gets `[]`, which
	// is an answer.
	Workspaces []BoardWorkspace `json:"workspaces"`
	Bubbles    []BubbleHeat     `json:"bubbles"`
	Threads    []ThreadHeat     `json:"threads"`
	// Las relaciones declaradas entre hilos de ESTE tablero y del mismo equipo:
	// las aristas del grafo. Sólo con los dos extremos aquí, así que nunca
	// nombran un hilo que quien mira no ve.
	Links []BoardLink `json:"links"`
}

// BoardLink es una relación entre dos hilos, como la dibuja el grafo:
// `dependency` va de lo que se hace antes a lo que espera; `related` no tiene
// sentido, y sus extremos van en orden de id.
type BoardLink struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// linksAmong son las relaciones entre los hilos dados, de un mismo equipo.
// `teamOf` dice el equipo de cada proyecto.
func linksAmong(app core.App, threads []ThreadHeat, teamOf map[string]string) []BoardLink {
	out := []BoardLink{}
	if len(threads) == 0 {
		return out
	}
	wsOf := make(map[string]string, len(threads))
	ids := make([]any, 0, len(threads))
	for _, t := range threads {
		wsOf[t.ID] = t.Workspace
		ids = append(ids, t.ID)
	}
	rows, err := app.FindAllRecords("thread_relations", dbx.In("thread", ids...))
	if err != nil {
		return out
	}
	seen := map[BoardLink]bool{}
	for _, r := range rows {
		a, b := r.GetString("thread"), r.GetString("related")
		wa, okA := wsOf[a]
		wb, okB := wsOf[b]
		if !okA || !okB || a == b || teamOf[wa] != teamOf[wb] {
			continue
		}
		l := BoardLink{Kind: "related"}
		switch r.GetString("type") {
		case "blocking":
			l = BoardLink{From: a, To: b, Kind: "dependency"}
		case "blocked_by":
			l = BoardLink{From: b, To: a, Kind: "dependency"}
		default:
			l.From, l.To = a, b
			if l.From > l.To {
				l.From, l.To = l.To, l.From
			}
		}
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// BoardWorkspace is enough of a workspace to label a row and to build the
// address that opens it: the slug is what `/w/<slug>/t/<seq>` is made of.
type BoardWorkspace struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Team string `json:"team,omitempty"`
	// el nombre del equipo, para que quien lee el tablero sepa el contexto
	TeamName string `json:"team_name,omitempty"`
}

// liveSteps: los pasos de la secuencia que tienen algo abierto, en orden, de
// la línea de UN equipo (cada equipo tiene la suya). Sólo números: no dice de
// qué hilos, así que no enseña nada de un proyecto que quien mira no alcanza.
func liveSteps(app core.App, team string) map[int]int {
	// Apagada, ningún hilo está en ningún paso: los datos se quedan, la línea
	// no se enseña.
	if !SequenceOn(app) {
		return map[int]int{}
	}
	var seqs []int
	_ = app.DB().NewQuery(
		"SELECT DISTINCT t.sequence FROM threads t LEFT JOIN states s ON s.id = t.state " +
			"JOIN workspaces w ON w.id = t.workspace " +
			"WHERE t.sequence > 0 AND w.team = {:team} AND (s.[[group]] IS NULL OR s.[[group]] != 'completed') ORDER BY t.sequence",
	).Bind(dbx.Params{"team": team}).Column(&seqs)
	out := make(map[int]int, len(seqs))
	for i, v := range seqs {
		out[v] = i + 1
	}
	return out
}

func stepOf(steps map[int]int, sequence int, done bool) int {
	if sequence <= 0 || done {
		return 0
	}
	return steps[sequence]
}

// BoardFor computes both axes for a workspace, storing nothing.
func BoardFor(app core.App, ws *core.Record) (Board, error) {
	tun, err := tuningOf(app)
	if err != nil {
		return Board{}, fmt.Errorf("no calibration: %w", err)
	}
	now := time.Now().UTC()

	rows, err := app.FindAllRecords("thread_evidence", dbx.HashExp{"workspace": ws.Id})
	if err != nil {
		return Board{}, err
	}

	prio := priorityOf(app, ws.Id)
	// Lo que el lead decidió de cada hilo —su prioridad y su paso en la
	// secuencia— vive en la tabla de hilos, no en la vista de evidencia: la vista
	// cuenta lo que pasó, y esto no es algo que pasó. Tampoco incluye el plazo:
	// se lee de la misma fila que las otras decisiones sobre la pieza.
	type plan struct {
		priority string
		sequence int
		due      string
		tags     []string
		words    int
		files    int
	}
	planned := map[string]plan{}
	steps := liveSteps(app, ws.GetString("team"))
	if recs, err := app.FindAllRecords("threads", dbx.HashExp{"workspace": ws.Id}); err == nil {
		for _, t := range recs {
			planned[t.Id] = plan{t.GetString("priority"), t.GetInt("sequence"), day(t.GetDateTime("due_date")), threadTags(t), t.GetInt("doc_words"), t.GetInt("doc_files")}
		}
	}
	byBubble := map[string][]heat.Result{}
	// The most recent warm evidence per bubble, which is not derivable from the
	// roll-up: the roll-up keeps a BAND, and the bar needs an instant.
	warmest := map[string]time.Time{}
	threads := make([]ThreadHeat, 0, len(rows))

	for _, r := range rows {
		ev := heat.Evidence{
			LastWarmAt: parseTS(r.GetString("last_warm_at")),
			LastAnyAt:  parseTS(r.GetString("last_any_at")),
			WarmCount:  r.GetInt("warm_count"),
			CreatedAt:  r.GetDateTime("created").Time(),
			Completed:  completedState(app, r.GetString("state")),
		}
		res := heat.Classify(ev, tun, now)
		b := r.GetString("bubble")
		byBubble[b] = append(byBubble[b], res)
		if w := ev.LastWarmAt; !w.IsZero() && w.After(warmest[b]) {
			warmest[b] = w
		}
		// The freshest thing that happened to it, warm or not: a comment is not
		// evidence and never warms anything, but "hace 2 d" is still true.
		at := ev.LastAnyAt
		if at.IsZero() {
			at = ev.CreatedAt
		}
		threads = append(threads, ThreadHeat{
			ID: r.Id, Seq: r.GetInt("seq"), Workspace: ws.Id,
			Name:   r.GetString("name"),
			Bubble: b, Heat: res,
			Pulse:     heat.HasPulse(ev, tun, now),
			Assignees: r.GetStringSlice("assignees"),
			At:        at.UTC().Format(time.RFC3339),
			Priority:  planned[r.Id].priority,
			Sequence:  planned[r.Id].sequence,
			Due:       planned[r.Id].due,
			Tags:      planned[r.Id].tags,
			Words:     planned[r.Id].words,
			Files:     planned[r.Id].files,
			Step:      stepOf(steps, planned[r.Id].sequence, completedState(app, r.GetString("state"))),
		})
	}
	// Hottest first, and within a band the buoyancy score orders it — the model's
	// whole claim, made spatial.
	sort.SliceStable(threads, func(i, j int) bool {
		return floats(threads[i].Heat) > floats(threads[j].Heat)
	})

	bubbles, _ := app.FindAllRecords("bubbles", dbx.HashExp{"workspace": ws.Id})
	out := make([]BubbleHeat, 0, len(bubbles))
	for _, b := range bubbles {
		closed := !b.GetDateTime("closed_at").IsZero()
		owners := b.GetStringSlice("owners")
		res := heat.RollUp(byBubble[b.Id], len(owners) > 0, tun)
		if closed {
			res = heat.Result{Lifecycle: heat.Closed, Code: heat.ReasonClosed,
				Reason: "outcome reached or explicitly abandoned"}
		}
		out = append(out, BubbleHeat{
			ID: b.Id, Workspace: ws.Id, Name: b.GetString("name"), Owners: owners,
			DueDate: b.GetString("due_date"),
			Outcome: b.GetString("outcome"), Closed: closed,
			Objective: b.GetString("objective"),
			Sequence:  b.GetInt("sequence"),
			Priority:  prio[b.Id],
			Closure:   b.GetString("closure"), WarmAt: stamp(warmest[b.Id]), Heat: res,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return floats(out[i].Heat) > floats(out[j].Heat) })

	return Board{
		Workspace: ws.Id, At: now.Format(time.RFC3339), Tuning: tun,
		Workspaces: []BoardWorkspace{{
			ID: ws.Id, Slug: ws.GetString("slug"), Name: ws.GetString("name"), Team: ws.GetString("team"),
			TeamName: TeamRefOf(app, ws.GetString("team")).Name,
		}},
		Bubbles: out, Threads: threads,
		Links: linksAmong(app, threads, map[string]string{ws.Id: ws.GetString("team")}),
	}, nil
}

// floats turns a verdict into one number to sort by: the band first, the score
// inside it. A cold thing never outranks a warm one on score alone.
func floats(r heat.Result) float64 {
	band := map[heat.Lifecycle]float64{
		heat.Hot: 3, heat.Dormant: 2, heat.Rip: 1, heat.Closed: 0,
	}
	return band[r.Lifecycle] + r.Score
}

func tuningOf(app core.App) (heat.Tuning, error) {
	rows, err := app.FindAllRecords("tuning")
	if err != nil || len(rows) == 0 {
		return heat.Tuning{}, err
	}
	r := rows[0]
	return heat.Tuning{
		CycleHours:     r.GetFloat("cycle_hours"),
		DormantCycles:  r.GetFloat("dormant_cycles"),
		DecayCycles:    r.GetFloat("decay_cycles"),
		GraceCycles:    r.GetFloat("grace_cycles"),
		OwnerlessIsRip: r.GetBool("ownerless_is_rip"),
	}, nil
}

func priorityOf(app core.App, workspace string) map[string]string {
	out := map[string]string{}
	rows, err := app.FindAllRecords("bubble_priority", dbx.HashExp{"workspace": workspace})
	if err != nil {
		return out
	}
	for _, r := range rows {
		if p := r.GetString("priority"); p != "" {
			out[r.Id] = p
		}
	}
	return out
}

// parseTS reads the timestamp shape PocketBase stores dates in. A view hands them
// back as text, so there is nothing typed to lean on.
func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000Z", "2006-01-02 15:04:05Z", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// stamp formats an instant for the wire, and an instant that never happened as
// the empty string rather than as year one — the difference between "nothing has
// happened here" and a date from before the calendar.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// AllBoards is the board of every workspace this person reaches, as one board.
//
// One clock and one calibration for all of them: heat is a function of evidence
// and TIME, so two rows compared against two different "now" are not comparable,
// and comparison is the entire point of putting them on one board.
func AllBoards(e *core.RequestEvent) (Board, error) {
	list, err := reachableWorkspaces(e)
	if err != nil {
		return Board{}, err
	}
	now := time.Now().UTC()
	tun, err := tuningOf(e.App)
	if err != nil {
		return Board{}, e.InternalServerError("no calibration", err)
	}
	out := Board{At: now.Format(time.RFC3339), Tuning: tun,
		Workspaces: []BoardWorkspace{}, Bubbles: []BubbleHeat{}, Threads: []ThreadHeat{}, Links: []BoardLink{}}
	teamOf := map[string]string{}

	for _, ws := range list {
		b, err := BoardFor(e.App, ws)
		if err != nil {
			// One workspace that cannot be computed must not blank the other
			// nine: it is missing from the board, and the rest still answers.
			continue
		}
		out.Workspaces = append(out.Workspaces, BoardWorkspace{
			ID: ws.Id, Slug: ws.GetString("slug"), Name: ws.GetString("name"), Team: ws.GetString("team"),
			TeamName: TeamRefOf(e.App, ws.GetString("team")).Name,
		})
		out.Bubbles = append(out.Bubbles, b.Bubbles...)
		out.Threads = append(out.Threads, b.Threads...)
		teamOf[ws.Id] = ws.GetString("team")
	}
	// Entre proyectos del mismo equipo también: una dependencia cruza proyectos.
	out.Links = linksAmong(e.App, out.Threads, teamOf)

	// Re-sorted across workspaces, by the same rule each board sorts by: the
	// band first and the buoyancy inside it. Keeping them grouped by project
	// would make this a list of boards, and the reason to look at all of them at
	// once is to see what is hottest ANYWHERE.
	sort.SliceStable(out.Bubbles, func(i, j int) bool {
		return floats(out.Bubbles[i].Heat) > floats(out.Bubbles[j].Heat)
	})
	sort.SliceStable(out.Threads, func(i, j int) bool {
		return floats(out.Threads[i].Heat) > floats(out.Threads[j].Heat)
	})
	return out, nil
}

// SharedBoards es «Todos los equipos»: el board de todos, sin un solo proyecto
// de un equipo personal. Sólo para un lead global (o superusuario): es la
// única persona que lidera todos los compartidos, así que para nadie más es
// una vista que exista.
func SharedBoards(e *core.RequestEvent) (Board, error) {
	if !isGlobalLead(e.Auth) && !IsSuperuser(e.Auth) {
		return Board{}, e.ForbiddenError("«Todos los equipos» es una vista del lead global", nil)
	}
	b, err := AllBoards(e)
	if err != nil {
		return Board{}, err
	}
	keep := map[string]bool{}
	ws := b.Workspaces[:0]
	for _, w := range b.Workspaces {
		if w.Team != "" && TeamRefOf(e.App, w.Team).Personal {
			continue
		}
		keep[w.ID] = true
		ws = append(ws, w)
	}
	b.Workspaces = ws
	bs := b.Bubbles[:0]
	for _, x := range b.Bubbles {
		if keep[x.Workspace] {
			bs = append(bs, x)
		}
	}
	b.Bubbles = bs
	ts := b.Threads[:0]
	for _, x := range b.Threads {
		if keep[x.Workspace] {
			ts = append(ts, x)
		}
	}
	b.Threads = ts
	b.Links = linksWithin(b.Links, ts)
	return b, nil
}

// linksWithin: las relaciones con los dos extremos entre estos hilos. Todo
// recorte de un tablero pasa por aquí, para que una arista no nombre un hilo
// que ya no está.
func linksWithin(links []BoardLink, threads []ThreadHeat) []BoardLink {
	keep := make(map[string]bool, len(threads))
	for _, t := range threads {
		keep[t.ID] = true
	}
	out := []BoardLink{}
	for _, l := range links {
		if keep[l.From] && keep[l.To] {
			out = append(out, l)
		}
	}
	return out
}

// reachableWorkspaces lists what this person can see, by the same rule
// `reachWorkspace` enforces one at a time.
//
// The rule lives in two places now, which is one more than it should — but the
// alternative is calling the single-workspace check once per row and eating a
// query each time. Both answer the same question: a superuser and the global
// lead see every workspace, and everybody else sees the ones they are a member
// of. Nothing here widens that.
func reachableWorkspaces(e *core.RequestEvent) ([]*core.Record, error) {
	if e.Auth == nil || (!isPersonAuth(e.Auth) && !IsSuperuser(e.Auth)) {
		return nil, e.NotFoundError("", nil)
	}
	// Los proyectos de los equipos que lideras, más aquellos de los que eres
	// miembro (también los de otro equipo al que te invitaron). Un equipo
	// personal ajeno no entra, ni para el lead global.
	all, err := e.App.FindAllRecords("workspaces")
	if err != nil {
		return nil, e.InternalServerError(err.Error(), err)
	}
	if IsSuperuser(e.Auth) {
		return all, nil
	}
	led := map[string]bool{}
	for _, t := range LedTeams(e.App, e.Auth) {
		led[t] = true
	}
	mine := map[string]bool{}
	if rows, err := e.App.FindAllRecords("memberships", dbx.HashExp{"user": e.Auth.Id}); err == nil {
		for _, m := range rows {
			mine[m.GetString("workspace")] = true
		}
	}
	out := make([]*core.Record, 0, len(all))
	for _, ws := range all {
		if led[ws.GetString("team")] || mine[ws.Id] {
			out = append(out, ws)
		}
	}
	return out, nil
}
