<script lang="ts">
  import Icon from './Icon.svelte';
  // The planner, against the real server.
  //
  // `PlannerView` draws; this owns the data and every write. The mapping is the
  // whole of it, and it is worth stating because the planner's vocabulary and
  // the model's are not the same words:
  //
  //     column  →  a `stage` the department's lead defined
  //     card    →  a BUBBLE, a body of work; its threads are how it is executed
  //     due     →  bubbles.due_date, which the calendar reads
  //     label   →  an objective, what the work is FOR
  //     inbox   →  inbox_items, what is captured and not yet work
  //
  // All of it is the DEPARTMENT's: stages, objectives and inbox are the lead's,
  // and the cards come from every project the viewer can see.
  //
  // There is no card collection, and that is the point: a planner with cards of
  // its own is a second inventory of the work, and two lists of the same work
  // disagree by Thursday.
  import {
    api,
    type BubbleRecord,
    type InboxItem,
    type Objective as ObjectiveRecord,
    type ThreadRecord,
    type ThreadHeat,
    type BubbleHeat,
    type QueueCard,
    type CalendarEvent,
    type Tuning,
    type Artifact,
    avatarUrl,
  } from '../lib/api';
  import PlannerView, { type Objective } from './PlannerView.svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
  import type { Card } from '../lib/cards';
  import type { CalEvent } from './PlannerCalendar.svelte';
  import type { Note } from './InboxSheet.svelte';
  import { priorityMap, priorityMeaning } from '../lib/priority';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import { limited, tooLong } from '../lib/limits.svelte';
  import type { SheetThread } from './CardSheet.svelte';
  import { type SeqThread } from './SequencePane.svelte';
  import SequenceCanvas, { type SeqEntry } from './SequenceCanvas.svelte';
  import { projectColors } from '../lib/projectColors.svelte';
  import { teams } from '../lib/teams.svelte';
  import { ago } from '../lib/when';
  import { untrack, onDestroy } from 'svelte';
  import { live } from '../lib/live.svelte';
  import DueDate from './DueDate.svelte';
  import FilterPick from './FilterPick.svelte';
  import { plannerFilters, NO_FILTERS, lastOpenBubble, type PlannerFilters } from '../lib/filters.svelte';
  import { bandFace, bandName } from '../lib/bands';
  import Crumbs from './Crumbs.svelte';
  import { occurrences, isoDay } from '../lib/repeat';

  let {
    onback,
    onsearch,
    onopenthread,
    onopenboard,
    sequenceEnabled = false,
  }: {
    onback?: () => void;
    onsearch?: () => void;
    /** abrir un hilo a pantalla completa, por su proyecto y su número */
    onopenthread?: (slug: string, seq: number) => void;
    /** ir al board de un proyecto; la burbuja que quede abierta la dice
     *  `lastOpenBubble`, como hace la agenda */
    onopenboard?: (slug: string) => void;
    /** el departamento tiene encendida la secuencia (Ajustes → Funciones) */
    sequenceEnabled?: boolean;
  } = $props();

  // The planner has NO workspace. Not a default one, not the one you came from:
  // this is the department's screen, and a hidden project underneath it is the
  // thing that made "which board am I actually editing" a question.
  //
  // What still needs one is BIRTH. A thread's document is a file in a
  // workspace's git repository, so something new has to be told where it lives —
  // and being asked once, out loud, is the honest version of a screen that used
  // to decide it for you.
  let born = $state<{ title: string; note?: Note } | null>(null);
  let bornIn = $state('');
  // No bubble is asked for here, and that is the difference between the two
  // screens rather than an oversight. The board is the OPERATIVE surface: work
  // on it lives in a bubble because the board draws bubbles. The planner is the
  // strategic one, where the sentence is "this is the work and it belongs to
  // this project" — which bubble carries it is a decision for the board, later,
  // by whoever runs it.

  // Lo que este tablero organiza son BURBUJAS.
  //
  // Un thread es ejecución: lo escribe quien opera, y cómo se organiza por
  // dentro es suyo. Un lead plantea actividades a corto, mediano y largo plazo,
  // y eso es una burbuja — un cuerpo de trabajo con un outcome. Los threads
  // siguen aquí para una sola cosa: sus fechas, que es lo que el calendario
  // muestra. Las fechas son de la ejecución.
  let bubbles = $state<BubbleRecord[]>([]);
  let dates = $state<CalendarEvent[]>([]);
  // Cuánto dura un ciclo AHORA. El tablero dice «en silencio 2 ciclos» y sin
  // esto nadie sabe si eso son dos días o dos semanas: `cycle_hours` se
  // recalibra desde Ajustes y cambia el veredicto de todo sin migración.
  let tuning = $state<Tuning | null>(null);
  const cycleWords = $derived.by(() => {
    const h = tuning?.cycle_hours ?? 0;
    if (!h) return '';
    if (h < 48) return `${Math.round(h)} h`;
    const d = Math.round(h / 24);
    return d % 7 === 0 ? `${d / 7} sem` : `${d} d`;
  });

  /** La secuencia ordena módulos o hilos; se elige dentro de su toolbox.
   *  Por persona y en el servidor (`prefs`): abrir el portátil y encontrarla
   *  en el otro modo es volver a tomar una decisión ya tomada. */
  let mode = $state<'bubbles' | 'tasks'>('bubbles');
  const tasksMode = $derived(mode === 'tasks');
  function setMode(next: 'bubbles' | 'tasks') {
    if (next === mode) return;
    mode = next;
    api.setPrefs({ planner: mode }).catch(() => {});
  }
  api
    .prefs()
    .then((p) => {
      if (p.planner === 'tasks' || p.planner === 'bubbles') mode = p.planner;
    })
    .catch(() => {});

  /** La secuencia es una vista más del selector de abajo, si el departamento
   *  la encendió; cuál se mira lo decide PlannerView. */
  const seqView = $derived(sequenceEnabled);

  let threads = $state<ThreadRecord[]>([]);
  let objectiveRows = $state<ObjectiveRecord[]>([]);
  let notes = $state<InboxItem[]>([]);
  let artifacts = $state<Artifact[]>([]);
  async function loadArtifacts() {
    try { artifacts = (await api.artifacts('inbox')).artifacts; } catch { /* la nota sigue siendo legible */ }
  }
  loadArtifacts();
  const offArtifacts = live.watch('artifacts', loadArtifacts);
  const artifactTick = setInterval(loadArtifacts, 60_000);
  onDestroy(() => { offArtifacts(); clearInterval(artifactTick); });
  // Priority is DERIVED, so it is read like any other server answer rather than
  // recomputed here from the same square the server used.
  let prios = $state<Record<string, string>>({});
  // Workspace id → name, so a card can say which project it is from. On a board
  // that spans the department, two cards called "Facturación" are two different
  // pieces of work.
  let places = $state<Record<string, string>>({});
  let slugs = $state<Record<string, string>>({});
  // Los hilos con su calor, su prioridad y su paso en la secuencia: los del
  // board de TODOS, que el servidor compone en un solo instante. Los registros
  // de `threads` no traen banda, y la cara de hilos de una tarjeta y la
  // secuencia la necesitan.
  let heated = $state<ThreadHeat[]>([]);
  let people = $state<Record<string, { name: string; avatar: string }>>({});
  let heatedBubbles = $state<BubbleHeat[]>([]);
  let bands = $state<Record<string, string>>({});

  // Sin barra de filtros (se quitó): todo a la vista. Lo que alguien dejó
  // filtrado antes se olvida, o escondería cosas sin forma de quitarlo.
  const filters: PlannerFilters = { ...NO_FILTERS };
  plannerFilters.set(filters);

  /** Las burbujas que pasan el filtro. En el cliente, sobre lo que ya se trajo:
   *  `allBubbles()` viene entero de una vez, y pedirle al servidor lo mismo
   *  otra vez por cada tecla sería más lento y no más cierto. */
  const shownBubbles = $derived.by(() => {
    const q = filters.q.trim().toLowerCase();
    return bubbles.filter(
      (b) =>
        (!q || b.name.toLowerCase().includes(q)) &&
        (!filters.project || b.workspace === filters.project) &&
        (!filters.owner || (b.owners ?? []).includes(filters.owner)) &&
        (!filters.objective ||
          (filters.objective === 'none' ? !b.objective : b.objective === filters.objective)) &&
        // La banda llega con el calor y puede tardar un instante más que la
        // fila. Mientras no se sabe, no se esconde: un tablero que parpadea a
        // vacío al cargar es peor que uno que filtra medio segundo tarde.
        (!filters.band || !bands[b.id] || bands[b.id] === filters.band),
    );
  });
  let error = $state('');
  // Nothing is destroyed without being asked first, and the question is asked
  // HERE — the writer knows the name of what is about to go and what goes with
  // it. See `Confirm.svelte`.
  let doom = $state<Doom>(null);

  // El planeador es de UN equipo: el elegido. Sus proyectos, objetivos,
  // calendario, inbox y secuencia; lo de otro contexto no entra.
  async function load() {
    try {
      const T = teams.current;
      const [bu0, ce0, th0, ob0, inb0, pr, ws0] = await Promise.all([
        api.allBubbles(),
        api.calendarEvents(),
        api.allThreads(),
        api.objectives(),
        api.inbox(),
        api.allPriorities(),
        api.workspaces(),
      ]);
      // Sin equipos (una base vieja) se ve todo, como antes.
      const ws = T ? ws0.filter((w) => !w.team || w.team === T) : ws0;
      const inT = new Set(ws.map((w) => w.id));
      const bu = bu0.filter((b) => inT.has(b.workspace));
      const th = th0.filter((t) => inT.has(t.workspace));
      const ce = T ? ce0.filter((e) => !e.team || e.team === T) : ce0;
      const ob = T ? ob0.filter((o) => !o.team || o.team === T) : ob0;
      const inb = T ? inb0.filter((n) => !n.team || n.team === T) : inb0;
      bubbles = bu;
      dates = ce;
      threads = th;
      places = Object.fromEntries(ws.map((w) => [w.id, w.name]));
      slugs = Object.fromEntries(ws.map((w) => [w.id, w.slug]));
      loadHeat();
      objectiveRows = ob;
      notes = inb;
      prios = Object.fromEntries(pr.map((x) => [x.id, x.priority]));
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  // Las preferencias, una vez. Si fallan o no hay, se ve lo de siempre.
  api
    .tuning()
    .then((t) => (tuning = t))
    .catch(() => {});

  async function loadHeat() {
    try {
      const [all, ps] = await Promise.all([api.allBoard(), api.people()]);
      const inT = (ws: string) => !teams.current || ws in places;
      all.threads = all.threads.filter((t) => inT(t.workspace));
      all.bubbles = all.bubbles.filter((b) => inT(b.workspace));
      heated = all.threads;
      // La banda de cada burbuja, que ya venía en la misma llamada y se tiraba.
      // Es lo único que el filtro de banda necesita: `allBubbles()` trae la
      // fila, no el calor, y el calor no se guarda — se calcula.
      heatedBubbles = all.bubbles;
      bands = Object.fromEntries(all.bubbles.map((b) => [b.id, b.heat.lifecycle]));
      people = Object.fromEntries(
        ps.map((p) => [p.id, { name: p.display_name || p.email, avatar: avatarUrl(p, '64x64') }]),
      );
    } catch (e) {
      error = (e as Error).message;
    }
  }

  const bubbleName = (id?: string) => (id ? (bubbles.find((b) => b.id === id)?.name ?? '') : '');

  /** Los hilos de una burbuja, para la otra cara de su tarjeta: como en su
   *  cajón del board, el más reciente primero. */
  function threadsOf(card: Card): SheetThread[] {
    return heated
      .filter((t) => t.bubble === card.id)
      .sort((a, b) => (b.at ?? '').localeCompare(a.at ?? ''))
      .map((t) => ({
        id: t.id,
        seq: t.seq,
        name: t.name,
        lifecycle: t.heat.lifecycle,
        priority: t.priority ?? '',
        owner: (t.assignees ?? []).map((id) => people[id]?.name).filter(Boolean).join(', '),
        age: ago(t.at ?? ''),
      }));
  }

  /** Todos los hilos, para el panel de secuencia. */
  const sequenceThreads = $derived<SeqThread[]>(
    heated.map((t) => ({
      id: t.id,
      seq: t.seq,
      name: t.name,
      project: places[t.workspace] ?? '',
      workspace: t.workspace,
      bubble: bubbleName(t.bubble),
      priority: t.priority ?? '',
      sequence: t.sequence ?? 0,
      done: t.heat.lifecycle === 'closed',
      assignees: (t.assignees ?? []).map((id) => ({
        id,
        name: people[id]?.name ?? '',
        avatar: people[id]?.avatar || undefined,
      })),
    })),
  );

  /** Los módulos, como filas de la línea. Dos líneas independientes: ésta
   *  contesta «¿qué cuerpo de trabajo atacamos antes?» y la de hilos «¿qué
   *  pieza se hace antes?». Un módulo en el paso 2 puede tener hilos en el
   *  paso 5 de la suya, y está bien.
   *
   *  Terminado es CERRADA: cerrar una burbuja ya es una decisión con fecha y
   *  frase. Derivarlo de que no le queden hilos abiertos haría que un módulo
   *  recién creado contara como terminado desde que nace. */
  const sequenceModules = $derived<SeqThread[]>(
    heatedBubbles.map((b) => ({
      id: b.id,
      name: b.name,
      project: places[b.workspace] ?? '',
      workspace: b.workspace,
      bubble: '',
      priority: b.priority ?? '',
      sequence: b.sequence ?? 0,
      done: b.closed,
      assignees: (b.owners ?? []).map((id) => ({
        id,
        name: people[id]?.name ?? '',
        avatar: people[id]?.avatar || undefined,
      })),
    })),
  );

  /** Cerrar un módulo desde la línea. Sin frase: la pide el board, donde hay
   *  sitio para escribirla, y obligar a redactarla aquí para avanzar la línea
   *  sería cobrar un peaje por terminar algo. */
  /** Abrir un módulo desde la línea.
   *
   *  Al board de su proyecto con él abierto, como hace la agenda: la hoja de la
   *  burbuja la dibuja `PlannerView` y abrirla desde fuera obligaría a que este
   *  componente alcanzara su estado interno. La línea dice el ORDEN; lo que ese
   *  cuerpo de trabajo ES se lee donde ya se leía. */
  function openModule(id: string) {
    const b = heatedBubbles.find((x) => x.id === id);
    const slug = b ? slugs[b.workspace] : '';
    if (!b || !slug) return;
    lastOpenBubble.set(b.id);
    onopenboard?.(slug);
  }

  /** Reabrir algo terminado, porque volvió a la línea.
   *
   *  Un módulo pierde su fecha de cierre —y su frase con ella: la frase decía
   *  cómo terminó, y no terminó—. Un hilo vuelve al estado por defecto de su
   *  proyecto, que es de donde salió: no hay un «estado anterior» guardado, y
   *  el por defecto es el que significa «esto está por hacer». */
  const reopenModule = (id: string) =>
    heatWrite(() => api.update('bubbles', id, { closed_at: '', closure: '' }));

  async function reopenThread(id: string) {
    const t = heated.find((x) => x.id === id);
    if (!t) return;
    const states = await api.states(t.workspace);
    const back = states.find((x) => x.is_default) ?? states.find((x) => x.group !== 'completed');
    if (!back) {
      error = 'Ese proyecto no tiene ningún estado abierto al que volver.';
      return;
    }
    await heatWrite(() => api.update('threads', id, { state: back.id }));
  }

  const closeModule = (id: string) =>
    heatWrite(() =>
      api.update('bubbles', id, { closed_at: new Date().toISOString().replace('T', ' ') }),
    );

  // ---- las dos columnas laterales de la línea ----
  //
  // Paginadas en el servidor: lo que está fuera de la línea y lo que ya se
  // terminó crecen sin techo, y `/api/board` compone el departamento entero
  // para poder clasificarlo por calor. Esa respuesta es la correcta para un
  // tablero que se lee de un vistazo, y la equivocada para una lista que se
  // recorre.
  type Side = { items: QueueCard[]; page: number; more: boolean; loading: boolean };
  const emptySide = (): Side => ({ items: [], page: 0, more: true, loading: false });
  let queueOpen = $state<Side>(emptySide());
  let queueDone = $state<Side>(emptySide());

  async function pull(side: Side, state: 'open' | 'done') {
    if (side.loading || !side.more) return;
    side.loading = true;
    try {
      const out = await api.queue({
        kind: tasksMode ? 'threads' : 'bubbles',
        state,
        page: side.page + 1,
        team: teams.current,
      });
      side.items = [...side.items, ...out.items];
      side.page = out.page;
      side.more = out.more;
    } catch (e) {
      // Sin más páginas que pedir: un error aquí no puede dejar el centinela
      // pidiendo la misma página para siempre.
      side.more = false;
      error = (e as Error).message;
    } finally {
      side.loading = false;
    }
  }

  /** Volver a empezar las dos columnas. Al cambiar de modo son otras listas
   *  —hilos o módulos—, y al escribir algo puede haber entrado o salido de
   *  ellas, así que se recargan en vez de parchearse: adivinar aquí en qué
   *  columna cae una fila es reimplementar el filtro del servidor. */
  function refillSides() {
    queueOpen = emptySide();
    queueDone = emptySide();
    if (!seqView) return;
    pull(queueOpen, 'open');
    pull(queueDone, 'done');
  }
  // Otro equipo es otro planeador: se recarga todo.
  let shownTeam = untrack(() => teams.current);
  $effect(() => {
    const t = teams.current;
    if (t === shownTeam) return;
    shownTeam = t;
    untrack(() => { load(); refillSides(); });
  });
  $effect(() => {
    // Depende del modo y de si la línea está a la vista; nada más.
    void mode;
    void seqView;
    // `untrack`, y no es un detalle: `refillSides` escribe `queueOpen` y
    // `queueDone`, y `pull` los LEE para saber si ya está pidiendo y si queda
    // otra página. Sin esto el efecto depende de lo que él mismo escribe y se
    // repite sin fin — se ve como `/api/queue` pedido en bucle hasta que Svelte
    // corta con «demasiada profundidad».
    untrack(() => refillSides());
  });

  /** Una fila de una columna, como la dibuja la línea. */
  const asSeq = (c: QueueCard, done: boolean): SeqThread => ({
    id: c.id,
    seq: c.seq,
    name: c.name,
    project: places[c.workspace] ?? '',
    workspace: c.workspace,
    bubble: c.bubble ? bubbleName(c.bubble) : '',
    priority: c.priority ?? '',
    sequence: 0,
    done,
    created: c.created,
    due: c.due_date,
    assignees: (c.people ?? []).map((id) => ({
      id,
      name: people[id]?.name ?? '',
      avatar: people[id]?.avatar || undefined,
    })),
  });

  async function heatWrite(run: () => Promise<unknown>) {
    try {
      await run();
    } catch (e) {
      error = (e as Error).message;
    }
    await loadHeat();
    refillSides();
  }

  /** Una burbuja soltada en la columna de la secuencia: sus hilos abiertos que
   *  aún no están en la línea entran al final, uno por paso, en su orden (#). */
  function dropBubble(cardId: string) {
    const name = bubbleName(cardId) || 'La burbuja';
    const mine = heated.filter((t) => t.bubble === cardId);
    const last = Math.max(0, ...heated.map((t) => t.sequence ?? 0));
    const fresh = mine
      .filter((t) => t.heat.lifecycle !== 'closed' && !(t.sequence ?? 0))
      .sort((a, b) => a.seq - b.seq);
    // Soltar y que no pase nada parece una caída que no llegó. Se dice por qué.
    if (!fresh.length) {
      const open = mine.filter((t) => t.heat.lifecycle !== 'closed').length;
      error = !mine.length
        ? `«${name}» no tiene hilos: créalos desde su tarjeta (🧵 Hilos) y vuelve a soltarla.`
        : open
          ? `Los hilos abiertos de «${name}» ya están todos en la secuencia.`
          : `«${name}» no tiene hilos abiertos: todos están terminados.`;
      return;
    }
    heatWrite(() =>
      Promise.all(fresh.map((t, i) => api.setSequence(t.id, last + (i + 1) * 10))),
    );
  }

  function openThreadById(id: string) {
    const t = heated.find((x) => x.id === id);
    const slug = t ? slugs[t.workspace] : '';
    if (t && slug) onopenthread?.(slug, t.seq);
  }

  /** The objective's ROW id, from the number the view shows. The view numbers
   *  them 1..n because the number is the priority order; the server does not. */
  const objectiveAt = (n: number) => objectiveRows[n - 1];

  const objectives = $derived<Objective[]>(
    objectiveRows.map((o, i) => ({
      n: i + 1,
      name: o.name,
      why: o.outcome ?? '',
      // Qué parte del trabajo cuelga de él. Contado sobre las BURBUJAS, que es
      // lo que un objetivo agrupa — contar threads decía «catorce» donde lo que
      // hay son tres cosas en marcha. Y contado, no guardado: un número que vive
      // en dos sitios es un número que deriva.
      share: bubbles.length
        ? Math.round((bubbles.filter((b) => b.objective === o.id).length / bubbles.length) * 100)
        : 0,
    })),
  );

  /** Las burbujas en el orden que deriva su prioridad, y por nombre al empatar,
   *  para que dos iguales no bailen entre recargas. */
  function sortBubbles(rows: BubbleRecord[]): BubbleRecord[] {
    return [...rows].sort((a, b) => {
      const pa = prios[a.id] || 'P9';
      const pb = prios[b.id] || 'P9';
      if (pa !== pb) return pa < pb ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  }

  /** Los hilos que pasan el filtro, para el tablero en modo tareas.
   *
   *  Los MISMOS filtros que las burbujas, leídos sobre un hilo: su proyecto es
   *  el suyo, su responsable es quien lo tiene asignado, y su objetivo es el de
   *  su burbuja — un hilo no tiene objetivo propio, a propósito. */
  const shownThreads = $derived.by(() => {
    const q = filters.q.trim().toLowerCase();
    const objOf = (id?: string) => (id ? (bubbles.find((b) => b.id === id)?.objective ?? '') : '');
    return heated.filter(
      (t) =>
        (!q || t.name.toLowerCase().includes(q)) &&
        (!filters.project || t.workspace === filters.project) &&
        (!filters.owner || (t.assignees ?? []).includes(filters.owner)) &&
        (!filters.objective ||
          (filters.objective === 'none' ? !objOf(t.bubble) : objOf(t.bubble) === filters.objective)) &&
        (!filters.band || t.heat.lifecycle === filters.band),
    );
  });

  /** Cómo se ordena una columna de TAREAS.
   *
   *  Derivado, y sin `rank`: primero la prioridad que el lead ya les puso, luego
   *  el paso de la secuencia —las dos cosas que existen justamente para decir
   *  qué va antes— y al final el nombre, para que dos iguales no bailen entre
   *  recargas. Añadir un orden a mano aquí sería un tercer sistema diciendo lo
   *  mismo.  */
  function sortTasks(rows: ThreadHeat[]): ThreadHeat[] {
    return [...rows].sort((a, b) => {
      const pa = a.priority || 'P9';
      const pb = b.priority || 'P9';
      if (pa !== pb) return pa < pb ? -1 : 1;
      const sa = a.sequence || Number.MAX_SAFE_INTEGER;
      const sb = b.sequence || Number.MAX_SAFE_INTEGER;
      if (sa !== sb) return sa - sb;
      return a.name.localeCompare(b.name);
    });
  }

  /** Cuántos threads tiene cada burbuja: lo que una tarjeta dice sin abrirse.
   *
   *  No es el detalle —eso vive en los threads y es de quien opera— sino su
   *  tamaño: «esto son ocho piezas» y «esto es una» se planean distinto. */
  const pieces = $derived.by(() => {
    const n: Record<string, number> = {};
    for (const t of threads) if (t.bubble) n[t.bubble] = (n[t.bubble] ?? 0) + 1;
    return n;
  });

  /** Lo que el planeador puede abrir: las tareas, o los módulos. Ya no hay
   *  columnas; esto es lo que el calendario y la secuencia abren por id. */
  const taskCards = $derived<Card[]>(
    sortTasks(shownThreads).map((t) => ({
      id: t.id,
      title: t.name,
      prio: t.priority || undefined,
      where: places[t.workspace],
      ws: t.workspace,
      owners: t.assignees ?? [],
      due: t.due_date || undefined,
      pieces: 0,
      notes: '',
    })),
  );

  const bubbleCards = $derived<Card[]>(
    sortBubbles(shownBubbles).map((b) => ({
      id: b.id,
      title: b.name,
      obj: b.objective ? objectiveRows.findIndex((o) => o.id === b.objective) + 1 : undefined,
      objName: b.objective ? objectiveRows.find((o) => o.id === b.objective)?.name : undefined,
      impact: b.impact,
      urgency: b.urgency,
      prio: prios[b.id] || undefined,
      // El BRIEF: el cuaderno del plan, no el outcome de una frase.
      notes: b.brief,
      pieces: pieces[b.id] ?? 0,
      where: places[b.workspace],
      ws: b.workspace,
      owners: b.owners ?? [],
      born: b.created,
      due: b.due_date?.slice(0, 10) || undefined,
    })),
  );

  const cards = $derived(tasksMode ? taskCards : bubbleCards);

  /** Lo que entró por WhatsApp y sigue sin decidirse: la pestaña «WhatsApp» de
   *  la barra de Secuencia. Son las MISMAS notas del inbox —quien las ve lo
   *  dicen sus reglas: quien encendió el canal y el lead global—, no una copia. */
  const whatsapp = $derived<SeqEntry[]>(
    notes
      .filter((n) => n.source === 'whatsapp' && !n.thread && !n.bubble)
      .map((n) => ({ id: n.id, text: n.note, from: n.source_from || 'WhatsApp', when: new Date(n.created).toLocaleDateString() })),
  );
  /** Qué muestra la barra de Secuencia: lo que falta por secuenciar, o lo que
   *  llegó por WhatsApp. Sólo de esta pantalla. */
  let seqTray = $state<'queue' | 'whatsapp'>('queue');
  /** Abrir una nota del inbox desde fuera de su vista (la barra de Secuencia). */
  let peek = $state<{ note: Note; at: number } | null>(null);
  function openEntry(id: string) {
    const n = inbox.find((x) => x.id === id);
    if (n) peek = { note: n, at: Date.now() };
  }
  /** Soltar una entrada en el lienzo: el mismo diálogo de «pasar al tablero»
   *  (proyecto › módulo › nombre). Resuelve con el hilo creado, para que el
   *  lienzo lo deje donde se soltó; con nada si se canceló o se paró en módulo. */
  let seqResolve: ((id: string | null) => void) | null = null;
  function entryToTask(id: string): Promise<string | null> {
    const n = inbox.find((x) => x.id === id);
    if (!n) return Promise.resolve(null);
    seqResolve?.(null);
    return new Promise((resolve) => {
      seqResolve = resolve;
      newTask = { note: n, title: n.text, onThread: false, ...blankWhere(), fresh: n.text };
    });
  }
  // Cerrar el diálogo sin crear también contesta: el lienzo no se queda esperando.
  $effect(() => {
    if (!newTask && seqResolve) { const r = seqResolve; seqResolve = null; r(null); }
  });

  const inbox = $derived<Note[]>(
    notes
      // Fuera las triadas, sea a un thread o a una burbuja: ya son de algún
      // sitio, y el inbox es lo que todavía no lo es.
      .filter((n) => !n.thread && !n.bubble)
      .map((n) => ({
        id: n.id,
        text: n.note,
        body: n.body ?? '',
        // Lo que entró por un canal dice quién lo mandó por ahí; lo escrito en
        // la app, quién lo capturó.
        from: n.source_from || (n.captured_by === api.me?.id ? 'tú' : 'alguien'),
        when: new Date(n.created).toLocaleDateString(),
        artifacts: artifacts.filter((a) => a.inbox_item === n.id).map((a) => ({
          id: a.id, title: a.title, expires_at: a.expires_at, pinned: a.pinned,
        })),
      })),
  );

  // El calendario enseña lo PLANEADO: cada burbuja con fecha, el día que tiene
  // que estar. Leía las fechas de los threads, y enseñaba piezas sueltas en vez
  // de los cuerpos de trabajo que el kanban de al lado organiza.
  /** El calendario: los módulos con plazo y las tareas con fecha, juntos.
   *
   *  Juntos y sin conmutador: son la misma pregunta —«¿qué vence?»— y esconder
   *  la mitad detrás de un interruptor que hay que recordar es cómo se llega
   *  tarde a algo que estaba escrito. Se distinguen por el prefijo y por la
   *  prioridad, y si el calendario se llena, lo que lo acota son los filtros
   *  del tablero, que valen para los dos.
   *
   *  Una tarea terminada no aparece: su fecha ya es historia, no plazo. */
  /** La ventana en la que se despliegan las repeticiones.
   *
   *  datesSet informa el rango visible, también al saltar de año. El rango
   *  inicial es sólo un fallback mientras el calendario carga; no limita las
   *  repeticiones a catorce meses ni guarda filas por ocurrencia. */
  let window = $state((() => {
    const now = new Date();
    return {
      from: isoDay(new Date(now.getFullYear(), now.getMonth() - 2, 1)),
      to: isoDay(new Date(now.getFullYear() + 1, now.getMonth() + 2, 0)),
    };
  })());

  const events = $derived<CalEvent[]>([
    // Lo que pasa en una fecha y NO es trabajo. Primero, porque es el marco:
    // una entrega el día de la junta se lee distinto.
    ...dates.flatMap((e) =>
      occurrences(e.start, e.repeat ?? '', window.from, window.to).map((day) => ({
        // Un id por ocurrencia: el calendario necesita que dos días distintos
        // de la misma junta sean dos eventos, o dibuja uno solo.
        id: `cal:${e.id}:${day}`,
        title: `📅 ${e.name}`,
        start: day,
        allDay: true,
      })),
    ),
    ...shownBubbles
      .filter((b) => b.due_date && !b.closed_at)
      .map((b) => ({
        id: b.id,
        title: b.name,
        start: b.due_date!.slice(0, 10),
        allDay: true,
        prio: prios[b.id] || undefined,
        color: projectColors.get(b.workspace) || undefined,
      })),
    ...shownThreads
      .filter((t) => t.due_date && t.heat.lifecycle !== 'closed')
      .map((t) => ({
        id: t.id,
        title: `🧵 ${t.name}`,
        start: t.due_date!.slice(0, 10),
        allDay: true,
        prio: t.priority || undefined,
        color: projectColors.get(t.workspace) || undefined,
      })),
  ]);

  // ---- las fechas del departamento ----
  //
  // Una junta, un cierre, una visita: pasa en una fecha y NO es trabajo. No se
  // completa, no produce evidencia y no calienta nada — por eso es una fila
  // suya y no un hilo con fecha.
  const isLead = $derived(api.me?.role === 'lead');
  let datesOpen = $state(false);
  let dateDraft = $state({ name: '', start: '', repeat: '' as '' | 'daily' | 'weekly' | 'biweekly' | 'monthly' });

  const repeatNames: Record<string, string> = {
    '': 'no se repite',
    daily: 'cada día',
    weekly: 'cada semana',
    biweekly: 'cada dos semanas',
    monthly: 'cada mes',
  };

  function addDate() {
    const name = dateDraft.name.trim();
    if (!name || !dateDraft.start) return;
    const fields = {
      name,
      start: `${dateDraft.start} 00:00:00.000Z`,
      repeat: dateDraft.repeat,
    };
    dateDraft = { name: '', start: '', repeat: '' };
    write(() => api.create('calendar_events', { ...fields, team: teams.current }));
  }

  const patchDate = (id: string, fields: Record<string, unknown>) =>
    write(() => api.update('calendar_events', id, fields));

  function deleteDate(id: string) {
    const e = dates.find((x) => x.id === id);
    doom = {
      title: `¿Borrar «${e?.name ?? id}»?`,
      body: e?.repeat
        ? 'Se va la serie entera, no un día suelto.'
        : 'Sale del calendario de todo el departamento.',
      go: () => write(() => api.remove('calendar_events', id)),
    };
  }

  /** Arrastrar un evento a otro día. El calendario lleva módulos y tareas, así
   *  que hay que preguntar de qué es el id ANTES de escribir: escribir la fecha
   *  de una tarea en la fila de una burbuja no fallaría con un error claro —
   *  guardaría el plazo equivocado en el sitio equivocado. */
  function moveEvent(id: string, day: string) {
    // Una ocurrencia de algo que se repite no se mueve arrastrándola: mover UNA
    // junta de la serie es una excepción, y las excepciones son justo lo que se
    // dejó fuera al elegir un menú cerrado en vez de RRULE. Se cambia la serie
    // entera, o no se cambia.
    if (id.startsWith('cal:')) {
      error = 'Una junta que se repite se cambia entera, no un día suelto.';
      return;
    }
    const stamp = day ? `${day} 00:00:00.000Z` : '';
    if (heated.some((t) => t.id === id)) {
      heatWrite(() => api.update('threads', id, { due_date: stamp }));
      return;
    }
    write(() => api.update('bubbles', id, { due_date: stamp }));
  }

  // Every write reloads rather than patching the local copy. The server decides
  // the seq, the default state and what a rule refuses; guessing all three in
  // the browser is how two views of the same board start disagreeing.
  async function write(run: () => Promise<unknown>) {
    try {
      await run();
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  // Moving a card says what the work is FOR. Not what state it is in: that is
  // the operative board's answer, and it belongs to the workspace that owns it.
  /** Ask where it is born — unless there is only one place it could be. */
  function ask(title: string, note?: Note) {
    const only = Object.keys(places);
    bornIn = only.length === 1 ? only[0] : bornIn || only[0] || '';
    born = { title, note };
    // With a single project there is nothing to choose, so nothing is asked.
    if (only.length === 1) create();
  }

  async function create() {
    const b = born;
    if (!b || !bornIn) return;
    // El título de la nota pasa a ser el NOMBRE de la burbuja, que cabe menos
    // que una nota. Se dice antes de intentarlo, no con un rechazo después.
    const long = tooLong('bubbles.name', b.title);
    if (long) {
      born = null;
      error = `No se creó: el título se vuelve el nombre de la burbuja. ${long}`;
      return;
    }
    born = null;
    await write(async () => {
      // Nace una BURBUJA: lo que se planea es un cuerpo de trabajo. Sus threads
      // los abre quien la ejecute, y ahí es donde se documenta el detalle.
      // Lo que se entendió de la nota viaja a la burbuja como su brief — con sus
      // imágenes, que siguen citando la nota por URL y siguen cargando porque
      // la nota NO se borra.
      const bub = await api.create<BubbleRecord>('bubbles', {
        workspace: bornIn,
        name: b.title,
        brief: b.note?.body ?? '',
      });
      // Una nota triada guarda a qué se convirtió, y sale del inbox por eso —
      // no porque se borre. Antes se borraba, y con ella se iban su cuerpo y
      // sus imágenes: lo único que se había entendido de la captura. Dos
      // escrituras en este orden: una nota que apunta a algo que no llegó a
      // crearse es peor que algo que nadie enlazó.
      if (b.note) await api.update('inbox_items', b.note.id, { bubble: bub.id });
    });
  }

  const deleteCard = (id: string) => {
    if (tasksMode) {
      const t = heated.find((x) => x.id === id);
      doom = {
        title: `¿Borrar la tarea «${t?.name ?? id}»?`,
        body: 'Se va la tarea y su documento del repositorio. Lo escrito sigue en la historia de git, que es de donde se recupera si hacía falta.',
        crumbs: [places[t?.workspace ?? ''] ?? '', bubbleName(t?.bubble)],
        go: () => write(() => api.deleteThread(id)),
      };
      return;
    }
    const b = bubbles.find((x) => x.id === id);
    const n = pieces[id] ?? 0;
    doom = {
      title: `¿Borrar la burbuja «${b?.name ?? id}»?`,
      body: n
        ? `Sus ${n} hilos NO se borran: quedan sin burbuja en el board de su proyecto, que es exactamente lo que pasó.`
        : 'Sale del plan y del board de su proyecto.',
      go: () => write(() => api.remove('bubbles', id)),
    };
  };

  const capture = (text: string) => write(() => api.create('inbox_items', { note: text, team: teams.current }));

  /** Guardar lo escrito en una nota.
   *
   *  Faltaba entero: el panel dejaba editar el título y el texto, los cambiaba
   *  en el objeto de la lista y nadie los mandaba al servidor — así que
   *  recargar los borraba. Es de las peores formas de perder algo, porque
   *  mientras la pantalla está abierta parece guardado. */
  function saveNote(id: string, patch: { text: string; body: string }) {
    const n = notes.find((x) => x.id === id);
    if (!n) return;
    const note = patch.text.trim();
    if (!note || (note === n.note && patch.body === (n.body ?? ''))) return;
    write(() => api.update('inbox_items', id, { note, body: patch.body }));
  }

  const deleteNote = (id: string) => {
    const n = notes.find((x) => x.id === id);
    doom = {
      title: '¿Borrar esta nota?',
      body: n?.note
        ? `«${n.note}» deja de existir: una nota no es trabajo y no queda en ningún otro lado.`
        : 'Una nota no es trabajo y no queda en ningún otro lado.',
      go: () => write(() => api.remove('inbox_items', id)),
    };
  };

  /** Triage: the note becomes a thread, and that is where its project is
   *  decided — which is the question an inbox exists to defer. */
  const promote = (note: Note) => {
    if (tasksMode) {
      newTask = {
        note,
        title: note.text,
        // Lo capturado sirve de nombre para las dos cosas: quien lo triee como
        // módulo no tiene que volver a escribir la misma frase.
        onThread: false,
        ...blankWhere(),
        fresh: note.text,
      };
      return;
    }
    ask(note.text, note);
  };

  /** Dónde empieza la elección. Con un solo proyecto no hay nada que elegir, así
   *  que se salta el primer paso — igual que hace el modo módulos. */
  const blankWhere = () => ({
    ws: Object.keys(places).length === 1 ? Object.keys(places)[0] : '',
    bubble: '',
    fresh: '',
  });

  /** Promover una nota a TAREA: proyecto, y después módulo.
   *
   *  Dos pasos y no dos listas encadenadas en un solo diálogo: con muchos
   *  módulos la segunda lista es larga, y haber elegido mal el proyecto
   *  obligaría a rehacer la elección entera dentro de la misma pantalla.
   *
   *  Una tarea NO nace colgando de nada: un hilo sin burbuja no aparece en el
   *  plan de nadie y es exactamente el trabajo que se pierde. */
  /** Lo que se está creando, y hasta dónde se ha bajado.
   *
   *  UN camino, siempre el mismo: Proyecto › Módulo › Hilo. Lo que nace lo
   *  decide dónde te paras — quien quiere un módulo pulsa «Crear» en el segundo
   *  paso y no llega al tercero. Antes eran dos flujos con un `kind` decidido
   *  antes de empezar, y eso obligaba a saber qué querías crear antes de ver
   *  qué había. */
  let newTask = $state<{
    /** la nota de la que sale, si sale de una */
    note?: Note;
    /** el nombre del HILO, en el tercer paso */
    title: string;
    ws: string;
    bubble: string;
    fresh: string;
    /** ya se bajó al tercer paso */
    onThread: boolean;
  } | null>(null);

  /** En qué paso va el diálogo. Derivado de lo que hay, no un contador: un
   *  número que hay que mantener en paso con tres campos es un número que se
   *  desincroniza. */
  const newStep = $derived<'project' | 'module' | 'thread'>(
    !newTask?.ws ? 'project' : newTask.onThread ? 'thread' : 'module',
  );

  const taskBubbles = $derived(
    newTask?.ws ? bubbles.filter((b) => b.workspace === newTask!.ws && !b.closed_at) : [],
  );

  /** Crear donde se paró.
   *
   *  `stop: 'module'` crea sólo el módulo; `'thread'` crea el módulo si hacía
   *  falta y dentro el hilo. Es una función y no dos porque el módulo nace
   *  igual en los dos casos, y dos copias de «cómo nace un módulo» acabarían
   *  naciendo distinto. */
  async function createTask(stop: 'module' | 'thread') {
    // Si el diálogo vino de soltar una entrada en la secuencia, se le contesta
    // con el hilo creado. Se toma antes de cerrar el diálogo.
    const answer = seqResolve;
    seqResolve = null;
    let madeThread: string | null = null;
    try { await createTaskInner(stop, (id) => (madeThread = id)); }
    finally { answer?.(madeThread); }
  }
  async function createTaskInner(stop: 'module' | 'thread', made: (id: string) => void) {
    const p = newTask;
    if (!p || !p.ws) return;
    const name = (stop === 'module' ? p.fresh : p.title).trim();
    if (!name) return;
    const long = tooLong(stop === 'module' ? 'bubbles.name' : 'threads.name', name);
    if (long) {
      newTask = null;
      error = `No pasó al tablero: el título se vuelve el nombre. ${long}`;
      return;
    }
    newTask = null;
    let created = false;
    await write(async () => {
      // El módulo, si hay que crearlo. Sin outcome: el contrato lo pone quien
      // orquesta, e inventarle uno aquí sería escribirlo por él.
      let bubble = p.bubble;
      if (!bubble && p.fresh.trim()) {
        const made = await api.create<BubbleRecord>('bubbles', {
          workspace: p.ws,
          name: p.fresh.trim(),
        });
        bubble = made.id;
        if (stop === 'module') created = true;
      }
      if (stop === 'module' || !bubble) return;

      const t = await api.createThread({ workspace: p.ws, bubble, name });
      created = true;
      made(t.id);
      if (p.note) {
        // Lo que se entendió de la nota viaja al documento de la tarea. La nota
        // NO se borra: guarda a qué se convirtió, y sale del inbox por eso.
        const body = p.note.body?.trim();
        if (body) {
          const doc = await api.readThread(t.id);
          await api.patchThread(t.id, { base: doc.hash, content: `${body}\n` });
        }
        await api.update('inbox_items', p.note.id, { thread: t.id });
      }
    });
    if (created) setMode(stop === 'thread' ? 'tasks' : 'bubbles');
    await loadHeat();
    // load() relee el board, no la cola paginada. Sin esto lo recién creado
    // existía en el servidor pero no aparecía en la caja hasta cambiar de modo.
    refillSides();
  }


  /** Abrir una tarjeta ya no pide nada: la descripción de una burbuja es su
   *  outcome, y viene con la fila. Antes había que ir a buscar el documento del
   *  thread — que es ejecución, y no es lo que se planea. */
  function openCard(_card: Card) {}

  /** Un campo editado en el panel, dicho en palabras del planeador y traducido
   *  a las de la burbuja.
   *
   *  `prio` no está: se DERIVA de impacto × urgencia en el servidor, y
   *  escribirla aquí sería el único sitio del producto donde las dos se
   *  contradicen a propósito. */
  function patchCard(id: string, fields: Record<string, unknown>) {
    const out: Record<string, unknown> = {};
    if ('title' in fields) out.name = fields.title;
    // El brief de la burbuja: el cuaderno del plan. No el outcome —ése es una
    // frase, el contrato— ni un documento de git, que son de los threads y son
    // la ejecución.
    if ('notes' in fields) out.brief = String(fields.notes ?? '');
    if ('owners' in fields) out.owners = (fields.owners as string[] | undefined) ?? [];
    if ('impact' in fields) out.impact = fields.impact || '';
    if ('urgency' in fields) out.urgency = fields.urgency || '';
    // Un día, o nada: lo que la tarjeta y el calendario dicen es «para el 30».
    if ('due' in fields) out.due_date = fields.due ? `${fields.due} 00:00:00.000Z` : '';
    if ('obj' in fields) {
      const n = fields.obj as number | undefined;
      out.objective = n ? (objectiveAt(n)?.id ?? '') : '';
    }
    // En modo tareas no se abre la hoja de la burbuja —una tarjeta lleva a su
    // hilo—, así que esto no corre. La guarda está por si alguien cablea otra
    // puerta: escribir campos de burbuja con el id de un hilo no fallaría con un
    // error claro, guardaría en la fila equivocada.
    if (tasksMode) return;
    if (Object.keys(out).length) write(() => api.update('bubbles', id, out));
  }

  const deleteObjectiveById = (id: string) => {
    if (!id) return; // "Sin objetivo" is not a row and cannot be deleted
    const o = objectiveRows.find((x) => x.id === id);
    doom = {
      title: `¿Borrar el objetivo «${o?.name ?? id}»?`,
      body: 'Las burbujas que colgaban de él no se borran: quedan sin objetivo, que es exactamente lo que pasó.',
      go: () => write(() => api.remove('objectives', id)),
    };
  };

  const addObjective = (name: string) =>
    write(() =>
      api.create('objectives', {
        name: `${name} ${objectiveRows.length + 1}`,
        position: objectiveRows.length,
        team: teams.current,
      }),
    );

  // The 🎯 dialog numbers its objectives; the kanban knows their ids. Both end
  // in the same place, so both ask the same question.
  const deleteObjective = (n: number) => deleteObjectiveById(objectiveAt(n)?.id ?? '');
</script>

{#if error}
  <!-- Above the view, not behind it. The planner is a fixed full-screen layer,
       so a banner rendered before it was painted underneath — every refusal the
       server made was invisible, and a write that failed looked exactly like a
       write that did nothing. -->
  <p class="failed" role="alert">
    {error}
    <button onclick={() => (error = '')} aria-label="cerrar">×</button>
  </p>
{/if}

{#if datesOpen}
  <Dialog open onOpenChange={() => (datesOpen = false)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-4xl space-y-4 p-6 shadow-xl">
          <Dialog.Title class="text-lg font-bold">Fechas del departamento</Dialog.Title>
          <Dialog.Description class="muted text-sm">
            Lo que pasa en una fecha y no es trabajo: una junta, un cierre, una
            visita. No se completa y no calienta nada — para eso están los hilos.
          </Dialog.Description>

          <ul class="dates">
            {#each dates as e (e.id)}
              <li class="date">
                <input
                  class="input dname"
                  autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                  value={e.name}
                  onchange={(ev) => patchDate(e.id, { name: ev.currentTarget.value })}
                  {@attach limited('calendar_events.name')} />
                <div class="dwhen">
                  <DueDate value={e.start?.slice(0, 10) ?? ''}
                    onchange={(day) => patchDate(e.id, { start: day ? `${day} 00:00:00.000Z` : '' })} />
                </div>
                <div class="drep">
                  <FilterPick portal={false} any={repeatNames['']} label="Repetición"
                    items={Object.entries(repeatNames).filter(([v]) => v).map(([value, label]) => ({ value, label }))}
                    value={e.repeat ?? ''} onchange={(repeat) => patchDate(e.id, { repeat })} />
                </div>
                <button class="x" title="borrar" onclick={() => deleteDate(e.id)}>×</button>
              </li>
            {/each}
            {#if !dates.length}
              <li><p class="muted text-sm">Todavía no hay ninguna.</p></li>
            {/if}
          </ul>

          <form class="date" onsubmit={(e) => { e.preventDefault(); addDate(); }}>
            <input
              class="input dname"
              placeholder="Junta semanal"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={dateDraft.name}
              {@attach limited('calendar_events.name')} />
            <div class="dwhen">
              <DueDate value={dateDraft.start} placeholder="Elegir fecha"
                onchange={(day) => (dateDraft.start = day)} />
            </div>
            <div class="drep">
              <FilterPick portal={false} any={repeatNames['']} label="Repetición"
                items={Object.entries(repeatNames).filter(([v]) => v).map(([value, label]) => ({ value, label }))}
                bind:value={dateDraft.repeat} />
            </div>
            <button
              class="btn btn-sm preset-filled-primary-500"
              disabled={!dateDraft.name.trim() || !dateDraft.start}>Añadir</button>
          </form>

          <div class="flex justify-end">
            <button class="btn btn-sm preset-tonal-surface" onclick={() => (datesOpen = false)}>
              Listo
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

{#if newTask}
  <!-- Dos pasos: proyecto, y después nombre y módulo. Con vuelta atrás, que es
       la mitad del argumento para que sean dos. La misma pantalla sirve para el
       «+» del tablero y para promover una nota: lo único que cambia es si el
       nombre llega escrito. -->
  <Dialog open onOpenChange={() => (newTask = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-sm space-y-4 p-5 shadow-xl">
          <div>
            <!-- El camino entero, siempre visible: dónde estás y cuánto falta.
                 El nivel en el que vas va sin rellenar hasta que lo eliges. -->
            <Crumbs
              parts={[
                places[newTask.ws] || 'Proyecto',
                newStep === 'project'
                  ? null
                  : bubbles.find((b) => b.id === newTask!.bubble)?.name ||
                    newTask.fresh ||
                    'Módulo',
                newStep === 'thread' ? newTask.title || 'Hilo' : null,
              ]} />
            <Dialog.Title class="text-lg font-bold">
              {#if newStep === 'project'}¿En qué proyecto nace?
              {:else if newStep === 'module'}¿En qué módulo?
              {:else}¿Cómo se llama el hilo?{/if}
            </Dialog.Title>
          </div>
          <Dialog.Description class="muted text-sm">
            {#if newStep === 'module'}
              Elige uno o escribe uno nuevo. Si lo que querías era el módulo,
              créalo aquí y ya está: al hilo se baja sólo si hace falta.
            {:else if newStep === 'thread'}
              Un hilo es una pieza ejecutable, y su documento vive en el
              repositorio de su proyecto.
            {:else}
              Todo cuelga de un proyecto: su repositorio es donde acaba escrito.
            {/if}
          </Dialog.Description>

          {#if newStep === 'project'}
            <ul class="places">
              {#each Object.entries(places) as [id, name] (id)}
                <li>
                  <button class="place" onclick={() => (newTask = { ...newTask!, ws: id })}>
                    {name}
                  </button>
                </li>
              {/each}
            </ul>
          {:else if newStep === 'module'}
            <ul class="places">
              {#each taskBubbles as b (b.id)}
                <li>
                  <button
                    class="place"
                    class:on={newTask.bubble === b.id}
                    onclick={() => (newTask = { ...newTask!, bubble: b.id, fresh: '' })}>
                    {b.name}
                  </button>
                </li>
              {/each}
              {#if !taskBubbles.length}
                <li><p class="muted text-sm">Este proyecto no tiene ningún módulo todavía.</p></li>
              {/if}
            </ul>
            <input
              class="input"
              placeholder="…o escribe un módulo nuevo"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              value={newTask.fresh}
              oninput={(e) => (newTask = { ...newTask!, fresh: e.currentTarget.value, bubble: '' })}
              {@attach limited('bubbles.name')} />
          {:else}
            <input
              class="input"
              placeholder="¿Cómo se llama el hilo?"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              value={newTask.title}
              oninput={(e) => (newTask = { ...newTask!, title: e.currentTarget.value })}
              {@attach limited('threads.name')}
              {@attach (el: HTMLInputElement) => { el.focus(); el.select(); }} />
          {/if}

          <div class="flex flex-wrap justify-end gap-2">
            {#if newStep === 'module' && Object.keys(places).length > 1}
              <button
                class="btn btn-sm preset-tonal-surface mr-auto"
                onclick={() => (newTask = { ...newTask!, ws: '', bubble: '', fresh: '' })}>
                ← Proyecto
              </button>
            {:else if newStep === 'thread'}
              <button
                class="btn btn-sm preset-tonal-surface mr-auto"
                onclick={() => (newTask = { ...newTask!, onThread: false })}>
                ← Módulo
              </button>
            {/if}
            <button class="btn btn-sm preset-tonal-surface" onclick={() => (newTask = null)}>
              Cancelar
            </button>

            {#if newStep === 'module'}
              <!-- Parar aquí: lo que nace es el módulo. Sólo con uno NUEVO
                   escrito — «crear» uno que ya existe no crea nada. -->
              <button
                class="btn btn-sm preset-tonal-primary"
                disabled={!newTask.fresh.trim()}
                onclick={() => createTask('module')}>Crear el módulo</button>
              <button
                class="btn btn-sm preset-filled-primary-500"
                disabled={!newTask.bubble && !newTask.fresh.trim()}
                onclick={() =>
                  (newTask = { ...newTask!, onThread: true, title: newTask!.title })}>
                Hilo →
              </button>
            {:else if newStep === 'thread'}
              <button
                class="btn btn-sm preset-filled-primary-500"
                disabled={!newTask.title.trim()}
                onclick={() => createTask('thread')}>Crear el hilo</button>
            {/if}
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

{#if born}
  <!-- The one question the department's screen cannot answer by itself. -->
  <Dialog open onOpenChange={() => (born = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-sm space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">¿En qué proyecto nace?</Dialog.Title>
          <Dialog.Description class="muted text-sm">
            «{born.title}» — su documento vive en el repositorio de ese proyecto.
          </Dialog.Description>
          <ul class="places">
            {#each Object.entries(places) as [id, name] (id)}
              <li>
                <button class="place" class:on={bornIn === id} onclick={() => (bornIn = id)}>
                  {name}
                </button>
              </li>
            {/each}
          </ul>

          <div class="flex justify-end gap-2">
            <button class="btn btn-sm preset-tonal-surface" onclick={() => (born = null)}>Cancelar</button>
            <button
              class="btn btn-sm preset-filled-primary-500"
              onclick={create}
              disabled={!bornIn}>Crear</button>
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

<Confirm bind:ask={doom} />

{#snippet modeTabs()}
  <!-- Qué contiene el toolbox: módulos (burbujas) o hilos. -->
  <div class="mode" role="radiogroup" aria-label="Qué se ordena">
    <button class="seg" class:on={!tasksMode && seqTray === 'queue'} role="radio" aria-checked={!tasksMode && seqTray === 'queue'}
      onclick={() => { seqTray = 'queue'; setMode('bubbles'); }}><Icon of="bubble" tint /> Módulos</button>
    <button class="seg" class:on={tasksMode && seqTray === 'queue'} role="radio" aria-checked={tasksMode && seqTray === 'queue'}
      onclick={() => { seqTray = 'queue'; setMode('tasks'); }}><Icon of="thread" tint /> Hilos</button>
    <!-- Lo que llega por WhatsApp se vuelve TAREA al soltarlo: el lienzo pasa a
         hilos. -->
    <button class="seg" class:on={seqTray === 'whatsapp'} role="radio" aria-checked={seqTray === 'whatsapp'}
      title="Entradas de WhatsApp sin decidir"
      onclick={() => { seqTray = 'whatsapp'; setMode('tasks'); }}><Icon of="chat" tint /> WhatsApp{#if whatsapp.length}<span class="seg-n">{whatsapp.length}</span>{/if}</button>
  </div>
{/snippet}

{#snippet seqPane()}
      <!-- El canvas conserva los mismos pasos del servidor; sólo las
           posiciones y la vista pertenecen a este navegador. -->
      <div class="seq-pane">
        {#key mode}
        <SequenceCanvas
          title="🧭 Secuencia"
          modes={modeTabs}
          what={tasksMode ? 'tareas' : 'módulos'}
          completeLabel={tasksMode ? '✓ Terminar' : '🏆 Cerrar'}
          threads={tasksMode ? sequenceThreads : sequenceModules}
          queue={queueOpen.items.map((c) => asSeq(c, false))}
          queueMore={queueOpen.more}
          onqueuemore={() => pull(queueOpen, 'open')}
          onreorder={(changes) =>
            heatWrite(() =>
              Promise.all(
                changes.map((c) =>
                  tasksMode
                    ? api.setSequence(c.id, c.sequence)
                    : api.update('bubbles', c.id, { sequence: c.sequence }),
                ),
              ),
            )}
          oncomplete={(id) => (tasksMode ? heatWrite(() => api.completeThread(id)) : closeModule(id))}
          onopen={(t) => (tasksMode ? openThreadById(t.id) : openModule(t.id))}
          onnew={() => (newTask = { title: '', onThread: false, ...blankWhere() })}
          entries={whatsapp}
          entriesOn={seqTray === 'whatsapp'}
          onentry={openEntry}
          onentrydrop={entryToTask} />
        {/key}
      </div>
{/snippet}

<PlannerView
  {cards}
  {inbox}
  {peek}
  {events}
  {objectives}
  priorities={priorityMeaning.map((p) => [...p] as [string, string, string, string])}
  {priorityMap}
  render={(md, card) => api.renderMarkdown(md, card?.ws ?? '')}
  onattachcard={(card, file) =>
    // Al `assets/` del workspace de la burbuja, como una imagen de thread: la
    // burbuja tiene workspace, así que su imagen tiene dónde vivir y se cita por
    // la ruta corta, la que sobrevive a una mudanza.
    card.ws ? api.uploadAsset(card.ws, file) : Promise.resolve(null)}
  onattachnote={(id, file) => api.attachToNote(id, file)}
  choosePriority={false}
  {onback}
  {onsearch}
  onopencard={openCard}
  onpatchcard={patchCard}
  loadPeople={(card) => (card.ws ? api.roster(card.ws) : Promise.resolve([]))}
  {threadsOf}
  canPrioritize={api.me?.role === 'lead'}
  onthreadpriority={(id, p) => heatWrite(() => api.setThreadPriority(id, p))}
  onnewthread={(card, name) =>
    card.ws
      ? heatWrite(() => api.createThread({ workspace: card.ws!, bubble: card.id, name }))
      : undefined}
  onopenthread={openThreadById}
  onmoveevent={moveEvent}
  oncalrange={(from, to) => { if (window.from !== from || window.to !== to) window = { from, to }; }}
  ondeletecard={deleteCard}
  oncapture={capture}
  onpromote={promote}
  onsavenote={saveNote}
  ondeletenote={deleteNote}
  onpatchobjective={(n, fields) => {
    const row = objectiveAt(n);
    if (row) write(() => api.update('objectives', row.id, fields));
  }}
  onaddobjective={addObjective}
  ondeleteobjective={deleteObjective}
  cardsAreTasks={tasksMode}
  onopendates={isLead ? () => (datesOpen = true) : undefined}
  boardPane={seqView ? seqPane : undefined}>
</PlannerView>

<style>
  .dates { display: flex; flex-direction: column; gap: 1rem; margin: 0; padding: 0; list-style: none; }
  .date { display: grid; grid-template-columns: minmax(12rem, 1fr) 12rem 13rem auto; align-items: center; gap: 0.8rem; }
  .date .dname, .date .dwhen, .date .drep { min-width: 0; }
  .date :global([data-scope='combobox'][data-part='root']) { width: 100%; }
  @media (max-width: 800px) {
    .date { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto; }
    .date .dname { grid-column: 1 / -1; }
  }
  .date .x {
    flex: none;
    padding: 0 0.4rem;
    border-radius: 7px;
    color: var(--faint);
  }
  .date .x:hover { background: var(--hover); color: var(--text); }

  /* Módulos / Hilos se ajusta a sus botones, no al ancho del toolbox. */
  .mode {
    display: inline-flex;
    width: max-content;
    max-width: 100%;
    gap: 2px;
    margin-left: 0;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
  }
  .seg {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.15rem 0.7rem;
    border-radius: 999px;
    color: var(--muted);
    font-size: 0.8rem;
    font-weight: 700;
  }
  .seg:hover { color: var(--text); }
  .seg.on { background: color-mix(in oklab, var(--accent) 22%, var(--surface-solid)); color: var(--text); }
  /* Dentro de la barra de Secuencia, las tres pestañas se reparten el ancho. */
  :global(.seq-nav) .mode { display: flex; width: 100%; }
  :global(.seq-nav) .seg { flex: 1 1 auto; min-width: 0; justify-content: center; padding: 0.2rem 0.4rem; white-space: nowrap; }
  /* Tres pestañas con icono no caben en la barra; la cabecera ya lleva el
     icono de lo que se ve. */
  :global(.seq-nav) .seg > :global(:not(.seg-n)) { display: none; }
  .seg-n { margin-left: 0.25rem; padding: 0 0.4rem; border-radius: 999px; background: var(--accent); color: white; font-size: 0.68rem; font-weight: 800; }

  /* La línea ocupa el panel entero, con su propio scroll dentro. */
  .seq-pane { flex: 1; min-height: 0; }

  .places {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    /* Room before the verbs. The list ended where the buttons began, which put
       "Crear" a pixel under the last project — close enough to read as part of
       the list rather than as what happens to it. */
    margin: 0 0 0.75rem;
    padding: 0;
    list-style: none;
  }
  .place {
    width: 100%;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: transparent;
    color: var(--muted);
    text-align: left;
    font-size: 0.9rem;
  }
  .place:hover { background: var(--hover); color: var(--text); }
  .place.on { border-color: var(--accent, var(--line)); color: var(--text); font-weight: 600; }

  .failed {
    position: fixed;
    top: 1rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: var(--z-toast);
    display: flex;
    align-items: center;
    gap: 0.75rem;
    max-width: min(92vw, 560px);
    margin: 0;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--color-error-500);
    border-radius: 12px;
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.85rem;
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.22);
  }
  .failed button { color: var(--faint); font-size: 1rem; line-height: 1; }
</style>
