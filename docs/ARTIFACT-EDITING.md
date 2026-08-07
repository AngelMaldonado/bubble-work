# Artifact editing — writing a thread's page from the board

Status: **Phases 0-1 shipped · Phases 2-7 pending** · Drafted 2026-08-07 · Companion
to [`MCP-ACCESS.md`](./MCP-ACCESS.md) and [`PLANE-SYNC.md`](./PLANE-SYNC.md).

## The intention

Edit a thread's Brief, Logbook and Definition of Done from the web interior,
with Notion-style `/` commands, and have the change land in Plane and warm the
thread the way an agent's edit already does.

## What is already here

`PATCH /api/threads/{id}` and `POST /api/threads/{id}/revisions` shipped with
MCP-ACCESS steps 1–3, together with write-through to the mirror and an SSE nudge
that repaints an open thread. **`frontend/src/lib/api.ts` calls neither** — a
surface-parity gap, and the cheapest part of this work.

The expensive part is not the UI.

## The problem, measured

Bodies live in Plane as `description_html`. We read them through `md.FromHTML`
and write them back through `md.RenderHTML`. Running that round trip over all 96
real bodies in the local mirror:

| | measured |
|---|---|
| bodies unchanged by a read→write round trip | **77 / 96** |
| `mention-component` nodes **silently deleted** by `FromHTML` | **48** |
| `image-component` → `<img src="plane-asset:…">`, which Plane cannot render | **193** |
| ordered lists renumbered (`1.` → `3.`), hard-break lines gain a leading space | the other 19 |

Task lists are fine: `taskState` already reads both Plane's `data-checked` shape
and goldmark's `<input type="checkbox">`, and a full write→read loop preserves
`- [x]` and `CountDone`. Verified end to end.

**This is already live.** `UpdateThread` re-derives the *whole* body from
markdown, so an agent editing a Logbook on a thread that has images or mentions
destroys them today. Fixing the write path is not a prerequisite for editing —
it is a bug fix that editing happens to force.

Three facts that shape the design:

- **Plane does not normalize what we write.** Our own `rev: reviewer feedback`
  body is stored verbatim as `<div><h2>…`, with none of Plane's
  `editor-heading-block` / `data-id` attributes. What we send is what is stored,
  so the fidelity is entirely ours to own.
- **Images cannot be displayed.** Plane serves description assets only to a web
  session; the API key is rejected on every asset endpoint, which is why
  `rewriteAssets` swaps them for an "open in Plane" link. Any editor shows a
  placeholder where 193 images are — which is one of the reasons a WYSIWYG would
  be a WYSIWYG that lies.
- **Section granularity is not enough.** Only **4 of 96** bodies have a Logbook
  section at all. For the other 92 the editable region *is* the whole body, so
  "splice the section" degenerates into "re-render everything" — exactly what we
  were avoiding.

## The design

**The client edits markdown. The server splices HTML at the block level.**

A body tiles exactly into top-level blocks — verified: **96/96 bodies, zero
bytes between blocks**, 947 blocks total, 10 per body on average, 124 at worst.
So each block has a byte range in the original `description_html` and a markdown
rendering derived from it.

On save the server diffs the submitted markdown against the markdown it derived,
**block by block**:

- a block whose markdown is unchanged → **its original bytes are reused**, never
  re-parsed and never re-rendered;
- a block that changed, or is new → rendered from markdown;
- a block that disappeared → dropped.

Blast radius is the blocks you actually typed in. The 212 blocks holding an
image or a mention survive edits anywhere else in the document by never being
touched. Untouched blocks are also immune to the round-trip defects above,
because the comparison is markdown-to-markdown and nothing is re-derived.

**The splice runs on the server, against the mirror's current HTML** — not
against the copy the browser loaded. Byte offsets stay server-side, the client
stays dumb, two people editing different regions cannot lose each other's work,
and CLI and MCP get the same primitive for free.

### Regions

`document` (everything that is not Logbook or DoD), `logbook`, `dod`. A region is
a contiguous run of blocks. `document` is the whole body on the 92 threads that
have no Logbook — which is fine now, because the granularity underneath it is the
block, not the region.

### Autosave

Chosen over an explicit save. Constraints it has to respect:

- **Debounce 1.5 s idle, and at most one Plane write per 10 s per thread** with a
  trailing flush, plus a flush on blur / navigate / close. Worst case ~6 writes a
  minute against a 60/min budget that already reserves a 15-request background
  floor.
- **Skip the write when the region's markdown is unchanged.** Focus and blur
  must not cost a request.
- **Editing the Brief generates no heat, correctly.** `progressEvidenceFromMirror`
  fingerprints only the Logbook and DoD — a Brief or title edit is explicitly not
  production. Logbook autosaves stamp recency rather than accumulating, so a long
  editing session does not inflate heat.
- **Don't rebuild the instance on every keystroke batch.** `rebuildAndNotify`
  currently refreshes the whole instance. For a `document` save — which cannot
  produce heat — broadcast the thread and skip the rebuild.

### Conflicts

Each region carries a base hash. A mismatch is a **409**, not a merge. Separately,
the SSE `thread` event must not overwrite an open editor's buffer: while a region
is dirty an inbound change raises a banner instead of reloading.

## Phases

### Phase 0 — repair the round trip, and make it measurable ✅

- [x] **A hard break owns the whitespace on both sides of it.** `RenderHTML`
      emits `<br>\n`, so on the way back the newline *after* the break collapsed
      to a leading space and the spaces *before* it were re-emitted on top of the
      `"  "` the break already carries. `"a   \nb"` came back as `"a  \n b"`, then
      as `"a  \nb"` — a body that rewrote itself on every save.
- [x] **The same rule had to reach `liText`.** Fixing only the paragraph path
      moved the number by 7 bodies and stalled; list items assemble their inline
      pieces separately, and Plane's editor makes a hard break on every
      shift-enter, so task items were the single biggest remaining source. Both
      paths now share `inlineBuf`.
- [x] **Whitespace across inline boundaries.** `collapseWS` only ever sees one
      text node, so `"ser: "` + `<span>` + `" 1 se"` survived as a double space
      that HTML never rendered and that collapsed on the next read.
- [x] **Nested lists are indented to the parent's content column**, not a fixed
      two spaces. Under `"- "` those agree; under `"3. "` two spaces is one short
      of nesting, so GFM flattened the child into its parent and renumbered it.
      `<ol start="N">` is honoured too.
- [x] `bubble admin sync-fidelity <instance>` + `GET /api/admin/sync/{slug}/fidelity`
      + a **Check fidelity** button in God Mode. Admin capability, so REST + CLI +
      web and no MCP, matching `autostate` and `tuning`. A GET rather than a POST
      because unlike `sync-diff` it reads only the mirror and spends no rate
      budget.
- [x] A synthetic fixture corpus in `internal/md/fidelity_test.go` covering the
      measured element census — `p span div taskList image-component
      mention-component blockquote table ol ul code s br h1–h6`, in the
      ProseMirror wrapper shapes Plane actually emits. Real bodies are client work
      and do not belong in the repo.

**Measured**, over the same 96 bodies both times:

| | before | after |
|---|---|---|
| whole bodies surviving a write | 77/96 (80.2%) | **90/96 (93.8%)** |
| **blocks** surviving a write | 914/947 (96.5%) | **944/947 (99.68%)** |
| mentions destroyed | 48 | 48 — Phase 2 |
| images broken | 193 | 193 — Phase 2 |

The per-block number is the one the design rests on, because splicing never
re-renders a block you did not edit.

Of the 6 whole bodies still drifting, 5 **settle after one pass** — a one-time
normalisation, not a body that churns. What is left is inherent rather than
sloppy, and worth naming:

- **Adjacent lists merge** (4 bodies). Plane emits a run of single-item `<ul>`s;
  markdown cannot express two adjacent lists as separate, so they come back as
  one. Visually identical in both renderers, and under block splicing each `<ul>`
  is its own block that keeps its own bytes.
- **Text that looks like markdown** (1 body) — a paragraph whose literal content
  is `## Problem / opportunity` re-parses as a heading. Fixable by escaping, at
  the cost of a noisier editor buffer for one pathological thread.
- **One body oscillates** rather than settling: a nested ordered list whose
  three-space indent is ambiguous when it follows a paragraph instead of a list
  item, so it flattens and renumbers on alternate passes. The only body of 96,
  and confined to that block once splicing lands.

### Phase 1 — the block splice engine ✅

- [x] `internal/md/splice.go`: `Blocks`, `RegionMarkdown`, `Splice`.
      Tokenizer-based, because `html.Parse` + re-serialise normalises attribute
      order and quoting — a "byte-identical" splice built that way would not be.
- [x] LCS block alignment, so inserting a paragraph at the top costs one
      rendered block instead of re-rendering everything below it.
- [x] Regions: `document` (everything before the first special section),
      `logbook`, `dod`. A section's heading is excluded from its editable content
      — the heading is the marker, and editing a Logbook must not rename it.

**The gate: every region of all 96 real bodies splices its own current markdown
back to byte-identical output — 101/101 regions, 100%.** That is what makes an
untouched save a no-op on Plane, and what keeps an edit to one region from
disturbing another.

Getting there took three fixes the synthetic tests could not have found. The
first pass scored **63%**, and every failure lost exactly 85 bytes or a multiple
of it — a signature, not noise:

- **Empty spacer paragraphs** (37 bodies). Plane's editor leaves `<p></p>`
  behind as spacing. They carry no markdown, so the editor cannot show them and
  the submitted text cannot mention them — and a diff that only saw content
  deleted every one on the first save. They now ride along, anchored to the
  block they follow.
- **`<img>` swallowed the block after it.** The tokenizer reports `<img …>` as a
  *start* tag and no end tag ever follows, so the generic path opened a block
  that never closed. Void elements are their own block now.
- **A double `<br>` tore a paragraph apart.** Pressing enter twice in Plane
  renders as `"  \n  \n"`, whose middle line is two spaces. Splitting blocks on
  any blank-*ish* line split that paragraph into pieces that matched nothing and
  were re-rendered on every save. Blocks are joined with a bare `"\n\n"`, so the
  separator to recognise is always *exactly* empty.

All three are pinned as named tests, and `sync-fidelity` now reports the splice
gate alongside the round trip, so it stays measured rather than asserted.

### Phase 2 — Plane's node vocabulary, both directions

- [ ] `image-component` ↔ `![](plane-asset:UUID)`.
- [ ] `mention-component` ↔ `@[Name](plane-mention:UUID)`, resolving the id
      against `mirror_members`. Today mentions vanish on read; this makes all 48
      visible in the interior for the first time, and makes them movable rather
      than merely survivable.
- [ ] Emit Plane's `taskList` / `taskItem` shape on render so Plane's own editor
      agrees with ours about what a checkbox is. **Unverified and worth one live
      test:** whether Plane's TipTap renders our `<ul><li><input type="checkbox">`
      as checkboxes or drops the inputs. The Logbook is the heat source, so this
      matters more than it looks.

### Phase 3 — the write path

- [ ] `UpdateThread` splices instead of re-rendering — this is the fix for the
      live data-loss bug, and it lands for MCP and CLI at the same time.
- [ ] Add the `dod` region; add per-region base hashes and 409 on conflict.
- [ ] `ToggleTodo(region, index, text, done)` — **guarded by text, not index
      alone**. A stale index must refuse rather than tick the wrong box. This is
      the same bug class PLANE-SYNC kept hitting: a local copy confidently
      answering a question it does not actually know.
- [ ] Surface parity: REST + `bubble thread edit` / `bubble thread todo` + MCP
      (`update_thread` gains `dod`, new `toggle_todo`).

### Phase 4 — the editor

- [ ] `ArtifactEditor.svelte` — a markdown textarea, autosave per the rules above,
      dirty/saving/saved/conflict states.
- [ ] Reuse the comment composer's `wrapSel` / `prefixLines` / `insertLink`, which
      already exist in `ThreadView.svelte` and do exactly this job at small scale.

### Phase 5 — slash commands

- [ ] `/` at a line start opens a filtered command list at the caret. Caret
      coordinates in a textarea need the hidden-mirror-div measurement; if that
      turns fiddly, CodeMirror 6 gives `coordsAtPos` and markdown highlighting for
      ~150 KB, lazy-loaded the way mermaid already is.
- [ ] Commands: heading 1–3, bullet, numbered, todo, quote, table, code block,
      mermaid, divider, link.
- [ ] Labels go through `i18n.svelte.ts` in **both** EN and ES — the type forces
      parity, and the last translation pass proved that strings built in script
      are the ones that get missed.

### Phase 6 — inline checkboxes

- [ ] Tick a todo straight from the rendered view. One narrow write, immediate
      heat, independent of everything above.
- [ ] Must update **both** `data-checked` on the `li` and `checked` on the
      `input`: Plane's own bodies carry both, and updating one would make Plane
      and us disagree about the same box.

### Phase 7 — live editing next to agents

- [ ] A dirty region is never clobbered by an inbound SSE `thread` event.
- [ ] "Changed elsewhere" banner; 409 surfaces as a choice, not a silent loss.
- [ ] The case worth demoing: an agent rewrites the Logbook through MCP while the
      Brief is open in the editor, and neither loses anything.

## Open

- Whether Plane's editor renders our checkbox shape (Phase 2) — one live write
  answers it.
- `mirror_members` holds 4 rows against 48 mentions, so some ids will not
  resolve. Fall back to the raw id rather than dropping the mention.
- Ordered-list numbering inside a block the user *did* edit will still be
  normalized by goldmark. Acceptable: you edited it.
