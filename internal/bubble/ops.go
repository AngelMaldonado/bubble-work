package bubble

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

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

// IsSuperuser reports whether the caller is operating the box rather than working
// in it.
//
// A superuser bypasses every PocketBase collection rule, so refusing them on OUR
// routes made the two halves disagree: everything visible in the dashboard, and a
// 404 from the board. They READ everything here for the same reason.
//
// They do not write. Every write in this system is attributed to a person —
// events.actor, a git commit's author, who signed a comment — and a superuser has
// no row in `users` to attribute it to. The same human can hold both accounts,
// with the same email; working is what the person account is for.
func IsSuperuser(auth *core.Record) bool {
	return auth != nil && auth.Collection().Name == core.CollectionNameSuperusers
}

// ErrNotAPerson is a write attempted by something the model cannot attribute.
var ErrNotAPerson = errors.New(
	"a write is attributed to a person — sign in as one rather than as a superuser")

// canSee answers the one question every operation starts with.
func canSee(app core.App, auth *core.Record, workspace string) bool {
	if IsSuperuser(auth) {
		return true // reads only; Apply refuses the write itself
	}
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
//
// `content` is the record — the markdown, which is what a write diffs against
// and what an agent quotes from. `html` is the SAME text rendered, and it is
// rendered HERE rather than in the browser so that every surface showing a
// document shows the identical thing. Two renderers agree until they do not.
type Doc struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Thread    string `json:"thread,omitempty"`
	Hash      string `json:"hash"`
	Content   string `json:"content"`
	HTML      string `json:"html"`
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
		Hash: hash, Content: content, HTML: withAssets(md.RenderHTML(content), ws.Id),
		Done: md.CountDone(content),
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
	if !isPersonAuth(auth) {
		return Doc{}, ErrNotAPerson
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
		Hash: hash, Content: next, HTML: withAssets(md.RenderHTML(next), ws.Id),
		Done: md.CountDone(next),
	}, nil
}

// CreateThread makes a thread and stamps everything the server owns: its number,
// its document path, and the evidence that defining a piece of work is production.
func CreateThread(app core.App, auth *core.Record, ws *core.Record,
	name, bubbleID, impact, urgency string) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}

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
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
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
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
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

// assetRef matches an image or a link pointing INTO `assets/`, which is how the
// on-disk layout says a picture is referenced from anywhere.
var assetRef = regexp.MustCompile(`(src|href)="assets/([^"]+)"`)

// withAssets turns `assets/x.png` into the route that actually serves it.
//
// The markdown keeps the short form on purpose: a document that spells out
// `/api/workspaces/<id>/file?path=…` is a document that cannot be moved, read
// from a git clone, or written by hand — and the on-disk layout is explicit that
// a picture is `assets/<name>` from anywhere. But a relative path in a
// single-page app resolves against the ADDRESS, so the same string in a thread
// at `/w/alpha/t/14` asks for `/w/alpha/t/14/assets/x.png` and gets the
// application shell back.
//
// So the short form is what is stored and the route is what is rendered. Done
// here rather than in `internal/md` because the workspace is what makes it
// resolvable, and the renderer does not know about workspaces.
func withAssets(html, workspace string) string {
	return assetRef.ReplaceAllString(
		html, `$1="/api/workspaces/`+workspace+`/file?path=assets/$2"`,
	)
}

// ---- the rest of the surface, for the other door -------------------------
//
// Everything below exists because the MCP grew a hole: an agent could write a
// thread's document and nothing else. It could not say what the work is FOR,
// when it is due, who is accountable for the bubble around it, or read what has
// already happened to it — so it worked blind and asked a person for everything
// that was not text.
//
// These are plain functions like the ones above, and the rule is the same one:
// the caller's own permissions, checked here, because `app.Save` does not know
// about collection rules. Each one states which rule it is mirroring.

// isGlobalLead is the department head — the role that sees every workspace and
// owns the strategic layer.
func isGlobalLead(auth *core.Record) bool {
	return isPersonAuth(auth) && auth.GetString("role") == "lead"
}

// CreateBubble makes one. Mirrors `bubbles.CreateRule` — any member.
func CreateBubble(app core.App, auth *core.Record, ws *core.Record, name, outcome string) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("a bubble needs a name")
	}
	col, err := app.FindCollectionByNameOrId("bubbles")
	if err != nil {
		return nil, err
	}
	r := core.NewRecord(col)
	r.Set("workspace", ws.Id)
	r.Set("name", name)
	r.Set("outcome", strings.TrimSpace(outcome))
	if err := app.Save(r); err != nil {
		return nil, err
	}
	// Creating a bubble is not evidence: an empty grouping has produced
	// nothing. The threads inside it are what warm it.
	return r, nil
}

// BubbleEdit is what may be said about a bubble from outside.
//
// Pointers, so "not mentioned" and "set to empty" are different requests:
// clearing an outcome and leaving it alone are not the same statement.
type BubbleEdit struct {
	Name    *string   `json:"name,omitempty"`
	Outcome *string   `json:"outcome,omitempty"`
	Owners  *[]string `json:"owners,omitempty" jsonschema:"user ids; empty means nobody is accountable"`
	Closure *string   `json:"closure,omitempty" jsonschema:"how it ended, in your words"`
	Closed  *bool     `json:"closed,omitempty" jsonschema:"true closes it, false reopens it"`
}

// SetBubble edits one. Mirrors `bubbles.UpdateRule` — any member.
func SetBubble(app core.App, auth *core.Record, id string, in BubbleEdit) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	b, err := app.FindRecordById("bubbles", id)
	if err != nil {
		return nil, ErrDenied
	}
	if _, _, err := WorkspaceFor(app, auth, b.GetString("workspace")); err != nil {
		return nil, err
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, fmt.Errorf("a bubble needs a name")
		}
		b.Set("name", strings.TrimSpace(*in.Name))
	}
	if in.Outcome != nil {
		b.Set("outcome", *in.Outcome)
	}
	if in.Owners != nil {
		b.Set("owners", *in.Owners)
	}
	if in.Closure != nil {
		b.Set("closure", *in.Closure)
	}
	if in.Closed != nil {
		if *in.Closed {
			b.Set("closed_at", time.Now().UTC())
		} else {
			// Reopening clears the sentence too: it described an ending that no
			// longer holds.
			b.Set("closed_at", "")
			b.Set("closure", "")
		}
	}
	if err := app.Save(b); err != nil {
		return nil, err
	}
	return b, nil
}

// ThreadEdit is what may be said about a thread that is not its document.
type ThreadEdit struct {
	Name      *string `json:"name,omitempty" jsonschema:"renaming moves its file, and git follows"`
	Bubble    *string `json:"bubble,omitempty" jsonschema:"a bubble id in the same workspace; empty takes it out"`
	Objective *string `json:"objective,omitempty" jsonschema:"what this work is FOR; empty unfiles it"`
	Due       *string `json:"due,omitempty" jsonschema:"a day, 2026-09-15; empty clears it"`
	Impact    *string `json:"impact,omitempty" jsonschema:"high, mid or low"`
	Urgency   *string `json:"urgency,omitempty" jsonschema:"high, mid or low"`
}

var dayOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// SetThread edits the record around the document. Mirrors `threads.UpdateRule`
// — any member of its workspace.
//
// Priority is NOT here, and cannot be: it is derived from impact × urgency by
// the server, and a second way to write it would be a second answer.
func SetThread(app core.App, auth *core.Record, id string, in ThreadEdit) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	th, ws, _, err := ThreadFor(app, auth, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, fmt.Errorf("a thread needs a name")
		}
		th.Set("name", strings.TrimSpace(*in.Name))
	}
	if in.Bubble != nil {
		if *in.Bubble != "" {
			b, err := app.FindRecordById("bubbles", *in.Bubble)
			if err != nil || b.GetString("workspace") != ws.Id {
				return nil, fmt.Errorf("bubble %q is not in this workspace", *in.Bubble)
			}
		}
		th.Set("bubble", *in.Bubble)
	}
	if in.Objective != nil {
		if *in.Objective != "" {
			if _, err := app.FindRecordById("objectives", *in.Objective); err != nil {
				return nil, fmt.Errorf("no objective %q", *in.Objective)
			}
		}
		// An objective belongs to the DEPARTMENT, so a thread from any project
		// may hang from any of them. That is the point of the strategic layer,
		// not a leak.
		th.Set("objective", *in.Objective)
	}
	if in.Due != nil {
		day := strings.TrimSpace(*in.Due)
		if day != "" && !dayOnly.MatchString(day) {
			return nil, fmt.Errorf("a due date is a day: 2026-09-15, not %q", day)
		}
		if day == "" {
			th.Set("due_date", "")
		} else {
			th.Set("due_date", day+" 00:00:00.000Z")
		}
	}
	for field, v := range map[string]*string{"impact": in.Impact, "urgency": in.Urgency} {
		if v == nil {
			continue
		}
		if *v != "" && *v != "high" && *v != "mid" && *v != "low" {
			return nil, fmt.Errorf("%s is high, mid or low — not %q", field, *v)
		}
		th.Set(field, *v)
	}
	// Renaming moves the file: the hook on the record request does it for the
	// web, and this door goes through the same app, so it happens here too.
	if err := app.Save(th); err != nil {
		return nil, err
	}
	return th, nil
}

// Plan is the department's strategic layer, as one answer: what the work is for,
// and what has been captured and not yet decided about.
type Plan struct {
	Objectives []PlanObjective `json:"objectives"`
	Inbox      []PlanNote      `json:"inbox"`
}

type PlanObjective struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Outcome string `json:"outcome,omitempty"`
	Due     string `json:"due_date,omitempty"`
	Threads int    `json:"threads"`
}

type PlanNote struct {
	ID     string `json:"id"`
	Note   string `json:"note"`
	Thread string `json:"thread,omitempty"`
	When   string `json:"captured_at"`
}

// ReadPlan mirrors the rules the collections carry: the objectives are the
// global lead's to read, and a note is read by whoever captured it or by the
// person whose job triaging is.
func ReadPlan(app core.App, auth *core.Record) (Plan, error) {
	if auth == nil {
		return Plan{}, ErrDenied
	}
	out := Plan{Objectives: []PlanObjective{}, Inbox: []PlanNote{}}
	lead := isGlobalLead(auth) || IsSuperuser(auth)
	if lead {
		rows, err := app.FindAllRecords("objectives")
		if err != nil {
			return Plan{}, err
		}
		for _, o := range rows {
			var n int
			_ = app.DB().NewQuery("SELECT COUNT(*) FROM threads WHERE objective = {:o}").
				Bind(dbx.Params{"o": o.Id}).Row(&n)
			out.Objectives = append(out.Objectives, PlanObjective{
				ID: o.Id, Name: o.GetString("name"), Outcome: o.GetString("outcome"),
				Due: o.GetDateTime("due_date").String(), Threads: n,
			})
		}
	}
	notes, err := app.FindAllRecords("inbox_items")
	if err != nil {
		return Plan{}, err
	}
	for _, n := range notes {
		if !lead && n.GetString("captured_by") != auth.Id {
			continue
		}
		out.Inbox = append(out.Inbox, PlanNote{
			ID: n.Id, Note: n.GetString("note"), Thread: n.GetString("thread"),
			When: n.GetDateTime("created").String(),
		})
	}
	return out, nil
}

// Capture puts a note in the department's inbox. Mirrors
// `inbox_items.CreateRule` — anybody signed in — and stamps the author like a
// comment: nobody captures as somebody else.
//
// Capturing is NOT evidence. A note has no outcome and no output; the thread it
// becomes is what warms anything.
func Capture(app core.App, auth *core.Record, note string) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return nil, fmt.Errorf("a note needs something in it")
	}
	col, err := app.FindCollectionByNameOrId("inbox_items")
	if err != nil {
		return nil, err
	}
	r := core.NewRecord(col)
	r.Set("note", note)
	r.Set("captured_by", auth.Id)
	if err := app.Save(r); err != nil {
		return nil, err
	}
	return r, nil
}

// SetObjective creates or edits one. Mirrors `objectives` — the global lead
// shapes the strategic layer, and only they.
func SetObjective(app core.App, auth *core.Record, id, name, outcome, due string) (*core.Record, error) {
	if !isGlobalLead(auth) {
		return nil, ErrDenied
	}
	var r *core.Record
	if id != "" {
		found, err := app.FindRecordById("objectives", id)
		if err != nil {
			return nil, fmt.Errorf("no objective %q", id)
		}
		r = found
	} else {
		col, err := app.FindCollectionByNameOrId("objectives")
		if err != nil {
			return nil, err
		}
		r = core.NewRecord(col)
	}
	if name != "" {
		r.Set("name", strings.TrimSpace(name))
	}
	if r.GetString("name") == "" {
		return nil, fmt.Errorf("an objective needs a name")
	}
	if outcome != "" {
		r.Set("outcome", outcome)
	}
	if due != "" {
		if !dayOnly.MatchString(due) {
			return nil, fmt.Errorf("a date is a day: 2026-09-15, not %q", due)
		}
		r.Set("due_date", due+" 00:00:00.000Z")
	}
	if err := app.Save(r); err != nil {
		return nil, err
	}
	return r, nil
}

// Moment is one thing that happened to a thread.
type Moment struct {
	Kind  string `json:"kind"`
	At    string `json:"at"`
	Actor string `json:"actor,omitempty"`
	Meta  string `json:"meta,omitempty"`
}

// Timeline is what a thread's log adds up to, newest first.
//
// This is the read that keeps an agent from repeating work: heat says a thread
// is warm, and the timeline says WHAT made it warm and when.
func Timeline(app core.App, auth *core.Record, threadID string, limit int) ([]Moment, error) {
	th, _, _, err := ThreadFor(app, auth, threadID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := app.FindRecordsByFilter("events", "target = {:t}", "-at", limit, 0,
		dbx.Params{"t": th.Id})
	if err != nil {
		return nil, err
	}
	out := make([]Moment, 0, len(rows))
	for _, e := range rows {
		out = append(out, Moment{
			Kind: e.GetString("kind"), At: e.GetDateTime("at").String(),
			Actor: e.GetString("actor"), Meta: e.GetString("meta"),
		})
	}
	return out, nil
}

// RemovePath deletes a document, with the one distinction the web route makes:
// a thread's OWN document goes when the thread does — letting it go from here
// would leave a row pointing at nothing — while everything else, a wiki page or
// a file a thread wrote beside its document, goes like any other file.
func RemovePath(app core.App, auth *core.Record, t *tree.Tree, ws *core.Record, repo, doc string) error {
	if !isPersonAuth(auth) {
		return ErrNotAPerson
	}
	area, err := tree.Classify(doc)
	if err != nil || (area == tree.AreaThread && path.Dir(doc) == tree.DirThreads) {
		return fmt.Errorf("a thread's own document goes when the thread does")
	}
	return t.Remove(repo, doc, actorLabel(auth), "delete: "+doc)
}
