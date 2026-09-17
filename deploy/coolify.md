# Una instancia en Coolify

`bubble.ayetec.space`, montada a mano en la UI de `coolify.ayetec.space`, sobre
el servidor `services.ayetec.space`. Es un contenedor y un volumen: la base es
SQLite dentro del propio servicio, así que no hay base de datos que montar
aparte.

Lo que describe el servicio está en [`../compose.coolify.yaml`](../compose.coolify.yaml).

## Qué hace el repositorio y qué hace Coolify

| Pieza | Quién |
| --- | --- |
| La imagen `ghcr.io/angelmaldonado/bubble-work` (`:stable` y `vX.Y.Z`) | `release.yml`, al empujar a main |
| Enrutar el dominio y terminar TLS | Coolify |
| Tirar de `:stable` y recrear el contenedor | Coolify, al redesplegar |
| Cambiar de versión y volver atrás si no contesta | `bubble boot`, dentro del contenedor |

El paquete de GHCR es **público**: Coolify no necesita credenciales de registro.

## Dos caminos para actualizar, y los dos valen

- **El botón de versión** de la aplicación (lead global). Descarga el binario
  del release, copia la base y reinicia al hijo sin recrear el contenedor.
- **Redesplegar en Coolify.** La imagen nueva trae una versión más nueva que la
  vigente, y `boot` la adopta al arrancar: copia la base en frío
  (`pb_data/backups/antes-de-<tag>.zip`) y la lanza con la anterior como red.

La más nueva gana. Una imagen más vieja que lo que ya instaló el botón no
retrocede nada, y una versión que no llegó a contestar queda anotada
(`/data/bin/failed`) para que reiniciar el contenedor no la reintente. La
siguiente imagen, o el botón, sí.

Nada de esto deshace el esquema: las migraciones corren al arrancar y son de un
solo sentido. Para eso es la copia.

## Montarlo

1. **Proyecto y recurso.** En Coolify, *New resource → Docker Compose Empty* en
   el servidor `ayetec-services`, y pegar `compose.coolify.yaml`. (O
   *Public/Private Repository* apuntando a este repositorio, con
   `compose.coolify.yaml` como archivo de compose.)
2. **Dominio.** En el servicio `bubble`: `https://bubble.ayetec.space:8090`.
   El `:8090` le dice a Coolify a qué puerto del contenedor enrutar; el público
   sigue siendo 443. `*.ayetec.space` ya resuelve a services, así que no hace
   falta ningún registro DNS.
3. **Volumen.** `ayetec-bubble-data` montado en `/data`. Comprobar en
   *Storages* que quedó como volumen con nombre y no como directorio efímero.
4. **Desplegar.** El healthcheck (`/api/version`) tiene 90 s de gracia.
5. **Las dos cuentas**, desde la *Terminal* del servicio en Coolify:

   ```sh
   bubble superuser upsert tu@correo '<contraseña>' --dir /data/pb_data
   bubble person       tu@correo '<contraseña>' lead --dir /data/pb_data
   ```

   `_superusers` opera la caja (el dashboard); `users` es quien trabaja. Sin la
   segunda, el login de la aplicación rechaza.
6. **Ajustes de PocketBase**, en `https://bubble.ayetec.space/_/` → *Settings*:
   - *Application URL*: `https://bubble.ayetec.space`.
   - *User IP proxy headers*: `X-Forwarded-For`. Sin esto, todos los límites de
     ritmo ven la IP del proxy de Coolify y una persona que se equivoca de
     contraseña bloquea a todas.
7. **Comprobar**:
   - `https://bubble.ayetec.space/api/version` contesta con `"boot": true`.
   - Entrar a la aplicación con la cuenta de `users`.
   - El MCP responde a un agente (`/mcp`) y los cambios de otra pestaña llegan
     solos: las dos cosas son conexiones largas, y el proxy no debe cortarlas.
   - Subir una imagen a un comentario.

## Pendiente, dicho en su sitio

- **Respaldos.** No hay ninguno programado. El volumen entero es el estado —la
  base, los repositorios de markdown y la sesión de WhatsApp—, y se copia con el
  servidor parado o no sirve. Las copias `antes-de-<tag>.zip` sólo cubren
  `pb_data`, no los repositorios.
- **El dashboard `/_/` queda en internet.** Con el puerto sólo expuesto a la red
  del stack no hay túnel SSH al que llegar, como en [`README.md`](./README.md).
  Cerrarlo con un middleware del proxy de Coolify está por decidir.
- **Redesplegar al publicar.** Hoy se redespliega a mano en la UI. Llamar a la
  API de Coolify al final de `release.yml` es el siguiente paso, cuando se
  quiera.
