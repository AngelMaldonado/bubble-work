# Dónde corre el CI

`check` y `release` corren en **`reko-ci`**: una VM de Linux dentro de la Mac
mini de reko, que atiende varios repositorios. Cero minutos alojados —los repos
privados se pagan— y el cruce de sistemas conservado: la máquina de quien
escribe es macOS, la que publica es Linux.

Esa diferencia no es teórica. La primera corrida en Linux encontró cinco
aserciones que en macOS pasaban **sin llegar nunca a la regla que decían
comprobar**.

**La VM no se provisiona desde aquí.** Es infraestructura personal que sirve a
varios proyectos —su receta, su LaunchAgent y el alta de runners viven en
`~/dev/scripts/reko-vm-ci`—, y meterla en este repositorio la habría atado al
primero que la estrenó. Lo que sí es de este repositorio son las dos cosas que
le pide a cualquier máquina que lo atienda:

## 1. La etiqueta

```yaml
runs-on: [self-hosted, reko-ci]
```

Nombra la MÁQUINA, no el proyecto. Dentro de ella hay un agente por repositorio
—GitHub sólo registra runners por repo, organización o empresa, y las cuentas
personales no tienen el nivel de organización— y a los workflows no les importa
cuál de ellos los atiende.

## 2. Ningún puerto fijo sin decirlo

La concurrencia de GitHub **no cruza repositorios**. El grupo `bubble-work-ci`
serializa los dos workflows de este repo, pero otro proyecto puede arrancar su
job al mismo tiempo en la misma VM.

Por eso `scripts/test.sh` saca de `BUBBLE_TEST_PORT` (8099 por defecto) el
puerto, el directorio de datos, el de repos y el log. Si algún día dos
proyectos coinciden aquí, el segundo pone otro puerto:

```yaml
    env:
      BUBBLE_TEST_PORT: 8109
```

Un proyecto que use puertos fijos sin decirlo es el que un día hace fallar el CI
de otro, y el error parecerá del código.

## Volver a GitHub

Publicar depende de que reko esté encendida. Es el precio de no pagar minutos, y
si algún día hace falta que no dependa de casa, `runs-on: ubuntu-latest` vuelve
en una línea — con la salvedad de que entonces el `Dockerfile` volvería a
construirse en un runner x86: sigue funcionando, porque compila cruzado, sólo
que la imagen arm64 pasaría a ser la emulada.
