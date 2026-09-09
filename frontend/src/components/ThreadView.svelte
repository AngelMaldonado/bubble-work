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
  import HistoryIcon from '@lucide/svelte/icons/history';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import type { Lifecycle } from '../lib/api';
  import DocEditor from './DocEditor.svelte';
  import SideTree, { type TreeNode } from './SideTree.svelte';
  import ThreadToc from './ThreadToc.svelte';
  import ThemeToggle from './ThemeToggle.svelte';
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
    revisions = [],
    pinned = false,
    elsewhere = false,
    onback,
    onsave,
    onreload,
    onattach,
    ondiff,
    pages = [],
    mainChurn,
    openPage = '',
    onopenpage,
    onnewpage,
    ondeletepage,
    onsearch,
    onfinish,
    onreopen,
    onmove,
    ondelete,
    ontalk,
    talk = 0,
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
    revisions?: { id: string; title: string }[];
    pinned?: boolean;
    elsewhere?: boolean;
    onback?: () => void;
    /** hand the edited markdown back. The caller owns the write, the hash and
     *  what to do when somebody else got there first. */
    onsave?: (markdown: string) => void;
    /** adjuntar una imagen al workspace, desde el editor */
    onattach?: (file: File) => Promise<{ path: string } | null | void>;
    /** qué cambió en este documento — lo calcula git, del lado del servidor */
    ondiff?: (since: string) => Promise<{ before: string; after: string; diff: string }>;
    /** what this thread wrote BESIDE its document, by path */
    pages?: { path: string; name: string; churn?: { added: number; removed: number } }[];
    /** cuánto se escribió en el documento propio, en este ciclo */
    mainChurn?: { added: number; removed: number };
    /** which of them is open; `''` is the thread's own document */
    openPage?: string;
    onopenpage?: (path: string) => void;
    onnewpage?: (name: string) => void;
    ondeletepage?: (path: string) => void;
    /** re-read the document, discarding what is in the editor */
    onreload?: () => void;
    /** open the omnibar — the one field that finds a thread, a bubble or a page */
    onsearch?: () => void;
    onfinish?: () => void;
    onreopen?: () => void;
    onmove?: () => void;
    ondelete?: () => void;
    /** abrir los comentarios */
    ontalk?: () => void;
    /** cuántos hay, para no tener que abrirlos para saberlo */
    talk?: number;
  } = $props();


  let sideCollapsed = $state(false);
  let addingPage = $state(false);
  let freshPage = $state('');
  let editing = $state(false);
  // Seeded from the prop and owned locally afterwards: pinning is the reader's
  // decision, and it should not blink back when the parent re-renders.
  let isPinned = $state(false);
  $effect(() => {
    isPinned = pinned;
  });
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



  // The HUD's verbs, in reading order. Finishing first because it is what the
  // thread is for; deleting last and tinted, because it is the one that cannot
  // be taken back.
  const actions = $derived([
    lifecycle === 'closed'
      ? { k: 'reopen', face: '↩', label: 'Reabrir', go: onreopen }
      : { k: 'finish', face: '🏆', label: 'Terminar', go: onfinish },
    { k: 'move', face: '↔', label: 'Mover de burbuja', go: onmove },
    { k: 'talk', face: '💬', label: talk ? `Comentarios (${talk})` : 'Comentarios', go: ontalk },
    { k: 'search', face: '🔍', label: 'Buscar · ⌘K', go: onsearch },
    { k: 'delete', face: '🗑', label: 'Borrar el thread', tone: 'danger', go: ondelete },
    // A verb nobody gave a handler is not drawn. The alternative is a button
    // that swallows the click, which reads as broken rather than as absent —
    // and the real view wires these one at a time.
  ].filter((a) => a.go));


  const tree = $derived<TreeNode[]>([
    {
      id: 'work',
      name: 'Trabajo',
      count: 1 + pages.length,
      // Los DOCUMENTOS del thread, y nada más. Los encabezados estuvieron aquí
      // un rato y sobraban: el riel de la derecha ya es la tabla de contenidos
      // —y además dice dónde vas leyendo, que un árbol no hace—, así que esto
      // repetía el mismo índice con menos información y le robaba el sitio a lo
      // único que esta columna contesta: qué archivos tiene este thread.
      children: [
        { id: 'doc', name: title, icon: FileTextIcon, churn: mainChurn },
        ...pages.map((p) => ({
          id: `page:${p.path}`, name: p.name, icon: FileTextIcon, churn: p.churn,
        })),
      ],
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
          <Navigation.Menu class="thread-tree">
            {#each rail as r (r.label)}
              {@const Icon = r.icon}
              <Navigation.Trigger title="{r.n} {r.label}" onclick={() => (sideCollapsed = false)}>
                <Icon class="size-5" />
                <Navigation.TriggerText>{r.n}</Navigation.TriggerText>
              </Navigation.Trigger>
            {/each}
          </Navigation.Menu>
        {:else}
          <Navigation.Group class="thread-tree">
            <SideTree
              nodes={tree}
              onselect={(node) => {
                if (node.id === 'doc') onopenpage?.('');
                else if (node.id.startsWith('page:')) onopenpage?.(node.id.slice(5));
              }}>
              {#snippet itemActions(node)}
                {#if node.id.startsWith('page:') && ondeletepage}
                  <button
                    class="pin-x"
                    aria-label="borrar {node.name}"
                    onclick={() => ondeletepage?.(node.id.slice(5))}>×</button>
                {/if}
              {/snippet}
            </SideTree>

          </Navigation.Group>
        {/if}
        <!-- Pinned to the bottom, like the wiki's: escribir otro documento no es
             el último elemento del árbol, es la acción que la columna siempre
             ofrece — así que vive donde la columna termina y no se va hacia
             abajo conforme se agregan páginas. -->
        {#if onnewpage}
          <Navigation.Footer>
            {#if addingPage && !sideCollapsed}
              <!-- Un thread tiene su documento y, si el trabajo lo pide, otros
                   al lado: una nota de investigación, un diseño, un registro
                   que no cabe en la página principal. -->
              <input
                class="new-page"
                placeholder="¿Cómo se llama?"
                autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                {@attach (el: HTMLInputElement) => el.focus()}
                bind:value={freshPage}
                onblur={() => (addingPage = false)}
                onkeydown={(e) => {
                  if (e.key === 'Escape') addingPage = false;
                  if (e.key === 'Enter' && freshPage.trim()) {
                    onnewpage?.(freshPage.trim());
                    freshPage = '';
                    addingPage = false;
                  }
                }} />
            {:else}
              <Navigation.Menu>
                <Navigation.Trigger
                  onclick={() => { sideCollapsed = false; addingPage = true; }}
                  title="otro documento">
                  <PlusIcon class={sideCollapsed ? 'size-5' : 'size-4'} />
                  <Navigation.TriggerText>Nuevo</Navigation.TriggerText>
                </Navigation.Trigger>
              </Navigation.Menu>
            {/if}
          </Navigation.Footer>
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
      <DocEditor
    {onattach}
    {ondiff}
        {markdown}
        {html}
        {elsewhere}
        bind:editing
        {onsave}
        {onreload}
        onheadings={(h) => (headings = h)} />
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
  /* La misma cadena flex que la wiki: la columna es una columna, el ÁRBOL es lo
     que scrollea, y el pie se queda donde termina la columna. Sin esto el
     contenido crecía y el botón se iba con él. */
  .body :global(.side) { display: flex; flex-direction: column; overflow: hidden; }
  .body :global([data-part='content'][data-layout='sidebar']) {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 0.5rem;
  }
  .body :global(.thread-tree) {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    /* La barra va sobre el borde de la columna, no flotando en el padding. */
    width: auto;
    margin-inline: -1rem;
    padding-inline: 1rem 1.5rem;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  /* Colapsada, el pulgar es una segunda línea vertical en 100px de columna. */
  .body :global(.side[data-layout='rail'] .thread-tree) { scrollbar-width: none; }
  .body :global(.side[data-layout='rail'] .thread-tree::-webkit-scrollbar) { display: none; }
  /* El separador: el pie es lo único que no se mueve, y la línea lo dice. */
  .body :global([data-part='footer']) {
    margin-top: auto;
    align-self: stretch;
    width: 100%;
    padding-top: 0.4rem;
    border-top: 1px solid var(--line);
  }

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

  .new-page {
    width: 100%;
    margin-top: 0.3rem;
    padding: 0.3rem 0.45rem;
    border: 1px solid var(--accent, var(--line));
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
    font-size: 0.82rem;
  }
  .pin-x { flex: none; border: none; background: none; color: var(--faint); cursor: pointer; padding: 0 0.4rem; }
  .pin-x:hover { color: var(--text); }

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
  .edit-bar button {
    border: 1px solid var(--line); border-radius: 999px;
    background: var(--surface-solid); color: var(--muted);
    padding: 0.25rem 0.7rem; font-size: 0.78rem; cursor: pointer;
  }
  .edit-bar button:hover { color: var(--text); background: var(--hover); }
  .edit-bar .finish { color: var(--warm); border-color: color-mix(in oklab, var(--warm) 45%, transparent); }
  .edit-bar .danger:hover { color: var(--hot); border-color: color-mix(in oklab, var(--hot) 45%, transparent); }

  .elsewhere .link { border: none; background: none; color: inherit; text-decoration: underline; cursor: pointer; padding: 0; }

  .prose { max-width: 940px; margin-inline: auto; white-space: pre-wrap; line-height: 1.65; }
  /* The box CodeMirror mounts into. `overflow: hidden` so the rounded corners
     clip its scroller, and the height is fixed so the editor scrolls itself
     rather than growing the page under it. */
  /* The positioning box IS the editor's box.
     The caret's coordinates come back relative to CodeMirror's own element, so
     the box they are applied against has to be that same rectangle. Wrapping a
     full-width div around a centred editor put the menu one margin to the left
     — which is exactly where it appeared. So the centring lives on the wrapper
     and the editor fills it. */
  /* vim's own status line, themed to match the rest. */

</style>
