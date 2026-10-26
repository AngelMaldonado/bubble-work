import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

/** Ajustes → Equipos: crear un equipo y agregarle a alguien, con la API simulada.
 * Monta el componente real servido por Vite; ninguna petición llega al servidor.
 * node teams.mjs URL [playwright/index.mjs] [chromium] [ruta-del-componente] */
export async function checkTeams(browser, baseURL, component = '/src/components/SettingsTeams.svelte') {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname), 'sólo Vite local');
  const me = { id: 'fixturelead0001', display_name: 'Lead fixture', email: 'lead@example.test', role: 'lead' };
  const bea = { id: 'fixturebea00001', display_name: 'Bea', email: 'bea@example.test', role: 'member' };
  // Equipo A ya tiene a todos: su selector de «agregar» no se dibuja. El equipo
  // nuevo nace sólo con quien lo crea, así que el suyo sí: es el caso reportado.
  const teams = [{ id: 'fixtureteam0001', name: 'Equipo A', role: 'lead', personal: false,
    members: [{ user: me.id, name: me.display_name, role: 'lead', global: true }, { user: bea.id, name: 'Bea', role: 'member' }] }];
  const writes = [];
  const errors = [];
  const context = await browser.newContext({ reducedMotion: 'reduce' });
  // `api` lee el token al importarse: tiene que estar antes que cualquier módulo.
  await context.addInitScript(() => localStorage.setItem('bubble.token', 'fixture-token'));
  await context.route('**/api/**', async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname;
    let data = {};
    if (path.endsWith('/auth-refresh')) data = { token: 'fixture-token', record: me };
    else if (path === '/api/teams' && req.method() === 'POST') {
      const { name } = req.postDataJSON();
      writes.push(`create:${name}`);
      const id = `fixturenew${String(teams.length).padStart(5, '0')}`;
      teams.push({ id, name, role: 'lead', personal: false, members: [{ user: me.id, name: me.display_name, role: 'lead', global: true }] });
      data = { id, name };
    } else if (path === '/api/teams') data = { teams, default: teams[0].id };
    else if (/^\/api\/teams\/[^/]+\/members$/.test(path)) {
      const team = teams.find((t) => t.id === path.split('/')[3]);
      const { user, role } = req.postDataJSON();
      writes.push(`member:${team?.name}:${user}`);
      team?.members.push({ user, name: user === bea.id ? 'Bea' : user, role });
      data = { members: team?.members };
    } else if (path.startsWith('/api/collections/users/records')) data = { items: [me, bea], page: 1, perPage: 500, totalItems: 2, totalPages: 1 };
    else if (path.startsWith('/api/collections/prefs/records')) data = { items: [], page: 1, perPage: 1, totalItems: 0, totalPages: 0 };
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
  });
  await context.route('**/__teams_fixture*', (route) => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html lang="es" data-theme="bubble" data-mode="light"><head><meta charset="utf-8"></head><body style="background:var(--bg);color:var(--text);font-family:var(--typo-base--font-family)"><main id="target" style="max-width:52rem;margin:2rem auto"></main><script type="module">
    import '/src/app.css';
    import { mount } from '/node_modules/.vite/deps/svelte.js';
    import { api } from '/src/lib/api.ts';
    import Teams from '${component}';
    const me = await api.refresh();
    if (!me) throw new Error('fixture: auth-refresh simulado no devolvió persona');
    mount(Teams, { target: document.querySelector('#target'), props: { me } });
    window.__mounted = true;
  </script></body></html>` }));
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', (e) => errors.push(e.message));
  try {
    await page.goto(baseURL + '/__teams_fixture');
    await page.waitForFunction(() => window.__mounted);
    await page.getByRole('button', { name: 'Equipo A', exact: true }).waitFor();
    await page.getByPlaceholder('Nombre del equipo (p. ej. Operaciones)').fill('Operaciones');
    await page.getByRole('button', { name: 'Crear equipo', exact: true }).click();
    // El equipo nuevo se dibuja con su selector de «agregar» vacío.
    await page.getByRole('button', { name: 'Operaciones', exact: true }).waitFor().catch(() => {});
    await page.waitForTimeout(300);
    assert.deepEqual(errors, [], 'crear un equipo no tumba la pantalla');
    const picker = page.getByRole('combobox', { name: 'Agregar a Operaciones', exact: true });
    await picker.click();
    await page.locator('[data-scope="combobox"][data-part="item"]:visible', { hasText: 'Bea' }).click();
    await page.locator('form.add').filter({ has: picker }).getByRole('button', { name: 'Agregar', exact: true }).click();
    await page.waitForFunction(() => document.body.innerText.includes('Bea'));
    assert.deepEqual(writes, ['create:Operaciones', 'member:Operaciones:fixturebea00001']);
    assert.deepEqual(errors, []);
    return { ok: 'crear equipo y agregarle a alguien sin error de Svelte', writes };
  } finally {
    await context.close();
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [baseURL = 'http://localhost:5173', modulePath, executablePath, component] = process.argv.slice(2);
  const { chromium } = await import(modulePath ? pathToFileURL(resolve(modulePath)).href : 'playwright');
  const browser = await chromium.launch(executablePath ? { executablePath } : {});
  try { console.log(JSON.stringify(await checkTeams(browser, baseURL, component))); }
  finally { await browser.close(); }
}
