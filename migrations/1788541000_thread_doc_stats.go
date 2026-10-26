package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Cuánto tiene escrito un hilo, para que la vista Blobs pueda medir con eso:
//
//   - `doc_words`: palabras del Markdown de su documento y de las páginas `.md`
//     de su carpeta, sin el frontmatter.
//   - `doc_files`: archivos del hilo: las páginas de su carpeta más las imágenes
//     de `assets/` que esos documentos citan, sin repetir.
//
// Son un índice, como `tags`: la fuente son los archivos, el servidor los
// recalcula al escribir o borrar algo del hilo y al arrancar, y lo que mande un
// cliente se ignora. No dicen nada del calor: escribir más no calienta más.
func init() {
	m.Register(func(app core.App) error {
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.Fields.Add(&core.NumberField{Name: "doc_words", OnlyInt: true, Min: ptr(0.0)})
		threads.Fields.Add(&core.NumberField{Name: "doc_files", OnlyInt: true, Min: ptr(0.0)})
		return app.Save(threads)
	}, func(app core.App) error {
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.Fields.RemoveByName("doc_words")
		threads.Fields.RemoveByName("doc_files")
		return app.Save(threads)
	})
}

func ptr[T any](v T) *T { return &v }
