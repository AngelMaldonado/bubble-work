import { expect, test } from 'bun:test';
import { encode, dueStatus, readEncoding, toggleSize, sizeApplies, DEFAULT_ENCODING, type Encodable, type Encoding } from '../src/lib/encoding';

const now = new Date('2026-10-20T12:00:00');
const n = (id: string, extra: Partial<Encodable> = {}): Encodable => ({
  id, name: id, team: 'ta', project: 'Alfa', group: 'pagos', life: 'hot', level: 'calm', priority: '', kind: 'threads', ...extra,
});
const nodes: Encodable[] = [
  n('a', { score: 0.9, priority: 'P1', due: '2026-10-19', people: 2, mine: true, tags: ['pagos', 'móvil'] }),
  n('b', { score: 0.1, priority: 'P4', due: '2026-10-24', people: 0, tags: ['pagos'] , life: 'rip' }),
  n('c', { score: 0.5, priority: 'P2', people: 1, tags: ['pagos', 'web'], project: 'Beta', life: 'dormant' }),
  n('d', { name: 'Conciliar pagos', project: 'Beta' }),
];
const links = [{ source: 'a', target: 'c', kind: 'related' as const, count: 1 }, { source: 'a', target: 'b', kind: 'dependency' as const, count: 2 }];
const enc = (e: Partial<Encoding>) => ({ ...DEFAULT_ENCODING, ...e });

test('por omisión: todas del mismo tamaño y con el color de su estado', () => {
  const { looks, legend } = encode(nodes, links, DEFAULT_ENCODING, now);
  expect(new Set([...looks.values()].map((l) => l.size))).toEqual(new Set([1]));
  expect(looks.get('b')!.sphere.key).toBe('rip');
  expect(legend.size).toBe('');
  expect(legend.filtered).toBe(4);
});

test('el tamaño sigue la medida elegida; sin dato, el tamaño base', () => {
  const heat = encode(nodes, links, enc({ size: ['heat'] }), now).looks;
  expect(heat.get('a')!.size).toBeGreaterThan(heat.get('c')!.size);
  expect(heat.get('c')!.size).toBeGreaterThan(heat.get('b')!.size);
  expect(heat.get('d')!.size).toBe(0.8);
  const linksBy = encode(nodes, links, enc({ size: ['links'] }), now).looks;
  expect(linksBy.get('a')!.size).toBe(2); // la más conectada llega al tope
  expect(linksBy.get('d')!.size).toBe(0.7);
  for (const l of encode(nodes, links, enc({ size: ['due'] }), now).looks.values()) {
    expect(l.size).toBeGreaterThanOrEqual(0.7); expect(l.size).toBeLessThanOrEqual(2);
  }
});

test('el color: prioridad, vencimiento, mío y categóricos estables por nombre', () => {
  const p = encode(nodes, links, enc({ color: 'priority' }), now).looks;
  expect(p.get('a')!.sphere.key).toBe('P1');
  expect(p.get('d')!.sphere.key).toBe('none');
  const due = encode(nodes, links, enc({ color: 'due' }), now).looks;
  expect([due.get('a')!.sphere.key, due.get('b')!.sphere.key, due.get('c')!.sphere.key]).toEqual(['overdue', 'week', 'none']);
  const mine = encode(nodes, links, enc({ color: 'mine' }), now).looks;
  expect([mine.get('a')!.sphere.key, mine.get('c')!.sphere.key, mine.get('b')!.sphere.key]).toEqual(['mine', 'others', 'none']);
  const proj1 = encode(nodes, links, enc({ color: 'project' }), now).looks;
  const proj2 = encode([...nodes].reverse(), links, enc({ color: 'project' }), now).looks;
  expect(proj1.get('c')!.sphere).toEqual(proj2.get('c')!.sphere);
  expect(proj1.get('a')!.sphere.key).not.toBe(proj1.get('c')!.sphere.key);
  expect(encode(nodes, links, enc({ color: 'tag' }), now).looks.get('a')!.sphere.label).toBe('móvil');
});

test('los filtros se combinan con Y, y el texto ignora acentos y mayúsculas', () => {
  const m = (e: Partial<Encoding>) => [...encode(nodes, links, enc(e), now).looks].filter(([, l]) => l.match).map(([id]) => id).sort();
  expect(m({ presets: ['urgent'] })).toEqual(['a', 'c']);
  expect(m({ presets: ['urgent', 'mine'] })).toEqual(['a']);
  expect(m({ presets: ['ownerless'] })).toEqual(['b', 'd']);
  expect(m({ presets: ['due'] })).toEqual(['a', 'b']);
  expect(m({ presets: ['cold'] })).toEqual(['b', 'c']);
  expect(m({ presets: ['connected'] })).toEqual(['a']);
  expect(m({ text: 'CONCILIAR' })).toEqual(['d']);
  expect(m({ text: 'movil' })).toEqual(['a']);
  expect(encode(nodes, links, enc({ presets: ['urgent'] }), now).legend.filtered).toBe(2);
});

test('vencimiento: hoy cuenta hasta el final del día', () => {
  expect(dueStatus('2026-10-20', now)).toBe('week');
  expect(dueStatus('2026-10-19', now)).toBe('overdue');
  expect(dueStatus('2026-11-30', now)).toBe('later');
  expect(dueStatus(undefined, now)).toBe('none');
  expect(dueStatus('no es fecha', now)).toBe('none');
});

test('una preferencia guardada se lee sin confiar en ella', () => {
  expect(readEncoding(null)).toEqual(DEFAULT_ENCODING);
  expect(readEncoding({ size: 'calor', color: 'priority', presets: ['mine', 'x'], text: 7, mode: 'hide' }))
    .toEqual({ size: ['uniform'], color: 'priority', presets: ['mine'], text: '', mode: 'hide' });
  // Lo guardado antes, una sola medida, se lee como lista de una.
  expect(readEncoding({ size: 'heat' }).size).toEqual(['heat']);
  expect(readEncoding({ size: ['heat', 'links', 'heat', 'nada'] }).size).toEqual(['heat', 'links']);
  expect(readEncoding({ size: ['heat', 'uniform'] }).size).toEqual(['uniform']);
});

test('varias medidas se suman: lo que destaca en las dos, más grande', () => {
  // a: mucho calor y la más conectada; c: calor medio, conexión 1; b: poco calor, conexión 2.
  const both = encode(nodes, links, enc({ size: ['heat', 'links'] }), now).looks;
  const heat = encode(nodes, links, enc({ size: ['heat'] }), now).looks;
  expect(both.get('a')!.size).toBe(2); // primera en las dos
  expect(both.get('b')!.size).toBeGreaterThan(heat.get('b')!.size); // la conexión la sube
  expect(encode(nodes, links, enc({ size: ['heat', 'links'] }), now).legend.size).toBe('Tamaño: Calor + Conexiones');
});

test('«Igual» excluye a las demás y quitar la última vuelve a «Igual»', () => {
  expect(toggleSize(['uniform'], 'heat')).toEqual(['heat']);
  expect(toggleSize(['heat'], 'links')).toEqual(['heat', 'links']);
  expect(toggleSize(['heat', 'links'], 'uniform')).toEqual(['uniform']);
  expect(toggleSize(['heat'], 'heat')).toEqual(['uniform']);
});

test('hilos en Burbujas; archivos y texto en Hilos', () => {
  expect([sizeApplies('threads', 'bubbles'), sizeApplies('threads', 'threads')]).toEqual([true, false]);
  expect([sizeApplies('files', 'threads'), sizeApplies('text', 'bubbles')]).toEqual([true, false]);
  const ts = [n('x', { words: 900, files: 4 }), n('y', { words: 100, files: 0 }), n('z', { words: 400, files: 1 })];
  const lk = encode(ts, [], enc({ size: ['text', 'files'] }), now).looks;
  expect(lk.get('x')!.size).toBe(2);
  expect(lk.get('y')!.size).toBe(0.7);
  expect(lk.get('z')!.size).toBeGreaterThan(0.7);
  // En Hilos, «cantidad de hilos» no aplica: es como «Igual».
  expect(new Set([...encode(ts, [], enc({ size: ['threads'] }), now).looks.values()].map((l) => l.size))).toEqual(new Set([1]));
  const bs = [{ ...n('p'), kind: 'bubbles' as const, threads: 9 }, { ...n('q'), kind: 'bubbles' as const, threads: 1 }];
  const lb = encode(bs, [], enc({ size: ['threads'] }), now).looks;
  expect(lb.get('p')!.size).toBeGreaterThan(lb.get('q')!.size);
});
