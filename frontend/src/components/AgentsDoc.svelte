<script lang="ts">
  // Un AGENTS.md del departamento: la forma de trabajar que el lead escribe y
  // que cada agente lee por MCP (`agents_md`) al empezar su sesión.
  //
  // El mismo editor que un hilo, con una diferencia a propósito: aquí guardar
  // NO es salir del editor. `DocEditor` entrega el texto al perder el foco, con
  // ⌘S o con Escape, y en un hilo eso es escribir —cuanto más, mejor—. Aquí
  // cada guardado es una VERSIÓN que llega a todos los agentes del
  // departamento, y el lead avisa al equipo de qué cambió; cinco versiones por
  // cinco clics fuera del campo son cinco avisos que no dicen nada. Así que lo
  // que entrega el editor queda como borrador, y la versión la crea un botón.
  import { api, type AgentsRole, type AgentsVersion } from '../lib/api';
  import { ago } from '../lib/when';
  import DocEditor from './DocEditor.svelte';
  import Prose from './Prose.svelte';

  let {
    role,
    canWrite = false,
    onback,
    onsearch,
  }: {
    role: AgentsRole;
    /** el lead edita; quien opera sólo lee el suyo */
    canWrite?: boolean;
    onback?: () => void;
    onsearch?: () => void;
  } = $props();

  let current = $state<AgentsVersion | null>(null);
  /** lo que el editor entregó y todavía no es una versión */
  let draft = $state<string | null>(null);
  let html = $state('');
  let editing = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  /** recién guardada: el recordatorio de avisar al equipo */
  let saved = $state(false);

  const title = $derived(role === 'planner' ? 'AGENTS.md (planner)' : 'AGENTS.md (operators)');
  const who = $derived(role === 'planner' ? 'el lead' : 'los operadores');
  const markdown = $derived(draft ?? current?.content ?? '');
  const dirty = $derived(draft !== null && draft !== (current?.content ?? ''));

  async function load() {
    loading = true;
    error = '';
    draft = null;
    saved = false;
    try {
      current = await api.agentsDoc(role);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void role;
    load();
  });

  // Lo que se lee, renderizado por el servidor como todo documento.
  $effect(() => {
    const md = markdown;
    api.renderMarkdown(md).then((h) => {
      if (md === markdown) html = h;
    }).catch(() => {});
  });

  async function publish() {
    if (!dirty || saving || draft === null) return;
    saving = true;
    error = '';
    try {
      await api.saveAgentsDoc(role, draft);
      current = await api.agentsDoc(role);
      draft = null;
      saved = true;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }
</script>

<div class="screen" aria-label={title}>
  <div class="topbar">
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <h2 class="ttl">{title}</h2>
    <span class="faint meta">
      {#if loading}
        …
      {:else if current}
        versión de {ago(current.created)} · por {current.authorName || 'sin nombre'}
      {:else}
        todavía no hay ninguna versión
      {/if}
    </span>
    <span class="grow"></span>
    {#if canWrite}
      {#if dirty}<span class="pending">cambios sin guardar</span>{/if}
      <button class="btn btn-sm preset-filled-primary-500" disabled={!dirty || saving} onclick={publish}>
        {saving ? 'guardando…' : 'Guardar versión'}
      </button>
    {:else}
      <span class="pending">sólo lectura</span>
    {/if}
  </div>

  <div class="body">
    {#if canWrite}
      <p class="muted intro">
        Lo leen los agentes de {who} por MCP (<code>agents_md</code>) al empezar cada sesión. Cada
        guardado es una versión nueva, y la anterior se conserva.
      </p>
    {:else}
      <p class="muted intro">
        La forma de trabajar del departamento, tal como la mantiene el lead. Es lo mismo que tu agente
        lee por MCP (<code>agents_md</code>) al empezar cada sesión.
      </p>
    {/if}
    {#if saved}
      <p class="card notice">
        Guardado. <strong>Avisa al equipo</strong> de qué cambió: sus agentes leerán esta versión en su
        próxima sesión.
      </p>
    {/if}
    {#if error}<p class="err" role="alert">{error}</p>{/if}
    {#if loading}
      <!-- nada todavía -->
    {:else if canWrite}
      <DocEditor {markdown} {html} bind:editing onsave={(md) => (draft = md)} />
    {:else if current}
      <Prose {html} />
    {:else}
      <p class="faint text-sm">El lead todavía no ha escrito ninguna versión.</p>
    {/if}
  </div>
</div>

<div class="hud">
  <button class="hud-btn" onclick={onsearch} title="Buscar · ⌘K"><span aria-hidden="true">🔍</span></button>
</div>

<style>
  .screen {
    display: flex;
    flex-direction: column;
    height: 100dvh;
    overflow: hidden;
  }
  .topbar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    height: 46px;
    padding: 0 0.9rem;
    border-bottom: 1px solid var(--line);
    background: color-mix(in oklab, var(--bg) 82%, transparent);
    backdrop-filter: blur(8px);
  }
  .back {
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.82rem;
  }
  .back:hover { color: var(--text); background: var(--hover); }
  .ttl { margin: 0; font-size: 0.95rem; font-weight: 700; }
  .meta { font-size: 0.8rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .grow { flex: 1; }
  .pending { font-size: 0.8rem; color: var(--muted); }
  .body {
    flex: 1;
    overflow: auto;
    width: 100%;
    max-width: 52rem;
    margin: 0 auto;
    padding: 1rem;
  }
  .intro { font-size: 0.85rem; margin: 0 0 0.75rem; }
  .notice, .err { padding: 0.6rem 0.8rem; margin: 0 0 0.75rem; font-size: 0.85rem; }
  .err {
    border-radius: 10px;
    background: color-mix(in oklab, var(--p1, tomato) 14%, transparent);
    color: var(--text);
  }
  .hud {
    position: fixed;
    right: 1rem;
    bottom: 1rem;
    z-index: var(--z-chrome);
    display: flex;
    gap: 0.5rem;
  }
  .hud-btn {
    width: 40px;
    height: 40px;
    display: grid;
    place-content: center;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    font-size: 1rem;
  }
  .hud-btn:hover { background: var(--hover); }
</style>
