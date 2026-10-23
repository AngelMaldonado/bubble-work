package bubble

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/fury"
)

// Furia de IA: lo que sabe leer la base para el motor puro (`internal/fury`).
//
// Se calcula dentro del tablero (`boardFor`), así que `project_board`, el
// tablero de la app, «Todos los equipos», `thread_get` y `sequence_next` dicen
// lo mismo: hay un solo cálculo. Con `features.fury` apagado no se calcula
// nada. Hilo #6 de Burbujas.

// FuryOn: la función del departamento.
func FuryOn(app core.App) bool { return globalFeature(app, "fury") }

// FuryRules son las reglas guardadas, o las de serie si no hay o no se leen.
func FuryRules(app core.App) fury.Rules {
	r := fury.Defaults()
	row, err := app.FindFirstRecordByFilter("fury_rules", "id != ''")
	if err != nil {
		return r
	}
	raw := row.GetString("rules")
	if raw == "" || raw == "null" {
		return r
	}
	// Sobre las de serie: una regla que se agregue después tiene valores aunque
	// la fila guardada sea anterior a ella.
	if err := json.Unmarshal([]byte(raw), &r); err != nil || r.Validate() != nil {
		return fury.Defaults()
	}
	return r
}

// activeFuryRules: las reglas si la función está encendida; nil si no.
func activeFuryRules(app core.App) *fury.Rules {
	if !FuryOn(app) {
		return nil
	}
	r := FuryRules(app)
	return &r
}

// activeDays: por burbuja, cuántos días distintos hubo evidencia (lo que
// calienta) en cualquiera de sus hilos.
func activeDays(app core.App, ws string) map[string]int {
	var rows []struct {
		Bubble string `db:"bubble"`
		Days   int    `db:"days"`
	}
	_ = app.DB().NewQuery(
		"SELECT t.bubble AS bubble, COUNT(DISTINCT substr(e.at, 1, 10)) AS days " +
			"FROM events e JOIN threads t ON t.id = e.target " +
			"WHERE e.workspace = {:ws} AND e.target_type = 'thread' AND t.bubble != '' " +
			"AND e.kind IN ('document-changed', 'thread-created', 'thread-completed', 'link-added') " +
			"GROUP BY t.bubble",
	).Bind(dbx.Params{"ws": ws}).All(&rows)
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.Bubble] = r.Days
	}
	return out
}

// applyFury pone la furia en los hilos y burbujas de un tablero de un proyecto.
func applyFury(app core.App, ws *core.Record, rules fury.Rules, now time.Time, threads []ThreadHeat, bubbles []BubbleHeat) {
	type acc struct {
		threads, words int
		open           []fury.Verdict
		ids, names     []string
	}
	per := map[string]*acc{}
	for i := range threads {
		t := &threads[i]
		done := t.Heat.Lifecycle == "closed"
		v := fury.Evaluate(fury.Facts{
			Kind: fury.Thread, Priority: t.Priority, Due: t.Due, Done: done,
			People: len(t.Assignees), Lifecycle: string(t.Heat.Lifecycle),
		}, rules, now)
		if v.Level.Level > fury.Zen {
			vv := v
			t.Fury = &vv
		}
		if t.Bubble == "" {
			continue
		}
		a := per[t.Bubble]
		if a == nil {
			a = &acc{}
			per[t.Bubble] = a
		}
		a.threads++
		a.words += t.Words
		if !done && v.Level.Level > fury.Zen {
			a.open = append(a.open, v)
			a.ids = append(a.ids, t.ID)
			a.names = append(a.names, t.Name)
		}
	}
	days := activeDays(app, ws.Id)
	created := map[string]time.Time{}
	if recs, err := app.FindAllRecords("bubbles", dbx.HashExp{"workspace": ws.Id}); err == nil {
		for _, b := range recs {
			created[b.Id] = b.GetDateTime("created").Time()
		}
	}
	for i := range bubbles {
		b := &bubbles[i]
		a := per[b.ID]
		if a == nil {
			a = &acc{}
		}
		own := fury.Evaluate(fury.Facts{
			Kind: fury.Bubble, Priority: b.Priority, Due: b.DueDate, Done: b.Closed,
			People: len(b.Owners), Lifecycle: string(b.Heat.Lifecycle),
			Threads: a.threads, Words: a.words, ActiveDays: days[b.ID], Created: created[b.ID],
		}, rules, now)
		if b.Closed {
			continue
		}
		if v := fury.Merge(own, a.open, a.ids, a.names); v.Level.Level > fury.Zen {
			b.Fury = &v
		}
	}
}

// ThreadFury es la furia de un hilo, del mismo cálculo que el tablero de su
// proyecto. nil: apagada, o en calma.
func ThreadFury(app core.App, ws *core.Record, thread string) *fury.Verdict {
	if !FuryOn(app) {
		return nil
	}
	b, err := BoardFor(app, ws)
	if err != nil {
		return nil
	}
	for _, t := range b.Threads {
		if t.ID == thread {
			return t.Fury
		}
	}
	return nil
}

// furyCache: la furia de los hilos por proyecto, calculada una vez por
// pregunta (`sequence_next` lee hilos de varios proyectos).
type furyCache struct {
	app core.App
	on  bool
	ws  map[string]map[string]*fury.Verdict
}

func newFuryCache(app core.App) *furyCache {
	return &furyCache{app: app, on: FuryOn(app), ws: map[string]map[string]*fury.Verdict{}}
}

func (c *furyCache) thread(wsID, thread string) *fury.Verdict {
	if !c.on {
		return nil
	}
	m, ok := c.ws[wsID]
	if !ok {
		m = map[string]*fury.Verdict{}
		if ws, err := c.app.FindRecordById("workspaces", wsID); err == nil {
			if b, err := BoardFor(c.app, ws); err == nil {
				for _, t := range b.Threads {
					if t.Fury != nil {
						m[t.ID] = t.Fury
					}
				}
			}
		}
		c.ws[wsID] = m
	}
	return m[thread]
}

// FuryPreviewItem es algo que dispararía con las reglas dadas.
type FuryPreviewItem struct {
	Kind      string        `json:"kind"`
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Workspace string        `json:"workspace"`
	Seq       int           `json:"seq,omitempty"`
	Fury      *fury.Verdict `json:"fury"`
}

// MyFury es la furia de una persona: la de los hilos asignados a ella y las
// burbujas de las que responde. Lo que enseña su cabeza en el dock y, cuando
// llegue, la luz de su semáforo (hilo #6, historia 3).
type MyFury struct {
	On    bool              `json:"on"`
	Level fury.Level        `json:"level"`
	Items []FuryPreviewItem `json:"items"`
}

func myFury(e *core.RequestEvent) (MyFury, error) {
	out := MyFury{On: FuryOn(e.App), Level: fury.Levels[fury.Zen], Items: []FuryPreviewItem{}}
	if !out.On {
		return out, nil
	}
	r := FuryRules(e.App)
	b, err := allBoards(e, &r)
	if err != nil {
		return out, err
	}
	me := e.Auth.Id
	has := func(ids []string) bool {
		for _, id := range ids {
			if id == me {
				return true
			}
		}
		return false
	}
	slug := map[string]string{}
	for _, w := range b.Workspaces {
		slug[w.ID] = w.Slug
	}
	for _, x := range b.Bubbles {
		if x.Fury != nil && has(x.Owners) {
			out.Items = append(out.Items, FuryPreviewItem{Kind: "bubble", ID: x.ID, Name: x.Name, Workspace: slug[x.Workspace], Fury: x.Fury})
		}
	}
	for _, x := range b.Threads {
		if x.Fury != nil && has(x.Assignees) {
			out.Items = append(out.Items, FuryPreviewItem{Kind: "thread", ID: x.ID, Name: x.Name, Workspace: slug[x.Workspace], Seq: x.Seq, Fury: x.Fury})
		}
	}
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].Fury.Level.Level > out.Items[j].Fury.Level.Level })
	if len(out.Items) > 0 {
		out.Level = out.Items[0].Fury.Level
	}
	return out, nil
}

func registerFury(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		person := apis.RequireAuth("users")

		// Mi furia: el ánimo de la IA respecto a lo mío.
		se.Router.GET("/api/fury/me", func(e *core.RequestEvent) error {
			out, err := myFury(e)
			if err != nil {
				return err
			}
			return e.JSON(http.StatusOK, out)
		}).Bind(person)

		// Las reglas, para quien quiera entender por qué algo está furioso.
		se.Router.GET("/api/fury/rules", func(e *core.RequestEvent) error {
			return e.JSON(http.StatusOK, map[string]any{
				"on": FuryOn(e.App), "rules": FuryRules(e.App), "defaults": fury.Defaults(), "levels": fury.Levels,
			})
		}).Bind(person)

		// Sólo el lead global escribe las reglas, iguales para todos los equipos.
		se.Router.POST("/api/fury/rules", func(e *core.RequestEvent) error {
			if !isGlobalLead(e.Auth) {
				return e.ForbiddenError("las reglas de la furia las define el lead global", nil)
			}
			var body struct {
				Rules *fury.Rules `json:"rules"`
				Reset bool        `json:"reset"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.BadRequestError("no se pudo leer", err)
			}
			r := fury.Defaults()
			if !body.Reset {
				if body.Rules == nil {
					return e.BadRequestError("faltan las reglas", nil)
				}
				r = *body.Rules
			}
			if err := r.Validate(); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			row, err := e.App.FindFirstRecordByFilter("fury_rules", "id != ''")
			if err != nil {
				col, err := e.App.FindCollectionByNameOrId("fury_rules")
				if err != nil {
					return e.InternalServerError(err.Error(), err)
				}
				row = core.NewRecord(col)
			}
			row.Set("rules", r)
			if err := e.App.Save(row); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"on": FuryOn(e.App), "rules": FuryRules(e.App)})
		}).Bind(person)

		// Vista previa: qué dispararía ahora con estas reglas (o las guardadas),
		// aunque la función esté apagada. Para calibrar con datos reales antes de
		// que nada brille. Del lead global: ve lo de todos los equipos que lidera.
		se.Router.POST("/api/fury/preview", func(e *core.RequestEvent) error {
			if !isGlobalLead(e.Auth) {
				return e.ForbiddenError("la vista previa es del lead global", nil)
			}
			var body struct {
				Rules *fury.Rules `json:"rules"`
			}
			_ = e.BindBody(&body)
			r := FuryRules(e.App)
			if body.Rules != nil {
				r = *body.Rules
			}
			if err := r.Validate(); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			b, err := allBoards(e, &r)
			if err != nil {
				return err
			}
			slug := map[string]string{}
			for _, w := range b.Workspaces {
				slug[w.ID] = w.Slug
			}
			items := []FuryPreviewItem{}
			counts := map[string]int{"thread": 0, "bubble": 0}
			levels := [4]int{}
			for _, x := range b.Bubbles {
				if x.Fury != nil {
					items = append(items, FuryPreviewItem{Kind: "bubble", ID: x.ID, Name: x.Name, Workspace: slug[x.Workspace], Fury: x.Fury})
					counts["bubble"]++
					levels[x.Fury.Level.Level]++
				}
			}
			for _, x := range b.Threads {
				if x.Fury != nil {
					items = append(items, FuryPreviewItem{Kind: "thread", ID: x.ID, Name: x.Name, Workspace: slug[x.Workspace], Seq: x.Seq, Fury: x.Fury})
					counts["thread"]++
					levels[x.Fury.Level.Level]++
				}
			}
			sort.SliceStable(items, func(i, j int) bool { return items[i].Fury.Level.Level > items[j].Fury.Level.Level })
			return e.JSON(http.StatusOK, map[string]any{"counts": counts, "levels": levels, "items": items})
		}).Bind(person)
		return se.Next()
	})
}
