# Bubble Work — Interior & Roles Plan

Companion to [`bubble-work-spec.md`](./bubble-work-spec.md) and
[`IMPLEMENTATION-PLAN.md`](./IMPLEMENTATION-PLAN.md). This covers two new bodies
of work discussed and scoped on 2026-08-02:

- **Roles** — an admin-provisioned, read-only *workspace board* plus read-only
  cross-member viewing for any team member.
- **Interior** — what you see when you open a **bubble** (its thread history)
  and a **thread** (its artifacts, logbook, revisions, and comments), rendered
  through the mds engine.

Phase numbers continue from the existing plan (which reached Phase 7). New work
is **Phases 8–12**.

---

## Locked decisions

| # | Decision | Choice |
|---|---|---|
| 1 | Thread content rendering | **Convert Plane `description_html` → Markdown, render with mds** (read-only in v1) |
| 2 | Bubble detail v1 | **Timeline first** (`git log --oneline` style); tree/graph layered later |
| 3 | Workspace board auth | **Both** — admin kiosk display token **and** a member read-only "view workspace / view member" toggle |
| 4 | Revision artifacts | **Separate Plane work items** linked to the thread via `relates_to`, using a name convention |

## Guiding principles

- **Plane is the system of record.** The interior is a *renderer*, not a second
  editor. v1 is read-only for thread bodies. (Editing = a much later phase.)
- **Read = workspace, write = owner.** Reads become workspace-scoped (any member
  may *see* any board or the whole-workspace board). Every mutation stays
  owner/assignee-scoped and server-enforced, exactly as today. The kiosk board
  and "peek at a teammate" are the same read query at different filter scopes.
- **Convert once, server-side.** HTML→Markdown happens in Go so the mds renderer
  always receives clean, consistent Markdown and there is a single place to
  handle Plane's ProseMirror quirks (task lists, mentions, embeds).
- **The server stays Plane's only client.** All new fetches go through
  `internal/plane` and are cached; the SPA and MCP only ever talk to our server.

---

## Plane API additions (all confirmed available)

New calls to add to `internal/plane/client.go` (current client only fetches
modules, module-issues, cycles, members, projects, states, activities):

| Purpose | Endpoint | Notes |
|---|---|---|
| Full work-item body | `GET {projectBase}/work-items/{id}/` | `description_html`, `state`, `assignees`, `labels`, `priority`, `parent`, `sequence_id`, `sort_order`, `completed_at`, `created_at` |
| Threads in a bubble (full fields) | extend `ListModuleWorkItems` with `fields`/`expand` | need dates, state, assignees, parent, sort_order for the timeline |
| Relations | `GET {projectBase}/work-items/{id}/relations/` | returns `blocking`, `blocked_by`, `relates_to`, `duplicate`, `start/finish_before/after` |
| Comments | `GET {projectBase}/work-items/{id}/comments/` | paginated; HTML content + actor + `created_at` |
| Sub-issues (later) | `parent` field + list children | for the parent-based tree (Phase 10.5) |

**Gotchas to handle in conversion:**
- Plane rich text is **HTML**, not Markdown. Use a Go HTML→MD converter
  (candidate: `JohannesKaufmann/html-to-markdown`) with custom rules for:
  - **Task lists** (the logbook): Plane emits `<ul data-type="taskList">` with
    checkbox inputs → must become GFM `- [ ]` / `- [x]` preserving checked state.
  - **Mentions / embeds / images** → sane Markdown fallbacks (don't crash the
    minimap on unknown nodes).
- **Comments are HTML too** → same conversion path.
- **Member UUIDs** (assignees, comment actors) need name resolution → reuse the
  per-instance members cache from `resolve`.

---

## Naming conventions

- **Revision artifacts.** A revision is its own Plane work item, linked to the
  thread by a `relates_to` relation, with a title prefix: **`rev: <label>`**.
  The server parses `relates_to`, keeps only titles matching `^rev:` (case-
  insensitive), and renders them in the sidebar's bottom slice. Everything else
  in `relates_to` is ignored by the sidebar (still available for the graph
  later). Content is free-form Markdown (converted from HTML).
- **Work artifacts ("files").** The thread's own `description_html` is split on
  top-level `# H1` headings. Each H1 becomes a "file" in the sidebar's top
  slice; a **TOC is mandatory** and is generated from that file's `H2+`
  headings. If there are zero H1s, the whole body is a single implicit file
  ("Brief").
- **Simple vs. complex threads.** Inferred, with an optional explicit override:
  - *Complex (phased)* if the Logbook contains ≥1 phase heading, OR the contract
    overlay marks it phased.
  - *Simple (tasks)* otherwise → the UI hides the phase scaffolding and shows a
    flat todo list. DoD moves **into** the Logbook section for both.

---

## Phase 8 — Interior read model (backend)

**Goal:** the server can serve a thread's full interior and a bubble's full
timeline, read-only, workspace-scoped. No UI yet.

**Plane client (`internal/plane`)**
- `GetWorkItem(ctx, project, id)` → new `WorkItem` fields (description_html,
  state, assignees, labels, priority, parent, sequence_id, sort_order, dates).
- `ListRelations(ctx, project, id)` → `Relations` struct.
- `ListComments(ctx, project, id)` → `[]Comment{ID, Actor, HTML, CreatedAt}`.
- Extend `ListModuleWorkItems` to pull the fields the timeline needs.

**Server (`internal/server`)**
- HTML→Markdown converter package (`internal/md` or similar) with the task-list
  and mention rules above; unit-tested against real Plane HTML samples.
- Artifact splitter: Markdown → `[]Artifact{Title, TOC[], BodyMarkdown}` +
  isolate the Logbook/DoD section.
- Members-name resolver (reuse `resolve`'s per-instance member cache).
- New read endpoints (all behind `restAuth`, workspace-read gated):
  - `GET /api/bubbles/{id}/threads` → timeline: `[]ThreadNode{id, short_id,
    title, state, level, created_at, completed_at, owner, parent, sort_order}`
  - `GET /api/threads/{id}` → `ThreadDetail{meta, artifacts[], logbook, dod,
    kind: simple|phased, revisions[]}`
  - `GET /api/threads/{id}/comments` → chat feed (converted, actor-resolved)
- Cache these with short TTLs (align with existing `bubblesTTL`/`partialTTL`);
  reuse the keep-last-good strategy so a Plane blip never blanks the interior.

**DoD:** given a real module, `GET /api/bubbles/{id}/threads` returns an ordered
timeline and `GET /api/threads/{id}` returns split artifacts + logbook + DoD +
`rev:` revisions, with all UUIDs resolved to names. Covered by tests.

---

## Phase 9 — RBAC & workspace board (backend + minimal UI)

**Goal:** read=workspace / write=owner, plus the two viewing modes.

**Server**
- Split scope enforcement: **reads** widen to any instance/project the caller is
  a member of (`actor.CanSee`), **writes** unchanged (owner/assignee, impersonated
  key). Verify every mutating handler still checks ownership.
- `GET /api/workspace/bubbles?project=&module=&member=` → all bubbles in the
  viewer's permitted workspace(s), read-only, filterable. (Generalizes today's
  `/api/admin/bubbles` without requiring godmode.)
- **Kiosk display token:** admin provisions a server-issued, read-only token
  bound to one workspace (new `bubble serve` admin subcommand or config). It
  authenticates the kiosk screen with *no personal login* and can only hit read
  endpoints — writes/MCP tools reject it.

**Frontend (minimal)**
- View-scope control in the status bar: **mine · workspace · member:▾** (reuses
  the existing instance/project filters). Non-write actions only when scope ≠ mine.
- `?kiosk=<token>` route → boots straight into a read-only workspace board
  (brand bubble + filters, no omnibar write commands, no birth).

**DoD:** a member can switch to a read-only workspace/member view; a kiosk URL
shows the whole workspace board with no login and cannot mutate anything.

---

## Phase 10 — Bubble interior UI (timeline)

**Goal:** clicking a bubble opens its thread history.

- Bubble-detail panel/route driven by `GET /api/bubbles/{id}/threads`.
- **Vertical SVG "commit rail"** (custom, not a git-graph lib): new→old, glowing
  nodes colored by heat/state on a gradient spine, matching the bubble aesthetic.
  Node → title, short-id, owner, state, dates. Click a node → open the thread.
- Reuse the frosted-glass + noise styling already in the board.

**Phase 10.5 (fast follow):** indent the rail by Plane `parent`/sub-issue
hierarchy for a real tree; draw `relates_to`/`blocks` as optional faint edges.

**DoD:** every bubble opens to a readable, time-ordered history; nodes navigate
to threads.

---

## Phase 11 — Thread interior UI (mds file-tree)

**Goal:** the mds-style reading experience for a thread.

- **PaneForge** resizable split: left sidebar + content + minimap.
- Sidebar, two slices:
  - **Top — work artifacts:** the H1-split "files"; selecting one renders it in
    the content pane with a **mandatory TOC** (from its H2+).
  - **Bottom — revision artifacts:** the `rev:`-prefixed related work items.
- **Content pane:** reuse the **mds render engine + minimap** on the converted
  Markdown.
- **Logbook + DoD:** one unified section; phased scaffolding shown only for
  complex threads, flat todos for simple ones.
- **Browser-pinned artifacts:** localStorage; a pin rail that survives reloads
  and is per-viewer (not written to Plane).

**DoD:** opening a thread shows its files with TOCs, the logbook/DoD, revision
artifacts, and pins persist across reloads.

---

## Phase 12 — Comments chat

**Goal:** the bottom-right comments bubble.

- Floating comments launcher → chat-like panel over `GET /api/threads/{id}/comments`.
- Actor avatars/initials, timestamps, converted Markdown, newest-anchored scroll.
- **Read-only first.** Posting a comment (write-back, impersonated key) is an
  optional follow-on once read is solid.

**DoD:** the comments bubble shows the thread's discussion, live-ish (polled like
the board), read-only.

---

## Cross-cutting / open questions

- **HTML→MD fidelity** is the main technical risk — validate the converter
  against your *actual* Plane content early in Phase 8 (task lists especially).
- **Editing** thread bodies from bubble.work is explicitly out of scope for
  v1; revisit after read is proven.
- **New dependencies:** a Go HTML→Markdown lib (Phase 8); `paneforge` +
  (optionally) Zag TreeView on the frontend (Phase 11). The commit rail and
  comments chat are hand-rolled — no new deps.
- **Kiosk token lifecycle** (rotation/revocation) — decide when Phase 9 lands.

## Suggested sequencing

Phase 8 unblocks everything (it's the read model). Then either:
- **Roles-first:** 8 → 9 → 10 → 11 → 12 (get the shared board out early), or
- **Interior-first:** 8 → 10 → 11 → 12 → 9 (make one thread beautiful before
  going multi-viewer).

Recommendation: **8 → 10 → 11** to nail the interior experience on your own
board, then **9** to open it up, then **12**.
