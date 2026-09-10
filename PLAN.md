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
**paths**, not regions — and the layout below is what makes a path enough to know
who a write belongs to:

| path | owner | event grain |
|---|---|---|
| `threads/<seq>-<slug>.md` | that thread | thread |
| `docs/**` and `README.md` | the workspace | workspace |
| anything else | refused | — |

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

**Comments, labels, relations and states have no MCP tool, deliberately.** They are
what people do to each other's work — discussing it, classifying it, saying what
blocks what — and none of them warms anything. They are reachable over REST for the
UI; an agent works without them.

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

### The superuser operates the box; it does not work in it

PocketBase's `_superusers` is not a person in the model. It bypasses every
collection rule, so it **reads** everything through Bubble's own routes too —
refusing it there made the halves disagree, with the dashboard showing every row
and the board answering 404.

It does not **write**. Every write here is attributed to a person: `events.actor`,
a git commit's author, who signed a comment. A principal with no row in `users` has
nothing to attribute, and unattributed writing is what phase 2 exists to prevent.
The attempt is refused with a sentence saying so rather than a mysterious 404.

The same human usually wants both accounts, **with the same email**: PocketBase
keeps them in separate collections and they do not collide. `just person <email>
<password> [lead|member]` creates the working one without the dashboard — until it
existed, a fresh clone could reach the dashboard and could not sign in to the UI at
all — and `just dev` now creates both from the same `.env`.

The cost, stated rather than hidden: two passwords that can drift. In development
they cannot, because `just dev` upserts both on every start; anywhere else,
changing yours is two commands.

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

Four kinds warm, and one of them covers every change to a thread's document:

| kind | grain | warms |
|---|---|---|
| `document-changed` | thread | **yes** — any write under `threads/`, whatever moved |
| `thread-created` | thread | yes — defining a piece of work is production |
| `thread-completed` | thread | yes — reaching a state in the `completed` group |
| `link-added` | thread | yes — proof reality changed outside this tool |
| `doc-changed` | workspace | **no** — the wiki is recorded, never warms |
| `comment` | thread | no — **pulse**: holds 😴, blocks 🪦, never wakes |

v0 had three document kinds — `body-updated`, `logbook-updated`, `completed-todo` —
because it was INFERRING what had happened from hashes and counters, and the shape
of the change was its only clue about the kind of work. Owning the write path
removes the need to guess, and with it the need to distinguish.

What is deliberately not collapsed is the evidence that is not a file write. A link
landing and a thread completing say something a document edit does not.

A write that leaves the file byte-identical records nothing. Pressing save is
activity, and activity is not evidence.

At five people this is roughly 18k rows a year, about 4 MB. There is no retention
policy to design.

**Never stored:** `heat`, `priority`, `lifecycle`. All three are pure functions of
rows that are stored plus the current time — so there is no cooling job that can
fall behind and no stored level that can be wrong.

### Where each derived axis is computed

**Priority is a view collection.** It is a pure function of two stored columns, so
`CASE WHEN impact='high' AND urgency='high' THEN 'P1' …` gives the planner
PocketBase's whole API — filter, sort, rules — with no endpoint written, and makes
the wrong value unrepresentable because the column does not exist. Two things every view here has to survive, both learned by hitting them:

- **CAST every computed column.** PocketBase infers a field's type from the view,
  and an uncast expression is typed `json` — the value arrives quoted and
  `priority = 'P1'` filters through `JSON_EXTRACT`.
- **One line, parenthesised.** PocketBase parses the SELECT list itself; a
  multi-line `CASE` comes back as `invalid identifier parts`.

**Four bands: Hot 🔥 → Dormant 😴 → Rip 🪦 → Closed 🏆.** There were five. *Warm*
and *Cooling* sat between Hot and Dormant and were a gradient nobody acted on:
"produced last cycle" and "produced neither cycle" lead to the same morning. What
is left is what v0 shipped (its `LEVELS`) minus `reviewed`, and every band names a
different **action** — leave it alone / somebody to ask / a decision to make /
read it once. Nothing stores a lifecycle, so collapsing them migrated no data;
the only thing on disk that mentioned a band was the tuning flag, renamed to
`ownerless_is_rip` in `1788488000_rip_band.go`.

**Rip is a band, not a badge on Dormant.** Quiet with somebody accountable is a
person to ask; quiet with nobody accountable is a decision to make. Same silence,
different problem, so it gets its own place to sit — and the board stops needing a
second signal beside the glyph to tell them apart.

**Ownerlessness is a BUBBLE rule.** A thread has assignees; the contract is what
needs somebody accountable, so it is applied in the roll-up and nowhere else — at
thread grain it could never fire. The order inside the roll-up is the model: a
bubble still producing stays Hot with nobody named, and the missing owner only
buries what had already stopped. Output outranks paperwork. Grace outranks the
grave: a newborn that has produced nothing has had no time to, and burying it on
its first morning is how a band stops being believed.

**A bubble takes the band of its hottest OPEN thread**, not the union of its
threads' evidence. That is not a setting: v0 made it one and then found the union
misleading, because it counts every thread's birth as the bubble's own output, so a
pile of untouched work reads Hot.

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
- **The ladder is not the score.** Hot/Dormant/Rip is an ordered rule set
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
  README.md                     the workspace's front page
  threads/                      planning and execution — one file per thread, FLAT
    1-primer-thread.md
  docs/                         the wiki — guides, references, whatever the team keeps
    onboarding.md
    diagrama.excalidraw
    arquitectura/decisiones.md
  assets/                       images, referenced from anywhere as assets/<name>
    mi-diagrama.png
```

Two directories, and they are the whole of the belonging rule. A tree where files
can appear anywhere is a tree nobody can reason about, and attribution would have
nowhere to come from — so the layout is **enforced**, not suggested: a path that
does not fit it is refused.

`threads/` is flat. Threads are enumerated by `seq`, nesting adds nothing, and it
would make the rename that already moves files ambiguous. `docs/` nests freely,
because a wiki wants folders.

**`docs/` has no rows anywhere.** The directory IS the index — which is what "the
file is the record" means when taken seriously — so a page is created by writing
to a path that does not exist yet, and a document's TITLE is its filename. Nothing
to keep in step, and no schema for a wiki to outgrow.

Writable extensions are `.md` and `.excalidraw` (the plugin's own JSON; its
`.excalidraw.md` form needs no special case).

**Images live in `assets/`, and only images live there.** One directory rather than
beside the page that uses them: a thread would have to write `../docs/…` and a
nested page `../../…`, and a path that depends on where the writer is standing is a
path people get wrong. From anywhere it is `assets/<name>`.

They arrive over multipart (`POST …/asset`) and are served by a route of their own
with the same boundary as the writing — a workspace's pictures are as private as
its documents. SVG is accepted because that is how diagrams arrive, and every asset
is served under `Content-Security-Policy: default-src 'none'; sandbox` with
`nosniff`, so neither an `<img>` nor somebody opening the URL can run anything out
of one.

There is a size ceiling (5 MiB, `BUBBLE_MAX_ASSET`) and the body is capped before
it is read. It is deliberately small: committing pictures is the honest consequence
of git being the content history, and **a repository that holds pictures grows and
never shrinks**. An image does not warm anything — it is something somebody
attached; the writing that uses it is the production.

- The file is the record. `threads.doc_path` points at a thread's; everything else
  is reached by path alone.
- Every server write is one git commit, authored as the actor. Git log is the content
  history; `events` stays the index.
- One lock per path across read-modify-write.
- `internal/md` is unchanged: it already speaks markdown regions, and the bytes come
  from a file instead of a column.
- Mermaid carries over from v0's renderer.

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

Plus the workspace tree — `GET /api/workspaces/{id}/tree` — which is two things at
once: the `list_files` an agent needs before it can discover anything, and the file
browser that makes `docs/` feel like a wiki rather than an invisible folder.

**Done when:** a thread exists, its `.md` exists, a wiki page exists without any
row, and editing any of them through the server keeps everything consistent and
produces a commit.

### Phase 2 — evidence and the two derived axes
`events` written on every observed production. The `tuning` row. The
`thread_priority` view. The `thread_evidence` aggregation view, with
`internal/heat` deciding over it, and `GET /api/workspaces/{id}/board` tying them
together.

`internal/heat` was rewritten rather than ported: v0's `domain.Bubble`, `Thread`
and `Tuning` were shaped by Plane, and two of its knobs stopped meaning anything
once any write under `threads/` became one kind of evidence. The classifier, the
ordering rules and the decay curve are the same, and `now` is still a parameter.

**Done when:** a bubble's temperature and a thread's priority are both computed and
neither is stored anywhere.

### Phase 3 — MCP
Eighteen tools on `/mcp`. The first ten: `guide` · `workspaces` · `board` · `tree` · `search` · `read` ·
`edit` · `create_thread` · `link` · `complete_thread`. Authentication is
PocketBase's own — an agent presents a person's token and IS that person for every
rule, so there is no agent identity to manage and nothing an agent can reach that
its person cannot.

**Surface parity is structural, not a promise.** The operations live in
`internal/bubble/ops.go` as plain functions taking `(app, auth, …)`, and both the
REST route and the MCP tool call the same one. They cannot drift, and neither door
can bypass what the other enforces. A surface's only job is to parse its own kind
of request and render its own kind of answer.

`search` was the missing half: the tree lists, and without a way to grep content an
agent only ever acts on what it was told about. Substring, case-insensitive,
threads before wiki pages. Not a regex — a wrong one from an agent is a silent
empty result or a runaway scan. Not an index — the walk is cheaper than keeping one
in step, and an index that can be stale is another source of truth.

Both addressing forms are accepted. A thread id never goes stale; a path is what an
agent already thinks in; and `workspace` takes a slug, so an agent works with
`"alpha"` without looking anything up.

**One guide, three doors, one file.** `prompts/bubble-work.md` is embedded and
served as an MCP prompt, an MCP tool and `GET /api/guide` in `text/markdown`. As a
tool as well as a prompt because not every client lists prompts, and one that does
not would never see it. Embedded rather than read from disk because the guide makes
promises about the API, and a guide deployed separately drifts from it and then
lies.

**v0's other prompts are not ported.** `create-a-thread`, `work-a-thread` and
`finish-a-thread` were full of opinions about shape — what a Brief needs, the DoD
gate, what belongs in a Logbook — and shipping them would put the linter back
through the side door. The one guide teaches the MODEL and says explicitly that
nothing validates a document's shape, then points at the workspace's own
`README.md`: a team's conventions are data in their repo, not code in ours.

`DisableLocalhostProtection` is set, with the reason next to it. The SDK refuses a
loopback request whose Host header is not loopback — correct against DNS rebinding,
and wrong behind a reverse proxy, where every legitimate request looks exactly like
that. v0 met it as `Forbidden: invalid Host header`, with the web UI fine and only
agents locked out.

**Eleven more, later, because the first ten left an agent blind.** *(built)* It
could write a document and nothing else: it could not say what the work is FOR,
when it is due, who is accountable for the bubble around it, or read what had
already happened to it — so it worked without context and asked a person for
everything that was not text. Added: `create_bubble` · `set_bubble` (outcome,
who is accountable, closed with its sentence, reopened) · `set_thread` (bubble,
objective, due date, impact and urgency — never priority, which is derived) ·
`plan` (the department's objectives and inbox) · `capture` · `set_objective` ·
`timeline` (what already happened, so the work is not done twice) ·
`delete_page` · `create_workspace` · `comments` and `comment`.

Commenting is the one an agent should reach for often, and the reason is the
model: a comment is PULSE, never heat. It keeps a thread out of the grave and
does not wake it — so an agent can say what it found, or that it found nothing,
without pretending it produced evidence. The alternative is an agent that either
stays silent or inflates the one number this system has.

`create_workspace` is the one that could not be "save a record": founding also
stamps `repo_path`, writes the founding membership so the creator is its lead,
and seeds the workflow — and all three live in hooks on the REQUEST, which this
door does not go through. Written straight, it would have produced a workspace
with no repository, no members and no states: visible to nobody and unable to
hold work. One transaction, because a half-founded workspace is worse than a
refused one.

Every one is a plain function in `ops.go` that mirrors the collection rule it
stands in for — `app.Save` does not know about rules, so each states which rule
it is enforcing and why. The guide was updated in the same pass: it said
`threads/` was FLAT, which stopped being true, and a guide that describes an API
it no longer matches is worse than no guide.

**Done when:** an agent does a real piece of work end to end and the bubble warms
because of it. — *it does: create by slug, write, tick, search, link, and the
thread reads hot with its priority beside it.*

### Phase 4 — the operative board

Svelte 5 + Tailwind 4, built with bun into `web/dist`, embedded and served at `/`.
The bundle is committed, so a plain `go build` needs no JS toolchain — and a UI
change is invisible until `just bundle` runs, which is why it belongs in the same
commit as the source that produced it.

The SPA's catch-all route is registered LAST, after `/api` and `/mcp`, or it
shadows them.

**Skeleton IS the component layer** — reversing what this said. The plan was to
carry only v0's token block, on the grounds that none of the components worth
taking imported Skeleton. That held right up to the first dialog, tree, combobox
and date picker: writing those by hand is writing focus traps and typeahead by
hand, twice. `@skeletonlabs/skeleton-svelte` (Zag.js underneath) supplies the
behaviour; the identity still comes from the token block in `app.css`, which came
across whole with the band accents renamed to the four lifecycles.

What that costs, and is worth knowing before reaching for a component: Skeleton
ships NO CSS for some anatomies (Dialog is styled with utilities in its own docs),
its class names collide with hand-written ones (`.card`, `.label` — ours were
renamed), and what a component is called is not what it writes to the DOM
(SegmentedControl is a Zag `radio-group`). Style against `data-scope`/`data-part`,
measured, not against what the name suggests.

**4a — the board.** *(built)* Sign-in against PocketBase, the workspace switcher,
and buoyancy made spatial: bands top to bottom, bubbles inside them, threads inside
those. What has gone cold sinks and desaturates, so the state is legible before
anybody reads a label. Each card carries its REASON and not just its verdict — a
band nobody can explain is a band nobody trusts — and the footer says which
calibration it was computed against, because a cold board and a short cycle look
identical otherwise.

Heat is refetched, never recomputed in the browser: it is a pure function of
evidence and TIME, the server holds both, and a board that ages client-side drifts
from the one everybody else is looking at.

The real screen and the mock now draw the SAME components: `Shell.svelte` (the
column of workspaces, the rail, the pinned "Nuevo", the one pane that scrolls)
and `BubbleBoard.svelte` (bands down the page, orbs centred, an empty band still
drawn). They were extracted OUT of the mock rather than reimplemented from it —
the mock is where the shape was worked out, and a second copy of that CSS is how
the drawing and the thing drift apart. The mock keeps only what has nothing
behind it yet: presence, and the planner.

What the column does is real: selecting, renaming in place, and creating a
workspace — whose slug is derived once and never follows a rename, because the
slug is the repository's directory on disk. Deleting asks first: the relations
cascade, so a workspace takes its bubbles, threads and evidence with it. The
markdown survives in its repo, which is the point of keeping it there.

**4b — the thread interior.** *(built)* `Prose.svelte` + `lib/prose.ts` — the ONE
renderer, because a second copy of that CSS is how two documents start looking
different — plus `ThreadToc`, a CodeMirror editor with optional vim, and mermaid
and excalidraw drawn from fenced blocks.

The write is the part with substance. `Thread.svelte` holds the document and the
hash it was READ at; every write carries that hash as `base`, so two people
editing cannot silently overwrite each other. A conflict now answers **409** and
not 400 — the body was fine, somebody else simply wrote first — and the view says
so and offers a reload instead of losing the paragraph. Reading and writing return
the SAME shape (`ReadDoc` on both sides), because the browser redraws from what a
write returned and a field that only one of them sends disappears on reload.

Saving is explicit and cheap: ⌘S, `:w`, Escape, blur, or switching to
"renderizado". Those were wired to an empty function for a while, which is worse
than having no shortcut — it looked like it saved.

**4c — the keyboard.** *(built)* ⌘K from any screen opens the omnibar, and one
field answers three questions in the order of how sure the answer is:

  · what the browser already holds — threads and bubbles, matched by
    `lib/fuzzy.ts` (subsequence, accent-folded, scored by word boundaries and by
    runs of consecutive characters), so the list moves with the keys and
    "mrkdwn" finds "Portar el motor de markdown";
  · what only the repository knows — the text INSIDE the documents, over
    `/api/workspaces/{id}/search`, asked for after a pause and with the last
    answer winning, because a request per keystroke is a request per keystroke;
  · commands, which are not found but NAMED, and so live behind `/` — the wiki,
    the board, the three themes, signing out.

A hit in a thread's document opens the THREAD, not the file: the path carries
the seq the server derived it from, which maps one back to the other with no
second round trip. `Wiki.svelte` wires `WikiView` to the real tree and reads a
page with `readPath`; the tree it draws holds `docs/` and the README and not the
threads, for the same reason.

The fuzzy matcher is forty lines here rather than a dependency: the libraries
that do this are 3–12 kB gzip for a list that never exceeds a few hundred rows.

Vim keys inside the document are the editor's (`@replit/codemirror-vim`); ⌘K is
NOT bound while it has focus, because taking a key from the thing being typed
into is how a shortcut becomes a surprise.

Not ported at all: `GodMode` (PocketBase's dashboard replaces it), `McpConnect`,
`ProjectCombobox`, and everything that named a Plane instance.

**4d — a bubble is edited where it is read.** *(built)* Right-clicking an orb
does the drawer's four things — open, `+ thread`, rename, close — and the drawer
itself is now where a bubble's own fields are written: the `outcome`, edited in
place under the title, and who is accountable, picked from the workspace's roster
with Skeleton's multi Combobox. Neither had a way in before, which made the model
unsayable in the product: a bubble without an outcome is a folder with a nice
name, and a bubble with nobody accountable is what the RIP band is FOR — cold
with nobody to ask — so a screen that cannot set an owner cannot distinguish 😴
from 🪦 on purpose.

`owner` became `owners`, plural in the schema because it is plural in the work:
the bubbles people actually run are shared, and naming one person made the other
invisible on the board. Nothing about the model resists it — `RollUp` asks
whether ANYBODY is accountable, and an empty list answers that exactly as a
missing name did.

The drawer's cycle bar and its two-line thread rows, drawn in the mock since
phase 4a, finally have data behind them: the board now carries `warm_at` per
bubble (when it last produced anything, which is what the bar measures against
the tuning's window), and per thread the `assignees` the evidence view never
selected plus the instant anything last happened to it. The instants travel raw —
"hace 2 d" is a sentence in a language, and the API does not have one.

Closing asks for one line, and that is the difference between it and a
confirmation: `closure` — how it ended, in the words of whoever closed it — has
existed as a field since phase 1 and nothing ever wrote it. It is optional, the
board carries it back so a closed bubble says why without a second fetch, and
reopening clears both it and `closed_at`, because it described an ending that no
longer holds.

**4e — belonging is a row, and now it can be written.** *(built)* A fresh account
was met with "crea el primero", because founding a workspace was the only move
the product offered somebody who belonged to none. That is the opposite of what
a workspace is — a body of work people are invited INTO — and the rules had
supported the other path since phase 1: a workspace's lead writes a membership,
the global lead writes any, and the last lead can neither leave nor demote
themselves. All of it was reachable only from the PocketBase dashboard.

So the shell now draws with an empty column and says so, both moves on the table,
and 👥 opens the workspace's roster: who is in it, their role, invite somebody who
already has an account, change a role, take somebody out. Reading it is every
member's; writing it is the lead's, and the panel simply stops offering the verbs
to everybody else — a button that always answers 404 is worse than no button. It
also refuses locally what the server refuses: the last lead's row is not editable
and not removable.

Creating the ACCOUNT is deliberately not here. It means passwords, and until
there is a considered answer for that it stays in the dashboard.

**4f — presence, as a heartbeat.** *(built)* A row of avatars saying who is
around, department-wide rather than per workspace: the people here work across
projects, and "who is about" is not a fact about one of them. The browser beats
while its tab is VISIBLE — a hidden tab reports a laptop, not a person — through
`POST /api/presence`, which upserts one row per person and stamps the SERVER's
clock; the collection itself takes no client writes at all, because a client that
stamps its own time is a client that can be online tomorrow. What "online" means
is decided when the row is READ (under two minutes is here, under fifteen is a
tab left open, beyond that not shown), since a stored verdict about time goes
stale in exactly the way this one does.

Rejected: deriving presence from the last write, which is the model's own error
run backwards — that says somebody is here because they saved a file three
minutes ago, which is activity, not attention. Also rejected for now: counting
live SSE connections, which is truer and lives only in memory, so a restart says
nobody is here.

**One connection, many topics.** `lib/live.svelte.ts` owns the single
`EventSource` and hands out `watch(topic, fn)`; presence was its first caller and
comments its second. A stream per screen would mean two reconnection handlers,
two "what did I miss" windows, and the browser counting connections against one
origin. Two things it does that are easy to forget: the subscription carries the
TOKEN while the EventSource is anonymous — so the client id is POSTed back, and
since that id changes on every reconnect the send hangs off the connect message
rather than off start-up — and a subscription begins at NOW, so every reconnect
tells its watchers to re-read.

Comments arrive live while the drawer is open, and the 💬 count moves while it is
closed — knowing there is something new to read is half of what a count is for.
Only while open: a stream held by a screen nobody is looking at is a connection
the server keeps for nothing.

Presence arrives over PocketBase's realtime stream, so somebody appears the
moment they open the app. Three parts, and the third is the one a subscription
cannot do: this tab beats, the stream brings everybody else's beats, and a local
ticker re-derives who is still fresh — nobody emits an event when a person STOPS
beating, and going quiet is precisely what presence has to notice. The stream is
best-effort: it reconnects on its own, a full read follows every connect (a
subscription starts at now), and a slow read underneath covers it never coming
back.

**Presence is never evidence.** No event, no heat, nothing that can move a band —
and a test pins it, because a heartbeat that warmed anything would be the most
efficient way ever built to manufacture activity.

**4g — the thread's sidebar stopped promising two things it never did.**
*(built)* "Enlaces" and "Relacionados" drew a section, a count and a `+` that
opened nothing, and they were not one afternoon away from working: relating is a
search, and links are an evidence channel with a heat weight behind them. What
they were competing with is the document, which holds as many links as somebody
writes and is where a person puts them anyway — those warm the thread as
`document-changed` rather than as `link-added`, which is a real difference and
not one the sidebar was earning.

`thread_links` and `thread_relations` stay in the schema and in the MCP: an agent
that lands a PR reports it with `link`, and that event is the strongest single
piece of evidence the model recognises. What is gone is a UI that said a person
could do it and could not.

**Pictures need a token, and an `<img>` cannot send one.** The route that serves
a workspace's images requires the Authorization header — dropping that guard
would make every picture public to anybody with the URL, and putting a token in
the URL would leave it in the history and in every copied link. So `Prose` fetches
them with the header and swaps in blob URLs, cached by path for the life of the
tab; the markdown and the HTML stay clean.

**Una pantalla que se cae, lo dice.** `<svelte:boundary>` envuelve la
aplicación entera. Hasta ahora un error al montar o desmontar una vista abortaba
la actualización EN SILENCIO: el estado ya había cambiado —la dirección, por
ejemplo— y el DOM se quedaba como estaba, así que el botón que lo provocó
parecía no hacer nada. Eso costó una tarde de diagnóstico por eliminación para
lo que resultó ser una línea: los avatares de un orbe estaban tecleados por el
NOMBRE de la persona, y dos personas a cargo cuyos nombres aún no habían cargado
llegaban como dos cadenas vacías — `each_key_duplicate`, actualización abortada,
board que no vuelve a dibujarse. Ahora el error se ve en pantalla con su rastro,
y la clave de ese `{#each}` incluye la posición.

**Cuánto cambió, en líneas.** *(built)* `+124 −18` beside every file in the
sidebar, and a third view next to *renderizado · markdown* that shows the diff
itself. Nothing computes it twice: every write here is already a commit, so
`git log --numstat` answers it for the whole repository in one call — and a
second implementation in the browser is a second answer to the same question.
`-M` matters more than it looks: without rename detection, renaming a thread
moves its file and reads as the biggest piece of work in the workspace.

**The window is the CYCLE**, and that is the point of the number rather than a
default: the same window heat is measured against decides what counts as
changed, so "+124 −18" reads as how much this document moved inside the window
everything else is judged in. The view offers *este ciclo · último cambio*,
because "nothing this cycle" is a true answer that is useless when what you
wanted was to see the last thing somebody did.

Drawn with `@codemirror/merge` — the same authors as the editor this app already
loads, so the diff arrives with the same font, theme and scrolling as the
markdown beside it, and no second styling system enters to make one screen look
like a different application. Skeleton has no diff component; `diff2html` would
have brought its own HTML and CSS. The two sides come from the server too: a
patch is what git prints, two documents is what a merge view needs, and
rebuilding one side from the other in the browser would be that second answer
again.

**Todos, and Shift+Tab to get there.** *(built)* Two halves of one thing: a board
of every workspace at once, and the key that lands on it without reaching for the
mouse. Both come back from v0, where the scope was a saved filter — here Todos is
an ADDRESS, `/todos`, because looking at every project at once is a screen, and a
screen you cannot link to is a screen you cannot send anybody.

**The server aggregates it**, in `GET /api/board`. Asking for one board per
workspace from the browser would be N answers computed at N different instants —
heat is a function of evidence and TIME, so two bubbles measured against two
"now" are not comparable, and comparing them is the only reason this screen
exists. One clock, one calibration, and the membership boundary applied where
every other rule is applied. Somebody who belongs to no workspace gets an empty
board rather than an error, because that is a true answer.

Every row now carries its `workspace`, and every board carries the list of the
workspaces it was composed of — including the single-workspace board, which
lists just itself. Not omitted when empty: a client that has to tell "field
absent" from "empty list" is a client with two shapes to handle. The list is what
turns an id into a name a person can read, and the slug in it is what
`/w/<slug>/t/<seq>` is made of — a row that cannot say where it lives cannot be
opened.

**On Todos you look and you open, and that is the whole offer.** Creating a
bubble or a thread, the wiki, the roster and renaming all need to know WHERE they
land, and a view of several projects cannot answer that. So they are not
disabled-looking, they are absent — from the orb's menu, the drawer, the floating
buttons and ⌘K. A command that is offered and does nothing teaches that the
palette is not to be trusted, and then it stops being used for anything. The orb
wears its project's name instead of its owner's initials, which is also what
saves N roster fetches: on a board of many projects, what you need from an orb is
where it is from.

**Shift+Tab is Alt+Tab, and the ring is ordered by RECENCY** — that is the
property that makes it worth having rather than a list with a shortcut. Shift is
held, Tab advances, releasing Shift commits; arrows move without committing, a
letter jumps, Esc puts back what was scoped before. Alphabetical order put the
neighbour you never visit next to the one you flick between all day, so a quick
Shift+Tab landed somewhere arbitrary; ordered by recency with the current scope
first, the second tile is always where you just came from. One tap out, one tap
back. Todos is IN the ring rather than beside it, because it is a real
destination.

The ring lives in `localStorage` because it is about this browser — which of your
projects you were in a minute ago is not a fact about the team — and it is
recorded from `current` rather than from the panel's own jumps: workspaces are
switched from the column and from ⌘K too, and a ring that counts only its own
moves drifts silently until it points at where you no longer are.

Shift+Tab is also how a keyboard user walks focus BACKWARDS, which is not a
shortcut worth breaking, so it is only taken when nothing else owns the keyboard:
not inside a field, an editor, a menu or a dialog, and not while the omnibar or a
naming prompt is up. The bubble counts under each tile are asked for once, when
the panel first opens — inside a workspace the board on screen only knows about
that one, and a tile reading 0 for the others lies worse than a tile with no
number at all.

**Where you were, when you come back.** *(built)* Coming back from a thread used
to land on a board scrolled to the top with the drawer shut, so the first thing
after writing was finding your way back to the bubble you had just left. Two
things persist now, and only two.

**Which projects, on Todos.** In `localStorage`, not in the address, which is the
same call v0 made and for the same reason: the address carries WHAT you are
looking at — a board, a thread, a page — and that is why it can be sent to
somebody. What you narrowed to is a preference of yours, and a link that drags
the sender's choice shows the person who opens it a board that is not theirs. It
is also exactly what makes coming back work: it was never in the address, so
navigating cannot disturb it. Only on Todos — inside one project, choosing the
project is saying the same thing twice — and a narrowed board says how many
bubbles it is hiding, because one that does not is lying about how much work
there is.

**Two axes were built and then removed: whose, and which band.** The band one was
wrong on the model's own terms — a band is not a filter, it is the shape of the
board, and hiding one changes the only thing the board has to say. "Whose"
answered a question the orb already answers with the initials of whoever is
accountable, and it cost a roster fetch per workspace on Todos to do it worse.
Both are gone rather than left switched off: a control nobody needs is a control
somebody has to understand before ignoring.

**The open bubble is `sessionStorage`, and deliberately not next to the project
choice.** It is not a preference, it is where you were going — good for this tab
and this hour, and finding it again tomorrow morning would be a panel nobody
asked for. It is kept in sync from ONE effect over the drawer's own state rather
than recorded at each exit, because there are four ways out — the thread, the ×,
Escape and the click outside — and four places to remember the same fact is three
places to forget it. The drawer does NOT close on the way to a thread: this
screen is leaving whole, and leaving it open is what keeps "where I was" true.
Restoring it also scrolls the bubble into view, because a drawer covering half
the screen over a bubble you cannot see tells half the story.

**Comments, in a drawer.** *(built)* The collection has existed since phase 1,
with the rule that matters — the author owns what they said, so nobody, not even
a lead, edits somebody else's words — and no surface. 💬 in the thread's HUD
carries the count and opens the same drawer shape the bubble uses: reading a
document and reading a conversation are different modes, and a panel that steals
width from the text makes both worse.

Skeleton's chat cookbook gave half of it — the stream and the composer pinned to
the bottom — and the other half was dropped on purpose: its contacts column is
our board, and its left/right bubbles encode "two parties talking", which a
thread is not. Everyone is on the same side; what matters is who said it and
when. Bodies are markdown rendered by the SERVER, like every other document here.

**Lo dicho, dicho.** Comments are append-only: `update` and `delete` are nil for
everyone, the author included. A thread's hilo is not a chat — it is the record
of a conversation about work, and its whole value is being readable later to
understand why something was decided. A thread where sentences change or vanish
stops answering that, and the worst part is not the deleted line: it is that the
replies left behind now answer something that is not there. Nil rather than
"only the lead", because handing it to the lead makes the record something that
can be tidied from above, which is the same problem wearing a badge. Correcting
what was said is saying another thing, underneath — which is also how it works
away from a screen.

The drawer says the thing the model is most often misread on: **commenting does
not warm the thread — it keeps its pulse.** Without that sentence the first
instinct is to comment so something "stays alive", which is exactly what the
model refuses to reward.

**Attaching, from where the writing happens.** *(built)* `POST …/asset` has
existed since phase 1 and nothing in the product called it: a thread could hold a
picture only if somebody uploaded it with curl. The document pane now takes a
file three ways — paste (which is how a screenshot arrives), drag, and a 📎 —
uploads it and writes `![nombre](assets/x.png)` at the caret. Both screens get it
at once, because it lives in `DocEditor`.

The document keeps the SHORT path. `assets/<name>` is what the layout says a
picture is called from anywhere, and it is what survives a rename, a move and a
git clone; the route that actually serves it is substituted when the server
renders — a single-page app resolves a relative path against the ADDRESS, so the
same string inside a thread asked for `/w/alpha/t/14/assets/x.png` and got the
application shell. Stored short, rendered resolvable.

**A thread may write more than one file.** `threads/` now nests EXACTLY one
level: a thread's own document stays `threads/<seq>-<slug>.md`, and anything else
it needs — a research note, a design, a log that does not belong on the main
page — lives beside it in `threads/<seq>-<slug>/`. One level because the thread
is what owns that directory; two would be a directory owned by nobody, and the
rename that already moves a thread's file would stop being decidable.

The main document stays a FILE beside its folder rather than becoming an index
inside it. Rejected on purpose: `doc_path` is one pointer to one file, the rename
moves it, `git log --follow` sees across the move, and turning every existing
thread into a directory would have been a migration of live repositories for no
gain.

Belonging follows the folder, and that is the part that matters: a write to
`threads/14-x/notas.md` is attributed to thread 14 and warms IT, not the
workspace — a thread's writing is a thread's writing wherever it put it. The
delete route learned the same distinction: a thread's own document goes when the
thread does, and its other pages go like any other file.

**The planner is a PLACE, so it lives in the column.** *(built)* It was a
floating 🗓 in the corner, which is where a verb goes — save, search, delete —
not where a place goes; reaching a place is what the left column is for. It sits
above the workspaces and separated by a hairline, because it is not one of them.

And what a member finds there is not a refusal. The objectives, the inbox and the
kanban of the whole department are the lead's, and stay that way — but the DATES
are not strategy: a thread with a due date is somebody's week, and the person
whose week it is should be able to look at it. So a member gets the same calendar
over the same `threads.due_date`, read-only, inside the shell (it was reached
from the column and the column should still be there), and clicking a day opens
the thread — which is where the work actually is. No filter is written for it:
`allThreads` returns what the rules allow, so a member's calendar is their
projects by construction.

**The plan is read by the person whose plan it is.** *(built)* 5a opened
`objectives` and `inbox_items` to everybody signed in, and the screen was later
made the global lead's — which left a screen that is one person's and an API that
is everybody's. That is a gap, not a decision, so the read now follows the
screen. The inbox keeps one opening, and it is not a compromise: whoever captured
a note reads it back, because a note you write and can never see again is a note
that stops being written.

**Every destructive action asks first**, through one `Confirm.svelte` rather than
five hand-written dialogs, and the question is asked by whoever performs the
WRITE — that side knows the name of what is going and what goes with it, and a
view that confirmed its own deletes would ask again in the mock, where nothing is
deleted.

**Done when:** the board looks and moves like v0's and nobody wrote a second
markdown renderer. — *the board, the thread interior, the wiki and the omnibar
do; what is not ported at all is listed above, and stays that way.*

**`/theme` and `/theme/mock`.** Two pages that ship with the app and need no
account: `/theme` is the tokens and the components, `/theme/mock` is the whole
product with invented data — board, sidebar of projects, thread, wiki and planner.
It exists so a screen can be designed and measured before there is a collection
behind it, and so a change to a token is visible everywhere at once. What is drawn
there is not evidence that it is wired.

### Phase 5 — the planner view
Inbox, calendar over `due_date` with timeline / week / day modes, the high-level
kanban by objective, and objectives editable in place. The role it is built for
already exists — see the global lead above.

Its SHAPE is drawn already, in `/theme/mock`: one focused view, three panes
separated by hairlines (inbox · calendar · kanban) that toggle the way Trello's
do, objectives and priorities as CRUD dialogs, and the verbs bottom-right where
the board keeps its own. Drag and drop is `@atlaskit/pragmatic-drag-and-drop`
(7.2 kB gzip, no peer dependencies) and the calendar is `@event-calendar/core`
(written in Svelte 5, so no wrapper) — both chosen against measured bundle size.

**5a — what the planner needs, and nothing more.** *(built, server side)* Two
collections, and the decision that matters is what is NOT here: a card. A
planner with cards of its own is a second inventory of work beside the threads,
and two lists of the same work disagree by Thursday. What moves across the kanban
is a THREAD, its columns are the OBJECTIVES — which is what this plan asked for
from the start, "the high-level kanban by objective" — and the calendar reads
`threads.due_date`, which has existed since phase 1.

A first cut made the columns the workspace's `states`, and that quietly turned
the strategic screen into a second operative board: moving a card there answered
"how is it going", which the buoyancy board already answers. By objective it
answers "what is this FOR", and the first column is the work nobody answered
for — which is the sentence the objectives dialog was already making.

  · `objectives` — the strategic layer, carrying the same word a bubble uses:
    an `outcome`, what is true when it is done. A thread points at one
    (optional, and NOT cascading — closing an objective must not delete the work
    done under it).
  · `inbox_items` — what was CAPTURED and is not yet work. This is the one that
    earns its place: a note has no outcome and no evidence, and making it a
    thread on the way in is how a backlog fills with rows nobody committed to.
    `captured_by` is stamped server-side like a comment's author; triaging sets
    `thread` and the item stays, because "we already decided about this" is
    worth being able to see.

**Both belong to the DEPARTMENT, not to a workspace** — corrected one migration
later, and worth recording because the first cut got it wrong. Objectives were
filed under a workspace, which contradicts what the strategic layer IS: the
department head, who sees every workspace without being a member of any. The
objectives people actually say out loud — clients, profitability, ISO 9001,
technical debt — are not a project's; the projects are what hangs from them, and
an objective living inside one of them cannot be what the others are measured
against. The inbox loses its workspace for a different reason: a note arrives
before anybody knows what it belongs to, and asking at the door is asking the
question triaging exists to answer.

So the rules split the way the rest of the model does: everybody signed in READS
the plan (including somebody who is in no workspace at all), the global lead
shapes it, and a note is edited by whoever wrote it or by the person whose job
triaging is. A thread from ANY workspace may hang from ANY objective — which is
what makes the objective worth stating.

Capturing is NOT evidence and warms nothing. The thread a triage creates is.

The workspace boundary got the same hole checked one collection over: a thread
may not point at an objective from another workspace, the way it may not be
filed into another workspace's bubble. Same guard, now written once for both.

**One document pane, two screens.** `DocEditor.svelte` — the renderizado /
markdown switch, the vim preference, CodeMirror, the slash menu, saving — was
the thread's, and the wiki now uses THAT rather than something like it. A page
and a thread's document are the same kind of thing (markdown in a repository,
rendered by the server); two panes would be two renderers and two sets of keys
to learn, and they would drift the week after somebody improved one.

The wiki writes now: edit, save with its base hash, create a page (a write to a
path that does not exist yet — the hash of nothing is what says "I know it is
not there"), and delete one under `docs/`. A thread's document is not reachable
by that door, which the server already refused.

**The planner does NOT ask for a bubble, and the board does.** That is the
difference between the two surfaces rather than an inconsistency: the board is
operative and draws bubbles, so work on it lives in one; the planner is
strategic, where the sentence is "this is the work and it belongs to this
project" and which bubble carries it is the board's decision, later.

**A workspace is born able to work.** Founding one now writes its workflow in
the same transaction as its founding membership: Por hacer (backlog, default),
En curso (started), Hecho (completed). Without them nothing could ever be
completed — completion is a thread reaching a state whose group is `completed`,
so "Terminar" had nowhere to point and said so. Three, because three is the
argument every team actually has; they are ordinary rows to rename, extend or
delete, and only the GROUP matters to the model.

**Making one, from the board.** Right-click on the empty space between the orbs:
a bubble, or a thread. A thread asks for its bubble too, and REFUSES when there
is none — work nobody can say the purpose of would not even appear on a board
that draws bubbles. The same two verbs are the first commands behind `/` in the
omnibar, because the board they belong to may not be the screen you are on.

**A thread's verbs are wired**: finish (the state whose group is `completed`),
reopen, move to another bubble, delete — each one asking the server and then
asking it again for the board, because writing changes what it would say.

**The slash menu came back from v0**, `lib/slash.ts` verbatim: the boundary rule
is what regresses, and a URL is full of slashes that must never open a menu.
What is new is that two editors share one catalogue of blocks rather than each
carrying its own copy.

**The URL is the screen.** `lib/routes.ts`: `/w/{slug}` a board, `/w/{slug}/t/{seq}`
a thread, `/w/{slug}/wiki/{path}` a page, `/planeador` the planner. Slug and seq
rather than ids — both are what people already say out loud, and an id in an
address is a string nobody can check against what they are looking at. Reloading
lands where you were and a pasted link opens what it says, which is why the board
is fetched ABOVE the screens: `/w/alpha/t/14` has to resolve `#14` before the
board component exists.

**5b — the planner, wired.** *(built)* `Planner.svelte` owns the data and every
write; `PlannerView` still draws, and still draws the mock. The translation is
the whole of it — column → `state`, card → THREAD, due → `due_date`, inbox →
`inbox_items` — and a card's DESCRIPTION is the thread's document, fetched when
the card opens and written with its base hash like every other write to that
file. Markdown is rendered by `POST /api/markdown`, the same goldmark the
documents go through: the browser has no renderer, and a second one is how two
screens show the same text differently.

Every write reloads instead of patching the local copy: the server decides the
seq, the default state and what a rule refuses, and guessing all three in the
browser is how two views of one board start disagreeing.

Nothing on the screen is scoped to a workspace: the objectives, the inbox and the
threads under them are the department's. A card carries the name of the project
it lives in, because on a board that spans the department two cards called
"Facturación" are two different pieces of work. What still needs a workspace is
CREATING one — a thread has to live somewhere, and the department is not a place
— so a new card is born in the workspace whose board the planner was opened
from.

The mock keeps its local behaviour precisely because it passes none of the write
callbacks — one component, two owners of the data, no second copy of the screen.

**The disagreement about priority is settled.** In the planner's mock a card's
priority is CHOSEN, because that is what was asked for there. Against the real
server it is DERIVED from impact × urgency like everywhere else, and the sheet
says so: the chooser becomes a read-only chip and the map beside it stops being
"consult before choosing" and becomes the rule that produced the answer. A
dropdown that writes nowhere is a control that lies.

**Done when:** a lead triages an inbox item into a thread without leaving the view.

## Inventario — dónde vive lo que hace funcionar todo esto

VPS, dominios, servicios contratados, licencias. No es trabajo y **no calienta
nada**: documentar un servidor no es evidencia de que la realidad cambió, igual
que capturar en el inbox. Pero es lo primero que alguien busca a las tres de la
mañana, y hasta ahora vivía en la cabeza de una persona o en un chat que nadie
encuentra.

Del departamento y no de un workspace: un VPS no es de un proyecto, los
proyectos viven encima de él.

**Verlo se ASIGNA, y se asigna con una fila** (`inventory_access`). Es como se
dice pertenecer en todo el resto de este modelo — una membresía es una fila —
y es la razón de no haber usado un booleano en `users`: un tercer eje sobre el
rol, invisible desde el lado del inventario y difícil de auditar. Una fila se ve,
se quita, y dice quién la dio. Cada quien lee SU propia fila y ninguna más, así
que la aplicación puede preguntarse «¿me toca?» sin pedir permiso para averiguar
si tiene permiso; el lead global las ve todas, porque repartirlas es su trabajo.

Dos niveles, porque «¿dónde está el DNS?» y «¿cuándo se renueva ESTE dominio?»
son preguntas distintas y una lista plana de cuarenta filas no contesta bien
ninguna: una galería de `inventory_groups` —datos, no un `select` en el código,
porque qué clases de cosas se contratan cambia sin que nadie recompile— y sus
`inventory_items` dentro, con proveedor, costo, fecha de renovación y notas en
markdown. La renovación es el campo que le gana la pantalla: un dominio que
expira es el clásico «nadie se dio cuenta».

**Nunca credenciales.** Hay un campo para el enlace a la bóveda: el inventario
dice DÓNDE está la contraseña, no cuál es. Un inventario que guarda secretos es
una brecha con buen diseño, y la primera persona que pegue una ahí lo hará
porque el campo existía.

Las imágenes van como archivo de PocketBase y no a `assets/`: ese árbol es el
repositorio de un proyecto, y una factura de dominio no pertenece a ninguno.

Un error que dejó una prueba escrita: la regla de acceso era
`@collection.inventory_access.user ?= @request.auth.id`, y con CERO filas eso
compara vacío contra vacío y da verdadero — el inventario se veía entero desde
una sesión anónima, y justo mientras estaba vacío, que es cuando nadie lo
habría notado. Un armario que se abre solo hasta que alguien le pone la primera
llave.

## Versioning and release

**A version is a binary, and a tag is what names it.** `scripts/build.sh`
already stamps `git describe --tags --always --dirty` into `main.version`, so a
deployed binary can say what it is; tagging is the only thing that was missing.

**Conventional Commits, keeping the sentence.** The convention constrains the
first line's prefix and nothing else, so `feat: un documento no decía cuánto
había cambiado` keeps the voice this repo has used since the beginning — a
subject that says what was WRONG, which is more than an imperative says.
Adopting the convention's imperative style as well was rejected: it would flatten
that for no machine-readable gain.

**Releases are automatic, from CI, to GitHub.** A push to the main branch runs
the checks and, when the commits since the last tag warrant it, cuts the tag,
writes the release notes from those commits, and attaches the built binary.
Nobody decides a number by hand, and nobody writes a changelog twice.

Rejected, and worth writing down: cutting tags by hand (`git tag -a`) is what
this was doing implicitly and it does not survive two people or a bad Friday —
the number gets skipped, or the changelog is written from memory. `cocogitto`
and `git-cliff` both do part of it well and still leave a human running a command
on a laptop, which is the step that stops happening.

**What BREAKING means here is not the HTTP API.** This is a self-hosted binary,
so the contract that can hurt somebody is their data and their agents:

  · a migration that cannot be rolled back — the database belongs to whoever
    hosts it, and `just migrate down` has to remain an honest offer;
  · an MCP tool that disappears or changes shape — an agent configured last
    month stops working, and it will not say why in a way its person can act on.

A change to the interface, however large it looks, breaks nothing that somebody
else depends on.

**Numbering starts at `v2.0.0-alpha.N`.** `v2.0.0` would claim v2 is delivered
while phase 6 has not started, and the one existing tag —
`v0-plane-as-record` — is an archive marker rather than a version. The alpha
prefix ends when the v0 databases have been migrated and this can hold the only
copy of somebody's writing.

### Phase 6 — migration
Read the two v0 `bubble.db` files: mint local ids, write the markdown tree from
`thread_docs` and `local_pages`, and backfill `events` from the timestamps that
exist. That backfill is lossy and says so; it does not invent history it does not
have.

### Later — editing from outside

Anything that writes to the tree without going through the server — Plane, a
person with an editor, Obsidian over a git clone — is the same problem wearing
different clothes: the write is not observed, so there is no base hash, no
attribution and no event, and heat goes back to being inferred. That is a CHANNEL
problem and it belongs here, after everything above, not in the main write path.

Obsidian was explored for this and set aside: a vault is a plain folder of `.md`
so the tree already is one, but there is no headless runtime — the official
headless client does Sync and Publish only, and the REST API plugin needs the
desktop app running. It is a fine client and cannot be a backend.

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
| `internal/md` | 260 | 190 | rendering, checkboxes, surgical edits |
| `internal/heat` | 265 | 327 | the pure function |

`internal/md` was ported verbatim and then cut in half, which is worth recording
because the plan used to promise the first part and not the second. What went:
`plane.go`, `html.go` and `fidelity.go` (Plane's ProseMirror HTML), every splice
function that operated on a `description_html` rather than on markdown, and the
LINTER — with 39 of its 45 tests, because they tested code that no longer exists.

The linter went on purpose, not as collateral. It warned about extra H1s, heading
skips, a Logbook with no todo, a Logbook with no `Next:`, more than five phases —
and refused a page with two `## Logbook` headings. Those are opinions about how
somebody writes, and how work gets written down is the writer's to decide: every
team thinks about its artifacts differently, and the guidance belongs in whatever
a team writes for itself. Its `Refuses()` also returned true for the zero value,
so a rule added without an explicit severity would silently start blocking writes.

The same reasoning removed one behaviour from the engine: a plain bullet is no
longer treated as a task when a section has no checkboxes. That was v0's Logbook
convention leaking into the parser. Only a real `- [ ]` box is a box.

The 5868 lines of tests in `internal/server` are the executable specification of what
the thing does — a source to re-key, not a cost to re-pay.

# Open

- Who commits, and how often. One commit per write is the plan; whether an agent's
  rapid edits should be squashed per session is not decided.
- Non-image attachments — PDFs and the like — are still refused. Nothing has
  needed one yet, and each type added is more in the repository forever.
- Excalidraw's editing surface. The sidecar file is decided; the editor is not.
- Whether `cycles` still earns its place now that no upstream tool supplies them.
