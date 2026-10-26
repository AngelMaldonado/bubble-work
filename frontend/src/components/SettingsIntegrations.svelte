<script lang="ts">
  // Integraciones: a qué otros sistemas avisa Bubble, y con qué.
  //
  // cuby.work es la primera. Deja trabajo aquí por /api/integrations con el token
  // de agente de una cuenta suya (Ajustes → Agente de esa cuenta), y se entera
  // por un webhook de cuándo terminó. Esto es lo segundo: los destinos, que
  // configura el lead global, como el webhook de los semáforos.
  import Loading from './Loading.svelte';
  import { api, type Webhook } from '../lib/api';
  import { ago } from '../lib/when';

  let hooks = $state<Webhook[]>([]);
  let events = $state<string[]>([]);
  let loaded = $state(false);
  let msg = $state('');
  let busy = $state('');
  let secret = $state('');
  let secretFor = $state('');
  let copied = $state(false);

  let name = $state('');
  let url = $state('');
  let skip = $state('');

  async function load() {
    try {
      const r = await api.webhooks();
      hooks = r.webhooks;
      events = r.events;
    } catch (e) {
      msg = (e as Error).message;
    } finally {
      loaded = true;
    }
  }
  load();

  async function run(what: string, p: () => Promise<unknown>) {
    busy = what;
    msg = '';
    try {
      await p();
      await load();
    } catch (e) {
      msg = (e as Error).message;
    } finally {
      busy = '';
    }
  }

  function create(e: SubmitEvent) {
    e.preventDefault();
    run('create', async () => {
      const r = await api.createWebhook({ name: name.trim(), url: url.trim(), events, skip_actor: skip.trim() });
      secret = r.secret;
      secretFor = r.name;
      name = url = skip = '';
    });
  }

  function toggleEvent(h: Webhook, ev: string) {
    const next = h.events.includes(ev) ? h.events.filter((x) => x !== ev) : [...h.events, ev];
    run(`ev:${h.id}`, () => api.saveWebhook(h.id, { events: next }));
  }

  async function copySecret() {
    try {
      await navigator.clipboard.writeText(secret);
      copied = true;
      setTimeout(() => (copied = false), 1800);
    } catch {
      msg = 'El navegador no dejó copiar al portapapeles.';
    }
  }
</script>

<section class="set-section">
  <p class="set-lead">
    Otro sistema —cuby.work, el primero— deja trabajo aquí con el token de agente de una cuenta suya y
    se entera de cuándo terminó por un webhook. Cada aviso va firmado
    (<code>X-Bubble-Signature</code>, HMAC-SHA256 del cuerpo) y se reintenta hasta que el receptor
    contesta 2xx.
  </p>

  {#if !loaded}
    <Loading label="Cargando integraciones" />
  {:else}
    {#if secret}
      <!-- La única vez que el secreto sale del servidor. -->
      <div class="set-card secret-box" role="status">
        <h3 class="m-0">«{secretFor}»</h3>
        <p class="help">Secreto para verificar las firmas. Cópialo ahora: no se vuelve a enseñar.</p>
        <code class="secret">{secret}</code>
        <div class="set-row">
          <button class="btn btn-sm preset-filled-primary-500" onclick={copySecret}>{copied ? '✓ copiado' : 'Copiar secreto'}</button>
          <button class="btn btn-sm preset-tonal-surface ml-auto" onclick={() => (secret = '')}>Ya lo guardé</button>
        </div>
      </div>
    {/if}

    {#each hooks as h (h.id)}
      <div class="set-card">
        <div class="set-row">
          <h3 class="m-0 grow">{h.name}</h3>
          <label class="toggle">
            <input type="checkbox" checked={h.active} disabled={!!busy}
              onchange={() => run(`active:${h.id}`, () => api.saveWebhook(h.id, { active: !h.active }))} />
            <span>{h.active ? 'Activo' : 'Pausado'}</span>
          </label>
        </div>
        <p class="help mono">{h.url}</p>
        <div class="set-row events">
          {#each events as ev (ev)}
            <label class="toggle">
              <input type="checkbox" checked={h.events.includes(ev)} disabled={!!busy} onchange={() => toggleEvent(h, ev)} />
              <code>{ev}</code>
            </label>
          {/each}
        </div>
        {#if h.skip_actor}
          <p class="help">No se le avisa lo que hace <b>{h.skip_actor}</b>: la cuenta de la integración no se oye a sí misma.</p>
        {/if}
        <div class="set-row">
          <button class="btn btn-sm preset-tonal-surface" disabled={!!busy} onclick={() => run(`test:${h.id}`, () => api.testWebhook(h.id))}>
            {#if busy === `test:${h.id}`}<Loading compact label="Enviando" />{:else}Enviar ping{/if}
          </button>
          <button class="btn btn-sm preset-tonal-surface" disabled={!!busy}
            onclick={() => run(`rotate:${h.id}`, async () => { const r = await api.saveWebhook(h.id, { rotate: true }); if (r.secret) { secret = r.secret; secretFor = r.name; } })}>
            Generar otro secreto
          </button>
          <button class="btn btn-sm preset-tonal-surface ml-auto" disabled={!!busy} onclick={() => run(`del:${h.id}`, () => api.deleteWebhook(h.id))}>
            Borrar
          </button>
        </div>
        <p class="help">
          {#if h.last_at}Último envío {ago(h.last_at)}: {h.last_status}.{:else}Todavía no ha enviado nada.{/if}
          {#if h.pending}{h.pending} pendiente{h.pending === 1 ? '' : 's'} de reintento.{/if}
          {#if h.dead}{h.dead} sin entregar tras todos los intentos.{/if}
        </p>
      </div>
    {/each}

    <form class="set-card" onsubmit={create}>
      <h3 class="m-0">Nuevo webhook</h3>
      <div class="fields">
        <input class="input" placeholder="Nombre, p. ej. cuby.work" maxlength="80" bind:value={name}
          autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
        <input class="input" type="url" placeholder="https://cuby.work/api/integrations/bubble/webhook" bind:value={url}
          autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
        <input class="input" type="email" placeholder="Correo de la cuenta de la integración (opcional)" bind:value={skip}
          autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
        <p class="help">Escucha todos los eventos que hay ({events.join(', ')}); se pueden quitar después.</p>
      </div>
      <div class="set-row">
        <button class="btn btn-sm preset-filled-primary-500" type="submit" disabled={!!busy || !name.trim() || !url.trim()}>
          {#if busy === 'create'}<Loading compact label="Creando" />{:else}Crear{/if}
        </button>
      </div>
    </form>
  {/if}
  {#if msg}<p class="set-err" role="alert">{msg}</p>{/if}
</section>

<style>
  .help { margin: 0; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  .mono { font-family: ui-monospace, monospace; word-break: break-all; }
  .grow { flex: 1; }
  .fields { display: flex; flex-direction: column; gap: 0.6rem; }
  .toggle { display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.85rem; color: var(--muted); }
  .events { flex-wrap: wrap; }
  .secret { display: block; padding: 0.5rem 0.7rem; border-radius: 8px; background: var(--hover); word-break: break-all; font-size: 0.8rem; }
</style>
