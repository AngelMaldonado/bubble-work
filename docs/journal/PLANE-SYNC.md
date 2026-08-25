# Plane Sync — SQLite as L1, one worker as the only Plane client

Status: **Phases 0-5 and 7 shipped · Phase 6 (payload-aware webhooks) is the
only one left, and it is an optimization rather than a correctness need** ·
Drafted 2026-08-04 · Companion to
[`AGENTS.md`](../../AGENTS.md), [the model](../../README.md) and
[`THREAD-LIFECYCLE.md`](./THREAD-LIFECYCLE.md).

This is a **worksheet**: every phase carries checkboxes and an acceptance test.
Tick them as they land and update the Status line, the way `THREAD-LIFECYCLE.md`
was kept current.

> **Journal.** This is the record of how the work went — the problem as it was
> measured, the phases, what the live instance changed. It is kept as written and
> is not updated when the system moves on. For what is true today, read [`modules/plane-channel.md`](../modules/plane-channel.md) and [`modules/storage.md`](../modules/storage.md).

## Why

Plane stays first-class — it remains the system of record and the place humans
edit work. What has to go is the **request-path dependency**: today every board
read, every thread open and every cold auth is a live Plane round-trip, and
Plane allows **60 requests/minute per API key**. We are sitting on that ceiling,
so the board is slow and refreshes arrive late.

Where the budget goes today, per instance, per 90 s tick (`fetchInstance` +
`progressEvidence` + `probePulse`):

```
  1                       ListProjects (unpinned instances)
+ P × 1                   ListModules
+ P × ceil(items/100)     ListProjectItems      (progress diff)
+ M × ceil(items/100)     ListModuleWorkItems   (the board itself)
+ ≤ 20                    ListComments          (pulse probe, pulseProbeLimit)
+ amortized               Members (10 min TTL), ListStates (30 min TTL)
```

For one project with 8 modules and ~200 items that is **~32 calls per tick ≈
21/min sustained** — before a single human touches the UI. On top of that:

- a **cold auth costs up to 2N calls** (`Me` on each instance until one accepts,
  then `Members` on *every* instance to scope the actor) — cached only 5 min,
  per credential (`server.go:393`);
- **opening one thread** costs ~4 (`GetWorkItem`, `ListChildren`, `ListStates`,
  `ListComments`);
- the **pulse probe alone is up to 20 of the 32** — and it exists only to answer
  "has this dying thread been commented on", which a mirror answers with a
  `SELECT`.

The refresher and the humans share one key's 60/min. Whoever loses, waits.

## What Plane actually gives us

Three facts from the API docs that the current client ignores, and that the
design leans on:

1. **`X-RateLimit-Remaining` / `X-RateLimit-Reset` are on every response.** We
   currently discover the limit by getting a 429 and backing off blindly
   (`client.go:79`). We can instead *budget*: know the remaining allowance at all
   times and never spend the last slice, reserving it for interactive work.
2. **`order_by=-updated_at` is supported on the work-item list** (there is no
   `updated_at__gte` filter, but descending order plus early-stop is equivalent).
   That makes an incremental delta sync possible: page until the first item older
   than our watermark, then stop. Steady state becomes **one page**, not a full
   re-poll.
3. **`fields=` and `expand=` control the payload.** Slimmer rows per page, and
   `expand` can pull related objects that would otherwise be a second call.

Self-hosted note: the limit is the `API_KEY_RATE_LIMIT`-style env var on your own
Plane. Raising it is a legitimate stopgap, but it is not the fix — the fix is not
needing the calls.

## Probed against the live instance (2026-08-04)

Run against `cuby` / `plane.cuby.work` / workspace `cuby-smart`. Scripts are
throwaway; the findings are not.

| Question | Answer |
|---|---|
| Does the work-item **list** carry full `description_html`? | **Yes, and untruncated.** A 14,221-char body came back byte-identical from list and from `GET`, across all 7 projects. |
| `order_by=-updated_at`? | **Honored.** Verified descending. |
| `fields=`? | **Honored.** `fields=id,name,updated_at` returned exactly those three keys. |
| `expand=`? | **Honored.** `expand=state` returns the state object inline (`{id,name,color,group}`) instead of a bare UUID. |
| `X-RateLimit-Remaining` / `-Reset`? | **Present on every response.** |
| Project- or workspace-wide **comment feed**? | **No.** All four candidate paths 404. |
| Does an item's `updated_at` move when it is **commented** on? | **Yes** — and on comment *delete* too. Verified by post → measure → delete. |

Two things the probe demonstrated by accident, both worth keeping:

- **The rate limit is real and tight.** Reading one page from each of 7 projects
  plus a handful of `GET`s took the budget from 60 to **16 remaining**, and a
  follow-up call was **429'd outright**. A dozen exploratory reads is enough to
  exhaust a minute. This is the whole justification for Phase 0.
- **`total_results` is not trustworthy.** Six of seven projects reported
  `total_results: 0` on a `per_page=1` request while a `per_page=100` request on
  the same project returned real items (including the 14 k body). Backfill sizing
  and progress reporting must **count what actually arrives**, never trust that
  field.

### What the answers change

- **`GetWorkItem` per item disappears.** Briefs and Logbooks arrive 100 at a time
  in the same page that builds the board. This was the plan's largest cost risk
  and it is gone — Phase 1's backfill is roughly `ceil(items/100)` calls per
  project, and steady-state delta is one page.
- **`expand=state` removes the `ListStates` join** from the hot path; states are
  still mirrored, but a stale state cache can no longer mislabel a thread.
- **Comment sync is targeted, not blind.** There is no feed, but a comment
  **bumps its parent item's `updated_at`** — so a commented item surfaces in the
  ordinary delta page. Fetch comments only for items the delta actually named.
  That is bounded by real change, not by table size, and it means **Phase 6
  (webhooks) is an optimization again rather than a dependency of Phase 3**. A
  webhook-less deployment is still correct, just one delta interval behind.

### How the syncer tells a comment from an edit

`updated_at` moving says *something* changed, not what. The mirror already stores
`description_hash`, which separates the cases without a second call:

```
item appears in delta page
├─ description_hash changed  → body edit (Brief/Logbook) → run the progress diff
├─ state_id changed          → state move
└─ neither, but updated_at moved
                             → comment / assignee / label churn
                             → fetch comments for THIS item only
```

Comment **deletes** bump `updated_at` too, so a removed comment is self-healing:
the item resurfaces in the delta and the re-fetch finds it gone.

> **Guardrail — `updated_at` is a sync hint, never evidence.** It moves on
> comments, labels and cosmetic edits, which is exactly what `AGENTS.md` says
> must *not* generate heat. The watermark drives *what to re-read*; heat stays
> derived from the Logbook fingerprint, revisions and `completed_at`, and
> comments stay pulse-only per `THREAD-LIFECYCLE.md`. Wiring the delta watermark
> into `EvidenceEvent` would silently turn motion into heat — don't.

## Core idea

**SQLite becomes the read plane. One worker becomes the only thing that talks to
Plane.**

```
  browser / CLI / MCP
        │  (never blocks on Plane)
        ▼
   ┌─────────────┐        reads: 100% sqlite
   │  L1: sqlite │◄──────────────────────────┐
   └─────────────┘                           │
        │ writes                             │ apply
        ▼                                    │
   ┌─────────────┐   drain (rate-budgeted)   │
   │   outbox    │──────────────┐            │
   └─────────────┘              ▼            │
                          ┌───────────────────────┐
                          │  sync worker (only    │
                          │  Plane client alive)  │
                          └───────────────────────┘
                                 ▲        │
                        webhooks │        │ delta poll
                                 └────────┴──── Plane
```

Heat, buoyancy and the Bubble contract do not change at all. They stop being
computed *during a fetch* and start being computed *over the mirror*.

### Decisions locked

| Question | Decision |
|---|---|
| **Write path** | **Optimistic + outbox.** A write lands in L1 and returns; the worker drains it to Plane with retry. The UI shows a pending marker. |
| **Conflict** | **Plane wins, pending writes shielded.** A field with an undrained outbox entry is not overwritten by an incoming sync. When the entry drains or is abandoned, Plane is truth again. |
| **Mirror scope** | **Everything** — projects, modules, states, members, work items, bodies, comments. Plane down ⇒ read-only, not broken. |
| **Blast radius** | **Strangler + targeted cleanup.** New `internal/mirror` and `internal/sync` behind the shapes `server.go` already uses; only the refactors the sync work forces. Every phase leaves `main` green. |

This keeps `AGENTS.md` honest: Plane is still the system of record, and the
server is still Plane's only client. The mirror is a **rebuildable projection** —
deleting it must cost a backfill, never data.

## The shape

```
internal/plane    HTTP client. Gains a rate BUDGET (headers, not guesswork)
                  and a reserved lane for interactive calls.
internal/mirror   Read side. Typed queries over the mirror tables. The only
                  thing server.go reads from. No HTTP anywhere in this package.
internal/sync     Write side + worker. Backfill, delta poll, webhook apply,
                  outbox drain, conflict shield. The only importer of
                  internal/plane once this lands.
internal/server   Handlers, heat, contract, rendering. Loses fetchInstance,
                  refreshInstance, the five caches, and probePulse.
```

Rule to hold the line: **after Phase 5, `internal/server` must not import
`internal/plane`.** That is a one-line grep in CI and it is what stops the old
pattern growing back.

### Schema sketch

Mirror tables, all keyed by `(instance, id)` so one DB holds every instance:

```sql
mirror_projects (instance, id, name, identifier, updated_at, synced_at)
mirror_modules  (instance, id, project_id, name, updated_at, synced_at)
mirror_module_items (instance, module_id, item_id)      -- membership edges
mirror_states   (instance, id, project_id, name, "group", is_default)
mirror_members  (instance, id, email, display_name, role)
mirror_items    (instance, id, project_id, seq, name, state_id, priority,
                 parent_id, assignees_json, created_at, updated_at,
                 completed_at, description_html, description_hash, synced_at)
mirror_comments (instance, id, item_id, actor_id, comment_html,
                 created_at, updated_at)
sync_cursors    (instance, resource, watermark, last_full_at, last_ok_at,
                 last_error)
outbox          (id, instance, kind, target_id, payload_json, field_lock,
                 status, attempts, next_attempt_at, created_at, last_error)
```

`field_lock` is the conflict shield: `"state"` on a pending state write means an
incoming sync applies every column of that row **except** `state_id`.
`description_hash` lets the progress diff (`progress.go`) run against L1 without
re-reading bodies.

Existing overlay tables (`bubble_contracts`, `thread_progress`, `thread_pulse`,
`thread_autostate`, …) are untouched — they are the overlay, not the mirror, and
the `mirror_` prefix keeps that boundary visible in the schema itself.

---

## Phase 0 — Rate discipline · **SHIPPED**

*No architecture change. Ships in a day and makes today measurably better, which
also gives us a baseline to prove the rest against.*

- [x] Parse `X-RateLimit-Remaining` / `X-RateLimit-Reset` on every response into
      a per-key budget in `internal/plane` (`budget.go`). The registry is keyed by
      base URL + key, **not** held on a `Client` — `plane.New(...)` is called
      ad-hoc at ~10 call sites, so a per-client budget would have seen nothing.
- [x] **Reserve a lane**: `backgroundFloor = 15`. The lane rides on the context
      (`plane.Background(ctx)`), so none of the ~25 client methods changed
      signature; `RunRefresher`, `RunTicker` and the webhook refresh mark theirs
      once at the entry point. Admin "refresh now" stays interactive on purpose —
      a person asked for it and is watching.
- [x] Replace the blind 0.4/0.8/1.6 s backoff with "wait until reset" when the
      budget is known (`backoffFor`). Retry-After still wins when Plane sends it.
- [x] Add `fields=` to the two hot list calls. Measured on a 6-item module:
      **1732 → 736 bytes, values identical**. `ListProjectItems` also stops
      pulling `description_binary`, a second copy of every body that nothing
      reads.
- [x] Expose budget state on `GET /api/admin/stats` → `bubble admin stats` and a
      **Plane rate budget** card in God Mode (surface parity; admin capability,
      so no MCP — matching `autostate` and `tuning`).
- [x] Document the `API_KEY_RATE_LIMIT` stopgap in `DEPLOY.md`.
- [x] **Put SQLite in WAL mode.** `store.Open` did a bare
      `sql.Open("sqlite", path)` — no journal mode, no busy timeout, no pool
      limits. Fine while writes were rare and tiny; it would **not** have
      survived a sync worker bulk-upserting while the board read the same file.
      Now `journal_mode(WAL)` + `busy_timeout(5000)` + `synchronous(NORMAL)`,
      with `Open` **verifying** the mode took (a file that silently stayed in
      rollback-journal mode is exactly the failure this prevents).

**Accepted:** `TestWALConcurrentReadWrite` was confirmed to *fail* without WAL —
`database is locked (5) (SQLITE_BUSY)` — and pass with it, so it is a real
regression guard rather than a tautology. `TestBackgroundYieldsInteractiveDoesNot`
pins the reservation: at the floor the background call is refused and the
interactive one goes through. Full suite green; `svelte-check` 0/0.

**Cleanup carried:** the retry/backoff block in `client.go` became `backoffFor` +
`sleepCtx`, shared by `get` and `send`. Writes are budgeted now too — they draw
on the same per-key allowance, so an unbudgeted auto-state writer could otherwise
drain what a page load needed.

**Deliberately not done:** `send` still has no retry. A failed write is the
outbox's job (Phase 5), not a silent repeat.

## Phase 1 — The mirror, in shadow

*Write the mirror, read nothing from it. Zero user-visible risk.*

- [x] ~~Spike: does the work-item list carry full `description_html`?~~
      **Answered 2026-08-04: yes, untruncated** (14 k body identical list vs get).
      Per-item `GetWorkItem` is off the critical path; bodies arrive 100/page.
      See *Probed against the live instance* above.
- [x] `internal/mirror`: schema + upsert/query API. The mirror borrows the
      store's `*sql.DB` rather than opening its own — two pools over one SQLite
      file would reintroduce exactly the contention WAL was added to remove.
- [x] `internal/sync`: `Backfill(instance)` — full walk of projects, modules,
      module membership, states, members, items. Uses `expand=state` so a row
      carries its state group inline, and **counts arriving rows** rather than
      trusting `total_results`.
- [x] `internal/sync`: `Delta(instance)` — `order_by=-updated_at`, early-stop at
      the watermark. Verified at **≤ 3 calls** for a quiet pass.
- [x] Worker loop: delta every 2 min, full reconcile hourly. Runs in the Phase 0
      background lane, so shadow-period double-running does not degrade the board.
- [x] `bubble admin sync-diff <slug>`, plus `sync` (census, no Plane calls) and
      `sync-backfill`. REST + CLI (admin capability → no MCP).

**Accepted:** the substrate comparison is clean after a backfill and catches both
a corrupted field and a module-membership change (the drift a delta structurally
cannot see). `TestBackfillThenDeltaIsCheap` asserts the economic claim numerically
rather than trusting it. **Verified against live Plane on 2026-08-05: 0 findings
across 7 projects / 22 modules / 118 items** — see the gate section above.

### What sync-diff compares, and why

It compares the **substrate** — module list, module membership, and the item
fields the board and interior read — not the rendered board. The board is a pure
function of that substrate, so equal substrate means an equal board by
construction, and a mismatch points straight at the sync bug instead of at a
downstream symptom. Diffing rendered bubbles would conflate "the mirror is wrong"
with "the board builder changed".

### Three things the first live run changed

Running this against `plane.cuby.work` for real found what the unit tests could
not, because the unit tests had a rate limit of infinity:

1. **A 429 on one sub-fetch aborted the entire pass.** Now a project that fails
   is logged, marked partial, and skipped — what arrived is kept. The invariant
   that makes this safe: **a project that did not walk cleanly is never pruned,
   and a partial pass never advances the watermark.** "I could not see it" must
   never be mistaken for "it is gone".
2. **A full walk ran in the interactive lane.** It is tempting to give an
   explicitly-requested backfill priority, but a walk that spends the whole
   minute's allowance is precisely what starves the board — and unlike a page
   load, it can afford to wait. All sync passes, including admin-triggered ones,
   are background now.
3. **Losing the project list aborted everything** — the worst possible response,
   since the mirror already knows the projects. It now falls back to the
   mirrored list and continues degraded; a genuinely new project waits for a
   pass that can read the list.

A fourth thing was fixed on the way: **the full walk is now resumable.** A pass
that ran out of budget used to restart from project one next time, and since a
partial pass cannot advance the watermark, it could never finish — self-
reinforcing under sustained pressure. Each project now carries its own cursor.
Note the deliberate asymmetry: the *automatic* hourly reconcile resumes (skips
projects still fresh), while an *explicit* `sync-backfill` always re-walks
everything — an admin asking for a backfill means "go and look".

Also learned, and worth remembering: the rate limit is **per key, shared across
processes**. Three consumers on one key (a scratch server, the live server, and a
few probe scripts) is enough that even `list projects` 429s. `X-RateLimit-Limit`
is not sent, so the inferred ceiling is "largest remaining ever seen" — a lower
bound that corrects upward, which is why it can read oddly low right after a
busy period.

### The gate: PASSED on live Plane (2026-08-05)

```
sync-diff cuby — compared 7 projects, 22 modules, 118 items in 180.1s
  ✓ mirror matches Plane          findings: 0
```

The mirror reached a complete full pass **six minutes after deploy** and then
matched Plane exactly on every compared field. Phase 2 is unblocked.

Two things this run settled that the earlier, noisier attempt could not:

- **The shadow-period contention is survivable.** The legacy fetch path and the
  syncer do share one key's 60/min, and the syncer does get the smaller share —
  but it converges rather than starving. The earlier failure to converge was an
  artifact of a scratch server running a *second* full legacy stack against the
  same key.
- **Phase 0 is doing exactly what it was built for.** Across the run:
  `429s = 0, bg_yields = 9`. Every rate-limit collision that would previously
  have been a failed fetch became a controlled wait instead. The inferred ceiling
  settled at 59, essentially the true 60.

The cost of `sync-diff` itself is worth noting: **180 s and a full Plane walk.**
It is a shadow-period verification tool, not something to leave running.

## Phase 2 — Board reads from L1 · **SHIPPED**

- [x] `buildInstance` / `buildBubbleFromMirror` (`internal/server/board.go`)
      replace `fetchInstance` / `buildBubble`. Heat, buoyancy and the contract
      are untouched — they stop being computed *during a fetch* and start being
      computed *over the mirror*, which is exactly what `sync-diff` verifies.
- [x] `/api/bubbles`, `/api/bubbles/{id}/threads`, `/api/threads`,
      `/api/bubbles/{id}/heat` and `/api/tick` all serve from SQLite.
- [x] **Cycles are mirrored too.** They were the one thing the board still needed
      from Plane; without them heat silently falls back to the rolling window.
      They stay best-effort in the syncer, because a project with the cycles
      feature switched off has no such endpoint and blocking on it would leave
      that project permanently unclean.
- [x] `progressEvidenceFromMirror` replaces the `ListProjectItems` call per
      project per tick.
- [x] Deleted: `fetchInstance`, `buildBubble`, `projectCycleWindow`,
      `plane.ListModuleWorkItems`, `plane.WorkItem`, `boardFields`,
      `fetchConcurrency`, and the snapshot's `partial` flag.

**Accepted:** `TestBoardMakesNoPlaneCalls` proxies the fake Plane through a
request counter and requires **exactly zero** calls across five full cold board
rebuilds. Not a claim in prose — a count.

### Where the worksheet was wrong

Two instructions above turned out to be mistakes, and following them literally
would have introduced bugs:

- **"`progressEvidence` diffs against `mirror_items.description_hash`."** No.
  That column hashes the WHOLE body, so a Brief or title edit would register as
  production — which `AGENTS.md` explicitly says it is not. The Logbook
  fingerprint is still recomputed per item. Hashing a body is local CPU; the
  *call* was the expensive part and that is what went away.
- **"Delete `statesCache` and `membersCache`."** Not yet. The thread interior
  still uses both, and it does not move to the mirror until Phase 3. They are
  no longer on the board's path, which was the point.

`bubble_snapshots` also survives: with the board rebuilt from SQLite it is no
longer a cost saver, but it still serves the last-known board during the window
between a restart and the first sync pass.

### Measured on the live server

| | before Phase 2 | after Phase 2 | after the structure fix + Phase 3 |
|---|---|---|---|
| Plane calls | ~21.2/min | 12.0/min | **4.1/min — 80% off** |
| 429s | (pre-Phase 0: routine) | 0 | **0** |
| board content | 22 bubbles · 4 😴 / 18 🪦 · 93 threads | identical | **identical** |

Measured over a 7-minute window starting two minutes after a deploy, so no
startup reconcile falls inside it. The earlier 21.2 and 12.0 figures were each
contaminated by a full walk and read high.

Where the remaining 4.1/min goes, and the floor beneath it:

```
  7 calls / 2 min   delta: one ListItemsSince per project   = 3.5/min
  8 calls / 10 min  structure: projects + modules           = 0.8/min
  amortized         hourly reconcile + comment fill-in      ≈ 0.1/min
```

The floor is **one call per project per delta** — the page that says "nothing
changed". Only Phase 6 lowers it: with webhooks carrying the changes, the delta
interval can stretch a long way without the board going stale.

The board rendering identically through three phases of replacing its entire
data source is the real result: the refactor changed *where* the data comes from
without changing *what* it says.

The first measurement also exposed a waste the design had not accounted for. A
delta was spending **one ListProjects plus one ListModules per project, every two
minutes**, producing nothing — module membership is only applied on a full pass
anyway. On a 7-project workspace that was 8 of every 15 calls.

Those lists could not simply move to the hourly reconcile either: a newly created
bubble would take up to an hour to appear, a visible regression from the 90 s the
old refresher managed. So structure got its own cadence, `StructureInterval`
(10 min) — rare enough to be cheap, frequent enough that making a bubble still
feels responsive. `TestDeltaDoesNotRefetchStructure` pins both halves: a delta
must not re-read the lists, and a new bubble must still appear once the cadence
elapses.

**Cleanup carried:** the board rebuild is now event-driven — the syncer fires
`OnChange` and the snapshot rebuilds immediately, instead of the board waiting
out a 90 s timer. Fixed-interval polling of a local file is latency for no
reason. Also fixed: `mirror.ReplaceComments` used a bare `INSERT`, breaking the
package's own idempotency promise the moment a comment id reappeared.

## Phase 3 — Interiors and comments from L1 · **SHIPPED**

- [x] Thread detail, artifacts, revisions and Logbook render from `mirror_items`;
      comments from `mirror_comments`.
- [x] Deleted `detailCache`, `commentsCache`, `statesCache`, `membersCache` and
      their TTLs. Nothing left to cache: a cache over a local read would only add
      a staleness window to something already fast and always current.
- [x] **`probePulse` and `pulseProbeLimit` are gone.** The pulse is now
      `SELECT max(created_at) FROM mirror_comments WHERE item_id = ?`.
- [x] Deleted from the Plane client: `GetWorkItem`, `ListChildren`,
      `ListProjectItems`, `WorkItemDetail`. From the store: `StalePulseCheck`.
- [x] Comment `👀` read receipts still use `comment_reads` — unchanged, because
      Plane has no comment-reaction API and that overlay was never Plane's.

**Accepted:** `TestBoardMakesNoPlaneCalls` now covers the interior too — board,
timeline, search, heat, thread detail and comments, still exactly **zero** calls.

### The revision walk was the real cost

Opening a thread fanned out four concurrent Plane calls, and one of them —
`ListChildren` — **paged the entire project** and filtered by parent in Go,
because Plane has no usable sub-item endpoint. That is why the detail was cached
for 60 s despite being a read. `mirror_items` has an index on `parent_id`.

### Why the stored pulse survives

`thread_pulse` is still consulted, and the LATER of (mirrored comments, stored
pulse) wins. They answer subtly different questions: an item with no mirrored
comments might genuinely have none, or might simply not have had its comments
fetched yet — the per-pass comment budget fills those in over several passes.
Taking the max means a thread is never wrongly declared abandoned because the
sync had not reached it, while a mirrored comment still corrects a stale stored
pulse. Reading a discussion no longer *writes* a pulse, because reading the
mirror observes nothing new about Plane; posting one still does.

### One correctness gain, not just a speed one

`autoStateBubble` resolved its target states through a 30-minute cache. A newly
added workflow state could therefore be invisible for half an hour while
auto-state wrote cards into the wrong one. It reads the mirror now.

## Phase 4 — Identity and members from L1 · **SHIPPED**

- [x] Actor scoping reads `mirror_members` instead of calling `Members` on every
      instance. A cold auth went from **up to 2N Plane calls to exactly one**.
- [x] `Me(cred)` stays live — see below.
- [x] `/api/admin/members` reads the mirror (and sorts, since map iteration order
      would otherwise reshuffle the list on every refresh).
- [x] `authTTL` now caches only the IDENTITY. Scope and role are recomputed per
      request, so a membership or role change lands immediately instead of
      lagging five minutes.

**Accepted:** `TestColdAuthCostsOneCall` registers three instances and asserts a
cold auth spends exactly one `/users/me`, and that a warm one spends none.

### Why identity is not mirrored

`/users/me` is the actual credential check. Serving it from the mirror would mean
a revoked key kept working for as long as the mirror remembered the person —
that is not a cache, it is an authentication bypass. Revocation still lags by
`authTTL` exactly as it always has, because that call is the only thing that can
tell us a key was withdrawn.

### An empty member table is not an answer

Moving scoping to the mirror introduced a failure mode the Plane version could
not have: **"no mirrored members" is indistinguishable from "this person is not a
member"**. A sync outage would therefore have produced an authoritative, silent
"you belong nowhere" — every board blacked out, with a `200`.

A Plane workspace always contains at least the key's own owner, so zero mirrored
members means *we have not found out yet*, and scoping fails retryably instead.
`TestTransientMembersFailIsRetryable` covers it: during an outage the answer is
`502`, never `401`, and never an empty-scope `200`.

## Phase 5 — Writes: the outbox · **SHIPPED (narrowed)**

Scoped deliberately to the two writes that benefit. **Creates stay
write-through** — see *Why creates are not queued* below.

- [x] `outbox` table, drainer with exponential backoff (1m…1h), abandon after 8
      attempts.
- [x] **Auto-state** moves queue on failure and drain automatically: they use the
      instance key the server already holds.
- [x] **Comment drafts** keep the text and nothing else. Re-sent by their author,
      from the thread they were written in.
- [x] Conflict shield: `LockedFields` + `Syncer.SetLocks`. A field with a pending
      write is not overwritten by an incoming sync, so the board never visibly
      reverts a change and then flips forward again when the write lands.
- [x] Surfaces — drafts: REST (`202` + retry/discard), CLI
      (`bubble comment <id> --retry|--discard <draft>`), web (in-thread, marked
      unsent). Admin queue: `GET/DELETE /api/admin/outbox`,
      `bubble admin outbox [drop <id>]`, a God Mode panel.

**Accepted:** `TestCommentSurvivesPlaneOutage` takes Plane down, posts, and
asserts a `202` with the text intact, the draft visible in its thread, **zero**
comments sent to Plane, and that the stored payload contains only `body` — no
credential. Then Plane returns, the author retries, exactly one comment is
posted, and the draft is gone rather than duplicated.

### The outbox is not in the mirror

The worksheet's schema sketch put it there. That is wrong and would have been a
data-loss bug: the mirror is explicitly rebuildable and `mirror.Reset` /
`sync-backfill` drop its tables, but the outbox holds writes that have **not**
reached Plane. It lives in `internal/store` with the overlay.

### Why comment drafts carry no credential

Comments are posted with the **caller's own** Plane key, so Plane records the
real author. Queueing one for later needs a credential at drain time, and the
options were: store every user's key at rest, post as the service account and
misattribute the comment, or don't queue at all.

None of those is good. The draft holds only the text, and its author re-sends it
with their live key — no credentials at rest, correct authorship, and the thing
a person actually loses (their typing) survives. The cost is that a draft needs
its author to come back; a worker cannot land it for them. That is the right
trade for a comment, which is someone's words rather than a system action.

### Why creates are not queued

Three writes are creates — `CreateProject`, `CreateModule`, and thread birth
(`CreateWorkItem` + `AddIssuesToModule`, which is two steps and can half-succeed).
Everything downstream keys off Plane's assigned id: a bubble's id is literally
`slug:project:module`. Queueing a create means minting a local id and rewriting
it everywhere once Plane answers — contracts, snapshots, links people may have
bookmarked.

Optimistic **updates** are easy; optimistic **creates** are a distributed-identity
problem. These run a few times a week and waiting a second for a real id is
fine, so they stay write-through with a clear error. Worth revisiting only if
creating things while Plane is down turns out to matter.

### An abandoned write stays visible

Past 8 attempts an entry stops retrying and releases its field lock, so Plane's
value becomes truth again — but the row remains, on all three admin surfaces.
Silently dropping someone's write is the one thing an outbox must never do.

## Phase 6 — Payload-aware webhooks

*Freshness, not correctness — the delta covers correctness on its own (see the
2026-08-04 probe). Today's webhook is a blind "refresh everything" trigger
(`server.go:1147`); the payload is thrown away.*

- [ ] Parse the Plane webhook payload and apply it straight to the mirror:
      `issue`, `issue_comment`, `module`, `module_issue`, `project`, `state`.
- [ ] Treat webhooks as a **hint, not a source of truth**: apply, but leave the
      watermark alone so the next delta still reconciles. Out-of-order events lose
      to a newer `updated_at`.
- [ ] With webhooks healthy, stretch the delta interval (90 s → several minutes)
      and let events carry freshness. Fall back automatically when events go quiet.
- [ ] Notifications fire off mirror transitions, which also fixes the known gap
      where a 😴→🪦 slide is silent.

**Accept:** a comment posted in Plane's UI appears in Bubble Work in under a
second with no polling, and the pulse updates without any `ListComments` call.

## Phase 7 — Cleanup and degraded mode · **SHIPPED**

- [x] **Degraded mode.** `GET /api/status` reports whether the board is being
      served from an ageing mirror, plus the caller's own unsent drafts. Banner
      in the web UI, a warning line above `bubble ls`.
- [x] Retention: `store.Prune` drops `thread_progress` / `thread_pulse` /
      `thread_autostate` rows for work items Plane no longer has, fired on
      `OnReconciled` — after a COMPLETE walk and never a partial one.
- [x] `bubble admin sync-rebuild <slug>` — drop the mirror and rebuild it.
- [x] `server.go` split: `admin.go` (the God Mode surface) and `stream.go` (SSE).
      2199 → **1798** lines, alongside `board.go`, `sync.go`, `outbox.go`,
      `status.go` and `progress.go` added across the earlier phases.
- [x] ~~Gate deploys on tests~~ — landed early, in `f3eb365`.

**Accepted:** `TestDegradedModeReportsStaleness` checks all four states — never
synced, fresh, one missed interval (must NOT alarm), several missed (must).
`TestPruneDropsOverlayForDeletedItems` checks that an empty live set prunes
nothing.

### Degraded mode is the counterweight to the whole refactor

Every read now comes from a local copy. That is the point — and it introduces a
failure this codebase did not previously have: **if the sync stops, the board
keeps rendering, confidently, from data that is quietly getting older.** Nothing
errors. Nothing looks wrong.

The threshold is three delta intervals. One missed pass is ordinary — a
rate-limit yield will do it — so alarming at the first would train people to
ignore the banner, which is worse than not having one.

Being behind is fine. Being behind silently is not.

### Why pruning only runs after a complete walk

The overlay tables are keyed by Plane work-item id and were only ever inserted
into, so a deleted item left its rows behind forever. That is a slow leak, and a
resurrection hazard: were an id ever reused, the new thread would inherit a
stranger's progress timestamps and be born warm.

Pruning needs a complete live set to be safe, so it fires on `OnReconciled`, and
an empty set prunes nothing — deleting the entire overlay on the strength of a
failed sync would be catastrophic and silent. This is the same invariant the sync
already holds for its own prunes: *"I could not see it" must never be mistaken
for "it is gone."*

### `sync-rebuild` keeps the central claim honest

The mirror is a projection: deleting it must cost a backfill and nothing else. A
command that does exactly that is how the claim stays true rather than becoming
folklore — and if it ever loses something, that something was in the wrong table.
Which is precisely why the outbox lives with the overlay (Phase 5).

---

## Risks

- **Comment mirroring is the softest part, though the probe softened it further.**
  There is no comment feed at any scope, so comments are always a per-item fetch.
  The saving grace is that `updated_at` bumps identify *which* items to fetch, so
  the cost tracks real activity. Worst case (a burst of comments across many
  items in one interval) is a burst of fetches — bounded by the Phase 0 budget,
  which will spread it over ticks rather than 429.
- **`updated_at` is noisy by design.** It moves for things that must never
  generate heat. The delta and the evidence model have to stay strictly separate;
  see the guardrail above. This is the most likely way for this refactor to
  quietly break `AGENTS.md`'s core rule.
- **Deletes are invisible to delta sync.** An item removed in Plane keeps its
  mirror row until the periodic full reconcile. Reconcile interval is therefore a
  correctness knob, not just a cost one.
- **Module membership rides on the full reconcile.** Bubbles *are* modules, so
  moving a work item between modules changes the board. Rather than probe whether
  that bumps `updated_at`, Phase 1 simply assumes it does not: membership is
  re-read only on a full pass. That makes the reconcile interval (1 h) the
  worst-case lag on a bubble gaining or losing a thread. `sync-diff` reports
  membership drift explicitly, so the assumption is observable rather than
  hidden — if an hour proves too slow in practice, tighten the interval.
- **A partial pass is sticky.** Holding the watermark back on a partial pass is
  correct, but it means every subsequent pass is a full walk until one completes
  cleanly. Under sustained rate pressure that is self-reinforcing: full walks cost
  more, so they are likelier to be partial. The escape is Phase 2 — once the
  legacy fetch path is gone, the syncer stops competing with it for the same
  budget.
- **Optimistic writes can be wrong.** A write that abandons after max retries has
  been shown to the user as if it succeeded. This is why the outbox needs a
  visible surface on all three admin surfaces, not just a log line.
- **The mirror is a second copy of live data.** If a phase ships a read path
  before its sync path is correct, users see stale data and trust it. Hence Phase
  1 ships in shadow with `sync-diff` as the gate.

## Non-goals

- Offline **writes from the client** — the browser still needs the server; only
  the *server* tolerates Plane being away.
- Replacing Plane as the place humans edit work. Plane stays first-class.
- Multi-writer conflict merging on descriptions. Last write wins on bodies; we
  shield fields, we do not merge text.

## Open

**None of these block implementation.** Each has a safe default, and each is
decided by evidence produced in the phase that cares — recorded here so they get
answered deliberately rather than by accident.

| Open question | Default if unanswered | Decided in |
|---|---|---|
| Should the mirror hold Plane **activities**? Richer evidence than timestamp inference, but one call per item and no feed. | **No.** Current evidence model is sufficient and cheaper. | Post-Phase 6, if webhooks prove out |
| Per-instance sync cadence, or one server-wide setting like `tuning`? | **Server-wide**, matching `tuning`. Reversible — a per-instance column is an ALTER. | Phase 1 |
| Does `bubble_snapshots` survive Phase 2, or does a mirror query beat it at boot? | **Keep it** until measured. | Phase 2, by measurement |
| Does module membership change bump `updated_at`? | **Assume no**; the full reconcile covers it. | Phase 1 probe |
| Full-reconcile interval? | **Hourly**, tightened if deletes or module moves prove to lag visibly. | Phase 1, revisited in Phase 6 |
