# Operations — install, run, tune, develop

Everything about *running* Bubble Work. The model is the
[README](../README.md); the design is [`ARCHITECTURE.md`](./ARCHITECTURE.md);
deploying to a server is [`DEPLOY.md`](../DEPLOY.md).

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

Your laptop can be **both server and client** — that is the default. Everything
lives in `~/.bubble/` (override with `$BUBBLE_HOME`): the client `config.json` and
the server's `bubble.db`. The client talks to `http://localhost:4006` out of the
box.

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
> email per org? Use credential profiles + `bubble use` (below).

To keep the server strictly local, bind loopback only:

```bash
bubble serve --addr 127.0.0.1:4006
```

## Multiple Plane instances

Register each deployment under its own slug. Who sees what is **derived from Plane
membership** — a caller sees an instance only if their email is in that Plane
workspace, which is the isolation boundary between separate orgs:

```bash
bubble instance add --slug cuby --url https://plane.cuby.work \
    --key <KEY> --workspace <ws>          # whole workspace; or add --project <id>
```

A person who belongs to both `ayetec` and `cuby` (same email) sees both from a
single Plane key; someone in only one sees only that one.

**Whole workspace vs. pinned project:** omit `--project` and the instance
federates every project in the workspace; pass `--project <id>` to track just one.

Ids are namespaced by instance — `<slug>:<project-id>` for a workspace,
`<slug>:<project-id>:<module-id>` for a bubble, `<slug>:<project-id>:<item-id>`
for a thread or page — so every operation routes to the right instance and project
automatically. The CLI accepts any unique prefix. Details in
[`modules/federation.md`](./modules/federation.md).

## Switching identities (credential profiles)

The unified cross-instance view requires the **same email** in each workspace. If
your Plane accounts use **different emails** (e.g. `you@gmail.com` on ayetec,
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
moves you between deployments too. Switching is instant and client-side. `whoami`
prints the active profile so you always know who you are.

## Admin (godmode)

Authenticates with `$BUBBLE_ADMIN_TOKEN` (break-glass) or your active profile when
your email is an admin email. Also available in the web UI.

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
bubble admin sync-fidelity <inst>    how faithfully Plane displays what we published
bubble admin sync-backfill <inst>    force a complete re-walk of one instance
bubble admin sync-rebuild <inst>     drop the local mirror and rebuild it
bubble admin outbox [drop <id>]      writes that have not reached Plane
bubble admin export <inst>           write the work to disk as markdown (--dir <abs>)
bubble admin adopt <inst>            take its bodies into the document store (once)
```

The calibration knobs are documented in [`modules/heat.md`](./modules/heat.md).

> `sync-rebuild` rebuilds the **mirror** — Plane's structure, states, members and
> comments. It does not rebuild artifact bodies, which Bubble Work owns rather than
> mirrors ([`modules/documents.md`](./modules/documents.md)). That is what makes
> `bubble admin export` a real obligation rather than a nicety: it is the copy that
> leaves this machine.

## Development

There is a [`justfile`](../justfile); `just` on its own lists every recipe.

```bash
just dist          # the web bundle AND the binary — the full rebuild
just build         # binary only (fast, and blind to frontend changes)
just check         # gofmt + vet + go test + svelte-check, exactly what CI gates

just dev           # THE dev session: server + UI in one terminal, Ctrl-C stops both
just dev-server    # server only, still in the foreground
just daemon        # the BACKGROUND server instead (what `bubble attach`/`stop` talk to)
just daemon-web    # ...rebuilding the web bundle first
just ui            # vite alone, proxying the API to an already-running server
just logs          # follow the background server's log
```

`just dev` runs both processes as children of one script, in its process group, so
Ctrl-C reaches both at once; the trap cleans up when the signal arrives some other
way. Server logs are tagged `[server]`, vite's `[ui]`. Ports: `PORT` (4006) and
`UI_PORT` (5173). **Open the UI on `UI_PORT`** while developing — that is the one
with hot reload; the server's own port serves the last built bundle.

The one thing worth internalising: **the web bundle is embedded in the binary**
(`web/embed.go`, `//go:embed all:dist`), so `just build` will not show a frontend
change. Use `just dist`, or `just ui` while iterating. `web/dist` is committed,
which is why a plain `go build` needs no JS toolchain — and why a UI change belongs
in your commit as a rebuilt bundle.

The frontend uses **bun** (a stray `package-lock.json` would resolve different
versions and is git-ignored). The underlying commands, if you would rather not go
through `just`:

```bash
cd frontend
bun install
bun run build      # → ../web/dist, which go:embed picks up (committed)
bun run test       # vitest
bun run check      # svelte-check
bun run dev        # vite, proxying /api and /mcp to $BUBBLE_DEV_BACKEND
```

### Layout

```
cmd/bubble/        entrypoint + CLI subcommands
internal/
  domain/          shared types + DTOs
  heat/            pure temperature function (Hot/Warm/Cooling/Dormant)
  config/          client/server settings (XDG)
  store/           SQLite overlay: documents, contracts, progress, instances, outbox
  mirror/          the local read model of Plane (L1)
  sync/            the single sync worker: backfill, delta, reconcile, diff
  plane/           Plane REST client + rate budget (the sole path to Plane)
  md/              Markdown: splice engine, lint, and the Plane render/import path
  mcpapi/          our own MCP server (agent front door)
  server/          the brain: REST + MCP + policy engine + SSE
  client/          thin CLI client
prompts/           the MCP prompts, as markdown files embedded into the binary
web/               the embedded SPA bundle (built from frontend/)
frontend/          Svelte 5 + Skeleton 5 + Tailwind 4, built with bun
docs/              the model's documentation (see docs/README.md)
scripts/dev.sh     server + UI in one foreground session (what `just dev` runs)
scripts/daemon.sh  the background server (what `just daemon` runs)
justfile           the build/dev recipes — `just` lists them
```

Each package's normative documentation is the matching file in
[`modules/`](./modules).
