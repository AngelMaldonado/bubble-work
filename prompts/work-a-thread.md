---
name: work-a-thread
title: Work a thread
description: The read → repair → edit → record loop, and the exact way to address todos and text.
args: thread_id (required)
---

Work thread `{{thread_id}}`.

## 0. If you are looking at a whole bubble, audit it

`audit_bubble` answers, for every thread at once: its level and why, what its document
carries, DoD and todo progress, what is still open, its declared `**Next:**` action,
and when it last produced anything. One call, no Plane traffic.

Use it instead of walking `thread_timeline` and then reading each thread. It also
reports what each thread is **missing** — a finish line, a next action — which is the
thing you cannot see one thread at a time. Missing is not illegal; it is a prompt to
go find out what the work is actually for.

Then read the single thread you are about to work on.

## 1. Read it, and read what you get back

Call `read_thread`. It returns several views of the same page, and **only one of them
is writable**:

| Field | What it is | Use it for |
|---|---|---|
| `regions["document"\|"logbook"\|"dod"].markdown` | the exact text a write diffs against | **quote from here** for `edits`; send `hash` as `base` |
| `regions[…].hash` | fingerprint of that region | optimistic concurrency |
| `logbook.todos[]`, `logbook.dod[]`, `artifacts[].todos[]` | parsed checklist items, each carrying `region` and `index` | pass those two straight to `toggle_todo` |
| `artifacts[].markdown`, `logbook.html` | the RENDERED read model | reading and reasoning only |

The most expensive mistake available to you: copying text out of
`artifacts[].markdown` and sending it as an `edits.old`. It will not match, and the
error will look like the text is missing when it is plainly on the page.

## 2. Repair before you build

No finish line? No next action? Nothing forces you to add them — but a thread nobody
can tell is finished never gets finished. Sort that out first, in whatever shape the
document already uses.

If a write is refused for **duplicate section headings**, the page has two `##
Logbook` (or two Definitions of Done). Merge them into one and delete the loser. Until you do,
that region is unreachable: the first section truncates at the second heading, so it
is empty and every region-scoped write addresses nothing.

## 3. Edit surgically

`update_thread` in order of preference:

1. **`edits`** — quote the old text exactly (from `regions`), give the new. `old`
   must appear exactly once unless you pass `all`. An empty `old` appends.
2. **`sections`** — write one `## section` of the document by heading name.
3. **whole-region** (`document` / `logbook` / `dod`) — a last resort. Replacing a whole
   region to change one line is how you destroy a paragraph you never read.

Send `base` with the hashes you read. A conflict answer means somebody else wrote
while you were thinking; re-read and re-apply.

## 4. Tick todos, do not rewrite them

`toggle_todo` takes `region` and `index` **exactly as `read_thread` reported them**,
plus the item's `text` as a guard. Indices are per region and start at 0 — the DoD's
first item is `dod[0]`, never "after the logbook items".

Checkboxes written in the document itself are region `document` and are reported on
`artifacts[].todos[]`. There, only real `- [ ]` boxes are addressable: a plain bullet
in prose is a sentence, not a task.

**Tick everything you finished in one call**, with `items`:

```json
{"thread_id": "…", "items": [
  {"region": "logbook", "index": 0, "text": "scaffold", "done": true},
  {"region": "dod",     "index": 1, "text": "it is measured", "done": true}
]}
```

All-or-nothing, one write. And you do **not** need to re-read the thread afterwards:
the answer states the persisted state — each item as it now stands, the open/done
counts per region, what still blocks completion, and the thread's level.

If it answers *no todo at that position*, do not guess a different number. Read the
thread again: the error says how many items that region has, and whether you asked
the wrong region.

## 5. Record, then stop

Before you finish a turn:

- update the todos you actually completed
- link the concrete evidence (commit, PR, deliverable) in the Logbook
- update the `**Next:**` line to the single next concrete action
- put anything decided in `### Decisions`

Do **not** post a comment to report progress: a comment earns no heat and warms
nothing. The Logbook is where progress goes, and writing it *is* the evidence.
