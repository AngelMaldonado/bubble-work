// El grafo de la vista Blobs: tipos, disposición y aristas.
//
// Puro: sin Svelte ni red, para que el tablero real y los prototipos dibujen
// con la misma geometría y las pruebas la fijen. Un punto es una burbuja o un
// hilo según el nivel; un blob es un tag: una constelación con nombre, sin
// calor propio. La posición sale del universo visible, no de la búsqueda:
// filtrar no reacomoda nada. Las aristas son relaciones declaradas, nunca
// «comparten tag».
import type { Lifecycle } from './api';

export type MapKind = 'bubbles' | 'threads';
/** La furia de un punto. Mientras no haya evaluador, todo es `calm`. */
export type Level = 'calm' | 'warning' | 'angry' | 'furious';
/** Un blob: un tag y su nombre. `''` es «Sin tags». */
export type GraphGroup = { id: string; name: string };
export type GraphNode = {
  id: string;
  name: string;
  /** el equipo de su proyecto */
  team: string;
  project: string;
  /** su blob (un tag); `''` es «Sin tags» */
  group: string;
  life: Lifecycle;
  level: Level;
  priority: string;
  kind: MapKind;
  /** en el nivel Hilos, su burbuja */
  bubbleId?: string;
  bubbleName?: string;
};
export type GraphLink = { source: string; target: string; kind: 'dependency' | 'related'; count: number };

export const levels: Record<Level, string> = { calm: 'Sin alerta', warning: 'Advertencia', angry: 'Enojada', furious: 'Furiosa' };
export const rank: Record<Level, number> = { calm: 0, warning: 1, angry: 2, furious: 3 };

export type Point = { x: number; y: number };
export type Camera = Point & { scale: number };
export type GroupPosition = Point & { id: string; radius: number };

/** Distancia aproximada entre vecinos de una constelación, en unidades del
 *  mundo. */
export const NODE_SPACING: Record<MapKind, number> = { bubbles: 110, threads: 46 };

/** Tamaño en pantalla del nombre de un blob: crece con el zoom entre 15 y
 *  28 px. Se devuelve en unidades del mundo, que es lo que pide el SVG. */
export function groupFontSize(scale: number) {
  return Math.min(28, Math.max(15, 30 * scale)) / scale;
}

// Lo que ocupa una constelación en el mundo: el círculo, su nombre encima y
// los nombres de sus puntos debajo. El ancho del nombre se estima (no hay DOM
// aquí) con el tamaño de letra que tiene a escala 1.
const NAME_CHAR = 11;
const ABOVE = 80;
const BELOW = 50;
const GAP = 70;
/** Ancho/alto que se busca para el conjunto: apaisado, como una pantalla. */
const ASPECT = 1.7;

/** Distribución espacial determinista. Se calcula sobre el universo, no sobre
 * la búsqueda: filtrar no vuelve a barajar nodos. Sin simulación continua.
 *
 * Cada blob es un girasol cuyo radio crece con sus puntos, a la distancia
 * de `NODE_SPACING`. Las constelaciones se acomodan en filas, en el orden de
 * los blobs, con el tamaño de cada una y el ancho de su nombre: una grande
 * no invade a la vecina. Antes eran dos columnas fijas, y con una docena de
 * grupos el mapa era una tira angosta con nombres encimados. */
export function graphLayout(nodes: readonly GraphNode[], groups: readonly GraphGroup[]) {
  const ids = [...groups.map((c) => c.id), ...nodes.map((n) => n.group).filter((id) => !groups.some((c) => c.id === id)).sort()];
  const unique = [...new Set(ids)];
  const positions = new Map<string, Point>();
  const placed = new Map<string, GroupPosition>();

  const boxes = unique.map((id) => {
    const members = nodes.filter((n) => n.group === id).sort((a, b) => a.id.localeCompare(b.id));
    const spacing = NODE_SPACING[members[0]?.kind ?? 'bubbles'];
    // En un girasol de N puntos y radio R, la distancia entre vecinos es
    // ~R·√(π/N); se despeja R para la distancia buscada.
    const radius = Math.max(60, spacing * Math.sqrt(members.length / Math.PI) * 1.1);
    const label = groups.find((x) => x.id === id)?.name ?? (id || 'Sin tags');
    const width = Math.max(2 * radius + 2 * spacing, label.length * NAME_CHAR + 40) + GAP;
    const height = ABOVE + 2 * radius + BELOW + GAP;
    return { id, members, radius, width, height };
  });

  // Filas de un ancho tal que el conjunto quede cerca de ASPECT.
  const area = boxes.reduce((s, b) => s + b.width * b.height, 0);
  const target = Math.max(...boxes.map((b) => b.width), Math.sqrt(area * ASPECT));
  const rows: (typeof boxes)[] = [];
  for (const box of boxes) {
    const row = rows.at(-1);
    const used = row?.reduce((s, b) => s + b.width, 0) ?? Infinity;
    if (row && used + box.width <= target) row.push(box);
    else rows.push([box]);
  }
  const total = rows.reduce((s, r) => s + Math.max(...r.map((b) => b.height)), 0);
  let top = -total / 2;
  let index = 0;
  for (const row of rows) {
    const rowHeight = Math.max(...row.map((b) => b.height));
    let left = -row.reduce((s, b) => s + b.width, 0) / 2;
    for (const box of row) {
      const x = left + box.width / 2;
      const y = top + ABOVE + GAP / 2 + box.radius + (rowHeight - box.height) / 2;
      placed.set(box.id, { id: box.id, x, y, radius: box.radius });
      const n = box.members.length;
      box.members.forEach((node, i) => {
        const angle = i * Math.PI * (3 - Math.sqrt(5)) + index * 0.71;
        const r = n === 1 ? 0 : box.radius * Math.sqrt((i + 0.5) / n);
        positions.set(node.id, { x: x + Math.cos(angle) * r, y: y + Math.sin(angle) * r });
      });
      left += box.width;
      index++;
    }
    top += rowHeight;
  }
  return { positions, groups: placed };
}

export function fitCamera(points: readonly Point[], width: number, height: number): Camera {
  if (!points.length) return { x: 0, y: 0, scale: 1 };
  const xs = points.map((p) => p.x), ys = points.map((p) => p.y);
  const left = Math.min(...xs) - 100, right = Math.max(...xs) + 100;
  const top = Math.min(...ys) - 120, bottom = Math.max(...ys) + 90;
  return { x: (left + right) / 2, y: (top + bottom) / 2, scale: Math.max(0.2, Math.min(2, width / (right - left), height / (bottom - top))) };
}

/** Acercar conservando el punto bajo el cursor. */
export function zoomCamera(camera: Camera, factor: number, dx = 0, dy = 0): Camera {
  const scale = Math.max(0.2, Math.min(4, camera.scale * factor));
  return { x: camera.x + dx / camera.scale - dx / scale, y: camera.y + dy / camera.scale - dy / scale, scale };
}

/** Las aristas del nivel visible. Sólo relaciones declaradas entre hilos, con
 * los dos extremos visibles y del mismo equipo; en Burbujas, las de sus hilos,
 * proyectadas y contadas. Compartir un tag no crea ninguna. */
export function projectLinks(kind: MapKind, visible: readonly GraphNode[], allThreads: readonly GraphNode[], relations: readonly GraphLink[]): GraphLink[] {
  const ids = new Set(visible.map((p) => p.id));
  const byThread = new Map(allThreads.map((p) => [p.id, p]));
  const result = new Map<string, GraphLink>();
  for (const edge of relations) {
    const from = byThread.get(edge.source), to = byThread.get(edge.target);
    if (!from || !to || from.team !== to.team) continue;
    let source = kind === 'bubbles' ? from.bubbleId ?? '' : from.id;
    let target = kind === 'bubbles' ? to.bubbleId ?? '' : to.id;
    if (!source || !target || source === target || !ids.has(source) || !ids.has(target)) continue;
    if (edge.kind === 'related' && source > target) [source, target] = [target, source];
    const key = `${source}/${target}/${edge.kind}`;
    const existing = result.get(key);
    if (existing) existing.count += 1;
    else result.set(key, { source, target, kind: edge.kind, count: 1 });
  }
  return [...result.values()];
}

/** Los blobs que dibujar para un conjunto de puntos: los que tienen alguno,
 *  en el orden dado, y «Sin tags» al final si hace falta. */
export function groupsFor(nodes: readonly GraphNode[], groups: readonly GraphGroup[]): GraphGroup[] {
  const used = new Set(nodes.map((n) => n.group));
  const out = groups.filter((c) => c.id && used.has(c.id));
  if (used.has('') || nodes.some((n) => n.group && !out.some((c) => c.id === n.group))) {
    out.push({ id: '', name: 'Sin tags' });
  }
  return out;
}
