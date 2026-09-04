// Package web carries the built browser UI, embedded into the binary so the
// server serves its own front end at "/" — one binary, one origin.
//
// The bundle is produced by frontend/ (`bun run build` → web/dist) and is
// COMMITTED, so a plain `go build` never needs a JS toolchain. The consequence
// worth knowing: a UI change is only visible after the bundle is rebuilt, so it
// belongs in the same commit as the source that produced it.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist is the bundle rooted at web/dist, so "index.html" and "assets/…" resolve
// directly.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // embedded at build time; this cannot fail at runtime
	}
	return sub
}
