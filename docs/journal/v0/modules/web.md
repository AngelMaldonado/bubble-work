# Web — the buoyancy board

**Status:** built · **Packages:** `frontend/` (Svelte 5 + Skeleton 5 + Tailwind 4,
built with bun), `web/` (the embedded bundle) · **Depends on:** the REST API and SSE
· **History:** [`journal/web-ui-design.md`](../journal/web-ui-design.md),
[`journal/ARTIFACT-EDITING.md`](../journal/ARTIFACT-EDITING.md)

## What it solves

Seeing what floats. The board is the one surface where buoyancy is *spatial* — the
model's whole claim is that attention behaves like a fluid, and a list of statuses
cannot show that.

## Model (normative)

One binary, one origin: the SPA is embedded in the server (`web/embed.go`,
`//go:embed all:dist`) and served at `/`. There is no separate frontend deployment,
and `web/dist` is committed — which is why a plain `go build` needs no JS toolchain,
and why a UI change belongs in the commit as a rebuilt bundle.

### What is there

- **The board** — bubbles bucketed by level, ordered by buoyancy inside a band.
  Filters are remembered across reloads. A bubble's badge counts in-progress threads.
- **Thread interiors** — the document, plus the Logbook and Definition of Done when
  the thread has them, rendered and editable in place, with a table of contents,
  inline checkboxes, slash commands, optional vim keys, and the thread's own level.
- **New thread** — free by default: a name and one markdown document, in whatever
  shape the work has. A *guided* mode offers the typed Brief template
  ([`decisions/0005`](../decisions/0005-the-framework-does-not-own-your-format.md)).
- **Pages** — a file-tree reading room per workspace, using the **same** prose
  renderer as a thread's artifacts. One renderer is the rule: a page and a thread's
  document must not be able to look different.
- **⌘K** — an omnibar over workspaces, threads and bubbles, plus a `>` command
  palette.
- **God Mode** — the admin surface: instances, tuning, the outbox, mirror census,
  diff, backfill, kiosk tokens.
- **Live updates** over SSE, EN/ES, light/dark.

### Rules the UI must hold

- **One renderer.** Prose rendering (`Prose.svelte`, `lib/prose.ts`) is shared by
  every markdown surface. A second copy of the CSS is how two documents start looking
  different.
- **Staleness is visible.** An ageing mirror or an unsent draft is a banner, never
  silence.
- **Optimistic, but honest.** An edit sends the hashes it read as `base`; a conflict
  is shown, not swallowed.
- **A capability the instance lacks is stated**, not hidden — pages being the case
  that matters ([`pages.md`](./pages.md)).

## Surfaces

The web is a surface; it consumes the same REST endpoints as the CLI plus
`GET /api/stream` for SSE. It gains a capability **after** the API, CLI and MCP
have it, and only when that capability has a visual form.

## Invariants

1. The bundle is embedded and committed; the binary is self-contained.
2. One prose renderer for every markdown surface.
3. Never render confidently from data known to be stale without saying so.
4. bun, never npm (a stray `package-lock.json` resolves a different tree than CI).

## Open

- Band collapse state is not persisted across reloads.
- Finishing a thread is only reachable from the interior, not from the bubble's
  timeline ([`threads.md`](./threads.md)).
