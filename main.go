// Command bubble is the Bubble Work server.
//
// One binary: PocketBase as an embedded Go framework carrying the database,
// identity, API rules, realtime and the admin dashboard, with Bubble's own model
// mounted on top of it. See PLAN.md.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/osutils"

	"github.com/AngelMaldonado/bubble-work/internal/boot"
	"github.com/AngelMaldonado/bubble-work/internal/bubble"
	"github.com/AngelMaldonado/bubble-work/internal/mcpapi"
	"github.com/AngelMaldonado/bubble-work/internal/release"
	"github.com/AngelMaldonado/bubble-work/internal/tree"
	"github.com/AngelMaldonado/bubble-work/web"

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

	app.RootCmd.AddCommand(personCommand(app))

	// El arranque que puede deshacer una actualización. `serve` sigue estando y
	// sigue sirviendo: `boot` es quien lo lanza y lo vigila, y en desarrollo
	// —donde el binario se recompila a mano— se usa `serve` directo.
	app.RootCmd.AddCommand(bootCommand(version))

	bubble.Register(app, t)

	// The agent surface. Every tool calls the same function the REST route calls,
	// so the two cannot drift and neither can bypass what the other enforces.
	mcpapi.Register(app, t)

	// Qué está corriendo aquí. Sin sesión y sin tocar la base a propósito: es la
	// comprobación de salud del contenedor y lo que mira un actualizador antes y
	// después de cambiar la imagen, y una respuesta que necesita la base no
	// distingue "arrancando" de "roto". La versión de un binario autoalojado no
	// es un secreto: quien puede pedirla puede leer el mismo número en la
	// interfaz.
	// Y si hay una más nueva. La caché nunca espera a la red: esta ruta es la
	// comprobación de salud del contenedor —dos veces por minuto— y una que
	// dependiera de internet daría por enferma a una instancia que sólo está
	// aislada.
	latest := release.New(6 * time.Hour)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/version", func(e *core.RequestEvent) error {
			tag, checked := latest.Latest()
			out := map[string]any{"version": version, "latest": tag}
			out["stale"] = release.Newer(version, tag)
			if !checked.IsZero() {
				out["checked"] = checked.UTC().Format(time.RFC3339)
			}
			// Si puede actualizarse sola. La UI no ofrece un botón que no
			// haría nada: una instancia lanzada con `serve` a mano no tiene
			// quién deshaga la actualización, y por eso no se le propone.
			out["boot"] = os.Getenv(boot.ChildEnv) != ""
			return e.JSON(http.StatusOK, out)
		})
		return se.Next()
	})

	mountUpdate(app, version, latest)

	// The SPA, last: a catch-all route must not shadow /api or /mcp, and
	// registering it after them is what keeps that true. `true` serves index.html
	// for an unknown path, which is what a client-side router needs.
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/{path...}", apis.Static(web.Dist(), true))
		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
