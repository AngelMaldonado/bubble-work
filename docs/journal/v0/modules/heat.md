# Heat — the temperature function

**Status:** built · **Packages:** `internal/heat`, `domain.Tuning` ·
**Depends on:** nothing (pure) · **History:**
[`journal/THREAD-LIFECYCLE.md`](../journal/THREAD-LIFECYCLE.md)

## What it solves

Turning a stream of evidence into a temperature, so the board can order work by
buoyancy without anyone maintaining a status field.

## Model (normative)

Heat is a **pure function** of evidence, the calibration, and the current time.
Nothing is stored, and nothing decays in a background job — time passing changes
the inputs, so the output changes for free. The consequence worth stating: there
is no "cooling job" that can fall behind, and no stored level that can be wrong.

The same classifier runs at two grains:

| Function | Grain | Notes |
|---|---|---|
| `Classify` | a bubble | comments are stripped: pulse is presence, not output |
| `ClassifyThread` | one thread | measured against **its bubble's** window, so the two are commensurable |

### The window

`WindowFor` resolves the heat window: Plane's real cycle boundaries when the
project has an active cycle, otherwise a rolling window of one cycle ending now.
When Plane's boundaries are known, the project's real cycle length replaces the
configured one.

### The verdict

| Condition | Result |
|---|---|
| evidence in the current window | **Hot** — `hot_current` |
| evidence in the previous window, work still open | **Warm** — `warm_previous` / `thread_warm_open` |
| nothing this window or last | **Cooling** — `cooling` |
| nothing ever, silent past the dormant threshold, or nobody accountable | **Dormant** — `dormant_never` / `dormant_silent` / `dormant_ownerless` |
| closed / completed | **Closed** — `closed_bubble` / `closed_thread` |

Ordering within a band comes from a **buoyancy score**: exponential decay of the
most recent evidence over the decay window. Fresh output ≈ 1.0; one decay window
old ≈ 0.37; two ≈ 0.14.

Note the ordering of the rules: something actively producing stays Hot or Warm
even with nobody named as owner. **Ownerlessness sinks what has already gone
quiet** — output outranks paperwork.

Each result carries an English `Reason` for logs and the CLI, plus a stable `Code`
and `Args` so a client can render it in its own language. The heat model has no
business knowing what language anyone reads.

### What counts as evidence

Evidence kinds live in `internal/domain`:

| Kind | Class |
|---|---|
| `thread-born` | **progress** — the birth artifacts exist ([`decisions/0002`](../decisions/0002-birth-is-production.md)) |
| `completed-todo` | progress |
| `logbook-updated` | progress — the plan or the DoD changed |
| `body-updated` | progress — anything else on the page changed ([`decisions/0004`](../decisions/0004-writing-is-the-work.md)) |
| `revision-added` | progress |
| `thread-completed` | progress |
| `thread-created` | neither — a work item exists, nothing was produced |
| `comment` | **pulse** — presence only: holds 😴, blocks 🪦, never wakes |

### What counts as an edit

Any change to the document, not only the plan (`md.BodyFingerprint` over the whole
body, `md.LogbookFingerprint` over the Logbook + DoD). Both are kept because the
difference labels the evidence; neither gates it. Whitespace is collapsed first, so a
reflow is not an edit, and the fingerprints are over the MARKDOWN — hashing Plane's
HTML would make Plane re-serialising a body look like work.

### Newborns and the pulse

- `Newborn` — a thread born less than `ThreadGraceCycles` ago that has produced
  nothing is 😴 (waiting its turn), not 🪦 (abandoned).
- `HasPulse` — a recent comment keeps a thread out of the grave without ever
  warming it. `bubblePulse` reads the same signal one grain up, so a bubble being
  discussed is not called abandoned either
  ([`decisions/0004`](../decisions/0004-writing-is-the-work.md)). Band only: not the
  lifecycle, not the score.

## Calibration

Every threshold is a `domain.Tuning` field, editable at runtime by a service admin
(`bubble admin tuning`, or God Mode). `Sanitize` clamps the numbers so a bad edit
cannot produce a zero-length decay.

| Knob | Default | What it moves |
|---|---|---|
| `cycle_hours` | 168 (one week) | the rolling window when Plane has no active cycle |
| `dormant_cycles` | 2 | cycles of silence before Dormant |
| `decay_cycles` | 1 | how long things stay buoyant in the ordering |
| `ownerless_is_dormant` | true | sinks what is quiet *and* unowned |
| `bubble_rip_needs_owner` | true | dormant + ownerless bubble reads 🪦 (only when roll-up is off) |
| `bubble_level_rollup` | true | a bubble's band is its hottest unfinished thread |
| `thread_grace_cycles` | 1 | how long a newborn stays 😴 before 🪦 |
| `thread_rip_needs_owner` | true | dormant + unassigned thread reads 🪦 |
| `pulse_cycles` | 1 | how long a comment keeps a thread out of the grave |
| `thread_terminal_state_wins` | true | Plane's `completed` → 🏆, `cancelled` → 🪦 |

### Birth heats (built)

`thread_birth_heats` is gone. It asked whether a work item's *creation* in Plane
should heat the thread, and the answer was always no. What replaced it needs no knob,
because "somebody defined this piece of work here" is a fact about the thread rather
than a preference: creating one emits `thread-born`, which is progress. A work item
that merely appeared in Plane still earns nothing.

Removing the knob needed no migration — the calibration is opaque JSON in
`server_settings`, so the field simply stopped unmarshalling.

The birth timestamp is durable (`thread_progress.born_at`), written by
`CreateThread` and carried forward by every sweep. It has to be: the event it
replaced lived in the in-memory cache and died with it, which is why a freshly born
thread used to read 😴 for a whole cycle.

## Surfaces

- REST — `GET /api/bubbles/{id}/heat` explains a temperature; every board and
  thread payload carries the derived level.
- CLI — `bubble heat <id>`, and the level marks in `bubble ls` / `show` / `thread`.
- MCP — levels ride along in `list_bubbles`, `thread_timeline`, `read_thread`.
- Web — the band a bubble sits in, and the per-thread mark.

## Storage

None. Heat itself is never persisted. Its *inputs* are: progress timestamps
(`thread_progress`) and the comment pulse (`thread_pulse`) —
[`storage.md`](./storage.md).

## Invariants

1. Heat is derived, never stored. A level that looks wrong is an evidence problem.
2. Comments never warm anything, at either grain.
3. Nothing warms a bubble that did not change reality — the README's §5.1 list is
   the whole list. A comment can floor a band; it can never raise a temperature or a
   score.
4. A thread is classified against its **bubble's** window, never its own.

## Open

- Score and band are computed per request; there is no cache. Fine at current
  sizes, worth measuring if a workspace ever holds thousands of threads.
