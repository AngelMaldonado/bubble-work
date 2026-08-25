# 0002 — Being born is production; being created is not

Status: **accepted** · 2026-08-15 · Refines the spec's §5.1 · Implemented by
[`modules/heat.md`](../modules/heat.md) and [`modules/threads.md`](../modules/threads.md)

## Context

A bubble with no threads is Dormant, which is correct — nothing has been
promised yet. But creating threads inside it did not change that, and it should
have.

The chain, as built:

| Fact | Where |
|---|---|
| `ThreadBirthHeats: false` in the default calibration | `internal/domain/domain.go:139` |
| `Progress()` deliberately excludes `EvThreadCreated` | `internal/domain/domain.go:51` |
| A newborn with no evidence classifies 😴 `thread_newborn` | `internal/heat/heat.go:147` |
| `BubbleLevelRollup: true` — a bubble's band is its hottest unfinished thread | `internal/domain/domain.go:138` |

So: you birth three threads, none has produced anything yet, the roll-up takes
the hottest of them (😴), and the bubble stays asleep. The board reports nothing
happening in the exact moment something started happening.

The original reason for `ThreadBirthHeats: false` is stated in its own comment:
*"turning it on makes every new Backlog item read 🔥 for a cycle"*. That reason
assumes **creating a thread means dropping an item into a tracker**. In Bubble
Work it does not. The birth rule (spec §3) refuses implementation until a Brief
with a Definition of Done and a seeded Logbook exist. Getting a thread born costs
two written artifacts, an intended outcome, and at least one actionable todo.

That is not motion. Under §5.1 it is two of the listed evidence kinds at once —
a recorded decision and a published deliverable — produced before any code is
touched.

## Decision

**Separate creation from birth, and let birth be evidence.**

| Event | Meaning | Heat |
|---|---|---|
| `EvThreadCreated` | a work item exists (e.g. someone made one in Plane) | none — unchanged |
| `EvThreadBorn` | the thread passed the birth rule: Brief with a DoD, seeded Logbook | **progress** |

- A thread born through Bubble Work is `in_progress` 🔥 from its first moment,
  because its birth artifacts are real output.
- Its bubble follows for free through `BubbleLevelRollup` — the bubble is the
  emergent state of its threads, so no separate bubble rule is added.
- A bubble with no threads keeps classifying Dormant (`dormant_never`). Nothing
  changes there.
- A work item that merely appeared, with no birth artifacts, still earns nothing.
  It is a draft, and `ThreadGraceCycles` keeps it 😴 rather than 🪦 while it waits
  to be born.

`ThreadBirthHeats` is retired as a knob about *creation*. The behaviour it
described — raw creation heating a thread — remains off, and is now the default
that needs no flag; what replaced it is not tunable, because "a Brief and a
Logbook exist" is a fact about the thread rather than a calibration.

## Consequences

**Repairing an old thread's artifacts resurrects it.** Writing the Brief that a
stale thread never had emits birth evidence, so the thread returns to 🔥 and
lifts its bubble. This is correct and intended: §5.1 counts a recorded decision
as heat, and repairing birth artifacts is the working protocol's *first* step
before implementation. It is also the honest reading — someone did work on that
thread today.

**Anti-gaming survives.** §8's guardrail is that heat requires evidence of
changed reality. Birth cannot be manufactured cheaply: an empty Brief with no
Definition of Done does not pass the birth rule, and the policy engine (§9.4) is
what decides, server-side, on every surface. Spamming births to keep a bubble
warm means writing real briefs with real outcomes — at which point the bubble
has a real problem, not a fake temperature.

**One asymmetry becomes visible and is accepted.** Threads imported from Plane do
not heat their bubble, while threads born here do. That is not an inconsistency;
it is the birth rule showing through the board. A bubble full of unborn work items
reads cold *because it is* — nothing in it has been defined well enough to start.
