# Thread Lifecycle — automatic buoyancy for work items

Status: **Phase A + C shipped · Phase B pending** · Drafted 2026-08-03 ·
Companion to [`AGENTS.md`](./AGENTS.md) and
[`INTERIOR-PLAN.md`](./INTERIOR-PLAN.md).

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
- **Phase C — evidence sharpening.** ✅ **Shipped** (pulled ahead of B, which
  needs it — see *Phase C as built*). Logbook-todo diffing and revision-added
  detection, so an OPEN thread can produce evidence at all. Comment-pulse
  tracking (hold 😴 / block 🪦) is still outstanding.

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

### Calibration (the knobs)

Every threshold the two grains use is a field on `domain.Tuning`, persisted in
`server_settings` and editable at runtime by a service admin. The defaults
reproduce the behaviour described in this document exactly; nothing about the
model changes unless someone turns a knob.

| Key | Grain | What moves |
|-----|-------|-----------|
| `cycle_hours` | both | the rolling pulse, used only when a project has no active Plane cycle |
| `dormant_cycles` | both | cycles of silence before Dormant (2 = "not this cycle or last") |
| `decay_cycles` | both | scales the buoyancy score used for ordering (not the bands) |
| `ownerless_is_dormant` | both | nobody accountable → sinks once quiet |
| `bubble_rip_needs_owner` | bubble | dormant + ownerless → 🪦 instead of 😴 |
| `thread_birth_heats` | thread | whether a work item's creation heats the item itself |
| `thread_grace_cycles` | thread | how long a newborn that produced nothing stays 😴 before 🪦 |
| `thread_rip_needs_owner` | thread | dormant + unassigned → 🪦 instead of 😴 |
| `thread_terminal_state_wins` | thread | Plane's `completed`/`cancelled` columns override the computed level |

`domain.TuningFields()` publishes the label, help text, kind and range for each
knob, so the **CLI listing and the God Mode form render from the same schema**
and cannot drift. Reads go through `Server.Tuning()`, and because everything is
derived at read time, an edit lands on the very next read — no recompute, no
cache flush, no migration. Values are clamped by `Tuning.Sanitize()` before they
are stored, so a bad edit can't produce a nonsensical board.

Surfaces: `GET|PUT /api/admin/tuning` (service admin), `bubble admin tuning
[set k=v… | reset]`, and the **Buoyancy calibration** card in `/god-mode`. Not on
MCP — like the other admin capabilities (instances, members, kiosk), calibrating
the model is an operator action, not an agent one. A PUT body is decoded ONTO the
live values, so a partial patch only changes the keys it names.

**Deliberately NOT done in A:** the bubble's own band is still computed from the
union of its evidence, not as `max(thread levels)`. The two agree in the common
case; they diverge when a bubble's only recent evidence belongs to a thread that
is already closed (union says Warm, roll-up says colder). Switching the band to
the roll-up changes which bubbles appear in which band on a live board, so it is
a one-line change held for an explicit decision rather than smuggled in with the
display work.

## Phase C as built

Phase A shipped the classifier but nothing fed it: the only progress evidence
was derived from Plane's `completed_at`, which exists only once a thread is
already closed. Every *open* thread therefore had an empty progress stream and
could never be 🔥. Phase C is what makes an open thread able to produce.

**Plane has no "a todo got ticked" event, so we diff.** Once per project per
sweep, `ListProjectItems` pages every work item (one call, carrying each body and
parent link). For each thread we count what it has produced — ticked Logbook/DoD
items via `md.CountDone`, and revision sub-items via how many items name it as
parent — and compare against the last observation in the new `thread_progress`
table. The moment a counter goes up is stamped.

**The stamp is the durable part.** Heat is derived from that timestamp on every
read, so one tick keeps the thread warm for a whole cycle rather than for the
single refresh that noticed it. The evidence is re-emitted at the stored time on
every subsequent sweep; it survives restarts because it lives in SQLite.

Two rules keep this honest:

- **First sighting baselines silently.** We have no idea *when* a pre-existing
  thread's todos were ticked, and stamping them "now" would fabricate heat for
  work that may be a year old. So the first sweep after this ships records
  counters and emits nothing — the board warms up as real work happens, not on
  deploy.
- **Counters going down is not evidence.** Unticking a todo or deleting a
  revision moves the counter but never the timestamp. History isn't rewritten,
  and heat can't be manufactured by toggling a checkbox back and forth.

Mechanics: the diff also *writes*, so it runs on the per-project goroutine —
one writer per project, never inside the module fan-out. A failed listing logs
and yields no evidence (threads simply don't warm, the safe direction) rather
than dropping the project from the board. Evidence kinds are now explicit:
`thread-created` (weak, never heats the thread itself), `completed-todo`,
`revision-added`, `thread-completed`. The per-project lookups moved into a
`projectCtx` struct so `buildBubble` takes one argument instead of eight.

**Still outstanding from C:** comment pulse (hold 😴, block 🪦, never wake).
Comments are fetched per thread on demand, so tracking them in a sweep would be
one Plane call per thread. It needs either a cheaper feed or a narrower trigger —
worth resolving before Phase B relies on the pulse check to avoid cancelling a
thread people are actively discussing.

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

- Exact thresholds (how many dormant cycles before Backlog vs Cancel) — driven
  by the computed level itself (Zzzz vs RIP) plus the pulse check, now that the
  thresholds behind those levels are admin-tunable (see *Calibration*).
- Whether the calibration should ever be **per instance** rather than
  server-wide. Today one calibration governs every federated instance; a
  workspace on a two-week cadence and one on a daily cadence would want their
  own. Deferred until a second rhythm actually exists.
- Commit/PR linkage as progress evidence (needs description-link parsing).
- Comment pulse — see the end of *Phase C as built*. A prerequisite for B.
- `thread_progress` rows are never pruned, so a deleted work item leaves one
  behind. Harmless (they're keyed by id and simply never match again), but worth
  a sweep if projects churn.
- Whether a structured "decision" comment should ever count as progress (kept as
  pulse for now).
- Whether a bubble's band should become `max(thread levels)` outright (see
  *Phase A as built*) — needs a look at a live board before flipping.
