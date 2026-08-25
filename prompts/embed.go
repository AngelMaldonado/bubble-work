// Package prompts holds the MCP prompts as MARKDOWN FILES, embedded into the
// binary at build time.
//
// They are files rather than string constants for one reason: a prompt is written
// for a reader, and prose that has to be re-escaped into Go source stops getting
// edited. Here they can be read, diffed and tuned like any other document, and the
// binary still ships them (the same trick web/embed.go plays with the SPA bundle).
//
// Editing one is a normal commit — `just build` picks it up. There is no reload at
// runtime, deliberately: what an agent was told must be reproducible from the
// commit that was deployed.
package prompts

import (
	"embed"
	"io/fs"
)

//go:embed *.md
var files embed.FS

// FS is the embedded prompt set.
func FS() fs.FS { return files }
