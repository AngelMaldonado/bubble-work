---
name: bubble-work
title: How Bubble Work works
description: The model, the vocabulary, and the rules the server will enforce on you. Read this first.
---

You are working inside **Bubble Work**: a way of working where attention behaves
like buoyancy. Work that produces evidence stays hot and rises; work that goes quiet
cools and sinks. Time is the governing force.

## Vocabulary — use these words, they are not decoration

| Term | What it is | Maps to |
|---|---|---|
| **Workspace** | the boundary for a body of work | a Plane *project* (not a Plane workspace) |
| **Bubble** | a durable grouping; the unit of *attention*. Has an outcome and can die | a Plane module |
| **Thread** | one executable unit of work | a Plane work item |
| **Cycle** | the repeating pulse heat is measured against | a Plane cycle |
| **Artifact** | what a thread produces: its document, and its revisions | a region of the work item's page |
| **Heat** | evidence of *changed reality* | derived, never stored |

## The framework does not own your format

A thread needs a **name**. Nothing else is required, and nothing about the shape of
its document is enforced (`docs/decisions/0005`): write it the way the work actually
has shape, in your own headings and your own order.

What the framework does instead is measure. It hosts the writing and tells you what
has gone cold — that is the whole trade. So the things worth writing are worth writing
because they make the work finishable, not because a gate demands them: why this
exists, what is true when it is done, the next concrete action, and checkboxes for the
steps.

Two headings still carry machinery, and are offered rather than demanded:
`## Definition of Done` (reported when you finish, tracked by `audit_bubble`) and
`## Logbook` (its own addressable region). Use either, both, or neither. Nothing else
on the page is parsed — there is no Brief template any more (`docs/decisions/0006`).

To say what KIND of work a thread is, use Plane's **labels**. That question already
had an answer in the tracker; writing it into the prose only created a second one.

## Heat is evidence, never motion

These warm a thread, and through it the bubble that holds it:

- being **created**
- ticking a todo, anywhere in the document
- **any edit to the document** — writing is the work here
- landing a revision, or finishing the thread

These warm **nothing**:

- **comments.** A comment is presence. It stops a bubble being called abandoned and
  never makes it hot. Do not post a comment to show progress — write the progress
  into the Logbook, which is the same act done honestly.
- renaming things, re-planning without output, or a work item merely existing

Never manufacture activity to keep a bubble warm. If work is cooling, the honest
moves are: revive it with real work, redefine it, or close it.

## One page, three regions

A thread has ONE document. Three parts of it are addressable:

| Region | The API calls it | Holds |
|---|---|---|
| document | `document` (older callers say `brief`) | everything that is not the Logbook or the DoD |
| logbook | `logbook` | the plan: phases, todos, published evidence |
| dod | `dod` | the Definition of Done |

A named `## section` is **not** a region — it is a heading inside the document, and
`update_thread`'s `sections` field is how you write one.

**Write each reserved heading at most once.** `## Logbook` and
`## Definition of Done` are what make those parts addressable, so a page with two of
the same one is refused: the first section truncates at the second, and the region
becomes unreachable. That is the only shape rule left, and it is machinery rather than
taste.

## What to do with a thread you did not write

Read it, in whatever shape its author left it. Find the outcome it is aiming at and
what it says finishing means; if neither is written down, that is the first thing
worth adding — ask, or write what you understood and say you did. Then work, and
record what you did in the document as you go, not at the end. Any edit to it is
evidence, so recording progress honestly is also what keeps the thread warm.
