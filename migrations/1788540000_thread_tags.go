package migrations

import (
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Tags del frontmatter (E4, adelantado para la vista de blobs; diseño del
// hilo #3, §6).
//
//   - `threads.tags`: los tags del documento del hilo, normalizados, en el orden
//     escrito. Es un ÍNDICE: la fuente es el frontmatter del archivo, el
//     servidor lo recalcula en cada escritura y al arrancar, y lo que mande un
//     cliente se ignora.
//   - `document-classified`: una escritura que sólo cambió los tags. Se registra
//     y no calienta: clasificar no es producir.
//   - La vista de evidencia deja ese evento fuera también del pulso
//     (`last_any_at`): renombrar o etiquetar no mantienen vivo un hilo, y
//     cambiar sus tags tampoco. Un comentario sí, como siempre.
func init() {
	m.Register(func(app core.App) error {
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.Fields.Add(&core.JSONField{Name: "tags", MaxSize: 4000})
		if err := app.Save(threads); err != nil {
			return err
		}
		events, err := app.FindCollectionByNameOrId("events")
		if err != nil {
			return err
		}
		kind := events.Fields.GetByName("kind").(*core.SelectField)
		if !slices.Contains(kind.Values, "document-classified") {
			kind.Values = append(kind.Values, "document-classified")
		}
		if err := app.Save(events); err != nil {
			return err
		}
		ev, err := app.FindCollectionByNameOrId("thread_evidence")
		if err != nil {
			return err
		}
		ev.ViewQuery = evidenceSelectNoClassified()
		return app.Save(ev)
	}, func(app core.App) error {
		ev, err := app.FindCollectionByNameOrId("thread_evidence")
		if err != nil {
			return err
		}
		ev.ViewQuery = evidenceSelect(true)
		if err := app.Save(ev); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery(`DELETE FROM events WHERE kind = 'document-classified'`).Execute(); err != nil {
			return err
		}
		events, err := app.FindCollectionByNameOrId("events")
		if err != nil {
			return err
		}
		kind := events.Fields.GetByName("kind").(*core.SelectField)
		kind.Values = slices.DeleteFunc(kind.Values, func(v string) bool { return v == "document-classified" })
		if err := app.Save(events); err != nil {
			return err
		}
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.Fields.RemoveByName("tags")
		return app.Save(threads)
	})
}

// evidenceSelectNoClassified es evidenceSelect(true) con el pulso sin
// `document-classified`. Escrita entera, como las anteriores: la vista ES el
// string, y PocketBase deriva sus campos de esta lista.
func evidenceSelectNoClassified() string {
	warming := []string{"document-changed", "thread-created", "thread-completed", "link-added"}
	quoted := "'" + strings.Join(warming, "','") + "'"
	return "SELECT t.id AS id, t.workspace AS workspace, t.bubble AS bubble," +
		" t.seq AS seq, t.name AS name, t.state AS state, t.created AS created, t.assignees AS assignees," +
		" CAST((SELECT MAX(e.at) FROM events e WHERE e.target = t.id AND e.kind IN (" + quoted + ")) AS TEXT) AS last_warm_at," +
		" CAST((SELECT MAX(e.at) FROM events e WHERE e.target = t.id AND e.kind != 'document-classified') AS TEXT) AS last_any_at," +
		" CAST((SELECT COUNT(*) FROM events e WHERE e.target = t.id AND e.kind IN (" + quoted + ")) AS INT) AS warm_count" +
		" FROM threads t"
}
