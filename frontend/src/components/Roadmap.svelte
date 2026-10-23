<script lang="ts">
  // Roadmap: el calendario como un camino.
  //
  // Un camino que serpentea sin fin: hacia arriba el pasado, hacia abajo el
  // futuro. Cada CARRIL va de un lado al otro —los pares de izquierda a
  // derecha, los impares de vuelta— y se une al siguiente con una curva.
  //
  // Qué llena la pantalla se elige (y se recuerda por persona):
  //   · Semana — cada carril es un día, 7 en pantalla; las casillas del día van
  //     una tras otra a lo largo del carril.
  //   · Mes    — cada carril es una semana, ~4,3 en pantalla; cada día tiene su
  //     lugar en el carril.
  //   · Año    — cada carril es un trimestre, 4 en pantalla; cada semana tiene
  //     su lugar, con marcas de mes.
  // La escala es FIJA: un carril vacío mide lo mismo que uno lleno, así que la
  // distancia es tiempo. Siempre caben entre 4 y 7 carriles: holgados.
  //
  // Casillas: los módulos y los hilos con fecha, con el icono de su banda; y las
  // fechas del calendario del equipo como señales (📅), sin pieza (en «Año» no
  // van las que se repiten cada día o cada semana: taparían el trabajo). Las
  // piezas son las personas: cada una está parada en lo PRIMERO que le toca —su
  // casilla abierta más antigua—; si esa fecha ya pasó, queda antes de hoy y
  // lleva el aro de «atrasado». Sin nada abierto, espera en hoy.
  //
  // De quién es una casilla: la misma regla que «lo mío» en la secuencia —un
  // hilo de sus asignados, o si no tiene, de los responsables de su módulo; un
  // módulo, de sus responsables—.
  //
  // Sólo lectura: un clic abre. Lo que se ve es lo que las reglas dejan ver (el
  // tablero de TODOS), del equipo elegido.
  import { onDestroy, tick, untrack } from 'svelte';
  import { fly } from 'svelte/transition';
  import { api, avatarUrl, type Board, type CalendarEvent, type Lifecycle, type Person } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { lastOpenBubble } from '../lib/filters.svelte';
  import { bandFace, bandName } from '../lib/bands';
  import { occurrences, isoDay, fromDay } from '../lib/repeat';
  import { teams } from '../lib/teams.svelte';
  import { weekStart } from '../lib/weekStart.svelte';
  import { dayInWeek, startOfWeek, weekNumber } from '../lib/week';
  import Icon from './Icon.svelte';
  import Loading from './Loading.svelte';
  import Glyphs from './Glyphs.svelte';

  // `onTeams`: en «Todos los equipos», los equipos con el roadmap encendido
  // (lib/features.ts); los proyectos de los demás no entran.
  let { onopen, onopenboard, onTeams }: { onopen?: (slug: string, seq: number) => void; onopenboard?: (slug: string) => void; onTeams?: string[] } = $props();

  // ---- datos -------------------------------------------------------------------
  let board = $state<Board | null>(null);
  let people = $state<Person[]>([]);
  let dates = $state<CalendarEvent[]>([]);
  let error = $state('');
  let stopped = false;
  let generation = 0;
  async function load() {
    const turn = ++generation;
    try {
      // En «Todos los equipos» lo personal ni siquiera viaja: lo recorta el servidor.
      const [b, ps, ce] = await Promise.all([api.allBoard(teams.viewing ? 'shared' : ''), api.people(), api.calendarEvents()]);
      if (stopped || turn !== generation) return;
      board = b;
      people = ps;
      dates = ce;
      error = '';
    } catch (e) {
      if (!stopped && turn === generation) error = (e as Error).message;
    }
  }
  $effect(() => {
    teams.viewing;
    untrack(load);
  });
  let timer: ReturnType<typeof setTimeout> | undefined;
  const offs = ['threads', 'bubbles', 'calendar_events', 'memberships', 'workspaces'].map((c) =>
    live.watch(c, () => { clearTimeout(timer); timer = setTimeout(load, 200); }),
  );
  onDestroy(() => { stopped = true; generation++; clearTimeout(timer); offs.forEach((off) => off()); });

  // ---- qué se ve ---------------------------------------------------------------
  let mode = $state<'all' | 'bubbles' | 'threads'>('all');

  type Zoom = 'week' | 'month' | 'year';
  const ZOOMS: Record<Zoom, { label: string; unit: 'day' | 'week' | 'quarter'; perScreen: number }> = {
    week: { label: 'Semana', unit: 'day', perScreen: 7 },
    month: { label: 'Mes', unit: 'week', perScreen: 30.44 / 7 },
    year: { label: 'Año', unit: 'quarter', perScreen: 4 },
  };
  const zoomKey = `bubble.roadmap.zoom.${api.me?.id ?? 'local'}`;
  function readZoom(): Zoom {
    try {
      const z = localStorage.getItem(zoomKey);
      return z === 'week' || z === 'month' || z === 'year' ? z : 'month';
    } catch {
      return 'month';
    }
  }
  let zoom = $state<Zoom>(readZoom());
  const unit = $derived(ZOOMS[zoom].unit);
  const perScreen = $derived(ZOOMS[zoom].perScreen);

  type Tile = {
    key: string;
    kind: 'bubble' | 'thread' | 'event';
    id: string;
    name: string;
    day: string;
    life?: Lifecycle;
    open: boolean;
    project?: string;
    slug?: string;
    seq?: number;
    owners: string[];
  };

  // El equipo elegido; los proyectos de otro equipo a los que te invitaron
  // entran siempre (como en la columna). En «Todos los equipos», los de los
  // compartidos.
  const shownWs = $derived.by(() => {
    const T = teams.current;
    return new Map((board?.workspaces ?? [])
      .filter((w) => !T || teams.inScope(w))
      .filter((w) => !onTeams || !teams.viewing || !w.team || onTeams.includes(w.team))
      .map((w) => [w.id, w]));
  });

  const work = $derived.by<Tile[]>(() => {
    if (!board) return [];
    const ws = shownWs;
    const owners = new Map(board.bubbles.map((b) => [b.id, b.owners ?? []]));
    const out: Tile[] = [];
    if (mode !== 'threads') {
      for (const b of board.bubbles) {
        const w = ws.get(b.workspace);
        if (!w || !b.due_date) continue;
        const life: Lifecycle = b.closed ? 'closed' : b.heat.lifecycle;
        out.push({ key: `b:${b.id}`, kind: 'bubble', id: b.id, name: b.name, day: b.due_date.slice(0, 10),
          life, open: life !== 'closed', project: w.name, slug: w.slug, owners: b.owners ?? [] });
      }
    }
    if (mode !== 'bubbles') {
      for (const t of board.threads) {
        const w = ws.get(t.workspace);
        if (!w || !t.due_date) continue;
        const life = t.heat.lifecycle;
        const who = t.assignees?.length ? t.assignees : (t.bubble ? owners.get(t.bubble) ?? [] : []);
        out.push({ key: `t:${t.id}`, kind: 'thread', id: t.id, name: t.name, day: t.due_date.slice(0, 10),
          life, open: life !== 'closed', project: w.name, slug: w.slug, seq: t.seq, owners: who });
      }
    }
    return out;
  });

  // ---- el tiempo, en carriles ----------------------------------------------------
  const DAY = 86_400_000;
  const todayDay = isoDay(new Date());
  const today = fromDay(todayDay);
  const addDays = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
  const daysBetween = (a: Date, b: Date) => Math.round((b.getTime() - a.getTime()) / DAY);
  // La semana empieza el día que eligió quien mira (Ajustes → Mi usuario).
  weekStart.load().catch(() => {});
  const first = $derived(weekStart.day);
  const weekOf = (d: Date) => startOfWeek(d, first);
  const quarterOf = (d: Date) => new Date(d.getFullYear(), Math.floor(d.getMonth() / 3) * 3, 1);
  const week0 = $derived(weekOf(today));
  const q0 = quarterOf(today);

  /** dónde empieza el carril `i` (0 = el que contiene hoy) */
  function laneStart(i: number): Date {
    if (unit === 'day') return addDays(today, i);
    if (unit === 'week') return addDays(week0, 7 * i);
    return new Date(q0.getFullYear(), q0.getMonth() + 3 * i, 1);
  }
  /** en qué carril cae un día */
  function laneIndex(day: string): number {
    const d = fromDay(day);
    if (unit === 'day') return daysBetween(today, d);
    if (unit === 'week') return Math.round(daysBetween(week0, weekOf(d)) / 7);
    return d.getFullYear() * 4 + Math.floor(d.getMonth() / 3) - (q0.getFullYear() * 4 + Math.floor(q0.getMonth() / 3));
  }
  /** en qué lugar del carril cae un día (en «Semana» lo decide el orden) */
  function slotIndex(day: string, lane: number): number {
    const d = fromDay(day);
    if (unit === 'week') return dayInWeek(d, first);
    if (unit === 'quarter') return Math.min(13, Math.floor(daysBetween(laneStart(lane), d) / 7));
    return 0;
  }
  const FIXED_SLOTS = { week: 7, quarter: 14 } as const;

  // La ventana de carriles que se dibuja; crece al acercarse a un borde.
  let lo = $state(-15);
  let hi = $state(25);
  const step = $derived(Math.ceil(perScreen * 2));
  function resetWindow() {
    lo = -Math.ceil(perScreen * 3);
    hi = Math.ceil(perScreen * 5);
  }
  untrack(resetWindow);

  const events = $derived.by<Tile[]>(() => {
    const T = teams.current;
    const from = isoDay(laneStart(lo));
    const to = isoDay(addDays(laneStart(hi + 1), -1));
    return dates
      .filter((e) => !T || !e.team || teams.inView(e.team))
      .filter((e) => !onTeams || !teams.viewing || !e.team || onTeams.includes(e.team))
      // En «Año» lo que se repite cada día o cada semana taparía el trabajo.
      .filter((e) => unit !== 'quarter' || !['daily', 'weekly', 'biweekly'].includes(e.repeat ?? ''))
      .flatMap((e) => occurrences(e.start, e.repeat ?? '', from, to).map((day) => ({
        key: `e:${e.id}:${day}`, kind: 'event' as const, id: e.id, name: e.name, day, open: false, owners: [],
      })));
  });

  // ---- geometría -----------------------------------------------------------------
  let width = $state(0);
  let viewH = $state(0);
  const narrow = $derived(width > 0 && width < 760);
  /** cuánto mide un carril: la pantalla entre los carriles que caben */
  const fit = $derived(Math.max(110, Math.round((viewH || 800) / perScreen)));
  // Cada carril mide un poco más que su parte de la pantalla: el aire entre un
  // tramo y el siguiente. El grueso sale de la medida justa (`fit`), así que todo
  // lo que se agrega es aire.
  const AIR = 1.3;
  const laneH = $derived(Math.round(fit * AIR));
  /** el grueso del camino */
  const RW = $derived(Math.max(30, Math.min(narrow ? 40 : 64, Math.round(fit * 0.32))));
  /** cuánto sale la vuelta hacia afuera: casi media circunferencia del carril */
  const TURN = $derived(narrow ? Math.max(30, Math.min(52, laneH * 0.28)) : Math.max(44, Math.min(110, laneH * 0.4)));
  /** el punto de control de la cúbica que da esa vuelta (una cúbica sale ¾ de él) */
  const CURVE = $derived((TURN * 4) / 3);
  // El margen guarda la vuelta, medio camino y un aire, para que la curva (y su
  // sombra, y la lupa) no toquen el borde en pantallas angostas.
  const M = $derived(Math.round(TURN + RW / 2 + (narrow ? 12 : 28)));
  const left = $derived(M);
  const right = $derived(Math.max(M + 140, width - M));
  const span = $derived(right - left);
  /** cuántas casillas caben en un carril de «Semana» */
  const daySlots = $derived(Math.max(1, Math.floor(span / 150)));

  /** carril → lugares → casillas (el trabajo primero, las señales al final) */
  const grid = $derived.by(() => {
    const byLane = new Map<number, Tile[]>();
    for (const t of [...work, ...events]) {
      const i = laneIndex(t.day);
      if (i < lo || i > hi) continue;
      if (!byLane.has(i)) byLane.set(i, []);
      byLane.get(i)!.push(t);
    }
    const g = new Map<number, Tile[][]>();
    for (const [i, ts] of byLane) {
      ts.sort((a, b) => a.day.localeCompare(b.day) || Number(a.kind === 'event') - Number(b.kind === 'event') || a.name.localeCompare(b.name));
      const n = unit === 'day' ? Math.min(daySlots, ts.length) : FIXED_SLOTS[unit];
      const slots: Tile[][] = Array.from({ length: n }, () => []);
      ts.forEach((t, j) => slots[unit === 'day' ? j % n : slotIndex(t.day, i)].push(t));
      g.set(i, slots);
    }
    return g;
  });
  const slotsIn = (i: number) => (unit === 'day' ? grid.get(i)?.length ?? 1 : FIXED_SLOTS[unit]);
  const slotW = (i: number) => span / slotsIn(i);
  // Debajo de 120px por lugar el nombre se corta a pocas letras y no dice nada:
  // mejor la ficha redonda (el nombre queda en el título, y la lupa la agranda).
  const compact = $derived(width > 0 && (width < 760 || span / (unit === 'day' ? daySlots : FIXED_SLOTS[unit]) < 120));
  const TILE_H = $derived(compact ? 46 : 54);
  const maxStack = $derived(Math.max(1, Math.floor((laneH - 64) / TILE_H)));

  type Lane = { i: number; y: number; mid: number; forward: boolean };
  const lanes = $derived.by<Lane[]>(() => {
    const out: Lane[] = [];
    for (let i = lo; i <= hi; i++) {
      const y = 24 + (i - lo) * laneH;
      out.push({ i, y, mid: y + laneH / 2, forward: ((i % 2) + 2) % 2 === 0 });
    }
    return out;
  });
  const total = $derived(48 + (hi - lo + 1) * laneH);
  const laneOf = (i: number) => lanes[i - lo];
  const xOf = (lane: Lane, slot: number, n = slotsIn(lane.i)) => {
    const off = (slot + 0.5) * (span / n);
    return lane.forward ? left + off : right - off;
  };

  /** El camino: un carril por tramo y una curva hacia el siguiente. */
  const road = $derived.by(() => {
    if (!lanes.length || !width) return '';
    const parts: string[] = [];
    lanes.forEach((ln, k) => {
      const a = ln.forward ? left : right;
      const b = ln.forward ? right : left;
      parts.push(k === 0 ? `M ${a} ${ln.mid}` : `L ${a} ${ln.mid}`);
      parts.push(`L ${b} ${ln.mid}`);
      const next = lanes[k + 1];
      if (next) {
        const bulge = CURVE * (ln.forward ? 1 : -1);
        parts.push(`C ${b + bulge} ${ln.mid} ${b + bulge} ${next.mid} ${b} ${next.mid}`);
      }
    });
    return parts.join(' ');
  });

  type Placed = Tile & { x: number; y: number; mid: number; lane: number; group: string; more?: number };
  const placed = $derived.by<Placed[]>(() => {
    const out: Placed[] = [];
    if (!width) return out;
    for (const [i, slots] of grid) {
      const ln = laneOf(i);
      if (!ln) continue;
      slots.forEach((day, s) => {
        const shown = day.slice(0, maxStack);
        shown.forEach((t, k) => {
          const y = ln.mid + (k - (shown.length - 1) / 2) * TILE_H;
          out.push({ ...t, x: xOf(ln, s, slots.length), y, mid: ln.mid, lane: i, group: `${i}:${s}`, more: k === shown.length - 1 && day.length > maxStack ? day.length - maxStack : undefined });
        });
      });
    }
    return out;
  });
  const placedByKey = $derived(new Map(placed.map((p) => [p.key, p])));

  /** dónde está hoy en el camino: en «Semana», cerca del inicio de su carril (pasando su etiqueta) */
  const now = $derived.by(() => {
    const ln = laneOf(0);
    if (!ln || !width) return null;
    const x = unit === 'day' ? (ln.forward ? left + 130 : right - 130) : xOf(ln, slotIndex(todayDay, 0));
    return { x, y: ln.mid };
  });

  // ---- la lupa ------------------------------------------------------------------
  // Donde está el cursor el camino se ensancha y lo que va encima crece, en
  // degradado: en el centro, al máximo; en el borde de la lupa, nada. Las
  // casillas del carril se abren como el Dock —cada hueco crece lo mismo que
  // las casillas, así no se enciman— sin salirse del carril. Es sólo vista: la
  // fecha de nada cambia. Con ratón; en pantallas táctiles no hay cursor.
  const fall = (u: number) => (u >= 1 ? 0 : (1 - u * u) ** 2);
  /** ∫₀ᵘ fall: cuánto se ha abierto el carril hasta `u` */
  const spread = (u: number) => (u >= 1 ? 8 / 15 : u - (2 * u ** 3) / 3 + u ** 5 / 5);
  const GROW = 0.45; // cuánto crecen las casillas bajo el cursor
  const SWELL = 0.45; // cuánto se ensancha el camino (ya es grueso: más cerraría el hueco de la vuelta)
  const LR = $derived(Math.max(170, Math.min(300, span * 0.22))); // radio a lo largo
  const LV = $derived(laneH * 0.7); // radio a lo alto: un carril, no el de al lado
  let lens = $state({ x: 0, y: 0, k: 0 });
  const aim = { x: 0, y: 0, k: 0, cx: 0, cy: 0, track: false };
  let raf = 0;
  function frame() {
    const nx = lens.x + (aim.x - lens.x) * 0.25;
    const ny = lens.y + (aim.y - lens.y) * 0.25;
    const nk = lens.k + (aim.k - lens.k) * 0.18;
    if (Math.abs(aim.x - nx) < 0.3 && Math.abs(aim.y - ny) < 0.3 && Math.abs(aim.k - nk) < 0.004) {
      lens = { x: aim.x, y: aim.y, k: aim.k };
      raf = 0;
      return;
    }
    lens = { x: nx, y: ny, k: nk };
    raf = requestAnimationFrame(frame);
  }
  // La lupa se enciende sólo SOBRE el camino: a poca distancia de su centro, o
  // encima de algo que va en él (una casilla, una señal, una pieza). En los
  // márgenes o entre carriles, no. Una vez encendida se apaga un poco más lejos
  // de donde se encendió, para que no parpadee en el borde.
  let onRoad = false;
  function nearRoad(x: number, y: number) {
    // Con el carril como tope: se apaga siempre antes de la mitad entre dos
    // carriles, por grueso que sea el camino.
    // Desde el BORDE del camino (medio grueso más un aire), no desde su centro:
    // si no, un camino grueso encendería la lupa lejos de él.
    const on = Math.min(RW / 2 + 28, laneH * 0.38);
    const reach = onRoad ? Math.min(on + 12, laneH * 0.45) : on;
    for (const p of samples) {
      if (Math.abs(p.y - y) > reach || Math.abs(p.x - x) > reach) continue;
      if (Math.hypot(p.x - x, p.y - y) <= reach) return true;
    }
    return false;
  }
  function aimAt(clientX: number, clientY: number, target: Element | null = document.elementFromPoint(clientX, clientY)) {
    aim.cx = clientX;
    aim.cy = clientY;
    if (!world) return;
    const r = world.getBoundingClientRect();
    const x = clientX - r.left, y = clientY - r.top;
    // Sobre la tarjeta de una pila la lupa sigue encendida pero quieta.
    if (target?.closest('.stack-card')) { onRoad = true; aim.k = 1; if (!raf) raf = requestAnimationFrame(frame); return; }
    onRoad = !zooming && (!!target?.closest('.tile, .sign, .piece') || nearRoad(x, y));
    aim.k = onRoad ? 1 : 0;
    if (onRoad) {
      aim.x = x;
      aim.y = y;
      // Al entrar, la lupa aparece donde está el cursor; no viaja desde donde se fue.
      if (lens.k < 0.01) lens = { x, y, k: lens.k };
    }
    if (!raf) raf = requestAnimationFrame(frame);
  }
  function onmove(e: PointerEvent) {
    if (e.pointerType !== 'mouse') return;
    aim.track = true;
    aimAt(e.clientX, e.clientY, e.target as Element | null);
  }
  function onleave() {
    aim.k = 0;
    aim.track = false;
    onRoad = false;
    if (!raf) raf = requestAnimationFrame(frame);
  }
  onDestroy(() => cancelAnimationFrame(raf));

  /** dónde se ve algo de un carril con la lupa encima, y a qué escala */
  function lensed(x: number, y: number, mid: number) {
    const k = lens.k;
    const v = k < 0.004 ? 0 : fall(Math.abs(mid - lens.y) / LV) * k;
    if (v < 0.004) return { x, y, s: 1 };
    const t = x - lens.x;
    const u = Math.abs(t) / LR;
    const s = 1 + GROW * v * fall(u);
    // Se abre hacia los lados, y cede antes de llegar al final del carril.
    const push = GROW * v * LR * spread(u);
    const room = Math.max(0, t >= 0 ? right - 14 - x : x - left - 14);
    const moved = room > 0 ? (push * room) / (room + push) : 0;
    return { x: x + Math.sign(t) * moved, y: mid + (y - mid) * s, s };
  }

  /** el centro del camino, muestreado: para ensancharlo bajo el cursor */
  type Sample = { x: number; y: number; nx: number; ny: number; curve?: number };
  const samples = $derived.by<Sample[]>(() => {
    const out: Sample[] = [];
    if (!lanes.length || !width) return out;
    const STEP = 6;
    lanes.forEach((ln, k) => {
      const a = ln.forward ? left : right;
      const b = ln.forward ? right : left;
      // La normal sale siempre de la dirección del camino (−dy, dx), en recta y
      // en curva: si una apuntara al revés que la otra, la figura se torcería
      // en la unión y dejaría un hueco.
      const ny = b >= a ? 1 : -1;
      const n = Math.max(1, Math.ceil(Math.abs(b - a) / STEP));
      for (let j = 0; j < n; j++) out.push({ x: a + ((b - a) * j) / n, y: ln.mid, nx: 0, ny });
      const next = lanes[k + 1];
      if (!next) { out.push({ x: b, y: ln.mid, nx: 0, ny }); return; }
      // La curva: la misma cúbica que dibuja `road`.
      const c = b + CURVE * (ln.forward ? 1 : -1);
      const P = [[b, ln.mid], [c, ln.mid], [c, next.mid], [b, next.mid]];
      const N = 28;
      for (let j = 0; j < N; j++) {
        const t = j / N, m = 1 - t;
        const x = m ** 3 * P[0][0] + 3 * m * m * t * P[1][0] + 3 * m * t * t * P[2][0] + t ** 3 * P[3][0];
        const y = m ** 3 * P[0][1] + 3 * m * m * t * P[1][1] + 3 * m * t * t * P[2][1] + t ** 3 * P[3][1];
        let dx = 3 * m * m * (P[1][0] - P[0][0]) + 6 * m * t * (P[2][0] - P[1][0]) + 3 * t * t * (P[3][0] - P[2][0]);
        let dy = 3 * m * m * (P[1][1] - P[0][1]) + 6 * m * t * (P[2][1] - P[1][1]) + 3 * t * t * (P[3][1] - P[2][1]);
        const len = Math.hypot(dx, dy) || 1;
        dx /= len; dy /= len;
        out.push({ x, y, nx: -dy, ny: dx, curve: k + 1 });
      }
    });
    return out;
  });

  /** cuánto ensancha la lupa el camino en un punto: 0 lejos, 1 bajo el cursor */
  const swellAt = (x: number, y: number) => fall(Math.hypot((x - lens.x) / LR, (y - lens.y) / LV)) * lens.k;

  /** el tramo ensanchado: una figura por capa (sombra, costado, cara, brillo) */
  const swell = $derived.by(() => {
    const none = { shadow: '', side: '', body: '', shine: '', box: null as null | { x: number; y: number; w: number; h: number } };
    if (lens.k < 0.01 || !samples.length) return none;
    type Pt = Sample & { w: number };
    const g = samples.map((p) => swellAt(p.x, p.y));
    const on = g.map((v) => v > 0.002);
    // Un tramo nunca termina dentro de una curva: si la toca, la toma entera y
    // un punto de recta a cada lado. En la recta el corte es vertical y las
    // capas corridas hacia arriba o abajo empalman exactas; en la curva el
    // brillo y el costado asomarían (o faltarían) más allá del corte.
    const touched = new Set(samples.filter((p, i) => p.curve && on[i]).map((p) => p.curve));
    samples.forEach((p, i) => {
      if (!p.curve || !touched.has(p.curve)) return;
      on[i] = true;
      if (i > 0) on[i - 1] = true;
      if (i + 1 < samples.length) on[i + 1] = true;
    });
    const runs: Pt[][] = [];
    let run: Pt[] | null = null;
    samples.forEach((p, i) => {
      if (on[i]) (run ??= []).push({ ...p, w: RW * (1 + SWELL * g[i]) });
      else if (run) { runs.push(run); run = null; }
    });
    if (run) runs.push(run);
    // Las mismas medidas que el trazo de abajo, redondeadas igual, y crecidas
    // en proporción: donde la lupa no ensancha, coinciden al píxel.
    const dS = Math.round(RW * 0.28), dH = Math.round(RW * 0.45), shW = Math.max(2, Math.round(RW * 0.14));
    const pt = (p: Pt, off: number, drop: number) => `${(p.x + p.nx * off).toFixed(1)} ${(p.y + p.ny * off + drop).toFixed(1)}`;
    const long = runs.filter((r) => r.length > 1);
    /** un listón de ancho variable, corrido hacia abajo */
    const band = (half: (q: number) => number, drop: (q: number) => number) =>
      long.map((r) => {
        const a = r.map((p) => pt(p, half(p.w / RW), drop(p.w / RW)));
        const b = r.map((p) => pt(p, -half(p.w / RW), drop(p.w / RW))).reverse();
        return `M ${a.join(' L ')} L ${b.join(' L ')} Z`;
      }).join(' ');
    // La sombra: sólo lo que la lupa agrega a cada lado (entre la sombra de
    // siempre y la crecida). Repetir la sombra entera la oscurecería al doble.
    const ring = long.flatMap((r) => [1, -1].map((side) => {
      const out = r.map((p) => pt(p, side * (p.w + 6) / 2, (dH * p.w) / RW));
      const inn = r.map((p) => pt(p, side * (RW + 6) / 2, dH)).reverse();
      return `M ${out.join(' L ')} L ${inn.join(' L ')} Z`;
    })).join(' ');
    // La caja de la sombra, con margen en píxeles para el desenfoque: un margen
    // en porcentaje de una franja tan delgada lo recortaría.
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (const r of long) for (const p of r) {
      x0 = Math.min(x0, p.x); x1 = Math.max(x1, p.x);
      y0 = Math.min(y0, p.y); y1 = Math.max(y1, p.y);
    }
    const pad = RW * (1 + SWELL) + 40;
    return {
      box: long.length ? { x: x0 - pad, y: y0 - pad, w: x1 - x0 + 2 * pad, h: y1 - y0 + 2 * pad } : null,
      shadow: ring,
      side: band((q) => (q * RW) / 2, (q) => q * dS),
      body: band((q) => (q * RW) / 2, () => 0),
      shine: band((q) => (q * shW) / 2, (q) => -q * dS),
    };
  });

  // ---- piezas --------------------------------------------------------------------
  type Piece = { id: string; name: string; avatar: string; x: number; y: number; mid: number; late: number; on?: string; day?: string; group?: string };
  const pieces = $derived.by<Piece[]>(() => {
    if (!width) return [];
    const mine = new Map<string, Tile[]>();
    for (const t of work) for (const p of t.owners) {
      if (!mine.has(p)) mine.set(p, []);
      mine.get(p)!.push(t);
    }
    const out: Piece[] = [];
    for (const [pid, ts] of mine) {
      const person = people.find((p) => p.id === pid);
      if (!person) continue;
      const next = ts.filter((t) => t.open).sort((a, b) => a.day.localeCompare(b.day))[0];
      // Sin nada abierto espera en hoy; si hoy no está a la vista (se saltó
      // lejos con el minimapa), esa pieza no se dibuja.
      if (!next && !now) continue;
      let x = now?.x ?? 0, y = now?.y ?? 0, mid = now?.y ?? 0, late = 0;
      let group: string | undefined = next ? undefined : 'now';
      if (next) {
        const spot = placedByKey.get(next.key);
        if (spot) { x = spot.x; y = spot.y; mid = spot.mid; group = spot.group; }
        else {
          // Fuera de la ventana o debajo de un «+N»: en su lugar, sobre el camino.
          const i = laneIndex(next.day);
          const ln = laneOf(Math.min(hi, Math.max(lo, i)));
          if (ln) { x = i < lo ? (ln.forward ? left : right) : i > hi ? (ln.forward ? right : left) : xOf(ln, unit === 'day' ? 0 : slotIndex(next.day, i)); y = mid = ln.mid; }
        }
        late = Math.max(0, daysBetween(fromDay(next.day), today));
      }
      out.push({ id: pid, name: person.display_name || person.email, avatar: avatarUrl(person, '64x64'), x, y, mid, late, on: next?.name, day: next?.day, group });
    }
    // Varias en el mismo sitio: en abanico, sin taparse.
    const at = new Map<string, Piece[]>();
    for (const p of out) {
      const k = `${Math.round(p.x)}:${Math.round(p.y)}`;
      if (!at.has(k)) at.set(k, []);
      at.get(k)!.push(p);
    }
    for (const group of at.values()) {
      group.sort((a, b) => a.name.localeCompare(b.name));
      group.forEach((p, i) => { p.x += (i - (group.length - 1) / 2) * 24; });
    }
    return out;
  });

  // ---- pilas --------------------------------------------------------------------
  // Donde caben varias cosas en un lugar —una pieza sobre su casilla, casillas
  // apiladas, las que se esconden detrás de «+N»— el hover abre la pila: una
  // tarjeta con TODO lo que hay ahí, cada trabajo con su enlace. Mientras está
  // abierta la lupa se queda quieta, para que nada se mueva debajo.
  type Stack = { key: string; x: number; y: number; mid: number; tiles: Tile[]; people: Piece[] };
  const stacks = $derived.by(() => {
    const out = new Map<string, Stack>();
    for (const t of placed) {
      if (out.has(t.group)) continue;
      const [i, sl] = t.group.split(':').map(Number);
      const all = grid.get(i)?.[sl] ?? [t];
      const top = placed.filter((q) => q.group === t.group).reduce((a, b) => (b.y < a.y ? b : a));
      out.set(t.group, { key: t.group, x: t.x, y: top.y, mid: t.mid, tiles: all, people: [] });
    }
    for (const p of pieces) {
      if (!p.group) continue;
      if (!out.has(p.group)) out.set(p.group, { key: p.group, x: p.x, y: p.y, mid: p.mid, tiles: [], people: [] });
      out.get(p.group)!.people.push(p);
    }
    return out;
  });
  /** abre tarjeta si en su lugar hay más de una cosa, o si las casillas son
      fichas redondas (Año, pantallas angostas): ahí no se lee nada sin ella */
  const crowded = (key?: string) => {
    const st = key ? stacks.get(key) : undefined;
    return !!st && (st.tiles.length + st.people.length > 1 || compact);
  };
  let stackKey = $state<string | null>(null);
  let stackTimer: ReturnType<typeof setTimeout> | undefined;
  function stackEnter(key?: string) {
    clearTimeout(stackTimer);
    if (!crowded(key)) return;
    stackTimer = setTimeout(() => (stackKey = key!), stackKey ? 0 : 140);
  }
  function stackLeave() {
    clearTimeout(stackTimer);
    stackTimer = setTimeout(() => (stackKey = null), 180);
  }
  onDestroy(() => clearTimeout(stackTimer));
  const stack = $derived(stackKey ? stacks.get(stackKey) ?? null : null);
  /** la tarjeta: arriba de la pila, o abajo si arriba no hay lugar en la vista */
  const stackAt = $derived.by(() => {
    if (!stack || !scroller) return null;
    const L = lensed(stack.x, stack.y, stack.mid);
    const below = L.y - scroller.scrollTop < 300;
    const half = Math.min(150, span / 2);
    // Arriba, por encima también de la pieza si hay una parada ahí.
    const up = (stack.people.length && stack.tiles.length ? 72 : 34) * L.s;
    return { x: Math.max(left + half - M / 2, Math.min(right - half + M / 2, L.x)), y: below ? L.y + 36 * L.s : L.y - up, below };
  });
  function stackRange(st: Stack) {
    const days = [...st.tiles.map((t) => t.day), ...st.people.filter((p) => !st.tiles.length && p.day).map((p) => p.day!)].sort();
    if (!days.length) return 'Hoy';
    return days[0] === days[days.length - 1] ? fmtDay(days[0]) : `${fmtDay(days[0])} – ${fmtDay(days[days.length - 1])}`;
  }

  // ---- desplazamiento sin fin ----------------------------------------------------
  let scroller = $state<HTMLElement | null>(null);
  /** qué altura del mundo está en el centro de la vista */
  let seen = $state(0);
  /** cuánto se ha bajado, y hacia dónde fue el último paso (1 abajo, -1 arriba) */
  let scrollY = $state(0);
  let scrollDir = $state(1);
  let centered = $state(false);
  let growing = false;
  async function goToday(smooth = true) {
    if (!scroller) return;
    // Tras un salto lejano hoy puede no estar en la ventana: se vuelve a armar.
    if (!laneOf(0)) { resetWindow(); await tick(); smooth = false; }
    if (!now) return;
    scroller.scrollTo({ top: Math.max(0, now.y - scroller.clientHeight / 2), behavior: smooth ? 'smooth' : 'auto' });
  }
  $effect(() => {
    if (!centered && board && width && viewH && now && scroller) {
      centered = true;
      tick().then(() => goToday(false));
    }
  });
  // Cambiar la escala es un zoom: hacia «Semana» se acerca, hacia «Año» se
  // aleja. Dos tiempos, los dos con origen en hoy: la vista vieja se va (se
  // agranda y se desvanece al acercarse; se encoge al alejarse) y la nueva llega
  // del lado contrario. Sin movimiento si la persona lo pidió así.
  const DEPTH: Record<Zoom, number> = { week: 0, month: 1, year: 2 };
  let world = $state<HTMLElement | null>(null);
  let zooming = false;
  async function setZoom(z: Zoom) {
    if (z === zoom || zooming) return;
    const closer = DEPTH[z] < DEPTH[zoom];
    const steps = Math.abs(DEPTH[z] - DEPTH[zoom]);
    const k = closer ? 1 + 0.45 * steps : 1 / (1 + 0.45 * steps);
    const still = !world || !scroller || !now || matchMedia('(prefers-reduced-motion: reduce)').matches;
    zooming = true;
    aim.k = 0;
    lens = { ...lens, k: 0 };
    try {
      if (!still) {
        world!.style.transformOrigin = `${now!.x}px ${scroller!.scrollTop + scroller!.clientHeight / 2}px`;
        await world!.animate(
          [{ transform: 'scale(1)', opacity: 1 }, { transform: `scale(${k})`, opacity: 0 }],
          { duration: 170, easing: 'cubic-bezier(.4,0,1,1)', fill: 'forwards' },
        ).finished.catch(() => {});
      }
      zoom = z;
      try { localStorage.setItem(zoomKey, z); } catch { /* sin memoria, sólo esta vez */ }
      resetWindow();
      await tick();
      goToday(false);
      if (!still && world && now) {
        world.getAnimations().forEach((a) => a.cancel());
        world.style.transformOrigin = `${now.x}px ${now.y}px`;
        await world.animate(
          [{ transform: `scale(${1 / k})`, opacity: 0 }, { transform: 'scale(1)', opacity: 1 }],
          { duration: 300, easing: 'cubic-bezier(.2,.8,.2,1)' },
        ).finished.catch(() => {});
      }
    } finally {
      world?.getAnimations().forEach((a) => a.cancel());
      zooming = false;
    }
  }
  async function onscroll() {
    if (scroller) {
      seen = scroller.scrollTop + scroller.clientHeight / 2;
      scrollDir = scroller.scrollTop >= scrollY ? 1 : -1;
      scrollY = scroller.scrollTop;
    }
    // El cursor no se movió pero el camino sí: ¿sigue encima?
    if (aim.track) aimAt(aim.cx, aim.cy);
    if (!scroller || growing || zooming || !centered) return;
    const { scrollTop, clientHeight, scrollHeight } = scroller;
    if (scrollTop < laneH * 2) {
      growing = true;
      const before = total;
      lo -= step;
      // Lo de arriba empuja todo `step` carriles: `scrollY` se corre ya, en el
      // mismo cambio, para que el badge no lea por un cuadro una altura vieja
      // sobre carriles nuevos (saltaba a un mes de antes y volvía).
      scrollY += step * laneH;
      await tick();
      // Lo que se agregó arriba empuja todo hacia abajo: se compensa para que
      // la vista no salte.
      scroller.scrollTop += total - before;
      growing = false;
    } else if (scrollTop + clientHeight > scrollHeight - laneH * 2) {
      growing = true;
      hi += step;
      await tick();
      growing = false;
    }
  }

  // ---- texto ---------------------------------------------------------------------
  const fmtDay = (day: string) => fromDay(day).toLocaleDateString('es', { day: 'numeric', month: 'short' });
  const cap = (s: string) => `${s[0].toUpperCase()}${s.slice(1)}`;
  const monthLabel = (d: Date) => `${cap(d.toLocaleDateString('es', { month: 'long' }))} ${d.getFullYear()}`;
  const short = (d: Date) => d.toLocaleDateString('es', { month: 'short' }).replace('.', '');
  /** el número de semana (ISO con el lunes; si no, la 1 es la del 1 de enero) */
  const isoWeek = (d: Date) => weekNumber(d, first);
  const weekRange = (a: Date) => {
    const b = addDays(a, 6);
    return a.getMonth() === b.getMonth() ? `${a.getDate()} – ${b.getDate()} ${short(b)}` : `${a.getDate()} ${short(a)} – ${b.getDate()} ${short(b)}`;
  };
  type LaneLabel = { head: string; week?: { n: number; range: string }; name: string };
  /** la etiqueta de cada carril, y los títulos por nivel: el mes con su año (o el
   *  año, en «Año») y, en «Semana», la semana cuando empieza */
  function laneLabel(ln: Lane): LaneLabel {
    const s = laneStart(ln.i);
    const e = addDays(laneStart(ln.i + 1), -1);
    if (unit === 'day') {
      return {
        head: s.getDate() === 1 || ln.i === lo ? monthLabel(s) : '',
        week: s.getDay() === first || ln.i === lo ? { n: isoWeek(s), range: weekRange(weekOf(s)) } : undefined,
        // el día, entre caminos: en «Semana» cada carril es uno
        name: `${s.toLocaleDateString('es', { weekday: 'short' }).replace('.', '')} ${s.getDate()} ${short(s)}`,
      };
    }
    if (unit === 'week') {
      const head = e.getMonth() !== s.getMonth() || s.getDate() === 1 || ln.i === lo ? monthLabel(e.getMonth() !== s.getMonth() ? e : s) : '';
      // la semana, entre caminos: en «Mes» cada carril es una
      return { head, name: `Semana ${isoWeek(s)} · ${weekRange(s)}` };
    }
    const q = Math.floor(s.getMonth() / 3) + 1;
    // En «Año», sin indicador entre caminos: el trimestre lo dicen las marcas de mes.
    return { head: q === 1 || ln.i === lo ? String(s.getFullYear()) : '', name: '' };
  }
  // Las metas: una franja a cuadros que cruza el camino donde algo empieza y
  // termina, con su bandera y, debajo, el día con su mes y año. En «Semana», al
  // empezar el lunes y al terminar el domingo; en «Mes», donde cambia el mes
  // (el último día de uno | el primero del otro). El día de antes va del lado
  // de donde viene el camino.
  type GateTag = { text: string; side: 'l' | 'r'; start: boolean };
  type Gate = { key: string; x: number; tags: GateTag[]; flag: boolean };
  /** el día con su mes y año: «31 oct 2026». En una pantalla angosta, sin el año
   *  (cerca del borde se saldría; el año va en el badge). */
  const fullDay = (d: Date) => (narrow ? `${d.getDate()} ${short(d)}` : `${d.getDate()} ${short(d)} ${d.getFullYear()}`);
  function gates(ln: Lane): Gate[] {
    const tail = ln.forward ? right : left;
    // del lado de antes / de después de la puerta, según hacia dónde corre el carril
    const before = ln.forward ? 'l' : 'r', after = ln.forward ? 'r' : 'l';
    const s = laneStart(ln.i);
    if (unit === 'day') {
      const out: Gate[] = [];
      // Una meta por semana, al terminar su último día: la bandera a cuadros es
      // la llegada. El inicio lo dicen el día del carril siguiente y el título
      // de la semana (dos franjas a los lados de una vuelta se leían como dos).
      if (s.getDay() === (first + 6) % 7) out.push({ key: `out${ln.i}`, x: tail, flag: true, tags: [{ text: fullDay(s), side: before, start: false }] });
      return out;
    }
    if (unit === 'week') {
      const w = span / 7;
      for (let k = 0; k < 7; k++) {
        const d = addDays(s, k);
        if (d.getDate() !== 1) continue;
        const prev = addDays(d, -1);
        return [{ key: `m${ln.i}`, x: ln.forward ? left + k * w : right - k * w, flag: true, tags: [
          { text: fullDay(prev), side: before, start: false },
          { text: fullDay(d), side: after, start: true },
        ] }];
      }
    }
    return [];
  }
  /** en «Año», dónde empieza cada mes del trimestre */
  function monthTicks(ln: Lane) {
    if (unit !== 'quarter') return [];
    const s = laneStart(ln.i);
    return [0, 1, 2].map((k) => {
      const m = new Date(s.getFullYear(), s.getMonth() + k, 1);
      return { label: short(m), x: xOf(ln, slotIndex(isoDay(m), ln.i)) };
    });
  }
  // ---- el título que se funde con el badge -------------------------------------
  // Al subir bajo el badge, un título (el mes con su año; en «Semana», también la
  // semana; en «Año», el año) se encoge y se desvanece, y el badge lo recibe: se
  // ensancha y lo muestra junto a «Roadmap». Al volver, sale de nuevo.
  let rmHead = $state<HTMLElement | null>(null);
  let headH = $state(0);
  /** la línea, en el scroller, donde un título ya pasó bajo el badge */
  const headLine = $derived((rmHead?.offsetTop ?? 16) + headH + 4);
  const MERGE = 80; // cuánto antes de la línea empieza a fundirse
  const merge = (ln: Lane) => Math.max(0, Math.min(1, 1 - (ln.y + 4 - scrollY - headLine) / MERGE));
  /** lo que el badge recibió: el mes con su año (el año, en «Año») y, en «Semana», la semana */
  type Ctx = { key: string; month: string; week?: number };
  const ctx = $derived.by<Ctx | null>(() => {
    if (!lanes.length) return null;
    // el carril cuyo título ya cruzó la línea, y hacia atrás el último de cada nivel
    const i0 = Math.max(lo, Math.min(hi, lo + Math.floor((scrollY + headLine - 28) / laneH)));
    let month = '', week: number | undefined;
    for (let i = i0; i >= lo && (!month || (unit === 'day' && !week)); i--) {
      const l = laneLabel(laneOf(i)!);
      if (!month && l.head) month = l.head;
      if (unit === 'day' && !week && l.week) week = l.week.n;
    }
    return month ? { key: `${month}·${week ?? ''}`, month, week } : null;
  });
  const reduceMotion = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  // El ancho del badge, animado: con `width: max-content` CSS no lo anima solo.
  // Sólo cuando cambia el TEXTO: `ctx` es un objeto nuevo en cada paso del
  // scroll, y depender de él volvía a medir a media animación y la reiniciaba
  // (el badge se abría y cerraba solo). Se mide lo que se VE antes de cambiar el
  // DOM, y se anima de ahí a la medida nueva, sin otra animación encima.
  const ctxKey = $derived(ctx?.key ?? '');
  let fromW = 0;
  $effect.pre(() => {
    void ctxKey;
    void narrow;
    if (rmHead) fromW = rmHead.getBoundingClientRect().width;
  });
  $effect(() => {
    void ctxKey;
    void narrow;
    const el = rmHead;
    if (!el || !fromW || reduceMotion) return;
    for (const a of el.getAnimations()) a.cancel();
    const to = el.getBoundingClientRect().width;
    if (Math.abs(to - fromW) > 1) {
      el.animate([{ width: `${fromW}px` }, { width: `${to}px` }], { duration: 280, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)' });
    }
  });

  // ---- minimapa ------------------------------------------------------------------
  // Como el índice del Markdown: una columna de rayas a la derecha, cada una un
  // mes (un trimestre en «Año»), más larga cuanto más trabajo tiene. Se ve la
  // FORMA del tiempo sin leer nada; la del centro de la vista se enciende; al
  // pasar se abre con nombres y cuentas, y un clic lleva ahí.
  type Mark = { key: string; start: Date; label: string; n: number; late: number; today: boolean };
  const byQuarter = $derived(unit === 'quarter');
  const markOf = (d: Date) => new Date(d.getFullYear(), byQuarter ? Math.floor(d.getMonth() / 3) * 3 : d.getMonth(), 1);
  const MAX_MARKS = 30;
  const marks = $derived.by<Mark[]>(() => {
    const stepM = byQuarter ? 3 : 1;
    const t0 = markOf(today);
    const around = byQuarter ? 4 : 6;
    let a = new Date(t0.getFullYear(), t0.getMonth() - around * stepM, 1);
    let b = new Date(t0.getFullYear(), t0.getMonth() + around * stepM, 1);
    const days = work.map((t) => t.day).sort();
    if (days.length) {
      const f = markOf(fromDay(days[0])), l = markOf(fromDay(days[days.length - 1]));
      if (f < a) a = f;
      if (l > b) b = l;
    }
    const count = new Map<string, { n: number; late: number }>();
    for (const t of work) {
      const k = isoDay(markOf(fromDay(t.day)));
      const c = count.get(k) ?? { n: 0, late: 0 };
      c.n++;
      if (t.open && t.day < todayDay) c.late++;
      count.set(k, c);
    }
    let out: Mark[] = [];
    for (let d = a; d <= b; d = new Date(d.getFullYear(), d.getMonth() + stepM, 1)) {
      const k = isoDay(d);
      const q = Math.floor(d.getMonth() / 3) + 1;
      out.push({ key: k, start: d, label: byQuarter ? `T${q} ${d.getFullYear()}` : monthLabel(d), n: count.get(k)?.n ?? 0, late: count.get(k)?.late ?? 0, today: k === isoDay(t0) });
    }
    if (out.length > MAX_MARKS) {
      const ti = out.findIndex((m) => m.today);
      const from = Math.max(0, Math.min(out.length - MAX_MARKS, ti - MAX_MARKS / 2));
      out = out.slice(from, from + MAX_MARKS);
    }
    return out;
  });
  /** la marca del centro de la vista */
  const activeMark = $derived.by(() => {
    if (!lanes.length) return '';
    const i = Math.max(lo, Math.min(hi, lo + Math.floor((seen - 24) / laneH)));
    return isoDay(markOf(laneStart(i)));
  });
  async function jump(m: Mark) {
    if (!scroller) return;
    if (m.today) { goToday(); return; }
    const i = laneIndex(isoDay(m.start));
    const ln = laneOf(i);
    // Dentro de lo que ya está dibujado, se desliza; lejos, se arma la ventana
    // alrededor del destino (sin dibujar todo lo de en medio) y se salta.
    if (ln && i - 2 >= lo && i + perScreen <= hi) {
      scroller.scrollTo({ top: Math.max(0, ln.y - 80), behavior: 'smooth' });
      return;
    }
    growing = true;
    lo = i - Math.ceil(perScreen * 3);
    hi = i + Math.ceil(perScreen * 5);
    // la altura a la que se va a saltar, ya (ver el crecimiento en `onscroll`)
    scrollY = Math.max(0, 24 + (i - lo) * laneH - 80);
    await tick();
    const to = laneOf(i);
    if (to) scroller.scrollTop = Math.max(0, to.y - 80);
    seen = scroller.scrollTop + scroller.clientHeight / 2;
    scrollY = scroller.scrollTop;
    growing = false;
  }

  const kindName = { bubble: 'Módulo', thread: 'Hilo', event: 'Fecha del calendario' };
  function describe(t: Tile) {
    const bits = [t.name, kindName[t.kind]];
    if (t.life) bits.push(bandName(t.life));
    bits.push(fmtDay(t.day));
    if (t.project) bits.push(t.project);
    return bits.join(' · ');
  }
  function open(t: Tile) {
    if (!t.slug) return;
    if (t.kind === 'thread' && t.seq) onopen?.(t.slug, t.seq);
    if (t.kind === 'bubble') { lastOpenBubble.set(t.id); onopenboard?.(t.slug); }
  }
  const initials = (name: string) => name.split(/\s+/).map((s) => s[0]).join('').slice(0, 2).toUpperCase();
</script>

<div class="roadmap">
  <!-- El mismo badge que Secuencia; los filtros y «Hoy», abajo al centro. El
       equipo es el de la columna: aquí no se elige otro. -->
  <header class="rm-head" bind:this={rmHead} bind:offsetHeight={headH} title="Sólo lectura · ↑ pasado · ↓ futuro">
    <h1 class="pane-name display" aria-label="Roadmap"><Glyphs text={narrow && ctx ? '🗺' : '🗺 Roadmap'} tint /></h1>
    {#if ctx}
      {#key ctxKey}
        <!-- Entra de donde viene el título: de abajo al bajar, de arriba al subir. -->
        <span class="ctx" in:fly={{ y: 12 * scrollDir, duration: reduceMotion ? 0 : 280 }}>
          <span class="ctx-month display">{ctx.month}</span>
          {#if ctx.week}<span class="ctx-week">{narrow ? `S${ctx.week}` : `Semana ${ctx.week}`}</span>{/if}
        </span>
      {/key}
    {/if}
    {#if error}<span class="err" role="alert">{error} <button onclick={load}>Reintentar</button></span>{/if}
  </header>

  <div class="scroller" role="presentation" bind:this={scroller} bind:clientHeight={viewH} onscroll={onscroll} onpointermove={onmove} onpointerleave={onleave}>
    {#if !board && !error}
      <div class="loading"><Loading /></div>
    {:else}
      <div class="world" bind:this={world} style:height={`${total}px`} style:--rw={`${RW}px`} bind:clientWidth={width}>
        {#if width}
          <!-- La cinta en relieve: sombra, costado, cara y brillo. -->
          <svg class="road" width={width} height={total} aria-hidden="true">
            <defs>
              <pattern id="rm-checker" width="8" height="8" patternUnits="userSpaceOnUse">
                <rect width="8" height="8" fill="#fff" />
                <rect width="4" height="4" fill="#0f172a" />
                <rect x="4" y="4" width="4" height="4" fill="#0f172a" />
              </pattern>
              <filter id="rm-shadow" x="-5%" y="-5%" width="110%" height="110%">
                <feGaussianBlur stdDeviation="7" />
              </filter>
              {#if swell.box}
                <filter id="rm-swell-shadow" filterUnits="userSpaceOnUse" x={swell.box.x} y={swell.box.y} width={swell.box.w} height={swell.box.h}>
                  <feGaussianBlur stdDeviation="7" />
                </filter>
              {/if}
            </defs>
            <!-- La sombra del tramo ensanchado va debajo de todo: así no mancha el camino. -->
            {#if swell.shadow && swell.box}<path class="road-shadow fill" d={swell.shadow} filter="url(#rm-swell-shadow)" />{/if}
            <path class="road-shadow" d={road} stroke-width={RW + 6} filter="url(#rm-shadow)" transform={`translate(0 ${Math.round(RW * 0.45)})`} />
            <path class="road-side" d={road} stroke-width={RW} transform={`translate(0 ${Math.round(RW * 0.28)})`} />
            <!-- El costado ensanchado va DEBAJO de la cara de siempre: en una curva
                 el costado corre a lo largo del camino y, encima, asomaría como una
                 barra oscura más allá del corte de la lupa. -->
            {#if swell.side}<path class="road-side fill swell" d={swell.side} />{/if}
            <path class="road-body" d={road} stroke-width={RW} />
            <path class="road-shine" d={road} stroke-width={Math.max(2, Math.round(RW * 0.14))} transform={`translate(0 ${-Math.round(RW * 0.28)})`} />
            {#if swell.body}
              <path class="road-body fill swell" d={swell.body} />
              <path class="road-shine fill swell" d={swell.shine} />
            {/if}
          </svg>

          {#each lanes as ln (ln.i)}
            {@const label = laneLabel(ln)}
            {#if ln.i === 0}<div class="now-band" style:top={`${ln.y}px`} style:height={`${laneH}px`}></div>{/if}
            <!-- Los títulos, por nivel: el mes con su año, grande; la semana, mediana. -->
            {#if label.head || label.week}
              {@const mp = merge(ln)}
              <div class="titles" class:big={unit !== 'quarter'} style:top={`${ln.y + 4}px`}
                style:opacity={mp ? 1 - mp : undefined} style:transform={mp ? `translateY(${-mp * 18}px) scale(${1 - mp * 0.3})` : undefined}>
                {#if label.head}<span class="month display">{label.head}</span>{/if}
                {#if label.week}<span class="week-head"><b class="display">Semana {label.week.n}</b> {label.week.range}</span>{/if}
              </div>
            {/if}
            <!-- Arriba del carril, en el lado donde empieza: en «Semana», el día;
                 en «Mes», la semana. En «Año» no hay. En una pantalla angosta los
                 títulos (al centro) lo taparían: baja lo que ellos ocupan. -->
            {#if label.name}
              {@const below = narrow ? (label.head && label.week ? 50 : label.head || label.week ? 28 : 0) : 0}
              <div class="lane-name" style:top={`${ln.y + 6 + below}px`}
                style:left={ln.forward ? `${left + 6}px` : undefined} style:right={ln.forward ? undefined : `${M + 6}px`}>{label.name}</div>
            {/if}
            {#each gates(ln) as g (g.key)}
              {@const L = lensed(g.x, ln.mid, ln.mid)}
              {@const half = (RW * (1 + SWELL * swellAt(g.x, ln.mid))) / 2}
              <div class="gate" aria-hidden="true" style:left={`${L.x}px`} style:top={`${ln.mid - half - 4}px`} style:height={`${half * 2 + 8}px`}></div>
              <!-- La bandera, plantada en el borde de arriba, ondea hacia donde va el camino. -->
              {#if g.flag}<svg class="gate-flag" class:rev={!ln.forward} aria-hidden="true" width="32" height="36" viewBox="0 0 32 36"
                style:left={`${L.x}px`} style:top={`${ln.mid - half - 40}px`}>
                <rect class="pole" x="0" y="0" width="2.5" height="36" rx="1.25" />
                <circle class="pole" cx="1.25" cy="1.25" r="2" />
                <path class="cloth" d="M2.5 3 C8 0.5 12 5 18 3 S27 1 31 3 V19 C27 17.5 23 21 18 19.5 S8 16.5 2.5 19 Z" fill="url(#rm-checker)" />
              </svg>{/if}
              {#each g.tags as tg (tg.text)}
                <div class={`gate-tag ${tg.side}`} class:start={tg.start} style:left={`${L.x}px`} style:top={`${ln.mid + half + 8}px`}>{tg.text}</div>
              {/each}
            {/each}
            {#each monthTicks(ln) as tk (tk.label)}
              <!-- Bajo el borde del camino: con la lupa se corre como las casillas
                   (marca la misma fecha), baja con el camino ensanchado y crece. -->
              {@const L = lensed(tk.x, ln.mid, ln.mid)}
              <div class="tick" style:left={`${L.x}px`} style:top={`${ln.mid + RW * (1 + SWELL * swellAt(tk.x, ln.mid)) * 0.78 + 8}px`} style:--s={L.s}>{tk.label}</div>
            {/each}
          {/each}

          {#if now}
            {@const L = lensed(now.x, now.y, now.y)}
            <div class="today" style:left={`${L.x}px`} style:top={`${L.y - (RW / 2) * L.s}px`} aria-label="Hoy">
              <span class="flag">HOY</span>
            </div>
          {/if}

          {#each placed as t (t.key)}
            {@const L = lensed(t.x, t.y, t.mid)}
            {#if t.kind === 'event'}
              <div class="sign" class:compact style:left={`${L.x}px`} style:top={`${L.y}px`} style:--s={L.s} style:z-index={2 + Math.round((L.s - 1) * 10)} title={crowded(t.group) ? undefined : describe(t)} role="img" aria-label={describe(t)}
                onpointerenter={() => stackEnter(t.group)} onpointerleave={stackLeave}>
                <Icon of="calendar" size={compact ? '18px' : '20px'} tint />
                {#if !compact}<span class="tname">{t.name}</span>{/if}
              </div>
            {:else}
              <button class={`tile life-${t.life}`} class:compact
                style:left={`${L.x}px`} style:top={`${L.y}px`} style:--s={L.s} style:z-index={2 + Math.round((L.s - 1) * 10)} style:width={compact ? undefined : `${Math.min(slotW(laneIndex(t.day)) - 10, 180)}px`}
                title={crowded(t.group) ? undefined : describe(t)} aria-label={describe(t)} onclick={() => open(t)}
                onpointerenter={() => stackEnter(t.group)} onpointerleave={stackLeave} onfocus={() => stackEnter(t.group)} onblur={stackLeave}>
                <Icon of={bandFace(t.life ?? 'dormant')} size={compact ? '20px' : '22px'} tint />
                {#if !compact}
                  <span class="tbody">
                    <span class="tname">{t.name}</span>
                    <span class="tmeta">{t.kind === 'bubble' ? 'módulo' : `#${t.seq}`} · {fmtDay(t.day)}</span>
                  </span>
                {/if}
                {#if t.more}<span class="more">+{t.more}</span>{/if}
              </button>
            {/if}
          {/each}

          {#each pieces as p (p.id)}
            {@const L = lensed(p.x, p.y, p.mid)}
            <div class="piece" class:late={p.late > 0} style:left={`${L.x}px`} style:top={`${L.y}px`} style:--s={L.s} style:z-index={L.s > 1.01 ? 11 : 10}
              role="img" aria-label={`${p.name}: ${p.on ? `va en «${p.on}» (${fmtDay(p.day!)})` : 'nada abierto con fecha'}${p.late ? `, ${p.late} días de atraso` : ''}`}
              title={crowded(p.group) ? undefined : `${p.name} · ${p.on ? `va en «${p.on}», ${fmtDay(p.day!)}` : 'nada abierto con fecha: espera en hoy'}${p.late ? ` · ${p.late} días de atraso` : ''}`}
              onpointerenter={() => stackEnter(p.group)} onpointerleave={stackLeave}>
              <span class="head">{#if p.avatar}<img src={p.avatar} alt="" />{:else}{initials(p.name)}{/if}</span>
            </div>
          {/each}

          {#if stack && stackAt}
            <div class="stack-card" class:below={stackAt.below} role="group" aria-label={`Lo que hay aquí: ${stackRange(stack)}`}
              style:left={`${stackAt.x}px`} style:top={`${stackAt.y}px`}
              onpointerenter={() => clearTimeout(stackTimer)} onpointerleave={stackLeave}>
              <header class="stack-head">
                <span>{stackRange(stack)}</span>
                <span class="stack-n">{stack.tiles.length + stack.people.length === 1 ? '1 cosa' : `${stack.tiles.length + stack.people.length} cosas`}</span>
              </header>
              {#each stack.people as p (p.id)}
                <div class="stack-row who" class:late={p.late > 0}>
                  <span class="mini">{#if p.avatar}<img src={p.avatar} alt="" />{:else}{initials(p.name)}{/if}</span>
                  <span class="stack-body">
                    <span class="tname">{p.name}</span>
                    <span class="tmeta">{p.on ? `va en «${p.on}»` : 'nada abierto con fecha'}{#if p.late}{' · '}<b class="late-n">{p.late} días de atraso</b>{/if}</span>
                  </span>
                </div>
              {/each}
              {#each stack.tiles as t (t.key)}
                {#if t.kind === 'event'}
                  <div class="stack-row">
                    <Icon of="calendar" size="20px" tint />
                    <span class="stack-body"><span class="tname">{t.name}</span><span class="tmeta">fecha del calendario · {fmtDay(t.day)}</span></span>
                  </div>
                {:else}
                  <button class="stack-row work" onclick={() => open(t)}>
                    <Icon of={bandFace(t.life ?? 'dormant')} size="20px" tint />
                    <span class="stack-body">
                      <span class="tname">{t.name}</span>
                      <span class="tmeta">{t.kind === 'bubble' ? 'módulo' : `#${t.seq}`} · {fmtDay(t.day)}{#if t.project}{` · ${t.project}`}{/if}</span>
                    </span>
                  </button>
                {/if}
              {/each}
            </div>
          {/if}
        {/if}
      </div>
    {/if}
  </div>

  <div class="rm-filters">
    <div class="seg" role="group" aria-label="Qué se ve en el camino">
      <button class:on={mode === 'all'} aria-pressed={mode === 'all'} onclick={() => (mode = 'all')}>Todo</button>
      <button class:on={mode === 'bubbles'} aria-pressed={mode === 'bubbles'} onclick={() => (mode = 'bubbles')}>Módulos</button>
      <button class:on={mode === 'threads'} aria-pressed={mode === 'threads'} onclick={() => (mode = 'threads')}>Hilos</button>
    </div>
    <div class="seg zoom" role="group" aria-label="Qué llena la pantalla">
      {#each Object.entries(ZOOMS) as [z, cfg] (z)}
        <button class:on={zoom === z} aria-pressed={zoom === z} onclick={() => setZoom(z as Zoom)}>{cfg.label}</button>
      {/each}
    </div>
    <button class="today-btn" onclick={() => goToday()}>Hoy</button>
  </div>

  {#if board && marks.length}
    <nav class="time-rail" aria-label="Ir a una fecha">
      <div class="rail-head">Ir a</div>
      <div class="list">
        {#each marks as m (m.key)}
          <button class="mark" class:active={m.key === activeMark} class:is-today={m.today} class:late={m.late > 0}
            style:--dash={`${m.n ? Math.min(18, 8 + m.n * 2) : 5}px`}
            aria-current={m.key === activeMark ? 'location' : undefined}
            onclick={() => jump(m)}>
            <span class="dash"></span>
            <span class="mlabel">{m.label}{#if m.today}<b class="mtoday">{'\u00a0· hoy'}</b>{/if}</span>
            {#if m.n}<span class="mcount" title={m.late ? `${m.late} atrasadas` : undefined}>{m.n}</span>{/if}
          </button>
        {/each}
      </div>
    </nav>
  {/if}
</div>

<style>
  .roadmap { position: relative; height: 100%; min-height: 0; display: flex; flex-direction: column; }
  /* El badge titular: el mismo que Secuencia, flotando arriba al centro. */
  .rm-head {
    position: absolute; top: 1rem; left: 50%; transform: translateX(-50%); z-index: 25;
    display: flex; flex-wrap: nowrap; justify-content: center; align-items: center; gap: 0.6rem; overflow: hidden; white-space: nowrap;
    width: max-content; max-width: calc(100% - 2rem); min-height: 42px; padding: 0.45rem 0.9rem;
    border: 1px solid var(--line); border-radius: 26px; background: var(--surface-solid);
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.16);
  }
  /* Los filtros, abajo al centro, planos en el HUD: cada grupo lleva su propio
     borde, sin otra píldora alrededor. */
  .rm-filters {
    position: absolute; bottom: 1rem; left: 50%; transform: translateX(-50%); z-index: 25;
    display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 0.5rem;
    width: max-content; max-width: calc(100% - 2rem);
  }
  /* Sin lugar al centro entre las esquinas de abajo: sube sobre ellas. */
  @media (max-width: 1320px) { .rm-filters { bottom: 4.75rem; } }
  /* Lo que el badge recibió del camino: el mes con su año, y la semana. */
  .ctx { display: flex; align-items: baseline; gap: 0.45rem; padding-left: 0.6rem; border-left: 1px solid var(--line); }
  .ctx-month { font-size: 1.2rem; color: var(--text); }
  .ctx-week { font-size: 0.85rem; font-weight: 700; color: var(--muted); }
  .pane-name { margin: 0; font-size: 1.5rem; font-weight: inherit; white-space: nowrap; display: flex; align-items: center; gap: 0.3rem; }
  .seg { display: flex; padding: 3px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface-solid); }
  .seg button { padding: 0.25rem 0.7rem; border-radius: 7px; font-size: 0.85rem; color: var(--muted); }
  .seg button.on { background: var(--hover); color: var(--text); font-weight: 700; }
  .today-btn { padding: 0.3rem 0.9rem; border-radius: 999px; border: 1px solid var(--line); background: var(--surface-solid); font-weight: 700; font-size: 0.85rem; }
  .today-btn:hover { background: var(--hover); }
  .err { color: var(--color-error-500); font-size: 0.8rem; }
  .err button { text-decoration: underline; }

  .scroller { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; overscroll-behavior: contain; }

  /* El minimapa: como el índice del Markdown (ThreadToc). Sin transform ni
     backdrop-filter en el riel, por las mismas razones que allá. No se le
     reserva margen (achicaría los lugares y las fichas perderían el nombre):
     las rayas son cortas y caen, cuando mucho, sobre el borde de una vuelta. */
  .time-rail {
    position: absolute; z-index: 24; right: 0.25rem; top: 0; bottom: 0; height: fit-content; margin-block: auto;
    display: flex; flex-direction: column; padding: 0.5rem 0.35rem;
    border: 1px solid transparent; border-radius: 16px; background: transparent;
    transition: background 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease, padding 0.18s ease;
  }
  .time-rail .list { display: flex; flex-direction: column; align-items: flex-end; gap: 3px; max-height: 70vh; overflow: hidden; }
  .rail-head {
    display: none; font-size: 0.66rem; font-weight: 800; letter-spacing: 0.09em; text-transform: uppercase; color: var(--faint);
    padding: 0 0.4rem 0.35rem; border-bottom: 1px solid var(--line); margin-bottom: 0.2rem;
  }
  .mark {
    display: flex; align-items: center; justify-content: flex-end; gap: 0.5rem; width: 100%;
    padding: 2px 0.15rem; border: none; border-radius: 7px; background: transparent; color: var(--muted);
  }
  .mark .dash { display: block; width: var(--dash); height: 3px; border-radius: 999px; background: currentColor; opacity: 0.45; flex: none; transition: width 0.18s ease, opacity 0.18s ease; }
  .mark.late .dash { background: #ef4444; opacity: 0.7; }
  .mark.is-today .dash { background: var(--accent); opacity: 0.8; }
  .mark.active .dash { background: var(--accent); opacity: 1; height: 4px; }
  .mlabel, .mcount, .mtoday { display: none; }
  .mlabel { font-size: 0.78rem; white-space: nowrap; }
  .mtoday { color: var(--accent); font-weight: 700; }
  .mcount { margin-left: auto; padding: 0 0.4rem; border-radius: 999px; background: var(--hover); color: var(--faint); font-size: 0.66rem; }
  .mark.late .mcount { color: #ef4444; }
  .time-rail:hover, .time-rail:focus-within {
    min-width: 200px; padding: 0.7rem 0.75rem;
    background: var(--surface-solid); border-color: var(--line); box-shadow: 0 20px 50px var(--shadow-strong);
  }
  .time-rail:hover .list, .time-rail:focus-within .list { align-items: stretch; gap: 1px; overflow-y: auto; }
  .time-rail:hover .rail-head, .time-rail:focus-within .rail-head { display: block; }
  .time-rail:hover .mark, .time-rail:focus-within .mark { justify-content: flex-start; padding: 5px 8px; }
  .time-rail:hover .mark:hover, .time-rail:focus-within .mark:hover { background: var(--hover); color: var(--text); }
  .time-rail:hover .mark.active, .time-rail:focus-within .mark.active { color: var(--accent); background: color-mix(in oklab, var(--accent) 14%, transparent); }
  .time-rail:hover .dash, .time-rail:focus-within .dash { display: none; }
  .time-rail:hover .mlabel, .time-rail:hover .mcount, .time-rail:hover .mtoday,
  .time-rail:focus-within .mlabel, .time-rail:focus-within .mcount, .time-rail:focus-within .mtoday { display: inline; }
  /* En un teléfono no hay lugar para el riel. */
  @media (max-width: 900px) { .time-rail { display: none; } }
  .loading { height: 100%; display: grid; place-items: center; }
  .world { position: relative; width: 100%; max-width: 1360px; margin: 0 auto; }

  /* El camino: una cinta en relieve. La sombra cae abajo, el costado (más
     oscuro) asoma debajo de la cara, y un brillo corre por el borde de arriba. */
  .road { position: absolute; inset: 0 auto auto 0; pointer-events: none; overflow: visible; }
  .road path { fill: none; stroke-linecap: round; stroke-linejoin: round; }
  .road path.fill { stroke: none; }
  .road-shadow.fill { fill: rgb(15 23 42 / 0.22); }
  .road-side.fill { fill: color-mix(in oklab, var(--accent) 62%, #0f172a 38%); }
  .road-body.fill { fill: color-mix(in oklab, var(--accent) 34%, var(--surface-solid)); }
  .road-shine.fill { fill: rgb(255 255 255 / 0.55); }
  :global([data-mode='dark']) .road-shadow.fill { fill: rgb(0 0 0 / 0.55); }
  :global([data-mode='dark']) .road-body.fill { fill: color-mix(in oklab, var(--accent) 42%, var(--surface-solid)); }
  :global([data-mode='dark']) .road-shine.fill { fill: rgb(255 255 255 / 0.18); }
  .road-shadow { stroke: rgb(15 23 42 / 0.22); }
  .road-side { stroke: color-mix(in oklab, var(--accent) 62%, #0f172a 38%); }
  .road-body { stroke: color-mix(in oklab, var(--accent) 34%, var(--surface-solid)); }
  .road-shine { stroke: rgb(255 255 255 / 0.55); }
  :global([data-mode='dark']) .road-shadow { stroke: rgb(0 0 0 / 0.55); }
  :global([data-mode='dark']) .road-body { stroke: color-mix(in oklab, var(--accent) 42%, var(--surface-solid)); }
  :global([data-mode='dark']) .road-shine { stroke: rgb(255 255 255 / 0.18); }

  .now-band { position: absolute; left: 0; right: 0; background: color-mix(in oklab, var(--accent) 7%, transparent); pointer-events: none; }
  .titles { position: absolute; left: 0; right: 0; transform-origin: 50% 0; display: flex; flex-wrap: wrap; justify-content: center; align-items: baseline; column-gap: 0.9rem; padding-inline: 0.5rem; pointer-events: none; white-space: nowrap; }
  .month { font-size: 1.15rem; color: var(--text); }
  .titles.big .month { font-size: 1.9rem; line-height: 1.1; }
  .week-head { font-size: 0.8rem; color: var(--muted); }
  .week-head b { font-size: 1.15rem; font-weight: inherit; color: var(--text); margin-right: 0.2rem; }
  .lane-name { position: absolute; font-size: 0.7rem; font-weight: 700; color: var(--muted); pointer-events: none; white-space: nowrap; }
  /* La meta: una franja a cuadros (dos columnas) que cruza el camino, y su bandera. */
  .gate {
    position: absolute; width: 12px; translate: -50% 0; z-index: 1; pointer-events: none; border-radius: 2px;
    background: conic-gradient(#0f172a 25%, #fff 0 50%, #0f172a 0 75%, #fff 0) 0 0 / 12px 12px;
    box-shadow: 0 0 0 1px rgb(15 23 42 / 0.45), 0 2px 6px rgb(0 0 0 / 0.25);
  }
  .gate-flag { position: absolute; z-index: 1; pointer-events: none; overflow: visible; translate: -1.25px 0; filter: drop-shadow(0 2px 3px rgb(0 0 0 / 0.25)); }
  .gate-flag.rev { transform: scaleX(-1); transform-origin: 1.25px 0; }
  .gate-flag .pole { fill: #334155; }
  .gate-flag .cloth { stroke: #0f172a; stroke-width: 0.8; stroke-linejoin: round; }
  :global([data-mode='dark']) .gate-flag .pole { fill: #cbd5e1; }
  .gate-tag {
    position: absolute; z-index: 1; pointer-events: none; white-space: nowrap;
    padding: 0.1rem 0.45rem; border-radius: 6px; border: 1px solid var(--line); background: var(--surface-solid);
    font-size: 0.64rem; font-weight: 800; letter-spacing: 0.05em; text-transform: uppercase; color: var(--muted);
  }
  .gate-tag.l { translate: calc(-100% - 3px) 0; }
  .gate-tag.r { translate: 3px 0; }
  .gate-tag.start { color: var(--accent); border-color: color-mix(in oklab, var(--accent) 45%, var(--line)); }
  .tick { position: absolute; translate: -50% 0; scale: var(--s, 1); transform-origin: 50% 0; font-size: 0.66rem; font-weight: 700; color: var(--faint); text-transform: uppercase; letter-spacing: 0.05em; pointer-events: none; }

  .today { position: absolute; width: 0; height: 0; z-index: 9; pointer-events: none; }
  .flag {
    position: absolute; left: 0; bottom: 12px; transform: translateX(-50%);
    padding: 0.1rem 0.5rem; border-radius: 6px; background: var(--accent); color: white;
    font-size: 0.7rem; font-weight: 900; letter-spacing: 0.06em;
    box-shadow: 0 3px 0 color-mix(in oklab, var(--accent) 60%, #0f172a), 0 6px 14px color-mix(in oklab, var(--accent) 40%, transparent);
  }
  .flag::after { content: ''; position: absolute; left: 50%; top: calc(100% + 3px); width: 2px; height: 10px; background: var(--accent); transform: translateX(-50%); }

  /* Las casillas: botones en relieve, del color de su banda. */
  .tile, .sign { position: absolute; translate: -50% -50%; scale: var(--s, 1); z-index: 2; }
  .tile {
    --band: #94a3b8;
    display: flex; align-items: center; gap: 0.45rem; padding: 0.35rem 0.55rem; min-height: 44px;
    border-radius: 14px; border: 1px solid color-mix(in oklab, var(--band) 55%, transparent);
    background: linear-gradient(180deg, color-mix(in oklab, var(--band) 10%, var(--surface-solid)), color-mix(in oklab, var(--band) 22%, var(--surface-solid)));
    color: var(--text); text-align: left;
    box-shadow: 0 4px 0 color-mix(in oklab, var(--band) 60%, #0f172a), 0 10px 18px color-mix(in oklab, var(--band) 25%, transparent);
    transition: transform 120ms ease, box-shadow 120ms ease;
  }
  .tile:hover { transform: translateY(-2px); box-shadow: 0 6px 0 color-mix(in oklab, var(--band) 60%, #0f172a), 0 14px 24px color-mix(in oklab, var(--band) 32%, transparent); }
  .tile:active { transform: translateY(2px); box-shadow: 0 2px 0 color-mix(in oklab, var(--band) 60%, #0f172a), 0 6px 12px color-mix(in oklab, var(--band) 25%, transparent); }
  .tile:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  .tile.compact { width: 40px; height: 40px; min-height: 0; padding: 0; justify-content: center; border-radius: 999px; }
  .life-hot { --band: #f97316; }
  .life-dormant { --band: #8b5cf6; }
  .life-rip { --band: #64748b; }
  .life-closed { --band: #eab308; }
  .tbody { display: flex; flex-direction: column; min-width: 0; line-height: 1.15; }
  .tname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 700; font-size: 0.82rem; }
  .tmeta { color: var(--muted); font-size: 0.7rem; }
  .more { position: absolute; right: -8px; top: -8px; padding: 0 0.35rem; border-radius: 999px; background: var(--text); color: var(--bg); font-size: 0.65rem; font-weight: 800; }
  .sign {
    display: flex; align-items: center; gap: 0.35rem; max-width: 150px; padding: 0.2rem 0.55rem; border-radius: 999px;
    border: 1px solid var(--line); background: var(--surface-solid); color: var(--muted); font-size: 0.75rem;
    box-shadow: 0 2px 0 color-mix(in oklab, var(--text) 18%, transparent), 0 6px 12px rgb(0 0 0 / 0.08);
  }
  .sign.compact { padding: 0.2rem; }
  .sign .tname { font-weight: 600; }

  /* Las piezas: el avatar, sin sombra, un poco arriba de su casilla. */
  .piece { position: absolute; z-index: 10; width: 0; height: 0; scale: var(--s, 1); }
  .piece .head {
    position: absolute; left: 0; bottom: 26px; transform: translateX(-50%);
    display: grid; place-items: center; width: 34px; height: 34px; overflow: hidden;
    border-radius: 999px; border: 3px solid var(--surface-solid); background: var(--accent); color: white;
    font-size: 0.72rem; font-weight: 800;
  }
  .piece .head img { width: 100%; height: 100%; object-fit: cover; }
  .piece.late .head { border-color: #ef4444; box-shadow: 0 0 0 3px color-mix(in oklab, #ef4444 35%, transparent); }

  @media (max-width: 700px) {
    .rm-head { top: 4rem; gap: 0.35rem; }
    .titles.big .month { font-size: 1.45rem; }
    .pane-name { font-size: 1.25rem; }
  }
  /* La pila abierta: una tarjeta que crece desde su lugar. */
  .stack-card {
    position: absolute; z-index: 30; width: min(300px, 86vw); max-height: 320px; overflow-y: auto;
    translate: -50% -100%; padding: 0.45rem; border-radius: 16px;
    border: 1px solid var(--line); background: var(--surface-solid); color: var(--text);
    box-shadow: 0 4px 0 color-mix(in oklab, var(--text) 14%, transparent), 0 18px 40px rgb(0 0 0 / 0.22);
    transform-origin: 50% 100%; animation: stack-in 160ms cubic-bezier(.2,.8,.2,1);
  }
  .stack-card.below { translate: -50% 0; transform-origin: 50% 0; }
  @keyframes stack-in { from { transform: scale(0.6); opacity: 0; } to { transform: scale(1); opacity: 1; } }
  .stack-card { display: flex; flex-direction: column; gap: 0.4rem; }
  .stack-head { display: flex; justify-content: space-between; gap: 0.5rem; padding: 0.15rem 0.4rem 0.1rem; font-size: 0.72rem; font-weight: 800; color: var(--muted); }
  .stack-n { font-weight: 600; }
  /* Todas las filas iguales: contorno, y al pasar, fondo y contorno marcado. */
  .stack-row {
    display: flex; align-items: center; gap: 0.6rem; width: 100%; padding: 0.55rem 0.65rem;
    border: 1px solid var(--line); border-radius: 12px; background: transparent; text-align: left;
    transition: background 120ms ease, border-color 120ms ease;
  }
  .stack-row:hover, .stack-row:focus-visible { background: var(--hover); border-color: color-mix(in oklab, var(--text) 28%, transparent); }
  .stack-row:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .stack-body { display: flex; flex-direction: column; min-width: 0; line-height: 1.2; }
  .stack-row .tname { font-size: 0.85rem; }
  .mini { display: grid; place-items: center; flex: none; width: 28px; height: 28px; overflow: hidden; border-radius: 999px; background: var(--accent); color: white; font-size: 0.65rem; font-weight: 800; border: 2px solid var(--surface-solid); }
  .mini img { width: 100%; height: 100%; object-fit: cover; }
  .who.late .mini { border-color: #ef4444; }
  .late-n { color: #ef4444; font-weight: 700; }

  @media (prefers-reduced-motion: reduce) { .tile { transition: none; } .stack-card { animation: none; } }
</style>
