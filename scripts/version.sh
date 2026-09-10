#!/usr/bin/env bash
# La versión que sigue, leída de los commits.
#
#   scripts/version.sh          -> imprime el tag siguiente, o NADA si no hay
#                                  nada que publicar
#
# Conventional Commits deciden el salto y nadie decide un número a mano:
# `BREAKING CHANGE` o `tipo!:` sube el mayor, `feat:` el menor, `fix:` y `perf:`
# el parche.
#
# La serie arranca en `v1.0.0`. No en 0.x: esto ya sostiene trabajo de verdad en
# instancias de verdad, y un 0.x dice a quien lo aloja que puede romperse sin
# aviso — que es justo lo contrario de lo que promete el resto de este modelo. Y
# no en v2: `v0-plane-as-record` es un tag con NOMBRE, el archivo de otra
# implementación, no la versión 0 de ésta.
#
# Silencio deliberado cuando no hay nada: `docs:`, `chore:` y `style:` no mueven
# un binario, y publicar una versión que no cambia nada enseña a la gente a
# ignorar las versiones.
set -euo pipefail
cd "$(dirname "$0")/.."

# El patrón lleva los tres números: el archivo de v0 se llama
# `v0-plane-as-record` y `v[0-9]*` lo habría cogido — no es una versión de esta
# serie, es un tag con nombre.
last="$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || true)"

range="HEAD"
[[ -n "$last" ]] && range="$last..HEAD"

# El cuerpo también: `BREAKING CHANGE:` vive ahí, que es donde lo pone la
# convención.
log="$(git log --format='%s%n%b%n--' "$range" 2>/dev/null || true)"
[[ -z "$log" ]] && exit 0

bump=""
while IFS= read -r line; do
  case "$line" in
    *"BREAKING CHANGE"*) bump="major" ;;
  esac
  # `tipo(scope)!: …` y `tipo!: …`
  if [[ "$line" =~ ^[a-z]+(\([^\)]+\))?\!: ]]; then bump="major"; fi
  if [[ -z "$bump" || "$bump" == "patch" ]]; then
    if [[ "$line" =~ ^feat(\([^\)]+\))?: ]]; then bump="minor"; fi
  fi
  if [[ -z "$bump" ]]; then
    if [[ "$line" =~ ^(fix|perf)(\([^\)]+\))?: ]]; then bump="patch"; fi
  fi
done <<< "$log"

[[ -z "$bump" ]] && exit 0

# La primera. Sale 1.0.0 pase lo que pase el commit: no hay nada anterior contra
# lo que medir un salto.
if [[ -z "$last" ]]; then
  echo "v1.0.0"
  exit 0
fi

if [[ "$last" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  ma="${BASH_REMATCH[1]}"; mi="${BASH_REMATCH[2]}"; pa="${BASH_REMATCH[3]}"
  case "$bump" in
    major) echo "v$(( ma + 1 )).0.0" ;;
    minor) echo "v${ma}.$(( mi + 1 )).0" ;;
    patch) echo "v${ma}.${mi}.$(( pa + 1 ))" ;;
  esac
  exit 0
fi

# Un tag con una forma que este script no sabe leer se dice en voz alta en vez de
# adivinarse: publicar sobre una serie que no se entiende es cómo se salta un
# número o se pisa uno.
echo "no sé leer el último tag: $last" >&2
exit 1
