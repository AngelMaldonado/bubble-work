# V2 — the build plan

**Status:** planned, nothing built · **Decision:**
[`decisions/0007`](./decisions/0007-bubble-owns-the-record.md) · **Archive:** tag
`v0-plane-as-record`

One binary: PocketBase as an embedded Go framework, Bubble's logic on its router,
markdown on disk under git, MCP as the operator surface. No CLI.

## The two layers this serves

| Layer | Who | Surface |
|---|---|---|
| **Strategic** | the lead — planning, objectives, what earns the organisation something | the planner view: inbox, calendar, high-level kanban, objectives |
| **Operative** | executors working through AI agents | **MCP**, and the buoyancy board |

MCP is the primary protocol for the operative layer. That is why the CLI is not
rebuilt: it was a second door to the same methods, and the door that matters now
is the one agents already speak.

---

## Collections

PocketBase collections. `users` is its built-in auth collection.

### Identity and boundary

| Collection | Fields |
|---|---|
| `users` *(auth)* | built-in, plus `display_name`, `kind` (`human` \| `agent`) |
| `workspaces` | `name`, `slug`, `repo_path` (root of the markdown tree) |
| `memberships` | `workspace` →, `user` →, `role` |

`memberships` is what every API rule reads. Authorization stops being derived from
a mirror of somebody else's membership.

### Structure

| Collection | Fields |
|---|---|
| `objectives` | `key`, `name`, `description`, `order` — the five: clients, software profitability, resource optimisation, ISO 9001, maintenance |
| `bubbles` | `workspace` →, `name`, `outcome`, `owner` →, `closure`, `closed_at`, `stage`, `objectives` →→ |
| `threads` | `workspace` →, `bubble` →, `seq`, `name`, `doc_path`, `state` →, `assignees` →→, `parent` →, `impact`, `urgency`, `due_date`, `born_at`, `completed_at`, `from_inbox` → |
| `states` | `workspace` →, `name`, `group`, `position` |
| `cycles` | `workspace` →, `name`, `start_date`, `end_date` |
| `labels` | `workspace` →, `name`, `color` |
| `thread_relations` | `thread` →, `type`, `related` → |
| `thread_links` | `thread` →, `url`, `title`, `added_by` → |
| `comments` | `thread` →, `author` →, `body` |
| `inbox_items` | `workspace` →, `raw_text`, `author` →, `triaged_to` →, `dismissed_at` |

`impact` and `urgency` are stored; **`priority` is not a column** — it is the map
in [`0007`](./decisions/0007-bubble-owns-the-record.md) applied at read time.

### Evidence

| Collection | Fields |
|---|---|
| `events` | `workspace` →, `target_type`, `target` , `kind`, `actor` →, `at`, `meta` |

Append-only, and the reason heat stops being inferred: with Bubble owning the write
path, a production event is **observed as it happens**. Kinds carry over from v0 —
`thread-born`, `completed-todo`, `logbook-updated`, `body-updated`, `revision-added`,
`link-added`, `thread-completed`, and `comment` as pulse.

At five people this is roughly 18k rows a year, about 4 MB. There is no retention
policy to design.

**Never stored:** `heat`, `priority`, `lifecycle`. All three are pure functions of
rows that are stored plus the current time.

---

## The artifact store

```
<repo_path>/
  <bubble-slug>/
    <thread-slug>.md
  pages/
    <page>.md
```

- The file is the record. `threads.doc_path` points at it.
- Every server write is one git commit, authored as the actor. Git log becomes the
  content history; `events` stays the queryable index.
- One lock per path across read-modify-write. The file and its row are written
  together or not at all.
- `internal/md` is unchanged: it already speaks markdown regions, and the bytes
  simply come from a file instead of a column.

Mermaid support carries over from v0's renderer. Excalidraw is new — sidecar
`.excalidraw` files next to the document, referenced from it.

---

## Phases

Each phase ends somewhere usable. Nothing is built ahead of the phase that needs it,
and no module doc is written before its capability exists.

### Phase 0 — skeleton
`main.go` with `pocketbase.New()` and `migratecmd`. First migration creates
`users` extensions, `workspaces`, `memberships`. API rules read `memberships`.
Superuser bootstrap is PocketBase's own, which removes v0's chicken-and-egg.

**Done when:** a person can sign in, a workspace exists, and the dashboard shows it.

### Phase 1 — structure and the markdown store
Remaining collections. `internal/md` ported verbatim with its tests. The path
resolver: `path → entity`, containment inside a workspace root, per-path locking,
git commit per write.

**Done when:** a thread exists, its `.md` file exists, editing either through the
server keeps them consistent and produces a commit.

### Phase 2 — evidence and the two derived axes
`events` written on every observed production. `internal/heat` ported and pointed
at the event stream. The priority map as a read-time function.

**Done when:** a bubble's temperature and a thread's priority are both computed and
neither is stored anywhere.

### Phase 3 — MCP
Route on PocketBase's router. File-shaped tools — read, write, patch by context,
list, search, move — with belonging, base-hash conflicts and containment enforced
by the server, per
[`0007`](./decisions/0007-bubble-owns-the-record.md). v0's four markdown prompts
are ported.

**Done when:** an agent does a real piece of work end to end and the bubble warms
because of it.

### Phase 4 — the operative board
Ported from v0's frontend, served from `pb_public`: `Prose.svelte` and
`lib/prose.ts` (the one renderer), `Omnibar.svelte` with `lib/fuzzy.ts`,
`lib/vim.svelte.ts`, `lib/slash.ts`, `lib/editor.ts`, `ThreadToc.svelte`, and
`Band` / `BubbleCard` / `BubblePool`. Live updates come from PocketBase realtime
instead of the bespoke SSE.

Not ported: `GodMode` (the dashboard replaces it), `McpConnect`,
`ProjectCombobox`, and everything that named an instance.

**Done when:** the board looks and moves like v0's and nobody wrote a second
markdown renderer.

### Phase 5 — the planner view
Inbox, calendar over `due_date` with timeline / week / day modes, the high-level
kanban by objective, and objectives editable in place.

**Done when:** a lead can triage an inbox item into a thread without leaving the
view.

### Phase 6 — migration
Read the two v0 `bubble.db` files through
[`DATABASE.md`](./DATABASE.md): mint local ids, write the markdown tree from
`thread_docs` and `local_pages`, and backfill `events` from the timestamps that
exist — `born_at`, `logbook_at`, `revisions_at`, `links_at`, `created_at`,
`completed_at`. That backfill is lossy and says so; it does not invent history it
does not have.

### Later — Plane as a channel
A channel that synchronises specific things and owns none of them. Deliberately
after everything above, so nothing is shaped around it a second time.

---

## Do this before Phase 0

**Export both live instances.** `thread_docs` and `local_pages` in their
`bubble.db` are the only copy in existence of what was written there. Build v0's
binary from the tag and run its export, and take a raw copy of each file. This is
what makes Phase 6 a migration rather than a loss.

```
git worktree add /tmp/bubble-v0 v0-plane-as-record
cd /tmp/bubble-v0 && go build -o bubble ./cmd/bubble
```

## Open

- Who commits, and how often. One commit per write is the plan; whether an agent's
  rapid edits should be squashed per session is not decided.
- Whether a page and a thread's document are the same thing on disk. v0 kept two
  stores for one renderer; the filesystem may make that distinction pointless.
- Excalidraw's editing surface — a sidecar file is decided, the editor is not.
- Whether `cycles` still earns its place now that no upstream tool supplies them.
