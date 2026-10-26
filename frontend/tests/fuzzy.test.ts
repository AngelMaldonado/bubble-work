import { expect, test } from 'bun:test';
import { match } from '../src/lib/fuzzy';

test('un texto que se alarga al plegarse no truena', () => {
  // El hangul se descompone en jamo: el texto plegado es más largo que el original.
  // «가가가» son 3 letras y 6 al plegarse: la tercera vocal cae en la 5, fuera del original.
  expect(() => match('\u1161\u1161\u1161', '가가가')).not.toThrow();
  expect(match('\u1161\u1161\u1161', '가가가')).not.toBeNull();
});

test('un texto en NFD (acentos separados) sigue encontrándose', () => {
  expect(match('cafe', 'cafe\u0301 con leche')).not.toBeNull();
});
