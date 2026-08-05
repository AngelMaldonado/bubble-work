# Plane Sync — SQLite as L1, one worker as the only Plane client

Status: **Phases 0 and 1 shipped · live `sync-diff` CLEAN on 2026-08-05, so
Phase 2 is unblocked · Phases 2-7 pending** ·
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
