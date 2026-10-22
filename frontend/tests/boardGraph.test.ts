// Del tablero real a la vista Blobs: en qué blob (tag) cae cada punto y qué
// aristas se dibujan.
import { expect, test } from 'bun:test';
import { boardGraph, bubbleGroup, visibleNodes } from '../src/lib/boardGraph';
import { projectLinks } from '../src/lib/graph';
import type { Board } from '../src/lib/api';

const heat = (lifecycle: 'hot' | 'dormant' | 'rip' | 'closed') => ({ lifecycle, score: 0.5, reason: '', code: '' });
const board: Board = {
  workspace: '', at: '', tuning: {},
  workspaces: [
    { id: 'wa', slug: 'a', name: 'Alfa', team: 'ta' },
    { id: 'wb', slug: 'b', name: 'Beta', team: 'tb' },
  ],
  bubbles: [
    { id: 'b1', workspace: 'wa', name: 'Facturación', heat: heat('hot'), closed: false, priority: 'P1' },
    { id: 'b2', workspace: 'wa', name: 'Respaldo', heat: heat('dormant'), closed: false },
    { id: 'b3', workspace: 'wb', name: 'Red', heat: heat('rip'), closed: false },
    { id: 'b4', workspace: 'wa', name: 'Vieja', heat: heat('closed'), closed: true },
  ] as Board['bubbles'],
  threads: [
    { id: 't1', seq: 1, workspace: 'wa', name: 'Reglas', bubble: 'b1', heat: heat('dormant'), pulse: false, priority: 'P3', tags: ['pagos', 'clientes'] },
    { id: 't2', seq: 2, workspace: 'wa', name: 'Cobro', bubble: 'b1', heat: heat('hot'), pulse: false, tags: ['pagos'] },
    { id: 't3', seq: 3, workspace: 'wa', name: 'Suelto', heat: heat('hot'), pulse: false },
    { id: 't4', seq: 1, workspace: 'wb', name: 'Switch', bubble: 'b3', heat: heat('rip'), pulse: false, tags: ['infra'] },
    { id: 't5', seq: 4, workspace: 'wa', name: 'Copia', bubble: 'b2', heat: heat('hot'), pulse: false, tags: ['infra'] },
  ] as Board['threads'],
  links: [
    { from: 't1', to: 't2', kind: 'dependency' },
    { from: 't2', to: 't5', kind: 'related' },
    { from: 't1', to: 't5', kind: 'dependency' },
    { from: 't3', to: 't4', kind: 'related' },
  ],
};
const g = boardGraph(board, (w) => w === 'wa' ? 'Alfa' : 'Beta');

test('cada hilo va al blob de su primer tag; sin tags, a Sin tags', () => {
  expect(Object.fromEntries(g.threads.map((t) => [t.id, t.group]))).toEqual({ t1: 'pagos', t2: 'pagos', t3: '', t4: 'infra', t5: 'infra' });
});

test('cada burbuja va al primer tag más común de sus hilos; sin hilos con tags, a Sin tags', () => {
  expect(Object.fromEntries(g.bubbles.map((b) => [b.id, b.group]))).toEqual({ b1: 'pagos', b2: 'infra', b3: 'infra', b4: '' });
  expect(bubbleGroup(['b', 'a', 'b', ''])).toBe('b');
  expect(bubbleGroup(['b', 'a'])).toBe('a');
  expect(bubbleGroup(['', ''])).toBe('');
});

test('el mismo tag de dos equipos es el mismo blob', () => {
  const infra = g.threads.filter((t) => t.group === 'infra');
  expect(new Set(infra.map((t) => t.team))).toEqual(new Set(['ta', 'tb']));
});

test('los hilos conservan su calor y su prioridad, no los de su burbuja', () => {
  const t1 = g.threads.find((t) => t.id === 't1')!;
  expect([t1.life, t1.priority, t1.bubbleName]).toEqual(['dormant', 'P3', 'Facturación']);
});

test('los blobs van del más poblado al menos, y Sin tags al final', () => {
  expect(g.groups.map((c) => c.id)).toEqual(['infra', 'pagos', '']);
});

test('sin evaluador de furia, todo es calm', () => {
  expect(new Set([...g.bubbles, ...g.threads].map((n) => n.level))).toEqual(new Set(['calm']));
});

test('las cerradas sólo se ven si se piden', () => {
  expect(visibleNodes(g, 'bubbles', false).map((n) => n.id)).not.toContain('b4');
  expect(visibleNodes(g, 'bubbles', true).map((n) => n.id)).toContain('b4');
});

test('en Burbujas las relaciones se proyectan desde los hilos y se cuentan; nunca entre equipos', () => {
  const links = projectLinks('bubbles', visibleNodes(g, 'bubbles', false), g.threads, g.relations);
  expect(links).toEqual([
    { source: 'b1', target: 'b2', kind: 'related', count: 1 },
    { source: 'b1', target: 'b2', kind: 'dependency', count: 1 },
  ]);
  // t1→t2 queda dentro de b1 (no es arista); t3 no tiene burbuja; t3–t4 cruza equipos.
});

test('en Hilos, sólo entre extremos visibles', () => {
  const visible = visibleNodes(g, 'threads', false).filter((n) => n.id !== 't5');
  const links = projectLinks('threads', visible, g.threads, g.relations);
  expect(links).toEqual([{ source: 't1', target: 't2', kind: 'dependency', count: 1 }]);
});
