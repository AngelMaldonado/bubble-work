<script lang="ts">
  // Mi espacio: un lienzo infinito personal con post-its.
  //
  // Cada nota guarda dónde está (x, y) y su ancho en el SERVIDOR, así el
  // tablero que acomodaste te sigue a otro equipo. La vista (pan/zoom) sí es de
  // este navegador. Nadie más lo lee —tampoco un
  // lead—: lo dicen las reglas del servidor; esta pantalla sólo lo dibuja.
  //
  // Los gestos copian el secuenciador: arrastrar el fondo mueve el lienzo,
  // arrastrar una nota la mueve, un clic sin mover la abre para escribir; la
  // rueda desplaza y ⌘/Ctrl+rueda acerca alrededor del puntero.
  import { onDestroy, tick } from 'svelte';
  import { api, type PersonalAsset, type PersonalNote, type Workspace } from '../lib/api';
  import { NOTE_COLORS, NOTE_COLOR_NAMES, projectColors, safeColor } from '../lib/projectColors.svelte';
  import { toggleMarkdownTodo } from '../lib/markdownTodo';
  import { vimPref } from '../lib/vim.svelte';
  import Loading from './Loading.svelte';
  import Prose from './Prose.svelte';
  import MarkdownField from './MarkdownField.svelte';
  import Icon from './Icon.svelte';
  import Confirm, { type Doom } from './Confirm.svelte';

  let { workspaces = [] }: { workspaces?: Workspace[] } = $props();

  // h = 0: tan alta como su texto. Si se le dio alto, el texto se desplaza dentro.
  const W0 = 240, WMIN = 180, WMAX = 1200, HMIN = 140, HMAX = 1600, EDIT_W = 560;
  const owner = api.me?.id ?? '';
  const key = (k: string) => `bubble.space.${owner || 'local'}.${k}`;
  function readLS<T>(k: string, fallback: T): T {
    try { const v = localStorage.getItem(key(k)); return v ? (JSON.parse(v) as T) : fallback; } catch { return fallback; }
  }
  function writeLS(k: string, v: unknown) { try { localStorage.setItem(key(k), JSON.stringify(v)); } catch { /* sin memoria */ } }

  // Un número que no lo es (null, NaN, un campo que el servidor aún no trae)
  // no puede llegar a un transform: uno solo traba todo el lienzo.
  const num = (v: unknown, d: number) => (typeof v === 'number' && Number.isFinite(v) ? v : d);
  type View = { x: number; y: number; z: number };
  const sane = (v: Partial<View> | null | undefined): View =>
    ({ x: num(v?.x, 0), y: num(v?.y, 0), z: Math.min(2, Math.max(0.3, num(v?.z, 1))) });
  const healthy = (v: Partial<View> | null) => !!v && [v.x, v.y, v.z].every((k) => typeof k === 'number' && Number.isFinite(k)) && v.z! >= 0.3 && v.z! <= 2;
  const norm = (n: PersonalNote, i = 0): PersonalNote => ({
    ...n,
    x: num(n.x, (i % 4) * 280), y: num(n.y, Math.floor(i / 4) * 300),
    w: Math.min(WMAX, Math.max(WMIN, num(n.w, W0) || W0)),
    h: num(n.h, 0) > 0 ? Math.min(HMAX, Math.max(HMIN, num(n.h, 0))) : 0,
  });

  let notes = $state<PersonalNote[]>([]);
  let assets = $state<PersonalAsset[]>([]);
  let html = $state<Record<string, string>>({});
  let token = $state('');
  let loaded = $state(false);
  let error = $state('');
  let saving = $state(0);
  let busy = $state('');
  let doom = $state<Doom>(null);
  let stopped = false;
  let generation = 0;
  const current = (turn: number) => !stopped && turn === generation && api.me?.id === owner;

  /** Qué desplegable de nota está abierto: `c:<id>` color, `p:<id>` proyecto. */
  let menuFor = $state('');
  // Restos de la caja de herramientas que ya no existe: un filtro guardado
  // escondía notas sin que hubiera dónde quitarlo.
  try { localStorage.removeItem(key('only')); localStorage.removeItem(key('toolbox')); } catch { /* sin memoria */ }

  const projectName = (id: string) => workspaces.find((w) => w.id === id)?.name ?? '';
  const filesOf = (note: string) => assets.filter((a) => a.note === note);
  const shown = $derived(notes);

  async function render(n: PersonalNote) {
    try {
      const out = n.body.trim() ? await api.renderMarkdown(n.body) : '';
      if (!stopped && api.me?.id === owner) html = { ...html, [n.id]: out };
    } catch { /* el texto sigue en el editor */ }
  }

  async function load() {
    const turn = ++generation;
    try {
      const [n, a, t] = await Promise.all([api.personalNotes(), api.personalAssets(), api.fileToken(), projectColors.load()]);
      if (!current(turn)) return;
      notes = n.items.map(norm); assets = a.items; token = t.token; error = '';
      order = notes.map((x) => x.id).reverse();
      for (const x of notes) void render(x);
    } catch (e) {
      if (current(turn)) error = (e as Error).message || 'No se pudo leer tu espacio';
    } finally {
      if (current(turn)) loaded = true;
    }
  }
  load();
  const renew = setInterval(async () => { try { const t = await api.fileToken(); if (!stopped) token = t.token; } catch { /* siguiente vuelta */ } }, 100_000);
  onDestroy(() => { stopped = true; ++generation; clearInterval(renew); clearTimeout(nudgeTimer); });

  async function patch(n: PersonalNote, fields: Partial<PersonalNote>) {
    const before = notes.find((x) => x.id === n.id) ?? n;
    notes = notes.map((x) => (x.id === n.id ? { ...x, ...fields } : x));
    saving++;
    try {
      const saved = await api.update<PersonalNote>('personal_notes', n.id, fields);
      if (stopped) return;
      // Un servidor sin la migración devuelve 200 y descarta el campo: dejarlo
      // en pantalla y decirlo, en vez de que el cambio se deshaga sin razón.
      const lost = Object.keys(fields).filter((k) => !(k in saved));
      const kept = Object.keys(fields).filter((k) => k in saved);
      notes = notes.map((x) => (x.id === n.id ? norm({ ...x, ...pick(saved, kept) }) : x));
      if (lost.length) error = `El servidor no guardó ${lost.join(', ')}: reinicia el servidor para que aplique las migraciones.`;
      if ('body' in fields) void render(saved);
    } catch (e) {
      if (stopped) return;
      notes = notes.map((x) => (x.id === n.id ? { ...x, ...pick(before, Object.keys(fields)) } : x));
      error = (e as Error).message;
    } finally { saving--; }
  }
  /** Sólo lo que este guardado escribió: otro guardado en vuelo no se pisa. */
  function pick(n: PersonalNote, keys: string[]) {
    return Object.fromEntries(keys.map((k) => [k, (n as unknown as Record<string, unknown>)[k]])) as Partial<PersonalNote>;
  }

  // ---- el lienzo ----
  let canvas = $state<HTMLElement | null>(null);
  const stored = readLS<Partial<View> | null>('view', null);
  let view = $state<View>(sane(stored));
  function setView(v: Partial<View>) { view = sane(v); }
  $effect(() => writeLS('view', sane(view)));
  // Una vista guardada rota se descarta y se vuelve a encuadrar.
  let centered = healthy(stored);
  $effect(() => {
    if (centered || !canvas || !loaded) return;
    centered = true;
    void tick().then(fit);
  });

  /** Orden de apilado: la última nota tocada queda arriba. */
  let order = $state<string[]>([]);
  const zOf = (id: string) => order.indexOf(id) + 1;
  function raise(id: string) { order = [...order.filter((x) => x !== id), id]; }

  const els: Record<string, HTMLElement> = {};
  function placed(el: HTMLElement, id: string) {
    els[id] = el;
    return { destroy() { if (els[id] === el) delete els[id]; } };
  }

  function toWorld(cx: number, cy: number) {
    const r = canvas!.getBoundingClientRect();
    return { x: (cx - r.left - view.x) / view.z, y: (cy - r.top - view.y) / view.z };
  }
  function scrollable(el: Element | null) {
    for (let n = el as HTMLElement | null; n && n !== canvas; n = n.parentElement) {
      if (n.scrollHeight > n.clientHeight + 1 && /(auto|scroll)/.test(getComputedStyle(n).overflowY)) return true;
    }
    return false;
  }
  function wheel(e: WheelEvent) {
    const t = e.target as HTMLElement;
    if (t.closest('.ctrl, .head, .pop') || (!e.ctrlKey && !e.metaKey && scrollable(t))) return;
    e.preventDefault();
    if (e.ctrlKey || e.metaKey) {
      const r = canvas!.getBoundingClientRect();
      const px = e.clientX - r.left, py = e.clientY - r.top;
      const z = Math.min(2, Math.max(0.3, view.z * Math.exp(-e.deltaY * 0.01)));
      setView({ x: px - ((px - view.x) * z) / view.z, y: py - ((py - view.y) * z) / view.z, z });
    } else {
      setView({ ...view, x: view.x - e.deltaX, y: view.y - e.deltaY });
    }
  }
  function zoomBy(f: number) {
    const r = canvas!.getBoundingClientRect();
    const px = r.width / 2, py = r.height / 2;
    const z = Math.min(2, Math.max(0.3, view.z * f));
    setView({ x: px - ((px - view.x) * z) / view.z, y: py - ((py - view.y) * z) / view.z, z });
  }
  function fit() {
    if (!canvas) return;
    const r = canvas.getBoundingClientRect();
    const left = 24;
    // Sin notas, el origen (donde vive el mensaje de bienvenida) va al centro.
    if (!shown.length) { setView({ x: r.width / 2, y: r.height / 2, z: 1 }); return; }
    const boxes = shown.map((n) => ({ x: n.x, y: n.y, w: n.w || W0, h: els[n.id]?.offsetHeight || 220 }));
    const x0 = Math.min(...boxes.map((b) => b.x)), x1 = Math.max(...boxes.map((b) => b.x + b.w));
    const y0 = Math.min(...boxes.map((b) => b.y)), y1 = Math.max(...boxes.map((b) => b.y + b.h));
    const aw = r.width - left - 40, ah = r.height - 200;
    const z = Math.min(1.2, Math.max(0.3, Math.min(aw / Math.max(1, x1 - x0), ah / Math.max(1, y1 - y0))));
    setView({ z, x: left + (aw - (x1 - x0) * z) / 2 - x0 * z, y: 120 - y0 * z });
  }

  // ---- gestos: un solo gesto a la vez ----
  type Axis = 'x' | 'y' | 'xy';
  const AXIS_CURSOR: Record<Axis, string> = { x: 'ew-resize', y: 'ns-resize', xy: 'nwse-resize' };
  type Origin = { id: string; ox: number; oy: number };
  type Gesture =
    | { kind: 'pan'; sx: number; sy: number; vx: number; vy: number; moved: boolean; close: boolean }
    | { kind: 'marquee'; sx: number; sy: number; base: string[]; moved: boolean }
    | { kind: 'note'; id: string; sx: number; sy: number; moved: boolean; open: boolean; toggle: boolean; group: Origin[] }
    | { kind: 'size'; id: string; axis: Axis; sx: number; sy: number; ow: number; oh: number };
  let gesture = $state<Gesture | null>(null);
  let drag = $state<{ id: string; x: number; y: number; w?: number; h?: number } | null>(null);
  /** Dónde se dibujan, mientras se arrastran, la nota o la selección entera. */
  let moving = $state<Record<string, { x: number; y: number }>>({});
  let marquee = $state<{ x: number; y: number; w: number; h: number } | null>(null);
  const at = (n: PersonalNote) => moving[n.id] ?? (drag?.id === n.id ? { x: drag.x, y: drag.y } : { x: n.x, y: n.y });
  const widthOf = (n: PersonalNote) => (editing === n.id ? Math.max(n.w || W0, EDIT_W) : drag?.id === n.id && drag.w ? drag.w : n.w || W0);
  /** Escribiendo, la nota crece con el editor; si no, el alto elegido o el de su texto. */
  const heightOf = (n: PersonalNote) => (editing === n.id ? 0 : drag?.id === n.id && drag.h !== undefined ? drag.h : n.h || 0);
  /** Sin palabras (vacía, o sólo una casilla vacía) se lee como vacía. */
  const blank = (n: PersonalNote) => !n.body.replace(/^\s*[-*+]\s*(\[[ xX]?\])?\s*$/gm, '').trim();

  // ---- selección: para mover y alinear varias a la vez ----
  // Shift/⌘/Ctrl + clic suma o quita una nota; Shift + arrastre en el fondo
  // traza un marco. Arrastrar una seleccionada mueve todas. Es de esta sesión:
  // no se guarda.
  let selected = $state<string[]>([]);
  const isSel = (id: string) => selected.includes(id);
  const picked = $derived(notes.filter((n) => selected.includes(n.id)));
  const boxOf = (n: PersonalNote) => ({ n, x: n.x, y: n.y, w: widthOf(n), h: els[n.id]?.offsetHeight || n.h || 200 });

  type Align = 'left' | 'hcenter' | 'right' | 'top' | 'vmiddle' | 'bottom' | 'hdist' | 'vdist';
  const ALIGN: { k: Align; label: string; min: number }[] = [
    { k: 'left', label: 'Alinear a la izquierda', min: 2 },
    { k: 'hcenter', label: 'Centrar en horizontal', min: 2 },
    { k: 'right', label: 'Alinear a la derecha', min: 2 },
    { k: 'top', label: 'Alinear arriba', min: 2 },
    { k: 'vmiddle', label: 'Centrar en vertical', min: 2 },
    { k: 'bottom', label: 'Alinear abajo', min: 2 },
    { k: 'hdist', label: 'Repartir en horizontal', min: 3 },
    { k: 'vdist', label: 'Repartir en vertical', min: 3 },
  ];
  /** Alinea contra la caja que envuelve la selección; repartir deja huecos iguales
   *  entre los extremos, que no se mueven. */
  function align(kind: Align) {
    const bs = picked.map(boxOf);
    if (bs.length < 2 || (kind.endsWith('dist') && bs.length < 3)) return;
    const x0 = Math.min(...bs.map((b) => b.x)), x1 = Math.max(...bs.map((b) => b.x + b.w));
    const y0 = Math.min(...bs.map((b) => b.y)), y1 = Math.max(...bs.map((b) => b.y + b.h));
    const to = new Map<string, { x: number; y: number }>();
    const spread = (axis: 'x' | 'y') => {
      const len = axis === 'x' ? 'w' : 'h';
      const sorted = [...bs].sort((a, b) => a[axis] - b[axis]);
      const gap = ((axis === 'x' ? x1 - x0 : y1 - y0) - sorted.reduce((t, b) => t + b[len], 0)) / (sorted.length - 1);
      let cur = axis === 'x' ? x0 : y0;
      for (const b of sorted) {
        to.set(b.n.id, axis === 'x' ? { x: Math.round(cur), y: b.y } : { x: b.x, y: Math.round(cur) });
        cur += b[len] + gap;
      }
    };
    if (kind === 'hdist') spread('x');
    else if (kind === 'vdist') spread('y');
    else for (const b of bs) {
      const x = kind === 'left' ? x0 : kind === 'right' ? x1 - b.w : kind === 'hcenter' ? (x0 + x1) / 2 - b.w / 2 : b.x;
      const y = kind === 'top' ? y0 : kind === 'bottom' ? y1 - b.h : kind === 'vmiddle' ? (y0 + y1) / 2 - b.h / 2 : b.y;
      to.set(b.n.id, { x: Math.round(x), y: Math.round(y) });
    }
    for (const b of bs) {
      const t = to.get(b.n.id);
      if (t && (t.x !== b.x || t.y !== b.y)) void patch(b.n, t);
    }
  }
  function spaceKey(e: KeyboardEvent) {
    if (doom || menuFor || editing) return;
    const t = e.target as HTMLElement;
    if (t.closest('input, textarea, [contenteditable], .cm-editor')) return;
    if (e.key === 'Escape' && selected.length) { selected = []; return; }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'a' && notes.length) { e.preventDefault(); selected = notes.map((n) => n.id); }
  }

  /** Marca la caja cuando su texto no cabe: ahí se corta con «…». */
  function clips(el: HTMLElement) {
    const check = () => el.classList.toggle('clipped', el.scrollHeight > el.clientHeight + 2);
    const ro = new ResizeObserver(check);
    ro.observe(el);
    const mo = new MutationObserver(check);
    mo.observe(el, { childList: true, subtree: true, characterData: true });
    check();
    return { destroy() { ro.disconnect(); mo.disconnect(); } };
  }

  function down(e: PointerEvent) {
    if (e.button !== 0) return;
    const el = e.target as HTMLElement;
    if (menuFor && !el.closest('.pop, .tool')) menuFor = '';
    const outside = !!editing && !el.closest(`[data-note="${editing}"]`);
    // El editor, los campos y los botones son de quien los toca; todo lo demás
    // —también el marco de la nota que se edita— arrastra.
    if (el.closest('button, a, input, select, textarea, label, .ctrl, .head, .pop, .editor, .edge')) {
      if (outside && !el.closest('.ctrl, .head')) void finishEdit();
      return;
    }
    const noteEl = el.closest<HTMLElement>('[data-note]');
    e.preventDefault();
    window.getSelection()?.removeAllRanges();
    canvas!.setPointerCapture(e.pointerId);
    if (noteEl) {
      const n = notes.find((x) => x.id === noteEl.dataset.note)!;
      raise(n.id);
      const toggle = e.shiftKey || e.metaKey || e.ctrlKey;
      const ids = isSel(n.id) ? selected : toggle ? [...selected, n.id] : [n.id];
      const group = notes.filter((x) => ids.includes(x.id)).map((x) => ({ id: x.id, ox: x.x, oy: x.y }));
      gesture = { kind: 'note', id: n.id, sx: e.clientX, sy: e.clientY, moved: false, open: !toggle && !!el.closest('.body'), toggle, group };
    } else if (e.shiftKey) {
      gesture = { kind: 'marquee', sx: e.clientX, sy: e.clientY, base: [...selected], moved: false };
    } else {
      // Mover el lienzo no cierra el editor; un clic quieto en el fondo, sí.
      gesture = { kind: 'pan', sx: e.clientX, sy: e.clientY, vx: view.x, vy: view.y, moved: false, close: !!editing };
    }
  }
  // Doble toque en el tirador: el alto vuelve a ser el de su texto. Se detecta
  // aquí y no con `dblclick`: la captura del puntero se lleva ese evento al lienzo.
  let lastSize = { id: '', t: 0 };
  function startSize(e: PointerEvent, n: PersonalNote, axis: Axis) {
    if (e.button !== 0) return;
    e.stopPropagation(); e.preventDefault();
    const now = performance.now();
    if (lastSize.id === n.id && now - lastSize.t < 400) {
      lastSize = { id: '', t: 0 };
      if (n.h && axis !== 'x') void patch(n, { h: 0 });
      return;
    }
    lastSize = { id: n.id, t: now };
    canvas!.setPointerCapture(e.pointerId);
    raise(n.id);
    gesture = { kind: 'size', id: n.id, axis, sx: e.clientX, sy: e.clientY, ow: n.w || W0, oh: n.h || els[n.id]?.offsetHeight || 220 };
  }
  function move(e: PointerEvent) {
    const g = gesture;
    if (!g) return;
    if (g.kind === 'size') {
      const n = notes.find((x) => x.id === g.id)!;
      const w = g.axis === 'y' ? n.w || W0 : Math.round(Math.min(WMAX, Math.max(WMIN, g.ow + (e.clientX - g.sx) / view.z)));
      const h = g.axis === 'x' ? n.h : Math.round(Math.min(HMAX, Math.max(HMIN, g.oh + (e.clientY - g.sy) / view.z)));
      drag = { id: g.id, x: n.x, y: n.y, w, h };
      return;
    }
    if (!g.moved && Math.hypot(e.clientX - g.sx, e.clientY - g.sy) < 4) return;
    g.moved = true;
    if (g.kind === 'pan') setView({ ...view, x: g.vx + e.clientX - g.sx, y: g.vy + e.clientY - g.sy });
    else if (g.kind === 'marquee') {
      const c = canvas!.getBoundingClientRect();
      const r = { x: Math.min(g.sx, e.clientX) - c.left, y: Math.min(g.sy, e.clientY) - c.top, w: Math.abs(e.clientX - g.sx), h: Math.abs(e.clientY - g.sy) };
      marquee = r;
      // El marco, en coordenadas del mundo: toca una nota si se cruzan.
      const wx0 = (r.x - view.x) / view.z, wy0 = (r.y - view.y) / view.z, wx1 = wx0 + r.w / view.z, wy1 = wy0 + r.h / view.z;
      const hits = notes.map(boxOf).filter((b) => b.x < wx1 && b.x + b.w > wx0 && b.y < wy1 && b.y + b.h > wy0).map((b) => b.n.id);
      selected = [...new Set([...g.base, ...hits])];
    } else {
      const dx = (e.clientX - g.sx) / view.z, dy = (e.clientY - g.sy) / view.z;
      moving = Object.fromEntries(g.group.map((o) => [o.id, { x: Math.round(o.ox + dx), y: Math.round(o.oy + dy) }]));
    }
  }
  function up() {
    const g = gesture, d = drag, mv = moving;
    gesture = null;
    drag = null;
    moving = {};
    marquee = null;
    if (!g) return;
    if (g.kind === 'pan') {
      // Un clic quieto en el fondo cierra el editor y suelta la selección.
      if (!g.moved) { if (g.close) void finishEdit(); selected = []; }
      return;
    }
    if (g.kind === 'marquee') return;
    const n = notes.find((x) => x.id === g.id);
    if (!n) return;
    if (g.kind === 'size') {
      if (!d) return;
      const out: Partial<PersonalNote> = {};
      if (d.w && d.w !== n.w) out.w = d.w;
      if (d.h !== undefined && d.h !== n.h) out.h = d.h;
      if (Object.keys(out).length) void patch(n, out);
      return;
    }
    if (g.moved) {
      for (const o of g.group) {
        const to = mv[o.id], m = notes.find((x) => x.id === o.id);
        if (to && m && (to.x !== m.x || to.y !== m.y)) void patch(m, to);
      }
      selected = g.group.length > 1 ? g.group.map((o) => o.id) : g.toggle ? [g.id] : isSel(g.id) ? selected : [];
      return;
    }
    if (g.toggle) { selected = isSel(n.id) ? selected.filter((x) => x !== n.id) : [...selected, n.id]; return; }
    selected = [];
    if (editing === n.id) { /* un clic en el marco de la nota que se edita: sigue editando */ }
    else if (g.open) startEdit(n);
    else if (editing) void finishEdit();
  }
  function cancel() { gesture = null; drag = null; moving = {}; marquee = null; }

  /** Moverla con las flechas: el mismo resultado que arrastrar, sin ratón. */
  let nudgeTimer: ReturnType<typeof setTimeout> | undefined;
  function noteKey(e: KeyboardEvent, n: PersonalNote) {
    if (e.target !== e.currentTarget) return;
    if (e.key === 'Enter') { e.preventDefault(); startEdit(n); return; }
    const step = e.shiftKey ? 80 : 16;
    if (e.altKey && e.key.startsWith('Arrow')) {
      e.preventDefault();
      const cur = { w: n.w || W0, h: n.h || els[n.id]?.offsetHeight || 220 };
      const dw = e.key === 'ArrowRight' ? step : e.key === 'ArrowLeft' ? -step : 0;
      const dh = e.key === 'ArrowDown' ? step : e.key === 'ArrowUp' ? -step : 0;
      const w = Math.min(WMAX, Math.max(WMIN, cur.w + dw)), h = Math.min(HMAX, Math.max(HMIN, cur.h + dh));
      notes = notes.map((x) => (x.id === n.id ? { ...x, w, h } : x));
      clearTimeout(nudgeTimer);
      nudgeTimer = setTimeout(() => { const now = notes.find((x) => x.id === n.id); if (now) void patch(now, { w: now.w, h: now.h }); }, 400);
      return;
    }
    const d = ({ ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] } as Record<string, number[]>)[e.key];
    if (!d) return;
    e.preventDefault();
    raise(n.id);
    const ids = isSel(n.id) ? [...selected] : [n.id];
    notes = notes.map((x) => (ids.includes(x.id) ? { ...x, x: x.x + d[0], y: x.y + d[1] } : x));
    clearTimeout(nudgeTimer);
    nudgeTimer = setTimeout(() => {
      for (const now of notes.filter((x) => ids.includes(x.id))) void patch(now, { x: now.x, y: now.y });
    }, 400);
  }

  // ---- notas ----
  let editing = $state('');
  let mdEditing = $state(false);
  let draft = $state('');
  async function add() {
    if (busy || !canvas) return;
    busy = 'new';
    const r = canvas.getBoundingClientRect();
    const c = toWorld(r.left + r.width / 2, r.top + r.height / 2);
    const k = notes.length % 5;
    try {
      const n = await api.create<PersonalNote>('personal_notes', {
        owner, title: 'Nueva nota', body: '- [ ] ', color: NEW_COLORS[notes.length % NEW_COLORS.length],
        workspace: '', x: Math.round(c.x - W0 / 2 + k * 24), y: Math.round(c.y - 110 + k * 24), w: W0,
      });
      if (stopped) return;
      const made = norm(n); notes = [made, ...notes]; raise(made.id); startEdit(made);
    } catch (e) { error = (e as Error).message; } finally { busy = ''; }
  }
  function startEdit(n: PersonalNote) {
    if (editing === n.id) return;
    if (editing) void finishEdit();
    raise(n.id); editing = n.id; draft = n.body; mdEditing = true;
  }
  async function finishEdit() {
    const id = editing, body = draft;
    if (!id) return;
    editing = ''; mdEditing = false;
    const n = notes.find((x) => x.id === id);
    if (n && body !== n.body) await patch(n, { body });
  }
  $effect(() => { if (editing && !mdEditing) void finishEdit(); });
  function tickTodo(n: PersonalNote, index: number, done: boolean) {
    const body = toggleMarkdownTodo(n.body, index, done);
    if (body !== n.body) return patch(n, { body });
  }
  /** Color: un desplegable, desde su botón o con clic derecho en la nota. */
  async function openMenu(kind: 'c' | 'p', n: PersonalNote) {
    const id = `${kind}:${n.id}`;
    menuFor = menuFor === id ? '' : id;
    if (!menuFor) return;
    raise(n.id);
    // Se dibuja invisible hacia abajo, se mide en pantalla (ya con el zoom) y se
    // abre hacia donde cabe: abajo si entra, si no hacia el lado con más sitio.
    menuUp = false; menuEnd = false; measuring = true;
    await tick();
    const m = els[n.id]?.querySelector<HTMLElement>('.pop');
    const note = els[n.id], c = canvas?.getBoundingClientRect();
    if (m && note && c) {
      const r = m.getBoundingClientRect(), nr = note.getBoundingClientRect();
      const foot = note.querySelector<HTMLElement>('.tools')?.getBoundingClientRect().top ?? nr.bottom;
      const below = c.bottom - nr.bottom, above = foot - c.top;
      menuUp = r.height + 12 > below && above > below;
      menuEnd = r.right > c.right - 8 && nr.right - r.width > c.left;
    }
    measuring = false;
    await tick();
    (m?.querySelector<HTMLElement>('[aria-checked="true"]') ?? m?.querySelector<HTMLElement>('button'))?.focus();
  }
  let menuUp = $state(false), menuEnd = $state(false), measuring = $state(false);
  // Una nota nueva va rotando entre los seis de siempre, no por los 24.
  const NEW_COLORS = ['#fff0b3', '#bcecf5', '#fbd1df', '#bcebd9', '#dfd3ff', '#ffdfc4'];
  function closeMenu(n: PersonalNote) {
    const kind = menuFor.slice(0, 1);
    menuFor = '';
    els[n.id]?.querySelector<HTMLElement>(kind === 'c' ? '.tool.color' : '.tool.project')?.focus();
  }
  function menuKey(e: KeyboardEvent, n: PersonalNote) {
    const items = [...(e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('button')];
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeMenu(n); }
    else if (e.key.startsWith('Arrow')) {
      // La paleta es una rejilla de 3: ←→ de a uno, ↑↓ por fila.
      e.preventDefault();
      const cols = (e.currentTarget as HTMLElement).classList.contains('colors') ? 4 : 1;
      const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols }[e.key] ?? 0;
      items[(Math.max(0, i) + step + items.length) % items.length]?.focus();
    } else if (e.key === 'Home' || e.key === 'End') { e.preventDefault(); items[e.key === 'Home' ? 0 : items.length - 1]?.focus(); }
    else if (e.key === 'Tab') closeMenu(n);
  }
  function pickColor(n: PersonalNote, c: string) {
    closeMenu(n);
    if (c !== safeColor(n.color)) void patch(n, { color: c });
  }
  function pickProject(n: PersonalNote, w: string) {
    closeMenu(n);
    if (w !== n.workspace) void patch(n, { workspace: w });
  }
  function noteMenu(e: MouseEvent, n: PersonalNote) {
    // Donde se escribe, el menú nativo (ortografía, copiar, enlaces) manda.
    if ((e.target as HTMLElement).closest('input, textarea, a, .cm-editor, .note.editing, .pop')) return;
    e.preventDefault();
    if (menuFor !== `c:${n.id}`) void openMenu('c', n);
  }
  function retitle(n: PersonalNote, el: HTMLInputElement) {
    const t = el.value.trim().slice(0, 150);
    if (t && t !== n.title) void patch(n, { title: t });
    else el.value = n.title;
  }

  function remove(n: PersonalNote) {
    doom = {
      title: `¿Borrar «${n.title}»?`, crumbs: ['Mi espacio'],
      body: `Se borra la nota${filesOf(n.id).length ? ' y sus archivos' : ''}, sin recuperación.`,
      go: async () => {
        try {
          await api.remove('personal_notes', n.id);
          if (stopped) return;
          if (editing === n.id) { editing = ''; mdEditing = false; }
          notes = notes.filter((x) => x.id !== n.id); assets = assets.filter((a) => a.note !== n.id); selected = selected.filter((x) => x !== n.id);
        } catch (e) { error = (e as Error).message; }
      },
    };
  }
  function removeAsset(a: PersonalAsset) {
    doom = {
      title: `¿Borrar «${a.name}»?`, crumbs: ['Mi espacio'], body: 'Se borra el archivo, sin recuperación.',
      go: async () => {
        try { await api.remove('personal_assets', a.id); if (!stopped) assets = assets.filter((x) => x.id !== a.id); }
        catch (e) { error = (e as Error).message; }
      },
    };
  }

  async function upload(files: FileList | File[] | null | undefined, note = '', workspace = '') {
    if (!files?.length) return;
    busy = note || 'files';
    try {
      for (const f of Array.from(files)) {
        const a = await api.uploadPersonal(f, workspace, note);
        if (stopped) return;
        assets = [a, ...assets];
      }
    } catch (e) { if (!stopped) error = (e as Error).message || 'No se pudo subir'; }
    finally { if (!stopped) busy = ''; }
  }
  let dropping = $state(false);
  const hasFiles = (e: DragEvent) => !!e.dataTransfer?.types.includes('Files');
  function dropOnCanvas(e: DragEvent) {
    if (!hasFiles(e)) return;
    e.preventDefault();
    dropping = false;
    const noteEl = (e.target as HTMLElement).closest<HTMLElement>('[data-note]');
    const n = noteEl ? notes.find((x) => x.id === noteEl.dataset.note) : undefined;
    // Sólo sobre una nota: un archivo suelto no tendría dónde verse.
    if (!n) { error = 'Suelta el archivo sobre una nota para adjuntarlo.'; return; }
    void upload(e.dataTransfer?.files, n.id, n.workspace);
  }
  const isImage = (a: PersonalAsset) => /\.(png|jpe?g|gif|webp|avif|svg)$/i.test(a.file);
</script>

<div class="space">
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="canvas"
    class:panning={gesture?.kind === 'pan'}
    class:dropping
    data-cursor={gesture && (gesture.kind === 'pan' || gesture.kind === 'note') ? 'grabbing' : gesture?.kind === 'marquee' ? null : 'grab'}
    data-cursor-lock={gesture?.kind === 'size' ? AXIS_CURSOR[gesture.axis] : null}
    bind:this={canvas}
    style:--gx="{view.x}px"
    style:--gy="{view.y}px"
    style:--gs="{24 * view.z}px"
    onpointerdown={down}
    onpointermove={move}
    onpointerup={up}
    onpointercancel={cancel}
    onwheel={wheel}
    ondragover={(e) => { if (hasFiles(e)) { e.preventDefault(); dropping = true; } }}
    ondragleave={(e) => { if (e.target === canvas) dropping = false; }}
    ondrop={dropOnCanvas}>

    <!-- Arriba: los controles en el borde izquierdo y el badge al centro; la
         fila de alinear, debajo de los controles. -->
    <div class="topbar">
      <div class="ctrl">
        <button onclick={add} class="add" disabled={!loaded || !!busy} title="Nueva nota" aria-label="Nueva nota">
          {#if busy === 'new'}<Loading compact label="Creando nota" />{:else}+ Nota{/if}
        </button>
        <span class="sep" aria-hidden="true"></span>
        <button onclick={() => zoomBy(1 / 1.2)} title="Alejar" aria-label="Alejar">−</button>
        <span class="zoom">{Math.round(view.z * 100)}%</span>
        <button onclick={() => zoomBy(1.2)} title="Acercar" aria-label="Acercar">+</button>
        <button onclick={fit} title="Ver todo" aria-label="Ver todo">⤢</button>
      </div>
      <header class="head">
        <span class="pane-name display"><Icon of="pin" tint /> Mi espacio</span>
        <span class="count" title="notas">{notes.length}</span>
        {#if saving}<Loading compact label="Guardando" />{/if}
      </header>
      {#if selected.length >= 2}
        <!-- Segunda fila del HUD, debajo de los controles: sólo con dos o más. -->
        <div class="ctrl align" role="toolbar" aria-label="Alinear notas seleccionadas">
          <span class="zoom" title="Notas seleccionadas">{selected.length} sel.</span>
          <span class="sep" aria-hidden="true"></span>
          {#each ALIGN as a, i (a.k)}
            {#if i === 3 || i === 6}<span class="sep" aria-hidden="true"></span>{/if}
            <button onclick={() => align(a.k)} title={a.min > 2 ? `${a.label} (3 o más)` : a.label} aria-label={a.label} disabled={selected.length < a.min}>
              {@render glyph(a.k)}
            </button>
          {/each}
          <span class="sep" aria-hidden="true"></span>
          <button onclick={() => (selected = [])} title="Quitar selección (Esc)" aria-label="Quitar selección">×</button>
        </div>
      {/if}
    </div>

    {#if !loaded}
      <Loading label="Cargando tu espacio" />
    {:else}
      <div class="world" style:transform="translate({view.x}px, {view.y}px) scale({view.z})">
        {#each shown as n (n.id)}
          {@const p = at(n)}
          {@const tint = safeColor(n.color) || '#fff0b3'}
          {@const pc = n.workspace ? projectColors.get(n.workspace) : ''}
          {@const files = filesOf(n.id)}
          <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
          <article
            class="note"
            class:editing={editing === n.id}
            class:moving={!!moving[n.id]}
            class:selected={isSel(n.id)}
            data-note={n.id}
            use:placed={n.id}
            tabindex="0"
            aria-label={`${isSel(n.id) ? 'Seleccionada. ' : ''}Nota ${n.title}. Flechas para mover, Alt+flechas para cambiar el tamaño, Enter para escribir, Shift+clic para seleccionar.`}
            onkeydown={(e) => noteKey(e, n)}
            oncontextmenu={(e) => noteMenu(e, n)}
            style:left="{p.x}px"
            style:top="{p.y}px"
            style:width="{widthOf(n)}px"
            style:height={heightOf(n) ? `${heightOf(n)}px` : null}
            class:sized={!!heightOf(n)}
            style:z-index={editing === n.id ? 9999 : zOf(n.id)}
            style:--paper={tint}
            style:--band={pc || 'transparent'}>
            <div class="bar">
              <span class="grip" aria-hidden="true">⠿</span>
              <input class="title" value={n.title} maxlength="150" aria-label="Título de la nota"
                onchange={(e) => retitle(n, e.currentTarget)}
                onkeydown={(e) => { if (e.key === 'Enter') e.currentTarget.blur(); }} />
            </div>
            {#if n.workspace}
              <span class="ptag" title="Proyecto"><span class="dot" style:background={pc || 'var(--muted)'}></span>{projectName(n.workspace) || 'Proyecto'}</span>
            {/if}

            {#if editing === n.id}
              <div class="editor">
                <div class="editor-surface">
                  <MarkdownField bind:value={draft} bind:editing={mdEditing} chrome={false} minHeight="18rem" render={(md) => api.renderMarkdown(md)} />
                </div>
                <div class="editor-actions">
                  <button class="vim" class:on={vimPref.on} aria-pressed={vimPref.on} title="Teclas de vim (:w guarda, :q cierra)"
                    onclick={() => vimPref.toggle()}>vim</button>
                  <button class="done" onclick={() => finishEdit()}>Listo</button>
                </div>
              </div>
            {:else}
              <div class="body" data-cursor="text" title="Clic para escribir" use:clips>
                {#if html[n.id] && !blank(n)}<Prose compact html={html[n.id]} ontodo={(i, d) => tickTodo(n, i, d)} />{:else}<p class="empty-note">Clic para escribir…</p>{/if}
              </div>
            {/if}

            {#if files.length}
              <ul class="files">
                {#each files as a (a.id)}
                  <li>
                    <a href={api.personalFile(a, token)} target="_blank" rel="noopener" title={a.name}>
                      {#if isImage(a) && token}<img src={api.personalFile(a, token)} alt={a.name} />{:else}<Icon of="document" />{/if}
                      <span>{a.name}</span>
                    </a>
                    <button class="x" aria-label={`Borrar ${a.name}`} onclick={() => removeAsset(a)}>×</button>
                  </li>
                {/each}
              </ul>
            {/if}

            <footer class="tools">
              <button class="tool color" aria-label="Color de la nota" title="Color · también con clic derecho en la nota"
                aria-haspopup="menu" aria-expanded={menuFor === 'c:' + n.id}
                onclick={() => openMenu('c', n)}><span class="swatch-icon" style:background={tint} aria-hidden="true"></span></button>
              <button class="tool project" aria-label="Proyecto de la nota" title="Proyecto" aria-haspopup="menu" aria-expanded={menuFor === 'p:' + n.id}
                onclick={() => openMenu('p', n)}><Icon of="board" /></button>
              <label class="tool" title="Adjuntar archivo">
                {#if busy === n.id}<Loading compact label="Subiendo" />{:else}<Icon of="paperclip" />{/if}
                <input type="file" multiple hidden aria-label={`Adjuntar a ${n.title}`} onchange={(e) => upload(e.currentTarget.files, n.id, n.workspace)} />
              </label>
              <button class="tool trash" aria-label={`Borrar nota ${n.title}`} title="Borrar nota" onclick={() => remove(n)}><Icon of="trash" danger /></button>
            </footer>
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <!-- Los bordes: derecho (ancho), inferior (alto) y la esquina (los dos).
                 Doble toque en el inferior o la esquina: alto según el texto. -->
            <span class="edge e" data-cursor="ew-resize" title="Cambiar ancho" aria-hidden="true" onpointerdown={(e) => startSize(e, n, 'x')}></span>
            <span class="edge s" data-cursor="ns-resize" title="Cambiar alto · doble clic: según el texto" aria-hidden="true" onpointerdown={(e) => startSize(e, n, 'y')}></span>
            <span class="edge se resize" data-cursor="nwse-resize" title="Cambiar tamaño · doble clic: alto según el texto" aria-hidden="true" onpointerdown={(e) => startSize(e, n, 'xy')}></span>

            {#if menuFor === 'c:' + n.id}
              <!-- svelte-ignore a11y_interactive_supports_focus -->
              <div class="pop menu colors" class:up={menuUp} class:end={menuEnd} class:measuring role="menu" aria-label="Color de la nota" onkeydown={(e) => menuKey(e, n)}>
                <!-- Cuatro por fila, cada muestra con su nombre en la letra display. -->
                <div class="grid">
                  {#each NOTE_COLORS as c, i}
                    <button role="menuitemradio" aria-checked={tint === c} aria-label={NOTE_COLOR_NAMES[i]} onclick={() => pickColor(n, c)}>
                      <span class="sw" style:background={c} aria-hidden="true">{#if tint === c}<span class="check">✓</span>{/if}</span>
                      <span class="cname display" aria-hidden="true">{NOTE_COLOR_NAMES[i]}</span>
                    </button>
                  {/each}
     {#if marquee}
      <div class="marquee" aria-hidden="true" style:left="{marquee.x}px" style:top="{marquee.y}px" style:width="{marquee.w}px" style:height="{marquee.h}px"></div>
    {/if}
               </div>
              </div>
            {:else if menuFor === 'p:' + n.id}
              <!-- svelte-ignore a11y_interactive_supports_focus -->
              <div class="pop menu" class:up={menuUp} class:end={menuEnd} class:measuring role="menu" aria-label="Proyecto de la nota" onkeydown={(e) => menuKey(e, n)}>
                <button role="menuitemradio" aria-checked={!n.workspace} onclick={() => pickProject(n, '')}>
                  <span class="dot none" aria-hidden="true"></span><span class="label">Sin proyecto</span>
                  {#if !n.workspace}<span class="check" aria-hidden="true">✓</span>{/if}
                </button>
                {#each workspaces as w (w.id)}
                  <button role="menuitemradio" aria-checked={n.workspace === w.id} title={w.name} onclick={() => pickProject(n, w.id)}>
                    <span class="dot" style:background={projectColors.get(w.id) || 'var(--line)'} aria-hidden="true"></span>
                    <span class="label">{w.name}</span>
                    {#if n.workspace === w.id}<span class="check" aria-hidden="true">✓</span>{/if}
                  </button>
                {/each}
              </div>
            {/if}
          </article>
        {/each}
        {#if !notes.length}
          <!-- En el origen del lienzo, no pegado a la pantalla: se mueve con él. -->
          <div class="origin">
            <Icon of="pin" size="3.5rem" tint />
            <p>Aquí empieza tu espacio.<br />Pulsa «+ Nota» para pegar un post-it y arrástralo a donde quieras.</p>
          </div>
        {/if}
      </div>

    {/if}

    {#if error}<p class="oops" role="alert">{error} <button onclick={() => (error = '')}>cerrar</button></p>{/if}


  </div>
</div>

<svelte:window onkeydown={spaceKey} />
<Confirm bind:ask={doom} />

{#snippet glyph(k: Align)}
  <svg class="glyph" viewBox="0 0 16 16" aria-hidden="true">
    {#if k === 'left'}<path d="M2 1.5v13" /><rect x="4" y="3.5" width="9" height="3" rx="1" /><rect x="4" y="9.5" width="5.5" height="3" rx="1" />
    {:else if k === 'hcenter'}<path d="M8 1.5v13" /><rect x="3" y="3.5" width="10" height="3" rx="1" /><rect x="5" y="9.5" width="6" height="3" rx="1" />
    {:else if k === 'right'}<path d="M14 1.5v13" /><rect x="3" y="3.5" width="9" height="3" rx="1" /><rect x="6.5" y="9.5" width="5.5" height="3" rx="1" />
    {:else if k === 'top'}<path d="M1.5 2h13" /><rect x="3.5" y="4" width="3" height="9" rx="1" /><rect x="9.5" y="4" width="3" height="5.5" rx="1" />
    {:else if k === 'vmiddle'}<path d="M1.5 8h13" /><rect x="3.5" y="3" width="3" height="10" rx="1" /><rect x="9.5" y="5" width="3" height="6" rx="1" />
    {:else if k === 'bottom'}<path d="M1.5 14h13" /><rect x="3.5" y="3" width="3" height="9" rx="1" /><rect x="9.5" y="6.5" width="3" height="5.5" rx="1" />
    {:else if k === 'hdist'}<path d="M1.5 2v12M14.5 2v12" /><rect x="6.5" y="4" width="3" height="8" rx="1" />
    {:else}<path d="M2 1.5h12M2 14.5h12" /><rect x="4" y="6.5" width="8" height="3" rx="1" />{/if}
  </svg>
{/snippet}

<style>
  .space { height: 100%; min-height: 0; display: flex; flex-direction: column; }
  .canvas {
    position: relative; flex: 1; min-height: 0; overflow: hidden;
    background-color: color-mix(in oklab, var(--surface-solid) 55%, transparent);
    background-image: radial-gradient(circle, color-mix(in oklab, var(--text) 22%, transparent) 1.1px, transparent 1.4px);
    background-size: var(--gs) var(--gs); background-position: var(--gx) var(--gy);
    cursor: grab; touch-action: none; user-select: none; -webkit-user-select: none;
  }
  .canvas :is(input, :global(.cm-editor)) { user-select: text; -webkit-user-select: text; }
  .canvas.panning { cursor: grabbing; }
  .canvas.dropping { box-shadow: inset 0 0 0 3px color-mix(in oklab, var(--accent) 55%, transparent); }
  .world { position: absolute; left: 0; top: 0; transform-origin: 0 0; }

  .topbar {
    position: absolute; top: 1rem; left: 1rem; right: 1rem; z-index: 10001;
    display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 0.6rem;
    pointer-events: none;
  }
  .topbar > * { pointer-events: auto; }
  .topbar > .ctrl { grid-column: 1; justify-self: start; }
  .topbar > .head { grid-column: 2; grid-row: 1; }
  .topbar > .ctrl.align { grid-row: 2; }
  .head {
    position: relative;
    display: flex; align-items: center; gap: 0.5rem; min-height: 42px; padding: 0.5rem 0.9rem;
    border: 1px solid var(--line); border-radius: 999px; background: var(--surface-solid);
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.16); cursor: default; white-space: nowrap;
  }
  .pane-name { font-size: 1.5rem; display: inline-flex; align-items: center; gap: 0.35rem; }
  .count { padding: 0 0.45rem; border-radius: 999px; background: var(--hover); font-size: 0.75rem; }

  .ctrl {
    display: flex; align-items: center; gap: 0.2rem; padding: 0.2rem;
    border: 1px solid var(--line); border-radius: 999px; background: var(--surface-solid); font-size: 0.78rem;
    box-shadow: 0 4px 14px rgb(0 0 0 / 0.15); cursor: default;
  }
  .ctrl button { min-width: 26px; height: 26px; border: 0; border-radius: 999px; background: none; color: var(--text); cursor: pointer; }
  .ctrl button:hover { background: var(--hover); }
  .ctrl .add { padding: 0 0.75rem; background: var(--accent); color: white; font-weight: 700; }
  .ctrl .add:hover { background: color-mix(in oklab, var(--accent) 85%, black); }
  .ctrl .sep { width: 1px; height: 18px; background: var(--line); margin: 0 0.2rem; }
  .zoom { min-width: 2.8rem; text-align: center; color: var(--muted); }
  .ctrl button:disabled { opacity: 0.35; }
  .glyph { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.4; stroke-linecap: round; vertical-align: middle; }
  .glyph rect { fill: currentColor; fill-opacity: 0.28; }
  .marquee { position: absolute; z-index: 10000; pointer-events: none; border: 1.5px dashed var(--accent); border-radius: 6px;
    background: color-mix(in oklab, var(--accent) 12%, transparent); }

  .dot { flex: none; display: inline-block; width: 0.75rem; height: 0.75rem; border-radius: 999px; border: 1px solid rgb(0 0 0 / 0.15); }
  .dot.none { border: 1.5px dashed var(--muted); }
  .sw { flex: none; width: 1.1rem; height: 1.1rem; border-radius: 999px; border: 1px solid rgb(0 0 0 / 0.18); }
  .x { border: 0; color: var(--color-error-500); font-weight: 800; line-height: 1; }

  /* El post-it. */
  .note {
    position: absolute; display: flex; flex-direction: column; gap: 0.35rem; padding: 0.55rem 0.75rem 0.45rem;
    border-radius: 4px 4px 14px 4px; border-top: 7px solid var(--band); background: var(--paper); color: #1f2430;
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.12), 0 10px 24px rgb(0 0 0 / 0.16); cursor: grab; outline: none;
    transition: box-shadow 0.15s ease;
  }
  .note:focus-visible { box-shadow: 0 0 0 3px var(--accent), 0 10px 24px rgb(0 0 0 / 0.16); }
  .note:hover { box-shadow: 0 1px 2px rgb(0 0 0 / 0.12), 0 16px 32px rgb(0 0 0 / 0.22); }
  .note.moving { cursor: grabbing; box-shadow: 0 22px 44px rgb(0 0 0 / 0.3); transform: rotate(-1deg); }
  .note.selected { box-shadow: 0 0 0 2.5px var(--accent), 0 10px 24px rgb(0 0 0 / 0.18); }
  .note.editing { cursor: default; box-shadow: 0 0 0 2px var(--accent), 0 22px 44px rgb(0 0 0 / 0.28); }
  .bar { display: flex; align-items: center; gap: 0.35rem; }
  .grip { color: rgb(31 36 48 / 0.35); font-size: 0.9rem; cursor: grab; }
  .title { flex: 1; min-width: 0; text-overflow: ellipsis; border: 0; border-radius: 4px; padding: 0.1rem 0.2rem; background: transparent; color: inherit; font-weight: 800; text-transform: uppercase; letter-spacing: 0.02em; font-size: 0.9rem; cursor: text; }
  .title:focus { background: rgb(255 255 255 / 0.55); outline: none; }
  .ptag { align-self: flex-start; display: inline-flex; align-items: center; gap: 0.3rem; max-width: 100%; padding: 0.05rem 0.45rem; border-radius: 999px; background: rgb(255 255 255 / 0.55); font-size: 0.7rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  /* Sitio de sobra para el cursor de texto: que se vea que ahí se escribe. Lo
     que no cabe se corta con «…»; escribiendo, se ve entero. */
  .body { position: relative; min-height: 5rem; max-height: 22rem; overflow: hidden; border-radius: 6px; transition: background 0.15s ease; }
  .body:hover { background: rgb(255 255 255 / 0.28); }
  .body:global(.clipped)::after {
    content: '…'; position: absolute; left: 0; right: 0; bottom: 0; height: 2rem; display: flex; align-items: flex-end; justify-content: flex-end;
    padding: 0 0.35rem 0.1rem; background: linear-gradient(to bottom, transparent, var(--paper) 75%);
    color: rgb(31 36 48 / 0.75); font-weight: 800; font-size: 1.1rem; line-height: 1; pointer-events: none;
  }
  .body :global(.prose) { color: #1f2430; font-size: 14.5px; }
  .body :global(.prose > :first-child) { margin-top: 0; }
  .body :global(.prose > :last-child) { margin-bottom: 0; }
  .empty-note { color: rgb(31 36 48 / 0.5); font-size: 0.85rem; font-style: italic; }
  .editor { display: flex; flex-direction: column; gap: 0.35rem; cursor: auto; }
  /* El editor usa la superficie del tema, no el papel: en oscuro, el texto claro
     de CodeMirror sobre un pastel no se leería. */
  .editor :global(.cm-editor) { border-radius: 8px; }
  .editor :global(.cm-content) { font-size: 15px; }
  .editor-surface { padding: 0.3rem; border-radius: 10px; background: var(--surface-solid); color: var(--text); box-shadow: inset 0 0 0 1px var(--line); }
  .editor-actions { display: flex; align-items: center; justify-content: flex-end; gap: 0.4rem; }
  .vim { padding: 0.15rem 0.5rem; border: 1px solid rgb(31 36 48 / 0.25); border-radius: 7px; color: rgb(31 36 48 / 0.65);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.72rem; }
  .vim:hover { background: rgb(0 0 0 / 0.06); }
  .vim.on { color: #0b6f86; border-color: #0b6f86; background: rgb(255 255 255 / 0.55); font-weight: 700; }
  .done { padding: 0.2rem 0.75rem; border-radius: 8px; background: #1f2430; color: white; font-size: 0.8rem; font-weight: 700; }
  .files { display: flex; flex-direction: column; gap: 0.2rem; font-size: 0.75rem; }
  .files li { display: flex; align-items: center; gap: 0.25rem; }
  .files a { flex: 1; min-width: 0; display: flex; align-items: center; gap: 0.35rem; text-decoration: underline; }
  .files a span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .files img { width: 2rem; height: 2rem; object-fit: cover; border-radius: 4px; }
  .tools { display: flex; align-items: center; gap: 0.15rem; padding-top: 0.3rem; border-top: 1px solid rgb(0 0 0 / 0.08); opacity: 0; transition: opacity 0.15s ease; }
  .note:is(:hover, :focus-within, .editing) .tools { opacity: 1; }
  .tool { display: inline-grid; place-items: center; width: 1.8rem; height: 1.8rem; border-radius: 8px; cursor: pointer; }
  .swatch-icon { width: 1rem; height: 1rem; border-radius: 999px; border: 2px solid rgb(31 36 48 / 0.45); box-shadow: inset 0 0 0 2px rgb(255 255 255 / 0.6); }
  .tool:hover { background: rgb(0 0 0 / 0.07); }
  .tool.trash { margin-left: auto; }
  .edge { position: absolute; z-index: 1; touch-action: none; }
  .edge.e { top: 10px; right: -4px; bottom: 18px; width: 9px; }
  .edge.s { left: 10px; right: 18px; bottom: -4px; height: 9px; }
  .edge.se { right: -2px; bottom: -2px; width: 22px; height: 22px; border-radius: 0 0 14px 0;
    background: linear-gradient(135deg, transparent 58%, rgb(31 36 48 / 0.3) 58%); opacity: 0; transition: opacity 0.15s ease; }
  .note:is(:hover, :focus-within) .edge.se { opacity: 1; }
  /* Escribiendo, el alto lo pone el editor: sólo el ancho se cambia. */
  .note.editing .edge:is(.s, .se) { display: none; }
  /* Con alto elegido, el texto ocupa lo que quede y el pie queda abajo. */
  .note.sized .body { flex: 1; min-height: 1.5rem; max-height: none; }
  .note.sized .files { flex: none; max-height: 30%; overflow: hidden; }
  .note.sized .tools { margin-top: auto; }
  /* Desplegable de nota: superficie y bordes del tema, como cualquier menú.
     Pegado al pie (donde están sus botones): debajo de la nota, o encima del
     pie si abajo no cabe. */
  .note > .pop {
    position: absolute; left: 0.4rem; top: calc(100% + 6px); z-index: 2; width: 14rem; max-height: 18rem; overflow-y: auto;
    display: flex; flex-direction: column; gap: 1px; padding: 0.3rem;
    border: 1px solid var(--line); border-radius: 12px; background: var(--surface-solid); color: var(--text);
    box-shadow: 0 14px 34px rgb(0 0 0 / 0.28); cursor: default; animation: drop 120ms ease-out;
  }
  .note > .pop.up { top: auto; bottom: 2.9rem; animation-name: rise; }
  .note > .pop.end { left: auto; right: 0.4rem; }
  .note > .pop.measuring { visibility: hidden; animation: none; }
  @keyframes drop { from { opacity: 0; transform: translateY(-4px); } }
  @keyframes rise { from { opacity: 0; transform: translateY(4px); } }
  .menu button { display: flex; align-items: center; gap: 0.55rem; width: 100%; padding: 0.38rem 0.5rem; border-radius: 8px;
    color: var(--text); font-size: 0.84rem; text-align: left; }
  .menu button:hover, .menu button:focus-visible { background: var(--hover); outline: none; }
  .menu button[aria-checked='true'] { background: color-mix(in oklab, var(--accent) 14%, transparent); font-weight: 700; }
  .menu .label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .menu .check { color: var(--accent); font-weight: 800; }
  /* Ancho propio: si se ajustara a la nota, en una angosta se encogía a una
     columna con scroll. Seis filas de cuatro caben sin scroll. */
  .note > .pop.menu.colors { width: 21rem; max-height: none; overflow: visible; padding: 0.5rem; }
  .menu.colors .grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 0.2rem; }
  .menu.colors button { flex-direction: column; align-items: center !important; gap: 0.25rem; padding: 0.45rem 0.2rem 0.35rem; border-radius: 10px; text-align: center; }
  .menu.colors .sw { width: 1.9rem; height: 1.9rem; display: grid; place-items: center; }
  .menu.colors .cname { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 1.05rem; line-height: 1.1; color: var(--text); }
  .menu.colors button[aria-checked='true'] { font-weight: 400; }
  .menu.colors .check { color: #1f2430; font-size: 0.9rem; }
  .menu.colors button[aria-checked='true'] .sw { outline: 2px solid var(--accent); outline-offset: 2px; }

  .origin { position: absolute; left: 0; top: 0; width: 22rem; transform: translate(-50%, -50%); display: grid; justify-items: center;
    gap: 0.75rem; text-align: center; color: var(--muted); pointer-events: none; }
  .origin p { line-height: 1.5; }
  .oops { position: absolute; bottom: 6rem; left: 50%; transform: translateX(-50%); z-index: 10002; padding: 0.5rem 0.8rem; border-radius: 10px; background: var(--surface-solid); color: var(--color-error-500); box-shadow: 0 6px 18px rgb(0 0 0 / 0.2); font-size: 0.85rem; }
  .oops button { margin-left: 0.5rem; text-decoration: underline; }

  @media (max-width: 650px) {
    /* La presencia es dueña de la esquina superior derecha: el título va a su
       izquierda. */
    .topbar { display: contents; }
    .head { position: absolute; z-index: 10001; top: 1rem; left: 1rem; max-width: calc(100% - 6.5rem); }
    .pane-name { font-size: 1.15rem; }
    .ctrl { position: absolute; z-index: 10001; right: 1rem; bottom: 10.5rem; }
    .ctrl.align { bottom: 13.1rem; max-width: calc(100% - 2rem); overflow-x: auto; }
  }
  @media (prefers-reduced-motion: reduce) { .tools, .note { transition: none; } .note > .pop { animation: none; } .note.moving { transform: none; } }
</style>
