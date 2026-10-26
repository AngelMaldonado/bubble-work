package bubble

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Releases de lo que lee un agente, y lo que el servidor ve que tiene cada uno.
//
// Tres versiones, todas huellas del texto vigente, así que no cambian al
// reiniciar y cambian con cualquier edición en Prompts:
//
//   - la RELEASE de `agents_md`: la huella de lo que devuelve para un rol (guía,
//     tabla de tools, markdown, sección y departamento). La calcula `mcpapi`,
//     que es quien tiene la tabla de tools;
//   - la de la SECCIÓN que el prompt de conexión instala en el `CLAUDE.md` /
//     `AGENTS.md` del agente (`ConnectVersion`). Va marcada en la sección y en
//     la cabecera `X-Bubble-Connect` de su configuración;
//   - `instructions`, que no tiene versión propia: dice las otras dos.
//
// Rechazado: un número que sube al guardar. `promptGen` ya existe, pero vive en
// memoria y vuelve a cero con cada arranque, y un número correlativo tendría que
// guardarse aparte para no repetirse. Una huella no se guarda: se recalcula.
// Rechazado también: borrador y «publicar». Guardar en Prompts sigue siendo
// publicar, como antes; esto sólo lo hace visible.

// AgentTokenKey es donde `agentTokenAuth` deja el id del token de agente que
// abrió esta petición; vacío si entró con una sesión del navegador.
const AgentTokenKey = "bubbleAgentToken"

// ConnectHeader es la cabecera con la que un agente dice con qué versión del
// prompt de conexión se instaló.
const ConnectHeader = "X-Bubble-Connect"

// InstructionsLimit: Claude Code corta las `instructions` de un servidor MCP a
// 2 048 caracteres (UTF-16, como cuenta JavaScript), sin avisar.
const InstructionsLimit = 2048

// Fingerprint es la huella corta de un texto.
func Fingerprint(s string) string { return fingerprint(s) }

// sectionOf es la sección que el agente se instala: el primer bloque
// ```markdown del prompt de conexión. Vacío si el lead lo quitó.
func sectionOf(tpl string) string {
	const open, close = "```markdown\n", "\n```"
	i := strings.Index(tpl, open)
	if i < 0 {
		return ""
	}
	rest := tpl[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// ConnectVersion es la versión de la sección que instala el prompt de conexión:
// la huella de la sección SIN rellenar. Editar la prosa de alrededor del prompt
// no envejece ninguna instalación; editar la sección sí. Sin sección, la huella
// del prompt entero.
func ConnectVersion(app core.App) string {
	tpl := SystemPrompt(app, "connect")
	if sec := sectionOf(tpl); sec != "" {
		return fingerprint(sec)
	}
	return fingerprint(tpl)
}

// ConnectText es el prompt de conexión con su versión puesta. `{{url}}` y
// `{{token}}` se quedan: los pone el navegador.
func ConnectText(app core.App) string {
	return strings.ReplaceAll(SystemPrompt(app, "connect"), "{{version}}", ConnectVersion(app))
}

// ConnectSection es la sección vigente, ya con su versión, tal como el agente
// la escribe en sus instrucciones (con `<SLUG>` por llenar).
func ConnectSection(app core.App) string { return sectionOf(ConnectText(app)) }

func fillInstructions(tpl, role, release, connect string) string {
	return strings.NewReplacer("{{release}}", release, "{{role}}", role, "{{connect}}", connect).Replace(tpl)
}

// utf16Len cuenta como cuenta el cliente.
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// Instructions es el texto de `instructions` para un rol y su release. Si una
// edición se pasara del límite (la validación lo impide al guardar), se corta
// aquí en vez de dejar que lo corte el cliente a ciegas.
func Instructions(app core.App, role, release string) string {
	s := fillInstructions(SystemPrompt(app, "instructions"), role, release, ConnectVersion(app))
	if utf16Len(s) <= InstructionsLimit {
		return s
	}
	r := []rune(s)
	for utf16Len(string(r)) > InstructionsLimit-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// validateInstructions rechaza una edición que, rellenada con los valores más
// largos posibles, no cabría en el límite.
func validateInstructions(content string) error {
	long := strings.Repeat("f", 16)
	n := utf16Len(fillInstructions(content, "operators", long, long))
	if n > InstructionsLimit {
		return fmt.Errorf("las instructions miden %d caracteres ya rellenadas y el límite es %d: Claude Code corta lo demás sin avisar", n, InstructionsLimit)
	}
	return nil
}

// ---- lo que se ve de cada agente ----
//
// Escrituras directas, como `last_used_at`: guardar el registro dispararía
// hooks y tiempo real para decir lo mismo otra vez. Y sólo cuando cambia algo.

// NoteAgentClient guarda el `clientInfo` de un `initialize`.
func NoteAgentClient(app core.App, tokenID, client string) {
	client = strings.TrimSpace(client)
	if tokenID == "" || client == "" {
		return
	}
	if r := []rune(client); len(r) > 120 {
		client = string(r[:120])
	}
	_, _ = app.DB().NewQuery("UPDATE agent_tokens SET client = {:c} WHERE id = {:id} AND client IS NOT {:c}").
		Bind(dbx.Params{"c": client, "id": tokenID}).Execute()
}

// NoteAgentRelease guarda la release de `agents_md` que se le entregó.
func NoteAgentRelease(app core.App, tokenID, role, release string) {
	if tokenID == "" || release == "" {
		return
	}
	stamp, _ := types.ParseDateTime(time.Now())
	_, _ = app.DB().NewQuery("UPDATE agent_tokens SET release_read = {:r}, release_role = {:role}, release_read_at = {:t} WHERE id = {:id}").
		Bind(dbx.Params{"r": release, "role": role, "t": stamp.String(), "id": tokenID}).Execute()
}

// cleanVersion: una versión es corta y de letras, dígitos, `.`, `-` o `_`. Lo
// demás se descarta: es una cabecera que escribe un modelo.
func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 32 {
		return ""
	}
	for _, c := range v {
		ok := c == '.' || c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !ok {
			return ""
		}
	}
	return v
}

// noteConnectHeader guarda la versión de instalación que manda el agente.
func noteConnectHeader(app core.App, tok *core.Record, raw string) {
	v := cleanVersion(raw)
	if v == "" || v == tok.GetString("connect_version") {
		return
	}
	_, _ = app.DB().NewQuery("UPDATE agent_tokens SET connect_version = {:v} WHERE id = {:id}").
		Bind(dbx.Params{"v": v, "id": tok.Id}).Execute()
}
