// Package domain holds the shared Bubble Work types and DTOs (§1–§5 of the spec).
package domain

import (
	"context"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// Lifecycle is a bubble's temperature/buoyancy state (§5.3).
type Lifecycle string

const (
	Hot     Lifecycle = "hot"
	Warm    Lifecycle = "warm"
	Cooling Lifecycle = "cooling"
	Dormant Lifecycle = "dormant"
	Closed  Lifecycle = "closed"
)

// EvidenceEvent is a meaningful output that generates heat (§5.1).
// Comments, pings and cosmetic edits are intentionally NOT evidence.
type EvidenceEvent struct {
	ThreadID string
	Kind     string
	At       time.Time
}

// Thread is an executable unit of work inside a bubble (maps to a Plane work
// item). The extra fields feed the timeline view straight from the snapshot, so
// opening a bubble never has to fetch Plane (§ freshness).
type Thread struct {
	ID          string
	Name        string
	Active      bool
	Seq         int
	Owner       string // resolved assignee display name (first assignee)
	Parent      string // parent work-item id ("" if top-level)
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// Instance is a configured Plane deployment the server federates over. Each maps
// to one pinned Plane project. API keys live server-side only (§9.2).
type Instance struct {
	Slug          string // short id, e.g. "ayetec"
	Name          string
	BaseURL       string // e.g. https://plane.ayetec.space
	APIKey        string
	Workspace     string // Plane workspace slug
	Project       string // pinned Plane project id = our Workspace (§7.1)
	WebhookSecret string // HMAC secret for verifying inbound Plane webhooks (§6)
}

// Bubble is a durable grouping of threads (maps to a Plane Module) plus its
// contract overlay (§4). Heat is never stored here — it is derived from Evidence.
// ID is namespaced as "<instance-slug>:<module-id>" so it is unique across
// federated instances.
type Bubble struct {
	ID          string
	Name        string
	Instance    string // origin instance slug
	Project     string // origin Plane project id (our Workspace)
	ProjectName string // human name of that project, when known
	Outcome     string // §4 contract: what "done" looks like
	Owner    string // §4 contract: who is accountable now
	Closure  string // §4 contract: the explicit close signal
	Closed   bool
	Stage    string // explicit overlay: "" | "reviewed"
	Threads  []Thread
	Evidence []EvidenceEvent

	// Cycle-aware heat window (§3.6): the active Plane cycle's start and the
	// previous cycle's start. Zero means the project has no active cycle, so heat
	// falls back to the rolling window (CycleHours).
	CycleStart     time.Time
	CyclePrevStart time.Time
}

// Actor is the resolved identity of a request. Humans are resolved by
// pass-through of their Plane API key (identity + role + scope derived from
// Plane); agents are resolved from a server-minted token (§9.3). Instances is
// the set of instance slugs the actor may see.
type Actor struct {
	ID           string   `json:"id"` // Plane user id, or "service-admin"
	Name         string   `json:"name"`
	Kind         string   `json:"kind"` // "human" | "agent" | "admin"
	Email        string   `json:"email,omitempty"`
	Admin        bool     `json:"admin"`                   // Plane workspace admin (role ≥ 20)
	ServiceAdmin bool     `json:"service_admin,omitempty"` // godmode — cross-org ops (§ admin)
	Instances    []string `json:"instances"`
}

// Label returns a stable human label for logs and attribution.
func (a Actor) Label() string {
	name := a.Name
	if name == "" {
		name = a.Email
	}
	if a.ID == "" {
		return "anonymous"
	}
	if name != "" {
		return name + " (" + a.Kind + ")"
	}
	return a.ID
}

// CanSee reports whether the actor is authorized for an instance slug.
func (a Actor) CanSee(slug string) bool {
	for _, s := range a.Instances {
		if s == slug {
			return true
		}
	}
	return false
}

type actorCtxKey struct{}

// WithActor attaches the acting identity to a context. Both front doors (REST
// middleware and MCP tool handlers) use this so the backend reads the actor
// uniformly (§9.3).
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorCtxKey{}, a)
}

// ActorFrom returns the acting identity previously attached with WithActor.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorCtxKey{}).(Actor)
	return a, ok
}

type credCtxKey struct{}

// WithCred stashes the caller's raw Plane API key on the context so write
// operations can act AS that person (impersonation). It is never serialized.
func WithCred(ctx context.Context, cred string) context.Context {
	return context.WithValue(ctx, credCtxKey{}, cred)
}

// CredFrom returns the caller's raw Plane key, if present.
func CredFrom(ctx context.Context) (string, bool) {
	c, ok := ctx.Value(credCtxKey{}).(string)
	return c, ok
}

// BubbleView is the derived, client-facing projection of a bubble (§9.5).
type BubbleView struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Instance    string    `json:"instance"`
	Project     string    `json:"project"`                 // Plane project id (our Workspace)
	ProjectName string    `json:"project_name,omitempty"`  // human name, when known
	Lifecycle   Lifecycle `json:"lifecycle"`
	Level       string    `json:"level"` // UI band: in_progress|zzzz|rip|reviewed|done
	Score       float64   `json:"score"` // 0..1 buoyancy score, for ordering
	Reason      string    `json:"reason"`
	Outcome     string    `json:"outcome,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Threads     int       `json:"threads"`
}

// ThreadHit is a searchable thread (task) for the ⌘K omnibar.
type ThreadHit struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BubbleID   string `json:"bubble_id"`
	BubbleName string `json:"bubble_name"`
	Instance   string `json:"instance"`
	Open       bool   `json:"open"`
}

// ThreadNode is one entry in a bubble's timeline — the git-log-oneline history
// (INTERIOR-PLAN.md Phase 10). ID is namespaced (slug:project:workitem) so a
// client can open the thread directly.
type ThreadNode struct {
	ID          string     `json:"id"`
	Seq         int        `json:"seq"`
	Title       string     `json:"title"`
	Active      bool       `json:"active"`
	Owner       string     `json:"owner,omitempty"`
	Parent      string     `json:"parent,omitempty"` // raw parent work-item id
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ThreadDetail is a thread's full interior: work-artifact files, the logbook,
// and revision artifacts (INTERIOR-PLAN.md Phase 11). Read-only in v1.
type ThreadDetail struct {
	ID          string        `json:"id"`
	Seq         int           `json:"seq"`
	Title       string        `json:"title"`
	Kind        string        `json:"kind"` // "simple" | "phased"
	Active      bool          `json:"active"`
	Priority    string        `json:"priority,omitempty"`
	Assignees   []string      `json:"assignees,omitempty"`
	Artifacts   []md.Artifact `json:"artifacts"`
	Logbook     *md.Logbook   `json:"logbook,omitempty"`
	Revisions   []md.Artifact `json:"revisions"`
	CreatedAt   time.Time     `json:"created_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
}

// Comment is one rendered entry in a thread's comment feed (Phase 12).
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Markdown  string    `json:"markdown"`
	CreatedAt time.Time `json:"created_at"`
}

// BirthRequest carries the two birth artifacts the policy engine enforces (§3).
type BirthRequest struct {
	BubbleID    string `json:"bubble_id"`
	Name        string `json:"name"`
	Brief       string `json:"brief"`
	Logbook     string `json:"logbook"`
	SmallThread bool   `json:"small_thread"` // §3.2 escape hatch
}

// BirthResult is returned once a thread's birth artifacts pass the policy gate.
type BirthResult struct {
	ThreadID string `json:"thread_id"`
	Created  bool   `json:"created"`
	Message  string `json:"message"`
}

// Contract is a bubble's §4 overlay as returned to clients.
type Contract struct {
	Outcome string `json:"outcome"`
	Owner   string `json:"owner"`
	Closure string `json:"closure"`
	Closed  bool   `json:"closed"`
}

// ContractInput is a partial update of a bubble's contract (§4). A nil field is
// left unchanged; a non-nil field (including empty string) is applied.
type ContractInput struct {
	Outcome *string `json:"outcome,omitempty"`
	Owner   *string `json:"owner,omitempty"`
	Closure *string `json:"closure,omitempty"`
}

// CreateWorkspaceRequest asks the server to create a Plane project (our
// Workspace). Modules, Cycles and Pages are on by default (Cycles is the heat
// cadence, §1; Pages backs docs); Views/Intake are opt-in.
type CreateWorkspaceRequest struct {
	Instance   string `json:"instance"`
	Name       string `json:"name"`
	Identifier string `json:"identifier,omitempty"` // auto-derived from name if empty
	NoCycles   bool   `json:"no_cycles,omitempty"`  // cycles on by default
	NoPages    bool   `json:"no_pages,omitempty"`   // pages on by default
	Views      bool   `json:"views,omitempty"`      // off by default
	Intake     bool   `json:"intake,omitempty"`     // off by default
}

// Workspace is a created Plane project (§7.1).
type Workspace struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Instance   string `json:"instance"`
}

// CreateBubbleRequest asks the server to create a Plane module (a Bubble) in a
// project (§3.5).
type CreateBubbleRequest struct {
	Instance string `json:"instance"`
	Project  string `json:"project"`
	Name     string `json:"name"`
}

// NewBubble is a freshly created bubble with its namespaced id.
type NewBubble struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AdminInstance is a redacted instance view for service admins (no secrets).
type AdminInstance struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	Workspace  string `json:"workspace"`
	Project    string `json:"project"`
	HasWebhook bool   `json:"has_webhook"`
	Cached     bool   `json:"cached"`
}

// AdminStats is a service-admin health snapshot.
type AdminStats struct {
	Instances       int    `json:"instances"`
	CachedInstances int    `json:"cached_instances"`
	CachedIdents    int    `json:"cached_identities"`
	Revision        string `json:"revision"`
	Built           string `json:"built"`
	StartedAt       string `json:"started_at"`
}

// Notification records a bubble crossing into a colder state (§5) — the push
// side of the buoyancy model: it tells you what's sinking without you looking.
type Notification struct {
	ID         int64  `json:"id"`
	At         string `json:"at"`
	Instance   string `json:"instance"`
	BubbleID   string `json:"bubble_id"`
	BubbleName string `json:"bubble_name"`
	Kind       string `json:"kind"` // "cooling" | "dormant"
	Message    string `json:"message"`
	Unread     bool   `json:"unread"`
}

// Inbox is a person's notification view: whether they've opted in, how many are
// unread, and the items themselves (with per-person read state).
type Inbox struct {
	Enabled       bool           `json:"enabled"`
	UnreadCount   int            `json:"unread_count"`
	Notifications []Notification `json:"notifications"`
}
