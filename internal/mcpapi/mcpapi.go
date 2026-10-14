// Package mcpapi is the agent surface.
//
// MCP is the primary protocol for the operative layer: the people doing the work
// drive it through agents, so this is the door that matters most. It is deliberately
// THIN — every tool calls the same function the REST route calls, so the two
// surfaces cannot drift and neither can bypass a rule the other enforces.
package mcpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/bubble"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/tree"
)

// caller is who is asking, carried through the SDK's context because a tool
// handler never sees the HTTP request.
type caller struct {
	app  core.App
	auth *core.Record
	tree *tree.Tree
	// where this agent reached the server; what its links and scripts use
	base string
}

type ctxKey struct{}

func from(ctx context.Context) (*caller, error) {
	c, ok := ctx.Value(ctxKey{}).(*caller)
	if !ok || c.auth == nil {
		return nil, fmt.Errorf("sin sesión: manda tu token en Authorization")
	}
	return c, nil
}

// Register mounts the MCP endpoint.
//
// Authentication is PocketBase's own: an agent presents a person's token and
// therefore IS that person for every rule in the system. There is no agent
// identity to manage, and nothing an agent can reach that its person cannot.
func Register(app core.App, t *tree.Tree) {
	// The server carries the tool descriptions, and those are editable in the
	// app (Prompts). So it is rebuilt — once — whenever a prompt is saved, and
	// the next request already sees the new text. Stateless transport: nothing
	// lives in the old server worth keeping.
	//
	// The first build runs before the database is open: it only registers the
	// built-in descriptions as the originals (with no app, Prompt returns them).
	// The first request builds again, now reading the edits.
	srv := build()
	promptApp = app
	var (
		mu    sync.Mutex
		built = int64(-1)
	)
	current := func() *mcp.Server {
		mu.Lock()
		defer mu.Unlock()
		if g := bubble.PromptsGen(); g != built {
			srv, built = build(), g
		}
		return srv
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return current() },
		&mcp.StreamableHTTPOptions{
			// Stateless: PocketBase may run behind anything, and a session held in
			// one process is a session the next request may not find.
			Stateless: true,
			// The SDK refuses a loopback request whose Host header is not loopback —
			// correct DNS-rebinding protection, and wrong behind a reverse proxy,
			// where every legitimate request looks exactly like that. v0 met this as
			// "Forbidden: invalid Host header" with the web UI working fine and only
			// agents locked out. Deployment decides the boundary here, not the SDK.
			DisableLocalhostProtection: true,
		})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		mount := func(e *core.RequestEvent) error {
			ctx := context.WithValue(e.Request.Context(), ctxKey{},
				&caller{app: e.App, auth: e.Auth, tree: t, base: bubble.BaseURL(e.App, e.Request)})
			handler.ServeHTTP(e.Response, e.Request.WithContext(ctx))
			return nil
		}
		for _, m := range []string{"GET", "POST", "DELETE"} {
			se.Router.Route(m, "/mcp", mount).Bind(apis.RequireAuth())
		}
		return se.Next()
	})
}

// promptApp is where edited prompts are read from while building the server.
var promptApp core.App

// Las tools se nombran `entidad_operacion`, en inglés como sus parámetros;
// lo que un agente LEE —descripciones, guía, protocolos, salidas— va en
// español. Rechazado: nombres en español (`hilo_crear`), que dejaban tools y
// parámetros en dos idiomas distintos del JSON que devuelven, y una sola tool
// por entidad con un parámetro `operation`, que vuelve el esquema una unión y
// diluye cada descripción.

// toolKind dice qué hace una tool con el mundo. De aquí salen las anotaciones
// MCP (un cliente puede aprobar sólo las lecturas, o pedir confirmación antes
// de lo que borra) y la columna de la tabla de `agents_md`.
type toolKind int

const (
	reads    toolKind = iota // sólo lee
	adds                     // escribe algo nuevo; no pisa nada
	sets                     // cambia un valor; repetirla da lo mismo
	destroys                 // borra o reemplaza
)

// toolEntry es una fila de la tabla de tools que lee un agente.
type toolEntry struct {
	group, name, summary string
	kind                 toolKind
}

// toolGroups es el orden de las categorías en la tabla.
var toolGroups = []string{"Guía", "Equipos", "Personas y cambios", "Proyectos", "Módulos (burbujas)", "Hilos",
	"Documentos", "Comentarios", "Inbox (ideas)", "Artefactos temporales", "Plan y secuencia",
	"Mi espacio", "Pines", "Infraestructura"}

// toolbox junta lo que se registra en un servidor.
type toolbox struct {
	s       *mcp.Server
	entries []toolEntry
}

// add es mcp.AddTool con la descripción vigente (la del binario se registra
// como original; una edición hecha en Prompts la reemplaza), las anotaciones
// que corresponden a su tipo, y una fila en el catálogo.
func add[In, Out any](tb *toolbox, group string, kind toolKind, title string, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	bubble.RegisterToolPrompt(t.Name, t.Description)
	t.Description = bubble.Prompt(promptApp, "tool/"+t.Name, t.Description)
	closed := false
	a := &mcp.ToolAnnotations{Title: title, ReadOnlyHint: kind == reads, OpenWorldHint: &closed}
	if kind != reads {
		d := kind == destroys
		a.DestructiveHint = &d
		a.IdempotentHint = kind == sets
	}
	t.Title = title
	t.Annotations = a
	mcp.AddTool(tb.s, t, h)
	tb.entries = append(tb.entries, toolEntry{group: group, name: t.Name, summary: firstSentence(t.Description), kind: kind})
}

// firstSentence es lo que cabe en una fila de la tabla: la primera oración de
// la descripción vigente, así una edición en Prompts también cambia la tabla.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' && (i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\n') {
			s = s[:i+1]
			break
		}
	}
	if r := []rune(s); len(r) > 220 {
		s = string(r[:219]) + "…"
	}
	return s
}

// catalog es la tabla de tools por categoría. Se genera del registro, así que
// no puede nombrar una tool que no existe ni olvidar una nueva.
func catalog(entries []toolEntry) string {
	var b strings.Builder
	b.WriteString("## Tools por categoría\n\n")
	b.WriteString("Nombres y parámetros en inglés; lo que devuelven, en español o JSON. ")
	b.WriteString("**L** sólo lee · **E** escribe sin pisar nada · **C** cambia un valor (repetirla da lo mismo) · **B** borra o reemplaza.\n")
	mark := map[toolKind]string{reads: "L", adds: "E", sets: "C", destroys: "B"}
	for _, g := range toolGroups {
		first := true
		for _, e := range entries {
			if e.group != g {
				continue
			}
			if first {
				b.WriteString("\n### " + g + "\n\n| tool | para qué | |\n|---|---|---|\n")
				first = false
			}
			b.WriteString("| `" + e.name + "` | " + strings.ReplaceAll(e.summary, "|", "\\|") + " | " + mark[e.kind] + " |\n")
		}
	}
	return b.String()
}

func build() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "bubble-work",
		Version: "3",
	}, nil)
	tb := &toolbox{s: s}

	// La guía es también un PROMPT MCP, de los mismos bytes: no todos los
	// clientes listan prompts, pero los que sí la encuentran ahí.
	s.AddPrompt(&mcp.Prompt{
		Name:        "bubble-work",
		Description: "Cómo funciona Bubble Work: entidades, qué calienta, cómo se escribe.",
	}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Bubble Work",
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: bubble.SystemPrompt(promptApp, "agents_guide")},
			}},
		}, nil
	})

	type none struct{}

	// ---- Guía ----------------------------------------------------------------

	type agentsArg struct {
		Role string `json:"role,omitempty" jsonschema:"planner u operators; vacío es el de tu rol"`
	}
	add(tb, "Guía", reads, "Guía y forma de trabajar", &mcp.Tool{
		Name: "agents_md",
		Description: "Léela al empezar cada sesión: la guía de Bubble Work y la forma de trabajar del departamento. " +
			"Trae las entidades, el ciclo de trabajo, cómo se escribe, todas las tools por categoría y, al " +
			"final, lo que mantiene el lead global, que manda. Al lead le llega la de quien planea; a los demás, " +
			"la de quien opera; `role` lee la otra. Se lee en vivo: manda sobre lo que recuerdes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in agentsArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		doc, err := bubble.ReadAgents(c.app, c.auth, in.Role)
		if err != nil {
			return nil, nil, err
		}
		// Un solo documento: la guía (editable en Prompts), la tabla de tools
		// (generada del registro), cómo se escribe markdown, y al final la forma
		// de trabajar del departamento, que manda si contradice lo anterior.
		head := fmt.Sprintf("AGENTS.md (%s) · todavía no hay ninguna versión", doc.Role)
		dept := "El lead todavía no ha escrito la forma de trabajar de este rol. Sigue la guía y el `README.md` de cada proyecto."
		if doc.Version != "" {
			head = fmt.Sprintf("AGENTS.md (%s) · versión %s · %s · por %s", doc.Role, doc.Version, doc.Created, doc.Author)
			dept = doc.Content
		}
		return text(head + "\n\n" +
			bubble.SystemPrompt(c.app, "agents_guide") + "\n\n" +
			catalog(tb.entries) + "\n---\n\n" +
			bubble.SystemPrompt(c.app, "markdown") + "\n\n---\n\n" +
			"# Forma de trabajar del departamento (" + doc.Role + ")\n\n" + dept + "\n"), nil, nil
	})

	// ---- Proyectos -------------------------------------------------------------

	type projArg struct {
		Project string `json:"project" jsonschema:"el id o el slug del proyecto"`
	}

	add(tb, "Proyectos", reads, "Listar proyectos", &mcp.Tool{
		Name: "project_list",
		Description: "Lista los proyectos que puedes ver, con su id, su slug, su nombre y su equipo (el " +
			"contexto: id, nombre y si es personal).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		rows, err := c.app.FindAllRecords("workspaces")
		if err != nil {
			return nil, nil, err
		}
		var out []map[string]any
		for _, w := range rows {
			if _, _, err := bubble.WorkspaceFor(c.app, c.auth, w.Id); err != nil {
				continue
			}
			out = append(out, map[string]any{
				"id": w.Id, "slug": w.GetString("slug"), "name": w.GetString("name"),
				"team": bubble.TeamRefOf(c.app, w.GetString("team")),
			})
		}
		return jsonOut(out), nil, nil
	})

	type newProjectArg struct {
		Team string `json:"team,omitempty" jsonschema:"el equipo (id o nombre, team_list los dice); vacío = tu equipo por defecto"`
		Name string `json:"name"`
		Slug string `json:"slug,omitempty" jsonschema:"su dirección: letras, dígitos y guiones. Si lo omites sale del nombre"`
	}
	add(tb, "Proyectos", adds, "Crear proyecto", &mcp.Tool{
		Name: "project_create",
		Description: "Funda un proyecto: la frontera de un cuerpo de trabajo, con su propio repositorio " +
			"git. Quedas como su lead, y empieza con un flujo de estados para que un hilo tenga dónde " +
			"estar. Nace en un EQUIPO (`team`): pregúntale a la persona en cuál si tiene más de uno. El slug " +
			"es su dirección en disco y en cada enlace, y no cambia nunca. Antes de crear uno sobre algo " +
			"que no se ha hablado, ofrece un interrogatorio.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in newProjectArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, err := bubble.CreateWorkspace(c.app, c.auth, in.Name, in.Slug, in.Team)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": ws.Id, "name": ws.GetString("name"), "slug": ws.GetString("slug"),
			"team": bubble.TeamRefOf(c.app, ws.GetString("team")),
		}), nil, nil
	})

	add(tb, "Proyectos", reads, "Tablero del proyecto", &mcp.Tool{
		Name: "project_board",
		Description: "Qué flota en un proyecto y por qué: sus módulos (`bubbles`) con su banda, y sus " +
			"hilos con calor, flotabilidad y prioridad. Nada de esto se guarda: se calcula ahora.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in projArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		b, err := bubble.BoardFor(c.app, ws)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(b), nil, nil
	})

	add(tb, "Proyectos", reads, "Repositorios del proyecto", &mcp.Tool{
		Name: "repo_list",
		Description: "Dónde vive el CÓDIGO del proyecto: los repositorios de GitHub enlazados a él. No es " +
			"el árbol de markdown del proyecto (eso es `document_list`). Cada uno trae su `id`; enlazar o " +
			"quitar uno es `repo_link` / `repo_unlink`, de un lead.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in projArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Repos(c.app, ws)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	// ---- Módulos ---------------------------------------------------------------

	type newBubbleArg struct {
		Project string `json:"project" jsonschema:"el id o el slug del proyecto"`
		Name    string `json:"name"`
		Outcome string `json:"outcome,omitempty" jsonschema:"qué es CIERTO cuando esto termine"`
	}
	add(tb, "Módulos (burbujas)", adds, "Crear módulo", &mcp.Tool{
		Name: "bubble_create",
		Description: "Crea un módulo (una burbuja): agrupa trabajo relacionado y es la unidad de atención. " +
			"Dale un resultado esperado (`outcome`): un módulo sin él es una carpeta con nombre bonito. " +
			"Antes de crear uno sobre algo que no se ha hablado, ofrece un interrogatorio.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in newBubbleArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		b, err := bubble.CreateBubble(c.app, c.auth, ws, in.Name, in.Outcome)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": b.Id, "name": b.GetString("name"), "project": ws.GetString("slug"),
			"team": bubble.TeamRefOf(c.app, ws.GetString("team"))}), nil, nil
	})

	bubbleOut := func(app core.App, b *core.Record) *mcp.CallToolResult {
		return jsonOut(map[string]any{
			"id": b.Id, "name": b.GetString("name"), "outcome": b.GetString("outcome"),
			"owners":    b.GetStringSlice("owners"),
			"objective": b.GetString("objective"),
			"impact":    b.GetString("impact"), "urgency": b.GetString("urgency"),
			"due_date": b.GetDateTime("due_date").String(),
			"closed":   !b.GetDateTime("closed_at").IsZero(),
			"closure":  b.GetString("closure"),
			"team":     bubble.WorkspaceTeam(app, b.GetString("workspace")),
		})
	}

	// Plano, no embebido: el esquema que infiere este SDK no aplana un struct
	// anónimo, y la mitad de los campos nunca llegaba a la tool — se aceptaban,
	// se ignoraban y se contestaba con el registro sin cambios.
	type setBubbleArg struct {
		Bubble    string    `json:"bubble" jsonschema:"el id del módulo"`
		Name      *string   `json:"name,omitempty"`
		Outcome   *string   `json:"outcome,omitempty" jsonschema:"qué es CIERTO cuando esto termine"`
		Owners    *[]string `json:"owners,omitempty" jsonschema:"ids de personas (people_list); quien no ve el proyecto se SUMA a él si puedes invitar (lead del proyecto o del equipo); vacío es que nadie responde"`
		Objective *string   `json:"objective,omitempty" jsonschema:"el id del objetivo al que sirve; vacío lo desarchiva"`
		Impact    *string   `json:"impact,omitempty" jsonschema:"high, mid o low"`
		Urgency   *string   `json:"urgency,omitempty" jsonschema:"high, mid o low"`
		Due       *string   `json:"due,omitempty" jsonschema:"un día, 2026-09-15: cuándo tiene que estar este cuerpo de trabajo; vacío la quita"`
	}
	add(tb, "Módulos (burbujas)", sets, "Actualizar módulo", &mcp.Tool{
		Name: "bubble_update",
		Description: "Cambia lo que se dice de un módulo: nombre, resultado esperado, quién responde, " +
			"a qué objetivo sirve, cuándo vence, impacto y urgencia. Que nadie responda es una " +
			"respuesta válida y tiene costo: un módulo callado sin responsable es una tumba, no una " +
			"siesta. La prioridad NO va aquí: el servidor la deriva de impacto × urgencia. Para " +
			"cerrarlo, `bubble_close`.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setBubbleArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		b, err := bubble.SetBubble(c.app, c.auth, in.Bubble, bubble.BubbleEdit{
			Name: in.Name, Outcome: in.Outcome, Owners: in.Owners,
			Objective: in.Objective, Impact: in.Impact, Urgency: in.Urgency, Due: in.Due,
		})
		if err != nil {
			return nil, nil, err
		}
		return bubbleOut(c.app, b), nil, nil
	})

	type closeBubbleArg struct {
		Bubble  string `json:"bubble" jsonschema:"el id del módulo"`
		Closure string `json:"closure" jsonschema:"cómo terminó, en una frase"`
	}
	add(tb, "Módulos (burbujas)", sets, "Cerrar módulo", &mcp.Tool{
		Name: "bubble_close",
		Description: "Cierra un módulo con una frase que diga cómo terminó. Cerrar es una decisión con " +
			"fecha, no un borrado: dice que este trabajo ya no compite por atención, y `bubble_reopen` " +
			"lo deshace. Recomiéndalo cuando su resultado ya no justifica el trabajo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in closeBubbleArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Closure) == "" {
			return nil, nil, fmt.Errorf("di cómo terminó en `closure`: cerrar es una decisión, y se escribe")
		}
		yes := true
		b, err := bubble.SetBubble(c.app, c.auth, in.Bubble, bubble.BubbleEdit{Closure: &in.Closure, Closed: &yes})
		if err != nil {
			return nil, nil, err
		}
		return bubbleOut(c.app, b), nil, nil
	})

	type bubbleArg struct {
		Bubble string `json:"bubble" jsonschema:"el id del módulo"`
	}
	add(tb, "Módulos (burbujas)", sets, "Reabrir módulo", &mcp.Tool{
		Name:        "bubble_reopen",
		Description: "Reabre un módulo cerrado. Su cierre queda en la historia.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in bubbleArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		no := false
		b, err := bubble.SetBubble(c.app, c.auth, in.Bubble, bubble.BubbleEdit{Closed: &no})
		if err != nil {
			return nil, nil, err
		}
		return bubbleOut(c.app, b), nil, nil
	})

	// ---- Hilos -----------------------------------------------------------------

	type threadArg struct {
		Thread string `json:"thread" jsonschema:"el id del hilo"`
	}

	type newThreadArg struct {
		Project string `json:"project" jsonschema:"el id o el slug del proyecto"`
		Name    string `json:"name"`
		Bubble  string `json:"bubble,omitempty" jsonschema:"el id del módulo que lo lleva; sin él queda suelto"`
	}
	add(tb, "Hilos", adds, "Crear hilo", &mcp.Tool{
		Name: "thread_create",
		Description: "Crea un hilo: una unidad ejecutable de trabajo. Antes, busca (`document_search`, " +
			"`project_board`): un hilo existente gana a uno nuevo, y abrir otro al lado del que estaba a " +
			"medias es cómo se pierde el contexto. Necesita un nombre; su documento se escribe aparte " +
			"(`document_replace` la primera vez), con la forma que tenga el trabajo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in newThreadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		th, err := bubble.CreateThread(c.app, c.auth, ws, in.Name, in.Bubble)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": th.Id, "seq": th.GetInt("seq"), "name": th.GetString("name"),
			"doc_path": th.GetString("doc_path"), "project": ws.GetString("slug"),
			"team": bubble.TeamRefOf(c.app, ws.GetString("team")),
		}), nil, nil
	})

	type setThreadArg struct {
		Thread    string    `json:"thread" jsonschema:"el id del hilo"`
		Name      *string   `json:"name,omitempty" jsonschema:"renombrar mueve su archivo, y git lo sigue"`
		Bubble    *string   `json:"bubble,omitempty" jsonschema:"el id de un módulo del mismo proyecto; vacío lo deja suelto"`
		Priority  *string   `json:"priority,omitempty" jsonschema:"P1, P2, P3 o P4: la prioridad propia de esta pieza, aparte de la de su módulo; vacío la quita. Sólo un lead del equipo"`
		Assignees *[]string `json:"assignees,omitempty" jsonschema:"ids de personas (people_list); quien no ve el proyecto se SUMA a él si puedes invitar (lead del proyecto o del equipo); vacío lo deja sin asignar"`
		Due       *string   `json:"due,omitempty" jsonschema:"un día, 2026-09-15: el plazo propio de esta pieza; vacío lo quita"`
		State     *string   `json:"state,omitempty" jsonschema:"el nombre de un estado del proyecto (thread_get los dice); a uno completado es terminarlo"`
		Labels    *[]string `json:"labels,omitempty" jsonschema:"nombres de etiquetas; las que no existen en el proyecto se crean; vacío las quita"`
	}
	add(tb, "Hilos", sets, "Actualizar hilo", &mcp.Tool{
		Name: "thread_update",
		Description: "Cambia lo de un hilo que no es su documento: nombre, módulo, asignados, plazo, estado, etiquetas y prioridad. " +
			"La prioridad propia (P1…P4) va aparte de la de su módulo y sólo la pone un lead del equipo del " +
			"proyecto; si no lo eres, proponla en un comentario. Para qué es y cuándo vence son del MÓDULO " +
			"(`bubble_update`). Su lugar en la secuencia lo pone un lead del equipo (`sequence_set`).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setThreadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		th, err := bubble.SetThread(c.app, c.auth, in.Thread, bubble.ThreadEdit{
			Name: in.Name, Bubble: in.Bubble, Priority: in.Priority,
			Assignees: in.Assignees, Due: in.Due, State: in.State, Labels: in.Labels,
		})
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": th.Id, "seq": th.GetInt("seq"), "name": th.GetString("name"),
			"doc_path": th.GetString("doc_path"), "bubble": th.GetString("bubble"),
			"priority": th.GetString("priority"),
			"team":     bubble.WorkspaceTeam(c.app, th.GetString("workspace")),
		}), nil, nil
	})

	add(tb, "Hilos", sets, "Completar hilo", &mcp.Tool{
		Name: "thread_complete",
		Description: "Pasa un hilo al estado completado de su proyecto. Terminar es una postura que alguien " +
			"toma, no una lista que el servidor califica: antes, deja escrito el resultado y agrega la " +
			"evidencia. Si quedó a medias, dilo en el documento y déjalo abierto.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in threadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		th, err := bubble.CompleteThread(c.app, c.auth, in.Thread)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": th.Id, "state": th.GetString("state")}), nil, nil
	})

	type historyArg struct {
		Thread string `json:"thread" jsonschema:"el id del hilo"`
		Limit  int    `json:"limit,omitempty" jsonschema:"cuántos, del más nuevo al más viejo (50 si lo omites)"`
	}
	add(tb, "Hilos", reads, "Historial del hilo", &mcp.Tool{
		Name: "thread_history",
		Description: "Lo que ya le pasó a un hilo, de lo más nuevo a lo más viejo. El calor dice que está " +
			"caliente; esto dice QUÉ lo calentó y cuándo. Léelo antes de empezar, o harás algo que se " +
			"terminó el martes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in historyArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Timeline(c.app, c.auth, in.Thread, in.Limit)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type evidenceArg struct {
		Thread string `json:"thread" jsonschema:"el id del hilo"`
		URL    string `json:"url"`
		Title  string `json:"title,omitempty"`
	}
	add(tb, "Hilos", adds, "Agregar evidencia", &mcp.Tool{
		Name: "evidence_add",
		Description: "Pega a un hilo evidencia externa: un commit, un PR, algo publicado. Es la prueba de " +
			"que la realidad cambió fuera de esta herramienta, y calienta el hilo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in evidenceArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		rec, err := bubble.AddLink(c.app, c.auth, in.Thread, in.URL, in.Title)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": rec.Id, "url": rec.GetString("url")}), nil, nil
	})

	// ---- Documentos ------------------------------------------------------------

	add(tb, "Documentos", reads, "Listar documentos", &mcp.Tool{
		Name:        "document_list",
		Description: "Los archivos de un proyecto: `README.md`, `threads/`, `docs/` (la wiki) y `assets/`.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in projArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		_, repo, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		entries, err := c.tree.Tree(repo)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(entries), nil, nil
	})

	type searchArg struct {
		Project string `json:"project" jsonschema:"el id o el slug del proyecto"`
		Query   string `json:"query" jsonschema:"el texto a buscar, sin distinguir mayúsculas"`
		Limit   int    `json:"limit,omitempty"`
		Around  int    `json:"around,omitempty" jsonschema:"líneas de contexto a cada lado de un resultado"`
	}
	add(tb, "Documentos", reads, "Buscar en documentos", &mcp.Tool{
		Name: "document_search",
		Description: "Busca texto en los documentos de un proyecto; los hilos salen primero. Cada resultado " +
			"trae su número de línea, que `document_read` acepta como `from`. `around` agrega unas líneas " +
			"de contexto: suelen bastar para citar en un `document_edit` sin leer el documento.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		_, repo, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		hits, err := c.tree.Search(repo, in.Query, in.Limit, in.Around)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(hits), nil, nil
	})

	type readArg struct {
		Project string `json:"project,omitempty" jsonschema:"obligatorio con path"`
		Path    string `json:"path,omitempty" jsonschema:"una ruta como docs/onboarding.md"`
		Thread  string `json:"thread,omitempty" jsonschema:"el id de un hilo, en vez de project+path"`
		From    int    `json:"from,omitempty" jsonschema:"primera línea a leer, contando desde 1"`
		Lines   int    `json:"lines,omitempty" jsonschema:"cuántas líneas desde ahí"`
	}
	add(tb, "Documentos", reads, "Leer documento", &mcp.Tool{
		Name: "document_read",
		Description: "Lee un documento, por id de hilo o por proyecto y ruta. Llega como el markdown mismo, " +
			"con una línea de cabecera antes: GUARDA EL HASH de esa línea, porque toda escritura lo manda " +
			"de vuelta como `base`. `from` y `lines` leen un trozo; sin ellos un documento largo llega " +
			"cortado y la cabecera lo dice.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in readArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, repo, path, _, err := locate(c, in.Thread, in.Project, in.Path)
		if err != nil {
			return nil, nil, err
		}
		doc, err := bubble.ReadDoc(c.app, c.tree, ws, repo, path)
		if err != nil {
			return nil, nil, err
		}
		return text(docText(doc, in.From, in.Lines)), nil, nil
	})

	// Las tres escrituras de un documento son tres tools: con una sola que
	// aceptaba «exactamente uno de content, edits o todo», mandar dos o
	// ninguno era el error más común.
	write := func(ctx context.Context, thread, project, path string, p bubble.Patch) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, repo, doc, subject, err := locate(c, thread, project, path)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Apply(c.app, c.auth, c.tree, ws, repo, doc, subject, p)
		if err != nil {
			return nil, nil, err
		}
		// La misma forma que `document_read`, con el hash NUEVO: encadenar dos
		// cambios es lo normal, y releer entre uno y otro sería una petición de
		// más para recuperar algo que ya está aquí.
		return text(docText(out, 0, 0)), nil, nil
	}

	type editArg struct {
		Project string        `json:"project,omitempty" jsonschema:"obligatorio con path"`
		Path    string        `json:"path,omitempty"`
		Thread  string        `json:"thread,omitempty" jsonschema:"el id de un hilo, en vez de project+path"`
		Base    string        `json:"base" jsonschema:"el hash que te dio document_read; obligatorio"`
		Edits   []bubble.Edit `json:"edits" jsonschema:"cambios quirúrgicos: cita old y da new; all reemplaza todas las apariciones"`
		Message string        `json:"message,omitempty" jsonschema:"el mensaje del commit"`
	}
	add(tb, "Documentos", adds, "Editar documento", &mcp.Tool{
		Name: "document_edit",
		Description: "Cambia un documento citando el texto: `edits: [{old, new}]` más el `base` que " +
			"leíste. Es la forma preferida de escribir. Si hay conflicto, relee y vuelve a aplicar: nunca " +
			"reintentes con la misma base.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in editArg) (*mcp.CallToolResult, any, error) {
		if len(in.Edits) == 0 {
			return nil, nil, fmt.Errorf("`edits` está vacío: no hay nada que cambiar")
		}
		return write(ctx, in.Thread, in.Project, in.Path, bubble.Patch{Base: in.Base, Message: in.Message, Edits: in.Edits})
	})

	type replaceArg struct {
		Project string `json:"project,omitempty" jsonschema:"obligatorio con path"`
		Path    string `json:"path,omitempty" jsonschema:"una ruta nueva crea el archivo"`
		Thread  string `json:"thread,omitempty" jsonschema:"el id de un hilo, en vez de project+path"`
		Base    string `json:"base" jsonschema:"el hash que te dio document_read (el de un documento vacío, si es nuevo); obligatorio"`
		Content string `json:"content" jsonschema:"el archivo completo"`
		Message string `json:"message,omitempty" jsonschema:"el mensaje del commit"`
	}
	add(tb, "Documentos", destroys, "Reemplazar documento", &mcp.Tool{
		Name: "document_replace",
		Description: "Reemplaza un documento completo, o crea uno nuevo. Para escribir uno nuevo o una reescritura " +
			"deliberada; para cambiar una parte usa `document_edit`, porque reemplazar todo para tocar una " +
			"línea destruye párrafos que no leíste.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in replaceArg) (*mcp.CallToolResult, any, error) {
		return write(ctx, in.Thread, in.Project, in.Path, bubble.Patch{Base: in.Base, Message: in.Message, Content: &in.Content})
	})

	type checkboxArg struct {
		Project string `json:"project,omitempty" jsonschema:"obligatorio con path"`
		Path    string `json:"path,omitempty"`
		Thread  string `json:"thread,omitempty" jsonschema:"el id de un hilo, en vez de project+path"`
		Base    string `json:"base" jsonschema:"el hash que te dio document_read; obligatorio"`
		Index   int    `json:"index" jsonschema:"qué casilla: 0 es la primera del documento"`
		Text    string `json:"text,omitempty" jsonschema:"el texto de la casilla, como protección por si el documento cambió"`
		Done    bool   `json:"done" jsonschema:"true la marca, false la desmarca"`
		Message string `json:"message,omitempty" jsonschema:"el mensaje del commit"`
	}
	add(tb, "Documentos", sets, "Marcar casilla", &mcp.Tool{
		Name: "checkbox_toggle",
		Description: "Marca o desmarca una casilla `- [ ]` de un documento, por su índice. Es la evidencia " +
			"más pequeña que puede dar un hilo: márcala cuando el paso se cumplió de verdad.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkboxArg) (*mcp.CallToolResult, any, error) {
		return write(ctx, in.Thread, in.Project, in.Path, bubble.Patch{Base: in.Base, Message: in.Message,
			Todo: &bubble.Todo{Index: in.Index, Text: in.Text, Done: in.Done}})
	})

	type dropArg struct {
		Project string `json:"project" jsonschema:"el id o el slug del proyecto"`
		Path    string `json:"path" jsonschema:"una página de docs/, u otro archivo de un hilo"`
	}
	add(tb, "Documentos", destroys, "Eliminar documento", &mcp.Tool{
		Name: "document_delete",
		Description: "Elimina un documento: una página de la wiki u otro archivo de la carpeta de un hilo. " +
			"El documento PROPIO de un hilo no se elimina así: se va cuando se va el hilo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in dropArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, repo, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.RemovePath(c.app, c.auth, c.tree, ws, repo, in.Path); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"path": in.Path, "deleted": true}), nil, nil
	})

	// ---- Comentarios -----------------------------------------------------------

	add(tb, "Comentarios", reads, "Leer comentarios", &mcp.Tool{
		Name: "comment_list",
		Description: "Lo que se ha dicho sobre un hilo, del más viejo al más nuevo. Léelo antes de " +
			"contestar algo que ya se contestó.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in threadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Comments(c.app, c.auth, in.Thread)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type commentArg struct {
		Thread string `json:"thread" jsonschema:"el id del hilo"`
		Body   string `json:"body" jsonschema:"markdown"`
	}
	add(tb, "Comentarios", adds, "Comentar", &mcp.Tool{
		Name: "comment_create",
		Description: "Comenta en un hilo: es pulso, no evidencia. Un comentario es PULSO, nunca calor: mantiene el hilo fuera " +
			"de la tumba y NO lo despierta. Por eso sirve para decir qué encontraste, o que no encontraste " +
			"nada, sin fingir que produjiste evidencia.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in commentArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		r, err := bubble.Comment(c.app, c.auth, in.Thread, in.Body)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": r.Id}), nil, nil
	})

	// ---- Artefactos temporales ---------------------------------------------------

	type createArtArg struct {
		Kind      string `json:"kind" jsonschema:"grill (interrogatorio) o markdown (documento efímero)"`
		Topic     string `json:"topic,omitempty" jsonschema:"de qué trata; crea la nota del inbox de la que cuelga"`
		InboxItem string `json:"inbox_item,omitempty" jsonschema:"una nota tuya del inbox de la que colgarlo, en vez de topic"`
		Team      string `json:"team,omitempty" jsonschema:"el equipo (id o nombre, team_list los dice); vacío = tu equipo por defecto"`
	}
	add(tb, "Artefactos temporales", adds, "Crear artefacto", &mcp.Tool{
		Name: "artifact_create",
		Description: "Abre un artefacto temporal: `grill` para interrogar con preguntas estructuradas, " +
			"`markdown` para publicar un `.md` que la persona lee en Bubble. Cuelga de una nota del inbox " +
			"(creada desde `topic`, o una tuya existente). No calienta nada. Devuelve su id, la url que le " +
			"das a la persona y el protocolo completo de ese tipo: síguelo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createArtArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		art, err := bubble.ArtifactStart(c.app, c.auth, in.Kind, in.InboxItem, in.Topic, in.Team)
		if err != nil {
			return nil, nil, err
		}
		v, err := bubble.ArtifactRead(c.app, c.auth, art.Id, 0)
		if err != nil {
			return nil, nil, err
		}
		out := jsonOut(bubble.WithLinks(c.app, v, c.base))
		// El protocolo llega justo cuando se usa, en un segundo bloque: el
		// primero sigue siendo el JSON de siempre, y una sesión que nunca abre
		// un artefacto no carga mil palabras que no va a leer.
		key := "grill"
		if art.GetString("kind") == "markdown" {
			key = "ephemeral_markdown"
		}
		out.Content = append(out.Content, &mcp.TextContent{Text: bubble.SystemPrompt(c.app, key)})
		return out, nil, nil
	})

	type entryIn struct {
		Type string         `json:"type" jsonschema:"grill: question | answer | note; markdown: document"`
		Data map[string]any `json:"data"`
	}
	type appendArg struct {
		ID      string    `json:"id"`
		Entries []entryIn `json:"entries" jsonschema:"en orden"`
	}
	add(tb, "Artefactos temporales", adds, "Agregar al artefacto", &mcp.Tool{
		Name: "artifact_append",
		Description: "Agrega entradas a un artefacto abierto, en orden. Un interrogatorio acepta `question` " +
			"(una ronda completa de una vez), `answer` (una que la persona dio en la terminal: regístrala " +
			"para que no haya huecos) y `note`. Nada se edita: una pregunta no se reescribe, y una " +
			"respuesta posterior reemplaza el veredicto anterior. Un markdown acepta `document` con " +
			"{filename, markdown, base}; cada uno es una revisión inmutable. Devuelve `watch_command` " +
			"para esperar respuestas.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appendArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		entries := make([]bubble.Entry, 0, len(in.Entries))
		for _, e := range in.Entries {
			entries = append(entries, bubble.Entry{Type: e.Type, Data: e.Data})
		}
		written, err := bubble.ArtifactAppend(c.app, c.auth, in.ID, entries, "harness")
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"appended": len(written), "last_seq": written[len(written)-1].Seq}
		if v, err := bubble.ArtifactRead(c.app, c.auth, in.ID, 0); err == nil {
			l := bubble.WithLinks(c.app, v, c.base)
			out["url"] = l.URL
			if l.WatchCommand != "" {
				out["watch_command"] = l.WatchCommand
			}
		}
		return jsonOut(out), nil, nil
	})

	type readArtArg struct {
		ID    string `json:"id"`
		Since int    `json:"since,omitempty" jsonschema:"sólo las entradas después de este seq; las preguntas llegan siempre completas"`
	}
	add(tb, "Artefactos temporales", reads, "Leer artefacto", &mcp.Tool{
		Name: "artifact_read",
		Description: "Lee un artefacto: sus preguntas con respuesta, o su documento vigente. De un interrogatorio: cada pregunta con su última respuesta " +
			"(`accepted` = se queda la recomendación, `override` = el texto de la persona) y su estado " +
			"(`answered`, `frontier` = se puede preguntar ya, `blocked`). De un markdown: el documento " +
			"vigente {filename, markdown, seq}, aunque `since` excluya su entrada.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in readArtArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		v, err := bubble.ArtifactRead(c.app, c.auth, in.ID, in.Since)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(bubble.WithLinks(c.app, v, c.base)), nil, nil
	})

	type waitArg struct {
		ID      string `json:"id"`
		Since   int    `json:"since,omitempty" jsonschema:"el último seq que viste (last_seq de tu append)"`
		Timeout int    `json:"timeout,omitempty" jsonschema:"segundos de espera, máximo 45 (45 si lo omites)"`
	}
	add(tb, "Artefactos temporales", reads, "Esperar respuestas", &mcp.Tool{
		Name: "artifact_wait",
		Description: "Espera a que la persona conteste en Bubble (pulse «Enviar»), hasta 45 s. Para " +
			"clientes SIN shell (apps de escritorio): llámala otra vez mientras diga `waiting`. Con " +
			"shell, mejor corre `watch_command` (en Claude Code, con la tool Monitor) y termina tu turno.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in waitArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if _, err := bubble.ArtifactRead(c.app, c.auth, in.ID, 0); err != nil {
			return nil, nil, err
		}
		secs := in.Timeout
		if secs < 1 || secs > 45 {
			secs = 45
		}
		got, err := bubble.WaitAnswers(ctx, c.app, in.ID, in.Since, time.Duration(secs)*time.Second)
		if err != nil {
			return nil, nil, err
		}
		if got.Closed {
			return jsonOut(map[string]any{"closed": true, "seq": got.Seq}), nil, nil
		}
		if len(got.Questions) == 0 {
			return jsonOut(map[string]any{"waiting": true, "seq": got.Seq,
				"next": "vuelve a llamar artifact_wait con el mismo since"}), nil, nil
		}
		return jsonOut(map[string]any{"answered": got.Questions, "seq": got.Seq,
			"next": "artifact_read para leer las respuestas; después, la siguiente ronda"}), nil, nil
	})

	type handoffArg struct {
		ID      string          `json:"id"`
		Targets []bubble.Target `json:"targets" jsonschema:"a dónde fue el trabajo: [{kind: thread|bubble, id}]"`
	}
	add(tb, "Artefactos temporales", sets, "Entregar artefacto", &mcp.Tool{
		Name: "artifact_handoff",
		Description: "Cierra un artefacto cuando su trabajo ya vive en hilos o módulos que escribiste. " +
			"Escríbelos ANTES para que se sostengan solos (decisiones con lo que descartaron, preguntas " +
			"abiertas, primeros pasos), porque nada apunta de vuelta y el artefacto caducará. Tría su nota " +
			"del inbox al primer hilo/módulo. Después queda de sólo lectura.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in handoffArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		v, err := bubble.ArtifactHandoff(c.app, c.auth, in.ID, in.Targets)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(v), nil, nil
	})

	// ---- Plan y secuencia ------------------------------------------------------

	type nextArg struct {
		Team string `json:"team,omitempty" jsonschema:"el equipo (id o nombre, team_list los dice); vacío = tu equipo por defecto"`
	}
	add(tb, "Plan y secuencia", reads, "Qué sigue", &mcp.Tool{
		Name: "sequence_next",
		Description: "Qué toca ahora en la secuencia de un equipo, y cuál es el primer hilo tuyo. Cada equipo tiene UNA secuencia de ejecución, puesta por el lead " +
			"del equipo: `now` es el primer paso con hilos abiertos (varios = van en paralelo), `then` el paso " +
			"siguiente, y `mine` el primer hilo abierto de la secuencia que es tuyo: asignado a ti, o sin " +
			"asignar en un módulo del que respondes. Terminar un hilo hace avanzar la secuencia sola.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in nextArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Next(c.app, c.auth, in.Team)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Plan y secuencia", reads, "Listar objetivos", &mcp.Tool{
		Name: "objective_list",
		Description: "Los objetivos de los equipos que lideras: para qué es el trabajo, cada uno con su " +
			"equipo. Un objetivo se sirve con MÓDULOS de los proyectos de ese equipo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		p, err := bubble.ReadPlan(c.app, c.auth)
		if err != nil {
			return nil, nil, err
		}
		// El inbox tiene sus propias tools (`inbox_*`), que lo presentan como lo
		// que es —ideas, no trabajo—. Por aquí no viaja: el plan es para qué es el
		// trabajo, y mezclarlo con ideas sin decidir lo confundiría.
		return jsonOut(map[string]any{"objectives": p.Objectives}), nil, nil
	})

	type newObjectiveArg struct {
		Team    string `json:"team,omitempty" jsonschema:"el equipo (id o nombre, team_list los dice); vacío = tu equipo por defecto"`
		Name    string `json:"name"`
		Outcome string `json:"outcome,omitempty" jsonschema:"qué es CIERTO cuando se cumpla"`
		Due     string `json:"due_date,omitempty" jsonschema:"un día: 2026-12-31"`
	}
	add(tb, "Plan y secuencia", adds, "Crear objetivo", &mcp.Tool{
		Name: "objective_create",
		Description: "Crea un objetivo de un equipo: para qué es el trabajo. Es del EQUIPO, no de un proyecto: " +
			"los módulos de cualquiera de sus proyectos cuelgan del mismo. Sólo un lead del equipo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in newObjectiveArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		o, err := bubble.SetObjective(c.app, c.auth, "", in.Team, in.Name, in.Outcome, in.Due)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": o.Id, "name": o.GetString("name"), "outcome": o.GetString("outcome")}), nil, nil
	})

	type setObjectiveArg struct {
		Objective string `json:"objective" jsonschema:"el id del objetivo"`
		Name      string `json:"name,omitempty"`
		Outcome   string `json:"outcome,omitempty" jsonschema:"qué es CIERTO cuando se cumpla"`
		Due       string `json:"due_date,omitempty" jsonschema:"un día: 2026-12-31"`
	}
	add(tb, "Plan y secuencia", sets, "Actualizar objetivo", &mcp.Tool{
		Name: "objective_update",
		Description: "Cambia el nombre, el resultado esperado o la fecha de un objetivo; lo que omitas " +
			"queda igual. Sólo un lead de su equipo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setObjectiveArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if in.Objective == "" {
			return nil, nil, fmt.Errorf("falta `objective`; para crear uno, `objective_create`")
		}
		o, err := bubble.SetObjective(c.app, c.auth, in.Objective, "", in.Name, in.Outcome, in.Due)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": o.Id, "name": o.GetString("name"), "outcome": o.GetString("outcome")}), nil, nil
	})

	// ---- Infraestructura -------------------------------------------------------

	add(tb, "Infraestructura", reads, "Inventario", &mcp.Tool{
		Name: "inventory_list",
		Description: "Dónde vive lo que mantiene todo esto corriendo: servidores, dominios, servicios " +
			"contratados y licencias, con su proveedor, costo y fecha de renovación. Sólo si te dieron " +
			"acceso. `vault` es un ENLACE a donde se guarda una credencial, nunca la credencial: este " +
			"servidor no guarda secretos y no puede darte uno.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		items, err := bubble.Inventory(c.app, c.auth)
		if err != nil {
			return nil, nil, err
		}
		groups, err := bubble.InventoryGroups(c.app, c.auth)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"groups": groups, "items": items}), nil, nil
	})

	// ---- Equipos -------------------------------------------------------------------

	add(tb, "Equipos", reads, "Mis equipos", &mcp.Tool{
		Name: "team_list",
		Description: "Los equipos que ves y tu papel en cada uno (lead o member), con quién está en ellos. Un " +
			"equipo es un CONTEXTO: sus proyectos, objetivos, secuencia, calendario, inbox y canal de WhatsApp " +
			"son suyos y no se mezclan con los de otro. Un equipo personal sólo lo ve quien está en él. " +
			"`default` es a dónde va lo que no dice equipo. Crear equipos y cambiar quién está en ellos se " +
			"hace a mano en la aplicación.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ts, err := bubble.TeamsOf(c.app, c.auth)
		if err != nil {
			return nil, nil, err
		}
		out := []map[string]any{}
		for _, t := range ts {
			members := []map[string]string{}
			for _, m := range bubble.TeamMembersOf(c.app, t.ID) {
				members = append(members, map[string]string{"id": m.User, "name": m.Name, "role": m.Role})
			}
			out = append(out, map[string]any{"id": t.ID, "name": t.Name, "personal": t.Personal,
				"role": t.Role, "members": members})
		}
		return jsonOut(map[string]any{"teams": out, "default": bubble.DefaultTeam(c.app, c.auth)}), nil, nil
	})

	// ---- Personas y cambios ------------------------------------------------------

	type peopleArg struct {
		Project string `json:"project,omitempty" jsonschema:"sólo quienes ven este proyecto (id o slug)"`
		Team    string `json:"team,omitempty" jsonschema:"sólo quienes están en este equipo (id o nombre)"`
	}
	add(tb, "Personas y cambios", reads, "Personas", &mcp.Tool{
		Name: "people_list",
		Description: "Las personas activas: id, nombre y rol, y cuál eres tú (`me`). Con `project`, sólo " +
			"quienes ya VEN ese proyecto; con `team`, quienes están en ese equipo. Asignar a alguien que " +
			"no ve el proyecto lo SUMA a él (sólo si puedes invitar): hazlo sólo si la persona lo pide. " +
			"Administrar personas, roles y tokens se hace a mano en la aplicación.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in peopleArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.People(c.app, c.auth, in.Project, in.Team)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type changesArg struct {
		Since   string `json:"since,omitempty" jsonschema:"desde cuándo: un día (2026-10-12), una fecha RFC 3339 o cycle; vacío = últimas 24 h"`
		Project string `json:"project,omitempty" jsonschema:"sólo este proyecto (id o slug); vacío = todos los que ves"`
		Limit   int    `json:"limit,omitempty" jsonschema:"cuántos, del más nuevo al más viejo (200 si lo omites, máximo 500)"`
	}
	add(tb, "Personas y cambios", reads, "Qué cambió", &mcp.Tool{
		Name: "change_list",
		Description: "Lo que pasó en los proyectos que ves desde una fecha: hilos creados y completados, " +
			"documentos escritos, evidencia y comentarios, con quién y cuándo. `warms` dice si calentó. " +
			"Sirve para resúmenes y revisiones: lo que no está aquí no pasó en Bubble.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in changesArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Changes(c.app, c.auth, in.Since, in.Project, in.Limit)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	// ---- Hilos: la ficha y las relaciones -------------------------------------------

	add(tb, "Hilos", reads, "Ficha del hilo", &mcp.Tool{
		Name: "thread_get",
		Description: "La ficha de un hilo, todo lo que no es su documento: proyecto, módulo, estado (y los " +
			"estados posibles), asignados, etiquetas, fecha, prioridad, lugar en la secuencia, relaciones, " +
			"evidencia y si lo tienes fijado. Su documento se lee con `document_read`.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in threadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.ThreadGet(c.app, c.auth, in.Thread)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type relateArg struct {
		Thread  string `json:"thread" jsonschema:"el id del hilo"`
		Related string `json:"related" jsonschema:"el id del otro hilo"`
		Type    string `json:"type" jsonschema:"relates_to, duplicate, blocking (thread bloquea a related) o blocked_by (thread espera a related)"`
	}
	add(tb, "Hilos", adds, "Relacionar hilos", &mcp.Tool{
		Name: "thread_relate",
		Description: "Relaciona dos hilos: `relates_to`, `duplicate`, `blocking` o `blocked_by`. Una " +
			"dependencia se dice así, no con la secuencia (la secuencia es el orden, no la dependencia). " +
			"Cambiar una relación es quitarla (`thread_unrelate`) y volverla a poner.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in relateArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.ThreadRelate(c.app, c.auth, in.Thread, in.Related, in.Type)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type unrelateArg struct {
		Relation string `json:"relation" jsonschema:"el id de la relación (thread_get lo dice)"`
	}
	add(tb, "Hilos", destroys, "Quitar relación", &mcp.Tool{
		Name:        "thread_unrelate",
		Description: "Quita una relación entre dos hilos. El id sale de `thread_get`.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in unrelateArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.ThreadUnrelate(c.app, c.auth, in.Relation); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"relation": in.Relation, "deleted": true}), nil, nil
	})

	// ---- Inbox: ideas -------------------------------------------------------------

	type ideaArg struct {
		Idea string `json:"idea" jsonschema:"el id de la idea"`
	}
	type ideasArg struct {
		Team    string `json:"team,omitempty" jsonschema:"sólo las de este equipo (id o nombre); vacío = todas las que ves"`
		Source  string `json:"source,omitempty" jsonschema:"whatsapp, app o vacío para todas"`
		Decided bool   `json:"decided,omitempty" jsonschema:"true incluye las que ya se volvieron trabajo"`
	}
	add(tb, "Inbox (ideas)", reads, "Listar ideas", &mcp.Tool{
		Name: "inbox_list",
		Description: "Las ideas del inbox que puedes ver: las que capturaste y las de los equipos que " +
			"lideras, cada una con su EQUIPO (su contexto), incluido lo que llega por el WhatsApp de cada " +
			"equipo (`source: whatsapp`, `from` dice quién lo mandó). Son IDEAS, no trabajo: ni hilos ni " +
			"módulos, ni pendientes de nadie. No las ejecutes ni las cuentes como avance; vuélvelas trabajo " +
			"sólo si la persona lo decide, y en un proyecto de SU equipo. Por defecto, sólo las que nadie ha decidido.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ideasArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Ideas(c.app, c.auth, in.Team, in.Source, in.Decided)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Inbox (ideas)", reads, "Leer idea", &mcp.Tool{
		Name: "inbox_read",
		Description: "Una idea completa: su texto, su cuerpo (el mensaje entero, si vino por WhatsApp) y sus " +
			"archivos. Sigue siendo una idea: leerla no la vuelve trabajo.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ideaArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.IdeaRead(c.app, c.auth, in.Idea)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type captureArg struct {
		Team string `json:"team,omitempty" jsonschema:"el equipo (id o nombre, team_list los dice); vacío = tu equipo por defecto"`
		Note string `json:"note" jsonschema:"la idea, en una línea"`
		Body string `json:"body,omitempty" jsonschema:"el detalle, en markdown"`
	}
	add(tb, "Inbox (ideas)", adds, "Capturar idea", &mcp.Tool{
		Name: "inbox_capture",
		Description: "Anota una idea, a tu nombre, en el inbox de un EQUIPO del que eres parte: el contexto " +
			"al que pertenece (`team`; si no es obvio, pregunta). Es algo que surgió, no pertenece a ningún " +
			"hilo y nadie ha decidido hacer: lo que NO es trabajo todavía. No abras un hilo ni un módulo para " +
			"guardar una idea. Lo que sí es del hilo en curso va en su documento.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in captureArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.IdeaCapture(c.app, c.auth, in.Team, in.Note, in.Body)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type ideaEditArg struct {
		Idea string  `json:"idea" jsonschema:"el id de la idea"`
		Note *string `json:"note,omitempty" jsonschema:"la idea, en una línea"`
		Body *string `json:"body,omitempty" jsonschema:"el detalle, en markdown"`
	}
	add(tb, "Inbox (ideas)", sets, "Corregir idea", &mcp.Tool{
		Name:        "inbox_update",
		Description: "Corrige el texto o el detalle de una idea. Sigue siendo una idea.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ideaEditArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.IdeaUpdate(c.app, c.auth, in.Idea, in.Note, in.Body)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type triageArg struct {
		Idea   string `json:"idea" jsonschema:"el id de la idea"`
		Thread string `json:"thread,omitempty" jsonschema:"el hilo al que fue"`
		Bubble string `json:"bubble,omitempty" jsonschema:"el módulo al que fue"`
	}
	add(tb, "Inbox (ideas)", sets, "Anotar a dónde fue una idea", &mcp.Tool{
		Name: "inbox_triage",
		Description: "Anota que una idea se volvió trabajo: el hilo o módulo al que fue. Sólo después de que " +
			"la PERSONA lo decidió y de que ese trabajo existe (búscalo primero; si hace falta, créalo con " +
			"su confirmación). La nota no se borra: queda apuntando al trabajo. Sin thread ni bubble vuelve a " +
			"ser una idea sin decidir.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in triageArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.IdeaDecide(c.app, c.auth, in.Idea, in.Thread, in.Bubble)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Inbox (ideas)", destroys, "Descartar idea", &mcp.Tool{
		Name:        "inbox_delete",
		Description: "Descarta una idea del inbox. Sólo si la persona lo pide: una idea vieja no estorba a nadie.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ideaArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.IdeaDelete(c.app, c.auth, in.Idea); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"idea": in.Idea, "deleted": true}), nil, nil
	})

	// ---- Mi espacio ----------------------------------------------------------------

	type spaceArg struct {
		Note string `json:"note" jsonschema:"el id de la nota"`
	}
	colorHint := "color: un nombre de la paleta (" + bubble.NoteColorNames() + ") o #rrggbb"
	type spaceEditArg struct {
		Note    string  `json:"note,omitempty"`
		Title   *string `json:"title,omitempty" jsonschema:"hasta 150 caracteres"`
		Body    *string `json:"body,omitempty" jsonschema:"markdown"`
		Color   *string `json:"color,omitempty"`
		Project *string `json:"project,omitempty" jsonschema:"un proyecto que ves (id o slug) para la franja de color; vacío lo quita"`
		X       *int    `json:"x,omitempty" jsonschema:"posición en el lienzo, en px; el origen está en 0,0"`
		Y       *int    `json:"y,omitempty"`
		W       *int    `json:"w,omitempty" jsonschema:"ancho, 180 a 1200 (240 por omisión)"`
		H       *int    `json:"h,omitempty" jsonschema:"alto, 140 a 1600; 0 = automático"`
	}
	edit := func(in spaceEditArg) bubble.SpaceEdit {
		return bubble.SpaceEdit{Title: in.Title, Body: in.Body, Color: in.Color, Project: in.Project, X: in.X, Y: in.Y, W: in.W, H: in.H}
	}

	add(tb, "Mi espacio", reads, "Listar notas de Mi espacio", &mcp.Tool{
		Name: "space_note_list",
		Description: "Las notas de Mi espacio de tu persona: su lienzo PRIVADO de post-its (título, texto, " +
			"color, proyecto, posición y tamaño). Nadie más las ve, ni el lead global. No es trabajo: no " +
			"calienta, no tiene hilos y no entra al tablero.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Space(c.app, c.auth)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Mi espacio", adds, "Crear nota en Mi espacio", &mcp.Tool{
		Name: "space_note_create",
		Description: "Pega una nota nueva en Mi espacio. Necesita `title`; sin posición queda a la derecha " +
			"de las que ya hay. " + colorHint + ".",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in spaceEditArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.SpaceCreate(c.app, c.auth, edit(in))
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Mi espacio", sets, "Cambiar nota de Mi espacio", &mcp.Tool{
		Name: "space_note_update",
		Description: "Cambia una nota de Mi espacio: título, texto, color, proyecto, posición o tamaño; lo " +
			"que omitas queda igual. " + colorHint + ".",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in spaceEditArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.SpaceUpdate(c.app, c.auth, in.Note, edit(in))
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type arrangeArg struct {
		Notes []bubble.SpaceMove `json:"notes" jsonschema:"[{note, x, y, w?, h?}]: dónde queda cada una"`
	}
	add(tb, "Mi espacio", sets, "Acomodar Mi espacio", &mcp.Tool{
		Name: "space_note_arrange",
		Description: "Acomoda varias notas de Mi espacio de una vez (todas o ninguna): en columnas por " +
			"proyecto, en rejilla, alineadas… Tú calculas las posiciones; una nota mide 240 px de ancho por " +
			"omisión y unos 220 de alto, y 40 px de separación se leen bien.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in arrangeArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.SpaceArrange(c.app, c.auth, in.Notes)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Mi espacio", destroys, "Borrar nota de Mi espacio", &mcp.Tool{
		Name:        "space_note_delete",
		Description: "Borra una nota de Mi espacio, con lo que tenga adjunto. Sólo si la persona lo pide.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in spaceArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.SpaceDelete(c.app, c.auth, in.Note); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"note": in.Note, "deleted": true}), nil, nil
	})

	// ---- Pines ---------------------------------------------------------------------

	add(tb, "Pines", reads, "Hilos fijados", &mcp.Tool{
		Name: "pin_list",
		Description: "Los hilos que tu persona fijó: sus atajos personales en la barra, en orden. Son " +
			"privados; no son prioridad ni calor.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Pins(c.app, c.auth)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Pines", sets, "Fijar hilo", &mcp.Tool{
		Name:        "pin_add",
		Description: "Fija un hilo en la barra de tu persona (al final). Fijarlo dos veces no lo duplica.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in threadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.SetPin(c.app, c.auth, in.Thread, true)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	add(tb, "Pines", sets, "Soltar hilo", &mcp.Tool{
		Name:        "pin_remove",
		Description: "Quita un hilo de los fijados de tu persona. El hilo no cambia.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in threadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.SetPin(c.app, c.auth, in.Thread, false)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	// ---- Repositorios ----------------------------------------------------------------

	type repoLinkArg struct {
		Project string `json:"project" jsonschema:"el id o el slug del proyecto"`
		URL     string `json:"url" jsonschema:"la URL del repositorio, por ejemplo https://github.com/org/repo"`
		Name    string `json:"name,omitempty"`
	}
	add(tb, "Proyectos", adds, "Enlazar repositorio", &mcp.Tool{
		Name: "repo_link",
		Description: "Enlaza un repositorio de código a un proyecto. Sólo un lead del proyecto o el lead " +
			"global. Una URL equivocada manda a la gente al código equivocado sin que nada falle: confírmala " +
			"con la persona.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in repoLinkArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Project)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.RepoLink(c.app, c.auth, ws, in.URL, in.Name)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type repoArg struct {
		Repo string `json:"repo" jsonschema:"el id del enlace (repo_list lo dice)"`
	}
	add(tb, "Proyectos", destroys, "Quitar repositorio", &mcp.Tool{
		Name:        "repo_unlink",
		Description: "Quita un repositorio enlazado a un proyecto (el repositorio no se toca). Sólo un lead del proyecto o el lead global.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in repoArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.RepoUnlink(c.app, c.auth, in.Repo); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"repo": in.Repo, "deleted": true}), nil, nil
	})

	// ---- Artefactos: los míos, fijar, borrar -----------------------------------------

	type artListArg struct {
		All bool `json:"all,omitempty" jsonschema:"los del departamento; sólo el lead global"`
	}
	add(tb, "Artefactos temporales", reads, "Mis artefactos", &mcp.Tool{
		Name: "artifact_list",
		Description: "Tus artefactos (interrogatorios y markdown efímeros), los abiertos primero, con su " +
			"estado y cuándo caducan. Para retomar en otra sesión uno que dejaste abierto.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in artListArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		list, err := bubble.ArtifactsOf(c.app, c.auth, in.All)
		if err != nil {
			return nil, nil, err
		}
		out := make([]map[string]any, 0, len(list))
		for _, v := range list {
			l := bubble.WithLinks(c.app, v, c.base)
			out = append(out, map[string]any{"id": l.ID, "kind": l.Kind, "title": l.Title, "status": l.Status,
				"pinned": l.Pinned, "expires_at": l.ExpiresAt, "url": l.URL})
		}
		return jsonOut(out), nil, nil
	})

	type artPinArg struct {
		ID     string `json:"id"`
		Pinned bool   `json:"pinned" jsonschema:"true lo fija (no caduca), false lo suelta (vuelve a correr su plazo)"`
	}
	add(tb, "Artefactos temporales", sets, "Fijar artefacto", &mcp.Tool{
		Name:        "artifact_pin",
		Description: "Fija un artefacto para que no caduque, o lo suelta. Sólo si la persona lo pide.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in artPinArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.ArtifactPin(c.app, c.auth, in.ID, in.Pinned); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": in.ID, "pinned": in.Pinned}), nil, nil
	})

	type artIDArg struct {
		ID string `json:"id"`
	}
	add(tb, "Artefactos temporales", destroys, "Borrar artefacto", &mcp.Tool{
		Name: "artifact_delete",
		Description: "Borra un artefacto ya, sin esperar a que caduque. Su nota del inbox se queda. Sólo " +
			"quien lo creó o el lead global, y sólo si la persona lo pide.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in artIDArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.ArtifactDelete(c.app, c.auth, in.ID); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": in.ID, "deleted": true}), nil, nil
	})

	// ---- Secuencia --------------------------------------------------------------------

	type seqArg struct {
		Thread   string `json:"thread,omitempty" jsonschema:"el id del hilo (o bubble)"`
		Bubble   string `json:"bubble,omitempty" jsonschema:"el id del módulo (o thread)"`
		Sequence int    `json:"sequence" jsonschema:"su paso: 10, 20, 30…; el mismo número = en paralelo; 0 lo saca"`
	}
	add(tb, "Plan y secuencia", sets, "Ordenar la secuencia", &mcp.Tool{
		Name: "sequence_set",
		Description: "Pone un hilo (o un módulo) en un paso de la secuencia de su equipo. Sólo un lead del " +
			"equipo. Cada equipo tiene su propia línea. Pasos de 10 en 10; el mismo número va en paralelo; 0 lo saca. La secuencia es el ORDEN, " +
			"no una dependencia: las dependencias son relaciones (`thread_relate`).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in seqArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		kind, id := "thread", in.Thread
		if in.Bubble != "" {
			kind, id = "bubble", in.Bubble
		}
		r, err := bubble.SetSequence(c.app, c.auth, kind, id, in.Sequence)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{kind: r.Id, "name": r.GetString("name"), "sequence": r.GetInt("sequence")}), nil, nil
	})

	// ---- Inventario: escribir -----------------------------------------------------------

	invOut := func(r *core.Record) *mcp.CallToolResult {
		return jsonOut(map[string]any{"id": r.Id, "name": r.GetString("name")})
	}
	type invGroupArg struct {
		Group string  `json:"group,omitempty" jsonschema:"el id del grupo (para cambiarlo)"`
		Name  *string `json:"name,omitempty"`
		Note  *string `json:"note,omitempty"`
	}
	add(tb, "Infraestructura", adds, "Crear grupo de inventario", &mcp.Tool{
		Name:        "inventory_group_create",
		Description: "Crea un grupo del inventario (VPS, dominios, servicios…). Sólo el lead global.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in invGroupArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		r, err := bubble.InvGroupSave(c.app, c.auth, "", bubble.InvEdit{Name: in.Name, Notes: in.Note})
		if err != nil {
			return nil, nil, err
		}
		return invOut(r), nil, nil
	})
	add(tb, "Infraestructura", sets, "Cambiar grupo de inventario", &mcp.Tool{
		Name:        "inventory_group_update",
		Description: "Cambia el nombre o la nota de un grupo del inventario. Sólo el lead global.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in invGroupArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if in.Group == "" {
			return nil, nil, fmt.Errorf("falta `group`")
		}
		r, err := bubble.InvGroupSave(c.app, c.auth, in.Group, bubble.InvEdit{Name: in.Name, Notes: in.Note})
		if err != nil {
			return nil, nil, err
		}
		return invOut(r), nil, nil
	})
	type invGroupIDArg struct {
		Group string `json:"group" jsonschema:"el id del grupo"`
	}
	add(tb, "Infraestructura", destroys, "Borrar grupo de inventario", &mcp.Tool{
		Name:        "inventory_group_delete",
		Description: "Borra un grupo VACÍO del inventario; con cosas dentro se rechaza. Sólo el lead global.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in invGroupIDArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.InvGroupDelete(c.app, c.auth, in.Group); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"group": in.Group, "deleted": true}), nil, nil
	})

	type invItemArg struct {
		Item     string  `json:"item,omitempty" jsonschema:"el id de la cosa (para cambiarla)"`
		Group    *string `json:"group,omitempty" jsonschema:"el id de su grupo"`
		Name     *string `json:"name,omitempty"`
		Provider *string `json:"provider,omitempty"`
		URL      *string `json:"url,omitempty" jsonschema:"su panel o su sitio, http(s)"`
		Vault    *string `json:"vault,omitempty" jsonschema:"ENLACE a donde se guarda su credencial; NUNCA la credencial"`
		Notes    *string `json:"notes,omitempty" jsonschema:"sin secretos"`
		Renews   *string `json:"renews_at,omitempty" jsonschema:"un día: 2026-12-31; vacío la quita"`
		Cost     *string `json:"cost,omitempty" jsonschema:"como se dice: 12 USD/mes"`
	}
	itemEdit := func(in invItemArg) bubble.InvEdit {
		return bubble.InvEdit{Group: in.Group, Name: in.Name, Provider: in.Provider, URL: in.URL,
			Vault: in.Vault, Notes: in.Notes, Renews: in.Renews, Cost: in.Cost}
	}
	add(tb, "Infraestructura", adds, "Crear cosa del inventario", &mcp.Tool{
		Name: "inventory_item_create",
		Description: "Da de alta algo del inventario: un servidor, un dominio, un servicio o una licencia, " +
			"con proveedor, costo y renovación. Sólo el lead global. NUNCA escribas una credencial: `vault` " +
			"es un enlace a donde se guarda. Quién ve el inventario se asigna a mano en la aplicación.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in invItemArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		r, err := bubble.InvItemSave(c.app, c.auth, "", itemEdit(in))
		if err != nil {
			return nil, nil, err
		}
		return invOut(r), nil, nil
	})
	add(tb, "Infraestructura", sets, "Cambiar cosa del inventario", &mcp.Tool{
		Name:        "inventory_item_update",
		Description: "Cambia algo del inventario; lo que omitas queda igual. Sólo el lead global. NUNCA escribas una credencial.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in invItemArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if in.Item == "" {
			return nil, nil, fmt.Errorf("falta `item`")
		}
		r, err := bubble.InvItemSave(c.app, c.auth, in.Item, itemEdit(in))
		if err != nil {
			return nil, nil, err
		}
		return invOut(r), nil, nil
	})
	type invItemIDArg struct {
		Item string `json:"item" jsonschema:"el id de la cosa"`
	}
	add(tb, "Infraestructura", destroys, "Borrar cosa del inventario", &mcp.Tool{
		Name:        "inventory_item_delete",
		Description: "Da de baja algo del inventario. Sólo el lead global, y sólo si la persona lo pide.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in invItemIDArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.InvItemDelete(c.app, c.auth, in.Item); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"item": in.Item, "deleted": true}), nil, nil
	})
	return s
}

// locate resolves either addressing form to one document. Both are accepted
// because a thread id never goes stale and a path is what an agent already
// thinks in.
func locate(c *caller, threadID, project, path string) (ws *core.Record, repo, doc, subject string, err error) {
	if threadID != "" {
		th, w, r, e := bubble.ThreadFor(c.app, c.auth, threadID)
		if e != nil {
			return nil, "", "", "", e
		}
		return w, r, th.GetString("doc_path"), th.GetString("name"), nil
	}
	if project == "" || path == "" {
		return nil, "", "", "", fmt.Errorf("da un `thread`, o un `project` y un `path`")
	}
	w, r, e := bubble.WorkspaceFor(c.app, c.auth, project)
	if e != nil {
		return nil, "", "", "", e
	}
	return w, r, path, path, nil
}

// docText devuelve un documento como lo que es: markdown.
//
// Antes salía como JSON, y el markdown dentro iba ESCAPADO —`\n` en vez de
// saltos de línea— envuelto en un objeto con `html` al lado. Medido sobre un
// documento pequeño de verdad: 427 bytes, de los cuales 122 eran el texto. El
// resto era ruido que un modelo tiene que des-escapar mentalmente para
// recuperar la estructura que los encabezados y las casillas ya daban gratis.
//
// El `html` no viaja: existe para la aplicación web, que lo pide por la API. Por
// esta puerta duplicaba el tamaño sin que nadie lo leyera.
//
// La cabecera va en TEXTO y no en un bloque aparte —ni en `structuredContent`,
// ni en un `EmbeddedResource`— porque el hash no se puede perder: sin él no hay
// escritura. Y esos canales se pierden: OpenCode descartaba entero el contenido
// de tipo `resource` (su issue 7878), Claude Desktop lo consume sin mostrarlo, y
// la propia especificación pide mandar un texto de respaldo junto a
// `structuredContent` «por compatibilidad». Un canal del que el estándar
// desconfía no es donde se pone lo que no puede faltar.
func docText(d bubble.Doc, from, lines int) string {
	asked := from > 0 || lines > 0
	if lines < 1 {
		// Sin pedir nada, se manda el principio y se DICE cuánto falta. Cortar en
		// silencio es peor que no cortar: el agente edita creyendo que lo vio
		// todo. El tope es holgado —un documento normal cabe entero— y existe
		// para los que no lo son: uno de 38 KB son diez mil tokens cada vez que
		// alguien quiere corregir una línea.
		lines = wide
	}
	body, first, last, total := md.Window(d.Content, from, lines)

	head := d.Path + " · hash " + d.Hash
	if d.Done > 0 {
		head += " · " + strconv.Itoa(d.Done) + " hechas"
	}
	if total == 0 {
		// Un documento vacío EXISTE: se escribe con el hash del vacío. Devolver
		// la cabecera sola parecería un error de lectura.
		return head + "\n(manda ese hash como `base` al escribir)\n\n" +
			"(vacío — todavía no se ha escrito nada aquí)\n"
	}
	if first > 1 || last < total {
		head += " · líneas " + strconv.Itoa(first) + "–" + strconv.Itoa(last) +
			" de " + strconv.Itoa(total)
	} else {
		head += " · " + strconv.Itoa(total) + " líneas"
	}
	head += "\n(manda ese hash como `base` al escribir"
	if last < total {
		// Cómo pedir lo que falta, en la misma línea en la que se entera de que
		// falta. Tener que deducir la llamada siguiente es una lectura de más.
		head += "; el resto, con from: " + strconv.Itoa(last+1)
		if !asked {
			head += " — o pide `lines` si quieres otro tamaño"
		}
	}
	head += ")\n\n"
	return head + body + "\n"
}

// wide es cuánto se manda de un documento que nadie acotó.
//
// Mil quinientas líneas: lo que un harness sobre disco trae por defecto es del
// mismo orden, y por la misma razón — casi todo cabe, y lo que no cabe es
// justamente lo que no se quiere entero.
const wide = 1500

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// jsonOut renders a result as pretty JSON text.
//
// Text rather than a typed output schema: an agent reads these, and a shape that
// changes with a field addition is a shape that breaks a client. The description
// on each tool is where the meaning lives.
func jsonOut(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return text(fmt.Sprintf("no se pudo armar la respuesta: %v", err))
	}
	return text(string(b))
}
