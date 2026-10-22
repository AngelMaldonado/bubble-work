#!/usr/bin/env bash
# Datos de ejemplo en el servidor de DESARROLLO.
#
#   just seed tags         equipos, proyectos e hilos «Demo» con tags, para la vista Blobs
#   just seed docs         lo mismo en grande: ~450 documentos con frontmatter en 3 equipos
#   just seed agents       personas y agentes simulados (@simulado.test) para Usuarios y
#                          Tokens; se borran solos al bajar el dev (`bubble simulated purge`)
#
# Escribe por la API del server local (BUBBLE_HTTP) como la persona de .env;
# nunca contra otra instancia. Correrlo otra vez no duplica nada.
# shellcheck source=./env.sh
. "$(dirname "$0")/env.sh"

what="${1:-}"
case "$what" in
  tags)
    BUBBLE_HTTP="$BUBBLE_HTTP" \
    BUBBLE_DEV_SUPERUSER_EMAIL="${BUBBLE_DEV_SUPERUSER_EMAIL:-}" \
    BUBBLE_DEV_SUPERUSER_PASSWORD="${BUBBLE_DEV_SUPERUSER_PASSWORD:-}" \
      python3 scripts/seed_tags.py
    ;;
  docs)
    BUBBLE_HTTP="$BUBBLE_HTTP" \
    BUBBLE_DEV_SUPERUSER_EMAIL="${BUBBLE_DEV_SUPERUSER_EMAIL:-}" \
    BUBBLE_DEV_SUPERUSER_PASSWORD="${BUBBLE_DEV_SUPERUSER_PASSWORD:-}" \
      python3 scripts/seed_docs.py
    ;;
  agents)
    BUBBLE_HTTP="$BUBBLE_HTTP" \
    BUBBLE_DEV_SUPERUSER_EMAIL="${BUBBLE_DEV_SUPERUSER_EMAIL:-}" \
    BUBBLE_DEV_SUPERUSER_PASSWORD="${BUBBLE_DEV_SUPERUSER_PASSWORD:-}" \
      python3 scripts/seed_agents.py
    ;;
  *)
    echo "usage: just seed tags | just seed docs | just seed agents" >&2
    exit 1
    ;;
esac
