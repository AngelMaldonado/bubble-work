# Deploying Bubble Work

The production instance runs on the **reko Mac mini** services platform under
angel's context. This documents the mount so it's reproducible.

## Topology

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
