# Bubble V2 — the plan

The model is [`README.md`](./README.md) and has not changed. This is everything
else: what is being built, what was decided and against what, and in what order.

**v0 is archived** at the tag `v0-plane-as-record`. Nothing is lost — its code, its
fourteen module files, its schema reference and decisions 0001–0007 are all
recoverable:

```
git show v0-plane-as-record:internal/md/splice.go
git checkout v0-plane-as-record -- internal/heat
git show 2cf0c71:docs/DATABASE.md        # the v0 schema, table by table
git show 2cf0c71:docs/decisions/         # the numbered decisions, as written
```

## Why v0 ended

Plane held the record. Structure, identity, cycles and states lived upstream, so a
bubble id was the literal string `<instance>:<project>:<module>`, every
thread-keyed row was a bare Plane uuid, and every client read was served from a
fourteen-table mirror of Plane's shape.

Three consequences arrived together, and they were one consequence:

- **A thread could not leave its project.** Plane's module rule limits a move to one
  project, and nothing local outlives a change of id.
- **Evidence had to be inferred.** The writes happened somewhere else, so heat was
  derived by diffing hashes and counters each sync pass: ten edits in a cycle were
  one event, authorship never reached the event, and a change reverted between two
  passes never happened.
- **Authentication *was* a Plane API key**, so "make Plane optional" and "keep
  anyone logged in" were the same problem.

None of that is reachable by extension. **Bubble owns the record**: workspaces,
bubbles, threads, states, cycles, assignment, objectives, priority inputs, labels,
relations, comments and people. Plane becomes one optional channel that owns none
of them.

## The two layers this serves

| Layer | Who | Surface |
|---|---|---|
| **Strategic** | the lead — objectives, planning, what earns the organisation something | the planner view: inbox, calendar, high-level kanban, objectives |
| **Operative** | executors working through AI agents | **MCP**, and the buoyancy board |

MCP is the primary protocol for the operative layer. That is why the CLI is not
rebuilt: it was a second door to the same methods, and the door that matters now is
the one agents already speak.

---

# The decisions

Each is written with what it rejected, so a reader in six months can tell a choice
from an accident.

## 1. PocketBase as an embedded Go framework, one binary

`pocketbase.New()` as a library. Bubble's logic mounts as routes on its router
(`app.OnServe().BindFunc` → `se.Router`), the SPA is served with
`apis.Static(os.DirFS("./pb_public"), false)`, and the database is embedded SQLite
through `modernc.org/sqlite` — the same driver v0 already used.

Chosen for what it **deletes**, not what it adds:

| v0 wrote by hand | PocketBase provides |
|---|---|
| the schema plus `ALTER TABLE`s whose errors were discarded, and no version row | **versioned migrations** with an explicit revert (`m.Register(up, down)`, `migratecmd`) |
| the identity work that blocked everything else | **auth collections** — password, OAuth2, OTP, tokens |
| authorization derived from a mirror of Plane's project membership | **API rules per collection**, evaluated server-side |
| a bespoke SSE endpoint | **realtime** subscriptions per collection or per record |
| God Mode | the **admin dashboard** |
| a backup story that was somebody's cron job | **backups** |
| the superuser chicken-and-egg — something must mint the first person before MCP can authenticate | PocketBase's own superuser bootstrap |

**Rejected:** PocketBase as a separate service Bubble calls over REST. It
reintroduces v0's defining problem — a remote client with a call budget and latency
on the read path — for an isolation nobody at this scale needs.

**Rejected:** PocketBase for auth and realtime only, domain tables by hand. It keeps
the two hardest things to write and gives up API rules and the dashboard for
everything that matters.

## 2. Artifact bodies are files on disk, versioned by git

A readable tree is the record. PocketBase holds the **path and the metadata, not the
bytes**.

This reverses v0's storage decision and pays two of its debts at once: its
`thread_docs_prev` was an undo one level deep, and the backup story was an
obligation nobody automated. Git gives real history, blame and diff, and makes
`export` stop being a feature.

**Git and the event log are not redundant.** Git is the *content* history; `events`
is the queryable *index*. Git cannot cheaply answer "all evidence in this cycle
across workspaces", and most evidence is not a file change at all — a link landing,
a thread completing, a todo ticked in a body that also changed.

**Rejected:** PocketBase `file` fields. Attachments live in its storage directory
under generated names — upload and download for free, but not organised,
human-readable directories.

**Rejected:** markdown in a text column, as v0 had. Fewer moving parts and real
transactions, but it is the thing being changed.

## 3. The MCP is file-shaped; the server supplies belonging and concurrency

The tool surface is generic file editing, modelled on
[`mcp-file-edit`](https://github.com/patrickomatik/mcp-file-edit). Agents address
**paths**, not regions.

The objection this had to answer: v0's model lives on *editing the document IS the
evidence*, and a generic `write_file` that nobody observes puts heat back to being
inferred — the exact failure that ended v0.

It is answered in the server, not in the tool's shape:

- **Belonging.** Every path resolves to an entity *before* the write lands. A write
  under a thread's `doc_path` is attributed to that thread and emits an event. A
  file under a workspace root belonging to no thread is a loose document: still
  versioned, still an event, at workspace grain. **Nothing is written that is not
  attributed.**
- **Concurrency.** A write carries the hash it read as its `base`; a mismatch is a
  conflict, not a silent overwrite. This is v0's mechanism, kept because it is
  stronger than the backup-and-dry-run approach the reference uses. The server holds
  a lock per path across read-modify-write, and the file and its row are written
  together or not at all.
- **Containment.** Paths resolve inside a workspace root; traversal out is refused.

**Taken** from the reference: `patch_file`'s context strategy — quote the surrounding
lines, not line numbers — which is v0's `edits` under another name.
**Not taken:** the code-analysis tools, the SSH tools, and the twelve git tools. The
server owns committing, so an agent must not.

**Rejected:** thread-and-region-shaped tools, as v0 had. Safer by construction, but
it makes every non-thread document unreachable and forces an agent to learn a
vocabulary instead of using the one it already has.

## 4. Priority is derived, exactly like heat

| Priority | Meaning | What happens |
|---|---|---|
| **P1** Critical | operation stopped | interrupts any work |
| **P2** High | strong impact, workaround exists | attended quickly |
| **P3** Normal | necessary ordinary work | enters planning |
| **P4** Low | improvement, nice-to-have | backlog |

Determined by a pure function of impact and urgency:

| | Urgency high | Medium | Low |
|---|---|---|---|
| **Impact high** | P1 | P2 | P3 |
| **Impact medium** | P2 | P2 | P3 |
| **Impact low** | P3 | P3 | P4 |

> "I need this urgently" → "does anything happen if it is not done today?"

So `impact` and `urgency` are stored and **`priority` is not**. A stored priority
field lets somebody write P1 over low impact and low urgency, and then the map is
decoration. This is heat's own lesson pointed at a second axis: store the inputs,
derive the verdict.

**Heat and priority are independent axes, both derived.** Heat says how *alive* a
bubble is (evidence × time); priority says how much it *matters* (impact × urgency).
The board orders by heat, the planner filters by priority, and the most useful
signal the system can produce is a **cold P1** — invisible the moment priority is
folded into the buoyancy score.

**Rejected:** weighting priority into buoyancy. One ordering to understand, but it
conflates *important* with *alive*, and an untouched P1 stops looking like a problem
because it still floats.

**Rejected:** a hand-set priority field with the map as UI guidance. Simpler and more
flexible, and it permits exactly the inconsistency the map exists to prevent.

---

# The shape

## Collections

PocketBase collections. `users` is its built-in auth collection.

### Identity and boundary

| Collection | Fields |
|---|---|
| `users` *(auth)* | built-in, plus `display_name`, `role` (`lead` \| `member`) |
| `workspaces` | `name`, `slug`, `repo_path` (root of the markdown tree) |
| `memberships` | `workspace` →, `user` →, `role` (`lead` \| `member`) |

`memberships` is what every API rule reads. Authorization stops being derived from a
mirror of somebody else's membership.

Only **people** are identified, and only people publish. An agent acts with a
person's credential and therefore *is* that person for every rule in the system —
which is v0's identity model unchanged, and why nothing here distinguishes a human
principal from a non-human one.

### Two things are called "lead"

The two layers meet here, and they use the same word for different scopes. Keep
them apart when reading a rule:

| | Where | What it is |
|---|---|---|
| **global lead** | `users.role` | the strategic layer — the department head. Sees **every** workspace and administers it without being a member of any. |
| **workspace lead** | `memberships.role` | whoever founded a workspace. Invites people, sets its workflow, deletes its bubbles. |

Every collection rule carries the global bypass, composed **once** rather than
restated:

```go
const globalLead = `@request.auth.role = 'lead'`
func orLead(rule string) string { return globalLead + " || (" + rule + ")" }
```

The original rule is parenthesised because a rule like "member AND lead" joined by
a bare `||` makes the meaning depend on which operator binds tighter. And the
bypass is defined once because a mis-typed rule does not fail — it grants the
wrong access, silently. One place to get wrong beats twenty-six, and the test that
catches an over-generous prefix is the **opposite** one: somebody with no
memberships must still see nothing.

`role` is not self-writable (`@request.body.role:isset = false` on the owner's own
update rule), or anybody promotes themselves by editing their profile.

### Who may do what

Nothing is scoped to a record's creator. The founder of a workspace simply gets
the founding `lead` membership; after that "lead" is a role, not a claim on what
you made.

| | see | create | edit | delete |
|---|---|---|---|---|
| workspace | member | anyone signed in | ws lead | ws lead |
| bubble | member | member | member | ws lead |
| thread | member | member | member | member |
| link · relation | member | member | member | member |
| comment | member | member | **its author** | **its author** |
| state (workflow) | member | ws lead | ws lead | ws lead |
| label | member | member | member | ws lead |
| membership | member | ws lead | ws lead | ws lead |

…and the global lead, everywhere.

Two rules cannot be written as filters and are hooks instead, because a filter
answers "may you touch this row" and never "what about the other rows":

- **A workspace keeps at least one lead.** A lead may demote or remove themselves;
  the last one may not, or the workspace reaches a state nobody can administer and
  only a superuser can repair.
- **Authorship is stamped, not submitted.** The rule can say "you may edit a
  comment whose author is you", which protects updates and says nothing about
  creates — so without the hook a member posts a comment signed as somebody else.

Listing people is open to anyone signed in. PocketBase's default is owner-only,
which makes inviting impossible: a lead cannot see the person they want to add.
The cost is that the department's names and emails are visible to the department,
which is what a staff list is.

### Structure

| Collection | Fields |
|---|---|
| `objectives` | `key`, `name`, `description`, `order` |
| `bubbles` | `workspace` →, `name`, `outcome`, `owner` →, `closure`, `closed_at`, `stage`, `objectives` →→ |
| `threads` | `workspace` →, `bubble` →, `seq`, `name`, `doc_path`, `state` →, `assignees` →→, `parent` →, `impact`, `urgency`, `due_date`, `born_at`, `completed_at`, `from_inbox` → |
| `states` | `workspace` →, `name`, `group`, `position` |
| `cycles` | `workspace` →, `name`, `start_date`, `end_date` |
| `labels` | `workspace` →, `name`, `color` |
| `thread_relations` | `thread` →, `type` (`relates_to` \| `duplicate` \| `blocking` \| `blocked_by`), `related` → |
| `thread_links` | `thread` →, `url`, `title`, `added_by` → |
| `comments` | `thread` →, `author` →, `body` |
| `inbox_items` | `workspace` →, `raw_text`, `author` →, `triaged_to` →, `dismissed_at` |

The five objectives seeded at install: **clients** (sales, retention, loyalty,
marketing, customer value) · **software profitability** · **resource optimisation**
· **ISO 9001** (documentary system, traceability, customer satisfaction — already
implemented elsewhere, so Bubble exposes rather than enforces) · **maintenance**
(avoiding technical debt).

### Evidence

| Collection | Fields |
|---|---|
| `events` | `workspace` →, `target_type`, `target`, `kind`, `actor` →, `at`, `meta` |

Append-only, and the reason heat stops being inferred: with Bubble owning the write
path, a production event is **observed as it happens**.

Kinds carry over from v0: `thread-born` · `completed-todo` · `logbook-updated` ·
`body-updated` · `revision-added` · `link-added` · `thread-completed`, plus
`thread-created` (a record exists; nothing was produced) and `comment` as **pulse** —
presence only: it holds 😴, blocks 🪦, never wakes.

At five people this is roughly 18k rows a year, about 4 MB. There is no retention
policy to design.

**Never stored:** `heat`, `priority`, `lifecycle`. All three are pure functions of
rows that are stored plus the current time — so there is no cooling job that can
fall behind and no stored level that can be wrong.

### Where each derived axis is computed

**Priority is a view collection.** It is a pure function of two stored columns, so
`CASE WHEN impact='high' AND urgency='high' THEN 'P1' …` gives the planner
PocketBase's whole API — filter, sort, rules — with no endpoint written, and makes
the wrong value unrepresentable because the column does not exist. Wrap the
expression in `CAST(… AS REAL)` for anything numeric: PocketBase infers a computed
column's type from the view, and without a cast it types it `json`, which makes
`GetFloat` return zero and sorting go through `JSON_EXTRACT`.

**Heat is a hybrid, and the reason is not what it first looked like.** The obvious
objection — "a view is schema, so recalibrating means a migration" — is **false**,
and was disproved rather than argued: put the calibration in a `tuning` ROW that
the view joins, and the SELECT text stays fixed while the numbers are data.
Recalibrating is an `UPDATE`, same view, no migration. The arithmetic is there too:
`exp()`, `ln()` and `pow()` are all available in `modernc.org/sqlite`, and a view
reproduces v0's documented curve exactly (1.0 at zero, 0.37 at one decay window,
0.14 at two).

What actually argues against a pure view is different:

- **Time stops being a parameter.** v0's `Classify(bubble, tuning, now)` takes
  `now` as an argument, which is why its tests can pin the curve at a fixed
  instant. A view calling `unixepoch()` cannot be frozen — and it closes the door
  on asking what a bubble's temperature was at some past moment, which an event log
  otherwise makes possible.
- **The ladder is not the score.** Hot/Warm/Cooling/Dormant is an ordered rule set
  (something producing stays Hot with no owner; ownerlessness sinks what has
  already gone quiet) plus stable reason codes and args so clients can translate.
  That is a `CASE` cascade and a `json_object` built in SQL: possible, ugly, and
  untested by construction.
- **The window depends on the workspace's cycle** when it has one and a rolling
  window when it does not, replacing the configured length with the real one.

So: **the view aggregates** — most recent evidence per kind per thread and bubble,
the counts, the decay score — because that runs over `events`, the largest table,
and aggregation is what SQL is for. **Go decides** — the ladder, the reason codes,
the window — because that is what `internal/heat`'s tests already pin, and it ports
unchanged. The `tuning` row is a row either way.

## On disk

```
<repo_path>/
  <bubble-slug>/
    <thread-slug>.md
    <thread-slug>.excalidraw
  pages/
    <page>.md
```

- The file is the record. `threads.doc_path` points at it.
- Every server write is one git commit, authored as the actor. Git log is the content
  history; `events` stays the index.
- One lock per path across read-modify-write.
- `internal/md` is unchanged: it already speaks markdown regions, and the bytes come
  from a file instead of a column.
- Mermaid carries over from v0's renderer. Excalidraw is new — sidecar files
  referenced from the document.

---

# Phases

Each ends somewhere usable. Nothing is built ahead of the phase that needs it.

### Phase 0 — skeleton
`main.go` with `pocketbase.New()` and `migratecmd`. First migration: `users`
extensions, `workspaces`, `memberships`. API rules read `memberships`.

**Done when:** a person signs in, a workspace exists, the dashboard shows it.

### Phase 1 — structure and the markdown store

Split, because the two halves fail in different worlds and mixing them means a
failure that could have come from either.

**1a — the structure.** `bubbles`, `threads`, `states`, `labels`,
`thread_relations`, `thread_links`, `comments`. The same boundary work as phase 0
at scale: every new collection is insecure until its rule says which workspace it
belongs to, and each one is proved the same way — assertions against a running
server.

Plus the global lead and the invite path, which were planned for phase 5 and moved
here: without somebody able to add a member, every workspace is single-player and
nothing above it can be tried at all.

**Done when:** a thread lives in a bubble in a workspace, a member of one workspace
can reach none of another's, and a lead can invite.

**1b — the markdown store.** `repo_path`, `internal/md` ported verbatim with its
tests, the path resolver (`path → entity`, containment inside a workspace root),
per-path locking, and a git commit per write.

The risk this half carries is the one the decision above already names: the file
and its row are two sources that can drift. Writing both under one lock is the
whole of the answer, so 1b has to prove it rather than assume it.

**Done when:** a thread exists, its `.md` exists, and editing either through the
server keeps them consistent and produces a commit.

### Phase 2 — evidence and the two derived axes
`events` written on every observed production. The `tuning` row. The priority view
collection. The aggregation view, with `internal/heat` ported and deciding over it.

**Done when:** a bubble's temperature and a thread's priority are both computed and
neither is stored anywhere.

### Phase 3 — MCP
A route on PocketBase's router. File-shaped tools — read, write, patch by context,
list, search, move — with belonging, base-hash conflicts and containment enforced by
the server. v0's four markdown prompts are ported.

**Done when:** an agent does a real piece of work end to end and the bubble warms
because of it.

### Phase 4 — the operative board
Ported from the tag, served from `pb_public`: `Prose.svelte` + `lib/prose.ts` (the
one renderer), `Omnibar.svelte` + `lib/fuzzy.ts`, `lib/vim.svelte.ts`,
`lib/slash.ts`, `lib/editor.ts`, `ThreadToc.svelte`, and `Band` / `BubbleCard` /
`BubblePool`. Live updates come from PocketBase realtime instead of the bespoke SSE.

Not ported: `GodMode` (the dashboard replaces it), `McpConnect`, `ProjectCombobox`,
and everything that named an instance.

**Done when:** the board looks and moves like v0's and nobody wrote a second markdown
renderer.

### Phase 5 — the planner view
Inbox, calendar over `due_date` with timeline / week / day modes, the high-level
kanban by objective, and objectives editable in place. The role it is built for
already exists — see the global lead above.

**Done when:** a lead triages an inbox item into a thread without leaving the view.

### Phase 6 — migration
Read the two v0 `bubble.db` files: mint local ids, write the markdown tree from
`thread_docs` and `local_pages`, and backfill `events` from the timestamps that
exist. That backfill is lossy and says so; it does not invent history it does not
have.

### Later — Plane as a channel
Synchronising specific things, owning none of them. Deliberately after everything
above, so nothing is shaped around it a second time.

---

# Before Phase 0 — export the live instances

This is the one item with an expiry date.

Two deployments still hold a v0 `bubble.db`. In it, **`thread_docs` and
`local_pages` are the only copy in existence** of what was written there — the
mirror is rebuildable from Plane by definition, the overlay never was. Plane holds a
rendered copy of every *published* document and nothing at all of the local pages.

The other irreplaceable row is `thread_progress.born_at`: no body says "somebody
created this here", and it is the seed of every `thread-born` event the backfill can
produce.

```
git worktree add /tmp/bubble-v0 v0-plane-as-record
cd /tmp/bubble-v0 && go build -o bubble ./cmd/bubble
./bubble admin export        # the markdown tree
cp ~/.bubble/bubble.db  <somewhere safe>
```

`git show 2cf0c71:docs/DATABASE.md` is the full schema reference — all 33 tables,
column by column with the reasoning — and is the source document for Phase 6.

# Ported from the tag, not rewritten

Two packages owe nothing to Plane:

| Package | Code | Tests | What it is |
|---|---|---|---|
| `internal/md` | 2408 | 1546 | the splice engine, regions, `ApplyEdits`, `ToggleTodo`, `ParseChecklist`, fingerprints, markdown/HTML fidelity |
| `internal/heat` | 265 | 327 | the pure function |

The 5868 lines of tests in `internal/server` are the executable specification of what
the thing does — a source to re-key, not a cost to re-pay.

# Open

- Who commits, and how often. One commit per write is the plan; whether an agent's
  rapid edits should be squashed per session is not decided.
- Whether a page and a thread's document are still different things now that both are
  files. v0 kept two stores for one renderer.
- Excalidraw's editing surface. The sidecar file is decided; the editor is not.
- Whether `cycles` still earns its place now that no upstream tool supplies them.
