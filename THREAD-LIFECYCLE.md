# Thread Lifecycle — automatic buoyancy for work items

Status: **design agreed, not yet built** · Drafted 2026-08-03 · Companion to
[`AGENTS.md`](./AGENTS.md) and [`INTERIOR-PLAN.md`](./INTERIOR-PLAN.md).

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

- **Phase A — compute + show (no writes, safe).** Attribute evidence per thread,
  run `heat.Classify` per thread, roll up to the bubble, and render a thread
  level chip in the timeline + interior. Read each thread's real Plane group for
  display. Zero risk; independently useful. First reviewable slice.
- **Phase B — auto-write policy (behind the per-instance toggle).** Extend the
  cooling `tick`: Zzzz→Backlog, RIP→Cancelled, resurrect→In Progress, provenance
  guardrail, group resolution, inbox notifications. Depends on A.
- **Phase C — evidence sharpening.** Logbook-todo diffing + revision-added
  detection for resurrection; comment-pulse tracking (hold 😴 / block 🪦).
  Some of this is needed by A/B and will be pulled forward as required.

## Decisions locked

- Buoyancy is computed **per thread** and **rolls up** to the bubble.
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
