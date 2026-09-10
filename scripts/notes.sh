#!/usr/bin/env bash
# Las notas de la versión, escritas por los commits que la componen.
#
#   scripts/notes.sh [tag-anterior]
#
# Agrupadas por tipo y con la FRASE intacta. La convención restringe el prefijo y
# nada más — «feat: un documento no decía cuánto había cambiado» dice qué estaba
# mal, que es más de lo que dice un imperativo — así que aquí sólo se quita el
# prefijo y se ordena. Un changelog escrito a mano se escribe dos veces y se
# escribe peor la segunda.
set -euo pipefail
cd "$(dirname "$0")/.."

last="${1:-$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || true)}"
range="HEAD"
[[ -n "$last" ]] && range="$last..HEAD"

# `if` y no `[[ … ]] && { … }`, que es lo que hizo fallar el primer release.
#
# Con `set -e`, una función cuyo ÚLTIMO comando es un `[[ ]]` falso devuelve 1, y
# una función que devuelve 1 mata el script. Una sección vacía —ningún `fix:` en
# esta versión, por ejemplo— es un caso normal, no un error, y estaba abortando
# la publicación entera.
section() { # $1 título, $2 regex de tipos
  local body
  body="$(git log --format='%s' "$range" | sed -nE "s/^($2)(\([^)]+\))?!?: (.*)/- \3/p")"
  if [[ -n "$body" ]]; then
    echo "### $1"
    echo
    echo "$body"
    echo
  fi
}

breaking="$(git log --format='%s%n%b%n--' "$range" | sed -nE 's/^BREAKING CHANGE: (.*)/- \1/p')"
if [[ -n "$breaking" ]]; then
  echo "### Rompe algo"
  echo
  echo "$breaking"
  echo
  echo "Esto es un binario autoalojado, así que lo que puede doler es tu base de"
  echo "datos y tus agentes: una migración que no se puede revertir, o una tool de"
  echo "MCP que desapareció. Haz una copia antes de actualizar."
  echo
fi

section "Nuevo" "feat"
section "Arreglado" "fix|perf"
section "Por dentro" "refactor|build|ci|test"

# La PRIMERA versión no tiene desde dónde: no hay ninguna anterior, y decirlo
# así —callándolo— es más honesto que inventar un punto de partida.
if [[ -n "$last" ]]; then
  echo "Desde \`$last\`."
fi

# Explícito, y por la misma razón: el último comando de un script es su código de
# salida, y aquí terminar sin nada que decir es éxito.
exit 0
