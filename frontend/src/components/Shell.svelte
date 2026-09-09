<script lang="ts">
  // The app's frame: the column of workspaces on the left, and one scrolling
  // pane on the right that every screen is drawn into.
  //
  // Extracted from the mock rather than rewritten for it. The mock was where
  // this shape was worked out — the rail, the pinned "Nuevo", the list that
  // scrolls while the column does not — and a second copy of that CSS is how
  // the real screen and the drawing of it start disagreeing.
  //
  // The shell does NOT scroll; the pane does. The sidebar and the floating
  // buttons stay put because they are outside the scrolling box, not because
  // they are pinned on top of one.
  import { Navigation, Portal, Menu } from '@skeletonlabs/skeleton-svelte';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import MoreVerticalIcon from '@lucide/svelte/icons/more-vertical';
  import BrandOrb from './BrandOrb.svelte';
  import { edgeFade } from '../lib/fade.svelte';
  import type { Snippet } from 'svelte';

  export type ShellItem = { id: string; name: string; hint?: string };

  let {
    items = [],
    current = '',
    label = 'Proyectos',
    newLabel = 'Nuevo',
    pane = $bindable<HTMLElement | null>(null),
    onselect,
    onrename,
    ondelete,
    /** create one and say which it is, so it can be named straight away */
    oncreate,
    children,
  }: {
    items?: ShellItem[];
    current?: string;
    label?: string;
    newLabel?: string;
    pane?: HTMLElement | null;
    onselect?: (id: string) => void;
    onrename?: (id: string, name: string) => void;
    ondelete?: (id: string) => void;
    oncreate?: () => Promise<string | void> | string | void;
    children?: Snippet;
  } = $props();

  let railed = $state(false);
  let editing = $state<string | null>(null);
  let draft = $state('');

  // Both scrolling boxes fade at the edge that still has content behind it.
  const listFade = edgeFade();
  const paneFade = edgeFade();

  function startRename(id: string, name: string) {
    editing = id;
    draft = name;
  }
  function commitRename() {
    // An empty name is not a rename, it is a deletion nobody asked for.
    if (editing && draft.trim()) onrename?.(editing, draft.trim());
    editing = null;
  }
  async function create() {
    const id = await oncreate?.();
    if (typeof id === 'string' && id) startRename(id, '');
  }
</script>

<div class="shell">
  <!-- The workspaces live here. A workspace is the boundary for a body of work
       and the thing you switch between all day, so it gets the persistent
       column; the views get the floating buttons at the bottom right. -->
  <Navigation layout={railed ? 'rail' : 'sidebar'} class="shell-nav">
    <Navigation.Content>
      <Navigation.Header>
        <!-- The mark reads alone, which is exactly what the rail needs; the
             wordmark is what gets dropped when the column narrows, not the
             logo. -->
        <Navigation.Trigger onclick={() => (railed = !railed)} title={railed ? 'expandir' : 'colapsar'}>
          <BrandOrb size={railed ? 26 : 22} />
          {#if !railed}<span class="display text-base">bubble.work</span>{/if}
        </Navigation.Trigger>
      </Navigation.Header>

      <!-- The label sits OUTSIDE the scrolling list: a heading that scrolls
           away is a heading that stops labelling anything. -->
      {#if !railed}<Navigation.Label>{label}</Navigation.Label>{/if}

      <Navigation.Group class="projects" style={listFade.style} {@attach listFade.attach}>
        <Navigation.Menu>
          {#each items as p (p.id)}
            {#if editing === p.id && !railed}
              <!-- Renaming happens IN PLACE. A dialog for one field is a dialog
                   asking you to confirm you meant to type.
                   Focused explicitly, not with `autofocus`: the attribute only
                   acts on a page's first parse, so an input that appears later
                   never gets it — the typing went to the page instead. -->
              <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                class="rename"
                {@attach (el: HTMLInputElement) => { el.focus(); el.select(); }}
                bind:value={draft}
                onblur={commitRename}
                onkeydown={(e) => {
                  if (e.key === 'Enter') commitRename();
                  if (e.key === 'Escape') editing = null;
                }} />
            {:else}
              <div class="proj" class:on={current === p.id}>
                <Navigation.Trigger
                  onclick={() => onselect?.(p.id)}
                  ondblclick={() => startRename(p.id, p.name)}
                  title={p.hint ? `${p.name} · ${p.hint}` : p.name}>
                  <span class="pin" aria-hidden="true">{p.name.slice(0, 1)}</span>
                  <Navigation.TriggerText>{p.name}</Navigation.TriggerText>
                </Navigation.Trigger>
                {#if !railed && (onrename || ondelete)}
                  <Menu onSelect={(e: { value: string }) => {
                    if (e.value === 'rename') startRename(p.id, p.name);
                    if (e.value === 'delete') ondelete?.(p.id);
                  }}>
                    <Menu.Trigger>
                      <span class="proj-more" title="acciones"><MoreVerticalIcon class="size-4" /></span>
                    </Menu.Trigger>
                    <Portal>
                      <Menu.Positioner>
                        <Menu.Content>
                          {#if onrename}<Menu.Item value="rename"><Menu.ItemText>Renombrar</Menu.ItemText></Menu.Item>{/if}
                          {#if ondelete}<Menu.Item value="delete"><Menu.ItemText>Eliminar</Menu.ItemText></Menu.Item>{/if}
                        </Menu.Content>
                      </Menu.Positioner>
                    </Portal>
                  </Menu>
                {/if}
              </div>
            {/if}
          {/each}
        </Navigation.Menu>
      </Navigation.Group>

      <!-- Pinned to the bottom in both layouts. Creating one is not the last
           item of the list — it is the one action the column always offers, so
           it sits where the column ends rather than drifting down as the list
           grows. -->
      {#if oncreate}
        <Navigation.Footer class="new-foot">
          <Navigation.Menu>
            <Navigation.Trigger onclick={create} title={newLabel}>
              <PlusIcon class={railed ? 'size-5' : 'size-4'} />
              <Navigation.TriggerText>{newLabel}</Navigation.TriggerText>
            </Navigation.Trigger>
          </Navigation.Menu>
        </Navigation.Footer>
      {/if}
    </Navigation.Content>
  </Navigation>

  <div class="col">
    <main class="pane" bind:this={pane} style={paneFade.style} {@attach paneFade.attach}>
      {@render children?.()}
    </main>
  </div>
</div>

<style>
  /* Nav column, then everything else. `align-items: stretch` is what lets
     Navigation's own `height: 100%` mean something. */
  .shell {
    height: 100vh;
    overflow: hidden;
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: stretch;
  }
  .shell :global(.shell-nav) {
    border-right: 1px solid var(--line);
    background: transparent;
  }
  /* Header, projects and footer hang off `content`, not off the root, so the
     root's rows never reached them — the button pinned in the rail (a flex
     column) and floated mid-way in the sidebar. Make `content` the full-height
     column instead. Only in the sidebar layout: the rail's content is
     `display: contents`, and giving that a height would undo the rail. */
  /* A flex chain rather than a percentage: `height: 100%` resolves against the
     root's own height, which comes from `align-items: stretch` and is not
     definite, so the percentage falls back to auto and the column grows to its
     contents — taking the pinned footer past the bottom edge with it. */
  .shell :global(.shell-nav) { display: flex; flex-direction: column; overflow: hidden; }
  .shell :global(.shell-nav [data-part='content'][data-layout='sidebar']) {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
    /* Skeleton aligns this column to `start`, which makes every child as wide as
       its text. The rows need the column. */
    align-items: stretch;
    gap: 0.75rem;
  }
  /* The rail's group is `display: contents`, which has no box and therefore
     cannot scroll. It needs to be one. */
  .shell :global(.shell-nav[data-layout='rail'] .projects) {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  /* The LIST scrolls, not the column. With the overflow on the root, a long
     list pushed the footer past the bottom edge and the button went with it —
     the point of pinning it was that it never moves. */
  .shell :global(.shell-nav .projects) {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    /* The scrolling box spans the whole column, so its bar rides the sidebar's
       own edge instead of floating in the middle of the padding. The padding
       comes back inside, which leaves the rows exactly where they were. */
    /* `width: auto` matters: Skeleton sets `width: 100%` on the group, and a
       fixed width plus negative margins SHIFTS the box instead of widening it —
       the bar ended up further from the edge, not nearer. */
    width: auto;
    margin-inline: -1rem;
    /* More on the right than the left: the scrollbar lives there. */
    padding-inline: 1rem 1.5rem;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  /* Collapsed, the thumb is a second vertical line in a column 100px wide, next
     to the one that already separates the rail from the board. It still
     scrolls; it just stops drawing. */
  .shell :global(.shell-nav[data-layout='rail'] .projects) { scrollbar-width: none; }
  .shell :global(.shell-nav[data-layout='rail'] .projects::-webkit-scrollbar) { display: none; }

  /* `margin-top: auto` is what pins it in both layouts. */
  .shell :global(.shell-nav [data-part='footer']) {
    margin-top: auto;
    /* `align-items: start` on the column would leave the rule as wide as the
       word, and a divider that stops mid-column reads as a mistake. */
    align-self: stretch;
    width: 100%;
    padding-top: 0.4rem;
    border-top: 1px solid var(--line);
  }
  .col { display: flex; flex-direction: column; min-width: 0; min-height: 0; }

  /* A row. Every selector here is scoped to `[data-scope='navigation']` on
     purpose: the ⋮ is a `Menu.Trigger`, which carries `data-part='trigger'`
     too, so an unscoped rule painted the menu button with the row's highlight
     and gave it a block of its own. Same part name, different component. */
  .proj {
    display: flex;
    align-items: center;
    gap: 0.15rem;
    width: 100%;
  }
  /* Skeleton's sidebar trigger is `width: 100%`, which in a flex row means the
     WHOLE row: the name ran under the ⋮ and the highlight spilled past both.
     `flex: 1` gives it everything that is actually left over, which is what the
     row is for — the name is the content, the ⋮ is furniture. */
  /* Only in the sidebar layout: the rail's trigger is a square with its own
     width, and taking that away collapses it. */
  :global(.shell-nav[data-layout='sidebar']) .proj :global([data-scope='navigation'][data-part='trigger']) {
    flex: 1 1 auto;
    width: auto;
    min-width: 0;
  }
  /* The name is one line that ends in an ellipsis, in BOTH layouts. In the rail
     it was running out of the square and across the divider — a name too long
     for the column is a fact to state, not a layout to break. */
  .proj :global([data-part='trigger-text']) {
    display: block;
    max-width: 100%;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .proj.on :global([data-scope='navigation'][data-part='trigger']) {
    background: var(--hover);
    font-weight: 600;
  }
  .proj-more {
    flex: none;
    display: grid;
    place-content: center;
    width: 24px;
    height: 28px;
    border-radius: 7px;
    color: var(--faint);
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.12s ease;
  }
  /* Shown when the row is being used — hovered, focused, or the current one.
     Three permanent ⋮ down the column is three invitations to a menu nobody
     opens; the affordance belongs to the row you are actually on. */
  .proj:hover .proj-more,
  .proj:focus-within .proj-more,
  .proj.on .proj-more {
    opacity: 1;
  }
  .proj-more:hover {
    background: var(--hover);
    color: var(--text);
  }
  .pin {
    flex: none; display: grid; place-content: center;
    width: 22px; height: 22px; border-radius: 7px;
    background: var(--hover); color: var(--muted);
    font-size: 0.72rem; font-weight: 700;
  }
  .rename {
    width: 100%; padding: 0.35rem 0.5rem;
    border: 1px solid var(--accent, var(--line)); border-radius: 8px;
    background: var(--surface); color: var(--text); font-size: 0.88rem;
  }

  .pane { min-width: 0; flex: 1; overflow-y: auto; }
</style>
