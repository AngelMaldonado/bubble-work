# 0001 — Plane is a channel, not the record

Status: **accepted and built**, with one part deliberately narrowed after the fact —
see [§Narrowed](#narrowed-plane-stays-a-writable-surface) · 2026-08-15 ·
Supersedes the spec's §9.2 for artifact bodies · Implemented by
[`modules/documents.md`](../modules/documents.md)

## Context

Until now Plane owned the writing. A thread's Brief and Logbook lived in the work
item's `description_html`, and the server borrowed them: every read went through
`md.FromHTML`, every write through `md.RenderHTML` / `md.RenderPlaneHTML`.

That made Plane's editor the arbiter of what a document *is*. Three consequences
followed, and all three are now visible in the code:

1. **The round trip is lossy by construction.** `internal/md/fidelity.go` exists
   only to measure how much of a body survives a read-then-write cycle, and
   `bubble sync fidelity` reports the number over a whole instance. A measurement
   tool for data loss is an admission that the model loses data.
2. **Plane's node vocabulary leaks into ours.** `internal/md/plane.go` translates
   `<image-component>`, `<mention-component>` and `<ul data-type="taskList">`,
   because those constructs have no GFM spelling. Markdown carries them as
   markers so they can be rendered back into something Plane will accept. Our
   document format is therefore shaped by an editor we do not control.
3. **Rendering is negotiated, not decided.** How a document *looks* depends on
   what Plane's ProseMirror schema tolerates, so improving the reading experience
   means first asking what Plane will keep.

There is already a precedent for taking a document type back. Plane Community
serves pages only on its internal, session-authenticated API, so
[`PAGES-CAPABILITY.md`](../journal/PAGES-CAPABILITY.md) made the server the record
for pages on those deployments — `local_pages` lives in the overlay, because the
alternative was not "Plane holds it" but "nobody does".

## Decision

**Artifact bodies are markdown, owned by the server. Plane receives a copy.**

- The canonical body of every artifact — Brief, Logbook, revision write-ups — is
  markdown stored in the **overlay** (`thread_docs`), not a projection of
  anything. It is authored, spliced and read as markdown end to end.
- Plane still gets the content: the outbox pushes a best-effort rendered copy
  into the work item's `description_html`, so Plane stays a readable mirror, a
  backup, and the place where people who do not use Bubble Work can follow along.
- Plane keeps what it is genuinely the best owner of: **projects, modules,
  cycles, states, members, and comments**. Identity and structure are not
  duplicated (spec §8), and a comment is pulse rather than a document, so its
  rendering fidelity does not matter the way an artifact's does.
- The markdown store is the only canonical copy, but not the only copy: Plane
  holds a rendered one, and `bubble admin export` writes the tree to disk for
  anyone who wants git to be their backup.

### Narrowed: Plane stays a writable surface

The first version of this decision said the server always wins: a body edited in
Plane would be snapshotted, flagged, and overwritten. **That was reversed before
it shipped.** What Bubble Work takes is authority over the *format* and over where
documents are authored — not the exclusive right to write.

So, on the next sync after somebody edits a work item in Plane's own editor:

1. the previous version is snapshotted into `thread_docs_prev` — an undo, not a
   history,
2. Plane's version is imported (`md.FromHTML`) and **becomes** the record,
3. the region is marked as coming from `plane`, which the interior shows,
4. `published_hash` is set to the incoming hash, which is what stops the same edit
   being imported again.

The outbox arbitrates the only ambiguous case, and it needed no new mechanism:

| Situation | Winner |
|---|---|
| Plane edited, no publication queued | **Plane** — import it |
| Plane edited, our publication queued (`FieldLock: "description"`) | **local** — the shield holds, ours lands, Plane converges |
| Plane unchanged since our publish | nothing to do |

**The cost, stated plainly, because this decision was written to avoid it:**
importing runs `md.FromHTML`, so a Plane-side edit takes exactly the fidelity loss
described above. What changed is the blast radius — that loss now applies only to
bodies actually edited in Plane, instead of to every read and write of every
thread. Losing somebody's edit is worse than importing it imperfectly.

## Consequences

**The mirror stops being uniformly rebuildable.** `PLANE-SYNC.md` §9.6 promised
that deleting the mirror costs a backfill and nothing else. That stays true for
structure, states, members and comments. It is **no longer true for bodies**:
`bubble admin sync-rebuild` cannot reconstruct a Brief, because Plane's copy is
downstream. The distinction is now load-bearing and documented in
[`modules/storage.md`](../modules/storage.md).

**Fidelity is demoted from a correctness problem to a publication problem — for
everything except an imported edit.** `md.FromHTML` is now an *importer*: it runs on
adoption (`bubble admin adopt`) and when a Plane-side edit is taken, not on every
read. `md.RenderPlaneHTML` stays on the publish path. What `fidelity.Check` measures
changes meaning accordingly: not "how much do we destroy when someone edits" but
"how faithfully does Plane display what we published, and what would an import cost
if somebody edited it there".

**SQLite becomes precious.** Before this decision, losing the overlay lost the
Bubble Work layer and the work survived in Plane. After it, losing the overlay
loses writing. That raises three obligations, none of them optional: the export
command, a documented backup story, and the Plane copy being genuinely complete
rather than a summary.

**Adoption is a migration, not a one-way door.** `bubble admin adopt <instance>`
imports every mirrored body into the document store, once, and skips threads that
already have one so re-running it cannot clobber newer writing. Because Plane stays
writable, adopting does *not* take anything away from anyone — it changes which copy
is canonical and which is a rendering.

**Rejected en route, and worth recording:** the plan sketched `published_hash` as a
column on `thread_docs`. It ended up as its own table (`thread_publish`), because a
publication is per THREAD — the body is one HTML document however many regions it is
authored in — and a per-region copy of the same hash would have been three chances
to disagree with itself.

**One more thing the store had to hold.** Region markdown excludes anything written
after the last section, because `regionRange` stops there deliberately. A store made
of the three regions alone would have been a lossy copy of the page, so there is a
fourth, non-editable stored region: `tail`.

## Alternatives rejected

| Alternative | Why not |
|---|---|
| Keep Plane canonical, improve the round trip | The loss is structural, not a bug backlog. Plane's schema will keep deciding what a document may contain. |
| ~~Re-import Plane edits as canonical~~ | Originally rejected for preserving the fidelity loss. **Adopted anyway** — see §Narrowed. The loss is real; losing an edit is worse. |
| Files on disk as canonical, SQLite as index | Most git-native, and closest to the spec's original file-native promise — but it moves write authority out of the server, which is where the policy engine lives, and makes agent concurrency the central problem. Kept as a possible later step; `bubble admin export` is the cheap half of it. |
| Silent overwrite of Plane-side edits | Simpler to explain, but destroys work without telling anyone. |
| Server always wins, divergence flagged | What this decision originally said. Reversed: it makes Plane read-only in practice, and a tracker people cannot write in is a tracker they stop opening. |
