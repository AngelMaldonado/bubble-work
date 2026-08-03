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

// Evidence kinds (§5.1). Birth is what a thread *is*, not what it produced, so
// it is deliberately weaker than the rest: a thread with nothing but its own
// birth event never "got going" (THREAD-LIFECYCLE.md).
const (
	EvThreadCreated   = "thread-created"   // the work item came into being (weak)
	EvCompletedTodo   = "completed-todo"   // a logbook/DoD item got ticked
	EvLogbookUpdated  = "logbook-updated"  // the plan itself changed (§ working protocol)
	EvRevisionAdded   = "revision-added"   // a revision artifact (sub-item) landed
	EvThreadCompleted = "thread-completed" // the work item reached a completed state
	EvComment         = "comment"          // PULSE, not progress — see Pulse()
)

// EvidenceEvent is a meaningful output that generates heat (§5.1).
// Comments, pings and cosmetic edits are intentionally NOT evidence.
type EvidenceEvent struct {
	ThreadID string
	Kind     string
	At       time.Time
}

// Pulse reports whether an event is mere PRESENCE — someone is paying attention,
// but nothing changed. A pulse never heats anything at either grain; it only
// keeps a thread out of the grave (THREAD-LIFECYCLE.md).
func (e EvidenceEvent) Pulse() bool { return e.Kind == EvComment }

// Progress reports whether an event is evidence of *production* by the thread
// itself. Being born is not producing (that heats the BUBBLE that gained the
// thread, not the thread), and presence is not producing. Only progress can
// resurrect a thread (THREAD-LIFECYCLE.md).
func (e EvidenceEvent) Progress() bool {
	return e.Kind != EvThreadCreated && !e.Pulse()
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
	State       string // Plane state name as configured (localized)
	StateGroup  string // Plane state group: backlog|unstarted|started|completed|cancelled
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// Tuning is the calibration of the buoyancy model — every threshold the
// lifecycle rules used to hardcode, at BOTH grains (bubbles and threads). It is
// persisted server-side and editable by a service admin (God Mode) so a
// workspace can match the model to its own rhythm without a redeploy.
//
// The defaults reproduce the behaviour documented in AGENTS.md and
// THREAD-LIFECYCLE.md exactly; every field below states what moves when it does.
type Tuning struct {
	// ---- the pulse (shared by both grains) ----

	// CycleHours is the rolling heat window used when a project has no active
	// Plane cycle. When Plane has one, its real boundaries win (§3.6).
	CycleHours float64 `json:"cycle_hours"`
	// DormantCycles is how many cycles of silence make something Dormant. 2 means
	// "nothing this cycle or last". Lowering it makes the board sink faster.
	DormantCycles float64 `json:"dormant_cycles"`
	// DecayCycles scales the buoyancy score's exponential decay, in cycles.
	// Larger = things stay buoyant (ordered higher) for longer.
	DecayCycles float64 `json:"decay_cycles"`
	// OwnerlessIsDormant sinks anything with nobody accountable to Dormant once
	// it has gone quiet (§4: a contract needs an owner). Something still actively
	// producing stays Hot/Warm either way — output outranks paperwork.
	OwnerlessIsDormant bool `json:"ownerless_is_dormant"`

	// ---- bubble grain ----

	// BubbleRipNeedsOwner sends a dormant, ownerless bubble to 🪦 instead of 😴.
	// A bubble that never produced anything is 🪦 either way. Only consulted when
	// BubbleLevelRollup is off.
	BubbleRipNeedsOwner bool `json:"bubble_rip_needs_owner"`
	// BubbleLevelRollup makes a bubble's band the band of its HOTTEST unfinished
	// thread, instead of classifying the union of its evidence. The union counts a
	// thread's birth as bubble output, so a bubble full of untouched new work
	// items reads 🔥; the roll-up reports what its threads are actually doing.
	BubbleLevelRollup bool `json:"bubble_level_rollup"`

	// ---- thread grain (THREAD-LIFECYCLE.md) ----

	// ThreadBirthHeats makes a work item's own creation heat the thread itself.
	// Off by default: being born is not producing, and turning it on makes every
	// new Backlog item read 🔥 for a cycle. (A birth always heats its BUBBLE.)
	ThreadBirthHeats bool `json:"thread_birth_heats"`
	// ThreadGraceCycles is how long a newborn thread that has produced nothing
	// stays 😴 before it is called 🪦. 0 = no grace.
	ThreadGraceCycles float64 `json:"thread_grace_cycles"`
	// ThreadRipNeedsOwner sends a dormant, unassigned thread to 🪦 instead of 😴.
	ThreadRipNeedsOwner bool `json:"thread_rip_needs_owner"`
	// PulseCycles is how long a comment keeps a thread out of the grave. A comment
	// is presence, not production: it NEVER warms a thread to 🔥, but while
	// someone is still talking about a thread we don't declare it abandoned.
	// 0 disables the pulse entirely.
	PulseCycles float64 `json:"pulse_cycles"`
	// ThreadTerminalStateWins lets Plane's terminal columns override the computed
	// level: `completed` → 🏆, `cancelled` → 🪦. Those are statements of fact;
	// every other column is status theatre and is ignored either way. Note that
	// `cancelled` is still overridden by real production — see threadLevel.
	ThreadTerminalStateWins bool `json:"thread_terminal_state_wins"`
}

// DefaultTuning is the model's out-of-the-box calibration.
func DefaultTuning() Tuning {
	return Tuning{
		CycleHours:              168, // one week
		DormantCycles:           2,
		DecayCycles:             1,
		OwnerlessIsDormant:      true,
		BubbleRipNeedsOwner:     true,
		BubbleLevelRollup:       true,
		ThreadBirthHeats:        false,
		ThreadGraceCycles:       1,
		ThreadRipNeedsOwner:     true,
		PulseCycles:             1,
		ThreadTerminalStateWins: true,
	}
}

// Sanitize clamps the numeric knobs into a sane range, so a bad edit can't
// produce a nonsensical board (or a zero-length decay).
func (t Tuning) Sanitize() Tuning {
	t.CycleHours = clampF(t.CycleHours, 1, 24*365)
	t.DormantCycles = clampF(t.DormantCycles, 1, 52)
	t.DecayCycles = clampF(t.DecayCycles, 0.1, 52)
	t.ThreadGraceCycles = clampF(t.ThreadGraceCycles, 0, 52)
	t.PulseCycles = clampF(t.PulseCycles, 0, 52)
	return t
}

// Cycle is the pulse length as a duration.
func (t Tuning) Cycle() time.Duration {
	return time.Duration(t.CycleHours * float64(time.Hour))
}

// TuningView is what the admin surfaces read: the live calibration, the stock
// defaults (so a client can mark drift and offer a reset), and the self-
// describing schema of the knobs.
type TuningView struct {
	Tuning   Tuning        `json:"tuning"`
	Defaults Tuning        `json:"defaults"`
	Fields   []TuningField `json:"fields"`
}

// TuningField describes one knob so every surface renders it the same way: the
// CLI listing, the God Mode form, and any future client. Keys match the JSON
// tags on Tuning, so a client can PATCH `{key: value}` blindly.
type TuningField struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Help  string  `json:"help"`
	Kind  string  `json:"kind"`  // "number" | "toggle"
	Group string  `json:"group"` // "pulse" | "bubble" | "thread"
	Min   float64 `json:"min,omitempty"`
	Max   float64 `json:"max,omitempty"`
	Step  float64 `json:"step,omitempty"`
}

// TuningFields is the self-describing schema of the buoyancy calibration —
// the single source of truth for how the knobs are presented.
func TuningFields() []TuningField {
	return []TuningField{
		{Key: "cycle_hours", Label: "Cycle length (hours)", Kind: "number", Group: "pulse", Min: 1, Max: 8760, Step: 1,
			Help: "The pulse recency is measured against. Only used when a project has no active Plane cycle — a real cycle always wins."},
		{Key: "dormant_cycles", Label: "Cycles before dormant", Kind: "number", Group: "pulse", Min: 1, Max: 52, Step: 1,
			Help: "How many cycles of silence make something dormant. 2 = \"nothing this cycle or last\". Lower sinks the board faster."},
		{Key: "decay_cycles", Label: "Score decay (cycles)", Kind: "number", Group: "pulse", Min: 0.1, Max: 52, Step: 0.1,
			Help: "Scales the buoyancy score used for ordering. Larger keeps things floating higher for longer; it does not change the bands."},
		{Key: "ownerless_is_dormant", Label: "No owner sinks to dormant", Kind: "toggle", Group: "pulse",
			Help: "Something with nobody accountable goes dormant once it has gone quiet. Anything still producing stays hot either way."},

		{Key: "bubble_level_rollup", Label: "Band = hottest thread", Kind: "toggle", Group: "bubble",
			Help: "A bubble sits in the band of its hottest unfinished thread. Off, it is classified from the union of its evidence instead — which counts a thread's birth as bubble output, so a bubble full of untouched new work items reads 🔥. Explicit close and review always win either way."},
		{Key: "bubble_rip_needs_owner", Label: "Ownerless bubble is RIP", Kind: "toggle", Group: "bubble",
			Help: "A dormant bubble with no owner reads 🪦 instead of 😴. A bubble that never produced anything is 🪦 regardless. Only used when the band is NOT rolled up from threads."},

		{Key: "thread_birth_heats", Label: "Creating a thread heats it", Kind: "toggle", Group: "thread",
			Help: "Off by default: being born is not producing. Turning this on makes every new work item read 🔥 for a whole cycle, even untouched in Backlog. A birth always heats its bubble."},
		{Key: "thread_grace_cycles", Label: "Newborn grace (cycles)", Kind: "number", Group: "thread", Min: 0, Max: 52, Step: 0.5,
			Help: "How long a new thread that has produced nothing stays 😴 before it is called 🪦. 0 = no grace."},
		{Key: "thread_rip_needs_owner", Label: "Unassigned thread is RIP", Kind: "toggle", Group: "thread",
			Help: "A dormant thread with no assignee reads 🪦 instead of 😴."},
		{Key: "pulse_cycles", Label: "Comment pulse (cycles)", Kind: "number", Group: "thread", Min: 0, Max: 52, Step: 0.5,
			Help: "How long a comment keeps a thread out of 🪦. Comments are presence, not production — they never warm a thread to 🔥, but we don't declare something abandoned while people are still discussing it. 0 turns the pulse off."},
		{Key: "thread_terminal_state_wins", Label: "Plane's terminal columns win", Kind: "toggle", Group: "thread",
			Help: "Let Plane decide the two terminal states: completed → 🏆, cancelled → 🪦. Cancelling is still undone by real production (a logbook edit or a revision resurrects the thread) but never by comments. Every other column is ignored either way — moving a card is motion, not evidence."},
	}
}

func clampF(v, lo, hi float64) float64 {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	}
	return v
}

// Buoyancy is a thread's derived lifecycle — the per-thread analogue of a
// bubble's heat, computed against the bubble's cycle window
// (THREAD-LIFECYCLE.md Phase A). Embedded flat into the thread DTOs.
type Buoyancy struct {
	Lifecycle Lifecycle `json:"lifecycle"`
	Level     string    `json:"level"` // in_progress | zzzz | rip | done
	Score     float64   `json:"score"` // 0..1, decayed recency of the last progress
	Reason    string    `json:"reason"`
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
	// AutoState opts this instance into writing derived thread levels back to
	// Plane (THREAD-LIFECYCLE.md Phase B). Off by default — until an operator
	// turns it on, Bubble never changes a card's state.
	AutoState bool
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
	Owner       string // §4 contract: who is accountable now
	Closure     string // §4 contract: the explicit close signal
	Closed      bool
	Stage       string // explicit overlay: "" | "reviewed"
	Threads     []Thread
	Evidence    []EvidenceEvent

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
	ReadOnly     bool     `json:"read_only,omitempty"`     // kiosk display token: reads only, no MCP (§9 Phase 9)
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
	Project     string    `json:"project"`                // Plane project id (our Workspace)
	ProjectName string    `json:"project_name,omitempty"` // human name, when known
	Lifecycle   Lifecycle `json:"lifecycle"`
	Level       string    `json:"level"` // UI band: in_progress|zzzz|rip|reviewed|done
	Score       float64   `json:"score"` // 0..1 buoyancy score, for ordering
	Reason      string    `json:"reason"`
	Outcome     string    `json:"outcome,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Members     []string  `json:"members,omitempty"` // distinct thread assignees + contract owner (Phase 9)
	Threads     int       `json:"threads"`
	// ThreadLevels rolls the bubble's threads up by their own derived level
	// (THREAD-LIFECYCLE.md Phase A) — e.g. {"in_progress":2,"zzzz":1}.
	ThreadLevels map[string]int `json:"thread_levels,omitempty"`
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
	State       string     `json:"state,omitempty"`  // Plane state name (localized)
	StateGroup  string     `json:"state_group,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Buoyancy               // lifecycle/level/score/reason, flattened into the JSON
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
	State       string        `json:"state,omitempty"` // Plane state name (localized)
	StateGroup  string        `json:"state_group,omitempty"`
	Artifacts   []md.Artifact `json:"artifacts"`
	Logbook     *md.Logbook   `json:"logbook,omitempty"`
	Revisions   []md.Artifact `json:"revisions"`
	CreatedAt   time.Time     `json:"created_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Buoyancy                  // lifecycle/level/score/reason, flattened into the JSON
}

// Comment is one rendered entry in a thread's comment feed (Phase 12). HTML is
// the goldmark render for the chat UI; Markdown serves CLI/MCP. Mine marks the
// caller's own messages so the UI can align them.
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	AuthorID  string    `json:"author_id,omitempty"`
	Markdown  string    `json:"markdown"`
	HTML      string    `json:"html"`
	Mine      bool      `json:"mine"`
	Readers   []Reader  `json:"readers,omitempty"` // 👀 read-receipts (server overlay)
	CreatedAt time.Time `json:"created_at"`
}

// Reader is one person who has read a comment (a 👀 read-receipt).
type Reader struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Member is a Plane workspace member, for the admin read-only member viewer.
// Membership is owned by Plane (system of record); Bubble only reads it.
type Member struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  int    `json:"role"`
	Admin bool   `json:"admin"` // Plane workspace admin (role ≥ 20)
}

// InstanceMembers groups an instance's members for the admin viewer.
type InstanceMembers struct {
	Instance string   `json:"instance"` // slug
	Name     string   `json:"name"`     // instance display name
	Members  []Member `json:"members"`
	Error    string   `json:"error,omitempty"` // set if this instance's members couldn't be fetched
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
	Outcome  string `json:"outcome,omitempty"` // optional §4 contract set at creation
	Owner    string `json:"owner,omitempty"`
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
	AutoState  bool   `json:"auto_state"` // writes derived levels back to Plane (Phase B)
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
