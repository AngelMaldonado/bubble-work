// Package mcpapi is the agent surface.
//
// MCP is the primary protocol for the operative layer: the people doing the work
// drive it through agents, so this is the door that matters most. It is deliberately
// THIN — every tool calls the same function the REST route calls, so the two
// surfaces cannot drift and neither can bypass a rule the other enforces.
package mcpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/bubble"
	"github.com/AngelMaldonado/bubble-work/internal/tree"
	"github.com/AngelMaldonado/bubble-work/prompts"
)

// caller is who is asking, carried through the SDK's context because a tool
// handler never sees the HTTP request.
type caller struct {
	app  core.App
	auth *core.Record
	tree *tree.Tree
}

type ctxKey struct{}

func from(ctx context.Context) (*caller, error) {
	c, ok := ctx.Value(ctxKey{}).(*caller)
	if !ok || c.auth == nil {
		return nil, fmt.Errorf("not authenticated")
	}
	return c, nil
}

// Register mounts the MCP endpoint.
//
// Authentication is PocketBase's own: an agent presents a person's token and
// therefore IS that person for every rule in the system. There is no agent
// identity to manage, and nothing an agent can reach that its person cannot.
func Register(app core.App, t *tree.Tree) {
	srv := build()

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{
			// Stateless: PocketBase may run behind anything, and a session held in
			// one process is a session the next request may not find.
			Stateless: true,
			// The SDK refuses a loopback request whose Host header is not loopback —
			// correct DNS-rebinding protection, and wrong behind a reverse proxy,
			// where every legitimate request looks exactly like that. v0 met this as
			// "Forbidden: invalid Host header" with the web UI working fine and only
			// agents locked out. Deployment decides the boundary here, not the SDK.
			DisableLocalhostProtection: true,
		})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		mount := func(e *core.RequestEvent) error {
			ctx := context.WithValue(e.Request.Context(), ctxKey{},
				&caller{app: e.App, auth: e.Auth, tree: t})
			handler.ServeHTTP(e.Response, e.Request.WithContext(ctx))
			return nil
		}
		for _, m := range []string{"GET", "POST", "DELETE"} {
			se.Router.Route(m, "/mcp", mount).Bind(apis.RequireAuth())
		}
		return se.Next()
	})
}

func build() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "bubble-work",
		Version: "2",
	}, nil)

	// The guide is both a PROMPT and a TOOL, from the same bytes. Not every client
	// lists prompts, and an agent that cannot list them would simply never see it.
	s.AddPrompt(&mcp.Prompt{
		Name:        "bubble-work",
		Description: "How this server works: what warms, where files live, how a write works.",
	}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Bubble Work",
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: prompts.Guide},
			}},
		}, nil
	})

	type none struct{}
	mcp.AddTool(s, &mcp.Tool{
		Name: "guide",
		Description: "Read this first. How Bubble Work works: what warms and what does not, " +
			"where files live, and the exact write protocol.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		return text(prompts.Guide), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "workspaces",
		Description: "List the workspaces you can see.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		rows, err := c.app.FindAllRecords("workspaces")
		if err != nil {
			return nil, nil, err
		}
		var out []map[string]any
		for _, w := range rows {
			if _, _, err := bubble.WorkspaceFor(c.app, c.auth, w.Id); err != nil {
				continue
			}
			out = append(out, map[string]any{
				"id": w.Id, "slug": w.GetString("slug"), "name": w.GetString("name"),
			})
		}
		return jsonOut(out), nil, nil
	})

	type wsArg struct {
		Workspace string `json:"workspace" jsonschema:"the workspace id or slug"`
	}

	mcp.AddTool(s, &mcp.Tool{
		Name: "board",
		Description: "What floats and why: bubbles with their band, threads with heat, " +
			"buoyancy score and priority. Nothing here is stored — it is computed now.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in wsArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		b, err := bubble.BoardFor(c.app, ws)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(b), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "tree",
		Description: "The workspace's files: threads/, docs/ and README.md.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in wsArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		_, repo, err := bubble.WorkspaceFor(c.app, c.auth, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		entries, err := c.tree.Tree(repo)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(entries), nil, nil
	})

	type searchArg struct {
		Workspace string `json:"workspace"`
		Query     string `json:"query" jsonschema:"text to look for, case-insensitive"`
		Limit     int    `json:"limit,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "Find text inside a workspace's documents. Threads come first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		_, repo, err := bubble.WorkspaceFor(c.app, c.auth, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		hits, err := c.tree.Search(repo, in.Query, in.Limit)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(hits), nil, nil
	})

	type readArg struct {
		Workspace string `json:"workspace,omitempty" jsonschema:"required with path"`
		Path      string `json:"path,omitempty" jsonschema:"a path like docs/onboarding.md"`
		Thread    string `json:"thread,omitempty" jsonschema:"a thread id, instead of workspace+path"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "read",
		Description: "Read a document, by thread id or by workspace and path. " +
			"KEEP THE HASH: every write must send it back as `base`.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in readArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, repo, path, _, err := locate(c, in.Thread, in.Workspace, in.Path)
		if err != nil {
			return nil, nil, err
		}
		doc, err := bubble.ReadDoc(c.app, c.tree, ws, repo, path)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(doc), nil, nil
	})

	type editArg struct {
		Workspace string        `json:"workspace,omitempty"`
		Path      string        `json:"path,omitempty"`
		Thread    string        `json:"thread,omitempty"`
		Base      string        `json:"base" jsonschema:"the hash you got from read — required"`
		Message   string        `json:"message,omitempty" jsonschema:"the commit message"`
		Content   *string       `json:"content,omitempty" jsonschema:"replace the whole file"`
		Edits     []bubble.Edit `json:"edits,omitempty" jsonschema:"surgical changes: quote old, give new"`
		Todo      *bubble.Todo  `json:"todo,omitempty" jsonschema:"tick or untick a checkbox by index"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "edit",
		Description: "Change a document. Send exactly ONE of content, edits or todo, " +
			"plus the `base` hash you read. On a conflict, read again and re-apply — " +
			"never retry with the same base. Prefer `edits`: replacing a whole file to " +
			"change one line destroys paragraphs you never read.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in editArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, repo, path, subject, err := locate(c, in.Thread, in.Workspace, in.Path)
		if err != nil {
			return nil, nil, err
		}
		doc, err := bubble.Apply(c.app, c.auth, c.tree, ws, repo, path, subject, bubble.Patch{
			Base: in.Base, Message: in.Message,
			Content: in.Content, Edits: in.Edits, Todo: in.Todo,
		})
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(doc), nil, nil
	})

	type newThreadArg struct {
		Workspace string `json:"workspace"`
		Name      string `json:"name"`
		Bubble    string `json:"bubble,omitempty"`
		Impact    string `json:"impact,omitempty" jsonschema:"high, mid or low"`
		Urgency   string `json:"urgency,omitempty" jsonschema:"high, mid or low"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_thread",
		Description: "Create a thread. It needs a name; its document is written " +
			"separately with `edit`, in whatever shape the work has.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in newThreadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		th, err := bubble.CreateThread(c.app, c.auth, ws, in.Name, in.Bubble, in.Impact, in.Urgency)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": th.Id, "seq": th.GetInt("seq"), "name": th.GetString("name"),
			"doc_path": th.GetString("doc_path"),
		}), nil, nil
	})

	type linkArg struct {
		Thread string `json:"thread"`
		URL    string `json:"url"`
		Title  string `json:"title,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "link",
		Description: "Attach external evidence to a thread — a commit, a PR, something " +
			"published. Landing one is production: it warms the bubble.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in linkArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		rec, err := bubble.AddLink(c.app, c.auth, in.Thread, in.URL, in.Title)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": rec.Id, "url": rec.GetString("url")}), nil, nil
	})

	type completeArg struct {
		Thread string `json:"thread"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "complete_thread",
		Description: "Move a thread to the workspace's completed state. Finishing is a " +
			"position somebody takes, not a checklist the server grades.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in completeArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		th, err := bubble.CompleteThread(c.app, c.auth, in.Thread)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": th.Id, "state": th.GetString("state")}), nil, nil
	})

	return s
}

// locate resolves either addressing form to one document. Both are accepted
// because a thread id never goes stale and a path is what an agent already
// thinks in.
func locate(c *caller, threadID, workspace, path string) (ws *core.Record, repo, doc, subject string, err error) {
	if threadID != "" {
		th, w, r, e := bubble.ThreadFor(c.app, c.auth, threadID)
		if e != nil {
			return nil, "", "", "", e
		}
		return w, r, th.GetString("doc_path"), th.GetString("name"), nil
	}
	if workspace == "" || path == "" {
		return nil, "", "", "", fmt.Errorf("give a thread, or a workspace and a path")
	}
	w, r, e := bubble.WorkspaceFor(c.app, c.auth, workspace)
	if e != nil {
		return nil, "", "", "", e
	}
	return w, r, path, path, nil
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// jsonOut renders a result as pretty JSON text.
//
// Text rather than a typed output schema: an agent reads these, and a shape that
// changes with a field addition is a shape that breaks a client. The description
// on each tool is where the meaning lives.
func jsonOut(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return text(fmt.Sprintf("could not render the answer: %v", err))
	}
	return text(string(b))
}
