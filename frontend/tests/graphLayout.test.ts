import { expect, test } from 'bun:test';
import { graphLayout, groupFontSize, NODE_SPACING, type GraphGroup, type GraphNode } from '../src/lib/graph';

const node = (id: string, group: string, kind: GraphNode['kind'] = 'bubbles'): GraphNode => ({
  id, name: id, team: 't', project: 'p', group, life: 'hot', level: 'calm', priority: '', kind,
});

// Doce blobs de 1 a 12 puntos y «Sin tags» con 15: lo que dejó ver el
// seed, cuando dos columnas fijas encimaban los grupos grandes.
const blobs: GraphGroup[] = [
  ...Array.from({ length: 12 }, (_, i) => ({ id: `c${i}`, name: i % 2 ? `tag número ${i} · largo/de/verdad` : `tag ${i}` })),
  { id: '', name: 'Sin tags' },
];
const nodes = [
  ...blobs.slice(0, 12).flatMap((c, i) => Array.from({ length: i + 1 }, (_, j) => node(`${c.id}-${j}`, c.id))),
  ...Array.from({ length: 15 }, (_, j) => node(`none-${j}`, '')),
];

test('las constelaciones no se tocan, aunque sean de tamaños muy distintos', () => {
  const { groups } = graphLayout(nodes, blobs);
  const list = [...groups.values()];
  expect(list.length).toBe(13);
  for (let i = 0; i < list.length; i++) {
    for (let j = i + 1; j < list.length; j++) {
      const a = list[i], b = list[j];
      expect(Math.hypot(a.x - b.x, a.y - b.y)).toBeGreaterThan(a.radius + b.radius + 40);
    }
  }
});

test('el conjunto queda apaisado, no una tira', () => {
  const { groups } = graphLayout(nodes, blobs);
  const list = [...groups.values()];
  const width = Math.max(...list.map((g) => g.x + g.radius)) - Math.min(...list.map((g) => g.x - g.radius));
  const height = Math.max(...list.map((g) => g.y + g.radius)) - Math.min(...list.map((g) => g.y - g.radius));
  expect(width / height).toBeGreaterThan(1);
  expect(width / height).toBeLessThan(3.5);
});

test('cada punto cae dentro de su blob, y los vecinos guardan distancia', () => {
  const { groups, positions } = graphLayout(nodes, blobs);
  for (const n of nodes) {
    const g = groups.get(n.group)!, p = positions.get(n.id)!;
    expect(Math.hypot(p.x - g.x, p.y - g.y)).toBeLessThanOrEqual(g.radius + 1e-9);
  }
  const big = nodes.filter((n) => n.group === '').map((n) => positions.get(n.id)!);
  const nearest = big.map((p, i) => Math.min(...big.filter((_, j) => j !== i).map((q) => Math.hypot(p.x - q.x, p.y - q.y))));
  expect(Math.min(...nearest)).toBeGreaterThan(NODE_SPACING.bubbles * 0.45);
});

test('el nombre de un blob mide entre 15 y 28 px en pantalla', () => {
  for (const scale of [0.2, 0.5, 1, 2, 4]) {
    const px = groupFontSize(scale) * scale;
    expect(px).toBeGreaterThanOrEqual(15);
    expect(px).toBeLessThanOrEqual(28);
  }
});
