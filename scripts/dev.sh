#!/usr/bin/env bash
# The dev session: the SERVER and the UI in one terminal, both dying on Ctrl-C.
#
# Two processes are the honest shape of this repo — the Go binary serves /api and
# /mcp, vite serves the frontend with hot reload and proxies the API back to it —
# and keeping them in two terminals meant the second one was routinely forgotten,
# left pointing at a server that had been rebuilt out from under it.
#
# So they run as children of this script, in ITS process group. Ctrl-C in the
# terminal signals that whole group, so both get the SIGINT at the same instant;
# the trap below is what cleans up when the signal arrives some other way (one of
# them crashing, a `kill` of this pid, the terminal closing).
#
# Logs are interleaved and tagged, because the interesting failures are the ones
# that span both: a 502 in vite's proxy is a server that did not come up.
#
# For the background server instead — the one `bubble attach` and `bubble stop`
# talk to — use `just daemon` (scripts/daemon.sh).
#
# Usage:
#   scripts/dev.sh            # server + UI
#   scripts/dev.sh --no-ui    # server only, still in the foreground
set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${PORT:-4006}"
UI_PORT="${UI_PORT:-5173}"
BIN="./dist/bubble"
WITH_UI=1
[[ "${1:-}" == "--no-ui" ]] && WITH_UI=0

server_pid=""
ui_pid=""

# cleanup runs on the way out however we got here. Each kill is best-effort: by
# the time a Ctrl-C reaches us the children have usually taken the same signal
# already, and killing a pid that is gone is not a failure.
cleanup() {
  trap - INT TERM EXIT
  echo
  echo "stopping…"
  # Written as if-blocks rather than `[[ … ]] && kill …`: under `set -e` a false
  # test is a failed command, so the tidy one-liner version exits the trap early
  # and leaves the other child running — which is the exact bug this script is
  # meant to prevent.
  if [[ -n "$ui_pid" ]]; then kill "$ui_pid" 2>/dev/null || true; fi
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; fi
  # Give the server its graceful window (it drains HTTP on SIGINT), then insist.
  local waited=0
  while [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null && [[ $waited -lt 60 ]]; do
    sleep 0.1
    waited=$((waited + 1))
  done
  if [[ -n "$server_pid" ]]; then kill -9 "$server_pid" 2>/dev/null || true; fi
  if [[ -n "$ui_pid" ]]; then kill -9 "$ui_pid" 2>/dev/null || true; fi
  wait 2>/dev/null || true
  return 0
}
trap cleanup INT TERM EXIT

# tag prefixes a stream so two logs in one terminal stay readable. `read` with an
# empty IFS keeps the line's own spacing, and the trailing `|| true` lets a
# partial last line through when the child dies mid-write.
tag() {
  local label="$1"
  while IFS= read -r line || [[ -n "$line" ]]; do
    printf '%s %s\n' "$label" "$line"
  done
}

echo "building…"
go build -o "$BIN" ./cmd/bubble

# A daemon from a previous `just daemon` holds the port AND the pidfile, and the
# server refuses to start a second time rather than fighting it. Stopping it here
# is the difference between "one command" and "one command plus a puzzle".
"$BIN" stop >/dev/null 2>&1 || true

# --verbose: in a foreground session the logs belong on the terminal. Without it
# they go only to the log file, which is right for the daemon and useless here.
# Process substitution rather than a pipe: `cmd | tag &` records the TAGGER's pid
# in $!, so the trap would politely kill the log prefixer and leave the server
# running on the port. This way $! is the server itself.
PORT="$PORT" "$BIN" serve --verbose > >(tag "[server]") 2>&1 &
server_pid=$!

if [[ $WITH_UI -eq 1 ]]; then
  # bun, not npm: the repo standardised on bun and package-lock.json is
  # git-ignored, so npm here resolves a different dependency tree than CI checks.
  (cd frontend && BUBBLE_DEV_BACKEND="http://127.0.0.1:${PORT}" exec bun run dev --port "$UI_PORT") \
    > >(tag "[ui]") 2>&1 &
  ui_pid=$!
fi

# Wait for health before printing the banner: a URL printed before the server is
# listening is a URL somebody clicks into an error page.
printf "waiting for :%s" "$PORT"
for _ in $(seq 1 40); do
  if curl -sf --max-time 1 "http://localhost:${PORT}/health" >/dev/null 2>&1; then
    echo
    cat <<MSG

  server   http://localhost:${PORT}      (REST /api, MCP /mcp)
$( [[ $WITH_UI -eq 1 ]] && echo "  web UI   http://localhost:${UI_PORT}      (hot reload, proxying to the server)" )
  MCP      claude mcp add --transport http bubble http://localhost:${PORT}/mcp \\
             --header "Authorization: Bearer <your plane.ayetec.space API key>"

  Ctrl-C stops both.

MSG
    break
  fi
  printf "."
  sleep 0.5
done

# Wait until EITHER child ends, then fall into cleanup: if the server dies there is
# nothing for the UI to proxy to, and a half-dead session that looks alive is worse
# than one that exits.
#
# Polled rather than `wait -n`, which needs bash 4.3 — macOS ships 3.2, and a dev
# script that only works with a homebrew bash is a dev script that greets half the
# team with a syntax error.
while :; do
  kill -0 "$server_pid" 2>/dev/null || break
  if [[ -n "$ui_pid" ]] && ! kill -0 "$ui_pid" 2>/dev/null; then
    break
  fi
  sleep 0.5
done
