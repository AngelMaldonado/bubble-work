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

Tira una vez al día, arranca la versión nueva, espera a `/api/version` y vuelve
a la imagen anterior si no contesta. La copia previa es opt-in
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

Actualizar es reemplazar el binario y reiniciar. Las migraciones corren al
arrancar.

## Lo que falta después, en los dos casos

**El superuser.** Es otra cuenta que las personas del producto — `_superusers`
frente a `users`.

```sh
# docker
docker compose exec bubble bubble superuser upsert tu@correo <contraseña> --dir /data/pb_data
# binario
sudo -u bubble bubble superuser upsert tu@correo <contraseña> --dir /var/lib/bubble/pb_data
```

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
