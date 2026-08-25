<script lang="ts">
  // A workspace's documentation, as a SCREEN — the same shape as the thread
  // interior (docs/journal/ARTIFACT-EDITING.md, docs/journal/PAGES-CAPABILITY.md).
  //
  // It used to be a modal, which was wrong for what this is. A spec is something
  // you read for twenty minutes with the sidebar open, follow a heading rail
  // through, and come back to; a modal is for a decision you make in five
  // seconds. So it now borrows the thread interior's chrome exactly: sticky
  // topbar with a way back, a collapsible sidebar on the left, the document in a
  // reading column, the heading rail on the right.
  //
  // The document itself is rendered by Prose — the SAME component a thread
  // artifact and a chat message use — so pages finally get diagrams, heading ids
  // and the reading typography instead of a bare {@html}.
  //
  // The sidebar is a tree. Plane models pages with a parent and shows them nested
  // in its own UI, so the hierarchy is read from it rather than invented here;
  // locally-held pages carry the same field.
  import { slide } from 'svelte/transition';
  import { api, ApiError } from '../lib/api';
  import { t } from '../lib/i18n.svelte';
  import { store } from '../lib/store.svelte';
  import type { Page, PageDetail } from '../lib/types';
  import type { Heading } from '../lib/prose';
  import ArtifactEditor from './ArtifactEditor.svelte';
  import ConfirmDelete from './ConfirmDelete.svelte';
  import Prose from './Prose.svelte';
  import ThreadToc from './ThreadToc.svelte';

  let {
    workspaceId,
    workspaceName,
    onclose,
  }: { workspaceId: string; workspaceName: string; onclose: () => void } = $props();

  let pages = $state<Page[]>([]);
  // Whether Plane is the record here. Plane Community serves pages only on its
  // internal session-authenticated API, so on those instances this server holds
  // them — which the reader has to be told, or they will go looking for a spec in
  // Plane's UI, not find it, and conclude the save failed.
  let planeHoldsPages = $state(true);
  let detail = $state<PageDetail | null>(null);
  let loading = $state(true);
  let bodyLoading = $state(false);
  let error = $state<string | null>(null);
  let editing = $state(false);
  let pendingDelete = $state<Page | null>(null);
  let deleting = $state(false);
  let headings = $state<Heading[]>([]);
  let sideCollapsed = $state(false);

  // Creating and renaming share one inline field: both ask for a title, and a
  // dialog for six characters of text is a dialog too many. `parent` carries
  // where a new page is going, so the same field serves "new page" and "new page
  // inside this one".
  let naming = $state<'new' | 'rename' | null>(null);
  let newParent = $state('');
  let draftTitle = $state('');
  let saving = $state(false);
  let titleField = $state<HTMLInputElement | null>(null);

  $effect(() => {
    if (naming) titleField?.focus();
  });

  // ---- the tree ----
  //
  // Which folders are open survives the session and the workspace: a tree that
  // re-collapses every time you open it makes you re-navigate to the document you
  // were just reading. Keyed by workspace so two do not fight over one setting.
  const OPEN_KEY = 'bubble.pagesOpen';

  function loadOpen(): Set<string> {
    try {
      const all = JSON.parse(localStorage.getItem(OPEN_KEY) ?? '{}') as Record<string, string[]>;
      return new Set(all[workspaceId] ?? []);
    } catch {
      return new Set();
    }
  }

  let openFolders = $state<Set<string>>(loadOpen());

  function persistOpen(): void {
    try {
      const all = JSON.parse(localStorage.getItem(OPEN_KEY) ?? '{}') as Record<string, string[]>;
      all[workspaceId] = [...openFolders];
      localStorage.setItem(OPEN_KEY, JSON.stringify(all));
    } catch {
      /* best-effort, like every other browser preference here */
    }
  }

  function toggleFolder(id: string): void {
    const next = new Set(openFolders);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    openFolders = next;
    persistOpen();
  }

  interface Row {
    page: Page;
    depth: number;
    kids: number;
    open: boolean;
  }

  // Flatten the tree into the rows to draw, honouring which folders are open.
  //
  // Roots are pages whose parent is missing from this list — not just pages with
  // no parent. A page whose parent was deleted, or lives in a project you cannot
  // see, would otherwise hang off nothing and simply never be drawn: present in
  // the database, absent from the only UI that can reach it. `seen` is the other
  // half of that: a parent cycle would loop forever, and a malformed tree should
  // degrade to a flat list, not hang the page.
  const rows = $derived.by<Row[]>(() => {
    const byId = new Map(pages.map((p) => [p.id, p]));
    const kids = new Map<string, Page[]>();
    const roots: Page[] = [];
    for (const p of pages) {
      if (p.parent && byId.has(p.parent)) {
        const list = kids.get(p.parent) ?? [];
        list.push(p);
        kids.set(p.parent, list);
      } else {
        roots.push(p);
      }
    }
    const out: Row[] = [];
    const seen = new Set<string>();
    const walk = (list: Page[], depth: number): void => {
      for (const p of list) {
        if (seen.has(p.id)) continue;
        seen.add(p.id);
        const children = kids.get(p.id) ?? [];
        const isOpen = openFolders.has(p.id);
        out.push({ page: p, depth, kids: children.length, open: isOpen });
        if (children.length && isOpen) walk(children, depth + 1);
      }
    };
    walk(roots, 0);
    // Anything the walk could not reach (a cycle) still gets drawn, flat, rather
    // than disappearing.
    for (const p of pages) {
      if (!seen.has(p.id)) out.push({ page: p, depth: 0, kids: 0, open: false });
    }
    return out;
  });

  // Opening a page reveals it: every folder above it is expanded, so selecting
  // from a search or a fresh load never lands on a row nobody can see.
  function revealPath(id: string): void {
    const byId = new Map(pages.map((p) => [p.id, p]));
    const next = new Set(openFolders);
    let cur = byId.get(id)?.parent;
    const guard = new Set<string>();
    while (cur && byId.has(cur) && !guard.has(cur)) {
      guard.add(cur);
      next.add(cur);
      cur = byId.get(cur)?.parent;
    }
    openFolders = next;
    persistOpen();
  }

  async function load(selectId?: string): Promise<void> {
    loading = true;
    error = null;
    try {
      const list = await api.pages(workspaceId);
      pages = list.pages;
      planeHoldsPages = list.plane_holds_pages;
      const want = selectId ?? detail?.id ?? pages[0]?.id;
      if (want && pages.some((p) => p.id === want)) await open(want);
      else detail = null;
    } catch (e) {
      // Reaching Plane can still fail for ordinary reasons — pages switched off
      // for the project, a rejected key. Worth saying plainly rather than
      // showing an empty reading room.
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
      revealPath(id);
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
        const d = await api.createPage(workspaceId, title, `# ${title}\n\n`, newParent || undefined);
        pages = [{ ...d }, ...pages];
        if (newParent) revealPath(d.id);
        detail = d;
        editing = true;
      } else if (detail) {
        const d = await api.updatePage(detail.id, { title });
        detail = { ...detail, title: d.title };
        pages = pages.map((p) => (p.id === d.id ? { ...p, title: d.title } : p));
      }
      naming = null;
      draftTitle = '';
      newParent = '';
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
      pendingDelete = null;
      detail = null;
      // Reloaded rather than filtered locally: deleting a page promotes its
      // children, so the tree's shape changed and guessing the new one here would
      // be a second implementation of a rule the server already applied.
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e);
    } finally {
      deleting = false;
    }
  }

  function startNew(parent = ''): void {
    naming = 'new';
    newParent = parent;
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

<div class="screen" aria-label={t('pages.title')}>
  <div class="topbar">
    <button class="back" onclick={onclose} aria-label={t('thread.back')}>
      <span aria-hidden="true">←</span> board
    </button>
    <h2 class="ttl">{t('pages.title')}</h2>
    <span class="ws">{workspaceName}</span>
    {#if !planeHoldsPages}
      <!-- In the topbar, not buried in the list: it is true of every page here,
           and it is what you need to know BEFORE writing a spec, not after. -->
      <span class="held" title={t('pages.heldHint')}>🫧 {t('pages.held')}</span>
    {/if}
  </div>

  <div class="body">
    <nav class="side" class:collapsed={sideCollapsed}>
      <div class="side-head">
        {#if !sideCollapsed}<span class="side-title">{t('pages.title')}</span>{/if}
        <button
          class="side-toggle"
          onclick={() => (sideCollapsed = !sideCollapsed)}
          aria-label={sideCollapsed ? 'expand sidebar' : 'collapse sidebar'}
          title={sideCollapsed ? 'expand' : 'collapse'}
        >
          {sideCollapsed ? '☰' : '‹'}
        </button>
      </div>

      {#if !sideCollapsed}
        <!-- the same collapse motion the thread interior uses -->
        <div class="side-scroll" transition:slide={{ duration: 220, axis: 'y' }}>
          {#if !store.kiosk}
            <button class="newpage" onclick={() => startNew()}>＋ {t('pages.new')}</button>
          {/if}
          {#if naming === 'new' && !newParent}
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
          {:else if rows.length === 0}
            <p class="dim">{t('pages.empty')}</p>
          {:else}
            {#each rows as r (r.page.id)}
              <div class="row" style="--depth: {r.depth}">
                <!-- The twisty is its own control, so opening a folder and reading
                     it are separate acts — clicking a title always opens the
                     document, which is what a title looks like it does. -->
                {#if r.kids}
                  <button
                    class="twisty"
                    onclick={() => toggleFolder(r.page.id)}
                    aria-expanded={r.open}
                    aria-label={r.open ? 'collapse' : 'expand'}
                  >
                    {r.open ? '▾' : '▸'}
                  </button>
                {:else}
                  <span class="twisty spacer" aria-hidden="true"></span>
                {/if}
                <button
                  class="item"
                  class:active={detail?.id === r.page.id}
                  onclick={() => open(r.page.id)}
                  title={r.page.title}
                >
                  <span class="ico">{r.kids ? (r.open ? '📂' : '📁') : '📄'}</span>
                  <span class="ptitle">{r.page.title}</span>
                  <!-- Only when Plane DOES hold pages: there a local page is the
                       exception worth marking. Where it does not, the topbar
                       already covers every page and a column of badges is noise. -->
                  {#if planeHoldsPages && r.page.storage === 'local'}
                    <span class="tag" title={t('pages.heldHint')}>🫧</span>
                  {/if}
                  {#if r.page.locked}<span class="tag" title={t('pages.locked')}>🔒</span>{/if}
                </button>
                {#if !store.kiosk}
                  <button
                    class="addkid"
                    onclick={() => startNew(r.page.id)}
                    title={t('pages.newInside')}
                    aria-label={t('pages.newInside')}>＋</button
                  >
                {/if}
              </div>
              {#if naming === 'new' && newParent === r.page.id}
                <form class="namer" style="--depth: {r.depth + 1}" onsubmit={submitTitle}>
                  <input
                    bind:this={titleField}
                    bind:value={draftTitle}
                    placeholder={t('pages.titlePlaceholder')}
                    disabled={saving}
                  />
                </form>
              {/if}
            {/each}
          {/if}
        </div>
      {/if}
    </nav>

    <main class="content">
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
            <span class="crumb">{detail.title}</span>
          {/if}
          <div class="acts">
            {#if !readOnly}
              <button onclick={startRename} title={t('pages.rename')}>✏️</button>
              <button onclick={() => startNew(detail!.id)} title={t('pages.newInside')}>＋</button>
              <div class="seg" role="group" aria-label={t('thread.viewMode')}>
                <button class:on={!editing} aria-pressed={!editing} onclick={() => (editing = false)}
                  >{t('thread.rendered')}</button
                >
                <button class:on={editing} aria-pressed={editing} onclick={() => (editing = true)}
                  >{t('thread.markdown')}</button
                >
              </div>
              <button class="danger" onclick={() => (pendingDelete = detail)} title={t('pages.delete')}
                >🗑</button
              >
            {:else if detail.locked}
              <span class="dim">🔒 {t('pages.locked')}</span>
            {/if}
          </div>
        </div>

        {#if editing && !readOnly}
          {#key detail.id}
            <ArtifactEditor
              threadId={detail.id}
              region="brief"
              initial={detail.markdown}
              hash={detail.hash}
              write={writeBody}
              onreload={() => open(detail!.id)}
              ondone={() => (editing = false)}
            />
          {/key}
        {:else}
          <!-- The same renderer a thread artifact gets: diagrams, heading ids,
               reading typography. -->
          <Prose html={detail.html} onheadings={(h) => (headings = h)} />
        {/if}
      {:else if !loading && !error}
        <p class="dim">{t('pages.pick')}</p>
      {/if}
    </main>

    {#if headings.length && !editing}
      <ThreadToc {headings} />
    {/if}
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
  /* The chrome is deliberately the thread interior's, rule for rule: the two are
     the same kind of place — a document with a sidebar — and a reader should not
     have to re-learn the furniture on the way from a Brief to the spec it is
     written against. */
  .screen {
    --topbar-h: 46px;
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    background: transparent;
  }
  .topbar {
    position: sticky;
    top: 0;
    z-index: 25;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.55rem 1.25rem;
    font-size: 0.75rem;
    color: var(--faint);
    background: color-mix(in oklab, var(--bg) 72%, transparent);
    backdrop-filter: blur(14px);
    border-bottom: 1px solid var(--line);
  }
  .back {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.7rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.75rem;
  }
  .back:hover {
    color: var(--text);
    border-color: color-mix(in oklab, var(--wip) 45%, var(--line));
  }
  .ttl {
    margin: 0;
    font-size: 0.9rem;
    font-weight: 750;
    color: var(--text);
  }
  .ws {
    color: var(--faint);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* A statement of fact, not a failure — informative rather than alarming. */
  .held {
    flex: none;
    margin-left: auto;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    font-size: 0.7rem;
    color: var(--muted);
    background: color-mix(in oklab, var(--text) 6%, transparent);
    border: 1px solid var(--line);
    cursor: help;
  }

  .body {
    flex: 1;
    display: flex;
    align-items: flex-start;
  }

  .side {
    position: sticky;
    top: calc(var(--topbar-h) + 0.8rem);
    flex: 0 0 256px;
    margin: 0.8rem 0 1rem 1rem;
    padding: 0.5rem;
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - var(--topbar-h) - 1.6rem);
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: color-mix(in oklab, var(--surface-solid) 60%, transparent);
    transition:
      flex-basis 0.24s ease,
      padding 0.24s ease;
  }
  /* Collapsed → just the toggle. align-self stops it stretching to the height of
     the document beside it, and the head has to drop its own padding: it is
     spacing for a title that is no longer there, and it left the button sitting
     off-centre in a rail taller than itself. */
  .side.collapsed {
    flex-basis: 46px;
    align-self: flex-start;
    padding: 0.4rem;
  }
  .side-head {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.15rem 0.4rem 0.35rem;
  }
  .side.collapsed .side-head {
    justify-content: center;
    padding: 0;
  }
  .side-title {
    flex: 1;
    min-width: 0;
    font-size: 0.66rem;
    font-weight: 800;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--faint);
    white-space: nowrap;
    overflow: hidden;
  }
  .side-toggle {
    flex: none;
    width: 28px;
    height: 28px;
    display: grid;
    place-content: center;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.9rem;
    line-height: 1;
  }
  .side-toggle:hover {
    color: var(--text);
  }
  .side-scroll {
    flex: 1 1 auto;
    min-height: 0;
    width: 236px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    padding: 0 0.1rem 0.25rem;
  }

  /* ---- the tree ---- */
  .row {
    display: flex;
    align-items: center;
    gap: 0.1rem;
    /* the indent that makes it a tree; the guide line makes depth readable
       without counting pixels */
    padding-left: calc(var(--depth) * 0.75rem);
  }
  .twisty {
    flex: none;
    width: 16px;
    height: 22px;
    display: grid;
    place-content: center;
    border: none;
    background: transparent;
    color: var(--faint);
    cursor: pointer;
    font-size: 0.6rem;
    padding: 0;
  }
  .twisty:hover {
    color: var(--text);
  }
  /* Keeps a leaf's title aligned with its siblings' — an unindented leaf next to
     a folder reads as a different level. */
  .twisty.spacer {
    cursor: default;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1;
    min-width: 0;
    text-align: left;
    padding: 0.35rem 0.4rem;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    font-size: 0.82rem;
    line-height: 1.2;
  }
  .item:hover {
    background: var(--hover);
    color: var(--text);
  }
  .item.active {
    background: color-mix(in oklab, var(--wip) 16%, transparent);
    color: var(--text);
    font-weight: 600;
  }
  .ico {
    font-size: 0.8rem;
    flex: none;
  }
  .ptitle {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tag {
    flex: none;
    font-size: 0.7rem;
  }
  /* Revealed on hover: every row having a visible ＋ turns the tree into a wall
     of buttons, and this is a reading surface first. */
  .addkid {
    flex: none;
    width: 20px;
    height: 22px;
    display: grid;
    place-content: center;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--faint);
    cursor: pointer;
    font-size: 0.75rem;
    opacity: 0;
    padding: 0;
  }
  .row:hover .addkid,
  .addkid:focus-visible {
    opacity: 1;
  }
  .addkid:hover {
    background: var(--hover);
    color: var(--text);
  }
  .newpage {
    width: 100%;
    margin-bottom: 0.3rem;
    padding: 0.4rem 0.5rem;
    border: 1px dashed var(--line);
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    font-size: 0.78rem;
  }
  .newpage:hover {
    color: var(--text);
    border-color: color-mix(in oklab, var(--wip) 50%, var(--line));
    background: color-mix(in oklab, var(--wip) 7%, transparent);
  }
  .namer {
    padding-left: calc(var(--depth, 0) * 0.75rem + 16px);
    margin: 0.15rem 0;
  }
  .namer input {
    width: 100%;
    padding: 0.35rem 0.5rem;
    border: 1px solid color-mix(in oklab, var(--wip) 45%, var(--line));
    border-radius: 8px;
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.8rem;
    outline: none;
  }
  .namer.wide {
    flex: 1;
    padding-left: 0;
  }

  .content {
    flex: 1;
    min-width: 0;
    padding: 1.4rem clamp(2.75rem, 5vw, 4rem) 6rem clamp(1.5rem, 4vw, 3.5rem);
  }
  .docbar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    max-width: 940px;
    margin: 0 auto 1.4rem;
    padding-bottom: 0.6rem;
    border-bottom: 1px solid var(--line);
  }
  .crumb {
    flex: 1;
    min-width: 0;
    font-size: 0.78rem;
    color: var(--faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .acts {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }
  .acts button {
    padding: 0.25rem 0.55rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.75rem;
  }
  .acts button:hover {
    color: var(--text);
    background: var(--hover);
  }
  .acts .danger:hover {
    color: oklch(0.62 0.2 25);
    border-color: color-mix(in oklab, oklch(0.62 0.2 25) 45%, var(--line));
  }
  .seg {
    display: flex;
    gap: 0;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid var(--line);
  }
  .seg button {
    border: none;
    border-radius: 0;
    font-size: 0.72rem;
  }
  .seg button.on {
    background: color-mix(in oklab, var(--wip) 18%, transparent);
    color: var(--text);
    font-weight: 600;
  }

  .dim {
    color: var(--faint);
    font-size: 0.8rem;
  }
  .err {
    max-width: 940px;
    margin: 0 auto 0.8rem;
    color: oklch(0.68 0.19 25);
    font-size: 0.82rem;
  }

  @media (max-width: 640px) {
    .side {
      flex-basis: 190px;
    }
    .side-scroll {
      width: 170px;
    }
    .content {
      padding: 1.2rem 1.1rem 3rem;
    }
  }
</style>
