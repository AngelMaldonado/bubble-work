import { expect, test } from 'bun:test';
import { lineAt, topOf, type Block } from '../src/lib/docPosition';

// Un título en la línea 1 (40 px), un párrafo de la 3 a la 6 (100 px) y una
// lista en la 8 (60 px), uno debajo del otro desde y = 0.
const blocks: Block[] = [
  { line: 1, top: 0, height: 40 },
  { line: 3, top: 50, height: 100 },
  { line: 8, top: 160, height: 60 },
];

test('en el borde de un bloque, su línea', () => {
  expect(lineAt(blocks, 0)).toBe(1);
  expect(lineAt(blocks, 50)).toBe(3);
  expect(lineAt(blocks, 45)).toBe(3); // en el hueco entre dos: el de abajo
});
test('a media altura de un bloque, en proporción hasta el siguiente', () => {
  expect(lineAt(blocks, 100)).toBe(5.5);
});
test('pasado el final, la última línea marcada', () => {
  expect(lineAt(blocks, 999)).toBe(8);
});
test('de línea a posición, y de vuelta, llega al mismo sitio', () => {
  expect(topOf(blocks, 3)).toBe(50);
  expect(topOf(blocks, 5.5)).toBe(100);
  expect(lineAt(blocks, topOf(blocks, 5.5)!)).toBe(5.5);
  expect(topOf(blocks, 0)).toBe(0);
});
test('sin marcas no hay traducción', () => {
  expect(lineAt([], 10)).toBeNull();
  expect(topOf([], 3)).toBeNull();
});
