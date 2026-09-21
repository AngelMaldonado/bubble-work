<script lang="ts" module>
  export type SeqThread = {
    id: string;
    /** el #N del hilo dentro de su proyecto */
    seq: number;
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
    assignees: { id: string; name: string; avatar?: string }[];
  };
</script>

<script lang="ts">
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
    onreorder,
    oncomplete,
    onopen,
    readonly = false,
    title = '🧭 Secuencia',
    headless = false,
  }: {
    threads?: SeqThread[];
    /** sólo mirar: sin arrastre, sin «Terminar» y sin la bandeja de lo que no
     *  está secuenciado. Es la secuencia de la columna, no la del planeador. */
    readonly?: boolean;
    title?: string;
    /** sin cabecera propia: quien lo contiene pone la suya (la columna del
     *  kanban, que es además el asa para moverla) */
    headless?: boolean;
    /** sólo los hilos cuyo `sequence` cambió: el resto no hay que tocarlo */
    onreorder?: (changes: { id: string; sequence: number }[]) => void | Promise<void>;
    oncomplete?: (id: string) => void | Promise<void>;
    onopen?: (t: SeqThread) => void;
  } = $props();

  // Copia local, para que un arrastre se vea al instante y no cuando vuelva el
  // servidor. Se re-deriva cada vez que llegan `threads` nuevos: la verdad sigue
  // siendo del dueño, esto sólo adelanta lo que va a decir.
  let local = $derived(threads.map((t) => ({ ...t })));

  type Step = { seq: number; threads: SeqThread[]; done: boolean };

  const steps = $derived.by<Step[]>(() => {
    const by = new Map<number, SeqThread[]>();
    for (const t of local) {
      if (!t.sequence) continue;
      const list = by.get(t.sequence);
      if (list) list.push(t);
      else by.set(t.sequence, [t]);
    }
    return [...by.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([seq, ts]) => ({ seq, threads: ts, done: ts.every((t) => t.done) }));
  });
  const doneSteps = $derived(steps.filter((s) => s.done));
  const liveSteps = $derived(steps.filter((s) => !s.done));

  // Lo terminado se pliega por defecto: ya no pide nada, y abierto empuja
  // «Ahora» fuera de la vista, que es justo lo que esta pantalla tiene que decir.
  let showDone = $state(false);
  let showTray = $state(true);

  // Sin secuenciar sólo cuenta lo que sigue abierto: un hilo terminado que nunca
  // entró en la línea no tiene ya dónde ir.
  let fProject = $state('');
  let fPrio = $state('');
  const unsequenced = $derived(local.filter((t) => !t.sequence && !t.done));
  const projects = $derived([...new Set(unsequenced.map((t) => t.project))].sort());
  const tray = $derived(
    unsequenced.filter(
      (t) =>
        (!fProject || t.project === fProject) &&
        (!fPrio || (fPrio === 'none' ? !t.priority : t.priority === fPrio)),
    ),
  );

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
    type Slot = { ids: string[] };
    const done: Slot[] = doneSteps.map((s) => ({ ids: s.threads.map((t) => t.id) }));
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
    for (const s of [...done, ...live]) {
      if (s !== dest) s.ids = s.ids.filter((x) => x !== id);
    }

    const next = new Map<string, number>();
    [...done, ...live]
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
  <article
    class="card"
    class:done={t.done}
    class:lifting={dragging === t.id}
    {@attach (el) => (movable ? card(el, t) : undefined)}>
    <div class="row">
      {#if t.done}<span class="tick" aria-label="terminado">✓</span>{/if}
      {#if t.priority}<span class="prio-chip prio-{t.priority}">{t.priority}</span>{/if}
      <span class="num">#{t.seq}</span>
      <button class="name" onclick={() => onopen?.(t)} title={t.name}>{t.name}</button>
    </div>
    <div class="row sub">
      <span class="where">{t.project}{t.bubble ? ` · ${t.bubble}` : ''}</span>
      {#if t.assignees.length}
        <span class="people">
          {#each t.assignees as p (p.id)}
            <span class="avatar" title={p.name}>
              {#if p.avatar}
                <img src={p.avatar} alt={p.name} />
              {:else}
                {(p.name || '?').slice(0, 1).toUpperCase()}
              {/if}
            </span>
          {/each}
        </span>
      {/if}
      {#if !t.done && oncomplete && !readonly}
        <!-- Fuera del botón del nombre: terminar algo por haber apuntado a
             abrirlo es un clic que nadie perdona. -->
        <button class="finish" onclick={() => oncomplete?.(t.id)}>✓ Terminar</button>
      {/if}
    </div>
  </article>
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
    </header>
  {/if}

  <div class="split">
    <div class="scroll">
    {#if doneSteps.length}
      <section class="done-steps">
        <button class="toggle" aria-expanded={showDone} onclick={() => (showDone = !showDone)}>
          ✓ Hecho ({doneSteps.length})
        </button>
        {#if showDone}
          <!-- No es destino: lo hecho ya pasó, y meter algo ahí sería reescribir
               la historia de la línea. -->
          {#each doneSteps as s (s.seq)}
            <div class="step past">
              <div class="rail"><span class="marker">✓</span></div>
              <div class="body">
                {#each s.threads as t (t.id)}{@render threadCard(t, false)}{/each}
              </div>
            </div>
          {/each}
        {/if}
      </section>
    {/if}

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
            : 'Arrastra hilos desde «Sin secuenciar» para armar la secuencia.'}
        </div>
      </div>
    {/if}

    </div>

    <!-- Al COSTADO, y no plegada al pie: secuenciar es arrastrar de aquí a la
         línea, y un origen que hay que desplegar primero —y que empuja la
         línea hacia abajo al hacerlo— convierte un gesto en tres. -->
    {#if !readonly}
    <section
      class="tray"
      class:shut={!showTray}
      class:over={over === 'tray'}
      {@attach (el) => target(el, { kind: 'tray' })}>
      <div class="tray-head">
        <button
          class="toggle"
          aria-expanded={showTray}
          title={showTray ? 'Esconder la bandeja' : 'Mostrar la bandeja'}
          onclick={() => (showTray = !showTray)}>
          {showTray ? '▸' : '◂'} {showTray ? 'Sin secuenciar' : ''} ({unsequenced.length})
        </button>
      </div>
      {#if showTray}
        <!-- Los filtros, apilados: en una columna estrecha dos selects uno al
             lado del otro no caben sin cortarse. -->
        <div class="tray-filters">
          <select class="select filter" bind:value={fProject} aria-label="filtrar por proyecto">
            <option value="">todos los proyectos</option>
            {#each projects as p (p)}<option value={p}>{p}</option>{/each}
          </select>
          <select class="select filter" bind:value={fPrio} aria-label="filtrar por prioridad">
            <option value="">todas las prioridades</option>
            <option value="P1">P1</option>
            <option value="P2">P2</option>
            <option value="P3">P3</option>
            <option value="P4">P4</option>
            <option value="none">sin prioridad</option>
          </select>
        </div>
        <div class="tray-body">
          {#each tray as t (t.id)}{@render threadCard(t, true)}{/each}
          {#if !tray.length}
            <p class="hint">{unsequenced.length ? 'Nada con esos filtros.' : 'Todo lo abierto ya está en la secuencia.'}</p>
          {/if}
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
    flex: 1 1 auto;
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

  .done-steps { display: flex; flex-direction: column; gap: 0.35rem; }

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
  .step.past { opacity: 0.55; }

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

  .card {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.45rem 0.55rem;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--surface-solid);
    cursor: grab;
  }
  .card:hover { border-color: color-mix(in oklab, var(--accent) 40%, transparent); }
  .card.done { opacity: 0.55; }
  .card.lifting { opacity: 0.4; }
  .row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
  }
  .tick { color: var(--muted); font-size: 0.78rem; }
  .num { flex: none; color: var(--faint); font-size: 0.72rem; font-variant-numeric: tabular-nums; }
  .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
    color: var(--text);
    font-size: 0.86rem;
  }
  .name:hover { text-decoration: underline; }
  .card.done .name { text-decoration: line-through; }
  .sub { font-size: 0.72rem; }
  .where {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--faint);
  }
  .people { display: flex; margin-left: auto; }
  .avatar {
    display: grid;
    place-content: center;
    width: 22px;
    height: 22px;
    margin-left: -5px;
    overflow: hidden;
    border: 2px solid var(--surface-solid);
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.66rem;
    font-weight: 700;
  }
  .avatar:first-child { margin-left: 0; }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }
  .finish {
    flex: none;
    padding: 0.1rem 0.4rem;
    border-radius: 6px;
    color: var(--faint);
    font-size: 0.72rem;
  }
  .people + .finish { margin-left: 0.2rem; }
  .where + .finish { margin-left: auto; }
  .finish:hover { background: var(--hover); color: var(--text); }

  .tray {
    flex: 0 0 auto;
    width: clamp(8.5rem, 34%, 13rem);
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    padding: 0.45rem;
    border: 1px dashed var(--line);
    border-radius: 10px;
  }
  /* Escondida ocupa lo que mide su botón, no una columna vacía. */
  .tray.shut {
    width: auto;
    overflow: visible;
  }
  .tray.over { border-color: var(--accent); background: color-mix(in oklab, var(--accent) 8%, transparent); }
  .tray-head {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.35rem;
  }
  .tray-head .toggle { margin-right: auto; white-space: nowrap; }
  .tray-filters { display: flex; flex-direction: column; gap: 0.3rem; }
  .filter { width: 100%; padding: 0.1rem 1.6rem 0.1rem 0.45rem; font-size: 0.74rem; }
  .tray-body {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .hint { color: var(--faint); font-size: 0.74rem; padding: 0.2rem 0.3rem; }
</style>
