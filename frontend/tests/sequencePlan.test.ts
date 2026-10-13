import { expect, test } from 'bun:test';
import { connectSequence, sequenceChanges, sequenceLayers } from '../src/lib/sequencePlan';
const row = (id: string, sequence = 0, done = false) => ({ id, sequence, done });

test('el primer nodo inicia el paso 1', () => {
  expect(sequenceChanges([row('a')], [['a']])).toEqual([{ id: 'a', sequence: 10 }]);
});
test('A → B → C forma tres pasos', () => {
  const rows = connectSequence(connectSequence([['a']], 'a', 'b'), 'b', 'c');
  expect(rows).toEqual([['a'], ['b'], ['c']]);
  expect(sequenceChanges([row('a', 10)], rows)).toEqual([
    { id: 'b', sequence: 20 }, { id: 'c', sequence: 30 },
  ]);
});
test('dos conexiones desde A son un paso paralelo', () => {
  expect(connectSequence([['a'], ['b']], 'a', 'c')).toEqual([['a'], ['b', 'c']]);
});
test('reconectar un nodo anterior mueve su paso, no crea un ciclo', () => {
  expect(connectSequence([['a'], ['b'], ['c']], 'c', 'a')).toEqual([['b'], ['c'], ['a']]);
});
test('no permite conectar un nodo consigo mismo ni muta el original', () => {
  const rows = [['a'], ['b', 'c']];
  expect(connectSequence(rows, 'b', 'b')).toEqual(rows);
  connectSequence(rows, 'a', 'c');
  expect(rows).toEqual([['a'], ['b', 'c']]);
});
test('un origen nuevo se incorpora al final', () => {
  expect(connectSequence([['a']], 'b', 'c')).toEqual([['a'], ['b'], ['c']]);
});
test('sacar un nodo compacta huecos sin cambiar los pasos terminados', () => {
  expect(sequenceChanges([row('a', 10), row('b', 20), row('c', 30), row('d', 40, true)], [['a'], [], ['c']]))
    .toEqual([{ id: 'c', sequence: 20 }, { id: 'b', sequence: 0 }]);
});
test('Ahora es el primer paso abierto; quedan fuera los no conectados', () => {
  expect(sequenceLayers([row('b', 40), row('a', 10, true), row('c', 40), row('d')]))
    .toEqual([[row('b', 40), row('c', 40)]]);
});
test('no emite escrituras cuando el orden no cambia', () => {
  expect(sequenceChanges([row('a', 10), row('b', 20)], [['a'], ['b']])).toEqual([]);
});
