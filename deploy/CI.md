# El CI vive en casa

`check` y `release` corren en una VM de Linux dentro de reko. Cero minutos
alojados —este repositorio es privado y se pagan— y el cruce de sistemas
conservado: la máquina de quien escribe es macOS, la que publica es Linux.

Esa diferencia no es teórica. La primera corrida en Linux encontró cinco
aserciones que en macOS pasaban **sin llegar nunca a la regla que decían
comprobar**.

## Levantarla (una vez)

```sh
# 1. La VM
limactl start --name=bubble-ci deploy/reko-ci.yaml

# 2. El runner dentro, con la etiqueta que los workflows piden
TOKEN=$(gh api -X POST repos/AngelMaldonado/bubble-work/actions/runners/registration-token -q .token)
limactl shell bubble-ci -- bash -lc "
  mkdir -p ~/runner && cd ~/runner
  curl -fsSL https://github.com/actions/runner/releases/latest/download/actions-runner-linux-arm64.tar.gz | tar xz
  ./config.sh --url https://github.com/AngelMaldonado/bubble-work \
    --token $TOKEN --name reko-linux --labels reko-linux --unattended
  sudo ./svc.sh install && sudo ./svc.sh start
"

# 3. Que sobreviva a un reinicio de la Mac
cp deploy/com.reko.bubble-ci.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.reko.bubble-ci.plist

# 4. Comprobar
gh api repos/AngelMaldonado/bubble-work/actions/runners \
  -q '.runners[] | .name + " · " + .status + " · " + ([.labels[].name] | join(","))'
```

`reko-linux` es la etiqueta que seleccionan los dos workflows. Tiene nombre
propio —no `linux` a secas— para que no la case por accidente otro runner.

## Retirar el viejo

El agente de macOS (`reko-bubble-work`, en `~/dev/_infra/runners/actions-services`)
existía para los workflows de despliegue de v0, que ya no están. Un runner
registrado que nadie mira es el que un día ejecuta algo que nadie esperaba:

```sh
cd ~/dev/_infra/runners/actions-services
sudo ./svc.sh stop && sudo ./svc.sh uninstall
TOKEN=$(gh api -X POST repos/AngelMaldonado/bubble-work/actions/runners/remove-token -q .token)
./config.sh remove --token "$TOKEN"
```

Y en ayetec, lo mismo, salvo que quieras conservarlo para desplegar: ese runner
corre con sudo sin contraseña en la máquina de la instancia pública, y por eso
**nunca** debe atender un `pull_request`.

## Qué esperar

- Los dos workflows comparten el grupo de concurrencia `reko-ci`: **una corrida
  a la vez**. En un runner alojado cada trabajo es una VM nueva; aquí comparten
  disco, y `scripts/test.sh` levanta el servidor en un puerto fijo sobre un
  directorio de datos fijo. Dos a la vez se pisarían y el fallo parecería del
  código.
- La primera corrida es lenta: `setup-go` y `setup-bun` bajan sus toolchains.
  Después quedan en caché.
- **Publicar depende de que reko esté encendida.** Es el precio de no pagar
  minutos. Si un día hace falta que no dependa de casa, `runs-on: ubuntu-latest`
  vuelve en una línea.

## Cuando algo va mal

```sh
limactl list                              # ¿está arriba?
limactl shell bubble-ci -- systemctl status actions.runner.*
limactl shell bubble-ci -- docker info    # buildx vive aquí
tail -f /tmp/bubble-ci.log                # el LaunchAgent
```

Si el disco de reko se llena, mira las otras VMs (`du -sh ~/.lima/*/`) antes de
tocar ésta.
