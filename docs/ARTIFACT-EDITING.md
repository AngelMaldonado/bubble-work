# Artifact editing — writing a thread's page from the board

Status: **Phases 0-6 shipped · 7 started · editing, renaming and deleting all live** · Drafted 2026-08-07 · Companion
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

### Phase 2 — Plane's node vocabulary, both directions ✅

`RenderHTML` renders for **our** view and is unchanged. A new `RenderPlaneHTML`
renders for **Plane**, mapping our markdown markers back onto Plane's own nodes.
Splicing, `birth`, `add_revision` and comment posting all write through it. It
re-serialises through the HTML parser, which is free here: a write produces new
bytes either way, and splicing is what preserves the old ones.

- [x] `image-component` ↔ `![](plane-asset:UUID)`. A real external URL stays an
      ordinary `<img>`.
- [x] `mention-component` ↔ `[@mention](plane-mention:UUID)`. The label is
      decoration — only the link target survives a write — so any surface that
      knows the person's name may substitute it freely.
- [x] Plane's `taskList` / `taskItem` shape on write, so a Logbook we save is a
      Logbook Plane can render **and tick**. Not cosmetic: a checkbox Plane drops
      is a todo that stops counting, and ticked todos are the primary evidence of
      production.
- [x] `rewriteMentions` in the display path, alongside `rewriteAssets` — and the
      Logbook now goes through both, which it never did before (an image in a
      plan rendered as a broken `<img>`). An unresolved id degrades to
      "@someone" rather than vanishing again.
- [x] **`briefLogbookHTML` renders Markdown.** It used to HTML-escape the birth
      artifacts and turn newlines into `<br/>`, so a birthed Logbook was one long
      paragraph — no headings, and no checkboxes for the todos. Both fields are
      Markdown by definition (§ thread birth rule), and a todo Plane cannot
      render is a todo nobody can tick.

**Measured**, same 96 bodies:

| | before | after |
|---|---|---|
| mentions destroyed by a write | 48 | **0** |
| images broken by a write | 193 | **0** |
| bodies splicing back byte-identically | 96/96 | **96/96** |
| whole bodies surviving a round trip | 90/96 | 90/96 — the same six |

`Fidelity` now *measures* what a write loses by round-tripping and counting what
came back, rather than counting what is present and assuming the worst.

One bug worth naming: the task-item transform trimmed the leading space off every
text node instead of only the separator goldmark puts after a checkbox, so
`**CREANDO** NO` became `**CREANDO**NO`. It cost three bodies of round-trip
stability and is pinned as a test.

Still open: whether Plane's editor renders **our** old plain
`<ul><li><input type="checkbox">`. Emitting Plane's own shape sidesteps the
question for everything written from here on, but bodies written before this
still carry the plain shape.

### Phase 3 — the write path ✅

- [x] **`UpdateThread` splices instead of re-rendering.** This retires the live
      data-loss bug, for the web, the CLI and MCP at once: editing a Logbook no
      longer destroys the images and mentions elsewhere on the page.
- [x] It reads the **current** body from the mirror rather than trusting a copy
      the caller loaded, so two people editing different regions cannot lose each
      other's work.
- [x] The `dod` region, alongside `brief` (which is the API's word for the
      document) and `logbook`.
- [x] **Per-region base hashes, 409 on conflict.** An absent base means "I did
      not read the page first" — legitimate for CLI and MCP writes — and is
      accepted rather than guessed at. `ThreadDetail.regions` carries the exact
      markdown a splice will diff against, plus its hash.
- [x] **A no-op save costs nothing.** Submitting a region unchanged makes no
      Plane call at all. Autosave fires on focus and blur; if that wrote, it
      would burn rate budget and, for a Logbook, stamp production for work nobody
      did.
- [x] **Only production rebuilds the board.** A Brief edit cannot change any
      band, so it broadcasts to the open thread and skips the instance rebuild.
      Logbook and DoD edits still rebuild.
- [x] `ToggleTodo(region, index, text, done)` — **guarded by text, not index
      alone**. A refused toggle writes nothing. This is the same failure the
      mirror work kept hitting: a local copy confidently answering a question it
      does not actually know. Ticking the wrong box is worse than failing,
      because it is silent *and* it manufactures evidence of production.
- [x] Surface parity: `PATCH /api/threads/{id}` and `POST /api/threads/{id}/todo`
      · `bubble logbook` / `bubble dod` / `bubble todo` · MCP `update_thread`
      (gains `dod`) and a new `toggle_todo`.

`TestArtifactWritesAreSplicedGuardedAndIdempotent` asserts all five guarantees
end to end through the real HTTP surface against a fake Plane: the no-op save
writes nothing, a Logbook edit leaves the mention and image untouched, a stale
base hash 409s, a mismatched todo text 409s **and writes nothing**, and the DoD
is writable in its own right.

### Phase 4 — the editor ✅

- [x] `ArtifactEditor.svelte` — markdown source, autosaved, with
      clean/dirty/saving/saved/conflict/error states and ⌘S to force a write.
- [x] Autosave as specified: 1.5 s idle, a 10 s floor between writes so holding a
      key down cannot outrun it, and a flush on blur, unmount and `beforeunload`.
      Unchanged text is never sent at all — the server would no-op it anyway, but
      not sending is what keeps focus and blur free.
- [x] Every write carries the region hash and **adopts the server's** afterwards:
      the server is the authority on what the body now is, and the next write has
      to be based on that rather than on what we sent.
- [x] The toolbar reuses the comment composer's `wrapSel` / `prefixLines` /
      `insertLink` shape, which already did this job at small scale.
- [x] Editing the Logbook shows **two** editors — the Definition of Done is its
      own region and is written separately. Revisions are separate work items
      with no region at all, so they stay read-only here.
- [x] Kiosk displays never see the edit toggle.

Two bugs worth naming, both caught before they shipped:

- **`state` shadowed the `$state` rune**, which cascaded into nine type errors.
  Exactly the shadowing that bit `t` during the translation pass; renamed to
  `status`.
- **Typing during an in-flight save was marked saved.** The post-`await` `text`
  is not what the server received, so those keystrokes would have been recorded
  as written and nothing scheduled to write them — they'd have survived only
  because of the blur flush. The buffer being sent is captured up front now, and
  a divergence re-schedules.

### Phase 7 — live editing next to agents (started)

- [x] **A dirty buffer is never repainted over.** The SSE `thread` event checks
      the editors first; when one has unsaved work it raises a "changed
      elsewhere" banner with a *Load theirs* action instead of reloading. An
      agent editing the Logbook while somebody has the Brief open is the exact
      case this surface exists for, and losing their typing to it would be the
      worst possible answer.
- [x] A 409 is surfaced as a choice, not a silent loss.
- [ ] Demo pass: agent rewrites the Logbook through MCP while the Brief is open
      in the editor, and neither loses anything.

### Phase 5 — slash commands ✅

- [x] `/` opens a filtered list **at the caret**, which is the whole difference
      between this and a command palette. `lib/caret.ts` does the hidden-mirror
      measurement — a `<textarea>` will not tell you where its caret is, so the
      standard answer is a hidden div with the same typography and box metrics.
      No CodeMirror needed, and no 150 KB.
- [x] Twelve commands: heading 1–3, bullet, numbered, to-do, quote, code block,
      diagram, table, divider, link. Each inserts plain markdown, because
      markdown is what the buffer *is* — there is no hidden document model that
      could disagree with the text on screen.
- [x] ↑↓ to move, ⏎/⇥ to insert, Esc to dismiss, blur to close. Aliases so
      "checkbox" finds the to-do and "diagram" finds mermaid.
- [x] Labels go through `i18n.svelte.ts` in both EN and ES, with **literal**
      keys rather than `t('cmd.' + id)`: a computed key is exactly the shape that
      silently stops being translated.
- [x] `lib/slash.ts` is split out and unit-tested. The boundary rule is what
      earns it: a URL is mostly slashes, and `https://example.com/x` must never
      open a menu.

### Phase 6 — inline checkboxes ✅

- [x] The server primitive landed in Phase 3: `ToggleTodo` +
      `POST /api/threads/{id}/todo` + `bubble todo` + the `toggle_todo` MCP tool.
- [x] Tick a todo straight from the rendered view. One narrow write, immediate
      heat. The client works out *which* item was clicked and sends its text; the
      server refuses an index whose text no longer matches, so a moved list
      cannot tick the wrong box — it says so and reloads.
- [x] The Logbook and the DoD render as separately tagged regions, because their
      todos are numbered independently and the click handler has to know which
      list it just counted.
- [x] **goldmark writes `<input type=checkbox disabled>`, and a disabled input
      receives no mouse events at all** — the control would have looked live and
      been dead. The rendered boxes are re-enabled after every paint. Plane's own
      taskList shape is not disabled, hence "if present".

Writing both `data-checked` on the `li` and `checked` on the `input` turned out
to be free: the toggle rewrites markdown and re-renders through
`RenderPlaneHTML`, which emits Plane's shape with both (Phase 2).


## Deleting

Permanent, in Plane, guarded by an explicit confirmation and open to any member
— the same reach as close and reopen. §5.3 still prefers a bubble dying by being
CLOSED, which keeps the record of what was done; this is for the things that
should never have existed.

| | takes | leaves |
|---|---|---|
| **bubble** | the Plane module | its threads — a module is a grouping, not a container, so they survive belonging to no bubble, which also removes them from the board |
| **thread** | its Brief, Logbook, comments and revisions | nothing |
| **artifact** | one region, heading and all | the thread, and every byte outside that region |

Children are deleted **explicitly and first** rather than trusting Plane to
cascade: whether it orphans or removes them is its business, and leaving that
undecided would leave sub-items pointing at a parent that no longer exists.

Every delete clears the **mirror** and the **overlay** too, for the same reason
creates write through — a projection that still lists a deleted thread is the
local copy being confidently wrong. The overlay matters more than it looks: a
stale `thread_progress` row is the baseline that answers "has this produced
anything since we last looked", so a reused id would be measured against a dead
thread's history.

The confirmation says what goes and what stays rather than "are you sure?", and
for a bubble it counts the threads it will unbubble — the consequence nobody
expects. Focus lands on Cancel, so an errant Enter destroys nothing.

Surface parity: `DELETE /api/bubbles/{id}`, `DELETE /api/threads/{id}`,
`DELETE /api/threads/{id}/regions/{region}` · `bubble delete <kind> <id> [-y]`,
which prompts unless `-y` · MCP `delete_bubble`, `delete_thread` and
`delete_artifact`, each described to agents as irreversible and told to ask the
person first.

## Open

- Whether Plane's editor renders our checkbox shape (Phase 2) — one live write
  answers it.
- `mirror_members` holds 4 rows against 48 mentions, so some ids will not
  resolve. Fall back to the raw id rather than dropping the mention.
- Ordered-list numbering inside a block the user *did* edit will still be
  normalized by goldmark. Acceptable: you edited it.
