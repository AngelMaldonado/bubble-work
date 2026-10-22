package main

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// Personas simuladas del dev: `just seed agents` las crea con un correo de
// `simulatedDomain`, y esto las quita del todo cuando el dev se baja.
//
// Corre sin servidor, contra el directorio de datos: `just stop`, `just dev`
// (que reemplaza la sesión) y la ventana del servidor al terminar —con Ctrl-C o
// porque se cayó— lo llaman con el servidor ya abajo, para que nada quede a
// medias entre una sesión y la siguiente.
//
// «Del todo» es BORRAR, no el borrado lógico de la aplicación: una persona
// simulada no firmó trabajo de nadie. Se recorren todas las colecciones y toda
// relación que apunte a una de ellas: una relación de una sola persona (dueño,
// autor) borra la fila; una de varias (asignados) sólo la quita de la lista.
// Recorrer en vez de enumerar: lo que una versión futura del seed toque también
// se va, sin acordarse de agregarlo aquí.
//
// Rechazado: limpiar por la API antes de bajar el servidor. No cubre un Ctrl-C
// ni una caída, que es justo cuando nadie se acuerda de limpiar.
const simulatedDomain = "@simulado.test"

func simulatedCommand(app *pocketbase.PocketBase) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "simulated",
		Short: "Personas simuladas del entorno de desarrollo",
	}
	cmd.AddCommand(&cobra.Command{
		Use:          "purge",
		Short:        "Borra a las personas " + simulatedDomain + " y todo lo que apunta a ellas",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			people, rows, err := purgeSimulated(app)
			if err != nil {
				return err
			}
			if people > 0 {
				fmt.Printf("simuladas: %d personas y %d filas suyas borradas\n", people, rows)
			}
			return nil
		},
	})
	return cmd
}

func purgeSimulated(app core.App) (people, rows int, err error) {
	sims, err := app.FindRecordsByFilter("users", "email ~ {:d}", "", 0, 0, dbx.Params{"d": simulatedDomain})
	if err != nil || len(sims) == 0 {
		return 0, 0, err
	}
	ids := map[string]bool{}
	for _, u := range sims {
		// `~` es «contiene»: el sufijo se comprueba aquí.
		if strings.HasSuffix(strings.ToLower(u.Email()), simulatedDomain) {
			ids[u.Id] = true
		}
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return 0, 0, err
	}
	cols, err := app.FindAllCollections(core.CollectionTypeBase, core.CollectionTypeAuth)
	if err != nil {
		return 0, 0, err
	}

	err = app.RunInTransaction(func(tx core.App) error {
		for _, col := range cols {
			for _, f := range col.Fields {
				rel, ok := f.(*core.RelationField)
				if !ok || rel.CollectionId != users.Id {
					continue
				}
				for id := range ids {
					n, err := dropReferences(tx, col, rel, id, ids)
					if err != nil {
						return fmt.Errorf("%s.%s: %w", col.Name, rel.Name, err)
					}
					rows += n
				}
			}
		}
		for _, u := range sims {
			if !ids[u.Id] {
				continue
			}
			// Puede haberse ido ya en cascada.
			fresh, err := tx.FindRecordById("users", u.Id)
			if err != nil {
				continue
			}
			if err := tx.Delete(fresh); err != nil {
				return fmt.Errorf("borrar %s: %w", u.Email(), err)
			}
			people++
		}
		return nil
	})
	return people, rows, err
}

// dropReferences quita a `id` de una relación hacia `users`.
func dropReferences(app core.App, col *core.Collection, rel *core.RelationField, id string, sims map[string]bool) (int, error) {
	op := "="
	if rel.IsMultiple() {
		op = "?="
	}
	recs, err := app.FindRecordsByFilter(col.Name, rel.Name+" "+op+" {:id}", "", 0, 0, dbx.Params{"id": id})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range recs {
		// Una persona simulada que apunta a otra se va igual, como persona.
		if col.Name == "users" && sims[r.Id] {
			continue
		}
		if rel.IsMultiple() {
			left := []string{}
			for _, v := range r.GetStringSlice(rel.Name) {
				if v != id {
					left = append(left, v)
				}
			}
			if len(left) > 0 || !rel.Required {
				r.Set(rel.Name, left)
				if err := app.Save(r); err != nil {
					return n, err
				}
				n++
				continue
			}
		}
		if err := app.Delete(r); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
