# 0007 — Bubble owns the record; Plane becomes a channel it does not need

**Status:** decided, not built · **Supersedes:** the second half of
[`0001`](./0001-plane-is-a-channel-not-the-record.md) and all of
[`0006`](./0006-plane-holds-the-relationships.md) · **Date:** 2026-09-03

## What changed

[`0001`](./0001-plane-is-a-channel-not-the-record.md) said Plane is a channel and
not the record, and then made that true only of the **bodies**. Structure,
identity, cycles and states stayed upstream, so a bubble id was the literal string
`<instance>:<project>:<module>` and every client read was served from a
fourteen-table mirror of Plane's shape.
[`0006`](./0006-plane-holds-the-relationships.md) leaned further in, handing labels,
links and relations back to Plane.

Three consequences arrived together, and they were one consequence:

- A thread could not leave its project. Plane's module rule limits a move to one
  project, and nothing local outlives a change of id.
- Evidence had to be **inferred**. The writes happened somewhere else, so heat was
  derived by diffing hashes and counters each sync pass: ten edits in a cycle were
  one event, authorship never reached the event, and a change reverted between two
  passes never happened.
- Authentication **was** a Plane API key, so "make Plane optional" and "keep anyone
  logged in" were the same problem.

**Bubble owns the record.** Workspaces, bubbles, threads, states, cycles,
assignment, objectives, priority inputs, labels, relations, comments and people are
Bubble's. Plane becomes one optional channel among several, synchronising specific
things and owning none of them.

## The four choices this rests on

### 1. PocketBase as an embedded Go framework, one binary

`pocketbase.New()` as a library. Bubble's own logic mounts as routes on its router
(`app.OnServe().BindFunc` → `se.Router`), the SPA is served with
`apis.Static(os.DirFS("./pb_public"), false)`, and the database is the embedded
SQLite through `modernc.org/sqlite` — the same driver v0 already used.

What that deletes, rather than what it adds:

| v0 wrote by hand | PocketBase provides |
|---|---|
| the schema plus `ALTER TABLE`s whose errors were discarded, and no version row | **versioned migrations** with an explicit revert (`m.Register(up, down)`) |
| the identity work that blocked everything else | **auth collections** — password, OAuth2, OTP, tokens |
| authorization derived from a mirror of Plane's project membership | **API rules per collection**, evaluated server-side |
| a bespoke SSE endpoint | **realtime** subscriptions per collection or per record |
| God Mode | the **admin dashboard** |
| a backup story that was somebody's cron job | **backups** |

**Rejected:** PocketBase as a separate service Bubble calls over REST. It
reintroduces v0's defining problem — a remote client with a call budget and latency
on the read path — for the sake of an isolation nobody at this scale needs.

**Rejected:** PocketBase for auth and realtime only, domain tables written by hand.
It keeps the two hardest things to write and gives up API rules and the dashboard
for everything that matters.

### 2. The artifact bodies are files on disk, versioned by git

A readable tree — `<workspace>/<bubble>/<thread>.md` — is the record.
PocketBase holds the **path and the metadata, not the bytes**.

This reverses the storage half of `0001`, and pays two of v0's debts at once:
`thread_docs_prev` was an undo one level deep, and the backup story was an
obligation nobody automated. Git gives real history, blame and diff, and makes
`export` stop being a feature.

Git and the event log are not redundant. **Git is the content history; `events` is
the index.** Git cannot cheaply answer "all evidence in this cycle across
workspaces", and most evidence is not a file change at all — a link landing, a
thread completing, a todo ticked in a body that also changed.

**Rejected:** PocketBase `file` fields. Attachments are stored under its own
storage directory with generated names — upload and download for free, but not the
organised, human-readable directories this decision is about.

**Rejected:** keeping the markdown in a text column, as v0 did. Fewer moving parts
and real transactions, but it is the thing being changed.

### 3. The MCP is file-shaped, and the server supplies belonging and concurrency

The tool surface is generic file editing, modelled on
[`mcp-file-edit`](https://github.com/patrickomatik/mcp-file-edit): read, write,
patch by context, list, search, move. Agents address **paths**, not regions.

The objection this had to answer: v0's model lives on *editing the document IS the
evidence*, and a generic `write_file` that nobody observes puts heat back to being
inferred — the exact failure that ended v0.

It is answered in the server, not in the tool's shape:

- **Belonging.** Every path resolves to an entity before the write lands. A write
  under a thread's `doc_path` is attributed to that thread and emits an event. A
  file under a workspace root that belongs to no thread is a loose document: still
  versioned, still an event, at workspace grain. **Nothing is written that is not
  attributed.**
- **Concurrency.** A write carries the hash it read as its base; a mismatch is a
  conflict, not a silent overwrite — v0's `base` mechanism, kept because it is
  stronger than the backup-and-dry-run approach the reference uses. The server
  holds a per-path lock across read-modify-write.
- **Containment.** Paths resolve inside a workspace root; traversal out is refused.

What is taken from the reference is `patch_file`'s context strategy — quote
surrounding lines, not line numbers — which is v0's `edits` under another name.
What is not taken: the code-analysis tools, the SSH tools, and the git tools (the
server owns committing, so an agent must not).

**Rejected:** thread-and-region-shaped tools, as v0 had. Safer by construction,
but it makes every non-thread document unreachable and forces an agent to learn a
vocabulary instead of using the one it already has.

### 4. Priority is derived, exactly like heat

The determinant map is a pure function of impact and urgency:

| | Urgency high | Medium | Low |
|---|---|---|---|
| **Impact high** | P1 | P2 | P3 |
| **Impact medium** | P2 | P2 | P3 |
| **Impact low** | P3 | P3 | P4 |

So `impact` and `urgency` are stored and `priority` is **not**. A stored priority
field lets somebody write P1 over low impact and low urgency, and then the map is
decoration. This is heat's own lesson pointed at a second axis: store the inputs,
derive the verdict.

Heat and priority are **independent axes and both derived**. Heat says how alive a
bubble is (evidence × time); priority says how much it matters (impact × urgency).
The board orders by heat, the planner filters by priority, and the most useful
signal the system can produce is a **cold P1** — which is invisible the moment
priority is folded into the buoyancy score.

**Rejected:** weighting priority into buoyancy. One ordering to understand, but it
conflates *important* with *alive*, and an untouched P1 stops looking like a
problem because it still floats.

**Rejected:** a hand-set priority field with the map as UI guidance only. Simpler
and more flexible, and it permits exactly the inconsistency the map exists to
prevent.

## What is kept from v0, unchanged

Two packages owe nothing to Plane and are ported rather than rewritten:

- `internal/md` — 2408 lines with 1546 of tests: the splice engine, regions,
  `ApplyEdits`, `ToggleTodo`, `ParseChecklist`, the fingerprints, markdown/HTML
  fidelity. This is the rendering and editing engine the model runs on.
- `internal/heat` — 265 lines with 327 of tests: the pure function. Unchanged in
  spirit; it stops consuming a diff and starts consuming a real event stream.

The model in [`README.md`](../../README.md) is unchanged, and decisions
[`0002`](./0002-birth-is-production.md), [`0004`](./0004-writing-is-the-work.md)
and [`0005`](./0005-the-framework-does-not-own-your-format.md) stand as written.

## What this costs

- **Two live deployments hold a v0 `bubble.db`**, and `thread_docs` and
  `local_pages` in them are the only copy in existence of what was written there.
  They must be exported before anything reads that file for the last time.
  [`../DATABASE.md`](../DATABASE.md) is the source document.
- **Two sources of truth for a body** — the file and its row — which can drift. The
  server writing both under one lock is the whole of the answer, and it has to hold.
- **Plane's own UI stops being a place work happens** unless a channel is built for
  it. That is a later decision, not this one.
