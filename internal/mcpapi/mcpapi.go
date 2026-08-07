// Package mcpapi exposes the Bubble Work server as an MCP server that LLM
// clients (Claude Code, Codex) consume as first-class members (§9.5). This is
// OUR MCP — the framework's own front door, not Plane's MCP.
package mcpapi

import (
	"context"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// Backend is the server capability set the MCP tools call into. Keeping it an
// interface avoids an import cycle and mirrors the HTTP surface exactly.
//
// DELETION IS DELIBERATELY ABSENT. Surface parity (AGENTS.md) exists so a
// capability does not drift between the REST, CLI and MCP surfaces — not so
// that every capability is equally reachable from each of them. The web and the
// CLI both put a person in the loop before a delete: a dialog that says what
// goes, or a typed confirmation. MCP has no such step, so an agent calling
// delete_thread would destroy a Brief, a Logbook, its comments and its
// revisions with nothing but a tool description asking it to check first —
// which is not a guard. Agents keep every read and every CONSTRUCTIVE write;
// destruction stays with a human. Deleting lives on REST + CLI + web.
type Backend interface {
	Bubbles(ctx context.Context) ([]domain.BubbleView, error)
	CreateBubble(ctx context.Context, req domain.CreateBubbleRequest) (domain.NewBubble, error)
	BirthThread(ctx context.Context, req domain.BirthRequest) (domain.BirthResult, error)
	SetContract(ctx context.Context, bubbleID string, in domain.ContractInput) (domain.Contract, error)
	CloseBubble(ctx context.Context, bubbleID string) error
	Timeline(ctx context.Context, bubbleID string) ([]domain.ThreadNode, error)
	ThreadDetail(ctx context.Context, threadID string) (domain.ThreadDetail, error)
	ThreadComments(ctx context.Context, threadID string) ([]domain.Comment, error)
	PostComment(ctx context.Context, threadID, body string) (domain.Comment, error)
	UpdateThread(ctx context.Context, threadID string, edit domain.ThreadEdit) (domain.ThreadDetail, error)
	ToggleTodo(ctx context.Context, threadID string, region md.Region, index int, text string, done bool) (domain.ThreadDetail, error)
	AddRevision(ctx context.Context, threadID, title, body string) (domain.ThreadDetail, error)
	MarkCommentsRead(ctx context.Context, threadID string, commentIDs []string) error
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
		&sdk.Tool{Name: "update_thread", Description: "Rewrite a thread's Logbook, Definition of Done, or (rarely) its Brief. This is how an agent records that the plan changed — re-phasing, noting a decision, adding a todo. A Logbook or DoD change is EVIDENCE of production (§5.1), so it warms the thread and its bubble; the board updates immediately. Only the sections you pass are touched, and within a section only the blocks you actually changed are rewritten — so images, mentions and formatting elsewhere on the page survive. To tick a single existing todo, prefer toggle_todo."},
		func(ctx context.Context, req *sdk.CallToolRequest, in updateThreadIn) (*sdk.CallToolResult, domain.ThreadDetail, error) {
			d, err := b.UpdateThread(withActor(ctx, req), in.ThreadID, domain.ThreadEdit{
				Title: in.Title, Brief: in.Brief, Logbook: in.Logbook, DoD: in.DoD,
			})
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
		&sdk.Tool{Name: "mark_comments_read", Description: "Mark thread comments as read by you (👀 read-receipt). Pass comments you didn't author; your own are never marked."},
		func(ctx context.Context, req *sdk.CallToolRequest, in markReadIn) (*sdk.CallToolResult, okOut, error) {
			if err := b.MarkCommentsRead(withActor(ctx, req), in.ThreadID, in.CommentIDs); err != nil {
				return nil, okOut{}, err
			}
			return nil, okOut{OK: true}, nil
		})

	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, nil)
}
