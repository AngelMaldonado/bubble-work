#!/usr/bin/env bash
# What CI gates, and what to run before asking anyone to look at a change.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

echo "==> gofmt"
unformatted="$(gofmt -l . | grep -v '^dist/' || true)"
if [[ -n "$unformatted" ]]; then
  echo "not gofmt'd:"; echo "$unformatted"; exit 1
fi

echo "==> go vet"; go vet ./...
echo "==> build";   scripts/build.sh
scripts/test.sh
