# Dónde corre el CI

`check` y `release` corren en **`ubuntu-latest`**, los runners alojados de
GitHub. El repositorio es público, así que no cuestan minutos, y publicar deja de
depender de que una máquina de casa esté encendida.

Antes corrían en `reko-ci`, una VM de Linux dentro de la Mac mini de reko, para
no pagar minutos cuando el repositorio era privado. Lo que se conserva de
entonces es el cruce de sistemas: la máquina de quien escribe es macOS y la que
comprueba es Linux, con bash 5 y coreutils de GNU. Esa diferencia no es teórica:
la primera corrida en Linux encontró cinco aserciones que en macOS pasaban **sin
llegar nunca a la regla que decían comprobar**.

Lo que motivó el cambio: un PR quedó en cola indefinidamente porque la VM estaba
apagada, y no había otro runner con su etiqueta.

## El check es idempotente por árbol

Comprobar el mismo contenido dos veces no puede dar otra respuesta, así que no se
repite. La clave es el **árbol** del commit (`git rev-parse HEAD^{tree}`), no el
commit:

- Al pasar, el CI empuja `HEAD` a `refs/ci/check/<árbol>`.
- Antes de comprobar, `scripts/ci-seen.sh` busca esa ref y verifica que el commit
  al que apunta tiene de verdad ese árbol. Si lo tiene, el check se salta.

Lo que eso ahorra: re-runs, y sobre todo el `release` tras mergear un PR cuya
base no se movió, porque el merge tiene el mismo árbol que el commit de prueba
del PR.

Por qué una ref y no `actions/cache`: la caché de un PR sólo la lee ese PR, y un
push a main no la alcanza. Una ref vive en el remoto, la lee cualquiera y un clon
normal no la descarga.

Sólo el CI escribe la marca. Que `just check` pasara en la máquina de alguien no
cuenta: si contara, `git commit --no-verify` se saltaría también esta puerta.

Ver qué árboles pasaron, o limpiar:

```sh
git ls-remote origin 'refs/ci/check/*'
git push origin --delete refs/ci/check/<árbol>
```

## Ningún puerto fijo sin decirlo

`scripts/test.sh` saca de `BUBBLE_TEST_PORT` (8099 por defecto) el puerto, el
directorio de datos, el de repos y el log. En un runner alojado cada job es una
VM limpia y no choca con nada. Sigue importando en la máquina de quien escribe,
donde otro proyecto puede estar usando el puerto:

```yaml
    env:
      BUBBLE_TEST_PORT: 8109
```

## La imagen

El `Dockerfile` compila Go **cruzado**, así que la imagen arm64 se construye en
un runner x86 sin emular el compilador. QEMU sólo ejecuta los `RUN` de la etapa
final (`apk add`). La caché de buildx va a GitHub (`type=gha`) con
`ignore-error`: si exportarla falla después de publicar, el trabajo no sale en
rojo.
