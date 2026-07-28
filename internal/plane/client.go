// Package plane is the server's REST client for Plane — the ONLY process that
// talks to Plane, and only over REST (§9.6). Clients never reach Plane directly.
package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client talks to a Plane workspace/project over the REST API using X-API-Key.
type Client struct {
	BaseURL   string
	APIKey    string
	Workspace string // workspace slug
	Project   string // project uuid = our "Workspace" (§7.1)
	HTTP      *http.Client
}

// New builds a Plane client. Base defaults to https://api.plane.so when empty.
func New(base, key, workspace, project string) *Client {
	if base == "" {
		base = "https://api.plane.so"
	}
	return &Client{
		BaseURL:   strings.TrimRight(base, "/"),
		APIKey:    key,
		Workspace: workspace,
		Project:   project,
		HTTP:      &http.Client{Timeout: 20 * time.Second},
	}
}

// Configured reports whether enough is set to reach Plane.
func (c *Client) Configured() bool {
	return c != nil && c.APIKey != "" && c.Workspace != "" && c.Project != ""
}

func (c *Client) projectBase() string {
	return fmt.Sprintf("/api/v1/workspaces/%s/projects/%s", c.Workspace, c.Project)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("plane GET %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Module is a Bubble (§7.1).
type Module struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkItem is a Thread (§7.1).
type WorkItem struct {
	ID        string
	Name      string
	Completed bool
}

// Activity is a raw Plane timeline entry; only some map to heat (see Meaningful).
type Activity struct {
	At    time.Time
	Field string
	Verb  string
}

// ListModules returns the bubbles in the project.
func (c *Client) ListModules(ctx context.Context) ([]Module, error) {
	var r struct {
		Results []Module `json:"results"`
	}
	if err := c.get(ctx, c.projectBase()+"/modules/", &r); err != nil {
		return nil, err
	}
	return r.Results, nil
}

// ListModuleWorkItems returns the threads in a bubble.
func (c *Client) ListModuleWorkItems(ctx context.Context, moduleID string) ([]WorkItem, error) {
	var r struct {
		Results []struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			CompletedAt *string `json:"completed_at"`
		} `json:"results"`
	}
	if err := c.get(ctx, c.projectBase()+"/modules/"+moduleID+"/module-issues/", &r); err != nil {
		return nil, err
	}
	out := make([]WorkItem, 0, len(r.Results))
	for _, it := range r.Results {
		out = append(out, WorkItem{ID: it.ID, Name: it.Name, Completed: it.CompletedAt != nil})
	}
	return out, nil
}

// ListActivities returns a work item's timeline (source of heat evidence, §5.1).
func (c *Client) ListActivities(ctx context.Context, workItemID string) ([]Activity, error) {
	var r struct {
		Results []struct {
			CreatedAt time.Time `json:"created_at"`
			Field     *string   `json:"field"`
			Verb      string    `json:"verb"`
		} `json:"results"`
	}
	if err := c.get(ctx, c.projectBase()+"/work-items/"+workItemID+"/activities/", &r); err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(r.Results))
	for _, a := range r.Results {
		f := ""
		if a.Field != nil {
			f = *a.Field
		}
		out = append(out, Activity{At: a.CreatedAt, Field: f, Verb: a.Verb})
	}
	return out, nil
}

// Meaningful maps a raw Plane activity to a heat-generating evidence kind (§5.1).
// Only outputs that changed reality count; comments/cosmetic edits return false.
// TODO: broaden the allowlist as we observe real Plane activity shapes.
func Meaningful(a Activity) (kind string, ok bool) {
	switch {
	case a.Verb == "created" && a.Field == "":
		return "thread-created", true
	case a.Field == "completed_at":
		return "completed-todo", true
	case a.Field == "state":
		return "state-change", true
	default:
		return "", false
	}
}
