<script lang="ts">
  import { bandFace, bandName } from '../lib/bands';
  // A thread's interior — a faithful port of v0's ThreadView.
  //
  // The shape was right and is kept: a sticky top bar carrying the title and the
  // chips, a collapsible sidebar card that stays in view while the page scrolls,
  // and the document taking the rest. The window scrolls, not a pane, so the bar
  // reliably blurs what passes under it.
  //
  // Three things are gone, and only because the MODEL dropped them, not because
  // the design did:
  //   · the region tabs (document / logbook / DoD) — a thread has one document
  //     now, in whatever shape its author gave it;
  //   · the `kind` chip — there is no thread type;
  //   · the "from Plane" chip — there is no Plane.
  // Everything else is where v0 put it.
  import { Portal, Menu, Navigation, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import PanelLeftIcon from '@lucide/svelte/icons/panel-left';
  import FileTextIcon from '@lucide/svelte/icons/file-text';
  import LinkIcon from '@lucide/svelte/icons/link';
  import ShuffleIcon from '@lucide/svelte/icons/shuffle';
  import HistoryIcon from '@lucide/svelte/icons/history';
  import HashIcon from '@lucide/svelte/icons/hash';
  import type { Lifecycle } from '../lib/api';
  import Prose from './Prose.svelte';
  import SideTree, { type TreeNode } from './SideTree.svelte';
  import ThreadToc from './ThreadToc.svelte';
  import ThemeToggle from './ThemeToggle.svelte';
  import { untrack } from 'svelte';
  import type { MarkdownEditor } from '../lib/editor';
  import { vimPref } from '../lib/vim.svelte';
  import type { Heading } from '../lib/prose';

  let {
    seq,
    title,
    lifecycle,
    reason = '',
    priority = '',
    threadState = '',
    assignees = [],
    labels = [],
    markdown = '',
    html = '',
    links = [],
    related = [],
    revisions = [],
    pinned = false,
    elsewhere = false,
    onback,
    onsearch,
    onfinish,
    onreopen,
    onmove,
    ondelete,
  }: {
    seq: number;
    title: string;
    lifecycle: Lifecycle;
    reason?: string;
    priority?: string;
    // NOT `state`: a prop by that name shadows the `$state` rune — Svelte reads
    // `$state(...)` as a store subscription on the prop and every piece of local
    // reactivity in the component silently stops working.
    threadState?: string;
    assignees?: string[];
    labels?: string[];
    /** the markdown: the record, and what an edit quotes from */
    markdown?: string;
    /** the SERVER's render of that same markdown */
    html?: string;
    links?: { id: string; url: string; title?: string }[];
    related?: { id: string; title: string; type: string }[];
    revisions?: { id: string; title: string }[];
    pinned?: boolean;
    elsewhere?: boolean;
    onback?: () => void;
    /** open the omnibar — the one field that finds a thread, a bubble or a page */
    onsearch?: () => void;
    onfinish?: () => void;
    onreopen?: () => void;
    onmove?: () => void;
    ondelete?: () => void;
  } = $props();


  let sideCollapsed = $state(false);
  let editing = $state(false);
  // Seeded from the prop and owned locally afterwards: pinning is the reader's
  // decision, and it should not blink back when the parent re-renders.
  let isPinned = $state(false);
  $effect(() => {
    isPinned = pinned;
  });
  let addingLink = $state(false);
  // The table of contents comes from the RENDERED document, not from parsing the
  // markdown a second time: the ids it links to are the ones goldmark produced,
  // and re-deriving them here is how a link ends up pointing at nothing.
  let headings = $state<Heading[]>([]);

  // The editor is built on demand and torn down when the mode changes, so
  // CodeMirror never exists while you are reading. `@attach` gives us the box
  // and the cleanup in one place.
  //
  // The caret position survives the round trip: switching to rendered and back
  // should not send you to the top of a long document.
  let editor: MarkdownEditor | null = null;
  let caretAt = 0;
  let draft = $state('');
  $effect(() => {
    draft = markdown;
  });

  function mountEditor(el: HTMLElement) {
    // What this attachment depends on, spelled out. `vimPref.on` is READ here,
    // synchronously, because toggling vim has to rebuild the editor — read
    // inside the dynamic import's callback it is outside the reactive context
    // and the toggle does nothing. `draft` is read UNTRACKED for the opposite
    // reason: it changes on every keystroke, and tracking it would tear the
    // editor down and build a new one per character.
    const useVim = vimPref.on;
    const doc = untrack(() => draft);

    let live = true;
    let made: MarkdownEditor | null = null;
    // Imported HERE, not at the top of the file. A static import puts
    // CodeMirror and its markdown grammar in the main bundle, which everyone
    // downloads to look at a board they may never edit — measured at +240 kB
    // gzip before this line was a function call.
    import('../lib/editor').then(({ createMarkdownEditor }) => createMarkdownEditor({
      parent: el,
      doc,
      dark: document.documentElement.getAttribute('data-mode') === 'dark',
      vim: useVim,
      cursor: caretAt,
      onChange: (doc) => (draft = doc),
      onSave: () => {},
      onEscape: () => (editing = false),
      onBlur: () => {},
    })).then((made_) => {
      // The mode can change while the dynamic import is in flight; without this
      // the editor lands in a box that is no longer on the page.
      if (!live) return made_.destroy();
      made = made_;
      editor = made_;
      made_.focus();
    });
    return () => {
      live = false;
      caretAt = made?.cursor() ?? caretAt;
      made?.destroy();
      if (editor === made) editor = null;
    };
  }

  // The HUD's verbs, in reading order. Finishing first because it is what the
  // thread is for; deleting last and tinted, because it is the one that cannot
  // be taken back.
  const actions = $derived([
    lifecycle === 'closed'
      ? { k: 'reopen', face: '↩', label: 'Reabrir', go: onreopen }
      : { k: 'finish', face: '🏆', label: 'Terminar', go: onfinish },
    { k: 'move', face: '↔', label: 'Mover de burbuja', go: onmove },
    { k: 'search', face: '🔍', label: 'Buscar · ⌘K', go: onsearch },
    { k: 'delete', face: '🗑', label: 'Borrar el thread', tone: 'danger', go: ondelete },
  ]);

  // The headings nest by depth, so the table of contents is a real tree rather
  // than a flat list wearing indentation.
  function nestHeadings(hs: Heading[]): TreeNode[] {
    const roots: TreeNode[] = [];
    const stack: { depth: number; node: TreeNode }[] = [];
    for (const h of hs) {
      const node: TreeNode = { id: `h:${h.id}`, name: h.title, icon: HashIcon, href: `#${h.id}` };
      while (stack.length && stack[stack.length - 1].depth >= h.depth) stack.pop();
      if (stack.length) (stack[stack.length - 1].node.children ??= []).push(node);
      else roots.push(node);
      stack.push({ depth: h.depth, node });
    }
    return roots;
  }

  const tree = $derived<TreeNode[]>([
    {
      id: 'work',
      name: 'Trabajo',
      count: headings.length,
      children: [{ id: 'doc', name: title, icon: FileTextIcon }, ...nestHeadings(headings)],
    },
    {
      id: 'links',
      name: 'Enlaces',
      count: links.length,
      children: links.length
        ? links.map((l) => ({
            id: `link:${l.id}`, name: l.title || l.url, icon: LinkIcon,
            href: l.url, external: true,
          }))
        // A section with nothing in it still has to render as a section — the
        // "+" that fills it lives on the branch row.
        : [{ id: 'links:empty', name: 'sin enlaces' }],
    },
    {
      id: 'related',
      name: 'Relacionados',
      count: related.length,
      children: related.length
        ? related.map((r) => ({
            id: `rel:${r.id}:${r.type}`, name: r.title, icon: ShuffleIcon,
            badge: r.type.replace('_', ' '),
          }))
        : [{ id: 'related:empty', name: 'sin relaciones' }],
    },
    ...(revisions.length
      ? [{
          id: 'revisions', name: 'Revisiones',
          children: revisions.map((r) => ({ id: `rev:${r.id}`, name: r.title, icon: HistoryIcon })),
        }]
      : []),
  ]);

  // The collapsed rail: the same counts, one icon each. Lucide rather than
  // emoji — an emoji is the LIFECYCLE's alphabet here, and reusing it for
  // furniture makes the one signal that matters harder to pick out.
  const rail = $derived([
    { icon: FileTextIcon, label: 'secciones', n: headings.length },
    { icon: LinkIcon, label: 'enlaces', n: links.length },
    { icon: ShuffleIcon, label: 'relaciones', n: related.length },
    { icon: HistoryIcon, label: 'revisiones', n: revisions.length },
  ]);
</script>

<div class="screen band-{lifecycle}" aria-label="thread">
  <div class="topbar">
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <span class="seq">#{seq}</span>
    <!-- The title is the thread's NAME, a property of the record rather than
         content inside it, so it is renamed on its own path and never through
         the document. -->
    <!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
    <h2
      class="ttl edit"
      contenteditable="plaintext-only"
      role="textbox"
      aria-label="renombrar"
      tabindex="0"
      spellcheck="false"
      title="clic para renombrar">{title}</h2>

    <div class="chips">
      {#each labels as l (l)}<span class="chip">{l}</span>{/each}
      <button class="chip add" onclick={() => {}} title="editar etiquetas">
        {labels.length ? '✎' : '+ etiquetas'}
      </button>
      <!-- The thread's own buoyancy, which subsumes open/closed. -->
      <span class="chip lvl lvl-{lifecycle}" title={reason}>
        {bandFace(lifecycle)} {bandName(lifecycle)}
      </span>
      {#if threadState}<span class="chip who" title="estado">{threadState}</span>{/if}
      {#if priority}<span class="chip">{priority}</span>{/if}
      {#if assignees.length}<span class="chip who">{assignees.join(', ')}</span>{/if}
    </div>

    <button
      class="pin-btn"
      class:on={isPinned}
      onclick={() => (isPinned = !isPinned)}
      aria-pressed={isPinned}
      title={isPinned ? 'quitar el pin' : 'fijar este artefacto'}>
      <span aria-hidden="true">📌</span>
    </button>
  </div>

  <div class="body">
    <!-- Skeleton's Navigation, in the shape its own example uses: Header and
         Menu INSIDE Content, the toggle as a Trigger rather than a button of
         ours, and the two layouts it already has — `sidebar` and `rail`. The
         rail is not a narrower sidebar, it is its own layout: square icon
         triggers, no labels, and it animates its own width. -->
    <Navigation
      layout={sideCollapsed ? 'rail' : 'sidebar'}
      class="side {sideCollapsed ? '' : 'grid grid-rows-[auto_1fr] gap-2'}">
      <Navigation.Content>
        <Navigation.Header>
          <Navigation.Trigger
            onclick={() => (sideCollapsed = !sideCollapsed)}
            title={sideCollapsed ? 'expandir' : 'colapsar'}>
            <PanelLeftIcon class={sideCollapsed ? 'size-5' : 'size-4'} />
            {#if !sideCollapsed}<span class="side-title">Contenido</span>{/if}
          </Navigation.Trigger>
        </Navigation.Header>

        {#if sideCollapsed}
          <!-- What is in the thread is exactly what you lose by closing the
               sidebar, so it is the one thing the rail keeps: an icon per kind
               of artifact, and how many. -->
          <Navigation.Menu>
            {#each rail as r (r.label)}
              {@const Icon = r.icon}
              <Navigation.Trigger title="{r.n} {r.label}" onclick={() => (sideCollapsed = false)}>
                <Icon class="size-5" />
                <Navigation.TriggerText>{r.n}</Navigation.TriggerText>
              </Navigation.Trigger>
            {/each}
          </Navigation.Menu>
        {:else}
          <Navigation.Group>
            <SideTree nodes={tree}>
              {#snippet branchActions(node)}
                {#if node.id === 'links'}
                  <button class="sect-add" onclick={() => (addingLink = !addingLink)} title="añadir enlace">+</button>
                {:else if node.id === 'related'}
                  <!-- Relating is a SEARCH: you have to find the other thread
                       before you can say how it relates. A picker and a text
                       field nailed to the bottom of the column were answering
                       the second half first, so this opens the omnibar and the
                       kind of relation is asked once you have picked one. -->
                  <button class="sect-add" onclick={onsearch} title="relacionar (buscar)">+</button>
                {/if}
              {/snippet}
              {#snippet itemActions(node)}
                {#if node.id.startsWith('link:')}
                  <button class="pin-x" aria-label="quitar enlace">×</button>
                {:else if node.id.startsWith('rel:')}
                  <button class="pin-x" aria-label="quitar relación">×</button>
                {/if}
              {/snippet}
            </SideTree>

            {#if addingLink}
              <form class="linkform" onsubmit={(e) => e.preventDefault()}>
                <input placeholder="https://…" />
                <input placeholder="título" />
                <button type="submit">añadir</button>
              </form>
            {/if}
          </Navigation.Group>
        {/if}
      </Navigation.Content>
    </Navigation>

    <!-- The heading rail: where you are in the document, and what else is in
         it, without opening the sidebar. -->
    <ThreadToc {headings} />

    <!-- The thread's verbs, in the same place and the same shape as the board's:
         a thread covers the board and its HUD, so what you can do to it has to
         be reachable from the same corner. Finish leads — it is the act the
         whole thread exists to reach — and delete sits at the far end, away
         from it. -->
    <div class="hud">
      {#each actions as a (a.k)}
        <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
          <Tooltip.Trigger>
            {#snippet element(attributes: Record<string, unknown>)}
              <button class="hud-btn {a.tone ?? ''}" {...attributes} onclick={a.go}>
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
      <!-- Last, and the same button the board uses: a control that changes place
           between screens is a control people stop looking for. -->
      <ThemeToggle floating={false} />
    </div>

    <main class="content">
      <!-- Only the view switch stays in the document's own bar: it changes how
           you READ this page, so it belongs to the page. What you can DO to the
           thread moved to the HUD, where the board keeps its verbs. -->
      <div class="edit-bar">
        <div class="seg" role="group" aria-label="modo de vista">
          <button class:on={!editing} aria-pressed={!editing} onclick={() => (editing = false)}>renderizado</button>
          <button class:on={editing} aria-pressed={editing} onclick={() => (editing = true)}>markdown</button>
        </div>
        {#if editing}
          <!-- Only while there is an editor to apply it to. A preference for how
               to type, shown where you chose to type. -->
          <button
            class="vim"
            class:on={vimPref.on}
            aria-pressed={vimPref.on}
            title="teclas de vim ({vimPref.on ? 'activadas' : 'desactivadas'})"
            onclick={() => vimPref.toggle()}>vim</button>
        {/if}
      </div>

      <!-- Somebody else wrote while this was open. Not an error and not a
           refusal: the base hash says so, and reloading is a choice offered
           rather than a save silently lost. -->
      {#if elsewhere}
        <p class="elsewhere">
          Este documento cambió en otro lado.
          <button type="button" class="link">recargar</button>
        </p>
      {/if}

      {#if editing}
        <!-- CodeMirror mounts into this box. It was a bare <textarea>, which is
             the honest first cut and a poor one: no highlighting, no list
             continuation, no undo grouping. `lang-markdown` brings the two
             commands that make markdown editing feel like markdown — Enter
             continues a list or a checkbox, Backspace unwinds the marker. -->
        <div class="editors" {@attach mountEditor}></div>
      {:else}
        <Prose {html} onheadings={(h) => (headings = h)} />
      {/if}
    </main>
  </div>
</div>

<style>
  /* The shell holds still and the DOCUMENT scrolls, the same way the board
     works: the tree and the top bar are not in the scrolling box, so they stay
     put because of where they are rather than because they are pinned on top of
     something moving. */
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
  .back:hover { color: var(--text); background: var(--hover); }

  .seq {
    flex: none;
    font-size: 0.82rem;
    font-weight: 700;
    color: var(--faint);
    font-variant-numeric: tabular-nums;
  }
  .ttl {
    flex: 0 1 auto;
    min-width: 0;
    margin: 0;
    font-size: 1rem;
    font-weight: 700;
    letter-spacing: -0.01em;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* Invisible until you go near it, so the bar still reads as a heading rather
     than a form. */
  .ttl.edit { border-radius: 7px; padding: 0 0.3rem; margin-left: -0.3rem; outline: none; cursor: text; }
  .ttl.edit:hover { background: color-mix(in oklab, var(--text) 7%, transparent); }
  .ttl.edit:focus { background: color-mix(in oklab, var(--accent) 12%, transparent); box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--accent) 50%, transparent); }

  .chips { display: flex; flex-wrap: nowrap; gap: 0.35rem; margin-left: auto; flex: none; }
  .chip {
    font-size: 0.72rem;
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    color: var(--muted);
    text-transform: lowercase;
    white-space: nowrap;
  }
  .chip.add { background: none; cursor: pointer; }
  .chip.lvl { text-transform: none; font-weight: 600; color: var(--accent); border-color: color-mix(in oklab, var(--accent) 45%, transparent); }
  .chip.who { text-transform: none; }

  .pin-btn {
    flex: none;
    display: inline-grid;
    place-content: center;
    width: 1.9rem;
    height: 1.9rem;
    border-radius: 999px;
    border: 1px solid transparent;
    background: transparent;
    cursor: pointer;
    font-size: 0.9rem;
    filter: grayscale(1) opacity(0.55);
  }
  .pin-btn:hover { background: var(--hover); filter: grayscale(0.4) opacity(0.9); }
  .pin-btn.on {
    filter: none;
    background: color-mix(in oklab, var(--accent) 18%, transparent);
    border-color: color-mix(in oklab, var(--accent) 35%, transparent);
  }

  /* `items-stretch`, as Skeleton's own example does it: Navigation asks for
     `height: 100%`, which means nothing unless the row lets it stretch. */
  .body { flex: 1; min-height: 0; display: flex; align-items: stretch; }

  /* The file tree lives in Skeleton's Navigation, which sets the width, the
     padding and the 200ms width transition per layout. Ours is the hairline
     that separates it from the document and the fact that it does not float:
     it is in the flow and scrolls with the page. */
  /* Skeleton aligns the navigation column to `start`, which makes every child
     as wide as its own text — so the tree had no width to be too long for and
     nothing ever ellipsised. Stretch it, and constrain the group. */
  /* Anchored on `.body`, not on `.side`: `class="side"` is a PROP handed to the
     Navigation component, so Svelte never stamps its scope hash on it and
     `.side :global(...)` matched nothing. `.body` is a real element here. */
  .body :global([data-part='content'][data-layout='sidebar']),
  .body :global([data-part='group']) {
    align-items: stretch;
    min-width: 0;
  }

  /* The column runs the full viewport. It was as tall as its own contents, so
     the hairline stopped where the tree stopped and the panel ended mid-page. */
  /* `:global`, and for the same reason as the rule above: `class="side"` is a
     prop handed to a component, so Svelte never stamps its scope hash on the
     element. `.side { … }` compiled to `.side.svelte-xxx` and matched nothing —
     which is why the column kept ending where its contents did. */
  .body :global(.side) {
    flex: none;
    /* The row already gives it the full height; `100dvh` here would add the top
       bar's height on top of it and push the bottom off screen. */
    height: 100%;
    border-right: 1px solid var(--line);
    background: transparent;
  }
  .side-title {
    font-size: 0.7rem; font-weight: 800; letter-spacing: 0.09em;
    text-transform: uppercase; color: var(--faint);
  }

  .sect-add { border: none; background: none; color: var(--faint); cursor: pointer; }
  .pin-x { flex: none; border: none; background: none; color: var(--faint); cursor: pointer; padding: 0 0.4rem; }
  .pin-x:hover { color: var(--text); }
  .linkform { display: flex; flex-direction: column; gap: 0.3rem; padding: 0.25rem 0.4rem 0.5rem; }
  .linkform input {
    border: 1px solid var(--line); border-radius: 8px;
    background: var(--surface-solid); color: var(--text);
    padding: 0.3rem 0.45rem; font-size: 0.8rem;
  }
  .linkform button {
    border: 1px solid var(--line); border-radius: 8px;
    background: var(--surface); color: var(--text);
    padding: 0.3rem; font-size: 0.8rem; cursor: pointer;
  }

  .content { flex: 1; min-width: 0; overflow-y: auto; padding: 1rem 1.5rem 4rem; }

  /* Same corner and same shape as the board's HUD: it is the same kind of
     control, and a verb that changes place between screens is a verb people
     stop looking for. */
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
  /* The only one that cannot be undone says so before it is pressed. */
  .hud-btn.danger:hover {
    background: color-mix(in oklab, var(--color-error-500) 16%, transparent);
    border-color: color-mix(in oklab, var(--color-error-500) 45%, transparent);
  }
  .edit-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem; margin-bottom: 1rem; }
  .edit-bar button {
    border: 1px solid var(--line); border-radius: 999px;
    background: var(--surface-solid); color: var(--muted);
    padding: 0.25rem 0.7rem; font-size: 0.78rem; cursor: pointer;
  }
  .edit-bar button:hover { color: var(--text); background: var(--hover); }
  .edit-bar .finish { color: var(--warm); border-color: color-mix(in oklab, var(--warm) 45%, transparent); }
  .edit-bar .danger:hover { color: var(--hot); border-color: color-mix(in oklab, var(--hot) 45%, transparent); }
  .seg { margin-left: auto; display: flex; border: 1px solid var(--line); border-radius: 999px; overflow: hidden; }
  .seg button { border: none; border-radius: 0; padding: 0.25rem 0.75rem; }
  .seg button.on { background: var(--hover); color: var(--text); }

  .elsewhere {
    margin: 0 0 1rem;
    padding: 0.5rem 0.75rem;
    border-radius: 10px;
    font-size: 0.82rem;
    color: var(--text);
    background: color-mix(in oklab, var(--warm) 16%, transparent);
    border: 1px solid color-mix(in oklab, var(--warm) 40%, transparent);
  }
  .elsewhere .link { border: none; background: none; color: inherit; text-decoration: underline; cursor: pointer; padding: 0; }

  .prose { max-width: 72ch; white-space: pre-wrap; line-height: 1.65; }
  /* The box CodeMirror mounts into. `overflow: hidden` so the rounded corners
     clip its scroller, and the height is fixed so the editor scrolls itself
     rather than growing the page under it. */
  .editors {
    max-width: 72ch;
    height: calc(100dvh - var(--topbar-h) - 8rem);
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface-solid);
    overflow: hidden;
  }
  /* vim's own status line, themed to match the rest. */
  .editors :global(.cm-vim-panel) {
    padding: 0.2rem 0.6rem;
    border-top: 1px solid var(--line);
    background: var(--surface);
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.78rem;
  }
  .editors :global(.cm-vim-panel input) { color: var(--text); background: transparent; }

  .vim {
    padding: 0.28rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: transparent;
    color: var(--faint);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.76rem;
  }
  .vim:hover { color: var(--text); background: var(--hover); }
  .vim.on { color: var(--accent); border-color: color-mix(in oklab, var(--accent) 45%, transparent); }
</style>
