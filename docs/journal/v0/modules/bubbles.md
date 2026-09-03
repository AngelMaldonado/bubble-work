# Bubbles & Workspaces — the structure tier

**Status:** built · **Packages:** `internal/server` (`board.go`, `move.go`,
`delete.go`, contract handlers) · **Depends on:**
[`heat.md`](./heat.md), [`threads.md`](./threads.md) · **History:**
[`journal/ARTIFACT-EDITING.md`](../journal/ARTIFACT-EDITING.md) (workspace tier)

## What it solves

The containers: a Workspace is the boundary, a Bubble is the unit of attention. Both
are Plane objects, but the things that make a bubble a bubble rather than a folder —
its contract, its heat, its stage — are ours.

## Model (normative)

### Workspace = Plane Project

Created, renamed and deleted through the server. An **empty workspace still
exists**: a project with no modules used to be invisible everywhere, which made
"create a workspace, then create a bubble in it" impossible as a first move.

### Bubble = Plane Module

Plane holds the module; the server holds:

| Field | Why it is ours |
|---|---|
| Intended outcome | Plane has nowhere to put it |
| Owner | accountability, distinct from Plane assignees |
| Closure condition | the explicit signal that says "close this" |
| Lifecycle level + heat | derived, never stored by Plane |
| `reviewed` 👀 stage | an explicit human overlay the model must not infer |

A bubble without a contract is a thematic folder that never dies, which is the
anti-pattern the contract exists to prevent.

### Closing vs deleting

**Closing is the normal death.** It keeps the record of what was done, which
matters because heat is evidence that work happened. A closed bubble stops taking
attention and can be reopened.

**Deleting is for what should never have existed** — a mistyped bubble, a test
thread, a revision on the wrong parent. It is irreversible and it deletes from
Plane. Two properties are deliberate:

- Deleting a bubble **does not delete its threads**. A Plane module is a grouping,
  not a container; the threads survive belonging to no bubble, which also means they
  leave the board, since the board is built from modules.
- Deleting a thread deletes its children **explicitly and first**, rather than
  trusting Plane to cascade — leaving that undecided would leave sub-items with a
  parent that no longer exists.

Every delete writes through to the mirror and the overlay immediately: the sync
worker is a reconciler, not the delivery mechanism, and a projection that still
lists a deleted thread is the local copy being confidently wrong. The same
write-through applies to renames, because modules are only re-read on the slow
structure cadence.

Destructive operations require the object's exact current name as confirmation.

### Re-homing

`move_thread` re-homes a thread into another bubble — module membership in Plane,
plus whatever the overlay keys by bubble.

### The board

A bubble that has gone quiet while people are still commenting on it reads 😴 rather
than 🪦 — the thread-grain pulse, read one level up (`bubblePulse`). The floor moves
the band only: the lifecycle, the score and therefore the ordering are untouched
([`decisions/0004`](../decisions/0004-writing-is-the-work.md)).

The board is built from modules, bucketed by level, ordered by buoyancy score
within a band. Filters are remembered across reloads. Threads carry their own level
mark, and a bubble's badge counts the **in-progress** threads rather than all of
them: the number that matters is how much is burning, not how much exists.

## Surfaces

| Capability | REST | CLI | MCP |
|---|---|---|---|
| list workspaces | `GET /api/workspaces` | `bubble workspace list` | `list_workspaces` |
| create / rename workspace | `POST /api/workspaces` · `PATCH /api/workspaces/{id}` | `bubble workspace new`, `rename` | `create_workspace` · `rename_workspace` |
| list bubbles | `GET /api/bubbles` | `bubble ls` | `list_bubbles` |
| create bubble | `POST /api/bubbles` | `bubble bubble new` | `create_bubble` |
| contract | `POST /api/bubbles/{id}/contract` | `bubble bubble set` | `set_contract` |
| rename bubble | `PATCH /api/bubbles/{id}` | `bubble bubble rename` | `rename_bubble` |
| close / reopen | `POST /api/bubbles/{id}/close` · `/reopen` | `bubble bubble close`, `open` | `close_bubble` |
| review stage | `POST /api/bubbles/{id}/review` · `/unreview` | `bubble bubble review`, `unreview` | — |
| delete | `DELETE /api/bubbles/{id}` · `/api/workspaces/{id}` · `/api/threads/{id}` | `bubble delete <kind> <id>` | `delete_bubble` · `delete_workspace` · `delete_thread` |

## Storage

`bubble_contracts`, `bubble_state` (closed / stage), `bubble_snapshots` (overlay).

## Invariants

1. Every bubble carries outcome, owner and closure condition, or it is flagged as
   contract-less.
2. Closing preserves the record; deleting is a separate, confirmed act.
3. Deleting a bubble never deletes work.
4. Creates, renames and deletes write through to the mirror in the same request.

## Open

- The review stage has no MCP tool, on purpose (a human decides) — worth revisiting
  if agents start reviewing each other's work.
- Band collapse state in the web is not persisted across reloads.
