# Documentation map

Four kinds of document, and the difference between them is the point:

| Kind | Answers | Kept current? |
|------|---------|---------------|
| [the model](../README.md) | *how do I think about work?* | yes — it is the framework |
| [`modules/`](./modules) | *what is true about this part today?* | **yes — normative** |
| [`decisions/`](./decisions) | *why did the model change?* | written once, superseded rather than edited |
| [`journal/`](./journal) | *how did the work actually go?* | **no** — kept as written |

The journals are worksheets: the problem as it was measured, the phases, what the
live instance changed. They are the best reading in the repository and the worst
place to learn how something works now, because they record a moving target. When a
journal and a module file disagree, **the module file wins**.

## Start here

| Question | File |
|---|---|
| What is Bubble Work? | [`../README.md`](../README.md) |
| How is it built? | [`ARCHITECTURE.md`](./ARCHITECTURE.md) |
| How do I run / tune / develop it? | [`operations.md`](./operations.md) |
| How do I deploy it on a server? | [`../DEPLOY.md`](../DEPLOY.md) |
| What rules do agents load? | [`../AGENTS.md`](../AGENTS.md) |

## Modules

| Module | Status | What it covers |
|---|---|---|
| [`heat.md`](./modules/heat.md) | built | the pure temperature function, evidence kinds, the calibration |
| [`threads.md`](./modules/threads.md) | built | levels, the roll-up, autostate, creating and finishing a thread |
| [`artifacts.md`](./modules/artifacts.md) | built | regions, surgical edits, splicing, todos, revisions, the editor |
| [`documents.md`](./modules/documents.md) | built | markdown as the record, publishing to Plane, importing Plane's edits, export |
| [`pages.md`](./modules/pages.md) | built | a Workspace's reference material, and who holds it |
| [`bubbles.md`](./modules/bubbles.md) | built | the structure tier: contracts, closing vs deleting, the board |
| [`plane-channel.md`](./modules/plane-channel.md) | built | one reader, the rate budget, the outbox, degraded mode, documents both ways |
| [`storage.md`](./modules/storage.md) | built | one SQLite file: the record and the cache, and which is which |
| [`identity.md`](./modules/identity.md) | built | Plane-native identity, derived scope, agent impersonation |
| [`federation.md`](./modules/federation.md) | built | several Plane deployments, namespaced ids, per-instance capabilities |
| [`notifications.md`](./modules/notifications.md) | built | the sweep, transitions, per-member read state |
| [`cli.md`](./modules/cli.md) | built | the command surface |
| [`mcp.md`](./modules/mcp.md) | built | the agent surface, and the rules that bite agents |
| [`web.md`](./modules/web.md) | built | the board, the interior, the reading room |

Each module file follows the same shape: what it solves · the normative model ·
surfaces · storage · invariants · what is open.

## Decisions

| # | Decision |
|---|---|
| [0001](./decisions/0001-plane-is-a-channel-not-the-record.md) | Plane is a channel, not the record — artifact bodies are markdown owned here |
| [0002](./decisions/0002-birth-is-production.md) | Being born is production; being created is not |
| [0003](./decisions/0003-briefs-have-a-type.md) | A Brief has a type, and the type keeps it short *(superseded by 0006)* |
| [0004](./decisions/0004-writing-is-the-work.md) | Any edit is production; a discussed bubble is not a grave |
| [0005](./decisions/0005-the-framework-does-not-own-your-format.md) | A thread needs a name; the birth rule and the DoD gate are gone |
| [0006](./decisions/0006-plane-holds-the-relationships.md) | No Brief template; labels, links and relations come from Plane |

Each decision's Status line says whether it is built, and 0001 and 0003 each record a
part that was deliberately changed or dropped on the way in. That is the point of the
files: a reader in six months can tell a choice from an accident.

## Journal

| Worksheet | Subject |
|---|---|
| [`PLANE-SYNC.md`](./journal/PLANE-SYNC.md) | the mirror, the sync worker, the outbox, degraded mode |
| [`THREAD-LIFECYCLE.md`](./journal/THREAD-LIFECYCLE.md) | per-thread buoyancy, evidence signals, the bubble roll-up |
| [`ARTIFACT-EDITING.md`](./journal/ARTIFACT-EDITING.md) | the splice engine, the editor, the workspace tier, pages |
| [`PAGES-CAPABILITY.md`](./journal/PAGES-CAPABILITY.md) | why a 404 could not be read, and who holds a page |
| [`MCP-ACCESS.md`](./journal/MCP-ACCESS.md) | agents writing artifacts, live updates, sign-in options |
| [`web-ui-design.md`](./journal/web-ui-design.md) | the original board design |

Two worksheets are cited in code comments but no longer exist as files:
`INTERIOR-PLAN.md` (the thread interior — now [`threads.md`](./modules/threads.md)
and [`artifacts.md`](./modules/artifacts.md)) and `bubble-work-spec.md` (the model —
now [`../README.md`](../README.md) and [`ARCHITECTURE.md`](./ARCHITECTURE.md)).

## Writing docs here

- A capability's **normative** description belongs in its module file, in the
  present tense, and states what is true — including what is broken.
- A **decision** gets its own numbered file when it changes the model rather than
  extending it. It records the alternatives, so a future reader can tell a choice
  from an accident.
- A **journal** is append-only in spirit: correct it while the work is live, then
  leave it alone.
- Anything not built yet says so in its Status line. A document that describes an
  intention as if it shipped is worse than no document.
- An invariant is a claim, so it should have a test named after it:
  `TestInvariant_<Module>_<Claim>`. That is what keeps these files honest without
  anyone re-reading them.
