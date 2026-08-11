# Pages when Plane cannot hold them

Status: **shipped**. Pages are a per-instance capability. Where the instance's
Plane serves them, Plane is the record; where it does not, this server is.

## The problem

[`bubble-work-spec.md`](./bubble-work-spec.md) §2.5 gives a Workspace a place for
the documentation that outlives every thread in it — the spec, the API contract,
the decision record. It maps that to a Plane **project page**, and the server
reads and writes it over Plane's public REST API like everything else.

On a Plane Community deployment there is no such endpoint.

```
GET /api/v1/workspaces/{ws}/projects/{id}/pages/   → 404
GET /api/v1/workspaces/{ws}/projects/{id}/modules/ → 200
GET     /api/workspaces/{ws}/projects/{id}/pages/  → 401 (route exists, wants a session)
```

Pages live in Plane's **internal** app API (`apps/api/plane/app/urls/page.py`),
which authenticates with a session cookie. The **public** API — the one an API key
reaches, `apps/api/plane/api/urls/` — has no page module at v1.4.1, the newest
release at the time of writing. `developers.plane.so` documents the endpoint, with
OAuth scopes like `projects.pages:read`; that is Plane Cloud's surface, and the
code was written against those docs.

**No upgrade fixes this.** That matters because the first version of the error
message said the opposite.

## Why the 404 could not be read directly

Plane answers **any** unrouted URL with 404 and this body:

```json
{"error": "Page not found."}
```

"Page" as in web page. A missing route and a missing Page object are therefore
indistinguishable by status code, and the wording actively suggests the wrong
reading. The original `explainPageFailure` called `GetProject`, saw `page_view:
true`, and concluded the endpoint "arrived in a later Plane release than the one
this instance runs" — a sentence assembled from a 404 that carried no such
information, pointing at an upgrade that would change nothing.

`Client.PagesAPIAvailable` settles it with a **control request** to a sibling path
that cannot possibly be routed:

```
GET .../pages/                     → 404 {"error": "Page not found."}
GET .../pages-capability-probe/    → 404 {"error": "Page not found."}   identical → no route
```

Identical answers mean the URL resolver matched neither, so the route is absent. A
**different** answer means a real handler produced the first 404 — the route
exists and the problem is this project, which is where the "pages are switched off
for X" message belongs. Two requests, once per instance.

## Where a page lives

| | Plane serves pages | Plane does not |
|---|---|---|
| Record | Plane project page | `local_pages` in the overlay |
| Page id | `slug:project:<uuid>` | `slug:project:loc_<random>` |
| `storage` in the DTO | `plane` | `local` |
| Visible in Plane's UI | yes | **no** |

The id announces its backend, so every read knows which system to ask before it
asks anything — including a read for an id that has since been deleted. Nothing
has to probe, and nothing has to guess.

Locally-held pages are **always** listed, even on an instance whose Plane does
serve pages. A page written while it could not must not disappear the day it is
upgraded, so both kinds coexist in one list and no migration is needed.

### They form one tree

Pages nest. Plane models that with `parent_id` and draws it as a tree in its own
UI, so the hierarchy is **read from Plane** rather than invented here, and a
locally-held page carries the same field — the two kinds are one tree, not two
lists side by side.

Two ways that goes quietly wrong, both closed:

- A **parent in another workspace**, or one that does not exist, is refused. A
  page whose parent no tree can find is present in the database and absent from
  the only UI that can reach it.
- **Deleting a parent promotes its children** onto whatever it sat under, rather
  than leaving them pointing at an id that is gone. The web tree also treats any
  unreachable parent as a root, which covers the case we do not control: a page
  deleted in Plane's own UI.

Re-parenting an existing page is not implemented — see the follow-ups.

### The overlay, not the mirror

`local_pages` belongs with the outbox, and deliberately not in the mirror. The
mirror is a projection Plane can rebuild — `mirror.Reset` and `sync-backfill` drop
its tables on purpose. These rows are the only copy in existence, so losing them
loses real work. See [`PLANE-SYNC.md`](./PLANE-SYNC.md) Phase 5 for the same
argument about pending writes.

They are deleted with the workspace they document, and only then: nothing else
will ever ask for them, and leaving them would leave the sole copy of a document
belonging to a workspace that no longer exists.

## The capability verdict

Cached in `instance_capabilities`, because it is a property of the deployment
rather than of a request, and probing costs requests against a 60 req/min budget.

- **Supported** is permanent. An API surface does not disappear.
- **Unsupported** is re-probed after 12 hours, so upgrading Plane or moving
  edition is noticed without restarting the server.
- An **inconclusive** probe — Plane unreachable, credential rejected — is reported
  as "Plane holds pages", so the normal Plane path runs and produces its own real
  error. Guessing "local" there would be worse than any error: it would quietly
  start a second, divergent copy of a workspace's documentation because Plane
  happened to be down.

## What each surface says

A reader has to be told, or they will look for a spec in Plane's UI, not find it,
and conclude the write failed.

- **REST** — `GET /api/workspaces/{id}/pages` returns `{pages, plane_holds_pages}`.
  An envelope rather than a bare array: "there are no pages" and "this Plane
  cannot store pages" look identical in a list and call for opposite reactions.
  It is also the shape MCP needs, since a tool returning a bare array announces
  `"type": "array"` and clients reject the whole tool list over it.
- **Web** — the documents screen says *kept in Bubble Work* in its topbar, with
  the full explanation on hover. Where Plane does hold pages, a local one is
  instead marked per row, because there it is the exception.
- **CLI** — `bubble page list` prints the same sentence before the list, and tags
  individual local pages only when Plane holds the rest.
- **MCP** — `list_pages` returns `plane_holds_pages`, and its description says
  what false means: these exist here and not in Plane's own UI.

## The exception this makes

`AGENTS.md` says Plane is the system of record. This is a deliberate, narrow
departure from that, and worth naming as one.

It is narrow because the alternative is not "Plane holds it" — it is "nobody
does". A workspace with no home for its standing documentation is worse than one
whose documentation lives in the server that reads it. Everything else the spec
says about pages still holds: they belong to the Workspace, they outlive bubbles
and threads, and **they earn no heat**, because writing documentation is not
producing the outcome a bubble is contracted to reach.

## Known follow-ups

- **Nothing re-parents an existing page.** A page is created where it is created:
  at the root, or inside the one you asked for. Moving one afterwards needs a verb
  on all three surfaces and a write of `parent_id` to Plane, and inventing it
  before anybody has asked is guessing.
- **Nothing migrates a local page into Plane** if an instance gains the pages API
  later. The pages keep working where they are; moving them would need a
  deliberate command, and inventing one before anybody wants it is guessing.
- **A locally-held page cannot be locked or archived** from any surface. The
  columns exist and are honoured on read, so the lock is enforced if something
  sets it — there is just no way to ask yet. Plane's own UI is where those verbs
  come from, and on these instances there is no Plane page to use them on.
