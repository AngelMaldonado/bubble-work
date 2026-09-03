# Bubble Work — Agent Operating Rules

You are working inside the Bubble Work repository. **There is no implementation
right now.** v0 is archived at the tag `v0-plane-as-record`; what is here is the
model, the decisions that produced it, and the record of what was built.

The canonical model is the [README](./README.md) — read it before non-trivial
work. [`docs/README.md`](./docs/README.md) is the map, and says what is current,
what is frozen, and what is history. This file is the operational summary both
Claude Code and Codex load.

## Vocabulary (keep the metaphor human-facing)

- **Workspace** — the boundary for a body of work. Not a Plane "Workspace".
- **Bubble** — a durable grouping of related work; the unit of *attention*.
  It has an outcome and can die.
- **Cycle** — the repeating pulse (heat window) recency is measured against.
- **Thread** — one executable unit of work.
- **Heat** — evidence of *changed reality*, not activity.

## Threads carry a document, not a template

A thread needs a **name**. Nothing about the shape of its document is enforced
([`docs/decisions/0005`](./docs/decisions/0005-the-framework-does-not-own-your-format.md)):
write it the way the work actually has shape. The framework hosts the writing and
measures what has gone cold — that is the whole trade.

Worth writing anyway, and said (never refused): why this exists, what is true when
it is done, the next concrete action, and checkboxes for the steps. Checkboxes
count **anywhere** in the document; ticking one is the smallest evidence a thread
can produce. Outside a Logbook only real `- [ ]` boxes count — a prose bullet is a
sentence.

What KIND of work a thread is, where its evidence lives, and what it depends on
are answered by labels, links (landing one is production) and relations
(relates_to / duplicate / blocking / blocked_by) — never by a template. There is
no thread `type` and no Brief template
([`docs/decisions/0006`](./docs/decisions/0006-plane-holds-the-relationships.md)).

Two headings carry machinery and are offered, not demanded:

- **`## Definition of Done`** — its open items are reported when a thread is
  completed, and tracked by an audit. Write items as verifiable facts, not tasks —
  nothing enforces it, it is just what makes the section answer its own question.
- **`## Logbook`** — its own addressable region, so the plan can be rewritten
  without touching the rest of the page.

The one shape rule left is machinery: write each reserved heading (`## Logbook`,
`## Definition of Done`) **at most once**. A duplicate truncates the first section,
leaving that region empty and unreachable — that write is refused.

## Heat = evidence, never motion

Only meaningful outputs warm a bubble: a thread being **created**, a completed
todo (anywhere in the document), **any edit to a thread's document** — writing is
the work — a **link** to external evidence, a committed/reviewed implementation, a
published deliverable, a recorded decision, validated feedback, or the removal of
a material blocker.

Comments, status pings, renames, labelling and relating do NOT generate heat. A
recent comment only stops a bubble being called 🪦 — it never warms it
([`docs/decisions/0004`](./docs/decisions/0004-writing-is-the-work.md)). Never
manufacture activity to keep a bubble warm — revive it with real work, redefine
it, or close it.

## Lifecycle

Hot → Warm → Cooling → Dormant → Closed, computed against the Cycle. Recommend
closing or redefining a bubble when its outcome no longer justifies the work.

## Where the repository stands

**v0 is archived.** In it, Plane held the record: structure, identity, cycles and
states lived upstream, every overlay row was keyed by a Plane id, and every client
read was served from a fourteen-table local mirror of Plane's shape. That is the
limit that ended it — Plane's shape reached into every key in the database, so
Plane could never become optional.

Recover any of it from the tag:

```
git show v0-plane-as-record:internal/md/splice.go
git checkout v0-plane-as-record -- internal/heat
```

**v1 is being designed, not built.** The direction, not yet ratified as a
decision:

- **Bubble owns the record** — workspaces, bubbles, threads, states, cycles,
  assignment, labels, relations, comments and people. Plane becomes one channel
  among several, optional per workspace.
- **Locally-minted ids**, with an external-reference table mapping to each
  channel. This is the piece that unblocks the rest: it makes moving work between
  projects and deployments a change of mapping rather than a loss.
- **Evidence is recorded, not inferred.** v0 diffed hashes and counters to guess
  what had happened, because the writes happened somewhere else. When Bubble owns
  the write path, every production event is observed as it occurs — so an
  append-only event log becomes the natural shape, and heat stays the same pure
  function over a real stream.
- **MCP is the surface.** The CLI is not being rebuilt. Three things that lived in
  it are not user-facing and must survive the move: `export` (one of the three
  legs of the backup story), bootstrap (something must mint the first person and
  token before MCP can authenticate anything), and the operational commands.
- **Versioned migrations, first.** v0 had `CREATE TABLE IF NOT EXISTS` plus a list
  of `ALTER TABLE`s whose errors were discarded, and no schema-version row. That
  holds only while every change is additive. Nothing planned here is.

Two live deployments still hold a v0 `bubble.db`, and `thread_docs` and
`local_pages` in them are the only copy in existence of what was written there.
[`docs/DATABASE.md`](./docs/DATABASE.md) is the source document for migrating
them. Do not design a schema that cannot read that one.

## Working protocol

Before: read the thread and its Bubble, and confirm what the work is for and what
finishing means. If neither is written down, write what you understood or ask.
During: update the plan when it materially changes; record blockers and decisions;
link concrete evidence (commits, PRs, deliverables).
After: update todos, link artifacts, record the next concrete action, and change
state only when it reflects reality.

Documenting a change to this repo: a change to the *model* gets a numbered file in
[`docs/decisions/`](./docs/decisions). A capability's normative description goes in
`docs/modules/` — that directory is empty because nothing is built, and the first
file is written with the first capability, not before it. The journals in
[`docs/journal/`](./docs/journal) are history and are not updated;
[`docs/journal/v0/`](./docs/journal/v0) is where the archived implementation's own
documentation went.
