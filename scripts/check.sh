#!/usr/bin/env bash
# What CI gates, and what to run before asking anyone to look at a change.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

# Run tidy and REFUSE if it changed anything, rather than tidying silently. A
# pre-commit hook that edits go.mod after the files are staged would commit the
# untidy version while the working tree holds the fixed one — which is worse than
# either outcome on its own.
echo "==> go mod tidy"
before="$(cat go.mod go.sum 2>/dev/null | shasum)"
go mod tidy
after="$(cat go.mod go.sum 2>/dev/null | shasum)"
if [[ "$before" != "$after" ]]; then
  echo "go.mod/go.sum were not tidy. They have been fixed — stage them and run again." >&2
  exit 1
fi

echo "==> gofmt"
unformatted="$(gofmt -l . | grep -v '^dist/' || true)"
if [[ -n "$unformatted" ]]; then
  echo "not gofmt'd:"; echo "$unformatted"; exit 1
fi

echo "==> go vet"; go vet ./...
echo "==> build";   scripts/build.sh

# The UI is type-checked when the toolchain is present. Skipped rather than
# failing when it is not: a Go-only contributor should be able to run `just check`
# on a fresh clone, and the committed bundle means the binary builds without bun.
if [[ -d frontend/node_modules ]]; then
  echo "==> svelte-check"
  (cd frontend && bun run check) || exit 1
else
  echo "==> svelte-check (skipped: frontend/node_modules missing — \`just install\`)"
fi
scripts/test.sh
