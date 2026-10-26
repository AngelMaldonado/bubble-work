package migrations

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// La bienvenida es para quien llega, no para quien ya está.
//
// El frontend la abre a quien no tiene `onboarded_at` en sus preferencias
// (`prefs.value`). Sin este relleno, el día que salió se la habría abierto a
// todas las personas que ya trabajaban aquí: a quien ya sabe usarlo se le
// explica lo que ya sabe. Así que toda persona que existe al aplicar esta
// migración queda como recibida, y sólo una creada DESPUÉS la ve.
//
// Una clave en el saco de preferencias y no una columna en `users`: es una
// preferencia de su dueña, que nadie consulta ni filtra (ver 1788518000_prefs).
//
// La vuelta atrás no borra nada: la marca que puso esto no se distingue de la
// de alguien que terminó la bienvenida, y quitarlas todas se la reabriría a
// quien ya la vio.
func init() {
	m.Register(func(app core.App) error {
		prefs, err := app.FindCollectionByNameOrId("prefs")
		if err != nil {
			return err
		}
		users, err := app.FindAllRecords("users")
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		for _, u := range users {
			row, err := app.FindFirstRecordByFilter(prefs, "user = {:u}", map[string]any{"u": u.Id})
			if err != nil {
				row = core.NewRecord(prefs)
				row.Set("user", u.Id)
			}
			value := map[string]any{}
			if err := row.UnmarshalJSONField("value", &value); err != nil || value == nil {
				value = map[string]any{}
			}
			if _, done := value["onboarded_at"]; done {
				continue
			}
			value["onboarded_at"] = now
			row.Set("value", value)
			if err := app.Save(row); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		return nil
	})
}
