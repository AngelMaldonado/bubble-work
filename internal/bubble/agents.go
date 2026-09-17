package bubble

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// El AGENTS.md del departamento: la forma de trabajar que el lead escribe y que
// cada agente lee al empezar su sesión. Una fila por versión en `agents_md`; la
// vigente es la más reciente de su rol.

// AgentsDoc es la versión vigente de un rol, tal como la lee un agente.
type AgentsDoc struct {
	Role    string `json:"role"`
	Version string `json:"version"`
	Created string `json:"created"`
	Author  string `json:"author"`
	Content string `json:"content"`
}

// AgentsRole dice qué documento le toca a quien llama: el lead global planea;
// todos los demás operan. Un superuser opera la máquina y no trabaja en ella,
// pero leer no le cuesta nada a nadie, así que recibe el de operadores.
func AgentsRole(auth *core.Record) string {
	if isGlobalLead(auth) {
		return "planner"
	}
	return "operators"
}

// ReadAgents trae la versión vigente de un rol. Vacío `role` es el de quien
// llama. Sin versiones devuelve un documento con `Version` vacía — no es un
// error: es un departamento que todavía no ha escrito cómo trabaja.
func ReadAgents(app core.App, auth *core.Record, role string) (AgentsDoc, error) {
	if auth == nil {
		return AgentsDoc{}, ErrDenied
	}
	if role == "" {
		role = AgentsRole(auth)
	}
	if role != "planner" && role != "operators" {
		return AgentsDoc{}, fmt.Errorf("role is planner or operators, not %q", role)
	}
	out := AgentsDoc{Role: role}
	rows, err := app.FindRecordsByFilter("agents_md", "role = {:r}", "-created", 1, 0, dbx.Params{"r": role})
	if err != nil || len(rows) == 0 {
		return out, err
	}
	r := rows[0]
	out.Version = r.Id
	out.Created = r.GetDateTime("created").String()
	out.Content = r.GetString("content")
	if u, err := app.FindRecordById("users", r.GetString("author")); err == nil {
		out.Author = u.GetString("display_name")
		if out.Author == "" {
			out.Author = u.GetString("email")
		}
	}
	return out, nil
}
