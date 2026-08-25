# 0003 — A Brief has a type, and the type keeps it short

Status: **superseded by [`0006`](./0006-plane-holds-the-relationships.md)** ·
2026-08-15 · The typed Brief and the thread `type` were removed; Plane's labels
answer that question now. Kept as written — a decision is superseded, not edited.

## Context

The Brief has one template for every kind of work:

```markdown
## Problem / opportunity
## Intended outcome
## Definition of Done
```

That is what `BirthForm.svelte` writes, and the spec adds `## Context &
constraints` and `## Links to source material` on top. It is short, which is
right, but it is also the same shape whether the thread is a one-line fix or a
subsystem rewrite — so it under-serves the big ones and over-asks of the small
ones.

The obvious move is to borrow a ticket template from a larger organisation. Those
templates are long for a specific reason: **one template has to serve every kind
of work**, so it accumulates fields until it covers the worst case, and every
ticket pays for the worst case.

## Decision

**The type picks the template.** Three types, each adding at most two sections to
a common spine, and a hard ceiling of five `##` sections.

### The spine — always

```markdown
## Brief                    ← added by birth_thread; callers must not write it
### Context                 ← why now, one paragraph (absorbs constraints)
### Outcome                 ← what is true when this is done
## Definition of Done       ← verifiable checkboxes (required by the birth rule)
### Links                   ← optional
```

> **Corrected after the first real agent test.** The parts were originally specified
> at `##`, which made them siblings of the `## Brief` that birth adds: the interior
> rendered an empty Brief followed by loose sections, and `ExtractSection(body,
> "brief")` returned nothing — quietly disabling every lint rule that measures a
> Brief. The levels now live in `domain.BriefSections` so the CLI, the web form and the
> MCP prompt cannot disagree about them again, and `brief-empty` warns when a page has
> the old shape.

### The type inserts

| Type | Adds | Where |
|---|---|---|
| `bug` | `### Symptom`, `### Repro` | before Outcome |
| `feature` | `### Scope` (in / out) | after Outcome |
| `chore` | nothing — usually a small thread, Brief only | — |
| *(none)* | nothing — the bare spine is legal | — |

The type is **server-owned overlay state** (`thread_meta`), exposed as `type` on the
thread DTO and set at birth.

> **Deviation from the plan, recorded rather than buried.** This was specified as a
> **Plane label**, so that Plane could filter on it too. `internal/plane` had no
> label support at all, and the probe that was supposed to establish whether the
> public API can write labels could not run: the Plane host is reachable only from
> the server's own network, not from the machine this was built on. Shipping an
> unverified write path to Plane is worse than owning one small field, so the type
> lives here and reflecting it outward as a label is a follow-up. The plan's own
> fallback branch said exactly this.

The template the type implies is ours either way — `domain.BriefSections` is the one
place it is written down, so the CLI (`bubble birth --template --type bug`), the web
form and an agent's prompt cannot drift into three different shapes.

`Scope`, with an explicit *out* list, is the highest-value section per line spent —
it is where scope creep dies, and it costs two lines.

### What is deliberately not borrowed

| Ticket field | Why not |
|---|---|
| Status / column | The level is derived from evidence. A manual column competes with it. |
| Priority | Buoyancy *is* the ordering. A priority field is the first step toward faking heat. |
| Estimate / story points | Invites gaming, and the useful part already exists as the binary `--small`. |
| Epic link | That is the Bubble. |
| Spike / research type | Considered and dropped: for a two-person team it is ceremony. Research is a thread whose Definition of Done is *"the answer is written in X"* — the spine already expresses that, and a recorded decision already earns heat. |

### The Logbook gets a status line

```markdown
## Logbook
**Owner:** @amald · **State:** building · **Next:** measure the push against the live instance

### Phase 1 — thread_docs
- [x] schema + migration
- [ ] reads served from the store

### Decisions
2026-08-15 — an edit made in Plane is imported and wins; the outbox lock arbitrates
```

`Next` is the addition that earns its place: the working protocol already demands
*"record the next concrete action"*, and until now there was nowhere to put it, so
it dissolved into prose. It is the first thing anyone picking up a cooled thread
needs.

`Decisions` is append-only, and it absorbs the urge to comment: a recorded decision
is heat, a comment is not.

It starts as **convention** — bold labels in markdown — not a parsed field. Parsing
markdown people already write is cheap; migrating a field into prose is not.

### The Definition of Done is facts, not tasks

**Every item must be verifiable by someone else without asking you.**

```markdown
❌ - [ ] implement the export command
✅ - [ ] `bubble admin export` writes BRIEF.md and LOGBOOK.md per thread, and a re-import round-trips identically
```

This is mechanical, not stylistic: `unmetDoD` already blocks completion on unticked
items. If the DoD is a task list it duplicates the Logbook, and the gate becomes
noise people force through.

## Consequences

**The linter gained a severity.** Every rule it had was a refusal, which is why
`md.Finding` had no such field. The advisory rules — Brief over ~25 lines, a DoD
outside 3–10 items or opening with a task verb, a Logbook with no `Next:`, more than
five phases — carry `SevWarn`, and `md.Refusals` is what the write paths gate on.
Warnings ride back on a write that SUCCEEDED, in the `warnings` field the editor
already showed.

The subtle part was the callers, not the rules: three sites in `pages.go` and one in
`artifacts.go` refused on `len(findings) > 0`, so adding a warning would have started
refusing writes with the message `bad request: <nil>`. A test caught it.

`NewFindings` already judges a write on what it broke rather than what it
inherited, so existing threads do not become uneditable.

**Surfaces that gained a type:** `BirthForm.svelte` (a type row, and the fields
change with it), `bubble birth --type` plus `--template` to print the skeleton,
`birth_thread` (which takes `domain.BirthRequest` directly, so its schema followed
for free), and the thread DTO's `type` — shown as a chip in the interior. Reflecting
it into Plane as a label is the one piece that did not land; see above.

**The old shape stays valid.** `Problem / opportunity` is `Context` renamed, and
nothing rejects the old heading. Threads already born are not migrated.

## Alternatives rejected

| Alternative | Why not |
|---|---|
| One richer template for everything | The reason corporate tickets are long. |
| Types configurable per workspace | A template editor, plus mediocre defaults, for a two-person team. Fixed types can be *good*. |
| Parse the status line as a field from day one | Cheap to add later once people write it; expensive to unpick if the shape is wrong. |
| Keep `spike` as a fourth type | Ceremony at this size — see above. |
