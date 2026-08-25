# Threads — levels, evidence, finishing

**Status:** built (Phases A, B, C) · **Packages:** `internal/server`
(`progress.go`, `autostate.go`, `complete.go`, `interior.go`), `internal/heat` ·
**Depends on:** [`heat.md`](./heat.md), [`artifacts.md`](./artifacts.md) ·
**History:** [`journal/THREAD-LIFECYCLE.md`](../journal/THREAD-LIFECYCLE.md)

## What it solves

A thread is the unit of execution, and its state should be a *consequence* of what
it produced rather than a column someone remembered to drag. Plane's
backlog/todo/in-progress/done pipeline is status theatre; this module replaces it
with a derived level, and writes back to Plane only when an operator asks.

## Model (normative)

### Levels

| Level | Meaning | Trigger |
|---|---|---|
| `in_progress` 🔥 | changed reality this cycle | recent progress evidence |
| `zzzz` 😴 | lived, then went quiet | had evidence, none lately — or newborn inside its grace |
| `rip` 🪦 | never got going, or abandoned | past dormant with no evidence ever, or unassigned |
| `done` 🏆 | complete | a `completed`-group state, or `completed_at` set |

Plus one **explicit stage overlay**, `reviewed` 👀, which a human sets and the
model never infers.

Terminal Plane columns are statements of fact and are honoured when
`thread_terminal_state_wins` is on: `completed` → 🏆, `cancelled` → 🪦. Every other
column is ignored. `cancelled` is still overridden by real production — a thread
someone is demonstrably working on is not dead because of a column.

**States are resolved by GROUP, never by name.** State names are per-project and
localized; groups (`backlog`, `unstarted`, `started`, `completed`, `cancelled`) are
Plane's own vocabulary.

### The roll-up

A bubble's band is the band of its **hottest unfinished thread**
(`bubble_level_rollup`, on by default). Nothing cascades downward: the bubble's
state *is* the emergent state of its threads. A bubble is 😴 because its open
threads went quiet, 🪦 because they were abandoned.

The alternative — classifying the union of a bubble's evidence — counts each
thread's creation as bubble output, so a bubble full of untouched items reads 🔥. The
roll-up reports what its threads are actually doing.

### Evidence, and two signals

- **Progress → heat → resurrects to 🔥.** A todo ticked *anywhere in the document*,
  the body materially changed, a revision landed, completion reached. Agents produce
  most of these through MCP.
- **Pulse → presence.** A plain comment. It holds a thread out of the grave for
  `pulse_cycles` and never warms it.

Resurrection is free: because everything is computed from evidence recency against
the cycle, the moment progress lands the thread recomputes to 🔥 and its bubble
floats up. There is no "reopen" for level.

### Creation

`POST /api/threads/birth` (MCP `create_thread`, aliased `birth_thread`) asks for a
**name** and nothing else
([`decisions/0005`](../decisions/0005-the-framework-does-not-own-your-format.md)).
`body` is the thread's document, markdown, written verbatim — no headings are added
and no shape is imposed. `brief`/`logbook` remain for the older two-field
assembly — `brief` is now appended as written, because `## Brief` is not a heading the
server knows any more ([`decisions/0006`](../decisions/0006-plane-holds-the-relationships.md)).

This used to be a policy gate: a Brief containing a `## Definition of Done` with real
checklist items, plus a Logbook unless the thread was declared small. What the gate
wanted was right and what it did was wrong — a refused call does not produce a better
document, it produces a document somewhere the tool cannot measure. `threadAdvice`
now **says** what is missing (no document, no finish line, nothing tickable) in the
result's `message`, having created the thread regardless.

Creation is **progress** ([`decisions/0002`](../decisions/0002-birth-is-production.md)),
so a new thread is 🔥 from its first moment and its bubble rises through the
roll-up. The mechanism is a durable `born_at` on `thread_progress`, written by
`CreateThread` and emitted as `thread-born` by the progress sweep — **including on the
pass that first sees the thread**. That sweep otherwise baselines a first-seen thread
silently, and correctly: it cannot know when a body it has never seen was last
touched.

A work item that merely appeared in Plane has no `born_at` and earns nothing. Threads
that predate the column are deliberately not reheated — synthesising a birth from
`created_at` would make the board lie about old work.

### Plane's relationships

What KIND of work a thread is, where its evidence lives and what it depends on are
read from Plane, not from the prose
([`decisions/0006`](../decisions/0006-plane-holds-the-relationships.md)):

- **labels** replaced the overlay thread `type` (and the `thread_meta` table). Set by
  NAME; an unknown name is created. Classification, so it warms nothing.
- **links** replaced the hand-written `### Links` section. Adding one is PRODUCTION —
  `thread_progress.links` is diffed by the sweep exactly like the revision count, and
  emits `link-added`.
- **relations** (`relates_to`, `duplicate`, `blocking`, `blocked_by`) replaced
  blockers described in a Logbook sentence. Both ends must be in the same workspace.

All three are written to Plane and written THROUGH to the mirror, so no read path
gained a Plane call. The sync's `edges` fill-in (8 items per pass) exists only to
notice edges added in Plane's own UI.

### Finishing

Finishing is explicit and separate from level: a thread is done when someone says
it is done, or when Plane's state says so.

- `CompleteThread` moves the thread to the project's default **`completed`**-group
  state; `ReopenThread` moves it back to `started`.
- **The Definition of Done reports, it does not gate.** Completing with unticked DoD
  items lands, and the answer carries them in `unmet_dod`
  ([`decisions/0005`](../decisions/0005-the-framework-does-not-own-your-format.md)):
  finishing is a position somebody takes, and a checklist written days ago is
  evidence rather than a warden. `force` is accepted and does nothing.
- It is idempotent: completing an already-completed thread returns the thread.
- On failure to reach Plane the write queues in the outbox with a lock on `state`,
  so a later sync cannot resurrect the old column.
- Completing **hands off autostate** for that thread: once a human has taken a
  position, the sweep stops managing it.

### Autostate (opt-in, off by default)

Per instance, an operator may let derived levels write back into Plane's columns:

| Level | Plane state group |
|---|---|
| `in_progress` | `started` |
| `zzzz` | `backlog` |
| `rip` | `cancelled` |
| `done` / `reviewed` | *hands off* — a human decided |

Off until `bubble admin autostate <inst> on`. Nothing writes to Plane's columns
before that.

## Surfaces

| Capability | REST | CLI | MCP |
|---|---|---|---|
| create | `POST /api/threads/birth` | `bubble new [--body\|--body-file]` | `create_thread` (alias `birth_thread`) |
| label | `POST /api/threads/{id}/labels` | `bubble label` | `set_labels` |
| link evidence | `POST /api/threads/{id}/links` | `bubble link` | `add_link` |
| unlink | `DELETE /api/threads/{id}/links/{link}` | `bubble link rm` | `remove_link` |
| relate | `POST /api/threads/{id}/relations` | `bubble relate` | `relate_threads` |
| unrelate | `DELETE /api/threads/{id}/relations/{other}` | `bubble relate rm` | `unrelate_threads` |
| read interior | `GET /api/threads/{id}` | `bubble thread` | `read_thread` |
| bubble timeline | `GET /api/bubbles/{id}/threads` | `bubble show` | `thread_timeline` |
| tick a todo | `POST /api/threads/{id}/todo` | `bubble todo` | `toggle_todo` |
| re-home | `POST /api/threads/{id}/move` | `bubble move` | `move_thread` |
| finish | `POST /api/threads/{id}/complete` | `bubble done` | `complete_thread` |
| reopen | `POST /api/threads/{id}/reopen` | `bubble reopen` | `reopen_thread` |
| review stage | `POST /api/bubbles/{id}/review` | `bubble bubble review` | — |
| audit a bubble | `GET /api/bubbles/{id}/audit` | `bubble audit` | `audit_bubble` |

Web: the thread interior shows the level, and finishing is a button in the edit
bar.

## Storage

`thread_progress` (progress timestamps, the logbook and body fingerprints, the
revision and link counts, and `born_at`), `thread_pulse` (last comment),
`thread_autostate` (hand-off marks). All overlay — [`storage.md`](./storage.md).
Labels, links and relations are mirrored from Plane rather than owned here
(`mirror_labels`, `mirror_item_links`, `mirror_item_relations`).

## Invariants

1. A level is derived. The only stored levels are Plane's own columns, which we
   read as facts and otherwise ignore.
2. Only progress wakes a thread; only a human closes one.
3. The DoD gate is server-side, so no surface can skip it.
4. Autostate never touches a thread a human has finished or reviewed.

## Open

- Finishing is only reachable from the thread interior in the web. The bubble's
  timeline lists threads with their level and would be the natural place to close
  one without opening it.
- A cold server answers the first request with "not authorized for this instance"
  when it means "project membership is not mirrored yet". It should be a 503.
- The audit reports the ONE most important missing artifact per thread, not all of
  them. Enough to act on, but a thread missing two things only says so once.
- `threadBuoyancyFor`'s standalone path (a thread the refresher has not put in a
  bubble yet) has to read `born_at` from the overlay itself. It works, but it is a
  second place that knows how birth becomes evidence.
