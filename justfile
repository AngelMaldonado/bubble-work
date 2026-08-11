# Bubble Work — build and dev recipes.
#
# `just` with no arguments lists everything. The one thing worth internalising:
# the web bundle is EMBEDDED in the binary (web/embed.go, //go:embed all:dist),
# so `just build` alone will not show a frontend change. `just dist` is the
# honest "rebuild everything" — it builds the bundle first, then the binary that
# carries it.
#
# bun, never npm: the repo standardised on bun (frontend/package-lock.json is
# git-ignored), so a stray npm install resolves a different tree than CI checks.

bin := "dist/bubble"
port := env_var_or_default("PORT", "4006")

# List the recipes (default).
default:
    @just --list --unsorted

# ---- building ----

# Compile the binary only. Fast, and blind to frontend changes.
build:
    go build -o {{bin}} ./cmd/bubble

# Build the web bundle into web/dist (committed, so this belongs in your commit).
web:
    cd frontend && bun run build

# Bundle + binary: the full rebuild, and what you want after touching frontend/.
dist: web build
    @echo "built {{bin}} with a fresh bundle"

# Install the binary onto your PATH via go install.
install:
    go install ./cmd/bubble

# ---- running ----

# Rebuild and restart the local server (Go only), then wait until it is healthy.
dev:
    scripts/dev.sh

# Same, rebuilding the web bundle first — the one to use after a UI change.
dev-web:
    scripts/dev.sh --web

# Run the server in the foreground (PORT, default 4006).
serve: build
    {{bin}} serve --addr ":{{port}}"

# Stop the running server (graceful; a missing one is not an error).
stop:
    -{{bin}} stop

# Follow the running server's log. Ctrl-C detaches without stopping it.
logs:
    {{bin}} attach

# THE way to iterate on the UI: no bundle build, no Go build, no restart. Vite
# proxies /api, /mcp, /webhooks and /health to the server, so that has to be
# running already (`just dev`) — this serves the frontend only.

# Vite with hot reload, proxying the API to the running server.
ui:
    cd frontend && bun run dev

# ---- checking ----

# Everything CI gates, in the order it gates it. Run this before you commit.
check: fmt-check vet test web-check
    @echo "all clear"

# gofmt, as a check rather than a fix — the same command CI fails on.
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted="$(gofmt -l ./cmd ./internal)"
    if [ -n "$unformatted" ]; then
        echo "gofmt needed on:" >&2
        echo "$unformatted" >&2
        exit 1
    fi
    echo "gofmt clean"

# Rewrite what gofmt-check would complain about.
fmt:
    gofmt -w ./cmd ./internal

# go vet across every package.
vet:
    go vet ./...

# The Go test suite.
test:
    go test ./...

# One package, or one test: `just test-one internal/server TestPagesNest`
test-one pkg test="":
    go test ./{{pkg}}/ {{ if test != "" { "-run " + test } else { "" } }} -v

# Types and Svelte diagnostics. Not a build: `bun run build` happily emits a
# bundle for code this rejects, which is why CI runs both.

# svelte-check: types and Svelte diagnostics.
web-check:
    cd frontend && bun run check

# The frontend unit tests (vitest).
web-test:
    cd frontend && bun run test

# Install/refresh frontend dependencies from bun.lock.
deps:
    cd frontend && bun install

# ---- housekeeping ----

# web/dist is deliberately NOT touched: it is committed, so removing it would
# leave the tree dirty and the binary unbuildable until the next `just web`.

# Remove the built binary.
clean:
    rm -f {{bin}}
