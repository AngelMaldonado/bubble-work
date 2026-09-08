<script lang="ts" module>
  export type Card = {
    id: string;
    title: string;
    obj?: number;
    /** DERIVED from impact × urgency — see CardSheet. Stored so the board can
     *  show it without recomputing, never typed in by hand. */
    prio?: string;
    due?: string;
    impact?: string;
    urgency?: string;
    notes?: string;
  };
  export type Column = { id: string; name: string; cards: Card[] };
</script>

<script lang="ts">
  // The orchestration board: high-level cards, one column per stage.
  //
  // Drag and drop is Atlassian's pragmatic-drag-and-drop, which is what Trello
  // itself runs on: 7 kB, no peer dependencies, and it owns the parts that are
  // genuinely hard — pointer capture, touch, autoscroll, and telling you WHICH
  // edge of a card you are nearest. What it deliberately does not own is the
  // state: moving a card is our reducer, not its.
  //
  // It loads on demand. A planner that is only being read should not download a
  // drag engine.
  import PlusIcon from '@lucide/svelte/icons/plus';
  import MoreIcon from '@lucide/svelte/icons/more-horizontal';
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';

  let {
    columns = $bindable([]),
    onopen,
    onadd,
  }: {
    columns?: Column[];
    onopen?: (card: Card) => void;
    onadd?: (columnId: string) => void;
  } = $props();

  // Columns are the board's own shape, and it belongs to whoever runs the
  // board. "Por decidir / Planeado / En curso" is a good default and a terrible
  // law: every team's stages are the argument they had about how work moves.
  let renaming = $state<string | null>(null);
  let draft = $state('');

  function startRename(c: Column) {
    renaming = c.id;
    draft = c.name;
  }
  function commitRename() {
    const col = columns.find((c) => c.id === renaming);
    // An empty name is not a rename; it is a deletion nobody asked for.
    if (col && draft.trim()) col.name = draft.trim();
    renaming = null;
  }
  function addColumn() {
    const id = 'col' + Date.now();
    columns = [...columns, { id, name: 'Columna nueva', cards: [] }];
    renaming = id;
    draft = 'Columna nueva';
  }
  function dropColumn(id: string) {
    columns = columns.filter((c) => c.id !== id);
  }
  function moveColumn(id: string, before: string | null) {
    const col = columns.find((c) => c.id === id);
    if (!col) return;
    const rest = columns.filter((c) => c.id !== id);
    const at = before ? rest.findIndex((c) => c.id === before) : -1;
    if (at < 0) rest.push(col);
    else rest.splice(at, 0, col);
    columns = rest;
  }

  /** Where a dragged COLUMN would land. Separate from the card one because a
   *  column and a card are different things being moved. */
  let overCol = $state<string | null>(null);
  let liftingCol = $state<string | null>(null);

  /** The card shows a date the way a person says it; the value stays ISO. */
  const fmt = new Intl.DateTimeFormat('es-MX', { day: 'numeric', month: 'short' });
  function shortDate(iso: string): string {
    const d = new Date(iso + 'T00:00:00');
    return Number.isNaN(d.getTime()) ? iso : fmt.format(d);
  }

  /** Where a dragged card would land right now: column id, and the card it goes
   *  before (null = at the end). Drawn as a gap, so the board shows the result
   *  rather than asking you to imagine it. */
  let over = $state<{ col: string; before: string | null } | null>(null);
  let dragging = $state<string | null>(null);

  function move(cardId: string, toCol: string, before: string | null) {
    let card: Card | undefined;
    const next = columns.map((c) => {
      const keep = c.cards.filter((x) => {
        if (x.id !== cardId) return true;
        card = x;
        return false;
      });
      return { ...c, cards: keep };
    });
    if (!card) return;
    const target = next.find((c) => c.id === toCol);
    if (!target) return;
    const at = before ? target.cards.findIndex((x) => x.id === before) : -1;
    if (at < 0) target.cards.push(card);
    else target.cards.splice(at, 0, card);
    columns = next;
  }

  /** Wires one card as both a drag source and a drop target. */
  function card(el: HTMLElement, c: Card, colId: string) {
    let stop: (() => void) | undefined;
    let live = true;
    Promise.all([
      import('@atlaskit/pragmatic-drag-and-drop/element/adapter'),
      import('@atlaskit/pragmatic-drag-and-drop/combine'),
      import('@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge'),
    ]).then(([{ draggable, dropTargetForElements }, { combine }, hitbox]) => {
      if (!live) return;
      stop = combine(
        draggable({
          element: el,
          getInitialData: () => ({ cardId: c.id, from: colId }),
          onDragStart: () => (dragging = c.id),
          onDrop: () => {
            dragging = null;
            over = null;
          },
        }),
        dropTargetForElements({
          element: el,
          canDrop: ({ source }) => source.data.cardId !== c.id,
          getData: ({ input, element }) =>
            // The hitbox says which edge the pointer is nearest, which is what
            // turns "over this card" into "before it" or "after it".
            hitbox.attachClosestEdge({ cardId: c.id, col: colId }, {
              input,
              element,
              allowedEdges: ['top', 'bottom'],
            }),
          onDrag: ({ self }) => {
            const edge = hitbox.extractClosestEdge(self.data);
            over = { col: colId, before: edge === 'bottom' ? nextOf(colId, c.id) : c.id };
          },
          onDragLeave: () => (over = null),
        }),
      );
    });
    return () => {
      live = false;
      stop?.();
    };
  }

  function nextOf(colId: string, cardId: string): string | null {
    const col = columns.find((x) => x.id === colId);
    const i = col?.cards.findIndex((x) => x.id === cardId) ?? -1;
    return col && i >= 0 ? (col.cards[i + 1]?.id ?? null) : null;
  }

  /** The header is the column's handle: dragging a column by its whole body
   *  would fight the cards inside it for the pointer. */
  function head(el: HTMLElement, col: Column) {
    let stop: (() => void) | undefined;
    let live = true;
    Promise.all([
      import('@atlaskit/pragmatic-drag-and-drop/element/adapter'),
      import('@atlaskit/pragmatic-drag-and-drop/combine'),
      import('@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge'),
    ]).then(([{ draggable, dropTargetForElements, monitorForElements }, { combine }, hitbox]) => {
      if (!live) return;
      stop = combine(
        draggable({
          element: el,
          getInitialData: () => ({ colId: col.id }),
          onDragStart: () => (liftingCol = col.id),
          onDrop: () => {
            liftingCol = null;
            overCol = null;
          },
        }),
        dropTargetForElements({
          element: el,
          canDrop: ({ source }) => !!source.data.colId && source.data.colId !== col.id,
          getData: ({ input, element }) =>
            hitbox.attachClosestEdge({ colTarget: col.id }, {
              input,
              element,
              allowedEdges: ['left', 'right'],
            }),
          onDrag: ({ self }) => {
            const edge = hitbox.extractClosestEdge(self.data);
            const i = columns.findIndex((c) => c.id === col.id);
            overCol = edge === 'right' ? (columns[i + 1]?.id ?? null) : col.id;
          },
        }),
        monitorForElements({
          canMonitor: ({ source }) => source.data.colId === col.id,
          onDrop: ({ location }) => {
            // Read the edge from the target being dropped ON, not from the last
            // `onDrag` we happened to see: the pointer can leave a header on its
            // way to mouse-up, and then the column lands wherever the stale
            // value pointed — which was always the end.
            const target = location.current.dropTargets.find((t) => t.data.colTarget);
            if (target) {
              const edge = hitbox.extractClosestEdge(target.data);
              const i = columns.findIndex((c) => c.id === target.data.colTarget);
              moveColumn(col.id, edge === 'right' ? (columns[i + 1]?.id ?? null) : columns[i].id);
            }
            overCol = null;
            liftingCol = null;
          },
        }),
      );
    });
    return () => {
      live = false;
      stop?.();
    };
  }

  /** The column itself takes a drop, which is what makes an EMPTY column
   *  reachable and what catches the space below the last card. */
  function column(el: HTMLElement, colId: string) {
    let stop: (() => void) | undefined;
    let live = true;
    Promise.all([
      import('@atlaskit/pragmatic-drag-and-drop/element/adapter'),
      import('@atlaskit/pragmatic-drag-and-drop/combine'),
    ]).then(([{ dropTargetForElements, monitorForElements }, { combine }]) => {
      if (!live) return;
      stop = combine(
        dropTargetForElements({
          element: el,
          getData: () => ({ col: colId }),
          // A card sitting over a CARD has already said where it goes; the
          // column only answers for the empty space around them.
          onDragEnter: ({ location }) => {
            if (location.current.dropTargets.length === 1) over = { col: colId, before: null };
          },
          onDragLeave: () => (over = null),
        }),
        monitorForElements({
          // A column drag passes through here too; only a card is ours.
          canMonitor: ({ source }) => !!source.data.cardId,
          onDrop: ({ source, location }) => {
            const target = location.current.dropTargets[0];
            if (!target) return;
            const to = (target.data.col as string) ?? colId;
            if (to !== colId) return; // one monitor acts, not five
            move(source.data.cardId as string, to, over?.col === to ? over.before : null);
            over = null;
            dragging = null;
          },
        }),
      );
    });
    return () => {
      live = false;
      stop?.();
    };
  }
</script>

<div class="board">
  {#each columns as col (col.id)}
    <section class="col" class:over={over?.col === col.id} {@attach (el) => column(el, col.id)}>
      {#if overCol === col.id}<div class="col-gap" aria-hidden="true"></div>{/if}
      <header {@attach (el) => head(el, col)} class:lifting={liftingCol === col.id}>
        {#if renaming === col.id}
          <input
            class="rename"
            bind:value={draft}
            onblur={commitRename}
            onkeydown={(e) => {
              if (e.key === 'Enter') commitRename();
              if (e.key === 'Escape') renaming = null;
            }}
            {@attach (el: HTMLInputElement) => { el.focus(); el.select(); }} />
        {:else}
          <button class="name" ondblclick={() => startRename(col)} title="doble clic para renombrar">
            {col.name}
          </button>
          <span class="count">{col.cards.length}</span>
          <Menu
            onSelect={(e: { value: string }) => {
              if (e.value === 'rename') startRename(col);
              if (e.value === 'delete') dropColumn(col.id);
            }}>
            <Menu.Trigger>
              <span class="more" title="acciones de la columna"><MoreIcon class="size-4" /></span>
            </Menu.Trigger>
            <Portal>
              <Menu.Positioner>
                <Menu.Content>
                  <Menu.Item value="rename"><Menu.ItemText>Renombrar</Menu.ItemText></Menu.Item>
                  <Menu.Item value="delete"><Menu.ItemText>Eliminar la columna</Menu.ItemText></Menu.Item>
                </Menu.Content>
              </Menu.Positioner>
            </Portal>
          </Menu>
        {/if}
      </header>

      <div class="cards">
        {#each col.cards as c (c.id)}
          {#if over?.col === col.id && over.before === c.id}
            <div class="gap" aria-hidden="true"></div>
          {/if}
          <article
            class="card"
            class:lifting={dragging === c.id}
            {@attach (el) => card(el, c, col.id)}>
            <button class="open" onclick={() => onopen?.(c)}>
              <span class="ttl">{c.title}</span>
              <span class="meta">
                {#if c.prio}<span class="prio-chip prio-{c.prio}">{c.prio}</span>{/if}
                {#if c.obj}<span class="obj">O{c.obj}</span>{/if}
                {#if c.due}<span class="due">{shortDate(c.due)}</span>{/if}
              </span>
            </button>
          </article>
        {/each}
        {#if over?.col === col.id && over.before === null}
          <div class="gap" aria-hidden="true"></div>
        {/if}
      </div>

      <button class="add" onclick={() => onadd?.(col.id)}>
        <PlusIcon class="size-4" /> tarjeta
      </button>
    </section>
  {/each}

  {#if overCol === null && liftingCol}<div class="col-gap" aria-hidden="true"></div>{/if}

  <!-- Adding a column is the one action the board always offers, so it sits at
       the end of the columns rather than hiding in a menu. -->
  <button class="add-col" onclick={addColumn}>
    <PlusIcon class="size-4" /> columna
  </button>
</div>

<style>
  .board {
    display: flex;
    gap: 0.85rem;
    height: 100%;
    padding: 0.9rem;
    overflow-x: auto;
    align-items: flex-start;
  }
  .col {
    flex: 0 0 272px;
    display: flex;
    flex-direction: column;
    max-height: 100%;
    padding: 0.6rem;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--surface);
  }
  /* The column you are over lights up as a whole, so the target is legible even
     when the gap is off screen. */
  .col.over { border-color: color-mix(in oklab, var(--accent) 55%, transparent); }

  .col > header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.1rem 0.35rem 0.55rem;
    cursor: grab;
  }
  .col > header.lifting { opacity: 0.4; }
  .col > header .name { text-align: left; }
  .more {
    display: grid;
    place-content: center;
    width: 22px;
    height: 22px;
    border-radius: 6px;
    color: var(--faint);
    opacity: 0;
    transition: opacity 0.12s ease;
  }
  .col:hover .more,
  .col:focus-within .more { opacity: 1; }
  .more:hover { background: var(--hover); color: var(--text); }
  .rename {
    width: 100%;
    padding: 0.2rem 0.35rem;
    border: 1px solid var(--accent);
    border-radius: 7px;
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.78rem;
  }

  /* Where a dragged COLUMN would land. Vertical, because columns move
     sideways — the same idea as the card gap, turned ninety degrees. */
  .col-gap {
    flex: none;
    align-self: stretch;
    width: 3px;
    border-radius: 999px;
    background: var(--accent);
  }

  .add-col {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    align-self: flex-start;
    padding: 0.55rem 0.7rem;
    border: 1px dashed var(--line);
    border-radius: 12px;
    color: var(--faint);
    font-size: 0.82rem;
  }
  .add-col:hover { background: var(--hover); color: var(--text); }
  .name {
    font-size: 0.72rem;
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .count {
    margin-left: auto;
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--faint);
    font-size: 0.68rem;
  }

  .cards {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-height: 0.5rem;
    overflow-y: auto;
  }
  /* The landing place, drawn: the board shows the result instead of asking you
     to imagine it. */
  .gap {
    height: 3px;
    border-radius: 999px;
    background: var(--accent);
  }

  .card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface-solid);
    box-shadow: 0 1px 2px var(--shadow);
  }
  .card.lifting { opacity: 0.4; }
  .open {
    display: block;
    width: 100%;
    padding: 0.55rem 0.6rem;
    text-align: left;
    background: transparent;
  }
  .card:hover { border-color: color-mix(in oklab, var(--accent) 40%, transparent); }
  .ttl { display: block; font-size: 0.86rem; line-height: 1.3; color: var(--text); }
  .meta { display: flex; align-items: center; gap: 0.45rem; margin-top: 0.35rem; font-size: 0.7rem; }
  .obj, .due { color: var(--faint); }
  .due { margin-left: auto; }

  .add {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin-top: 0.45rem;
    padding: 0.4rem 0.5rem;
    border-radius: 8px;
    color: var(--faint);
    font-size: 0.8rem;
  }
  .add:hover { background: var(--hover); color: var(--text); }
</style>
