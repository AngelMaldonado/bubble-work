<script lang="ts">
  // Tokens: las llaves con que los agentes de una persona entran al MCP.
  //
  // Un token es del agente, no de la sesión: se revoca uno sin cerrar nada más,
  // caduca cuando se decidió (o nunca) y dice cuándo se usó por última vez. El
  // servidor guarda sólo su huella, así que el token se enseña UNA vez, al
  // generarlo, junto al prompt que conecta al asistente.
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import { api, type AgentToken, type Person } from '../lib/api';
  import { setupPrompt } from '../lib/connect';
  import { ago, when } from '../lib/when';
  import Confirm, { type Doom } from './Confirm.svelte';

  let { me }: { me: Person } = $props();
  const isLead = $derived(me.role === 'lead');

  type Row = AgentToken & { ownerName: string };
  let rows = $state<Row[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let doom = $state<Doom>(null);
  // El lead global ve los de todos; por defecto, los suyos: es lo que viene a
  // administrar casi siempre.
  let everyone = $state(false);

  async function load() {
    try {
      rows = await api.tokens();
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }
  $effect(() => {
    load();
  });

  const shown = $derived(everyone ? rows : rows.filter((r) => r.owner === me.id));

  const EXPIRY = [
    { days: 30, label: '30 días' },
    { days: 90, label: '90 días' },
    { days: 365, label: '1 año' },
    { days: 0, label: 'Sin caducidad' },
  ];

  // ---- generar ----
  let name = $state('');
  let days = $state(90);
  let creating = $state(false);
  let fresh = $state<AgentToken | null>(null);
  let copied = $state('');

  async function create(e: SubmitEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    creating = true;
    error = '';
    try {
      fresh = await api.createToken(name.trim(), days);
      name = '';
      await load();
    } catch (err) {
      error = (err as Error).message;
    } finally {
      creating = false;
    }
  }

  async function copy(what: 'token' | 'prompt') {
    if (!fresh?.token) return;
    try {
      const text = what === 'token' ? fresh.token : await setupPrompt(`${location.origin}/mcp`, fresh.token);
      await navigator.clipboard.writeText(text);
      copied = what;
      setTimeout(() => (copied = ''), 1800);
    } catch {
      error = 'El navegador no dejó copiar al portapapeles.';
    }
  }

  // ---- lo demás ----
  type Status = { key: 'revoked' | 'expired' | 'soon' | 'ok' | 'forever'; text: string };
  function status(t: AgentToken): Status {
    if (t.revoked_at) return { key: 'revoked', text: `revocado ${ago(t.revoked_at)}` };
    if (!t.expires_at) return { key: 'forever', text: 'sin caducidad' };
    const left = when(t.expires_at) - Date.now();
    if (left <= 0) return { key: 'expired', text: `caducó ${ago(t.expires_at)}` };
    const d = Math.ceil(left / 86_400_000);
    return { key: d <= 7 ? 'soon' : 'ok', text: `caduca en ${d === 1 ? '1 día' : `${d} días`}` };
  }

  async function run(p: Promise<unknown>) {
    try {
      await p;
      error = '';
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  let renaming = $state('');
  let draft = $state('');
  function commitRename(t: Row) {
    const next = draft.trim();
    renaming = '';
    if (next && next !== t.name) run(api.renameToken(t.id, next));
  }

  function act(t: Row, value: string) {
    if (value === 'rename') {
      renaming = t.id;
      draft = t.name;
    } else if (value === 'revoke') {
      doom = {
        title: `¿Revocar «${t.name}»?`,
        body:
          'El agente que lo use deja de entrar al MCP en su siguiente llamada. No se deshace: si hace falta otra vez, se genera uno nuevo.',
        verb: 'Revocar',
        go: () => run(api.revokeToken(t.id)),
      };
    } else if (value.startsWith('extend:')) {
      run(api.extendToken(t.id, Number(value.slice(7))));
    }
  }
</script>

<section class="set-section">
  <p class="set-lead">
    Cada agente —Claude Code, Codex, Cursor— entra al MCP con un token propio y trabaja como tú:
    mismas reglas, y lo que escribe queda firmado con tu nombre. Revocar uno no toca a los demás
    ni cierra tu sesión.
  </p>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  <form class="set-card" onsubmit={create}>
    <h3>Generar un token</h3>
    <div class="set-row">
      <input
        class="input grow"
        autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
        placeholder="¿Para qué agente? p. ej. Claude Code del portátil"
        maxlength="80"
        bind:value={name} />
      <select class="select w-auto" bind:value={days} aria-label="caducidad">
        {#each EXPIRY as x (x.days)}<option value={x.days}>{x.label}</option>{/each}
      </select>
      <button class="btn btn-sm preset-filled-primary-500" type="submit" disabled={creating || !name.trim()}>
        {creating ? '…' : 'Generar'}
      </button>
    </div>
  </form>

  {#if fresh?.token}
    <!-- La única vez que el token existe fuera del servidor. -->
    <div class="set-card fresh" role="status">
      <h3>🔑 «{fresh.name}»</h3>
      <p class="help">
        Cópialo ahora: no se vuelve a enseñar. Lo más directo es copiar el prompt y pegarlo en tu
        asistente, que se configura solo.
      </p>
      <code class="secret">{fresh.token}</code>
      <div class="set-row">
        <button class="btn btn-sm preset-filled-primary-500" onclick={() => copy('prompt')}>
          {copied === 'prompt' ? '✓ copiado' : 'Copiar el prompt de conexión'}
        </button>
        <button class="btn btn-sm preset-tonal-surface" onclick={() => copy('token')}>
          {copied === 'token' ? '✓ copiado' : 'Copiar sólo el token'}
        </button>
        <button class="btn btn-sm preset-tonal-surface ml-auto" onclick={() => (fresh = null)}>Ya lo guardé</button>
      </div>
    </div>
  {/if}

  <div class="set-card">
    <div class="set-row head">
      <h3>{everyone ? 'Todos los tokens' : 'Mis tokens'}</h3>
      {#if isLead}
        <label class="faint ml-auto text-xs">
          <input type="checkbox" bind:checked={everyone} /> ver los de todos
        </label>
      {/if}
    </div>

    {#if !loaded}
      <p class="faint text-sm">leyendo…</p>
    {:else if !shown.length}
      <p class="faint text-sm">
        Ninguno todavía. Un agente conectado con tu sesión en vez de un token sigue funcionando,
        pero no se puede revocar solo: genera uno y vuelve a conectarlo.
      </p>
    {:else}
      <ul class="list">
        {#each shown as t (t.id)}
          {@const st = status(t)}
          <li class="tok" class:dead={st.key === 'revoked' || st.key === 'expired'}>
            <div class="min-w-0 flex-1">
              {#if renaming === t.id}
                <input
                  class="input"
                  maxlength="80"
                  bind:value={draft}
                  {@attach (el: HTMLInputElement) => el.focus()}
                  onblur={() => commitRename(t)}
                  onkeydown={(e) => {
                    if (e.key === 'Enter') commitRename(t);
                    if (e.key === 'Escape') renaming = '';
                  }} />
              {:else}
                <p class="tname">{t.name}</p>
              {/if}
              <p class="meta">
                <code>{t.prefix}…</code>
                {#if everyone}· {t.owner === me.id ? 'tuyo' : t.ownerName}{/if}
                · creado {ago(t.created)}
                · {t.last_used_at ? `usado ${ago(t.last_used_at)}` : 'nunca usado'}
              </p>
            </div>
            <span class="badge {st.key}">{st.text}</span>
            {#if st.key !== 'revoked'}
              <Menu onSelect={(e: { value: string }) => act(t, e.value)}>
                <Menu.Trigger class="more" aria-label="acciones de {t.name}">⋯</Menu.Trigger>
                <Portal>
                  <Menu.Positioner>
                    <Menu.Content>
                      {#if t.owner === me.id}
                        <Menu.Item value="extend:30"><Menu.ItemText>Extender 30 días</Menu.ItemText></Menu.Item>
                        <Menu.Item value="extend:90"><Menu.ItemText>Extender 90 días</Menu.ItemText></Menu.Item>
                        <Menu.Item value="extend:365"><Menu.ItemText>Extender 1 año</Menu.ItemText></Menu.Item>
                        <Menu.Item value="extend:0"><Menu.ItemText>Quitar la caducidad</Menu.ItemText></Menu.Item>
                        <Menu.Separator />
                        <Menu.Item value="rename"><Menu.ItemText>Renombrar</Menu.ItemText></Menu.Item>
                      {/if}
                      <Menu.Item value="revoke"><Menu.ItemText><span class="danger">Revocar</span></Menu.ItemText></Menu.Item>
                    </Menu.Content>
                  </Menu.Positioner>
                </Portal>
              </Menu>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</section>

<Confirm bind:ask={doom} />

<style>
  .grow { flex: 1; min-width: 14rem; }
  .help { margin: 0 0 0.6rem; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  .fresh { border-color: color-mix(in oklab, var(--accent, var(--text)) 45%, var(--line)); }
  .secret {
    display: block;
    margin: 0 0 0.75rem;
    padding: 0.5rem 0.7rem;
    overflow-x: auto;
    border-radius: 8px;
    background: var(--hover);
    font-size: 0.8rem;
    white-space: nowrap;
    user-select: all;
  }
  .head h3 { margin: 0; }
  .list { margin: 0.6rem 0 0; padding: 0; list-style: none; }
  .tok {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.6rem 0;
    border-top: 1px solid var(--line);
  }
  .tok.dead { opacity: 0.55; }
  .tname { margin: 0; font-size: 0.9rem; font-weight: 600; }
  .meta { margin: 0.1rem 0 0; color: var(--faint); font-size: 0.76rem; }
  .meta code { font-size: 0.74rem; }
  .badge {
    flex: none;
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.72rem;
    white-space: nowrap;
  }
  .badge.soon { background: color-mix(in oklab, var(--color-warning-500) 20%, transparent); color: var(--text); }
  .badge.expired,
  .badge.revoked { background: color-mix(in oklab, var(--color-error-500) 14%, transparent); }
  :global(.more) {
    flex: none;
    width: 28px;
    height: 28px;
    border-radius: 7px;
    color: var(--muted);
  }
  :global(.more:hover) { background: var(--hover); color: var(--text); }
  .danger { color: var(--color-error-500); }
</style>
