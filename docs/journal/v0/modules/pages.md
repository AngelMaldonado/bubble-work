# Pages — a Workspace's reference material

**Status:** built · **Packages:** `internal/server/pages.go`,
`internal/store` (`local_pages`, `instance_capabilities`),
`frontend/src/components/PagesView.svelte` · **Depends on:**
[`plane-channel.md`](./plane-channel.md), [`artifacts.md`](./artifacts.md) ·
**History:** [`journal/PAGES-CAPABILITY.md`](../journal/PAGES-CAPABILITY.md)

## What it solves

A Workspace needs somewhere to keep what outlives the work: a spec, a reference, an
architecture decision. Not a thread's Brief — that belongs to one piece of work —
but the thing every thread has to stay consistent with.

## Model (normative)

A Page belongs to the **Workspace**, never to a bubble or a thread. Pages form a
**tree**: a page may have a parent, and the parent must live in the same workspace.

Writing a Page is **not** evidence of production, so pages earn no heat.
Documenting what you intend is not the same as changing reality.

### Two possible holders

| Storage | When | Id |
|---|---|---|
| `plane` | the instance's Plane exposes project pages on its public API | Plane's own id |
| `local` | it does not | prefixed `loc_` |

Plane Community serves pages only on its **internal, session-authenticated** API
(`apps/api/plane/app/`), not the public API-key one (`apps/api/plane/api/`). No
upgrade changes that — it is an edition boundary, not a version. On those
deployments **the server is the record**, which is a deliberate departure from
"Plane owns the work": the alternative is not "Plane holds it" but "nobody does".

### Establishing the capability

A 404 from Plane cannot be read directly — any unrouted URL returns
`{"error": "Page not found."}`, so a missing route and a missing object are
indistinguishable by status. The capability is therefore established by probe **plus
a control request** to a URL that certainly does not exist: identical status *and
identical body* means the route is absent, not the object. The verdict is cached in
`instance_capabilities` and re-checked on a long interval (~12h).

### Local pages

`local_pages` lives in the **overlay**, beside the outbox, because it is the only
copy in existence — not a projection of anything. Consequences:

- Deleting a workspace forgets its local pages.
- Deleting a page **promotes its children** rather than orphaning them.
- A cross-workspace parent is refused.
- These rows are never pruned by a Plane walk ([`storage.md`](./storage.md)).

### What each surface says

Every surface states which holder it is talking to, because a reader deserves to
know whether the page they are reading also exists in Plane. The UI shows the tree
regardless of holder, and says plainly when this instance's Plane cannot hold pages.

## Surfaces

| Capability | REST | CLI | MCP |
|---|---|---|---|
| list | `GET /api/workspaces/{id}/pages` | `bubble page list` | `list_pages` |
| read | `GET /api/pages/{id}` | `bubble page read` | `read_page` |
| create | `POST /api/workspaces/{id}/pages` | `bubble page new` | `create_page` |
| update | `PATCH /api/pages/{id}` | `bubble page edit` | `update_page` |
| delete | `DELETE /api/pages/{id}` | `bubble delete page` | `delete_page` |

Web: a file-tree reading room in the workspace, using the same prose renderer as a
thread's artifacts — one renderer, so a page and a Brief cannot look different.
Open folders are remembered per workspace.

## Storage

`local_pages` (overlay, with a `parent` column), `instance_capabilities` (overlay).
Plane-held pages are read through the mirror.

## Invariants

1. A page belongs to a Workspace and outlives every thread in it.
2. Pages never earn heat.
3. A parent must be in the same workspace.
4. Deleting a parent promotes its children.
5. The capability verdict is never inferred from a bare 404.

## Open

- Re-parenting an existing page.
- Migrating a local page into Plane when an instance gains the capability.
- Lock / archive verbs for local pages.
- Pages are not currently exported by anything, which matters more now that they
  can be the only copy ([`storage.md`](./storage.md)).
