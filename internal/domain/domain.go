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

// Evidence kinds (§5.1).
//
// CREATED and BORN are different events, and only one of them is evidence
// (docs/decisions/0002). A work item that merely appeared — someone made one in
// Plane — has produced nothing by existing, and stays weak. A thread CREATED here
// is different: somebody defined a piece of work, which is production before any
// code is touched.
const (
	EvThreadCreated   = "thread-created"   // the work item came into being (weak — not production)
	EvThreadBorn      = "thread-born"      // somebody defined a piece of work here (docs/decisions/0005)
	EvCompletedTodo   = "completed-todo"   // a logbook/DoD item got ticked
	EvLogbookUpdated  = "logbook-updated"  // the plan itself changed (§ working protocol)
	EvBodyUpdated     = "body-updated"     // the document changed outside the plan (docs/decisions/0004)
	EvRevisionAdded   = "revision-added"   // a revision artifact (sub-item) landed
	EvLinkAdded       = "link-added"       // external evidence was published (docs/decisions/0006)
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
// itself. Merely being CREATED is not producing, and presence is not producing.
// Being BORN is: the birth rule cannot be satisfied without writing the two
// artifacts (docs/decisions/0002). Only progress can resurrect a thread.
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

	// (There was a ThreadBirthHeats knob here. It asked whether a work item's own
	// CREATION should heat it, and the answer was always no. What replaced it is not
	// tunable: a thread that passed the birth rule emits EvThreadBorn, which is
	// production, while a work item that merely appeared emits EvThreadCreated,
	// which is not — see docs/decisions/0002.)

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
	ID    string `json:"id"`
	Seq   int    `json:"seq"`
	Title string `json:"title"`
	Kind  string `json:"kind"` // "simple" | "phased" — the SHAPE of the page
	// Labels are Plane's own labels on the work item. They replace the overlay
	// "thread type" we used to keep (docs/decisions/0006): categorising work is
	// something the tracker already does, and doing it a second time here only
	// created a second answer.
	Labels      []Label       `json:"labels,omitempty"`
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
	// (docs/journal/ARTIFACT-EDITING.md). Distinct from Artifacts, which is the RENDERED
	// read model: this is the exact markdown a splice will diff against.
	Regions map[string]EditableRegion `json:"regions,omitempty"`
	// Warnings are markdown-standard violations on a write that was allowed
	// through leniently — the editor shows them next to the save status.
	Warnings []md.Finding `json:"warnings,omitempty"`
	// FromPlane says the body being shown was last written in PLANE's editor and
	// imported (docs/decisions/0001). Not a warning — Plane is a writable surface
	// and this is normal operation — but a reader deserves to know, because the
	// import runs through the HTML bridge and can therefore have lost detail the
	// author put there.
	FromPlane  bool       `json:"from_plane,omitempty"`
	ImportedAt *time.Time `json:"imported_at,omitempty"`
	// Links are the external evidence hung off the work item in Plane: a commit, a
	// PR, a published deliverable (docs/decisions/0006). Publishing one is
	// production, which is why they are worth reading here rather than being left
	// to Plane's UI.
	Links []Link `json:"links,omitempty"`
	// Related are Plane's typed relationships to other threads.
	Related []Related `json:"related,omitempty"`
	// UnmetDoD is set when a thread was just COMPLETED with Definition of Done items
	// still unticked. It is a report, not a refusal (docs/decisions/0005): finishing
	// is the author's call, and this is what they closed it over.
	UnmetDoD []string `json:"unmet_dod,omitempty"`
	Buoyancy          // lifecycle/level/score/reason, flattened into the JSON
}

// Label is one of Plane's labels on a work item.
//
// Plane already has a vocabulary for "what kind of work is this" — labels, work item
// types, priorities — and we spent an overlay table answering the same question a
// second way (docs/decisions/0006). Labels come back embedded in the work item, so
// reading them costs nothing on top of the sync we already run.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// Link is external evidence attached to a thread: the commit, the PR, the thing
// that shipped. Held by Plane, mirrored here.
type Link struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Related is one typed edge to another thread. Type is Plane's own vocabulary —
// relates_to, duplicate, blocking, blocked_by, start_before/after,
// finish_before/after — and Title/State are filled in from the mirror when the
// other end is a thread we know about.
type Related struct {
	ID    string `json:"id"` // namespaced thread id, so a client can open it
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	State string `json:"state,omitempty"`
	Level string `json:"level,omitempty"`
}

// EditableRegion is one writable part of a thread's page.
type EditableRegion struct {
	Markdown string `json:"markdown"`
	// Hash fingerprints Markdown. Sending it back with an edit proves the editor
	// is writing over what it read; the server answers 409 when it is not.
	Hash string `json:"hash"`
}

// SectionEdit writes ONE named section of a thread's document, addressed by its
// heading rather than by quoting its text.
//
// A thread page is one document (ParseThread does not split it), so a "new work
// artifact" inside it is a new `## ` section — and until now there was no way to
// name one. You could replace the whole document, or quote a fragment and splice
// around it; neither is "add a section" or "rewrite that section".
type SectionEdit struct {
	// Title is the heading text, matched case-insensitively. Created as an H2
	// when absent, which is what the standard wants (§3.1) and what the table of
	// contents can actually see — it starts at H2.
	Title string `json:"title"`
	// Markdown is the section's new content, WITHOUT its heading.
	Markdown *string `json:"markdown,omitempty"`
	// Delete removes the section, heading and all.
	Delete bool `json:"delete,omitempty"`
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

	// Sections add, rewrite or remove named sections of the document. Applied
	// before any whole-region write, and refused alongside one: naming both the
	// document and a section inside it is a contradiction.
	Sections []SectionEdit `json:"sections,omitempty"`
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
	return e.Title == nil && e.Brief == nil && e.Logbook == nil && e.DoD == nil &&
		len(e.Edits) == 0 && len(e.Sections) == 0
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
	// failed post (docs/journal/PLANE-SYNC.md Phase 5). It carries no credential, so
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

// BirthRequest asks for a new thread. The only thing it must carry is a NAME
// (docs/decisions/0005): the framework hosts the writing, it does not dictate its
// shape — and since docs/decisions/0006 it does not read the writing either.
type BirthRequest struct {
	BubbleID string `json:"bubble_id"`
	Name     string `json:"name"`
	// Body is the thread's document, in whatever shape its author wants. Written
	// verbatim: no headings are added around it.
	Body string `json:"body,omitempty"`
	// Brief and Logbook are the older two-field shape. Brief is appended to the
	// document as written — no `## Brief` heading is added any more, because Brief
	// is no longer a thing the server knows about — and Logbook keeps its heading,
	// because that section is still an addressable region.
	Brief   string `json:"brief,omitempty"`
	Logbook string `json:"logbook,omitempty"`
	// SmallThread is kept so old payloads still parse. Nothing reads it.
	SmallThread bool `json:"small_thread,omitempty"`
}

// BirthResult is returned once a thread exists. Message carries whatever the
// server wants to SAY about the document — a missing finish line, a Logbook with no
// todo — none of which stopped the thread being created.
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

// Page is one project-level document: reference material, a product spec,
// anything that outlives a single thread.
//
// Pages are NOT threads and carry no buoyancy. Heat is evidence that a body of
// work moved, and a document exists to be read — writing one is not the same as
// producing the outcome a bubble is contracted to reach. So a page has no level,
// no cycle and no bearing on the board.
type Page struct {
	// ID is namespaced "<instance>:<project>:<page>", like every other id here.
	ID       string `json:"id"`
	Title    string `json:"title"`
	Instance string `json:"instance"`
	Project  string `json:"project"`
	// Locked pages are read-only in Plane, and stay read-only here.
	Locked    bool      `json:"locked"`
	Archived  bool      `json:"archived"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
	// Storage says which system is the record for this page: "plane" when Plane
	// holds it, "local" when this server does because the instance's Plane has no
	// pages API (docs/journal/PAGES-CAPABILITY.md). A reader deserves to know which of the
	// two they are editing, since only one of them is visible in Plane's own UI.
	Storage string `json:"storage"`
	// Parent is the namespaced id of the page this one sits under, or "" at the
	// root. Plane already models this and its own UI shows pages as a tree, so the
	// hierarchy is read from it rather than invented here.
	Parent string `json:"parent,omitempty"`
}

// PageStorage values for Page.Storage.
const (
	PageInPlane = "plane"
	PageInLocal = "local"
)

// PageList is a workspace's pages plus the one fact a reader needs to interpret
// them: whether Plane is holding them at all.
//
// An envelope rather than a bare array, because the answer "there are no pages"
// and "this Plane cannot store pages" look identical in a list and call for
// completely different reactions. It is also the shape MCP needs — a tool that
// returns a bare array announces `"type": "array"` and clients reject the whole
// tool list over it.
type PageList struct {
	Pages []Page `json:"pages"`
	// PlaneHoldsPages is false when this instance's Plane does not serve pages on
	// its public API, so pages created here are recorded by this server instead.
	PlaneHoldsPages bool `json:"plane_holds_pages"`
}

// PageDetail is a page with its body, as markdown and as rendered HTML.
//
// Both, for the same reason a thread artifact carries both: the editor writes
// markdown, and the reader wants the rendered document without shipping a
// markdown engine to the browser.
type PageDetail struct {
	Page
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	// Hash fingerprints Markdown, so an editor can prove it is writing over what
	// it read — the same 409 contract a thread artifact uses.
	Hash string `json:"hash"`
}

// PageEdit writes to a page. A nil field is left exactly as it was.
type PageEdit struct {
	Title    *string `json:"title,omitempty"`
	Markdown *string `json:"markdown,omitempty"`
	// BaseHash is the hash the editor last read. When set and stale the write is
	// refused rather than silently overwriting somebody else.
	BaseHash string `json:"base_hash,omitempty"`
}

// CreatePageRequest adds a page to a workspace.
type CreatePageRequest struct {
	// Workspace is namespaced "<instance>:<project>".
	Workspace string `json:"workspace"`
	Title     string `json:"title"`
	Markdown  string `json:"markdown"`
	// Parent nests the new page under an existing one. Empty puts it at the root.
	// The namespaced id, as every other reference here is.
	Parent string `json:"parent,omitempty"`
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
	// every response; this is that, surfaced (docs/journal/PLANE-SYNC.md Phase 0).
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

// ---- Plane mirror (docs/journal/PLANE-SYNC.md) ----

// InstanceStatus is one instance's sync freshness.
type InstanceStatus struct {
	Instance      string `json:"instance"`
	Stale         bool   `json:"stale"`
	LastOK        string `json:"last_ok,omitempty"`
	BehindSeconds int    `json:"behind_seconds,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// ServiceStatus tells a client whether what it is showing can be trusted to be
// current (docs/journal/PLANE-SYNC.md Phase 7). Reads come from a local mirror now, so
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

// TodoToggle is one checklist item to tick or un-tick, addressed the way read_thread
// reports it: per-region index, base 0, with the item's text as a guard.
type TodoToggle struct {
	Region string `json:"region,omitempty"` // "document" | "logbook" | "dod"; empty means logbook
	Index  int    `json:"index"`
	Text   string `json:"text,omitempty"` // guards the position; refuses if it moved
	Done   bool   `json:"done"`
}

// TodoResult is the explicit confirmation of a toggle: what is PERSISTED now, plus
// the two numbers a caller was going to re-read the whole thread for anyway.
//
// It exists because returning the full interior technically answered "did it land?"
// and practically did not: the answer was buried in a large object, so an agent
// re-read the thread to be sure. This says it in one place.
type TodoResult struct {
	ThreadID string `json:"thread_id"`
	// Applied is each item as it now stands, read back from the stored page rather
	// than echoed from the request.
	Applied []md.Todo `json:"applied"`
	// Open/Done counts per region, after the write. The document has its own pair
	// because a checklist written under somebody's own heading is a plan too
	// (docs/decisions/0005).
	LogbookOpen  int `json:"logbook_open"`
	LogbookDone  int `json:"logbook_done"`
	DoDOpen      int `json:"dod_open"`
	DoDDone      int `json:"dod_done"`
	DocumentOpen int `json:"document_open,omitempty"`
	DocumentDone int `json:"document_done,omitempty"`
	// Unmet is the Definition of Done's outstanding items, when the thread has one.
	// It no longer blocks anything — completing is the author's call
	// (docs/decisions/0005) — it is what they check that call against.
	Unmet []string `json:"unmet,omitempty"`
	// Level is the thread's derived level after the write — a ticked todo is
	// production, so this is where a caller sees the heat it just produced.
	Level  string `json:"level"`
	Reason string `json:"reason,omitempty"`
}

// LogbookTotal and DoDTotal are the sums a caller would otherwise compute.
func (r TodoResult) LogbookTotal() int { return r.LogbookDone + r.LogbookOpen }
func (r TodoResult) DoDTotal() int     { return r.DoDDone + r.DoDOpen }

// NewTodoResult reads the confirmation back out of the thread as it now stands.
//
// `want` is what the caller asked for; every value reported here is looked up in the
// STORED page instead of being echoed, so "done: true" in the answer means the page
// says so — which is the whole point of the confirmation.
func NewTodoResult(d ThreadDetail, want []TodoToggle) TodoResult {
	res := TodoResult{ThreadID: d.ID, Level: d.Level, Reason: d.Reason}
	byRegion := map[string][]md.Todo{}
	for _, a := range d.Artifacts {
		if a.ID != "" { // a revision is its own work item, not this page
			continue
		}
		byRegion["document"] = append(byRegion["document"], a.Todos...)
		for _, td := range a.Todos {
			if td.Done {
				res.DocumentDone++
			} else {
				res.DocumentOpen++
			}
		}
	}
	byRegion["brief"] = byRegion["document"] // the API's older word for it
	if d.Logbook != nil {
		byRegion["logbook"] = d.Logbook.Todos
		byRegion["dod"] = d.Logbook.DoD
		for _, td := range d.Logbook.Todos {
			if td.Done {
				res.LogbookDone++
			} else {
				res.LogbookOpen++
			}
		}
		for _, td := range d.Logbook.DoD {
			if td.Done {
				res.DoDDone++
			} else {
				res.DoDOpen++
				res.Unmet = append(res.Unmet, td.Text)
			}
		}
	}
	for _, w := range want {
		region := w.Region
		if region == "" {
			region = "logbook"
		}
		list := byRegion[region]
		if w.Index >= 0 && w.Index < len(list) {
			res.Applied = append(res.Applied, list[w.Index])
		}
	}
	return res
}

// ThreadAudit is one thread as the framework's own questions see it: is it born,
// what is its finish line, how far along is it, and what happens next.
type ThreadAudit struct {
	ID     string   `json:"id"`
	Seq    int      `json:"seq"`
	Title  string   `json:"title"`
	Labels []string `json:"labels,omitempty"` // Plane's labels on the work item
	Owner  string   `json:"owner,omitempty"`

	Level      string `json:"level"` // in_progress | zzzz | rip | done
	Reason     string `json:"reason,omitempty"`
	State      string `json:"state,omitempty"` // Plane's own column (localized)
	StateGroup string `json:"state_group,omitempty"`

	// HasBrief says the thread has a DOCUMENT at all — anything that is not the
	// Logbook or the DoD. The field name predates docs/decisions/0006 and is kept so
	// clients do not break; there is no Brief any more, only writing.
	HasBrief   bool `json:"has_brief"`
	HasDoD     bool `json:"has_dod"`
	HasLogbook bool `json:"has_logbook"`

	DoDDone    int      `json:"dod_done"`
	DoDTotal   int      `json:"dod_total"`
	TodosDone  int      `json:"todos_done"`
	TodosTotal int      `json:"todos_total"`
	Unmet      []string `json:"unmet,omitempty"` // Definition of Done items still open

	// Next is the Logbook's declared next concrete action, parsed from its status
	// line. Empty means nobody wrote one, which is itself the finding.
	Next string `json:"next,omitempty"`
	// Missing names the ONE thing that would most improve this thread's legibility —
	// a document, a finish line, something to tick, a next action. An OBSERVATION,
	// never a violation (docs/decisions/0005): none of it was ever required.
	Missing        string     `json:"missing,omitempty"`
	LastProgressAt *time.Time `json:"last_progress_at,omitempty"`
}

// BubbleAudit answers "what is the state of this bubble?" in one call, from local
// data only — no Plane traffic, so it costs the same as showing the board.
type BubbleAudit struct {
	BubbleID    string `json:"bubble_id"`
	Name        string `json:"name"`
	Instance    string `json:"instance"`
	ProjectName string `json:"project_name,omitempty"`

	Level     string `json:"level"`
	Lifecycle string `json:"lifecycle"`
	Reason    string `json:"reason,omitempty"`

	Outcome string `json:"outcome,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Closure string `json:"closure,omitempty"`
	Closed  bool   `json:"closed,omitempty"`

	Threads []ThreadAudit  `json:"threads"`
	Counts  map[string]int `json:"counts"` // threads by level
	// NeedsRepair counts threads with a Missing observation. A bubble can be warm and
	// still hold work nobody has defined a finish line for.
	NeedsRepair int `json:"needs_repair"`
}

// ExportResult is what one `bubble admin export` wrote (docs/decisions/0001).
//
// The counts are the point: a backup you cannot check is a hope. Threads and
// Files differing tells you how many threads had nothing worth writing, and Pages
// covers the ones held HERE — the copies that exist nowhere else.
type ExportResult struct {
	Instance   string    `json:"instance"`
	Dir        string    `json:"dir"`
	At         time.Time `json:"at"`
	Workspaces int       `json:"workspaces"`
	Bubbles    int       `json:"bubbles"`
	Threads    int       `json:"threads"`
	Revisions  int       `json:"revisions"`
	Pages      int       `json:"pages"`
	Files      int       `json:"files"`
}

// AdoptResult is what one `bubble admin adopt` did (docs/decisions/0001).
//
// Skipped is threads that already had a stored document — re-running an adoption
// must never overwrite writing done since the last one.
type AdoptResult struct {
	Instance string    `json:"instance"`
	At       time.Time `json:"at"`
	Threads  int       `json:"threads"`
	Adopted  int       `json:"adopted"`
	Skipped  int       `json:"skipped"`
	Empty    int       `json:"empty"`
}

// SyncFidelity measures what a read→write round trip would do to an instance's
// bodies (docs/journal/ARTIFACT-EDITING.md Phase 0). Cheap — it reads the mirror and
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
