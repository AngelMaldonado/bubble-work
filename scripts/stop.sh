#!/usr/bin/env bash
# Stop the dev session and everything running in it.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"
# shellcheck source=./simulated.sh
. "$(dirname "$0")/simulated.sh"

if tmux kill-session -t "$BUBBLE_TMUX" 2>/dev/null; then
  echo "stopped $BUBBLE_TMUX"
else
  echo "$BUBBLE_TMUX is not running"
fi
# Las personas de `just seed agents` no sobreviven al dev que las tenía.
purge_simulated
