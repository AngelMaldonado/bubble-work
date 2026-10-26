// Package prompts holds what an agent is told, as MARKDOWN FILES embedded into
// the binary at build time.
//
// Files rather than string constants for one reason: prose that has to be
// re-escaped into Go source stops getting edited. Here it can be read, diffed and
// tuned like any other document.
//
// Lo que lee un agente sale de aquí: `Guide` es la guía de Bubble Work (la
// parte 1 de `agents_md`), `Markdown` cómo se escribe, `Grill` y
// `EphemeralMarkdown` los protocolos que llegan al crear su artefacto, y
// `Connect` lo que una persona copia para conectar su asistente (incluye la
// sección breve que el agente se instala en su CLAUDE.md / AGENTS.md).
//
// Embedded rather than read from disk, for a different one: this guide makes
// PROMISES about the API — which shapes a PATCH takes, what a conflict looks like
// — and a guide deployed separately from the binary drifts from it. Then it lies,
// and an agent believes it. Embedding makes "the guide describes the server that
// is running" true by construction, at the price of a rebuild to edit one. That is
// the right price for something that documents an interface.
//
// There is no runtime reload, deliberately: what an agent was told must be
// reproducible from the commit that was deployed.
package prompts

import (
	_ "embed"
)

// Guide es la guía de Bubble Work para agentes: entidades, ciclo de trabajo,
// cómo se escribe. Es la primera parte de cada respuesta de `agents_md`, antes
// de la tabla de tools (generada) y de la forma de trabajar del departamento.
//
//go:embed bubble-work.md
var Guide string

// Connect is what a PERSON copies to configure their assistant. A template with
// two holes, `{{url}}` and `{{token}}`, filled by the server that serves it.
//
// It lived as a string literal in the web app, which is exactly the shape prose
// stops getting edited in: escaped into source, invisible in a diff of the
// prompts, and impossible to tune without touching the client. It is the same
// argument the rest of this package already made.
//
//go:embed connect.md
var Connect string

// Grill es cómo un agente interroga a alguien A TRAVÉS de Bubble: preguntas
// estructuradas que la persona contesta en la aplicación. Llega en la respuesta
// de `artifact_create(kind: grill)`, justo cuando se usa; `agents_md` sólo dice
// cuándo ofrecerlo.
//
//go:embed grill.md
var Grill string

// Markdown es cómo se escribe markdown aquí; va en cada respuesta de agents_md.
//
//go:embed markdown.md
var Markdown string

// Instructions es el campo `instructions` del `initialize` (y del `discover`)
// de MCP: lo que el cliente pone en el contexto del agente en cada sesión, sin
// que el modelo tenga que acordarse de nada. Claude Code lo corta a 2 048
// caracteres sin avisar, así que es un cargador: dice la release vigente y
// manda a `agents_md`. Lleva `{{release}}`, `{{role}}` y `{{connect}}`.
//
//go:embed instructions.md
var Instructions string

// EphemeralMarkdown es el protocolo del `.md` efímero; llega en la respuesta de
// `artifact_create(kind: markdown)`.
//
//go:embed ephemeral-markdown.md
var EphemeralMarkdown string
