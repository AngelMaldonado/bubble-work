import { expect, test } from 'bun:test';
import { parallelFrames } from '../src/lib/sequenceLayout';

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
