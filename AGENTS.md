# Bubble Work — Agent Operating Rules

You are working inside the Bubble Work repository. The canonical model is the
[README](./README.md) — read it before non-trivial work. How the repo is built is
[`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md), and what each part does today is
the matching file in [`docs/modules/`](./docs/modules) — that is where a change
belongs, not in the journals ([`docs/README.md`](./docs/README.md) is the map).
This file is the operational summary both Claude Code and Codex load.

## Vocabulary (keep the metaphor human-facing)

- **Workspace** — the boundary for a body of work. Maps to a Plane **Project**.
  Not a Plane "Workspace".
- **Bubble** — a durable grouping of related work; the unit of *attention*.
  Maps to a Plane **Module**. It has an outcome and can die.
- **Cycle** — the repeating pulse (heat window) recency is measured against.
- **Thread** — one executable unit of work. Maps to a Plane **work item**.
- **Heat** — evidence of *changed reality*, not activity.

## Threads carry a document, not a template

A thread needs a **name**. Nothing about the shape of its document is enforced
([`docs/decisions/0005`](./docs/decisions/0005-the-framework-does-not-own-your-format.md)):
write it the way the work actually has shape. The framework hosts the writing and
measures what has gone cold — that is the whole trade.

Worth writing anyway, and said (never refused) by `create_thread` and `audit_bubble`:
why this exists, what is true when it is done, the next concrete action, and
checkboxes for the steps. Checkboxes count **anywhere** in the document; ticking one
is the smallest evidence a thread can produce. Outside a Logbook only real `- [ ]`
boxes count — a prose bullet is a sentence.

What KIND of work a thread is, where its evidence lives, and what it depends on are
**Plane's** questions, not the prose's: use labels (`set_labels`), links
(`add_link` — landing one is production) and relations (`relate_threads`:
relates_to / duplicate / blocking / blocked_by). There is no thread `type` and no
Brief template ([`docs/decisions/0006`](./docs/decisions/0006-plane-holds-the-relationships.md)).

Two headings carry machinery and are offered, not demanded:

- **`## Definition of Done`** — its open items are reported when you complete the
  thread, and tracked by `audit_bubble`. Write items as verifiable facts, not tasks —
  nothing enforces it, it is just what makes the section answer its own question.
- **`## Logbook`** — its own addressable region, so the plan can be rewritten without
  touching the rest of the page.

The one shape rule left is machinery: write each reserved heading (`## Logbook`,
`## Definition of Done`) **at most once**. A duplicate truncates the first section,
leaving that region empty and unreachable — that write is refused.

## Heat = evidence, never motion

Only meaningful outputs warm a bubble: a thread being **created**, a completed todo
(anywhere in the document), **any edit to a thread's document** — writing is the work
— a **link** to external evidence, a committed/reviewed implementation, a published
deliverable, a recorded decision, validated feedback, or the removal of a material
blocker.

Comments, status pings, renames, labelling, relating and the mere *creation* of a
work item in Plane do NOT generate heat. A recent comment only stops a bubble being called 🪦 — it never warms
it ([`docs/decisions/0004`](./docs/decisions/0004-writing-is-the-work.md)). Never
manufacture activity to keep a bubble warm — revive it with real work, redefine it,
or close it.

## Lifecycle

Hot → Warm → Cooling → Dormant → Closed, computed against the Cycle. Recommend
closing or redefining a bubble when its outcome no longer justifies the work.

## Architecture (how this repo is built)

- One Go binary, two modes: `bubble serve` (the brain) and the thin client.
- **Plane is a channel, not the record.** It owns structure (projects, modules,
  cycles, states), identity, and comments. The **server** owns heat, lifecycle, the
  Bubble contract, the explicit stage, pages, and the **artifact bodies** — markdown
  in `thread_docs`, published outward to Plane
  ([`docs/decisions/0001`](./docs/decisions/0001-plane-is-a-channel-not-the-record.md)).
  Plane stays writable: a body edited there is imported and wins, unless one of our
  publications is still queued. Who the members are still comes from Plane.
- **Never add `thread_docs`, `thread_publish` or `local_pages` to a prune path.**
  Those rows are the only copy of the writing; everything else in the overlay is
  re-derivable ([`docs/modules/storage.md`](./docs/modules/storage.md)).
- The server is Plane's ONLY client, over REST only. No component talks to
  Plane directly or via Plane's MCP. Plane's 60 req/min budget is the binding
  constraint, so **every client read is served from a local SQLite mirror** and a
  single sync worker is Plane's only reader; writes land locally and drain
  through an outbox. Never add a Plane call to a read path — see
  [`docs/journal/PLANE-SYNC.md`](./docs/journal/PLANE-SYNC.md).
- Agents consume the server's OWN MCP endpoint (`/mcp`), authenticated as a
  member. The framework's rules are enforced server-side, so they cannot be
  bypassed from any client.

## Surface parity (default)

A server capability lands on **all three client surfaces in the same change**:
the REST API, the CLI thin client, and the MCP tools. The logic lives in a
server method that each surface reuses (CLI over HTTP, MCP calling the method
directly), so they never drift. The web UI follows when the capability has a
visual form. Only skip a surface when the capability is inherently specific to
one (e.g. mds rendering is UI-only) — and say so.

## Working protocol

Before: read the thread and its Bubble, and confirm what the work is for and what
finishing means. If neither is written down, write what you understood or ask.
During: update the plan when it materially changes; record blockers and decisions;
link concrete evidence (commits, PRs, deliverables).
After: update todos, link artifacts, record the next concrete action, and change
state only when it reflects reality.

Documenting a change to this repo: the normative description goes in the module
file under [`docs/modules/`](./docs/modules); a change to the *model* gets a
numbered file in [`docs/decisions/`](./docs/decisions); the journals in
[`docs/journal/`](./docs/journal) are history and are not updated.
