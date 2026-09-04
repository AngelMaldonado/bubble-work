# Bubble Work — the command surface for developing it.
#
# Every recipe is a thin call into scripts/, so the same thing runs from a
# terminal, from CI and from an editor task without three copies of the logic.
# Configuration comes from .env (see .env.example); scripts/env.sh holds the
# defaults, so none of this needs a .env to work.

# List the recipes.
default:
    @just --list

# ---- running ----

# THE dev session: build, then run every service in a tmux session and attach.
dev:
    scripts/dev.sh

# Stop the dev session and everything in it.
stop:
    scripts/stop.sh

# ---- building ----

# Compile the binary to $BUBBLE_BIN (default ./dist/bubble).
build:
    scripts/build.sh

# Install the frontend toolchain. bun, never npm: package-lock.json is ignored,
# so npm resolves a different tree than anything else here.
install:
    cd frontend && bun install

# The bundle is COMMITTED, so `just build` needs no JS toolchain and a UI change
# is invisible until this runs.

# Rebuild the SPA into web/dist.
bundle:
    cd frontend && bun run build

# ---- checking ----

# Run once per clone. `git commit --no-verify` is the escape hatch.

# Point git at .githooks, so `just check` runs before every commit.
hooks:
    git config core.hooksPath .githooks
    @echo 'hooks installed - just check now runs on every commit'

# gofmt + vet + build + test. What CI gates.
check:
    scripts/check.sh

# Every test: the Go ones, plus the ones that need a running server.
test:
    scripts/test.sh

# ---- database ----

# `just migrate up`, `just migrate down 1`, `just migrate history-sync`.
# `down` asks for confirmation on the terminal, so it cannot run unattended.

# Run migrations against $BUBBLE_DATA without starting the server.
migrate *ARGS:
    scripts/build.sh
    {{env('BUBBLE_BIN', './dist/bubble')}} migrate {{ARGS}} --dir {{env('BUBBLE_DATA', './pb_data')}}

# Create or update a superuser. `just superuser upsert you@example.com <password>`
superuser *ARGS:
    scripts/build.sh
    {{env('BUBBLE_BIN', './dist/bubble')}} superuser {{ARGS}} --dir {{env('BUBBLE_DATA', './pb_data')}}

# ---- cleaning ----

# Remove build output. Leaves the data directory alone, on purpose.
clean:
    rm -rf dist

# Delete the LOCAL DATABASE and start over. Asks first.
[confirm("This deletes the local database. Type yes to continue:")]
reset:
    rm -rf {{env('BUBBLE_DATA', './pb_data')}}
    @echo "gone. `just dev` will migrate a fresh one."
