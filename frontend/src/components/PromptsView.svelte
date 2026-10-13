<script lang="ts">
  import Loading from './Loading.svelte';
  // Todo lo que se le dice a la IA, en un sitio: los AGENTS.md, los documentos
  // del binario (guía, reglas de la casa, conexión, interrogatorio) y la
  // descripción de cada tool MCP — que es lo que un agente lee para decidir si
  // la llama y cómo.
  //
  // La misma experiencia que la wiki: la columna de la izquierda elige, el
  // documento se lee renderizado y se edita con el mismo DocEditor (CodeMirror,
  // vim, el menú `/`). Lo que el editor entrega es un borrador; la versión la
  // crea un botón, porque cada guardado llega a todos los agentes en su próxima
  // llamada.
  import { api, type PromptInfo } from '../lib/api';
  import { ago } from '../lib/when';
  import DocEditor from './DocEditor.svelte';
  import Prose from './Prose.svelte';
  import Icon from './Icon.svelte';
  import SideTree, { type TreeNode } from './SideTree.svelte';
  import TreePanel from './TreePanel.svelte';
  import { readTreeSelection, saveTreeSelection } from '../lib/treeSelection';

  let {
    key = '',
    onback,
    onsearch,
    onpick,
  }: {
    key?: string;
    onback?: () => void;
    onsearch?: () => void;
    /** cambiar de prompt cambia la dirección */
    onpick?: (key: string) => void;
  } = $props();

  let list = $state<PromptInfo[]>([]);
  let remembered = $state('');
  let doc = $state<PromptInfo | null>(null);
  let draft = $state<string | null>(null);
  let html = $state('');
  let editing = $state(false);
  let saving = $state(false);
  let error = $state('');
  let saved = $state(false);
  let filter = $state('');

  const groups: { id: PromptInfo['group']; name: string; face: string }[] = [
    { id: 'agentes', name: 'Agentes', face: 'robot' },
    { id: 'sistema', name: 'Sistema', face: 'gear' },
    { id: 'tools', name: 'Tools', face: 'tools' },
  ];
  const shown = $derived(
    list.filter((p) => !filter || (p.title + ' ' + p.key).toLowerCase().includes(filter.toLowerCase())),
  );
  const current = $derived(key || remembered || list[0]?.key || '');
  const markdown = $derived(draft ?? doc?.content ?? '');
  const dirty = $derived(draft !== null && draft !== (doc?.content ?? ''));

  async function loadList() {
    try {
      list = await api.prompts();
      remembered = readTreeSelection(api.me?.id ?? 'local', 'prompts', list.map((p) => p.key));
    } catch (e) {
      error = (e as Error).message;
    }
  }
  loadList();

  async function loadDoc(k: string) {
    if (!k) return;
    draft = null;
    saved = false;
    error = '';
    try {
      const next = await api.prompt(k);
      if (current !== k) return;
      doc = next;
      saveTreeSelection(api.me?.id ?? 'local', 'prompts', k);
    } catch (e) {
      if (current !== k) return;
      doc = null;
      error = (e as Error).message;
    }
  }
  $effect(() => {
    loadDoc(current);
  });

  $effect(() => {
    const md = markdown;
    api.renderMarkdown(md).then((h) => {
      if (md === markdown) html = h;
    }).catch(() => {});
  });

  async function publish(reset = false) {
    if (!doc || saving || (!reset && (!dirty || draft === null))) return;
    saving = true;
    error = '';
    try {
      doc = await api.savePrompt(doc.key, reset ? '' : (draft ?? ''), reset);
      draft = null;
      saved = true;
      loadList();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      saving = false;
    }
  }

  /** Las categorías como ramas del árbol de Skeleton, como la wiki. */
  const tree = $derived<TreeNode[]>(
    groups
      .map((g) => ({
        id: `group:${g.id}`,
        name: g.name,
        face: g.face,
        children: shown
          .filter((p) => p.group === g.id)
          .map((p) => ({
            id: p.key,
            name: p.title,
            face: faceOf(p),
            mono: p.group === 'tools',
            dot: p.stale ? ('warn' as const) : p.edited && p.group !== 'agentes' ? ('on' as const) : undefined,
          })),
      }))
      .filter((g) => g.children.length),
  );

  function faceOf(p: PromptInfo) {
    if (p.group === 'agentes') return p.key.endsWith('planner') ? 'map' : 'tools';
    if (p.group === 'tools') return 'link';
    return { agents_guide: 'book', markdown: 'document', connect: 'robot', grill: 'brain', ephemeral_markdown: 'document' }[p.key] ?? 'prompt';
  }
</script>

<div class="screen" aria-label="prompts">
  <div class="topbar">
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <span class="ttl"><Icon of="prompt" tint /> Prompts</span>
    {#if doc}
      <span class="sep" aria-hidden="true">/</span>
      <h2 class="doc">{doc.title}</h2>
      <span class="faint meta">
        {#if doc.group === 'agentes'}
          {doc.updated ? `versión de ${ago(doc.updated)} · por ${doc.author || 'sin nombre'}` : 'todavía no hay ninguna versión'}
        {:else if doc.edited}
          editado {ago(doc.updated ?? '')} · por {doc.author || 'sin nombre'}
        {:else}
          el original del binario
        {/if}
      </span>
    {/if}
    <span class="grow"></span>
    {#if doc?.editable}
      {#if dirty}<span class="pending">cambios sin guardar</span>{/if}
      {#if doc.group !== 'agentes' && doc.edited}
        <button class="btn btn-sm preset-tonal-surface" disabled={saving} onclick={() => publish(true)}
          title="Volver al texto que trae el binario">Restaurar el original</button>
      {/if}
      <button class="btn btn-sm preset-filled-primary-500" disabled={!dirty || saving} onclick={() => publish()}>
        {#if saving}<Loading compact label="Guardando prompt" />{:else}Guardar versión{/if}
      </button>
    {:else if doc}
      <span class="pending">sólo lectura</span>
    {/if}
  </div>

  <div class="body">
    <TreePanel name="prompts">
    <aside class="side">
      <input class="input find" type="search" placeholder="Buscar un prompt…" bind:value={filter}
        autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
      <div class="tree">
        <SideTree nodes={tree} selected={current} onselect={(n) => !n.children && onpick?.(n.id)} />
      </div>
    </aside>
    </TreePanel>

    <main class="content" class:pending={!doc && !error}>
      {#if doc}
        <p class="hint"><Icon of="question" tint /> <span>{doc.hint}</span></p>
        {#if doc.stale}
          <p class="card warn">
            <Icon of="warning" tint /> El original del binario cambió desde que se escribió esta versión.
            Revisa si lo nuevo debería estar aquí, o restaura el original.
          </p>
        {/if}
        {#if saved}
          <p class="card notice">
            <Icon of="check" tint /> Guardado. Los agentes lo leen en su <strong>próxima llamada</strong>; avisa al
            equipo de qué cambió.
          </p>
        {/if}
        {#if error}<p class="err" role="alert">{error}</p>{/if}
        {#key doc.key}
          {#if doc.editable}
            <DocEditor {markdown} {html} scrollKey={`prompts/${doc.key}`} bind:editing onsave={(md) => (draft = md)} />
          {:else if markdown}
            <Prose {html} scrollKey={`prompts/${doc.key}`} />
          {:else}
            <p class="faint text-sm">Todavía no hay nada escrito aquí.</p>
          {/if}
        {/key}
      {:else if error}
        <p class="err" role="alert">{error}</p>
      {:else}
        <Loading label="Leyendo prompt" />
      {/if}
    </main>
  </div>
</div>

<div class="hud">
  <button class="hud-btn" onclick={onsearch} title="Buscar · ⌘K"><Icon of="search" size="1.35rem" tint /></button>
</div>

<style>
  .screen { display: flex; flex-direction: column; height: 100dvh; overflow: hidden; }
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
  .ttl { display: inline-flex; align-items: center; gap: 0.4rem; font-weight: 750; font-size: 0.95rem; }
  .sep { color: var(--faint); }
  .doc { margin: 0; font-size: 0.95rem; font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .meta { font-size: 0.8rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .grow { flex: 1; }
  .pending { font-size: 0.8rem; color: var(--muted); }

  .body { flex: 1; min-height: 0; display: flex; }
  .side {
    flex: none;
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 0.8rem 0.7rem;
    border-right: 1px solid var(--line);
    overflow: hidden;
  }
  .find { font-size: 0.85rem; }
  .tree { flex: 1; overflow-y: auto; scrollbar-width: thin; }
  .content { flex: 1; min-width: 0; overflow-y: auto; padding: 1rem 1.5rem 4rem; }
  .content.pending { padding: 0; display: flex; }
  .hint {
    display: flex;
    gap: 0.5rem;
    align-items: flex-start;
    max-width: 940px;
    margin: 0 auto 0.8rem;
    color: var(--muted);
    font-size: 0.88rem;
  }
  .warn, .notice, .err {
    display: flex;
    gap: 0.5rem;
    align-items: flex-start;
    max-width: 940px;
    margin: 0 auto 0.8rem;
    padding: 0.6rem 0.8rem;
    font-size: 0.86rem;
  }
  .warn { border: 1px solid #f59e0b; border-radius: 10px; }
  .err {
    border-radius: 10px;
    background: color-mix(in oklab, var(--p1, tomato) 14%, transparent);
    color: var(--text);
  }
  .hud { position: fixed; right: 1rem; bottom: 1rem; z-index: var(--z-chrome); display: flex; gap: 0.5rem; }
  .hud-btn {
    width: 40px;
    height: 40px;
    display: grid;
    place-content: center;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
  }
  .hud-btn:hover { background: var(--hover); }
  @media (max-width: 760px) {
    .side { display: none; }
  }
</style>
