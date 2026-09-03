#!/usr/bin/env bash
# Rebuild and restart the LOCAL server (~/.bubble, :4006 by default).
#
# The local server points at plane.ayetec.space, which is public — unlike
# plane.cuby.work and the deployed board, both of which live on 192.168.20.0/24
# and are unreachable from off the LAN. That is why local development happens
# against ayetec: it works from anywhere.
#
# Usage:
#   scripts/dev.sh          # rebuild, restart, wait until healthy
#   scripts/dev.sh --web    # also rebuild the frontend bundle first
set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${PORT:-4006}"
BIN="./dist/bubble"

if [[ "${1:-}" == "--web" ]]; then
  echo "building the web bundle…"
  # bun, not npm: the repo standardised on bun and package-lock.json is
  # git-ignored, so npm here resolves a different dependency tree than CI checks.
  (cd frontend && bun run build >/dev/null)
fi

echo "building…"
go build -o "$BIN" ./cmd/bubble

# Graceful stop; a missing server is not an error here.
"$BIN" stop >/dev/null 2>&1 || true

# nohup + disown so the server outlives this shell — otherwise closing the
# terminal (or a tool run finishing) takes it with you.
nohup "$BIN" serve >/dev/null 2>&1 &
disown || true

printf "waiting for :%s" "$PORT"
for _ in $(seq 1 40); do
  if curl -sf --max-time 1 "http://localhost:${PORT}/health" >/dev/null 2>&1; then
    echo
    curl -s "http://localhost:${PORT}/health"; echo
    cat <<MSG

  web UI   http://localhost:${PORT}
  MCP      claude mcp add --transport http bubble http://localhost:${PORT}/mcp \\
             --header "Authorization: Bearer <your plane.ayetec.space API key>"
  logs     ./dist/bubble attach
  stop     ./dist/bubble stop

MSG
    exit 0
  fi
  printf "."
  sleep 0.5
done
echo
echo "did not come up — check ./dist/bubble attach" >&2
exit 1
