# Bubble Work

A personal way-of-working where **attention behaves like buoyancy**: work that
produces evidence stays *hot* and rises; work that goes quiet *cools* and sinks.
Time is the governing force — your only standing job is to tend what floats at
the top.

Bubble Work ships as a single Go binary (`bubble`) that runs a small **server**
(the "brain") and a thin **client**, and serves its own web UI. It sits on top of
one or more [Plane](https://plane.so) instances: Plane stays the system of record
for the actual work items, while the server owns the Bubble Work overlay — heat,
lifecycle, contracts, and membership.

> **The full model lives in [`docs/bubble-work-spec.md`](./docs/bubble-work-spec.md).**
> This README is how to run it. The operating rules both Claude Code and Codex
> load are in [`AGENTS.md`](./AGENTS.md); the design worksheets are in
> [`docs/`](./docs).

## Concepts (one line each)

| Term | Meaning | Plane mapping |
|------|---------|---------------|
| **Workspace** | boundary for a body of work | Project |
| **Bubble** | a durable grouping; the unit of *attention* | Module |
| **Thread** | one executable unit of work | Work item |
| **Cycle** | the repeating pulse heat is measured against | Cycle |
| **Artifact** | the evidence a thread produces (Brief, Logbook, revisions) | Work-item body / sub-item |
| **Page** | reference material for a whole workspace | Project page |
| **Heat** | evidence of *changed reality*, never mere activity | (derived) |

A bubble is **Hot → Warm → Cooling → Dormant → Closed**, computed from the
meaningful outputs on its threads. Threads carry their own derived buoyancy
(🔥 in progress · 😴 zzzz · 🪦 rip · 🏆 done) which rolls up into the bubble — see
[`docs/THREAD-LIFECYCLE.md`](./docs/THREAD-LIFECYCLE.md).

## Architecture

```
  Browser (SPA)      Humans (CLI)      Agents (Claude Code / Codex, MCP)
        \                 │                    /
         \                │                   /   auth: a Plane API key
          v               v                  v
   ┌──────────────────────────────────────────────────┐
   │              Bubble Work Server                  │ ← the only thing
   │   /  SPA        /api  REST        /mcp  tools     │   clients talk to
   │   policy engine · heat · lifecycle · scheduler    │
   │   ┌─────────────────────┐  ┌───────────────────┐  │
   │   │ overlay (SQLite)    │  │ mirror — L1       │  │
   │   │ heat · contracts    │  │ local read model  │  │
   │   │ members · outbox    │  │ of Plane          │  │
   │   └─────────────────────┘  └───────────────────┘  │
   └────────────────────────┬─────────────────────────┘
                            │ one sync worker · REST only
                            v
              Plane instances (system of record)
```

- **One binary, two modes:** `bubble serve` is the brain; the other subcommands
  are the thin client. The web bundle is embedded, so there is one origin and
  one process.
- **Reads come from the local mirror**, not from Plane. A single sync worker is
  Plane's only reader (delta every ~2 min, full reconcile hourly), which is what
  keeps the whole system inside Plane's 60 req/min budget.
- **Writes are optimistic**: they land locally and drain to Plane through an
  outbox with retry. `GET /api/status` reports when the mirror is ageing or you
  have unsent drafts, so being behind is never silent.
- **Members, not anonymous callers.** Every request authenticates as a member
  (human or agent) and is attributed.
- **Federates multiple Plane instances**, with members scoped per instance.
- **The server is Plane's only client, over REST only** — no client ever calls
  Plane directly or via Plane's MCP, so the policy engine cannot be bypassed.

## Install

Requires Go 1.26+. The built web bundle (`web/dist`) is committed, so a plain
build needs **no JS toolchain**:

```bash
go build -o dist/bubble ./cmd/bubble
```

Optionally add an alias:

```bash
echo "alias bubble='$(pwd)/dist/bubble'" >> ~/.zshrc && source ~/.zshrc
```

## Quickstart (single machine)

Your laptop can be **both server and client** — that's the default. Everything
lives in `~/.bubble/` (override with `$BUBBLE_HOME`): the client `config.json`
and the server's `bubble.db`. The client talks to `http://localhost:4006` out of
the box.

```bash
# 1. Register a Plane instance (secrets stay server-side, never printed back).
#    Omit --project to federate the WHOLE workspace (all its projects);
#    add --project <id> to pin a single project instead.
bubble instance add --slug ayetec \
    --url https://plane.ayetec.space \
    --key   <PLANE_API_KEY> \
    --workspace <workspace-slug>

# 2. Authenticate as yourself with your OWN Plane API key.
#    The server derives your identity, role, and instance scope from Plane —
#    no member to create, no grant to hand out.
bubble init --token <your-plane-api-key>

# 3. Run the brain, then look at your bubbles
bubble serve            # foreground on :4006 (or `bubble serve &` to background it)
bubble whoami           # confirm Plane resolved you (name, role, instances)
bubble ls               # bubbles from every instance you belong to, hottest first
```

The first start backfills the mirror from Plane; `bubble admin sync <slug>` shows
the census and cursor without spending a single Plane call.

Then open **http://localhost:4006** for the board.

> **Humans don't get registered.** Your Plane API key *is* your credential —
> identity, admin role, and which instances you see all come from Plane. If you
> belong to several instances under the **same email**, one key gives you a
> unified view (the server matches you by email across instances). Different
> email per org? Use **credential profiles** + `bubble use` (see below).

To keep the server strictly local, bind loopback only:

```bash
bubble serve --addr 127.0.0.1:4006
```

## Three surfaces, one server

A server capability lands on all three client surfaces in the same change (the
web UI follows when it has a visual form). The logic lives in a server method
each surface reuses, so they don't drift.

- **Web** — the buoyancy board at `/`: bubbles bucketed by level, thread
  interiors with the Brief / Logbook / Definition of Done rendered and
  **editable in place**, a ⌘K omnibar over workspaces · threads · bubbles and a
  `>` command palette, per-workspace pages, God Mode for admins, live updates
  over SSE, EN/ES, light/dark, and optional vim keys in the editor.
- **CLI** — the commands below.
- **MCP** — the same operations as agent tools at `/mcp`.

## Commands

```
# server
bubble serve [--addr :4006]      run the server (REST + MCP + web brain)
bubble stop                      stop the running server (graceful)
bubble attach                    follow the running server's log (Ctrl-C detaches)

# looking around
bubble ls                        list bubbles, hottest first (buoyancy view)
bubble heat <id>                 explain a bubble's temperature
bubble show <id>                 a bubble's thread timeline (git-log-oneline)
bubble thread <id> [--comments]  a thread's interior: artifacts, logbook, revisions
bubble search <query>            fuzzy-search threads across your instances
bubble whoami                    the identity resolved from your credential

# doing the work
bubble birth <id> [flags]        create a thread in a bubble (needs Brief + Logbook)
bubble logbook <id> [text|-]     rewrite a thread's Logbook (evidence → warms it)
bubble dod <id> [text|-]         rewrite a thread's Definition of Done
bubble section --id <id> --title <t> [--file <f>|-] [--delete]
                                 add / rewrite / remove a ## section of a thread
bubble todo <id> <n> done <text> tick the nth todo (text guards the position)
bubble revision <id> <title> [-] attach a revision artifact to a thread
bubble comment <id> <text...>    post a comment (as you; earns no heat)
bubble rename <id> <title>       retitle a thread or a revision
bubble move <thread> <bubble>    re-home a thread into another bubble

# structure
bubble workspace new|list|rename a Plane project — our Workspace
bubble bubble new|set|close|open create a bubble or set its contract (§4)
bubble bubble review|unreview    the explicit 👀 stage overlay
bubble page list|read|new|edit   a workspace's docs and specs (Plane pages)
bubble delete <kind> <id> [-y]   PERMANENTLY delete a workspace/bubble/thread/artifact

# attention
bubble notifications             your inbox of cooling/dormant alerts (alias: inbox)
bubble notifications on|off      opt in/out of notifications
bubble notifications read <id|all>  mark notifications read
bubble tick                      sweep now for cooling bubbles

# setup
bubble init [flags]              configure server URL + a credential profile
bubble use [name]                switch active credential profile (no arg: list)
bubble reset [--force]           purge all local state and start from scratch
bubble version

# server-host admin
bubble instance add|list|remove [--force]
bubble admin <cmd>               see below
```

`bubble birth` enforces the §3 birth rule: the Brief must contain a "Definition
of Done", and a Logbook is required unless you pass `--small`.

### Admin (godmode)

Authenticates with `$BUBBLE_ADMIN_TOKEN` (break-glass) or your active profile
when your email is an admin email. Also available in the web UI.

```
bubble admin instances               all instances, across orgs
bubble admin bubbles                 bubbles across ALL instances
bubble admin stats                   server health + Plane rate budget
bubble admin refresh                 flush server caches
bubble admin tick                    force a cooling sweep now
bubble admin tuning [set k=v|reset]  the buoyancy calibration
bubble admin autostate <inst> on|off write derived levels back to Plane (opt-in)
bubble admin kiosk ls|new|rm         read-only display tokens
bubble admin sync <inst>             mirror census + cursor (no Plane calls)
bubble admin sync-diff <inst>        compare the mirror against a live fetch
bubble admin sync-fidelity <inst>    what a write would do to mirrored bodies
bubble admin sync-backfill <inst>    force a complete re-walk of one instance
bubble admin sync-rebuild <inst>     drop the local mirror and rebuild it
bubble admin outbox [drop <id>]      writes that have not reached Plane
```

## Multiple Plane instances

Register each deployment under its own slug. Who sees what is **derived from
Plane membership** — a caller sees an instance only if their email is in that
Plane workspace, which is the isolation boundary between separate orgs:

```bash
bubble instance add --slug cuby --url https://plane.cuby.work \
    --key <KEY> --workspace <ws>          # whole workspace; or add --project <id>
```

A person who belongs to both `ayetec` and `cuby` (same email) sees both from a
single Plane key; someone in only one sees only that one.

**Whole workspace vs. pinned project:** omit `--project` and the instance
federates every project in the workspace; pass `--project <id>` to track just
one.

Ids are namespaced by instance — `<slug>:<project-id>` for a workspace,
`<slug>:<project-id>:<module-id>` for a bubble, `<slug>:<project-id>:<item-id>`
for a thread or page — so every operation routes to the right instance and
project automatically. The CLI accepts any unique prefix.

## Switching identities (credential profiles)

The unified cross-instance view requires the **same email** in each workspace.
If your Plane accounts use **different emails** (e.g. `you@gmail.com` on ayetec,
`you@corp.com` on cuby), one key can't show both — each key is a distinct
identity. Store one profile per identity and switch between them:

```bash
bubble init --name ayetec --token <ayetec-key> --server <url>
bubble init --name cuby   --token <cuby-key>   --server <url>

bubble use                # list profiles ('*' = active)
bubble use ayetec         # switch; now whoami/ls resolve as your ayetec identity
bubble use --tokens       # keys masked to a fingerprint (--reveal for full)
```

A profile carries **both** the credential and the server URL, so one `bubble use`
moves you between deployments too. Switching is instant and client-side.
`whoami` prints the active profile so you always know who you are.

## Agents as members (MCP)

The server exposes its **own** MCP endpoint at `/mcp` (not Plane's). An agent
**impersonates a human** by using that human's Plane API key as its Bearer
credential — so it acts as that person, with their identity, scope, and (on the
write path) Plane attribution. The framework's rules are enforced server-side,
so an agent cannot bypass them:

```bash
claude mcp add --transport http bubble http://localhost:4006/mcp \
    --header "Authorization: Bearer <your plane API key>"
```

The web UI's **Connect** panel generates that command for you, plus a setup
prompt, with the key masked until you ask to see it.

26 tools, grouped by what they do:

| | Tools |
|---|---|
| **Read** | `list_workspaces` `list_bubbles` `thread_timeline` `read_thread` `thread_comments` `list_pages` `read_page` |
| **Create** | `create_workspace` `create_bubble` `birth_thread` `add_revision` `create_page` |
| **Change** | `update_thread` `toggle_todo` `delete_artifact` `set_contract` `move_thread` `rename_workspace` `update_page` |
| **Discuss** | `post_comment` `mark_comments_read` |
| **End** | `close_bubble` `delete_bubble` `delete_thread` `delete_page` `delete_workspace` |

`birth_thread` is rejected without a Brief (carrying a Definition of Done) and a
Logbook. `update_thread` prefers surgical `edits` (quote the old text, give the
new) over wholesale section replacement, and `toggle_todo` refuses the write if
the item text no longer matches that position. Destructive tools require the
exact current name as confirmation.

> Trade-off: the agent is indistinguishable from the human it impersonates —
> no agent-level audit, and revoking it means rotating that human's Plane key.

## Development

```bash
go build ./...     # compile everything
go vet ./...       # static checks
go test ./...      # unit + server/store/sync/md tests

scripts/dev.sh          # rebuild + restart the local server, wait until healthy
scripts/dev.sh --web    # rebuild the web bundle first
```

The frontend uses **bun** (a stray `package-lock.json` would resolve different
versions and is git-ignored):

```bash
cd frontend
bun install
bun run build      # → ../web/dist, which go:embed picks up (committed)
bun run test       # vitest
bun run check      # svelte-check
bun run dev        # vite, proxying /api and /mcp to $BUBBLE_DEV_BACKEND
```

Layout:

```
cmd/bubble/        entrypoint + CLI subcommands
internal/
  domain/          shared types + DTOs
  heat/            pure temperature function (Hot/Warm/Cooling/Dormant)
  config/          client/server settings (XDG)
  store/           SQLite overlay: contracts, progress, instances, outbox
  mirror/          the local read model of Plane (L1)
  sync/            the single sync worker: backfill, delta, reconcile, diff
  plane/           Plane REST client + rate budget (the sole path to Plane)
  md/              Markdown ↔ Plane HTML: splice engine, lint, fidelity
  mcpapi/          our own MCP server (agent front door)
  server/          the brain: REST + MCP + policy engine + SSE
  client/          thin CLI client
web/               the embedded SPA bundle (built from frontend/)
frontend/          Svelte 5 + Skeleton 5 + Tailwind 4, built with bun
docs/              the design worksheets (see below)
```

## Documentation

| Doc | What it covers |
|-----|----------------|
| [`AGENTS.md`](./AGENTS.md) | the operating rules agents load (`CLAUDE.md` imports it) |
| [`docs/bubble-work-spec.md`](./docs/bubble-work-spec.md) | the canonical model, and the server/client design |
| [`docs/THREAD-LIFECYCLE.md`](./docs/THREAD-LIFECYCLE.md) | per-thread buoyancy, evidence signals, the bubble roll-up |
| [`docs/PLANE-SYNC.md`](./docs/PLANE-SYNC.md) | the mirror, the sync worker, the outbox, degraded mode |
| [`docs/ARTIFACT-EDITING.md`](./docs/ARTIFACT-EDITING.md) | editing a thread's page: splice engine, editor, workspaces, pages |
| [`docs/MCP-ACCESS.md`](./docs/MCP-ACCESS.md) | agents writing artifacts, live updates, how sign-in could work |
| [`docs/web-ui-design.md`](./docs/web-ui-design.md) | the original board design |
| [`DEPLOY.md`](./DEPLOY.md) | running it on a server, CI/CD, the Plane rate limit |

## Status

Working today, end to end: the server/client spine · Plane read **and** write
paths · multi-instance federation with per-member scoping · the SQLite mirror as
the read layer with an outbox for writes and degraded-mode reporting · derived
thread and bubble buoyancy with opt-in write-back to Plane · the notification
scheduler · the web board with in-place artifact editing · project pages ·
workspace and bubble lifecycle including deletes · 26 MCP tools · God Mode.

Still open, each tracked in its own worksheet:

- **Payload-aware webhooks** (`PLANE-SYNC.md` Phase 6). Today's webhook is a
  blind "refresh everything" trigger; the delta covers correctness on its own,
  so this buys freshness, not correctness.
- **A stable public HTTPS URL and an OAuth AS** (`MCP-ACCESS.md` steps 4 and 6),
  for hosted clients that can't paste a header.

The repository runs on its own framework: see [`AGENTS.md`](./AGENTS.md).
