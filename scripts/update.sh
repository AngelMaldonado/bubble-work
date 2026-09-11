#!/usr/bin/env bash
# Actualizar ESTA instancia. Tira, no la empujan.
#
#   scripts/update.sh              actualiza si hay algo nuevo
#   scripts/update.sh --check      dice qué hay, sin tocar nada
#
# Se ejecuta desde el directorio donde vive el `compose.yml` de la instancia, a
# mano o desde el timer de systemd (ver `deploy/`).
#
# Tira en vez de que el CI empuje por SSH: así no hay una llave con acceso al
# servidor guardada en el repositorio, una instancia detrás de NAT se actualiza
# igual, y una que estaba apagada el día del release se pone al día en su
# siguiente vuelta en vez de perderse el evento.
set -euo pipefail
cd "$(dirname "$0")/.."

[[ -f .env ]] && { set -a; . ./.env; set +a; }

CHANNEL="${BUBBLE_CHANNEL:-stable}"
# Copia antes de actualizar: APAGADA por defecto.
#
# La migración es la puerta de un solo sentido de toda actualización —corre sola
# al arrancar— así que una copia previa es lo único que deja volver. Está en 0
# porque el estado es de quien aloja esto: hay quien ya respalda el volumen
# entero desde fuera, y un script que empieza a llenar el disco de tarballs sin
# que nadie lo pidiera es un script que alguien desactiva del todo. Ponlo en 1 en
# el `.env` de la instancia si aquí no hay otra copia.
BACKUP="${BUBBLE_BACKUP:-0}"
KEEP="${BUBBLE_BACKUP_KEEP:-5}"
BACKUP_DIR="${BUBBLE_BACKUP_DIR:-./backups}"
HEALTH_URL="${BUBBLE_HEALTH_URL:-http://127.0.0.1:${BUBBLE_PORT:-8090}/api/version}"
WAIT="${BUBBLE_HEALTH_WAIT:-90}"

say() { printf '==> %s\n' "$1"; }

compose() { docker compose "$@"; }

digest() { # el digest que la instancia está corriendo AHORA
  docker inspect --format '{{.Image}}' "$(compose ps -q bubble 2>/dev/null)" 2>/dev/null || true
}

running="$(digest)"
say "canal: $CHANNEL"
say "corriendo: ${running:-nada}"

compose pull bubble
pulled="$(docker image inspect --format '{{.Id}}' \
  "ghcr.io/angelmaldonado/bubble-work:${CHANNEL}" 2>/dev/null || true)"

if [[ -n "$running" && "$running" == "$pulled" ]]; then
  say "ya está al día"
  exit 0
fi

if [[ "${1:-}" == "--check" ]]; then
  say "hay una versión nueva: $pulled"
  exit 0
fi

if [[ "$BACKUP" == "1" ]]; then
  mkdir -p "$BACKUP_DIR"
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  say "copia en $BACKUP_DIR/$stamp.tar.gz"
  # El volumen ENTERO, no sólo la base: la base y los repositorios de markdown
  # son un solo estado, y una copia con la mitad restaura una instancia que dice
  # cosas que sus archivos no dicen.
  #
  # Con el servidor PARADO. PocketBase escribe SQLite en WAL y git escribe
  # archivos: copiar en caliente es copiar una foto movida, que es la peor clase
  # de respaldo — el que parece bueno hasta el día que hace falta.
  compose stop bubble
  # `--volumes-from` en vez de nombrar el volumen: monta lo que el contenedor
  # tenga montado, se llame como se llame y esté donde esté. Nombrarlo aquí sería
  # repetir el `compose.yml` en un sitio donde nadie se acuerda de cambiarlo.
  docker run --rm --volumes-from "$(compose ps -aq bubble)" \
    -v "$(cd "$BACKUP_DIR" && pwd)":/backup alpine:3.21 \
    tar czf "/backup/$stamp.tar.gz" -C /data .
  # Deja las últimas, borra el resto. Un directorio de copias que crece sin
  # límite acaba llenando el disco, y un disco lleno es una instancia caída.
  # Un bucle y no `xargs -r`: esa bandera es de GNU y en macOS —donde corre una
  # de las instancias— xargs la rechaza, así que la rotación fallaba justo en la
  # máquina que más la necesita.
  ls -1t "$BACKUP_DIR"/*.tar.gz 2>/dev/null | tail -n "+$((KEEP + 1))" | while IFS= read -r old; do
    rm -f -- "$old"
  done
fi

say "arrancando la versión nueva"
compose up -d bubble

# Sana o vuelve atrás. Las migraciones ya corrieron, así que volver a la imagen
# anterior no deshace el esquema — pero deja el servicio ARRIBA con la versión
# que funcionaba, que es lo que hace falta a las tres de la mañana, y dice en el
# log qué pasó para que alguien lo mire con luz.
say "esperando a que conteste ($WAIT s)"
ok=0
for _ in $(seq 1 "$WAIT"); do
  if curl -fsS --max-time 2 "$HEALTH_URL" >/dev/null 2>&1; then ok=1; break; fi
  sleep 1
done

if [[ "$ok" == "1" ]]; then
  say "arriba: $(curl -fsS "$HEALTH_URL")"
  exit 0
fi

say "NO contestó. Volviendo a la imagen anterior."
if [[ -n "$running" ]]; then
  docker tag "$running" "ghcr.io/angelmaldonado/bubble-work:${CHANNEL}"
  compose up -d bubble
  say "de vuelta en la anterior. Revisa: docker compose logs --tail=200 bubble"
else
  say "no había ninguna anterior a la que volver."
fi
exit 1
