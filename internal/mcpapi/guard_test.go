package mcpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// The name is the guard on an irreversible act, and it is the same shape
// toggle_todo uses for a todo's text: an id alone is a question the caller
// cannot check its own answer to.
func TestSameNameGuardsADelete(t *testing.T) {
	// Spacing and case are cosmetic — an agent that re-typed the name from a
	// rendered page should not be punished for it.
	for _, claimed := range []string{"Kiosko", "kiosko", "  Kiosko  ", "Kiosko"} {
		if err := sameName(claimed, "Kiosko", "bubble"); err != nil {
			t.Errorf("a matching name was refused (%q): %v", claimed, err)
		}
	}

	// Naming something ELSE is exactly the failure this exists to catch: a
	// transposed or hallucinated id pointing at a stranger's work.
	err := sameName("Kiosko", "Login y Perfil", "bubble")
	if err == nil {
		t.Fatal("deleting a bubble the caller misnamed was allowed")
	}
	// The message has to say what it actually is, or the agent cannot recover.
	if got := err.Error(); !contains(got, "Login y Perfil") || !contains(got, "Kiosko") {
		t.Errorf("the refusal does not say what was expected vs found: %s", got)
	}

	// Omitting it is not a shortcut.
	if err := sameName("", "Kiosko", "thread"); err == nil {
		t.Error("deleting without naming anything was allowed")
	} else if !contains(err.Error(), "Kiosko") {
		t.Errorf("the refusal should tell the caller what to pass: %s", err)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// One malformed tool takes down ALL of them.
//
// list_workspaces returned a bare []Workspace, so its generated output schema
// was `"type": "array"`. Clients validate the whole tools/list response, so they
// rejected every tool in it — the entire MCP surface went dark, not just the new
// one. This lists the tools the way a client does and checks the shape.
func TestEveryToolAnnouncesAnObjectSchema(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	srv := newServer(stubBackend{})
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer ss.Close()

	cl := mcp.NewClient(&mcp.Implementation{Name: "guard", Version: "0"}, nil)
	cs, err := cl.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("no tools registered")
	}
	for _, tool := range res.Tools {
		if got := schemaType(t, tool.InputSchema); got != "" && got != "object" {
			t.Errorf("%s: input schema is %q, must be object", tool.Name, got)
		}
		// A tool may legitimately return text only, hence the empty case.
		if got := schemaType(t, tool.OutputSchema); got != "" && got != "object" {
			t.Errorf("%s: output schema is %q, must be object — wrap the value in a struct",
				tool.Name, got)
		}
	}
}

// schemaType reads a schema's declared type. Schemas come off the wire as raw
// JSON, which is the form the client validates, so this checks what the client
// actually saw rather than what we meant.
func schemaType(t *testing.T, schema any) string {
	t.Helper()
	if schema == nil {
		return ""
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	// "type" is a string, but JSON Schema also allows a list of them — and a
	// generated schema for a nullable slice is exactly ["array","null"]. Decode
	// loosely so a violation is REPORTED rather than crashing the decoder, which
	// would hide which tool is at fault.
	var s struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	switch v := s.Type.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// stubBackend satisfies Backend so the tools can be registered. Nothing calls
// through it here — the point is the SCHEMAS, which are built at registration.
type stubBackend struct{}

func (stubBackend) Bubbles(context.Context) ([]domain.BubbleView, error) { return nil, nil }
func (stubBackend) CreateWorkspace(context.Context, domain.CreateWorkspaceRequest) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}
func (stubBackend) Workspaces(context.Context) ([]domain.Workspace, error) { return nil, nil }
func (stubBackend) Pages(context.Context, string) ([]domain.Page, error)   { return nil, nil }
func (stubBackend) Page(context.Context, string) (domain.PageDetail, error) {
	return domain.PageDetail{}, nil
}
func (stubBackend) CreatePage(context.Context, domain.CreatePageRequest) (domain.PageDetail, error) {
	return domain.PageDetail{}, nil
}
func (stubBackend) UpdatePage(context.Context, string, domain.PageEdit) (domain.PageDetail, error) {
	return domain.PageDetail{}, nil
}
func (stubBackend) DeletePage(context.Context, string) error { return nil }
func (stubBackend) RenameWorkspace(context.Context, string, string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}
func (stubBackend) DeleteWorkspace(context.Context, string) (int, int, error) { return 0, 0, nil }
func (stubBackend) CreateBubble(context.Context, domain.CreateBubbleRequest) (domain.NewBubble, error) {
	return domain.NewBubble{}, nil
}
func (stubBackend) BirthThread(context.Context, domain.BirthRequest) (domain.BirthResult, error) {
	return domain.BirthResult{}, nil
}
func (stubBackend) SetContract(context.Context, string, domain.ContractInput) (domain.Contract, error) {
	return domain.Contract{}, nil
}
func (stubBackend) CloseBubble(context.Context, string) error { return nil }
func (stubBackend) Timeline(context.Context, string) ([]domain.ThreadNode, error) {
	return nil, nil
}
func (stubBackend) ThreadDetail(context.Context, string) (domain.ThreadDetail, error) {
	return domain.ThreadDetail{}, nil
}
func (stubBackend) ThreadComments(context.Context, string) ([]domain.Comment, error) {
	return nil, nil
}
func (stubBackend) PostComment(context.Context, string, string) (domain.Comment, error) {
	return domain.Comment{}, nil
}
func (stubBackend) UpdateThread(context.Context, string, domain.ThreadEdit) (domain.ThreadDetail, error) {
	return domain.ThreadDetail{}, nil
}
func (stubBackend) MoveThread(context.Context, string, string) (domain.ThreadDetail, error) {
	return domain.ThreadDetail{}, nil
}
func (stubBackend) DeleteBubble(context.Context, string) (int, error) { return 0, nil }
func (stubBackend) DeleteThread(context.Context, string) (int, error) { return 0, nil }
func (stubBackend) DeleteRegion(context.Context, string, md.Region) (domain.ThreadDetail, error) {
	return domain.ThreadDetail{}, nil
}
func (stubBackend) ToggleTodo(context.Context, string, md.Region, int, string, bool) (domain.ThreadDetail, error) {
	return domain.ThreadDetail{}, nil
}
func (stubBackend) AddRevision(context.Context, string, string, string) (domain.ThreadDetail, error) {
	return domain.ThreadDetail{}, nil
}
func (stubBackend) MarkCommentsRead(context.Context, string, []string) error { return nil }
