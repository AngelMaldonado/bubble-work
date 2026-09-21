<script lang="ts" module>
  export type SeqThread = {
    id: string;
    /** el #N del hilo dentro de su proyecto. Un MÓDULO no tiene: es una fila de
     *  la línea igual que un hilo, pero las burbujas no llevan número. */
    seq?: number;
    name: string;
    /** el NOMBRE del proyecto, para leerlo */
    project: string;
    /** el nombre de la burbuja; puede venir vacío */
    bubble: string;
    /** 'P1'..'P4', o nada */
    priority?: string;
    /** su paso en la secuencia. 0 = no está en ella. Dos hilos con el mismo
     *  valor son UN paso y van en paralelo. */
    sequence: number;
    /** su lugar en la línea viva de TODO el departamento (1 = ahora), si se
     *  sabe. Una vista filtrada lo usa para no renumerar lo que no ve. */
    step?: number;
    done: boolean;
    /** cuándo nació: la tarjeta lo dice como edad, y es por lo que se ordenan
     *  las columnas laterales */
    created?: string;
    /** cuándo vence, un día */
    due?: string;
    assignees: { id: string; name: string; avatar?: string }[];
  };
</script>

<script lang="ts">
  import BoardCard from './BoardCard.svelte';
  // La secuencia de ejecución del departamento: UNA línea, la que decide quien
  // lleva todo. Cada paso es un valor de `sequence`, y los hilos que lo
  // comparten van a la vez.
  //
  // «Ahora» no se guarda en ningún sitio: es el primer paso que aún tiene algo
  // sin terminar. Guardarlo sería un segundo dato que alguien tendría que
  // acordarse de mover; derivado, terminar hilos hace avanzar la línea sola.
  //
  // El arrastre es el mismo motor que el Kanban (pragmatic-drag-and-drop), y
  // por la misma razón se carga a demanda: una secuencia que sólo se lee no
  // tiene por qué descargar un motor de arrastre.
  let {
    threads = [],
    queue = [],
    queueMore = false,
    onqueuemore,
    done = [],
    doneMore = false,
    ondonemore,
    onreorder,
    oncomplete,
    onopen,
    onnew,
    onreopen,
    onreview,
    readonly = false,
    title = '🧭 Secuencia',
    completeLabel = '✓ Terminar',
    what = 'hilos',
    headless = false,
  }: {
    threads?: SeqThread[];
    /** sólo mirar: sin arrastre, sin «Terminar» y sin la bandeja de lo que no
     *  está secuenciado. Es la secuencia de la columna, no la del planeador. */
    readonly?: boolean;
    title?: string;
    /** Cómo se llama dar por terminada una fila. Un hilo se TERMINA; un módulo
     *  se CIERRA, que es una decisión con fecha y frase. */
    completeLabel?: string;
    /** Qué se ordena aquí, para decirlo donde no hay nada que ordenar. */
    what?: string;
    /** sin cabecera propia: quien lo contiene pone la suya (la columna del
     *  kanban, que es además el asa para moverla) */
    headless?: boolean;
    /** Lo que todavía no está en la línea, lo más reciente primero. Lo pagina
     *  quien lo pide: esta lista crece sin techo y no cabe entera. */
    queue?: SeqThread[];
    queueMore?: boolean;
    onqueuemore?: () => void;
    /** Lo que ya se terminó, también paginado. */
    done?: SeqThread[];
    doneMore?: boolean;
    ondonemore?: () => void;
    /** sólo los hilos cuyo `sequence` cambió: el resto no hay que tocarlo */
    onreorder?: (changes: { id: string; sequence: number }[]) => void | Promise<void>;
    oncomplete?: (id: string) => void | Promise<void>;
    onopen?: (t: SeqThread) => void;
    /** crear algo nuevo y meterlo en la línea; sin esto no se ofrece */
    onnew?: () => void;
    /** algo terminado vuelve a la línea: hay que reabrirlo */
    onreopen?: (id: string) => void;
    /** dar por revisado y cerrado del todo lo que ya estaba hecho */
    onreview?: (id: string) => void;
  } = $props();

  // Copia local, para que un arrastre se vea al instante y no cuando vuelva el
  // servidor. Se re-deriva cada vez que llegan `threads` nuevos: la verdad sigue
  // siendo del dueño, esto sólo adelanta lo que va a decir.
  let local = $derived(threads.map((t) => ({ ...t })));

  type Step = { seq: number; threads: SeqThread[]; done: boolean };

  /** Los pasos de la línea. SÓLO trabajo vivo: lo terminado vive en la columna
   *  de la derecha, y tenerlo además aquí era la misma cosa en dos sitios —
   *  empujando «Ahora» fuera de la vista, que es lo único que esta pantalla
   *  tiene que decir. */
  const steps = $derived.by<Step[]>(() => {
    const by = new Map<number, SeqThread[]>();
    for (const t of local) {
      if (!t.sequence || t.done) continue;
      const list = by.get(t.sequence);
      if (list) list.push(t);
      else by.set(t.sequence, [t]);
    }
    return [...by.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([seq, ts]) => ({ seq, threads: ts, done: false }));
  });
  const liveSteps = $derived(steps);

  // Las dos columnas laterales ya no se derivan de `threads`: llegan del
  // servidor, por páginas, ordenadas por fecha de creación. Filtrarlas aquí
  // sería filtrar una página y no el conjunto — para acotar está la barra de
  // filtros del planeador, que filtra en el sitio donde se pide.
  let showTray = $state(true);
  let showDone = $state(true);

  /** El centinela del final de una columna: cuando entra en pantalla, se pide
   *  la página siguiente. Un observador y no un `scroll`: el navegador ya sabe
   *  cuándo algo se ve, y escuchar el scroll es preguntarlo sesenta veces por
   *  segundo. */
  function sentinel(more: boolean, ask?: () => void) {
    return (el: HTMLElement) => {
      if (!more || !ask) return;
      const io = new IntersectionObserver(
        (entries) => entries.some((e) => e.isIntersecting) && ask(),
        // Un poco antes de llegar: la página siguiente aterriza mientras
        // todavía queda algo que leer.
        { root: el.closest('.side'), rootMargin: '240px' },
      );
      io.observe(el);
      return () => io.disconnect();
    };
  }

  /** El número de un paso: el de la línea entera si el hilo lo trae —una vista
   *  filtrada no ve todos los pasos, y renumerar lo visible diría «ahora» de algo
   *  que no lo es— o su posición aquí. */
  function stepNumber(s: { threads: SeqThread[] }, i: number): number {
    return (readonly && s.threads.find((t) => t.step)?.step) || i + 1;
  }

  function stepLabel(i: number): string {
    if (i === 0) return 'Ahora';
    if (i === 1) return 'Siguiente';
    // Del tercero en adelante basta el número del carril: ponerle nombre a
    // cada paso haría que «Ahora» y «Siguiente» dejaran de destacar.
    return '';
  }

  /** Dónde caería lo que se arrastra ahora mismo, como clave de destino. Se
   *  dibuja, para que la pantalla enseñe el resultado en vez de pedir que uno
   *  se lo imagine. */
  let over = $state<string | null>(null);
  let dragging = $state<string | null>(null);

  type Target = { kind: 'step'; seq: number } | { kind: 'gap'; at: number } | { kind: 'tray' };

  function keyOf(t: Target): string {
    if (t.kind === 'step') return `step:${t.seq}`;
    if (t.kind === 'gap') return `gap:${t.at}`;
    return 'tray';
  }

  /** Recoloca un hilo y renumera TODA la línea de 10 en 10. Renumerar entera, y
   *  no buscar un hueco entre dos valores, es lo que mantiene la secuencia
   *  legible: sin 15, 17, 18 que acaban sin sitio entre medias. Al dueño sólo
   *  le llega lo que cambió. */
  function drop(id: string, target: Target) {
    // Lo que viene de una columna lateral todavía no está en la línea, así que
    // primero entra. Si venía de lo hecho, se REABRE: «ahora» es el primer paso
    // con algo abierto, y colocar algo terminado en la línea la dejaría
    // pareciendo atascada en trabajo que ya está.
    if (!local.some((t) => t.id === id)) {
      const from = [...queue, ...done].find((t) => t.id === id);
      if (!from) return;
      if (from.done) onreopen?.(from.id);
      local = [...local, { ...from, sequence: 0, done: false }];
    }

    type Slot = { ids: string[] };
    const live: Slot[] = liveSteps.map((s) => ({ ids: s.threads.map((t) => t.id) }));

    // El destino se coloca ANTES de quitar el hilo de donde estaba: si su paso
    // de origen se queda vacío y desaparece, los índices de los huecos se
    // correrían, y el hilo caería un paso más allá de donde se apuntó.
    let dest: Slot | null = null;
    if (target.kind === 'step') {
      const i = liveSteps.findIndex((s) => s.seq === target.seq);
      if (i < 0) return;
      dest = live[i];
      if (dest.ids.includes(id)) return;
      dest.ids = [...dest.ids, id];
    } else if (target.kind === 'gap') {
      dest = { ids: [id] };
      live.splice(target.at, 0, dest);
    }
    for (const s of live) {
      if (s !== dest) s.ids = s.ids.filter((x) => x !== id);
    }

    const next = new Map<string, number>();
    live
      .filter((s) => s.ids.length)
      .forEach((s, i) => s.ids.forEach((x) => next.set(x, (i + 1) * 10)));

    const changes: { id: string; sequence: number }[] = [];
    local = local.map((t) => {
      const sequence = next.get(t.id) ?? 0;
      if (sequence === t.sequence) return t;
      changes.push({ id: t.id, sequence });
      return { ...t, sequence };
    });
    if (changes.length) onreorder?.(changes);
  }

  function dnd() {
    return Promise.all([
      import('@atlaskit/pragmatic-drag-and-drop/element/adapter'),
      import('@atlaskit/pragmatic-drag-and-drop/combine'),
    ]);
  }

  /** Una tarjeta es sólo origen. Los destinos son los pasos, los huecos y la
   *  bandeja; si las tarjetas también lo fueran, el destino bajo el puntero
   *  dependería de sobre qué tarjeta se pasa, y no de dónde se quiere soltar. */
  function card(el: HTMLElement, t: SeqThread) {
    let stop: (() => void) | undefined;
    let live = true;
    dnd().then(([{ draggable }]) => {
      if (!live) return;
      stop = draggable({
        element: el,
        getInitialData: () => ({ seqThread: t.id }),
        onDragStart: () => (dragging = t.id),
        onDrop: () => {
          dragging = null;
          over = null;
        },
      });
    });
    return () => {
      live = false;
      stop?.();
    };
  }

  function target(el: HTMLElement, tg: Target) {
    let stop: (() => void) | undefined;
    let live = true;
    dnd().then(([{ dropTargetForElements }]) => {
      if (!live) return;
      stop = dropTargetForElements({
        element: el,
        // Sólo un hilo de ESTA secuencia: el planeador tiene otros arrastres, y
        // una columna soltada aquí no significa nada.
        canDrop: ({ source }) => typeof source.data.seqThread === 'string',
        getData: () => ({ seqTarget: keyOf(tg) }),
        onDragEnter: () => (over = keyOf(tg)),
        onDragLeave: () => {
          if (over === keyOf(tg)) over = null;
        },
      });
    });
    return () => {
      live = false;
      stop?.();
    };
  }

  /** Un solo monitor para todo el panel. La posición sale del destino sobre el
   *  que se SOLTÓ, no del último `onDragEnter` visto: el puntero puede salir de
   *  un destino camino del botón del ratón, y entonces el hilo caería donde
   *  apuntaba un valor ya viejo. */
  function monitor(_el: HTMLElement) {
    let stop: (() => void) | undefined;
    let live = true;
    dnd().then(([{ monitorForElements }]) => {
      if (!live) return;
      stop = monitorForElements({
        canMonitor: ({ source }) => typeof source.data.seqThread === 'string',
        onDrop: ({ source, location }) => {
          const key = location.current.dropTargets[0]?.data.seqTarget as string | undefined;
          over = null;
          dragging = null;
          if (!key) return;
          const id = source.data.seqThread as string;
          if (key === 'tray') drop(id, { kind: 'tray' });
          else if (key.startsWith('step:')) drop(id, { kind: 'step', seq: Number(key.slice(5)) });
          else if (key.startsWith('gap:')) drop(id, { kind: 'gap', at: Number(key.slice(4)) });
        },
      });
    });
    return () => {
      live = false;
      stop?.();
    };
  }
</script>

{#snippet threadCard(t: SeqThread, movable: boolean)}
  <!-- La MISMA tarjeta que el kanban. La secuencia dibujaba la suya, parecida
       pero no igual, y dos tarjetas de la misma cosa acaban divergiendo. Lo
       único que añade una fila de la línea es quién la tiene. -->
  <BoardCard
    card={{
      id: t.id,
      title: t.name,
      prio: t.priority || undefined,
      where: t.project,
      born: t.created,
      due: t.due,
    }}
    people={t.assignees}
    lifting={dragging === t.id}
    attach={movable ? (el) => card(el, t) : undefined}
    onopen={() => onopen?.(t)}>
    <!-- Dentro de la tarjeta y fuera del botón que la abre: son verbos suyos,
         pero terminar algo por haber apuntado a abrirlo es un clic que nadie
         perdona. Como snippet hijo, para que vea el `t` de esta fila. -->
    {#snippet footer()}
      {#if !t.done && oncomplete && !readonly}
        <button class="finish" onclick={() => oncomplete?.(t.id)}>{completeLabel}</button>
      {/if}
      {#if t.done && onreview}
        <!-- Revisar lo saca de la columna para siempre: es lo que la convierte
             en una cola que se vacía y no en un historial que sólo crece.
             Siempre visible, y con su nombre — un botón que sólo aparece al
             pasar por encima es un botón que la mitad de la gente no descubre. -->
        <button
          class="review"
          title="Dar por revisado: sale de esta columna"
          onclick={() => onreview?.(t.id)}>☑ Revisado</button>
      {/if}
    {/snippet}
  </BoardCard>
{/snippet}

{#snippet gap(at: number)}
  <!-- Un hueco entre pasos: soltar aquí crea un paso NUEVO en esa posición.
       Crece mientras se arrastra, que un blanco de tres píxeles no se acierta. -->
  <div
    class="gap"
    class:armed={!!dragging}
    class:over={over === `gap:${at}`}
    aria-hidden="true"
    {@attach (el) => target(el, { kind: 'gap', at })}>
  </div>
{/snippet}

<div class="pane" {@attach monitor}>
  {#if !headless}
    <header class="head">
      <span class="pane-name">{title}</span>
      <span class="count" title="pasos en la secuencia">{steps.length}</span>
      {#if onnew && !readonly}
        <button class="new" onclick={onnew} title="Crear y meter en la línea">+</button>
      {/if}
    </header>
  {/if}

  <div class="split">
    <!-- La columna IZQUIERDA: lo que todavía no está en la línea, lo más
         reciente primero. Arrastrar de aquí al centro es meter algo en la
         línea; del centro aquí, sacarlo. -->
    {#if !readonly}
      <section
        class="side left"
        class:shut={!showTray}
        class:over={over === 'tray'}
        {@attach (el) => target(el, { kind: 'tray' })}>
        <div class="side-head">
          <button
            class="toggle"
            aria-expanded={showTray}
            title={showTray ? 'Esconder la bandeja' : 'Mostrar la bandeja'}
            onclick={() => (showTray = !showTray)}>
            {showTray ? '◂' : '▸'} {showTray ? 'Sin secuenciar' : ''}
          </button>
        </div>
        {#if showTray}
          <div class="side-body">
            {#each queue as t (t.id)}{@render threadCard(t, true)}{/each}
            {#if !queue.length}
              <p class="hint">Todo lo abierto ya está en la línea.</p>
            {/if}
            <div class="more" {@attach sentinel(queueMore, onqueuemore)}>
              {#if queueMore}<span class="hint">cargando…</span>{/if}
            </div>
          </div>
        {/if}
      </section>
    {/if}

    <div class="scroll">
    {#if liveSteps.length}
      <div class="line">
        {#if !readonly}{@render gap(0)}{/if}
        {#each liveSteps as s, i (s.seq)}
          <div
            class="step"
            class:now={stepNumber(s, i) === 1}
            class:over={over === `step:${s.seq}`}
            {@attach (el) => (readonly ? undefined : target(el, { kind: 'step', seq: s.seq }))}>
            <div class="rail"><span class="marker">{stepNumber(s, i)}</span></div>
            <div class="body">
              {#if stepLabel(stepNumber(s, i) - 1) || s.threads.length > 1}
                <div class="label">
                  {#if stepLabel(stepNumber(s, i) - 1)}<span>{stepLabel(stepNumber(s, i) - 1)}</span>{/if}
                  {#if s.threads.length > 1}<span class="parallel">en paralelo</span>{/if}
                </div>
              {/if}
              {#each s.threads as t (t.id)}{@render threadCard(t, !readonly)}{/each}
            </div>
          </div>
          {#if !readonly}{@render gap(i + 1)}{/if}
        {/each}
      </div>
    {:else}
      <div class="line">
        <!-- Vacía, la línea entera es el hueco del primer paso: si no, no habría
             dónde soltar el primer hilo. -->
        <div
          class="empty"
          class:over={over === 'gap:0'}
          {@attach (el) => (readonly ? undefined : target(el, { kind: 'gap', at: 0 }))}>
          {readonly
            ? 'Nada en la secuencia todavía.'
            : `Arrastra ${what} desde «Sin secuenciar» para armar la secuencia.`}
        </div>
      </div>
    {/if}

    </div>

    <!-- La columna DERECHA: lo que ya se terminó, lo más reciente primero. No
         es destino de caída — lo hecho ya pasó, y meter algo ahí sería
         reescribir la historia de la línea. -->
    {#if done.length || doneMore}
      <section class="side right" class:shut={!showDone}>
        <div class="side-head">
          <button
            class="toggle"
            aria-expanded={showDone}
            title={showDone ? 'Esconder lo hecho' : 'Mostrar lo hecho'}
            onclick={() => (showDone = !showDone)}>
            {showDone ? '▸' : '◂'} {showDone ? '✓ Hecho' : '✓'}
          </button>
        </div>
        {#if showDone}
          <div class="side-body">
            {#each done as t (t.id)}{@render threadCard(t, true)}{/each}
            {#if !done.length}<p class="hint">Todavía no hay nada terminado.</p>{/if}
            <div class="more" {@attach sentinel(doneMore, ondonemore)}>
              {#if doneMore}<span class="hint">cargando…</span>{/if}
            </div>
          </div>
        {/if}
      </section>
    {/if}
  </div>
</div>

<style>
  .pane {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.35rem 0.55rem;
  }
  .pane-name {
    font-size: 0.78rem;
    font-weight: 700;
    color: var(--muted);
  }
  .new {
    margin-left: auto;
    padding: 0 0.45rem;
    border-radius: 7px;
    color: var(--faint);
    font-size: 0.95rem;
    line-height: 1.4;
  }
  .new:hover { background: var(--hover); color: var(--text); }
  .count {
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--faint);
    font-size: 0.68rem;
  }
  /* La línea y la bandeja, lado a lado. Quien se estrecha primero es la
     bandeja: la línea es lo que se lee, la bandeja es de donde se saca. */
  .split {
    flex: 1;
    min-height: 0;
    display: flex;
    gap: 0.4rem;
  }
  .scroll {
    flex: 1 1 0;
    min-width: 0;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 0 0.2rem 0.6rem;
  }

  .toggle {
    padding: 0.25rem 0.45rem;
    border-radius: 7px;
    color: var(--faint);
    font-size: 0.74rem;
    text-align: left;
  }
  .toggle:hover { background: var(--hover); color: var(--text); }

  
  .line { display: flex; flex-direction: column; }

  .step {
    display: flex;
    gap: 0.55rem;
    padding: 0.45rem;
    border: 1px solid transparent;
    border-radius: 10px;
  }
  /* «Ahora» es lo único que esta pantalla tiene que decir de un vistazo. */
  .step.now {
    border-color: color-mix(in oklab, var(--accent) 45%, transparent);
    background: color-mix(in oklab, var(--accent) 12%, transparent);
  }
  .step.over {
    border-color: var(--accent);
    background: color-mix(in oklab, var(--accent) 18%, transparent);
  }

  .rail {
    flex: none;
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 22px;
  }
  /* El carril sigue por debajo del número: una línea, no una lista de cajas. */
  .rail::after {
    content: '';
    flex: 1;
    width: 2px;
    margin-top: 0.25rem;
    border-radius: 999px;
    background: var(--line);
  }
  .marker {
    display: grid;
    place-content: center;
    width: 22px;
    height: 22px;
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.72rem;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .now .marker { background: var(--accent); color: var(--surface-solid); }

  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .label {
    display: flex;
    align-items: baseline;
    gap: 0.45rem;
    font-size: 0.74rem;
    font-weight: 700;
    color: var(--muted);
  }
  .now .label { color: var(--text); }
  .parallel { font-weight: 400; font-size: 0.68rem; color: var(--faint); }

  /* El hueco: fino en reposo para no separar la línea, generoso mientras se
     arrastra, y una raya de acento cuando soltar ahí crearía un paso. */
  .gap {
    position: relative;
    height: 0.35rem;
    transition: height 0.12s ease;
  }
  .gap.armed { height: 1.1rem; }
  .gap.over::after {
    content: '';
    position: absolute;
    left: 0.3rem;
    right: 0.3rem;
    top: 50%;
    height: 3px;
    margin-top: -1.5px;
    border-radius: 999px;
    background: var(--accent);
  }

  .empty {
    padding: 1rem 0.8rem;
    border: 1px dashed var(--line);
    border-radius: 10px;
    color: var(--faint);
    font-size: 0.78rem;
    text-align: center;
  }
  .empty.over { border-color: var(--accent); color: var(--text); }

  .finish {
    flex: none;
    padding: 0.1rem 0.4rem;
    border-radius: 6px;
    color: var(--faint);
    font-size: 0.72rem;
  }
  .review {
    padding: 0.05rem 0.4rem;
    border-radius: 6px;
    color: var(--faint);
    font-size: 0.7rem;
    line-height: 1.4;
  }
  .review:hover { background: var(--hover); color: var(--accent); }
  .finish:hover { background: var(--hover); color: var(--text); }

  /* Las tres columnas: lo que falta, la línea, y lo hecho. Las laterales son
     estrechas y fijas; el centro se queda con lo que sobre — es lo que se lee.
     Si el panel se estrecha, se pliegan a una tira, como hace la columna de
     terminadas del kanban. */
  .side {
    /* Un tercio cada una, como el centro: las tres columnas son la misma
       pregunta en tres momentos —lo que falta, lo que se hace, lo hecho— y
       ninguna manda sobre las otras. */
    flex: 1 1 0;
    min-width: 8rem;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    padding: 0.45rem;
    border: 1px dashed var(--line);
    border-radius: 10px;
    overflow-y: auto;
  }
  /* Plegada ocupa lo que mide su botón, no una columna vacía. */
  .side.shut { flex: 0 0 auto; min-width: 0; overflow: visible; }
  .side.over { border-color: var(--accent); background: color-mix(in oklab, var(--accent) 8%, transparent); }
  /* Lo hecho ya pasó: se lee, no compite. */
  .side.right { border-style: solid; opacity: 0.75; }
  .side.right:hover { opacity: 1; }
  .side-head { display: flex; align-items: center; gap: 0.35rem; }
  .side-head .toggle { margin-right: auto; white-space: nowrap; }
  .side-body { display: flex; flex-direction: column; gap: 0.4rem; }
  /* El centinela del final: sin alto propio no entra nunca en pantalla y la
     página siguiente no se pide jamás. */
  .more { min-height: 1px; padding: 0.2rem 0; text-align: center; }

  .hint { color: var(--faint); font-size: 0.74rem; padding: 0.2rem 0.3rem; }
</style>
