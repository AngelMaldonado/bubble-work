# Sourced by every script in here. Not executable on its own.
#
# Precedence, highest first:
#
#   1. the environment          BUBBLE_HTTP=:9000 just dev
#   2. .env                     what this machine changed
#   3. the defaults below       what a fresh clone runs with
#
# The environment has to win. A file that silently overrides what you typed on
# the command line is a file that makes you doubt the command line — and that is
# exactly the bug this ordering was written to fix.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

# What was set explicitly, before .env gets a say. Captured rather than listed,
# so a variable added later is covered without touching this line.
_bubble_explicit="$(env | grep '^BUBBLE_' || true)"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

# Put the explicit values back on top of whatever .env said.
if [[ -n "$_bubble_explicit" ]]; then
  while IFS= read -r kv; do [[ -n "$kv" ]] && export "$kv"; done <<< "$_bubble_explicit"
fi
unset _bubble_explicit

# Defaults, applied last and only where nothing above set a value.

# Where the server listens.
BUBBLE_HTTP="${BUBBLE_HTTP:-127.0.0.1:8090}"

# The PocketBase data directory: the database, backups and file storage.
#
# NOT under dist/. PocketBase defaults this to a directory beside the executable,
# which for us is dist/pb_data — and dist/ is a build output somebody will
# eventually delete. Data next to a build artifact is data waiting to be lost.
BUBBLE_DATA="${BUBBLE_DATA:-./pb_data}"

# Where the markdown tree lives: one git repository per workspace. Beside the
# database rather than inside it, so a person can open it with their own git.
BUBBLE_REPOS="${BUBBLE_REPOS:-./repos}"

# Where vite serves the UI in development. THIS is the one to open while
# developing: the server's own port serves the last BUILT bundle.
BUBBLE_UI_PORT="${BUBBLE_UI_PORT:-5173}"

# Where the binary is built.
BUBBLE_BIN="${BUBBLE_BIN:-./dist/bubble}"

# The tmux session `just dev` runs the services in, and `just stop` kills.
BUBBLE_TMUX="${BUBBLE_TMUX:-bubble}"

# Dev only: `just dev` creates this superuser if it does not exist, so a fresh
# clone reaches the dashboard without a separate step. Ignored by every other
# script, and never used against anything but a local data directory.
BUBBLE_DEV_SUPERUSER_EMAIL="${BUBBLE_DEV_SUPERUSER_EMAIL:-}"
BUBBLE_DEV_SUPERUSER_PASSWORD="${BUBBLE_DEV_SUPERUSER_PASSWORD:-}"

export BUBBLE_HTTP BUBBLE_DATA BUBBLE_REPOS BUBBLE_BIN BUBBLE_TMUX BUBBLE_UI_PORT
