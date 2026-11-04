import { expect, test } from 'bun:test';
import { connectSequence, insertStep, sequenceChanges, sequenceLayers } from '../src/lib/sequencePlan';
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
test('una tarea suelta enlazada con una de la línea entra justo antes, sin moverla', () => {
  expect(connectSequence([['a'], ['b']], 'x', 'a')).toEqual([['x'], ['a'], ['b']]);
  expect(connectSequence([['a'], ['b', 'c'], ['d']], 'x', 'c')).toEqual([['a'], ['x'], ['b', 'c'], ['d']]);
});
test('insertar en un hueco crea un paso entre dos', () => {
  expect(insertStep([['a'], ['b']], 'x', 1)).toEqual([['a'], ['x'], ['b']]);
  expect(insertStep([['a'], ['b']], 'x', 0)).toEqual([['x'], ['a'], ['b']]);
  expect(insertStep([['a'], ['b']], 'x', 2)).toEqual([['a'], ['b'], ['x']]);
});
test('insertar algo que ya estaba lo saca de su paso, y el paso vacío desaparece', () => {
  expect(insertStep([['a'], ['b'], ['c']], 'c', 1)).toEqual([['a'], ['c'], ['b']]);
  expect(insertStep([['a'], ['b', 'c'], ['d']], 'c', 2)).toEqual([['a'], ['b'], ['c'], ['d']]);
  const rows = [['a'], ['b']];
  insertStep(rows, 'a', 2);
  expect(rows).toEqual([['a'], ['b']]);
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
