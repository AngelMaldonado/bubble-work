// Package mcpapi exposes the Bubble Work server as an MCP server that LLM
// clients (Claude Code, Codex) consume as first-class members (§9.5). This is
// OUR MCP — the framework's own front door, not Plane's MCP.
package mcpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// Backend is the server capability set the MCP tools call into. Keeping it an
// interface avoids an import cycle and mirrors the HTTP surface exactly.
type Backend interface {
	Bubbles(ctx context.Context) ([]domain.BubbleView, error)
	CreateWorkspace(ctx context.Context, req domain.CreateWorkspaceRequest) (domain.Workspace, error)
	RenameWorkspace(ctx context.Context, id, name string) (domain.Workspace, error)
	DeleteWorkspace(ctx context.Context, id string) (int, int, error)
	CreateBubble(ctx context.Context, req domain.CreateBubbleRequest) (domain.NewBubble, error)
	BirthThread(ctx context.Context, req domain.BirthRequest) (domain.BirthResult, error)
	SetContract(ctx context.Context, bubbleID string, in domain.ContractInput) (domain.Contract, error)
	CloseBubble(ctx context.Context, bubbleID string) error
	Timeline(ctx context.Context, bubbleID string) ([]domain.ThreadNode, error)
	ThreadDetail(ctx context.Context, threadID string) (domain.ThreadDetail, error)
	ThreadComments(ctx context.Context, threadID string) ([]domain.Comment, error)
	PostComment(ctx context.Context, threadID, body string) (domain.Comment, error)
	UpdateThread(ctx context.Context, threadID string, edit domain.ThreadEdit) (domain.ThreadDetail, error)
	MoveThread(ctx context.Context, threadID, bubbleID string) (domain.ThreadDetail, error)
	DeleteBubble(ctx context.Context, bubbleID string) (int, error)
	DeleteThread(ctx context.Context, threadID string) (int, error)
	DeleteRegion(ctx context.Context, threadID string, region md.Region) (domain.ThreadDetail, error)
	ToggleTodo(ctx context.Context, threadID string, region md.Region, index int, text string, done bool) (domain.ThreadDetail, error)
	AddRevision(ctx context.Context, threadID, title, body string) (domain.ThreadDetail, error)
	MarkCommentsRead(ctx context.Context, threadID string, commentIDs []string) error
}

// confirmWorkspaceName checks the caller named the workspace correctly. It reads
// the name off the bubbles the workspace contains, which is what list_bubbles
// already reports — so an agent that has looked can answer, and one that has not
// cannot guess.
func confirmWorkspaceName(ctx context.Context, b Backend, id, claimed string) error {
	bubbles, err := b.Bubbles(ctx)
	if err != nil {
		return err
	}
	for _, x := range bubbles {
		if strings.HasPrefix(x.ID, id+":") {
			return sameName(claimed, x.ProjectName, "workspace")
		}
	}
	return fmt.Errorf("no workspace %q, or it holds nothing you can see — read it before deleting it", id)
}

// confirmBubbleName resolves a bubble and checks the caller named it correctly.
func confirmBubbleName(ctx context.Context, b Backend, id, claimed string) error {
	bubbles, err := b.Bubbles(ctx)
	if err != nil {
		return err
	}
	for _, x := range bubbles {
		if x.ID == id || strings.HasSuffix(x.ID, ":"+id) {
			return sameName(claimed, x.Name, "bubble")
		}
	}
	return fmt.Errorf("no bubble %q — read it before deleting it", id)
}

// sameName compares a claimed name to the real one. Spacing and case are
// cosmetic; anything else means the caller is not looking at what it thinks.
func sameName(claimed, actual, kind string) error {
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	if strings.TrimSpace(claimed) == "" {
		return fmt.Errorf("refusing to delete a %s without naming it: pass name=%q to confirm", kind, actual)
	}
	if norm(claimed) != norm(actual) {
		return fmt.Errorf("that %s is called %q, not %q — read it again before deleting it", kind, actual, claimed)
	}
	return nil
}

// withActor lifts the MCP-verified identity (carried in req.Extra.TokenInfo by
// the bearer middleware) into the context, so the backend attributes MCP tool
// calls exactly like REST calls (§9.3).
func withActor(ctx context.Context, req *sdk.CallToolRequest) context.Context {
	if req == nil {
		return ctx
	}
	ti := req.Extra.TokenInfo
	if ti == nil || ti.Extra == nil {
		return ctx
	}
	if a, ok := ti.Extra["actor"].(domain.Actor); ok {
		ctx = domain.WithActor(ctx, a)
	}
	if cred, ok := ti.Extra["cred"].(string); ok {
		ctx = domain.WithCred(ctx, cred)
	}
	return ctx
}

// updateThreadIn patches a thread's artifact page. Both fields are optional and
// a nil one is left untouched — an agent revising a plan must not be able to
// erase the human's Brief.
type updateThreadIn struct {
	ThreadID string  `json:"thread_id" jsonschema:"the namespaced thread id"`
	Title    *string `json:"title,omitempty" jsonschema:"rename the thread. Not evidence of production — a title is what the work is called, not what has been done"`
	Logbook  *string `json:"logbook,omitempty" jsonschema:"replace the Logbook section: the plan in phases, its todos, current owner and state. Markdown"`
	Brief    *string `json:"brief,omitempty" jsonschema:"replace the Brief section: problem, intended outcome, constraints, links. Rarely what an agent should touch"`
	DoD      *string `json:"dod,omitempty" jsonschema:"replace the Definition of Done: the checklist that says the work is finished. Markdown"`
	// Edits are the PREFERRED way to change an existing section.
	Edits []editIn `json:"edits,omitempty" jsonschema:"change PART of a section instead of replacing it. Strongly preferred for an existing section: quote the exact text to change rather than reproducing the whole thing, which is how sections get paraphrased, truncated, or appended to twice"`
}

// editIn is one find-and-replace. Old is the guard, exactly as text guards the
// index in toggle_todo: quoting what is there is how the caller proves it is
// looking at the current version.
type editIn struct {
	Old    string `json:"old" jsonschema:"the exact text to replace, copied from read_thread. Must appear EXACTLY ONCE in the section — include surrounding words if it would otherwise be ambiguous. Leave empty to append instead"`
	New    string `json:"new" jsonschema:"what it becomes. Empty deletes the matched text"`
	Region string `json:"region,omitempty" jsonschema:"logbook (default), dod, or brief"`
	All    bool   `json:"all,omitempty" jsonschema:"replace every occurrence instead of refusing an ambiguous match"`
}

// toggleTodoIn ticks one checklist item. Text guards Index: an index alone is a
// question the caller cannot answer, because the list may have been re-ordered
// since it was read, and ticking the wrong box silently manufactures evidence.
type toggleTodoIn struct {
	ThreadID string `json:"thread_id" jsonschema:"the namespaced thread id"`
	Index    int    `json:"index" jsonschema:"zero-based position of the item within the section, as read_thread lists them"`
	Text     string `json:"text" jsonschema:"the item's text as you read it — the write is REFUSED if it no longer matches, rather than ticking the wrong box"`
	Done     bool   `json:"done" jsonschema:"true to tick, false to un-tick"`
	Region   string `json:"region,omitempty" jsonschema:"logbook (default) or dod"`
}

// deleteIn asks for the thing's id AND its name.
//
// The name is the guard, and it is the same one toggle_todo uses for a todo's
// text: an id alone is a question the caller cannot check its own answer to. It
// forces an agent to have READ the thing it is about to destroy, so a
// transposed or hallucinated id fails loudly instead of deleting a stranger's
// work. There is deliberately no "force" flag to route around it.
// createWorkspaceIn creates a WORKSPACE — the boundary for a body of work,
// which maps to a Plane project (AGENTS.md vocabulary; NOT a Plane "workspace").
// Bubbles live inside one, so this is the outermost thing an agent can make.
type createWorkspaceIn struct {
	Instance   string `json:"instance" jsonschema:"the instance slug, e.g. one of those list_bubbles reports"`
	Name       string `json:"name" jsonschema:"what this body of work is called"`
	Identifier string `json:"identifier,omitempty" jsonschema:"short key Plane prefixes work items with, e.g. KIOSK. Derived from the name when omitted"`
	NoCycles   bool   `json:"no_cycles,omitempty" jsonschema:"turn cycles off. On by default: the cycle is the pulse heat is measured against (§3.6)"`
	NoPages    bool   `json:"no_pages,omitempty" jsonschema:"turn Plane pages off. On by default"`
	Views      bool   `json:"views,omitempty" jsonschema:"turn Plane views on. Off by default"`
	Intake     bool   `json:"intake,omitempty" jsonschema:"turn Plane intake on. Off by default"`
}

// renameWorkspaceIn retitles a workspace.
type renameWorkspaceIn struct {
	ID   string `json:"id" jsonschema:"the workspace id as slug:project"`
	Name string `json:"name" jsonschema:"the new name"`
}

// deleteWorkspaceIn destroys a workspace and EVERYTHING in it. The name guard
// is the same one delete_bubble uses, and it matters far more here: this is the
// most destructive call on the surface.
type deleteWorkspaceIn struct {
	ID   string `json:"id" jsonschema:"the workspace id as slug:project"`
	Name string `json:"name" jsonschema:"the workspace's exact current name. The delete is REFUSED if it does not match — read it first"`
}

type deleteIn struct {
	ID   string `json:"id" jsonschema:"the namespaced id of the thing to delete"`
	Name string `json:"name" jsonschema:"the thing's exact current name, as you just read it. The delete is REFUSED if it does not match — read it first"`
}

type deleteRegionIn struct {
	ThreadID string `json:"thread_id" jsonschema:"the namespaced thread id"`
	Region   string `json:"region" jsonschema:"logbook, dod, or brief (the document)"`
}

// moveThreadIn re-homes a thread. No guard beyond authorization: a move loses
// nothing — the work item, its artifacts, its comments and its id all survive,
// and moving it back is the same call.
type moveThreadIn struct {
	ThreadID string `json:"thread_id" jsonschema:"the namespaced thread id"`
	BubbleID string `json:"bubble_id" jsonschema:"the bubble to move it into. Must be in the same workspace — Plane groups work items only within their own project"`
}

type addRevisionIn struct {
	ThreadID string `json:"thread_id" jsonschema:"the namespaced thread id"`
	Title    string `json:"title" jsonschema:"what this revision is, e.g. 'first pass' — a 'rev:' prefix is added if missing"`
	Body     string `json:"body" jsonschema:"the revision's content, in Markdown"`
}

type listOut struct {
	Bubbles []domain.BubbleView `json:"bubbles"`
}
type closeIn struct {
	BubbleID string `json:"bubble_id" jsonschema:"the bubble (Plane module) id to close"`
}
type contractIn struct {
	BubbleID string  `json:"bubble_id" jsonschema:"the namespaced bubble id"`
	Outcome  *string `json:"outcome,omitempty" jsonschema:"what done looks like"`
	Owner    *string `json:"owner,omitempty" jsonschema:"who is accountable now"`
	Closure  *string `json:"closure,omitempty" jsonschema:"the explicit close signal"`
}
type closeOut struct {
	OK bool `json:"ok"`
}
type timelineIn struct {
	BubbleID string `json:"bubble_id" jsonschema:"the bubble id (short or namespaced) whose thread history to read"`
}
type timelineOut struct {
	Threads []domain.ThreadNode `json:"threads"`
}
type threadIn struct {
	ThreadID string `json:"thread_id" jsonschema:"the thread id from thread_timeline, or a work-item id"`
}
type commentsOut struct {
	Comments []domain.Comment `json:"comments"`
}
type postCommentIn struct {
	ThreadID string `json:"thread_id" jsonschema:"the thread id from thread_timeline, or a work-item id"`
	Body     string `json:"body" jsonschema:"the comment text (Markdown); posted to Plane as you"`
}
type markReadIn struct {
	ThreadID   string   `json:"thread_id" jsonschema:"the thread id whose comments to mark read"`
	CommentIDs []string `json:"comment_ids" jsonschema:"ids of comments you've read (not your own)"`
}

// deletedOut reports what a delete cost: threads unbubbled, or revisions taken
// down with a thread.
type deletedOut struct {
	OK       bool `json:"ok"`
	Affected int  `json:"affected"`
}

type okOut struct {
	OK bool `json:"ok"`
}

// Handler builds the MCP server and returns a streamable-HTTP handler to mount.
func Handler(b Backend) http.Handler {
	srv := sdk.NewServer(&sdk.Implementation{Name: "bubble-work", Version: "0.1.0"}, nil)

	sdk.AddTool(srv,
		&sdk.Tool{Name: "list_bubbles", Description: "List bubbles sorted by temperature — the buoyancy view (§2.2, §5)."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, listOut, error) {
			vs, err := b.Bubbles(withActor(ctx, req))
			if err != nil {
				return nil, listOut{}, err
			}
			return nil, listOut{Bubbles: vs}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "create_workspace", Description: "Create a WORKSPACE — the boundary for a body of work, which maps to a Plane project. Note the vocabulary: this is NOT a Plane workspace, it is a project inside one (AGENTS.md). Bubbles live inside a workspace, so make one only when the work genuinely does not belong in any existing workspace; a new body of work is usually a new BUBBLE. Modules are enabled on it, because bubbles need them."},
		func(ctx context.Context, req *sdk.CallToolRequest, in createWorkspaceIn) (*sdk.CallToolResult, domain.Workspace, error) {
			w, err := b.CreateWorkspace(withActor(ctx, req), domain.CreateWorkspaceRequest{
				Instance: in.Instance, Name: in.Name, Identifier: in.Identifier,
				NoCycles: in.NoCycles, NoPages: in.NoPages, Views: in.Views, Intake: in.Intake,
			})
			if err != nil {
				return nil, domain.Workspace{}, err
			}
			return nil, w, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "create_bubble", Description: "Create a bubble (a Plane module) in a project. Optionally set its §4 outcome and owner. A bubble is the unit of attention; birth threads into it afterward."},
		func(ctx context.Context, req *sdk.CallToolRequest, in domain.CreateBubbleRequest) (*sdk.CallToolResult, domain.NewBubble, error) {
			nb, err := b.CreateBubble(withActor(ctx, req), in)
			if err != nil {
				return nil, domain.NewBubble{}, err
			}
			return nil, nb, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "birth_thread", Description: "Create a thread. REJECTED unless a Brief (with Definition of Done) and a Logbook exist (§3)."},
		func(ctx context.Context, req *sdk.CallToolRequest, in domain.BirthRequest) (*sdk.CallToolResult, domain.BirthResult, error) {
			res, err := b.BirthThread(withActor(ctx, req), in)
			if err != nil {
				return nil, domain.BirthResult{}, err
			}
			return nil, res, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "set_contract", Description: "Set a bubble's contract — outcome, owner, closure condition (§4). Omitted fields are unchanged."},
		func(ctx context.Context, req *sdk.CallToolRequest, in contractIn) (*sdk.CallToolResult, domain.Contract, error) {
			c, err := b.SetContract(withActor(ctx, req), in.BubbleID, domain.ContractInput{
				Outcome: in.Outcome, Owner: in.Owner, Closure: in.Closure,
			})
			if err != nil {
				return nil, domain.Contract{}, err
			}
			return nil, c, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "close_bubble", Description: "Close a bubble once its outcome is reached or abandoned (§4, §5.3)."},
		func(ctx context.Context, req *sdk.CallToolRequest, in closeIn) (*sdk.CallToolResult, closeOut, error) {
			if err := b.CloseBubble(withActor(ctx, req), in.BubbleID); err != nil {
				return nil, closeOut{}, err
			}
			return nil, closeOut{OK: true}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "thread_timeline", Description: "List a bubble's threads newest-first — how the bubble has progressed over time (INTERIOR Phase 10). Read this to understand a bubble before acting. Each thread carries its OWN derived buoyancy (level: in_progress|zzzz|rip|done, with the reason) plus its real Plane state — use it to find what has gone quiet."},
		func(ctx context.Context, req *sdk.CallToolRequest, in timelineIn) (*sdk.CallToolResult, timelineOut, error) {
			ts, err := b.Timeline(withActor(ctx, req), in.BubbleID)
			if err != nil {
				return nil, timelineOut{}, err
			}
			return nil, timelineOut{Threads: ts}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "read_thread", Description: "Read a thread's interior: work artifacts (Brief), the Logbook, its Definition of Done, and revisions, plus its derived buoyancy (level/reason) and its Plane state. REQUIRED before implementing a thread — the birth rule says confirm the outcome and DoD first (§3)."},
		func(ctx context.Context, req *sdk.CallToolRequest, in threadIn) (*sdk.CallToolResult, domain.ThreadDetail, error) {
			d, err := b.ThreadDetail(withActor(ctx, req), in.ThreadID)
			if err != nil {
				return nil, domain.ThreadDetail{}, err
			}
			return nil, d, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "thread_comments", Description: "Read a thread's discussion (comments), oldest-first (INTERIOR Phase 12)."},
		func(ctx context.Context, req *sdk.CallToolRequest, in threadIn) (*sdk.CallToolResult, commentsOut, error) {
			cs, err := b.ThreadComments(withActor(ctx, req), in.ThreadID)
			if err != nil {
				return nil, commentsOut{}, err
			}
			return nil, commentsOut{Comments: cs}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "post_comment", Description: "Post a comment to a thread's discussion, written to Plane as you. Comments are communication, not evidence — posting does NOT warm the bubble (§4)."},
		func(ctx context.Context, req *sdk.CallToolRequest, in postCommentIn) (*sdk.CallToolResult, domain.Comment, error) {
			cm, err := b.PostComment(withActor(ctx, req), in.ThreadID, in.Body)
			if err != nil {
				return nil, domain.Comment{}, err
			}
			return nil, cm, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "update_thread", Description: "Rewrite a thread's Logbook, Definition of Done, or (rarely) its Brief. This is how an agent records that the plan changed — re-phasing, noting a decision, adding a todo. A Logbook or DoD change is EVIDENCE of production (§5.1), so it warms the thread and its bubble; the board updates immediately. PREFER `edits` for anything that already exists: quote the exact text and say what it becomes, and everything else stays untouched. Passing a whole section replaces it, which is how sections get paraphrased, truncated, or appended to twice — only do that when writing one from scratch. Within a section only the blocks you actually changed are rewritten, so images, mentions and formatting elsewhere survive either way. To tick a single existing todo, prefer toggle_todo."},
		func(ctx context.Context, req *sdk.CallToolRequest, in updateThreadIn) (*sdk.CallToolResult, domain.ThreadDetail, error) {
			edits := make([]domain.RegionEdit, 0, len(in.Edits))
			for _, e := range in.Edits {
				edits = append(edits, domain.RegionEdit{
					Region: e.Region, Old: e.Old, New: e.New, All: e.All,
				})
			}
			d, err := b.UpdateThread(withActor(ctx, req), in.ThreadID, domain.ThreadEdit{
				Title: in.Title, Brief: in.Brief, Logbook: in.Logbook, DoD: in.DoD,
				Edits: edits,
			})
			if err != nil {
				return nil, domain.ThreadDetail{}, err
			}
			return nil, d, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "move_thread", Description: "Move a thread into a different bubble. It LEAVES every other bubble, so this is a move and not a copy. Nothing is lost — the Brief, the Logbook, the comments, the history and the id are untouched, and moving it back is the same call. Use it when a thread turns out to belong to a different body of work; it is not evidence of production and warms nothing."},
		func(ctx context.Context, req *sdk.CallToolRequest, in moveThreadIn) (*sdk.CallToolResult, domain.ThreadDetail, error) {
			d, err := b.MoveThread(withActor(ctx, req), in.ThreadID, in.BubbleID)
			if err != nil {
				return nil, domain.ThreadDetail{}, err
			}
			return nil, d, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "add_revision", Description: "Attach a revision artifact to a thread (a Plane sub-work-item): a findings write-up, a review pass, a deliverable. Landing one is evidence of production (§5.1) and warms the thread."},
		func(ctx context.Context, req *sdk.CallToolRequest, in addRevisionIn) (*sdk.CallToolResult, domain.ThreadDetail, error) {
			d, err := b.AddRevision(withActor(ctx, req), in.ThreadID, in.Title, in.Body)
			if err != nil {
				return nil, domain.ThreadDetail{}, err
			}
			return nil, d, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "toggle_todo", Description: "Tick or un-tick one checklist item in a thread's Logbook or Definition of Done. A completed todo is EVIDENCE of production (§5.1): it warms the thread and its bubble. Pass the item's text as you read it — if it no longer matches that position the write is refused rather than ticking the wrong box."},
		func(ctx context.Context, req *sdk.CallToolRequest, in toggleTodoIn) (*sdk.CallToolResult, domain.ThreadDetail, error) {
			region := md.Region(in.Region)
			if region == "" {
				region = md.RegionLogbook
			}
			d, err := b.ToggleTodo(withActor(ctx, req), in.ThreadID, region, in.Index, in.Text, in.Done)
			if err != nil {
				return nil, domain.ThreadDetail{}, err
			}
			return nil, d, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "rename_workspace", Description: "Rename a workspace. Not evidence of production — what a body of work is called is not what has been done."},
		func(ctx context.Context, req *sdk.CallToolRequest, in renameWorkspaceIn) (*sdk.CallToolResult, domain.Workspace, error) {
			w, err := b.RenameWorkspace(withActor(ctx, req), in.ID, in.Name)
			if err != nil {
				return nil, domain.Workspace{}, err
			}
			return nil, w, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "delete_workspace", Description: "PERMANENTLY delete a workspace and EVERY bubble, thread, artifact and comment inside it. This is the most destructive call available and nothing about it is recoverable. Pass the workspace's exact name to confirm — the delete is refused if it does not match. Ask the person first, and be sure they mean the whole workspace rather than one bubble."},
		func(ctx context.Context, req *sdk.CallToolRequest, in deleteWorkspaceIn) (*sdk.CallToolResult, deletedOut, error) {
			actx := withActor(ctx, req)
			if err := confirmWorkspaceName(actx, b, in.ID, in.Name); err != nil {
				return nil, deletedOut{}, err
			}
			bubbles, threads, err := b.DeleteWorkspace(actx, in.ID)
			if err != nil {
				return nil, deletedOut{}, err
			}
			return nil, deletedOut{OK: true, Affected: bubbles + threads}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "delete_bubble", Description: "PERMANENTLY delete a bubble from Plane. IRREVERSIBLE. Its threads survive but end up in no bubble, which also removes them from the board. §5.3 says a bubble should normally die by being CLOSED — that keeps the record of what was done — Pass the bubble's exact name to confirm — the delete is refused if it does not match, so read it first. Prefer close_bubble unless this bubble should never have existed."},
		func(ctx context.Context, req *sdk.CallToolRequest, in deleteIn) (*sdk.CallToolResult, deletedOut, error) {
			actx := withActor(ctx, req)
			if err := confirmBubbleName(actx, b, in.ID, in.Name); err != nil {
				return nil, deletedOut{}, err
			}
			n, err := b.DeleteBubble(actx, in.ID)
			if err != nil {
				return nil, deletedOut{}, err
			}
			return nil, deletedOut{OK: true, Affected: n}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "delete_thread", Description: "PERMANENTLY delete a thread — or a revision, which is also a work item — from Plane, taking its Brief, Logbook, comments and revisions with it. IRREVERSIBLE, and it erases evidence of work that actually happened. Pass the thread's exact title to confirm — the delete is refused if it does not match, so read it first."},
		func(ctx context.Context, req *sdk.CallToolRequest, in deleteIn) (*sdk.CallToolResult, deletedOut, error) {
			actx := withActor(ctx, req)
			d, err := b.ThreadDetail(actx, in.ID)
			if err != nil {
				return nil, deletedOut{}, err
			}
			if err := sameName(in.Name, d.Title, "thread"); err != nil {
				return nil, deletedOut{}, err
			}
			n, err := b.DeleteThread(actx, in.ID)
			if err != nil {
				return nil, deletedOut{}, err
			}
			return nil, deletedOut{OK: true, Affected: n}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "delete_artifact", Description: "Remove one artifact from a thread's page — its Logbook, its Definition of Done, or its document — heading and all. The thread itself survives. Everything you do not name keeps its exact bytes, so images and mentions elsewhere on the page are untouched."},
		func(ctx context.Context, req *sdk.CallToolRequest, in deleteRegionIn) (*sdk.CallToolResult, domain.ThreadDetail, error) {
			region := md.Region(in.Region)
			if region == "brief" {
				region = md.RegionDocument
			}
			d, err := b.DeleteRegion(withActor(ctx, req), in.ThreadID, region)
			if err != nil {
				return nil, domain.ThreadDetail{}, err
			}
			return nil, d, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "mark_comments_read", Description: "Mark thread comments as read by you (👀 read-receipt). Pass comments you didn't author; your own are never marked."},
		func(ctx context.Context, req *sdk.CallToolRequest, in markReadIn) (*sdk.CallToolResult, okOut, error) {
			if err := b.MarkCommentsRead(withActor(ctx, req), in.ThreadID, in.CommentIDs); err != nil {
				return nil, okOut{}, err
			}
			return nil, okOut{OK: true}, nil
		})

	// The SDK auto-enables DNS-rebinding protection: a request arriving over
	// loopback whose Host header is NOT loopback gets a 403. That is the right
	// default for an MCP server a browser could reach directly, and it is wrong
	// behind a reverse proxy — colima forwards ports over an SSH tunnel, so the
	// request lands on 127.0.0.1 with the real public Host and is refused.
	//
	// It is turned off here and replaced by trustedHost, which is an explicit
	// ALLOWLIST rather than an absence of checking. The SDK only offers the
	// boolean, so the list has to live on our side.
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return srv },
		&sdk.StreamableHTTPOptions{DisableLocalhostProtection: true},
	)
}
