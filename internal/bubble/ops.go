package bubble

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/tree"
)

// The operations, as plain functions.
//
// Every capability lands on more than one surface — the REST routes and the MCP
// tools — and the logic lives HERE so they cannot drift. A surface's job is to
// parse its own kind of request and render its own kind of answer; what actually
// happens is one function, called from both doors.
//
// They take `app` and the authenticated person rather than a request, so nothing
// in here depends on how the caller arrived.

// ErrDenied means invisible rather than forbidden — the same answer the collection
// rules give, and for the same reason: a 404 tells you nothing about what exists.
var ErrDenied = errors.New("not found")

// ErrConflict is a write against a version somebody has already replaced.
var ErrConflict = errors.New("the document changed since you read it")

// canSee answers the one question every operation starts with.
func canSee(app core.App, auth *core.Record, workspace string) bool {
	if auth == nil || auth.Collection().Name != "users" {
		return false
	}
	if auth.GetString("role") == "lead" {
		return true // the global lead sees every workspace
	}
	var n int
	err := app.DB().NewQuery(
		`SELECT COUNT(*) FROM memberships WHERE workspace = {:ws} AND user = {:u}`).
		Bind(dbx.Params{"ws": workspace, "u": auth.Id}).Row(&n)
	return err == nil && n > 0
}

// WorkspaceFor loads a workspace the caller may see, with its repository.
func WorkspaceFor(app core.App, auth *core.Record, id string) (*core.Record, string, error) {
	ws, err := app.FindRecordById("workspaces", id)
	if err != nil {
		// Also accept the slug: an agent reads slugs, not ids.
		ws, err = app.FindFirstRecordByData("workspaces", "slug", id)
		if err != nil {
			return nil, "", ErrDenied
		}
	}
	if !canSee(app, auth, ws.Id) {
		return nil, "", ErrDenied
	}
	repo := ws.GetString("repo_path")
	if repo == "" {
		return nil, "", fmt.Errorf("workspace %s has no repository", ws.Id)
	}
	return ws, repo, nil
}

// ThreadFor loads a thread the caller may see, with its workspace and repository.
func ThreadFor(app core.App, auth *core.Record, id string) (*core.Record, *core.Record, string, error) {
	th, err := app.FindRecordById("threads", id)
	if err != nil {
		return nil, nil, "", ErrDenied
	}
	ws, repo, err := WorkspaceFor(app, auth, th.GetString("workspace"))
	if err != nil {
		return nil, nil, "", err
	}
	return th, ws, repo, nil
}

// Doc is a document as anyone reads it.
type Doc struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Thread    string `json:"thread,omitempty"`
	Hash      string `json:"hash"`
	Content   string `json:"content"`
	Done      int    `json:"done"`
}

// ReadDoc returns a document by path within a workspace.
func ReadDoc(app core.App, t *tree.Tree, ws *core.Record, repo, path string) (Doc, error) {
	content, hash, err := t.Read(repo, path)
	if err != nil {
		return Doc{}, err
	}
	threadID, _ := ownerOf(app, ws, path)
	return Doc{
		Workspace: ws.Id, Path: path, Thread: threadID,
		Hash: hash, Content: content, Done: md.CountDone(content),
	}, nil
}

// Edit is one surgical change: quote what is there, give what replaces it.
type Edit struct {
	Old string `json:"old"`
	New string `json:"new"`
	All bool   `json:"all,omitempty"`
}

// Todo addresses a checkbox by position, with the text as a guard.
type Todo struct {
	Index int    `json:"index"`
	Text  string `json:"text,omitempty"`
	Done  bool   `json:"done"`
}

// Patch is a change to a document: exactly one of Content, Edits or Todo.
type Patch struct {
	Base    string
	Message string
	Content *string
	Edits   []Edit
	Todo    *Todo
}

// Apply performs a patch: check the base, compute the new text, write it, commit
// it, and record the evidence.
func Apply(app core.App, auth *core.Record, t *tree.Tree,
	ws *core.Record, repo, path, subject string, p Patch) (Doc, error) {

	if path == "" {
		return Doc{}, fmt.Errorf("no document path")
	}
	shapes := 0
	for _, present := range []bool{p.Content != nil, len(p.Edits) > 0, p.Todo != nil} {
		if present {
			shapes++
		}
	}
	if shapes != 1 {
		return Doc{}, fmt.Errorf("say exactly one of content, edits or todo")
	}

	cur, curHash, err := t.Read(repo, path)
	if err != nil {
		return Doc{}, err
	}
	// Checked here as well as inside the write, so a patch is applied to the
	// version the caller actually saw rather than to whatever is there now.
	if p.Base != curHash {
		return Doc{}, fmt.Errorf("%w (on disk %s, you had %s)", ErrConflict, curHash, p.Base)
	}

	threadID, _ := ownerOf(app, ws, path)
	if subject == "" {
		subject = path
	}
	next := cur
	msg := strings.TrimSpace(p.Message)

	switch {
	case p.Content != nil:
		next = *p.Content
		if msg == "" {
			msg = "write: " + subject
		}
	case len(p.Edits) > 0:
		edits := make([]md.Edit, 0, len(p.Edits))
		for _, x := range p.Edits {
			edits = append(edits, md.Edit{Old: x.Old, New: x.New, All: x.All})
		}
		if next, err = md.ApplyEdits(cur, edits); err != nil {
			return Doc{}, err // "quote more of it" is the useful half
		}
		if msg == "" {
			msg = "edit: " + subject
		}
	default:
		if next, err = md.ToggleTodo(cur, p.Todo.Index, p.Todo.Text, p.Todo.Done); err != nil {
			return Doc{}, err
		}
		if msg == "" {
			verb := "untick"
			if p.Todo.Done {
				verb = "tick"
			}
			msg = verb + ": " + subject
		}
	}

	hash, err := t.Write(repo, path, p.Base, next, actorLabel(auth), msg)
	if err != nil {
		return Doc{}, err
	}

	// Evidence, and only when something actually moved. A write that leaves the
	// file byte-identical is not production — it is somebody pressing save.
	if next != cur {
		actor := ""
		if isPersonAuth(auth) {
			actor = auth.Id
		}
		if threadID != "" {
			record(app, ws.Id, "thread", threadID, EvDocumentChanged, actor,
				map[string]any{"path": path, "done": md.CountDone(next)})
		} else {
			record(app, ws.Id, "workspace", ws.Id, EvDocChanged, actor,
				map[string]any{"path": path})
		}
	}

	return Doc{
		Workspace: ws.Id, Path: path, Thread: threadID,
		Hash: hash, Content: next, Done: md.CountDone(next),
	}, nil
}

// CreateThread makes a thread and stamps everything the server owns: its number,
// its document path, and the evidence that defining a piece of work is production.
func CreateThread(app core.App, auth *core.Record, ws *core.Record,
	name, bubbleID, impact, urgency string) (*core.Record, error) {

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("a thread needs a name")
	}
	col, err := app.FindCollectionByNameOrId("threads")
	if err != nil {
		return nil, err
	}
	if bubbleID != "" {
		b, err := app.FindRecordById("bubbles", bubbleID)
		if err != nil || b.GetString("workspace") != ws.Id {
			return nil, fmt.Errorf("bubble %q is not in this workspace", bubbleID)
		}
	}

	r := core.NewRecord(col)
	r.Set("workspace", ws.Id)
	r.Set("name", name)
	if bubbleID != "" {
		r.Set("bubble", bubbleID)
	}
	for k, v := range map[string]string{"impact": impact, "urgency": urgency} {
		if v != "" {
			r.Set(k, v)
		}
	}

	var next int
	if err := app.DB().NewQuery(
		"SELECT COALESCE(MAX(seq), 0) + 1 FROM threads WHERE workspace = {:ws}").
		Bind(dbx.Params{"ws": ws.Id}).Row(&next); err != nil {
		return nil, err
	}
	r.Set("seq", next)
	if err := app.Save(r); err != nil {
		return nil, err
	}
	r.Set("doc_path", docPathFor(r))
	if err := app.Save(r); err != nil {
		return nil, err
	}

	actor := ""
	if isPersonAuth(auth) {
		actor = auth.Id
	}
	record(app, ws.Id, "thread", r.Id, EvThreadCreated, actor, map[string]any{"name": name})
	return r, nil
}

// AddLink hangs external evidence off a thread. Landing one is production: it is
// proof that reality changed somewhere this tool cannot see.
func AddLink(app core.App, auth *core.Record, threadID, url, title string) (*core.Record, error) {
	th, ws, _, err := ThreadFor(app, auth, threadID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("a link needs a url")
	}
	col, err := app.FindCollectionByNameOrId("thread_links")
	if err != nil {
		return nil, err
	}
	r := core.NewRecord(col)
	r.Set("thread", th.Id)
	r.Set("url", url)
	r.Set("title", title)
	if isPersonAuth(auth) {
		r.Set("added_by", auth.Id)
	}
	if err := app.Save(r); err != nil {
		return nil, err
	}
	actor := ""
	if isPersonAuth(auth) {
		actor = auth.Id
	}
	record(app, ws.Id, "thread", th.Id, EvLinkAdded, actor, map[string]any{"url": url})
	return r, nil
}

// CompleteThread moves a thread into the workspace's completed state.
//
// Finishing is a position somebody takes. Nothing is graded on the way out: v0
// refused to complete a thread with an unticked Definition of Done, and a
// checklist written days ago is evidence, not a warden.
func CompleteThread(app core.App, auth *core.Record, threadID string) (*core.Record, error) {
	th, ws, _, err := ThreadFor(app, auth, threadID)
	if err != nil {
		return nil, err
	}
	done, err := app.FindFirstRecordByFilter("states",
		"workspace = {:ws} && group = 'completed'", dbx.Params{"ws": ws.Id})
	if err != nil {
		return nil, fmt.Errorf("this workspace has no completed state — a lead defines one first")
	}
	was := completedState(app, th.GetString("state"))
	th.Set("state", done.Id)
	if err := app.Save(th); err != nil {
		return nil, err
	}
	if !was {
		actor := ""
		if isPersonAuth(auth) {
			actor = auth.Id
		}
		record(app, ws.Id, "thread", th.Id, EvThreadCompleted, actor,
			map[string]any{"name": th.GetString("name")})
	}
	return th, nil
}
