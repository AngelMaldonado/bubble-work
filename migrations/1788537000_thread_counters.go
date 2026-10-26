package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El número de un hilo (#N, la dirección /w/<proyecto>/t/N y el nombre de su
// archivo) sale de un contador por ámbito, no de MAX(seq)+1 sobre los hilos
// que quedan: con MAX, borrar el último hilo regalaba su número —y su URL— al
// siguiente que se creara, y dos altas simultáneas leían el mismo MAX.
//
// Es la primera pieza de E1 (diseño del hilo #3 de Burbujas, §2.3): el ámbito
// es hoy sólo `workspace`; el inbox de equipo será otro `scope_kind` con su
// propia numeración.
//
// Tabla interna, sin colección de PocketBase: no tiene API ni reglas porque
// nadie la lee ni la escribe salvo el servidor al crear un hilo.
//
// Límite conocido: el relleno inicial parte del MAX(seq) actual. Un número que
// se borró ANTES de esta migración y era el más alto todavía puede repetirse
// una vez; de aquí en adelante, ninguno.
func init() {
	m.Register(func(app core.App) error {
		if _, err := app.DB().NewQuery(`CREATE TABLE IF NOT EXISTS thread_counters (
			scope_kind TEXT NOT NULL,
			scope_id   TEXT NOT NULL,
			last       INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (scope_kind, scope_id)
		)`).Execute(); err != nil {
			return err
		}
		_, err := app.DB().NewQuery(`INSERT OR IGNORE INTO thread_counters (scope_kind, scope_id, last)
			SELECT 'workspace', workspace, MAX(seq) FROM threads
			WHERE workspace != '' GROUP BY workspace`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`DROP TABLE IF EXISTS thread_counters`).Execute()
		return err
	})
}
