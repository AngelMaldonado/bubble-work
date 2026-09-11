package bubble

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/tree"
	"github.com/AngelMaldonado/bubble-work/prompts"
)

// The document half: a thread's markdown is a FILE, and the row only says where.
//
// It deliberately does not live in a PocketBase field, so it cannot be reached
// through the record API at all — reading and writing it goes through the two
// routes below, which is what lets every write take the per-path lock and land a
// git commit.

// registerDocuments wires the document routes and the hooks that keep the tree and
// the rows agreeing.
func registerDocuments(app core.App, t *tree.Tree) {
	app.OnRecordAfterCreateSuccess("workspaces").BindFunc(func(e *core.RecordEvent) error {
		// Best effort AFTER the row exists: a workspace with no repository yet is
		// harmless (the first write creates it), while refusing to create the
		// workspace because git hiccupped would not be.
		if err := t.EnsureRepo(e.Record.GetString("repo_path")); err != nil {
			app.Logger().Error("could not create the workspace repository",
				"workspace", e.Record.Id, "err", err)
		}
		return e.Next()
	})

	app.OnRecordCreateRequest("workspaces").BindFunc(func(e *core.RecordRequestEvent) error {
		// Stamped from the slug at founding and never derived again: renaming a
		// workspace must not move a git repository out from under its history.
		e.Record.Set("repo_path", slugify(e.Record.GetString("slug")))
		return e.Next()
	})

	app.OnRecordCreateRequest("threads").BindFunc(func(e *core.RecordRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		// After the save, because the path carries `seq` and only the save knows it.
		if err := stampDocPath(e.App, e.Record); err != nil {
			return err
		}
		// Recorded here rather than in a model hook: defining a piece of work is
		// production (decision 0002) and the credit belongs to whoever did it,
		// which only the REQUEST knows.
		actor := ""
		if isPersonAuth(e.Auth) {
			actor = e.Auth.Id
		}
		record(e.App, e.Record.GetString("workspace"), "thread", e.Record.Id,
			EvThreadCreated, actor, map[string]any{"name": e.Record.GetString("name")})
		return nil
	})

	app.OnRecordUpdateRequest("threads").BindFunc(func(e *core.RecordRequestEvent) error {
		before, err := e.App.FindRecordById("threads", e.Record.Id)
		if err != nil {
			return e.Next()
		}
		oldName, oldPath := before.GetString("name"), before.GetString("doc_path")
		if err := e.Next(); err != nil {
			return err
		}
		if e.Record.GetString("name") == oldName || oldPath == "" {
			return nil
		}
		// The name changed, so the file follows it. `Move` is a rename, which is
		// what keeps `git log --follow` able to see across it.
		newPath := docPathFor(e.Record)
		repo, err := repoOf(e.App, e.Record)
		if err != nil {
			return nil
		}
		if err := t.Move(repo, oldPath, newPath, actorLabel(e.Auth), "rename: "+e.Record.GetString("name")); err != nil {
			e.App.Logger().Error("could not move the document", "thread", e.Record.Id, "err", err)
			return nil
		}
		// And everything the thread wrote beside it. One move per file rather
		// than one of the directory: every path that moves is a path the layout
		// has already checked, and each move stays a commit git can follow.
		for _, p := range pagesOf(t, repo, oldPath) {
			to := pagesDir(newPath) + "/" + path.Base(p)
			if err := t.Move(repo, p, to, actorLabel(e.Auth), "rename: "+to); err != nil {
				e.App.Logger().Error("could not move a page", "path", p, "err", err)
			}
		}
		e.Record.Set("doc_path", newPath)
		return e.App.Save(e.Record)
	})

	app.OnRecordAfterDeleteSuccess("threads").BindFunc(func(e *core.RecordEvent) error {
		repo, err := repoOf(app, e.Record)
		if err == nil {
			doc := e.Record.GetString("doc_path")
			for _, p := range pagesOf(t, repo, doc) {
				if err := t.Remove(repo, p, "bubble", "deleted: "+p); err != nil {
					app.Logger().Error("could not remove a page", "path", p, "err", err)
				}
			}
			if err := t.Remove(repo, doc, "bubble", "deleted: "+e.Record.GetString("name")); err != nil {
				app.Logger().Error("could not remove the document", "thread", e.Record.Id, "err", err)
			}
		}
		return e.Next()
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/threads/{id}/document", func(e *core.RequestEvent) error {
			th, repo, err := reachThread(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			ws, err := e.App.FindRecordById("workspaces", th.GetString("workspace"))
			if err != nil {
				return e.NotFoundError("", err)
			}
			// ReadDoc rather than a map built here: reading and writing have to
			// answer with the SAME shape, or a client that redraws from what a
			// write returned redraws something a read never gives it.
			doc, err := ReadDoc(e.App, t, ws, repo, th.GetString("doc_path"))
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, doc)
		}).Bind(apis.RequireAuth())

		se.Router.PATCH("/api/threads/{id}/document", func(e *core.RequestEvent) error {
			th, repo, err := reachThread(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			ws, err := e.App.FindRecordById("workspaces", th.GetString("workspace"))
			if err != nil {
				return e.NotFoundError("", err)
			}
			return patchDocument(e, t, ws, repo, th.GetString("doc_path"), th.GetString("name"))
		}).Bind(apis.RequireAuth())

		se.Router.GET("/api/threads/{id}/history", func(e *core.RequestEvent) error {
			th, repo, err := reachThread(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			log, err := t.Log(repo, th.GetString("doc_path"), 50)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"commits": log})
		}).Bind(apis.RequireAuth())

		// The workspace's own tree, and the documents in it that are not threads.
		//
		// `docs/` and `README.md` have no row anywhere: the DIRECTORY is the index,
		// which is the whole point of the file being the record. So they are reached
		// by path, and a path is all an agent needs to know.
		// What moved, in lines.
		//
		// Every write here is a commit, so "what changed" is a question git has
		// already answered — computing it a second time in the browser would be a
		// second answer to the same question, and the two agree until they do not.
		//
		// The window is the CYCLE by default, and that is the whole point of the
		// number: the same window heat is measured against decides what counts as
		// "changed", so "+124 −18" reads as how much this document moved inside
		// the window everything else is judged in.
		se.Router.GET("/api/workspaces/{id}/changes", func(e *core.RequestEvent) error {
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			rows, err := t.Changed(repo, sinceOf(e.App, e.Request.URL.Query().Get("since")))
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			if rows == nil {
				rows = []tree.Churn{}
			}
			return e.JSON(http.StatusOK, map[string]any{"changes": rows})
		}).Bind(apis.RequireAuth())

		// And the same thing in words: the unified diff git writes, for one file.
		se.Router.GET("/api/workspaces/{id}/diff", func(e *core.RequestEvent) error {
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			doc := e.Request.URL.Query().Get("path")
			since := e.Request.URL.Query().Get("since")
			var out string
			if since == "last" {
				// "Nothing this cycle" is true and useless when what you wanted
				// was to see the last thing somebody did to this document.
				out, err = t.LastDiff(repo, doc)
			} else {
				out, err = t.Diff(repo, doc, sinceOf(e.App, since))
			}
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			// Los dos lados además del parche: un parche es lo que git imprime,
			// y dos documentos es lo que una vista de diff necesita. Los dos
			// salen de aquí para que nadie reconstruya un lado en el navegador.
			window := "last"
			if since != "last" {
				window = sinceOf(e.App, since)
			}
			before, _ := t.Before(repo, doc, window)
			now, _, _ := t.Read(repo, doc)
			return e.JSON(http.StatusOK, map[string]any{
				"path": doc, "since": since, "diff": out,
				"before": before, "after": now,
			})
		}).Bind(apis.RequireAuth())

		se.Router.GET("/api/workspaces/{id}/tree", func(e *core.RequestEvent) error {
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			entries, err := t.Tree(repo)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			if entries == nil {
				entries = []tree.Entry{}
			}
			return e.JSON(http.StatusOK, map[string]any{"entries": entries})
		}).Bind(apis.RequireAuth())

		se.Router.GET("/api/workspaces/{id}/document", func(e *core.RequestEvent) error {
			ws, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			doc, err := ReadDoc(e.App, t, ws, repo, e.Request.URL.Query().Get("path"))
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, doc)
		}).Bind(apis.RequireAuth())

		// Same three shapes as a thread's document, addressed by path. A path that
		// does not exist yet is created by writing to it with the hash of nothing —
		// which is how a wiki page comes into being without a record anywhere.
		se.Router.PATCH("/api/workspaces/{id}/document", func(e *core.RequestEvent) error {
			ws, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			var where struct {
				Path string `json:"path"`
			}
			if err := e.BindBody(&where); err != nil || where.Path == "" {
				return e.BadRequestError("say which `path`", err)
			}
			_, subject := ownerOf(e.App, ws, where.Path)
			return patchDocument(e, t, ws, repo, where.Path, subject)
		}).Bind(apis.RequireAuth())

		se.Router.GET("/api/workspaces/{id}/search", func(e *core.RequestEvent) error {
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			q := e.Request.URL.Query()
			// La aplicación web no pide contexto: su lista de resultados enseña la
			// línea, y el documento entero está a un clic.
			hits, err := t.Search(repo, q.Get("q"), atoiOr(q.Get("limit"), 50), 0)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			if hits == nil {
				hits = []tree.Hit{}
			}
			return e.JSON(http.StatusOK, map[string]any{"hits": hits})
		}).Bind(apis.RequireAuth())

		// Markdown → HTML, by the SAME renderer every document goes through.
		//
		// The web needs this for text that is being written and has not been
		// saved yet — a card's description in the planner, a preview beside an
		// editor. The alternative is a markdown renderer in the browser, which is
		// a second renderer: they agree until they do not, and then two screens
		// show the same document differently and nobody can say which is right.
		se.Router.POST("/api/markdown", func(e *core.RequestEvent) error {
			var body struct {
				Content string `json:"content"`
				// Optional, and only for one thing: `assets/x.png` is resolvable
				// against a workspace and nowhere else. Without it the preview
				// still renders — with a picture that does not load, which is
				// the honest outcome of asking to render a document outside the
				// place its files live.
				Workspace string `json:"workspace"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.BadRequestError("could not read the body", err)
			}
			html := md.RenderHTML(body.Content)
			if body.Workspace != "" {
				ws, _, err := reachWorkspace(e, body.Workspace)
				if err != nil {
					return err
				}
				html = withAssets(html, ws.Id)
			}
			return e.JSON(http.StatusOK, map[string]any{"html": html})
		}).Bind(apis.RequireAuth())

		// The one document this server ships, served as markdown so a browser, a
		// curl and the web UI reach the same bytes the MCP prompt does.
		se.Router.GET("/api/guide", func(e *core.RequestEvent) error {
			e.Response.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, err := e.Response.Write([]byte(prompts.Guide))
			return err
		})

		// Lo que un agente se pega a sí mismo, y la plantilla que una persona
		// copia para configurar el suyo. Markdown como la guía, por la misma
		// razón: un navegador, un curl y la aplicación leen los mismos bytes.
		//
		// `connect.md` sale SIN rellenar. Los dos huecos —la URL del servidor y
		// el token— los pone el cliente, que sabe con certeza por qué dirección
		// llegó; el servidor tendría que adivinarla de `Host` y de las cabeceras
		// que ponga el proxy de enfrente, y un prompt con la URL equivocada es
		// un prompt que configura un servidor que no existe. Y el token no
		// vuelve a viajar por una respuesta que nadie necesitaba.
		se.Router.GET("/api/house-rules", func(e *core.RequestEvent) error {
			e.Response.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, err := e.Response.Write([]byte(prompts.House))
			return err
		})

		se.Router.GET("/api/connect", func(e *core.RequestEvent) error {
			e.Response.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, err := e.Response.Write([]byte(prompts.Connect))
			return err
		})

		se.Router.DELETE("/api/workspaces/{id}/document", func(e *core.RequestEvent) error {
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			doc := e.Request.URL.Query().Get("path")
			area, err := tree.Classify(doc)
			// A thread's own DOCUMENT is the thread's to remove, by deleting the
			// thread: letting it go from here would leave a row pointing at
			// nothing. What a thread wrote BESIDE it — `threads/14-x/notas.md` —
			// is an ordinary file and goes like any other.
			if err != nil || (area == tree.AreaThread && path.Dir(doc) == tree.DirThreads) {
				return e.BadRequestError(
					"a thread's own document goes when the thread does", err)
			}
			if err := t.Remove(repo, doc, actorLabel(e.Auth), "delete: "+doc); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"path": doc, "deleted": true})
		}).Bind(apis.RequireAuth())

		return se.Next()
	})
}

// patchDocument parses an HTTP body and hands it to bubble.Apply, which is the
// one place a document actually changes. The MCP tools call the same function
// with the same arguments; only the door differs.
//
//	{"base": h, "content": "..."}                    replace the whole file
//	{"base": h, "edits": [{"old": …, "new": …}]}      surgical, by quoting
//	{"base": h, "todo": {"index": 0, "done": true}}   tick a checkbox
//
// `base` is the hash the caller read. It is the only thing standing between two
// writers and a lost paragraph, so it is required.
func patchDocument(e *core.RequestEvent, t *tree.Tree, ws *core.Record, repo, doc, subject string) error {
	var body struct {
		Base    string  `json:"base"`
		Message string  `json:"message"`
		Content *string `json:"content"`
		Edits   []Edit  `json:"edits"`
		Todo    *Todo   `json:"todo"`
	}
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("could not read the body", err)
	}
	out, err := Apply(e.App, e.Auth, t, ws, repo, doc, subject, Patch{
		Base: body.Base, Message: body.Message,
		Content: body.Content, Edits: body.Edits, Todo: body.Todo,
	})
	if err != nil {
		// A conflict is not a bad request: the body was fine and somebody else
		// simply wrote first. It gets its own status so a client can offer to
		// reload instead of guessing from the sentence.
		if errors.Is(err, ErrConflict) {
			return e.Error(http.StatusConflict, err.Error(), err)
		}
		return e.BadRequestError(err.Error(), err)
	}
	return e.JSON(http.StatusOK, out)
}

// reachWorkspace is reachThread's other half: same question, one level up.
func reachWorkspace(e *core.RequestEvent, id string) (*core.Record, string, error) {
	if e.Auth == nil || (!isPersonAuth(e.Auth) && !IsSuperuser(e.Auth)) {
		return nil, "", e.NotFoundError("", nil)
	}
	ws, err := e.App.FindRecordById("workspaces", id)
	if err != nil {
		return nil, "", e.NotFoundError("", err)
	}
	if !IsSuperuser(e.Auth) && e.Auth.GetString("role") != "lead" {
		var n int
		err := e.App.DB().NewQuery(
			`SELECT COUNT(*) FROM memberships WHERE workspace = {:ws} AND user = {:u}`).
			Bind(map[string]any{"ws": ws.Id, "u": e.Auth.Id}).Row(&n)
		if err != nil || n == 0 {
			return nil, "", e.NotFoundError("", nil)
		}
	}
	repo := ws.GetString("repo_path")
	if repo == "" {
		return nil, "", e.BadRequestError("this workspace has no repository", nil)
	}
	return ws, repo, nil
}

// ownerOf resolves a path to the thread that owns it, if any.
//
// This IS the belonging rule. A path under threads/ belongs to a thread even when
// it is reached by path, so both the commit message and the EVENT name that
// thread; everything else belongs to the workspace. Nothing is written that is not
// attributed.
func ownerOf(app core.App, ws *core.Record, doc string) (threadID, subject string) {
	if area, err := tree.Classify(doc); err == nil && area == tree.AreaThread {
		// A thread's own document, or anything it wrote beside it: `notas.md`
		// inside `threads/14-portar/` is the thread's writing as much as its
		// document is, so it warms the thread rather than the workspace.
		main := doc
		if dir := path.Dir(doc); dir != tree.DirThreads {
			main = dir + ".md"
		}
		th, err := app.FindFirstRecordByFilter("threads",
			"workspace = {:ws} && doc_path = {:p}",
			map[string]any{"ws": ws.Id, "p": main})
		if err == nil {
			return th.Id, th.GetString("name")
		}
	}
	return "", doc
}

// pagesDir is the folder a thread keeps its other files in:
// `threads/14-x.md` → `threads/14-x`.
func pagesDir(docPath string) string { return strings.TrimSuffix(docPath, ".md") }

// pagesOf lists what a thread wrote BESIDE its document.
func pagesOf(t *tree.Tree, repo, docPath string) []string {
	entries, err := t.Tree(repo)
	if err != nil {
		return nil
	}
	dir := pagesDir(docPath) + "/"
	var out []string
	for _, e := range entries {
		if !e.Dir && strings.HasPrefix(e.Path, dir) {
			out = append(out, e.Path)
		}
	}
	return out
}

func isPersonAuth(auth *core.Record) bool {
	return auth != nil && auth.Collection().Name == "users"
}

// reachThread loads a thread and its repository, refusing anybody who could not
// see the thread through the record API.
//
// The routes are our own, so PocketBase's collection rules do not apply to them —
// which means the boundary has to be re-stated here rather than inherited. It is
// the same question the rules ask: are you a member of this thread's workspace, or
// the global lead.
func reachThread(e *core.RequestEvent, id string) (*core.Record, string, error) {
	if e.Auth == nil || (!isPersonAuth(e.Auth) && !IsSuperuser(e.Auth)) {
		return nil, "", e.NotFoundError("", nil)
	}
	th, err := e.App.FindRecordById("threads", id)
	if err != nil {
		return nil, "", e.NotFoundError("", err)
	}
	if !IsSuperuser(e.Auth) && e.Auth.GetString("role") != "lead" {
		var n int
		err := e.App.DB().NewQuery(
			`SELECT COUNT(*) FROM memberships WHERE workspace = {:ws} AND user = {:u}`).
			Bind(map[string]any{"ws": th.GetString("workspace"), "u": e.Auth.Id}).Row(&n)
		if err != nil || n == 0 {
			// Invisible rather than forbidden, the same answer the rules give.
			return nil, "", e.NotFoundError("", nil)
		}
	}
	repo, err := repoOf(e.App, th)
	if err != nil {
		return nil, "", e.NotFoundError("", err)
	}
	if th.GetString("doc_path") == "" {
		return nil, "", e.BadRequestError("this thread has no document path", nil)
	}
	return th, repo, nil
}

func repoOf(app core.App, thread *core.Record) (string, error) {
	ws, err := app.FindRecordById("workspaces", thread.GetString("workspace"))
	if err != nil {
		return "", err
	}
	repo := ws.GetString("repo_path")
	if repo == "" {
		return "", fmt.Errorf("workspace %s has no repo_path", ws.Id)
	}
	return repo, nil
}

func stampDocPath(app core.App, thread *core.Record) error {
	thread.Set("doc_path", docPathFor(thread))
	return app.Save(thread)
}

// docPathFor is `threads/<seq>-<name>.md`, flat.
//
// Flat rather than a directory per bubble: a thread can be re-filed, and a path
// that encodes its bubble goes stale the moment it is. The bubble is a column;
// the tree stays something a person can read without it.
func docPathFor(thread *core.Record) string {
	slug := slugify(thread.GetString("name"))
	if slug == "" {
		slug = "thread"
	}
	return fmt.Sprintf("threads/%d-%s.md", thread.GetInt("seq"), slug)
}

func actorLabel(auth *core.Record) string {
	if auth == nil {
		return "bubble"
	}
	if e := auth.GetString("email"); e != "" {
		return e
	}
	return auth.Id
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = notSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// sinceOf turns the window a caller asked for into an instant git understands.
//
// Empty or "cycle" means the calibration's own window — the one the bands are
// computed against — read from `tuning` rather than hard-coded, so recalibrating
// moves this number with everything else. Anything else is passed through: a
// caller that knows the instant it wants should get it.
func sinceOf(app core.App, since string) string {
	if since != "" && since != "cycle" {
		return since
	}
	tun, err := tuningOf(app)
	if err != nil {
		return time.Now().UTC().Add(-168 * time.Hour).Format(time.RFC3339)
	}
	return time.Now().UTC().Add(-tun.Cycle()).Format(time.RFC3339)
}
