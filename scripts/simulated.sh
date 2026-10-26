#!/usr/bin/env bash
# Las personas simuladas de `just seed agents` (@simulado.test) viven lo que vive
# el servidor de dev: se borran cuando se baja. Tres puertas, porque el dev se
# baja por tres lados:
#
#   - `just stop` y `just dev` (que reemplaza la sesión) llaman a
#     `purge_simulated` después de matar la sesión;
#   - la ventana del servidor corre `bubble simulated purge` cuando `serve`
#     termina por su cuenta: Ctrl-C en el panel, o una caída.
#
# Se purga SIN servidor, contra el directorio de datos: por eso se espera a que
# el puerto quede libre. Repetirlo no hace nada.

# Espera a que nadie escuche en BUBBLE_HTTP (hasta ~5 s) y purga.
purge_simulated() {
  local port="${BUBBLE_HTTP##*:}" _
  for _ in $(seq 1 50); do
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1 || break
    sleep 0.1
  done
  [[ -x "$BUBBLE_BIN" && -d "$BUBBLE_DATA" ]] || return 0
  "$BUBBLE_BIN" simulated purge --dir "$BUBBLE_DATA" || echo "  (no se pudieron borrar las personas simuladas)" >&2
}
