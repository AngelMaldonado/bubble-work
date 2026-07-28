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
	CloseBubble(ctx context.Context, bubbleID string) error
}

type listOut struct {
	Bubbles []domain.BubbleView `json:"bubbles"`
}
type closeIn struct {
	BubbleID string `json:"bubble_id" jsonschema:"the bubble (Plane module) id to close"`
}
type closeOut struct {
	OK bool `json:"ok"`
}

// Handler builds the MCP server and returns a streamable-HTTP handler to mount.
func Handler(b Backend) http.Handler {
	srv := sdk.NewServer(&sdk.Implementation{Name: "bubble-work", Version: "0.1.0"}, nil)

	sdk.AddTool(srv,
		&sdk.Tool{Name: "list_bubbles", Description: "List bubbles sorted by temperature — the buoyancy view (§2.2, §5)."},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, listOut, error) {
			vs, err := b.Bubbles(ctx)
			if err != nil {
				return nil, listOut{}, err
			}
			return nil, listOut{Bubbles: vs}, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "birth_thread", Description: "Create a thread. REJECTED unless a Brief (with Definition of Done) and a Logbook exist (§3)."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in domain.BirthRequest) (*sdk.CallToolResult, domain.BirthResult, error) {
			res, err := b.BirthThread(ctx, in)
			if err != nil {
				return nil, domain.BirthResult{}, err
			}
			return nil, res, nil
		})

	sdk.AddTool(srv,
		&sdk.Tool{Name: "close_bubble", Description: "Close a bubble once its outcome is reached or abandoned (§4, §5.3)."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in closeIn) (*sdk.CallToolResult, closeOut, error) {
			if err := b.CloseBubble(ctx, in.BubbleID); err != nil {
				return nil, closeOut{}, err
			}
			return nil, closeOut{OK: true}, nil
		})

	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, nil)
}
