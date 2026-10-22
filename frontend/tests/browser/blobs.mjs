import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

/** La vista Blobs en la app real, con toda /api/ simulada. Sólo servidor local.
 * Los blobs son los TAGS del frontmatter que el servidor indexa por hilo. No
 * son pruebas de ACL ni del índice: eso lo prueba scripts/test.sh. */
export async function checkBlobs(page, baseURL) {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname));
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const now = new Date().toISOString();
  const me = { id: 'fixturelead0001', display_name: 'Lead fixture', email: 'lead@example.test', role: 'lead' };
  const teams = [
    { id: 'fixtureteam000a', name: 'Equipo A', role: 'lead', personal: false },
    { id: 'fixtureteam000b', name: 'Equipo B', role: 'lead', personal: false },
    { id: 'fixtureteam000p', name: 'Personal · Lead', role: 'lead', personal: true, owner: me.id },
  ];
  const workspaces = teams.map((t, i) => ({ id: `fixtureproject${i}`, slug: `fixture-${i}`, name: `Proyecto ${i}`, team: t.id }));
  const heat = (lifecycle = 'hot') => ({ lifecycle, score: 1, reason: 'evidencia fixture', code: 'producing' });
  const bubbles = [
    { id: 'fixturebubblea1', workspace: workspaces[0].id, name: 'Cobros A' },
    { id: 'fixturebubblea2', workspace: workspaces[0].id, name: 'Respaldos A' },
    { id: 'fixturebubbleb1', workspace: workspaces[1].id, name: 'Cobros B' },
    { id: 'fixturebubbleb2', workspace: workspaces[1].id, name: 'Respaldos B' },
    { id: 'fixturebubblep1', workspace: workspaces[2].id, name: 'Sólo personal' },
    { id: 'fixturebubblea3', workspace: workspaces[0].id, name: 'Terminada A', closed: true },
  ].map((b) => ({ owners: [], warm_at: now, closed: false, ...b, heat: heat(b.closed ? 'closed' : 'hot') }));
  // Un hilo por burbuja, con los tags de su frontmatter (ya indexados).
  const tagsOf = { 'Cobros A': ['pagos', 'clientes'], 'Respaldos A': ['infra'], 'Cobros B': ['pagos'], 'Respaldos B': ['infra'], 'Sólo personal': ['secreto'], 'Terminada A': ['pagos'] };
  const threads = bubbles.map((b, i) => ({ id: `fixturethread${i}x`, workspace: b.workspace, bubble: b.id, seq: i + 1, name: `Hilo ${b.name}`, heat: heat(b.closed ? 'closed' : 'dormant'), pulse: false, assignees: [], priority: 'P3', tags: tagsOf[b.name], words: 40 + i * 90, files: i % 4 }));
  threads.push({ id: 'fixtureloose001', workspace: workspaces[0].id, bubble: '', seq: 7, name: 'Hilo suelto', heat: heat(), pulse: false, assignees: [], priority: 'P2' });
  const links = [
    { from: threads[0].id, to: threads[1].id, kind: 'dependency' },
    { from: threads[2].id, to: threads[3].id, kind: 'related' },
  ];
  const boardFor = (id, shared) => {
    const ws = workspaces.filter((w) => (!id || w.id === id) && (!shared || w.team !== teams[2].id));
    const ids = new Set(ws.map((w) => w.id));
    const ts = threads.filter((t) => ids.has(t.workspace));
    const tids = new Set(ts.map((t) => t.id));
    return { workspace: id, at: now, tuning: { cycle_hours: 168, decay_cycles: 2 },
      workspaces: ws.map((w) => ({ ...w, team_name: teams.find((t) => t.id === w.team)?.name })),
      bubbles: bubbles.filter((b) => ids.has(b.workspace)), threads: ts,
      links: links.filter((l) => tids.has(l.from) && tids.has(l.to)),
    };
  };
  let prefs = { team: teams[0].id };
  const errors = [], unexpected = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.addInitScript(() => {
    if (!sessionStorage.getItem('blobs-fixture-started')) {
      localStorage.clear();
      sessionStorage.setItem('blobs-fixture-started', '1');
    }
    localStorage.setItem('bubble.token', 'fixture-token');
  });
  await page.route('**/api/**', async (route) => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname;
    let data = {};
    if (path.endsWith('/auth-refresh')) data = { token: 'fixture-token', record: me };
    else if (path === '/api/realtime') {
      if (req.method() === 'GET') {
        await route.fulfill({ contentType: 'text/event-stream', body: 'event: PB_CONNECT\ndata: {"clientId":"fixture"}\n\n' });
        return;
      }
    } else if (path === '/api/teams') data = { teams, default: teams[0].id };
    else if (path === '/api/version') data = { version: 'fixture', latest: 'fixture', stale: false, boot: false };
    else if (path === '/api/artifacts') data = { agent: { live: false }, artifacts: [] };
    else if (path === '/api/presence') data = { at: now };
    else if (path === '/api/limits') data = {};
    else if (path === '/api/board') data = boardFor('', url.searchParams.get('scope') === 'shared');
    else if (/^\/api\/workspaces\/[^/]+\/board$/.test(path)) data = boardFor(path.split('/')[3], false);
    else if (/\/tree$/.test(path)) data = { entries: [] };
    else if (/\/changes$/.test(path)) data = { changes: [] };
    else if (/^\/api\/threads\/[^/]+\/document$/.test(path)) {
      const t = threads.find((t) => t.id === path.split('/')[3]);
      data = { workspace: t.workspace, thread: t.id, path: `threads/${t.seq}-fixture.md`, content: '# Fixture', html: '<h1>Fixture</h1>', hash: 'fixturehash', done: 0 };
    } else if (path.startsWith('/api/collections/')) {
      const collection = path.split('/')[3], id = path.split('/')[5];
      let items = [];
      if (collection === 'workspaces') items = workspaces;
      if (collection === 'users') items = [me];
      if (collection === 'memberships') items = [{ id: 'fixturemember01', user: me.id, role: 'lead', expand: { user: me } }];
      if (collection === 'features') items = [{ id: 'fixturefeatures', sequence: false, space: false, roadmap: false }];
      if (collection === 'bubbles') items = bubbles;
      if (collection === 'prefs') {
        if (req.method() !== 'GET') prefs = req.postDataJSON().value;
        items = [{ id: 'fixtureprefs001', owner: me.id, value: prefs }];
      }
      if (collection === 'contexts') unexpected.push('contexts: ya no existen');
      data = id ? items.find((c) => c.id === id) ?? {} : { items, totalItems: items.length, page: 1, perPage: 500, totalPages: 1 };
    } else {
      unexpected.push(`${req.method()} ${path}`);
      await route.fulfill({ status: 500, json: { message: 'API no prevista' } }); return;
    }
    await route.fulfill({ json: data });
  });
  const healthy = async () => {
    assert.deepEqual(errors, []);
    assert.deepEqual(unexpected, []);
    assert.equal(await page.locator('.crash').count(), 0);
  };
  const graph = () => page.locator('.blob-board .graph');
  const stableBox = async (loc) => {
    let last = null;
    for (let i = 0; i < 40; i++) {
      const box = await loc.boundingBox();
      if (box && last && Math.abs(box.x - last.x) < 0.5 && Math.abs(box.y - last.y) < 0.5) return box;
      last = box;
      await page.waitForTimeout(100);
    }
    throw new Error('la posición del nodo no se asienta');
  };
  const blob = (id) => graph().locator(`[data-group="${id}"]`);

  await page.goto(baseURL + '/todos');
  await page.getByRole('button', { name: 'Burbujas', exact: true }).waitFor();
  assert.equal(await page.locator('.orb[aria-label="Cobros A"]').count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Contextos', exact: true }).count(), 0, 'ya no hay disposición Contextos');
  await page.getByRole('button', { name: 'Blobs', exact: true }).click();
  await blob('pagos').waitFor();
  // Equipo A: Cobros A va a «pagos» (su hilo lo pone primero), Respaldos A a «infra».
  assert.equal(await blob('pagos').locator('[data-piece]').count(), 1);
  assert.equal(await blob('infra').locator('[data-piece]').count(), 1);
  assert.equal(await graph().locator('[data-link]').count(), 1);
  assert.equal(await graph().locator('[data-fury]').count(), 0, 'sin evaluador, no hay aro ficticio');
  // Los cursores de los lienzos (app.css): mano abierta, puntero en un nodo y
  // mano cerrada mientras se arrastra.
  const cursorOf = (loc) => loc.evaluate((el) => getComputedStyle(el).cursor);
  assert.match(await cursorOf(graph().locator('svg')), /grab-(light|dark)/);
  assert.match(await cursorOf(graph().getByRole('button', { name: 'Abrir Cobros A', exact: true })), /pointer-(light|dark)/);
  const box = await graph().locator('svg').boundingBox();
  await page.mouse.move(box.x + 30, box.y + box.height - 160);
  await page.mouse.down();
  await page.mouse.move(box.x + 90, box.y + box.height - 200, { steps: 4 });
  assert.match(await cursorOf(graph().locator('svg')), /grabbing-(light|dark)/);
  await page.mouse.up();
  // Los puntos no se mueven: arrastrar sobre uno mueve la cámara. Y crecen con el hover.
  const piece = graph().locator('[data-piece="fixturebubblea1"]');
  const at = await piece.getAttribute('transform');
  const hit = await graph().getByRole('button', { name: 'Abrir Cobros A', exact: true }).boundingBox();
  await page.mouse.move(hit.x + hit.width / 2, hit.y + hit.height / 2);
  await page.waitForTimeout(450);
  const grown = await piece.locator('.ball').evaluate((el) => Number(getComputedStyle(el).transform.match(/^matrix\(([\d.]+)/)?.[1] ?? 1));
  assert(grown >= 1.8, `crece con el hover (escala ${grown})`);
  assert.equal(await graph().locator('.node.emphasized').count(), 1);
  await page.mouse.down();
  await page.mouse.move(hit.x + hit.width / 2 + 60, hit.y + hit.height / 2 + 40, { steps: 5 });
  await page.mouse.up();
  assert.equal(await piece.getAttribute('transform'), at, 'un nodo no se arrastra');
  assert.equal(await page.locator('.drawer').count() && await page.locator('.drawer').isVisible(), false, 'arrastrar no abre el cajón');
  await healthy();

  // El cajón abre desde el grafo, y ya no ofrece clasificar por contexto.
  await graph().getByRole('button', { name: 'Abrir Cobros A', exact: true }).press('Enter');
  await page.locator('.drawer').waitFor({ state: 'visible' });
  assert.equal(await page.getByRole('combobox', { name: 'Contexto de la burbuja' }).count(), 0);
  await page.locator('.drawer [data-part="close-trigger"]').click();
  await page.locator('.drawer').waitFor({ state: 'hidden' });

  // Todos los equipos: el mismo tag de dos equipos es un solo blob, y nada personal.
  await page.getByRole('button', { name: 'Equipo en el que trabajas: Equipo A', exact: true }).click();
  await page.getByRole('menuitem', { name: /Todos los equipos/ }).click();
  await blob('infra').waitFor();
  assert.equal(await blob('pagos').locator('[data-piece]').count(), 2);
  // Hover y luego clic con el ratón: abre. (Hubo una versión que reordenaba
  // el nodo en el DOM con el hover y a veces perdía el click.)
  // Al entrar a Todos el tablero se recarga y se reencuadra: medir antes de que
  // se asiente apunta al vacío.
  const again = await stableBox(graph().getByRole('button', { name: 'Abrir Cobros A', exact: true }));
  await page.mouse.move(again.x + again.width / 2, again.y + again.height / 2);
  await page.waitForTimeout(400);
  await page.mouse.down();
  await page.mouse.up();
  await page.locator('.drawer').waitFor({ state: 'visible' });
  await page.locator('.drawer [data-part="close-trigger"]').click();
  await page.locator('.drawer').waitFor({ state: 'hidden' });
  assert.equal(await blob('infra').locator('[data-piece]').count(), 2);
  assert.equal(await blob('secreto').count(), 0);
  assert.equal(await graph().getByText('Sólo personal', { exact: true }).count(), 0);
  await page.getByRole('button', { name: 'Incluir cerradas', exact: true }).click();
  await graph().getByRole('button', { name: 'Abrir Terminada A', exact: true }).waitFor();
  const position = await graph().locator('[data-piece="fixturebubblea1"]').getAttribute('transform');
  await page.getByRole('button', { name: 'Incluir cerradas', exact: true }).click();
  assert.equal(await graph().locator('[data-piece="fixturebubblea1"]').getAttribute('transform'), position, 'las cerradas no mueven a nadie');

  // Hilos: cada uno en el blob de su primer tag; el suelto sin tags, en «Sin tags».
  await page.getByRole('button', { name: 'Hilos', exact: true }).click();
  await blob('none').waitFor();
  assert.equal(await blob('clientes').count(), 0, 'sólo cuenta el primer tag');
  await graph().getByRole('button', { name: 'Abrir Hilo suelto', exact: true }).press('Enter');
  await page.waitForURL('**/w/fixture-0/t/7');
  await healthy();

  // Tamaño y color: el panel cambia lo que dicen las burbujas y filtra.
  await page.goto(baseURL + '/todos');
  await graph().waitFor();
  await page.getByRole('button', { name: 'Burbujas', exact: true }).click();
  await page.getByRole('button', { name: /^Tamaño y color/ }).click();
  const panel = page.getByRole('dialog', { name: 'Tamaño, color y filtros', exact: true });
  await panel.getByRole('button', { name: 'Proyecto', exact: true }).click();
  await page.getByRole('complementary', { name: 'Leyenda de tamaño y color' }).getByText('Color: Proyecto').waitFor();
  const before = await page.locator('.blob-board [data-piece]').count();
  await panel.getByRole('button', { name: 'Ocultar lo demás', exact: true }).click();
  await panel.getByRole('textbox', { name: 'Buscar en las burbujas' }).fill('cobros');
  await page.waitForFunction(() => document.querySelectorAll('.blob-board [data-piece]').length === 2);
  await page.getByText(`2 de ${before} pasan el filtro`).waitFor();
  await panel.getByRole('button', { name: 'Restablecer', exact: true }).click();
  await page.waitForFunction((n) => document.querySelectorAll('.blob-board [data-piece]').length === n, before);
  // Tamaño con varios atributos (en Hilos): se suman, la leyenda los nombra y
  // «Igual» los quita. Cantidad de hilos sólo existe en Burbujas.
  await page.getByRole('button', { name: 'Hilos', exact: true }).click();
  assert.equal(await panel.getByRole('button', { name: 'Cantidad de hilos', exact: true }).count(), 0);
  await panel.getByRole('button', { name: 'Archivos', exact: true }).click();
  await panel.getByRole('button', { name: 'Texto', exact: true }).click();
  await page.getByText('Tamaño: Archivos + Texto').waitFor();
  const radii = await page.locator('.blob-board .ball .base').evaluateAll((els) => new Set(els.map((e) => e.getAttribute('r'))).size);
  assert(radii > 1, 'los tamaños varían');
  await panel.getByRole('button', { name: 'Igual', exact: true }).click();
  assert.equal(await panel.getByRole('button', { name: 'Archivos', exact: true }).getAttribute('aria-pressed'), 'false');
  await page.getByRole('button', { name: 'Burbujas', exact: true }).click();
  await panel.getByRole('button', { name: 'Cantidad de hilos', exact: true }).waitFor();
  await panel.getByRole('button', { name: 'Cerrar', exact: true }).click();
  await healthy();

  // 3D: se dibuja, el cursor encuentra una burbuja y un clic la abre.
  await page.getByRole('button', { name: '3D', exact: true }).click();
  const canvas3d = page.locator('.g3d canvas');
  await canvas3d.waitFor();
  // El gizmo: tres ejes debajo del zoom; pulsar uno gira la vista hacia él.
  assert.equal(await page.locator('.gizmo .axis-hit').count(), 3);
  const yBefore = await page.getByRole('button', { name: 'Ver desde arriba (eje Y)' }).getAttribute('style');
  await page.getByRole('button', { name: 'Ver desde arriba (eje Y)' }).click();
  await page.waitForTimeout(450);
  assert.notEqual(await page.getByRole('button', { name: 'Ver desde arriba (eje Y)' }).getAttribute('style'), yBefore, 'el gizmo gira con la vista');
  await page.getByRole('button', { name: 'Ver de frente (eje Z)' }).click();
  await page.waitForTimeout(450);
  // Invertir el arrastre: el mismo gesto gira al revés, y se recuerda.
  const xOf = async () => parseFloat(await page.getByRole('button', { name: /eje X/ }).evaluate((e) => e.style.left));
  const drag3d = async () => {
    const bx = await page.locator('.g3d canvas').boundingBox();
    await page.mouse.move(bx.x + bx.width / 2, bx.y + bx.height / 2);
    await page.mouse.down();
    await page.mouse.move(bx.x + bx.width / 2 + 80, bx.y + bx.height / 2, { steps: 6 });
    await page.mouse.up();
    await page.waitForTimeout(150);
  };
  let x0 = await xOf(); await drag3d(); const normalTurn = (await xOf()) - x0;
  await page.getByRole('button', { name: 'Invertir el arrastre', exact: true }).click();
  x0 = await xOf(); await drag3d(); const invertedTurn = (await xOf()) - x0;
  assert(normalTurn !== 0 && Math.sign(normalTurn) === -Math.sign(invertedTurn), `invertido gira al revés (${normalTurn}, ${invertedTurn})`);
  await page.reload();
  await page.locator('.g3d canvas').waitFor();
  assert.equal(await page.getByRole('button', { name: /Arrastre invertido/ }).getAttribute('aria-pressed'), 'true', 'se recuerda');
  await page.getByRole('button', { name: /Arrastre invertido/ }).click();
  const c3 = await canvas3d.boundingBox();
  let hit3d = '';
  for (let y = c3.y + 120; y < c3.y + c3.height - 120 && !hit3d; y += 10) {
    for (let x = c3.x + 120; x < c3.x + c3.width - 120 && !hit3d; x += 10) {
      await page.mouse.move(x, y);
      hit3d = await page.locator('.g3d').getAttribute('data-hover');
    }
  }
  assert(hit3d, 'el cursor encuentra una burbuja en 3D');
  await page.mouse.down();
  await page.mouse.up();
  await page.locator('.drawer').waitFor({ state: 'visible' });
  await page.locator('.drawer [data-part="close-trigger"]').click();
  await page.locator('.drawer').waitFor({ state: 'hidden' });
  await healthy();
  await page.getByRole('button', { name: 'Blobs', exact: true }).click();
  await page.getByRole('button', { name: 'Hilos', exact: true }).click();

  // Regreso y recarga conservan nivel y disposición.
  await page.goto(baseURL + '/todos');
  await graph().waitFor();
  assert.equal(await page.getByRole('button', { name: 'Hilos', exact: true }).getAttribute('aria-pressed'), 'true');
  await page.getByRole('button', { name: 'Burbujas', exact: true }).click();

  // Ajustes ya no tiene Contextos.
  await page.goto(baseURL + '/ajustes');
  await page.getByText('Equipos', { exact: true }).first().waitFor();
  assert.equal(await page.getByText('Contextos', { exact: true }).count(), 0);
  await healthy();

  // Viewport estrecho: sin scroll horizontal y con los controles a mano.
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(baseURL + '/todos');
  await graph().waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
  await graph().getByRole('button', { name: 'Acercar mapa', exact: true }).click();
  await healthy();
  return { ok: 'Blobs por tag, niveles, aristas, cajón, Todos sin lo personal, cerradas, tamaño y color, 3D, preferencias, cursores, sin contextos y móvil' };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [baseURL = 'http://localhost:5173', modulePath, executablePath] = process.argv.slice(2);
  const { chromium } = await import(modulePath ? pathToFileURL(resolve(modulePath)).href : 'playwright');
  const browser = await chromium.launch(executablePath ? { executablePath } : {});
  try {
    const page = await (await browser.newContext({ viewport: { width: 1300, height: 900 } })).newPage();
    page.setDefaultTimeout(15000);
    try { console.log(JSON.stringify(await checkBlobs(page, baseURL))); }
    catch (e) {
      await page.screenshot({ path: '/tmp/bw-blobs-failure.png', fullPage: true }).catch(() => {});
      console.error((await page.locator('body').innerText().catch(() => '')).slice(0, 3000)); throw e;
    }
  } finally { await browser.close(); }
}
