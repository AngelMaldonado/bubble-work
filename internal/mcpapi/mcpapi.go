// Package mcpapi exposes the Bubble Work server as an MCP server that LLM
// clients (Claude Code, Codex) consume as first-class members (§9.5). This is
// OUR MCP — the framework's own front door, not Plane's MCP.
package mcpapi

import (
	"context"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

// Backend is the server capability set the MCP tools call into. Keeping it an
// interface avoids an import cycle and mirrors the HTTP surface exactly.
type Backend interface {
	Bubbles(ctx context.Context) ([]domain.BubbleView, error)
	BirthThread(ctx context.Context, req domain.BirthRequest) (domain.BirthResult, error)
	SetContract(ctx context.Context, bubbleID string, in domain.ContractInput) (domain.Contract, error)
	CloseBubble(ctx context.Context, bubbleID string) error
	Timeline(ctx context.Context, bubbleID string) ([]domain.ThreadNode, error)
	ThreadDetail(ctx context.Context, threadID string) (domain.ThreadDetail, error)
	ThreadComments(ctx context.Context, threadID string) ([]domain.Comment, error)
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
		&sdk.Tool{Name: "thread_timeline", Description: "List a bubble's threads newest-first — how the bubble has progressed over time (INTERIOR Phase 10). Read this to understand a bubble before acting."},
		func(ctx context.Context, req *sdk.CallToolRequest, in timelineIn) (*sdk.CallToolResult, timelineOut, error) {
			ts, err := b.Timeline(withActor(ctx, req), in.BubbleID)
			if err != nil {
				return nil, timelineOut{}, err
			}
			return nil, timelineOut{Threads: ts}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "read_thread", Description: "Read a thread's interior: work artifacts (Brief), the Logbook, its Definition of Done, and revisions. REQUIRED before implementing a thread — the birth rule says confirm the outcome and DoD first (§3)."},
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

	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, nil)
}
