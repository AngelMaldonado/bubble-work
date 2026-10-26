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

**Dos más, de sólo lectura, y ninguna de escritura al lado.** *(built)* `repos`
dice dónde vive el CÓDIGO de un workspace, e `inventory` dónde viven las cosas
que lo sostienen — servidor, dominio, servicio, con su proveedor y su
renovación, y sólo si a quien pregunta le fue asignado el acceso.

Las contrapartes de escritura se rechazaron a propósito. Enlazar un repositorio
o dar de alta un servidor es una decisión de pertenencia que alguien toma en la
aplicación: es rara, la toma un lead, y una URL inventada manda a la gente al
código equivocado sin que nada falle. Un agente que encuentre uno que falta lo
propone con `capture`, que es el camino de todo lo que se sugiere y no se
decide, y deja rastro de quién lo propuso.

Y una frase que la guía repite porque esta es la puerta donde alguien la pondría
a prueba: `vault` es un ENLACE a dónde está la credencial, nunca la credencial.
Este servidor no guarda secretos, así que no puede entregar uno por ninguna
puerta — preguntarlo de otra forma tampoco produce uno.

**Tres documentos, y todos en `prompts/` como markdown.** *(built)* `guide`
describe el SISTEMA; `house_rules` es la sección que un agente se instala a sí
mismo en el archivo de instrucciones que ya lee cada sesión —`CLAUDE.md`,
`AGENTS.md`, `.cursorrules`, el que corresponda a lo que sea— y `connect.md` es
lo que una persona copia para configurar su asistente.

`house_rules` existe porque configurar el MCP no basta: un agente con las
herramientas y sin la instrucción abre un thread nuevo al lado del que ya estaba
a medias, o no anota nada y el board dice que aquí no pasó nada. La sección dice
el orden — mirar lo que hay antes de empezar, escribir en el thread que
corresponde, cerrar sólo lo que terminó — y lleva un hueco `<SLUG>` que quien la
instala sustituye: el documento no puede saber en qué proyecto se está pegando.

El prompt de conexión vivía como cadena dentro de `frontend/src/lib/connect.ts`,
que es exactamente la forma en que la prosa deja de editarse: escapada dentro de
código, invisible en un diff de los prompts, imposible de afinar sin tocar el
cliente. Ahora se sirve en `/api/connect` SIN rellenar y los dos huecos —la URL y
el token— los pone el navegador: él sabe con certeza por dónde llegó, mientras
que el servidor tendría que deducirlo del `Host` y de lo que ponga el proxy de
enfrente, y un prompt con la URL equivocada configura un servidor que no existe.
De paso, el token no vuelve a viajar dentro de una respuesta que nadie
necesitaba.

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

## El planeador orquesta BURBUJAS, en ETAPAS del departamento

*(built)* Era un tablero de threads con una columna por objetivo, y estaba una
altura por debajo de lo que hace un lead. Un thread es ejecución: lo escribe
quien opera, y cómo se organiza por dentro es suyo. Lo que se planea a corto,
mediano y largo plazo es un cuerpo de trabajo con un outcome — una burbuja.

**Las columnas son `stages`, del departamento.** No los `states` de un
workspace, y la diferencia cabe en una frase: **los estados son de quien
ejecuta; las etapas, de quien orquesta.** Un lead que mira seis proyectos
necesita un vocabulario común, y el de cada proyecto no lo es — se siembran
cuatro por horizonte (corto, mediano, largo, hecho) y el lead las renombra.

Se rechazó usar el `group` de los estados (`backlog`/`started`/…) como columnas
compartidas: no inventaba vocabulario, pero le quitaba al lead la columna suya
—«Esperando al cliente»— que es justo lo que hace de un tablero el suyo. Y se
rechazó un kanban por workspace: el planeador no tiene workspace a propósito,
porque orquestar es ver todo junto.

**Una burbuja pasa a tener dos ciclos de vida**, y son preguntas distintas: la
banda (🔥😴🪦🏆) dice si la realidad está cambiando y la deriva el servidor; la
etapa dice qué decidimos que es y la pone el lead. Verlas juntas es el valor —
«En curso» y 😴 dos ciclos seguidos es exactamente la fila que hay que mirar.

**El objetivo y la prioridad suben a la burbuja.** Un objetivo dice para qué
sirve un cuerpo de trabajo; colgado de cada thread obligaba a repetir la misma
respuesta en cada pieza y dejaba que dos piezas de lo mismo se contradijeran. La
prioridad es lo mismo por el otro lado: la decide quien orquesta. La tabla no
cambia —impacto × urgencia— sólo cambia de dueño, y sigue siendo una vista
(`bubble_priority`): un veredicto guardado es uno que alguien puede escribir en
contra del mapa.

El thread se queda con lo suyo: nombre, burbuja, estado del flujo del proyecto,
responsables y documento.

**La fecha también sube.** Primero se quedó abajo, con el argumento de que
vencer es de la ejecución. No se sostuvo: lo que el lead pone en el calendario
es cuándo tiene que estar un cuerpo de trabajo —«la facturación, para el 30»—,
la misma decisión que ponerlo en corto o en largo plazo. Con la fecha en cada
thread, la tarjeta del plan no tenía fecha, el calendario enseñaba piezas en vez
de lo planeado, y cada thread nuevo obligaba a reescribir un plazo ya decidido.
Cómo se reparte ese plazo entre las piezas es de quien ejecuta, y no necesita un
campo. Se rechazó tener fecha en los dos niveles: dos fechas para lo mismo es
decidir cuál manda cada vez que no coinciden. El traspaso (`dates_move_up`) toma
la fecha más tardía entre los threads ABIERTOS —el cuerpo de trabajo no está
hecho hasta su última pieza; la de un thread cerrado es historia, y sólo cuenta
si no queda ninguno abierto—, anota en el log las burbujas con fechas distintas
y quita el campo de los threads. Tampoco se deshace.

**Por la regla de este plan sería el primer cambio mayor de verdad, y sale como
menor a propósito.** El backfill sube el objetivo más repetido entre los threads
de cada burbuja y el máximo de sus impactos y urgencias; después se quitan las
columnas, y `down` devuelve el esquema pero no lo que decía. Las dos reglas del
backfill eligen no inventar: la mayoría cuando hay conflicto —anotado en el log,
que es donde una persona puede mirarlo— y el máximo por eje, porque bajar la nota
sería decir que lo urgente de dentro no cuenta.

Menor y no mayor porque hoy quien usa Bubble es un solo equipo, en una etapa
temprana: un salto a v2 anunciaría una ruptura a instancias ajenas que no
existen, y gastaría el número que sí la anunciará cuando las haya. Se rechazó
publicarlo como v2.0.0 por esa razón, no porque el cambio sea reversible — no lo
es, y quien actualice debe saber que el plan viejo por thread no vuelve. La
regla (base de datos que no se puede bajar ⇒ mayor) sigue en pie para cuando
haya alguien más a quien avisar.

## El inbox y el plan escriben con el motor de los threads

*(built)* Los paneles de una nota del inbox y de una tarjeta del plan ya usaban
el mismo editor (CodeMirror, el menú `/`) y el mismo renderizado (`Prose`, que
dibuja mermaid y excalidraw) que el documento de un thread. Les faltaba lo único
que no se hereda: **dónde guardar una imagen pegada**, porque eso depende de
quién es dueño de lo que se escribe.

- **Una burbuja tiene workspace**, así que su imagen va a `assets/` de ese
  workspace —commiteada, como la de un thread— y se cita por la ruta corta. El
  render del panel se pide con el workspace de la tarjeta, que es lo único que
  hace resolver `assets/x.png`.
- **Una nota no tiene workspace** —no tenerlo es lo que la hace una nota— así
  que sus imágenes van como archivos del propio registro, como las del
  inventario, y se citan por URL. En un registro de la base la URL no tiene el
  problema que tendría en un documento de git: nadie lo va a leer desde un clon.
  Sin SVG: servido desde nuestro origen podría ejecutar código con la sesión de
  quien lo mira.

La inserción (`![nombre](ruta)` en el cursor) vive una sola vez en
`lib/attach.ts` y la usan los tres editores; tres copias de «qué es una imagen
pegada» se habrían separado.

**Una burbuja gana `brief`, que no es el outcome.** El outcome es el contrato:
una frase de 500 caracteres, «qué es cierto cuando esto esté hecho», que se lee
de un vistazo en el board. Convertirlo en el cuaderno del plan —con diagramas e
imágenes— lo habría estirado hasta dejar de ser eso. Lo largo va aparte.

**Y una nota triada ya no se borra.** Se convertía en burbuja y desaparecía, y
con ella su cuerpo y sus imágenes — lo único que se había entendido de la
captura. Ahora su cuerpo pasa al brief de la burbuja, la nota queda apuntando a
ella (`inbox_items.bubble`) y sale del inbox por eso. Sus imágenes siguen
cargando porque el registro que las guarda sigue existiendo.

**Cuerpo y brief caben un diagrama: tope de 2 millones de caracteres.** Se
empezó con 20000, pensando en prosa. Pero el motor compartido dibuja, y un
bloque ```excalidraw es la escena entera en JSON: una docena de cajas pasa de
30000 caracteres. Pegar un diagrama —justo lo que el motor prometía— era un PATCH
rechazado entero, título incluido. Sin tope no es opción: un campo de texto sin
`Max` en PocketBase es 5000, no ilimitado. Se rechazó sacar el diagrama a un
archivo aparte: el documento de un thread lo lleva dentro, y dos formas de
guardar lo mismo según dónde se escriba es la separación que el motor compartido
venía a quitar. El editor avisa si se pasa, en vez de dejar que falle la red.

## El orden de una columna: derivado por defecto, a mano cuando alguien lo dice

*(built)* Una columna del planeador se ordena sola —por la prioridad que el
servidor deriva de impacto × urgencia, luego la fecha, luego el nombre— y eso
sigue siendo el valor por defecto. Es lo que dice el modelo: el orden se deriva,
no se administra.

Pero derivado no siempre es lo que una persona sabe. «Esto va primero porque el
cliente llama el martes» no cabe en impacto × urgencia, y la salida que quedaba
era **falsear la urgencia para colocar la tarjeta** — que corrompe el dato con el
que se calcula todo lo demás, incluido el board. Entre un segundo orden explícito
y un primer orden mentiroso, el explícito es menos malo y además se ve.

Así que `threads.rank`: cero es «ninguno, ordéname tú». Con el primer arrastre la
columna entera pasa a llevar rangos —mezclar tarjetas con rango y sin él sería
tener dos órdenes a la vez sin saber cuál gana— y la columna **lo dice** con una
etiqueta «a mano» y una salida en su menú. Un orden que no se sabe que está
puesto es un orden que no se puede quitar.

Lo que no cambia: ordenar NO es evidencia. Mover una tarjeta de sitio no es que
la realidad haya cambiado, así que no calienta nada y hay una aserción que lo
comprueba.

Rechazado: que arrastrar dentro de una columna no hiciera nada, que es lo que
había. La pantalla dibujaba el hueco igual, así que prometía un reordenado que
al soltar no ocurría — y eso se lee como una pantalla rota, no como una función
que no existe.

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

## `read` devolvía markdown, escapado dentro de un JSON

*(built)* Un documento salía por MCP como un objeto con el markdown en un campo,
con los saltos de línea escapados y el `html` al lado. Medido sobre un documento
pequeño de verdad: **427 bytes, de los cuales 122 eran el texto**. El resto,
ruido que un modelo tiene que des-escapar mentalmente para recuperar la
estructura que los encabezados y las casillas ya daban gratis — y el `html`
existe para la aplicación web, que lo pide por la API; por esta puerta duplicaba
el tamaño sin que nadie lo leyera.

Ahora `read` y `edit` contestan el markdown tal cual, con dos líneas de cabecera
delante: la ruta, el hash, cuántas casillas van hechas, y la frase que dice que
ese hash se devuelve al escribir.

**En TEXTO, y no en un canal más elegante.** El protocolo ofrece dos:
`EmbeddedResource` —un bloque con su `mimeType`— y `structuredContent`. Los dos
se pierden por el camino: OpenCode descartaba entero el contenido de tipo
`resource` (su issue 7878; con el servidor oficial de GitHub el agente recibía
«successfully downloaded…» y el archivo se tiraba), Claude Desktop lo consume
sin mostrarlo, y la propia especificación pide mandar un texto de respaldo junto
a `structuredContent` «por compatibilidad hacia atrás». Un canal del que el
estándar desconfía no es donde se pone lo que no puede faltar — y aquí lo que no
puede faltar es el hash: sin él no hay escritura.

`tree`, `board` y `search` siguen devolviendo JSON: son listas, y ahí el JSON es
la forma correcta.

**Y se lee por trozos, como un harness lee un archivo del disco.** `read` acepta
`from` y `lines`; sin ellos manda las primeras 1500 líneas y la cabecera DICE
cuántas faltan y cómo pedirlas — cortar en silencio es peor que no cortar,
porque el agente edita creyendo que lo vio todo. El hash es siempre el del
documento entero: es lo que hace correcta una escritura parcial.

La medida que lo justifica son los documentos que se importaron de v0: el de
Clickhouse son 38 KB, unos **9 600 tokens cada vez que alguien quiere corregir
una línea** — y otros 9 600 si tiene que releer tras un conflicto.

`search` gana `around`, que trae unas líneas a cada lado del acierto **sin
recortarlas**: lo que se va a citar en un `edit` tiene que ser el texto exacto, y
un «…» al final lo convierte en una cita que no encuentra nada. Con eso el ciclo
es el de un harness sobre disco —buscar, mirar el trozo, citar el contexto— y
cuesta unos cientos de tokens en vez de diez mil.

**Lo que NO se copió: numerar las líneas al leer.** Un harness las numera y
luego tiene que advertir «quita el prefijo antes de buscar» — una trampa que él
mismo crea. Aquí `edit` cita por contexto, así que numerar añadiría ese modo de
fallo sin resolver nada: `search` ya dice la línea, y `read` la acepta como
`from`.

## Dónde vive el código — la caja de repositorios

Una caja 3D fija abajo a la izquierda, enfrente de la HUD. Cerrada es una caja;
al pasar por encima se abre y saca una marca de GitHub por repositorio
asociado, cada una un enlace.

**Una fila por repositorio** (`workspace_repos`: workspace, url, name), no una
lista separada por comas dentro del workspace: son varios, entran y salen, y
cada uno quiere su propio nombre — que en una cadena es exactamente el momento
en que alguien inventa un formato. Además una fila se puede mirar, contar y
quitar. Lo ven los miembros del workspace, lo escribe su lead; el índice único
`(workspace, url)` impide enlazar dos veces lo mismo.

**No es el repositorio del workspace.** Ése es `repo_path`, el árbol de markdown
que este servidor escribe, y no se elige ni se enlaza. Estos son los sitios
donde vive el CÓDIGO, que este servidor no toca: un enlace, y la honestidad de
decir que la evidencia vive ahí afuera.

**Enfrente de la HUD, y no dentro de ella.** Los botones de la derecha son
verbos —buscar, invitar, conectar un agente—; esto es un lugar, y un lugar al
que se va tiene su propia esquina. El nombre se deduce de la URL (`owner/repo`)
en vez de pedirse: es como se llama un repositorio en voz alta, y un campo que
casi siempre repite lo que ya se escribió es un campo que se deja vacío.

La marca vive en `frontend/public/marks/` y no en `public/logos/`, que se GENERA
desde `simple-icons` y está ignorado: un archivo que la interfaz necesita
siempre no puede vivir donde `npm run logos` lo borra y lo rehace.

## El token del agente duraba cinco días

El agente se conecta con el token de la sesión de quien copió el prompt: el
botón «Copiar el prompt que conecta tu IA» mete `api.token` en la cabecera
`Authorization`. El navegador renueva el suyo solo con `auth-refresh`; la copia
pegada en la configuración del agente no se renueva nunca. Con el valor de
PocketBase —432000 s— el MCP respondía 401 a los cinco días, y el cliente no
podía recuperarse: con la cabecera puesta no hay OAuth al que caer.

**`users.authToken.duration` sube a 94670856 s, unos tres años** (migración
`1788509000_long_tokens`). Es el tope que PocketBase acepta para ese ajuste. Sube
también la sesión del navegador, que ya se renovaba sola. Revocar no cambia:
cambiar la contraseña rota el `tokenKey` y mata todos los tokens de esa persona,
agentes incluidos. Un token ya emitido lleva la caducidad dentro, así que el
agente sólo gana los tres años al volver a copiar el prompt.

Lo que se descartó:

- **Un JWT estático sin tope** (`NewStaticAuthToken` con cien años). Nunca
  caduca, pero sólo se revoca rotando el `tokenKey`: si se filtra uno, no hay
  forma de matar ése sin cerrar todas las sesiones de la persona. Tres años
  tienen el mismo problema de revocación, pero al menos se acaban.
- **Keys propias por agente** (una colección con el hash, `last_used` y
  `revoked`, y un middleware en `/mcp`). Es la respuesta correcta cuando haga
  falta revocar un agente sin tocar a su persona, o saber qué agente sigue vivo.
  Hoy no hace falta, y el cambio de una línea resuelve el síntoma.
- **OAuth en el MCP.** El cliente renovaría solo, pero es un servidor de
  autorización entero para una sola puerta.

**Hecho cuando:** un prompt copiado después del despliegue lleva un token cuyo
`exp` cae tres años más tarde, y el MCP sigue respondiendo pasados cinco días.

## Ajustes: flotabilidad, personas, perfil y tokens de agente

Una pantalla `/ajustes/<categoría>` con barra propia y ← al board, y una
**columna con las categorías** (la `Navigation` de Skeleton, como la del hilo o
el toolbox de la secuencia; se encoge a rail con la misma preferencia que la de
proyectos, y siempre en una pantalla angosta; la lista desplaza sola, con el
desvanecido en el borde, igual que la de proyectos). Cada fila es un ancla con su
dirección: la categoría se enlaza y sobrevive a recargar. Antes eran tabs de
Skeleton; se cambiaron por la columna, que deja ver todas las categorías con su
icono y no compite por el ancho con el contenido. Entra desde el menú del pie de
la columna y ⌘K.

- **Usuarios**: la lista de personas con un botón «+ Dar de alta» que abre un
  diálogo (correo, nombre, contraseña, rol); al crearla, el mismo diálogo enseña
  la contraseña una vez. Antes el formulario era una tarjeta fija arriba de la
  lista.
- **Equipos**: la explicación de qué es un equipo va en un icono ⓘ con tooltip,
  junto al título, y no como párrafo fijo; el equipo en el que trabajas ya no
  lleva la barra de color a la izquierda (lo dice «trabajando aquí»). El nombre
  se edita pulsándolo (Enter o salir guarda, Escape cancela), borrar es un botón
  danger con el asset de basura, los badges son más grandes, y cada miembro es
  la misma fila de persona que en Usuarios (`PersonTile`: cara, nombre, correo).
- **Cada sección lleva su título** en la fuente display; la de los tokens se
  llama **Agente** (la dirección sigue siendo `/ajustes/tokens`).
- **Canales → WhatsApp**: el logo de WhatsApp; el número vinculado se ve en un
  campo deshabilitado (para cambiarlo se desvincula) y, sin número, se dice de
  frente «Falta vincular un número»; el grupo se elige en un dropdown con los
  grupos de la cuenta. El número y el grupo se guardan en la configuración del
  canal, así que se ven también con el canal apagado.

- **Funciones** incluye **Mi espacio** (`features.space`), encendida por
  defecto (la migración la enciende en la fila existente): apagada sale de la
  columna y `/mi-espacio` lleva a Todos; las notas se quedan. Las tools del MCP
  de notas personales no dependen de ella.
- **Planas:** las secciones van sobre el fondo, sin caja, separadas por una
  línea fina; sólo el token recién generado lleva un fondo (se enseña una vez).
- **Flotabilidad** (*buoyancy*): edita la fila `tuning` en palabras de lo que
  produce (días, ciclos, % que sigue flotando), con un slider de Skeleton por número, de
  uno en uno y con marcas (antes eran campos numéricos) y los iconos de banda en su versión iluminada. La leen todos, la cambia el lead
  global; recalibrar sigue siendo un UPDATE, no una migración.
  **Artefactos temporales:** en la misma pantalla, un slider (1 a 180 días)
  para `tuning.artifact_days`: cuántos días sin movimiento tarda en borrarse un
  artefacto no fijado (30 por defecto). Ya era un valor de esa fila que leían
  el job y la caducidad que muestra cada pantalla; faltaba dónde cambiarlo.
- **Mi usuario**: nombre visible, avatar (`users.avatar`, 2 MB, sin SVG) y
  contraseña. El correo es de sólo lectura: cambiarlo bien pide verificar por
  correo, y la instancia no manda correos.

### Los agentes entran con un token suyo, no con la sesión

La colección `agent_tokens` (dueño, nombre, sha256 oculto, prefijo, caducidad,
último uso, revocación) y un middleware que, **sólo en `/mcp`**, acepta
`Bearer bw_…` y pone al dueño como quien pide. El agente sigue SIENDO su persona
—mismas reglas, misma firma—: no aparece una identidad de agente. Crear,
extender (desde hoy o desde la caducidad, la más tarde; 0 la quita), renombrar y
revocar son rutas propias: la huella la calcula el servidor. Se permite «sin
caducidad» (decisión del usuario). El token se enseña una vez, con el prompt de
conexión ya relleno; el 🤖 lleva a Ajustes → Tokens en vez de copiar la sesión.
El JWT de sesión se sigue aceptando en `/mcp`, así lo ya configurado no se rompe.

**Conectar desde Ajustes → Agente:** cada token tuyo tiene «Copiar prompt»; como
el servidor sólo guarda la huella, el prompt va con un hueco `<TU_TOKEN bw_…>` y
una nota que dice cuál pegar (no se finge que se puede recuperar). Y los
comandos por harness, con su botón de copiar —completos con el token al
generarlo, con `<TU_TOKEN>` en la lista fija—, según su documentación:
Claude Code `claude mcp add --scope user --transport http bubble-work <url>
--header "Authorization: Bearer …"`; Codex `codex mcp add bubble-work --url <url>
--bearer-token-env-var BUBBLE_WORK_TOKEN` (lee el token de esa variable);
Antigravity no tiene comando: una entrada `serverUrl` + `headers` en
`~/.gemini/config/mcp_config.json` (o `.agents/mcp_config.json`).

**Los revocados se purgan:** un job (`bubbleTokensPurge`, cada hora) borra los
tokens revocados hace más de una semana. Esa semana deja ver «se revocó, y
cuándo»; después ya no abren nada ni dicen nada nuevo. Los caducados se quedan:
se pueden extender.

Lo que se descartó:

- **Seguir con la sesión** (la migración de tres años). No se revoca sola, no
  dice qué agente la usa, y cambiar la contraseña corta a todos los agentes.
- **JWT estático por agente** (`NewStaticAuthToken`). Sólo se invalida rotando
  el `tokenKey` de la persona, que mata todas sus sesiones y tokens a la vez.
- **Que el token abra toda la API REST.** Un agente habla MCP; abrir más es dar
  más de lo que se pidió al generarlo.

Queda por decidir: bajar la sesión del navegador de tres años a algo corto,
ahora que los agentes ya no dependen de ella.

### Borrar a una persona es marcarla

`users.deleted_at`. Una persona borrada no entra (`authRule`), sus sesiones
mueren (se rota su `tokenKey`), sus tokens de agente dejan de valer, desaparece
de los selectores y lo que escribió se queda con «(eliminado)» junto al nombre.
Sus membresías **se conservan** (decisión del usuario): restaurar —vaciar la
fecha— la deja como estaba. El lead global da de alta, cambia el rol global,
borra y restaura desde la app; los candados (no borrarse ni quitarse el rol a uno
mismo, no dejar el departamento sin lead global) están en el servidor, porque una
regla no sabe contar cuántos leads quedarían. El borrado duro por la API se
cierra: la regla por defecto de PocketBase dejaba a cada persona borrarse.

**A los 30 días, se purgan sus datos personales** (decisión del usuario): un
job diario (`bubblePeoplePurge`) borra los tokens, preferencias, presencia, Mi
espacio (notas y archivos), accesos al inventario, membresías de proyecto y de
equipo, el avatar y su equipo personal si no tiene proyectos (con proyectos se
queda: es trabajo). La cuenta se queda —nombre y correo— como firma de lo que
escribió, y `users.purged_at` dice cuándo se purgó. Dentro de los 30 días,
restaurar la deja como estaba; después vuelve sin nada de eso, y Ajustes →
Usuarios lo dice. Rechazado también aquí el borrado total (pasar su autoría a
una «persona eliminada»): se perdería el nombre en toda su historia.

Lo que se descartó:

- **Borrado duro.** PocketBase lo rechaza (un comentario exige autor) y, si no,
  se llevaría la historia del departamento.
- **Quitar las membresías al borrar.** Restaurar dejaría a la persona sin nada
  y habría que recordar dónde estaba.

**Hecho cuando:** los tests en vivo de tokens y personas pasan, y el usuario lo
valida en la app.

## Los hilos vuelven a tener prioridad, y el departamento una secuencia

**La prioridad vuelve al hilo, separada de la de su burbuja.** Había subido a la
burbuja («El planeador orquesta BURBUJAS») con el argumento de que la decide
quien orquesta. Se sostiene para la burbuja, y no alcanza para las piezas: dentro
de un mismo cuerpo de trabajo hay piezas que importan más que otras. Las dos
conviven y no se tocan: la burbuja dice cuánto importa un cuerpo de trabajo (se
deriva de impacto × urgencia, como antes), el hilo cuánto importa esa pieza. En
el hilo se GUARDA (`threads.priority`, P1…P4) y la escribe sólo el lead global:
el mapa sirve para decidir un cuerpo de trabajo, y para una pieza basta con
elegir la letra. Se ve y se cambia con un badge en el cajón de la burbuja, en la
pantalla del hilo y en la cara «Hilos» de la tarjeta del planeador.

Lo que se descartó:

- **Heredar la de la burbuja y poder sobrescribirla.** Era la propuesta; el
  usuario la quiso separada del todo, y una herencia obliga a explicar en cada
  badge si es propia o prestada.
- **Derivarla del mapa también en el hilo.** Un menú P1…P4 no tiene vuelta al
  mapa (P2 y P3 salen de tres casillas cada una), y elegir dos ejes para una
  pieza pequeña es un clic de más.

**El orden de ejecución es UNA secuencia para todo el departamento**, no un orden
por proyecto ni por burbuja: las tareas van hacia los mismos objetivos, y en qué
orden se hacen es una decisión de organización. `threads.sequence` es el paso de
un hilo (10, 20, 30…, con huecos para insertar); dos hilos con el mismo paso van
en paralelo; vacío es «no está en la secuencia». La arma el lead global en un
cuarto panel del planeador, «🧭 Secuencia», arrastrando desde una bandeja «Sin
secuenciar». **«Ahora» no se guarda**: es el primer paso con algo abierto, así
que terminar un hilo —desde la secuencia, desde su pantalla o por MCP— la recorre
sola. Los agentes la leen con la tool `next` (ahora, luego, y lo primero asignado
a quien pregunta).

**La secuencia vive como una columna del kanban, no como un panel.** Primero fue
un cuarto panel del planeador (con un tope de tres paneles a la vez); probándolo,
se vio que el kanban ya es donde el lead ordena, y que otro panel sólo quitaba
ancho. «🧭 Secuencia» es una columna FIJA —no se renombra, no se borra, no se
mueve— justo a la derecha de «Sin planear»; lo que se edita es su contenido.
Soltar ahí la tarjeta de una burbuja mete sus hilos abiertos al final de la
línea, uno por paso en su orden. Se descartó el panel aparte (y con él el tope
de tres paneles, que ya no hace falta).

**La secuencia es una función que se enciende** (`features.sequence`, una fila
como `tuning`, que sólo cambia el lead global en Ajustes → Funciones). Nace
apagada: cambia cómo se decide qué se hace primero, y eso lo decide el lead, no
una migración. Apagada no aparece en ningún sitio —columna, planeador, cajón— y
`next` lo dice; los datos de `threads.sequence` se quedan, y encenderla trae la
línea como estaba. Se rechazó guardarla dentro de `tuning`: aquello calibra la
flotabilidad, esto dice qué partes del producto existen.

Lo que se descartó:

- **Una línea por burbuja.** El orden no depende del proyecto.
- **Derivar el orden de dependencias** (`blocked_by`). Dice qué no puede ir
  antes, no qué se decidió hacer primero.
- **Guardar un puntero a «ahora».** Un puntero se queda atrás el día que alguien
  termina un hilo por otra puerta.
- **Una línea estrictamente secuencial.** El usuario pidió paralelismo: varias
  personas trabajan a la vez.

**Hecho cuando:** los tests en vivo de prioridad y secuencia pasan, y el usuario
lo valida en el planeador.

## Canales del inbox: plugins compilados, y WhatsApp el primero

Lo que llega por fuera de la app entra al inbox sin copiarlo a mano. Un **canal**
es un plugin COMPILADO (`internal/channels`): un paquete que se registra desde su
`init` con la interfaz `Channel` (`Start`, `Stop`, `Status`, `Action`) y entrega
mensajes normalizados (`Inbound`) a un `Sink`. El canal no conoce el inbox y el
inbox no conoce WhatsApp. Las rutas son genéricas (`/api/channels/{kind}/…`, sólo
el lead global) y lo propio de cada canal va por `Action`. Un canal se deja fuera
del binario con una etiqueta de build (`-tags nowhatsapp`).

`channels` guarda una fila por tipo: si está encendido, su configuración no
secreta y a nombre de quién se captura (`inbox_items.captured_by` exige una
persona: quien enciende el canal). Las notas ganan `source`, `source_ref` (índice
único: un mensaje que llega dos veces entra una) y `source_from` (quién lo mandó
allí, que no tiene por qué tener cuenta).

**WhatsApp escucha UN grupo** con whatsmeow (go.mau.fi/whatsmeow, MPL-2.0), que
habla el protocolo de WhatsApp Web multidispositivo — el de Baileys, en Go. Se
vincula por QR o por código de teléfono, se elige el grupo por nombre, y cada
mensaje del grupo entra al inbox (texto; las imágenes como archivos de la nota;
audio, video y documentos se nombran sin adjuntarse) y recibe la reacción 📥; si
reaccionar falla, se contesta «📥 Recibido en el inbox». La sesión (llaves de la
cuenta) vive aparte, en `pb_data/channels/whatsapp.db`, con el driver SQLite sin
CGO que ya usa PocketBase: el binario sigue siendo uno.

Lo que se descartó:

- **El paquete `plugin` de Go.** Sólo funciona en Linux y se rompe entre
  versiones.
- **Baileys o WAHA como servicio aparte.** El mismo riesgo que whatsmeow, con otro
  runtime que desplegar y vigilar.
- **La WhatsApp Cloud API oficial, por ahora.** Sin riesgo de bloqueo, pero exige
  un endpoint HTTPS público (las instancias viven en una LAN) y, desde octubre de
  2026, cobra cada mensaje del negocio. Cabe como otro canal con esta interfaz.
- **Capturar mensajes directos de cualquiera.** Un grupo decide quién puede
  mandar al inbox sin inventar una lista de números.

El riesgo asumido: whatsmeow no es oficial, y WhatsApp bloquea cuentas que
automatizan envíos. Se mitiga con un número dedicado y con un canal que sólo
escucha y reacciona, nunca escribe primero.

**Hecho cuando:** un mensaje en el grupo aparece en el inbox con quién lo mandó y
recibe 📥, una sola vez, y el usuario lo valida con un grupo real.

## La forma de trabajar la escribe el lead, y los agentes la leen en vivo

Dos documentos del departamento, **AGENTS.md (planner)** y **AGENTS.md
(operators)**, que el lead global edita desde el menú del pie de la columna
(`/agentes/<rol>`); quien opera ve en el mismo menú sólo el suyo, de sólo
lectura — lo mismo que recibe su agente. Viven en la colección `agents_md`, una fila por VERSIÓN
(rol, contenido, autor, fecha): guardar crea una fila, la vigente es la más
reciente, y ninguna se edita ni se borra. La herramienta MCP `agents_md` le da a
cada agente la vigente de su rol —el lead recibe la de planner, los demás la de
operators— con versión, fecha y autor en la cabecera; `role` lee la otra.

`house_rules` deja de ser un resumen de las reglas y pasa a ser una sección
breve: el framework en un párrafo, la **obligación** de leer `agents_md` al
empezar cada sesión, y `guide` antes de la primera escritura. Lo que cambia
cómo trabaja el equipo ya no pide un commit: pide que el lead guarde una versión
y avise de qué cambió.

En la pantalla, lo que entrega el editor al perder el foco es un borrador; la
versión la crea un botón. En un hilo escribir más es mejor; aquí cada guardado
llega a todos los agentes del departamento.

Lo que se descartó:

- **Copiar el documento al CLAUDE.md/AGENTS.md de cada quien.** Cada copia se
  queda vieja en silencio, y actualizarla dependería de que cada operador vuelva
  a correr el prompt. Leído en vivo, la siguiente sesión ya trae el cambio.
- **Dejar la sección fija de `house_rules` junto al documento del lead.** Dos
  fuentes de reglas que pueden contradecirse, y una la edita quien programa.
- **Un archivo markdown bajo git.** El árbol es por workspace, y esto es del
  departamento, como los objetivos. La historia la dan las filas.
- **Una fila por rol que se sobrescribe.** Se pierde qué leyó un agente un
  martes, que es justo la trazabilidad que se quiere para ISO 9001.

**Hecho cuando:** los tests en vivo de `agents_md` pasan, y el lead guarda una
versión de cada documento desde la app y la ve leída por un agente.

## Lo que pasa en una fecha y no es trabajo *(built)*

Una junta semanal no tenía dónde ponerse: el calendario sólo sabía de burbujas y
de tareas con fecha. La salida tentadora era crear un hilo y darle fecha, y es
justo la que hay que cerrar — un hilo se completa, produce evidencia y
**calienta**. Un hilo que renaciera cada lunes emitiría `thread-created` todas
las semanas: una burbuja 🔥 para siempre sin que nadie trabaje, que es lo
contrario de lo que el calor dice.

Así que `calendar_events` es una colección aparte, del departamento, escrita por
el lead global. **No emite ningún evento y no calienta nada**, y hay una
aserción viva que lo fija contando las filas de `events` antes y después.

**La repetición es un menú cerrado** —diaria, semanal, quincenal, mensual— y se
rechaza la RRULE de iCalendar: trae una librería más y mucha superficie que
probar (excepciones, fin de serie, «el tercer jueves») por casos que todavía no
existen aquí. Tampoco hay fin de serie: una repetición dura hasta que alguien la
borra, porque un `until` que nadie pone es una columna vacía y uno puesto mal es
una junta que desaparece sin que se sepa por qué.

Se guarda **una fila con su primera fecha**, no una por ocurrencia: las
siguientes se calculan al dibujar (`frontend/src/lib/repeat.ts`). Una tabla con
una fila por lunes crece sola, hay que podarla, y mover la junta obliga a
reescribir todo el futuro en vez de un campo. Y por eso mismo una ocurrencia
suelta **no se arrastra** a otro día: mover una de la serie es una excepción, y
las excepciones son lo que se dejó fuera al elegir el menú cerrado.

## El ciclo NO es una rejilla del calendario

Estaba planeado «pintar dónde empieza y termina el ciclo actual», y al ir a
hacerlo resultó estar mal planteado: **no hay un ciclo del departamento**. El
ciclo se mide desde el último calor de CADA burbuja (`cycleLeft(warmAt, …)`), no
desde una fecha común, así que una burbuja calentada ayer y otra calentada hace
una semana están en puntos distintos del suyo. Una rejilla global tendría que
inventar un ancla que el modelo no tiene, y diría algo falso de casi todas.

Lo que sí faltaba es más pequeño y es verdad: el tablero dice «en silencio 2
ciclos» y nadie sabía si eso son dos días o dos semanas, porque `cycle_hours` se
recalibra desde Ajustes y cambia el veredicto sin migración. Ahora la duración
del ciclo se dice junto al filtro de bandas, que es donde se lee el número.

## Instalable, y sin escritura sin conexión *(built)*

La aplicación se instala desde el navegador: un manifiesto, los iconos y un
`theme-color`. Los PNG se generan del mismo dibujo que el favicon
(`frontend/scripts/icons.mjs`) en vez de dibujarse aparte — dos dibujos del
mismo logo se separan en cuanto alguien toca uno.

**No hay service worker, y es una decisión.** Lo que se rechaza es la escritura
sin conexión: cada escritura va por el servidor, que la anota en `events` y hace
un commit de git, y `edit` exige el hash del documento que se leyó. Escribir sin
red significa inventar resolución de conflictos contra git y contra el calor —
dos cosas que el modelo define con precisión y que un buzón de cambios pendientes
volvería aproximadas.

La lectura sin conexión es otra pregunta y está abierta: cachear el shell y lo
último visto no rompe nada. No se hace ahora porque sin lo anterior el valor es
pequeño — se vería un tablero viejo sin poder tocarlo — y porque una caché es
una segunda copia que hay que saber invalidar.

## Las fechas vuelven a bajar: una tarea tiene la suya *(built)*

`1788508000_dates_move_up.go` quitó `threads.due_date` y subió los valores a las
burbujas. Media razón sigue en pie: lo que el lead pone en el calendario es
cuándo tiene que estar un CUERPO de trabajo, y eso es de la burbuja. La otra
mitad —que repartir ese plazo entre las piezas no necesita un campo— no se
sostuvo.

**Sí lo necesita.** Sin fecha por pieza no se puede decir «esto vence el jueves»
de nada más pequeño que un módulo. Con cinco tareas dentro de una burbuja que
vence el 30, las cinco parecen vencer el 30, y la que en realidad bloquea a las
otras el martes no se distingue de las demás. Repartir el plazo en la cabeza de
quien ejecuta es justo lo que se pierde cuando esa persona no está.

**Las dos fechas son independientes a propósito.** La de la burbuja es el
compromiso; la de la tarea es cómo se reparte. Se **rechaza** derivar la de la
burbuja de sus tareas —que es lo que hacía el traspaso de aquella migración— por
dos motivos: deja dos sitios diciendo el mismo plazo, uno de ellos calculado, y
hace imposible decir «la burbuja entera vence el 30» cuando ninguna pieza suelta
lo hace.

**Los valores viejos no vuelven.** Aquella migración los subió y su `down` ya
decía que no los devuelve. Lo que hay en las burbujas se queda; las tareas nacen
sin fecha, que es la verdad — nadie ha repartido nada todavía.

**El calendario lleva las dos, sin conmutador.** Son la misma pregunta —«¿qué
vence?»— y esconder la mitad detrás de un interruptor que hay que recordar es
cómo se llega tarde a algo que estaba escrito. Si se llena, lo que lo acota son
los filtros del tablero, que valen para los dos.

## En `assets/` vive lo que se ADJUNTA, no sólo lo que se ve *(built)*

`assets/` admitía sólo imágenes, así que un contrato en PDF o un CSV exportado
no tenían dónde ir. Ahora admite además `.pdf`, `.txt`, `.csv` y `.json`.

**Van en git, dentro del repositorio del proyecto.** Se rechazó la alternativa
obvia —un `FileField` en `threads`, como el que ya tiene `inbox_items`— porque
rompe en silencio lo que el producto promete: el markdown en disco es el
registro, y un documento que cita un archivo guardado fuera del repositorio deja
de ser legible desde un clon. Clonar el proyecto tiene que traer el trabajo
entero.

**Lista blanca y corta, no lista negra.** Git guarda cada versión de cada byte
para siempre, así que lo que se deja entrar decide cuánto crece el repositorio.
Sin comprimidos, sin binarios, sin ofimática: un `.zip` de 5 MB por adjunto
infla un repositorio rápido y nadie puede leerlo desde un clon. Con una lista
negra no se podría razonar sobre eso.

**`.md` no entra**, aunque es texto. Un markdown ya tiene casa —`docs/` o la
carpeta de su hilo—, y un segundo sitio para lo mismo es cómo se pierde. Además
es la única extensión de la lista que la API de documentos sabe escribir:
admitirla en `assets/` sería una puerta para escribir un documento por la
ventana de subida. Todo lo demás sólo puede llegar subido.

**La respuesta de subida dice si es imagen.** Un `![]()` se ve dentro del
documento y un `[]()` se abre; quién es cada cosa lo decide `internal/tree`, y
el cliente lo pregunta en vez de guardar su propia lista de extensiones —dos
listas se separan, y la que se queda atrás escribe la referencia equivocada.

**Las cajitas de la UI se derivan del documento.** Un adjunto se dibuja porque
el markdown lo referencia, no porque exista una fila en una lista de adjuntos.
Borrar la referencia quita la cajita, que es lo que cualquiera espera. Una lista
paralela acabaría discrepando de lo que el documento dice.

## Artefactos efímeros: pensar antes de que exista el trabajo

Lo que se piensa antes de escribir un hilo —un interrogatorio o un documento
Markdown efímero— vive en `artifacts` + `artifact_entries`, cuelga de una nota del inbox y
se ve en la aplicación mientras el agente pregunta desde la terminal. El primer
`kind` es `grill`: preguntas estructuradas (`id`, ronda, cuerpo, opciones,
recomendación, `depends_on`) que la persona acepta o cambia en
`/artefacto/<id>`. El árbol y la frontera se DERIVAN de `depends_on`.

- **Entradas que sólo se añaden**, no un JSON por artefacto: el agente escribe
  una ronda mientras la persona contesta otra, y con un solo documento los dos
  escribirían la misma fila. Escribe sólo el servidor (orden, autor, validación
  por clase en Go).
- **No calienta nada** y no entra en git: es pensar, no producir. Lo que vale
  la pena pasa al hilo en el traspaso (`artifact_handoff`), que se escribe para
  sostenerse solo —decisiones con lo que rechazaron— porque **el artefacto se
  borra** `tuning.artifact_days` (30) después de su último movimiento, salvo que
  se fije. La nota del inbox no caduca: es el backlog.
- **Lo ven quien lo abrió y el lead global.** La caja de la esquina (en todas
  las pantallas) lista los tuyos y dice si tu agente llamó al MCP en los últimos
  5 minutos (`agent_tokens.last_used_at`).
- **Protocolos del binario** (`prompts/grill.md`, adaptado de
  `grilling` de mattpocock/skills, MIT, y `prompts/ephemeral-markdown.md`), que
  viajan en cada respuesta de `agents_md` y se editan desde Prompts. Bubble no
  corre ningún modelo.

Rechazado: el interrogatorio dentro de la hoja del inbox (Q36) — se pidió una
caja propia y una pantalla con dirección, que es el enlace que el agente da.
Rechazado: dos prompts según si el harness tiene la skill instalada — el
servidor no puede saberlo, y dos métodos se separan. Rechazado: dibujar el árbol
el agente — un dibujo guardado es otra copia que puede mentir.

**El agente se entera solo de que contestaste.** Ninguna IA se despierta desde
fuera cuando ya terminó su turno, así que es el agente quien espera, con lo que
tenga a la mano. Para un `grill` abierto, `artifact_append` devuelve un `watch_command`: un bucle de
`curl` contra `GET /api/artifacts/{id}/wait` que imprime UNA línea por cada
«Enviar» y sale cuando el artefacto se traspasa o desaparece. En Claude Code va con la tool
`Monitor`, que entrega cada línea al agente; con otro shell, en primer plano; sin
shell (apps de escritorio), `artifact_wait` por MCP en bucle de 45 s.

El script no puede usar el token del agente —vive en la configuración del
harness, y sólo abre `/mcp`— así que cada artefacto lleva una **llave de
vigilancia** (`watch_key`, oculta) que sólo contesta «¿ya contestaron?» y muere
con él. La aplicación dice «tu agente espera» mientras hay una espera abierta
(en memoria: es un hecho de las conexiones de este proceso, no una fila).

Rechazado: notificaciones MCP (`resources/subscribe`) — los clientes no
despiertan al modelo con ellas. Rechazado: escribir «listo» en el panel del
agente con tmux — sólo sirve en terminal y depende del multiplexor. Rechazado:
abrir la API REST al token del agente para el script — rompe la regla de que ese
token abre el MCP y nada más.

El razonamiento completo, pregunta por pregunta, en
`docs/grill/2026-10-08-field-feedback.md`.

**Hecho cuando:** un interrogatorio real de Cuby llega del harness al traspaso
contestado en la aplicación, y el usuario lo valida.

## El kanban se quita; el planeador muestra lo capturado flotando *(built)*

Probado en Cuby con un equipo de cuatro: las burbujas y los hilos salían más
rápido de lo que un lead los colocaba en columnas, y el plan se pasó a una junta
semanal. Se quitan las dos tablas de columnas (`stages`, `thread_stages`), el
campo que colocaba cada fila (`stage`), el orden a mano dentro de una columna
(`bubbles.rank`), la opción `stage` de `set_bubble` y el componente entero. En
su sitio, el planeador elige UNA vista desde el selector inferior: Inbox,
Calendario o Secuencia (si está encendida). Por defecto abre el inbox. Las notas
flotan con la misma esfera de 84 px y el nombre debajo que el board principal,
sin halo exterior. No hay barra de filtros ni campo de captura arriba.

El TTL que se ve junto a una nota pertenece a sus **artefactos**, no a la nota:
se muestra la caducidad más cercana (días, horas o minutos), o «fijado» si todos
están fijados. El hover desglosa cada artefacto y aclara que la nota permanece.
`/api/artifacts?scope=inbox` devuelve los del equipo sólo al lead global; la
lista del cerebro, sin ese scope, sigue siendo personal.

Esto deshace «El planeador orquesta BURBUJAS, en ETAPAS del departamento» y
«El orden de una columna». `down` devuelve el esquema, no en qué columna estaba
cada cosa. La vista larga que lo sustituye es la de objetivos (grill Q1),
todavía sin construir.

## La secuencia del planeador se arma en un canvas *(built)*

El panel de columnas se sustituye por un lienzo punteado con pan y zoom que
ocupa **todo el cuerpo**, debajo de la cabecera principal «← board / Planeador».
Se descartó extenderlo detrás de esa cabecera: la navegación conserva su franja,
y sólo el contenido es canvas. Toolbox y botones se superponen dentro del cuerpo;
desplazar el lienzo no los mueve. El drag de fondo y de nodos no selecciona texto;
el campo de búsqueda conserva su selección y edición normales.

La caja de elementos sin secuenciar contiene el selector Módulos / Hilos, cuyo
ancho se ajusta a sus botones, no se estira al ancho de la caja. Al cerrar se
contrae con una animación que respeta `prefers-reduced-motion`, y su contenido
queda inerte.

Arrastrar coloca un nodo; en una línea vacía el primero inicia el paso 1.
Mantener el «+» bajo A y soltarlo sobre B dice «B va después de A». Dos destinos
desde A van en paralelo. Los pasos con varios elementos se dibujan dentro de
una región común «En paralelo · Paso N», calculada desde sus posiciones; el
mismo número también aparece en cada nodo. Moverlos a alturas diferentes no
cambia su paso. La «×» saca un nodo de la línea y lo devuelve a la caja; el
nombre abre su trabajo.

Crear desde «+ nuevo» relee también la cola paginada, no sólo el board, y elige
el modo correspondiente a lo que nació (hilo o módulo). Antes el registro se
creaba pero no aparecía en el toolbox hasta cambiar de modo. El botón limpia
la búsqueda de la caja para no ocultar con ella lo recién creado.

**No cambia el registro**: `sequence` sigue siendo un número, compartido por
los elementos de un mismo paso. Las flechas se derivan entre todos los nodos
de pasos consecutivos: esto es una secuencia con paralelismo, **no un grafo de
dependencias arbitrarias**. Se rechazó guardar aristas aparte en esta iteración:
serían otro orden capaz de contradecir los pasos existentes. Dibujar una
conexión hacia un nodo anterior lo recoloca, no crea un ciclo.

Las posiciones, el encuadre y los ids colocados pero aún sin conectar se
recuerdan por persona y modo en este navegador, no se comparten entre máquinas.
Los nombres y el acceso se releen del servidor; no se guardan copias del trabajo
localmente. Mientras se guarda el orden se bloquea otra conexión, para que dos
gestos seguidos no sobrescriban los pasos desde una lectura vieja.

Verificado con Chromium contra una base aislada: módulos e hilos, serie y
paralelo, mover, quitar, abrir, cambiar de modo y persistir posiciones tras
recargar. Verificadas además la creación por «+ nuevo» sin recargar, el cambio
de modo al crear otra clase de trabajo, los controles fijos durante pan y los
anchos intermedios de la animación. Doce pruebas del orden y de las regiones
paralelas se ejecutan en `just check`.

## Calendario de papel y accesos personales a hilos *(built)*

Mes, semana y agenda comparten una hoja cálida con textura tenue, cantos,
sombra y anillas, de hasta 1160 px de ancho, centrada en el cuerpo. Se reutiliza
el icono 3D del calendario; no hay otra pila de rendering ni imagen que estirar
con la grilla. La revisión siguiente hizo la hoja cuadrada (`aspect-ratio: 1`,
con mínimo de 680 px para ventanas estrechas), en un contenedor principal con
scroll y espacio inferior para el HUD. Su cabecera queda sticky: mes y año se
eligen con `FilterPick`, también hay saltos de año por flechas. La colección del
combobox se inicia con sus opciones, no vacía: el valor preseleccionado debe
mostrar su etiqueta desde el primer render, no «cualquiera».

Se rechazó el rango fijo de repeticiones «dos meses atrás / catorce adelante»:
`datesSet` pasa la ventana visible al planner, de modo que saltar varios años
también proyecte los eventos recurrentes en ese año. No se guarda una fila por
ocurrencia. Las tres vistas, la hoja cuadrada, el clearance del dock, los
controles sticky, saltos de mes/año y recurrencias a seis años fueron probados
en Chromium, con ambas paletas y ventana estrecha.

El papel tiene su paleta clara y oscura y el título sigue el rango visible al
navegar. La prueba con fechas encontró otro fallo: el board
buscaba `due_date` en la vista de evidencia, que no la proyecta. Ahora lee el
plazo de la fila del hilo, junto con prioridad y secuencia, tanto para un
workspace como para el board agregado. Dos pruebas live fijan ese contrato.

El pin es un **acceso personal a un hilo**, no el pin de un artefacto que impide
su caducidad. Se guarda sólo su id en `prefs.value.thread_pins`, privado incluso
frente al lead global. La revisión siguiente revirtió la ubicación: el pin
vive a la derecha del cerebro en el HUD, también dentro del hilo; ahí fija o
quita el hilo actual y abre los accesos personales. Ya no hay otro pin en la
cabecera. Los nombres, proyectos y
permisos se releen; un hilo borrado o sin acceso no se muestra. Fijarlo no cambia
calor, prioridad, secuencia ni producción.

Se rechazó un flag compartido en `threads` y guardar nombres en localStorage:
quien fija es el lector y su decisión debe seguirlo entre dispositivos, no
cambiarle el tablero a los demás. `setPrefs` ahora recibe parches, relee el saco
y serializa los guardados de esta app para que cambiar Módulos / Hilos no borre
los pins. No es una transacción entre navegadores: dos cambios simultáneos del
mismo campo en dispositivos distintos siguen siendo último guardado gana.
Si falla la lectura inicial de pins no se permite sobrescribir a ciegas; se
muestra el error con un botón para reintentar.

La cabecera del hilo no repite el nombre de la banda como otro estado, ni la
misma prioridad de hilo y burbuja; se quita el botón de etiquetas que no tenía
acción. Un estado distinto o prioridades distintas conservan su información.

Verificados en Chromium: las tres vistas y ambos temas, drag sin selección,
ancho del selector, pin tras recargar, navegación por el acceso, quitarlo y
cambiar de modo sin perderlo. El menú también se abre con teclado en el board
y se mantiene dentro del viewport al estrechar la ventana. Probado un fallo de
lectura de preferencias: muestra el error, no escribe y permite reintentar.
Cuatro pruebas frontend cubren la fusión y los fallos de preferencias; tres
live prueban persistencia y privacidad de pins.

## Lectura persistente, checks y controles de atención *(built)*

Los docks de repositorios, artefactos e hilos fijados usan el mismo lenguaje:
objetos 3D sin un círculo detrás, barro blanco cuando están vacíos y colores
naturales cuando contienen algo. Cerebro rosa, caja kraft y pin rojo; los dos
assets nuevos se generaron con Codex CLI usando los originales como referencia.
No es el color de la prioridad ni del calor. Se rechazó pintar el objeto por
hover: ahora la presencia de contenido decide su color, y hover sólo lo eleva.

Usuarios abre por hover, foco o click un roster del workspace. El `+` invita a
personas existentes; `−` pide confirmación para quitar su membresía, no borrar a
la persona. Leads pueden actuar; miembros sólo ven el roster. No se puede sacar
al último lead. El editor de roles y del perfil sigue disponible desde la lista.
«Conectar tu IA» queda en el menú de la barra lateral, no en el HUD. Las acciones
laterales `…`, borrar página, quitar repo/pin e invitar/expulsar igualan el alto
interior de su fila y siguen siendo cuadradas.

La memoria de lectura pertenece al navegador: user + identidad del documento +
contenedor de scroll. Se comparte por todos los `Prose`, incluidos los que
usan el viewport, y el editor recuerda su propia posición. No hay preferencia
ni evento en el servidor. Hilo, página auxiliar y wiki mantienen posiciones
independientes; un documento nuevo comienza arriba. Se guarda la última posición
observada, no el cero de desmontar el DOM, y restaurar cede a la interacción del
lector o a un hash de navegación. Una actualización del hilo no debe cambiar
la página auxiliar seleccionada; lecturas tardías de otro archivo se descartan.

Los checks de documentos ahora guardan desde `DocEditor`: hilo/wiki hacen el
PATCH quirúrgico con hash base; conflicto o fallo revierte el control y muestra
el error. El render asigna `data-todo-index` desde el AST a la línea de fuente:
no usa el orden de inputs visibles, porque un ejemplo cercado ocupa un índice
en el motor de checklists actual pero no produce un input. Los checks sin una
operación de guardado o sin dirección válida quedan deshabilitados. No se
cambió el espacio de índices del motor ni se añadió un template.

La columna general de lectura/edición llega a 1200 px sin forzar un mínimo que
rompa móvil. Se quita el hard-wrap global del renderer: un salto lógico de fuente
no introduce un `<br>`; los saltos explícitos de Markdown se conservan. El prompt
compartido `markdown`, editable en Prompts y adjunto a cada `agents_md`, indica
párrafos de una línea lógica, sin cortes a 72/80 columnas ni HTML/CSS para forzar
ancho. Se rechazó pretender que Markdown controle el `min-width` de la UI.

Probado en Chromium contra una base desechable: invitación/expulsión real,
protección del último lead, miembro sin acciones, acceso desde el menú, pin junto
al cerebro, check por mouse y teclado, rollback por conflicto y persistencia de
scroll de hilo/auxiliar/wiki/Prompts/artefacto. Pruebas Go fijan dirección de
checks (cercados, anidados, Unicode y CRLF) y saltos de línea; las live fijan
persistencia del tick y que la guía va a ambos roles de `agents_md`. Verificación
final: `just bundle`, `just check` (552 live y 23 frontend, sin fallos; Svelte
sin errores ni warnings) y suite Chromium sin errores de página.

## El archivo elegido se recuerda en el navegador *(built)*

Hilos, Wiki y Prompts recuerdan el id del archivo elegido en localStorage,
separado por usuario y contexto (workspace/hilo para Hilos, workspace para Wiki,
departamento para Prompts). No se guarda contenido ni se escribe una preferencia
compartida. Se valida contra la lista actual que devuelve el servidor: si falta,
se borró o dejó de estar disponible, se elige el primer archivo. El filetree
recibe explícitamente esa selección para marcarlo desde el primer render.

La URL explícita de Wiki/Prompts tiene prioridad. Abrir la Wiki desde el HUD o
su dirección sin archivo resuelve la memoria —o el primer archivo disponible— y
reemplaza esa dirección por la página real, sin agregar un salto de historia.
Hilos restaura su página auxiliar sólo después de conocer los archivos de ese
hilo, nunca con la lista del anterior. Crear/seleccionar/borrar actualiza esa
memoria. Se rechazó recordar una ruta sin validar ni forzar README como primer
archivo: el primero es el que realmente aparece en ese árbol.

La caja kraft tenía el mismo tamaño de `<img>` que la blanca, pero más margen
transparente en el asset. Se normalizó el encuadre por el cuerpo visible, no por
los píxeles casi transparentes: en la imagen de 400 px ambos cuerpos y centros
difieren como máximo 3 px (menos de medio píxel en el dock). No se agrandó el
botón ni se añadió otra transformación al hover.

Verificado con Chromium: selección inicial, recarga y retorno al auxiliar,
aislamiento entre hilos, archivo borrado, retorno a Wiki desde HUD, enlaces
explícitos, Prompts y memoria inválida. También los bounds de ambos assets y
capturas de la caja vacía/con repos. `just bundle` y `just check` pasan: 552 live,
28 frontend; Svelte sin errores ni warnings. Cinco pruebas frontend cubren la
selección, aislamiento, storage bloqueado y dirección de Wiki.

## El menú pertenece al tile del proyecto *(built)*

En el sidebar expandido, el tile completo del proyecto —navegación y `…`—
pinta el fondo y el borde, con el mismo ancho y alineación que Todos/Secuencia.
El menú sigue siendo un botón hermano, no un botón anidado: pulsarlo no debe
navegar al proyecto. Se mantiene cuadrado y su hover se dibuja con un inset de
3 px dentro del tile, sin un bloque suelto fuera de su borde. El hover/foco de
cualquiera de los dos controles resalta toda la fila y conserva el baño cálido
de un proyecto produciendo. En el rail estrecho queda el trigger original.

Se rechazó encoger el área seleccionada para hacer sitio al menú fuera: el
menú es parte de la fila, no otra columna. Verificado en Chromium: bounds
iguales a Todos/Secuencia, menú contenido y cuadrado, hover cálido, oscuro/claro,
nombre largo truncado, renombrado real y confirmación de borrar desde teclado.
`just bundle` y `just check` pasan (552 live, 28 frontend, Svelte sin warnings).

## Markdown efímero y planeador de lectura por participación *(built)*

El segundo `kind` es `markdown`: el agente publica mediante MCP y la persona
lee en `/artefacto/<id>`, sin editor ni checkboxes escribibles. Comparte cerebro,
captura, permisos, caducidad, fijado y traspaso con el grill. La migración añade
el valor al selector; no hay una colección ni un árbol git paralelo.

Cada entrada `document` lleva `{filename, markdown, base}`: nombre plano `.md`,
fuente completa de hasta 128 KiB y secuencia de la publicación que reemplaza
(0 al principio). Se conservan las revisiones como entradas inmutables; una base
vieja rechaza el lote entero. La vista se deriva de la última publicación,
incluso con `artifact_read(since)`. La web usa goldmark + Prose, con diagramas,
imágenes plegadas y memoria de lectura; MCP recibe fuente, no HTML. La puerta
web de respuestas no escribe documentos y la API de registros sigue cerrada.
Se revalida que el artefacto siga abierto dentro de la transacción de publicación,
para no resucitarlo si se traspasó mientras el agente esperaba el lock.

Rechazado: editar este documento en Bubble —se pidió sólo publicación del agente—;
y tratar revisiones como notas acumuladas del grill: hay un documento vigente,
no una conversación. Rechazado: meterlo en Wiki o Hilos para prestarle su UI,
porque convertiría un borrador que caduca en producción. El protocolo nuevo se
sirve en todos los `agents_md`, se edita desde Prompts y no propone watcher.

El cerebro muestra **ocho** entradas como máximo; después `+N` abre
`/planeador/inbox`, de sólo lectura incluso para un lead. Clic en el cerebro
abre también ese Inbox; hover/foco siguen ofreciendo los accesos individuales.
Ambas entradas conservan la ruta de origen para el botón volver. El contador usa todos
los artefactos propios, no una lista truncada a cien. La captura ofrece enlaces
a sus artefactos; sigue siendo navegable tras traspasarse mientras alguno viva.
No hay formulario, promoción, descarte ni autosave en esta hoja read-only.

El planeador para operadores deja de ser sólo Agenda: ofrece Inbox, calendario
y secuencia si está habilitada. `GET /api/planner/read` recorta en el servidor:

- Membresía del workspace como frontera exterior, incluso si hay una asignación.
- Burbujas de las que es responsable o con al menos un hilo asignado a él.
- Hilos asignados a él, incluidos los sueltos; también los hijos de módulos que
  tiene a cargo. Estar asignado a un hijo no expone sus hermanos ajenos.
- Capturas y artefactos propios, y sólo nombres de proyectos presentes en el
  resultado. El lead global conserva su alcance departamental.
- Fechas de ese trabajo; las juntas departamentales siguen siendo compartidas.

El board que alimenta el buscador global usa esa misma proyección en estas
rutas: no queda una lista general oculta detrás de la vista filtrada. Sus
comandos de creación desaparecen y busca títulos del subconjunto, no el texto
completo del workspace. Lecturas tardías de otro contexto se descartan. Este
recorte es **del planeador de lectura**: no cambia los permisos ordinarios del
board operativo ni la Wiki dentro de un workspace.

Rechazado: esconder sólo tarjetas en Svelte manteniendo una respuesta general;
y equiparar pertenecer a un workspace con participar en todas sus burbujas.
Verificado: validación, base vieja y rollback de lote; fuente/HTML y read desde
`since`; privacidad, ausencia de calor, captura tras handoff, responsables,
asignados, hermanos y frontera de membresía. Chromium prueba ocho +3, Inbox
para operador, documento con checkbox deshabilitado y Mermaid, secuencia
personal, calendario, buscador acotado, ruta de lectura para lead y ausencia de
escrituras al navegar. `just bundle` y `just check`: 575 live, 30 frontend,
Svelte sin errores ni warnings. Sin commit ni despliegue.

## Volver al contexto, canvas de lectura y affordances de interfaz *(built)*

Los accesos del HUD (pins, artefactos y el Inbox de lectura) guardan una pila de
retornos por usuario y sesión: destino y ruta desde la que se abrió. Volver
consume sólo el destino actual; un pin de otro proyecto no sustituye el origen,
y artefacto → pin → volver se deshace por niveles. Sobrevive a una recarga y
rechaza URLs externas; si el navegador bloquea storage conserva memoria local.
Rechazado: deducir el origen del workspace del elemento abierto, o del primer
workspace al que pertenece la persona.

El proyecto cambia la inicial por la ilustración de su banda, derivada de su
burbuja más caliente; el tooltip muestra la última producción disponible de sus
burbujas y se refresca cada minuto. Es un **Tooltip real sobre el ícono**, con
120 ms de delay y portal, no el `title` nativo lento que se perdía con el del
botón padre. Usa un span de trigger (sin botones anidados), conserva el clic de
navegación y ofrece la misma información mediante `aria-describedby` en la fila.
Sin evidencia, dice «Sin producción registrada», no inventa «ahora». No se
almacena una clasificación de workspace. Chromium validado claro/oscuro y rail,
con recencia relativa, ausencia de producción y navegación desde el ícono.

`SequenceCanvas` admite lectura: pan, zoom, fit y navegación, pero sin toolbox,
conexiones, completar, sacar ni mover nodos. Se reutiliza en `/secuencia` y el
planeador de lectura. La secuencia de operador tiene Tareas/Módulos, conserva
el alcance por participación en servidor, y el buscador global usa también ese
alcance. Los leads conservan Organización/Lo mío. Rechazado: mantener la lista
vertical como una segunda visualización del mismo modelo.

En Todos se pide una vez el directorio autorizado para resolver responsables
y asignados del drawer; no aparecen IDs como nombres ni hacen falta N rosters.
Eliminar burbuja es una acción explícita con confirmación del nombre/proyecto,
visible sólo para lead global o de ese workspace. Es la **última opción** del
drawer, debajo de cerrar/reabrir, y del menú contextual del orbe; ambas puertas
usan la misma confirmación y el ID objetivo, sin necesitar abrir el drawer para
borrar desde clic derecho. La regla del servidor sigue siendo la autoridad. Eliminar **no** borra hilos ni documentos: quedan sueltos;
cerrar conserva la burbuja y se puede deshacer. La acción puede abrirse desde
Todos porque su objetivo es un ID y workspace concretos en la confirmación.

Mes conserva la hoja cuadrada. Semana y agenda expanden la hoja con `height:
auto` y usan el scroll del contenedor principal, no un segundo scroll interior.
Mes/año tienen controles más anchos y los menús de FilterPick tienen altura
máxima y scroll propios, acotados al viewport. Rechazado: limitar la franja de
horas para ocultar el segundo scrollbar, porque quitaría parte del día.

El cursor comparte una familia de trazo redondeado y contraste: flecha,
dedo, mano abierta/cerrada y mano de redimensionado, en claro/oscuro. Las manos
y el dedo se diseñan como SVG y se sirven rasterizados en PNG transparente de
32×32 para no caer al cursor nativo cuando un navegador rechace cursores SVG. La mano
abierta inicialmente tenía seis dedos: corregida a cuatro dedos y un pulgar y
revisada visualmente a escala ampliada. Los scrollbars estilizados de Blink/
WebKit también declaran el cursor; un scrollbar overlay controlado por el SO
puede ignorar CSS, y no se promete poder sustituir esa affordance nativa.
La regla universal ahora hereda el cursor del hit target: no vuelve a poner
flecha sobre el texto, la imagen ni los pseudo-elementos de un botón/menuitem.
Cubre globalmente botones, enlaces, menús en portales, opciones, tabs, árboles,
checkboxes y demás inputs clickeables, eventos del calendario, arrastres y
separadores de redimensionado. La auditoría de estilos computados en las vistas
probadas no deja elementos visibles con cursor nativo; los ocho PNG decodifican
con sus dimensiones correctas.
Rechazado: forzar flecha en cada descendiente, que hacía que sólo la orilla del
menú tuviera el dedo y que un botón dentro del canvas conservara la mano.

Los contadores del dock son círculos de 18px para un dígito y píldoras fully
rounded si necesitan más ancho. El pin abandona la clase global `badge`, cuyo
padding de Skeleton lo convertía en un óvalo; todos declaran box-sizing,
line-height y padding propios.

Verificado en Chromium: retorno desde pin de otro proyecto, artefacto tras
reload, ilustraciones, nombres en Todos, borrado conservando el hilo, canvas
read-only con pan y cambio de modo, dropdown acotado/ancho, un solo scroll del
calendario y cursores por contexto, incluyendo imágenes/texto/pseudo-elementos
de los botones del HUD y del menú completo, y dimensiones/radio de los
contadores de un dígito. `just check`: 575 live, 36 frontend, Svelte
sin errores ni warnings. Bundle reconstruido; sin commit ni despliegue.

## Brain por capas y cursores Retina *(built)*

El anillo de conexión queda detrás del dibujo y del contador, no se pinta al
final por ser `::after`: el botón aísla su stacking context; anillo z=0,
ilustración z=1, número z=2. También conserva ese orden al hacer hover.

Los PNG de cursor de 32×32 eran suficientes en 1x pero se pixelaban en Retina.
Se rasterizaron los mismos SVG a 64×64 y se sirven con `image-set` de densidades
1x/2x, sólo cuando el navegador soporta esa sintaxis para `cursor`. El cursor
sigue midiendo 32 píxeles CSS y conserva su hotspot; el fallback 1x sigue siendo
PNG. Rechazado: usar el PNG de 64 como un cursor de 64 CSS px, que lo agrandaría,
o volver al SVG sin fallback y reintroducir el cursor nativo.

Chromium verificado en DPR 1 y DPR 2: capas del anillo/brain/badge, carga de la
imagen de la densidad correcta, 2x de 64×64, pan y ambos temas. Bundle
reconstruido; `just check` pasa (575 live, 36 frontend, Svelte sin errores ni
warnings). Sin commit ni despliegue.

## Editor, contexto de módulo, secuencia entre módulos y Mi espacio *(built)*

**Ortografía.** El editor Markdown (CodeMirror) y el título de la tarea ya no
desactivan el corrector: `spellcheck`, `autocorrect` y `lang="es"`. Las
sugerencias son las del navegador, con **clic derecho**, y la corrección pasa por
el historial del editor y se guarda como cualquier escritura. Rechazado: un
corrector propio con sugerencias al seleccionar o clic izquierdo — un segundo
diccionario y otra UI para lo que el sistema ya ofrece.

**Formato.** `/negrita`, `/cursiva`, `/subrayado` en el menú `/` (en línea, no
abren bloque nuevo), `Ctrl/Cmd+B/I/U` y una barra B·I·U encima del editor de
hilos, wiki, prompts y campos Markdown. Con selección envuelve; aplicarlo de nuevo
lo quita; sin selección deja un marcador seleccionado. El subrayado es
`++texto++`: extensión inline de goldmark que sólo emite `<u>`; el renderer sigue
escapando HTML crudo y código. Rechazado: habilitar HTML (`<u>`) en Markdown.
La guía `prompts/markdown.md` lo enseña a los agentes.

**Dónde estoy.** El hilo lleva su ruta dentro de la barra que ya tenía, entre «←
board» y el nombre editable, igual en renderizado, markdown y cambios:
`Proyecto / Módulo / #N Tarea [/ página]`. Proyecto y módulo son botones en
píldora, como «← board» (enlaces con forma de botón: ⌘/Ctrl+clic abre otra
pestaña), separados por `/`; los largos se cortan con «…». Rechazado (a pedido
del usuario): flechas «→» y enlaces de texto. Rechazado: una segunda barra encima, que repetía el título. Con
proyecto y módulo navegables (el módulo abre el board con su cajón). Tareas sueltas
dicen «Sin módulo». No se escribe en el documento: renombrar o mover la tarea no
deja un encabezado viejo.

**Secuencia entre módulos.** El modelo no cambia: `threads.sequence` es UNA
línea compartida del departamento; valores iguales = paralelo; las flechas se
derivan. En modo Hilos cualquier tarea se encadena con cualquier otra — del
mismo módulo o de otros — con el `+`. Cada nodo muestra ahora
`Proyecto · Módulo` en su propia línea (nodo 128 px) y el buscador del toolbox
encuentra por módulo. Es orden, no dependencia: no bloquea nada. El lead la
edita; la vista de quien opera es la misma secuencia, de lectura.

**Mi espacio** (`/mi-espacio`). Escritorio personal: notas adhesivas Markdown
con checkboxes, color de papel, proyecto opcional y adjuntos; archivos e imágenes
sueltos; y colores personales por proyecto. Colecciones `personal_notes` y
`personal_assets` (`1788529000_personal_space.go`) con reglas `owner =
@request.auth.id` en lectura, escritura y borrado, **sin excepción para el lead
global**; el dueño no se puede cambiar; sólo se asocia a proyectos donde se
participa; los archivos son `Protected` y se sirven con token de archivo de su
dueño (20 MB). Borrar una nota se lleva sus archivos. No calienta, no entra en
git, no aparece en el board. Los colores viven en `prefs.project_colors` y tiñen
calendario y secuencia **sólo para esa persona**; no sustituyen banda, calor ni
prioridad. Rechazado: modelarlo como burbuja (una burbuja tiene resultado,
dueño y muere; esto no) y guardar contraseñas (fuera de alcance).

Validado: 611 live (privacidad frente a otro usuario y al lead global, token
de archivo, cambio de dueño, proyecto ajeno, color inválido, cascada, no-heat,
`++subrayado++`), 40 frontend (`formatEdit`, slash en línea), Go del subrayado
(código, `+` sueltos, HTML escapado). Chromium: encadenar tarea del mismo módulo en
paralelo con la de otro y luego una de otro módulo, etiquetas sin truncar, vista
de lectura compartida, ruta del hilo en ambos modos y navegación al módulo,
atributos de ortografía, atajo/barra/slash y su render, notas (Markdown, casilla,
título, proyecto, colores, adjunto, imagen protegida, recarga, borrado), color en
la secuencia, privacidad frente al lead, claro/oscuro/420 px. Sin commit ni
despliegue.

### Mi espacio como lienzo infinito *(built)*

La rejilla de notas se veía como un formulario. Ahora Mi espacio es un lienzo
infinito con los gestos del secuenciador:
- arrastrar el fondo mueve la vista;
- ⌘/Ctrl+rueda acerca alrededor del puntero;
- hay controles −, %, + y «Ver todo».

Cada nota es un post-it libre:
- Arrastrarlo lo mueve; un clic sin mover lo abre para escribir.
- Los bordes cambian el tamaño, cada uno con su cursor: el derecho el ancho (↔),
  el inferior el alto (↕) y la esquina los dos (⤡). Durante el gesto el cursor
  queda fijo (`data-cursor-lock`). Doble toque en el borde inferior o la esquina
  devuelve el alto al del texto. Lo que no cabe se corta con «…» en vez de hacer
  scroll; escribiendo se ve entero.
- El cuerpo reserva al menos 5 rem con cursor de texto, para que se note que ahí
  se escribe aunque la nota esté vacía.
- Mientras se escribe, la nota se arrastra por su marco, y el lienzo se mueve con
  arrastre y rueda sin cerrar el editor. Un clic quieto fuera de la nota lo
  cierra.
- Con el teclado: las flechas lo mueven 16 px (80 px con Shift), Alt+flechas
  cambian su tamaño y Enter lo abre.
- La última nota tocada queda arriba.

El papel es el color de la nota y la franja superior es el color personal del
proyecto. Color y proyecto se eligen en desplegables con la superficie y los
bordes del tema:
- La paleta tiene 12 pasteles con nombre, en una rejilla (←→ / ↑↓).
- El color se abre desde su botón o con clic derecho en la nota. El clic derecho
  abre el desplegable; no cambia el color.
- Cada desplegable se mide en pantalla, ya con el zoom, y se abre hacia abajo si
  cabe; si no, hacia el lado con más sitio.

Al escribir, la nota se ensancha a 560 px. El editor (18 rem) va sobre la
superficie del tema, para que en oscuro no quede texto claro sobre pastel, y
tiene el mismo interruptor de vim que el resto (`:w` guarda, `:q` cierra).

Sin caja de herramientas: su filtro guardado escondía notas sin que hubiera dónde
quitarlo, y el resto no hacía falta en el lienzo. Su estado guardado se borra al
cargar. Un archivo se adjunta soltándolo sobre una nota; soltado sobre el fondo,
avisa en vez de subir un archivo suelto que no se vería. Sin notas, el mensaje de
bienvenida vive en el 0,0 del mundo (se mueve con el lienzo) y «Ver todo» centra
ese origen. En el sidebar, «Mi espacio» está en el grupo superior, antes de
Secuencia.

Cursores: `app.css` hace que todo herede (`* { cursor: inherit !important }`),
así que ningún campo mostraba el I-beam y los `cursor:` de cada componente no
llegaban. Ahora los campos editables (`input` de texto, `textarea`,
`contenteditable`, `.cm-content` y `[data-cursor='text']`) muestran el I-beam en
toda la app, y hay `data-cursor` por eje de resize.

Si el servidor responde 200 sin un campo escrito (le falta la migración), la nota
conserva el cambio en pantalla y avisa que se reinicie el servidor, en vez de
deshacerlo sin razón.

Arreglo de paso: `main.ts` bloqueaba en toda la app el menú contextual nativo
salvo `[data-native-menu]`, que nadie usaba. Así, la ortografía por clic derecho
de R20 nunca funcionó en editores ni títulos. Ahora `input`, `textarea`,
`contenteditable` y `.cm-editor` recuperan el menú nativo.

`x`, `y`, `w` y `h` viven en `personal_notes`:
- `1788530000_personal_note_place.go`: posición ±1 000 000; las notas existentes
  se acomodan en rejilla.
- `1788531000_personal_note_size.go`: ancho hasta 1200 y alto 0–1600 (0 = el del
  texto).

Así el lienzo te sigue a otro equipo, y las reglas de dueño siguen sin excepción
para el lead. Se guarda al soltar, no en cada movimiento. La vista (pan y zoom)
es de cada navegador. Una vista guardada rota (`null`/`NaN`) se descarta y se
reencuadra, y las coordenadas o tamaños no numéricos caen a un valor por defecto:
un solo `NaN` en el `transform` trababa el pan y el arrastre. Rechazado: guardar
la posición en `localStorage`, porque se perdería al cambiar de equipo.

Validado: checks live de posición, tamaño (1000×420), alto y ancho fuera de rango
(400) y el lead sin poder moverla (404). Chromium (`/tmp/bw-pw/r21.mjs`):
- orden del sidebar; una vista corrupta y un filtro viejo se sanan; sin caja ni
  «privado»;
- mensaje en el 0,0 que se mueve con el lienzo;
- editor grande; vim (persistido, conserva el texto, `A` y `:w`);
- arrastre persistido;
- desplegable de color: 12 nombres, foco en el actual, Escape devuelve el foco,
  botón y clic derecho, teclado en rejilla; abre hacia arriba cerca del borde
  inferior y hacia abajo con sitio; el de proyecto con el mismo tema;
- menú nativo en título y editor, no en el fondo;
- resize en las dos direcciones (pantalla = servidor), mínimos, corte con «…»,
  doble toque, Alt+flechas; cursores por borde y fijos durante el gesto; borde
  derecho solo ancho, inferior solo alto, y el alto persiste al recargar; I-beam en
  cuerpo y título; nota vacía con al menos 5 rem para escribir;
- editando: arrastrar la nota, el pan y la rueda no cierran; un clic fuera, sí;
- un campo que el servidor descarta avisa en vez de revertir;
- soltar sobre una nota y sobre el fondo; «Ver todo»;
- claro, oscuro y 420 px; el lead no ve nada; borrado.

### Cursores de resize, paleta de 24, alineación y la barra de Secuencia *(built)*

- **Cursores propios:** esquina (`corner-*`, mano con ↖↘) y borde inferior
  (`vresize-*`, mano con ↑↓), de la misma familia que `resize-*`: mismo trazo de
  dos capas, claro y oscuro, PNG de 1x y 2x renderizados desde su SVG, con
  hotspot 16,16. `--cur-corner` y `--cur-vresize` en `app.css`.
- **Lo arrastrable muestra la mano aunque sea un botón:** la regla del puntero
  pesa (0,2,1). La de la mano iguala ese peso con `html :is(…)` y gana por ir
  después.
- **Paleta de 24 pasteles** en orden de tono, incluidos los 12 anteriores, así
  ninguna nota pierde su color:
  - Rejilla de 4 columnas con el nombre de cada muestra en la letra display.
  - Ancho propio de 21 rem, para que en una nota angosta no se encoja a una
    columna con scroll.
  - Teclado: ←→ de a uno, ↑↓ por fila.
  - Las notas nuevas rotan entre los seis colores de siempre.
- **Selección y alineación en Mi espacio:**
  - Se selecciona con Shift/⌘/Ctrl+clic, con un marco (Shift+arrastre en el
    fondo) o con ⌘/Ctrl+A. Esc o un clic quieto en el fondo sueltan la
    selección.
  - Con dos o más notas aparece una segunda fila del HUD para alinear a la
    izquierda, al centro, a la derecha, arriba, al medio o abajo, contra la caja
    que envuelve la selección.
  - Repartir en horizontal o vertical pide tres o más y deja huecos iguales sin
    mover los extremos.
  - Arrastrar una nota seleccionada, o moverla con las flechas, mueve todas.
  - La selección no se guarda; las posiciones sí, como siempre.
- **La caja de Secuencia es la Navigation de Skeleton:** `layout="sidebar"`
  abierta y `"rail"` plegada (con el contador), acoplada al borde izquierdo y a
  todo el alto del lienzo.
  - Cada tarjeta es un `Navigation.Trigger` arrastrable.
  - La lista nunca hace scroll horizontal: el nombre usa hasta dos líneas y se
    corta aunque no tenga espacios; «Proyecto · Módulo» se corta con «…», y el
    texto entero queda en el `title`.
  - En los nodos del lienzo, proyecto y módulo también se cortan con «…».
  - En un teléfono empieza como rail.
  - Rechazado: seguir con la caja flotante propia, que desbordaba con nombres
    largos.

Validado:
- `/tmp/bw-pw/r21.mjs`: cursores por borde; paleta de 24 en 4 columnas, letra
  display, sin scroll y con 21 rem en una nota de 180 px; selección por clic,
  marco, ⌘A y Esc; cada alineación y reparto comprobados en el servidor;
  arrastre y flechas en grupo.
- `/tmp/bw-pw/r22.mjs`: ruta en una sola fila de la barra, con nombre editable y
  el módulo navegable; sidebar acoplada, sin overflow con nombres larguísimos;
  arrastrar al lienzo secuencia; rail con contador; oscuro; rail en 420 px.

### WhatsApp entra al planeador, y la barra de Secuencia como la principal *(built)*

- **Las entradas de WhatsApp son una tercera pestaña de la barra de Secuencia**
  (Módulos · Hilos · WhatsApp, con contador). Son las mismas notas del inbox con
  `source = whatsapp` sin hilo ni módulo, no una copia, y las ven quienes ya
  las veían: quien encendió el canal y el lead global.
  - Cada fila muestra qué llegó, quién lo mandó (`source_from`) y cuándo.
  - Un clic la abre en la hoja de la nota, con su texto e imágenes.
  - Arrastrarla al lienzo abre el diálogo de «organizar» (proyecto › módulo ›
    nombre) con el nombre ya puesto. El hilo queda en el lienzo donde se soltó,
    y la nota apunta a él, igual que al organizar desde el Inbox.
  - Cancelar no crea nada. Parar en «Crear el módulo» triagea a módulo y no
    deja nodo.
  - Rechazado: que cada mensaje se vuelva hilo solo. Un mensaje no dice en qué
    proyecto va, y adivinarlo es escribir el veredicto sin evidencia.
- **Esto revierte en parte la Q55 del grill del 2026-10-08**, que quería purgar
  WhatsApp y el framework de canales. El canal se queda porque ahora alimenta
  la secuencia. Purgarlo queda abierto si deja de usarse.
- **La barra tiene la apariencia del sidebar principal (`Shell.svelte`):**
  - Es una columna aparte, ya no encima del lienzo, sin fondo propio y con una
    línea a la derecha.
  - Las filas van a todo el ancho, con icono de 26 px y texto del mismo tamaño
    que en la principal; el pie queda fijo.
  - En el rail, el icono va arriba y el nombre abajo, y la lista empieza arriba
    (Skeleton la centra).
  - Las vistas de sólo lectura (`/secuencia` y `/planeador/inbox`) no cambian:
    allí no hay barra.

Validado: `/tmp/bw-pw/r22.mjs` (columna aparte y transparente, tamaño de texto
igual al de la principal medido en `/todos`, pestaña con contador, sólo las
notas de WhatsApp, la hoja al hacer clic, cancelar sin efecto, soltar → hilo en
el módulo elegido, nota enlazada, nodo donde se soltó, contador que baja, rail
alineado arriba).

## Tools `entidad_operacion` y un solo documento de entrada *(built)*

Reemplaza lo que dicen «Tres documentos, y todos en `prompts/`» y las listas de tools de las fases anteriores: aquellas secciones quedan como historia.

- **`agents_md` es la única entrada.** Devuelve un documento: la guía de Bubble para agentes (`prompts/bubble-work.md`, clave `agents_guide` en Prompts), la tabla de tools por categoría, cómo se escribe markdown y, al final, la forma de trabajar del departamento del rol, que manda si contradice lo anterior.
  - La guía explica todas las entidades, incluidas la wiki, los artefactos temporales, los pines (de artefacto y de hilo), el inbox con WhatsApp y Mi espacio. También el ciclo de trabajo y el protocolo de escritura.
  - La tabla se **genera del registro** (primera oración de la descripción vigente y su tipo): no puede nombrar una tool que no existe.
  - Se quitaron las tools `guide` y `house_rules`, y la ruta `/api/house-rules`. La sección breve que un agente se instala vive dentro de `connect.md`. `/api/guide` sirve la guía nueva.
  - La clave es nueva (`agents_guide`, no `guide`), para que una edición vieja no tape el texto nuevo. Las ediciones viejas de `guide`, `house_rules` y `tool/<nombre viejo>` quedan como historia y nadie las lee.
- **Los protocolos llegan cuando se usan.** `artifact_create` devuelve el JSON de siempre y, en un segundo bloque de texto, el protocolo de su tipo (interrogatorio o markdown efímero). `agents_md` sólo dice cuándo ofrecer un interrogatorio. Rechazado: cargarlos en cada sesión, unas mil palabras que la mayoría no usa.
- **Tools `entidad_operacion`, en inglés:** `project_*`, `bubble_*`, `thread_*`, `document_*`, `checkbox_toggle`, `evidence_add`, `comment_*`, `artifact_*`, `sequence_next`, `objective_*`, `repo_list`, `inventory_list` (33 en total; ver la sección siguiente, que las lleva a 64). Los parámetros siguen la misma idea: `project` en vez de `workspace`, y `bubble` para el módulo (la interfaz y la guía dicen «módulo»; las tools, `bubble`, igual que el JSON). En los datos el proyecto sigue siendo `workspace`, y la guía lo dice. Se probó `module_*` y se descartó a favor de `bubble_*`, el nombre del modelo y de los datos.
  - Una forma por tool: `edit` se partió en `document_edit`, `document_replace` y `checkbox_toggle`; `set_bubble` en `bubble_update`, `bubble_close` (exige `closure`) y `bubble_reopen`; `set_objective` en `objective_create` y `objective_update`.
  - Anotaciones MCP (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `title`), para que un cliente pueda aprobar sólo lecturas o confirmar lo que borra.
  - `agents_md` conserva su nombre: es la entrada, y hay hooks que lo llaman por nombre.
  - Rechazado: nombres y parámetros en español (`hilo_crear`), que dejaban dos idiomas frente al JSON en inglés. Rechazado: una tool por entidad con un parámetro `operation`, que vuelve el esquema una unión y diluye las descripciones. Rechazado: alias de los nombres viejos, que duplicaban la lista.
- **Todo lo que lee un agente está en español:** guía, protocolos, descripciones de tools y parámetros, y las frases de ayuda de las salidas. Los mensajes de error del servidor siguen en inglés porque los comparte la API REST; la guía cita los que importan tal como llegan.
- **Corte duro.** Las secciones instaladas en otros repos nombran tools viejas: se actualizan pegando de nuevo el prompt de conexión, que reemplaza la sección completa.

Validado: `just check` (624 live, 40 frontend). Las pruebas cubren:
- la lista exacta de 33 tools y sus anotaciones;
- la guía, las entidades, la tabla generada de 33 filas y la forma del departamento al final;
- el protocolo de interrogatorio fuera de `agents_md` y dentro de `artifact_create`, para los dos tipos;
- el prompt de conexión con la sección y sin `house_rules`;
- `document_edit` vacío rechazado, y `bubble_close` sin frase rechazado;
- una guía editada que se sirve, con la tabla todavía generada.

## La IA alcanza todo lo que no es administración *(built)*

Un token de agente sólo abre `/mcp`, así que las tools son todo lo que una IA ve. Pasan de 33 a 64. La regla nueva: **todo es AI driven salvo la administración**, que se queda a mano en la aplicación (personas, roles, tokens, membresías, canales, quién ve el inventario, prompts). Rechazado: dar tools de administración; un error ahí deja a alguien fuera, o a todos dentro.

- **Inbox: ideas, no trabajo.** Revierte «el inbox es de las personas, ningún agente lo lee». `inbox_list`, `inbox_read`, `inbox_capture`, `inbox_update`, `inbox_triage` e `inbox_delete`, con la misma visibilidad que la aplicación (quien capturó, más el lead global; lo de WhatsApp lo captura quien encendió el canal).
  - La guía y cada descripción repiten lo mismo: una idea no es un hilo, ni un módulo, ni un pendiente. No se ejecuta ni cuenta como avance. Se vuelve trabajo sólo cuando la PERSONA lo decide, y entonces `inbox_triage` anota a dónde fue, sin borrar la nota.
  - `inbox_capture` es donde va lo que surge y no es de ningún hilo. Antes la guía decía «díselo a la persona».
  - Capturar no produce eventos; está probado.
  - Rechazado: que el agente «organice» solo, o que una idea abra un hilo. El riesgo que motivó la regla vieja era un agente trabajando desde el inbox en vez de desde su hilo, y eso se cuida con el texto, no ocultando el inbox.
- **Mi espacio**: `space_note_list/create/update/arrange/delete`. Sólo las notas del dueño del token, ni siquiera las del lead global. Colores por nombre de la paleta (los mismos 24) o `#rrggbb`. Tamaños acotados como en la aplicación. `arrange` mueve varias de una vez, todas o ninguna.
- **Pines de hilo**: `pin_list`, `pin_add`, `pin_remove`, sobre `prefs.value.thread_pins` del dueño.
- **Repositorios**: `repo_link` y `repo_unlink`, sólo para el lead del proyecto o el lead global (la regla de `workspace_repos`). `repo_list` trae ids.
- **Inventario**: `inventory_group_*` e `inventory_item_*`, sólo para el lead global. `vault` tiene que ser un enlace http(s); nunca una credencial. Un grupo con cosas no se borra; primero se vacía. `inventory_list` trae grupos e ids. Quién ve el inventario sigue a mano.
  - Arreglo de paso: la nota de cada cosa salía siempre vacía, porque se leía `note` y el campo es `notes`.
- **Personas, cambios, ficha y relaciones** (lo que faltaba para que la IA oriente y reparta):
  - `people_list`: id, nombre, rol y cuál soy, sin correos.
  - `change_list`: eventos de los proyectos visibles desde una fecha, con nombres y si calentaron.
  - `thread_get`: estado y estados posibles con su grupo, asignados, etiquetas, plazo, relaciones, evidencia y pin.
  - `thread_update` suma asignados, plazo, estado por nombre y etiquetas por nombre (las que faltan se crean). Llevar un hilo a un estado completado deja el mismo evento que completarlo.
  - `thread_relate` y `thread_unrelate`: las dependencias van aquí, no en la secuencia.
- **Artefactos y secuencia**: `artifact_list` (para retomar en otra sesión; `GET /api/artifacts` usa la misma función), `artifact_pin`, `artifact_delete`, y `sequence_set` sólo para el lead global.
- Cada operación nueva está en `internal/bubble/agentops.go` y dice qué regla de colección refleja, porque `app.Save` no las aplica.

Pendiente:
- Mi espacio y los pines no se actualizan solos en una pestaña abierta; lo que cambia un agente aparece al volver a abrirlos.
- Los objetivos siguen siendo sólo del lead global; un operador ve el id del objetivo en su módulo, no su nombre. Abrirlos es otra decisión.

Validado: `just check` (688 live, 40 frontend). Las pruebas cubren:
- la visibilidad del inbox por persona y del lead global, WhatsApp con remitente y mensaje, que capturar no produce eventos, el triaje y el descarte;
- Mi espacio privado incluso frente al lead global, colores, posición por omisión, acotado y «todo o nada»;
- los pines, idempotentes;
- la ficha y la actualización completa de un hilo, incluido el evento de completar y las relaciones;
- `change_list` sin fugas, `sequence_set` sólo del lead, repositorios por rol, artefactos;
- el inventario: sólo el lead escribe, `vault` sólo enlaces, no se borra un grupo con cosas;
- y que no exista ninguna tool de administración.

Nota para las pruebas: en el bash 3.2 de macOS, un JSON escrito en línea dentro de `"$( … )"` con valores que llevan espacios llega partido. Va siempre en una variable (`ARG=…`), como ya decía el script.

## Equipos: contextos aislados, y un equipo personal por lead *(built)*

Motivo: va a entrar un segundo lead global, arriba en el organigrama, con proyectos propios. Cada lead necesita su contexto privado y, además, el del equipo de TI. Hasta ahora «lead global» significaba «ve todo».

- **Equipo** (`teams`, `team_members` con papel lead|member): agrupa proyectos y es dueño de lo que se planea sobre ellos. `workspaces`, `objectives`, `calendar_events`, `inbox_items` y `channels` llevan `team`. La migración `1788532000_teams.go` crea el equipo «TI» y pone ahí todo lo que existía y a todas las personas (los leads globales, como lead). Nadie pierde nada.
- **Visibilidad.** Donde una regla decía «lead global», ahora dice «lead del equipo del proyecto»: el lead global, si el equipo no es personal, o una fila de lead en el equipo. Se reescribieron las reglas de 15 colecciones que cuelgan de un proyecto, más objetivos, calendario, inbox y artefactos. `canSee`, `reachWorkspace`/`reachThread` y los tableros agregados preguntan lo mismo. La membresía de proyecto no cambia: invitar a alguien a un proyecto de otro equipo le enseña ese proyecto y nada más.
- **Un equipo personal** (`personal`, `owner`) es el contexto privado de un lead global: sólo lo ve quien está en él, ni otro lead global. No entra nadie más; a otra persona se la invita a un PROYECTO. Uno por persona.
  - Lo tiene **todo lead global, sin pedirlo**: la migración se lo crea a los que ya existen, y el servidor a quien se da de alta como lead o pasa a serlo. El nombre es «Personal · <nombre visible>» (con el correo si se repite) y se puede renombrar. Quitarle el rol no se lo quita.
- **El rol `lead` es administración** (personas, tokens, prompts, inventario, funciones, calibración) y además hace lead implícito de todo equipo compartido. Ser admin no da acceso al contenido de un equipo personal ajeno.
- **Por equipo:** objetivos (un módulo sólo cuelga de un objetivo de su equipo), secuencia (una línea por equipo; `Next`, la cola y los pasos del tablero se calculan por equipo), calendario, inbox (una idea se vuelve trabajo sólo en un proyecto de su propio equipo) y canal de WhatsApp. El canal guarda su sesión en `pb_data/channels/<equipo>/`; la sesión anterior se muda sola al primer equipo que lo arranca, así no se pide otro QR.
- **Equipo por defecto:** el que la persona eligió en la aplicación (`prefs.team`), si no su primer equipo compartido. Lo que no dice equipo va ahí: proyectos, ideas, objetivos, fechas, y también lo que crea la IA. Una persona nueva entra al primer equipo compartido (lead si es lead global).
- **Mover un proyecto de equipo** es de un lead global que lidere los dos.
- **Interfaz:** selector de equipo arriba de la columna (sólo con dos o más equipos), un menú del tema como el del pie —no un `<select>` nativo— que marca el equipo actual y los privados; proyectos del equipo elegido más «Compartidos contigo»; «Mover a…» en el menú de un proyecto; el planeador, la secuencia de sólo lectura y la agenda trabajan sobre el equipo elegido, con su propio selector; Ajustes → Equipos (crear compartido, crear el personal si falta, renombrar, quién está y con qué papel, borrar uno vacío); Ajustes → Canales con el canal de cada equipo que lideras.
- **IA:** `team_list`, y `team` en `project_create`, `inbox_capture`, `inbox_list`, `objective_create`, `sequence_next` y `artifact_create`.
  - **Las respuestas dicen el contexto:** `project_list`, `project_create`, `bubble_*`, `thread_create`, `thread_update` y `thread_get` traen `team` (id, nombre y si es personal); `change_list` y el tablero traen el nombre del equipo.
  - **Asignar es invitar.** Hacer responsable de un módulo o asignar un hilo a alguien que no ve el proyecto lo suma al proyecto como miembro: ve ESE proyecto y nada más de su equipo, ni de uno personal. Así se invita también desde «Todos», donde el selector ofrece a todas las personas. Sólo si quien asigna puede invitar (lead del proyecto o de su equipo); un miembro común recibe «pídele a un lead que lo invite»: asignar no es una puerta para saltarse quién invita. Vale igual por la interfaz (hooks de `bubbles.owners` y `threads.assignees`, en la misma transacción) y por MCP. `people_list` acepta `project` (quién ya lo ve) y `team` (quién está en él). Rechazado: rechazar la asignación y obligar a invitar antes; rechazado también que cualquier miembro invite asignando.
- Rechazado:
  - un campo `private` en el proyecto (un equipo personal es el mismo caso, y los equipos además separan el plan de cada contexto);
  - separar en dos departamentos (él decide también en TI; se duplicarían objetivos, secuencia e inventario);
  - un rol intermedio de «ejecutivo» (no resuelve la privacidad).
- **Límite:** es privacidad dentro de la aplicación. Quien opera el servidor (superusuario, `pb_data`, repositorios en disco) puede leer todo.

Pendiente:
- `agents_md` sigue siendo uno por rol para todo el departamento; uno por equipo queda para después.
- La presencia (quién está conectado) sigue siendo de toda la aplicación.

Validado:
- `just check` (760 live, 40 frontend). Las pruebas cubren, con un segundo lead global y su equipo personal: lo que no ve el otro lead (equipo, proyecto, tablero, hilos, documento, cambios, ideas, objetivos, por REST y por MCP); el proyecto compartido con una persona sin abrirle el equipo; objetivos, secuencia, cola y canal por equipo; crear un equipo compartido y un lead que no es global; mover proyectos con sus reglas; borrar sólo equipos vacíos.
- `/tmp/bw-pw/r23.mjs`: el recorrido en el navegador.
- La migración, probada sobre una copia de `pb_data` real.

## El agente ve y sube adjuntos *(built)*

Hasta aquí el agente no veía las imágenes ni los archivos de un hilo, ni podía
subir uno: los adjuntos se sirven por REST y su token sólo abre `/mcp`. Ahora, en
los cuatro lugares donde hay adjuntos —`assets/` de un proyecto (git), los
archivos de una idea del inbox, Mi espacio y las imágenes del inventario—, con
las reglas de cada uno (un proyecto que ves; una idea tuya o de un equipo que
lideras; Mi espacio sólo su dueño, ni el lead global; el inventario si te lo
asignaron):

- **`attachment_read`** manda una imagen como contenido `image` del MCP,
  **reducida ligeramente** (lado largo de 2000 px como mucho; una chica va tal
  cual; PNG o JPEG), un texto (txt, csv, json, svg) como texto, y **siempre un
  comando de descarga de un solo uso** (10 minutos). La descarga es el camino
  garantizado: Claude Code y Codex le pasan la imagen al modelo, pero Antigravity
  no lo documenta, y un PDF no viaja bien en ningún cliente (Codex descarta los
  bloques `resource`). Los tres tienen terminal y leen un archivo del disco.
- **`attachment_list`** da los adjuntos de un lugar; **`document_read`** dice en
  su cabecera qué adjuntos cita el documento.
- **`attachment_upload`** da un **enlace firmado** de un solo uso (10 minutos)
  para subir con `curl -F`, a `assets/` de un proyecto (mismo límite, tipos y
  commit firmado que desde la app; la respuesta trae el markdown para citarlo),
  a una idea o a Mi espacio. Subir no calienta nada: lo que lo cita, sí.

Los enlaces viven en memoria (un reinicio los invalida, y vencen solos) y se
re-comprueba a la persona al usarlos.

Rechazado: **mandar el archivo en base64 dentro de la llamada** (el modelo
escribiría el archivo token por token: una imagen de 1 MB son cientos de miles
de tokens); **abrirle al token del agente las rutas REST de adjuntos** (rompe que
el token sólo abra `/mcp`, y obliga a tener el token en la terminal); **la imagen
original sin reducir** (contexto pagado por píxeles que el modelo reduce igual).

## Roadmap: el calendario como un camino *(built)*

Una vista más de la columna («Roadmap», junto a Secuencia), detrás de una función que el departamento enciende en Ajustes → Funciones (`features.roadmap`, migración `1788533000_roadmap.go`, apagada al nacer). Apagada no aparece y `/roadmap` lleva a Todos. No cambia ningún dato.

- **Un camino que serpentea sin fin:** hacia arriba el pasado, hacia abajo el futuro. Cada carril va de un lado al otro (los pares de izquierda a derecha, los impares de vuelta) y se une al siguiente con una curva. Abre centrado en hoy, con la bandera «HOY» y el carril actual resaltado; el botón «Hoy» vuelve. Al acercarse a un borde carga más carriles, y al cargar el pasado compensa el desplazamiento para que la vista no salte.
- **Qué llena la pantalla se elige** (y se recuerda por persona); al cambiar, se recentra en hoy:
  - **Semana:** cada carril es un día, 7 en pantalla; las casillas del día van una tras otra a lo largo del carril.
  - **Mes** (el de entrada): cada carril es una semana, ~4,3 en pantalla; cada día tiene su lugar en el carril.
  - **Año:** cada carril es un trimestre, 4 en pantalla; cada semana tiene su lugar, con marcas de mes. Las juntas que se repiten cada día o cada semana no van: taparían el trabajo.
  Cambiar de escala es un zoom con origen en hoy: hacia «Semana» la vista se acerca, hacia «Año» se aleja (la vieja se va y la nueva llega del lado contrario; sin movimiento si la persona pidió reducirlo). El camino mide a lo más 1360px de ancho, centrado: en una pantalla ancha un carril de lado a lado ya no se lee.
  **Aire entre carriles:** cada carril mide 1,3 veces su parte de la pantalla, así que caben unos 5,4 días, 3,3 semanas o 3 trimestres; el grueso del camino sale de la parte justa, de modo que todo lo agregado es aire entre un tramo y el siguiente. Rechazado: adelgazar el camino para conservar 7 / 4,3 / 4 carriles (se pidió más grueso).
  La escala es **fija** dentro de cada vista: un carril vacío mide lo mismo que uno lleno, así que la distancia es tiempo. El alto de un carril es la pantalla entre los carriles que caben, así que siempre son entre 4 y 7, holgados. Rechazado: «Año» como 12 carriles de un mes (quedaría tan apretado como el primer corte).
- **Las vueltas y el grueso** salen del alto del carril: el camino mide 0,32 del carril (30 a 64px; en teléfono hasta 40), y cada vuelta sale 0,4 del carril hacia afuera (44 a 110px; en teléfono 0,28, de 30 a 52), casi media circunferencia. El margen a cada lado guarda la vuelta, medio camino y un aire (28px; 12 en teléfono), para que la curva, su sombra y la lupa no toquen el borde en pantallas angostas. Por ese margen el ancho máximo pasó de 1240 a 1360px. Con menos de 120px por lugar, las casillas son fichas redondas: el nombre cortado a pocas letras no decía nada.
- **Con relieve:** el camino es una cinta (sombra, costado oscuro, cara y un brillo en el borde de arriba), sin línea punteada; las casillas son botones en relieve del color de su banda, que se hunden al pulsarlos.
- **La lupa:** con ratón y **sólo sobre el camino** (a menos de 28px de su borde, o encima de una casilla, una señal o una pieza; en los márgenes, entre carriles o en el hueco de una curva no se enciende, y se apaga un poco más lejos de donde se encendió para no parpadear en el borde, siempre antes de la mitad entre dos carriles), donde está el cursor el camino se ensancha y lo que va encima crece, en degradado (al máximo en el centro, nada en el borde de la lupa, que abarca un carril y no el de al lado). Las casillas del carril se abren como el Dock: cada hueco crece lo mismo que las casillas, así no se enciman, y ceden antes del final del carril. Es sólo vista, ninguna fecha cambia. Sin librería: un `requestAnimationFrame` que suaviza la posición y la fuerza. El tramo ensanchado es una figura de ancho variable sobre el trazo de siempre, con las mismas medidas; de la sombra sólo dibuja lo que agrega, para no oscurecerla al doble. En las curvas: la normal sale siempre de la dirección del camino (si la de la recta apuntara al revés que la de la curva, la figura se torcería en la unión y dejaría un hueco), el costado ensanchado va debajo de la cara de siempre, y un tramo nunca termina dentro de una curva (la toma entera), porque ahí las capas corridas hacia arriba o abajo no empalman con el corte. La sombra del tramo lleva su propia caja de desenfoque, con margen en píxeles: el margen en porcentaje de una franja tan delgada la recortaba. Las marcas de mes también entran en la lupa: se corren como las casillas (marcan la misma fecha), bajan con el borde del camino ensanchado y crecen. Rechazado: deformar toda la vista (una lente sobre la página entera), porque movería también las etiquetas y las fechas que sirven de referencia.
- **Pilas:** donde caben varias cosas en un lugar (una pieza sobre su casilla, casillas apiladas, las escondidas detrás de «+N», varias piezas esperando en hoy), el hover —o el foco— abre una tarjeta con todo lo que hay ahí: las personas con su atraso y cada casilla con su enlace. Crece desde su lugar, arriba o abajo según haya espacio en la vista; el cursor puede pasar a ella sin que se cierre, y mientras está abierta la lupa se queda quieta. En esos lugares no va el título nativo encima. Donde las casillas son fichas redondas (Año, pantallas angostas) la tarjeta se abre también para una sola, porque sin ella no se lee nada. Sus filas son todas iguales —contorno, y al pasar, fondo—, sin la marca de color a la izquierda.
- **Casillas:** los módulos y los hilos con fecha, con el icono y el color de su banda (🔥 😴 🪦 🏆). Se puede ver todo, sólo módulos o sólo hilos. Las fechas del calendario del equipo van como señales 📅, sin pieza. Se apilan las que caben en el alto del carril; las demás se cuentan con «+N».
- **Piezas:** cada persona con casillas es una ficha con su avatar, parada en lo **primero que le toca**: su casilla abierta más antigua. Si esa fecha ya pasó, la ficha queda antes de hoy y lleva un aro rojo con los días de atraso. Sin nada abierto, espera en hoy. Una casilla es de una persona con la misma regla que «lo mío» en la secuencia: un hilo, de sus asignados o, si no tiene, de los responsables de su módulo; un módulo, de sus responsables.
- **Alcance:** lo que las reglas ya dejan ver (el tablero de Todos), del equipo elegido **en la columna**: ni el roadmap ni la secuencia tienen selector propio (dos selectores del mismo estado confundían). Cambiar de equipo en la columna deja al usuario donde está; sólo una pantalla de UN proyecto de otro equipo lo manda a Todos. Los proyectos de otro equipo a los que te invitaron entran siempre. Sin reglas nuevas.
- **El badge titular**, el mismo de Secuencia (flota arriba al centro): «🗺 Roadmap». **Los filtros y «Hoy», abajo al centro**, planos en el HUD: cada grupo con su propio borde, sin otra píldora alrededor (Todo/Módulos/Hilos, Semana/Mes/Año); en Secuencia de sólo lectura igual (Equipo/Lo mío, Tareas/Módulos), en vez de la esquina. Donde no hay lugar entre las esquinas de abajo (cerebro y pin a la izquierda, los botones redondos a la derecha; debajo de 1150px de ventana) la píldora sube sobre ellas. Antes iban dentro del badge; se bajaron a pedido, tras verlos ahí. **Los controles del lienzo en el borde izquierdo**, en la fila del badge, en Mi espacio («+ Nota» y el zoom; la fila de alinear, debajo de ellos) y en Secuencia (el zoom): una franja de tres columnas (controles | badge | vacía) deja el badge exactamente al centro sin medirlo. Antes iban pegados al badge; se llevaron al borde a pedido. En teléfono la franja se deshace y cada pieza vuelve a su esquina.
- **Los intervalos, por nivel:** el mes con su año es el título grande (en «Semana» y «Mes»; «Año» queda como estaba). En «Semana» (un día por carril) la semana es el título mediano, en su primer día —«Semana 43 · 19 – 25 oct»—, y cada carril lleva su día encima («mar 15 sept»); en «Mes» cada carril lleva su semana («Semana 43 · 19 – 25 oct»), en el mismo lugar. **En «Año» no hay indicador entre caminos** (el trimestre lo dicen las marcas de mes). Se probó quitar el indicador en «Semana» y «Mes» y se devolvió: ahí cada carril ES un día o una semana. **Metas:** una franja a cuadros de dos columnas cruza el camino donde un intervalo empieza o termina, con una bandera a cuadros en su asta (ondea hacia donde va el camino) y, bajo el camino, el día con su mes y año: en «Semana», UNA por semana, al terminar su último día, con la fecha de ese día (la bandera a cuadros es la llegada; el inicio lo dicen el día del carril siguiente y el título de la semana) —se probó una franja al empezar y otra al terminar, a los dos lados de una vuelta, y se leían como dos metas—; en «Mes», donde cambia el mes, «31 oct 2026 | 1 nov 2026», cada uno de su lado (el de antes, del lado de donde viene el camino; en los carriles de regreso, al revés). Antes decían «Fin de octubre | Inicio de noviembre»; se cambió por la fecha, que dice lo mismo y además cuándo. Las metas se corren con la lupa como las casillas. En pantallas angostas la fecha va sin el año (cerca del borde se saldría; el año está en el badge).
- **El día en que empieza la semana** lo elige cada quien (Ajustes → Mi usuario → Calendario; `prefs.week_start`, lunes si no): lo siguen el roadmap (los carriles de «Mes», las semanas y metas de «Semana», el número de semana), el calendario del planeador y los selectores de fecha. Con lunes la semana se numera como ISO 8601; con otro día, la semana 1 es la que contiene el 1 de enero. Rechazado: un ajuste por equipo o de la instancia (es cómo lee cada quien su semana, no una regla del trabajo).
- **El título se funde con el badge:** al subir bajo el badge, un título se encoge, sube y se desvanece en los últimos 80px, y el badge lo recibe: se ensancha (el ancho, animado) y muestra junto a «Roadmap» el mes con su año (el año, en «Año») y, en «Semana», la semana. El texto nuevo entra de donde viene el título (de abajo al bajar, de arriba al subir); al volver, el título sale de nuevo y el badge lo suelta. El ancho se anima sólo cuando cambia el texto (depender del objeto entero, nuevo en cada paso del scroll, lo hacía abrirse y cerrarse solo), y al crecer la ventana de carriles hacia el pasado la altura leída se corrige en el mismo cambio (por un cuadro leía la vieja sobre carriles nuevos y saltaba a un mes de antes). En «Año» se funde el año («2027»). En teléfono el badge deja sólo el icono y la semana va corta («S43»).
- **Minimapa del tiempo:** como el índice del Markdown, a la derecha, una columna de rayas: un mes (un trimestre en «Año») cada una, más larga cuanto más trabajo tiene, roja si algo ahí va atrasado, la de hoy en el color de acento y la del centro de la vista encendida. Al pasar se abre con los nombres y las cuentas; un clic lleva ahí. Abarca desde lo más viejo con fecha hasta lo más lejano (al menos medio año, o un año en trimestres, a cada lado de hoy; tope de 30). Un salto lejano arma la ventana alrededor del destino, sin dibujar todo lo de en medio, y «Hoy» la rearma si hoy quedó fuera. No se le reserva margen: achicaría los lugares y las casillas perderían el nombre. En teléfono no se muestra. Rechazado: un minimapa a escala de todo el camino (como el del tablero), porque el camino no tiene fin y a esa escala no se distingue nada.
- **Sólo lectura:** un clic abre el hilo o el módulo.
- En pantallas angostas las casillas son fichas redondas con su icono, y el nombre va en el título.
- **Sin `<select>` nativos** (el sistema dibuja su menú y no le llega el tema): el selector de equipo de las barras (planeador y canales; roadmap y secuencia ya no tienen) es `TeamPick`, el mismo menú del tema que el de la columna. Los papeles y las personas en Ajustes → Equipos, el rol en Ajustes → Usuarios, el rol de un miembro en el diálogo de personas del proyecto y la caducidad en Tokens usan `FilterPick`, el combobox del tema que ya usaban los filtros. Donde el rol no se puede cambiar (el tuyo, o el del último lead) se dice por qué en vez de ofrecer un menú inerte. Sólo la página de muestras del tema (`/theme`) conserva un `<select>`, porque enseña los controles base.
- Rechazado:
  - escala comprimida (sólo fechas con algo): la distancia dejaría de decir tiempo;
  - pieza en su última casilla terminada: no dice qué le toca ni si va atrasada;
  - editar arrastrando, por ahora.
- Pendiente: arrastrar una casilla para cambiar su fecha, animar el avance de la pieza, filtrar a mi pieza, y los hitos de los objetivos en el camino.

Validado:
- `just check` (763 live, 40 frontend): la función nace apagada, un miembro no la enciende y un lead global sí.
- `/tmp/bw-pw/r24.mjs` en el navegador: apagado no está en la columna y la ruta redirige; se enciende desde Ajustes; abre centrado en hoy; las casillas llevan su banda; la junta semanal aparece cada semana; una persona atrasada queda antes de hoy con aro y otra sobre su primera casilla abierta; los filtros funcionan; Mes/Semana/Año dan su carril con aire (1,3 de ~4,3/7/4 por pantalla), se animan, recentran en hoy y se recuerdan; el camino no pasa de 1360px; Año muestra trimestres con marcas de mes y sin la junta semanal; el camino tiene costado y sombra y ninguna línea punteada; la pila de una pieza sobre su casilla muestra a ambos y se cierra al salir; en Año una ficha sola también muestra su detalle; la lupa agranda la casilla bajo el cursor, ensancha el camino, no se enciende entre carriles y al salir todo vuelve; pasado y futuro cargan sin saltar; «Hoy» vuelve; un clic abre hilo y módulo; un miembro ve su proyecto; en oscuro y en teléfono, sin scroll horizontal.

## Papelera en el menú del cerebro *(built)*

Eliminar está también a la derecha de cada una de las ocho filas visibles del
menú del cerebro, no sólo en el detalle. Nombre/progreso y papelera son botones
hermanos: abrir conserva el retorno al origen; eliminar no abre ni cambia de
pantalla por accidente. El icono mantiene el danger del tema, incluso en filas
traspasadas. Rechazado: anidar otro botón dentro del botón que abría toda la fila.

Usa la misma confirmación y `DELETE /api/artifacts/{id}`, con borradores/entradas
perdidos y nota/trabajo duradero conservados. Durante la petición bloquea las
papeleras y abrir la fila afectada, con logo compacto; un fallo deja la fila y
muestra error para reintentar. Éxito quita sólo ese ID y actualiza lista, badge y
`+N`, invalidando lecturas antiguas para que no lo repongan. Si se estaba leyendo
ese mismo artefacto, vuelve al origen; si se borró otro, permanece donde estaba.
La fila adapta el nombre/filename dejando espacio a sus cifras, caducidad y
papelera; los nombres completos siguen accesibles y en tooltip.

Chromium: posición derecha, objetivos independientes, ningún botón anidado,
confirmar/cancelar, fallo/reintento, bloqueo/loading, borrado desde detalle y
desde board, regreso correcto, documentos/notas intactos, badge/overflow,
teclado, claro/oscuro y viewport de 400px sin papelera fuera de pantalla. Bundle
reconstruido; `just check` pasa (592 live + 36 frontend; Svelte sin errores ni
warnings). Sin commit ni despliegue.

## Eliminar artefactos efímeros *(built)*

La cabecera de `/artefacto/<id>` termina con **Eliminar artefacto**, con asset y
color danger del tema y confirmación compartida. Sirve para Markdown y grills,
abiertos o traspasados, incluso fijados. Advierte que se pierden publicaciones,
preguntas/respuestas y borradores, sin recuperación, mientras la nota del Inbox
y los hilos/documentos duraderos se conservan. Tras éxito vuelve al origen real
(board o Inbox); si falla muestra el error y permite reintentar, sin navegar.

`DELETE /api/artifacts/{id}` llama `ArtifactDelete`: creador o lead global,
comprobado dentro de la transacción del borrado. No concede derechos al lead de
un workspace ni abre el DeleteRule crudo. Entradas en cascada, nota y destinos
intactos; no eventos, calor ni git. Rechazado: borrar la captura o el trabajo
traspasado para quitar sólo su adjunto efímero; rechazado también ocultar el botón
sin proteger la operación en servidor.

La espera activa termina con `fin` al desaparecer el artefacto. El comando de
watch también termina en 404 entre polls, sin repetir por siempre; errores de
red/5xx siguen con backoff. Conserva una línea por respuesta y el siguiente seq
sin el salto de línea del body. Lecturas/respuestas tardías en la vista se
invalidan al borrar, cambiar de artefacto/usuario o desmontar; un 404 ya no deja
mostrándose contenido viejo como si siguiera disponible.

Validado: 17 checks live nuevos de sesión/propiedad/lead, cascade, notas y hash
intactos, Markdown fijado/traspasado, grill abierto/traspasado, watcher y no-heat;
tres casos de shell watch (desaparece antes/durante la espera y después de una
respuesta). Chromium: confirmar/cancelar/Escape, fallo/reintento, loading compacto,
retorno a board/Inbox, refresh del dock, permisos, ambos temas y ancho estrecho.
`just check`: 592 live + 36 frontend, Svelte sin errores ni warnings; bundle
reconstruido. Sin commit ni despliegue.

## Loading, display y Secuencia a todo el contenedor *(built)*

`Loading.svelte` reutiliza el logo flotante (`BrandOrb`) centrado en su región,
con `role=status`, etiqueta accesible y reduced motion. Sustituye los puntos y
«leyendo/cargando» en board, documentos, cambios, calendario, artefactos,
ajustes, usuarios y fijados; tiene tamaño compacto para acciones pendientes y
búsqueda. Los loaders de documento de wiki/prompts no heredan el padding inferior
de lectura que los desplazaba del centro. Para una pantalla completa se usa
`viewport`, sin fijar la altura de toda la aplicación y cortar documentos largos.
No se reemplazan los puntos que son contenido, truncamiento o placeholders.

La escala de `.display` crece manteniendo la jerarquía: body y nombres de trabajo
no cambian de font. El wordmark pasa a 20px, títulos de board a 30px, calendario
a 32px (28 en estrecho), y título flotante del canvas a 24px.

`/secuencia` elimina la barra exterior: canvas y contenedor tienen el mismo
rectángulo. Título, estado sólo lectura, alcance Organización/Lo mío y selector
Tareas/Módulos quedan en HUD fijo, con errores/reintentar ahí también. El
planeador conserva su cabecera. En el canvas edge-to-edge el zoom ocupa una
segunda fila de HUD bajo la presencia, para que varios avatares no lo tapen; en
móvil queda separado del dock inferior. No cambia el alcance de servidor ni
aparece ninguna escritura de secuencia en la vista de lectura.

El fondo fijo se pinta en `html`, sobre todo el canvas del documento, con body
transparente: no termina al acabar su primera pantalla ni revela otro fondo al
seguir scrolleando. Se conserva el scroll del documento (sin uno interno nuevo),
el grano, y el apilamiento de portales. Rechazado: forzar `#app` a 100dvh o
agrandar el body por un número arbitrario para esconder el corte.

Chromium validado: centrado de loaders de secuencia/hilo/wiki/artefacto, loading
compacto en login, reduced motion, tamaños de display, HUD y canvas sin barra,
zoom sin intersección con presencia (también fila de varios avatares), ambos
temas y viewport estrecho. Artefacto de 65 secciones: fondo al inicio/medio/final
sin corte, con comparación de píxeles de fondo en claro/oscuro. Bundle
reconstruido; `just check` pasa (575 live, 36 frontend, Svelte sin errores ni
warnings). Sin commit ni despliegue.

## Asset de eliminar en danger *(built)*

El ícono de «Eliminar burbuja» usa `Icon danger`: parte del mismo render clay y
lo colorea con el token `--color-error-500`, conservando el sombreado 3D y alpha
mediante filtro SVG por instancia. El color no queda horneado en un PNG ni
cambia al degradado de marca en hover. La confirmación y permisos no cambian.
Chromium validado en claro/oscuro, hover y actualización del token de tema;
Svelte sin errores ni warnings y 36 pruebas de frontend pasan. Bundle
reconstruido; sin commit ni despliegue.

## Cards del canvas: hover y apertura en toda la superficie *(built)*

La card completa ofrece borde/sombra/fondo de hover y nombre en accent, también
al recibir foco de teclado. No se mueve ni cambia de tamaño para dar ese feedback;
respeta reduced motion. El título sigue siendo un botón accesible.

Clic en el paso, badge de proyecto, personas, padding o cuerpo abre el mismo
elemento que el título, tanto editable como read-only. Se conserva el ID del
nodo al capturar el pointer, porque `pointerup` acaba dirigido al canvas. El
umbral de arrastre son 4 píxeles de pantalla (independiente del zoom); si hubo
arrastre no abre, aunque se vuelva al punto inicial. Editable mueve la card;
read-only sólo panea. `×`, completar y `+` mantienen sus gestos propios.
El cursor de mano cerrada durante el gesto gana al cursor de clic de la card.
Rechazado: abrir en pointerdown, envolver botones en otro botón, o mover la
card en hover y desalinear sus flechas.

Chromium verificado: hover claro/oscuro, clic en footer y paso, teclado, arrastre
editable sin apertura, pan read-only sin cambiar posiciones, apertura de hilo y
módulo, y quitar/completar sin abrir. Bundle reconstruido; `just check` pasa
(575 live, 36 frontend, Svelte sin errores ni warnings). Sin commit ni despliegue.

## Más aire en los modales y ancho ajustable de los árboles *(built)*

Los diálogos de formulario usan un gap vertical común, independiente del
margin de párrafos y listas. Objetivos y Prioridades separan también sus filas.
Fechas usa `DueDate` y `FilterPick`, sin controles nativos, en un diálogo más
ancho; sus popups permanecen dentro de la capa modal para recibir clics.

Prompts, Wiki y Hilos comparten `TreePanel`: el borde derecho se arrastra, el
ancho se recuerda por persona y pantalla, las flechas de teclado lo ajustan y
un doble clic lo restaura. La navegación colapsada conserva su rail. El tema
no se dibuja globalmente encima del botón de una pantalla que ya tiene el suyo.

Verificados los controles dentro del modal, el arrastre y la persistencia del
árbol, y los temas claro y oscuro con capturas en Chromium.

## Todo lo que se le dice a la IA se edita en «Prompts» *(built)*

Una sola pantalla, con la experiencia de la wiki (DocEditor), para los dos
AGENTS.md, los documentos del binario (`guide`, `house_rules`, `connect`,
`grill`) y la descripción de cada tool MCP. El lead edita; los demás leen lo
que su agente recibe (el AGENTS.md del planner sigue siendo sólo del lead).
Cada guardado es una versión (`prompt_versions`, como `agents_md`); `reset`
vuelve al texto del binario, que queda como el ORIGINAL. El servidor MCP se
reconstruye cuando cambia un prompt, así que la siguiente llamada de un agente
ya lee la descripción nueva.

**Esto revierte una decisión de `prompts/embed.go`**: «una guía desplegada aparte
del binario deriva de él, y entonces miente». El riesgo sigue: la guía promete
cosas sobre la API. Se acota con la huella del original (`base`): si el binario
cambia ese texto después de una edición, la pantalla avisa de que la versión
editada se quedó atrás y ofrece restaurar. Rechazado: dejar fijos los prompts
del sistema y editar sólo los AGENTS.md — se pidió editar todo lo que instruye
a la IA, incluidas las tools.

## Releases del AGENTS.md, y qué tiene cada agente *(built, sin revisión visual)*

Guardar en Prompts ya publicaba en caliente, pero que el agente lo leyera dependía de una línea en su `CLAUDE.md` («llama a `agents_md`») que el modelo puede ignorar, y nadie veía quién la leía. Hilo: [#4 en Burbujas](https://bubble.ayetec.space/w/ws-nbu6tp/t/4).

- **`instructions` en el `initialize` (y en `discover`, el protocolo nuevo).** Es lo que el cliente pone en el contexto del agente en cada sesión, sin que el modelo se acuerde de nada. Claude Code lo corta a 2 048 caracteres (UTF-16) sin avisar y Codex lo usa como guía del servidor, así que es un **cargador**: dice la release vigente y manda a `agents_md`. Es un prompt más (`instructions`, `prompts/instructions.md`, con `{{release}}`, `{{role}}` y `{{connect}}`). Guardar una edición que, rellenada, pase de 2 048 se rechaza. Se pone por persona en un middleware de recepción, porque la release depende del rol y el servidor es uno. Rechazado: un servidor por rol (dos reconstrucciones por guardado para cambiar una línea) y meter `agents_md` entero en `instructions` (unos 30 000 caracteres: se perdería todo menos el principio).
- **La release es una huella, no un número.** Es la huella de lo que devuelve `agents_md` para un rol, sin su cabecera: guía, tabla de tools, markdown, sección de conexión y departamento. Va en la cabecera de `agents_md` y en `instructions`. No cambia al reiniciar y cambia con cualquier edición, también la de la descripción de una tool. Rechazado: `promptGen` (en memoria, vuelve a cero al arrancar), un correlativo guardado aparte, y el id de la fila de `agents_md` (no cambia al editar la guía ni una tool). Rechazado también, por ahora: borrador y «publicar»; guardar sigue siendo publicar.
- **La sección instalada lleva versión.** `ConnectVersion` es la huella del bloque ```` ```markdown ```` del prompt de conexión SIN rellenar: editar la prosa de alrededor no envejece ninguna instalación, y editar la sección sí. El servidor rellena `{{version}}`, que aparece en una línea visible de la sección («Versión de esta sección: `bubble-work connect …`») y en la cabecera `X-Bubble-Connect` que el prompt pide configurar. También la manda en `X-Bubble-Connect-Version` de `/api/connect`, para los comandos por harness. `agents_md` trae la sección vigente y pide **reemplazar** una vieja, nunca agregarla donde no la hay: pi la inyecta de otra forma y no debe escribir `CLAUDE.md` ajenos. Codex no acepta cabeceras extra en `codex mcp add`, solo en `config.toml` (`http_headers`), así que en su caso se explica. Lo revisé en el código de Codex, no en un Codex corriendo. Rechazado: la marca como comentario HTML (`<!-- … -->`), que fue la primera versión. Claude Code quita los comentarios del `CLAUDE.md` antes de dárselo al modelo, y en la prueba con un Claude Code real el agente creyó que su sección estaba vieja e intentó reescribirla.
- **Qué tiene cada agente** (`agent_tokens`, migración `1788542000`): `client` (el `clientInfo` de su `initialize`), `connect_version` (la cabecera, saneada: hasta 32 caracteres de `[A-Za-z0-9._-]`), y `release_read` / `release_role` / `release_read_at` (lo que se le entregó al llamar a `agents_md` con su propio rol). Lo escribe sólo el servidor, de forma directa como `last_used_at` y sólo cuando cambia. Rechazado: ponerlo en `events`, que es evidencia de trabajo y no telemetría de agentes. Prueba que el texto **llegó**, no que el modelo lo siguiera. Un agente que entra con la sesión del navegador no anota nada.
- **Ajustes → Tokens** compara con `GET /api/agents/release` (huellas por rol y de la sección) y enseña por token: *al día*, *release anterior* (se arregla sola en su siguiente sesión), *nunca leyó agents_md* (su cliente ignora `instructions` y su sección no funciona: hay que volver a pegar el prompt) e *instalación vieja* (aparte). La lógica está en `lib/agentHealth.ts`.
- **Ajustes → Usuarios** (lead global) enseña por persona el logo de cada cliente con que trabajan sus agentes y la última release de AGENTS.md que les llegó: *al día*, *anterior* o *sin leer* (`lib/agentClients.ts`). Sale de los tokens, que el lead global ya ve; no hay nada nuevo en el servidor. Es el CLIENTE, no el modelo: MCP no dice qué modelo hay detrás. Los logos son de simple-icons (`/logos/`); Codex (OpenAI) y Antigravity no tienen y van con inicial (`Cx`, `Ag`). Para Claude Code se usa el logo de Claude y no `claudecode` (la mascota pixelada), que no se reconoce de un vistazo.
- **Personas simuladas en el dev** (`just seed agents`, `scripts/seed_agents.py`): 13 personas `@simulado.test` con agentes que hacen los handshakes MCP reales, una por estado (al día, anterior, nunca leyó, instalación vieja, sin usar, revocado, borrada) y con 9 clientes distintos. Para que haya una release anterior, «Lead simulado» vuelve a publicar el AGENTS.md de operadores con el mismo texto y un salto de línea. Viven lo que vive el servidor de dev: `bubble simulated purge` (`cmd_simulated.go`) las borra del todo, junto con toda fila que apunte a ellas, recorriendo las relaciones hacia `users` en vez de enumerarlas. Corre sin servidor, en `just stop`, en `just dev` (antes de reemplazar la sesión) y en la ventana del servidor cuando `serve` termina solo (Ctrl-C o caída). Rechazado: limpiar por la API antes de bajar el servidor, porque no cubre un Ctrl-C ni una caída. Probado con los tres caminos en un dev desechable y con `TestPurgeSimulated`.
- Validado localmente con `just check` (856 pruebas live, 16 de ellas nuevas; 109 de frontend; Go `release_test.go`). **Sin validar:** la revisión visual de Ajustes → Tokens, que Claude Code, Codex y Antigravity reales manden `clientInfo` y la cabecera como se espera, y si Codex usa `instructions` igual que Claude Code. Sin commit ni despliegue.

## Funciones por equipo *(built, sin revisión visual)*

`features` era una sola fila global, pero la secuencia y el roadmap ya son de cada equipo: un equipo que no los usaba los veía porque otro sí. Hilo: [#5 en Burbujas](https://bubble.ayetec.space/w/ws-nbu6tp/t/5).

- **El valor del equipo, si lo dice; si no, el del departamento.** `team_features` (migración `1788543000`) guarda una fila por equipo con `sequence` y `roadmap` como select `on`/`off`; vacío es heredar. Ajustes → Funciones queda como el valor por defecto. Rechazado: columnas en `teams`, porque un booleano no sabe decir «hereda».
- **Mi espacio se queda global:** es de cada persona, no de un equipo.
- **Quién decide:** quien lidera el equipo (`LeadsTeam`), es decir el lead global en uno compartido o una fila `lead`; en uno personal, su dueño. Ni lectura ni escritura por la API de registros: viaja en `GET /api/teams` (también en `team_list` por MCP), que ya decide quién ve qué equipo, y se escribe por `POST /api/teams/{id}/features`. Decidido sin respuesta de Angel, con lo que se propuso; cambiar quién decide es una línea.
- **Servidor:** `FeatureOn(app, team, key)`; `SequenceOn` pasa a ser por equipo en `sequence_next` y en los pasos vivos del tablero. `sequence_set` no lo mira: apagar no borra la línea.
- **Navegador:** `lib/features.ts` resuelve las funciones del equipo actual y, en «Todos los equipos», las cuenta encendidas si algún equipo compartido las tiene. `/secuencia` y el roadmap sólo ofrecen esos equipos. Las redirecciones esperan a que carguen los equipos, para no echar a nadie de una ruta que su equipo sí tiene. Ajustes → Equipos lleva un selector Hereda (sí/no) / Encendida / Apagada por función; quien no lidera el equipo ve el estado.
- Validado: 870 pruebas live (14 nuevas: quién decide, herencia en los dos sentidos, `sequence_next`, valores inválidos, Mi espacio fuera), 119 de frontend (5 nuevas de resolución) y una prueba de punta a punta en el dev (apagarla en TI la quita del menú; heredar la devuelve). **Sin validar:** la revisión visual de Angel.

## Furia de IA — historia 1: motor y reglas *(built, sin revisión de Angel)*

Una tercera señal, derivada, de que algo importante se está rompiendo. Acordada en un interrogatorio de 18 preguntas el 22 oct; [hilo #6 de Burbujas](https://bubble.ayetec.space/w/ws-nbu6tp/t/6). Es el primer incremento del diseño largo del [hilo #3](https://bubble.ayetec.space/w/ws-nbu6tp/t/3) (E6), sin compromisos ni zonas horarias.

- **`internal/fury`**, puro, con `now` como parámetro (como `internal/heat`). Cuatro niveles: 0 😌 Zen, 1 😤 Irritada, 2 😡 Furiosa, 3 🔥 Desatada (verde, amarillo, rojo, rojo parpadeando). Un sujeto toma el nivel de su regla más grave y lleva todas las razones; una burbuja abierta suma las de sus hilos abiertos, diciendo de cuál vienen. Hecho, completado o cerrado: sin furia.
- **Reglas, un catálogo con perillas** que edita sólo el lead global, iguales para todos los equipos: R1 burbuja inflada (≥ 8 hilos, ≥ 6 000 palabras, ≥ 10 días activos y ≥ 21 días abierta → 1; el doble de todo → 2; hilos y palabras cuentan los cerrados), R2 vencida (P1/P2 → 2; más de 7 días → 3), R4 olvidada (P1/P2 dormida → 1, tumba → 2), R5 sin dueño (P1/P2 → 1). «Prioridad alta» es P1/P2, en hilos la propia y en burbujas la de impacto × urgencia; un hilo no hereda la de su burbuja. Días activos = días distintos con eventos que calientan en sus hilos. Rechazados: fórmulas libres, reglas evaluadas por un LLM, marcarla a mano y reglas por equipo.
- **Nunca se guarda.** `fury_rules` (migración `1788544000`) guarda sólo el catálogo, sin reglas de API: se lee y escribe por `/api/fury/rules`, que valida (niveles 0–3, umbrales ≥ 1). `features.fury` nace apagado; apagado no se calcula nada.
- **Un solo cálculo:** dentro de `boardFor`, así que el tablero, `project_board`, «Todos los equipos», `thread_get` y `sequence_next` dicen lo mismo. Sólo viaja `fury` cuando alguna regla disparó. No cambia el calor ni el orden. La guía de agentes dice que lo furioso de su persona va primero y que se apaga arreglando la causa.
- **Calibrar antes de encender:** `POST /api/fury/preview` (lead global) calcula con las reglas que se están editando aunque la función esté apagada. Ajustes → Furia lo enseña en vivo; los demás leen las reglas.
- **Límite conocido:** un plazo vence al terminar su día en UTC; las zonas por equipo son del diseño largo.
- **Iconos de la furia:** la cabeza del robot (`robot`) con cuatro ánimos, `zen`, `irritated`, `furious` y `unleashed`, en barro, degradado y color natural (menta, ámbar, rojo, rojo con llamas), en `public/i3d/`. Se generaron como los demás: Codex CLI con su herramienta de imágenes, el robot como referencia, fondo magenta y el mismo recorte a 128 px; al halo de `zen` se le quitó a mano el rosa del magenta. `furyIcon(level)` los pide por nombre, porque el 🔥 de «Desatada» es el mismo emoji que la banda caliente del calor. Las variantes a color llevan la bola de la antena encendida en el color de su ánimo, como la cian del robot original (edición con Codex sobre las imágenes generadas, recortadas con la misma caja que el barro y el degradado). Ajustes → Furia los enseña en grande arriba (cabeza, nombre en la fuente display y el LED del semáforo), y el nivel de cada regla se elige con un desplegable de cabezas (`FuryLevelPick`); la vista previa enseña sólo cabezas. El desplegable es blanco y sin número; sólo «Desatada» lleva tema, un fuego que sube y baja de ida y vuelta, sin salto entre ciclos ni brillo que late (quieto con «reducir movimiento»), con 3 o 4 llamas animadas en sitios al azar (fuera de la cabeza) que asoman por encima de la pastilla y suben dentro de su opción del menú. Las llamas son el Lottie «Fire» de LottieFiles Mobile (Lottie Simple License), dibujado una vez con lottie-web en un sprite WebP de 26 fotogramas (`public/fx/fire-sprite.webp`, 54 KB, con su `NOTICE.md`) y animado con CSS `steps(26)`. Rechazado: meter un reproductor Lottie en el bundle (~250 KB por una llama). La cabeza de Desatada se dibuja un 18 % más grande y subida (`.icon[data-icon='unleashed']`), porque las llamas la dejaban más chica que las otras; las llamas salen del recuadro.
- Validado: `internal/fury` con pruebas por regla y bordes (mismo día, 7 vs 8 días, doble umbral, máximo de causas, validación); 889 pruebas live (19 nuevas: apagada no calcula, lectura y escritura de reglas, vista previa, niveles, color y parpadeo, burbuja con su hilo, `sequence_next`, asignar y completar la calman); la página revisada en el dev contra datos reales. **Falta:** la revisión de Angel, y las historias 2 (UI: aro, banner, contador) y 3 (semáforos por webhook).

## Furia de IA — historia 2: la cabeza en el dock *(built, sin revisión de Angel)*

Angel cambió el plan de la historia 2 (23 oct): en vez de aro, banner y contador en la barra, el ánimo de la IA es **una cabeza más en el dock**, junto al cerebro y el pin (`FuryOrb.svelte`).

- **Qué dice:** la furia de TU trabajo, como la luz del semáforo acordada para la historia 3: el nivel más grave entre los hilos asignados a ti y las burbujas de las que respondes (`GET /api/fury/me`, del mismo cálculo que el tablero). Zen si nada tuyo está en problemas; no se dibuja con la furia apagada.
- **Cómo:** la cabeza del nivel, con un contador de cuántas cosas tuyas tienen furia (ámbar o rojo). Desatada no se mueve: le brotan llamas Lottie a los lados, a alturas al azar, y sus filas (y la del nivel) llevan el fondo de fuego de los desplegables. El fuego es compartido: `FuryFlames.svelte` (llamas al azar, `top` / `inside` / `around`) y la clase `.fury-fire` de `app.css` (fondo). Al pasar por encima se abre hacia arriba, como el cerebro: el nivel, su luz, y cada cosa con su cabeza y la primera razón; clic abre el hilo o el tablero con la burbuja abierta. Se recalcula con el stream (hilos, burbujas, funciones, reglas) y cada minuto, porque vencer no escribe nada.
- **Las esquinas crecieron:** con tres objetos, la barra central del tablero, Blobs, 3D, secuencia y roadmap ya no cabía a 1 280 px; su corte para subir sobre las esquinas pasó de 1 150 a 1 320 px.
- **Alcance confirmado (23 oct):** Angel descartó el aro de color en las burbujas. Se queda la cabeza del dock; no se añadió un banner.
- Validado: 893 pruebas live (nuevas: lo asignado aparece, lo ajeno no, apagada no se dibuja), 120 de frontend, capturas en el dev a 1 280 y 1 440 px en ambos temas con `just seed fury`.

## Furia de IA — historia 3: el webhook de los semáforos *(built, sin hardware real)*

Los semáforos de los escritorios de la oficina (hilo #6). Bubble **avisa** a una sola URL; el receptor del hardware reparte a cada escritorio.

- **La luz de cada persona** es la furia de lo suyo (hilos asignados y burbujas de las que responde), como su cabeza en el dock: `furyLights` calcula todos los tableros una vez. Con la furia apagada, todas verdes. Fuera de horario sigue diciendo el nivel: el horario es del firmware.
- **Cuándo:** al cambiar un hilo, una burbuja, un evento, una persona, las funciones o las reglas (agrupado 2 s), un aviso `fury.changed` por persona cuyo nivel cambió; y un latido cada 5 minutos (`bubbleFuryHeartbeat`) a **todas** las personas activas (`fury.heartbeat` si no cambió), que es también lo que corrige lo que vence solo. El último nivel avisado vive en memoria: al arrancar, el primer cálculo avisa a todos. Sin reintentos: el latido corrige. El firmware apaga la luz si pasan 15 minutos sin avisos.
- **Formato:** `{"type","person","email","name","level","mood","color","blink","reasons","at"}`, con las razones de la más grave a la menos (máximo 5). Firmado: `X-Bubble-Signature: sha256=<hex>`, HMAC-SHA256 del cuerpo con un secreto `bfs_…` que se enseña una sola vez (al configurar o al rotar).
- **Dónde:** `fury_hook` (migración `1788545000`, sin reglas, secreto oculto) y las rutas `/api/fury/hook`, `/api/fury/hook/test` del lead global; Ajustes → Furia → Semáforos (URL, secreto una vez, enviar latido, generar otro secreto, último envío).
- **Sin hardware:** `BUBBLE_FURY_SECRET=… just fury listen [puerto]` (`scripts/fury_receiver.py`) verifica la firma, rechaza las malas (401) y pinta una luz por persona; es también el ejemplo para quien construye el receptor de la oficina.
- **Rechazados:** pull desde el semáforo y MQTT (decisión de Angel); una URL por semáforo (Bubble tendría que alcanzar IPs de la oficina); registrar dispositivos (un destino por persona basta: la persona va en el aviso).
- Validado: 904 pruebas live (11 nuevas: quién configura, secreto una vez, latido firmado a todos, rojo parpadeando para quien tiene algo desatado, verde para quien no, `fury.heartbeat` sin cambios, terminar un hilo baja la luz al momento, secreto rotado rechazado y contado como fallo); de punta a punta en el dev con la interfaz y el receptor en tmux (15 personas). **Falta:** el receptor y los semáforos reales, y la revisión de Angel.

### Prueba del gateway en reko-3 *(built, simulado, no hardware)*

`just fury lab up|relay|test|state|logs|status|stop` monta sólo `bubble-fury-lab` en Docker **contexto reko-3** y su router propio en la red `proxy` de Traefik, sin reiniciarlo. `deploy/fury-lab/compose.yaml` y su Dockerfile empaquetan `scripts/fury_receiver.py`; el contexto de build excluye el resto del repositorio y los secretos. No hay puertos publicados en el host: Traefik termina HTTPS y manda al puerto 8787 del contenedor no-root.

- URL: **https://fury-gateway.cubytest.space/fury**. El DNS existente de `*.cubytest.space` apunta a reko-3 (`192.168.20.250`), y el certificado wildcard existente es válido. `*.cuby.work` apunta a **otro equipo**, `192.168.20.200`: no se modificó ese DNS ni `bubble.cuby.work`.
- `GET /health` informa sólo que es un gateway simulado. `GET /state` requiere un Bearer token distinto del secreto HMAC (nombres, correos y razones nunca quedan públicos). `just fury lab state` lo consulta sin imprimir el token. Las claves del lab se guardan en `deploy/fury-lab/.env` (gitignored, chmod 600). `POST /fury` verifica HMAC, limita el cuerpo y valida el JSON. El receptor sólo recuerda el último aviso de cada persona **en memoria**: no es firmware, no aplica ACK de nodos, orden de mensajes ni apagado por caducidad.
- **Excepción de este Mac:** Python y el emisor Go dev devolvieron `errno 65 / no route to host` a la IP LAN, mientras el curl del sistema sí llegó con TLS válido. Para no cambiar permisos ni reiniciar el dev, `just fury lab relay` deja un puente HTTP **sólo en loopback** (`127.0.0.1:18787`, tmux `bubble-work-fury-lab`): Bubble firma, el puente conserva cuerpo/cabeceras y curl los entrega al gateway HTTPS. La causa exacta del bloqueo no se diagnosticó; no se desactivó TLS. El gateway real no requiere este puente si Bubble puede alcanzarlo en LAN.
- Validado: latido de 15 personas; hilo temporal de una persona simulada que produce `fury.changed` nivel 3 y vuelve a 0 al completarse (hilo eliminado después); firma mala → 401; `/state` sin token → 401. Go y 904 pruebas live pasan, más 9 pruebas nuevas del receptor integradas en `just test` (sin Docker ni red remota).
- Para revisar: `just fury lab state`, `just fury lab test`, `just fury lab logs`. **Desmontar:** `just fury lab stop`, que desconecta sólo el webhook del dev que apunte al lab, elimina el contenedor/router y cierra el puente si no está adjunto. El Bubble de producción no se configuró ni desplegó. Falta el gateway y los semáforos físicos.

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

**Una instancia se actualiza SOLA, y sabe deshacerlo.** *(built)* El binario
instalado dejó de ser el servidor y pasó a ser quien lo arranca: `bubble boot`
resuelve la versión vigente, la lanza como hijo y la vigila. Si esa versión no
llega a contestar —no arranca, una migración explota, el puerto no abre— vuelve
a la última que sí lo hizo y la relanza. **El que deshace nunca puede ser el que
se actualizó**, y por eso son dos procesos.

Los binarios viven en el VOLUMEN (`/data/bin/versions/<tag>`), no en la imagen.
En la imagen, un `docker compose up` los borraría y la instancia «volvería» a la
versión de la imagen sin que nadie entienda por qué. El volumen es el estado; la
imagen es sólo el arranque — y eso hace que el bootloader, la pieza que nada
puede revertir, sólo cambie cuando una persona cambia la imagen.

`POST /api/update` (lead global) descarga el binario de su plataforma, comprueba
su SHA256 contra el `SHA256SUMS` del release, **copia la base** y se va con
`EX_TEMPFAIL`; el arranque lee `next` y hace el cambio. La copia no es opcional
aquí, a diferencia de la de `update.sh`: volver atrás devuelve el binario y no el
esquema, así que si la versión nueva migra y luego falla, esa copia es lo único
que queda.

Rechazado, y por qué: montar el socket de docker en el contenedor para que se
recreara a sí mismo. Funciona, y convierte cualquier fallo de seguridad de la
aplicación en root del host — un precio enorme por ahorrar el minuto que tardaba
la alternativa. Rechazado también un actualizador externo con ese socket: menos
superficie, pero una pieza más que mantener y el socket seguía ahí.

**Lo que la comprobación NO hace, dicho en su sitio:** `SHA256SUMS` viaja en el
mismo release que el binario, así que comprueba integridad —una descarga a
medias, un proxy que mete basura— y no procedencia: quien pudiera publicar un
release falso publicaría también sus sumas. Firmarlo (cosign o minisign, con la
clave en el CI) es lo que lo convertiría en confianza, y está pendiente.

**Actualizar una instancia: ella TIRA, el CI no empuja.** *(built)* Un despliegue
por SSH desde Actions necesita una llave con acceso al servidor guardada en el
repositorio, no alcanza una instancia detrás de NAT, y si la máquina estaba
apagada ese día el evento se pierde. Tirando, añadir una instancia cuesta cero y
ese secreto no existe.

Las piezas: el CI publica en cada tag los binarios y una imagen multiarco en
GHCR (`:stable` es el canal, el tag con número congela); `compose.yml` describe
una instancia con UN volumen —la base y los repositorios de markdown son un solo
estado, y una copia con la mitad restaura algo que dice cosas que sus archivos no
dicen—; y `scripts/update.sh`, disparado por un timer diario de systemd, tira,
arranca, espera a `/api/version` y **vuelve a la imagen anterior** si no
contesta. Sin docker sale gratis igual: los binarios se publican con sus sumas.

`/api/version` contesta sin sesión y sin tocar la base a propósito. Es la
comprobación de salud, y una que necesita la base no distingue «arrancando» de
«roto».

**La copia previa es opt-in, y por defecto está apagada.** La migración corre
sola al arrancar y es la puerta de un solo sentido de toda actualización, así que
esa copia es lo único que deja volver — y aun así el valor por defecto es 0,
porque el estado es de quien aloja esto: hay quien ya respalda el volumen entero
desde fuera, y un actualizador que empieza a llenarle el disco de tarballs sin
que lo pidiera es uno que acaba desactivado del todo. Lo que sí hace el script es
pararlo antes de copiar: SQLite en WAL y git escribiendo archivos, copiados en
caliente, dan la peor clase de respaldo — el que parece bueno hasta el día que
hace falta.

Rechazado: Watchtower. Reinicia con cualquier digest nuevo, sin sitio donde
meter la copia ni la vuelta atrás, y «se actualizó solo y ahora no arranca» es
exactamente el caso que hay que cubrir.

**Una imagen más nueva que la vigente es una actualización.** *(built)* En
Coolify quien actualiza es el orquestador: tira de `:stable` y recrea el
contenedor. Con `boot` tal cual, eso no hacía nada —la imagen sólo sembraba el
volumen la primera vez, y la versión vigente seguía siendo la del volumen—. Ahora
`boot` compara al arrancar: si la imagen trae una versión MÁS NUEVA, copia la
base en frío (`pb_data/backups/antes-de-<tag>.zip`, con la forma de un respaldo
de PocketBase), la instala y la lanza con la vigente como red. Una más vieja o
igual no toca nada, y una que no llegó a contestar queda en `bin/failed` para
que reiniciar el contenedor no la reintente cada vez — un minuto sin servicio
por reinicio. Así valen los dos caminos, el botón y el orquestador, y el más
nuevo gana. Montarlo está en `deploy/coolify.md`.

Rechazado: `serve` en vez de `boot` bajo Coolify. Deja que mande la imagen, que
es lo más simple, y mata el botón de actualizar y la vuelta atrás — la versión
que no contesta se queda sin contestar hasta que alguien redespliega a mano.

**La serie arranca en `v1.0.0`.** No en 0.x: esto ya sostiene trabajo de verdad
en instancias de verdad, y un 0.x le dice a quien lo aloja que puede romperse sin
aviso, que es lo contrario de lo que promete el resto de este modelo. Y no en v2:
`v0-plane-as-record` es un tag con NOMBRE —el archivo de otra implementación— y
no la versión 0 de ésta. De ahí en adelante lo deciden los commits:
`BREAKING CHANGE` o `tipo!:` el mayor, `feat:` el menor, `fix:` y `perf:` el
parche. `docs:`, `chore:` y `style:` no publican nada — una versión que no cambia
nada enseña a ignorar las versiones.

**La versión se VE, abajo del todo en la columna.** Un producto autoalojado donde
nadie sabe qué versión tiene es uno que nunca se actualiza, y toda incidencia
empieza adivinando. En desarrollo dice `dev` en vez de un número: lo que sale de
`git describe` ahí es `v1.0.0-12-gabc1234-dirty`, que como número de versión no
es cierto — el binario no es esa versión, es doce commits después de ella.

**What BREAKING means here is not the HTTP API, y tampoco la forma de una tool
de MCP** — corrigiendo lo que esto decía. Decía «una tool de MCP que
desapareció», y es demasiado ancho: un agente vuelve a leer la descripción de
cada tool al empezar cada sesión, así que una salida que cambia de forma se
absorbe sola. Nadie compila contra ella. Lo que no se relee es una base de
datos.

Lo que sí sería BREAKING por esa puerta es que la aplicación web dependiera del
MCP — y no depende: la UI habla con la API HTTP, y el MCP es una puerta aparte
para los agentes. Si algún día se cruzan, esta regla cambia con ellas.

This is a self-hosted binary, so the contract that can hurt somebody is their
data and their agents:

  · a migration that cannot be rolled back — the database belongs to whoever
    hosts it, and `just migrate down` has to remain an honest offer;
  · an MCP tool that disappears or changes shape — an agent configured last
    month stops working, and it will not say why in a way its person can act on.

A change to the interface, however large it looks, breaks nothing that somebody
else depends on.

**Numbering starts at `v1.0.0`** — corrigiendo lo que esto decía. Decía
`v2.0.0-alpha.N`, con dos argumentos: que `v2.0.0` reclamaría una v2 entregada
con la fase 6 sin empezar, y que el único tag existente es un marcador de
archivo. El segundo sigue en pie y es justo por lo que el número NO es 2:
`v0-plane-as-record` es un tag con nombre, el archivo de otra implementación, no
la versión 0 de ésta — así que no hay ninguna serie previa que continuar y el
«2» no significaba nada fuera de esta conversación.

El primero se cayó solo: la fase 6 es leer dos bases de v0, no una promesa
pendiente sobre lo que este binario hace. Lo que hay ya sostiene trabajo de
verdad en instancias de verdad, y una serie alpha indefinida le dice a quien lo
aloja que puede romperse sin aviso — que es lo contrario de lo que promete el
resto de este modelo, y la razón por la que «BREAKING» aquí significa su base de
datos y sus agentes.

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

## Crear un equipo no tumba Ajustes *(built)*

Al crear un equipo, Ajustes → Equipos caía con `Svelte error: props_invalid_value — Cannot do bind:value={undefined} when value has a fallback value` (en `FilterPick`). El selector «Agregar a alguien…» de cada equipo hacía `bind:value={adding[t.id]}`; un equipo nuevo aún no tiene clave en `adding`, y Svelte 5 prohíbe enlazar `undefined` a un prop declarado `$bindable('')`. Se ve en cuanto un equipo tiene a alguien fuera, y el nuevo nace sólo con quien lo crea.

- Arreglo: *function binding* `bind:value={() => adding[t.id] ?? '', (v) => (adding[t.id] = v)}` en `SettingsTeams.svelte`. Rechazado: quitar el valor por defecto de `FilterPick` (16 usos dependen de él) o precargar `adding` con cada equipo (se rompe con el siguiente equipo que llegue por recarga).
- Revisados los demás `bind:value` a entradas de diccionario: son `<input>` nativos sin valor por defecto, o variables simples; no tienen este fallo.
- Regresión: `frontend/tests/browser/teams.mjs` (componente real, API simulada) — la versión anterior falla con el mismo error; la actual crea el equipo y agrega a una persona. Manual, no forma parte de `just check`.

## Un nombre ausente no tira el selector de personas *(built)*

La captura del error `Cannot read properties of undefined (reading 'toLowerCase')` se trazó al bundle histórico `index-DkkrsQ63.js` de `9fab794`, posición `11:19561`: el filtro de `BubbleDrawer` llamaba `p.name.toLowerCase()`. En Todos, `Board` construía ese nombre con `display_name || email`, pero el nombre es opcional y el correo puede venir oculto. No era el arreglo anterior de los índices Unicode de `fuzzy.ts` ni un evento de teclado sin `key`.

- `frontend/src/lib/people.ts` concentra `named` (nombre/correo visible/respaldo, espacios y marca de eliminado). `api.ts` reutiliza la misma política que ya tenía; `Board` la aplica también al directorio de Todos, conservando id y avatar.
- `matchingPeople` normaliza además las opciones del cajón antes de buscar y construir la colección del combobox: incluso un consumidor que omita `name` entrega una etiqueta válida, sin quitar a esa persona ni alterar los datos originales.
- Rechazado: hacer públicos correos para obtener un respaldo; descartar opciones sin nombre; dar el incidente por resuelto porque se corrigió otro `.toLowerCase()`.
- `frontend/tests/people.test.ts`: 7 pruebas nuevas de nombres, correos omitidos, null/espacios, eliminados, búsquedas Unicode y conservación de identidades. La expresión anterior falla con el mismo fixture mínimo.
- `frontend/tests/browser/people.mjs` y su README conservan el recorrido sin credenciales: intercepta toda la API con datos inventados, prueba Todos/cajón, búsqueda del respaldo, asignación por id y cambio de equipo. En Vite monta el componente real con `name` omitido; sustituir sólo en el navegador el predicado por el anterior reproduce el TypeError, y el actual pasa. No afirma reproducir la sesión/registro original: abrir Todos sin más no hizo caer el bundle anterior, porque el derivado puede no evaluarse allí.
- Validación local: `just bundle` y `just check` (797 live + 58 frontend, 0 fallos; svelte-check sin errores/advertencias), y navegador contra Vite y un binario recién construido con directorios de fixture. La prueba de navegador es manual, no parte de `just check`; no se añadió dependencia al producto.

El arreglo está en el árbol de trabajo y en `web/dist`; **sin commit, push ni despliegue** en esta ejecución. No se consultó producción ni se reinició el dev del usuario.

## Todos los equipos *(built)*

La vista del §10 del [diseño del hilo #3](https://bubble.ayetec.space/w/ws-nbu6tp/t/3), adelantada a petición de Angel antes del resto de la evolución del modelo (era parte de la entrega M6). Sólo para el lead global con dos o más equipos compartidos: el selector de equipo ofrece arriba **Todos los equipos** («vista»), que junta los equipos compartidos y deja fuera todo lo personal, también el propio.

- **Es una vista, no un equipo.** `teams.all` vive en el navegador (`localStorage`, por persona); `prefs.team` sigue siendo un equipo concreto, así que lo que el servidor y la IA mandan «al equipo por defecto» no cambia al abrir la vista. Entrar a un equipo (selector, Ajustes, «Entrar a») sale de ella. Rechazado: guardar un valor «todos» en `prefs.team` — el servidor tendría que resolverlo en cada escritura sin equipo, que es justo lo que §10 descarta (crear sin equipo en Todos).
- **El servidor recorta.** `GET /api/board?scope=shared` (`SharedBoards`): 403 a quien no es lead global, 400 a un `scope` desconocido, y ningún proyecto de un equipo personal —ni el propio ni uno al que te invitaron—. El cliente también filtra (`teams.inScope` / `teams.inView`); el servidor existe para que un filtro mal escrito no baste para colarlo. Sin `scope`, `/api/board` no cambia.
- **Crear pide equipo.** En la vista, el menú ofrece «Nuevo proyecto en <equipo>» por cada compartido y `NewWorkspace` lleva un selector; nunca se usa el equipo escondido de las preferencias. Burbujas e hilos ya no se creaban desde Todos (no hay proyecto).
- **La secuencia de un equipo a la vez.** `/secuencia` es un solo lienzo a pantalla completa, de sólo lectura; la insignia del título es un menú con los equipos compartidos. Elegir ahí no cambia de equipo (se recuerda en el navegador, `bubble.sequence.team.<id>`); «Entrar a <equipo>» va abajo, junto a Equipo/Lo mío y Tareas/Módulos. No hay línea global que las mezcle y reordenar es de quien entra. Antes había un lienzo apilado por equipo; Angel prefirió uno entero con selector (20 oct).
- **El planeador pide entrar a un equipo.** Escribe objetivos, calendario e inbox en el equipo actual: desde la vista, enseña «El planeador es de un equipo» y un botón por equipo. Rechazado: abrirlo con el equipo escondido (escribiría en uno que nadie eligió).
- Tablero: título «Todos los equipos» y proyectos nombrados «Equipo · Proyecto». Roadmap, ReadPlanner y Agenda filtran con las mismas dos funciones.
- **Fuera de alcance, sin cambiar:** el omnibar busca en el board cargado sin recortar por equipo, igual que antes (en `/secuencia` eso incluye lo personal propio). La vista de blobs y el nivel Burbujas/Hilos llegaron después (sección siguiente); la furia sigue siendo prototipo.
- Validación local: `just check` (803 live — 6 nuevas de privacidad de `scope=shared` — y 71 frontend, 6 de ellas `teamsView.test.ts`); `frontend/tests/browser/allteams.mjs` contra Vite con la API simulada (vista, recorte, columna, crear con equipo, secuencias por equipo, planeador, recarga), manual. Falta la revisión visual de Angel. Sin commit, push ni despliegue.

## Vista de blobs por tags *(built)*

La vista espacial del [diseño del hilo #3](https://bubble.ayetec.space/w/ws-nbu6tp/t/3) (§8, funcional §10), sin furia. Agrupa por los **tags del frontmatter** de los documentos de hilo (entrega E4, adelantada para esto).

- **Por qué tags y no contextos (20 oct).** Primero se construyeron *contextos*: un catálogo global (del lead global) y por equipo (de sus leads), con una burbuja eligiendo uno, Ajustes → Contextos y cuatro tools MCP. Angel lo descartó: era algo más que mantener. El frontmatter es gratis, porque cada persona conoce su área y lo escribe con su IA, que lo discute y lo acuerda con las de los demás. Se quitó todo: colección, campo de la burbuja, preflight al mover un proyecto, tools, selector del cajón, pantalla de Ajustes y pruebas. La migración que lo creaba nunca se publicó y se borró; `1788539000_drop_contexts.go` limpia las bases de desarrollo que la aplicaron y no hace nada en una nueva. Rechazado: mantener los contextos junto a los tags (dos clasificaciones que pueden contradecirse), y `context:` como clave propia del frontmatter (los tags ya acordados bastan).
- **Fuente e índice.** `tags: [pagos, cliente/acme]` (o `tags: pagos, móvil`) al principio del documento del hilo. `md.Tags` los lee con un parser YAML mantenido (`go.yaml.in/yaml/v3`), los normaliza (sin `#`, case folding, NFC, espacios colapsados, jerarquía con `/`, sin repetir, máximo 20 de 60 caracteres) y conserva el orden escrito. `threads.tags` es un índice que escribe sólo el servidor: se recalcula al escribir el documento principal del hilo (no sus páginas) y al arrancar, por si un archivo cambió con el servidor apagado; lo que mande un cliente se ignora. Un YAML roto no rompe la escritura: no da tags. El resto del frontmatter se conserva sin autoridad.
- **Clasificar no calienta.** Si una escritura sólo cambió `tags` (mismo cuerpo, mismo resto de frontmatter), se registra `document-classified`, que no calienta y tampoco cuenta como pulso: la vista `thread_evidence` lo deja fuera de `last_any_at` (migración `1788540000_thread_tags.go`). Renombrar o etiquetar no mantienen vivo un hilo, y cambiar sus tags tampoco. Cambiar el cuerpo sigue siendo `document-changed`.
- **El frontmatter no se dibuja.** `RenderHTML` y `RenderThreadHTML` lo quitan; el editor (CodeMirror sobre el Markdown crudo) lo muestra y lo conserva.
- **Agrupar.** Un hilo va al blob de su **primer** tag; sin tags, a «Sin tags». Una burbuja, que no tiene documento en git, va al primer tag más común entre sus hilos (empate: alfabético). Un tag es una palabra, no un registro con dueño: en Todos los equipos el mismo tag de dos equipos es el mismo blob, y nada personal entra. Los blobs van del más poblado al menos. `project_board` y `thread_get` devuelven los tags; la guía de agentes explica cómo escribirlos y que reutilicen los existentes. **Pendiente de E4:** unir las `labels` actuales del hilo con los tags (hoy conviven), editar tags desde la UI y jerarquías como blobs anidados.
- **Aristas.** `Board.links` (`{from,to,kind}`): sólo relaciones declaradas entre hilos, con los dos extremos visibles y del mismo equipo. `blocking`/`blocked_by` → `dependency` dirigida (de lo que va antes a lo que espera); el resto → `related`. Compartir un tag nunca es arista. Todos los boards recortan las aristas a lo que devuelven, así que nadie recibe el id de un extremo que no ve.
- **Tablero.** Una barra abajo al centro, con los grupos segmentados de la secuencia: **Burbujas / Hilos**, **Blobs / Flotabilidad** e «Incluir cerradas». Va en la franja del HUD, a la altura de las esquinas (`bottom: 1rem`, fila de 42 px), montada con `Portal` en la columna del pane, porque dentro del pane se movía con el scroll y quedaba bajo la máscara de su borde. En pantallas de menos de 1150 px sube sobre las esquinas. Flotabilidad sigue siendo la disposición por defecto y la única con orden por calor y minimapa. La preferencia guardada «contexts» se lee como Blobs.
- **Blobs es un lienzo:** `BlobGraph.svelte` con `canvas` llena el pane, con el fondo punteado de la secuencia que sigue a la cámara, el zoom arriba a la izquierda, el título y las abiertas en una insignia arriba al centro, y la leyenda sobre el HUD. «Ver todo» encuadra en la banda libre. Cursores de los lienzos (`data-cursor`: mano abierta, puntero en un nodo, mano cerrada sólo cuando el gesto ya es arrastre). Sin puntos, el aviso vive en el origen del mapa y se mueve y escala con él, como en Mi espacio y en la secuencia (que además se centra en ese origen al abrirse vacía o al pulsar «Ver todo»). En pantallas angostas el zoom va en su fila y el título debajo. **Pendiente:** en un teléfono, al zoom mínimo, los nombres de blobs vecinos todavía pueden tocarse.
- **Burbujas de verdad, quietas.** Los puntos son las mismas esferas de Flotabilidad (`Orb.svelte`), dibujadas en SVG con sus degradados por banda (caliente, dormida, tumba, cerrada): degradado de 150°, oscurecido abajo a la derecha, volumen del borde y brillo. No se arrastran: la disposición es la que es, y arrastrar —empiece donde empiece— mueve la cámara; un clic sin movimiento abre el detalle. Al pasar el cursor **dentro de un blob** (su círculo más 30 px), una **lente** agranda ese blob como el dock de macOS: sus burbujas en ~150 px crecen de forma progresiva (hasta ~34 px de diámetro bajo el cursor) y se abren hacia afuera con un ojo de pez (Sarkar–Brown, g(x) = (k+1)·x / (k·x+1)), que en el borde de la lente no mueve nada; las aristas las siguen. En el fondo vacío no se agranda nada, y los otros blobs no se tocan. Sólo se ve el nombre de la burbuja bajo el cursor o con el foco del teclado (16 px, fijo); ninguno más, ni cuando el cursor está fuera (Angel: los que «cabían» se leían como etiquetas sueltas). Los títulos de los blobs miden 15–28 px y van, con los nombres, en una capa encima de las esferas, así que un blob inflado no tapa su título. El cursor se lee una vez por cuadro; al salir, todo vuelve; al arrastrar, la lente se apaga. Hover y clic son del punto más cercano al cursor dentro de 24 px (lo conserva hasta 36 px), a cualquier zoom. Es lectura: ninguna posición cambia. Rechazados: crecer sólo la burbuja bajo el cursor (a zoom bajo, 9 px que crecen 30 % no se ven); mostrar y agrandar los nombres de las vecinas (se encimaban y Angel prefirió sólo el del cursor); una lente activa también en el fondo vacío; y subir el nodo en hover al final del DOM (lo movía entre pointerdown y pointerup y el navegador perdía el click).
- **Vista 3D** (`Graph3D.svelte`, `lib/graph3d.ts`): tercera disposición, **Blobs | 3D | Flotabilidad**, para leer las conexiones. Cada blob es una esfera de puntos (Fibonacci) y sus centros, otra esfera mayor; determinista y calculada sobre el universo. Un `<canvas>` con perspectiva hecha a mano y las esferas de Orb pre-dibujadas por color: sin dependencias (rechazado: three.js, cientos de KB para algo que una proyección resuelve). Las aristas se atenúan con la profundidad y las dependencias llevan flecha. Un **piso** bajo la figura da la noción de rotación: una retícula en un plano horizontal que gira con ella y se desvanece en disco hacia el borde, con los dos ejes centrales marcados (las sombras de los blobs en el piso se quitaron a pedido de Angel). Se dibuja primero, debajo de todo; cada línea va en tramos para que la perspectiva la curve y la recorte bien. Debajo del zoom, un **gizmo** de 112 px con los ejes X, Y (arriba) y Z girados como la cámara (sin perspectiva, el de adelante encima y el extremo negativo como punto tenue); cada eje es un botón que lleva la vista, en 320 ms (al instante si se pide menos movimiento) y por el giro más corto, a mirar a lo largo de él: de lado, desde arriba o de frente. En la píldora de controles, «⇄» invierte el arrastre (los dos ejes del giro); esa preferencia y el modo de giro se guardan por persona en el navegador (`bubble.board.3d.<id>`). El giro tiene dos modos: **Automático** (gira desde que se abre la vista: al cargar, al volver desde otra disposición o nivel y al volver a la pestaña del navegador) y **Desactivado**. Lo pausan sólo dos cosas: tener el cursor sobre una burbuja o un blob (se reanuda 300 ms después de quitarlo; si una burbuja pasa girando bajo el cursor quieto, también pausa) y arrastrar (10 s desde que se suelta; un clic sin moverse no cuenta). La rueda, el zoom, «Ver todo» y el gizmo no lo pausan. Parado, no se redibuja: un cambio de estado o el temporizador lo despiertan. Antes empezaba tras 30 s quieta y cualquier interacción lo pausaba; Angel prefirió esto. Lo guardado antes como booleano se lee: verdadero es Automático. Nunca gira si el sistema pide menos movimiento. Con el cursor sobre una burbuja se iluminan sus aristas y sus vecinas, y lo demás se atenúa; sobre un blob, las aristas que entran y salen de él. Arrastrar rota, la rueda acerca (listener no pasivo), «Ver todo» encuadra; gira sola hasta que la tocas, nunca si el sistema pide menos movimiento. Un clic abre como en Blobs. **Límite:** un canvas no se navega con teclado; la etiqueta para lectores de pantalla lo dice y remite a Blobs.
- **Tamaño y color a elección** (`lib/encoding.ts`, `EncodingPanel.svelte`, `EncodingLegend.svelte`): en Blobs y 3D cada persona elige qué dice el **tamaño**, con **varios atributos a la vez que se suman** (cada uno escalado de 0 a 1 entre los puntos visibles; el total da el tamaño, con área proporcional, de 0.7× a 2×): calor, prioridad, conexiones, responsables, vencimiento, y según el nivel **cantidad de hilos** (Burbujas) o **archivos** y **texto** (Hilos). «Igual» excluye a los demás y quitar el último vuelve a «Igual»; lo guardado antes con un solo atributo se lee como lista de uno. Archivos y texto salen de `threads.doc_files` y `threads.doc_words` (migración `1788541000_thread_doc_stats.go`): índice del servidor, como los tags, recalculado al escribir o borrar algo del hilo y al arrancar; palabras del Markdown del documento y sus páginas sin frontmatter, y archivos = páginas de su carpeta más imágenes de `assets/` citadas, sin repetir. Un cliente no los escribe. No es calor y el **color** (estado, prioridad, proyecto, equipo, tag secundario, vencimiento, mío; los categóricos con doce tonos separados por orden alfabético, estables). **Filtros inteligentes**, combinados con Y: lo mío, urgentes P1–P2, sin responsable, vencidas o por vencer, muy conectadas (cuartil alto), frías, más búsqueda por nombre, proyecto o tag sin acentos ni mayúsculas. Lo que no pasa se atenúa u oculta; ocultar no reacomoda. La leyenda dice qué significa cada cosa y cuántas pasan. Se guarda por persona en el navegador (`bubble.board.encoding.<id>`), leída sin confiar en ella. Sin dato (sin fecha, sin prioridad), el tamaño base y el color «sin…», declarados en la leyenda: no se inventa nada.
- **Rendimiento (medido, 20 oct).** Con ~410 hilos en Todos los equipos, entrar a Flotabilidad congelaba la UI ~20 s (una tarea de 14 s); Blobs y 3D tardaban 114 y 18 ms. El perfil de CPU lo atribuyó a cada `Orb`: (1) medía si su nombre cabía leyendo `scrollHeight` al montarse, después de que otras burbujas cambiaran el DOM, un layout de la página entera por burbuja (~13 s); (2) cebaba su menú contextual al montarse, lo que hacía a floating-ui posicionar 410 menús (2–5 s); (3) montaba su menú y sus tooltips aunque nadie los abriera. Arreglos: medir los recortes en lote (`lib/clip.ts`: un cuadro lee todo y después escribe), y montar y cebar el menú y los tooltips al primer acercamiento (hover o foco). Resultado en dev: Flotabilidad de hilos **19 759 → ~215–330 ms**, de burbujas **1 934 → ~70–150 ms**, recargar con Flotabilidad sin tareas de más de ~130 ms, y la página de 10 807 a 5 887 elementos. Además, Blobs sólo dibuja los nombres que se ven (5 859 → 5 039 elementos) y el 3D lee los colores del tema una vez, no en cada cuadro. Comprobado que no cambió: el primer clic derecho sitúa el menú junto al cursor, la tecla de menú contextual lo abre, los tooltips de las insignias salen y la detección de nombres recortados da 0 discrepancias (39 de 40 en Demo Plataforma). Si con más volumen vuelve a pesar, lo siguiente sería mostrar las primeras N burbujas por banda con «+N más», que es un cambio visible y queda a decisión de Angel.
- **Disposición que escala.** Los blobs se acomodan en filas, con el tamaño de cada uno y el ancho de su nombre, en un conjunto apaisado y determinista sobre todo el universo (abierto + cerrado), así que «Incluir cerradas» no mueve nada. El nombre de un punto se dibuja sólo si, con el zoom actual, no choca con el nombre ni con el punto de otro punto visible (`visibleLabels`). Rechazados: dos columnas fijas (una tira angosta con grupos encimados), un umbral único de zoom (apagaba los nombres de un proyecto chico) y la distancia al vecino más cercano (dejaba nombres sobre los puntos de abajo).
- **Menú en Todos los equipos:** el Planeador se ve deshabilitado con «entra a un equipo» (`ShellPin.off`). Inventario, Prompts, Conectar tu IA y Ajustes no dependen del equipo.
- **Datos de ejemplo:** `just seed tags` (`scripts/seed_tags.py`) crea por la API del dev local, como la persona de `.env`, dos equipos «Demo», cuatro proyectos, burbujas e hilos con frontmatter `tags: [tema, superficie]` (algunos repetidos entre equipos), hilos sin tags y sueltos, una burbuja cerrada y dependencias y relaciones dentro de cada equipo. Sólo corre contra localhost y es idempotente (comprobado en un servidor desechable: la segunda corrida no escribe nada).
- **Volumen de ejemplo:** `just seed docs` (ahora también con 0–3 páginas por hilo, 0–2 imágenes reales citadas y textos de largo variable, sacados de un generador propio por hilo para que el plan de equipos, burbujas e hilos no cambie respecto de corridas anteriores) (`scripts/seed_docs.py`) siembra en el dev, de forma determinista e idempotente, tres equipos «Demo», once proyectos, ~80 burbujas y ~400 hilos, cada uno con un documento real (para qué, hecho cuando con casillas, notas) y frontmatter de uno a tres tags de un vocabulario compartido (con `cliente/…`). Incluye hilos sin tags, hilos cuyo primer tag no es el de su burbuja, hilos completados, burbujas cerradas y sin responsable, y relaciones cercanas: cadenas de dependencia dentro de una burbuja, algunas entre burbujas del proyecto y pocas entre proyectos del equipo. Borra las relaciones entre sus propios hilos que ya no están en su plan; lo que no sembró no lo toca. Tarda ~75 s la primera vez y menos de un segundo después. No falsea fechas, así que no hay hilos dormidos ni en tumba.
- **Sin furia, a propósito.** Todos los nodos reales son `calm` y el grafo se monta con `fury={false}` hasta que exista el evaluador (E6).
- **Validación local:** `just check` en verde (828 live: tags normalizados por MCP y REST, frontmatter fuera del HTML, clasificación sin calor ni pulso, cuerpo como evidencia, índice que un cliente no escribe, sin colección ni campo de contexto; 86 frontend; `internal/md/frontmatter_test.go`; `migrations/thread_team_test.go` revierte hasta el contador y reaplica, incluida la de tags). `drop_contexts` probada sobre una copia de la base de dev: quita tabla, colección y campo, y conserva las burbujas. `frontend/tests/browser/blobs.mjs` con toda `/api/` simulada: ambas disposiciones × ambos niveles, primer tag, mismo tag entre equipos, nada personal, aristas, sin furia, cursores, cerradas sin mover nodos, hilo suelto, preferencias, sin Contextos en Ajustes ni en el cajón, móvil. `allteams`, `people` y `model` siguen pasando. **Falta la revisión visual de Angel; sin commit, push ni despliegue.**

## E1 — hilo de equipo sin proyecto: en curso

La base de la evolución (diseño técnico §2–§3). Se construye por tramos; cada uno se valida antes del siguiente. **Un hilo todavía no puede existir sin proyecto**: lo construido prepara el modelo sin abrir ningún caso nuevo.

- **Números que no se reutilizan** *(built)*. `thread_counters` (tabla interna, sin colección ni API): un contador por `(scope_kind, scope_id)`, hoy sólo `workspace`. `nextThreadNumber` lo sube y lo devuelve en un único upsert con `RETURNING`, y REST (`assignSeq`) y MCP (`CreateThread`) usan esa misma función. Antes era `MAX(seq)+1`: borrar el último hilo le regalaba su número —y su URL `/w/<slug>/t/N`— al siguiente, y dos altas a la vez chocaban contra el índice único. Un alta fallida deja un hueco; rechazado: llenar huecos, porque un número repetido abre otro hilo desde una URL vieja. El contador nunca queda por debajo de `MAX(seq)`. Límite: el relleno parte del máximo actual, así que un número más alto que se borró *antes* de la migración todavía puede salir una vez.
- **El hilo dice de qué equipo es** *(built)*. `threads.team`, que **deriva el servidor** en `OnRecordValidate` (cubre REST, MCP y `app.Save`): con proyecto es el equipo del proyecto, y lo que mande el cliente se pisa. Mover un proyecto de equipo actualiza sus hilos en la misma transacción que el proyecto (`OnRecordUpdate` envuelto en `RunInTransaction`, porque el CRUD de PocketBase no la abre). Ninguna regla ni permiso lee aún este campo; el acceso sigue colgando del proyecto. Rechazado: deducir el equipo de una ruta, o dejar que el cliente lo elija.
- **Validación:** `just check` en verde, 843 live (sin reutilizar número tras borrar, por REST y MCP; 8 altas simultáneas con 8 números distintos y seguidos; contador sin API; equipo derivado aunque el cliente mande otro, por REST y MCP, que no se cambia a mano y que acompaña al mover el proyecto). `migrations/thread_team_test.go` revierte las dos migraciones sobre una app real, crea hilos como antes y comprueba el relleno de `team` y el contador; se comprobó que la prueba falla con el relleno roto.
- **Siguiente tramo:** el hilo de equipo, sin proyecto. `workspace` opcional; numeración con `scope_kind = team`; documento en `repos/team-<id>/` resuelto por un sujeto de documento del servidor; lo ven los miembros del equipo (en un equipo personal, sólo su dueño). Por defecto un hilo sin proyecto sigue siendo invisible: `ThreadFor` falla cerrado y cada superficie se abre de forma explícita, con sus pruebas. Después: organizarlo en un proyecto con diario recuperable (§3), y direcciones e historia por tramos.

## Evolución del modelo: propuesta técnica, no construida

Definición funcional confirmada y trabajo de diseño en [Burbujas, hilo #3](https://bubble.ayetec.space/w/ws-nbu6tp/t/3). El [documento técnico durable del hilo](https://bubble.ayetec.space/w/ws-nbu6tp/wiki/threads/3-dise-ar-la-evoluci-n-del-modelo-de-bubble-work/diseno-tecnico.md) mantiene contratos, migración, matriz de pruebas, wireframes y entregas estimadas; es la fuente del plan de ejecución, sin otra copia de sus casillas aquí.

- **Modelo propuesto:** un solo inbox de ideas y cosas por hacer; toda captura es hilo durable/versionado, posible sin proyecto; activación explícita independiente de organizar. Responsabilidad equivalente múltiple. Prioridad por entradas/procedencia. Furia derivada de compromiso y estancamiento, máximo de causas, sin alterar calor/flotabilidad. Objetivos, focos y tags son conceptos distintos (los contextos de la propuesta se descartaron el 20 oct: ver «Vista de blobs por tags»).
- **Fronteras:** abrir deliberadamente también inbox/temporales antiguos de equipos compartidos; no lo personal ni los proyectos restringidos. Reclamar/mover no invita en silencio. Vistas Todos no son un equipo nuevo ni una secuencia global superior. Avisos primero en app+MCP, sin LLM servidor ni canales externos de salida.
- **Almacenamiento recomendado para revisar:** extender el sujeto de documento a equipo/proyecto, repositorio técnico de inbox por equipo sin fila de proyecto, id de hilo estable y alias de direcciones/números no reutilizados. Mover con diario recuperable DB+git, comprobación de permisos actual y tramos de historial: `git log --follow` no cruza repositorios por copiar un archivo.
- **Contratos propuestos:** operaciones de dominio compartidas por REST/MCP y hooks contra CRUD que evada controles; reclamación aditiva transaccional con procedencia; presencia no exclusiva/caducable; compromisos versionados; esperas causales y tiempo descontado por unión de intervalos; un evaluador puro de furia con reloj inyectado; avisos/resúmenes deduplicados y comprobación de ACL incluso al leer avisos antiguos.
- **Descartado:** proyecto público/ficticio para el inbox, copia DB del cuerpo antes de git, prioridad/furia editables, exclusión por sesión, dependencia entre equipos, nueva IA como persona, frontmatter como autoridad, presión disfrazada de evidencia y una migración que borre/prometa historia que no tiene.
- **Prototipo disponible, no integración:** `frontend/src/prototypes/ModelEvolution.svelte` abre directamente en **`http://localhost:5173/theme/model`** con `just dev`. `main.ts` lo monta en vez de App sólo bajo `import.meta.env.DEV`; se verificó que URL, datos y chunk no aparecen en el bundle de producción. La URL directa/interacción/recarga/barra final pasa con sesión simulada y cero peticiones API/errores. Se rechazó exigir Playwright/otro comando para la revisión habitual: el runner `frontend/tests/browser/model.mjs` queda para automatización (bloquea `/api/`) y comprueba además alternancia/filtros, explicaciones, contextos globales/locales, secuencias paralelas, elección de equipo, cerradas, preferencia aislada, viewport estrecho y teclado básico. Capturas/guía en `docs/prototypes/model-evolution/`; `--review` sigue como alternativa. Furia/compromisos son fixtures, no un motor nuevo; el tablero normal `/todos` no cambia. La ejecución comprobada es headless, no una aprobación visual de Angel.
- **Revisión visual, Obsidian y dos niveles:** Angel descartó las cajas de tarjetas y pidió un grafo espacial y pestañas **Burbujas / Hilos**, independientes de Contextos/Flotabilidad. El prototipo usa Tabs de Skeleton y `ContextGraph.svelte`: contextos como constelaciones/halos con etiquetas, nodos de calor y aro de furia, zoom/pan/arrastre/detalle. Layout determinista sin simulación continua; no rebaraja al filtrar. Las aristas son relaciones explícitas de fixtures (proyectadas desde hilos para el nivel burbujas), nunca por compartir contexto; no se dibujan extremos filtrados ni vínculos entre equipos. Los hilos conservan compromiso propio, no el de la burbuja. Se conservan filtros y preferencias aisladas de nivel/disposición. En el grafo no hay orden vertical por calor; Flotabilidad sigue para ese criterio. Sin dependencias nuevas, cambios de API ni integración en `/todos`.
- **Validación de esta revisión:** 7 pruebas nuevas de fixtures/layout/proyección/cámara (`modelGraph.test.ts`), navegador con ambos niveles, filtros, teclado, zoom/pan/arrastre, detalle y viewport estrecho; cero API/errores. Suite completa 797 live + 65 frontend, bundle reconstruido y exclusión de datos/URL/chunk de prototipo en producción comprobada. Capturas nuevas de hilos y guías actualizadas; aprobación visual de Angel sigue pendiente.
- **En la app maqueta para iterar:** a petición de Angel, `/theme/mock` (la app completa con datos inventados) carga en DEV `prototypes/MockModelBoard.svelte`: pestañas Burbujas/Hilos y Contextos/Flotabilidad sobre el tablero real de orbes, el grafo, el cajón y la vista de hilo reales. Datos en `prototypes/mockModel.ts`. Import dinámico bajo `import.meta.env.DEV`: el `/theme/mock` publicado no cambia (bundle comprobado). Rechazado: meter datos falsos en `/todos` con sesión real (mezclaría trabajo real con inventado) y publicar la capa en producción. Límites: un solo proyecto, sin filtros, orbes sin furia, documento de hilo fijo. Navegador cubre ambos ejes, apertura de hilo/cajón, furia y persistencia.
- **Aún no validado:** revisión técnica/visual por Angel, geometría/movimiento en el tablero real, lectores de pantalla/dispositivo táctil/carga, spike de movimiento/reconciliación, ensayo de migración/restauración y las pruebas del backend nuevo. Las estimaciones son orientativas, no compromisos. En el producto están el arreglo de nombres, la vista Todos los equipos y la vista de blobs por tags (secciones anteriores), probados localmente; furia, compromisos y el inbox como hilos siguen sólo en el prototipo.

En conflictos con ideas anteriores de Later/Open manda la definición confirmada del hilo; conservar aquellas notas no mantiene vigentes sus alternativas descartadas.

La migración propuesta del V2 es distinta de Phase 6: no altera ni borra los originales v0 (`thread_docs`, `local_pages`, `v0-reko/`). Se conserva su compatibilidad como criterio de aceptación. El siguiente paso del diseño es revisar el documento técnico y autorizar entregas, no ejecutar una migración ni declarar disponible la nueva semántica.

# Open

- Who commits, and how often. One commit per write is the plan; whether an agent's
  rapid edits should be squashed per session is not decided.
- Non-image attachments — PDFs and the like — are still refused. Nothing has
  needed one yet, and each type added is more in the repository forever.
- Excalidraw's editing surface. The sidecar file is decided; the editor is not.
- Whether `cycles` still earns its place now that no upstream tool supplies them.
