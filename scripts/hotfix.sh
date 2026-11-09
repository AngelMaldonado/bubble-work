#!/usr/bin/env bash
# Un arreglo a lo publicado sin tocar lo que está a medias en `wip`.
#
#   scripts/hotfix.sh start   -> el worktree `hotfix` (../bubble-work-hotfix),
#                                limpio y en origin/main
#   scripts/hotfix.sh ship    -> empuja `hotfix` y abre (o enseña) el PR a main
#
# El camino entero:
#
#   start ─▶ commits `fix:` ─▶ ship ─▶ PR hotfix→main (check) ─▶ merge
#         ─▶ release (CI) ─▶ PR main→wip (CI, backmerge.yml) ─▶ merge cuando toque
#
# Un worktree y no un `git switch` en el mismo directorio: el trabajo a medias de
# `wip` no se guarda en un stash ni se mueve, se queda donde está.
#
# El worktree lleva su propio `.env` con otros puertos y otra sesión tmux, para
# que `just dev` y el hook de commit corran a la vez en los dos sin pisarse.
set -euo pipefail

main_wt="$(git worktree list --porcelain | awk 'NR==1 {print $2}')"
default_wt="$(dirname "$main_wt")/$(basename "$main_wt")-hotfix"

# Dónde está `hotfix` ahora mismo, si está en algún worktree.
hotfix_wt() {
  git worktree list --porcelain |
    awk '/^worktree / {wt=$2} /^branch refs\/heads\/hotfix$/ {print wt}'
}

die() { echo "hotfix: $*" >&2; exit 1; }

# ¿Lo que tiene `hotfix` encima de main ya entró? Por PR mergeado con ESTE tip, y
# no por ascendencia: un squash deja los commits fuera de main aunque el arreglo
# ya esté dentro.
landed() {
  local tip; tip="$(git rev-parse hotfix)"
  [[ -z "$(git rev-list origin/main..hotfix)" ]] && return 0
  command -v gh >/dev/null 2>&1 || return 1
  [[ "$(gh pr list --head hotfix --base main --state merged \
         --json headRefOid --jq "map(select(.headRefOid == \"$tip\")) | length")" -gt 0 ]]
}

start() {
  git fetch --quiet origin main
  local wt; wt="$(hotfix_wt)"

  if [[ -z "$wt" ]]; then
    if git show-ref --verify --quiet refs/heads/hotfix; then
      landed || die "la rama hotfix tiene commits que no están en main; ciérrala a mano antes de empezar otro"
      git worktree add "$default_wt" hotfix
    else
      # Sin upstream: un `git push` sin argumentos no tiene a dónde ir, y mucho
      # menos a main. El upstream lo pone `ship`, y es origin/hotfix.
      git worktree add --no-track -b hotfix "$default_wt" origin/main
    fi
    wt="$default_wt"
  fi

  [[ -z "$(git -C "$wt" status --porcelain)" ]] ||
    die "$wt tiene cambios sin commitear; termínalos o descártalos antes"
  landed || die "hotfix tiene commits que no han entrado en main: \`just hotfix ship\` o descártalos a mano"

  git -C "$wt" reset --quiet --hard origin/main

  # Otros puertos y otra sesión tmux. Se escribe una vez: si ya existe, es de
  # quien lo editó.
  if [[ ! -f "$wt/.env" ]]; then
    {
      [[ -f "$main_wt/.env" ]] && grep -v -E '^BUBBLE_(HTTP|UI_PORT|TMUX|TEST_PORT)=' "$main_wt/.env"
      printf '\n# ---- worktree hotfix: no chocar con %s ----\n' "$main_wt"
      echo "BUBBLE_HTTP=127.0.0.1:8091"
      echo "BUBBLE_UI_PORT=5174"
      echo "BUBBLE_TMUX=bubble-hotfix"
      echo "BUBBLE_TEST_PORT=8109"
    } > "$wt/.env"
    chmod 600 "$wt/.env"
  fi

  # Sin node_modules el hook se salta svelte-check; el CI no.
  if [[ ! -d "$wt/frontend/node_modules" ]] && command -v bun >/dev/null 2>&1; then
    (cd "$wt/frontend" && bun install --frozen-lockfile)
  fi

  echo "hotfix en $(git -C "$wt" log --oneline -1)"
  echo "  cd $wt"
}

ship() {
  local wt; wt="$(hotfix_wt)"
  [[ -n "$wt" ]] || die "no hay worktree de hotfix: \`just hotfix start\`"
  [[ -z "$(git -C "$wt" status --porcelain)" ]] || die "$wt tiene cambios sin commitear"

  git fetch --quiet origin main
  [[ -n "$(git rev-list origin/main..hotfix)" ]] || die "hotfix no tiene nada encima de main"

  # Lo que el release va a hacer, dicho antes de abrir el PR: un hotfix escrito
  # como `chore:` se mergea y no publica nada.
  local next; next="$(cd "$wt" && scripts/version.sh || true)"
  if [[ -n "$next" ]]; then
    echo "al mergear, el CI publica $next"
  else
    echo "al mergear, el CI NO publica nada: ningún commit es fix:/perf:/feat:/breaking"
  fi

  # `--force-with-lease`: `start` rehace la rama sobre main, así que origin/hotfix
  # puede llevar el historial del hotfix anterior. Sólo se pisa si es el que se
  # vio la última vez.
  git -C "$wt" push --force-with-lease -u origin hotfix

  local url; url="$(gh pr list --head hotfix --base main --state open --json url --jq '.[0].url')"
  if [[ -n "$url" ]]; then
    echo "PR ya abierto: $url"
  else
    (cd "$wt" && gh pr create --base main --head hotfix --fill)
  fi
}

case "${1:-}" in
  start) start ;;
  ship)  ship ;;
  *) echo "uso: just hotfix start|ship" >&2; exit 2 ;;
esac
