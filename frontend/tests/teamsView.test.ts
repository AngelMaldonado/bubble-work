// «Todos los equipos»: una vista del lead global que junta los equipos
// compartidos. Lo personal no entra (tampoco el propio), nunca va a
// `prefs.team`, y entrar a un equipo sale de la vista.
import { afterAll, beforeAll, beforeEach, describe, expect, test } from 'bun:test';

const hadLocalStorage = 'localStorage' in globalThis;
const prevLocalStorage = (globalThis as Record<string, unknown>).localStorage;
// El store usa runas; fuera del compilador de Svelte, `$state` es el valor.
(globalThis as Record<string, unknown>).$state = <T>(v: T) => v;
const store = new Map<string, string>();
(globalThis as Record<string, unknown>).localStorage = {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => void store.set(k, v),
  removeItem: (k: string) => void store.delete(k),
};

type Mod = typeof import('../src/lib/teams.svelte');
let teams: Mod['teams'];
let api: typeof import('../src/lib/api')['api'];
const prefsWrites: unknown[] = [];
// `api` es un singleton que otros archivos de prueba usan: se devuelve como estaba.
const saved: Record<string, unknown> = {};

const TEAMS = [
  { id: 'ti', name: 'TI', personal: false, role: 'lead' as const, implicit: true },
  { id: 'ops', name: 'Operaciones', personal: false, role: 'lead' as const, implicit: true },
  { id: 'mine', name: 'Personal · Carol', personal: true, owner: 'carol', role: 'lead' as const },
];

beforeAll(async () => {
  ({ teams } = await import('../src/lib/teams.svelte'));
  ({ api } = await import('../src/lib/api'));
  const a = api as unknown as Record<string, unknown>;
  for (const k of ['teams', 'prefs', 'setPrefs', 'me']) saved[k] = Object.getOwnPropertyDescriptor(a, k);
  a.teams = async () => ({ teams: TEAMS, default: 'ti' });
  a.prefs = async () => ({ team: 'ops' });
  a.setPrefs = async (p: unknown) => void prefsWrites.push(p);
});

afterAll(() => {
  const a = api as unknown as Record<string, unknown>;
  for (const [k, d] of Object.entries(saved)) {
    if (d) Object.defineProperty(a, k, d as PropertyDescriptor);
    else delete a[k];
  }
  const g = globalThis as Record<string, unknown>;
  if (hadLocalStorage) g.localStorage = prevLocalStorage;
  else delete g.localStorage;
});

beforeEach(async () => {
  store.clear();
  prefsWrites.length = 0;
  (api as unknown as { me: unknown }).me = { id: 'carol', email: 'c@x', role: 'lead' };
  await teams.load();
});

describe('Todos los equipos', () => {
  test('se ofrece al lead global con dos equipos compartidos', () => {
    expect(teams.canAll).toBe(true);
    expect(teams.sharedTeams.map((t) => t.id)).toEqual(['ti', 'ops']);
  });

  test('no se ofrece a un member', async () => {
    (api as unknown as { me: unknown }).me = { id: 'bob', email: 'b@x', role: 'member' };
    await teams.load();
    teams.showAll(true);
    expect(teams.canAll).toBe(false);
    expect(teams.viewing).toBe(false);
  });

  test('junta los compartidos y deja fuera lo personal, también el propio', () => {
    teams.showAll(true);
    expect(teams.inScope({ team: 'ti' })).toBe(true);
    expect(teams.inScope({ team: 'ops' })).toBe(true);
    expect(teams.inScope({ team: 'mine' })).toBe(false);
    // un equipo fuera de la lista de un lead global es el personal de otro
    expect(teams.inScope({ team: 'personal-de-gina' })).toBe(false);
    expect(teams.inScope({})).toBe(true);
    expect(teams.inView('ops')).toBe(true);
    expect(teams.inView('mine')).toBe(false);
    expect(teams.inView(undefined)).toBe(false);
  });

  test('fuera de la vista, sólo el equipo elegido y lo invitado de otro', () => {
    expect(teams.current).toBe('ops');
    expect(teams.inScope({ team: 'ops' })).toBe(true);
    expect(teams.inScope({ team: 'ti' })).toBe(false);
    expect(teams.inScope({ team: 'mine' })).toBe(false);
    expect(teams.inScope({ team: 'personal-de-gina' })).toBe(true);
    expect(teams.inView('ops')).toBe(true);
    expect(teams.inView('ti')).toBe(false);
  });

  test('nunca va a prefs.team, y se recuerda aparte por persona', async () => {
    teams.showAll(true);
    expect(prefsWrites).toEqual([]);
    expect(teams.current).toBe('ops');
    expect(store.get('bw.team-view.carol')).toBe('all');
    await teams.load();
    expect(teams.viewing).toBe(true);
  });

  test('entrar a un equipo sale de la vista, también al que ya era el actual', async () => {
    teams.showAll(true);
    await teams.set('ops');
    expect(teams.viewing).toBe(false);
    expect(prefsWrites).toEqual([]);
    teams.showAll(true);
    await teams.set('ti');
    expect(teams.viewing).toBe(false);
    expect(prefsWrites).toEqual([{ team: 'ti' }]);
  });
});
