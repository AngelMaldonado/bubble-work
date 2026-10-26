# Pruebas de navegador con fixtures

## Regresión de personas sin nombre

`people.mjs` corre contra la app real, pero intercepta **todas** las peticiones `/api/` con datos inventados. No requiere cuentas, no consulta usuarios del dev y ninguna asignación llega al servidor. Sólo acepta una URL local.

Usa Playwright/Chromium del entorno de pruebas; no añade una dependencia al producto. Con Playwright disponible para Node:

```sh
node frontend/tests/browser/people.mjs http://localhost:5173
```

También acepta el módulo de Playwright y el ejecutable ya instalados, como argumentos (no son credenciales):

```sh
node frontend/tests/browser/people.mjs \
  http://localhost:5173 \
  /ruta/al/entorno/node_modules/playwright/index.mjs \
  /ruta/a/chromium \
  --source
```

- Sin `--source`: Todos, abrir cajón, pasar a proyecto, buscar el respaldo «sin nombre», seleccionar la persona por su id y cambiar de equipo. Sirve también contra un binario recién construido en un puerto de fixture.
- Con `--source` (Vite): además monta `BubbleDrawer` directamente con una opción cuyo `name` falta. Primero demuestra el crash al sustituir **sólo en ese navegador** el filtro por la expresión anterior; después demuestra que el componente actual permite buscar/seleccionar esa opción. Los archivos del repositorio y el servidor no se revierten.
- Esta prueba no afirma haber reproducido la sesión/registro de la captura original. La apertura simple de Todos no hizo caer el bundle anterior: su selector no editable puede no evaluar ese derivado.

La normalización, búsquedas, identidades repetidas y la expresión que fallaba también están cubiertas por `bun test tests/people.test.ts`, dentro de `just check`. El recorrido de navegador es manual: no forma parte de `just check` ni exige instalar un navegador para trabajar sólo en Go.

## Crear un equipo en Ajustes

`teams.mjs` monta el `SettingsTeams.svelte` real en Vite con la API simulada: crea un equipo y le agrega a una persona. Reproduce el `props_invalid_value` reportado al crear un equipo: el selector «Agregar a alguien…» enlazaba `adding[t.id]`, que para un equipo recién creado es `undefined`, a un prop con valor por defecto. El cuarto argumento opcional monta otra ruta del componente; así se comparó contra la versión anterior (copia temporal desde `git show HEAD:…`, borrada después), que falla, frente a la actual, que pasa.

```sh
node frontend/tests/browser/teams.mjs http://localhost:5173 /ruta/playwright/index.mjs /ruta/a/chromium
```

## Todos los equipos

`allteams.mjs` corre contra la app real en Vite con **toda** `/api/` interceptada: un lead global con dos equipos compartidos y uno personal. Comprueba que el selector ofrece la vista, que el tablero se pide con `scope=shared` y no enseña lo personal (ni en la columna), que «Todos» no se escribe en `prefs.team`, que el menú sólo ofrece «Nuevo proyecto en <equipo>», que `/secuencia` es un solo lienzo cuyo título elige el equipo sin tocar la preferencia (y lo recuerda tras recargar), que «Entrar a» sí la cambia, que la vista sobrevive una recarga y que el planeador pide entrar a un equipo.

```sh
node frontend/tests/browser/allteams.mjs http://localhost:5173 /ruta/playwright/index.mjs /ruta/a/chromium
```

El recorte del servidor y su 403/400 se prueban aparte, en el live suite (`just test`).

## Vista de blobs

`blobs.mjs` corre contra la app real en Vite con **toda** `/api/` interceptada (cualquier ruta no prevista, o pedir `contexts`, falla la prueba): un lead global con dos equipos compartidos, uno personal e hilos con tags ya indexados. Comprueba la barra de abajo (Burbujas/Hilos × Blobs/Flotabilidad e «Incluir cerradas»): que cada hilo cae en el blob de su primer tag y cada burbuja en el más común de sus hilos, que el mismo tag de dos equipos es un solo blob y nada personal aparece en Todos, aristas, sin aro de furia, cursores de lienzo, cajón desde el grafo sin selector de contexto, cerradas sin mover nodos, abrir un hilo suelto, preferencias tras recarga, Ajustes sin Contextos y móvil sin scroll horizontal.

```sh
node frontend/tests/browser/blobs.mjs http://localhost:5173 /ruta/playwright/index.mjs /ruta/a/chromium
```

No prueba el índice ni los permisos: la API es simulada. Leer el frontmatter, clasificar sin calor ni pulso y que un cliente no escriba el índice están en el live suite. Si falla, deja `/tmp/bw-blobs-failure.png`.

## Prototipo del modelo

Para revisión manual basta **http://localhost:5173/theme/model** con `just dev` corriendo. Arriba, **Burbujas / Hilos** cambia el nivel; abajo, **Blobs / Flotabilidad** cambia la disposición. Blobs es un grafo espacial con zoom/pan/arrastre, no cajas de tarjetas. `main.ts` monta los fixtures en vez de App sólo en DEV; Vite elimina esa rama y el chunk al construir producción. No requiere cuenta, runner ni acceso a datos reales; `/todos` sigue siendo el tablero normal.

La misma capa vive dentro de la app maqueta en **http://localhost:5173/theme/mock** (sólo DEV): tablero real de orbes en Flotabilidad, grafo en Blobs, cajón y vista de hilo reales al pulsar. `model.mjs` la recorre también (`checkMockApp`): ambas pestañas × ambas disposiciones, minimapa sólo con orbes de burbuja, vista de hilo con el título pulsado, explicación de furia → cajón, recarga con preferencias y sin errores de página.

`model.mjs` comprueba primero esa URL directa con sesión simulada, interacción, recarga y barra final: cero peticiones API. Después monta `frontend/src/prototypes/ModelEvolution.svelte` en una página interceptada por el runner para automatizar escenarios. **No está importado por App ni tiene ruta de producto; el runner bloquea cualquier `/api/`**. Los nombres, estados y niveles son fixtures; esto no implementa furia, compromisos ni permisos del backend. El grafo (`components/BlobGraph.svelte`, `lib/graph.ts`) es ya el mismo código que usa el tablero real; lo que sigue siendo fixture es la furia.

Desde la raíz, con los mismos módulos/ejecutables del ejemplo anterior:

```sh
node frontend/tests/browser/model.mjs \
  http://localhost:5173 \
  /ruta/al/entorno/node_modules/playwright/index.mjs \
  /ruta/a/chromium
```

- Comprueba tabs Burbujas/Hilos por ratón y teclado, Blobs/Flotabilidad, conservación de filtros, posiciones estables al buscar, zoom/pan/arrastre sin abrir detalle accidental, detalle de hilo, burbuja dormida/furiosa, explicación, tags compartidos entre equipos, secuencias paralelas, elección de equipo para crear, cerradas, preferencia aislada del fixture, viewport móvil y teclado básico. La simulación de miembro no ofrece Todos; no es una prueba de ACL real.
- Guarda capturas inventadas en `docs/prototypes/model-evolution/` (cuarto argumento para cambiar salida).
- Añadir `--review` ejecuta las comprobaciones y deja Chrome visible para interactuar; cerrar la pestaña termina. La URL interceptada sólo existe en esa sesión y se puede recargar. No se añade un endpoint ni un modo público del producto.
- Requiere Vite: a diferencia de `people.mjs`, no corre contra el bundle. No forma parte de `just check`; `svelte-check` sí comprueba el componente.

