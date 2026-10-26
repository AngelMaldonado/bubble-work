#!/usr/bin/env bash
# La furia de IA desde la terminal.
#
#   just fury listen [puerto]   receptor de ejemplo del webhook de los semáforos:
#                               verifica la firma y pinta una luz por persona.
#                               El secreto va en BUBBLE_FURY_SECRET.
#   just fury lab up|relay|test|state|logs|status|stop   lab HTTPS en reko-3.
set -euo pipefail
cd "$(dirname "$0")/.."
case "${1:-}" in
  listen) shift; exec python3 scripts/fury_receiver.py "$@" ;;
  lab)
    shift
    # shellcheck source=scripts/env.sh
    . scripts/env.sh
    exec python3 scripts/fury_lab.py "$@"
    ;;
  *) echo "usage: just fury listen [puerto] | lab up|relay|test|state|logs|status|stop" >&2; exit 1 ;;
esac
