<script lang="ts">
  // The whole interface, with invented data. Not a catalogue of controls —
  // that is /theme — but the product: the shell, the board, a bubble, a thread,
  // the wiki and the planner, reachable from each other the way they will be.
  //
  // It exists because a design system can be right control by control and still
  // add up to the wrong product. The only way to see that is to look at the
  // whole thing at once, before any of it is wired to real data.
  import { Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import Shell from './Shell.svelte';
  import BubbleBoard from './BubbleBoard.svelte';
  import BubbleDrawer from './BubbleDrawer.svelte';
  import ThreadView from './ThreadView.svelte';
  import Minimap, { type MapItem } from './Minimap.svelte';
  import Omnibar, { type Hit } from './Omnibar.svelte';
  import WikiView from './WikiView.svelte';
  import PlannerView from './PlannerView.svelte';
  import { renderMock } from '../lib/mockmd';
  import {
    bubbles, drawerThreads,
    threadDoc, threadHTML, wiki, wikiHTML,
    objectives, priorityMeaning, priorityMap, inbox, kanban, calEvents, projects, me, online,
    type MockBubble,
  } from '../lib/mock';
  import { bandFace } from '../lib/bands';

  type View = 'board' | 'planner' | 'wiki';
  let view = $state<View>('board');

  // No views in the sidebar. The sidebar is for PROJECTS — the thing you switch
  // between all day. The board is where you already are, and the other two
  // belong to the project you are looking at rather than to a list of places to
  // go, so they are floating buttons: the wiki for everyone, the planner only
  // for a lead, because planning is the strategic layer's job.
  function toggle(v: View) {
    view = view === v ? 'board' : v;
  }


  let paneEl = $state<HTMLElement | null>(null);

  // You first. A presence row is read left to right, and the one avatar you
  // already know is yours is the one that should not have to be searched for.
  // ⌘K, because the omnibar is the one control worth a shortcut.
  function hotkey(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      search();
    }
  }

  const here = $derived([
    ...online.filter((o) => o.name === me.name),
    ...online.filter((o) => o.name !== me.name),
  ]);

  // One drawer, one thread page. A drawer per bubble would be nine drawers; it
  // is one panel that changes what it shows.
  // Two pieces of state, not one: the drawer owns whether it is open (it can
  // close itself), and this page owns WHICH bubble it is showing.
  let drawerOpen = $state(false);
  let open = $state<MockBubble | null>(null);
  let thread = $state(false);
  // The board and its chrome are one screen, and a view REPLACES it rather than
  // covering it. Painting a view over a live board left the orbs and a second
  // HUD visible underneath — visible and unclickable, which is the worst of
  // both. The page's gradient stays because it belongs to the body, not here.
  const onBoard = $derived(view === 'board' && !thread);

  let picked = $state('docs/arquitectura/overview.md');

  // The planner's state, wired for real: dragging a card, capturing a note and
  // editing an objective all have to survive being looked at.
  let board = $state(
    kanban.map((c, i) => ({
      id: 'c' + i,
      name: c.col,
      // Spread, not a field list: picking fields by hand is how `notes` and the
      // two axes were silently dropped on the way to the board.
      cards: c.cards.map((x, j) => ({ ...x, id: `c${i}k${j}` })),
    })),
  );
  let notes = $state([...inbox]);
  let goals = $state(objectives.map((o) => ({ ...o })));
  let prios = $state(priorityMeaning.map((p) => [...p] as [string, string, string, string]));

  // Wired for real, like the projects: an action you can only look at tells you
  // nothing about whether it is in the right place.
  let threads = $state([...drawerThreads]);
  let nextSeq = $state(Math.max(...drawerThreads.map((t) => t.seq)) + 1);

  // The omnibar searches everything the workspace holds, which is why it is one
  // field and not three.
  let omni = $state(false);
  let omniPrompt = $state('');
  const omniItems = $derived<Hit[]>([
    ...threads.map((t) => ({ id: `t${t.seq}`, kind: 'thread', icon: '🧵', title: t.title, hint: `#${t.seq}` })),
    ...bubbles.map((b) => ({ id: `b:${b.name}`, kind: 'burbuja', icon: bandFace(b.life), title: b.name, hint: b.outcome })),
    ...wiki.flatMap(function walk(n): Hit[] {
      return n.children?.length
        ? n.children.flatMap(walk)
        : [{ id: `d:${n.id}`, kind: 'página', icon: '📄', title: n.name, hint: n.id }];
    }),
  ]);
  function search(prompt = '') {
    omniPrompt = prompt;
    omni = true;
  }

  // The mock's bubbles have names, not ids. One place to turn one into the other.
  const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-');
  const mapItems = $derived<MapItem[]>(
    bubbles.map((b) => ({ id: slug(b.name), name: b.name, life: b.life })),
  );
  // The board takes ids; the mock's fixtures have names. One place to bridge it.
  const orbs = $derived(
    bubbles.map((b) => ({
      id: slug(b.name), name: b.name, life: b.life,
      burning: b.burning, owner: b.owner, people: [...b.people],
    })),
  );

  // The projects. Creating, renaming and deleting are wired for real so the
  // interaction can be judged, not just the picture of it.
  let list = $state(projects.map((p) => ({ ...p })));
  let current = $state(projects[0].id);

  const currentName = $derived(list.find((p) => p.id === current)?.name ?? '');

  function rename(id: string, name: string) {
    const p = list.find((x) => x.id === id);
    if (p) p.name = name;
  }
  function remove(id: string) {
    list = list.filter((p) => p.id !== id);
    if (current === id) current = list[0]?.id ?? '';
  }
  function create() {
    const id = 'p' + Date.now();
    list = [...list, { id, name: 'Proyecto nuevo', bubbles: 0, cycle: 'sin ciclo' }];
    current = id;
    return id;
  }

  const prioTone: Record<string, string> = {
    P1: 'text-[var(--color-error-500)]',
    P2: 'text-[var(--color-warning-500)]',
    P3: 'muted',
    P4: 'faint',
  };
</script>

<svelte:window onkeydown={hotkey} />

{#if onBoard}
<Shell
  items={list.map((p) => ({ id: p.id, name: p.name, hint: `${p.bubbles} burbujas` }))}
  {current}
  bind:pane={paneEl}
  onselect={(id) => (current = id)}
  onrename={rename}
  ondelete={remove}
  oncreate={create}>
  <BubbleBoard
    title={currentName}
    bubbles={orbs}
    onopen={(b) => {
      open = bubbles.find((x) => x.name === b.name) ?? null;
      drawerOpen = true;
    }} />
</Shell>

<!-- The board's table of contents. Only the board has bands to map. -->
<Minimap items={mapItems} scroller={paneEl} />

<!-- Who is here, top right. Fixed rather than in the pane: presence is about
     now, and it should not scroll away with the board. -->
<div class="presence" aria-label="en línea">
  {#each here as who, i (who.name)}
    <!-- A real tooltip rather than `title`: the browser's takes a second to
         appear, is drawn by the OS where no stylesheet reaches it, and cannot
         be themed. The trigger is passed as the `element` snippet so it stays
         OUR span — Skeleton's own trigger is a <button>, and wrapping the
         avatar in one would put a box around it and break the overlap. -->
    <Tooltip openDelay={120} closeDelay={60}>
      <Tooltip.Trigger>
        {#snippet element(attributes: Record<string, unknown>)}
          <span
            class="who"
            class:me={who.name === me.name}
            style="--hue: {who.hue}; --i: {i}"
            {...attributes}>
            {who.name.slice(0, 1)}
          </span>
        {/snippet}
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content>
            {who.name}{who.name === me.name ? ' (tú)' : ''}
          </Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip>
  {/each}
</div>

<!-- The floating stack, above the theme button. Emoji rather than lucide: these
     three are the only controls that live over the page instead of in it, and
     an emoji reads as a thing you press rather than as chrome. -->
{#if me.isLead}
  <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
    <Tooltip.Trigger>
      {#snippet element(attributes: Record<string, unknown>)}
        <button class="float-btn planner-btn" {...attributes} onclick={() => toggle('planner')}>
          <span aria-hidden="true">{view === 'planner' ? '←' : '🗓'}</span>
        </button>
      {/snippet}
    </Tooltip.Trigger>
    <Portal>
      <Tooltip.Positioner>
        <Tooltip.Content>{view === 'planner' ? 'Volver al board' : 'Planeador'}</Tooltip.Content>
      </Tooltip.Positioner>
    </Portal>
  </Tooltip>
{/if}

<Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
  <Tooltip.Trigger>
    {#snippet element(attributes: Record<string, unknown>)}
      <button class="float-btn search-btn" {...attributes} onclick={() => search()}>
        <span aria-hidden="true">🔍</span>
      </button>
    {/snippet}
  </Tooltip.Trigger>
  <Portal>
    <Tooltip.Positioner>
      <Tooltip.Content>Buscar · ⌘K</Tooltip.Content>
    </Tooltip.Positioner>
  </Portal>
</Tooltip>

{/if}

<Omnibar bind:open={omni} items={omniItems} prompt={omniPrompt} />

<Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
  <Tooltip.Trigger>
    {#snippet element(attributes: Record<string, unknown>)}
      <button class="float-btn wiki-btn" {...attributes} onclick={() => toggle('wiki')}>
        <span aria-hidden="true">{view === 'wiki' ? '←' : '📖'}</span>
      </button>
    {/snippet}
  </Tooltip.Trigger>
  <Portal>
    <Tooltip.Positioner>
      <Tooltip.Content>
        {view === 'wiki' ? 'Volver al board' : `Wiki de ${currentName}`}
      </Tooltip.Content>
    </Tooltip.Positioner>
  </Portal>
</Tooltip>

<!-- One panel, driven by whichever orb was clicked. -->
<BubbleDrawer
  bind:open={drawerOpen}
  name={open?.name ?? ''}
  outcome={open?.outcome ?? ''}
  lifecycle={open?.life ?? 'hot'}
  reason={open?.why ?? ''}
  owner={open?.owner ?? ''}
  cycleLeft="quedan 6 d"
  cyclePct={57}
  threads={threads}
  onopenthread={() => { drawerOpen = false; thread = true; }}
  onnewthread={(name) => (threads = [{ seq: nextSeq++, title: name, lifecycle: 'hot', state: 'Backlog', age: 'ahora' }, ...threads])}
  ondeletethread={(seq) => (threads = threads.filter((t) => t.seq !== seq))} />

{#if view === 'planner'}
  <!-- The planner is a view too — the strategic layer gets a screen, not a tab
       inside the operational one. -->
  <!-- No background of its own: the page's gradient and its grain are the app's
       ground, and a view that paints over them is a screen that arrived from
       somewhere else. -->
  <div class="fixed inset-0" style="z-index: var(--z-view)">
    <PlannerView
      workspace={currentName}
      bind:columns={board}
      bind:inbox={notes}
      events={calEvents}
      bind:objectives={goals}
      bind:priorities={prios}
      {priorityMap}
      render={renderMock}
      onback={() => (view = 'board')}
      onsearch={() => search()} />
  </div>
{/if}

{#if view === 'wiki'}
  <!-- The wiki is a VIEW, like a thread: same shell, its own way back. It used
       to be a column inside the board's pane, which made a page feel like a
       panel on the board rather than a place. -->
  <div class="fixed inset-0" style="z-index: var(--z-view)">
    <WikiView
      workspace={currentName}
      path={picked}
      html={wikiHTML}
      tree={wiki}
      onback={() => (view = 'board')}
      onopen={(id) => (picked = id)}
      onsearch={() => search()}
      onnew={() => search('¿Dónde va la página nueva?')} />
  </div>
{/if}

{#if thread}
  <!-- A view, not a modal: it covers the board and brings its own chrome. -->
  <div class="fixed inset-0" style="z-index: var(--z-view)">
    <ThreadView
      seq={14}
      title="Evidencia observada, no inferida"
      lifecycle="hot"
      reason="produjo algo en el ciclo actual"
      priority="P1"
      threadState="En curso"
      assignees={['Angel', 'Bea']}
      labels={['backend', 'migración']}
      markdown={threadDoc}
      html={threadHTML}
      links={[
        { id: '1', url: 'https://github.com/x/y/pull/412', title: 'PR #412' },
        { id: '2', url: 'https://example.com/adr-7', title: 'ADR 0007' },
      ]}
      related={[
        { id: 'a', title: 'El árbol de markdown y sus commits', type: 'blocked_by' },
        { id: 'b', title: 'Portar el motor de markdown', type: 'relates_to' },
      ]}
      revisions={[{ id: 'r1', title: 'Revisión de Bea · 2 sep' }]}
      onback={() => (thread = false)}
      onsearch={() => search('Buscar el thread a relacionar…')}
      onfinish={() => (thread = false)}
      onmove={() => search('¿A qué burbuja se mueve?')}
      ondelete={() => { threads = threads.filter((t) => t.seq !== 14); thread = false; }} />
  </div>
{/if}

<style>
  /* What is left here is the mock's own: presence, which has no server behind
     it yet, and the planner's two-column placeholder. The shell, the board and
     the floating buttons moved to `Shell.svelte`, `BubbleBoard.svelte` and
     `app.css` when the real screen started drawing them too — a second copy of
     that CSS is how the drawing and the thing drift apart. */

  /* Who is here. The only thing on this page with no server behind it: there is
     no presence yet, and inventing one in the real app would be a lie about
     who is looking. */
  .presence {
    position: fixed;
    top: 1rem;
    right: 1rem;
    z-index: var(--z-chrome);
    display: flex;
  }
  .who {
    /* Overlapped, and stacked so the leftmost — you — sits on top. */
    margin-left: -8px;
    z-index: calc(20 - var(--i));
    width: 30px;
    height: 30px;
    display: grid;
    place-content: center;
    border-radius: 999px;
    border: 2px solid var(--bg);
    background: oklch(0.72 0.13 var(--hue));
    color: oklch(0.22 0.05 var(--hue));
    font-size: 0.78rem;
    font-weight: 700;
    line-height: 1;
    transition: transform 0.14s ease;
  }
  .who:first-child { margin-left: 0; }
  /* Hover lifts one out of the stack: it grows and comes to the front, which is
     how you read a name in a row of initials without a tooltip getting there
     first. */
  .who:hover {
    transform: scale(1.25);
    z-index: 30;
  }

  @media (prefers-reduced-motion: reduce) {
    .who { transition: none; }
  }
  /* You get the ring rather than a label: it is the only one you never need
     named. */
  .me { box-shadow: 0 0 0 2px var(--accent, oklch(0.68 0.2 40)); }

  .planner {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
    padding: 1.25rem;
  }
  @media (max-width: 900px) {
    .planner { grid-template-columns: minmax(0, 1fr); }
  }

</style>
