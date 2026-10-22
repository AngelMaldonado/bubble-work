// Qué dicen el tamaño y el color de una burbuja en la vista Blobs (2D y 3D), y
// qué se resalta. Lo elige cada persona; se guarda en su navegador.
//
// Puro, para fijarlo con pruebas. No inventa datos: si un punto no tiene lo que
// se mide (sin fecha, sin responsables), cae en el tamaño base y en el color
// «sin dato», y la leyenda lo dice.
import type { GraphLink, GraphNode } from './graph';

export type SizeBy = 'uniform' | 'heat' | 'priority' | 'links' | 'threads' | 'files' | 'text' | 'people' | 'due';
export type ColorBy = 'state' | 'priority' | 'project' | 'team' | 'tag' | 'due' | 'mine';
export type Preset = 'mine' | 'urgent' | 'ownerless' | 'due' | 'connected' | 'cold';
/** `size`: lo que decide el tamaño; varios se suman. `['uniform']` es «Igual». */
export type Encoding = { size: SizeBy[]; color: ColorBy; presets: Preset[]; text: string; mode: 'dim' | 'hide' };

export const DEFAULT_ENCODING: Encoding = { size: ['uniform'], color: 'state', presets: [], text: '', mode: 'dim' };

export const SIZE_LABELS: Record<SizeBy, string> = {
  uniform: 'Igual', heat: 'Calor', priority: 'Prioridad', links: 'Conexiones', threads: 'Cantidad de hilos', files: 'Archivos', text: 'Texto', people: 'Responsables', due: 'Vencimiento',
};
/** En qué nivel tiene sentido cada medida: los hilos de una burbuja, en
 *  Burbujas; lo escrito de un hilo (archivos y texto), en Hilos. */
export function sizeApplies(k: SizeBy, level: 'bubbles' | 'threads') {
  if (k === 'threads') return level === 'bubbles';
  if (k === 'files' || k === 'text') return level === 'threads';
  return true;
}
/** Marcar o desmarcar una medida: «Igual» excluye a las demás, y quitar la
 *  última vuelve a «Igual». */
export function toggleSize(current: readonly SizeBy[], k: SizeBy): SizeBy[] {
  if (k === 'uniform') return ['uniform'];
  const rest = current.filter((x) => x !== 'uniform');
  const next = rest.includes(k) ? rest.filter((x) => x !== k) : [...rest, k];
  return next.length ? next : ['uniform'];
}
export const COLOR_LABELS: Record<ColorBy, string> = {
  state: 'Estado', priority: 'Prioridad', project: 'Proyecto', team: 'Equipo', tag: 'Tag secundario', due: 'Vencimiento', mine: 'Mío',
};
export const PRESET_LABELS: Record<Preset, string> = {
  mine: 'Lo mío', urgent: 'Urgentes P1–P2', ownerless: 'Sin responsable', due: 'Vencidas o por vencer', connected: 'Muy conectadas', cold: 'Frías',
};

/** Lo que un punto necesita para codificarse, además de lo del grafo. */
export type Facts = {
  score?: number;
  due?: string; // AAAA-MM-DD
  people?: number;
  mine?: boolean;
  tags?: string[];
  threads?: number; // en Burbujas: todos sus hilos
  words?: number; // en Hilos: palabras de su Markdown
  files?: number; // en Hilos: páginas e imágenes citadas
  teamName?: string;
};
export type Encodable = GraphNode & Facts;

/** Un color de esfera: los tres tonos del degradado de Orb.svelte. */
export type Sphere = { key: string; label: string; g1: string; g2: string; g3: string };

export const STATE_SPHERE: Record<string, Sphere> = {
  hot: { key: 'hot', label: 'Caliente', g1: 'oklch(0.82 0.16 40)', g2: 'oklch(0.64 0.21 15)', g3: 'oklch(0.55 0.2 350)' },
  dormant: { key: 'dormant', label: 'Dormida', g1: 'oklch(0.8 0.11 295)', g2: 'oklch(0.62 0.17 300)', g3: 'oklch(0.54 0.18 268)' },
  rip: { key: 'rip', label: 'Tumba', g1: 'oklch(0.74 0.02 265)', g2: 'oklch(0.56 0.02 265)', g3: 'oklch(0.44 0.02 265)' },
  closed: { key: 'closed', label: 'Cerrada', g1: 'oklch(0.92 0.12 98)', g2: 'oklch(0.81 0.16 85)', g3: 'oklch(0.68 0.15 68)' },
};
const hue = (key: string, label: string, h: number, c = 0.17): Sphere => ({
  key, label, g1: `oklch(0.84 ${(c * 0.75).toFixed(3)} ${h + 22})`, g2: `oklch(0.66 ${c} ${h})`, g3: `oklch(0.52 ${(c * 0.95).toFixed(3)} ${h - 22})`,
});
const NONE = (label: string): Sphere => ({ key: 'none', label, g1: 'oklch(0.7 0.01 260)', g2: 'oklch(0.52 0.01 260)', g3: 'oklch(0.4 0.01 260)' });
const PRIORITY: Record<string, Sphere> = {
  P1: hue('P1', 'P1 · crítica', 25, 0.22), P2: hue('P2', 'P2 · alta', 60, 0.19), P3: hue('P3', 'P3 · normal', 230, 0.14), P4: hue('P4', 'P4 · baja', 260, 0.05),
};
const DUE: Record<string, Sphere> = {
  overdue: hue('overdue', 'Vencida', 25, 0.22), week: hue('week', 'Esta semana', 70, 0.18), later: hue('later', 'Más adelante', 160, 0.13),
};
const MINE: Record<string, Sphere> = { mine: hue('mine', 'Mía', 200, 0.18), others: hue('others', 'De otros', 290, 0.12) };
// Categóricos (proyecto, equipo, tag): doce tonos bien separados, asignados
// por orden alfabético de los nombres presentes. Estable (no depende del orden
// de llegada) y sin repetir tono hasta pasar de doce.
const HUES = [25, 145, 265, 70, 205, 330, 105, 235, 0, 175, 295, 45];
function categorical(name: string, index: number): Sphere {
  const h = HUES[index % HUES.length];
  const c = index >= HUES.length ? 0.1 : 0.17; // de la vuelta 13 en adelante, más apagados
  return hue(`c:${name}`, name, h, c);
}

const DAY = 86_400_000;
export function dueStatus(due: string | undefined, now: Date): 'overdue' | 'week' | 'later' | 'none' {
  if (!due) return 'none';
  const t = Date.parse(`${due.slice(0, 10)}T23:59:59`);
  if (Number.isNaN(t)) return 'none';
  if (t < now.getTime()) return 'overdue';
  return t - now.getTime() <= 7 * DAY ? 'week' : 'later';
}

export function degrees(links: readonly GraphLink[]): Map<string, number> {
  const d = new Map<string, number>();
  for (const l of links) {
    d.set(l.source, (d.get(l.source) ?? 0) + l.count);
    d.set(l.target, (d.get(l.target) ?? 0) + l.count);
  }
  return d;
}

/** La medida cruda de un punto para el tamaño; `null` si no la tiene. */
function measure(n: Encodable, by: SizeBy, deg: Map<string, number>, now: Date): number | null {
  switch (by) {
    case 'uniform': return null;
    case 'heat': return n.score ?? null;
    case 'priority': return n.priority ? 5 - Number(n.priority.slice(1)) : null; // P1 → 4 … P4 → 1
    case 'links': return deg.get(n.id) ?? 0;
    case 'threads': return n.kind === 'bubbles' ? (n.threads ?? 0) : null;
    case 'files': return n.kind === 'threads' ? (n.files ?? 0) : null;
    case 'text': return n.kind === 'threads' ? (n.words ?? 0) : null;
    case 'people': return n.people ?? 0;
    case 'due': {
      if (!n.due) return null;
      const days = (Date.parse(`${n.due.slice(0, 10)}T23:59:59`) - now.getTime()) / DAY;
      return Number.isNaN(days) ? null : Math.max(0, 30 - Math.min(30, Math.max(-30, days)));
    }
  }
}

function categoryOf(n: Encodable, by: ColorBy): string {
  return by === 'project' ? n.project : by === 'team' ? (n.teamName || n.team) : by === 'tag' ? (n.tags?.[1] ?? '') : '';
}

function sphereOf(n: Encodable, by: ColorBy, now: Date, cats: Map<string, number>): Sphere {
  switch (by) {
    case 'state': return STATE_SPHERE[n.life] ?? STATE_SPHERE.hot;
    case 'priority': return PRIORITY[n.priority] ?? NONE('Sin prioridad');
    case 'project': return n.project ? categorical(n.project, cats.get(n.project)!) : NONE('Sin proyecto');
    case 'team': { const t = n.teamName || n.team; return t ? categorical(t, cats.get(t)!) : NONE('Sin equipo'); }
    case 'tag': return n.tags?.[1] ? categorical(n.tags[1], cats.get(n.tags[1])!) : NONE('Sin segundo tag');
    case 'due': { const s = dueStatus(n.due, now); return s === 'none' ? NONE('Sin fecha') : DUE[s]; }
    case 'mine': return n.mine ? MINE.mine : (n.people ?? 0) > 0 ? MINE.others : NONE('Sin nadie');
  }
}

function presetMatch(n: Encodable, p: Preset, deg: Map<string, number>, connected: number, now: Date): boolean {
  switch (p) {
    case 'mine': return !!n.mine;
    case 'urgent': return n.priority === 'P1' || n.priority === 'P2';
    case 'ownerless': return (n.people ?? 0) === 0;
    case 'due': { const s = dueStatus(n.due, now); return s === 'overdue' || s === 'week'; }
    case 'connected': return (deg.get(n.id) ?? 0) >= connected;
    case 'cold': return n.life === 'dormant' || n.life === 'rip';
  }
}

const fold = (s: string) => s.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase();

export type Look = { size: number; sphere: Sphere; match: boolean };
export type Legend = { size: string; colors: Sphere[]; filtered: number; total: number };

/** Lo que se dibuja de cada punto: su tamaño (factor sobre el radio base, 0.6
 *  a 2), su esfera y si pasa los filtros. Los filtros se combinan con Y. */
export function encode(nodes: readonly Encodable[], links: readonly GraphLink[], enc: Encoding, now = new Date()) {
  const deg = degrees(links);
  // «Muy conectadas»: el cuartil alto de las que tienen alguna conexión.
  const ds = [...nodes].map((n) => deg.get(n.id) ?? 0).filter((d) => d > 0).sort((a, b) => a - b);
  const connected = ds.length ? Math.max(2, ds[Math.floor(ds.length * 0.75)]) : Infinity;
  // Cada medida elegida se escala de 0 a 1 entre los puntos visibles y se suman;
  // el total, sobre el número de medidas, da el tamaño. Las que no aplican a
  // este nivel no cuentan.
  const level = nodes[0]?.kind ?? 'bubbles';
  const active = enc.size.filter((k) => k !== 'uniform' && sizeApplies(k, level));
  const scales = active.map((k) => {
    const raw = new Map(nodes.map((n) => [n.id, measure(n, k, deg, now)]));
    const vs = [...raw.values()].filter((v): v is number => v !== null);
    return { raw, lo: Math.min(...vs), hi: Math.max(...vs) };
  });
  const sizeOf = (id: string) => {
    if (!active.length) return 1;
    let sum = 0, any = false;
    for (const { raw, lo, hi } of scales) {
      const v = raw.get(id);
      if (v === null || v === undefined || !Number.isFinite(lo)) continue;
      any = true;
      sum += hi === lo ? 0.5 : (v - lo) / (hi - lo);
    }
    // Área proporcional a la medida: el radio va con la raíz.
    return any ? 0.7 + 1.3 * Math.sqrt(sum / active.length) : 0.8;
  };
  const cats = new Map([...new Set(nodes.map((n) => categoryOf(n, enc.color)).filter(Boolean))].sort((a, b) => a.localeCompare(b)).map((name, i) => [name, i]));
  const text = fold(enc.text.trim());
  const looks = new Map<string, Look>();
  const used = new Map<string, Sphere>();
  let filtered = 0;
  for (const n of nodes) {
    const size = sizeOf(n.id);
    const sphere = sphereOf(n, enc.color, now, cats);
    used.set(sphere.key, sphere);
    const hay = text ? fold([n.name, n.project, n.bubbleName ?? '', ...(n.tags ?? [])].join(' ')) : '';
    const match = enc.presets.every((p) => presetMatch(n, p, deg, connected, now)) && (!text || hay.includes(text));
    if (match) filtered++;
    looks.set(n.id, { size, sphere, match });
  }
  const order = (s: Sphere) => (s.key === 'none' ? 1 : 0);
  const legend: Legend = {
    size: active.length ? `Tamaño: ${active.map((k) => SIZE_LABELS[k]).join(' + ')}${active.includes('due') ? ' (vencimiento: más cerca o pasada, más grande)' : ''}` : '',
    colors: [...used.values()].sort((a, b) => order(a) - order(b) || a.label.localeCompare(b.label)).slice(0, 12),
    filtered, total: nodes.length,
  };
  return { looks, legend };
}

/** Lee una codificación guardada sin confiar en ella. */
export function readEncoding(raw: unknown): Encoding {
  const v = (raw && typeof raw === 'object' ? raw : {}) as Partial<Encoding>;
  return {
    // Antes era una sola medida (un texto): se lee como lista de una.
    size: (() => {
      const list = (Array.isArray(v.size) ? v.size : typeof v.size === 'string' ? [v.size] : []).filter((k): k is SizeBy => typeof k === 'string' && k in SIZE_LABELS);
      return !list.length || list.includes('uniform') ? ['uniform'] : [...new Set(list)];
    })(),
    color: v.color && v.color in COLOR_LABELS ? v.color : 'state',
    presets: Array.isArray(v.presets) ? v.presets.filter((p): p is Preset => p in PRESET_LABELS) : [],
    text: typeof v.text === 'string' ? v.text.slice(0, 80) : '',
    mode: v.mode === 'hide' ? 'hide' : 'dim',
  };
}
