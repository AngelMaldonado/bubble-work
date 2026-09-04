package bubble

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/tree"
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
		e.Record.Set("doc_path", newPath)
		return e.App.Save(e.Record)
	})

	app.OnRecordAfterDeleteSuccess("threads").BindFunc(func(e *core.RecordEvent) error {
		repo, err := repoOf(app, e.Record)
		if err == nil {
			if err := t.Remove(repo, e.Record.GetString("doc_path"), "bubble", "deleted: "+e.Record.GetString("name")); err != nil {
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
			content, hash, err := t.Read(repo, th.GetString("doc_path"))
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{
				"thread":  th.Id,
				"path":    th.GetString("doc_path"),
				"hash":    hash,
				"content": content,
			})
		}).Bind(apis.RequireAuth())

		se.Router.PATCH("/api/threads/{id}/document", func(e *core.RequestEvent) error {
			th, repo, err := reachThread(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			return patchDocument(e, t, th.GetString("workspace"), repo,
				th.GetString("doc_path"), th.Id, th.GetString("name"))
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
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			doc := e.Request.URL.Query().Get("path")
			content, hash, err := t.Read(repo, doc)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{
				"path": doc, "hash": hash, "content": content,
			})
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
			threadID, subject := ownerOf(e.App, ws, where.Path)
			return patchDocument(e, t, ws.Id, repo, where.Path, threadID, subject)
		}).Bind(apis.RequireAuth())

		se.Router.DELETE("/api/workspaces/{id}/document", func(e *core.RequestEvent) error {
			_, repo, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			doc := e.Request.URL.Query().Get("path")
			// A thread's document is the thread's to remove, by deleting the thread.
			// Letting it go from here would leave a row pointing at nothing.
			if area, err := tree.Classify(doc); err != nil || area == tree.AreaThread {
				return e.BadRequestError("only documents under docs/ are removed this way", err)
			}
			if err := t.Remove(repo, doc, actorLabel(e.Auth), "delete: "+doc); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{"path": doc, "deleted": true})
		}).Bind(apis.RequireAuth())

		return se.Next()
	})
}

// patchDocument is the one way a document changes, whichever door it came through.
//
// ONE endpoint shape for every kind of change, because an agent that has to pick
// between three verbs picks wrong. The body says which shape it is; exactly one
// may be present:
//
//	{"base": h, "content": "..."}                    replace the whole file
//	{"base": h, "edits": [{"old": …, "new": …}]}      surgical, by quoting
//	{"base": h, "todo": {"index": 0, "done": true}}   tick a checkbox
//
// `base` is the hash the caller read. It is the only thing standing between two
// writers and a lost paragraph, so it is required — an edit is always against a
// version somebody has seen.
func patchDocument(e *core.RequestEvent, t *tree.Tree, workspace, repo, doc, threadID, subject string) error {
	if doc == "" {
		return e.BadRequestError("this document has no path", nil)
	}
	var body struct {
		Base    string  `json:"base"`
		Message string  `json:"message"`
		Content *string `json:"content"`
		Edits   []struct {
			Old string `json:"old"`
			New string `json:"new"`
			All bool   `json:"all"`
		} `json:"edits"`
		Todo *struct {
			Index int    `json:"index"`
			Text  string `json:"text"`
			Done  bool   `json:"done"`
		} `json:"todo"`
	}
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("could not read the body", err)
	}

	shapes := 0
	for _, present := range []bool{body.Content != nil, len(body.Edits) > 0, body.Todo != nil} {
		if present {
			shapes++
		}
	}
	if shapes != 1 {
		return e.BadRequestError("say exactly one of `content`, `edits` or `todo`", nil)
	}

	cur, curHash, err := t.Read(repo, doc)
	if err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	// Checked here as well as inside the write, so an edit is applied to the version
	// the caller actually saw rather than to whatever is there now.
	if body.Base != curHash {
		return e.BadRequestError(fmt.Sprintf(
			"the document changed since you read it (on disk %s, you had %s)", curHash, body.Base), nil)
	}

	next := cur
	msg := strings.TrimSpace(body.Message)
	switch {
	case body.Content != nil:
		next = *body.Content
		if msg == "" {
			msg = "write: " + subject
		}
	case len(body.Edits) > 0:
		edits := make([]md.Edit, 0, len(body.Edits))
		for _, x := range body.Edits {
			edits = append(edits, md.Edit{Old: x.Old, New: x.New, All: x.All})
		}
		next, err = md.ApplyEdits(cur, edits)
		if err != nil {
			// "quote more of it" is the useful half of this error, so it reaches the
			// caller verbatim.
			return e.BadRequestError(err.Error(), err)
		}
		if msg == "" {
			msg = "edit: " + subject
		}
	default:
		next, err = md.ToggleTodo(cur, body.Todo.Index, body.Todo.Text, body.Todo.Done)
		if err != nil {
			return e.BadRequestError(err.Error(), err)
		}
		if msg == "" {
			verb := "untick"
			if body.Todo.Done {
				verb = "tick"
			}
			msg = verb + ": " + subject
		}
	}

	hash, err := t.Write(repo, doc, body.Base, next, actorLabel(e.Auth), msg)
	if err != nil {
		return e.BadRequestError(err.Error(), err)
	}

	// Evidence, and only when something actually moved. A write that leaves the
	// file byte-identical is not production — it is somebody pressing save, which
	// is exactly the "activity is not evidence" line the model is built on.
	if next != cur {
		actor := ""
		if isPersonAuth(e.Auth) {
			actor = e.Auth.Id
		}
		if threadID != "" {
			record(e.App, workspace, "thread", threadID, EvDocumentChanged, actor,
				map[string]any{"path": doc, "done": md.CountDone(next)})
		} else {
			record(e.App, workspace, "workspace", workspace, EvDocChanged, actor,
				map[string]any{"path": doc})
		}
	}

	return e.JSON(http.StatusOK, map[string]any{
		"path": doc, "hash": hash, "content": next, "done": md.CountDone(next),
	})
}

// reachWorkspace is reachThread's other half: same question, one level up.
func reachWorkspace(e *core.RequestEvent, id string) (*core.Record, string, error) {
	if e.Auth == nil || e.Auth.Collection().Name != "users" {
		return nil, "", e.NotFoundError("", nil)
	}
	ws, err := e.App.FindRecordById("workspaces", id)
	if err != nil {
		return nil, "", e.NotFoundError("", err)
	}
	if e.Auth.GetString("role") != "lead" {
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
		th, err := app.FindFirstRecordByFilter("threads",
			"workspace = {:ws} && doc_path = {:p}",
			map[string]any{"ws": ws.Id, "p": doc})
		if err == nil {
			return th.Id, th.GetString("name")
		}
	}
	return "", doc
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
	if e.Auth == nil || e.Auth.Collection().Name != "users" {
		return nil, "", e.NotFoundError("", nil)
	}
	th, err := e.App.FindRecordById("threads", id)
	if err != nil {
		return nil, "", e.NotFoundError("", err)
	}
	if e.Auth.GetString("role") != "lead" {
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
