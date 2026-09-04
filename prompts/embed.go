// Package prompts holds what an agent is told, as MARKDOWN FILES embedded into
// the binary at build time.
//
// Files rather than string constants for one reason: prose that has to be
// re-escaped into Go source stops getting edited. Here it can be read, diffed and
// tuned like any other document.
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
