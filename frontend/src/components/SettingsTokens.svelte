<script lang="ts">
  import FilterPick from './FilterPick.svelte';
  import Loading from './Loading.svelte';
  import Icon from './Icon.svelte';
  // Tokens: las llaves con que los agentes de una persona entran al MCP.
  //
  // Un token es del agente, no de la sesión: se revoca uno sin cerrar nada más,
  // caduca cuando se decidió (o nunca) y dice cuándo se usó por última vez. El
  // servidor guarda sólo su huella, así que el token se enseña UNA vez, al
  // generarlo, junto al prompt que conecta al asistente.
  import { Menu, Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import ClientLogo from './ClientLogo.svelte';
  import AgentsMdBadge from './AgentsMdBadge.svelte';
  import { api, type AgentReleases, type AgentToken, type Person } from '../lib/api';
  import { connectVersion, placeholderNote, setupPrompt, tokenPlaceholder } from '../lib/connect';
  import { agentHealth, short } from '../lib/agentHealth';
  import { brandOf } from '../lib/agentClients';
  import HarnessCommands from './HarnessCommands.svelte';
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

  // Contra qué se compara lo que leyó cada agente. Si falla (un servidor
  // anterior a esto), la lista sale como antes, sin estado.
  let releases = $state<AgentReleases | null>(null);
  let version = $state('');

  async function load() {
    api.agentReleases().then((r) => (releases = r), () => (releases = null));
    connectVersion().then((v) => (version = v), () => (version = ''));
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

  // Copiar el prompt de un token que ya existe. El servidor sólo tiene su
  // huella: el prompt va con un hueco y una nota que dice cuál token pegar.
  let rowCopied = $state('');
  async function copyRow(t: Row) {
    try {
      const text = placeholderNote(t.name, t.prefix) + (await setupPrompt(mcpUrl, tokenPlaceholder(t.prefix)));
      await navigator.clipboard.writeText(text);
      rowCopied = t.id;
      setTimeout(() => (rowCopied = ''), 1800);
    } catch {
      error = 'El navegador no dejó copiar al portapapeles.';
    }
  }
  const mcpUrl = `${location.origin}/mcp`;

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
    Cada agente —Claude Code, Codex, Antigravity— entra al MCP con un token propio y trabaja como tú:
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
      <span class="expiry"><FilterPick items={EXPIRY.map((x) => ({ value: String(x.days), label: x.label }))} value={String(days)}
        allowEmpty={false} label="caducidad" onchange={(v) => (days = Number(v))} /></span>
      <button class="btn btn-sm preset-filled-primary-500" type="submit" disabled={creating || !name.trim()}>
        {#if creating}<Loading compact label="Generando token" />{:else}Generar{/if}
      </button>
    </div>
  </form>

  {#if fresh?.token}
    <!-- La única vez que el token existe fuera del servidor. -->
    <div class="set-card fresh" role="status">
      <h3><Icon of="key" /> «{fresh.name}»</h3>
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
      <details class="manual">
        <summary>O agrégalo a mano en tu harness</summary>
        <HarnessCommands url={mcpUrl} token={fresh.token} {version} />
      </details>
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
      <Loading label="Leyendo tokens" />
    {:else if !shown.length}
      <p class="faint text-sm">
        Ninguno todavía. Un agente conectado con tu sesión en vez de un token sigue funcionando,
        pero no se puede revocar solo: genera uno y vuelve a conectarlo.
      </p>
    {:else}
      <ul class="list">
        {#each shown as t (t.id)}
          {@const st = status(t)}
          {@const health = agentHealth(t, releases)}
          {@const brand = t.client ? brandOf(t.client) : null}
          {@const role = t.release_role || (t.owner === me.id && isLead ? 'planner' : 'operators')}
          <li class="tok" class:dead={st.key === 'revoked' || st.key === 'expired'}>
            <!-- Su cara: el cliente que se conectó con él (el clientInfo de su
                 initialize), o una llave si todavía no se conecta nadie. -->
            {#if brand}
              <ClientLogo {brand} size={32} tip={`${brand.name}${brand.version ? ' ' + brand.version : ''}`} />
            {:else}
              <span class="nokey" role="img" aria-label="todavía no se conecta ningún agente"><Icon of="key" size="16px" /></span>
            {/if}
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
              <!-- El token, como se enseña una contraseña: el principio que lo
                   reconoce y puntos por lo que el servidor ya no tiene. -->
              <p class="meta">
                <span class="secret-mask" aria-label="token que empieza con {t.prefix}">{t.prefix}<span class="dots" aria-hidden="true">••••••••••</span></span>
                {#if everyone}· {t.owner === me.id ? 'tuyo' : t.ownerName}{/if}
              </p>
              <!-- Debajo, cuándo: su caducidad (o revocación) y sus tiempos. -->
              <p class="meta when">
                <!-- Un revocado se queda una semana y lo borra un job (tokens.go). -->
                <span class="pill {st.key}" title={st.key === 'revoked' ? 'Se borra solo a la semana de revocado' : undefined}>{st.text}</span>
                <span>creado {ago(t.created)} · {t.last_used_at ? `usado ${ago(t.last_used_at)}` : 'nunca usado'}</span>
              </p>
            </div>
            {#if health}
              <!-- Lo que el servidor vio: prueba que el texto llegó, no que se siguiera. -->
              <AgentsMdBadge read={t.release_read} at={t.release_read_at} {role}
                want={(role === 'planner' ? releases?.planner : releases?.operators) ?? ''} />
              {#if health.install === 'old'}
                <!-- Informativo, en gris: la sección del CLAUDE.md la actualiza
                     el agente solo al leer agents_md; esto mide la cabecera de
                     su configuración, que sólo cambia al volver a pegar el prompt. -->
                <Tooltip openDelay={120} closeDelay={60} positioning={{ placement: 'top' }}>
                  <Tooltip.Trigger>
                    {#snippet element(attributes: Record<string, unknown>)}
                      <span class="pill" aria-label="instalación vieja" {...attributes}>instalación vieja</span>
                    {/snippet}
                  </Tooltip.Trigger>
                  <Portal>
                    <Tooltip.Positioner>
                      <Tooltip.Content>
                        <span class="rel-tip">
                          <strong>Instalación anterior</strong>
                          <span>se instaló con <b class="ver">{short(t.connect_version)}</b></span>
                          <span>vigente <b class="ver">{short(releases?.connect)}</b></span>
                          <span>Su sección se actualiza sola al leer agents_md;</span>
                          <span>la cabecera, al volver a pegar el prompt.</span>
                        </span>
                      </Tooltip.Content>
                    </Tooltip.Positioner>
                  </Portal>
                </Tooltip>
              {/if}
            {/if}
            {#if st.key !== 'revoked' && t.owner === me.id}
              <!-- El prompt de conexión, con un hueco para el token: el servidor
                   no lo tiene, sólo su huella. -->
              <button class="btn btn-sm preset-tonal-surface copy-prompt" onclick={() => copyRow(t)}
                title="Copia el prompt de conexión; el token lo pegas tú (empieza con {t.prefix})">
                {rowCopied === t.id ? '✓ copiado' : 'Copiar prompt'}
              </button>
            {/if}
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
  <!-- Conectar a mano, sin pegarle un prompt al asistente. -->
  <div class="set-card">
    <h3>Comandos por harness</h3>
    <p class="help">
      Para agregar el servidor tú mismo. Reemplaza <code>&lt;TU_TOKEN&gt;</code> por un token tuyo; al
      generar uno nuevo, aquí arriba salen ya con él.
    </p>
    <HarnessCommands url={mcpUrl} token="<TU_TOKEN>" {version} />
  </div>
</section>

<Confirm bind:ask={doom} />

<style>
  .grow { flex: 1; min-width: 14rem; }
  .help { margin: 0 0 0.6rem; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  /* El token recién generado sí se resalta (se enseña una vez): un fondo suave. */
  .fresh { padding: 1rem !important; border-radius: 12px; background: color-mix(in oklab, var(--accent) 10%, transparent) !important; }
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
  .nokey { display: inline-grid; flex: none; place-content: center; width: 32px; height: 32px; border-radius: 9px; border: 1px dashed var(--line); color: var(--muted); }
  .tname { margin: 0; font-size: 0.9rem; font-weight: 600; }
  .meta { margin: 0.1rem 0 0; color: var(--faint); font-size: 0.76rem; }
  .meta.when { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem; margin-top: 0.3rem; }
  /* El token: el prefijo y puntos, en monoespaciada y sin caja. */
  .secret-mask { font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace); font-size: 0.74rem; color: var(--muted); }
  .secret-mask .dots { margin-left: 0.1em; letter-spacing: -0.12em; opacity: 0.7; }
  /* `pill` y no `badge`: Skeleton tiene una utilidad `badge` que fuerza
     ~0.6rem y negrita, y ganaba sobre esto. Mismas medidas que AgentsMdBadge. */
  .pill {
    flex: none;
    display: inline-flex;
    align-items: center;
    padding: 0.15rem 0.55rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.74rem;
    line-height: 1.35;
    white-space: nowrap;
  }
  .pill.soon { background: color-mix(in oklab, var(--color-warning-500) 20%, transparent); color: var(--text); }
  .pill.expired,
  .pill.revoked { background: color-mix(in oklab, var(--color-error-500) 14%, transparent); }
  :global(.more) {
    flex: none;
    width: 28px;
    height: 28px;
    border-radius: 7px;
    color: var(--muted);
  }
  :global(.more:hover) { background: var(--hover); color: var(--text); }
  .danger { color: var(--color-error-500); }
  .expiry { width: 10rem; flex: none; }
  .copy-prompt { flex: none; }
  .manual { margin-top: 0.9rem; }
  .manual summary { cursor: pointer; font-size: 0.82rem; font-weight: 600; color: var(--muted); }
  .manual[open] summary { margin-bottom: 0.75rem; }
</style>
