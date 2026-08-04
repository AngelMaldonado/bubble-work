# Plane Sync — SQLite as L1, one worker as the only Plane client

Status: **Drafted, nothing shipped · Phase 1 spike answered (see below)** ·
Drafted 2026-08-04 · Companion to
[`AGENTS.md`](../AGENTS.md), [`bubble-work-spec.md`](./bubble-work-spec.md) and
[`THREAD-LIFECYCLE.md`](./THREAD-LIFECYCLE.md).

This is a **worksheet**: every phase carries checkboxes and an acceptance test.
Tick them as they land and update the Status line, the way `THREAD-LIFECYCLE.md`
was kept current.

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

## Phase 0 — Rate discipline

*No architecture change. Ships in a day and makes today measurably better, which
also gives us a baseline to prove the rest against.*

- [ ] Parse `X-RateLimit-Remaining` / `X-RateLimit-Reset` on every response into
      a per-key budget in `internal/plane`.
- [ ] **Reserve a lane**: background work must stop at a floor (e.g. 15 of 60),
      leaving the rest for interactive calls. Background callers block or skip;
      interactive callers always pass.
- [ ] Replace the blind 0.4/0.8/1.6 s backoff with "wait until reset" when the
      budget is known.
- [ ] Add `fields=` to the two hot list calls so pages carry only what the board
      reads.
- [ ] Expose budget state on `GET /api/admin/stats` (remaining, reset, calls in
      the last minute, 429 count) → surface in God Mode.
- [ ] Raise `API_KEY_RATE_LIMIT` on the self-hosted Plane as an explicit,
      documented stopgap in `DEPLOY.md`.
- [ ] **Put SQLite in WAL mode.** `store.Open` currently does a bare
      `sql.Open("sqlite", path)` (`store.go:119`) — no journal mode, no busy
      timeout, no pool limits. That is fine today because writes are rare and
      tiny; it will **not** survive a sync worker bulk-upserting while every
      board read queries the same file. Default rollback journal means a reader
      and the writer collide as `SQLITE_BUSY`, surfacing as intermittent 500s
      under load — the worst class of bug to chase later. Needed:
      `_pragma=journal_mode(WAL)`, `_pragma=busy_timeout(5000)`,
      `_pragma=synchronous(NORMAL)`, and `SetMaxOpenConns` bounded with a single
      writer path. **This must land before Phase 1 writes bulk data.**

**Accept:** God Mode shows live budget; a forced burst degrades background
refresh instead of 429-ing a human's thread open. A concurrent
read-while-bulk-write test passes without `SQLITE_BUSY`.

**Cleanup carried:** the retry/backoff block in `client.go:79-120` becomes one
budget-aware helper shared by `get` and `send`.

## Phase 1 — The mirror, in shadow

*Write the mirror, read nothing from it. Zero user-visible risk.*

- [x] ~~Spike: does the work-item list carry full `description_html`?~~
      **Answered 2026-08-04: yes, untruncated** (14 k body identical list vs get).
      Per-item `GetWorkItem` is off the critical path; bodies arrive 100/page.
      See *Probed against the live instance* above.
- [ ] `internal/mirror`: schema + upsert/query API. Migrations in `store.Open`
      alongside the existing best-effort ALTERs.
- [ ] `internal/sync`: `Backfill(instance)` — full walk of projects, modules,
      module membership, states, members, items. Rate-budgeted, resumable via
      `sync_cursors`. Use `expand=state` so a row carries its state group inline,
      and **count arriving rows** rather than trusting `total_results`.
- [ ] `internal/sync`: `Delta(instance)` — `order_by=-updated_at`, early-stop at
      the watermark, one page in steady state.
- [ ] Worker loop replacing the tick's fetch role: delta every `snapshotRefresh`,
      full reconcile on a long interval (catches deletes, which delta cannot see).
- [ ] Shadow verification: a `bubble admin sync-diff <slug>` that builds the
      board from the mirror and from the live path and reports differences.

**Accept:** `sync-diff` is empty for every instance across a full cycle. Nothing
in `server.go` has changed behaviour yet.

## Phase 2 — Board reads from L1

- [ ] `bubbleHeat` / `buildBubble` / `threadBuoyancy` take mirror rows instead of
      `plane.WorkItem`.
- [ ] `/api/bubbles`, `/api/bubbles/{id}/threads`, `/api/threads`, `/api/tick`
      read the mirror.
- [ ] Delete `bubblesCache`, `statesCache`, `membersCache` and `fetchInstance`.
      `bubble_snapshots` survives only if it still beats a mirror query at boot —
      measure, then decide.
- [ ] `progressEvidence` diffs against `mirror_items.description_hash`; drop
      `ListProjectItems`.

**Accept:** board renders with **zero** Plane calls in the request path; a `tcpdump`
/ call-counter over a minute of heavy browsing shows only worker traffic.

**Cleanup carried:** `server.go` sheds the fetch/refresh path (~500 lines) into
`internal/sync`. `projectCtx` moves with it.

## Phase 3 — Interiors and comments from L1

- [ ] Thread detail, artifacts, revisions (children) and Logbook render from
      `mirror_items` + `mirror_comments`.
- [ ] Delete `detailCache` and `commentsCache`.
- [ ] **Delete `probePulse` and `pulseProbeLimit` entirely** — the pulse becomes
      `SELECT max(created_at) FROM mirror_comments WHERE item_id = ?`. This is the
      single biggest budget win in the whole plan.
- [ ] Comment sync — **targeted off the delta**, per the 2026-08-04 probe. There
      is no comment feed, but a comment bumps its parent's `updated_at`:
  - the delta page names the items that changed; fetch comments only for those
    whose `description_hash` and `state_id` did **not** change;
  - one-time backfill of comments during Phase 1's initial walk;
  - webhook `issue_comment` events (Phase 6) collapse the delay to ~0, but are
    not required for correctness.
- [ ] Assert the guardrail in code: the delta watermark must not feed
      `EvidenceEvent`. A test that comments on a thread and asserts it stays 😴
      (neither 🔥 nor 🪦) is the regression guard.
- [ ] Comment `👀` read receipts keep using `comment_reads` — unchanged.

**Accept:** opening any thread issues zero Plane calls. Pulse correctness is
verified against a thread that was commented on but not progressed (it must hold
😴, not fall to 🪦, and not rise to 🔥).

## Phase 4 — Identity and members from L1

- [ ] Actor scoping (`resolveUncached`) reads `mirror_members` instead of calling
      `Members` on every instance. Cost per cold auth: **2N → 1**.
- [ ] `Me(cred)` stays live — it is the actual credential check and must not be
      served from a mirror — but its result caches on the *email*, and a
      successful identification refreshes that member row.
- [ ] `/api/admin/members` reads the mirror.
- [ ] Keep `authTTL` for the `Me` call only; scope is now free, so it can be
      recomputed per request.

**Accept:** a cold browser load with an empty identity cache costs exactly one
Plane call regardless of instance count.

## Phase 5 — Writes: the outbox

- [ ] `outbox` table + `internal/sync` drainer, rate-budgeted, exponential
      backoff, `abandoned` after a bounded number of attempts.
- [ ] Migrate each write in turn, L1 first then enqueue:
      comment post, thread birth, work-item state (auto-state, Phase B),
      module create, project create.
- [ ] **Conflict shield**: `field_lock` on pending rows; the sync applier skips
      locked columns. On abandon, Plane's value wins and the row surfaces.
- [ ] `thread_autostate`'s `handed_off` latch composes with this rather than
      duplicating it — the latch is permanent provenance, the lock is transient.
- [ ] Pending state on the wire: DTOs carry `pending?: string[]` (field names);
      the UI shows ⧗ on those fields.
- [ ] Surface parity — the outbox is operator-facing, so:
      **REST** `GET /api/admin/outbox`, `POST /api/admin/outbox/{id}/retry`,
      `DELETE /api/admin/outbox/{id}`; **CLI** `bubble admin outbox [retry|drop]`;
      **web** a God Mode panel. No MCP (admin capability, matching `autostate`
      and `tuning`).
- [ ] **Enforce the boundary:** CI greps that `internal/server` no longer imports
      `internal/plane`.

**Accept:** with Plane stopped, a comment posts, appears immediately marked ⧗,
survives a server restart, and lands when Plane returns. A state changed by hand
in Plane's UI while a write is queued is not clobbered, and is not reverted.

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

## Phase 7 — Cleanup and degraded mode

- [ ] Split `server.go` (2199 → ~1200) along the seams the refactor exposes:
      `board.go`, `admin.go`, `stream.go`, leaving `server.go` as wiring.
- [ ] **Degraded mode in the UI**: when `last_ok_at` is stale or the outbox is
      backing up, show it. Being read-only because Plane is down is fine; being
      read-only *silently* is not.
- [ ] Retention: prune `mirror_comments` for closed threads, and the
      `thread_progress` / `thread_pulse` / `thread_autostate` rows that currently
      grow forever.
- [ ] `bubble admin mirror rebuild <slug>` — drop and backfill, proving the mirror
      is genuinely disposable.
- [ ] Gate `deploy.yml` on `go test ./...` + `npx svelte-check` (unblocked as of
      `026e899`; a refactor this size should not deploy unchecked).

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
- **Module membership may be invisible too.** Bubbles *are* modules, so moving a
  work item between modules changes the board — but it is unknown whether that
  bumps the item's `updated_at`. Probe it in Phase 1 (cheap: move an item, re-read
  the delta page). If it does not, module membership rides on the full reconcile
  alone, which tightens the reconcile interval further. Safety net either way.
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
