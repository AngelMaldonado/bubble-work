#!/usr/bin/env bash
# The dev session: every service in one tmux session, one window each.
#
# tmux rather than a foreground process because of what v0 learned: the second
# process is the one nobody remembers. There is one service today and there will
# be two once the SPA lands, and a session you attach to cannot leave one of them
# running against a server that was rebuilt out from under it — `just stop` takes
# the whole thing down, and reattaching shows you both.
#
# Go has no hot reload: after a change to .go, `just dev` again. It rebuilds and
# replaces the running session rather than stacking a second one on the same port.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

# Build FIRST, outside tmux. A compile error belongs on the terminal you are
# looking at, not buried in a pane you have to go find.
scripts/build.sh

# The superuser is part of starting the session, not a nicety beside it: without
# one there is no way into the dashboard, so a failure here ABORTS rather than
# warns. An earlier version sent this to /dev/null and carried on, which started a
# server nobody could sign in to and turned a one-line fix (`password: Must be at
# least 8 character(s)`) into a debugging session.
#
# `upsert` creates or updates, so this is safe to run on every start and is how a
# changed password in .env takes effect. Leave both variables blank to skip it —
# that is the deliberate "I manage superusers myself" case, and it is not an error.
if [[ -n "$BUBBLE_DEV_SUPERUSER_EMAIL" && -n "$BUBBLE_DEV_SUPERUSER_PASSWORD" ]]; then
  if out="$("$BUBBLE_BIN" superuser upsert \
      "$BUBBLE_DEV_SUPERUSER_EMAIL" "$BUBBLE_DEV_SUPERUSER_PASSWORD" \
      --dir "$BUBBLE_DATA" 2>&1)"; then
    echo "superuser  $BUBBLE_DEV_SUPERUSER_EMAIL  (from .env)"
  else
    cat >&2 <<ERR

  the superuser in .env was NOT created or updated, so nothing would be able to
  sign in. Not starting.

      ${out##*: }

  fix BUBBLE_DEV_SUPERUSER_* in .env and run \`just dev\` again, or clear both to
  manage superusers yourself with \`just superuser\`.

ERR
    exit 1
  fi
fi

banner() {
  cat <<MSG

  server     http://${BUBBLE_HTTP}/
  dashboard  http://${BUBBLE_HTTP}/_/
  data       ${BUBBLE_DATA}
  session    ${BUBBLE_TMUX}
MSG
}

if ! command -v tmux >/dev/null 2>&1; then
  # Still works without tmux, and says so rather than failing on a missing tool.
  echo "tmux not found — running the server in the foreground instead." >&2
  banner
  echo "  Ctrl-C stops it."; echo
  exec "$BUBBLE_BIN" serve --http "$BUBBLE_HTTP" --dir "$BUBBLE_DATA" --dev
fi

# A rebuild has to replace the running services, not race them for the port.
tmux kill-session -t "$BUBBLE_TMUX" 2>/dev/null || true

# --dev prints logs and SQL into the pane, which is the whole reason to look at
# one. remain-on-exit keeps a crashed service's last output on screen instead of
# closing the window on the error you needed to read.
tmux new-session -d -s "$BUBBLE_TMUX" -n server \
  "$BUBBLE_BIN serve --http $BUBBLE_HTTP --dir $BUBBLE_DATA --dev"
tmux set-option -t "$BUBBLE_TMUX" -w remain-on-exit on >/dev/null

banner
cat <<MSG
  detach     Ctrl-b d          (services keep running)
  stop       just stop

MSG

# Attach only from a terminal, so this stays runnable from a script or CI. Inside
# an existing tmux, attaching would nest one session in another — switch instead.
if [[ ! -t 0 ]]; then
  echo "  (not a terminal — session left running detached)"; echo
elif [[ -n "${TMUX:-}" ]]; then
  tmux switch-client -t "$BUBBLE_TMUX"
else
  tmux attach-session -t "$BUBBLE_TMUX"
fi
