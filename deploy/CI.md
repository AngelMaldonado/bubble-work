# El CI vive en casa

Una VM de Linux dentro de reko, llamada **`reko-ci`**, que atiende los
workflows de este repositorio y de los que se le vayan añadiendo. Cero minutos
alojados —los repos privados se pagan— y el cruce de sistemas conservado: la
máquina de quien escribe es macOS, la que publica es Linux.

Esa diferencia no es teórica. La primera corrida en Linux encontró cinco
aserciones que en macOS pasaban **sin llegar nunca a la regla que decían
comprobar**.

Se llama por la máquina y no por el proyecto que la estrenó: dentro de un año
sería una VM con el nombre de uno de los cinco software que corre, y nadie
sabría si se puede tocar.

## Arquitectura: aarch64, nativa

`vmType: vz`, `arch: aarch64`. Es la del propio Mac, así que corre sobre
Virtualization.framework a velocidad casi nativa. Una VM x86_64 sería QEMU
emulando, y no haría falta para nada:

- **Los binarios de Linux amd64** los cross-compila Go (`GOOS=linux
  GOARCH=amd64`), que es justo lo que Go hace bien.
- **La imagen de docker** también: el `Dockerfile` usa `--platform=$BUILDPLATFORM`
  y `GOARCH=$TARGETARCH`, así que las dos arquitecturas salen a la misma
  velocidad sin emular nada.
- **Lo que la VM aporta es la semántica de Linux** —bash 5, coreutils de GNU— y
  eso no depende de la arquitectura.

El agente de Actions que hay que bajar dentro es, por lo mismo,
`actions-runner-linux-arm64`.

## La que está corriendo

Lima `reko-ci`, `vz` aarch64, 4 cpu / 4 GiB / 100 GiB, **Ubuntu 26.04**. En la
tailnet contesta en `root@172.31.99.32`.

`deploy/reko-ci.yaml` es la receta para REHACERLA — borrarla y levantarla de
nuevo no debería depender de que alguien se acuerde de cómo se hizo:

```sh
limactl start --name=reko-ci deploy/reko-ci.yaml

# Que sobreviva a un reinicio de la Mac. Sin esto, un reinicio deja los PR en
# cola sin decir por qué — que es la peor forma de fallar: parece lento, no roto.
cp deploy/com.reko.ci.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.reko.ci.plist
```

> **Quita el montaje del host.** La plantilla por defecto de lima monta `~` de
> reko dentro de la VM. Ahora mismo ese montaje no está activo, pero está
> declarado: en cuanto lima lo levante, el código de cualquier PR podría leer —y
> escribir— tu directorio personal. `mounts: []`, como en el yaml de al lado.

## Dar de alta un repositorio

Un agente de Actions se registra **por repositorio** — GitHub no tiene runners
a nivel de cuenta personal, sólo de organización. Así que cada repo tiene el
suyo dentro de la misma VM; un agente en espera no consume nada.

```sh
deploy/add-runner.sh AngelMaldonado/bubble-work
deploy/add-runner.sh AngelMaldonado/reversa
```

Todos llevan la etiqueta **`reko-ci`**, que nombra la máquina. Los workflows
piden `runs-on: [self-hosted, reko-ci]` y no les importa cuál de los agentes los
atiende.

> Si algún día los repos viven bajo una **organización**, un solo agente los
> atiende todos con un runner group. Ése es el momento de decidirlo: migrar
> runners después es rehacer las altas.

## La regla que hay que respetar entre proyectos

La concurrencia de GitHub **no cruza repositorios**: el grupo `bubble-work-ci`
serializa los dos workflows de este repo, y otro repo puede arrancar su job al
mismo tiempo en la misma VM.

Ahí duele lo que sea fijo. `scripts/test.sh` levanta el servidor en un puerto y
un directorio de datos que salen de `BUBBLE_TEST_PORT` (8099 por defecto), así
que dos proyectos que hagan lo mismo **tienen que pedir puertos distintos**:

```yaml
    env:
      BUBBLE_TEST_PORT: 8109      # uno por proyecto
```

Un proyecto que use puertos fijos sin decirlo es el que un día hace fallar el
CI de otro, y el error parecerá del código.

## Retirar el runner viejo de macOS

El agente `reko-bubble-work` (en `~/dev/_infra/runners/actions-services`) existía
para los workflows de despliegue de v0, que ya no están. Un runner registrado
que nadie mira es el que un día ejecuta algo que nadie esperaba:

```sh
cd ~/dev/_infra/runners/actions-services
sudo ./svc.sh stop && sudo ./svc.sh uninstall
TOKEN=$(gh api -X POST repos/AngelMaldonado/bubble-work/actions/runners/remove-token -q .token)
./config.sh remove --token "$TOKEN"
```

En ayetec, igual — salvo que quieras conservarlo para desplegar. Ese corre con
sudo sin contraseña en la máquina de la instancia pública, y por eso **nunca**
debe atender un `pull_request`.

## Qué esperar

- La primera corrida es lenta: `setup-go` y `setup-bun` bajan sus toolchains.
  Después quedan en caché.
- **Publicar depende de que reko esté encendida.** Es el precio de no pagar
  minutos; si algún día hace falta que no dependa de casa, `ubuntu-latest`
  vuelve en una línea.
- Lo que se llena primero es el **disco**: cachés de Go, de bun y capas de
  docker. La VM tiene 100 GiB (93 libres), pero es un archivo sparse sobre reko,
  donde quedaban ~31 GiB reales con las otras VMs ya ocupando 33. Lo que la VM
  crea, reko lo paga.

```sh
limactl list                                          # ¿está arriba?
ssh root@172.31.99.32 systemctl status 'actions.runner.*'
ssh root@172.31.99.32 docker system prune -af --volumes    # cuando apriete
ssh root@172.31.99.32 journalctl -u 'actions.runner.*' -n 50
du -sh ~/.lima/*/                                     # quién ocupa el disco en reko
tail -f /tmp/reko-ci.log                              # el LaunchAgent
```

## Nada que te importe vive aquí

Esta VM ejecuta código de ramas propuestas en un PR. Su valor es que se puede
tirar y rehacer con `limactl delete reko-ci` y el yaml de al lado. Un servicio
que dependa de ella deja de ser desechable.
