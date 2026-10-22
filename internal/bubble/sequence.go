package bubble

import (
	"net/http"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// La secuencia de ejecución del departamento.
//
// `threads.sequence` es el paso de un hilo en una sola línea para todo el
// departamento; dos hilos con el mismo paso van en paralelo. La escribe el lead
// global desde el planeador. Lo que NO se guarda es dónde va la línea: «ahora»
// es el primer paso con algo abierto, así que terminar un hilo —desde la
// secuencia, desde su pantalla o por MCP— la recorre sola.

func registerSequence(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// Terminar un hilo por REST, con la misma función que la tool del MCP:
		// la secuencia lo necesita, y cambiar el estado a mano obliga a quien
		// llama a saber cuál es el estado «completado» de cada proyecto.
		se.Router.POST("/api/threads/{id}/complete", func(e *core.RequestEvent) error {
			th, err := CompleteThread(e.App, e.Auth, e.Request.PathValue("id"))
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"id": th.Id, "state": th.GetString("state")})
		}).Bind(apis.RequireAuth("users"))
		return se.Next()
	})
}

// SequenceOn dice si un equipo tiene encendida la secuencia de ejecución: lo
// que decida el equipo (`team_features`) o, si no dice nada, el departamento
// (`features.sequence`). Sin la fila global —una base a medio migrar— está
// apagada.
func SequenceOn(app core.App, team string) bool { return FeatureOn(app, team, "sequence") }

// NextStep es un paso de la secuencia: los hilos que lo comparten.
type NextStep struct {
	Sequence int              `json:"sequence"`
	Threads  []map[string]any `json:"threads"`
}

// Next contesta «¿qué sigue?» en la secuencia de UN equipo (vacío: el equipo
// por defecto de quien pregunta): el paso de ahora (sus hilos abiertos, en
// paralelo), el siguiente, y el primer hilo abierto de la secuencia que es suyo
// (asignado, o sin asignar en una burbuja a su cargo). Sólo con hilos que puede
// ver. Cada equipo tiene su propia línea.
func Next(app core.App, auth *core.Record, team string) (map[string]any, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	teamID := ""
	teamName := ""
	if ts, err := TeamsOf(app, auth); err == nil && (team != "" || len(ts) > 0) {
		id, err := ResolveTeam(app, auth, team)
		if err != nil {
			return nil, err
		}
		teamID = id
		for _, t := range ts {
			if t.ID == id {
				teamName = t.Name
			}
		}
	}
	if !SequenceOn(app, teamID) {
		return map[string]any{
			"enabled": false,
			"note":    "La secuencia de ejecución está apagada en este equipo: un lead del equipo la enciende en Ajustes → Equipos (o el lead global, para todos, en Ajustes → Funciones).",
		}, nil
	}
	rows, err := app.FindRecordsByFilter("threads", "sequence > 0", "sequence,seq", 0, 0, dbx.Params{})
	if err != nil {
		return nil, err
	}
	reach := map[string]bool{}
	byStep := map[int][]*core.Record{}
	for _, r := range rows {
		wsID := r.GetString("workspace")
		ok, seen := reach[wsID]
		if !seen {
			ws, _, err := WorkspaceFor(app, auth, wsID)
			ok = err == nil && (teamID == "" || ws.GetString("team") == teamID)
			reach[wsID] = ok
		}
		if !ok || completedState(app, r.GetString("state")) {
			continue
		}
		s := r.GetInt("sequence")
		byStep[s] = append(byStep[s], r)
	}
	steps := make([]int, 0, len(byStep))
	for s := range byStep {
		steps = append(steps, s)
	}
	sort.Ints(steps)

	row := func(r *core.Record) map[string]any {
		slug := ""
		if ws, err := app.FindRecordById("workspaces", r.GetString("workspace")); err == nil {
			slug = ws.GetString("slug")
		}
		return map[string]any{
			"id": r.Id, "seq": r.GetInt("seq"), "name": r.GetString("name"),
			"workspace": slug, "bubble": r.GetString("bubble"),
			"priority": r.GetString("priority"), "assignees": r.GetStringSlice("assignees"),
		}
	}
	step := func(i int) *NextStep {
		if i >= len(steps) {
			return nil
		}
		out := &NextStep{Sequence: steps[i]}
		for _, r := range byStep[steps[i]] {
			out.Threads = append(out.Threads, row(r))
		}
		return out
	}

	// «Lo mío»: asignado a quien pregunta o, si el hilo no tiene a nadie, de una
	// burbuja a su cargo — una pieza sin dueño propio es de quien responde por
	// el cuerpo de trabajo. La misma regla que la pantalla «Secuencia».
	owns := map[string]bool{}
	isMine := func(r *core.Record) bool {
		if a := r.GetStringSlice("assignees"); len(a) > 0 {
			for _, x := range a {
				if x == auth.Id {
					return true
				}
			}
			return false
		}
		b := r.GetString("bubble")
		if b == "" {
			return false
		}
		if v, ok := owns[b]; ok {
			return v
		}
		v := false
		if rec, err := app.FindRecordById("bubbles", b); err == nil {
			for _, o := range rec.GetStringSlice("owners") {
				if o == auth.Id {
					v = true
				}
			}
		}
		owns[b] = v
		return v
	}
	var mine map[string]any
	for _, s := range steps {
		for _, r := range byStep[s] {
			if mine == nil && isMine(r) {
				mine = row(r)
			}
		}
	}
	out := map[string]any{"enabled": true, "now": step(0), "then": step(1), "mine": mine}
	if teamID != "" {
		out["team"] = map[string]any{"id": teamID, "name": teamName}
	}
	return out, nil
}
