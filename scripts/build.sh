#!/usr/bin/env bash
# Build the binary. One command, no toolchain but Go.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

# -trimpath so the binary does not carry this machine's paths, and the version
# comes from git so a deployed binary can say what it is.
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo unknown)"

mkdir -p "$(dirname "$BUBBLE_BIN")"
go build -trimpath -ldflags "-X main.version=${VERSION}" -o "$BUBBLE_BIN" .
echo "built $BUBBLE_BIN  ($VERSION)"
