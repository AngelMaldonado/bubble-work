# Bubble Work — Implementation Plan (Logbook)

> This is the living Logbook (§3.2) for the thread **"Build the Bubble Work server & clients."**
> It decomposes the build into phases; each phase has a Definition of Done.
> Update the status column and check boxes as reality changes — nothing here is heat until it ships.

**Status legend:** ✅ done · 🔄 in progress · ⬜ not started · 🚫 blocked

## Progress at a glance

| # | Phase | Outcome | Status |
|---|-------|---------|--------|
| 0 | Skeleton & spine | One binary boots; REST + MCP + policy gate live | ✅ |
| 1 | Identity & membership | Clients authenticate as team members; writes attributed | ✅ |
| 1.5 | Federation (multi-instance) | Server federates N Plane instances; members scoped per instance | ✅ |
| 1.6 | Plane-native identity | Humans auth via Plane pass-through; role + scope derived from Plane | ✅ |
| 2 | Plane read path | `bubble ls` shows real bubbles with correct heat (per instance) | ⬜ |
| 3 | Plane write path | Birth & contracts flow into Plane | ⬜ |
| 4 | Heat & lifecycle hardening | Tested, explainable temperature rules | ⬜ |
| 5 | Scheduler & push | Cooling/dormant transitions notify with nobody at the keyboard | ⬜ |
| 6 | Sync robustness & webhooks | Real-time updates; conflict policy enforced | ⬜ |
| 7 | Packaging & release | One-command install / documented deploy | ⬜ |

## Phase dependencies

```mermaid
flowchart LR
    P0["0 · Skeleton ✅"] --> P1["1 · Identity"]
    P0 --> P2["2 · Plane read"]
    P1 --> P3["3 · Plane write"]
    P2 --> P3
    P2 --> P4["4 · Heat & lifecycle"]
    P4 --> P5["5 · Scheduler & push"]
    P2 --> P6["6 · Webhooks / sync"]
    P3 --> P7["7 · Release"]
    P5 --> P7
    P6 --> P7

    classDef done fill:#065f46,stroke:#047857,color:#ecfdf5;
    class P0 done;
```

---

## Phase 0 — Skeleton & spine ✅

**Goal:** a single binary that boots and proves the architecture (Model A, two front doors, unbypassable policy).

- [x] One Go binary, subcommands `serve | ls | heat | init | version`
- [x] SQLite overlay store (pure-Go `modernc`, single static binary)
- [x] `heat.Classify()` — pure temperature function (§0/§5)
- [x] Plane REST client (read stubs) — the sole path to Plane
- [x] Server: REST `/api` + our own MCP `/mcp`, tools registered
- [x] Policy gate: `birth_thread` rejects missing Brief / Definition of Done / Logbook
- [x] `AGENTS.md` + `CLAUDE.md` so the repo runs on its own framework
- [x] Build, `go vet`, boot, and endpoint smoke-tests pass

**Definition of Done:** `go build ./...` clean; `bubble serve` boots; REST, MCP handshake, and the birth-rule reject/accept paths verified. ✅ **Met.**

---

## Phase 1 — Identity & membership ✅

**Goal:** turn a client into an actual team member (§9.3). Every request resolves to a Member; every write is attributed.

- [x] `members` table CRUD in `store` (`CreateMember` / `ListMembers` / `RevokeMember` / `MemberByToken`)
- [x] `bubble member add --name --kind human|agent [--plane-id]` → issues a token (local DB, bootstraps the first token)
- [x] Auth middleware on REST (`restAuth`) + MCP (SDK `auth.RequireBearerToken`): reject unauthenticated; put the Member in `context`
- [x] Thread the actor through `BirthThread` / `CloseBubble` for attribution (logged, both front doors)
- [x] Capture Member → Plane identity (`plane_id` stored & carried; *used* for assignee/comment in Phase 3)
- [x] Client sends its `token` on every call; friendly 401 guidance
- [x] Tests: unauthenticated → 401, bad token → 401, valid → 200; birth policy reject/accept

**Dependencies:** Phase 0.
**Definition of Done:** no state-changing call succeeds without a valid member token; each request logs *which member* acted; `bubble ls` works when authenticated and is refused when not. ✅ **Met** — verified via `go test ./...` and an end-to-end REST + MCP smoke test.

---

## Phase 1.5 — Federation (multiple Plane instances) ✅

**Goal:** the server federates over several Plane deployments; members are scoped to the instances they may see (isolation between orgs).

- [x] `plane_instances` + `member_instances` tables; instance CRUD + grant/revoke in `store`
- [x] `bubble instance add|list|remove` (API keys stored server-side, never printed)
- [x] `bubble member grant|deny <member-id> <instance-slug>` with existence validation
- [x] `collect()` fans out over the caller's authorized instances; bad instance logged + skipped
- [x] Instance pins one project, or federates the whole workspace when `--project` is omitted (auto-discovers all projects)
- [x] Bubble ids namespaced `<instance-slug>:<project-id>:<module-id>`; `bubble ls` shows an INSTANCE column
- [x] Plane creds removed from `config.json` (moved into the store)
- [x] Store test: a member sees only granted instances; revoke removes visibility

**Definition of Done:** two instances registered; two members each see only their granted instance's bubbles; a query never touches an unauthorized instance. ✅ **Met** — verified via store test + an end-to-end isolation smoke test.

---

## Phase 1.6 — Plane-native identity (pass-through) ✅

**Goal:** stop duplicating Plane's identity (§8). Humans authenticate with their own Plane API key; agents keep server tokens.

- [x] `plane.Client.Me()` (`/users/me`) and `Members()` (`/members/`, roles 20/15/5)
- [x] Request identity unified as `domain.Actor` (id, name, kind, email, admin, instances)
- [x] `resolve()` — agent token → local member+grants; else Plane key → `/users/me` + cross-instance email match for scope + role, cached (TTL)
- [x] REST middleware + MCP verifier both resolve to an `Actor`; attribution updated
- [x] Admin derived from Plane role ≥ 20; `close_bubble` refuses instances you can't see
- [x] `bubble whoami` + `GET /api/whoami` to confirm the resolved identity
- [x] `bubble member add` reframed as agent-only; human `member add`/grants no longer needed
- [x] Tests: Plane pass-through (identity + admin + scope from a fake Plane); agent path

**Definition of Done:** a human runs `bubble init --token <their-plane-key>` and `bubble whoami` shows their name, admin role, and every instance they belong to — with no member created or grant issued. ✅ **Met** — verified via the pass-through test and a live smoke test.

> Note: this supersedes Phase 1's member registry **entirely**. Agents impersonate
> a human by using that human's Plane key (direct-key impersonation), so there is
> no local member / token / grant machinery — auth is pass-through only. The
> `members` and `member_instances` tables and the `bubble member` command were
> removed; the isolation boundary is now Plane workspace membership itself.

---

## Phase 2 — Plane read path (real buoyancy)

**Goal:** `bubble ls` reflects each live Plane instance with correct temperatures.

- [ ] Verify JSON shapes against a live Plane project (modules, `module-issues`, activities)
- [ ] Handle cursor pagination (`?cursor=…`) in `plane.Client` list calls
- [ ] Harden `Meaningful()` against real activity `field`/`verb` values (§5.1 allowlist)
- [ ] Instance connectivity check (`bubble instance add` verifies workspace/project resolve)
- [ ] Read-model cache table + refresh (per instance), so `ls` is fast and rate-limit friendly
- [ ] Tests: `Meaningful()` mapping; heat over a fixture activity set

**Dependencies:** Phase 1.5.
**Definition of Done:** against your real `ayetec` and `cuby` instances, `bubble ls` lists their modules as bubbles, hottest-first, tagged by instance, and `bubble heat <instance>:<id>` explains the state from actual activity — no manual data.

---

## Phase 3 — Plane write path (birth & contracts flow to Plane)

**Goal:** the framework writes back — births create work items; contracts and closures persist.

- [ ] `plane.Client` write methods: create work item, set description/page, link to module
- [ ] `BirthThread` POSTs the work item with **Brief + Logbook in one description page** (§7.1)
- [ ] `bubble bubble new|set --outcome --owner --closure` → `store.SetContract` (§4)
- [ ] `close_bubble` optionally reflects closure into Plane (label/state), not just overlay
- [ ] Idempotency / error surfacing on partial Plane failures
- [ ] Tests: birth creates a work item (against a scratch project or mocked transport)

**Dependencies:** Phase 1 (attribution), Phase 2 (read shapes).
**Definition of Done:** birthing a thread through CLI or MCP creates a real Plane work item carrying its Brief + Logbook; setting/closing a bubble's contract survives a server restart.

---

## Phase 4 — Heat & lifecycle hardening

**Goal:** the temperature rules are precise, tested, and explainable.

- [ ] Finalize the cycle model (configurable pulse; document current vs previous windows)
- [ ] Confirm transition rules Hot→Warm→Cooling→Dormant→Closed against §5.3 edge cases
- [ ] `bubble heat` shows the contributing evidence events + which cycle each fell in
- [ ] Table-driven tests for `Classify()` across every lifecycle branch
- [ ] Document the buoyancy `Score` formula in the spec

**Dependencies:** Phase 2.
**Definition of Done:** every lifecycle branch has a passing test; `bubble heat` explains *why* with the evidence that produced the verdict.

---

## Phase 5 — Scheduler & push

**Goal:** time-driven side effects happen with nobody at the keyboard (§6 of the earlier brainstorm).

- [ ] `bubble tick` one-shot: recompute, detect cooling/dormant transitions
- [ ] Notification sink(s): stderr/log first, then webhook or CLI digest
- [ ] "Recommend closing" output for zombie bubbles (§8)
- [ ] Optional in-process ticker inside `serve` (interval configurable)
- [ ] Doc: sample `launchd`/cron/systemd-timer entry to run `bubble tick`
- [ ] Tests: a bubble crossing into Dormant emits exactly one notification

**Dependencies:** Phase 4.
**Definition of Done:** a bubble that goes quiet past its threshold produces a notification on a scheduled `tick`, and never manufactures heat to stay warm.

---

## Phase 6 — Sync robustness & webhooks (optional)

**Goal:** real-time reaction and a clean conflict policy; only if polling proves insufficient.

- [ ] `POST /webhooks/plane` endpoint to ingest Plane events → invalidate/refresh cache
- [ ] Conflict policy per §9.2 (Plane wins its fields; server wins the overlay)
- [ ] Rate-limit / backoff around Plane calls
- [ ] Fallback to polling when webhooks are unavailable

**Dependencies:** Phase 2.
**Definition of Done:** a change made directly in Plane's UI appears in `bubble ls` within seconds via webhook, with polling as a proven fallback.

---

## Phase 7 — Packaging & release

**Goal:** minimal, easy to publish (the original ask).

- [ ] `README.md` quickstart (serve, init, connect an agent)
- [ ] `claude mcp add --transport http bubble http://<host>/mcp` documented
- [ ] Cross-compiled static binaries (goreleaser) — no cgo, single file
- [ ] Optional: `Dockerfile` + a one-line deploy (fly.io / container)
- [ ] Secrets handling: env + config file; optional OS keyring for the member token
- [ ] Versioning + `CHANGELOG`

**Dependencies:** Phases 3, 5, 6.
**Definition of Done:** a new user can install the binary, `bubble init`, `bubble serve`, connect Claude Code as a member, and see bubbles — from documented steps alone.

---

## Cross-cutting (every phase)

- [ ] Unit tests alongside each package; `go test ./...` green
- [ ] CI: build + vet + test on push
- [ ] Structured logging with the acting member (from Phase 1) on every mutation
- [ ] Keep `bubble-work-spec.md` authoritative — code changes that alter behavior update the spec

---

*Heat rule for this plan: a checked box is only heat when it maps to a committed/verified change (§5.1). Checking a box you haven't shipped is exactly the "activity gaming" the framework is designed to prevent.*
