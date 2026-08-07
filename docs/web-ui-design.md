# Bubble Work — Web UI design (the buoyancy workspace)

The human-facing surface: a full-screen **floating-bubbles workspace** where your
attention literally floats. Backed by the existing server APIs (§9); the browser
is just another thin client.

## The model — 5 levels = automatic heat + explicit stage

Heat is **automatic**, derived from a bubble's thread activity (§5). An optional
explicit **stage** overlays it. The five levels resolve like this (first match
wins):

| Level | Resolves from | Source |
|-------|---------------|--------|
| 🏆 **Done** | bubble is closed (§5.3) | explicit (`close`) |
| 👀 **Reviewed** | `stage = reviewed` | explicit |
| 🔥 **In progress** | heat is Hot or Warm | automatic |
| 😴 **Zzzz** (slept) | heat is Cooling/Dormant *and* it has history/owner | automatic |
| 🪦 **RIP** (backlog) | Dormant with no owner / no evidence ever | automatic |

So the engine floats bubbles between In-progress ↔ Zzzz ↔ RIP on its own; you
only ever explicitly mark **Reviewed** or **Done**. `Reviewed` is a WIP concept —
surfaced in the UI and stored server-side — but it does not affect heat.

- **In progress** is capped with a **soft warning** (default 5): the band still
  accepts more, but the header nudges you (`6 / 5 focus ⚠`).
- **Done** is a **trophy case** — hidden by default, opened on demand.

## Layout

```
┌─ bubble.work ─────────────────────────────────── angel@cuby.mx ▾ · cuby ▾ ─┐
│  🔥 IN PROGRESS                                           2 / 5 focus        │
│      ╭─────────────╮  ╭─────────────╮  ╭ + birth ╮                          │
│      │ Onboarding  │  │Billing revamp│ ╰─────────╯                          │
│      │ ●●○ 2 open  │  │ ● 1 open     │                                        │
│      ╰─────────────╯  ╰─────────────╯                                        │
│  👀 REVIEWED           ( Q3 report )                                          │
│  😴 ZZZZ               ( Infra Atwoz ) ( grafana ) ( warehouse )              │
│  🪦 RIP · backlog (12) ·········································· ▸ expand      │
│  🏆 Done ▸ 27 trophies (hidden)                                              │
│  ⌘K search · > commands                                        ● polling     │
└──────────────────────────────────────────────────────────────────────────────┘
```

**⌘K omnibar** — fuzzy search over your instance-scoped **threads** (tasks):

```
┌───────────────────────────────────────────────┐
│ ⌘K  sign|                                       │
│  THREADS                                        │
│   › Design signup screen   Onboarding · 🔥 open  │
│   › Wire up auth flow      Onboarding · 🔥 open  │
│  BUBBLES                                        │
│   ◯ Onboarding flow        in progress          │
│   ↑↓ move  ⏎ open  ⌘⏎ open in Plane             │
└───────────────────────────────────────────────┘
```

**`>` command palette** — same bar, `>` flips to actions (no cmd+shift+p):

```
┌───────────────────────────────────────────────┐
│ ⌘K  > |                                         │
│   > new bubble                                  │
│   > birth thread…        (enforces Brief + DoD) │
│   > mark reviewed…       → 👀                    │
│   > close bubble (done)… → 🏆                    │
│   > set owner / outcome…                        │
│   > switch instance  cuby · ayetec              │
└───────────────────────────────────────────────┘
```

`> birth thread` opens a form that requires a **Brief (with Definition of Done)**
and a **Logbook** — the §3 birth rule as a first-class UI action.

## Tech stack (mirrors `~/dev/mdserve`)

- **Svelte 5 + Skeleton 5** (`@skeletonlabs/skeleton`, `@skeletonlabs/skeleton-svelte`)
- **Tailwind 4** (`@tailwindcss/vite`), **Vite** + **bun**, TypeScript — plain SPA (no SvelteKit)
- Built to `web/dist`, embedded in the Go binary via `//go:embed all:web/dist` and
  served at `/` (single origin, single binary — true to §9).
- Dev: `vite` proxies `/api`, `/mcp` to the running `bubble serve` (`BUBBLE_DEV_BACKEND`).

```
frontend/
  index.html
  vite.config.ts        # proxy /api → backend; build → .vite-dist
  package.json          # svelte 5, skeleton 5, tailwind 4 (from mdserve)
  scripts/package.ts    # copy build → ../web/dist for go:embed
  src/
    main.ts  App.svelte
    lib/api.ts          # typed client (whoami, bubbles, threads, actions)
    lib/store.ts        # bubbles/levels, active instance, polling
    components/ Workspace.svelte  Band.svelte  Bubble.svelte
                Omnibar.svelte    CommandPalette.svelte  BirthForm.svelte
web/dist/               # embedded build output (git-ignored or committed — see build)
```

## Auth in the browser

The browser presents the same credential as the CLI — a **Plane API key** — via a
minimal "paste your key" screen, stored in `localStorage` and sent as
`Authorization: Bearer`. Acceptable for a personal LAN/tailnet tool (v1); a real
session/cookie login is a later hardening step. Everything else (scope, identity,
inbox) is derived server-side exactly as for the CLI.

## Backend (Go) changes this requires

1. **`reviewed` stage overlay** — add `stage` to the contract store; endpoints
   `POST /api/bubbles/{id}/review` and `/unreview`; patch cache in place.
2. **Level in the derived view** — add `Level` to `BubbleView`, computed from
   heat + stage + closed (the table above), so the client just buckets by it.
3. **Threads/search endpoint** — `GET /api/threads?q=` returning threads across
   the caller's instances (id, name, bubble id/name, instance, open/done) for ⌘K.
   Today threads aren't exposed to clients (only a count).
4. **Serve the SPA** — `//go:embed all:web/dist`, mounted at `/` with SPA
   fallback to `index.html`; `/api`, `/mcp`, `/webhooks` unchanged.

Nothing else changes — the workspace reads `GET /api/bubbles` (now with `Level`),
inbox, and drives actions through `birth`/`contract`/`review`/`close`.

## Build & deploy

- `frontend` builds with bun → `web/dist`; `go build` embeds it. The `deploy/`
  script on the mini must run the frontend build first, so the mini needs **bun**
  (one-time `brew install oven-sh/bun/bun`), OR we commit `web/dist` to the repo.
  Recommended: build on the mini to match "build from source".
- The GitHub Actions deploy already rebuilds on push; extend the deploy script to
  `bun install && bun run build` in `frontend/` before `go build`.

## v1 scope vs later

- **v1:** the 5-band workspace, ⌘K thread search, `>` palette (new bubble, birth,
  review, close, set owner/outcome, switch instance), WIP soft-warning, trophy
  drawer, paste-key auth.
- **Later:** true physics/animation for the floating bubbles, drag between bands,
  real-time via the (deferred) webhook path, session login, inbox surfaced as a
  badge, multi-instance unified view in one canvas.
