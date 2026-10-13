import { afterAll, afterEach, expect, test } from 'bun:test';

const originalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: { getItem: () => null, setItem() {}, removeItem() {} } });
const { api } = await import('../src/lib/api');
const fetchOriginal = globalThis.fetch;
afterEach(() => { globalThis.fetch = fetchOriginal; api.me = null; });
afterAll(() => { if (originalStorage) Object.defineProperty(globalThis, 'localStorage', originalStorage); else delete (globalThis as any).localStorage; });

function mock(initial: any = { planner: 'tasks' }, failRead = false) {
  let row: any = initial === null ? null : { id: 'prefs1234567890', value: initial };
  let created = 0, written = 0;
  api.me = { id: 'person123456789', email: 'test@example.invalid' };
  globalThis.fetch = (async (_url: unknown, options?: RequestInit) => {
    const method = options?.method ?? 'GET';
    if (method === 'GET') {
      if (failRead) return Response.json({ message: 'read failed' }, { status: 503 });
      return Response.json({ items: row ? [row] : [] });
    }
    await new Promise((resolve) => setTimeout(resolve, 8));
    const body = JSON.parse(String(options?.body));
    if (method === 'POST') { created++; row = { id: 'prefs1234567890', ...body }; }
    else { written++; row = { ...row, ...body }; }
    return Response.json(row);
  }) as typeof fetch;
  return { row: () => row, counts: () => ({ created, written }) };
}

test('cambiar el modo no borra pins ni otras preferencias', async () => {
  const server = mock({ planner: 'tasks', thread_pins: ['abcdefghijklmno'], view: 'pool' });
  await api.setPrefs({ planner: 'bubbles' });
  expect(server.row().value).toEqual({ planner: 'bubbles', thread_pins: ['abcdefghijklmno'], view: 'pool' });
});
test('modo y pin simultáneos se fusionan sin pisarse', async () => {
  const server = mock({ planner: 'tasks' });
  await Promise.all([api.setPrefs({ thread_pins: ['abcdefghijklmno'] }), api.setPrefs({ planner: 'bubbles' })]);
  expect(server.row().value).toEqual({ planner: 'bubbles', thread_pins: ['abcdefghijklmno'] });
});
test('un fallo de lectura aborta: no sustituye el saco por uno vacío', async () => {
  const server = mock({ thread_pins: ['abcdefghijklmno'] }, true);
  await expect(api.setPrefs({ planner: 'bubbles' })).rejects.toThrow('read failed');
  expect(server.counts()).toEqual({ created: 0, written: 0 });
});
test('dos primeros cambios concurrentes crean una única fila personal', async () => {
  const server = mock(null);
  await Promise.all([api.setPrefs({ planner: 'tasks' }), api.setPrefs({ thread_pins: ['abcdefghijklmno'] })]);
  expect(server.counts()).toEqual({ created: 1, written: 1 });
  expect(server.row().value).toEqual({ planner: 'tasks', thread_pins: ['abcdefghijklmno'] });
});
