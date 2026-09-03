# Storage — one SQLite file, two very different halves

**Status:** built · **Packages:** `internal/store`, `internal/mirror` ·
**Depends on:** nothing · **History:**
[`journal/PLANE-SYNC.md`](../journal/PLANE-SYNC.md)

## What it solves

Holding state without an external database, so the server stays a single static
binary that anyone can run on a small box.

## Model (normative)

One SQLite file (`$BUBBLE_HOME/bubble.db`, default `~/.bubble/`), WAL mode, **one
connection pool** shared by both halves. Two pools over one file is a lock fight
with itself.

### The two halves

| | What it is | If you delete it |
|---|---|---|
| **Overlay** | server-owned record | **real work is lost** |
| **Mirror (L1)** | a projection of Plane | nothing is lost; costs a backfill |

**Overlay tables:** `bubble_contracts`, `bubble_state`, `bubble_snapshots`,
`plane_instances`, `instance_capabilities`, `thread_progress` (including `born_at`),
`thread_pulse`, `thread_autostate`, `comment_reads`, `notifications`,
`notification_reads`, `member_prefs`, `server_settings`, `kiosk_tokens`, `outbox`,
`local_pages`, and the document store: `thread_docs`, `thread_publish`,
`thread_docs_prev` ([`documents.md`](./documents.md)).

**Mirror tables:** `mirror_projects`, `mirror_modules`, `mirror_module_items`,
`mirror_items`, `mirror_comments`, `mirror_cycles`, `mirror_states`,
`mirror_labels`, `mirror_item_links`, `mirror_item_relations`, `mirror_members`,
`mirror_project_members`, `mirror_project_member_sync`, `sync_cursors`.

Column by column, with the reasoning attached to each one:
[`DATABASE.md`](../DATABASE.md).

### What is precious, and why it changed

The mirror is rebuildable by definition: `bubble admin sync-rebuild` drops it and
walks Plane again. That is what keeps the central claim honest — the mirror is a
cache, not a second record.

The overlay never was rebuildable, and three kinds of row make that acute:

- `local_pages` — where the instance's Plane cannot hold pages, this is the only
  copy in existence ([`pages.md`](./pages.md)).
- `thread_docs` — per
  [`decisions/0001`](../decisions/0001-plane-is-a-channel-not-the-record.md), the
  canonical body of every artifact.
- `thread_progress.born_at` — the one timestamp nothing can reconstruct: no body
  says "somebody created this here". `SaveThreadProgress` will only ever SET it, so a
  routine sweep cannot clear what it does not know about.

So the honest statement of the backup story is: **losing `bubble.db` loses
writing.** Three things stand between that and a disaster, and all three are
obligations rather than niceties — the Plane copy of every published document,
`bubble admin export` writing the markdown tree to disk, and ordinary file backups
of one SQLite file.

### Pruning

The overlay is keyed by Plane id, so an id Plane no longer has must not keep
overlay rows: otherwise a deleted item leaks progress timestamps forever, and — were
an id ever reused — a new thread would inherit a stranger's history and be born
warm.

Two rules keep that safe:

1. Prune only after a **complete** walk. A partial pass proves nothing about
   absence.
2. Never prune what only we hold. `Prune` names its tables explicitly, and
   `thread_docs`, `thread_docs_prev`, `thread_publish` and `local_pages` are not among
   them — they are cleared by `ForgetThreads`, which runs when somebody deliberately
   deletes a thread or its workspace. For the pruned tables a wrong prune costs a
   timestamp the next sweep re-derives; for these it would cost the work.

### Concurrency

WAL, one pool, and a regression test that hammers concurrent reads against a write
— the failure it guards is a reader blocking behind the sync worker mid-page-load.

## Surfaces

Admin: `bubble admin sync <inst>` (census, no Plane calls), `sync-rebuild`,
`outbox`, `stats`. Everything else touches storage only through server methods.

## Invariants

1. One file, one pool, WAL.
2. The mirror is a cache: dropping it must never lose anything.
3. The overlay is the record: it must be backed up.
4. Prune only after a complete walk, and never rows with no upstream copy.

## Open

- No automatic backup or retention. `bubble admin export` exists and is the honest
  answer, but running it is still somebody's cron job.
- Nothing verifies a backup. An export that silently wrote an empty tree would look
  exactly like a successful one until it was needed.
