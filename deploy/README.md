# Desplegar una instancia

Dos caminos. El de arriba es el que se actualiza solo; el de abajo es un binario
y una unidad, para una máquina donde no quieras docker.

En los dos, **el estado son dos directorios**: la base de PocketBase y el árbol
de markdown (un git por workspace). Se respaldan y se restauran **juntos** — un
commit sin su fila, o una fila sin su archivo, no significan nada.

## Con docker (recomendado)

```sh
git clone https://github.com/AngelMaldonado/bubble-work /srv/bubble-work
cd /srv/bubble-work
cp .env.example .env          # BUBBLE_CHANNEL, BUBBLE_PORT, BUBBLE_BACKUP
docker compose up -d
```

Estado: un volumen, `bubble-data`, con `/data/pb_data` y `/data/repos` dentro.

Actualizarse solo:

```sh
sudo cp deploy/bubble-update.* /etc/systemd/system/
sudo systemctl enable --now bubble-update.timer
```

En **macOS** no hay systemd; el equivalente es un LaunchAgent:

```sh
mkdir -p scripts && cp <repo>/scripts/update.sh scripts/   # `update.sh` espera
                                                           # el compose.yml un
                                                           # nivel por encima
cp deploy/com.reko.bubble-update.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.reko.bubble-update.plist
```

En los dos casos: tira una vez al día, arranca la versión nueva, espera a
`/api/version` y vuelve a la imagen anterior si no contesta. La copia previa es opt-in
(`BUBBLE_BACKUP=1`): la migración corre sola al arrancar y es la puerta de un
solo sentido de toda actualización, así que enciéndela si en esa máquina no hay
otra copia.

## Sólo el binario

```sh
sudo useradd --system --home /var/lib/bubble --create-home bubble
curl -fLO https://github.com/AngelMaldonado/bubble-work/releases/latest/download/bubble-linux-amd64
curl -fLO https://github.com/AngelMaldonado/bubble-work/releases/latest/download/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS      # antes de ejecutarlo, no después
sudo install -m 755 bubble-linux-amd64 /usr/local/bin/bubble
sudo cp deploy/bubble.service /etc/systemd/system/
sudo systemctl enable --now bubble
```

`git` tiene que estar instalado: el árbol de markdown lo escribe git de verdad,
llamando al programa, que es el mismo con el que una persona puede clonarlo y
leerlo.

La unidad corre `boot`, no `serve`: el binario instalado lanza la versión
vigente —que vive en `/var/lib/bubble/bin`— y la vigila. Actualizar es pulsar la
etiqueta de versión en la aplicación; reemplazar el binario a mano y reiniciar
sigue funcionando, y es lo que actualiza al propio arranque.

## Lo que falta después, en los dos casos

**Las dos cuentas.** Son distintas a propósito: `_superusers` **opera la caja**
—el dashboard, las colecciones, las reglas— y `users` es quien **trabaja**. Con
sólo la primera, el login de la aplicación te rechaza; es el paso que más se
olvida.

```sh
# docker
docker compose exec bubble bubble superuser upsert tu@correo '<contraseña>' --dir /data/pb_data
docker compose exec bubble bubble person       tu@correo '<contraseña>' lead --dir /data/pb_data

# binario
sudo -u bubble bubble superuser upsert tu@correo '<contraseña>' --dir /var/lib/bubble/pb_data
sudo -u bubble bubble person       tu@correo '<contraseña>' lead --dir /var/lib/bubble/pb_data
```

`--dir` va explícito porque `exec` no pasa por el `CMD` de la imagen. `lead` es
el lead **global** —quien ve el planeador y reparte el acceso al inventario—;
`member` es lo normal para el resto. El mismo correo puede existir en las dos
colecciones sin chocar.

Fundar un workspace **no** hace falta para entrar: una cuenta nueva ve el board
vacío con las dos salidas a la vista, empezar uno o esperar una invitación.

**El proxy.** El servidor escucha en `127.0.0.1` a propósito: delante va lo que
termina TLS. El token de MCP viaja en una cabecera, así que sin TLS viaja en
claro.

**El dashboard de PocketBase vive en `/_/`** y es acceso total a la base. Lo
mejor es no proxearlo y llegar por un túnel:

```sh
ssh -L 8090:127.0.0.1:8090 usuario@servidor
# y abrir http://127.0.0.1:8090/_/
```

Se entra unas cuantas veces al año; un formulario de login para eso en internet
abierto es la superficie más grande que tendría este binario.
