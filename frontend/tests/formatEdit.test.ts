import { describe, expect, test } from 'bun:test';
import { formatEdit } from '../src/lib/formatEdit';
import { COMMANDS } from '../src/lib/slashmenu.svelte';

describe('formato en línea', () => {
  test('envuelve la selección y la deja seleccionada', () => {
    const r = formatEdit('hola mundo', 5, 10, '**');
    expect(r.text).toBe('**mundo**');
    expect([r.anchor, r.head]).toEqual([7, 12]);
  });
  test('sin selección deja un marcador listo para escribir', () => {
    const r = formatEdit('ab', 1, 1, '++', 'texto');
    expect(r.text).toBe('++texto++');
  });
  test('aplicarlo otra vez lo quita, con los marcadores fuera o dentro', () => {
    const doc = 'x **y** z';
    expect(formatEdit(doc, 4, 5, '**')).toMatchObject({ from: 2, to: 7, text: 'y' });
    expect(formatEdit(doc, 2, 7, '**')).toMatchObject({ from: 2, to: 7, text: 'y' });
  });
  test('el menú / ofrece los tres formatos sin partir la línea', () => {
    for (const id of ['bold', 'italic', 'underline']) expect(COMMANDS.find((c) => c.id === id)?.inline).toBe(true);
  });
});
