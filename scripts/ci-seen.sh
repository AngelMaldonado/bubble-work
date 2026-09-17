#!/usr/bin/env bash
# ¿Este árbol ya pasó `scripts/check.sh` en el CI?
#
# Escribe en $GITHUB_OUTPUT `tree`, `ref` (dónde vive la marca) y `hit=1` si la
# marca existe y apunta a un commit con ESTE árbol. La marca la deja el CI al
# pasar, empujando `HEAD` a `refs/ci/check/<árbol>`: una ref que un clon normal
# no descarga, en el mismo remoto que el código, y que un push a main puede leer
# aunque la haya escrito un PR.
set -euo pipefail

tree="$(git rev-parse 'HEAD^{tree}')"
ref="refs/ci/check/$tree"
hit=0

if marked="$(git ls-remote --exit-code origin "$ref" 2>/dev/null | cut -f1)" && [[ -n "$marked" ]]; then
  # No basta con que exista: se comprueba que el commit marcado tiene de verdad
  # este árbol. Una ref con el nombre correcto apuntando a otra cosa no es un
  # check que pasó.
  if git fetch --quiet --no-tags origin "$ref" &&
     [[ "$(git rev-parse 'FETCH_HEAD^{tree}')" == "$tree" ]]; then
    hit=1
    echo "el árbol $tree ya pasó el check en $marked; no se repite"
  fi
fi

[[ "$hit" == 1 ]] || echo "el árbol $tree no ha pasado el check todavía"

{
  echo "tree=$tree"
  echo "ref=$ref"
  echo "hit=$hit"
} >> "${GITHUB_OUTPUT:-/dev/stdout}"
