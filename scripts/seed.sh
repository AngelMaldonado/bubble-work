#!/usr/bin/env bash
# Datos de ejemplo en el servidor de DESARROLLO.
#
#   just seed              todo lo de abajo, en orden
#   just seed tags         equipos, proyectos e hilos «Demo» con tags, para la vista Blobs
#   just seed docs         lo mismo en grande: ~450 documentos con frontmatter en 3 equipos
#   just seed fury         «Demo Furia»: hilos y burbujas en cada nivel de furia, y la
#                          enciende en el dev
#   just seed agents       personas y agentes simulados (@simulado.test) para Usuarios y
#                          Tokens; se borran solos al bajar el dev (`bubble simulated purge`)
#   just seed artifacts    un interrogatorio y un markdown efímero «Demo: …»
#
# Escribe por la API del server local (BUBBLE_HTTP) como la persona de .env;
# nunca contra otra instancia. Correrlo otra vez no duplica nada.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

export BUBBLE_HTTP
export BUBBLE_DEV_SUPERUSER_EMAIL="${BUBBLE_DEV_SUPERUSER_EMAIL:-}"
export BUBBLE_DEV_SUPERUSER_PASSWORD="${BUBBLE_DEV_SUPERUSER_PASSWORD:-}"

ALL=(tags docs fury agents artifacts)

one() {
  case "$1" in
    tags) python3 scripts/seed_tags.py ;;
    docs) python3 scripts/seed_docs.py ;;
    fury) python3 scripts/seed_fury.py ;;
    agents) python3 scripts/seed_agents.py ;;
    artifacts) python3 scripts/seed_artifacts.py ;;
    *)
      echo "usage: just seed [${ALL[*]// / | }]" >&2
      exit 1
      ;;
  esac
}

if [[ $# -eq 0 ]]; then
  for what in "${ALL[@]}"; do
    echo "==> seed $what"
    one "$what"
  done
else
  for what in "$@"; do one "$what"; done
fi
