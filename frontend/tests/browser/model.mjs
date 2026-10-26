import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { mkdir } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

/** La URL directa de revisión monta fixtures, incluso si hay una sesión previa. */
export async function checkModelRoute(browser, baseURL) {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname), 'sólo Vite local');
  const context = await browser.newContext();
  const apiRequests = [];
  const errors = [];
  await context.addInitScript(() => localStorage.setItem('bubble.token', 'fixture-token-no-real-credential'));
  await context.route('**/api/**', (route) => {
    apiRequests.push(route.request().url());
    return route.abort();
  });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', (e) => errors.push(e.message));
  try {
    await page.goto(baseURL + '/theme/model');
    await page.locator('.prototype').waitFor();
    assert.match(await page.title(), /Prototipo del modelo/);
    await page.getByRole('button', { name: 'Blobs', exact: true }).click();
    assert.equal(await page.locator('[data-group="clientes"] [data-piece]').count(), 2);
    await page.getByRole('button', { name: 'Furia de Facturación: Furiosa', exact: true }).press('Enter');
    await page.getByRole('dialog').waitFor({ state: 'visible' });
    await page.keyboard.press('Escape');
    await page.reload();
    await page.locator('.prototype').waitFor();
    assert.equal(await page.getByRole('button', { name: 'Blobs', exact: true }).getAttribute('aria-pressed'), 'true');
    await page.goto(baseURL + '/theme/model/');
    await page.locator('.prototype').waitFor();
    assert.deepEqual(apiRequests, [], 'la URL directa no monta la app ni usa su sesión/API');
    assert.deepEqual(errors, []);
    return { ok: '/theme/model directo, interacción, recarga y barra final; sesión simulada no dispara API', apiRequests: 0 };
  } finally {
    await context.close();
  }
}

/** /theme/mock: la app maqueta con el modelo encima (sólo DEV). */
export async function checkMockApp(browser, baseURL, output) {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname), 'sólo Vite local');
  const context = await browser.newContext({ viewport: { width: 1440, height: 950 }, reducedMotion: 'reduce' });
  const errors = [];
  await context.route('**/api/**', (route) => route.abort());
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', (e) => errors.push(e.message));
  const shot = async (name) => {
    await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
    await page.screenshot({ path: resolve(output, name + '.png') });
  };
  const ttl = () => page.getByRole('textbox', { name: 'renombrar' });
  try {
    await page.goto(baseURL + '/theme/mock');
    await page.getByRole('tab', { name: 'Hilos', exact: true }).waitFor();
    assert.equal(await page.getByRole('button', { name: 'Flotabilidad', exact: true }).getAttribute('aria-pressed'), 'true', 'default Flotabilidad');
    const orb = (name) => page.locator('button.orb').and(page.getByRole('button', { name, exact: true }));
    assert(await orb('Facturación electrónica').isVisible(), 'orbes de burbuja');
    assert.equal(await page.getByRole('button', { name: 'Facturación electrónica', exact: true }).count(), 2, 'minimapa presente en Burbujas/Flotabilidad');

    await page.getByRole('tab', { name: 'Hilos', exact: true }).click();
    assert.equal(await page.getByRole('button', { name: 'Facturación electrónica', exact: true }).count(), 0, 'sin minimapa de burbujas en Hilos');
    await orb('Timbrado CFDI 4.0').click();
    await ttl().waitFor();
    assert.equal((await ttl().innerText()).trim(), 'Timbrado CFDI 4.0', 'abre la vista de hilo de ese hilo');
    await page.getByRole('button', { name: 'volver al board', exact: true }).click();

    await page.getByRole('button', { name: 'Blobs', exact: true }).click();
    await page.locator('.graph[data-map-kind="threads"]').waitFor();
    assert.equal(await page.locator('.graph [data-group="clientes"]').count(), 1);
    await shot('mock-hilos');
    await page.getByRole('button', { name: 'Abrir Cancelaciones', exact: true }).click();
    await ttl().waitFor();
    assert.equal((await ttl().innerText()).trim(), 'Cancelaciones');
    await page.getByRole('button', { name: 'volver al board', exact: true }).click();

    await page.getByRole('tab', { name: 'Burbujas', exact: true }).click();
    await page.locator('.graph[data-map-kind="bubbles"]').waitFor();
    assert.equal(await page.locator('[data-piece="auditor-a-iso-9001"]').count(), 0, 'cerradas fuera del grafo');
    await shot('mock-blobs');
    await page.getByRole('button', { name: 'Furia de Facturación electrónica: Furiosa', exact: true }).press('Enter');
    const dialog = page.getByRole('dialog');
    await dialog.waitFor({ state: 'visible' });
    assert.match(await dialog.innerText(), /inamovible/);
    await shot('mock-furia');
    await page.getByRole('button', { name: 'Abrir burbuja', exact: true }).click();
    await page.locator('.mock-model-detail').waitFor({ state: 'hidden' });
    await page.getByRole('dialog').filter({ hasText: 'Facturación electrónica' }).waitFor({ state: 'visible' });
    assert.equal(await page.getByRole('dialog').filter({ hasText: 'inamovible' }).count(), 0, 'la explicación cede al cajón');
    await page.keyboard.press('Escape');

    await page.reload();
    await page.locator('.graph[data-map-kind="bubbles"]').waitFor();
    assert.equal(await page.getByRole('button', { name: 'Blobs', exact: true }).getAttribute('aria-pressed'), 'true', 'recuerda disposición');
    await page.evaluate(() => { document.documentElement.dataset.mode = 'dark'; document.documentElement.style.colorScheme = 'dark'; });
    await shot('mock-blobs-dark');
    assert.deepEqual(errors, []);
    return { ok: '/theme/mock: Burbujas/Hilos × Blobs/Flotabilidad, vista de hilo, cajón, furia y persistencia' };
  } finally {
    await context.close();
  }
}

/** Prototipo aislado servido por Vite, sin datos del servidor.
 * Node model.mjs URL [playwright/index.mjs] [chromium] [salida-de-capturas]. */
export async function checkModel(browser, baseURL, output, { review = false } = {}) {
  assert(['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname), 'sólo Vite local');
  await mkdir(output, { recursive: true });
  const context = await browser.newContext({ viewport: { width: 1360, height: 1000 }, reducedMotion: 'reduce' });
  const apiRequests = [];
  const errors = [];
  await context.route('**/api/**', (route) => {
    apiRequests.push(route.request().url());
    return route.abort();
  });
  await context.route('**/__model_fixture*', (route) => {
    const globalLead = new URL(route.request().url()).searchParams.get('member') !== '1';
    return route.fulfill({ contentType: 'text/html', body: `<!doctype html><html lang="es" data-theme="bubble" data-mode="light"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="background:var(--bg);color:var(--text);font-family:var(--typo-base--font-family);min-height:100dvh;margin:0"><main id="target"></main><script type="module">
      import '/src/app.css';
      import '/node_modules/@fontsource/boogaloo/400.css';
      import '/node_modules/@fontsource-variable/dm-sans/index.css';
      import { mount } from '/node_modules/.vite/deps/svelte.js';
      import Model from '/src/prototypes/ModelEvolution.svelte';
      mount(Model, { target: document.querySelector('#target'), props: { globalLead: ${globalLead} } });
    </script></body></html>` });
  });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', (e) => errors.push(e.message));
  const mount = async (globalLead = true) => {
    await page.goto(baseURL + '/__model_fixture' + (globalLead ? '' : '?member=1'));
    await page.locator('.prototype').waitFor();
    await page.evaluate(() => document.fonts.ready);
  };
  const pick = async (label, value) => {
    await page.getByRole('combobox', { name: label, exact: true }).click();
    await page.locator(`[data-scope="combobox"][data-part="item"][data-value="${value}"]:visible`).click();
  };
  const snapshot = async (name) => {
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await page.screenshot({ path: resolve(output, name + '.png'), fullPage: true });
  };
  try {
    await mount();
    assert.equal(await page.getByRole('button', { name: 'Flotabilidad', exact: true }).getAttribute('aria-pressed'), 'true');
    await page.getByRole('button', { name: 'Blobs', exact: true }).click();
    assert.equal(await page.locator('[data-group="clientes"] [data-piece]').count(), 2);
    await snapshot('blobs-light');

    await page.getByRole('searchbox').fill('Fact');
    await page.getByRole('button', { name: 'Sólo mías' }).click();
    await page.getByRole('button', { name: 'Flotabilidad', exact: true }).click();
    assert.equal(await page.locator('[data-piece]').count(), 1);
    assert.equal(await page.locator('[data-band="dormant"] [data-piece="billing"]').count(), 1, 'furia no cambia calor');
    assert.equal(await page.getByRole('searchbox').inputValue(), 'Fact');
    assert.equal(await page.getByRole('button', { name: 'Sólo mías' }).getAttribute('aria-pressed'), 'true');
    await page.getByRole('button', { name: 'Furia de Facturación: Furiosa' }).click();
    await page.getByRole('dialog').waitFor({ state: 'visible' });
    assert.match(await page.getByRole('dialog').innerText(), /máximo de las causas/);
    assert.match(await page.getByRole('dialog').innerText(), /48 h/);
    await snapshot('explicacion-light');
    await page.keyboard.press('Escape');
    await page.getByRole('searchbox').fill('');
    await page.getByRole('button', { name: 'Sólo mías' }).click();
    await page.getByRole('button', { name: 'Blobs', exact: true }).click();
    await pick('Equipo del prototipo', 'all');
    assert.equal(await page.locator('.prototype').getAttribute('data-scope'), 'all');
    assert.equal(await page.locator('[data-group="clientes"] [data-piece]').count(), 3);
    // El mismo tag de dos equipos es un solo blob.
    assert.equal(await page.locator('[data-group="infra"]').count(), 1);
    assert.equal(await page.locator('.team-plans article').count(), 2);
    await page.evaluate(() => { document.documentElement.dataset.mode = 'dark'; document.documentElement.style.colorScheme = 'dark'; });
    await snapshot('todos-dark');

    // Nivel y disposición son ejes independientes: tabs reales, mismo filtro.
    await page.getByRole('tab', { name: 'Hilos', exact: true }).click();
    assert.equal(await page.getByRole('tab', { name: 'Hilos', exact: true }).getAttribute('aria-selected'), 'true');
    assert.equal(await page.locator('.graph[data-map-kind="threads"]').count(), 1);
    assert(await page.locator('[data-piece]').count() > 6);
    await snapshot('hilos-dark');
    const beforeZoom = await page.locator('.graph svg').getAttribute('viewBox');
    await page.getByRole('button', { name: 'Acercar mapa', exact: true }).click();
    assert.notEqual(await page.locator('.graph svg').getAttribute('viewBox'), beforeZoom);
    await page.getByRole('button', { name: 'Encuadrar', exact: true }).click();
    const canvas = await page.locator('.graph svg').boundingBox();
    const beforePan = await page.locator('.graph svg').getAttribute('viewBox');
    await page.mouse.move(canvas.x + 10, canvas.y + canvas.height / 2);
    await page.mouse.down();
    await page.mouse.move(canvas.x + 45, canvas.y + canvas.height / 2 + 25, { steps: 5 });
    await page.mouse.up();
    assert.notEqual(await page.locator('.graph svg').getAttribute('viewBox'), beforePan, 'arrastrar fondo mueve cámara');
    const node = await page.getByRole('button', { name: 'Abrir Contrato de API', exact: true }).boundingBox();
    const beforeDrag = await page.locator('[data-piece="integration:1"]').getAttribute('transform');
    const viewBeforeDrag = await page.locator('.graph svg').getAttribute('viewBox');
    await page.mouse.move(node.x + node.width / 2, node.y + node.height / 2);
    await page.mouse.down();
    await page.mouse.move(node.x + node.width / 2 + 30, node.y + node.height / 2 + 20, { steps: 5 });
    await page.mouse.up();
    // Los puntos no se mueven: arrastrar sobre uno mueve la cámara.
    assert.equal(await page.locator('[data-piece="integration:1"]').getAttribute('transform'), beforeDrag, 'un nodo no se arrastra');
    assert.notEqual(await page.locator('.graph svg').getAttribute('viewBox'), viewBeforeDrag, 'arrastrar sobre un nodo mueve la cámara');
    assert.equal(await page.getByRole('dialog').count(), 0, 'un arrastre no abre detalle');
    await page.getByRole('button', { name: 'Encuadrar', exact: true }).click();
    const position = await page.locator('[data-piece="billing:1"]').getAttribute('transform');
    await page.getByRole('searchbox').fill('Fact');
    await page.getByRole('button', { name: 'Sólo mías', exact: true }).click();
    assert.equal(await page.locator('[data-piece="billing:1"]').getAttribute('transform'), position, 'buscar no rebaraja posiciones');
    await page.getByRole('button', { name: 'Abrir Reglas de cobro', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'visible' });
    assert.match(await page.getByRole('dialog').innerText(), /Hilo de Facturación/);
    await page.waitForFunction(() => !!document.activeElement?.closest('[role="dialog"]'));
    await page.keyboard.press('Escape');
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await page.getByRole('tab', { name: 'Burbujas', exact: true }).click();
    assert.equal(await page.getByRole('searchbox').inputValue(), 'Fact');
    assert.equal(await page.getByRole('button', { name: 'Sólo mías', exact: true }).getAttribute('aria-pressed'), 'true');
    assert.equal(await page.locator('[data-piece]').count(), 1);
    assert.equal(await page.locator('[data-link]').count(), 0, 'no hay aristas con extremo oculto');
    await page.getByRole('tab', { name: 'Burbujas', exact: true }).focus();
    await page.keyboard.press('ArrowRight');
    await page.waitForFunction(() => document.activeElement?.getAttribute('data-value') === 'threads');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => document.querySelector('[data-scope="tabs"][data-value="threads"][data-part="trigger"]')?.getAttribute('aria-selected') === 'true');
    await page.getByRole('button', { name: 'Flotabilidad', exact: true }).click();
    assert.equal(await page.locator('[data-band="dormant"] [data-piece="billing:1"]').count(), 1);
    await page.getByRole('tab', { name: 'Burbujas', exact: true }).click();
    await page.getByRole('button', { name: 'Blobs', exact: true }).click();
    await page.getByRole('searchbox').fill('');
    await page.getByRole('button', { name: 'Sólo mías', exact: true }).click();

    await page.getByRole('button', { name: 'Crear…', exact: true }).click();
    assert(await page.getByRole('button', { name: 'Usar este equipo', exact: true }).isDisabled());
    await pick('Equipo de destino del prototipo', 'b');
    await page.getByRole('button', { name: 'Usar este equipo', exact: true }).click();
    assert.equal(await page.locator('.prototype').getAttribute('data-scope'), 'b');
    assert.match(await page.getByRole('status').innerText(), /No se creó ningún registro/);
    assert.equal(await page.locator('[data-piece="billing"]').count(), 0);
    assert.equal(await page.locator('[data-piece="onboarding"]').count(), 1);

    await pick('Equipo del prototipo', 'a');
    await pick('Estado del prototipo', 'all');
    assert.equal(await page.locator('[data-piece="closed"]').count(), 1);
    await pick('Estado del prototipo', 'open');
    assert.equal(await page.locator('[data-piece="closed"]').count(), 0);
    await page.keyboard.press('Tab');
    assert(await page.evaluate(() => document.activeElement !== document.body), 'hay navegación por teclado');
    await mount();
    assert.equal(await page.getByRole('button', { name: 'Blobs', exact: true }).getAttribute('aria-pressed'), 'true', 'se conserva la preferencia del fixture');
    await page.setViewportSize({ width: 390, height: 844 });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'sin desborde horizontal');
    await snapshot('blobs-phone-light');
    await page.evaluate(() => { document.documentElement.dataset.mode = 'dark'; document.documentElement.style.colorScheme = 'dark'; });
    await snapshot('blobs-phone-dark');
    await page.getByRole('tab', { name: 'Hilos', exact: true }).click();
    await snapshot('hilos-phone-dark');
    await mount(false);
    assert.equal(await page.getByRole('tab', { name: 'Hilos', exact: true }).getAttribute('aria-selected'), 'true', 'nivel recordado en la preferencia del fixture');
    await page.getByRole('combobox', { name: 'Equipo del prototipo', exact: true }).click();
    assert.equal(await page.locator('[data-scope="combobox"][data-part="item"][data-value="all"]:visible').count(), 0, 'el fixture miembro no ofrece Todos');
    await page.keyboard.press('Escape');
    assert.deepEqual(apiRequests, [], 'el prototipo no hace llamadas API');
    assert.deepEqual(errors, []);
    if (review) {
      await page.setViewportSize({ width: 1360, height: 1000 });
      await mount();
      console.log('Revisión del prototipo: interactúa en esta pestaña; ciérrala para terminar. No es una ruta del producto.');
      await page.waitForEvent('close', { timeout: 0 });
    }
    return { ok: 'grafo Burbujas/Hilos, tabs/teclado, filtros, zoom/pan/arrastre, detalle, Todos, creación con equipo, cerradas, persistencia, móvil y miembro', screenshots: output, apiRequests: 0 };
  } finally {
    await context.close();
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const review = process.argv.includes('--review');
  const [baseURL = 'http://localhost:5173', modulePath, executablePath, output = 'docs/prototypes/model-evolution'] = process.argv.slice(2).filter((arg) => arg !== '--review');
  const { chromium } = await import(modulePath ? pathToFileURL(resolve(modulePath)).href : 'playwright');
  const browser = await chromium.launch({ ...(executablePath ? { executablePath } : {}), headless: !review });
  try {
    console.log(JSON.stringify(await checkModelRoute(browser, baseURL)));
    console.log(JSON.stringify(await checkMockApp(browser, baseURL, output)));
    console.log(JSON.stringify(await checkModel(browser, baseURL, output, { review })));
  }
  finally { await browser.close(); }
}
