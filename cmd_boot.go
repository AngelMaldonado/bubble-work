package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AngelMaldonado/bubble-work/internal/boot"
)

// bootCommand arranca el servidor de forma que una actualización se pueda
// deshacer.
//
//	bubble boot --http 0.0.0.0:8090 --dir /data/pb_data
//
// Es lo que corre el servicio. Este proceso no sirve nada: instala la versión
// que trae dentro si no había ninguna —o si es más nueva que la vigente, que es
// como llega una actualización cuando un orquestador cambia la imagen—, lanza la
// vigente como hijo y la vigila.
// Si la versión nueva no llega a contestar, vuelve a la última que sí lo hizo.
//
// El que deshace nunca puede ser el que se actualizó — por eso son dos
// procesos, y por eso el de fuera es el que instaló una persona y sólo cambia
// cuando una persona lo cambia.
func bootCommand(version string) *cobra.Command {
	c := &cobra.Command{
		Use:   "boot [banderas de serve]",
		Short: "Serve, and be able to undo an update that does not work",
		// Las banderas se pasan tal cual al hijo: aquí no se interpretan, sólo
		// se leen dos para saber dónde están los datos y a dónde preguntar.
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data := flagOf(args, "--dir", os.Getenv("BUBBLE_DATA"))
			addr := flagOf(args, "--http", os.Getenv("BUBBLE_HTTP"))

			store := boot.Store{Dir: boot.Dir(data)}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			// Una imagen más nueva que la vigente es una actualización, y se trata
			// como la del botón: la base se copia antes, porque la migración
			// corre al arrancar y volver atrás devuelve el binario, no el esquema.
			if store.Adopts(version) {
				if data == "" {
					data = "pb_data" // el mismo valor por defecto que `serve`
				}
				name := fmt.Sprintf("antes-de-%s.zip", version)
				if err := boot.Snapshot(data, name); err != nil {
					return fmt.Errorf("no pude copiar la base antes de adoptar %s, así que no la adopto: %w", version, err)
				}
				log.Printf("boot: la imagen trae %s, más nueva que %s; base copiada en backups/%s",
					version, store.Current(), name)
			}
			if err := store.Seed(exe, version); err != nil {
				return err
			}
			s := &boot.Supervisor{
				Store:  store,
				Args:   append([]string{"serve"}, args...),
				Health: boot.HealthURL(addr),
				// Un minuto para llegar a contestar: PocketBase aplica las
				// migraciones al arrancar, y una grande sobre una base grande
				// tarda más que un `hola`.
				Grace: time.Minute,
				// Y medio minuto sosteniéndolo. Una versión que contesta una vez
				// y se muere en bucle no es una versión buena, y marcarla como
				// tal dejaría a la instancia sin a dónde volver.
				Settle: 30 * time.Second,
			}
			return s.Run()
		},
	}
	return c
}

// flagOf saca el valor de una bandera de una lista de argumentos sin
// interpretarla. Acepta las dos formas que la gente escribe.
func flagOf(args []string, name, def string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return def
}
