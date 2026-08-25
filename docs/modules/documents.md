# Documents — markdown, owned here

**Status:** built · **Packages:** `internal/store` (`thread_docs`,
`thread_publish`, `thread_docs_prev`), `internal/md` (`AssembleBody`,
`TailMarkdown`), `internal/server` (`artifacts.go`, `importdocs.go`) ·
**Depends on:** [`storage.md`](./storage.md),
[`plane-channel.md`](./plane-channel.md) · **Decision:**
[`0001`](../decisions/0001-plane-is-a-channel-not-the-record.md)

## What it solves

A thread's writing used to live in Plane's `description_html`, borrowed. Every read
was an `md.FromHTML`, every write an `md.RenderPlaneHTML`, and
`internal/md/fidelity.go` exists to measure how much that pair destroys. The
document format was therefore whatever Plane's ProseMirror schema tolerated, and
improving how a document reads meant first asking Plane's permission.

Markdown is now the canonical form and Bubble Work owns it. Plane is a place the
document is *published to* — and still a place it can be *edited*.

## Model (normative)

### The record

```
thread_docs(thread_id, region, markdown, hash, updated_at, updated_by)
    PRIMARY KEY (thread_id, region)
thread_publish(thread_id, published_hash, published_at)
thread_docs_prev(… same shape …, replaced_at)   -- the version an import overwrote
```

`hash` is `md.Hash(markdown)` — the value an editor sends back as `base` to prove it
is writing over what it read. `updated_by` is the Actor's label, or `plane` for an
imported edit, or `adopted` for a bulk adoption.

**There are exactly four stored regions, and only three are writable:**

| Region | Writable | What it is |
|---|---|---|
| `document` | yes (`brief` in the API) | everything before the first special section |
| `logbook` | yes | the plan |
| `dod` | yes | the Definition of Done |
| `tail` | **no** | anything written after the last section |

Named `##` sections are *edits inside the document region*
(`applySectionEdits`), not regions of their own. `tail` exists because
`regionRange` deliberately excludes what follows the Logbook and the DoD — it
belongs to no region, and a store made of the other three would have been a lossy
copy of the page.

`thread_publish` is per thread, not per region: the body is one HTML document
however many regions it is authored in.

### Reading

`storedBody` assembles `document + logbook + dod + tail` with the section headings
put back (`md.AssembleBody`), and `md.ParseThread` reads it exactly as before.
**No Plane HTML is parsed to show a thread.** A thread with no stored rows falls
back to the mirrored body, so nothing is invisible before adoption.

One gap, stated rather than glossed: a **revision**'s body is its own work item and
is still read through the mirror (`revisions()`).

### Writing

`writeBody` is the single choke point, and the order is the whole point of the
inversion:

1. the markdown goes into `thread_docs` (`recordDocs`),
2. Plane is published to (`SetWorkItemBody`),
3. on success, `published_hash` is recorded.

If Plane fails, the write still succeeds: the publication is queued
(`OutDoc`, `FieldLock: "description"`), the mirror keeps our version, and the field
lock stops an incoming sync reverting an edit that is only waiting to be sent. A
second edit **supersedes** the queued publication rather than stacking another —
the intermediate body was never Plane's and nobody is waiting to see it.

`checkBase` compares against the store when the thread has one, because that is
what the read served; comparing against Plane's HTML would invent conflicts
whenever the two representations differ by a byte.

### Editing in Plane

Plane stays a **writable** surface. An edit made there is imported and wins — see
[`plane-channel.md`](./plane-channel.md) for the mechanism and the arbitration
table. The thread then reports `from_plane`, shown as a chip in the interior, a
line in `bubble thread`, and a field in `read_thread`.

### Adoption

`bubble admin adopt <instance>` imports every mirrored body once. Threads that
already have a stored document are skipped, so re-running it cannot clobber newer
writing, and `published_hash` is set from the mirror's hash — Plane already holds
exactly that body, so it is published by definition. Without that, the first sync
after adoption would read every thread as edited in Plane.

### Export

`bubble admin export <instance> [--dir …]` writes the whole tree as markdown
(`<project>/<bubble>/<thread>/{BRIEF,LOGBOOK}.md`, revisions inside their parent,
locally-held pages under `pages/`). It reads the mirror, costs no Plane calls, and
is the backup story: this shipped **before** the read path moved, because a backup
that arrives after the thing it protects is not a backup.

## What this changed elsewhere

| Thing | Before | After |
|---|---|---|
| `md.FromHTML` | every read path | importer only: adoption, imported edits, export |
| `md.RenderPlaneHTML` | the write path to the record | the publish path to a copy |
| `fidelity.Check` | "how much do we destroy when someone edits" | "how faithfully does Plane display what we published" |
| `sync-rebuild` | rebuilds everything | rebuilds structure; **cannot** rebuild bodies |
| Losing `bubble.db` | costs a backfill | costs the writing (hence export) |
| A failed Plane write | failed the edit | queues, and the edit stands |

## Surfaces

Every existing artifact capability keeps its endpoint, CLI command and MCP tool
([`artifacts.md`](./artifacts.md)) — they simply read and write the document store.
New:

| Capability | REST | CLI | MCP |
|---|---|---|---|
| provenance of the body | `from_plane` on `GET /api/threads/{id}` | a line in `bubble thread` | in `read_thread` |
| export | `POST /api/admin/sync/{slug}/export` | `bubble admin export` | — (writes to the server's disk) |
| adopt | `POST /api/admin/sync/{slug}/adopt` | `bubble admin adopt` | — |

## Invariants

1. Markdown in `thread_docs` is the record; Plane's HTML is a rendering.
2. A read of a stored thread parses no Plane HTML.
3. A Plane-side edit is imported, never discarded — and the version it replaced is
   kept in `thread_docs_prev`.
4. A publication is compared against `published_hash`, never against "did this row
   change" — otherwise publishing feeds itself.
5. `thread_docs` rows are pruned only on deliberate deletion, never by a Plane walk.

## Open

- **Revision bodies** are still read from the mirror. A revision is an artifact too,
  so it deserves the same treatment.
- **History.** `thread_docs_prev` holds exactly one version — an undo, not a log.
  Owning the record makes real per-edit history possible for the first time.
- **Publishing is wholesale.** The publish path still renders the assembled body,
  so Plane's own block-level spacing is rewritten on every publish. Cosmetic by
  decision 0001, but it makes Plane's activity feed noisier than it needs to be.
- **A file tree on disk, continuously** (git as the real backup) rather than only on
  `export` — deliberately deferred in
  [`0001`](../decisions/0001-plane-is-a-channel-not-the-record.md).
