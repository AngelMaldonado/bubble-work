# Bubble Work — guía para agentes

Bubble Work es donde este departamento planea el trabajo, lo escribe y ve qué se ha enfriado. Tú trabajas aquí por MCP, en nombre de la persona dueña de tu token: lo que escribes queda firmado con su nombre y sólo ves lo que ella ve.

Al final de este documento está la **forma de trabajar del departamento**, la que escribe el lead. Si algo de allí contradice esta guía, manda la del departamento.

## Cómo empezar una sesión

1. `team_list`: en qué equipos está tu persona y con qué papel. Cada equipo es un contexto aparte; si hay más de uno, sabe en cuál estás trabajando antes de escribir.
2. `sequence_next` (con `team`): qué toca ahora en la secuencia de ese equipo y cuál es el primer hilo que es tuyo.
3. Antes de crear nada, busca lo que ya existe: `project_board` y `document_search`. Casi nada es nuevo; lo normal es que ya haya un hilo a medias con el contexto de por qué se dejó.
4. Del hilo en el que vas a trabajar, lee su ficha (`thread_get`), su documento (`document_read`), su historial (`thread_history`) y sus comentarios (`comment_list`). Empezar sin eso es cómo se rehace lo que se terminó el martes.
5. Lee el `README.md` del proyecto antes de escribir: ahí viven las convenciones del equipo, si las hay.

## Entidades

- **Equipo** (`team`): un CONTEXTO de trabajo con sus propios proyectos, objetivos, secuencia, calendario, inbox de ideas y canal de WhatsApp. Lo de un equipo no se mezcla con lo de otro.
  - Cada quien está en uno o más equipos como `lead` o `member`. Un lead global es lead de todo equipo que no sea personal.
  - Un **equipo personal** es el contexto privado de una persona: sólo lo ve quien está en él, ni siquiera otro lead global. A otra persona se la invita a un PROYECTO del equipo, y ve ese proyecto y nada más.
  - Lo que no dice equipo va al equipo por defecto de tu persona (`team_list` lo dice). Si tiene más de uno y no es obvio, pregunta.
  - Crear equipos y decidir quién está en ellos se hace a mano en la aplicación.
- **Proyecto** (`project` en los parámetros; `workspace` en los datos): la frontera de un cuerpo de trabajo, con su propio repositorio git de markdown. Vive en un equipo. Su dirección es el `slug` y nunca cambia.
- **Módulo** o **burbuja** (`bubble` en las tools, los parámetros y los datos): agrupa trabajo relacionado y es la unidad de *atención*. Tiene un resultado esperado (`outcome`: qué es cierto cuando termine), responsables, un objetivo al que sirve, impacto, urgencia y fecha. Se cierra con una frase y se puede reabrir.
- **Hilo** (`thread`): una unidad ejecutable de trabajo, de horas a pocos días. Necesita un nombre; lleva un **documento markdown** donde se planea, se sigue el avance y queda la evidencia. Puede vivir en un módulo o suelto.
- **Documento**: cualquier `.md` del proyecto. Tres clases:
  - el del hilo, en `threads/`, más la carpeta junto a él para lo que el hilo necesite aparte (una investigación, un diseño, un log);
  - la **wiki**, en `docs/`: guías, referencias y notas que duran más que un hilo. Anida libremente. Escribir ahí se registra pero no calienta nada;
  - el `README.md` del proyecto: su portada y sus convenciones.
- **Tags**: de qué trata un hilo, en el frontmatter de SU documento, al principio del archivo:
  ```
  ---
  tags: [pagos, cliente/acme]
  ---
  ```
  Los escribes tú con la persona: ella conoce su área. Antes de inventar uno, mira los que ya usa el equipo (`project_board` y `thread_get` traen los de cada hilo) y reutiliza el mismo nombre; se comparan sin `#` y sin mayúsculas, y `/` hace jerarquías. El primero importa: la vista de blobs del tablero agrupa cada hilo por su primer tag, y cada módulo por el que más se repite entre sus hilos. Cambiar sólo los tags no calienta nada (se registra como clasificación); no dan permisos ni cambian fechas, asignados o prioridad. El frontmatter no se dibuja al leer el documento.
- **Evidencia**: un enlace externo pegado a un hilo (commit, PR, entregable publicado). Es la prueba de que la realidad cambió fuera de esta herramienta.
- **Comentario**: lo que se dice sobre un hilo. Es pulso, no evidencia (ver «Calor»).
- **Objetivo**: para qué es el trabajo. Es de un EQUIPO, no de un proyecto: los módulos de cualquiera de sus proyectos cuelgan de él (nunca de un objetivo de otro equipo). Los definen y los leen los leads del equipo.
- **Secuencia**: el orden de ejecución, UNA línea por equipo, que ponen sus leads en el planeador. Hilos con el mismo número van en paralelo. Terminar un hilo la hace avanzar sola.
- **Prioridad**: la de un módulo la deriva el servidor de impacto × urgencia; la de un hilo (P1…P4) la pone sólo un lead del equipo y es independiente de la de su módulo.
- **Artefactos temporales**: lo que se piensa antes de que exista el trabajo. No calientan, no hacen commits y caducan (por defecto, 30 días después de su último movimiento; el departamento lo ajusta) salvo que alguien los fije.
  - **Interrogatorio** (`grill`): preguntas estructuradas que la persona contesta en Bubble, para llegar a un entendimiento compartido de un plan o una decisión.
  - **Markdown efímero** (`markdown`): un `.md` que publicas para que la persona lo lea ahora (un borrador, una propuesta, un informe) sin abrir un hilo ni tocar la wiki.
  - El protocolo completo de cada uno llega en la respuesta de `artifact_create`.
- **Pines**: dos clases, las dos de la persona.
  - **Fijar un artefacto** evita que caduque (`artifact_pin`), sólo si la persona lo pide.
  - **Fijar un hilo** es un atajo personal en la barra de la persona (`pin_list`, `pin_add`, `pin_remove`). Es privado y no es calor ni prioridad.
- **Inbox: IDEAS, no trabajo, y cada una en SU equipo.** Es donde las personas capturan lo que se les ocurre y nadie ha decidido hacer, también lo que llega por el WhatsApp de cada equipo (`source: whatsapp`; `from` dice quién lo mandó). Una idea pertenece al equipo en el que nació: es su contexto. Una idea **no es un hilo ni un módulo, ni un pendiente de nadie**:
  - no la ejecutes, no la cuentes como avance ni como trabajo atrasado, y no la conviertas en trabajo por tu cuenta;
  - se vuelve trabajo sólo cuando la PERSONA lo decide, y en un proyecto de SU MISMO equipo: entonces buscas (o creas, con su confirmación) el hilo o módulo y anotas a dónde fue con `inbox_triage`; la nota queda apuntando a él;
  - no muevas ideas de un contexto a otro ni las mezcles al resumir: lo que llegó al equipo de alguien es de ese equipo;
  - lo que surge y no pertenece a ningún hilo, guárdalo como idea con `inbox_capture` en el equipo al que pertenece; nunca abras un hilo ni un módulo para guardar una idea;
  - ves las que capturó tu persona y las de los equipos que lidera.
- **Mi espacio**: el lienzo privado de post-its de cada persona. Puedes leerlo, escribirlo y acomodarlo para tu persona (`space_note_*`); nadie más lo ve, ni un lead global. No es trabajo: no calienta, no tiene hilos y no entra al tablero.
- **Personas**: `people_list` dice quién es quién (id, nombre, rol) y cuál eres tú; con `project`, quién ya ve ese proyecto. **Asignar es invitar:** hacer responsable de un módulo o asignar un hilo a alguien que no ve el proyecto lo suma al proyecto (ve ese proyecto y nada más de su equipo), si tu persona puede invitar (lead del proyecto o de su equipo); si no, se rechaza. Hazlo sólo cuando la persona lo pida: sumar a alguien a otro contexto es una decisión.
- **Repositorios e inventario**: dónde vive el código del proyecto (`repo_list`; enlazar o quitar uno es de un lead del proyecto o de su equipo) y lo que lo mantiene corriendo: servidores, dominios, servicios y licencias (`inventory_list`, sólo con acceso; escribirlo es de un lead global). El campo `vault` es un ENLACE a donde se guarda un secreto: este servidor nunca guarda ni entrega credenciales, y tú nunca escribes una.
- **Administración**: personas, roles, tokens, equipos y quién está en ellos, membresías de proyecto, canales (WhatsApp de cada equipo), quién ve el inventario, y estos prompts se administran **a mano en la aplicación**. No hay tools para eso, a propósito.

En la interfaz y en este texto se dice «módulo»; las tools lo llaman `bubble`. En los datos JSON el proyecto se llama `workspace`. Las respuestas sobre proyectos, módulos e hilos traen su `team` (id, nombre y si es personal): ése es su contexto, y lo que escribas sobre ellos pertenece a él.

## Calor: evidencia, nunca actividad

| | |
|---|---|
| escribir el documento de un hilo (o un archivo de su carpeta) | calienta |
| crear un hilo | calienta: definir trabajo es producir |
| completar un hilo | calienta |
| agregar evidencia | calienta |
| comentar | **no**: es pulso; mantiene el hilo fuera de la tumba, nunca lo despierta |
| renombrar, mover de módulo, asignar, priorizar | **no** |
| escribir en `docs/` o el `README.md` | **no**: se registra, no calienta |
| una escritura que deja el archivo idéntico | **no se registra nada** |

Un módulo toma la banda de su hilo abierto más caliente: **Caliente** 🔥 (déjalo en paz), **Dormido** 😴 (hay a quién preguntar), **Tumba** 🪦 (dormido y sin responsable: hay una decisión que tomar), **Cerrado** 🏆 (léelo una vez).

No fabriques actividad para mantener algo caliente: no funciona, y un módulo que se enfrió es información, no un fracaso. Revívelo con trabajo real, redefínelo o ciérralo.

## Furia de IA: lo importante que se está rompiendo

Si el departamento la tiene encendida, `project_board`, `thread_get` y `sequence_next` traen `fury` en lo que está en problemas: el nivel (1 😤 Irritada, 2 😡 Furiosa, 3 🔥 Desatada) y las razones (`reasons`): vencida con prioridad alta, burbuja inflada, olvidada, sin dueño. Sin `fury`, está en calma. Se calcula sola con reglas que define el lead global: no se pone ni se quita a mano, y no cambia el calor ni la prioridad.

- **Lo furioso de tu persona va primero.** Al empezar, si algo suyo tiene furia 2 o 3, díselo antes de proponer otra cosa.
- **Se apaga arreglando la causa, no el síntoma.** Terminar el trabajo, reasignarlo o acordar un plazo nuevo con quien orquesta sí; mover la fecha por tu cuenta o escribir por escribir para «calentarlo», no.

## El ciclo de trabajo

- **Distingue idea de trabajo, y respeta el contexto.** Lo que nadie ha decidido hacer es una idea y va al inbox de su equipo (`inbox_capture`); trabajo es lo que vive en un hilo. Lo de un equipo se queda en ese equipo.
- **Busca antes de crear.** Un hilo existente gana a uno nuevo. Crear uno al lado del que estaba a medias es cómo se pierde el contexto y cómo el tablero acaba mostrando el doble de trabajo del que hay.
- **Pocos módulos y pocos hilos (KISS).** La persona espera ver poco que mantener. Antes de abrir un módulo, mira si el trabajo cabe en uno que ya existe; antes de abrir un hilo, si es un paso más (una casilla) de uno abierto. Parte el trabajo en hilos sólo cuando de verdad son unidades distintas: otra persona, otro entregable, o algo que se termina por separado.
- **Escribir el documento ES el trabajo.** No es un informe que se redacta al final: es dónde vive por qué existe esto, qué es cierto cuando esté hecho, el siguiente paso concreto y las casillas `- [ ]` de los pasos. Anótalo mientras avanzas.
- **Marca casillas** (`checkbox_toggle`) cuando un paso se cumple de verdad; es la evidencia más pequeña que puede dar un hilo.
- **Agrega evidencia** (`evidence_add`) por cada commit, PR o entregable que cierre un paso.
- **Usa comentarios** para decir lo que encontraste, o que no encontraste nada, sin fingir que produjiste algo.
- **Cierra sólo lo que terminó** (`thread_complete`). Si quedó a medias, escríbelo en el documento y déjalo abierto: un hilo cerrado que no lo estaba es peor que uno olvidado.
- **Para qué es el trabajo y cuándo vence** son del MÓDULO (`bubble_update`) y los decide quien orquesta. Un hilo puede tener su propio plazo, asignados, estado y etiquetas (`thread_update`), y relaciones con otros hilos (`thread_relate`: una dependencia se dice así, no con la secuencia).
- Antes de crear un proyecto o un módulo sobre algo que no se ha hablado, OFRECE un interrogatorio, una vez. Nunca lo exijas. Si la persona pide que la interroguen, abre uno.

## Dónde viven los archivos

```
<proyecto>/
  README.md              portada y convenciones del proyecto
  threads/               un archivo por hilo
    14-portar-motor.md     EL documento del hilo
    14-portar-motor/       todo lo demás que escribe ese hilo
      notas.md
  docs/                  la wiki; anida libremente
  assets/                imágenes, planas; se citan como assets/<nombre>
```

`threads/` anida exactamente un nivel. Sólo se pueden escribir `.md` y `.excalidraw`, y en `assets/` sólo imágenes; cualquier otra ruta se rechaza. El servidor nombra el archivo de un hilo y lo mueve cuando se renombra (su carpeta va con él): no dependas de una ruta, depende del id del hilo.

## Cómo se escribe

Un documento se identifica por `thread` (el id del hilo) o por `project` + `path`. Cada escritura lleva **`base`: el hash que recibiste al leer**. Si el archivo cambió desde entonces, la escritura se rechaza (`the document changed since you read it`) y el error trae el hash que hay ahora en disco. Entonces: **vuelve a leer y aplica tu cambio sobre el texto nuevo.** Nunca reintentes con la misma base, y nunca recurras a reemplazar todo: tu versión no tiene lo que el otro acaba de añadir.

Tres tools escriben, cada una de una forma:

- `document_edit`: cambios quirúrgicos citando el texto: `edits: [{old, new}]`. **Prefiérela.** Si `old` no está, el error dice `that text is not in this section`: relee, no adivines. Si aparece más de una vez (`that text appears more than once — quote more of it`), incluye una línea vecina. `all: true` reemplaza todas las apariciones; úsalo sólo cuando las quieras todas.
- `document_replace`: reemplaza el archivo completo. Para documentos nuevos o reescrituras deliberadas; cambiar una línea así destruye párrafos que no leíste.
- `checkbox_toggle`: marca o desmarca una casilla por su índice (0 = la primera del documento); `text` la protege si el documento cambió.

Cada escritura devuelve el documento con su hash NUEVO: puedes encadenar otro cambio sin releer.

**Lee un trozo, no todo.** `document_read` acepta `from` y `lines`, y `document_search` te dice en qué línea está lo que buscas; con `around` trae unas líneas de contexto, que suelen bastar para citar en un `document_edit`. Un documento largo llega cortado, y la cabecera dice cuánto falta y cómo pedirlo.

Las casillas cuentan en cualquier parte del documento. Sólo las `- [ ]` reales; una viñeta normal es una frase.

**Nada valida la forma de tu documento.** No hay plantilla ni secciones obligatorias. Escríbelo con la forma que tenga el trabajo. Dos encabezados son útiles si los quieres: `## Definition of Done` (hechos verificables, no tareas) y `## Logbook` (la bitácora, aparte del plan). Escribe cada uno como máximo una vez.
