package bubble

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/prompts"
)

// Prompts: everything the AI is told, editable in the app.
//
// Three groups, one screen:
//   - Agentes: the two AGENTS.md, which already lived in `agents_md`;
//   - Sistema: the embedded documents (agents_guide, markdown, grill,
//     ephemeral_markdown, connect);
//   - Tools: the description of every MCP tool, which is what an agent reads
//     to decide whether and how to call it.
//
// The embedded text stays as the ORIGINAL. An edit is a row in
// `prompt_versions`; the newest row for a key wins, and a `reset` row means
// "back to the original". Nobody edits or deletes a row.

// PromptDef is one entry of the registry.
type PromptDef struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Group string `json:"group"` // agentes | sistema | tools
	Hint  string `json:"hint"`
	def   func() string
}

var (
	promptMu   sync.RWMutex
	promptDefs = map[string]PromptDef{}
	// bumped on every saved version: the MCP server rebuilds its tool
	// descriptions when it sees a new number.
	promptGen atomic.Int64
)

func init() {
	for _, d := range []PromptDef{
		{Key: "agents/planner", Title: "AGENTS.md (planner)", Group: "agentes",
			Hint: "La forma de trabajar que lee el agente del lead (`agents_md`)."},
		{Key: "agents/operators", Title: "AGENTS.md (operators)", Group: "agentes",
			Hint: "La forma de trabajar que lee el agente de quien opera (`agents_md`)."},
		// Clave nueva (no `guide`): la guía se reescribió en español con las
		// tools renombradas, y una edición vieja de `guide` taparía el texto
		// nuevo con nombres que ya no existen. Las filas viejas quedan como
		// historia y nadie las lee.
		{Key: "agents_guide", Title: "Guía para agentes", Group: "sistema", def: func() string { return prompts.Guide },
			Hint: "Primera parte de cada respuesta de `agents_md`: entidades, ciclo de trabajo y cómo se escribe. La tabla de tools se genera sola. Promete cosas sobre la API: cámbiala con cuidado."},
		{Key: "instructions", Title: "Instrucciones del servidor MCP", Group: "sistema", def: func() string { return prompts.Instructions },
			Hint: "Lo que el cliente pone en el contexto del agente en CADA sesión, sin que el modelo tenga que acordarse (campo `instructions` del `initialize`). Máximo 2 048 caracteres ya rellenado: Claude Code corta lo demás. Lleva `{{release}}`, `{{role}}` y `{{connect}}`."},
		{Key: "markdown", Title: "Escritura de markdown", Group: "sistema", def: func() string { return prompts.Markdown },
			Hint: "Va en cada respuesta de `agents_md`: idioma, forma sugerida, qué se renderiza."},
		{Key: "grill", Title: "Protocolo de interrogatorio", Group: "sistema", def: func() string { return prompts.Grill },
			Hint: "Llega en la respuesta de `artifact_create(kind: grill)`: cómo interroga un agente a través de Bubble."},
		{Key: "ephemeral_markdown", Title: "Documentos Markdown efímeros", Group: "sistema", def: func() string { return prompts.EphemeralMarkdown },
			Hint: "Llega en la respuesta de `artifact_create(kind: markdown)`: publicación por el agente, lectura temporal en Bubble."},
		{Key: "connect", Title: "Prompt de conexión", Group: "sistema", def: func() string { return prompts.Connect },
			Hint: "Lo que una persona copia para conectar su IA. Lleva `{{url}}`, `{{token}}` y `{{version}}`, y la sección que el agente se instala en su CLAUDE.md / AGENTS.md (el bloque ```markdown). Cambiar esa sección envejece las instalaciones; cambiar el resto, no."},
	} {
		promptDefs[d.Key] = d
	}
}

// RegisterToolPrompt records an MCP tool's built-in description, so the screen
// can list it and offer the original back. Called by the MCP package.
func RegisterToolPrompt(name, description string) {
	promptMu.Lock()
	defer promptMu.Unlock()
	promptDefs["tool/"+name] = PromptDef{
		Key: "tool/" + name, Title: name, Group: "tools",
		Hint: "La descripción de la tool `" + name + "`: lo que un agente lee para decidir si la llama y cómo.",
		def:  func() string { return description },
	}
}

// PromptsGen is the version counter of the prompts; it changes on every save.
func PromptsGen() int64 { return promptGen.Load() }

func promptDef(key string) (PromptDef, bool) {
	promptMu.RLock()
	defer promptMu.RUnlock()
	d, ok := promptDefs[key]
	return d, ok
}

func fingerprint(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

func latestVersion(app core.App, key string) *core.Record {
	rows, err := app.FindRecordsByFilter("prompt_versions", "key = {:k}", "-created", 1, 0, dbx.Params{"k": key})
	if err != nil || len(rows) == 0 {
		return nil
	}
	return rows[0]
}

// Prompt is the text in force for a system or tool key: the newest edit, or
// the original when there is none or the newest says "reset".
func Prompt(app core.App, key, original string) string {
	if app == nil {
		return original
	}
	if r := latestVersion(app, key); r != nil && !r.GetBool("reset") {
		return r.GetString("content")
	}
	return original
}

// PromptView is a prompt as the screen shows it.
type PromptView struct {
	PromptDef
	Content  string `json:"content"`
	Original string `json:"original"`
	// edited: an edit is in force. stale: the original changed in the binary
	// after that edit was written.
	Edited  bool   `json:"edited"`
	Stale   bool   `json:"stale"`
	Updated string `json:"updated,omitempty"`
	Author  string `json:"author,omitempty"`
	// Editable is false for the AGENTS.md of a role nobody may write.
	Editable bool `json:"editable"`
}

func authorName(app core.App, id string) string {
	u, err := app.FindRecordById("users", id)
	if err != nil {
		return ""
	}
	if n := u.GetString("display_name"); n != "" {
		return n
	}
	return u.GetString("email")
}

func promptView(app core.App, auth *core.Record, d PromptDef, withText bool) PromptView {
	v := PromptView{PromptDef: d, Editable: isGlobalLead(auth)}
	if strings.HasPrefix(d.Key, "agents/") {
		doc, _ := ReadAgents(app, auth, strings.TrimPrefix(d.Key, "agents/"))
		v.Edited = doc.Version != ""
		v.Updated, v.Author = doc.Created, doc.Author
		if withText {
			v.Content = doc.Content
		}
		return v
	}
	orig := d.def()
	if withText {
		v.Original = orig
		v.Content = orig
	}
	if r := latestVersion(app, d.Key); r != nil {
		v.Updated = r.GetDateTime("created").String()
		v.Author = authorName(app, r.GetString("author"))
		if !r.GetBool("reset") {
			v.Edited = true
			v.Stale = r.GetString("base") != fingerprint(orig)
			if withText {
				v.Content = r.GetString("content")
			}
		}
	}
	return v
}

func registerPrompts(app core.App) {
	app.OnRecordAfterCreateSuccess("prompt_versions", "agents_md").BindFunc(func(e *core.RecordEvent) error {
		promptGen.Add(1)
		return e.Next()
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/prompts", func(e *core.RequestEvent) error {
			if !isPersonAuth(e.Auth) {
				return e.UnauthorizedError("sign in first", nil)
			}
			promptMu.RLock()
			defs := make([]PromptDef, 0, len(promptDefs))
			for _, d := range promptDefs {
				defs = append(defs, d)
			}
			promptMu.RUnlock()
			// Agentes, then the system documents in a fixed order, then the tools
			// by name.
			rank := map[string]int{"agents/planner": 0, "agents/operators": 1, "instructions": 2, "agents_guide": 3,
				"markdown": 4, "grill": 5, "ephemeral_markdown": 6, "connect": 7}
			sort.Slice(defs, func(i, j int) bool {
				ri, iok := rank[defs[i].Key]
				rj, jok := rank[defs[j].Key]
				switch {
				case iok && jok:
					return ri < rj
				case iok != jok:
					return iok
				}
				return defs[i].Key < defs[j].Key
			})
			out := make([]PromptView, 0, len(defs))
			for _, d := range defs {
				if !mayRead(e.Auth, d.Key) {
					continue
				}
				out = append(out, promptView(e.App, e.Auth, d, false))
			}
			return e.JSON(http.StatusOK, out)
		}).Bind(apis.RequireAuth())

		se.Router.GET("/api/prompt", func(e *core.RequestEvent) error {
			if !isPersonAuth(e.Auth) {
				return e.UnauthorizedError("sign in first", nil)
			}
			d, ok := promptDef(e.Request.URL.Query().Get("key"))
			if !ok || !mayRead(e.Auth, d.Key) {
				return e.NotFoundError("no such prompt", nil)
			}
			return e.JSON(http.StatusOK, promptView(e.App, e.Auth, d, true))
		}).Bind(apis.RequireAuth())

		// One write door for all of them. AGENTS.md goes to `agents_md`, the
		// rest to `prompt_versions` with the fingerprint of the original.
		se.Router.POST("/api/prompt", func(e *core.RequestEvent) error {
			if !isGlobalLead(e.Auth) {
				return e.ForbiddenError("only the global lead edits what the agents are told", nil)
			}
			var body struct {
				Key     string `json:"key"`
				Content string `json:"content"`
				Reset   bool   `json:"reset"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.BadRequestError("could not read the body", err)
			}
			d, ok := promptDef(body.Key)
			if !ok {
				return e.NotFoundError("no such prompt", nil)
			}
			if strings.HasPrefix(d.Key, "agents/") {
				if body.Reset {
					return e.BadRequestError("an AGENTS.md has no original to go back to", nil)
				}
				col, err := e.App.FindCollectionByNameOrId("agents_md")
				if err != nil {
					return e.InternalServerError(err.Error(), err)
				}
				r := core.NewRecord(col)
				r.Set("role", strings.TrimPrefix(d.Key, "agents/"))
				r.Set("content", body.Content)
				r.Set("author", e.Auth.Id)
				if err := e.App.Save(r); err != nil {
					return e.BadRequestError(err.Error(), err)
				}
			} else {
				col, err := e.App.FindCollectionByNameOrId("prompt_versions")
				if err != nil {
					return e.InternalServerError(err.Error(), err)
				}
				orig := d.def()
				if !body.Reset && strings.TrimSpace(body.Content) == "" {
					return e.BadRequestError("an empty prompt tells the agent nothing; restore the original instead", nil)
				}
				if d.Key == "instructions" && !body.Reset {
					if err := validateInstructions(body.Content); err != nil {
						return e.BadRequestError(err.Error(), err)
					}
				}
				r := core.NewRecord(col)
				r.Set("key", d.Key)
				r.Set("reset", body.Reset)
				if !body.Reset {
					r.Set("content", body.Content)
				}
				r.Set("base", fingerprint(orig))
				r.Set("author", e.Auth.Id)
				if err := e.App.Save(r); err != nil {
					return e.BadRequestError(err.Error(), err)
				}
			}
			return e.JSON(http.StatusOK, promptView(e.App, e.Auth, d, true))
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}

// mayRead: the planner's AGENTS.md is the lead's, as it was before Prompts.
// Everything else is readable by anybody who works here — it is what their
// own agent is told.
func mayRead(auth *core.Record, key string) bool {
	return key != "agents/planner" || isGlobalLead(auth)
}

// guard against a key typo at a call site: the defaults are known.
func mustPrompt(key string) string {
	d, ok := promptDef(key)
	if !ok || d.def == nil {
		panic(fmt.Sprintf("no built-in prompt %q", key))
	}
	return d.def()
}

// SystemPrompt is the text in force for one of the embedded documents.
func SystemPrompt(app core.App, key string) string { return Prompt(app, key, mustPrompt(key)) }
