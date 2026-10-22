package mcpapi

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/bubble"
)

// La release de `agents_md` y lo que el servidor ve que tiene cada agente.
//
// Vive aquí y no en `bubble` porque la tabla de tools es parte de lo que lee el
// agente, y la tabla sale del registro de este paquete.

// agentsRelease es lo que `agents_md` devuelve a un rol, sin la cabecera, y su
// huella. La forma de trabajar del departamento va AL FINAL: lo último que lee
// el agente es lo que manda.
type agentsRelease struct {
	doc  bubble.AgentsDoc
	body string
	id   string
}

func releaseFor(app core.App, auth *core.Record, role string, entries []toolEntry) (agentsRelease, error) {
	doc, err := bubble.ReadAgents(app, auth, role)
	if err != nil {
		return agentsRelease{}, err
	}
	dept := "El lead todavía no ha escrito la forma de trabajar de este rol. Sigue la guía y el `README.md` de cada proyecto."
	if doc.Version != "" {
		dept = doc.Content
	}
	body := bubble.SystemPrompt(app, "agents_guide") + "\n\n" +
		catalog(entries) + "\n---\n\n" +
		bubble.SystemPrompt(app, "markdown") + "\n\n---\n\n" +
		sectionNote(app) +
		"# Forma de trabajar del departamento (" + doc.Role + ")\n\n" + dept + "\n"
	return agentsRelease{doc: doc, body: body, id: bubble.Fingerprint(body)}, nil
}

// sectionNote trae la sección vigente que el prompt de conexión instala, para
// que un agente con una vieja la reemplace sin que nadie vuelva a pegar nada.
// Sólo REEMPLAZA: un agente sin esa sección (pi la inyecta de otra forma) no
// tiene por qué escribirla.
func sectionNote(app core.App) string {
	sec := bubble.ConnectSection(app)
	if sec == "" {
		return ""
	}
	v := bubble.ConnectVersion(app)
	return "## Tu sección «Bubble Work» (vigente: `" + v + "`)\n\n" +
		"Si tus instrucciones del proyecto (`CLAUDE.md`, `AGENTS.md`, `.cursorrules`…) tienen una sección «Bubble Work» " +
		"que no marca `bubble-work connect " + v + "`, reemplázala COMPLETA por esta, conservando el proyecto que ya " +
		"tenía en lugar de `<SLUG>`. Si no tienen ninguna, no la agregues por tu cuenta. Si tu configuración del MCP " +
		"lleva la cabecera `" + bubble.ConnectHeader + "`, ponle `" + v + "` (surte efecto al reconectar).\n\n" +
		"```markdown\n" + sec + "\n```\n\n---\n\n"
}

// agentsHead es la primera línea de `agents_md`.
func agentsHead(r agentsRelease) string {
	if r.doc.Version == "" {
		return "AGENTS.md (" + r.doc.Role + ") · release " + r.id + " · el departamento todavía no ha escrito ninguna versión"
	}
	return "AGENTS.md (" + r.doc.Role + ") · release " + r.id + " · versión " + r.doc.Version + " · " +
		r.doc.Created + " · por " + r.doc.Author
}

// clientOf es «nombre versión» del `clientInfo` que mandó el cliente al
// conectarse, si esta sesión lo tiene.
func clientOf(s mcp.Session) string {
	ss, ok := s.(*mcp.ServerSession)
	if !ok || ss == nil {
		return ""
	}
	p := ss.InitializeParams()
	if p == nil || p.ClientInfo == nil {
		return ""
	}
	return strings.TrimSpace(p.ClientInfo.Name + " " + p.ClientInfo.Version)
}

// instructionsMiddleware pone las `instructions` de quien se conecta en la
// respuesta de `initialize` (y de `discover`, el protocolo nuevo), y anota su
// cliente.
//
// Por persona y no fijas en el servidor: la release depende del rol, y el
// servidor es uno para todos. Rechazado: un servidor por rol, que duplicaba la
// reconstrucción en cada guardado para cambiar una línea.
func instructionsMiddleware(tb *toolbox) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil || (method != "initialize" && method != "discover") {
				return res, err
			}
			c, cerr := from(ctx)
			if cerr != nil {
				return res, err
			}
			bubble.NoteAgentClient(c.app, c.token, clientOf(req.GetSession()))
			r, rerr := releaseFor(c.app, c.auth, "", tb.entries)
			if rerr != nil {
				return res, err
			}
			text := bubble.Instructions(c.app, r.doc.Role, r.id)
			switch v := res.(type) {
			case *mcp.InitializeResult:
				v.Instructions = text
			case *mcp.DiscoverResult:
				v.Instructions = text
			}
			return res, err
		}
	}
}

// releases son las releases vigentes, para que Ajustes → Tokens compare lo
// que leyó cada agente con lo que hay.
func releases(app core.App, auth *core.Record, entries []toolEntry) map[string]string {
	out := map[string]string{"connect": bubble.ConnectVersion(app)}
	for _, role := range []string{"operators", "planner"} {
		if r, err := releaseFor(app, auth, role, entries); err == nil {
			out[role] = r.id
		}
	}
	return out
}
