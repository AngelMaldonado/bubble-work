#!/usr/bin/env bash
# `gh` donde no lo haya.
#
# `release.yml` y `backmerge.yml` lo usan. En un runner alojado de GitHub viene
# puesto; en el self-hosted de reko-3 no, y su ausencia no se nota hasta que un
# release muere DESPUÉS de empujar el tag (ya pasó una vez, ver release.yml). Se
# baja el binario oficial a $RUNNER_TEMP, sin sudo ni apt: el runner queda como
# estaba al terminar el job.
set -euo pipefail

if command -v gh >/dev/null 2>&1; then
  gh --version | head -1
  exit 0
fi

# Con el token del job: sin él, la API de GitHub da 60 peticiones por hora a toda
# la IP, y en reko-3 hay más cosas preguntando.
auth=()
[[ -n "${GH_TOKEN:-}" ]] && auth=(-H "Authorization: Bearer $GH_TOKEN")
v="$(curl -fsSL "${auth[@]}" https://api.github.com/repos/cli/cli/releases/latest | jq -r .tag_name)"
v="${v#v}"
arch="$(dpkg --print-architecture)" # amd64 | arm64: los nombres de los tarballs de gh

dir="${RUNNER_TEMP:-/tmp}/gh"
mkdir -p "$dir"
curl -fsSL "https://github.com/cli/cli/releases/download/v${v}/gh_${v}_linux_${arch}.tar.gz" |
  tar xz -C "$dir" --strip-components=1
echo "$dir/bin" >> "$GITHUB_PATH"
"$dir/bin/gh" --version | head -1
