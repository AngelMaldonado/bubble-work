# The Plane channel — one reader, one writer, a rate budget

**Status:** built (Phases 0–5, 7; Phase 6 optional; documents publish and import) · **Packages:** `internal/plane`, `internal/sync`,
`internal/mirror`, `internal/server/outbox.go`, `internal/store/outbox.go` ·
**Depends on:** [`storage.md`](./storage.md) · **History:**
[`journal/PLANE-SYNC.md`](../journal/PLANE-SYNC.md)

## What it solves

Plane allows **60 requests per minute per key**. A board that fetched live would
spend a whole minute's budget on one page load. Everything in this module follows
from that number, plus one rule: the server is Plane's **only** client, over REST
only, so the policy engine cannot be bypassed.

## Model (normative)

### One reader

A single sync worker is the only thing that reads Plane:

| Pass | Cadence | What it does |
|---|---|---|
| delta | short interval | `order_by=-updated_at`, early-stop at the watermark |
| reconcile | hourly | a complete walk: catches deletes and module membership |
| structure | ~10 min | projects, modules, cycles, states, members |

A **partial pass holds the watermark back** rather than advancing past what it did
not see. *"I could not see it" must never be mistaken for "it is gone."*

### The rate budget

`internal/plane/budget.go` parses Plane's own `X-RateLimit-*` headers and waits
until the stated reset instead of guessing, and reserves a lane so background sync
can never starve a human request. `bubble admin stats` reports the remaining
allowance.

**Never add a Plane call to a read path.** That is the one rule this module exists
to protect.

**Per-item streams are budgeted.** Comments, links and relations have no bulk
endpoint, so each costs a call PER WORK ITEM — the single biggest cost lever in the
sync. Each has a cap for the WHOLE pass, shared across projects: `commentFetchPerPass`
(25 items) and `edgeFetchPerPass` (8 items, two calls each). Neither can hold back a
project's cursor: they are fill-ins, and a project that walked cleanly is recorded as
clean even if its edges did not fit this pass. Labels are the exception and cost
nothing extra — they ride along in the work-item payload the delta already reads
([`decisions/0006`](../decisions/0006-plane-holds-the-relationships.md)).

### Writes: the outbox

A write lands in SQLite and returns immediately. The drainer sends it to Plane with
exponential backoff and abandons after a bounded number of attempts.

- A field with an undrained entry is **shielded** by a field lock: an incoming sync
  will not overwrite it. When the entry drains or is abandoned, Plane is truth
  again for that field.
- An abandoned write stays **visible** — a lost write that nobody is told about is
  the failure mode this design exists to avoid.
- Comment drafts deliberately carry **no credential**, so a failed post is retried
  by the person who wrote it rather than replayed by the server on their behalf.
- Labels, links and relations are written straight through to Plane AND to the
  mirror, so the answer a caller gets already reflects the change. They are not
  queued: unlike a state write, there is no sync pass that would revert them.

### Capabilities are per instance

Plane deployments differ in what their public API exposes, and a 404 does not say
which kind of 404 it is: any unrouted URL returns `{"error": "Page not found."}` —
"Page" as in web page. So a capability is established by **probe plus control
request**, cached (`instance_capabilities`), and re-checked on a long interval. The
worked example is pages: [`pages.md`](./pages.md).

### Degraded mode

If sync stops, reads keep succeeding from local data that is quietly getting older
and nothing errors. So the server reports its own staleness: `GET /api/status`
carries mirror age and the caller's unsent drafts, rendered as a banner in the web
UI and a warning line above `bubble ls`.

> Being behind is fine; being behind silently is not.

### Webhooks

`POST /webhooks/plane/{slug}` is accepted as a **freshness hint**. The delta is
what guarantees correctness; an event only makes the mirror fresher, sooner.
Payload-aware handling (Phase 6) would make it cheaper, not more correct.

### Documents flow both ways, with a rule

Bodies are published **outward**: the outbox renders markdown to Plane's node
vocabulary and records the hash it sent (`thread_publish`). And Plane stays a
writable surface, so bodies also come **inward** when somebody edits a work item
there.

The syncer reports what Plane is serving (`OnBodies`) and decides nothing: it does
not know what was published, what is queued, or what the overlay holds. The server
owns all three, so `importPlaneEdits` makes the call:

| Situation | Winner |
|---|---|
| hash differs from `published_hash`, nothing queued | **Plane** — import it, keep the replaced version |
| hash differs, our publication queued (`description` lock) | **local** — the shield holds, ours lands |
| hash equals `published_hash` | nobody edited anything; this is our own publish |

That last row is the whole loop guard. Publishing bumps Plane's `updated_at`, so
"this row changed" is true after every publication — comparing against the hash we
*sent* is what stops the two directions feeding each other. A thread we have never
published has no opinion recorded and is left alone.

An import is **not** evidence in itself: it flows through the ordinary progress
fingerprint, so editing a plan in Plane warms the thread exactly as editing it here
does, and editing prose warms nothing.

## Surfaces

Admin only. `bubble admin sync <inst>` (census + cursor, no Plane calls),
`sync-diff`, `sync-fidelity`, `sync-backfill`, `sync-rebuild`, `outbox [drop <id>]`
— plus the same panels in God Mode. `GET /api/status` is public to any member.

## Storage

The mirror (`mirror_*` tables) and the outbox. Both share the store's **single
file and connection pool** — two pools over one SQLite file is a lock fight —
[`storage.md`](./storage.md).

## Invariants

1. The server is Plane's only client, and speaks REST only.
2. No read path calls Plane.
3. One worker reads; nothing else does.
4. A partial pass never advances the watermark.
5. Pruning overlay rows against the live set happens only after a **complete** walk.
6. A field with an undrained write is not overwritten by sync.
7. A failed write is visible, never silently dropped.

## Open

- Payload-aware webhooks (freshness, not correctness).
- `OnBodies` hands the server every body a pass saw, which is fine at current sizes
  and is a map of whole documents in memory at large ones.
