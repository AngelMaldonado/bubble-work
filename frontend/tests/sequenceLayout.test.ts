import { expect, test } from 'bun:test';
import { parallelFrames, sequenceLinks } from '../src/lib/sequenceLayout';

const node = (id: string, x: number, y: number) => ({ id, x, y });
test('entre dos pasos de una tarea, una flecha directa que corta a la de la punta', () => {
  const { links, gaps } = sequenceLinks([[node('a', 0, 0)], [node('b', 0, 200)]], (p) => p, 200, 100);
  expect(links).toEqual([{ key: 'a>b', from: { x: 100, y: 114 }, to: { x: 100, y: 194 }, arrow: true, cut: 'b' }]);
  expect(gaps).toEqual([{ at: 1, x: 100, y: 154 }]);
});
test('entre pasos con varias tareas, m+n tramos por un nudo y no m×n flechas', () => {
  const rows = [[node('a', 0, 0), node('b', 300, 0), node('c', 600, 0)], [node('d', 0, 300), node('e', 600, 300)]];
  const { links, gaps } = sequenceLinks(rows, (p) => p, 200, 100);
  expect(links.length).toBe(5);
  expect(links.filter((l) => l.cut).map((l) => l.cut)).toEqual(['d', 'e']);
  expect(links.filter((l) => !l.arrow).length).toBe(3);
  expect(gaps).toEqual([{ at: 1, x: 400, y: 193 }]); // entre los dos marcos
});
test('un solo paso no tiene flechas ni huecos', () => {
  expect(sequenceLinks([[node('a', 0, 0)]], (p) => p, 200, 100)).toEqual({ links: [], gaps: [] });
});

test('un paso de un solo nodo no lleva región de paralelo', () => {
  expect(parallelFrames([[{ x: 0, y: 0 }]], (p) => p, 230, 112)).toEqual([]);
});
test('la región identifica el paso y abarca todos sus nodos con aire para el rótulo', () => {
  const rows = [[{ x: 0, y: 0 }], [{ x: -100, y: 200 }, { x: 200, y: 200 }]];
  expect(parallelFrames(rows, (p) => p, 230, 112)).toEqual([
    { step: 1, count: 2, x: -120, y: 156, width: 570, height: 192 },
  ]);
});
test('mover nodos a alturas distintas no cambia que van en el mismo paso', () => {
  const row = [{ x: 20, y: 100 }, { x: 280, y: 400 }];
  expect(parallelFrames([row], (p) => p, 230, 112)).toEqual([
    { step: 0, count: 2, x: 0, y: 56, width: 530, height: 492 },
  ]);
});
