# Thread Lifecycle — automatic buoyancy for work items

Status: **Phase A shipped · Phase B/C pending** · Drafted 2026-08-03 · Companion
to [`AGENTS.md`](./AGENTS.md) and [`INTERIOR-PLAN.md`](./INTERIOR-PLAN.md).

## Why

Bubbles already have an automatic lifecycle: heat (evidence of *changed reality*)
is classified against the **Cycle** into Hot → Warm → Cooling → Dormant, and the
board renders that as `in_progress / reviewed / zzzz / rip / done`. Threads
(Plane work items) do not — today the server collapses a work item to a **binary
`Active = completed_at == nil`** and never writes state. Plane's own
backlog/todo/in-progress/done pipeline is managed entirely in Plane.

This document specifies making **threads buoyant too**, using the same evidence
model, so a work item automatically sleeps, dies, and *resurrects* — and
optionally reflects that back onto Plane.

## Core idea: push buoyancy down, roll it up

Compute `heat.Classify` **per thread** from that work item's own evidence stream
against the Cycle, then make a **bubble's state the roll-up of its threads'**.
A bubble is Zzzz because its open threads went cold; RIP because they were
abandoned. Nothing "cascades onto" work items top-down — the bubble's state *is*
the emergent state of its threads. (`EvidenceEvent` already carries a
`ThreadID`, so evidence is largely thread-attributable already.)

### Thread levels

| Level | Meaning | Trigger |
|-------|---------|---------|
| `in_progress` 🔥 | changed reality this cycle | recent progress evidence |
| `zzzz` 😴 | lived, then went quiet | had evidence, none lately |
| `rip` 🪦 | born but never produced / abandoned | past dormant, no evidence ever or no assignee |
| `done` 🏆 | complete | Plane `completed_at` / `completed` group |

Reuse the existing `bubbleLevel` logic verbatim at the work-item grain (including
the "never got going vs went quiet" distinction).

## Evidence: two signals

Keep the anti-gaming principle from `AGENTS.md` ("heat = evidence, never
motion"), but split touch into two kinds:

- **Progress = heat → resurrects to 🔥.** Real changed reality: `completed_at`
  flips, a logbook todo gets checked, a revision artifact (sub-issue) is added, a
  commit/PR is linked, a decision is recorded. LLM agents produce most of these
  via MCP (updating the logbook, adding revisions). **Only progress wakes a
  thread.**
- **Pulse = presence → holds 😴, blocks 🪦, never wakes.** Plain comments
  (including LLM-posted). A recent comment means someone's paying attention, so
  it keeps a thread from being declared abandoned — but it is **not** progress
  and never marks a thread 🔥. This is the agreed role for comments.

## Resurrection

Because everything is computed from **evidence recency vs the Cycle**, the moment
progress evidence lands the recency window resets → the thread recomputes to
`in_progress` → the bubble floats back up. No manual "reopen." Resurrection is
automatic and free.

## Auto-write to Plane (opt-in)

**Policy: per-instance toggle, OFF by default.** When on, the existing cooling
`tick` sweep also moves a thread's Plane state:

| Computed | Plane **group** (resolve by group, never by name) |
|----------|---------------------------------------------------|
| Zzzz 😴 | `backlog` |
| RIP 🪦 | `cancelled` |
| resurrect (fixed active lane) | `started` (In Progress) |
| done | left alone (`completed` / `completed_at`) |

- **Resolve targets by group.** State names are localized and there can be more
  than one state per group. On this instance the groups resolve to: `backlog`
  → "Backlog", `unstarted` → "Todo", `started` → "En Progreso", `completed` →
  "Pruebas"/"Finalizado", `cancelled` → "Cancelado". Pick the (default) state of
  the target group per project, the way `DefaultState` already does.
- **Resurrect to a fixed active lane** (`started`), not the thread's prior lane.
- **Provenance guardrail — never fight a human.** Record per thread the state
  *we* set. Before any auto-move, if the thread's current Plane state ≠ what we
  last left it as, a human (or agent) took over → **back off permanently** for
  that thread (drop our record, stop auto-managing it). Only act on threads we've
  never touched or whose state still matches our last write.
- **Attribution + audit.** Auto-writes have no human caller → they use the
  instance key, are logged, and post an inbox notification (e.g. "3 dormant
  threads parked to Backlog"). Auto-writes are housekeeping, **not heat** — they
  never warm anything.

## Plane capability (verified 2026-08-03)

Unlike relations and comment-reactions (both 404 on this instance), work-item
**state writes work**:

- `GET  …/projects/{p}/states/` returns all five groups (backlog, unstarted,
  started, completed×2, cancelled).
- `PATCH …/work-items/{id}/` with `{"state": "<stateId>"}` → **200**.

So auto-write is buildable here. If a future instance lacks it, the feature must
degrade gracefully to overlay-only (compute + display, no writes).

## Phasing

- **Phase A — compute + show (no writes, safe).** ✅ **Shipped.** Attribute
  evidence per thread, run the classifier per thread, roll up to the bubble, and
  render a thread level chip in the timeline + interior. Read each thread's real
  Plane group for display. Zero risk; independently useful. See *Phase A as
  built* below.
- **Phase B — auto-write policy (behind the per-instance toggle).** Extend the
  cooling `tick`: Zzzz→Backlog, RIP→Cancelled, resurrect→In Progress, provenance
  guardrail, group resolution, inbox notifications. Depends on A.
- **Phase C — evidence sharpening.** Logbook-todo diffing + revision-added
  detection for resurrection; comment-pulse tracking (hold 😴 / block 🪦).
  Some of this is needed by A/B and will be pulled forward as required.

## Phase A as built

One classifier, two grains. `heat.WindowFor` resolves the heat window once (Plane
cycle when present, rolling otherwise) and a private `classify` applies the
identical rule to a bubble (`heat.Classify`) and to a single thread
(`heat.ClassifyThread`), so thread and bubble temperature are commensurable by
construction. `heat.Attribute` buckets a bubble's evidence by `ThreadID`;
evidence with no thread is dropped rather than smeared across threads.

- **Server.** `threadBuoyancy(bubble)` is the one place per-thread lifecycle is
  derived — the timeline, the interior and the roll-up all read it.
- **Birth does not heat a thread.** A new work item is a real output for the
  *bubble* that gained it, but the item itself has produced nothing by existing,
  so `ClassifyThread` classifies from **progress evidence only**
  (`EvidenceEvent.Progress()`; kinds are the constants `domain.EvThreadCreated` /
  `EvCompletedTodo`). Without this, every work item created in the current cycle
  read 🔥 for a full cycle while sitting untouched in Backlog. A thread born
  inside the current cycle with nothing produced yet is 😴 *"born this cycle;
  nothing produced yet"* — it gets the cycle before we call it 🪦.
- **Evidence decides the band; Plane's column does not.** Dragging a card into
  "In Progress" is motion, not evidence (`AGENTS.md`), so `started` never by
  itself makes a thread 🔥, and a thread sitting in Backlog whose todos are
  actually getting ticked genuinely is 🔥. `threadLevel` consults the state group
  for the two **terminal** columns only — `completed` → 🏆 and `cancelled` → 🪦 —
  because those are statements of fact, not status theatre.
- **Plane state, read-only.** `plane.ListStates` + a per-project
  `statesCache` (30 min) resolve a work item's state uuid → `{name, group}`.
  Threads carry both; **only `group` is ever matched on**, `name` is display-only
  (it is localized). `module-issues` already returns the state uuid, so the
  snapshot pays one extra cached call per project, not per thread.
- **Surfaces (parity).** `ThreadNode` and `ThreadDetail` embed
  `domain.Buoyancy` (`lifecycle`/`level`/`score`/`reason`) flattened into their
  JSON, plus `state`/`state_group` — so REST, MCP (`thread_timeline`,
  `read_thread`) and the CLI all get it from the same structs. CLI: `bubble show`
  marks each row with the thread's level icon and a Plane-state column, `bubble
  thread` prints a `buoyancy:` and `plane:` line, `bubble heat` appends the
  roll-up. Web: a level chip per timeline row, a level + Plane-state chip in the
  thread header, and a tally on the bubble hover card.
- **Freshness.** Buoyancy is time-dependent, so it is never baked into the
  60s-cached thread body — `ThreadDetail` derives it per request from the
  snapshot. A thread outside the snapshot (a revision sub-issue, or one the
  refresher hasn't picked up) is classified standalone against the rolling
  window.
- **Roll-up.** `BubbleView.thread_levels` is a histogram of its threads' levels
  (`{"in_progress":2,"zzzz":1}`).

**Deliberately NOT done in A:** the bubble's own band is still computed from the
union of its evidence, not as `max(thread levels)`. The two agree in the common
case; they diverge when a bubble's only recent evidence belongs to a thread that
is already closed (union says Warm, roll-up says colder). Switching the band to
the roll-up changes which bubbles appear in which band on a live board, so it is
a one-line change held for an explicit decision rather than smuggled in with the
display work.

## Decisions locked

- Buoyancy is computed **per thread** and **rolls up** to the bubble.
- A thread's **birth does not heat it** (it heats its bubble); only progress does.
- **Evidence leads, not the Plane column.** The state group short-circuits only
  `completed` and `cancelled`.
- **Progress resurrects; comments are pulse** (hold/block, never wake).
- Auto-write is **opt-in per instance**, off by default.
- Guardrail is **provenance** (hands-off the moment a human changes the state).
- Resurrection restores to a **fixed active lane** (In Progress), not the prior
  state.
- Plane targets are resolved **by group**, and auto-writes are **not heat**.

## Open / deferred

- Exact thresholds (how many dormant cycles before Backlog vs Cancel) — likely
  driven by the computed level itself (Zzzz vs RIP) plus the pulse check, rather
  than new counters.
- Commit/PR linkage as progress evidence (needs description-link parsing).
- Whether a structured "decision" comment should ever count as progress (kept as
  pulse for now).
- Whether a bubble's band should become `max(thread levels)` outright (see
  *Phase A as built*) — needs a look at a live board before flipping.
