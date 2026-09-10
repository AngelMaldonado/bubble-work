#!/usr/bin/env bash
# Stop the dev session and everything running in it.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

if tmux kill-session -t "$BUBBLE_TMUX" 2>/dev/null; then
  echo "stopped $BUBBLE_TMUX"
else
  echo "$BUBBLE_TMUX is not running"
fi
