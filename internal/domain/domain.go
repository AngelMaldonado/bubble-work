// Package domain holds the shared Bubble Work types and DTOs (§1–§5 of the spec).
package domain

import (
	"context"
	"time"
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

// Thread is an executable unit of work inside a bubble (maps to a Plane work item).
type Thread struct {
	ID     string
	Name   string
	Active bool
}

// Instance is a configured Plane deployment the server federates over. Each maps
// to one pinned Plane project. API keys live server-side only (§9.2).
type Instance struct {
	Slug      string // short id, e.g. "ayetec"
	Name      string
	BaseURL   string // e.g. https://plane.ayetec.space
	APIKey    string
	Workspace string // Plane workspace slug
	Project   string // pinned Plane project id = our Workspace (§7.1)
}

// Bubble is a durable grouping of threads (maps to a Plane Module) plus its
// contract overlay (§4). Heat is never stored here — it is derived from Evidence.
// ID is namespaced as "<instance-slug>:<module-id>" so it is unique across
// federated instances.
type Bubble struct {
	ID       string
	Name     string
	Instance string // origin instance slug
	Outcome  string // §4 contract: what "done" looks like
	Owner    string // §4 contract: who is accountable now
	Closure  string // §4 contract: the explicit close signal
	Closed   bool
	Threads  []Thread
	Evidence []EvidenceEvent
}

// Actor is the resolved identity of a request. Humans are resolved by
// pass-through of their Plane API key (identity + role + scope derived from
// Plane); agents are resolved from a server-minted token (§9.3). Instances is
// the set of instance slugs the actor may see.
type Actor struct {
	ID        string   `json:"id"`   // Plane user id, or agent member id
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // "human" | "agent"
	Email     string   `json:"email,omitempty"`
	Admin     bool     `json:"admin"`
	Instances []string `json:"instances"`
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

// BubbleView is the derived, client-facing projection of a bubble (§9.5).
type BubbleView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Instance  string    `json:"instance"`
	Lifecycle Lifecycle `json:"lifecycle"`
	Score     float64   `json:"score"` // 0..1 buoyancy score, for ordering
	Reason    string    `json:"reason"`
	Outcome   string    `json:"outcome,omitempty"`
	Owner     string    `json:"owner,omitempty"`
	Threads   int       `json:"threads"`
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
