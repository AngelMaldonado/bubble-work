import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

/** Regresión de navegador sin credenciales ni datos reales.
 * Un runner con Playwright llama checkPeople(page, 'http://localhost:5173').
 * Todas las peticiones /api/ se interceptan: jamás se escriben datos del dev. */
export async function checkPeople(page, baseURL) {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname), 'sólo un servidor local');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const now = new Date().toISOString();
  const me = { id: 'fixturelead0001', display_name: 'Lead fixture', email: 'lead@example.test', role: 'lead' };
  const hidden = { id: 'fixturehidden01', display_name: '' }; // correo deliberadamente ausente
  const visible = { id: 'fixturevisible1', display_name: 'Ángela', email: 'visible@example.test' };
  const people = [me, hidden, visible];
  const teams = [
    { id: 'fixtureteam0001', name: 'Equipo A', role: 'lead', personal: false },
    { id: 'fixtureteam0002', name: 'Equipo B', role: 'lead', personal: false },
  ];
  const workspaces = teams.map((team, i) => ({ id: `fixtureproject${i}`, slug: `fixture-${i}`, name: `Proyecto ${i}`, team: team.id }));
  const heat = { lifecycle: 'hot', score: 1, reason: 'evidencia fixture', code: 'producing' };
  const bubbles = workspaces.map((ws, i) => ({ id: `fixturebubble${i}`, workspace: ws.id, name: `Personas ${i}`, owners: [], heat, closed: false, warm_at: now }));
  let prefs = { team: teams[0].id };
  const writes = [];
  const errors = [];
  const collections = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.addInitScript(() => {
    localStorage.clear();
    localStorage.setItem('bubble.token', 'fixture-token-no-real-credential');
  });
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
    else if (path === '/api/board' || /^\/api\/workspaces\/[^/]+\/board$/.test(path)) {
      const id = path.split('/')[3];
      data = { workspace: id || '', at: now, tuning: { cycle_hours: 168, decay_cycles: 2 }, workspaces, bubbles: id ? bubbles.filter((b) => b.workspace === id) : bubbles, threads: [] };
    } else if (path.startsWith('/api/collections/')) {
      const collection = path.split('/')[3];
      collections.push(collection);
      let items = [];
      if (collection === 'workspaces') items = workspaces;
      if (collection === 'users') items = people;
      if (collection === 'prefs') {
        if (req.method() !== 'GET') prefs = req.postDataJSON().value;
        items = [{ id: 'fixtureprefs001', value: prefs }];
      }
      if (collection === 'features') items = [{ id: 'fixturefeatures', sequence: false, space: false, roadmap: false }];
      if (collection === 'memberships') items = people.map((p, i) => ({ id: `fixturemember${i}`, user: p.id, role: p.role || 'member', expand: { user: p } }));
      if (collection === 'bubbles') {
        const bubble = bubbles.find((b) => b.id === path.split('/')[5]);
        if (req.method() === 'PATCH') {
          assert(bubble, 'la asignación apunta a una burbuja del fixture');
          const patch = req.postDataJSON();
          writes.push(patch);
          Object.assign(bubble, patch);
        }
        if (bubble) data = bubble;
        else items = bubbles;
      }
      if (!Object.keys(data).length) data = { items, totalItems: items.length, page: 1, perPage: 500, totalPages: 1 };
    } else {
      errors.push(`API no prevista: ${req.method()} ${path}`);
      await route.fulfill({ status: 500, json: { message: 'API no prevista en el fixture' } });
      return;
    }
    await route.fulfill({ json: data });
  });

  const stable = async () => {
    await page.waitForFunction(() => document.querySelector('.orb') || document.querySelector('.crash'));
    await page.waitForTimeout(250);
  };
  const healthy = async () => {
    const crashes = page.locator('.crash');
    const count = await crashes.count();
    assert.equal(count, 0, count ? await crashes.innerText() : 'sin crash');
    assert.deepEqual(errors, []);
  };
  await page.goto(baseURL + '/todos');
  await stable();
  await healthy();
  assert(collections.includes('users'), 'Todos leyó el directorio incompleto');
  await page.locator('.orb[aria-label="Personas 0"]').click();
  await page.locator('.drawer').waitFor({ state: 'visible' });
  await healthy();
  await page.keyboard.press('Escape');

  await page.goto(baseURL + '/w/fixture-0');
  await stable();
  await healthy();
  await page.locator('.orb[aria-label="Personas 0"]').click();
  const drawer = page.locator('.drawer');
  await drawer.waitFor({ state: 'visible' });
  const box = drawer.getByRole('combobox');
  await box.click();
  await box.fill('sin nombre');
  const option = page.locator('.who-list [data-part="item"]').filter({ hasText: /sin nombre/ });
  await option.waitFor({ state: 'visible' });
  await healthy();
  await option.click();
  await page.waitForFunction(() => !!document.querySelector('button[aria-label="quitar a sin nombre"]'));
  assert.deepEqual(writes.at(-1)?.owners, [hidden.id], 'se conserva la identidad de la persona sin nombre');
  assert.equal('email' in hidden, false, 'no se amplían datos para resolver el nombre');
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');

  await page.getByRole('button', { name: 'Equipo en el que trabajas: Equipo A', exact: true }).click();
  await page.getByRole('menuitem', { name: /Equipo B/ }).click();
  await page.waitForFunction(() => document.querySelector('button[aria-label="Equipo en el que trabajas: Equipo B"]'));
  await healthy();
  await page.locator('.orb[aria-label="Personas 1"]').click();
  await page.locator('.drawer').waitFor({ state: 'visible' });
  await healthy();
  return { ok: 'Todos, cajón, búsqueda/respaldo, asignación por id y cambio de equipo', writes: writes.length };
}

/** Inyecta una opción incompleta directamente al componente real: demuestra la
 * defensa del cajón aunque un consumidor omita la normalización del adaptador.
 * Requiere Vite (módulos fuente); no se usa contra un bundle desplegado. */
export async function checkIncompleteDrawer(page, baseURL) {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname), 'sólo un servidor local');
  await page.route('**/__people_drawer_fixture', (route) => route.fulfill({
    contentType: 'text/html', body: '<!doctype html><html><head><meta charset="utf-8"></head><body><main id="target"></main></body></html>',
  }));
  await page.goto(baseURL + '/__people_drawer_fixture');
  await page.evaluate(async () => {
    // La misma instancia optimizada que usa el componente compilado por Vite.
    const { mount } = await import('/node_modules/.vite/deps/svelte.js');
    const { default: Drawer } = await import('/src/components/BubbleDrawer.svelte');
    window.__fixtureOwners = null;
    mount(Drawer, { target: document.querySelector('#target'), props: {
      open: true, name: 'Opciones incompletas', people: [{ id: 'missing', avatar: '' }],
      onowners: (ids) => { window.__fixtureOwners = ids; },
    } });
  });
  const box = page.locator('.drawer').getByRole('combobox');
  await box.click();
  await box.fill('sin nombre');
  const option = page.locator('.who-list [data-part="item"]').filter({ hasText: /sin nombre/ });
  await option.waitFor({ state: 'visible' });
  await option.click();
  assert.deepEqual(await page.evaluate(() => window.__fixtureOwners), ['missing']);
  return { ok: 'el componente real tolera un name omitido y conserva el id al seleccionar' };
}

/** Contraste con el predicado anterior, sólo interceptando el módulo en este
 * navegador. No revierte archivos ni altera la sesión de dev de otra persona. */
export async function checkPreviousFilter(page, baseURL) {
  let mutated = false;
  const handler = async (route) => {
    const response = await route.fetch();
    const original = await response.text();
    const body = original.replace(/export function matchingPeople\([\s\S]*?\n\}/, `export function matchingPeople(people, query) {\n  return people.filter((p) => p.name.toLowerCase().includes(query.trim().toLowerCase()));\n}`);
    assert.notEqual(body, original, 'el fixture debe usar el predicado anterior');
    mutated = true;
    await route.fulfill({ response, body });
  };
  await page.route('**/api/**', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/src/lib/people.ts*', handler);
  const pageError = new Promise((resolve) => page.once('pageerror', resolve));
  try {
    const check = checkIncompleteDrawer(page, baseURL)
      .then(() => new Error('el filtro anterior no falló')).catch((e) => e);
    const error = await Promise.race([pageError, check]);
    assert.match(error.message, /Cannot read properties of undefined \(reading 'toLowerCase'\)/);
    assert(mutated);
    return { ok: 'el componente real reproduce toLowerCase con la expresión anterior y name omitido' };
  } finally {
    await page.unroute('**/src/lib/people.ts*', handler);
  }
}

// Playwright es una dependencia del runner, no del producto. Se puede usar el
// ya instalado por el entorno indicando su index.mjs y, si hace falta, Chrome.
if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [baseURL = 'http://localhost:5173', modulePath, executablePath] = process.argv.slice(2).filter((arg) => arg !== '--source');
  const { chromium } = await import(modulePath ? pathToFileURL(resolve(modulePath)).href : 'playwright');
  const browser = await chromium.launch(executablePath ? { executablePath } : {});
  try {
    const page = await browser.newPage({ viewport: { width: 1300, height: 900 } });
    page.setDefaultTimeout(15000);
    console.log(JSON.stringify(await checkPeople(page, baseURL)));
    if (process.argv.includes('--source')) {
      const previous = await browser.newPage();
      previous.setDefaultTimeout(15000);
      try { console.log(JSON.stringify(await checkPreviousFilter(previous, baseURL))); }
      finally { await previous.close(); }
      console.log(JSON.stringify(await checkIncompleteDrawer(page, baseURL)));
    }
  } finally {
    await browser.close();
  }
}
