// Package domain holds the shared Bubble Work types and DTOs (§1–§5 of the spec).
package domain

import "time"

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

// Bubble is a durable grouping of threads (maps to a Plane Module) plus its
// contract overlay (§4). Heat is never stored here — it is derived from Evidence.
type Bubble struct {
	ID       string
	Name     string
	Outcome  string // §4 contract: what "done" looks like
	Owner    string // §4 contract: who is accountable now
	Closure  string // §4 contract: the explicit close signal
	Closed   bool
	Threads  []Thread
	Evidence []EvidenceEvent
}

// Member is a client identity — human or agent — that acts as a team member (§9.3).
type Member struct {
	ID      string
	Name    string
	Kind    string // "human" | "agent"
	PlaneID string
}

// BubbleView is the derived, client-facing projection of a bubble (§9.5).
type BubbleView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
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
