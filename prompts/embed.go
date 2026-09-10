// Package prompts holds what an agent is told, as MARKDOWN FILES embedded into
// the binary at build time.
//
// Files rather than string constants for one reason: prose that has to be
// re-escaped into Go source stops getting edited. Here it can be read, diffed and
// tuned like any other document.
//
// Tres documentos, y cada uno contesta algo distinto: `Guide` describe el
// SISTEMA, `House` es lo que un agente se pega a sí mismo en sus instrucciones
// del proyecto para que la próxima sesión no empiece a ciegas, y `Connect` es lo
// que una persona copia para configurar su asistente.
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

// Guide is the one document this server ships. It describes the SYSTEM — what
// warms, where files live, how a write works. It says nothing about how to write a
// document, because that is the writer's to decide: a team's own conventions live
// in the README of its workspace, which the guide points at.
//
//go:embed bubble-work.md
var Guide string

// House is what an agent installs into ITS OWN instructions file — `CLAUDE.md`,
// `AGENTS.md`, `.cursorrules`, whichever it happens to read — so that the next
// session starts knowing where the work is written down.
//
// Sin ella, cada conversación vuelve a inventarlo: abre un thread nuevo al lado
// del que ya estaba a medias, o no anota nada y el board dice que aquí no pasó
// nada. Lleva un hueco, `<SLUG>`, que quien la instala sustituye por el
// workspace del proyecto — el documento no puede saber en cuál se está pegando.
//
//go:embed house-rules.md
var House string

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
