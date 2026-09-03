# Federation — several Plane deployments at once

**Status:** built · **Packages:** `internal/server` (instance registry, `collect`),
`internal/store` (`plane_instances`, `instance_capabilities`) ·
**Depends on:** [`identity.md`](./identity.md),
[`plane-channel.md`](./plane-channel.md)

## What it solves

One person works across separate orgs — `plane.ayetec.space`, `plane.cuby.work` —
and wants one board, without either org being able to see the other's work.

## Model (normative)

An **instance** is a named connection to a Plane workspace, with its API key stored
server-side and never printed back. It either **pins one project** or, when no
project is set, **federates the whole workspace**, auto-discovering every project in
it.

### Scope follows Plane membership

A caller sees an instance only if that Plane workspace's member list contains their
email. That is the isolation boundary between orgs, and it is Plane's own answer
rather than a grant table here — a `cuby` member never sees `ayetec` bubbles.

The unified cross-instance view therefore requires the **same email** in each
workspace. Different emails per org are different identities, and the CLI's
credential profiles are how you switch between them
([`operations.md`](../operations.md)).

### Namespaced ids

| Object | Id |
|---|---|
| Workspace | `<slug>:<project-id>` |
| Bubble | `<slug>:<project-id>:<module-id>` |
| Thread / Page | `<slug>:<project-id>:<item-id>` |
| Local page | `<slug>:<project-id>:loc_<random>` |

So every operation routes to the right instance and project automatically. The CLI
accepts any unique prefix.

### Fan-out

`collect()` fans a read out over the caller's authorized instances. **One
unreachable instance is logged and skipped**, never blanking the whole view — a
board that shows three of four orgs is useful; a blank board is not.

### Capabilities are per instance

What one Plane can hold, another cannot — pages being the worked example
([`pages.md`](./pages.md)). Capability verdicts are cached per instance, so two
instances of the same server can legitimately behave differently, and each surface
says which one it is describing.

Autostate is per instance too, and off until an operator turns it on
([`threads.md`](./threads.md)).

## Surfaces

Server-host admin only: `bubble instance add|list|remove [--force]`, plus
`GET /api/admin/instances` and the God Mode panel. Membership and roles are read
from Plane, never managed here.

## Storage

`plane_instances` (overlay — URL, workspace, optional project, credentials),
`instance_capabilities` (overlay).

## Invariants

1. API keys never leave the server and never appear in `instance list`.
2. Scope is derived from Plane membership, not stored as grants.
3. One unreachable instance degrades the view, never empties it.
4. Ids are namespaced, so no operation can act on the wrong instance.

## Open

- A first request against a cold server can answer "not authorized for this
  instance" when it means "membership is not mirrored yet"; it should be a 503
  ([`threads.md`](./threads.md)).
