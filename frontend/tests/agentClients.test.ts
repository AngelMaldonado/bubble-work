import { expect, test } from 'bun:test';
import { agentsByPerson, brandOf } from '../src/lib/agentClients';
import type { AgentToken } from '../src/lib/api';

test('marca y versión salen del clientInfo', () => {
  expect(brandOf('claude-code 2.1.304')).toMatchObject({ key: 'claude', slug: 'claude', version: '2.1.304' });
  expect(brandOf('codex-mcp-client 0.160.0')).toMatchObject({ key: 'codex', mono: 'Cx', version: '0.160.0' });
  expect(brandOf('antigravity 1.11.2')).toMatchObject({ key: 'antigravity', mono: 'Ag' });
  expect(brandOf('antigravity 1.11.2').slug).toBeUndefined();
  expect(brandOf('Visual Studio Code 1.105.0')).toMatchObject({ key: 'copilot' });
  expect(brandOf('gemini-cli-mcp-client 0.9')).toMatchObject({ key: 'gemini', slug: 'googlegemini' });
  expect(brandOf('pi 1.3.0')).toMatchObject({ key: 'pi', mono: 'π' });
});

test('un cliente desconocido va con su inicial, sin inventar marca', () => {
  expect(brandOf('mi-agente')).toMatchObject({ key: 'other:mi-agente', name: 'mi-agente', mono: 'M', version: '' });
});

const cur = { operators: 'opsopsopsopsops1', planner: 'plnplnplnplnpln1', connect: 'c' };
const tok = (o: Partial<AgentToken>): AgentToken => ({
  id: Math.random().toString(36), owner: 'ana', name: 'x', prefix: 'bw_', expires_at: '', revoked_at: '',
  created: '2026-10-01', last_used_at: '2026-10-21 10:00:00.000Z', ...o,
});

test('agrupa por persona y por marca, lo más reciente primero', () => {
  const by = agentsByPerson([
    tok({ client: 'claude-code 2.1.290', last_used_at: '2026-10-20 10:00:00.000Z' }),
    tok({ client: 'claude-code 2.1.304', last_used_at: '2026-10-21 09:00:00.000Z' }),
    tok({ client: 'codex-mcp-client 0.160.0', last_used_at: '2026-10-21 11:00:00.000Z' }),
    tok({ owner: 'luis', client: 'antigravity 1.0' }),
  ], cur);
  expect(by.ana.clients.map((c) => [c.key, c.version, c.tokens])).toEqual([['codex', '0.160.0', 1], ['claude', '2.1.304', 2]]);
  expect(Object.keys(by).sort()).toEqual(['ana', 'luis']);
});

test('fuera los revocados y los que nunca se usaron', () => {
  const by = agentsByPerson([
    tok({ client: 'claude-code 1', revoked_at: '2026-10-21 00:00:00.000Z' }),
    tok({ owner: 'sofia', client: '', last_used_at: '' }),
  ], cur);
  expect(by).toEqual({});
});

test('la release más reciente que le llegó, y si es la vigente de su rol', () => {
  const by = agentsByPerson([
    tok({ client: 'codex 1', release_read: 'viejaviejavieja1', release_role: 'operators', release_read_at: '2026-10-19 00:00:00.000Z' }),
    tok({ client: 'claude-code 2', release_read: cur.operators, release_role: 'operators', release_read_at: '2026-10-21 00:00:00.000Z' }),
    tok({ owner: 'angel', client: 'claude-code 2', release_read: cur.operators, release_role: 'planner', release_read_at: '2026-10-21 00:00:00.000Z' }),
    tok({ owner: 'marta', client: 'antigravity 1' }),
  ], cur);
  expect(by.ana.release).toMatchObject({ id: cur.operators, current: true });
  expect(by.angel.release?.current).toBe(false); // la de otro rol no es la suya
  expect(by.marta).toMatchObject({ release: null, neverRead: true });
  expect(agentsByPerson([tok({ client: 'x', release_read: cur.operators, release_read_at: 'a' })], null).ana.release?.current).toBe(false);
});
