import { expect, test } from 'bun:test';
import { agentHealth, short } from '../src/lib/agentHealth';
import type { AgentToken } from '../src/lib/api';

const cur = { operators: 'aaaaaaaaaaaaaaaa', planner: 'pppppppppppppppp', connect: 'cccccccccccccccc' };
const base: AgentToken = {
  id: 't', owner: 'u', name: 'claude', prefix: 'bw_x', expires_at: '', revoked_at: '',
  created: '2026-10-01 00:00:00.000Z', last_used_at: '2026-10-21 10:00:00.000Z',
};
const keys = (t: Partial<AgentToken>, c: typeof cur | null = cur) =>
  agentHealth({ ...base, ...t }, c)?.chips.map((x) => x.key) ?? null;

test('sin uso, revocado o sin releases del servidor: no dice nada', () => {
  expect(keys({ last_used_at: '' })).toBeNull();
  expect(keys({ revoked_at: '2026-10-20 00:00:00.000Z' })).toBeNull();
  expect(keys({}, null)).toBeNull();
});

test('usado sin leer agents_md', () => {
  expect(keys({})).toEqual(['never']);
});

test('al día con la release de su rol', () => {
  expect(keys({ release_read: cur.operators, release_role: 'operators', connect_version: cur.connect })).toEqual(['current']);
  expect(keys({ release_read: cur.planner, release_role: 'planner' })).toEqual(['current']);
});

test('la de otro rol no cuenta como vigente', () => {
  expect(keys({ release_read: cur.operators, release_role: 'planner' })).toEqual(['stale']);
});

test('release anterior, e instalación vieja aparte', () => {
  const h = agentHealth({ ...base, release_read: 'viejaviejavieja1', release_role: 'operators', connect_version: 'otra' }, cur)!;
  expect(h.release).toBe('stale');
  expect(h.install).toBe('old');
  expect(h.chips.map((c) => c.key)).toEqual(['stale', 'old']);
  expect(h.chips[0].hint).toContain(short(cur.operators));
});

test('sin cabecera de instalación no se inventa nada', () => {
  const h = agentHealth({ ...base, release_read: cur.operators, release_role: 'operators' }, cur)!;
  expect(h.install).toBe('unknown');
  expect(h.chips.map((c) => c.key)).toEqual(['current']);
});

test('short son siete caracteres', () => {
  expect(short('0123456789abcdef')).toBe('0123456');
  expect(short(undefined)).toBe('');
});
