package bubble

import (
	"encoding/json"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/tree"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Los tags de un hilo salen del frontmatter de SU documento (md.Tags), no de
// lo que diga un cliente. `threads.tags` es un índice: se recalcula cuando el
// servidor escribe el documento principal del hilo y al arrancar, por si un
// archivo cambió con el servidor apagado (una restauración, un git a mano).
// Las páginas de la carpeta del hilo no cuentan: clasifica su documento.

// EvDocumentClassified: una escritura que sólo cambió los tags. Se registra y
// no calienta, ni siquiera como pulso (ver la migración 1788540000).
const EvDocumentClassified = "document-classified"

func threadTags(r *core.Record) []string {
	var out []string
	_ = r.UnmarshalJSONField("tags", &out)
	return out
}

// Cuánto tiene escrito un hilo (migración 1788541000): las palabras de su
// documento y de las páginas `.md` de su carpeta, sin frontmatter, y sus
// archivos: las páginas más las imágenes de `assets/` que citan, sin repetir.
var mdAssetRef = regexp.MustCompile(`assets/[^\s)"'\]>|]+`)

type docStats struct{ words, files int }

func countWords(content string) int { return len(strings.Fields(md.StripFrontmatter(content))) }

func threadDocStats(t *tree.Tree, repo, docPath, main string, entries []tree.Entry) docStats {
	words := countWords(main)
	refs := map[string]bool{}
	for _, r := range mdAssetRef.FindAllString(main, -1) {
		refs[r] = true
	}
	pages := pagesFrom(entries, docPath)
	for _, p := range pages {
		if !strings.HasSuffix(p, ".md") {
			continue
		}
		c, _, err := t.Read(repo, p)
		if err != nil {
			continue
		}
		words += countWords(c)
		for _, r := range mdAssetRef.FindAllString(c, -1) {
			refs[r] = true
		}
	}
	return docStats{words: words, files: len(pages) + len(refs)}
}

// indexThread actualiza el índice de un hilo (tags y lo escrito) después de
// escribir o borrar `path`, que es suyo. `content` es lo que quedó en `path`
// (vacío si se borró).
func indexThread(app core.App, t *tree.Tree, repo, threadID, path, content string) {
	th, err := app.FindRecordById("threads", threadID)
	if err != nil {
		return
	}
	docPath := th.GetString("doc_path")
	main := content
	if path != docPath {
		if main, _, err = t.Read(repo, docPath); err != nil {
			main = ""
		}
	}
	entries, _ := t.Tree(repo)
	tags := md.Tags(main)
	st := threadDocStats(t, repo, docPath, main, entries)
	if slices.Equal(tags, threadTags(th)) && st.words == th.GetInt("doc_words") && st.files == th.GetInt("doc_files") {
		return
	}
	th.Set("tags", tags)
	th.Set("doc_words", st.words)
	th.Set("doc_files", st.files)
	// app.Save, no SQL: así llega por realtime a quien mira el tablero.
	if err := app.Save(th); err != nil {
		slog.Error("índice del hilo: no se pudo guardar", "thread", threadID, "err", err)
	}
}

// indexOwner: después de borrar `doc`, reindexa el hilo dueño, si lo tiene.
func indexOwner(app core.App, t *tree.Tree, ws *core.Record, repo, doc string) {
	if id, _ := ownerOf(app, ws, doc); id != "" {
		indexThread(app, t, repo, id, doc, "")
	}
}

// reindexTags recorre los hilos y corrige el índice contra los archivos.
func reindexTags(app core.App, t *tree.Tree) {
	rows, err := app.FindAllRecords("threads", dbx.NewExp("doc_path != ''"))
	if err != nil {
		slog.Error("tags: no se pudieron leer los hilos", "err", err)
		return
	}
	repos := map[string]string{}
	trees := map[string][]tree.Entry{}
	fixed := 0
	for _, th := range rows {
		ws := th.GetString("workspace")
		repo, ok := repos[ws]
		if !ok {
			if w, err := app.FindRecordById("workspaces", ws); err == nil {
				repo = w.GetString("repo_path")
			}
			repos[ws] = repo
		}
		if repo == "" {
			continue
		}
		content, _, err := t.Read(repo, th.GetString("doc_path"))
		if err != nil {
			continue
		}
		entries, ok := trees[repo]
		if !ok {
			entries, _ = t.Tree(repo)
			trees[repo] = entries
		}
		tags := md.Tags(content)
		st := threadDocStats(t, repo, th.GetString("doc_path"), content, entries)
		if slices.Equal(tags, threadTags(th)) && st.words == th.GetInt("doc_words") && st.files == th.GetInt("doc_files") {
			continue
		}
		b, _ := json.Marshal(tags)
		// SQL directo: al arrancar no hay nadie mirando, y no es una edición.
		if _, err := app.DB().NewQuery("UPDATE threads SET tags = {:t}, doc_words = {:w}, doc_files = {:f} WHERE id = {:id}").
			Bind(dbx.Params{"t": string(b), "w": st.words, "f": st.files, "id": th.Id}).Execute(); err == nil {
			fixed++
		}
	}
	if fixed > 0 {
		slog.Info("índice de hilos (tags y lo escrito) corregido contra los documentos", "hilos", fixed)
	}
}

func registerThreadTags(app core.App, t *tree.Tree) {
	// Un cliente no escribe el índice: ni al crear ni al editar el hilo.
	app.OnRecordCreateRequest("threads").BindFunc(func(e *core.RecordRequestEvent) error {
		e.Record.Set("tags", nil)
		e.Record.Set("doc_words", 0)
		e.Record.Set("doc_files", 0)
		return e.Next()
	})
	app.OnRecordUpdateRequest("threads").BindFunc(func(e *core.RecordRequestEvent) error {
		for _, f := range []string{"tags", "doc_words", "doc_files"} {
			e.Record.Set(f, e.Record.Original().Get(f))
		}
		return e.Next()
	})
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		reindexTags(se.App, t)
		return se.Next()
	})
}
