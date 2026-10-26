// La vista 3D de los blobs: disposición y proyección. Puro, para fijarlo con
// pruebas; el dibujo vive en Graph3D.svelte.
//
// Cada blob es una esfera de puntos (Fibonacci) alrededor de su centro, y los
// centros se reparten sobre una esfera mayor. Determinista: la misma entrada da
// la misma figura, y filtrar no reacomoda (se calcula sobre el universo).
import type { GraphGroup, GraphNode } from './graph';

export type Vec3 = { x: number; y: number; z: number };
export type Orbit = { yaw: number; pitch: number; dist: number };
export type Projected = { x: number; y: number; scale: number; depth: number };

const GOLDEN = Math.PI * (3 - Math.sqrt(5));

/** n puntos repartidos parejo sobre una esfera de radio r. */
export function fibonacciSphere(n: number, r: number): Vec3[] {
  if (n <= 0) return [];
  if (n === 1) return [{ x: 0, y: 0, z: 0 }];
  return Array.from({ length: n }, (_, i) => {
    const y = 1 - (i / (n - 1)) * 2;
    const ring = Math.sqrt(1 - y * y);
    const a = i * GOLDEN;
    return { x: Math.cos(a) * ring * r, y: y * r, z: Math.sin(a) * ring * r };
  });
}

/** Radio de la esfera de un blob: crece con la raíz cúbica de sus puntos, como
 *  un volumen, para que la densidad se parezca entre blobs. */
export const blobRadius = (n: number) => (n <= 1 ? 0 : 34 * Math.cbrt(n));

export function layout3d(nodes: readonly GraphNode[], groups: readonly GraphGroup[]) {
  const ids = [...groups.map((g) => g.id), ...nodes.map((n) => n.group).filter((id) => !groups.some((g) => g.id === id)).sort()];
  const unique = [...new Set(ids)].filter((id) => nodes.some((n) => n.group === id));
  const members = new Map(unique.map((id) => [id, nodes.filter((n) => n.group === id).sort((a, b) => a.id.localeCompare(b.id))]));
  const biggest = Math.max(0, ...unique.map((id) => blobRadius(members.get(id)!.length)));
  // Los centros, separados lo bastante para que dos blobs grandes no se toquen.
  const spread = Math.max(160, (biggest * 2 + 60) * Math.sqrt(unique.length) / 2);
  const centers = fibonacciSphere(unique.length, unique.length > 1 ? spread : 0);
  const positions = new Map<string, Vec3>();
  const blobs = new Map<string, Vec3 & { radius: number }>();
  unique.forEach((id, i) => {
    const c = centers[i];
    const list = members.get(id)!;
    const r = blobRadius(list.length);
    blobs.set(id, { ...c, radius: r });
    fibonacciSphere(list.length, r).forEach((p, j) => positions.set(list[j].id, { x: c.x + p.x, y: c.y + p.y, z: c.z + p.z }));
  });
  const extent = Math.max(1, ...[...blobs.values()].map((b) => Math.hypot(b.x, b.y, b.z) + b.radius));
  return { positions, blobs, extent };
}

/** Gira y proyecta en perspectiva: `focal` es la distancia focal en píxeles,
 *  (cx, cy) el centro de la pantalla. `null` si queda detrás de la cámara. */
export function project(p: Vec3, o: Orbit, focal: number, cx: number, cy: number): Projected | null {
  const cy0 = Math.cos(o.yaw), sy0 = Math.sin(o.yaw);
  const x1 = p.x * cy0 - p.z * sy0;
  const z1 = p.x * sy0 + p.z * cy0;
  const cp = Math.cos(o.pitch), sp = Math.sin(o.pitch);
  const y2 = p.y * cp - z1 * sp;
  const z2 = p.y * sp + z1 * cp;
  const w = o.dist - z2;
  if (w <= 1) return null;
  const scale = focal / w;
  return { x: cx + x1 * scale, y: cy + y2 * scale, scale, depth: z2 };
}

/** Sólo la rotación de la cámara, sin perspectiva: para el gizmo. `x`/`y` en
 *  pantalla (y crece hacia abajo) y `depth` hacia la cámara. */
export function rotate(p: Vec3, o: Pick<Orbit, 'yaw' | 'pitch'>): { x: number; y: number; depth: number } {
  const cy0 = Math.cos(o.yaw), sy0 = Math.sin(o.yaw);
  const x1 = p.x * cy0 - p.z * sy0;
  const z1 = p.x * sy0 + p.z * cy0;
  const cp = Math.cos(o.pitch), sp = Math.sin(o.pitch);
  return { x: x1, y: p.y * cp - z1 * sp, depth: p.y * sp + z1 * cp };
}

/** Hacia dónde mirar para ver a lo largo de un eje. El de arriba (Y) se queda
 *  cerca del límite del pitch, que no llega a la vertical. */
export const AXIS_VIEWS = {
  x: { yaw: Math.PI / 2, pitch: 0 },
  y: { yaw: 0, pitch: -1.35 },
  z: { yaw: 0, pitch: 0 },
} as const;

/** El giro más corto de un ángulo a otro (para no dar media vuelta de más). */
export function nearestYaw(from: number, to: number): number {
  const turn = Math.PI * 2;
  return to + Math.round((from - to) / turn) * turn;
}

/** La distancia de cámara que encuadra todo en un lienzo de lado corto `side`. */
export function fitOrbit(extent: number, side: number, focal: number): number {
  // El borde de la figura (extent) ocupa ~48 % del lado corto, girada como
  // esté: la cara más cercana queda dentro.
  return Math.max(extent * 1.2, (extent * focal) / (side * 0.48) + extent);
}
