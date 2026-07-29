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

	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, nil)
}
