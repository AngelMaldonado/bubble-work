# Documentation map

The v0 implementation is **archived**. What is in this repository right now is the
model, the reasoning that produced it, and the record of what was built — not a
running system. See [the archive](#the-archive).

| Kind | Answers | Kept current? |
|------|---------|---------------|
| [the model](../README.md) | *how do I think about work?* | yes — it is the framework |
| [`decisions/`](./decisions) | *why did the model change?* | written once, superseded rather than edited |
| [`V2-PLAN.md`](./V2-PLAN.md) | *what is being built, in what order?* | yes — the current plan |
| [`DATABASE.md`](./DATABASE.md) | *what did v0 store, and why?* | frozen — the migration source |
| [`journal/`](./journal) | *how did the work actually go?* | **no** — kept as written |
| `modules/` | *what is true about this part today?* | **empty** — v1 has not been built |

## Start here

| Question | File |
|---|---|
| What is Bubble Work? | [`../README.md`](../README.md) |
| Why is the code gone? | [`decisions/0007`](./decisions/0007-bubble-owns-the-record.md) |
| What is being built? | [`V2-PLAN.md`](./V2-PLAN.md) |
| What did v0 store? | [`DATABASE.md`](./DATABASE.md) |
| What rules do agents load? | [`../AGENTS.md`](../AGENTS.md) |

## The archive

v0 — the implementation in which **Plane held the record** — is tagged
`v0-plane-as-record`. Everything is recoverable from it:

```
git show v0-plane-as-record:internal/md/splice.go
git checkout v0-plane-as-record -- internal/heat
```

Its documentation moved to [`journal/v0/`](./journal/v0) rather than being
deleted, because the repository's own rule is that a module file states what is
true **today** — and the moment the code went, every one of them became history.
That is what a journal is for.

| Archived | Was |
|---|---|
| [`journal/v0/ARCHITECTURE.md`](./journal/v0/ARCHITECTURE.md) | how the v0 repo was built |
| [`journal/v0/operations.md`](./journal/v0/operations.md) | how to run, tune and develop it |
| [`journal/v0/DEPLOY.md`](./journal/v0/DEPLOY.md) | how it was deployed |
| [`journal/v0/modules/`](./journal/v0/modules) | the fourteen normative module files |

[`DATABASE.md`](./DATABASE.md) stayed at this level on purpose. It is not history:
two live deployments still hold a `bubble.db` in exactly that shape, and it is the
source document for whatever migrates them.

## Decisions

Still in force. The model is not what was archived.

| # | Decision |
|---|---|
| [0001](./decisions/0001-plane-is-a-channel-not-the-record.md) | Plane is a channel, not the record — artifact bodies are markdown owned here |
| [0002](./decisions/0002-birth-is-production.md) | Being born is production; being created is not |
| [0003](./decisions/0003-briefs-have-a-type.md) | A Brief has a type, and the type keeps it short *(superseded by 0006)* |
| [0004](./decisions/0004-writing-is-the-work.md) | Any edit is production; a discussed bubble is not a grave |
| [0005](./decisions/0005-the-framework-does-not-own-your-format.md) | A thread needs a name; the birth rule and the DoD gate are gone |
| [0006](./decisions/0006-plane-holds-the-relationships.md) | No Brief template; labels, links and relations come from Plane *(superseded by 0007)* |
| [0007](./decisions/0007-bubble-owns-the-record.md) | Bubble owns the record; PocketBase, markdown on disk under git, priority derived like heat |

0001 said Plane is a channel and then made that true only of the *bodies*;
structure and identity stayed upstream, which is why every v0 overlay row was
keyed by a Plane id. 0006 leaned further in, handing labels, links and relations
back to Plane. 0007 turns both around: Bubble owns the record, Plane becomes one
optional channel, and the four choices that rest on it — PocketBase embedded as a
Go framework, markdown on disk under git, a file-shaped MCP whose server supplies
belonging and concurrency, and priority derived the way heat is.

## Journal

Worksheets. The best reading in the repository and the worst place to learn how
something works now, because they record a moving target.

| Worksheet | Subject |
|---|---|
| [`PLANE-SYNC.md`](./journal/PLANE-SYNC.md) | the mirror, the sync worker, the outbox, degraded mode |
| [`THREAD-LIFECYCLE.md`](./journal/THREAD-LIFECYCLE.md) | per-thread buoyancy, evidence signals, the bubble roll-up |
| [`ARTIFACT-EDITING.md`](./journal/ARTIFACT-EDITING.md) | the splice engine, the editor, the workspace tier, pages |
| [`PAGES-CAPABILITY.md`](./journal/PAGES-CAPABILITY.md) | why a 404 could not be read, and who holds a page |
| [`MCP-ACCESS.md`](./journal/MCP-ACCESS.md) | agents writing artifacts, live updates, sign-in options |
| [`web-ui-design.md`](./journal/web-ui-design.md) | the original board design |
| [`v0/`](./journal/v0) | the archived implementation's own documentation |

## Writing docs here

- A capability's **normative** description belongs in a `modules/` file, in the
  present tense, and states what is true — including what is broken. That
  directory is empty because nothing is built; the first one is written with the
  first capability, not before it.
- A **decision** gets its own numbered file when it changes the model rather than
  extending it. It records the alternatives, so a future reader can tell a choice
  from an accident.
- A **journal** is append-only in spirit: correct it while the work is live, then
  leave it alone.
- Anything not built yet says so in its Status line. A document that describes an
  intention as if it shipped is worse than no document — and a module file
  describing code that no longer exists is the same failure pointed backwards.
- An invariant is a claim, so it should have a test named after it:
  `TestInvariant_<Module>_<Claim>`. That is what keeps these files honest without
  anyone re-reading them.
