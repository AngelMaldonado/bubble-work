#!/usr/bin/env bash
# Dar de alta un repositorio en la VM de CI (ver deploy/CI.md).
#
#   deploy/add-runner.sh AngelMaldonado/bubble-work
#   REKO_CI_HOST=root@172.31.99.32 deploy/add-runner.sh otro/repo
#
# Un agente de Actions se registra POR REPOSITORIO —GitHub no tiene runners a
# nivel de cuenta personal, sólo de organización— así que cada repo tiene el
# suyo dentro de la misma VM. Un agente en espera no consume nada.
#
# Todos llevan la etiqueta `reko-ci`, que nombra la MÁQUINA. Los workflows piden
# `runs-on: [self-hosted, reko-ci]` y no les importa cuál de los agentes los
# atiende.
set -euo pipefail

REPO="${1:-}"
HOST="${REKO_CI_HOST:-root@172.31.99.32}"
[[ -z "$REPO" ]] && { echo "uso: $0 <owner/repo>   (opcional: REKO_CI_HOST=user@ip)" >&2; exit 1; }

NAME="$(echo "$REPO" | tr '/' '-')"
# arm64 porque la VM es aarch64 nativa. Ver deploy/CI.md.
PKG="actions-runner-linux-arm64"

echo "==> token de registro para $REPO"
TOKEN="$(gh api -X POST "repos/$REPO/actions/runners/registration-token" -q .token)"

echo "==> instalando el agente $NAME en $HOST"
ssh "$HOST" "set -eu
  su - runner -c '
    set -eu
    dir=\$HOME/runners/$NAME
    # Ya dado de alta: reconfigurarlo encima deja dos agentes con el mismo
    # nombre, y GitHub reparte los trabajos entre ellos al azar.
    if [ -d \"\$dir\" ]; then echo \"ya existe: \$dir\"; exit 1; fi
    mkdir -p \"\$dir\" && cd \"\$dir\"
    # La última versión del agente, preguntada a GitHub: fijarla aquí sería un
    # número que caduca en silencio.
    v=\$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest | python3 -c \"import sys,json;print(json.load(sys.stdin)[\\\"tag_name\\\"].lstrip(\\\"v\\\"))\")
    curl -fsSL https://github.com/actions/runner/releases/download/v\$v/$PKG-\$v.tar.gz | tar xz
    ./config.sh --url https://github.com/$REPO --token $TOKEN \
      --name $NAME --labels reko-ci --unattended --replace
  '
  cd /home/runner/runners/$NAME && ./svc.sh install runner && ./svc.sh start"

echo "==> registrados en $REPO:"
gh api "repos/$REPO/actions/runners" \
  -q '.runners[] | "  " + .name + " · " + .status + " · " + ([.labels[].name] | join(","))'
