# Bubble Work — Agent Operating Rules

You are working inside the Bubble Work repository. **There is no implementation
right now.** v0 is archived at the tag `v0-plane-as-record`; what is here is the
model, the decisions that produced it, and the record of what was built.

The canonical model is the [README](./README.md) — read it before non-trivial
work. [`PLAN.md`](./PLAN.md) is everything else: what is being built, what was
decided and against what, and in what order. This file is the operational summary
both Claude Code and Codex load.

## Vocabulary (keep the metaphor human-facing)

- **Workspace** — the boundary for a body of work. Not a Plane "Workspace".
- **Bubble** — a durable grouping of related work; the unit of *attention*.
  It has an outcome and can die.
- **Cycle** — the repeating pulse (heat window) recency is measured against.
- **Thread** — one executable unit of work.
- **Heat** — evidence of *changed reality*, not activity.

## Threads carry a document, not a template

A thread needs a **name**. Nothing about the shape of its document is enforced
([`PLAN.md`](./PLAN.md)):
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
no thread `type` and no Brief template.

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
recent comment only stops a bubble being called 🪦 — it never warms it. Never
manufacture activity to keep a bubble warm — revive it with real work, redefine
it, or close it.

## Lifecycle

Hot → Warm → Cooling → Dormant → Closed, computed against the Cycle. Recommend
closing or redefining a bubble when its outcome no longer justifies the work.

## Where the repository stands

**v0 is archived** at the tag `v0-plane-as-record`, and nothing has been built to
replace it. Recover anything from the tag:

```
git show v0-plane-as-record:internal/md/splice.go
git checkout v0-plane-as-record -- internal/heat
git show 2cf0c71:docs/DATABASE.md        # the v0 schema, table by table
git show 2cf0c71:docs/decisions/         # decisions 0001-0007, as written
```

V2 is decided and not built: PocketBase embedded as a Go framework, markdown on
disk under git, a file-shaped MCP whose server supplies belonging and concurrency,
and priority derived the way heat is. The reasoning, the collections, the on-disk
layout and the phase order are all in [`PLAN.md`](./PLAN.md) — read it before
writing code.

**Two live deployments still hold a v0 `bubble.db`**, and `thread_docs` and
`local_pages` in them are the only copy in existence of what was written there.
Exporting them comes before Phase 0. Do not design a schema that cannot read that
one.

## Working protocol

Before: read the thread and its Bubble, and confirm what the work is for and what
finishing means. If neither is written down, write what you understood or ask.
During: update the plan when it materially changes; record blockers and decisions;
link concrete evidence (commits, PRs, deliverables).
After: update todos, link artifacts, record the next concrete action, and change
state only when it reflects reality.

Documenting a change to this repo: the model lives in [`README.md`](./README.md)
and everything else in [`PLAN.md`](./PLAN.md). A choice that changes the model is
recorded in PLAN.md **with what it rejected** — that is what lets a reader tell a
choice from an accident. A document that describes an intention as if it shipped is
worse than no document, so a phase says "done when", never "done".
