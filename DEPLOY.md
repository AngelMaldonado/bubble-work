# Deploying Bubble Work

Two instances run, and a push to `main` deploys to both independently:

| | reko | ayetec |
|---|---|---|
| URL | `https://bubble.angel.cubytest.space` | `https://bubble.ayetec.space` |
| Host | Mac mini, macOS arm64, LAN/tailnet | VPS, Debian x86_64, public |
| Reach | LAN or tailnet only | anywhere |
| Supervisor | pm2 | systemd |
| Plane | `plane.cuby.work` (LAN) | `plane.ayetec.space` (public) |
| Managed by | `~/dev/_infra` on the mini | Ansible, `ayetec/infra` |
| Workflow | `.github/workflows/deploy.yml` | `.github/workflows/deploy-ayetec.yml` |

They are deliberately uncoupled: ayetec is the instance reachable from
anywhere, so hanging its deploys off a test job pinned to a Mac mini on a LAN
would let an unreachable box block them.

The reko mount is documented first; [ayetec](#ayetec-vps) follows.

## Topology (reko)

```
  Your laptop ──Tailscale/LAN──▶ https://bubble.angel.cubytest.space
                                   │  (NPM: wildcard TLS → host.docker.internal:3104)
                                   ▼
                          angel-bubble-work (pm2)  ──REST──▶ Plane instances (LAN)
                          :3104  BUBBLE_HOME=…/.bubble

  git push main ─▶ GitHub Actions ─▶ self-hosted runner (mini) ─▶ deploy-service
```

## Service facts

| | |
|---|---|
| pm2 app | `angel-bubble-work` |
| Host | reko Mac mini, macOS arm64, Tailnet `100.88.150.73` |
| Code | `/Users/reko/dev-angel/bubble-work` (clone of `AngelMaldonado/bubble-work`) |
| Binary | built from source with Go (`/opt/homebrew/bin/go`) → `./bubble` |
| Port | `3104` (angel's range is 3100–3199) |
| State | `BUBBLE_HOME=/Users/reko/dev-angel/bubble-work/.bubble` (config + `bubble.db`) |
| Public URL | `https://bubble.angel.cubytest.space` (NPM + wildcard `*.angel.cubytest.space` TLS) |
| Health | `GET /health` → `{status, revision, built}` |

## Platform wiring (already in place)

- **PM2 ecosystem** — an `angel-bubble-work` app in `~/dev/_infra/pm2/ecosystem.config.js`
  (`interpreter: "none"` since it's a Go binary; `args: "serve"`; env sets `PORT=3104`
  and `BUBBLE_HOME`). Logs → `/dev/null` per platform convention.
- **Build step** — `~/dev/_infra/deploy/angel-bubble-work.sh`: `git pull --ff-only` +
  `go build -o bubble ./cmd/bubble`.
- **Registry** — an entry in `~/dev/_infra/services.yml` (hostname → `host.docker.internal:3104`,
  `health_url`).
- Backups of the shared files are saved as `*.bak-bubble`.

## CI/CD (push to deploy)

- A **self-hosted GitHub Actions runner** (`reko-bubble-work`, labels
  `self-hosted,reko,services,…`) runs on the mini as a launchd service.
- `.github/workflows/deploy.yml` triggers on push to `main` (docs-only pushes are
  ignored) and runs `deploy-service angel-bubble-work` on the runner →
  git pull + `go build` + `pm2 reload`.
- Verify a deploy: `curl -s https://bubble.angel.cubytest.space/health` — the
  `revision` should equal `main` HEAD.

**Manual deploy / rollback** (on the mini):
```bash
~/dev/_infra/bin/deploy-service angel-bubble-work        # rebuild + reload from main
# rollback: cd ~/dev-angel/bubble-work && git checkout <sha> && deploy-service angel-bubble-work
```

## Operating it (on the mini)

```bash
export BUBBLE_HOME=~/dev-angel/bubble-work/.bubble
cd ~/dev-angel/bubble-work

# instances (federated Plane connections) — server-side admin, local DB
./bubble instance list
./bubble instance add --slug <slug> --url <plane-url> --key <KEY> --workspace <ws>
./bubble instance remove <slug>

# pm2
~/dev/_infra/bin/services-pm2 logs angel-bubble-work
~/dev/_infra/bin/services-pm2 restart angel-bubble-work
~/dev/_infra/bin/services-status
```

Handy laptop alias for remote admin:
```bash
alias bubble-admin='ssh -t reko@100.88.150.73 "cd ~/dev-angel/bubble-work && BUBBLE_HOME=~/dev-angel/bubble-work/.bubble ./bubble"'
```

## ayetec VPS

`https://bubble.ayetec.space` — public, so it works from anywhere, unlike the
mini. Provisioned entirely from `ayetec/infra`: `just apply bubble` converges
the Go toolchain and the `bubble_work` role. Nothing here is configured by hand,
so the record of it is the role, not this file.

| | |
|---|---|
| Unit | `bubble-work.service` (systemd) |
| Host | `ayetec.space`, Debian x86_64, 2 vCPU / 3.8 GB, shared with Plane |
| Code | `/opt/bubble-work/src`, read-only deploy key |
| Binary | `/opt/bubble-work/bin/bubble`, built on the box |
| Listen | `172.18.0.1:4006` — the `proxy` bridge, not `0.0.0.0` |
| State | `BUBBLE_HOME=/opt/bubble-work/home` |
| Runner | `ayetec-bubble-work`, labels `ayetec,bubble-work` |

Three things differ from the mini in ways that matter:

- **Traefik routes it through the file provider.** A native process has no
  container for Docker labels to hang off, so the route is a watched file at
  `/opt/traefik/config/bubble-work.yml`.
- **Compiling is confined to a cgroup.** Builds share two cores and under four
  gigabytes with Plane, Postgres, Redis and MinIO, and `modernc.org/libc` — via
  the pure-Go SQLite driver — is one of the heaviest packages in the ecosystem
  to compile. `build-scope.sh` caps the compiler so overshooting kills the build
  and not a database. CI runs `go vet` and `go test` through it too.
- **Webhooks are possible here.** Plane rejects webhook targets that resolve to
  private IPs, which is what rules them out on the mini. Everything on ayetec is
  public, so the polling fallback below is a choice rather than a constraint.

```bash
# operate (on the VPS)
systemctl status bubble-work
journalctl -u bubble-work -f
sudo /opt/bubble-work/deploy.sh                 # rebuild + reload from the checkout

# rollback — self-contained, no GitHub in the loop
cd /opt/bubble-work/src && sudo -u angel git checkout <sha>
sudo /opt/bubble-work/deploy.sh                 # a detached HEAD is built as-is

# instances (server-side admin, local DB)
sudo -u angel BUBBLE_HOME=/opt/bubble-work/home /opt/bubble-work/bin/bubble instance list
```

The runner executes as a user with passwordless sudo, so **push access to
`main` is the trust boundary** — anything CI runs, runs as root on that host.
`deploy-ayetec.yml` therefore triggers only on `push` and `workflow_dispatch`,
never on `pull_request`.

## Plane rate limit

Plane allows **60 requests per minute per API key**, and it reports the state of
that limit on every response. The server budgets against those headers and
reserves the last 15 of the allowance for interactive calls, so background work
(the refresher, the tick, auto-state writes) stands aside when the budget runs
thin rather than 429-ing a page load. Watch it with:

```bash
./bubble admin stats     # remaining / limit, reset, 429s, background yields
```
…or the **Plane rate budget** card in God Mode.

If `429s` climbs or background yields are constant, the real fix is
[`docs/journal/PLANE-SYNC.md`](./docs/journal/PLANE-SYNC.md) — not needing the calls. As a
stopgap on a **self-hosted** Plane the ceiling itself is configurable; on the
Plane host set the API rate-limit env var (`API_KEY_RATE_LIMIT`, format
`number/timeunit`, e.g. `120/minute`) and restart it. This is a knob on *your*
Plane deployment, not on Bubble Work.

## Client setup (your laptop)

```bash
bubble init --server https://bubble.angel.cubytest.space
bubble init --name cuby --token <your-cuby-plane-key>   # a credential profile
bubble use cuby
bubble whoami
bubble ls
```
Works on the LAN or over the tailnet (both resolve/route to the box). A machine on
neither can't reach it — same boundary as before, nicer name.

## Real-time webhooks — deferred

Plane can push webhooks for near-instant `ls` freshness, but it is **not enabled**:
Plane's webhook validator rejects targets that resolve to **private IPs**, and both
Plane and this server sit on the LAN (`*.cubytest.space` → `192.168.20.x`). Polling
(60s cache + hourly cooling sweep) is the fallback and is plenty for this domain.

To enable later: route a **public** hostname through the box's Cloudflare tunnel
(e.g. `bubblehook.cubytest.space` → `http://localhost:3104`) so it resolves to a
public IP, then register `https://bubblehook.cubytest.space/webhooks/plane/<slug>`
in Plane and `bubble instance webhook <slug> --secret <secret>` on the mini.
