package bubble

import (
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// nextThreadNumber reserva el siguiente #N de un proyecto. Una sola sentencia:
// el upsert sube el contador y lo devuelve, así que dos altas a la vez no
// pueden leer el mismo número, y un número no vuelve a salir aunque su hilo se
// borre. Si el alta falla después, queda un hueco: un hueco no confunde a
// nadie; un número repetido abre otro hilo desde una URL vieja.
//
// No baja de lo que ya existe: si el contador quedara por detrás de los hilos
// (una fila perdida, datos importados a mano), parte de MAX(seq).
func nextThreadNumber(app core.App, workspaceID string) (int, error) {
	if workspaceID == "" {
		return 0, errors.New("a thread is numbered within a workspace")
	}
	var next int
	err := app.DB().NewQuery(`INSERT INTO thread_counters (scope_kind, scope_id, last)
		VALUES ('workspace', {:ws}, (SELECT COALESCE(MAX(seq), 0) FROM threads WHERE workspace = {:ws}) + 1)
		ON CONFLICT (scope_kind, scope_id)
		DO UPDATE SET last = MAX(thread_counters.last, excluded.last - 1) + 1
		RETURNING last`).
		Bind(dbx.Params{"ws": workspaceID}).Row(&next)
	if err != nil {
		return 0, fmt.Errorf("next thread number: %w", err)
	}
	return next, nil
}
