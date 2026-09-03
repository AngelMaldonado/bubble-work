# The database — every table, and why it exists

**Scope:** the complete schema of `bubble.db`, table by table. What each half is
*for* is [`modules/storage.md`](./modules/storage.md); this is the reference you
read with a query in your hand.

Two source files define it, and both apply their own DDL on open:
`internal/store/store.go` (the overlay) and `internal/mirror/mirror.go` (the
mirror). There is no migration tool and no schema-version row — see
[Migrations](#migrations).

## The file

One SQLite file at `$BUBBLE_HOME/bubble.db` (default `~/.bubble/`), opened once
into **one connection pool** shared by both halves. Two pools over one file is a
lock fight with itself.

| Pragma | Value | Why |
|---|---|---|
| `journal_mode` | `WAL` | readers do not block the writer, the writer does not block readers. Verified after open, not assumed: a pre-existing file only converts on the first successful call, and silently staying in rollback-journal mode surfaces later as intermittent `SQLITE_BUSY`. |
| `busy_timeout` | `5000` | SQLite still allows one writer at a time; the loser waits for the lock instead of failing instantly. |
| `synchronous` | `NORMAL` | syncs at checkpoints rather than every commit. Exposure is the last commits on an **OS** crash, not a process crash. |

`MaxOpenConns(16)`, `MaxIdleConns(8)` — a bound on file handles and memory, not a
serialization device.

## The two halves

| | What it is | If you delete it |
|---|---|---|
| **Overlay** (19 tables) | the server's own record | **real work is lost** |
| **Mirror / L1** (14 tables) | a projection of Plane | nothing is lost; costs a backfill |

`bubble admin sync-rebuild` drops the mirror and walks Plane again. That is what
keeps the central claim honest: the mirror is a cache, not a second record.

## Reading the ids

The vocabulary maps onto Plane, and the schema is only legible once you hold the
mapping:

| Bubble Work | Plane | Mirror table |
|---|---|---|
| Workspace | Project | `mirror_projects` |
| Bubble | Module | `mirror_modules` |
| Thread | work item | `mirror_items` |
| Cycle | Cycle | `mirror_cycles` |

Two id shapes live in this file, and the difference is deliberate:

- **Namespaced** — `"<instance-slug>:<project-id>:<module-id>"` for a bubble,
  `"<instance>:<project>:<work-item>"` for a thread on the wire. Used wherever a
  row is scoped to a deployment: `bubble_contracts`, `bubble_state`,
  `local_pages.id`.
- **Bare Plane uuid** — every thread-keyed overlay table (`thread_progress`,
  `thread_docs`, `thread_pulse`, `thread_autostate`, `thread_publish`,
  `thread_docs_prev`). Safe because a Plane work-item id is a uuid, so it cannot
  collide across instances; the cost is that these rows do not say which
  deployment they belong to, and a query that needs to know must join through the
  mirror.

Mirror tables all carry `instance` in the primary key
([`modules/federation.md`](./modules/federation.md)).

---

# Overlay — the record

## Structure and identity

### `plane_instances`
One row per Plane deployment this server talks to. **Holds credentials.**

| Column | Purpose |
|---|---|
| `slug` PK | `"ayetec"`, `"cuby"` — the first segment of every namespaced id |
| `name` | display |
| `base_url` | e.g. `https://plane.ayetec.space` |
| `api_key` | the service credential, **at rest in plain text** |
| `workspace` | Plane workspace slug |
| `project` | pinned project id, or `''` for the whole workspace |
| `webhook_secret` | HMAC secret for inbound Plane webhooks |
| `auto_state` | write derived levels back to Plane? (off by default) |

### `instance_capabilities`
What a *deployment* can do, cached because probing costs a request against the
60 req/min budget and the answer is a property of the deployment, not of the
request.

`(slug, capability)` PK · `supported` · `checked_at`. Today the only capability
is `'pages'`. A `no` is re-probed in case the instance is upgraded; a `yes` cannot
become false. The same reasoning now applies to links and relations at runtime —
see [Open](#open).

### `bubble_contracts`
The Bubble contract: the part of a bubble Plane cannot express.

`bubble_id` PK (namespaced) · `outcome` · `owner` · `closure` · `closed` ·
`stage` (`''` | `'reviewed'`, the explicit stage overlay).

**Last-write-wins.** Redefining a bubble leaves no trace of what it used to be.

### `bubble_state`
Last-known lifecycle per bubble, so the notification sweep can detect a
*transition* rather than re-announcing a state. `bubble_id` PK · `lifecycle` ·
`updated_at`.

### `bubble_snapshots`
The materialized read model: one row **per instance**, `bubbles` holding a
JSON-encoded `[]domain.Bubble`, plus `updated_at`. Serves board reads with no
Plane call and no re-derivation.

Opaque by construction — not queryable, not paginable, rewritten whole on every
refresh, and sized by the entire workspace rather than by what was asked for.

## Evidence and progress

### `thread_progress`
The change detector heat is derived from. One row per work item, **bare** id.

| Column | Purpose |
|---|---|
| `thread_id` PK | Plane work-item id |
| `logbook_hash` | fingerprint of the Logbook + DoD text |
| `body_hash` | fingerprint of the **whole** document, so an edit anywhere counts as production ([`0004`](./decisions/0004-writing-is-the-work.md)). Kept alongside `logbook_hash` because the difference is what labels the evidence: a plan change or a document change. |
| `logbook_kind` | what the last change was (an `Ev*` kind) |
| `done_todos` | ticked items last time we looked — tells a tick from a re-plan |
| `revisions` | sub-work-items (revision artifacts) last time we looked |
| `links` / `links_at` | Plane links on the item: publishing external evidence is production ([`0006`](./decisions/0006-plane-holds-the-relationships.md)). Counted, not hashed. |
| `logbook_at` | RFC3339 of the last observed Logbook **change** (`''` = never) |
| `revisions_at` | RFC3339 of the last observed revision **added** |
| `born_at` | when the thread was born **here**. Empty for a work item that merely appeared in Plane: created is not born ([`0002`](./decisions/0002-birth-is-production.md)). |

Two properties are worth stating plainly, because everything downstream inherits
them:

- `born_at` is the one timestamp nothing can reconstruct — no body says "somebody
  created this here". `SaveThreadProgress` will only ever **set** it, so a routine
  sweep cannot clear what it does not know about.
- The rest is a **snapshot, not a log**. One timestamp per kind, per thread. The
  evidence stream heat consumes is rebuilt each pass in `server/progress.go` by
  diffing this against current reality, so ten edits in a cycle are one event,
  authorship never reaches the event, and a change that is reverted between two
  sweeps never happened.

### `thread_pulse`
Comment presence, budgeted. `thread_id` PK · `last_comment_at` (newest comment
seen) · `checked_at` (last time we looked, for probe budgeting). Pulse is
presence, not output: it stops a bubble being called 🪦 and never warms it
([`0004`](./decisions/0004-writing-is-the-work.md)).

### `thread_autostate`
Provenance for state we wrote back to Plane. `thread_id` PK · `state_id` (the
state **we** last wrote) · `written_at` · `handed_off` (1 = a human overrode us;
never touch it again).

### `comment_reads`
Per-reader comment read state, because Plane has no comment-reactions API.
`(instance, comment_id, reader_id)` PK · `reader_name` (display name at read
time) · `read_at`.

## The documents — the precious half

Per [`0001`](./decisions/0001-plane-is-a-channel-not-the-record.md) these rows
are the **record**. Plane holds a rendered copy and the mirror can be rebuilt from
Plane, but nothing upstream can reconstruct these.

### `thread_docs`
`(thread_id, region)` PK · `markdown` (canonical content) · `hash`
(`md.Hash(markdown)`, the optimistic-write base) · `updated_at` · `updated_by`
(the Actor, or `'plane'` for an imported edit).

Exactly three regions — `document`, `logbook`, `dod` — the splice engine's.
Named `##` sections are edits **inside** the document region, not regions of their
own.

### `thread_publish`
What we last published to Plane, **per thread** rather than per region: the body
is one HTML document however many regions it is authored in. `thread_id` PK ·
`published_hash` (`mirror.HashBody` of the HTML we sent) · `published_at`.
Comparing Plane's current hash against this is how a Plane-side edit is detected
without mistaking our own publish for one.

### `thread_docs_prev`
The version an import **replaced**. Same columns as `thread_docs` plus
`replaced_at`, `(thread_id, region)` PK.

This is an **undo, not a history** — one row per region, overwritten by the next
import. It exists because a Plane-side edit wins, and winning must not mean the
other version is gone. Two imports in a row and the original is gone anyway.

### `local_pages`
A Workspace's standing documentation, where the instance's Plane cannot hold it.
Plane Community exposes pages only on its internal session-authenticated API, so
on those deployments **this is the only copy in existence**
([`journal/PAGES-CAPABILITY.md`](./journal/PAGES-CAPABILITY.md)).

`id` PK — `"<instance>:<project>:loc_<uuid>"`, and the `loc_` prefix is what tells
a read which backend owns the page · `instance` · `project` · `title` · `body`
(markdown, the shape the editor writes) · `locked` · `archived` · `created_at` ·
`updated_at` · `author` · `parent` (namespaced id of the page above it, `''` at
the root).

Index: `idx_local_pages_project (instance, project)`.

## Delivery

### `outbox`
Writes that have **not** reached Plane. Lives with the overlay and deliberately
not in the mirror: `mirror.Reset` and `sync-backfill` drop mirror tables on
purpose, and losing these loses real work.

| Column | Purpose |
|---|---|
| `id` PK | autoincrement |
| `instance`, `target_id` | which deployment, which work item |
| `kind` | `'state'` (auto-drains) \| `'comment'` (a draft) |
| `payload` | JSON, shape depends on kind |
| `author_email` | whose draft. Comments carry no credential — that would put every user's Plane key at rest — so a draft can only be re-sent by its author, with their live key. |
| `field_lock` | the mirror column an incoming sync must **not** overwrite while this is pending, so a sync cannot revert a write that has not landed |
| `status` | `pending` \| `abandoned` |
| `attempts`, `next_at`, `last_error` | backoff for the auto-drain |
| `created_at` | |

Indexes: `idx_outbox_pending (status, instance)`,
`idx_outbox_target (instance, target_id, status)`.

## People and settings

### `notifications`
Append-only feed. `id` PK autoincrement · `created_at` · `instance` · `bubble_id`
· `bubble_name` (denormalized, so a deleted bubble still reads) · `kind` ·
`message`.

### `notification_reads`
`(email, notification_id)` PK · `read_at`. Keyed by **email** — the stable
per-person key across instances, since a Plane user id is per-deployment.

### `member_prefs`
`email` PK · `notify_enabled`.

### `server_settings`
`key` PK · `value`, opaque JSON owned by the caller. Today: `"tuning"`, the
buoyancy calibration.

### `kiosk_tokens`
Server-issued read-only display credentials. `token` PK · `instance` (the one
slug this token may view) · `name` (human label, e.g. "lobby screen") ·
`created_at`.

---

# Mirror — the cache

Every table is keyed by `(instance, …)`. A single sync worker is the only writer;
no read path touches Plane
([`journal/PLANE-SYNC.md`](./journal/PLANE-SYNC.md)).

## Structure (slow cadence)

| Table | PK | Holds |
|---|---|---|
| `mirror_projects` | `(instance, id)` | Workspaces. `identifier` is Plane's short project key · `synced_at` |
| `mirror_modules` | `(instance, id)` | Bubbles. `project_id` · `name` · `synced_at` |
| `mirror_module_items` | `(instance, module_id, item_id)` | which threads are in which bubble. Index `idx_mirror_module_items_item (instance, item_id)` for the reverse lookup |
| `mirror_states` | `(instance, id)` | `project_id` · `name` (**localized — display only, never matched on**) · `state_group` (`backlog\|unstarted\|started\|completed\|cancelled`, the one that is matched) · `is_default` |
| `mirror_cycles` | `(instance, id)` | `project_id` · `name` · `start_date` / `end_date` stored **verbatim**: Plane emits RFC3339 or a bare `YYYY-MM-DD`, and draft cycles emit null. `plane.Cycle` already parses both leniently, so the mirror does not become a second date parser that could disagree with the first. |
| `mirror_labels` | `(instance, id)` | the project's label catalogue: what each label id on an item actually says. `project_id` · `name` · `color`. Index `idx_mirror_labels_project` |

## Identity and authorization

| Table | PK | Holds |
|---|---|---|
| `mirror_members` | `(instance, id)` | `email` · `display_name` · `role` |
| `mirror_project_members` | `(instance, project_id, member_id)` | who may see **which project**. Plane scopes membership per project and a private project's members are a subset of the workspace's, so scoping the board by workspace alone would show every project to everyone who can log in. |
| `mirror_project_member_sync` | `(instance, project_id)` | one row per project whose membership we have actually **read**. Distinct from the membership itself: a project with no member rows might have no members we know of, or might never have been fetched — and an authorization boundary cannot afford to guess which. |

## Work items (fast cadence)

### `mirror_items`
`(instance, id)` PK. Threads.

`project_id` · `seq` · `name` · `state_id` / `state_name` / `state_group` ·
`priority` · `parent_id` · `assignees_json` · `labels_json` (Plane's label ids;
they ride along in the same work-item payload the delta already reads, so they
cost nothing extra — [`0006`](./decisions/0006-plane-holds-the-relationships.md))
· `description_html` · `description_hash` (lets the syncer tell a body edit from a
comment without a second call, and lets the progress diff run without re-reading
bodies) · `created_at` / `updated_at` / `completed_at` · `synced_at`.

Indexes: `idx_mirror_items_project (instance, project_id)`,
`idx_mirror_items_parent (instance, parent_id)` — the parent index is what makes
revision sub-items countable without a scan.

### `mirror_comments`
`(instance, id)` PK · `item_id` · `actor_id` · `comment_html` · `created_at`.
Index `idx_mirror_comments_item`.

### `mirror_item_links`
External evidence hung off a work item: a commit, a PR, a published deliverable.
`(instance, id)` PK · `item_id` · `url` · `title` · `created_at`. Index
`idx_mirror_links_item`.

Unlike labels these need a call **per item**, so they are filled in on a budget
the same way comments are — `edgeFetchPerPass` bounds them at 8 items a pass,
newest-updated first. Our own writes go straight to the mirror, so the fill-in
exists only to notice links added in Plane's own UI.

### `mirror_item_relations`
Typed relationships: `relates_to`, `duplicate`, `blocking`, `blocked_by`.
`(instance, item_id, relation_type, related_id)` PK. Index
`idx_mirror_relations_item`. Same per-item budget as links.

### `sync_cursors`
`(instance, resource)` PK where resource is `"items"` or `"structure"` ·
`watermark` (newest `updated_at` applied) · `last_full` (last complete reconcile)
· `last_ok` (last successful pass of any kind) · `last_error`.

`last_full` is load-bearing: pruning is only safe after a **complete** walk, since
a partial pass proves nothing about absence.

---

## Pruning

The overlay is keyed by Plane id, so an id Plane no longer has must not keep
overlay rows: a deleted item would otherwise leak progress timestamps forever,
and — were an id ever reused — a new thread would inherit a stranger's history and
be born warm.

1. **Prune only after a complete walk.**
2. **Never prune what only we hold.** `Prune` names its tables explicitly, and
   `thread_docs`, `thread_docs_prev`, `thread_publish` and `local_pages` are not
   among them. They are cleared by `ForgetThreads`, which runs when somebody
   deliberately deletes a thread or its workspace.

For the pruned tables a wrong prune costs a timestamp the next sweep re-derives;
for those four it would cost the work.

## Migrations

There is no migration framework and no schema-version row. The whole strategy is:

- `CREATE TABLE IF NOT EXISTS` for every table, re-applied on every open.
- A list of best-effort `ALTER TABLE` statements whose errors are **discarded** —
  an error on an already-migrated database is the expected case.

Applied today:

| Statement | |
|---|---|
| `plane_instances ADD COLUMN webhook_secret` | |
| `plane_instances ADD COLUMN auto_state` | |
| `bubble_contracts ADD COLUMN stage` | |
| `thread_progress ADD COLUMN logbook_hash` | |
| `thread_progress ADD COLUMN logbook_kind` | |
| `thread_progress RENAME COLUMN todos_at TO logbook_at` | |
| `thread_progress ADD COLUMN born_at` | [`0002`](./decisions/0002-birth-is-production.md) |
| `thread_progress ADD COLUMN body_hash` | [`0004`](./decisions/0004-writing-is-the-work.md) |
| `thread_progress ADD COLUMN links`, `links_at` | [`0006`](./decisions/0006-plane-holds-the-relationships.md) |
| `local_pages ADD COLUMN parent` | |
| `mirror_items ADD COLUMN labels_json` | a row that keeps the default carries no labels until the next pass reads the item again |

This holds only while every change is additive. A change that needs to **rewrite**
rows has nowhere to record that it ran, and re-running it on every open is not the
same thing as running it once.

## Backup

**Losing `bubble.db` loses writing.** Three things stand between that and a
disaster, and all three are obligations rather than niceties:

1. the Plane copy of every published document,
2. `bubble admin export`, which writes the markdown tree to disk,
3. ordinary file backups of one SQLite file.

## Invariants

1. One file, one pool, WAL.
2. The mirror is a cache: dropping it must never lose anything.
3. The overlay is the record: it must be backed up.
4. Prune only after a complete walk, and never rows with no upstream copy.

## Open

- **The evidence is a snapshot, not a log.** `thread_progress` holds one timestamp
  per kind per thread, so density, authorship and any reverted-then-restored change
  are all invisible, and no past cycle can be replayed against a changed calibration.
- **`thread_docs_prev` is one level deep.** Two imports in a row and the version
  before them is gone.
- **`bubble_contracts` is last-write-wins.** Redefining a bubble is a move the model
  recommends and the schema does not record.
- **Overlay keys are Plane ids.** A thread cannot move between projects or
  instances without losing its progress, its documents and its history — Plane's
  module rule limits `move_thread` to one project, and nothing local outlives a
  change of id.
- **`bubble_snapshots` is a JSON blob per instance.** It scales with the whole
  workspace and cannot be queried or paged.
- **Evidence exists only at thread grain.** Publishing a page, recording a decision
  or redefining a contract are production and warm nothing.
- **`instance_capabilities` does not cover links and relations.** Both are now
  probed at runtime and switched off for the rest of a pass on a 404, which is
  correct but re-learned every pass rather than remembered here.
- **`plane_instances.api_key` is at rest in plain text.**
- No automatic backup or retention, and nothing verifies a backup: an export that
  silently wrote an empty tree would look exactly like a successful one until it
  was needed.
