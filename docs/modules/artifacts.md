# Artifacts — writing a thread's page

**Status:** built (Phases 0–7) · **Packages:** `internal/md`,
`internal/server/artifacts.go`, `frontend/src/components/ArtifactEditor.svelte` ·
**Depends on:** [`documents.md`](./documents.md) (where the bytes live),
[`threads.md`](./threads.md) (edits are evidence) · **History:**
[`journal/ARTIFACT-EDITING.md`](../journal/ARTIFACT-EDITING.md)

## What it solves

A thread's document is the framework's load-bearing artifact, so it has to be
editable from wherever the work is happening: the board, the CLI, and an agent's MCP
session — without three editors fighting over one body.

## Model (normative)

### One page, several regions

A thread has **one** document, exposed as named regions:

| Region | What it is |
|---|---|
| `brief` | everything that is not the Logbook or the DoD — the API's older word for the document, still accepted; `document` is the engine's own name |
| `logbook` | the plan: phases, todos, published evidence |
| `dod` | the Definition of Done, when the author wrote one |

A named `## section` is **not** a region: it is an edit *inside* the document region
(`applySectionEdits`), which is why writing one cannot rename the Logbook. The
document store adds a fourth, non-writable region for what follows the last section
— see [`documents.md`](./documents.md).

The document and its Logbook live in the **same** page, because the framework's rule
is one source of truth per piece of work: a second tracker is exactly what the model
forbids.

### There is no shape of a Brief

Removed — see [`decisions/0006`](../decisions/0006-plane-holds-the-relationships.md).
The typed Brief ([`decisions/0003`](../decisions/0003-briefs-have-a-type.md)) is gone
along with `domain.BriefSections`, the thread `type` and every lint rule that measured
a Brief. A thread's document is prose in whatever shape its author wants; the only
sections the server reads are the two addressable regions below.

What the thread's document IS: everything on the page that is not the Logbook or the
Definition of Done. It has no required sections, no ceiling on how many it has, and
no reserved names of its own — `## Brief` is now an ordinary heading somebody may or
may not write.

One leftover is worth knowing, because it still bites anyone writing a Brief-shaped
document by hand: a `##` heading is a SIBLING of every other `##`. Writing
`## Brief` with `## Context` under it produces two sections, not one nested in the
other. Use `###` for the parts of a section.

A Definition-of-Done item reads best when it is **verifiable by someone else without
asking the author**. Nothing enforces it — that is exactly what a DoD full of tasks
costs you: `unmetDoD` reports those items at completion, so a task-shaped DoD reports
the Logbook back at you instead of answering "is this finished?".

Checkboxes are not confined to the Logbook or the DoD. `ParseChecklist` reads real
`- [ ]` boxes anywhere in the document, `CountDone` counts ticks across the whole
page, and `toggle_todo` addresses them under region `document`. Only real boxes count
there — the Logbook's bullet-as-todo fallback would read prose bullets as a task
list.

### The shape of a Logbook

**Convention, not parsed** — deliberately. A seeded Logbook opens with one status line —
`**Owner:** … · **State:** … · **Next:** …` — then phases as `###`, then an
append-only `### Decisions` list. `Next` is what the operating loop already demands
and had nowhere to live. Parsing it into a field (so the board can show it without
opening a thread) is deliberately deferred until people are writing it.

Agents write region names loosely; the server maps synonyms (`bitácora` → logbook,
`definition of done` → dod) and defaults an unnamed section to the Logbook, which
is where an agent almost always means.

### Writes are surgical first

A write may name:

- **`edits`** — find-and-replace inside a region: quote the old text, give the new.
  `old` must appear exactly once unless `all` is set; empty `old` appends. Applied
  **before** any whole-region write, and refused alongside one for the same region.
- **whole-region content** — replace `brief` / `logbook` / `dod` / a section.
- **`base`** — the region hashes the editor read, per region. A mismatch is a
  conflict, not a silent overwrite.

Preferring `edits` is not a style choice: replacing a whole region to change one
line is how an agent destroys a human's paragraph it never read.

### Splicing

`internal/md/splice.go` replaces a region's blocks in place, leaving every other
byte untouched. The property that matters is not "the output looks right" but
"nothing outside the target moved".

### Todos and the DoD

Ticking a todo (`POST /api/threads/{id}/todo`) carries the item's text as a guard:
if the text at that position no longer matches, the write is refused rather than
applied to whatever moved into place. A ticked todo is progress evidence.

**Several items tick in one operation.** `items` on the same endpoint applies a batch
all-or-nothing and writes to Plane ONCE — ticking five things is one change, and it
used to cost five calls and five writes against a 60-per-minute budget. Indices stay
valid across a batch because toggling rewrites a line in place.

**The answer states the persisted state.** A toggle returns the per-region open/done
counts, each item as it now stands, what still blocks completion, and the thread's new
level — read back from the stored page rather than echoed from the request.

**Every todo a reader receives carries its own address** — `region` and `index`,
which are exactly what the write takes. Indices are **per region** and start at 0, so
the DoD's first item is `dod[0]` and never "after the logbook items". They exist
because inferring an address is silent when it goes wrong: an index counted across the
page ticks a different item, or none.

And a refusal explains itself. "No todo at that position" now says how many items that
region has, or that the region is missing, or that the page's sections are duplicated
— and points at the region that does have items. Same for a failed exact edit: it
names the closest line it can find and says to quote from `regions[…].markdown`
rather than from the rendered artifact. An agent recovers from an error that carries
the fix and gives up on one that does not.

### One of each section, enforced

`## Logbook` and `## Definition of Done` are **reserved headings**: they are what
makes each part addressable. Two of either is refused (`duplicate-section`), and the
create path strips a leading `## Logbook` from what a caller sends so including it is
harmless. `## Brief` is no longer reserved
([`decisions/0006`](../decisions/0006-plane-holds-the-relationships.md)).

This is the one structural rule that is not a matter of taste, because a duplicate
does not merely look untidy: a section's range ends at the next heading of its level,
so the first `## Logbook` truncates at the second and its region becomes **empty**.
Every region-scoped operation then addresses nothing — `toggle_todo` answers "no todo
at that position", exact edits cannot find text that is plainly visible in the
rendered page, and the reader shows a Logbook that disagrees with the document. One
real MCP session lost an afternoon to exactly this.

The DoD's unticked items are what gate finishing ([`threads.md`](./threads.md)).

### Revisions

A revision — a findings write-up, a deliverable — is a **sub-work-item** of the
thread, not a thread of its own. Landing one is progress evidence. Revisions have
bodies like any artifact.

### The editor

In the browser: regions editable in place, autosave with the read hashes as `base`,
slash commands, inline checkboxes that tick through the same guarded endpoint, and
optional vim keys. An agent editing the same thread pushes an SSE nudge, so an open
interior repaints mid-edit rather than saving over the agent's work.

### The house standard

`internal/md/lint.go` holds the markdown standard the README's §3 describes — what a
Brief and a Logbook read like when somebody chooses them — so a page that has broken
its own addressing is caught at the door, and everything else is merely mentioned.

Its governing principle: **a rule about a structure only fires when that structure
is there**, and even then it advises rather than refuses. Most real threads are
free-form documents, and demanding a Definition of Done from a page that never
claimed to be a Brief would make the tool unusable on the work that already exists.
And a write is judged on what it *broke*
(`NewFindings`), never on what it inherited — otherwise a one-line fix is blocked
because somebody else left the page messy.

**One refusal remains:** a duplicate `Logbook` / `Definition of Done` heading. That is machinery — the first section truncates at the second, leaving the
canonical region empty and every region-scoped write addressing nothing.

**Everything else warns** (`SevWarn`, filtered by `md.Refusals` at every write path):
more than one H1, a heading-level skip, a Brief with no Definition of Done, a Logbook
with no todo, a Brief over ~25 lines or with more than five sections, a DoD outside
3–10 items or with no checkboxes, a DoD item that opens with a task verb, a Logbook
with no `Next:`, more than five phases. A warning rides back on a write that
SUCCEEDED. The framework does not own anybody's format
([`decisions/0005`](../decisions/0005-the-framework-does-not-own-your-format.md)); a
linter that refuses on shape is one people route around.

## Surfaces

| Capability | REST | CLI | MCP |
|---|---|---|---|
| read the page | `GET /api/threads/{id}` | `bubble thread` | `read_thread` |
| write regions / edits | `PATCH /api/threads/{id}` | `bubble logbook` · `dod` · `section` | `update_thread` |
| tick a todo | `POST /api/threads/{id}/todo` | `bubble todo` | `toggle_todo` |
| add a revision | `POST /api/threads/{id}/revisions` | `bubble revision` | `add_revision` |
| drop a region | `DELETE /api/threads/{id}/regions/{region}` | `bubble section --delete` | `delete_artifact` |
| retitle | `PATCH /api/threads/{id}` (`title`) | `bubble rename` | `update_thread` |

## Storage

Bodies live in `thread_docs` ([`documents.md`](./documents.md)); the mirrored
`description_html` is the fallback for threads not yet adopted. Editing a Logbook
fingerprints it in
`thread_progress`, which is how "the plan materially changed" becomes evidence
rather than a guess.

## Invariants

1. One page per thread. The Brief and the Logbook are regions of it, never separate
   documents.
2. A write with a stale `base` is a conflict.
3. `edits` never coexist with a whole-region write for the same region.
4. A todo write that cannot find its text is refused.
5. Splicing may not move bytes outside the region it targets.

## Open

- Parsing the Logbook's `Next:` into a field, so the board can show the next concrete
  action without opening a thread.
- Region-level history (see [`documents.md`](./documents.md)).
- The editor is one region at a time; there is no "edit the whole page as markdown"
  mode.
