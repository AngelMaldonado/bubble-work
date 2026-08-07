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
	// ReasonCode says the same thing structurally so a client can render it in
	// its own language; Reason stays the English sentence the CLI and logs read.
	// The heat model has no business knowing what language anyone reads.
	ReasonCode string            `json:"reason_code,omitempty"`
	ReasonArgs map[string]string `json:"reason_args,omitempty"`
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
	// Projects is the set of "slug:projectID" the actor may see. nil means
	// "not project-scoped" (a kiosk display, or a service admin); an empty map
	// means "scoped, and a member of nothing" — a different fact entirely.
	Projects map[string]bool `json:"-"`
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
// CanSeeProject reports whether the actor may see one project inside an
// instance. Plane scopes membership PER PROJECT — a private project's members
// are a subset of the workspace's — so instance membership answers "may this
// person use this Plane at all", which is a weaker question.
//
// A nil Projects map means the actor is not project-scoped at all: a kiosk
// display minted for an instance, or a service admin. Those see the instance
// they were granted, which is what granting them meant.
func (a Actor) CanSeeProject(slug, projectID string) bool {
	if !a.CanSee(slug) {
		return false
	}
	if a.Projects == nil {
		return true
	}
	return a.Projects[slug+":"+projectID]
}

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
	// see Buoyancy.ReasonCode — a stable key + params so clients can translate.
	ReasonCode string            `json:"reason_code,omitempty"`
	ReasonArgs map[string]string `json:"reason_args,omitempty"`
	Outcome    string            `json:"outcome,omitempty"`
	Owner      string            `json:"owner,omitempty"`
	Members    []string          `json:"members,omitempty"` // distinct thread assignees + contract owner (Phase 9)
	Threads    int               `json:"threads"`
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
	// Regions is what an editor loads and writes back, keyed by region name
	// (docs/ARTIFACT-EDITING.md). Distinct from Artifacts, which is the RENDERED
	// read model: this is the exact markdown a splice will diff against.
	Regions map[string]EditableRegion `json:"regions,omitempty"`
	// Warnings are markdown-standard violations on a write that was allowed
	// through leniently — the editor shows them next to the save status.
	Warnings []md.Finding `json:"warnings,omitempty"`
	Buoyancy              // lifecycle/level/score/reason, flattened into the JSON
}

// EditableRegion is one writable part of a thread's page.
type EditableRegion struct {
	Markdown string `json:"markdown"`
	// Hash fingerprints Markdown. Sending it back with an edit proves the editor
	// is writing over what it read; the server answers 409 when it is not.
	Hash string `json:"hash"`
}

// ThreadEdit is a write to a thread's artifact page. A nil field is left exactly
// as it was — an agent revising a plan must not be able to erase the human's
// Brief, so the shape makes "touch only what you name" the default.
type ThreadEdit struct {
	// Title renames the work item. Not a region: it lives in Plane's `name`,
	// not in the description, and it is not production either — what the work is
	// called is not what has been done.
	Title   *string `json:"title,omitempty"`
	Brief   *string `json:"brief,omitempty"`
	Logbook *string `json:"logbook,omitempty"`
	DoD     *string `json:"dod,omitempty"`
	// Edits change PART of a region instead of replacing it.
	//
	// Replacing a whole region to change one line is expensive and unsafe: a
	// caller that must reproduce a long Logbook to tick one box will paraphrase
	// it, drop a phase, or append to a stale copy. An edit says which text to
	// change and leaves the rest alone — the same idea as the block splice, one
	// level up.
	//
	// Applied BEFORE the whole-region fields, so naming both for one region is
	// rejected rather than silently resolved.
	Edits []RegionEdit `json:"edits,omitempty"`
	// Lenient downgrades a markdown-standard violation from a refusal to a
	// warning. The web editor sets it because autosave that stops mid-sentence
	// is its own kind of broken; nothing else does, so the standard is enforced
	// by default on every other surface — including MCP, where §9.3 makes an
	// agent deliberately indistinguishable from the person it acts for.
	Lenient bool `json:"lenient,omitempty"`
	// Base carries the region hashes the editor read, keyed by region name.
	// Absent means "I did not look" — accepted, because CLI and MCP callers
	// legitimately write without having loaded the page first.
	Base map[string]string `json:"base,omitempty"`
}

// RegionEdit is one find-and-replace inside a region.
type RegionEdit struct {
	Region string `json:"region"`
	// Old must appear exactly once in the region unless All. Empty appends.
	Old string `json:"old"`
	New string `json:"new"`
	All bool   `json:"all,omitempty"`
}

// Empty reports whether an edit names nothing to change.
func (e ThreadEdit) Empty() bool {
	return e.Title == nil && e.Brief == nil && e.Logbook == nil && e.DoD == nil && len(e.Edits) == 0
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

	// Pending marks a comment that has NOT reached Plane — a draft kept after a
	// failed post (docs/PLANE-SYNC.md Phase 5). It carries no credential, so
	// only its author can re-send it, with their live key; that is also the only
	// way Plane records the right author. DraftID addresses it for retry/discard.
	Pending bool   `json:"pending,omitempty"`
	DraftID int64  `json:"draft_id,omitempty"`
	Error   string `json:"error,omitempty"` // why it did not land
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
	// RateBudgets is each instance's live Plane rate-limit state. Plane allows
	// 60 requests/minute per API key and reports the remaining allowance on
	// every response; this is that, surfaced (docs/PLANE-SYNC.md Phase 0).
	RateBudgets []RateBudget `json:"rate_budgets,omitempty"`
}

// RateBudget is one instance's view of its Plane rate limit. Mirrors
// plane.BudgetStat — kept here so domain stays free of the transport package.
type RateBudget struct {
	Instance  string `json:"instance"`
	Known     bool   `json:"known"`     // false until a response carried the headers
	Remaining int    `json:"remaining"` // -1 when unknown
	Limit     int    `json:"limit"`     // inferred ceiling (largest remaining seen)
	ResetIn   int    `json:"reset_in"`  // seconds until the window rolls over
	Throttled int    `json:"throttled"` // 429s observed since start
	Waits     int    `json:"waits"`     // times background work yielded to the floor
	Spent     int    `json:"spent"`     // requests issued since start
	Floor     int    `json:"floor"`     // allowance reserved for interactive work
}

// ---- Plane mirror (docs/PLANE-SYNC.md) ----

// InstanceStatus is one instance's sync freshness.
type InstanceStatus struct {
	Instance      string `json:"instance"`
	Stale         bool   `json:"stale"`
	LastOK        string `json:"last_ok,omitempty"`
	BehindSeconds int    `json:"behind_seconds,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// ServiceStatus tells a client whether what it is showing can be trusted to be
// current (docs/PLANE-SYNC.md Phase 7). Reads come from a local mirror now, so
// a stopped sync would otherwise leave the board rendering confidently from data
// that is quietly getting older. Being behind is fine; being behind silently is
// not.
type ServiceStatus struct {
	Stale        bool             `json:"stale"`
	Reason       string           `json:"reason,omitempty"`
	Instances    []InstanceStatus `json:"instances,omitempty"`
	UnsentDrafts int              `json:"unsent_drafts,omitempty"` // the caller's own
}

// OutboxItem is one write that has not reached Plane.
type OutboxItem struct {
	ID        int64  `json:"id"`
	Instance  string `json:"instance"`
	Kind      string `json:"kind"` // state | comment
	TargetID  string `json:"target_id"`
	Author    string `json:"author,omitempty"` // comment drafts only
	Status    string `json:"status"`           // pending | abandoned
	Attempts  int    `json:"attempts"`
	FieldLock string `json:"field_lock,omitempty"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	NextAt    string `json:"next_at,omitempty"`
	Summary   string `json:"summary"` // pre-rendered, so surfaces agree
}

// OutboxView is the queue of unsent writes. An abandoned entry is the point of
// this: a write that never reached Plane must stay visible, because silently
// dropping one is the only thing an outbox must never do.
type OutboxView struct {
	Pending   int          `json:"pending"`
	Abandoned int          `json:"abandoned"`
	Entries   []OutboxItem `json:"entries,omitempty"`
}

// SyncStatus is one instance's mirror census and cursor. Cheap: no Plane calls.
type SyncStatus struct {
	Instance  string `json:"instance"`
	Projects  int    `json:"projects"`
	Modules   int    `json:"modules"`
	Items     int    `json:"items"`
	States    int    `json:"states"`
	Members   int    `json:"members"`
	Comments  int    `json:"comments"`
	Watermark string `json:"watermark,omitempty"`  // newest updated_at applied
	LastFull  string `json:"last_full,omitempty"`  // last complete reconcile
	LastOK    string `json:"last_ok,omitempty"`    // last successful pass
	LastError string `json:"last_error,omitempty"` // why the mirror may be stale
}

// SyncResult reports what one sync pass did.
type SyncResult struct {
	Instance  string `json:"instance"`
	Full      bool   `json:"full"`
	Projects  int    `json:"projects"`
	Modules   int    `json:"modules"`
	Items     int    `json:"items"`
	Comments  int    `json:"comments"`
	Pruned    int    `json:"pruned"`
	Watermark string `json:"watermark,omitempty"`
	TookMS    int64  `json:"took_ms"`
	// Partial means some projects could not be walked completely (usually a
	// 429). What arrived is kept; prunes and the watermark are held back.
	Partial bool     `json:"partial,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// SyncFinding is one disagreement between the mirror and Plane.
type SyncFinding struct {
	Kind  string `json:"kind"`  // missing-in-mirror | missing-in-plane | field
	Scope string `json:"scope"` // module | membership | item
	ID    string `json:"id"`
	Label string `json:"label"`
	Field string `json:"field,omitempty"`
	Plane string `json:"plane,omitempty"`
	Local string `json:"local,omitempty"`
	Text  string `json:"text"` // pre-rendered one-liner, so surfaces agree
}

// SyncDiff is the Phase 1 acceptance gate: the mirror compared against a live
// fetch. Expensive — it runs the very calls the mirror exists to remove.
type SyncDiff struct {
	Instance  string        `json:"instance"`
	Projects  int           `json:"projects"`
	Modules   int           `json:"modules"`
	Items     int           `json:"items"`
	Clean     bool          `json:"clean"`
	Findings  []SyncFinding `json:"findings,omitempty"`
	Watermark string        `json:"watermark,omitempty"`
	LastFull  string        `json:"last_full,omitempty"`
	LastOK    string        `json:"last_ok,omitempty"`
	LastError string        `json:"last_error,omitempty"`
	TookMS    int64         `json:"took_ms"`
}

// SyncFidelity measures what a read→write round trip would do to an instance's
// bodies (docs/ARTIFACT-EDITING.md Phase 0). Cheap — it reads the mirror and
// costs no Plane calls — so it can be re-run after every change to the markdown
// bridge rather than measured once and asserted forever.
type SyncFidelity struct {
	Instance string `json:"instance"`
	Bodies   int    `json:"bodies"` // bodies with enough content to be worth checking
	Stable   int    `json:"stable"` // survive a round trip unchanged
	// SpliceClean counts bodies where every region splices its own markdown
	// back to identical bytes. This is the gate an editor depends on.
	SpliceClean int `json:"splice_clean"`
	Mentions    int `json:"mentions"` // <mention-component> nodes a write would delete
	Assets      int `json:"assets"`   // <image-component> nodes a write would break
	// Threads that would be damaged, most-damaged first, capped for readability.
	Unstable []FidelityIssue `json:"unstable,omitempty"`
	Elided   int             `json:"elided,omitempty"` // unstable threads not listed
	TookMS   int64           `json:"took_ms"`
}

// FidelityIssue is one thread whose body does not survive a round trip.
type FidelityIssue struct {
	ThreadID string `json:"thread_id"`
	Title    string `json:"title"`
	Line     int    `json:"line"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Mentions int    `json:"mentions,omitempty"`
	Assets   int    `json:"assets,omitempty"`
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
