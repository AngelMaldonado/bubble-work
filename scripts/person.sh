#!/usr/bin/env bash
# Create or update a PERSON — the account that works.
#
#   just person you@example.com <password> [lead|member]
#
# A superuser operates the box; it has no row in `users` and therefore nothing to
# attribute a write to. The same human usually wants both, with the same email:
# PocketBase keeps them in separate collections and they do not collide.
#
# This exists because until now the only way to make the first person was the
# PocketBase dashboard — which meant a fresh clone could reach the dashboard and
# could not sign in to the UI at all.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

email="${1:-}"
password="${2:-}"
role="${3:-member}"
if [[ -z "$email" || -z "$password" ]]; then
  echo "usage: just person <email> <password> [lead|member]" >&2
  exit 1
fi

[[ -x "$BUBBLE_BIN" ]] || scripts/build.sh >/dev/null

# Straight into the database rather than over HTTP: this has to work before the
# server is up, which is exactly when somebody needs it.
"$BUBBLE_BIN" person "$email" "$password" "$role" --dir "$BUBBLE_DATA"
