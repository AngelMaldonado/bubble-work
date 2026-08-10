<script lang="ts">
  // A workspace's documentation, in the web (docs/ARTIFACT-EDITING.md).
  //
  // Pages are the standing reference a body of work accumulates — the product
  // spec, the API contract, the decision record. They are NOT threads: no
  // buoyancy, no heat, no place on the board. So this is a reading room, not
  // another view of the board: a list on the left, the document on the right.
  //
  // The editor is ArtifactEditor with its destination swapped. Everything that
  // makes markdown editing bearable already lives there — CodeMirror, the slash
  // menu, vim, the autosave rhythm, the hash contract — and forking it for pages
  // would mean two of each, drifting.
  import { api, ApiError } from '../lib/api';
  import { t } from '../lib/i18n.svelte';
  import { store } from '../lib/store.svelte';
  import type { Page, PageDetail } from '../lib/types';
  import ArtifactEditor from './ArtifactEditor.svelte';
  import ConfirmDelete from './ConfirmDelete.svelte';

  let {
    workspaceId,
    workspaceName,
    onclose,
  }: { workspaceId: string; workspaceName: string; onclose: () => void } = $props();

  let pages = $state<Page[]>([]);
  let detail = $state<PageDetail | null>(null);
  let loading = $state(true);
  let bodyLoading = $state(false);
  let error = $state<string | null>(null);
  let editing = $state(false);
  let pendingDelete = $state<Page | null>(null);
  let deleting = $state(false);

  // Creating and renaming share one inline field: both ask for a title, and a
  // dialog for six characters of text is a dialog too many.
  let naming = $state<'new' | 'rename' | null>(null);
  let draftTitle = $state('');
  let saving = $state(false);
  let titleField = $state<HTMLInputElement | null>(null);

  $effect(() => {
    if (naming) titleField?.focus();
  });

  async function load(selectId?: string): Promise<void> {
    loading = true;
    error = null;
    try {
      pages = await api.pages(workspaceId);
      const want = selectId ?? detail?.id ?? pages[0]?.id;
      if (want && pages.some((p) => p.id === want)) await open(want);
      else detail = null;
    } catch (e) {
      // A Plane older than the pages API answers 404 here, which is worth
      // saying plainly rather than showing an empty reading room.
      error = e instanceof ApiError ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  async function open(id: string): Promise<void> {
    if (detail?.id === id && !bodyLoading) return;
    bodyLoading = true;
    editing = false;
    try {
      detail = await api.page(id);
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e);
    } finally {
      bodyLoading = false;
    }
  }

  // The save seam ArtifactEditor calls. It hands back the new hash, which is
  // what the next write has to be based on.
  async function writeBody(text: string, base: string): Promise<{ hash: string }> {
    const d = await api.updatePage(detail!.id, { markdown: text, base_hash: base });
    detail = d;
    pages = pages.map((p) => (p.id === d.id ? { ...p, updated_at: d.updated_at } : p));
    return { hash: d.hash };
  }

  async function submitTitle(e: Event): Promise<void> {
    e.preventDefault();
    const title = draftTitle.trim();
    if (!title || saving) return;
    saving = true;
    error = null;
    try {
      if (naming === 'new') {
        // Seeded with its own H1: an empty document is a blank page, and the
        // standard wants exactly one top-level heading anyway.
        const d = await api.createPage(workspaceId, title, `# ${title}\n\n`);
        pages = [{ ...d }, ...pages];
        detail = d;
        editing = true;
      } else if (detail) {
        const d = await api.updatePage(detail.id, { title });
        detail = { ...detail, title: d.title };
        pages = pages.map((p) => (p.id === d.id ? { ...p, title: d.title } : p));
      }
      naming = null;
      draftTitle = '';
    } catch (e2) {
      error = e2 instanceof ApiError ? e2.message : String(e2);
    } finally {
      saving = false;
    }
  }

  async function confirmDelete(): Promise<void> {
    if (!pendingDelete || deleting) return;
    deleting = true;
    try {
      await api.deletePage(pendingDelete.id);
      const gone = pendingDelete.id;
      pendingDelete = null;
      pages = pages.filter((p) => p.id !== gone);
      detail = null;
      if (pages.length) await open(pages[0].id);
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e);
    } finally {
      deleting = false;
    }
  }

  function startNew(): void {
    naming = 'new';
    draftTitle = '';
  }
  function startRename(): void {
    if (!detail) return;
    naming = 'rename';
    draftTitle = detail.title;
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key !== 'Escape') return;
    // Escape unwinds one layer at a time: the field, then the editor, then the
    // room. Closing everything on the first press loses unsaved intent.
    if (naming) {
      naming = null;
      return;
    }
    if (editing) return; // the editor answers Escape itself
    onclose();
  }

  const readOnly = $derived(store.kiosk || !!detail?.locked);

  load();
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="scrim" onclick={onclose}>
  <div
    class="panel"
    role="dialog"
    tabindex="-1"
    aria-modal="true"
    aria-label={t('pages.title')}
    onclick={(e) => e.stopPropagation()}
  >
    <header class="head">
      <div class="who">
        <h2>{t('pages.title')}</h2>
        <span class="ws">{workspaceName}</span>
      </div>
      <button class="x" onclick={onclose} aria-label={t('del.cancel')}>×</button>
    </header>

    <div class="body">
      <nav class="rail">
        {#if !store.kiosk}
          <button class="newpage" onclick={startNew}>＋ {t('pages.new')}</button>
        {/if}
        {#if naming === 'new'}
          <form class="namer" onsubmit={submitTitle}>
            <input
              bind:this={titleField}
              bind:value={draftTitle}
              placeholder={t('pages.titlePlaceholder')}
              disabled={saving}
            />
          </form>
        {/if}

        {#if loading}
          <p class="dim">{t('board.loading')}</p>
        {:else if pages.length === 0}
          <p class="dim">{t('pages.empty')}</p>
        {:else}
          {#each pages as p (p.id)}
            <button class="item" class:on={detail?.id === p.id} onclick={() => open(p.id)}>
              <span class="ptitle">{p.title}</span>
              {#if p.locked}<span class="lock" title={t('pages.locked')}>🔒</span>{/if}
            </button>
          {/each}
        {/if}
      </nav>

      <main class="doc">
        {#if error}
          <p class="err">{error}</p>
        {/if}

        {#if bodyLoading}
          <p class="dim">{t('board.loading')}</p>
        {:else if detail}
          <div class="docbar">
            {#if naming === 'rename'}
              <form class="namer wide" onsubmit={submitTitle}>
                <input bind:this={titleField} bind:value={draftTitle} disabled={saving} />
              </form>
            {:else}
              <h3>{detail.title}</h3>
            {/if}
            <div class="acts">
              {#if !readOnly}
                <button onclick={startRename} title={t('pages.rename')}>✏️</button>
                <button class:on={editing} onclick={() => (editing = !editing)}>
                  {editing ? t('thread.rendered') : t('thread.markdown')}
                </button>
                <button
                  class="danger"
                  onclick={() => (pendingDelete = detail)}
                  title={t('pages.delete')}>🗑</button
                >
              {:else if detail.locked}
                <span class="dim">🔒 {t('pages.locked')}</span>
              {/if}
            </div>
          </div>

          {#if editing && !readOnly}
            <ArtifactEditor
              threadId={detail.id}
              region="brief"
              initial={detail.markdown}
              hash={detail.hash}
              write={writeBody}
              onreload={() => open(detail!.id)}
              ondone={() => (editing = false)}
            />
          {:else}
            <article class="prose">{@html detail.html}</article>
          {/if}
        {:else if !loading && !error}
          <p class="dim">{t('pages.pick')}</p>
        {/if}
      </main>
    </div>
  </div>
</div>

{#if pendingDelete}
  <ConfirmDelete
    what={pendingDelete.title}
    detail={t('del.page')}
    busy={deleting}
    oncancel={() => (pendingDelete = null)}
    onconfirm={confirmDelete}
  />
{/if}

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 150;
    display: grid;
    place-items: center;
    padding: 2rem 1.5rem;
    background: oklch(0.1 0.02 265 / 0.55);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .panel {
    width: min(96vw, 68rem);
    height: min(86vh, 52rem);
    display: flex;
    flex-direction: column;
    border-radius: 20px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 30px 80px var(--shadow-strong);
    font-family: var(--sans);
    overflow: hidden;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.9rem 1.1rem;
    border-bottom: 1px solid var(--line);
  }
  .who {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    min-width: 0;
  }
  h2 {
    margin: 0;
    font-size: 1rem;
    font-weight: 800;
  }
  .ws {
    font-size: 0.78rem;
    color: var(--faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .x {
    flex: none;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 1.3rem;
    line-height: 1;
    cursor: pointer;
  }
  .body {
    flex: 1;
    display: grid;
    grid-template-columns: 15rem 1fr;
    min-height: 0;
  }
  .rail {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0.6rem;
    border-right: 1px solid var(--line);
    overflow-y: auto;
  }
  .newpage {
    margin-bottom: 0.35rem;
    padding: 0.45rem 0.6rem;
    border-radius: 9px;
    border: 1px dashed var(--line);
    background: transparent;
    color: var(--muted);
    font-family: inherit;
    font-size: 0.78rem;
    font-weight: 700;
    cursor: pointer;
  }
  .newpage:hover {
    color: var(--text);
    border-color: var(--wip);
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.45rem 0.6rem;
    border: none;
    border-radius: 9px;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    font-size: 0.82rem;
    text-align: left;
    cursor: pointer;
  }
  .item:hover {
    background: var(--hover);
  }
  .item.on {
    background: color-mix(in oklab, var(--wip) 16%, transparent);
    font-weight: 700;
  }
  .ptitle {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .lock {
    font-size: 0.7rem;
  }
  .doc {
    padding: 1rem 1.3rem 1.5rem;
    overflow-y: auto;
    min-width: 0;
  }
  .docbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 0.7rem;
  }
  h3 {
    margin: 0;
    font-size: 1.05rem;
    font-weight: 800;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .acts {
    display: flex;
    gap: 0.3rem;
    flex: none;
  }
  .acts button {
    padding: 0.3rem 0.6rem;
    border-radius: 8px;
    border: 1px solid var(--line);
    background: transparent;
    color: var(--muted);
    font-family: inherit;
    font-size: 0.75rem;
    font-weight: 700;
    cursor: pointer;
  }
  .acts button:hover {
    color: var(--text);
    background: var(--hover);
  }
  .acts button.on {
    color: oklch(0.99 0 0);
    background: var(--wip);
    border-color: transparent;
  }
  .acts .danger:hover {
    color: oklch(0.65 0.19 25);
    border-color: color-mix(in oklab, oklch(0.65 0.19 25) 45%, var(--line));
  }
  .namer input {
    width: 100%;
    padding: 0.4rem 0.55rem;
    border-radius: 9px;
    border: 1px solid var(--wip);
    background: var(--surface);
    color: var(--text);
    font-family: inherit;
    font-size: 0.82rem;
  }
  .namer.wide {
    flex: 1;
  }
  .dim {
    color: var(--faint);
    font-size: 0.8rem;
  }
  .err {
    margin: 0 0 0.6rem;
    font-size: 0.8rem;
    color: oklch(0.7 0.18 25);
  }

  @media (max-width: 46rem) {
    .body {
      grid-template-columns: 1fr;
      grid-template-rows: auto 1fr;
    }
    .rail {
      max-height: 11rem;
      border-right: none;
      border-bottom: 1px solid var(--line);
    }
  }
</style>
