import { expect, test } from 'bun:test';
import { bubbles, threads, groups, visibleLinks } from '../src/prototypes/modelFixtures';
import { graphLayout, fitCamera, zoomCamera } from '../src/lib/graph';

test('burbujas e hilos tienen identidades distintas y padre visible identificable', () => {
  expect(new Set([...bubbles, ...threads].map((p) => p.id)).size).toBe(bubbles.length + threads.length);
  for (const thread of threads) {
    const bubble = bubbles.find((b) => b.id === thread.bubbleId)!;
    expect(bubble).toBeDefined();
    expect(thread.group).toBe(bubble.group);
    expect(thread.team).toBe(bubble.team);
    expect(thread.kind).toBe('threads');
  }
});

test('la promesa de una burbuja no se copia a sus hilos', () => {
  const undated = threads.find((t) => t.id === 'integration:1')!;
  expect(bubbles.find((b) => b.id === 'integration')!.deadline).toContain('3 días');
  expect(undated.deadline).toContain('Sin compromiso');
  expect(undated.level).toBe('calm');
});

test('la proyección sólo enlaza extremos visibles, no tags compartidos ni equipos distintos', () => {
  const one = bubbles.filter((b) => b.id === 'billing');
  expect(visibleLinks('bubbles', one)).toEqual([]);
  const links = visibleLinks('bubbles', bubbles);
  expect(links.length).toBeGreaterThan(0);
  for (const link of links) {
    const source = bubbles.find((p) => p.id === link.source)!;
    const target = bubbles.find((p) => p.id === link.target)!;
    expect(source.id).not.toBe(target.id);
    expect(source.team).toBe(target.team);
  }
  expect(visibleLinks('threads', threads.filter((t) => t.id === 'integration:1'))).toEqual([]);
});

test('layout determinista, sin mutar fixtures ni depender del orden de entrada', () => {
  const before = JSON.stringify(threads);
  const a = graphLayout(threads, groups), b = graphLayout([...threads].reverse(), groups);
  expect([...a.positions]).toEqual([...b.positions]);
  expect(JSON.stringify(threads)).toBe(before);
  for (const position of a.positions.values()) {
    expect(Number.isFinite(position.x) && Number.isFinite(position.y)).toBe(true);
  }
});

test('el mismo tag de dos equipos es un solo blob y un tag desconocido no pierde nodos', () => {
  const layout = graphLayout(threads, groups);
  const infra = threads.filter((t) => t.group === 'infra');
  expect(new Set(infra.map((t) => t.team))).toEqual(new Set(['a', 'b']));
  expect(layout.groups.has('infra')).toBe(true);
  const unknown = { ...threads[0], id: 'unknown', group: 'other' };
  expect(graphLayout([unknown], groups).positions.has('unknown')).toBe(true);
  expect(graphLayout([], []).positions.size).toBe(0);
});

test('encuadre vacío y de un nodo es finito y acotado', () => {
  for (const points of [[], [{ x: 1, y: 1 }]]) {
    const camera = fitCamera(points, 390, 544);
    expect(Number.isFinite(camera.x) && Number.isFinite(camera.y)).toBe(true);
    expect(camera.scale).toBeGreaterThanOrEqual(0.2);
    expect(camera.scale).toBeLessThanOrEqual(2);
  }
});

test('el zoom mantiene el punto bajo el cursor y respeta límites', () => {
  const before = { x: 30, y: -10, scale: 1 };
  const after = zoomCamera(before, 2, 60, 20);
  expect(after.x + 60 / after.scale).toBe(before.x + 60 / before.scale);
  expect(after.y + 20 / after.scale).toBe(before.y + 20 / before.scale);
  expect(zoomCamera(before, 100).scale).toBe(4);
  expect(zoomCamera(before, 0.001).scale).toBe(0.2);
});
