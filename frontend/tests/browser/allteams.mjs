import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

/** «Todos los equipos» en la app real, sin credenciales ni datos reales.
 *  Toda petición /api/ se intercepta con fixtures: un lead global con dos
 *  equipos compartidos y uno personal. Comprueba que la vista junta los
 *  compartidos y deja fuera lo personal, que no se guarda en `prefs.team`, que
 *  crear pide equipo, que la secuencia es la de un equipo elegido en su
 *  título (sin cambiar de equipo) y que el planeador pide entrar a uno. */
export async function checkAllTeams(page, baseURL) {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname), 'sólo un servidor local');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const now = new Date().toISOString();
  const me = { id: 'fixturelead0001', display_name: 'Lead fixture', email: 'lead@example.test', role: 'lead' };
  const teams = [
    { id: 'fixtureteam000a', name: 'Equipo A', role: 'lead', implicit: true, personal: false },
    { id: 'fixtureteam000b', name: 'Equipo B', role: 'lead', implicit: true, personal: false },
    { id: 'fixtureteam000p', name: 'Personal · Lead', role: 'lead', personal: true, owner: me.id },
  ];
  const tag = ['A', 'B', 'personal'];
  const workspaces = teams.map((t, i) => ({ id: `fixtureproject${i}`, slug: `fixture-${i}`, name: `Proyecto ${tag[i]}`, team: t.id }));
  const heat = { lifecycle: 'hot', score: 1, reason: 'evidencia fixture', code: 'producing' };
  const bubbles = workspaces.map((ws, i) => ({ id: `fixturebubble${i}`, workspace: ws.id, name: `Burbuja ${tag[i]}`, owners: [], heat, closed: false, warm_at: now }));
  const threads = workspaces.map((ws, i) => ({ id: `fixturethread0${i}`, seq: 1, workspace: ws.id, name: `Paso ${tag[i]}`, bubble: bubbles[i].id, heat, pulse: false, sequence: 1, assignees: [] }));
  let prefs = { team: teams[0].id };
  const prefWrites = [];
  const scopes = [];
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  // Una sola vez por sesión: la vista se recuerda entre recargas, y eso se prueba.
  await page.addInitScript(() => {
    if (!sessionStorage.getItem('fixture-started')) {
      localStorage.clear();
      sessionStorage.setItem('fixture-started', '1');
    }
    localStorage.setItem('bubble.token', 'fixture-token-no-real-credential');
  });
  const boardFor = (scope, id) => {
    const ws = workspaces.filter((w) => (id ? w.id === id : scope !== 'shared' || !teams.find((t) => t.id === w.team)?.personal));
    const ids = new Set(ws.map((w) => w.id));
    return {
      workspace: id || '', at: now, tuning: { cycle_hours: 168, decay_cycles: 2 },
      workspaces: ws.map((w) => ({ ...w, team_name: teams.find((t) => t.id === w.team)?.name })),
      bubbles: bubbles.filter((b) => ids.has(b.workspace)), threads: threads.filter((t) => ids.has(t.workspace)),
    };
  };
  await page.route('**/api/**', async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname;
    let data = {};
    if (path.endsWith('/auth-refresh')) data = { token: 'fixture-token-no-real-credential', record: me };
    else if (path === '/api/realtime') {
      if (req.method() === 'GET') {
        await route.fulfill({ contentType: 'text/event-stream', body: 'event: PB_CONNECT\ndata: {"clientId":"fixture"}\n\n' });
        return;
      }
    } else if (path === '/api/teams') data = { teams, default: teams[0].id };
    else if (path === '/api/version') data = { version: 'fixture', latest: 'fixture', stale: false, boot: false };
    else if (path === '/api/artifacts') data = { agent: { live: false }, artifacts: [] };
    else if (path === '/api/limits') data = {};
    else if (path === '/api/presence') data = { at: now };
    else if (path === '/api/board') {
      const scope = url.searchParams.get('scope') ?? '';
      scopes.push(scope);
      data = boardFor(scope, '');
    } else if (/^\/api\/workspaces\/[^/]+\/board$/.test(path)) data = boardFor('', path.split('/')[3]);
    // La proyección de lectura, como la da el servidor al lead: todo lo que alcanza.
    else if (path === '/api/planner/read') data = { board: boardFor('', ''), inbox: [], artifacts: [] };
    else if (path.startsWith('/api/collections/')) {
      const collection = path.split('/')[3];
      let items = [];
      if (collection === 'workspaces') items = workspaces;
      if (collection === 'users') items = [me];
      if (collection === 'prefs') {
        if (req.method() !== 'GET') {
          prefs = req.postDataJSON().value;
          prefWrites.push(prefs);
        }
        items = [{ id: 'fixtureprefs001', owner: me.id, value: prefs }];
      }
      if (collection === 'features') items = [{ id: 'fixturefeatures', sequence: true, space: false, roadmap: false }];
      if (!Object.keys(data).length) data = req.method() === 'GET'
        ? { items, totalItems: items.length, page: 1, perPage: 500, totalPages: 1 }
        : items[0] ?? {};
    } else {
      errors.push(`API no prevista: ${req.method()} ${path}`);
      await route.fulfill({ status: 500, json: { message: 'API no prevista en el fixture' } });
      return;
    }
    await route.fulfill({ json: data });
  });

  const orb = (name) => page.locator(`.orb[aria-label="${name}"]`);
  const healthy = async () => {
    const crashes = page.locator('.crash');
    const count = await crashes.count();
    assert.equal(count, 0, count ? await crashes.innerText() : 'sin crash');
    assert.deepEqual(errors, []);
  };
  const trigger = (label) => page.locator(`button[aria-label="${label}"]`);
  const sidebar = () => page.locator('.projects').innerText();

  // 1. Con un equipo elegido, sólo lo suyo.
  await page.goto(baseURL + '/todos');
  await orb('Burbuja A').waitFor();
  await page.waitForTimeout(250);
  assert.equal(await orb('Burbuja B').count(), 0, 'Equipo A no enseña lo de B');
  await healthy();

  // 2. Todos los equipos: A y B, nada personal; el servidor recorta.
  await trigger('Equipo en el que trabajas: Equipo A').click();
  await page.getByRole('menuitem', { name: /Todos los equipos/ }).click();
  await trigger('Vista: Todos los equipos').waitFor();
  await orb('Burbuja B').waitFor();
  assert.equal(await orb('Burbuja A').count(), 1);
  assert.equal(await orb('Burbuja personal').count(), 0, 'lo personal no entra');
  assert(scopes.includes('shared'), 'el tablero se pide con scope=shared');
  const side = await sidebar();
  assert(side.includes('Proyecto A') && side.includes('Proyecto B'), 'la columna lista los compartidos');
  assert(!side.includes('Proyecto personal'), 'la columna no lista lo personal');
  assert(await page.getByText('Todos los equipos', { exact: true }).first().isVisible());
  assert.deepEqual(prefWrites, [], '«Todos» no se guarda en prefs.team');
  await healthy();

  // 3. Crear pide equipo.
  await page.locator('button.menu-row').click();
  await page.getByRole('menuitem', { name: 'Nuevo proyecto en Equipo A' }).waitFor();
  assert.equal(await page.getByRole('menuitem', { name: 'Nuevo proyecto en Equipo B' }).count(), 1);
  // El planeador es de un equipo: en la vista se ve, deshabilitado y con el motivo.
  const planner = page.getByRole('menuitem', { name: /Planeador/ });
  assert.equal(await planner.getAttribute('aria-disabled'), 'true', 'el planeador no se elige desde Todos');
  assert.match(await planner.innerText(), /entra a un equipo/);
  assert.equal(await page.getByRole('menuitem', { name: 'Nuevo proyecto', exact: true }).count(), 0, 'sin equipo no se crea');
  assert.equal(await page.getByRole('menuitem', { name: /Personal · Lead/ }).count(), 0, 'no ofrece crear en lo personal desde la vista');
  await page.keyboard.press('Escape');

  // 4. Secuencia: la de UN equipo en el lienzo entero; el título elige cuál.
  // Elegir no es entrar: no escribe prefs, y sobrevive la recarga.
  await page.goto(baseURL + '/secuencia');
  await page.locator('section[aria-label="Secuencia de Equipo A"]').waitFor();
  assert.equal(await page.locator('section[aria-label^="Secuencia de"]').count(), 1, 'un solo lienzo');
  const reader = page.locator('.sequence-reader');
  await page.getByRole('button', { name: 'Secuencia del equipo: Equipo A' }).click();
  assert.equal(await page.getByRole('menuitem', { name: /Personal/ }).count(), 0, 'lo personal no se ofrece');
  await page.getByRole('menuitem', { name: 'Equipo B' }).click();
  await page.locator('section[aria-label="Secuencia de Equipo B"]').waitFor();
  await reader.getByText('Paso B').first().waitFor();
  assert.equal(await reader.getByText('Paso personal').count(), 0, 'ningún paso personal en la secuencia');
  assert.deepEqual(prefWrites, [], 'elegir en el título no cambia de equipo');
  await page.reload();
  await page.locator('section[aria-label="Secuencia de Equipo B"]').waitFor();
  await healthy();
  await page.getByRole('button', { name: 'Entrar a Equipo B' }).click();
  await trigger('Equipo en el que trabajas: Equipo B').waitFor();
  // La preferencia se escribe después de cambiar la vista: se espera, no se supone.
  for (let i = 0; i < 50 && !prefWrites.length; i++) await page.waitForTimeout(100);
  assert.equal(await page.getByRole('button', { name: /^Secuencia del equipo/ }).count(), 0, 'dentro del equipo el título no es un menú');
  assert.deepEqual(prefWrites, [{ team: teams[1].id }], 'entrar a un equipo sí es la preferencia');
  await healthy();

  // 5. El planeador es de un equipo: desde la vista se pide entrar a uno.
  await trigger('Equipo en el que trabajas: Equipo B').click();
  await page.getByRole('menuitem', { name: /Todos los equipos/ }).click();
  await trigger('Vista: Todos los equipos').waitFor();
  await page.goto(baseURL + '/planeador');
  await page.getByText('El planeador es de un equipo').waitFor();
  assert.equal(await page.getByRole('button', { name: 'Entrar a Equipo A' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: /Entrar a Personal/ }).count(), 0);
  await healthy();
  assert.deepEqual(prefWrites, [{ team: teams[1].id }], 'volver a la vista no escribe prefs');
  return { ok: 'vista, recorte del servidor, columna, crear con equipo, secuencia con equipo en el título, planeador', scopes: [...new Set(scopes)] };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [baseURL = 'http://localhost:5173', modulePath, executablePath] = process.argv.slice(2);
  const { chromium } = await import(modulePath ? pathToFileURL(resolve(modulePath)).href : 'playwright');
  const browser = await chromium.launch(executablePath ? { executablePath } : {});
  try {
    const page = await (await browser.newContext({ viewport: { width: 1300, height: 900 } })).newPage();
    page.setDefaultTimeout(15000);
    try { console.log(JSON.stringify(await checkAllTeams(page, baseURL))); }
    catch (e) {
      await page.screenshot({ path: '/tmp/bw-allteams-failure.png', fullPage: true }).catch(() => {});
      console.error((await page.locator('body').innerText().catch(() => '')).slice(0, 3000));
      throw e;
    }
  } finally { await browser.close(); }
}
