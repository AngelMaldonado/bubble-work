<script lang="ts">
  // The whole interface, with invented data. Not a catalogue of controls —
  // that is /theme — but the product: the shell, the board, a bubble, a thread,
  // the wiki and the planner, reachable from each other the way they will be.
  //
  // It exists because a design system can be right control by control and still
  // add up to the wrong product. The only way to see that is to look at the
  // whole thing at once, before any of it is wired to real data.
  import { Navigation, Portal, Progress, Tooltip, Menu } from '@skeletonlabs/skeleton-svelte';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import MoreVerticalIcon from '@lucide/svelte/icons/more-vertical';
  import InboxIcon from '@lucide/svelte/icons/inbox';
  import Orb from './Orb.svelte';
  import BubbleDrawer from './BubbleDrawer.svelte';
  import ThreadView from './ThreadView.svelte';
  import SideTree from './SideTree.svelte';
  import Minimap, { type MapItem } from './Minimap.svelte';
  import BrandOrb from './BrandOrb.svelte';
  import { edgeFade } from '../lib/fade.svelte';
  import Prose from './Prose.svelte';
  import {
    bubbles, drawerThreads,
    threadDoc, threadHTML, wiki, wikiHTML,
    objectives, priorityMeaning, priorityMap, inbox, kanban, week, projects, me, online,
    type MockBubble,
  } from '../lib/mock';
  import { bandOrder, bandFace, bandName } from '../lib/bands';

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

  let railed = $state(false);
  let paneEl = $state<HTMLElement | null>(null);

  // You first. A presence row is read left to right, and the one avatar you
  // already know is yours is the one that should not have to be searched for.
  const here = $derived([
    ...online.filter((o) => o.name === me.name),
    ...online.filter((o) => o.name !== me.name),
  ]);

  // Both scrolling boxes fade at the edge that still has content behind it.
  const listFade = edgeFade();
  const paneFade = edgeFade();

  // One drawer, one thread page. A drawer per bubble would be nine drawers; it
  // is one panel that changes what it shows.
  // Two pieces of state, not one: the drawer owns whether it is open (it can
  // close itself), and this page owns WHICH bubble it is showing.
  let drawerOpen = $state(false);
  let open = $state<MockBubble | null>(null);
  let thread = $state(false);

  let picked = $state('docs/arquitectura/overview.md');

  // Wired for real, like the projects: an action you can only look at tells you
  // nothing about whether it is in the right place.
  let threads = $state([...drawerThreads]);
  let nextSeq = $state(Math.max(...drawerThreads.map((t) => t.seq)) + 1);

  // The mock's bubbles have names, not ids. One place to turn one into the other.
  const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-');
  const mapItems = $derived<MapItem[]>(
    bubbles.map((b) => ({ id: slug(b.name), name: b.name, life: b.life })),
  );

  // The projects. Creating, renaming and deleting are wired for real so the
  // interaction can be judged, not just the picture of it.
  let list = $state(projects.map((p) => ({ ...p })));
  let current = $state(projects[0].id);
  let editing = $state<string | null>(null);
  let draft = $state('');

  const currentName = $derived(list.find((p) => p.id === current)?.name ?? '');

  function startRename(id: string, name: string) {
    editing = id;
    draft = name;
  }
  function commitRename() {
    const p = list.find((x) => x.id === editing);
    // An empty name is not a rename, it is a deletion nobody asked for.
    if (p && draft.trim()) p.name = draft.trim();
    editing = null;
  }
  function remove(id: string) {
    list = list.filter((p) => p.id !== id);
    if (current === id) current = list[0]?.id ?? '';
  }
  function create() {
    const id = 'p' + Date.now();
    list = [...list, { id, name: 'Proyecto nuevo', bubbles: 0, cycle: 'sin ciclo' }];
    current = id;
    startRename(id, 'Proyecto nuevo');
  }

  const prioTone: Record<string, string> = {
    P1: 'text-[var(--color-error-500)]',
    P2: 'text-[var(--color-warning-500)]',
    P3: 'muted',
    P4: 'faint',
  };
</script>

<div class="mock">
  <!-- The projects live here. A workspace is the boundary for a body of work
       and the thing you switch between all day, so it gets the persistent
       column; the views get the two triggers at the bottom of it. -->
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
      {#if !railed}<Navigation.Label>Proyectos</Navigation.Label>{/if}

      <Navigation.Group class="projects" style={listFade.style} {@attach listFade.attach}>
        <Navigation.Menu>
          {#each list as p (p.id)}
            {#if editing === p.id && !railed}
              <!-- Renaming happens IN PLACE. A dialog for one field is a dialog
                   asking you to confirm you meant to type.
                   Focused explicitly, not with `autofocus`: the attribute only
                   acts on a page's first parse, so an input that appears later
                   never gets it — the typing went to the page instead, and the
                   space in the name re-activated the button that was still
                   focused, creating a second project. -->
              <input
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
                  onclick={() => (current = p.id)}
                  ondblclick={() => startRename(p.id, p.name)}
                  title="{p.name} · {p.bubbles} burbujas">
                  <span class="pin" aria-hidden="true">{p.name.slice(0, 1)}</span>
                  <Navigation.TriggerText>{p.name}</Navigation.TriggerText>
                </Navigation.Trigger>
                {#if !railed}
                  <Menu onSelect={(e: { value: string }) => {
                    if (e.value === 'rename') startRename(p.id, p.name);
                    if (e.value === 'delete') remove(p.id);
                  }}>
                    <Menu.Trigger>
                      <span class="proj-more" title="acciones"><MoreVerticalIcon class="size-4" /></span>
                    </Menu.Trigger>
                    <Portal>
                      <Menu.Positioner>
                        <Menu.Content>
                          <Menu.Item value="rename"><Menu.ItemText>Renombrar</Menu.ItemText></Menu.Item>
                          <Menu.Item value="delete"><Menu.ItemText>Eliminar</Menu.ItemText></Menu.Item>
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

      <!-- Pinned to the bottom in both layouts. Creating a project is not the
           last item of the list — it is the one action the column always
           offers, so it sits where the column ends rather than drifting down as
           projects are added. -->
      <Navigation.Footer class="new-foot">
        <Navigation.Menu>
          <Navigation.Trigger onclick={create} title="nuevo proyecto">
            <PlusIcon class={railed ? 'size-5' : 'size-4'} />
            <Navigation.TriggerText>Nuevo</Navigation.TriggerText>
          </Navigation.Trigger>
        </Navigation.Menu>
      </Navigation.Footer>

    </Navigation.Content>
  </Navigation>

  <div class="col">
    <main class="pane" bind:this={paneEl} style={paneFade.style} {@attach paneFade.attach}>
      {#if view === 'board'}
        <!-- ── the board ────────────────────────────────────────────────── -->
        <!-- v0's board, restyled. Bands stacked down the page, orbs CENTRED
             inside each one, a hairline between bands, and an empty band still
             drawn — a band with nothing in it is a fact about the work, and
             hiding it makes the board's shape change under you. Closed lives at
             the bottom: what is done is worth seeing and is never what you look
             at first. -->
        <div class="board">
          <header class="board-head">
            <h1 class="display text-2xl">{currentName}</h1>
          </header>

          {#each bandOrder as band (band)}
            {@const items = bubbles.filter((b) => b.life === band)}
            <section id="band-{band}" class="band band-{band}" class:empty={items.length === 0}>
              <header>
                <span class="ico">{bandFace(band)}</span>
                <span class="band-name">{bandName(band)}</span>
                <span class="count">{items.length}</span>
              </header>
              {#if items.length === 0}
                <p class="none">— nada aquí —</p>
              {:else}
                <div class="orbs">
                  {#each items as b, i (b.name)}
                    <!-- The anchor the minimap scrolls to. It wraps the orb
                         rather than sitting on it: `scrollIntoView` on a
                         floating element lands wherever the bob left it. -->
                    <div id="bw-{slug(b.name)}">
                    <Orb
                      name={b.name} lifecycle={b.life} burning={b.burning}
                      owner={b.owner} people={[...b.people]} index={i}
                      onclick={() => { open = b; drawerOpen = true; }} />
                    </div>
                  {/each}
                </div>
              {/if}
            </section>
          {/each}

        </div>

      {:else if view === 'planner'}
        <!-- ── the planner: the strategic layer ─────────────────────────── -->
        <div class="planner">
          <section class="col-span-full">
            <h2 class="display text-xl">Planeador</h2>
            <p class="faint text-sm">
              La capa que decide. El board dice qué está vivo; esto dice qué debería.
            </p>
          </section>

          <!-- Inbox: what arrived and has not been shaped yet. -->
          <section class="card glass p-4">
            <header class="mb-3 flex items-center gap-2">
              <InboxIcon class="size-4" />
              <h3 class="text-sm font-bold tracking-widest uppercase">Inbox</h3>
              <span class="faint rounded-full px-2 text-xs" style="background: var(--hover)">{inbox.length}</span>
            </header>
            <ul class="space-y-2">
              {#each inbox as it (it.id)}
                <li class="rounded-lg p-2 text-sm" style="background: var(--hover)">
                  <p>{it.text}</p>
                  <p class="faint mt-1 text-xs">{it.from} · {it.when}</p>
                </li>
              {/each}
            </ul>
            <p class="faint mt-3 text-xs">
              Entra vago y sale con objetivo, impacto y urgencia — de ahí sale la
              prioridad, nunca de teclearla.
            </p>
          </section>

          <!-- The week. Its whole job is to answer what lands before Friday. -->
          <section class="card glass p-4">
            <header class="mb-3 flex items-center gap-3">
              <h3 class="text-sm font-bold tracking-widest uppercase">Semana</h3>
              <div class="faint ml-auto flex gap-1 text-xs">
                <span class="rounded px-2 py-0.5" style="background: var(--hover)">semana</span>
                <span>timeline</span><span>día</span>
              </div>
            </header>
            <div class="grid grid-cols-7 gap-1">
              {#each week as d (d.date)}
                <div class="min-h-24 rounded-lg p-1.5" style="background: var(--hover)">
                  <div class="faint text-[0.65rem] uppercase">{d.day} {d.date}</div>
                  {#each d.items as it (it.t)}
                    <div class="mt-1 rounded p-1 text-[0.68rem] leading-tight"
                         style="background: var(--surface-solid)">
                      <span class={prioTone[it.p]}>{it.p}</span>
                      <div class="truncate">{it.t}</div>
                    </div>
                  {/each}
                </div>
              {/each}
            </div>
          </section>

          <!-- The high-level kanban: organisation by objective, not by person. -->
          <section class="card glass col-span-full p-4">
            <h3 class="mb-3 text-sm font-bold tracking-widest uppercase">Kanban de alto nivel</h3>
            <div class="grid gap-3" style="grid-template-columns: repeat({kanban.length}, minmax(0, 1fr))">
              {#each kanban as col (col.col)}
                <div>
                  <div class="faint mb-2 flex items-center gap-2 text-xs uppercase">
                    {col.col}
                    <span class="rounded-full px-1.5" style="background: var(--hover)">{col.cards.length}</span>
                  </div>
                  <div class="space-y-2">
                    {#each col.cards as c (c.title)}
                      <article class="rounded-lg border p-2 text-sm"
                               style="border-color: var(--line); background: var(--surface-solid)">
                        <div class="leading-snug">{c.title}</div>
                        <div class="mt-1.5 flex items-center gap-2 text-xs">
                          <span class={prioTone[c.prio]}>{c.prio}</span>
                          <Tooltip>
                            <Tooltip.Trigger>
                              <span class="faint">O{c.obj}</span>
                            </Tooltip.Trigger>
                            <Portal>
                              <Tooltip.Positioner>
                                <Tooltip.Content>{objectives[c.obj - 1].name}</Tooltip.Content>
                              </Tooltip.Positioner>
                            </Portal>
                          </Tooltip>
                          {#if c.due}<span class="faint ml-auto">{c.due}</span>{/if}
                        </div>
                      </article>
                    {/each}
                  </div>
                </div>
              {/each}
            </div>
          </section>

          <!-- The objectives, and how the work actually divides between them. -->
          <section class="card glass p-4">
            <h3 class="mb-3 text-sm font-bold tracking-widest uppercase">Objetivos</h3>
            <ul class="space-y-3">
              {#each objectives as o (o.n)}
                <li>
                  <div class="flex items-baseline gap-2 text-sm">
                    <span class="faint tabular-nums">{o.n}</span>
                    <span class="font-medium">{o.name}</span>
                    <span class="faint ml-auto text-xs tabular-nums">{o.share}%</span>
                  </div>
                  <p class="faint text-xs">{o.why}</p>
                  <Progress value={o.share} class="mt-1"><Progress.Track><Progress.Range /></Progress.Track></Progress>
                </li>
              {/each}
            </ul>
          </section>

          <!-- The map that decides the priority, and what each one means. -->
          <section class="card glass p-4">
            <h3 class="mb-3 text-sm font-bold tracking-widest uppercase">Prioridad</h3>
            <table class="w-full text-center text-sm">
              <thead class="faint text-xs">
                <tr>
                  <th></th>
                  {#each priorityMap.cols as c (c)}<th class="font-normal">{c}</th>{/each}
                </tr>
              </thead>
              <tbody>
                {#each priorityMap.rows as row (row[0])}
                  <tr>
                    <th class="faint py-1 pr-2 text-right text-xs font-normal">{row[0]}</th>
                    {#each row.slice(1) as cell, i (i)}
                      <td class="py-1">
                        <span class="inline-block rounded px-2 py-0.5 {prioTone[cell]}"
                              style="background: var(--hover)">{cell}</span>
                      </td>
                    {/each}
                  </tr>
                {/each}
              </tbody>
            </table>
            <dl class="mt-4 space-y-1.5 text-xs">
              {#each priorityMeaning as [p, name, means, what] (p)}
                <div class="flex gap-2">
                  <dt class="w-16 shrink-0 {prioTone[p]}">{p} {name}</dt>
                  <dd class="faint"><b class="muted">{means}.</b> {what}.</dd>
                </div>
              {/each}
            </dl>
            <p class="faint mt-3 text-xs">
              «Lo necesito urgente» se responde con «¿pasa algo si no se hace hoy?».
              La prioridad se deriva del mapa, como el heat se deriva de la evidencia.
            </p>
          </section>
        </div>

      {:else}
        <!-- ── the wiki ─────────────────────────────────────────────────── -->
        <div class="wiki">
          <aside class="wiki-tree">
            <div class="faint mb-2 px-1 text-xs font-bold tracking-widest uppercase">
              Workspace
            </div>
            <SideTree nodes={wiki} onselect={(n) => (picked = n.id)} />
          </aside>
          <article class="wiki-doc">
            <div class="faint mb-3 font-mono text-xs">{picked}</div>
            <Prose html={wikiHTML} />
          </article>
        </div>
      {/if}
    </main>
  </div>
</div>

{#if view === 'board'}
  <!-- The board's table of contents. Only the board has bands to map. -->
  <Minimap items={mapItems} scroller={paneEl} />
{/if}

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
  <button
    class="float-btn planner-btn"
    title={view === 'planner' ? 'volver al board' : 'Planeador'}
    onclick={() => toggle('planner')}>
    <span aria-hidden="true">{view === 'planner' ? '←' : '🗓'}</span>
  </button>
{/if}

<button
  class="float-btn wiki-btn"
  title={view === 'wiki' ? 'volver al board' : `Wiki de ${currentName}`}
  onclick={() => toggle('wiki')}>
  <span aria-hidden="true">{view === 'wiki' ? '←' : '📖'}</span>
</button>

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
  onnewthread={() => (threads = [{ seq: nextSeq++, title: 'Thread nuevo', lifecycle: 'hot', state: 'Backlog', age: 'ahora' }, ...threads])}
  ondeletethread={(seq) => (threads = threads.filter((t) => t.seq !== seq))} />

{#if thread}
  <div class="fixed inset-0 z-[70] overflow-y-auto" style="background: var(--bg)">
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
      onback={() => (thread = false)} />
  </div>
{/if}

<style>
  /* Nav column, then everything else. `items-stretch` is what lets
     Navigation's own `height: 100%` mean something. */
  /* The shell does NOT scroll — the board does. The sidebar and the HUD stay
     put because they are not in the scrolling box, rather than because they are
     pinned on top of one. */
  .mock {
    height: 100vh;
    overflow: hidden;
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: stretch;
  }
  .mock :global(.shell-nav) {
    border-right: 1px solid var(--line);
    background: transparent;
  }
  /* Header, projects and footer hang off `content`, not off the root, so the
     root's rows never reached them — the button pinned in the rail (a flex
     column) and floated mid-way in the sidebar. Make `content` the full-height
     column instead. Only in the sidebar layout: the rail's content is
     `display: contents`, and giving that a height would undo the rail. */
  .mock :global(.shell-nav [data-part='content'][data-layout='sidebar']) {
    /* `height`, not `min-height`: a minimum still lets the column grow past it,
       and a grown column pushed the footer under the bottom edge — which is the
       bug pinning it was supposed to fix. A definite height is what makes
       `flex: 1` on the list mean "whatever is left". */
    height: 100%;
    display: flex;
    flex-direction: column;
    /* Skeleton aligns this column to `start`, which makes every child as wide as
       its text. The rows need the column. */
    align-items: stretch;
    gap: 0.75rem;
  }
  /* The rail's group is `display: contents`, which has no box and therefore
     cannot scroll. It needs to be one. */
  .mock :global(.shell-nav[data-layout='rail'] .projects) {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  /* The LIST scrolls, not the column. With the overflow on the root, a long
     list pushed the footer past the bottom edge and the button went with it —
     the point of pinning it was that it never moves. */
  .mock :global(.shell-nav) { overflow: hidden; }
  .mock :global(.shell-nav .projects) {
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
    padding-inline: 1rem;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  /* Collapsed, the thumb is a second vertical line in a column 100px wide, next
     to the one that already separates the rail from the board. It still
     scrolls; it just stops drawing. */
  .mock :global(.shell-nav[data-layout='rail'] .projects) { scrollbar-width: none; }
  .mock :global(.shell-nav[data-layout='rail'] .projects::-webkit-scrollbar) { display: none; }

  /* `margin-top: auto` is what pins it in both layouts. */
  .mock :global(.shell-nav [data-part='footer']) {
    margin-top: auto;
    /* `align-items: start` on the column would leave the rule as wide as the
       word, and a divider that stops mid-column reads as a mistake. */
    align-self: stretch;
    width: 100%;
    padding-top: 0.4rem;
    border-top: 1px solid var(--line);
  }
  .col { display: flex; flex-direction: column; min-width: 0; min-height: 0; }

  /* A project row. Every selector here is scoped to `[data-scope='navigation']`
     on purpose: the ⋮ is a `Menu.Trigger`, which carries `data-part='trigger'`
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

  /* The board, from v0: one column, centred, read down. */
  .board { max-width: 1100px; margin: 0 auto; padding: 2rem 1.5rem 6rem; }
  .board-head { text-align: center; margin-bottom: 2rem; }

  /* Room around the band. The hairline is a divider between groups of work,
     and with the orbs close to it the page read as one list with lines drawn
     through it rather than as bands. */
  .band { padding: 2rem 0 2.4rem; border-bottom: 1px solid var(--line); }
  .band:last-of-type { border-bottom: none; }
  /* An empty band is still drawn, just quieter. */
  .band.empty { opacity: 0.72; }
  .band > header {
    display: flex; align-items: center; justify-content: center;
    gap: 0.55rem; margin-bottom: 1.5rem;
  }
  .band .ico { font-size: 1.05rem; }
  /* `band-name`, not `label`: Skeleton owns `.label` (`width: 100%; display:
     block`), so a span of ours by that name stretched to the full row and threw
     the count against the right edge. Same collision as `.card`, which is why
     ours is `.glass`. */
  .band .band-name {
    font-weight: 750; letter-spacing: 0.04em; text-transform: uppercase;
    font-size: 0.82rem; color: var(--muted);
  }
  .band .count {
    font-size: 0.72rem; color: var(--muted);
    background: var(--hover); padding: 0.05rem 0.45rem; border-radius: 999px;
  }
  .orbs { display: flex; flex-wrap: wrap; justify-content: center; gap: 0.9rem; }
  .none { margin: 0; text-align: center; color: var(--faint); font-size: 0.8rem; font-style: italic; }

  /* The floating buttons, stacked above the theme one on the right. The
     numbers are one row (2.6rem) plus a 0.6rem gap, so adding a fourth is
     another 3.2rem and nothing else. */
  .float-btn {
    position: fixed;
    z-index: var(--z-chrome);
    width: 42px;
    height: 42px;
    display: grid;
    place-content: center;
    font-size: 1.15rem;
    line-height: 1;
    cursor: pointer;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    transition: transform 0.15s ease, background 0.15s ease;
  }
  .float-btn:hover { background: var(--hover); transform: translateY(-1px); }
  .float-btn:active { transform: translateY(0); }
  /* A ROW along the bottom edge, not a column: three buttons read as a group of
     three, and a stack reads as a menu you have to climb. The theme button
     holds the corner, and these sit to its left. */
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

  .float-btn { bottom: 1rem; }
  .wiki-btn { right: 4.2rem; }
  .planner-btn { right: 7.4rem; }


  .planner {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
    padding: 1.25rem;
  }
  @media (max-width: 900px) {
    .planner { grid-template-columns: minmax(0, 1fr); }
  }

  .wiki { display: grid; grid-template-columns: 260px minmax(0, 1fr); align-items: start; }
  .wiki-tree { padding: 1rem 0.75rem; border-right: 1px solid var(--line); }
  .wiki-doc { padding: 1.25rem 1.5rem; min-width: 0; }
</style>
