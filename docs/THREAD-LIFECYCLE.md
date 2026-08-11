# Thread Lifecycle — automatic buoyancy for work items

Status: **Phases A, B and C shipped** · Drafted 2026-08-03 · Companion to
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

- **Phase A — compute + show (no writes, safe).** ✅ **Shipped.** Attribute
  evidence per thread, run the classifier per thread, roll up to the bubble, and
  render a thread level chip in the timeline + interior. Read each thread's real
  Plane group for display. Zero risk; independently useful. See *Phase A as
  built* below.
- **Phase B — auto-write policy (behind the per-instance toggle).** ✅
  **Shipped** — see *Phase B as built*. Extends the cooling `tick`:
  Zzzz→Backlog, RIP→Cancelled, resurrect→In Progress, provenance guardrail,
  group resolution, inbox notifications.
- **Phase C — evidence sharpening.** ✅ **Shipped** (pulled ahead of B, which
  needs it — see *Phase C as built*). Logbook diffing and revision-added
  detection so an OPEN thread can produce evidence at all, plus comment pulse
  (hold 😴 / block 🪦, never wake).

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
  itself makes a thread 🔥, and a thread sitting in Backlog whose logbook is
  actually moving genuinely is 🔥. `threadLevel` consults the state group for the
  two **terminal** columns only — `completed` → 🏆 and `cancelled` → 🪦 — because
  those are statements of fact, not status theatre. The case ordering in
  `threadLevel` *is* the precedence.
- **Cancelling is undone by production, never by chatter.** `completed` always
  wins — finishing is the goal. `cancelled` normally wins too, but a thread that
  someone cancelled and then went back to work on resurrects to 🔥: evidence
  outranks a stale declaration. Comments cannot do this — the pulse is stripped
  before the lifecycle is computed, so only a logbook edit or a landed revision
  can reverse a cancellation. (While the two disagree, the board shows 🔥 and
  Plane still shows Cancelled, until a human moves the card or Phase B writes it
  back.)
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
| `bubble_level_rollup` | bubble | band = hottest unfinished thread (off = classify the union of the evidence) |
| `bubble_rip_needs_owner` | bubble | dormant + ownerless → 🪦 instead of 😴 (union mode only) |
| `thread_birth_heats` | thread | whether a work item's creation heats the item itself |
| `thread_grace_cycles` | thread | how long a newborn that produced nothing stays 😴 before 🪦 |
| `thread_rip_needs_owner` | thread | dormant + unassigned → 🪦 instead of 😴 |
| `pulse_cycles` | thread | how long a comment blocks 🪦 (0 = comments carry no weight) |
| `thread_terminal_state_wins` | thread | Plane's `completed`/`cancelled` columns override the computed level (`cancelled` still yields to production) |

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

**The roll-up** (deferred out of A, shipped later — see *The bubble roll-up*).

## Phase C as built

Phase A shipped the classifier but nothing fed it: the only progress evidence
was derived from Plane's `completed_at`, which exists only once a thread is
already closed. Every *open* thread therefore had an empty progress stream and
could never be 🔥. Phase C is what makes an open thread able to produce.

**Plane has no "the logbook changed" event, so we diff.** Once per project per
sweep, `ListProjectItems` pages every work item (one call, carrying each body and
parent link). For each thread we fingerprint the Logbook + DoD
(`md.LogbookFingerprint`) and count its revision sub-items (how many items name it
as parent), then compare against the last observation in the new
`thread_progress` table. The moment either changes is stamped.

**Any Logbook change is progress, not just a tick.** The Logbook is the plan, and
the working protocol says to update it when the plan materially changes — so
re-phasing it, adding a todo or striking one is a recorded decision, which is
changed reality. We still track the ticked count, but only to *label* the
evidence: a tick is `completed-todo`, anything else is `logbook-updated`. Both
warm the thread identically. Prose outside the Logbook (the Brief, the title) is
not diffed, and whitespace is normalized first, so a reflow or re-indent is not a
change — cosmetic edits must never generate heat (`AGENTS.md`).

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
- **Only the plan and the revisions are diffed.** Renaming the thread, editing
  the Brief, changing priority or dragging the card to another column produce no
  heat. Deleting a revision moves the counter but never the timestamp — history
  is not rewritten.

Known trade-off: because *any* Logbook edit counts, heat can in principle be
manufactured by editing the plan back and forth. That is the deliberate price of
treating re-planning as real work; the anti-gaming guarantee now rests on the
Logbook being the plan of record rather than on the edit being monotonic.

Mechanics: the diff also *writes*, so it runs on the per-project goroutine —
one writer per project, never inside the module fan-out. A failed listing logs
and yields no evidence (threads simply don't warm, the safe direction) rather
than dropping the project from the board. Evidence kinds are now explicit:
`thread-created` (weak, never heats the thread itself), `completed-todo`,
`logbook-updated`, `revision-added`, `thread-completed`. The per-project lookups moved into a
`projectCtx` struct so `buildBubble` takes one argument instead of eight.

### Comment pulse

A comment is **presence, not production**. `EvidenceEvent.Pulse()` marks it, and
both classifiers strip it: it never warms a bubble, never warms a thread, never
reaches 🔥. Its only job is to block the grave — `heat.HasPulse` holds a thread at
😴 when someone commented within `pulse_cycles` (default 1, 0 disables). The
pulse rides the same evidence stream as everything else, so nothing needed a new
plumbing path.

The hard part was cost: Plane has no project-wide comment feed, so reading
comments is **one call per thread** — unaffordable as a sweep. The resolution is
that the pulse is load-bearing in exactly ONE place, blocking 🪦, so it only has
to be accurate for threads that are actually dying:

- **Free, exact, everywhere:** any time comments are fetched or posted
  (`commentsFor`, `PostComment`), the newest comment's real timestamp is
  recorded. No extra traffic.
- **Bounded probe for the rest:** each `tick` computes which threads currently
  read 🪦 and fetches comments for at most `pulseProbeLimit` (20) of them,
  least-recently-checked first. Cost scales with the number of *dying* threads,
  not with the size of the board, and it is skipped entirely when
  `pulse_cycles` is 0.

A recorded pulse reaches the board via the next snapshot refresh, which emits it
as a non-progress evidence event. `last_comment_at` never moves backwards, so
deleting a comment doesn't erase the fact that someone was paying attention.

**Not built:** Plane's `issue_comment` webhook would make the probe nearly
redundant — the handler currently ignores the payload and just triggers a
refresh. Worth wiring once the event shape can be verified against a live
instance.

## Phase B as built

`Server.autoState` runs at the end of every `tick`, after the pulse probe, over
the same snapshot the notifications were computed from. `plane.SetWorkItemState`
is the only call in the codebase that moves a card, and it exists on no read
path.

- **Opt-in per instance, off by default** (`plane_instances.auto_state`).
  Until an operator flips it, Bubble reads Plane and never writes state. The
  switch is service-admin only, audited, and lives on all three admin surfaces:
  `POST /api/admin/instances/{slug}/autostate`, `bubble admin autostate <inst>
  on|off`, and a per-row toggle in God Mode's instance table (with a confirm,
  since it changes someone's tracker).
- **Targets resolved by group.** `defaultStateOf` picks the target group's
  default state per project, else its first by name for stability. A project
  with no state in the target group is skipped rather than guessed at.
- **The provenance guardrail.** `thread_autostate` records the state id *we*
  wrote. Before each move: a thread we have never written to is fair game; one
  whose current state still matches our last write is still ours; one whose state
  differs has been moved by a person, so it is latched `handed_off` and **never
  auto-managed again**. The latch is permanent by design — the alternative is a
  tug-of-war with whoever is actually doing the work.
- **No-ops are free.** A card already in the target group is skipped, so a
  settled board writes nothing at all after its first pass.
- **Bounded.** At most `autoWriteLimit` (50) cards move per tick. A calibration
  change can reclassify a whole board at once, and without a cap the next sweep
  would rewrite hundreds of items in one burst.
- **Attribution and audit.** Writes use the instance key (there is no human
  caller), are logged per card, and post a per-bubble inbox notification —
  *"Bubble A: moved 2 → Backlog and 1 → Cancelled in Plane"*. They are
  housekeeping, **not evidence**: they touch no progress row and warm nothing.
- **Closed bubbles are left alone** entirely, as is anything computing to 🏆.

Note the loop this closes with the resurrection rule: if we auto-cancel a thread
and someone later edits its Logbook, the thread recomputes to 🔥, our provenance
still matches (we wrote that Cancelado), and the next tick moves it to In
Progress. The card follows the work back.

## Finishing a thread (as built)

🏆 is derived from Plane, and for a long time that meant Bubble Work could show it
and never cause it. Autostate refuses to write it on purpose — `autoTarget`
returns `""` for `done`, "hands off" — so the one transition the framework asks
you to make was the one you had to leave the framework to make. A thread carries a
**Definition of Done** stating exactly when it is finished and todos to verify it;
you could tick the last box and still have to go find the card in Plane. An agent
that had satisfied the DoD had no way to say so at all.

`CompleteThread` and `ReopenThread` close that. They are on all three surfaces:
`POST /api/threads/{id}/complete` and `/reopen`, `bubble done` and `bubble reopen`,
`complete_thread` and `reopen_thread`, plus a 🏆 button in the thread interior.

What they do and deliberately do not do:

- **They write Plane's state, not an overlay flag.** There is no second source of
  truth to disagree with the first, and a thread finished here is indistinguishable
  afterwards from one finished by dragging the card.
- **The target is resolved by GROUP**, never by name. State names are
  project-configured and localized; a project whose completed column is called
  *Entregado* works, and one with no completed state at all is told so rather than
  having its card moved somewhere arbitrary.
- **The Definition of Done gates it.** Unticked DoD items refuse the completion,
  and the refusal names them. This is the closing counterpart of the §3 birth rule
  — the same artifact gates both ends of a thread's life.
- **The Logbook does not gate it.** A plan can legitimately carry items that
  outlive the thread ("monitor for a week"); gating on those would make the rule
  unpassable and therefore routed around.
- **No DoD is not an unmet DoD.** A small thread may close on a paragraph
  (AGENTS.md), and so may a DoD written as prose rather than a checklist.
- **`force` is the override**, for a DoD that turned out to be wrong. Logged as
  one, never the default on any surface, and never offered to an agent as a
  shortcut.
- **A human's move hands off autostate** for good, the rule already stated for a
  move made in Plane's UI, applied to one made through here.
- **Failure queues.** The write goes through the outbox with a `state` field lock,
  so a Plane that is down delays the card rather than losing the declaration
  (PLANE-SYNC.md Phase 5).
- **It is idempotent.** Completing an already-completed thread is a no-op, not a
  second write and not an error.

## The bubble roll-up

`Server.bubbleHeat` is the single place a bubble's temperature, band and
per-thread breakdown are decided, so the board, the CLI and the cooling sweep
cannot disagree. With `bubble_level_rollup` on (the default), **a bubble is its
threads**: it sits in the band of its hottest unfinished thread and reports that
thread's reason.

This is what "push buoyancy down, roll it up" was always meant to mean. The
alternative — classifying the union of a bubble's evidence — has one specific
flaw: `thread-created` is bubble-level evidence, so a bubble whose work items
were all created moments ago and touched by nobody reads 🔥. The roll-up asks the
threads instead, and every one of them is still waiting its turn (😴).

| Situation | Band |
|-----------|------|
| Any thread unfinished | that thread's band, hottest wins |
| Every thread finished, bubble not closed | 😴 *"every thread is finished — close or redefine this bubble"* |
| No threads at all | 🪦 *"no threads yet"* |
| Bubble closed / reviewed | 🏆 / 👀 — explicit human declarations always win |

Consequences worth knowing:

- **Birthing threads no longer warms a bubble.** Creating work is not doing work
  — the same rule threads already follow, now applied consistently at both
  grains. A newly filled bubble reads 😴 until something is actually produced.
- **`bubble_rip_needs_owner` is inert** while the roll-up is on: naming an owner
  is paperwork, and the band now comes from the threads.
- **A bubble of graves is a grave.** Under the union a bubble with any threads
  essentially never reached 🪦, because their births counted as evidence forever.
- The knob flips the whole thing back to union mode instantly, with no
  recompute — useful for comparing the two against a live board.

Known coarseness (pre-existing, not introduced here): notifications fire on
`Lifecycle` transitions, and 😴 and 🪦 are both `Dormant`, so a bubble sliding
from asleep to abandoned is silent. Making notifications track the *band* would
fix it in both modes.

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
- Notifications track `Lifecycle`, which cannot see a 😴 → 🪦 slide. Tracking the
  band instead would fix it for both modes.
- Completing a thread is only reachable from the thread interior in the web. The
  bubble's timeline lists threads with their level and would be the natural place
  to finish one without opening it.
