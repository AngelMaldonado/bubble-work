// Package plane is the server's REST client for Plane — the ONLY process that
// talks to Plane, and only over REST (§9.6). Clients never reach Plane directly.
package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIError is a non-2xx response from Plane, carrying the status code so callers
// can distinguish a rejected credential (401/403) from a transient upstream
// failure (429/5xx) that should be retried rather than treated as "no access".
type APIError struct {
	Status     int
	Path       string
	retryAfter time.Duration // from Retry-After, used internally for backoff
}

func (e *APIError) Error() string {
	return fmt.Sprintf("plane %s: HTTP %d", e.Path, e.Status)
}

// IsAuthError reports whether err is a Plane 401/403 (credential rejected).
// Anything else (transient status, timeout, network) is not an auth failure.
func IsAuthError(err error) bool {
	var e *APIError
	if errors.As(err, &e) {
		return e.Status == 401 || e.Status == 403
	}
	return false
}

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
	const maxAttempts = 4 // 1 try + 3 retries
	bud := c.budget()
	ln := laneOf(ctx)
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoffFor(lastErr, bud, attempt)); err != nil {
				return err
			}
		}
		// Background work yields here when the remaining allowance is down to the
		// reserved slice; interactive callers are never delayed.
		if err := bud.wait(ctx, ln); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-API-Key", c.APIKey)
		bud.begin()
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err // network/timeout → retry
			continue
		}
		bud.observe(resp.Header, resp.StatusCode)
		// Retry only on rate-limit / server errors; other statuses are final.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = &APIError{Status: resp.StatusCode, Path: "GET " + path, retryAfter: retryAfter(resp)}
			resp.Body.Close()
			continue
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			return &APIError{Status: resp.StatusCode, Path: "GET " + path}
		}
		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return err
	}
	return lastErr
}

// backoffFor decides how long to wait before a retry. Retry-After wins when
// Plane sends it; otherwise, if we were rate-limited and know when the window
// rolls over, wait for the reset — retrying sooner is guaranteed to fail and
// only burns more of the allowance. Everything else falls back to exponential.
func backoffFor(lastErr error, bud *Budget, attempt int) time.Duration {
	exp := time.Duration(400*(1<<(attempt-1))) * time.Millisecond
	ae, ok := lastErr.(*APIError)
	if !ok {
		return exp
	}
	if ae.retryAfter > 0 {
		return ae.retryAfter
	}
	if ae.Status == http.StatusTooManyRequests {
		if d := bud.resetIn(); d > 0 {
			if d > maxLaneWait {
				d = maxLaneWait
			}
			return d
		}
	}
	return exp
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryAfter parses a Retry-After header expressed in whole seconds, capped at
// 5s so a hostile value can't stall a fetch.
func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}
	if secs > 5 {
		secs = 5
	}
	return time.Duration(secs) * time.Second
}

// send writes a JSON body to Plane with the given method and decodes the
// response (out may be nil).
func (c *Client) send(ctx context.Context, method, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	// Writes draw on the same per-key allowance as reads, so they must be
	// budgeted too — otherwise the auto-state writer could quietly drain what a
	// human's page load needs. No retry here on purpose: a failed write is the
	// outbox's job (docs/PLANE-SYNC.md Phase 5), not a silent repeat.
	bud := c.budget()
	if err := bud.wait(ctx, laneOf(ctx)); err != nil {
		return err
	}
	bud.begin()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bud.observe(resp.Header, resp.StatusCode)
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plane %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.send(ctx, http.MethodPost, path, body, out)
}

func (c *Client) patch(ctx context.Context, path string, body, out any) error {
	return c.send(ctx, http.MethodPatch, path, body, out)
}

func (c *Client) del(ctx context.Context, path string) error {
	return c.send(ctx, http.MethodDelete, path, nil, nil)
}

// SetProjectName renames a workspace (a Plane project).
func (c *Client) SetProjectName(ctx context.Context, projectID, name string) error {
	return c.patch(ctx, c.workspaceBase()+"/projects/"+projectID+"/", map[string]any{"name": name}, nil)
}

// DeleteProject removes a workspace from Plane, and with it every work item,
// module, comment and attachment inside. IRREVERSIBLE, and by a wide margin the
// most destructive call in this client.
func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	return c.del(ctx, c.workspaceBase()+"/projects/"+projectID+"/")
}

// DeleteModule removes a bubble from Plane. Plane keeps the work items that
// were in it — a module is a grouping, not a container — so they survive,
// belonging to no bubble.
//
// IRREVERSIBLE. §5.3 says a bubble should normally die by being CLOSED, which
// keeps the record of what was done; this is for the ones that should never
// have existed.
func (c *Client) DeleteModule(ctx context.Context, moduleID string) error {
	return c.del(ctx, c.projectBase()+"/modules/"+moduleID+"/")
}

// DeleteWorkItem removes a thread — or a revision, which is a sub-work-item —
// from Plane, taking its Brief, Logbook, comments and children with it.
// IRREVERSIBLE.
func (c *Client) DeleteWorkItem(ctx context.Context, workItemID string) error {
	return c.del(ctx, c.projectBase()+"/work-items/"+workItemID+"/")
}

// CreateProject creates a Plane project (our Workspace) and enables the given
// feature toggles (module_view etc.). Minimal by default: modules on, rest off.
func (c *Client) CreateProject(ctx context.Context, name, identifier string, features map[string]bool) (Project, error) {
	var out Project
	if err := c.post(ctx, c.workspaceBase()+"/projects/", map[string]any{"name": name, "identifier": identifier}, &out); err != nil {
		return Project{}, err
	}
	if len(features) > 0 {
		body := make(map[string]any, len(features))
		for k, v := range features {
			body[k] = v
		}
		if err := c.patch(ctx, c.workspaceBase()+"/projects/"+out.ID+"/", body, nil); err != nil {
			return out, fmt.Errorf("project %s created but enabling features failed: %w", out.ID, err)
		}
	}
	return out, nil
}

// CreateModule creates a Plane module (a Bubble) in the client's pinned project.
func (c *Client) CreateModule(ctx context.Context, name string) (Module, error) {
	var out Module
	if err := c.post(ctx, c.projectBase()+"/modules/", map[string]any{"name": name}, &out); err != nil {
		return Module{}, err
	}
	return out, nil
}

// State is one of a project's workflow states. Group is the stable machine
// grouping (backlog|unstarted|started|completed|cancelled); Name is whatever the
// project configured and is localized, so ALWAYS match on Group, never on Name.
type State struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Group   string `json:"group"`
	Default bool   `json:"default"`
}

// ListStates returns every workflow state in the client's project.
func (c *Client) ListStates(ctx context.Context) ([]State, error) {
	var out []State
	err := c.getPaged(ctx, c.projectBase()+"/states/", func(raw json.RawMessage) error {
		var page []State
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	})
	return out, err
}

// DefaultState returns a state id to create work items in: the project's default
// state, else an unstarted/backlog one, else the first.
func (c *Client) DefaultState(ctx context.Context) (string, error) {
	states, err := c.ListStates(ctx)
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
	return c.CreateChildWorkItem(ctx, name, descriptionHTML, stateID, "")
}

// CreateChildWorkItem creates a work item, optionally hanging it off a parent.
// A child IS a revision artifact — that is how revisions are modelled
// (INTERIOR-PLAN.md), and the mirror indexes them by parent_id.
func (c *Client) CreateChildWorkItem(ctx context.Context, name, descriptionHTML, stateID, parentID string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	body := map[string]any{"name": name, "state": stateID, "description_html": descriptionHTML}
	if parentID != "" {
		body["parent"] = parentID
	}
	if err := c.post(ctx, c.projectBase()+"/work-items/", body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// SetWorkItemBody rewrites a work item's description. This is how an artifact
// (Brief + Logbook, one page by design) changes.
//
// It returns the item's updated_at so the caller can write the same row into the
// mirror rather than waiting for a delta pass to rediscover its own write.
func (c *Client) SetWorkItemBody(ctx context.Context, workItemID, descriptionHTML string) (time.Time, error) {
	var out struct {
		UpdatedAt time.Time `json:"updated_at"`
	}
	err := c.patch(ctx, c.projectBase()+"/work-items/"+workItemID+"/",
		map[string]any{"description_html": descriptionHTML}, &out)
	return out.UpdatedAt, err
}

// SetWorkItemName renames a work item. A thread's title is its Plane name, and
// so is a revision's — a revision IS a sub-work-item — so one call covers both.
// A rename is not production: it changes what the work is called, not what has
// been done.
func (c *Client) SetWorkItemName(ctx context.Context, workItemID, name string) (time.Time, error) {
	var out struct {
		UpdatedAt time.Time `json:"updated_at"`
	}
	err := c.patch(ctx, c.projectBase()+"/work-items/"+workItemID+"/",
		map[string]any{"name": name}, &out)
	return out.UpdatedAt, err
}

// SetWorkItemState moves a work item to a state. This is the ONLY call that
// changes a card's position in Plane's pipeline, and it is made solely by the
// opt-in auto-state sweep (THREAD-LIFECYCLE.md Phase B) — never on a read path.
// Pass a state id resolved by GROUP: state names are project-configured and
// localized, so matching on them would break on any non-English project.
func (c *Client) SetWorkItemState(ctx context.Context, workItemID, stateID string) error {
	return c.patch(ctx, c.projectBase()+"/work-items/"+workItemID+"/", map[string]any{"state": stateID}, nil)
}

// RemoveIssueFromModule unlinks a work item from a module. The work item stays
// in the project — only the grouping changes, which is what makes a re-home
// safe: the Brief, the Logbook, the comments and the id all survive.
func (c *Client) RemoveIssueFromModule(ctx context.Context, moduleID, issueID string) error {
	return c.del(ctx, c.projectBase()+"/modules/"+moduleID+"/module-issues/"+issueID+"/")
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// ListProjectMembers returns the people who are members of ONE project.
//
// Plane scopes membership per project, not just per workspace: every project on
// a real workspace can be private (network 0) with its own member list. The
// workspace member list answers "may this person use Plane at all", which is a
// different question from "may this person see this work".
func (c *Client) ListProjectMembers(ctx context.Context, projectID string) ([]Member, error) {
	// A BARE ARRAY, not a paginated envelope — verified against a live Plane on
	// 2026-08-07, and the same shape the workspace /members/ endpoint uses.
	var out []Member
	err := c.get(ctx, c.workspaceBase()+"/projects/"+projectID+"/members/", &out)
	return out, err
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

// Relations groups a work item's typed relationships (INTERIOR-PLAN.md). Each
// slice holds related work-item ids. RelatesTo drives revision artifacts.
type Relations struct {
	Blocking     []string `json:"blocking"`
	BlockedBy    []string `json:"blocked_by"`
	Duplicate    []string `json:"duplicate"`
	RelatesTo    []string `json:"relates_to"`
	StartAfter   []string `json:"start_after"`
	StartBefore  []string `json:"start_before"`
	FinishAfter  []string `json:"finish_after"`
	FinishBefore []string `json:"finish_before"`
}

// Comment is one work-item comment (INTERIOR-PLAN.md Phase 12). HTML is the
// raw ProseMirror body; the server converts it to Markdown for rendering.
type Comment struct {
	ID        string
	ActorID   string
	HTML      string
	CreatedAt time.Time
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

// Cycle is a project's repeating pulse (§1). start_date/end_date may be null on
// draft cycles, so they are decoded as strings and parsed leniently.
type Cycle struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// ListCycles returns every cycle in the client's project (needs Project set).
func (c *Client) ListCycles(ctx context.Context) ([]Cycle, error) {
	var out []Cycle
	err := c.getPaged(ctx, c.projectBase()+"/cycles/", func(raw json.RawMessage) error {
		var page []Cycle
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	})
	return out, err
}

// Start parses a cycle's start_date; ok is false when it is unset/invalid.
func (c Cycle) Start() (t time.Time, ok bool) { return parsePlaneDate(c.StartDate) }

// End parses a cycle's end_date (treated as end-of-day for date-only values).
func (c Cycle) End() (t time.Time, ok bool) {
	t, ok = parsePlaneDate(c.EndDate)
	if ok && len(c.EndDate) == len("2006-01-02") {
		t = t.Add(24*time.Hour - time.Second) // inclusive end of that day
	}
	return t, ok
}

// parsePlaneDate accepts RFC3339 timestamps or bare YYYY-MM-DD dates.
func parsePlaneDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// ListRelations returns a work item's typed relationships (INTERIOR-PLAN.md).
func (c *Client) ListRelations(ctx context.Context, workItemID string) (Relations, error) {
	var r Relations
	if err := c.get(ctx, c.projectBase()+"/work-items/"+workItemID+"/relations/", &r); err != nil {
		return Relations{}, err
	}
	return r, nil
}

// ListComments returns a work item's comments oldest-first as Plane paginates
// them (INTERIOR-PLAN.md Phase 12).
func (c *Client) ListComments(ctx context.Context, workItemID string) ([]Comment, error) {
	var out []Comment
	err := c.getPaged(ctx, c.projectBase()+"/work-items/"+workItemID+"/comments/", func(raw json.RawMessage) error {
		var page []struct {
			ID          string    `json:"id"`
			Actor       string    `json:"actor"`
			CommentHTML string    `json:"comment_html"`
			CreatedAt   time.Time `json:"created_at"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		for _, cm := range page {
			out = append(out, Comment{ID: cm.ID, ActorID: cm.Actor, HTML: cm.CommentHTML, CreatedAt: cm.CreatedAt})
		}
		return nil
	})
	return out, err
}

// CreateComment posts a comment on a work item and returns the created entry.
// The caller's key (passed as c's key) determines the Plane actor.
func (c *Client) CreateComment(ctx context.Context, workItemID, commentHTML string) (Comment, error) {
	var out struct {
		ID          string    `json:"id"`
		Actor       string    `json:"actor"`
		CommentHTML string    `json:"comment_html"`
		CreatedAt   time.Time `json:"created_at"`
	}
	body := map[string]any{"comment_html": commentHTML}
	if err := c.post(ctx, c.projectBase()+"/work-items/"+workItemID+"/comments/", body, &out); err != nil {
		return Comment{}, err
	}
	return Comment{ID: out.ID, ActorID: out.Actor, HTML: out.CommentHTML, CreatedAt: out.CreatedAt}, nil
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
// Kinds mirror the domain.Ev* constants. Currently unused — the snapshot derives
// evidence from timestamps and diffing (THREAD-LIFECYCLE.md Phase C) rather than
// paying an activity fetch per work item.
func Meaningful(a Activity) (kind string, ok bool) {
	switch {
	case a.Verb == "created" && a.Field == "":
		return "thread-created", true
	case a.Field == "completed_at":
		return "thread-completed", true
	case a.Field == "state":
		return "state-change", true
	default:
		return "", false
	}
}

// ---- pages ----
//
// A Plane PAGE is project-level prose: documentation, a product spec, anything
// that outlives one work item. Plane exposes them at
// /workspaces/{slug}/projects/{id}/pages/ and they carry description_html in the
// same editor shape as a work item's description, so internal/md reads and
// writes them with no special casing.
//
// The list endpoint does NOT return description_html — only the metadata. The
// body costs one GET per page, which is why nothing here fetches bodies in bulk.
type Page struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description_html"`
	// Access is 0 for a project-visible page and 1 for a private one.
	Access     int     `json:"access"`
	IsLocked   bool    `json:"is_locked"`
	ArchivedAt *string `json:"archived_at"`
	OwnedBy    string  `json:"owned_by"`
	ParentID   string  `json:"parent_id"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// Archived reports whether Plane has archived this page.
func (p Page) Archived() bool { return p.ArchivedAt != nil && *p.ArchivedAt != "" }

// ListPages returns a project's pages, metadata only.
func (c *Client) ListPages(ctx context.Context, projectID string) ([]Page, error) {
	var out []Page
	err := c.getPaged(ctx, c.pagesBase(projectID), func(raw json.RawMessage) error {
		var page []Page
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	})
	return out, err
}

// GetPage returns one page WITH its body.
func (c *Client) GetPage(ctx context.Context, projectID, pageID string) (Page, error) {
	var p Page
	err := c.get(ctx, c.pagesBase(projectID)+pageID+"/", &p)
	return p, err
}

// CreatePage adds a page to a project. Access 0 = visible to the project, which
// is the only kind worth creating from here: a private page nobody else can read
// is not documentation.
func (c *Client) CreatePage(ctx context.Context, projectID, name, html string) (Page, error) {
	var p Page
	body := map[string]any{"name": name, "access": 0}
	if html != "" {
		body["description_html"] = html
	}
	err := c.post(ctx, c.pagesBase(projectID), body, &p)
	return p, err
}

// UpdatePage patches a page. Only the named fields are sent, so a title change
// cannot silently blank a body.
func (c *Client) UpdatePage(ctx context.Context, projectID, pageID string, patch map[string]any) error {
	return c.patch(ctx, c.pagesBase(projectID)+pageID+"/", patch, nil)
}

// DeletePage removes a page from Plane.
func (c *Client) DeletePage(ctx context.Context, projectID, pageID string) error {
	return c.del(ctx, c.pagesBase(projectID)+pageID+"/")
}

// pagesBase is spelled out rather than using projectBase() because pages are
// addressed per project id, and a client is often built for a different one.
func (c *Client) pagesBase(projectID string) string {
	return fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/pages/", c.Workspace, projectID)
}
