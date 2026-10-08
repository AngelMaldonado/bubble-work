# Bubble Work — Agent Operating Rules

You are working inside the Bubble Work repository: one Go binary — PocketBase
embedded as a framework — that holds the record, writes markdown to disk under
git, serves a Svelte board and planner, and speaks MCP to agents.

The canonical model is the [README](./README.md) — read it before non-trivial
work. [`PLAN.md`](./PLAN.md) is everything else: what was built, what was decided
and against what, and what is still open. This file is the operational summary
both Claude Code and Codex load.

## Vocabulary (keep the metaphor human-facing)

- **Workspace** — the boundary for a body of work. Not a Plane "Workspace".
- **Bubble** — a durable grouping of related work; the unit of *attention*.
  It has an outcome and can die.
- **Cycle** — the repeating pulse (heat window) recency is measured against.
- **Thread** — one executable unit of work.
- **Heat** — evidence of *changed reality*, not activity.

## Threads carry a document, not a template

A thread needs a **name**. Nothing about the shape of its document is enforced:
write it the way the work actually has shape. The framework hosts the writing and
measures what has gone cold — that is the whole trade.

Worth writing anyway, and said (never refused): why this exists, what is true when
it is done, the next concrete action, and checkboxes for the steps. Checkboxes
count **anywhere** in the document; ticking one is the smallest evidence a thread
can produce. Only real `- [ ]` boxes count — a prose bullet is a sentence.

What KIND of work a thread is, where its evidence lives, and what it depends on
are answered by labels, links (landing one is production) and relations
(relates_to / duplicate / blocking / blocked_by) — never by a template. There is
no thread `type` and no Brief template.

Two headings carry machinery and are offered, not demanded:

- **`## Definition of Done`** — its open items are reported when a thread is
  completed. Write items as verifiable facts, not tasks.
- **`## Logbook`** — its own addressable region, so the plan can be rewritten
  without touching the rest of the page.

Write each reserved heading **at most once**. A duplicate truncates the first
section, leaving that region empty and unreachable — that write is refused.

## Heat = evidence, never motion

Heat is **observed**, not inferred: every write goes through the server, which
appends to `events`. Four kinds warm:

| kind | warms |
|---|---|
| `document-changed` | any write under `threads/` — writing is the work |
| `thread-created` | defining a piece of work is production |
| `thread-completed` | reaching a state in the `completed` group |
| `link-added` | proof reality changed outside this tool |

`doc-changed` (the wiki, `docs/**` and `README.md`) is recorded and never warms. A
**comment is pulse**: it holds 😴 and blocks 🪦, never wakes anything. Renames,
labels, relations, presence and a byte-identical save warm nothing — and a test
pins presence to that. Never manufacture activity to keep a bubble warm — revive
it with real work, redefine it, or close it.

`heat`, `priority` and `lifecycle` are **never stored**: they are pure functions of
stored rows plus the current time. Priority is derived from `impact` × `urgency`;
do not add a column that lets somebody write the verdict over its inputs.

## Lifecycle

Hot 🔥 → Dormant 😴 → Rip 🪦 → Closed 🏆, computed against the Cycle. Four
bands, each naming a different action: leave it alone / somebody to ask / a
decision to make / read it once. Rip is Dormant with nobody accountable, applied
at bubble grain. A bubble takes the band of its hottest OPEN thread. Recommend
closing or redefining a bubble when its outcome no longer justifies the work.

## Where the repository stands

V2 is built and released (the series starts at `v1.0.0`, cut by CI from
Conventional Commits on `main`). Phases 0–5 of [`PLAN.md`](./PLAN.md) are
built; what is left is **Phase 6** (reading the v0 databases) and the "Later" and
"Open" sections. Sections still marked "Hecho cuando" / "Done when" without
*(built)* are not validated yet — do not describe them as shipped.

| Where | What |
|---|---|
| `main.go`, `cmd_*.go` | the binary: PocketBase's `serve`, plus `boot` (self-updating launcher), `person`, and `POST /api/update` |
| `migrations/` | versioned PocketBase migrations — the schema and every API rule |
| `internal/bubble` | the operations (`ops.go`) both REST and MCP call — one function, two doors |
| `internal/mcpapi` | the MCP tools on `/mcp` |
| `internal/tree` | the markdown store: path → entity, per-path lock, one git commit per write |
| `internal/md` | markdown regions, checkboxes, surgical edits |
| `internal/heat` | the classifier; `now` is a parameter |
| `internal/channels` | inbox channels as compiled plugins (WhatsApp first; `-tags nowhatsapp`) |
| `internal/boot`, `internal/release` | launch, health check, rollback, download + SHA256 |
| `prompts/` | `guide`, `house_rules`, `connect` — embedded, served by MCP and HTTP |
| `frontend/` → `web/dist` | Svelte 5 + Tailwind 4 + Skeleton, built with bun; **the bundle is committed** |
| `deploy/` | systemd, launchd and Coolify |

A workspace on disk is `README.md`, `threads/`, `docs/` and `assets/`; any other
path is refused. Details in PLAN.md → "On disk".

**v0 is archived** at the tag `v0-plane-as-record`. Recover anything from it:

```
git show v0-plane-as-record:internal/md/splice.go
git show 2cf0c71:docs/DATABASE.md        # the v0 schema, table by table
git show 2cf0c71:docs/decisions/         # decisions 0001-0007, as written
```

**The live v0 `bubble.db` files are still the only copy** of what was written in
`thread_docs` and `local_pages`. A local copy of reko's sits in `v0-reko/`, which
is gitignored and must never be committed. Do not change the model in a way that
makes Phase 6 unable to read them.

## Commands

`just` is the surface; every recipe is a thin call into `scripts/`.

- `just dev` — build and run everything in a tmux session; `just stop` ends it.
  Go has no hot reload: after a `.go` change, `just dev` again.
- `just check` — tidy, gofmt, vet, build, test. What CI and the pre-commit hook
  (`just hooks`) gate.
- `just test` — Go tests plus the live suite against a throwaway server. API rules
  are proved there, not assumed.
- `just bundle` — rebuild `web/dist`. A UI change is invisible until it runs, so
  the bundle goes in the same commit as the source.
- `just migrate …`, `just person <email> <password> [lead|member]`,
  `just release-preview`.

## Working protocol

The work on this repository is tracked in Bubble Work itself, workspace
`bubble-work` (see [`CLAUDE.md`](./CLAUDE.md)).

Before: read the thread and its Bubble, and confirm what the work is for and what
finishing means. If neither is written down, write what you understood or ask.
During: update the thread's document as the plan changes; record blockers and
decisions; link concrete evidence (commits, PRs, deliverables).
After: tick todos, link artifacts, record the next concrete action, and complete a
thread only when it is really done.

Documenting a change to this repo: the model lives in [`README.md`](./README.md)
and everything else in [`PLAN.md`](./PLAN.md). A choice that changes the model is
recorded in PLAN.md **with what it rejected** — that is what lets a reader tell a
choice from an accident. A document that describes an intention as if it shipped is
worse than no document, so a section says "done when" until it is validated, and
only then *(built)*.
