# Bubble Work

A personal way-of-working where **attention behaves like buoyancy**: work that
produces evidence stays *hot* and rises; work that goes quiet *cools* and sinks.
Time is the governing force — your only standing job is to tend what floats at
the top.

Bubble Work ships as a single Go binary (`bubble`) that runs a small **server**
(the "brain") and a thin **client**. It sits on top of one or more
[Plane](https://plane.so) instances: Plane stays the system of record for the
actual work items, while the server owns the Bubble Work overlay — heat,
lifecycle, contracts, and membership.

> **The full model lives in [`bubble-work-spec.md`](./bubble-work-spec.md).**
> This README is how to run it. Build progress is tracked in
> [`IMPLEMENTATION-PLAN.md`](./IMPLEMENTATION-PLAN.md).

## Concepts (one line each)

| Term | Meaning | Plane mapping |
|------|---------|---------------|
| **Workspace** | boundary for a body of work | Project |
| **Bubble** | a durable grouping; the unit of *attention* | Module |
| **Thread** | one executable unit of work | Work item |
| **Cycle** | the repeating pulse heat is measured against | Cycle |
| **Heat** | evidence of *changed reality*, never mere activity | (derived) |

A bubble is **Hot → Warm → Cooling → Dormant → Closed**, computed from the
meaningful outputs on its threads. See the spec for the full rules.

## Architecture

```
   Humans (CLI)     Agents (Claude Code / Codex, via MCP)
        \                 /   auth as a member (own token)
         v               v
   ┌───────────────────────────────┐
   │       Bubble Work Server       │  ← the only thing clients talk to
   │  REST /api + MCP /mcp          │
   │  policy engine · heat · SQLite │
   └───────────────┬───────────────┘
                   │ REST only; sole Plane client
                   v
      Plane instances (system of record)
```

- **One binary, two modes:** `bubble serve` is the brain; the other subcommands
  are the thin client.
- **Members, not anonymous callers.** Every request authenticates as a member
  (human or agent) and is attributed.
- **Federates multiple Plane instances**, with members scoped per instance.
- **The server is Plane's only client, over REST only** — no client ever calls
  Plane directly or via Plane's MCP.

## Install

Requires Go 1.26+.

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

> **Humans don't get registered.** Your Plane API key *is* your credential —
> identity, admin role, and which instances you see all come from Plane. If you
> belong to several instances under the **same email**, one key gives you a
> unified view (the server matches you by email across instances). Different
> email per org? Use **credential profiles** + `bubble use` (see below).

To keep the server strictly local, bind loopback only:

```bash
bubble serve --addr 127.0.0.1:4006
```

## Commands

```
bubble serve [--addr :4006]          run the server (REST + MCP brain)
bubble ls                            list bubbles, hottest first (buoyancy view)
bubble heat <instance:project:mod>   explain a bubble's temperature
bubble whoami                        show the identity resolved from your credential
bubble notifications                 your inbox of cooling/dormant alerts (alias: inbox)
bubble notifications on|off          opt in/out of notifications
bubble notifications read <id|all>   mark notifications read
bubble tick                          sweep now for cooling bubbles
bubble birth <id> [flags]            create a thread in a bubble (needs Brief + Logbook)
bubble bubble set|close|open <id>    set a bubble's contract (§4) or open/close it
bubble use [name]                    switch active credential profile (no arg: list)
bubble init [--name --token …]       configure server URL + a credential profile
bubble reset [--force]               purge all local state and start from scratch
bubble version

# admin (operate on the local DB; run on the server host)
bubble instance add|list|remove [--force]
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

Bubble ids are namespaced `<instance-slug>:<project-id>:<module-id>` (e.g.
`ayetec:9f3c…:1a77…`), so `bubble heat` and close operations route to the right
instance and project automatically.

## Switching identities (credential profiles)

The unified cross-instance view requires the **same email** in each workspace.
If your Plane accounts use **different emails** (e.g. `you@gmail.com` on ayetec,
`you@corp.com` on cuby), one key can't show both — each key is a distinct
identity. Store one profile per identity and switch between them:

```bash
bubble init --name ayetec --token <ayetec-key>   # stores + activates "ayetec"
bubble init --name cuby   --token <cuby-key>      # stores + activates "cuby"

bubble use                # list profiles ('*' = active)
bubble use ayetec         # switch; now whoami/ls resolve as your ayetec identity
```

Switching is instant and client-side (it just changes which key you present — no
server restart). `init --token` without `--name` still works as a single
credential. `whoami` prints the active profile so you always know who you are.

## Agents as members (MCP)

The server exposes its **own** MCP endpoint at `/mcp` (not Plane's). An agent
**impersonates a human** by using that human's Plane API key as its Bearer
credential — so it acts as that person, with their identity, scope, and (on the
write path) Plane attribution. It gets the framework's tools (`list_bubbles`,
`birth_thread`, `close_bubble`), same rules enforced:

```bash
claude mcp add --transport http bubble http://localhost:4006/mcp
# use a human's Plane API key as the Bearer credential for that MCP server
```

> Trade-off: the agent is indistinguishable from the human it impersonates —
> no agent-level audit, and revoking it means rotating that human's Plane key.

## Development

```bash
go build ./...     # compile everything
go vet ./...       # static checks
go test ./...      # unit + server/store tests
```

Layout:

```
cmd/bubble/        entrypoint + CLI subcommands
internal/
  domain/          shared types + DTOs
  heat/            pure temperature function (Hot/Warm/Cooling/Dormant)
  config/          client/server settings (XDG)
  store/           SQLite overlay: members, contracts, instances, grants
  plane/           Plane REST client (the sole path to Plane)
  mcpapi/          our own MCP server (agent front door)
  server/          the brain: REST + MCP + policy engine
  client/          thin CLI client
```

## Status

Working today: the server/client spine, member authentication + attribution
(REST and MCP), and multi-instance federation with per-member scoping. The Plane
**read path** (live `bubble ls`) and **write path** (creating work items from a
thread birth) are in progress — see
[`IMPLEMENTATION-PLAN.md`](./IMPLEMENTATION-PLAN.md).

The repository runs on its own framework: see [`AGENTS.md`](./AGENTS.md).
