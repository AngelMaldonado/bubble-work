import { expect, test } from 'bun:test';
import { fibonacciSphere, layout3d, project, fitOrbit, blobRadius, rotate, AXIS_VIEWS, nearestYaw } from '../src/lib/graph3d';
import type { GraphGroup, GraphNode } from '../src/lib/graph';

const node = (id: string, group: string): GraphNode => ({ id, name: id, team: 't', project: 'p', group, life: 'hot', level: 'calm', priority: '', kind: 'threads' });
const groups: GraphGroup[] = [{ id: 'a', name: 'a' }, { id: 'b', name: 'b' }, { id: 'c', name: 'c' }, { id: '', name: 'Sin tags' }];
const nodes = [
  ...Array.from({ length: 30 }, (_, i) => node(`a${i}`, 'a')),
  ...Array.from({ length: 8 }, (_, i) => node(`b${i}`, 'b')),
  node('c0', 'c'),
  ...Array.from({ length: 12 }, (_, i) => node(`n${i}`, '')),
];

test('la esfera de Fibonacci reparte en la superficie', () => {
  for (const p of fibonacciSphere(50, 10)) expect(Math.hypot(p.x, p.y, p.z)).toBeCloseTo(10, 6);
  expect(fibonacciSphere(1, 10)).toEqual([{ x: 0, y: 0, z: 0 }]);
  expect(fibonacciSphere(0, 10)).toEqual([]);
});

test('determinista, sin depender del orden de entrada, y cada punto dentro de su blob', () => {
  const a = layout3d(nodes, groups), b = layout3d([...nodes].reverse(), groups);
  expect([...a.positions].sort()).toEqual([...b.positions].sort());
  for (const n of nodes) {
    const p = a.positions.get(n.id)!, c = a.blobs.get(n.group)!;
    expect(Math.hypot(p.x - c.x, p.y - c.y, p.z - c.z)).toBeLessThanOrEqual(c.radius + 1e-9);
  }
});

test('los blobs no se tocan', () => {
  const { blobs } = layout3d(nodes, groups);
  const list = [...blobs.values()];
  for (let i = 0; i < list.length; i++) for (let j = i + 1; j < list.length; j++) {
    const d = Math.hypot(list[i].x - list[j].x, list[i].y - list[j].y, list[i].z - list[j].z);
    expect(d).toBeGreaterThan(list[i].radius + list[j].radius);
  }
  expect(blobRadius(30)).toBeGreaterThan(blobRadius(8));
});

test('la proyección: lo cercano se ve más grande, lo de atrás de la cámara no se dibuja', () => {
  const o = { yaw: 0, pitch: 0, dist: 500 };
  const near = project({ x: 10, y: 0, z: 100 }, o, 800, 0, 0)!, far = project({ x: 10, y: 0, z: -100 }, o, 800, 0, 0)!;
  expect(near.scale).toBeGreaterThan(far.scale);
  expect(near.depth).toBeGreaterThan(far.depth);
  expect(project({ x: 0, y: 0, z: 600 }, o, 800, 0, 0)).toBeNull();
  // Girar media vuelta intercambia delante y detrás.
  const turned = project({ x: 10, y: 0, z: 100 }, { ...o, yaw: Math.PI }, 800, 0, 0)!;
  expect(turned.depth).toBeCloseTo(-100, 6);
});

test('encuadrar deja la figura entera en pantalla', () => {
  const { extent } = layout3d(nodes, groups);
  const side = 800, focal = 900;
  const dist = fitOrbit(extent, side, focal);
  for (const yaw of [0, 1, 2, 3]) {
    for (const p of layout3d(nodes, groups).positions.values()) {
      const q = project(p, { yaw, pitch: 0.3, dist }, focal, side / 2, side / 2)!;
      expect(q).not.toBeNull();
      expect(q.x).toBeGreaterThan(0); expect(q.x).toBeLessThan(side);
      expect(q.y).toBeGreaterThan(0); expect(q.y).toBeLessThan(side);
    }
  }
});

test('el gizmo: mirar a lo largo de un eje lo deja apuntando a la cámara', () => {
  const unit = { x: { x: 1, y: 0, z: 0 }, y: { x: 0, y: -1, z: 0 }, z: { x: 0, y: 0, z: 1 } };
  for (const axis of ['x', 'z'] as const) {
    const r = rotate(unit[axis], AXIS_VIEWS[axis]);
    expect(r.depth).toBeCloseTo(1, 6);
    expect(Math.hypot(r.x, r.y)).toBeCloseTo(0, 6);
  }
  // Desde arriba, el eje Y (hacia arriba) apunta casi a la cámara.
  expect(rotate(unit.y, AXIS_VIEWS.y).depth).toBeGreaterThan(0.97);
  // Coincide con la proyección, sin perspectiva.
  const o = { yaw: 0.7, pitch: -0.4, dist: 1e9 }, p = { x: 3, y: -2, z: 5 };
  const q = project(p, o, 1e9, 0, 0)!, r = rotate(p, o);
  expect(q.x).toBeCloseTo(r.x, 3); expect(q.y).toBeCloseTo(r.y, 3);
});

test('el giro más corto no da vueltas de más', () => {
  expect(nearestYaw(Math.PI * 4 + 0.1, 0)).toBeCloseTo(Math.PI * 4, 6);
  expect(nearestYaw(-0.2, Math.PI / 2)).toBeCloseTo(Math.PI / 2, 6);
  expect(Math.abs(nearestYaw(6.1, 0.1) - 6.1)).toBeLessThan(Math.PI);
});
