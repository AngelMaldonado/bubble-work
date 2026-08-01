// Package web carries the built browser UI, embedded into the binary so the
// server serves its own front end at "/" (§9 — one binary, single origin).
// The bundle is produced by `frontend/` (bun run build → web/dist) and committed
// so the Go build never needs a JS toolchain.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built SPA bundle rooted at web/dist (so "index.html",
// "assets/…" resolve directly).
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded at build time; this cannot fail at runtime
	}
	return sub
}
