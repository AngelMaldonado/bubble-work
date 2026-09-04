// Command bubble is the Bubble Work server.
//
// One binary: PocketBase as an embedded Go framework carrying the database,
// identity, API rules, realtime and the admin dashboard, with Bubble's own model
// mounted on top of it. See PLAN.md.
package main

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/osutils"

	"github.com/AngelMaldonado/bubble-work/internal/bubble"
	"github.com/AngelMaldonado/bubble-work/internal/mcpapi"
	"github.com/AngelMaldonado/bubble-work/internal/tree"

	// Registers the schema migrations by side effect. Without this import the
	// binary starts against an empty database and every rule silently refers to
	// collections that do not exist.
	_ "github.com/AngelMaldonado/bubble-work/migrations"
)

// version is stamped at build time from git — see scripts/build.sh. A deployed
// binary that cannot say what it is makes every incident start with a guess.
var version = "dev"

func main() {
	app := pocketbase.New()
	app.RootCmd.Version = version

	// Automigrate writes a migration file when the schema is changed from the
	// dashboard, and only while running under `go run` — so a change made by
	// clicking in dev still ends up in the repository, and a production binary
	// never writes to its own source tree.
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: osutils.IsProbablyGoRun(),
	})

	// Where the markdown tree lives: one git repository per workspace, beside the
	// database rather than inside it. `BUBBLE_REPOS` moves it without touching a row.
	repos := os.Getenv("BUBBLE_REPOS")
	if repos == "" {
		repos = "./repos"
	}
	t, err := tree.New(repos)
	if err != nil {
		log.Fatal(err)
	}

	bubble.Register(app, t)

	// The agent surface. Every tool calls the same function the REST route calls,
	// so the two cannot drift and neither can bypass what the other enforces.
	mcpapi.Register(app, t)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
