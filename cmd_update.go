package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/boot"
	"github.com/AngelMaldonado/bubble-work/internal/release"
)

// mountUpdate pone la puerta que actualiza esta instancia.
//
// No actualiza nada por sí misma: descarga, comprueba, deja el binario
// instalado y se va con `PendingExit`. Quien cambia la versión vigente y sabe
// deshacerlo es `bubble boot`, el proceso de fuera — y es de fuera precisamente
// para que siga vivo cuando el de dentro no arranque.
//
// Sólo el lead global. Es la misma frontera que el resto de la capa del
// departamento, y aquí además es la única: actualizar afecta a todo el mundo.
func mountUpdate(app core.App, version string, latest *release.Cache) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/update", func(e *core.RequestEvent) error {
			auth := e.Auth
			if auth == nil || auth.Collection().Name != "users" || auth.GetString("role") != "lead" {
				return e.JSON(http.StatusForbidden, map[string]string{
					"message": "actualizar es del lead del departamento",
				})
			}
			// Sin arranque vigilante no hay a quién pedírselo, y dejar que
			// alguien pulse un botón que no hace nada es peor que no tenerlo.
			if os.Getenv(boot.ChildEnv) == "" {
				return e.JSON(http.StatusConflict, map[string]string{
					"message": "esta instancia no corre bajo `bubble boot`, así que no puede actualizarse sola",
				})
			}

			tag, _ := latest.Latest()
			if body := struct {
				Version string `json:"version"`
			}{}; e.BindBody(&body) == nil && body.Version != "" {
				tag = body.Version
			}
			if !release.Newer(version, tag) {
				return e.JSON(http.StatusBadRequest, map[string]string{
					"message": fmt.Sprintf("no hay nada más nuevo que %s", version),
				})
			}

			store := boot.Store{Dir: boot.Dir(os.Getenv("BUBBLE_DATA"))}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			dst := store.Binary(tag)
			if err := os.MkdirAll(store.Dir+"/versions", 0o755); err != nil {
				return e.JSON(http.StatusInternalServerError, map[string]string{"message": err.Error()})
			}
			cl := &http.Client{Timeout: 5 * time.Minute}
			if err := release.Download(ctx, cl, tag, runtime.GOOS, runtime.GOARCH, dst); err != nil {
				return e.JSON(http.StatusBadGateway, map[string]string{"message": err.Error()})
			}

			// La copia ANTES de reiniciar, y no opcional aquí.
			//
			// Volver atrás devuelve el binario, no el esquema: las migraciones
			// corren al arrancar y son de un solo sentido. Si la versión nueva
			// migra y luego falla, esta copia es lo único que queda. Es un
			// archivo de la base, no el respaldo completo que `update.sh` hace
			// opcional.
			name := fmt.Sprintf("antes-de-%s.zip", tag)
			if err := app.CreateBackup(ctx, name); err != nil {
				return e.JSON(http.StatusInternalServerError, map[string]string{
					"message": "no pude copiar la base antes de actualizar, así que no actualizo: " + err.Error(),
				})
			}

			if err := store.SetNext(tag); err != nil {
				return e.JSON(http.StatusInternalServerError, map[string]string{"message": err.Error()})
			}

			// Se contesta ANTES de irse: quien pulsó el botón tiene derecho a
			// saber que se aceptó, y en cuanto este proceso termine no habrá
			// nadie para decírselo.
			_ = e.JSON(http.StatusAccepted, map[string]any{
				"from": version, "to": tag, "backup": name,
			})
			go func() {
				time.Sleep(time.Second)
				os.Exit(boot.PendingExit)
			}()
			return nil
		})
		return se.Next()
	})
}
