package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Qué tiene cada agente: lo que el servidor observa por token, para saber si el
// agente de cada persona tiene la release vigente del AGENTS.md.
//
//   - `client`: el `clientInfo` (nombre y versión) de su último `initialize`;
//     lo manda el cliente, no el modelo.
//   - `connect_version`: la cabecera `X-Bubble-Connect` que dejó en su
//     configuración el prompt de conexión, es decir con qué sección se instaló.
//   - `release_read`, `release_role`, `release_read_at`: la última release de
//     `agents_md` que el servidor le ENTREGÓ, de qué rol y cuándo. Prueba que el
//     texto llegó, no que el modelo lo siguiera.
//
// Lo escribe sólo el servidor, con escrituras directas como `last_used_at`
// (nadie escribe en `agent_tokens` por la API de registros). No va a `events`:
// eso es evidencia de trabajo, y esto es telemetría de los agentes.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("agent_tokens")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.TextField{Name: "client", Max: 120})
		c.Fields.Add(&core.TextField{Name: "connect_version", Max: 32})
		c.Fields.Add(&core.TextField{Name: "release_read", Max: 32})
		c.Fields.Add(&core.TextField{Name: "release_role", Max: 16})
		c.Fields.Add(&core.DateField{Name: "release_read_at"})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("agent_tokens")
		if err != nil {
			return err
		}
		for _, f := range []string{"client", "connect_version", "release_read", "release_role", "release_read_at"} {
			c.Fields.RemoveByName(f)
		}
		return app.Save(c)
	})
}
