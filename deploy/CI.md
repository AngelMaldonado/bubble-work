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

## Hotfix sin parar wip

`wip` es la rama de trabajo, y puede estar a medias cuando hace falta arreglar
lo que ya está publicado. El arreglo se hace en otro **worktree**, en la rama
`hotfix`, que sale de `origin/main`:

```
just hotfix start      # ../bubble-work-hotfix, limpio y en origin/main
# commits fix: …       # en ese directorio
just hotfix ship       # push de hotfix + PR hotfix→main
```

```
          hotfix ──fix──┐
                        ▼ PR (check)
main ───────────────────●──── release vX.Y.Z+1
                        │
                        ▼ backmerge.yml abre PR main→wip
wip  ──…a medias…───────●──── merge cuando le venga bien a wip
```

- **`check` y `release` no cambian.** El PR `hotfix → main` corre el mismo
  `check` que cualquier PR a main, y el merge publica igual que uno de `wip`. El
  salto sale de los commits: un hotfix tiene que ser `fix:` (o `perf:`); escrito
  como `chore:` se mergea y no publica nada. `ship` lo avisa antes de abrir el PR.
- **El worktree no choca con `wip`.** `start` le escribe un `.env` propio:
  servidor en `8091`, UI en `5174`, tests en `8109`, sesión tmux
  `bubble-hotfix`. Los datos (`pb_data`, `repos`) son los del directorio del
  worktree, vacíos al principio. `just test` directo no lee `.env`; el hook y
  `just check` sí.
- **Volver a wip es un PR, no un push.** `backmerge.yml` corre en cada push a
  main y, si main lleva commits que wip no tiene (sin contar merges: los de cada
  PR wip→main no son trabajo que falte), abre un PR `main → wip`, o deja el que
  ya estaba abierto, que se actualiza solo. Mergearlo lo decide quien trabaja en
  wip, y después `git pull`. Si choca, se resuelve en local con
  `git merge origin/main` y push, y el PR se cierra solo.
- **No cuenta doble.** El `fix:` del hotfix queda detrás del tag del release; el
  siguiente release desde wip mide desde ese tag y no lo vuelve a contar.

Para que `backmerge.yml` pueda abrir el PR con el `GITHUB_TOKEN` hay que tener
activado, en el repositorio, Settings → Actions → General → «Allow GitHub Actions
to create and approve pull requests».

Lo que se descartó: un `git merge` automático de main en `origin/wip`, porque
mueve la rama mientras alguien trabaja en ella y, si choca, deja un rojo que
nadie pidió; y hacer el hotfix con `git switch` en el mismo directorio, porque
obliga a guardar en un stash lo que está a medias.
