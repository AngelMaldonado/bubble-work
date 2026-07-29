// Package plane is the server's REST client for Plane — the ONLY process that
// talks to Plane, and only over REST (§9.6). Clients never reach Plane directly.
package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Configured reports whether enough is set to reach Plane at the workspace
// level. A pinned project is optional (empty = whole workspace).
func (c *Client) Configured() bool {
	return c != nil && c.APIKey != "" && c.Workspace != ""
}

func (c *Client) workspaceBase() string {
	return fmt.Sprintf("/api/v1/workspaces/%s", c.Workspace)
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

// post sends a JSON body to Plane and decodes the response (out may be nil).
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plane POST %s: %s: %s", path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// DefaultState returns a state id to create work items in: the project's default
// state, else an unstarted/backlog one, else the first.
func (c *Client) DefaultState(ctx context.Context) (string, error) {
	var states []struct {
		ID      string `json:"id"`
		Group   string `json:"group"`
		Default bool   `json:"default"`
	}
	err := c.getPaged(ctx, c.projectBase()+"/states/", func(raw json.RawMessage) error {
		var page []struct {
			ID      string `json:"id"`
			Group   string `json:"group"`
			Default bool   `json:"default"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		states = append(states, page...)
		return nil
	})
	if err != nil {
		return "", err
	}
	var unstarted, first string
	for _, s := range states {
		if first == "" {
			first = s.ID
		}
		if s.Default {
			return s.ID, nil
		}
		if unstarted == "" && (s.Group == "unstarted" || s.Group == "backlog") {
			unstarted = s.ID
		}
	}
	if unstarted != "" {
		return unstarted, nil
	}
	if first == "" {
		return "", fmt.Errorf("project has no states")
	}
	return first, nil
}

// CreateWorkItem creates a work item (thread) and returns its id.
func (c *Client) CreateWorkItem(ctx context.Context, name, descriptionHTML, stateID string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	body := map[string]any{"name": name, "state": stateID, "description_html": descriptionHTML}
	if err := c.post(ctx, c.projectBase()+"/work-items/", body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// AddIssuesToModule links work items to a module (bubble).
func (c *Client) AddIssuesToModule(ctx context.Context, moduleID string, issueIDs []string) error {
	return c.post(ctx, c.projectBase()+"/modules/"+moduleID+"/module-issues/", map[string]any{"issues": issueIDs}, nil)
}

// getPaged walks a cursor-paginated list endpoint, invoking each with every
// page's raw `results` array until Plane reports no further pages.
func (c *Client) getPaged(ctx context.Context, path string, each func(json.RawMessage) error) error {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	cursor := ""
	for {
		u := fmt.Sprintf("%s%sper_page=100", path, sep)
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}
		var env struct {
			Results    json.RawMessage `json:"results"`
			NextCursor string          `json:"next_cursor"`
			NextPage   bool            `json:"next_page_results"`
		}
		if err := c.get(ctx, u, &env); err != nil {
			return err
		}
		if len(env.Results) > 0 {
			if err := each(env.Results); err != nil {
				return err
			}
		}
		if !env.NextPage || env.NextCursor == "" {
			return nil
		}
		cursor = env.NextCursor
	}
}

// User is the profile behind an API key (from /users/me).
type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// Verify checks the client can reach Plane and resolve its target: it
// authenticates via /users/me and confirms the workspace (or pinned project)
// exists. Used by `instance add` to fail fast on typos before storing.
func (c *Client) Verify(ctx context.Context) (User, error) {
	u, err := c.Me(ctx)
	if err != nil {
		return User{}, fmt.Errorf("API key or base URL rejected: %w", err)
	}
	if c.Project == "" {
		if _, err := c.ListProjects(ctx); err != nil {
			return User{}, fmt.Errorf("workspace %q unreachable (check --workspace and key scope): %w", c.Workspace, err)
		}
	} else {
		var p struct {
			ID string `json:"id"`
		}
		if err := c.get(ctx, c.projectBase()+"/", &p); err != nil {
			return User{}, fmt.Errorf("project %q not found in workspace %q: %w", c.Project, c.Workspace, err)
		}
	}
	return u, nil
}

// Me identifies the user that owns this client's API key (§9.3 pass-through).
func (c *Client) Me(ctx context.Context) (User, error) {
	// /users/me may return the user flat or wrapped as {"user": {...}}.
	var resp struct {
		User
		Wrapped *User `json:"user"`
	}
	if err := c.get(ctx, "/api/v1/users/me", &resp); err != nil {
		return User{}, err
	}
	if resp.Wrapped != nil && resp.Wrapped.ID != "" {
		return *resp.Wrapped, nil
	}
	return resp.User, nil
}

// Plane workspace roles.
const (
	RoleAdmin  = 20
	RoleMember = 15
	RoleGuest  = 5
)

// Member is a Plane workspace member with their role (from /members/).
type Member struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
}

// Members lists the workspace members and their roles (needs workspaces.members:read).
func (c *Client) Members(ctx context.Context) ([]Member, error) {
	var out []Member // this endpoint returns a bare JSON array
	if err := c.get(ctx, c.workspaceBase()+"/members/", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Project is a Workspace (§7.1). Used to auto-discover a whole workspace.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListProjects returns every project in the workspace (project need not be set).
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var out []Project
	err := c.getPaged(ctx, c.workspaceBase()+"/projects/", func(raw json.RawMessage) error {
		var page []Project
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	})
	return out, err
}

// Module is a Bubble (§7.1).
type Module struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkItem is a Thread (§7.1). Timestamps come straight from the list response,
// so heat can be derived without a per-item activity fetch.
type WorkItem struct {
	ID          string
	Name        string
	CreatedAt   time.Time
	CompletedAt *time.Time
	Active      bool
}

// Activity is a raw Plane timeline entry; only some map to heat (see Meaningful).
type Activity struct {
	At    time.Time
	Field string
	Verb  string
}

// ListModules returns the bubbles in the project.
func (c *Client) ListModules(ctx context.Context) ([]Module, error) {
	var out []Module
	err := c.getPaged(ctx, c.projectBase()+"/modules/", func(raw json.RawMessage) error {
		var page []Module
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	})
	return out, err
}

// ListModuleWorkItems returns the threads in a bubble, with the timestamps and
// state group used to derive heat (§5) without extra calls.
func (c *Client) ListModuleWorkItems(ctx context.Context, moduleID string) ([]WorkItem, error) {
	var out []WorkItem
	err := c.getPaged(ctx, c.projectBase()+"/modules/"+moduleID+"/module-issues/", func(raw json.RawMessage) error {
		// On module-issues, `state` is the state UUID string (not the expanded
		// object), so we don't decode it — `completed_at` tells us if it's active.
		var page []struct {
			ID          string     `json:"id"`
			Name        string     `json:"name"`
			CreatedAt   time.Time  `json:"created_at"`
			CompletedAt *time.Time `json:"completed_at"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		for _, it := range page {
			out = append(out, WorkItem{
				ID:          it.ID,
				Name:        it.Name,
				CreatedAt:   it.CreatedAt,
				CompletedAt: it.CompletedAt,
				Active:      it.CompletedAt == nil,
			})
		}
		return nil
	})
	return out, err
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
