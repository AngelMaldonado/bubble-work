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

	// ---- the bubble, the plan, and what already happened -------------------
	//
	// The first ten tools let an agent write a document and nothing else: it
	// could not say what the work is FOR, when it is due, who is accountable
	// for the bubble around it, or read what had already been done to it. So it
	// worked blind and asked a person for everything that was not text.

	type newWorkspaceArg struct {
		Name string `json:"name"`
		Slug string `json:"slug,omitempty" jsonschema:"its address: letters, digits and dashes. Derived from the name if you leave it out"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_workspace",
		Description: "Found a workspace — the boundary for a body of work, with its own " +
			"git repository. You become its lead, and it starts with a workflow so a " +
			"thread has somewhere to be. The slug is its address on disk and in every " +
			"link, and it never changes afterwards.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in newWorkspaceArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, err := bubble.CreateWorkspace(c.app, c.auth, in.Name, in.Slug)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": ws.Id, "name": ws.GetString("name"), "slug": ws.GetString("slug"),
		}), nil, nil
	})

	type newBubbleArg struct {
		Workspace string `json:"workspace"`
		Name      string `json:"name"`
		Outcome   string `json:"outcome,omitempty" jsonschema:"what is TRUE when this is done"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_bubble",
		Description: "Create a bubble — a durable grouping of related work, and the " +
			"unit of attention. Give it an outcome: a bubble without one is a folder " +
			"with a nice name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in newBubbleArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, _, err := bubble.WorkspaceFor(c.app, c.auth, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		b, err := bubble.CreateBubble(c.app, c.auth, ws, in.Name, in.Outcome)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": b.Id, "name": b.GetString("name")}), nil, nil
	})

	// Flat, not embedded: the schema this SDK infers does not flatten an
	// anonymous struct, so half the fields never reached the tool — they were
	// accepted, ignored, and answered with an unchanged record. One shape here,
	// mapped explicitly below.
	type setBubbleArg struct {
		Bubble  string    `json:"bubble" jsonschema:"the bubble id"`
		Name    *string   `json:"name,omitempty"`
		Outcome *string   `json:"outcome,omitempty" jsonschema:"what is TRUE when this is done"`
		Owners  *[]string `json:"owners,omitempty" jsonschema:"user ids; empty means nobody is accountable"`
		Closure *string   `json:"closure,omitempty" jsonschema:"how it ended, in your words"`
		Closed  *bool     `json:"closed,omitempty" jsonschema:"true closes it, false reopens it"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "set_bubble",
		Description: "Say something about a bubble: its outcome, who is accountable, " +
			"or that it is closed. Closing is a decision with a date, not a delete — " +
			"say how it ended in `closure`, and `closed: false` reopens it. Nobody " +
			"accountable is a real answer, and it has a cost: a quiet bubble with no " +
			"owner is a grave, not a nap.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setBubbleArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		b, err := bubble.SetBubble(c.app, c.auth, in.Bubble, bubble.BubbleEdit{
			Name: in.Name, Outcome: in.Outcome, Owners: in.Owners,
			Closure: in.Closure, Closed: in.Closed,
		})
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": b.Id, "name": b.GetString("name"), "outcome": b.GetString("outcome"),
			"owners": b.GetStringSlice("owners"),
			"closed": !b.GetDateTime("closed_at").IsZero(),
		}), nil, nil
	})

	type setThreadArg struct {
		Thread    string  `json:"thread"`
		Name      *string `json:"name,omitempty" jsonschema:"renaming moves its file, and git follows"`
		Bubble    *string `json:"bubble,omitempty" jsonschema:"a bubble id in the same workspace; empty takes it out"`
		Objective *string `json:"objective,omitempty" jsonschema:"what this work is FOR; empty unfiles it"`
		Due       *string `json:"due,omitempty" jsonschema:"a day, 2026-09-15; empty clears it"`
		Impact    *string `json:"impact,omitempty" jsonschema:"high, mid or low"`
		Urgency   *string `json:"urgency,omitempty" jsonschema:"high, mid or low"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "set_thread",
		Description: "Everything about a thread that is not its document: its name, " +
			"which bubble carries it, which objective it is FOR, when it is due, and " +
			"its impact and urgency. Priority is NOT here — the server derives it from " +
			"impact × urgency, and a second way to write it would be a second answer.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setThreadArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		th, err := bubble.SetThread(c.app, c.auth, in.Thread, bubble.ThreadEdit{
			Name: in.Name, Bubble: in.Bubble, Objective: in.Objective,
			Due: in.Due, Impact: in.Impact, Urgency: in.Urgency,
		})
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": th.Id, "seq": th.GetInt("seq"), "name": th.GetString("name"),
			"doc_path": th.GetString("doc_path"), "bubble": th.GetString("bubble"),
			"objective": th.GetString("objective"),
			"due_date":  th.GetDateTime("due_date").String(),
		}), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "plan",
		Description: "The department's strategic layer: its objectives — what the work " +
			"is FOR — and the inbox, what has been captured and not yet decided about. " +
			"The objectives are the global lead's to read; a note is read by whoever " +
			"captured it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		p, err := bubble.ReadPlan(c.app, c.auth)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(p), nil, nil
	})

	type captureArg struct {
		Note string `json:"note"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "capture",
		Description: "Put a note in the department's inbox — something raised and not " +
			"yet work. It has no outcome and no evidence, and making it a thread on the " +
			"way in is how a backlog fills with rows nobody committed to. Capturing " +
			"warms nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in captureArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		n, err := bubble.Capture(c.app, c.auth, in.Note)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": n.Id, "note": n.GetString("note")}), nil, nil
	})

	type objectiveArg struct {
		Objective string `json:"objective,omitempty" jsonschema:"an id to edit; leave it out to create"`
		Name      string `json:"name,omitempty"`
		Outcome   string `json:"outcome,omitempty" jsonschema:"what is TRUE when this is met"`
		Due       string `json:"due_date,omitempty" jsonschema:"a day: 2026-12-31"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "set_objective",
		Description: "Create or edit an objective. It belongs to the DEPARTMENT, not to " +
			"a workspace: threads from any project hang from the same one, which is what " +
			"makes it worth stating. Only the global lead shapes this layer.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in objectiveArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		o, err := bubble.SetObjective(c.app, c.auth, in.Objective, in.Name, in.Outcome, in.Due)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{
			"id": o.Id, "name": o.GetString("name"), "outcome": o.GetString("outcome"),
		}), nil, nil
	})

	type timelineArg struct {
		Thread string `json:"thread"`
		Limit  int    `json:"limit,omitempty" jsonschema:"how many, newest first (default 50)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "timeline",
		Description: "What has already happened to a thread, newest first. Heat says a " +
			"thread is warm; this says WHAT made it warm and when — read it before you " +
			"start, or you will do something that was done on Tuesday.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in timelineArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Timeline(c.app, c.auth, in.Thread, in.Limit)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type talkArg struct {
		Thread string `json:"thread"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "comments",
		Description: "What has been said on a thread, oldest first. Read it before you " +
			"answer a question that was already answered.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in talkArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := bubble.Comments(c.app, c.auth, in.Thread)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(out), nil, nil
	})

	type sayArg struct {
		Thread string `json:"thread"`
		Body   string `json:"body" jsonschema:"markdown"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "comment",
		Description: "Say something on a thread. A comment is PULSE, never heat: it keeps " +
			"the thread out of the grave and does NOT wake it. That is why you can say " +
			"what you found, or that you found nothing, without pretending you produced " +
			"evidence — the writing and the links are what warm anything.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sayArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		r, err := bubble.Comment(c.app, c.auth, in.Thread, in.Body)
		if err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"id": r.Id}), nil, nil
	})

	type dropPageArg struct {
		Workspace string `json:"workspace"`
		Path      string `json:"path" jsonschema:"a page under docs/, or a thread's other file"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "delete_page",
		Description: "Remove a document. A thread's OWN document is not removed this " +
			"way — it goes when the thread does, or the row would point at nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in dropPageArg) (*mcp.CallToolResult, any, error) {
		c, err := from(ctx)
		if err != nil {
			return nil, nil, err
		}
		ws, repo, err := bubble.WorkspaceFor(c.app, c.auth, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		if err := bubble.RemovePath(c.app, c.auth, c.tree, ws, repo, in.Path); err != nil {
			return nil, nil, err
		}
		return jsonOut(map[string]any{"path": in.Path, "deleted": true}), nil, nil
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
