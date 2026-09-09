<script lang="ts">
  // The board, against the real server: the same component the mock draws, fed
  // by `/board` instead of by fixtures.
  //
  // Buoyancy is spatial. Bubbles float in the band their hottest thread put
  // them in, and clicking one opens the drawer rather than a page: the whole
  // point of the board is comparison, and a navigation costs you the thing you
  // were comparing against.
  import { api, type Board, type BubbleHeat, type ThreadHeat, type Workspace } from '../lib/api';
  import BubbleBoard, { type BoardBubble } from './BubbleBoard.svelte';
  import BubbleDrawer from './BubbleDrawer.svelte';
  import Minimap, { type MapItem } from './Minimap.svelte';

  let {
    workspace,
    board,
    onOpen,
    onreload,
    onnew,
    scroller = null,
  }: {
    workspace: Workspace;
    /** The board, fetched by whoever owns the screen.
     *
     *  Not fetched here any more: a link straight to a thread has to resolve
     *  `#14` before this component is on the page at all, so the one copy of
     *  the board lives above and everybody reads THAT one. Two fetches of the
     *  same board are two answers that can disagree. */
    board: Board | null;
    onOpen: (t: ThreadHeat) => void;
    /** ask for it again — writing here changes what the server would say */
    onreload?: () => void;
    /** right-click on the empty board: make a bubble, or a thread */
    onnew?: (what: 'bubble' | 'thread') => void;
    /** the box that actually scrolls, for the minimap's spy */
    scroller?: HTMLElement | null;
  } = $props();

  let error = $state('');
  const load = () => onreload?.();

  const threadsOf = (bubble: string) => (board?.threads ?? []).filter((t) => t.bubble === bubble);

  const orbs = $derived<BoardBubble[]>(
    (board?.bubbles ?? []).map((b) => ({
      id: b.id,
      name: b.name,
      life: b.heat.lifecycle,
      // The flame counts what is PRODUCING inside it, which is the one number
      // an orb can carry without becoming a card.
      burning: threadsOf(b.id).filter((t) => t.heat.lifecycle === 'hot').length,
    })),
  );

  // A thread with no bubble does NOT appear here. The board is bubbles — the
  // unit of attention — and a bubble is what floats or sinks; loose threads
  // under a "Sin burbuja" heading turned it into a list of everything, which is
  // the thing the board exists instead of. They are not lost: the planner shows
  // every thread by objective, and ⌘K finds any of them by name.

  // The minimap reads the same list the board draws, so the two cannot disagree
  // about what is on the page.
  const mapItems = $derived<MapItem[]>(orbs.map((b) => ({ id: b.id, name: b.name, life: b.life })));

  let open = $state<BubbleHeat | null>(null);
  let drawer = $state(false);

  async function newThread(name: string) {
    if (!open) return;
    try {
      await api.createThread({ workspace: workspace.id, bubble: open.id, name });
      // Creating a thread IS evidence, so the band this bubble sits in can have
      // changed by the time the drawer closes. Ask the server rather than
      // guessing: it is the one that classifies.
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function removeThread(seq: number) {
    const t = threadsOf(open?.id ?? '').find((x) => x.seq === seq);
    if (!t) return;
    try {
      await api.deleteThread(t.id);
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }
</script>

<Minimap items={mapItems} {scroller} />

{#if error}
  <p class="card glass m-6 p-4 text-sm text-error-500">{error}</p>
{/if}

{#if !board}
  <p class="faint p-6 text-sm">…</p>
{:else}
  <BubbleBoard
    title={workspace.name}
    bubbles={orbs}
    {onnew}
    onopen={(b) => {
      open = board?.bubbles.find((x) => x.id === b.id) ?? null;
      drawer = true;
    }} />

  <BubbleDrawer
    bind:open={drawer}
    name={open?.name ?? ''}
    outcome={open?.outcome ?? ''}
    lifecycle={open?.heat.lifecycle ?? 'hot'}
    reason={open?.heat.reason ?? ''}
    threads={threadsOf(open?.id ?? '').map((t) => ({
      seq: t.seq,
      title: t.name,
      lifecycle: t.heat.lifecycle,
      priority: t.priority,
    }))}
    onopenthread={(seq) => {
      const t = threadsOf(open?.id ?? '').find((x) => x.seq === seq);
      if (t) {
        drawer = false;
        onOpen(t);
      }
    }}
    onnewthread={newThread}
    ondeletethread={removeThread} />
{/if}
