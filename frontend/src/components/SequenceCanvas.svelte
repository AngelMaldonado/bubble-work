<script lang="ts" module>
  /** Lo que entró por WhatsApp y nadie decidió todavía: una nota del inbox. */
  export type SeqEntry = { id: string; text: string; from: string; when: string };
</script>

<script lang="ts">
  // La secuencia como un lienzo: un canvas punteado infinito, una caja de
  // herramientas plegable con lo que falta por secuenciar, y nodos que se
  // conectan arrastrando desde su «+».
  //
  // El modelo NO cambia: la secuencia sigue siendo un número por fila (`sequence`,
  // de 10 en 10; mismo número = en paralelo). Lo que el lienzo dibuja como
  // flechas se DERIVA de esos pasos —de cada nodo de un paso a cada nodo del
  // siguiente— y conectar A → B es decir «B va en el paso justo después de A».
  // Así no hay un segundo dato (las aristas) que pueda contradecir al primero.
  //
  // Lo que es sólo de esta pantalla vive en el navegador: dónde dejó cada quien
  // cada nodo, y los nodos colocados en el lienzo que todavía no conectó.
  import type { Snippet } from 'svelte';
  import type { SeqThread } from './SequencePane.svelte';
  import Glyphs from './Glyphs.svelte';
  import Icon from './Icon.svelte';
  import { Navigation, Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import CheckIcon from '@lucide/svelte/icons/check';
  import Loading from './Loading.svelte';
  import { api } from '../lib/api';
  import { sequenceLayers, connectSequence, insertStep, sequenceChanges } from '../lib/sequencePlan';
  import { linkPath, parallelFrames, sequenceLinks, type Point } from '../lib/sequenceLayout';
  import { projectColors } from '../lib/projectColors.svelte';

  let {
    threads = [],
    queue = [],
    queueMore = false,
    onqueuemore,
    onreorder,
    oncomplete,
    onopen,
    onnew,
    title = '🧭 Secuencia',
    what = 'módulos',
    completeLabel = '🏆 Cerrar',
    modes,
    modesBottom = false,
    readonly = false,
    loading = false,
    edgeToEdge = false,
    entries = [],
    entriesOn = false,
    onentry,
    onentrydrop,
    titleMenu,
  }: {
    threads?: SeqThread[];
    queue?: SeqThread[];
    queueMore?: boolean;
    onqueuemore?: () => void;
    onreorder?: (changes: { id: string; sequence: number }[]) => void | Promise<void>;
    oncomplete?: (id: string) => void | Promise<void>;
    onopen?: (t: SeqThread) => void;
    onnew?: () => void;
    title?: string;
    what?: string;
    completeLabel?: string;
    modes?: Snippet;
    /** los filtros de sólo lectura van en una píldora abajo al centro, no en la esquina */
    modesBottom?: boolean;
    readonly?: boolean;
    loading?: boolean;
    edgeToEdge?: boolean;
    /** Las entradas de WhatsApp, para la pestaña que las muestra en la barra. */
    entries?: SeqEntry[];
    /** La barra muestra las entradas en vez de lo que falta por secuenciar. */
    entriesOn?: boolean;
    /** Abrir una entrada para leerla. */
    onentry?: (id: string) => void;
    /** Soltar una entrada en el lienzo: quien llama la vuelve tarea y devuelve
     *  el id del hilo nuevo (o nada si se canceló). El lienzo la deja donde
     *  se soltó, igual que algo arrastrado desde la cola. */
    onentrydrop?: (id: string) => Promise<string | null | undefined> | string | null | undefined;
    /** El título se vuelve un menú: elegir qué secuencia se ve (en «Todos los
     *  equipos», el equipo). `title` sigue siendo lo que se lee en la insignia. */
    titleMenu?: { label: string; options: { value: string; label: string }[]; value: string; onselect: (value: string) => void };
  } = $props();

  const W = 230; // ancho de un nodo
  const H = 128; // alto fijo: las flechas y el «+» anclan al borde real
  const key = (k: string) => `bubble.seqcanvas.${api.me?.id ?? 'local'}.${k}.${what}`;
  function readLS<T>(k: string, fallback: T): T {
    try {
      return JSON.parse(localStorage.getItem(key(k)) ?? 'null') ?? fallback;
    } catch {
      return fallback;
    }
  }
  function writeLS(k: string, v: unknown) {
    try {
      localStorage.setItem(key(k), JSON.stringify(v));
    } catch {
      // no poder recordarlo no impide trabajar
    }
  }

  // ---- lo que hay ----
  /** Sólo ids en el navegador; el nombre y los permisos se releen del servidor. */
  // svelte-ignore state_referenced_locally
  let stagedIds = $state<string[]>(readLS('staged-ids', []));
  const staged = $derived(stagedIds.map((id) => threads.find((t) => t.id === id) ?? queue.find((t) => t.id === id))
    .filter((t): t is SeqThread => !!t && !t.done && !t.sequence));
  // svelte-ignore state_referenced_locally
  let pos = $state<Record<string, { x: number; y: number }>>(readLS('pos', {}));
  $effect(() => writeLS('staged-ids', stagedIds));
  $effect(() => writeLS('pos', pos));

  const live = $derived(threads.filter((t) => t.sequence > 0 && !t.done));
  // Lo colocado que, mientras tanto, entró a la secuencia (o se cerró) deja de
  // estar «sin conectar».
  $effect(() => {
    const inLine = new Set(threads.filter((t) => t.sequence > 0 || t.done).map((t) => t.id));
    if (stagedIds.some((id) => inLine.has(id))) stagedIds = stagedIds.filter((id) => !inLine.has(id));
  });

  /** Los pasos, en orden: cada uno, los ids que van en paralelo. */
  const layers = $derived(sequenceLayers(threads));
  const layerOf = $derived(
    new Map(layers.flatMap((l, i) => l.map((t) => [t.id, i] as [string, number]))),
  );

  /** Dónde va un nodo: donde lo dejaron, o donde le toca por su paso. */
  function at(t: SeqThread): { x: number; y: number } {
    if (pos[t.id]) return pos[t.id];
    const li = layerOf.get(t.id);
    if (li === undefined) return { x: 0, y: 0 };
    const row = layers[li];
    const i = row.findIndex((x) => x.id === t.id);
    return { x: (i - (row.length - 1) / 2) * (W + 40), y: li * (H + 90) };
  }
  const nodes = $derived(readonly ? live : [...live, ...staged]);
  const parallel = $derived(parallelFrames(layers, at, W, H));
  const tray = $derived.by(() => {
    const placed = new Set(nodes.map((n) => n.id));
    // La cola llega por páginas y se pidió en algún momento; `threads` es el
    // board de ahora. Lo que el board ya sabe terminado o en la línea no se
    // ofrece como «sin secuenciar», aunque la página que lo trajo sea vieja.
    const settled = new Set(threads.filter((t) => t.done || t.sequence > 0).map((t) => t.id));
    const q = find.trim().toLowerCase();
    return queue.filter(
      (t) => !placed.has(t.id) && !settled.has(t.id) && !t.done &&
        (!q || `${t.name} ${t.project} ${t.bubble}`.toLowerCase().includes(q)),
    );
  });

  /** Las flechas y los huecos entre pasos: ver `sequenceLinks`. */
  const net = $derived(sequenceLinks(layers, at, W, H));
  const bend = linkPath;
  function curve(a: Point, b: Point) {
    return bend({ x: a.x + W / 2, y: a.y + H + 14 }, { x: b.x + W / 2, y: b.y - 6 });
  }

  // ---- escribir la secuencia ----
  /** Renumera de 10 en 10 y manda sólo lo que cambió. */
  let saving = $state(false);
  let saveError = $state('');
  const ROW = H + 90; // de un paso al siguiente, en el acomodo automático
  const ids = () => layers.map((r) => r.map((t) => t.id));

  /** Escribe la línea nueva. `subject` es la tarea que se está colocando: va al
   *  sitio que le toca por su paso. Las demás que alguien acomodó a mano se
   *  corren tantas filas como se corrió su paso, para que insertar o sacar un
   *  paso no deje flechas cruzando el lienzo ni tarjetas una encima de otra.
   *  Devuelve si se guardó. */
  async function commit(rows: string[][], subject?: string): Promise<boolean> {
    if (readonly || saving || !onreorder) return false;
    const changes = sequenceChanges(threads, rows);
    if (!changes.length) return true;
    const next = new Map(rows.filter((r) => r.length).flatMap((r, i) => r.map((id) => [id, i] as [string, number])));
    if (subject) delete pos[subject];
    for (const [id, was] of layerOf) {
      const now = next.get(id);
      if (id === subject || now === undefined || now === was || !pos[id]) continue;
      pos[id] = { x: pos[id].x, y: pos[id].y + (now - was) * ROW };
    }
    saving = true;
    saveError = '';
    try { await onreorder(changes); return true; }
    catch (e) { saveError = (e as Error).message; return false; }
    finally { saving = false; }
  }

  /** A → B: B va en el paso justo después de A (en paralelo con lo que ya
   *  estuviera ahí). Si A no estaba en la línea, entra como paso propio justo
   *  antes de B. */
  function connect(a: string, b: string) {
    if (a === b) return;
    commit(connectSequence(ids(), a, b), b);
  }

  /** `id` como un paso nuevo en el hueco `gap`. */
  function insertAt(id: string, gap: number) {
    if (readonly || saving) return;
    commit(insertStep(ids(), id, gap), id);
  }

  /** Cortar la flecha que llega a una tarea: sale de la secuencia y se queda
   *  suelta en el lienzo, donde estaba, para volver a conectarla bien. Era la
   *  pieza que faltaba: sin ella, una conexión equivocada sólo se arreglaba
   *  sacando la tarea y arrastrándola otra vez desde la caja. */
  //
  // Desde la flecha, la de su punta. Desde el nudo gris de en medio, las que
  // salen de él hacia abajo: una sola en una flecha directa, y a elegir cuando
  // el paso de abajo va en paralelo.
  let cut = $state<{ ids: string[]; x: number; y: number } | null>(null);
  async function cutLink(id: string) {
    cut = null;
    const t = live.find((x) => x.id === id);
    if (readonly || saving || !t) return;
    pos[id] = at(t);
    const ok = await commit(ids().map((r) => r.filter((x) => x !== id)));
    // Después de guardar: mientras la fila siga en la línea, el efecto de
    // arriba la quitaría de «sin conectar» en cuanto entrara.
    if (ok && !stagedIds.includes(id)) stagedIds = [...stagedIds, id];
  }

  /** Volver al acomodo automático: cada paso en su fila. Lo suelto se queda
   *  donde está. */
  function tidy() {
    const keep = new Set(staged.map((s) => s.id));
    pos = Object.fromEntries(Object.entries(pos).filter(([id]) => keep.has(id)));
  }

  /** Sacar de la línea (vuelve a la caja), o quitar del lienzo si no estaba. */
  function remove(id: string) {
    if (readonly || saving) return;
    if (staged.some((s) => s.id === id)) {
      stagedIds = stagedIds.filter((x) => x !== id);
      delete pos[id];
      return;
    }
    delete pos[id];
    commit(ids().map((r) => r.filter((x) => x !== id)));
  }

  /** El hueco bajo el puntero, si lo hay. `elementsFromPoint` y no
   *  `elementFromPoint`: al arrastrar una tarea, lo primero bajo el puntero es
   *  ella misma. */
  let hoverGap = $state(0);
  function gapAt(cx: number, cy: number): number {
    const el = document.elementsFromPoint(cx, cy).find((x) => x instanceof HTMLElement && x.dataset.gap);
    return el ? Number((el as HTMLElement).dataset.gap) : 0;
  }

  // ---- el lienzo: mover, acercar ----
  let canvas = $state<HTMLElement | null>(null);
  // svelte-ignore state_referenced_locally
  let view = $state<{ x: number; y: number; z: number }>(readLS('view', { x: 0, y: 0, z: 1 }));
  $effect(() => writeLS('view', view));
  let centered = readLS('view', null) !== null;
  $effect(() => {
    // La primera vez, el paso 1 arriba y al centro.
    if (centered || !canvas) return;
    centered = true;
    const r = canvas.getBoundingClientRect();
    view = { x: r.width / 2 - W / 2 + 120, y: 112, z: 1 };
  });

  // Una secuencia vacía se abre con su mensaje al centro.
  let emptyCentered = false;
  $effect(() => {
    if (loading || nodes.length || !canvas) { if (nodes.length) emptyCentered = false; return; }
    if (emptyCentered) return;
    emptyCentered = true;
    centerOrigin();
  });

  function toWorld(cx: number, cy: number) {
    const r = canvas!.getBoundingClientRect();
    return { x: (cx - r.left - view.x) / view.z, y: (cy - r.top - view.y) / view.z };
  }

  function wheel(e: WheelEvent) {
    if ((e.target as HTMLElement).closest('.toolbox, .ctrl, .head')) return;
    e.preventDefault();
    if (e.ctrlKey || e.metaKey) {
      // pellizco del trackpad o ⌘+rueda: acercar alrededor del puntero
      const r = canvas!.getBoundingClientRect();
      const px = e.clientX - r.left, py = e.clientY - r.top;
      const z = Math.min(1.8, Math.max(0.35, view.z * Math.exp(-e.deltaY * 0.01)));
      view = { x: px - ((px - view.x) * z) / view.z, y: py - ((py - view.y) * z) / view.z, z };
    } else {
      view = { ...view, x: view.x - e.deltaX, y: view.y - e.deltaY };
    }
  }
  function zoomBy(f: number) {
    const r = canvas!.getBoundingClientRect();
    const px = r.width / 2, py = r.height / 2;
    const z = Math.min(1.8, Math.max(0.35, view.z * f));
    view = { x: px - ((px - view.x) * z) / view.z, y: py - ((py - view.y) * z) / view.z, z };
  }
  // Sin pasos, el mensaje vive en el origen del lienzo (como en Mi espacio):
  // encuadrar es centrarlo.
  function centerOrigin() {
    if (!canvas) return;
    const r = canvas.getBoundingClientRect();
    view = { x: r.width / 2, y: r.height / 2, z: 1 };
  }
  function fit() {
    if (!canvas) return;
    if (!nodes.length) { centerOrigin(); return; }
    const ps = nodes.map(at);
    const x0 = Math.min(...ps.map((p) => p.x)), x1 = Math.max(...ps.map((p) => p.x + W));
    const y0 = Math.min(...ps.map((p) => p.y - 44)), y1 = Math.max(...ps.map((p) => p.y + H + 36));
    const r = canvas.getBoundingClientRect();
    // Ancho de la barra lateral (sidebar 17.5rem / rail 4.25rem) más aire.
    // La barra es una columna aparte, no tapa el lienzo.
    const left = 76;
    const available = r.width - left - 60;
    const z = Math.min(1.4, Math.max(0.35, Math.min(available / (x1 - x0), (r.height - 210) / (y1 - y0))));
    view = { z, x: left + (available - (x1 - x0) * z) / 2 - x0 * z, y: 100 - y0 * z };
  }

  // Un solo gesto a la vez: mover el lienzo, mover un nodo o conectar.
  type Gesture =
    | { kind: 'pan'; sx: number; sy: number; vx: number; vy: number; node?: string; moved: boolean }
    | { kind: 'node'; id: string; sx: number; sy: number; ox: number; oy: number; moved: boolean }
    | { kind: 'link'; from: string };
  let gesture = $state<Gesture | null>(null);
  let pointer = $state<{ x: number; y: number } | null>(null);
  let hoverTarget = $state('');

  function down(e: PointerEvent) {
    if (e.button !== 0) return;
    const el = e.target as HTMLElement;
    if (!el.closest('.cut')) cut = null;
    const hit = el.closest<SVGElement>('[data-cut]');
    if (hit && !readonly) {
      const p = toWorld(e.clientX, e.clientY);
      cut = { ids: [hit.dataset.cut!], x: p.x, y: p.y };
      return;
    }
    const knot = el.closest<HTMLElement>('[data-gap]');
    if (knot && !readonly && !gesture) {
      const g = net.gaps.find((x) => x.at === Number(knot.dataset.gap));
      const ids = layers[Number(knot.dataset.gap)]?.map((t) => t.id) ?? [];
      if (g && ids.length) cut = { ids, x: g.x, y: g.y + 22 };
      return;
    }
    if (el.closest('button, a, input, .toolbox, .ctrl, .head, .read-modes')) return;
    e.preventDefault();
    window.getSelection()?.removeAllRanges();
    const node = el.closest<HTMLElement>('[data-node]');
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    if (node && !readonly) {
      const id = node.dataset.node!;
      const t = nodes.find((n) => n.id === id)!;
      const p = at(t);
      gesture = { kind: 'node', id, sx: e.clientX, sy: e.clientY, ox: p.x, oy: p.y, moved: false };
    } else {
      gesture = { kind: 'pan', sx: e.clientX, sy: e.clientY, vx: view.x, vy: view.y, node: node?.dataset.node, moved: false };
    }
  }
  function startLink(e: PointerEvent, id: string) {
    if (readonly || saving || e.button !== 0) return;
    e.stopPropagation();
    e.preventDefault();
    canvas!.setPointerCapture(e.pointerId);
    gesture = { kind: 'link', from: id };
    pointer = toWorld(e.clientX, e.clientY);
  }
  function move(e: PointerEvent) {
    const g = gesture;
    if (!g) return;
    if (g.kind === 'pan') {
      if (!g.moved && Math.hypot(e.clientX - g.sx, e.clientY - g.sy) < 4) return;
      g.moved = true;
      view = { ...view, x: g.vx + e.clientX - g.sx, y: g.vy + e.clientY - g.sy };
    } else if (g.kind === 'node') {
      const dx = (e.clientX - g.sx) / view.z, dy = (e.clientY - g.sy) / view.z;
      if (!g.moved && Math.hypot(e.clientX - g.sx, e.clientY - g.sy) < 4) return;
      g.moved = true;
      pos[g.id] = { x: Math.round(g.ox + dx), y: Math.round(g.oy + dy) };
      hoverGap = gapAt(e.clientX, e.clientY);
    } else {
      pointer = toWorld(e.clientX, e.clientY);
      const under = document.elementFromPoint(e.clientX, e.clientY)?.closest<HTMLElement>('[data-node]');
      hoverTarget = under && under.dataset.node !== g.from ? under.dataset.node! : '';
      hoverGap = hoverTarget ? 0 : gapAt(e.clientX, e.clientY);
    }
  }
  function up(e: PointerEvent) {
    const g = gesture;
    gesture = null;
    const gap = g && g.kind !== 'pan' ? gapAt(e.clientX, e.clientY) : 0;
    hoverGap = 0;
    if (g?.kind === 'link') {
      const under = document.elementFromPoint(e.clientX, e.clientY)?.closest<HTMLElement>('[data-node]');
      if (under?.dataset.node && under.dataset.node !== g.from) connect(g.from, under.dataset.node);
      else if (gap) insertAt(g.from, gap);
      pointer = null;
      hoverTarget = '';
    } else if (g?.kind === 'node' && g.moved && gap) {
      insertAt(g.id, gap);
    } else if (g && !g.moved && Math.hypot(e.clientX - g.sx, e.clientY - g.sy) < 4) {
      // Capture retargets pointerup to the canvas, not to the card clicked.
      const id = g.kind === 'node' ? g.id : g.node;
      const t = nodes.find((n) => n.id === id);
      if (t) onopen?.(t);
    }
  }

  // ---- de la caja al lienzo ----
  // En un teléfono la barra taparía el lienzo: empieza como rail.
  let toolboxOpen = $state(typeof window === 'undefined' || window.innerWidth > 650);
  let find = $state('');
  let dropping = $state(false);
  function dragStart(e: DragEvent, t: SeqThread) {
    e.dataTransfer?.setData('text/x-seq', t.id);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
  }
  function entryDrag(e: DragEvent, n: SeqEntry) {
    e.dataTransfer?.setData('text/x-entry', n.id);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'copy';
  }
  const entryTray = $derived.by(() => {
    const q = find.trim().toLowerCase();
    return entries.filter((n) => !q || `${n.text} ${n.from}`.toLowerCase().includes(q));
  });
  async function placeEntry(entry: string, at: { x: number; y: number }) {
    const id = await onentrydrop?.(entry);
    if (!id) return;
    pos[id] = { x: Math.round(at.x - W / 2), y: Math.round(at.y - H / 2) };
    if (!live.length && !staged.length) { await commit([[id]]); return; }
    if (!stagedIds.includes(id)) stagedIds = [...stagedIds, id];
  }
  function drop(e: DragEvent) {
    e.preventDefault();
    dropping = false;
    if (readonly || saving) return;
    const entry = e.dataTransfer?.getData('text/x-entry');
    if (entry) { void placeEntry(entry, toWorld(e.clientX, e.clientY)); return; }
    const id = e.dataTransfer?.getData('text/x-seq');
    const t = queue.find((q) => q.id === id);
    if (!t || nodes.some((n) => n.id === id)) return;
    const gap = gapAt(e.clientX, e.clientY);
    hoverGap = 0;
    if (gap) { insertAt(t.id, gap); return; }
    const p = toWorld(e.clientX, e.clientY);
    pos[t.id] = { x: Math.round(p.x - W / 2), y: Math.round(p.y - H / 2) };
    // El primero de una línea vacía no necesita conexión: es el paso 1.
    if (!live.length && !staged.length) {
      commit([[t.id]]);
      return;
    }
    stagedIds = [...stagedIds, t.id];
  }

  function stepTag(t: SeqThread) {
    const i = layerOf.get(t.id);
    if (i === undefined) return 'sin conectar';
    return i === 0 ? '1 · Ahora' : i === 1 ? '2 · Siguiente' : `Paso ${i + 1}`;
  }
</script>

<div class="wrap" class:edge-to-edge={edgeToEdge}>
  <div class="stage">
  {#if !readonly}
    <!-- La barra lateral con la MISMA apariencia que la del panel principal:
         Navigation de Skeleton, columna propia sin fondo (se ve el degradado de
         la app), filas a todo el ancho con icono y texto, pie fijo. Plegada es
         el rail de Skeleton, igual que la principal. -->
    <Navigation layout={toolboxOpen ? 'sidebar' : 'rail'} class="seq-nav toolbox" role="complementary" aria-label="Toolbox de secuencia">
      <Navigation.Content>
        <Navigation.Header>
          <div class="brand" class:railed={!toolboxOpen}>
            <Navigation.Trigger onclick={() => (toolboxOpen = !toolboxOpen)} aria-expanded={toolboxOpen}
              aria-controls="sequence-toolbox-body" aria-label={toolboxOpen ? 'Cerrar toolbox' : 'Abrir toolbox'}
              title={toolboxOpen ? 'Cerrar toolbox' : 'Abrir toolbox'}>
              <Icon of={entriesOn ? 'chat' : what === 'módulos' ? 'bubble' : 'thread'} size="26px" tint />
              {#if toolboxOpen}<span class="display text-base">{entriesOn ? 'WhatsApp' : 'Sin secuenciar'}</span>{/if}
            </Navigation.Trigger>
            <span class="n" title={entriesOn ? 'entradas sin decidir' : 'sin secuenciar'}>{entriesOn ? entries.length : `${tray.length}${queueMore ? '+' : ''}`}</span>
          </div>
        </Navigation.Header>
        {#if toolboxOpen}
          {#if modes}<div class="tb-modes">{@render modes()}</div>{/if}
          <input class="input find" type="search" placeholder="Buscar…" bind:value={find} aria-label="Buscar en la barra"
            autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
          <Navigation.Label>{entriesOn ? 'Entradas de WhatsApp' : `${what === 'módulos' ? 'Módulos' : 'Hilos'} sin secuenciar`}</Navigation.Label>
        {/if}
        <Navigation.Group class="tb-list" id="sequence-toolbox-body">
          <Navigation.Menu>
            {#if entriesOn}
              {#each entryTray as n (n.id)}
                <Navigation.Trigger class="item" draggable="true" data-cursor="grab" ondragstart={(e: DragEvent) => entryDrag(e, n)}
                  onclick={() => onentry?.(n.id)} title={`${n.text} — ${n.from} · ${n.when} · clic para leerla, arrástrala al lienzo para volverla tarea`}>
                  <Icon of="chat" size="26px" tint />
                  <span class="txt">
                    <Navigation.TriggerText class="iname">{n.text}</Navigation.TriggerText>
                    {#if toolboxOpen}<span class="iproj">{n.from} · {n.when}</span>{/if}
                  </span>
                </Navigation.Trigger>
              {:else}
                {#if toolboxOpen}<p class="none">{find ? 'Ninguna entrada coincide.' : 'Nada nuevo por WhatsApp.'}</p>{/if}
              {/each}
            {:else}
              {#each tray as t (t.id)}
                <Navigation.Trigger class="item" draggable="true" data-cursor="grab" ondragstart={(e: DragEvent) => dragStart(e, t)}
                  title={`${t.name}${t.project ? ` — ${t.project}${what !== 'módulos' ? ` · ${t.bubble || 'Sin módulo'}` : ''}` : ''} · arrástralo al lienzo`}>
                  <Icon of={what === 'módulos' ? 'bubble' : 'thread'} size="26px" tint />
                  <span class="txt">
                    <Navigation.TriggerText class="iname">{t.name}</Navigation.TriggerText>
                    {#if toolboxOpen && t.project}<span class="iproj">{t.project}{#if what !== 'módulos'} · {t.bubble || 'Sin módulo'}{/if}</span>{/if}
                  </span>
                </Navigation.Trigger>
              {:else}
                {#if toolboxOpen}<p class="none">Todo está en el lienzo.</p>{/if}
              {/each}
              {#if queueMore && toolboxOpen}<button class="more" onclick={onqueuemore}>cargar más</button>{/if}
            {/if}
          </Navigation.Menu>
        </Navigation.Group>
        {#if toolboxOpen}
          <Navigation.Footer>
            {#if entriesOn}
              <p class="help">Clic para leer una entrada. Arrástrala al canvas para volverla tarea: eliges proyecto y módulo, y queda donde la soltaste.</p>
            {:else}
              <p class="help">Arrastra al canvas. Mantén el <b>+</b> y suelta sobre otro nodo para ponerlo después; desde una tarea suelta, va antes de ése. Dos destinos desde el mismo <b>+</b> van en paralelo. Suelta sobre <b>+ paso</b> para meter un paso en medio. Clic en una flecha para cortarla.</p>
              {#if onnew}<button class="new" onclick={() => { find = ''; onnew?.(); }}>+ nuevo</button>{/if}
            {/if}
          </Navigation.Footer>
        {/if}
      </Navigation.Content>
    </Navigation>
  {/if}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="canvas"
    class:panning={gesture?.kind === 'pan'}
    data-cursor={gesture ? 'grabbing' : 'grab'}
    class:dropping
    bind:this={canvas}
    style:--gx="{view.x}px"
    style:--gy="{view.y}px"
    style:--gs="{24 * view.z}px"
    onpointerdown={down}
    onpointermove={move}
    onpointerup={up}
    onpointercancel={() => ((gesture = null), (pointer = null))}
    onwheel={wheel}
    ondragover={(e) => {
      if (readonly) return;
      e.preventDefault();
      dropping = true;
      hoverGap = (e.target as HTMLElement).closest?.('[data-gap]') ? gapAt(e.clientX, e.clientY) : 0;
    }}
    ondragleave={() => ((dropping = false), (hoverGap = 0))}
    ondrop={drop}>
    <!-- Arriba: los controles en el borde izquierdo y el badge al centro (la
         tercera columna va vacía y sin eventos). -->
    <div class="topbar">
      <div class="ctrl">
        <button onclick={() => zoomBy(1 / 1.2)} title="Alejar">−</button>
        <span>{Math.round(view.z * 100)}%</span>
        <button onclick={() => zoomBy(1.2)} title="Acercar">+</button>
        <button onclick={fit} title="Ver todo">⤢</button>
        {#if !readonly}<button onclick={tidy} title="Reordenar: cada paso en su fila (lo suelto se queda donde está)">≡</button>{/if}
      </div>
      <header class="head">
        {#if titleMenu}
          <Menu positioning={{ placement: 'bottom' }} onSelect={(e: { value: string }) => titleMenu.onselect(e.value)}>
            <Menu.Trigger>
              {#snippet element(attributes: Record<string, unknown>)}
                <button {...attributes} class="title-pick" aria-label={`${titleMenu.label}: ${titleMenu.options.find((o) => o.value === titleMenu.value)?.label ?? ''}`}>
                  <span class="pane-name display"><Glyphs text={title} tint /></span>
                  <ChevronDownIcon class="size-4 chev" aria-hidden="true" />
                </button>
              {/snippet}
            </Menu.Trigger>
            <Portal>
              <Menu.Positioner>
                <Menu.Content class="shell-menu team-menu">
                  {#each titleMenu.options as o (o.value)}
                    <Menu.Item value={o.value}>
                      <span class="face"><Icon of="people" tint /></span>
                      <Menu.ItemText>{o.label}</Menu.ItemText>
                      {#if o.value === titleMenu.value}<CheckIcon class="size-4 check" aria-label="actual" />{/if}
                    </Menu.Item>
                  {/each}
                </Menu.Content>
              </Menu.Positioner>
            </Portal>
          </Menu>
        {:else}
          <span class="pane-name display"><Glyphs text={title} tint /></span>
        {/if}
        <span class="count" title="pasos en la secuencia">{layers.length}</span>
        {#if saving}<Loading compact label="Guardando secuencia" />{/if}
        {#if saveError}<span class="sync" role="alert">{saveError}</span>{/if}
      </header>
    </div>

    <div class="world" style:transform="translate({view.x}px, {view.y}px) scale({view.z})">
      {#each parallel as group (group.step)}
        <div class="parallel-group" class:current={group.step === 0}
          style:left="{group.x}px" style:top="{group.y}px"
          style:width="{group.width}px" style:height="{group.height}px">
          <span class="group-label" id="parallel-step-{group.step}">
            <b>∥ En paralelo</b>
            <span>Paso {group.step + 1} · {group.count} {what}</span>
          </span>
        </div>
      {/each}
      <svg class="edges" aria-hidden="true">
        <defs>
          <marker id="seq-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" />
          </marker>
        </defs>
        {#each net.links as l (l.key)}
          <g class="link" class:cuttable={!!l.cut && !readonly} class:picked={!!l.cut && !!cut?.ids.includes(l.cut)}>
            <path class="line" d={bend(l.from, l.to)} marker-end={l.arrow ? 'url(#seq-arrow)' : undefined} />
            {#if l.cut && !readonly}<path class="hit" d={bend(l.from, l.to)} data-cut={l.cut} />{/if}
          </g>
        {/each}
        {#if gesture?.kind === 'link' && pointer}
          {@const f = nodes.find((n) => n.id === (gesture as { from: string }).from)}
          {#if f}
            <path class="draft" d={curve(at(f), { x: pointer.x - W / 2, y: pointer.y + 6 })} marker-end="url(#seq-arrow)" />
          {/if}
        {/if}
      </svg>
      <!-- Los huecos entre pasos: soltar ahí una tarea (arrastrada, desde su «+»
           o desde la caja) la mete como un paso nuevo en medio. -->
      {#if !readonly}
        {#each net.gaps as g (g.at)}
          <div class="gap" class:armed={dropping || gesture?.kind === 'link' || (gesture?.kind === 'node' && gesture.moved)}
            class:hot={hoverGap === g.at} data-gap={g.at} style:left="{g.x}px" style:top="{g.y}px"
            title="Clic para cortar · suelta aquí una tarea para meterla como un paso nuevo entre éstos">
            <span>+ paso</span>
          </div>
        {/each}
      {/if}
      {#if cut}
        <div class="cut" style:left="{cut.x}px" style:top="{cut.y}px">
          {#each cut.ids as id (id)}
            {@const who = nodes.find((n) => n.id === id)}
            <button onclick={() => cutLink(id)}
              title={`«${who?.name ?? ''}» sale de la secuencia y queda suelta en el lienzo`}>
              ✂ {cut.ids.length > 1 ? `cortar «${who?.name ?? ''}»` : 'cortar'}
            </button>
          {/each}
        </div>
      {/if}

      {#each nodes as t (t.id)}
        {@const p = at(t)}
        {@const li = layerOf.get(t.id)}
        <div
          class="node"
          class:clickable={!!onopen}
          class:now={li === 0}
          class:loose={li === undefined}
          class:parallel-node={li !== undefined && layers[li].length > 1}
          class:target={hoverTarget === t.id}
          class:moving={gesture?.kind === 'node' && gesture.id === t.id}
          data-node={t.id}
          style:left="{p.x}px"
          style:top="{p.y}px"
          style:width="{W}px">
          <div class="top">
            <span class="step">{stepTag(t)}</span>
            {#if t.priority}<span class="prio p-{t.priority.toLowerCase()}">{t.priority}</span>{/if}
            {#if !readonly}<button class="x" title={li === undefined ? 'Quitar del lienzo' : 'Sacar de la secuencia'}
              onclick={() => remove(t.id)}>×</button>{/if}
          </div>
          <button class="name" title="Abrir"
            aria-describedby={li !== undefined && layers[li].length > 1 ? `parallel-step-${li}` : undefined}
            onclick={() => onopen?.(t)}>{t.name}</button>
          {#if t.project || what !== 'módulos'}<span class="where" title={[t.project, what !== 'módulos' ? t.bubble || 'Sin módulo' : ''].filter(Boolean).join(' · ')}>{#if t.workspace && projectColors.get(t.workspace)}<span class="swatch" style:background={projectColors.get(t.workspace)}></span>{/if}<span class="proj">{t.project}</span>{#if what !== 'módulos'}<span class="sep" aria-hidden="true">·</span><span class="mod">{t.bubble || 'Sin módulo'}</span>{/if}</span>{/if}
          <div class="foot">
            <span class="faces">
              {#each t.assignees.slice(0, 3) as a (a.id)}
                {#if a.avatar}<img src={a.avatar} alt={a.name} title={a.name} />{:else}<span title={a.name}>{a.name.slice(0, 1)}</span>{/if}
              {/each}
            </span>
            {#if li !== undefined && !readonly && oncomplete}
              <button class="done" onclick={() => oncomplete?.(t.id)}><Glyphs text={completeLabel} /></button>
            {/if}
          </div>
          <!-- El «+»: mantenerlo y soltar sobre otro nodo dice «ése va después». -->
          {#if !readonly}<button class="plus" disabled={saving} title="Mantén y suelta sobre otro nodo: va después de éste"
            onpointerdown={(e) => startLink(e, t.id)}>+</button>{/if}
        </div>
      {/each}
      {#if !loading && !nodes.length}
        <!-- En el origen del lienzo, no pegado a la pantalla: se mueve con él,
             como en Mi espacio. -->
        <div class="origin">
          <Icon of="compass" size="3.5rem" tint />
          <p>{readonly ? 'Todavía no hay pasos de esta secuencia en tu alcance.' : `Arrastra ${what} desde la caja para armar la secuencia. El primero que sueltes es el paso 1.`}</p>
        </div>
      {/if}
    </div>

    {#if loading}<div class="loading-layer"><Loading /></div>{/if}

    {#if readonly && modes}<div class="read-modes" class:bottom={modesBottom}>{@render modes()}</div>{/if}
  </div>
  </div>
</div>

<style>
  .read-modes { position: absolute; left: 1rem; top: 1rem; z-index: 5; max-width: calc(50% - 7rem); }
  .loading-layer { position: absolute; inset: 0; pointer-events: none; }
  @media (max-width: 1000px) { .read-modes { top: 4.75rem; max-width: min(18rem, calc(100% - 2rem)); } }
  .wrap { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .topbar {
    position: absolute; top: 1rem; left: 1rem; right: 1rem; z-index: 4;
    display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 0.6rem;
    pointer-events: none;
  }
  .topbar > * { pointer-events: auto; }
  .topbar > .ctrl { grid-column: 1; justify-self: start; }
  .topbar > .head { grid-column: 2; max-width: none; }
  /* Los filtros de sólo lectura, abajo al centro, planos en el HUD: cada grupo
     lleva su propio borde, sin otra píldora alrededor. */
  .wrap .read-modes.bottom {
    left: 50%; top: auto; bottom: 1rem; transform: translateX(-50%);
    display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 0.5rem;
    width: max-content; max-width: calc(100% - 2rem); cursor: default;
  }
  /* Sin lugar al centro entre las esquinas de abajo: sube sobre ellas. */
  @media (max-width: 1320px) { .wrap .read-modes.bottom { bottom: 4.75rem; } }
  .head {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 42px;
    max-width: calc(100% - 2rem);
    flex-wrap: wrap;
    justify-content: center;
    padding: 0.5rem 0.9rem;
    border: 1px solid var(--line);
    border-radius: 26px;
    background: var(--surface-solid);
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.16);
    cursor: default;
  }
  .pane-name { font-size: 1.5rem; white-space: nowrap; }
  .title-pick { display: inline-flex; align-items: center; gap: 0.35rem; padding: 0 0.2rem 0 0; border-radius: 12px; }
  .title-pick:hover { color: var(--accent); }
  .title-pick:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  .title-pick :global(.chev) { color: var(--muted); }
  .count { padding: 0 0.45rem; border-radius: 999px; background: var(--hover); font-size: 0.75rem; }
  .sync { font-size: 0.76rem; color: var(--muted); }

  .plus:disabled { opacity: 0.5; cursor: wait; }

  /* El lienzo: puntos que se mueven y escalan con la vista, así se lee
     infinito. */
  .canvas {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    background-color: color-mix(in oklab, var(--surface-solid) 55%, transparent);
    background-image: radial-gradient(circle, color-mix(in oklab, var(--text) 22%, transparent) 1.1px, transparent 1.4px);
    background-size: var(--gs) var(--gs);
    background-position: var(--gx) var(--gy);
    cursor: grab;
    touch-action: none;
    /* Un gesto de trackpad que llega al borde no rebota el pane entero. */
    overscroll-behavior: contain;
    user-select: none;
    -webkit-user-select: none;
  }
  .canvas.panning { cursor: grabbing; }
  .canvas.dropping { box-shadow: inset 0 0 0 2px color-mix(in oklab, var(--accent) 50%, transparent); }
  .world { position: absolute; left: 0; top: 0; transform-origin: 0 0; }
  .parallel-group {
    position: absolute;
    border: 1px dashed color-mix(in oklab, var(--accent) 48%, var(--line));
    border-radius: 20px;
    background: color-mix(in oklab, var(--accent) 5%, transparent);
    pointer-events: none;
  }
  .parallel-group.current { border-color: var(--accent); background: color-mix(in oklab, var(--accent) 8%, transparent); }
  .group-label {
    position: absolute;
    left: 18px;
    top: 8px;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.15rem 0.45rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    font-size: 0.72rem;
    color: var(--muted);
  }
  .group-label b { color: var(--accent); font-weight: 750; }
  .parallel-node:not(.now) { border-color: color-mix(in oklab, var(--accent) 36%, var(--line)); }
  .edges { position: absolute; left: 0; top: 0; width: 1px; height: 1px; overflow: visible; color: var(--muted); pointer-events: none; }
  /* Sólo los trazos: el triángulo del marcador sí se rellena. */
  .edges .link path, .edges path.draft { fill: none; }
  .edges .line { stroke: currentColor; stroke-width: 1.8; opacity: 0.7; }
  .edges .hit { stroke: transparent; stroke-width: 16; pointer-events: stroke; cursor: pointer; }
  .edges .cuttable:hover, .edges .picked { color: var(--accent); }
  .edges .cuttable:hover .line, .edges .picked .line { opacity: 1; stroke-width: 2.4; }
  .edges .picked .line { stroke-dasharray: 6 4; }
  .edges path.draft { stroke: var(--accent); color: var(--accent); stroke-width: 1.8; stroke-dasharray: 6 5; }
  /* El nudo entre dos pasos. Quieto es un punto; mientras se arrastra algo,
     una píldora donde soltarlo. */
  .gap {
    position: absolute; z-index: 2; width: 10px; height: 10px; transform: translate(-50%, -50%);
    display: grid; place-items: center; border-radius: 999px;
    background: color-mix(in oklab, var(--muted) 70%, transparent);
    transition: width 0.12s ease, height 0.12s ease, background-color 0.12s ease;
  }
  /* El punto es pequeño; el blanco del clic, no. */
  .gap::before { content: ''; position: absolute; inset: -9px; border-radius: 999px; }
  .gap:not(.armed) { cursor: pointer; }
  .gap:not(.armed):hover { background: var(--accent); }
  .gap span { display: none; font-size: 0.72rem; font-weight: 750; white-space: nowrap; }
  .gap.armed {
    width: auto; height: 30px; padding: 0 0.7rem; border: 1px dashed var(--accent);
    background: var(--surface-solid); color: var(--accent);
  }
  .gap.armed span { display: inline; }
  .gap.hot { background: var(--accent); color: white; border-style: solid; }
  .cut {
    position: absolute; z-index: 4; transform: translate(-50%, -50%);
    display: flex; flex-direction: column; align-items: center; gap: 0.25rem;
  }
  .cut button {
    max-width: 16rem; overflow: hidden; text-overflow: ellipsis;
    padding: 0.2rem 0.6rem; border: 1px solid var(--accent); border-radius: 999px;
    background: var(--surface-solid); color: var(--accent); font-size: 0.75rem; font-weight: 700;
    white-space: nowrap; box-shadow: 0 4px 12px rgb(0 0 0 / 0.2); cursor: pointer;
  }
  .cut button:hover { background: var(--accent); color: white; }

  .node {
    position: absolute;
    height: 128px;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.55rem 0.7rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--surface-solid);
    box-shadow: 0 8px 22px rgb(0 0 0 / 0.18);
    cursor: grab;
    user-select: none;
    transition: border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.16s ease;
  }
  .node.moving { cursor: grabbing; z-index: 3; box-shadow: 0 14px 34px rgb(0 0 0 / 0.3); }
  .node.now { border-color: color-mix(in oklab, var(--accent) 60%, var(--line)); box-shadow: 0 0 0 3px color-mix(in oklab, var(--accent) 18%, transparent), 0 8px 22px rgb(0 0 0 / 0.18); }
  .node.loose { border-style: dashed; }
  .node.clickable:not(.moving):is(:hover, :focus-within) {
    border-color: var(--accent);
    background: color-mix(in oklab, var(--accent) 5%, var(--surface-solid));
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--accent) 24%, transparent), 0 12px 28px rgb(0 0 0 / 0.24);
  }
  .node.clickable:is(:hover, :focus-within) .name { color: var(--accent); }
  .node.target { border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in oklab, var(--accent) 35%, transparent); }
  .top { display: flex; align-items: center; gap: 0.4rem; }
  .step { font-size: 0.68rem; font-weight: 800; letter-spacing: 0.06em; text-transform: uppercase; color: var(--faint); }
  .node.now .step { color: var(--accent); }
  .prio { padding: 0 0.35rem; border-radius: 5px; font-size: 0.66rem; font-weight: 800; color: white; background: var(--muted); }
  .p-p1 { background: oklch(0.55 0.2 25); }
  .p-p2 { background: oklch(0.6 0.16 55); }
  .p-p3 { background: oklch(0.6 0.12 95); }
  .p-p4 { background: oklch(0.55 0.12 150); }
  .x { margin-left: auto; border: 0; background: none; color: var(--faint); font-size: 1rem; line-height: 1; cursor: pointer; }
  .x:hover { color: var(--text); }
  .name {
    border: 0;
    background: none;
    padding: 0;
    color: var(--text);
    font-size: 0.95rem;
    font-weight: 700;
    text-align: left;
    line-height: 1.25;
    cursor: pointer;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .name:hover { color: var(--accent); }
  .foot { display: flex; align-items: center; gap: 0.4rem; min-height: 1.4rem; margin-top: auto; }
  .where { display: flex; align-items: center; min-width: 0; font-size: 0.72rem; color: var(--muted); white-space: nowrap; overflow: hidden; }
  /* Proyecto y módulo comparten la línea; cualquiera de los dos se corta con «…». */
  .where .proj { flex: 0 1 auto; min-width: 2.5rem; overflow: hidden; text-overflow: ellipsis; }
  .where .mod { flex: 1 1 auto; }
  .where .sep { margin: 0 0.3rem; }
  .mod { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .foot .faces { margin-right: auto; }
  .swatch { flex: none; display: inline-block; width: 0.55rem; height: 0.55rem; border-radius: 999px; margin-right: 0.3rem; vertical-align: middle; }
  .proj { padding: 0 0.45rem; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.7rem; white-space: nowrap; }
  .faces { display: flex; }
  .faces img, .faces span {
    width: 20px; height: 20px; margin-left: -4px; border-radius: 999px; border: 2px solid var(--surface-solid);
    background: var(--hover); font-size: 0.62rem; display: grid; place-items: center; object-fit: cover;
  }
  .done { margin-left: auto; border: 0; background: none; color: var(--muted); font-size: 0.76rem; cursor: pointer; }
  .done:hover { color: var(--text); }
  /* El «+», colgando debajo del nodo. */
  .plus {
    position: absolute;
    left: 50%;
    bottom: -14px;
    width: 26px;
    height: 26px;
    margin-left: -13px;
    display: grid;
    place-items: center;
    border: 1px solid color-mix(in oklab, var(--accent) 55%, var(--line));
    border-radius: 999px;
    background: var(--surface-solid);
    color: var(--accent);
    font-weight: 800;
    cursor: crosshair;
    box-shadow: 0 3px 8px rgb(0 0 0 / 0.2);
    transition: transform 0.12s ease;
  }
  .plus:hover { transform: scale(1.18); background: var(--accent); color: white; }

  .origin { position: absolute; left: 0; top: 0; width: 22rem; transform: translate(-50%, -50%); display: grid; justify-items: center;
    gap: 0.7rem; color: var(--muted); text-align: center; pointer-events: none; }

  .ctrl {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    padding: 0.2rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    font-size: 0.78rem;
    box-shadow: 0 4px 14px rgb(0 0 0 / 0.15);
  }
  .ctrl button { width: 26px; height: 26px; border: 0; border-radius: 999px; background: none; color: var(--text); cursor: pointer; }
  .ctrl button:hover { background: var(--hover); }
  .ctrl span { min-width: 2.8rem; text-align: center; color: var(--muted); }

  .stage { flex: 1; min-height: 0; display: flex; }
  .stage > .canvas { min-width: 0; }
  /* La columna, con las reglas del sidebar principal (Shell.svelte): sin fondo,
     una línea a la derecha, el contenido como columna flexible, la LISTA es lo
     que hace scroll y el pie queda fijo abajo. */
  .wrap :global(.seq-nav) { flex: none; display: flex; flex-direction: column; overflow: hidden; height: auto; border-right: 1px solid var(--line); background: transparent; cursor: default; }
  .wrap :global(.seq-nav [data-part='content'][data-layout='sidebar']) { flex: 1 1 auto; min-height: 0; display: flex; flex-direction: column; align-items: stretch; gap: 0.75rem; }
  .wrap :global(.seq-nav .tb-list) {
    flex: 1 1 auto; min-height: 0; overflow-y: auto; overflow-x: hidden; width: auto;
    margin-inline: -1rem; padding-inline: 1rem 1.5rem;
    scrollbar-width: thin; scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  .wrap :global(.seq-nav[data-layout='rail'] .tb-list) { display: flex; flex-direction: column; gap: 1rem; margin-inline: 0; padding-inline: 0; scrollbar-width: none; }
  .wrap :global(.seq-nav[data-layout='rail'] .tb-list::-webkit-scrollbar) { display: none; }
  /* Skeleton centra el menú del rail en vertical; una lista empieza arriba. */
  .wrap :global(.seq-nav[data-layout='rail'] [data-part='menu']) { flex: none; justify-content: flex-start; }
  .wrap :global(.seq-nav [data-part='footer']) { margin-top: auto; align-self: stretch; width: 100%; padding-top: 0.4rem; border-top: 1px solid var(--line); display: flex; flex-direction: column; gap: 0.4rem; }
  .brand { display: flex; align-items: center; gap: 0.4rem; min-width: 0; }
  .brand.railed { flex-direction: column; gap: 0.2rem; }
  .brand > :global(button:first-child) { flex: 0 1 auto; width: auto; min-width: 0; }
  .n { flex: none; padding: 0.05rem 0.45rem; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.72rem; font-weight: 700; }
  .tb-modes { flex: none; }
  .help { flex: none; margin: 0; color: var(--faint); font-size: 0.72rem; line-height: 1.45; }
  .new { flex: none; padding: 0.45rem; border-radius: 8px; }
  .new:hover { background: var(--hover); }
  @media (max-width: 650px) {
    /* En un teléfono no cabe la fila: cada pieza vuelve a su esquina. */
    .topbar { display: contents; }
    .head { position: absolute; z-index: 4; top: 1rem; right: 1rem; max-width: calc(100% - 2rem); }
    .ctrl { position: absolute; z-index: 4; bottom: 6rem; right: 1rem; }
    .edge-to-edge .head { top: 4rem; }
    .edge-to-edge .read-modes { top: 8rem; }
    .edge-to-edge .ctrl { bottom: 13rem; }
    .wrap .read-modes.bottom { bottom: 4.75rem; }
  }
  @media (prefers-reduced-motion: reduce) { .node, .wrap :global(.toolbox) { transition: none; } }
  .find { font-size: 0.82rem; }
  /* Una fila como las del sidebar principal: el trigger de Skeleton a todo lo
     que queda, icono de 26 px y el nombre en texto base. Nunca hay scroll
     horizontal: el nombre usa dos líneas y se corta aunque no tenga espacios;
     lo demás, una línea con «…». El texto entero queda en el title. */
  .wrap :global(.seq-nav[data-layout='sidebar'] .item) { flex: none; width: 100%; min-width: 0; gap: 0.6rem; text-align: left; white-space: normal; }
  .txt { display: flex; flex-direction: column; min-width: 0; flex: 1; }
  .wrap :global(.seq-nav[data-layout='sidebar'] .iname) {
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; line-height: 1.25;
  }
  .wrap :global(.seq-nav[data-layout='rail'] .txt) { align-items: center; max-width: 100%; }
  .wrap :global(.seq-nav[data-layout='rail'] .iname) { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .iproj { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--muted); font-size: 0.78rem; font-weight: 400; }
  .none { color: var(--faint); font-size: 0.8rem; padding: 0.4rem; }
  .more, .new { border: 0; background: none; color: var(--accent); font-size: 0.8rem; cursor: pointer; }
</style>
