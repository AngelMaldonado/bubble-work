# 0004 — Writing is the work; discussion is not a grave

Status: **accepted and built** · 2026-08-16 · Refines the README's §5.1 and §5.2 ·
Implemented by [`modules/heat.md`](../modules/heat.md) and
[`modules/bubbles.md`](../modules/bubbles.md)

## Context

Two complaints, one session apart, about the same thing: bubbles reading colder
than the work felt.

The model's rule is *heat = evidence of changed reality, never motion*. It was
implemented literally, and two of its edges turned out to be sharper than
intended:

1. **Only the Logbook and the Definition of Done counted as edits.**
   `progressEvidenceFromMirror` fingerprinted those two sections and nothing else,
   with the reasoning that "prose outside the Logbook — the Brief, the title — is
   not progress". So spending an hour sharpening why a piece of work matters
   produced exactly no heat.
2. **A discussed bubble could read 🪦.** Comments are pulse, not progress, and the
   pulse blocked the grave at the *thread* grain only (`threadLevel`). A bubble
   whose threads had all gone quiet showed as abandoned however much anyone was
   still talking about it.

## Decision

### Any edit to the document is production

Not only the plan. Writing **is** the work in this framework — the birth rule
exists because a Brief is load-bearing — so improving one is changed reality.

Two fingerprints are kept, and the difference labels rather than gates:

| What moved | Evidence kind |
|---|---|
| a todo got ticked | `completed-todo` |
| the Logbook or the DoD text | `logbook-updated` |
| anything else on the page | `body-updated` *(new)* |

All three are progress, all three warm the thread identically, and the thread's
bubble follows through the roll-up.

Unchanged: the **title** is still not progress — it lives in Plane's `name`, not in
the document, and what work is called is not what has been done. Whitespace is
still collapsed before hashing, so a reflow or a re-indent is not an edit.

Also unchanged, and worth stating because it follows: an edit **imported from
Plane** now warms too. It is an edit to the body; where the person typed it is not
the framework's business.

### Discussion holds a bubble out of the grave

Comments still **never warm anything**. §5.2 stands at both grains. What they now do
at the bubble grain is what they already did at the thread grain: a recent comment
floors the band at 😴 instead of 🪦.

Deliberately a floor on the **band only**:

| Affected | Not affected |
|---|---|
| the UI band a bubble sits in | its lifecycle (`Classify` is untouched) |
| | its buoyancy score, and therefore its ordering |
| | notifications, which key off the lifecycle |

So a bubble people are discussing is not called abandoned, and it also does not
climb over a bubble that shipped something. Presence must not outrank output.

`pulse_cycles = 0` disables it, exactly as it disables the thread-grain pulse.

## Alternatives rejected

| Alternative | Why not |
|---|---|
| Comments become progress (one line in `Progress()`) | Deletes the invariant the whole model rests on: an abandoned thread somebody chats about would read 🔥. Three tests exist to prevent precisely this. |
| Comments warm the bubble but not the thread | Delivers "comments heat the bubble" as literally asked, at the price of a bubble that can be 🔥 with nothing but talk in it. The floor gets the useful half — nothing sinks while it is being discussed — without buying the gameable half. |
| Fingerprint Plane's `description_hash` instead of the markdown | Cheaper, and wrong: that hash is over Plane's HTML, so Plane re-serialising a body would register as production. |
| One fingerprint over the whole body | Simpler, and loses the plan/prose distinction — which is worth keeping, because "the plan changed" and "the intent got sharper" are different facts about a thread. |

## Consequences

**Autosave now rebuilds the snapshot.** The exception for Brief edits existed for a
reason: autosave sends them every few seconds while somebody types, and each one now
marks production and rebuilds the instance snapshot. It is a local rebuild (SQLite
only, no Plane traffic) and singleflight coalesces concurrent ones, so it is
affordable at current sizes. Debouncing is the obvious next move if a typing session
ever shows up in a profile, and it is safe: skipping a rebuild **delays** heat rather
than losing it, because the fingerprint diff compares against what is stored.

**One new column, one silent baseline.** `thread_progress.body_hash`. A row that
predates it has an empty hash, which baselines on the next sweep rather than
announcing somebody's months-old prose edit as new work — the same rule that governs
a thread the sweep is seeing for the first time.

**`body-updated` is a new evidence kind** and will appear in reasons and timelines.
Anything that renders evidence kinds needs to know the word.

**Notifications still key off the lifecycle**, so a bubble that goes quiet while
being discussed will still generate its "cooling" notification even though the board
shows 😴 rather than 🪦. Defensible — *we told you it went quiet; we did not say it
was dead* — but it is a seam, and it is recorded in
[`modules/notifications.md`](../modules/notifications.md) rather than left to be
discovered.
