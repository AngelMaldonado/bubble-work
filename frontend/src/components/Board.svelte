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
    onOpen,
    onload,
    scroller = null,
  }: {
    workspace: Workspace;
    onOpen: (t: ThreadHeat) => void;
    /** the board as it arrived, for whoever else needs to read the same one */
    onload?: (board: Board) => void;
    /** the box that actually scrolls, for the minimap's spy */
    scroller?: HTMLElement | null;
  } = $props();

  let board = $state<Board | null>(null);
  let error = $state('');

  // Refetched rather than recomputed on the client: heat is a pure function of
  // evidence and TIME, and the server is the one holding both. A board that
  // ages in the browser would drift from the one everyone else sees.
  async function load() {
    try {
      board = await api.board(workspace.id);
      onload?.(board);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => {
    workspace.id;
    load();
  });

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

  // Threads with no bubble. They are work too, and a board that only draws what
  // is filed is a board that hides the backlog it should be embarrassed by.
  const unfiled = $derived((board?.threads ?? []).filter((t) => !t.bubble && t.heat.lifecycle !== 'closed'));

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
    onopen={(b) => {
      open = board?.bubbles.find((x) => x.id === b.id) ?? null;
      drawer = true;
    }}>
    {#if unfiled.length}
      <section class="unfiled">
        <h2 class="faint display mb-3 text-center text-sm tracking-widest uppercase">Sin burbuja</h2>
        <ul class="card glass divide-y-[1px] divide-[var(--line)] p-2">
          {#each unfiled as t (t.id)}
            <li>
              <button
                class="band-{t.heat.lifecycle} flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:[background:var(--hover)]"
                onclick={() => onOpen(t)}>
                <span class="dot"></span>
                <span class="faint tabular-nums">#{t.seq}</span>
                <span class="min-w-0 flex-1 truncate">{t.name}</span>
                {#if t.priority}<span class="faint text-xs">{t.priority}</span>{/if}
              </button>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <!-- Say what it was measured against. A cold board and a short cycle look
         identical until somebody can see the calibration. -->
    <p class="faint mt-8 text-center text-xs">
      ciclo de {board.tuning.cycle_hours}h · dormido tras {board.tuning.dormant_cycles} ciclos ·
      calculado {new Date(board.at).toLocaleString()}
    </p>
  </BubbleBoard>

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

<style>
  .unfiled { padding-top: 2rem; max-width: 620px; margin: 0 auto; }
</style>
