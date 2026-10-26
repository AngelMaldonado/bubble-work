package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Se quitan los contextos (20 oct 2026). Eran un catálogo que alguien tenía
// que mantener —globales del lead global, locales de cada equipo— y una
// burbuja elegía uno. Angel decidió que la vista de blobs agrupe por los TAGS
// del frontmatter de los documentos: los escribe cada persona con su IA, que
// conocen su área, y no hay nada que administrar.
//
// La migración que los creaba (1788536000_contexts.go) nunca se publicó y se
// borró del repositorio. Ésta limpia las bases de desarrollo que sí la
// aplicaron; en una base nueva no encuentra nada y no hace nada. No tiene
// vuelta atrás: no hay a qué volver.
func init() {
	m.Register(func(app core.App) error {
		if bubbles, err := app.FindCollectionByNameOrId("bubbles"); err == nil && bubbles.Fields.GetByName("context") != nil {
			bubbles.Fields.RemoveByName("context")
			if err := app.Save(bubbles); err != nil {
				return err
			}
		}
		if c, err := app.FindCollectionByNameOrId("contexts"); err == nil {
			return app.Delete(c)
		}
		return nil
	}, func(app core.App) error { return nil })
}
