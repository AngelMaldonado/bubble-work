# 0006 — Plane holds the relationships; the prose is nobody's business

Status: **accepted and built** · 2026-08-19 · Completes
[`0005`](./0005-the-framework-does-not-own-your-format.md) and supersedes
[`0003`](./0003-briefs-have-a-type.md) ·
Implemented by [`modules/threads.md`](../modules/threads.md),
[`modules/artifacts.md`](../modules/artifacts.md),
[`modules/plane-channel.md`](../modules/plane-channel.md) and
[`modules/mcp.md`](../modules/mcp.md)

## Context

[`0005`](./0005-the-framework-does-not-own-your-format.md) stopped *refusing*
documents that were not shaped like a Brief. It did not stop the server *reading*
them that way: `## Brief` was still a heading we wrote, `### Context` / `### Outcome`
/ `### Symptom` / `### Repro` / `### Scope` were still a template three surfaces
scaffolded, and a thread still carried a `type` (bug / feature / chore) in an overlay
table of our own.

Using the tool for a few weeks made two things obvious.

1. **The reserved words were doing nothing.** The rendered page already shows a
   thread's sections; the reader does not need `Context` to be one of five blessed
   names to understand it. Meanwhile everyone wrote around them — the sections people
   actually wanted were "Lo que quiero", "Pasos", "Dudas".
2. **We were answering questions Plane already answers.** `thread_meta.kind` existed
   because the label write path could not be verified against a live instance when it
   was built — its own comment said "reflecting it outward as a label stays a
   follow-up". A `### Links` section held URLs that Plane models as links. Blockers
   were sentences in a Logbook, while Plane has `blocked_by`.

Two answers to one question is worse than either answer. A thread labelled `bug` in
Plane and typed `chore` here is not a small inconsistency; it means neither is
trusted.

## Decision

### The document is prose, end to end

No Brief. No template. The server writes **no heading it invented** and reads no
section except the two that are addressable regions:

| Heading | Why it survives |
|---|---|
| `## Logbook` | its own editable region — the plan can be rewritten without touching the rest of the page |
| `## Definition of Done` | its own region, its open items are reported at completion and tracked by `audit_bubble` |

`domain.BriefSections`, `domain.ValidThreadType`, the `type` field, the
`thread_meta` table, and every lint rule about a Brief are gone. `## Brief` is no
longer a reserved heading: a page with two of them is now just a page with two
headings.

### The relationships come from Plane

| Question | Was | Is |
|---|---|---|
| what kind of work is this? | `thread_meta.kind`, one of three values | **labels** — Plane's, by name, any word your team uses |
| where is the evidence? | a `### Links` section typed by hand | **links** on the work item |
| what does this depend on? | a sentence in the Logbook | **relations**: `relates_to`, `duplicate`, `blocking`, `blocked_by` |
| what is this part of? | — | parent / sub-item, as before (revisions) |

Five verbs, on every surface: `set_labels`, `add_link`, `remove_link`,
`relate_threads`, `unrelate_threads` (REST `POST /api/threads/{id}/labels|links|
relations`, CLI `bubble label|link|relate`).

Labels are set by NAME, and a name the project does not have is created. Requiring a
uuid would mean nobody used the verb; Plane's own UI creates on the fly for the same
reason.

### Only one of them is production

- **Adding a link is production.** It is the proof that reality changed outside this
  tool — the commit, the PR, the deployed thing — which is exactly what §5.1 means by
  a published deliverable. New evidence kind: `link-added`.
- **Labelling, relating, unlinking are not.** Classifying work, or saying two pieces
  of work touch, changes neither of them. Same rule as renaming.

### Reading them costs almost nothing

- **Labels ride along free.** They are a field on the work item the delta already
  reads (`itemFields` gained `labels`); the project's label catalogue is one call on
  the ten-minute structure cadence.
- **Links and relations are per-item calls**, and there is no bulk endpoint — so they
  follow the comment pattern exactly: a small budget per pass (`edgeFetchPerPass`,
  8 items), newest-updated first, never a reason to hold back a project's cursor.
  **Our own writes update the mirror directly**, so the fill-in exists only to notice
  edges somebody added in Plane's UI.

No read path gained a Plane call ([`0001`](./0001-plane-is-a-channel-not-the-record.md)).

## Alternatives rejected

| Alternative | Why not |
|---|---|
| Keep `type` and mirror it to a label | Two writes, one truth, and the overlay still wins when they disagree. The whole point is to have one place. |
| Parse `### Links` and publish those URLs as Plane links | Round-trips prose into structure and back; an edit to the paragraph would have to diff links. Structure should be entered as structure. |
| Offer all eight relation types | Four of them (`start_before/after`, `finish_before/after`) are scheduling, and nothing in the heat model reads a date. They are still read back if somebody sets them in Plane; they are just not offered. |
| Fetch links/relations for every item on every pass | 2 calls × N items against a 60/min budget. At 96 threads that is the whole minute, forever, for data that changes rarely. |
| Drop the Logbook and DoD regions too | Then nothing is addressable and every write is a whole-page rewrite, which is what block splicing exists to avoid. These two earn their keep mechanically, not stylistically. |

## Consequences

**`thread_meta` is gone**, and with it `SetThreadKind` / `ThreadKind` /
`ThreadKinds`. Existing databases keep an orphan table; nothing reads it. A thread's
old type is not migrated to a label — there were three possible values and no way to
know whether the person still means it. Set the labels you want.

**`ThreadDetail.type` is gone from the API**, replaced by `labels[]`. A caller still
sending `type` on create is ignored rather than refused: rejecting an unknown key
would break scripts written last week.

**Two new mirror tables** (`mirror_item_links`, `mirror_item_relations`), one new
catalogue (`mirror_labels`), one new column (`mirror_items.labels_json`) and two on
`thread_progress` (`links`, `links_at`). All of the mirror ones are rebuildable; the
`thread_progress` pair baselines silently on the first sweep so links that already
existed are not announced as new work.

**`has_brief` in the audit now means "has a document at all"**. The field name is
kept so clients do not break, and the word is a leftover.

**Relations are project-scoped.** Plane relates work items inside one project, so
relating threads across workspaces is refused with that explanation rather than a
confusing 404 from Plane.
