---
name: finish-a-thread
title: Finish a thread
description: Close a thread honestly — the Definition of Done is the evidence you check yourself against.
args: thread_id (required)
---

Finish thread `{{thread_id}}`.

## The check is yours to make

`complete_thread` no longer refuses anything (`docs/decisions/0005`). Finishing is a
position somebody takes, and a checklist written days ago is evidence, not a warden.

What it does is **report**: complete a thread with Definition of Done items still
unticked and the answer lists them in `unmet_dod`. That is the honest record — the
thread shipped, and here is what was left.

So, in order:

1. `read_thread` and look at `logbook.dod[]` and `artifacts[].todos[]`. Each item
   carries its `region` and `index`.
2. For everything genuinely satisfied, `toggle_todo` with that `region`, `index` and
   `text`, `done: true`. Tick them all in one call with `items`.
3. Anything you cannot honestly tick is a decision, not a formality. Either the thread
   is not finished — write what remains into the `**Next:**` line and stop — or the
   item turned out to be wrong, in which case **say so in the document in the same
   turn**, so the record shows a decision rather than a shortcut.

## Then

`complete_thread`. It moves the thread into the project's `completed` state, records
the completion as evidence, and hands the thread off — from that moment the automatic
state sweep leaves it alone, because a human (or you, acting as one) took a position.

`force` is still accepted and now means nothing: there is no check left to skip.

## What finishing is not

- Finishing a thread does **not** close its bubble. A bubble closes when its outcome
  is reached, and that is a human decision (`close_bubble`).
- If every thread in a bubble is finished, the board says so and asks for that
  decision rather than making it.
