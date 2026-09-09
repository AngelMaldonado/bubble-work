<script lang="ts">
  // The wiki, in a thread's clothes.
  //
  // Same shell on purpose: a top bar that gets you back, the tree on the left,
  // the document in the middle, the heading rail on the right, the verbs in the
  // corner. A thread's document and a wiki page are the same KIND of thing —
  // markdown in a workspace's repo — and giving them two different readers
  // would mean learning the same screen twice.
  //
  // What is missing is what a page genuinely does not have: no band, no
  // priority, no assignees, no links or relations. A page is not work; it is
  // what the work knows.
  import { Portal, Tooltip, Navigation } from '@skeletonlabs/skeleton-svelte';
  import PanelLeftIcon from '@lucide/svelte/icons/panel-left';
  import FileTextIcon from '@lucide/svelte/icons/file-text';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import DocEditor from './DocEditor.svelte';
  import ThreadToc from './ThreadToc.svelte';
  import ThemeToggle from './ThemeToggle.svelte';
  import SideTree, { type TreeNode } from './SideTree.svelte';
  import type { Heading } from '../lib/prose';

  let {
    workspace,
    path,
    html = '',
    markdown = '',
    tree = [],
    onback,
    onopen,
    onsave,
    onattach,
    onsearch,
    onnew,
    ondelete,
  }: {
    workspace: string;
    /** the file, as it sits on disk: docs/arquitectura/overview.md */
    path: string;
    html?: string;
    /** the page's markdown — the record; `html` is the SERVER's render of it */
    markdown?: string;
    tree?: TreeNode[];
    onback?: () => void;
    onopen?: (id: string) => void;
    /** hand the edited markdown back; the caller owns the write and its hash */
    onsave?: (markdown: string) => void;
    /** adjuntar una imagen al workspace, desde el editor */
    onattach?: (file: File) => Promise<{ path: string } | null | void>;
    onsearch?: () => void;
    onnew?: () => void;
    ondelete?: () => void;
  } = $props();

  let sideCollapsed = $state(false);

  // Reading is the default and writing is a decision — the switch lives in the
  // document's bar, inside `DocEditor`.
  let editing = $state(false);
  let headings = $state<Heading[]>([]);
  let contentEl = $state<HTMLElement | null>(null);

  // The path IS the title. A page named by its own frontmatter and filed under a
  // different filename is a page you cannot find twice.
  const parts = $derived(path.split('/'));
  const name = $derived(parts.at(-1) ?? path);
  const folders = $derived(parts.slice(0, -1));

  // How many pages there are, counted from the tree rather than passed in: two
  // numbers that can disagree are one number too many.
  const pages = $derived(
    (function count(ns: TreeNode[]): number {
      return ns.reduce((n, x) => n + (x.children?.length ? count(x.children) : 1), 0);
    })(tree),
  );

  // No "+" here: the column already has it, pinned, and one screen offering the
  // same action twice is one of them going stale.
  // No "edit" verb here: the document's own bar carries the renderizado /
  // markdown switch, exactly as a thread's does. A second way in would be a
  // second thing to look for.
  const actions = $derived(
    [
      { k: 'search', face: '🔍', label: 'Buscar · ⌘K', go: onsearch },
      ondelete ? { k: 'delete', face: '🗑', label: 'Borrar la página', go: ondelete } : null,
    ].filter(Boolean) as { k: string; face: string; label: string; go?: () => void }[],
  );
</script>

<div class="screen" aria-label="wiki">
  <div class="topbar">
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <!-- The breadcrumb is the path, in the order the filesystem has it: where
         this page lives is half of what it is. -->
    <nav class="crumbs" aria-label="ruta">
      <span class="ws">{workspace}</span>
      {#each folders as f (f)}
        <span class="sep" aria-hidden="true">/</span><span class="dir">{f}</span>
      {/each}
      <span class="sep" aria-hidden="true">/</span><h2 class="ttl">{name}</h2>
    </nav>
    <span class="chips">
      <span class="chip">wiki</span>
    </span>
  </div>

  <div class="body">
    <Navigation
      layout={sideCollapsed ? 'rail' : 'sidebar'}
      class="side {sideCollapsed ? '' : 'grid grid-rows-[auto_1fr] gap-2'}">
      <Navigation.Content>
        <Navigation.Header>
          <Navigation.Trigger
            onclick={() => (sideCollapsed = !sideCollapsed)}
            title={sideCollapsed ? 'expandir' : 'colapsar'}>
            <PanelLeftIcon class={sideCollapsed ? 'size-5' : 'size-4'} />
            {#if !sideCollapsed}<span class="side-title">Wiki</span>{/if}
          </Navigation.Trigger>
        </Navigation.Header>

        {#if sideCollapsed}
          <!-- Collapsed, the rail keeps the count: how much there is to read is
               exactly what you lose by closing the column. -->
          <Navigation.Group class="wiki-tree">
            <Navigation.Menu>
              <Navigation.Trigger title="{pages} páginas" onclick={() => (sideCollapsed = false)}>
                <FileTextIcon class="size-5" />
                <Navigation.TriggerText>{pages}</Navigation.TriggerText>
              </Navigation.Trigger>
            </Navigation.Menu>
          </Navigation.Group>
        {:else}
          <Navigation.Group class="wiki-tree">
            <SideTree nodes={tree} onselect={(n) => onopen?.(n.id)} />
          </Navigation.Group>
        {/if}

        <!-- Pinned to the bottom in both layouts, like every other column here.
             Creating a page is not the last item of the tree — it is the one
             action the column always offers, so it sits where the column ends
             rather than drifting down as pages are added. -->
        <Navigation.Footer>
          <Navigation.Menu>
            <Navigation.Trigger onclick={onnew} title="nueva página">
              <PlusIcon class={sideCollapsed ? 'size-5' : 'size-4'} />
              <Navigation.TriggerText>Nueva</Navigation.TriggerText>
            </Navigation.Trigger>
          </Navigation.Menu>
        </Navigation.Footer>
      </Navigation.Content>
    </Navigation>

    <ThreadToc {headings} scroller={contentEl} />

    <div class="hud">
      {#each actions as a (a.k)}
        <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
          <Tooltip.Trigger>
            {#snippet element(attributes: Record<string, unknown>)}
              <button class="hud-btn" {...attributes} onclick={a.go}>
                <span aria-hidden="true">{a.face}</span>
              </button>
            {/snippet}
          </Tooltip.Trigger>
          <Portal>
            <Tooltip.Positioner>
              <Tooltip.Content>{a.label}</Tooltip.Content>
            </Tooltip.Positioner>
          </Portal>
        </Tooltip>
      {/each}
      <ThemeToggle floating={false} />
    </div>

    <main class="content" bind:this={contentEl}>
      <!-- The SAME pane a thread uses. A page and a thread's document are the
           same kind of thing — markdown in a repository, rendered by the server
           — so they are read and written by the same component rather than by
           two that drift. -->
      <DocEditor
    {onattach}
        {markdown}
        {html}
        bind:editing
        onsave={(md) => onsave?.(md)}
        onheadings={(h) => (headings = h)} />
    </main>
  </div>
</div>

<style>
  /* The shell holds still and the DOCUMENT scrolls — the same arrangement a
     thread uses, for the same reason. */
  .screen {
    --topbar-h: 46px;
    display: flex;
    flex-direction: column;
    height: 100dvh;
    overflow: hidden;
    background: transparent;
  }
  .topbar {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    height: var(--topbar-h);
    padding: 0 0.9rem;
    border-bottom: 1px solid var(--line);
    background: color-mix(in oklab, var(--bg) 82%, transparent);
    backdrop-filter: blur(8px);
  }
  .back {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.82rem;
  }
  .back:hover { color: var(--text); background: var(--hover); }

  .crumbs {
    display: flex;
    align-items: baseline;
    gap: 0.35rem;
    min-width: 0;
    overflow: hidden;
  }
  .ws, .dir, .sep { flex: none; font-size: 0.82rem; color: var(--faint); }
  .ttl {
    margin: 0;
    min-width: 0;
    font-size: 0.95rem;
    font-weight: 700;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chips { margin-left: auto; display: flex; gap: 0.4rem; }
  .chip {
    padding: 0.1rem 0.55rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    color: var(--faint);
    font-size: 0.72rem;
  }

  .body { flex: 1; min-height: 0; display: flex; align-items: stretch; }
  /* `:global`, because `class="side"` is a prop handed to a component and
     Svelte only stamps its scope hash on elements it writes itself. */
  .body :global(.side) {
    flex: none;
    height: 100%;
    border-right: 1px solid var(--line);
    background: transparent;
  }
  /* A flex chain, not a percentage. `height: 100%` on the content resolved
     against a root whose own height comes from `align-items: stretch` — not a
     definite height — so the percentage fell back to auto and the column was as
     tall as its contents. Flex asks no such question. */
  .body :global(.side) { display: flex; flex-direction: column; overflow: hidden; }
  .body :global([data-part='content'][data-layout='sidebar']) {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 0.5rem;
  }
  .body :global([data-part='group']) { align-items: stretch; min-width: 0; }
  /* The TREE scrolls, not the column, so the footer never moves. */
  .body :global(.wiki-tree) {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    /* The bar rides the column's own edge instead of floating in the padding. */
    width: auto;
    margin-inline: -1rem;
    /* More on the right than the left: the scrollbar lives there, and without
       the extra the counts sat against it. */
    padding-inline: 1rem 1.5rem;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  /* Collapsed, the thumb is a second vertical line in a 100px column. */
  .body :global(.side[data-layout='rail'] .wiki-tree) { scrollbar-width: none; }
  .body :global(.side[data-layout='rail'] .wiki-tree::-webkit-scrollbar) { display: none; }
  .body :global([data-part='footer']) {
    margin-top: auto;
    align-self: stretch;
    width: 100%;
    padding-top: 0.4rem;
    border-top: 1px solid var(--line);
  }
  .side-title {
    font-size: 0.7rem;
    font-weight: 800;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--faint);
  }

  .content { flex: 1; min-width: 0; overflow-y: auto; padding: 1rem 1.5rem 4rem; }

  .hud {
    position: fixed;
    right: 1rem;
    bottom: 1rem;
    z-index: var(--z-chrome);
    display: flex;
    gap: 0.6rem;
  }
  .hud-btn {
    width: 42px;
    height: 42px;
    display: grid;
    place-content: center;
    font-size: 1.15rem;
    line-height: 1;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    transition: transform 0.15s ease, background 0.15s ease;
  }
  .hud-btn:hover,
  .hud :global(.theme-toggle:hover) { background: var(--hover); transform: translateY(-1px); }
  .hud-btn:active { transform: translateY(0); }
</style>
