import { expect, test } from 'bun:test';
import { matchingPeople, named } from '../src/lib/people';

test('usa el nombre, después sólo el correo que ya es visible, por último el respaldo', () => {
  expect(named({ display_name: 'Ángel', email: 'visible@example.test' })).toBe('Ángel');
  expect(named({ display_name: '', email: 'visible@example.test' })).toBe('visible@example.test');
  expect(named({ display_name: '' })).toBe('sin nombre');
  expect(named({})).toBe('sin nombre');
  expect(named(null)).toBe('sin nombre');
  expect(named(undefined)).toBe('sin nombre');
});

test('los espacios y null no cuentan como nombre ni correo', () => {
  expect(named({ display_name: '  ', email: '  ' })).toBe('sin nombre');
  expect(named({ display_name: null, email: null })).toBe('sin nombre');
  expect(named({ display_name: '  Ángela  ' })).toBe('Ángela');
});

test('conserva el aviso de persona eliminada y los respaldos de cada contexto', () => {
  expect(named({ display_name: 'Ana', deleted_at: '2026-10-19' })).toBe('Ana (eliminado)');
  expect(named({ deleted_at: '2026-10-19' }, 'alguien')).toBe('alguien (eliminado)');
  expect(named(undefined, 'alguien')).toBe('alguien');
  expect(named(undefined, '')).toBe('');
});

test('regresión Todos: nombre vacío y correo omitido no caen en toLowerCase', () => {
  const users = [{ id: 'hidden', display_name: '', avatar: 'cara.png' }];
  const before = users.map((p) => ({ id: p.id, name: p.display_name || (p as { email?: string }).email }));
  expect(() => before.filter((p) => p.name!.toLowerCase().includes(''))).toThrow(TypeError);

  const people = users.map((p) => ({ id: p.id, name: named(p), avatar: p.avatar }));
  expect(matchingPeople(people, '')).toEqual([{ id: 'hidden', name: 'sin nombre', avatar: 'cara.png' }]);
  expect(matchingPeople(people, ' SIN NOMBRE ')).toHaveLength(1);
});

test('el selector tolera opciones incompletas sin perder a nadie ni mutar datos', () => {
  const people = [
    { id: 'missing', avatar: '/cara.png', extra: 42 },
    { id: 'null', name: null },
    { id: 'empty', name: '' },
    { id: 'spaces', name: ' ' },
    { id: 'named', name: 'Ana' },
  ];
  const original = structuredClone(people);
  const options = matchingPeople(people, '');
  expect(options.map((p) => p.id)).toEqual(people.map((p) => p.id));
  expect(options.slice(0, 4).map((p) => p.name)).toEqual(Array(4).fill('sin nombre'));
  expect(options[0]).toMatchObject({ avatar: '/cara.png', extra: 42 });
  expect(people).toEqual(original);
});

test('buscar el respaldo conserva cada identidad aunque coincidan las etiquetas', () => {
  const options = matchingPeople([{ id: 'one' }, { id: 'two', name: '' }, { id: 'three', name: 'Luz' }], 'nombre');
  expect(options.map((p) => p.id)).toEqual(['one', 'two']);
  expect(options.every((p) => typeof p.name === 'string')).toBe(true);
});

test('búsqueda sin distinción de mayúsculas, Unicode y listas vacías', () => {
  const people = [{ id: 'one', name: 'Ángela' }, { id: 'two', name: '가나다' }];
  expect(matchingPeople(people, ' ÁN ')).toEqual([people[0]]);
  expect(matchingPeople(people, '가')).toEqual([people[1]]);
  expect(matchingPeople(people, 'nadie')).toEqual([]);
  expect(matchingPeople([], '')).toEqual([]);
});
