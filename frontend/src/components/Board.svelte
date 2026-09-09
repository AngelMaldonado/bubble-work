<script lang="ts">
  // The board, against the real server: the same component the mock draws, fed
  // by `/board` instead of by fixtures.
  //
  // Buoyancy is spatial. Bubbles float in the band their hottest thread put
  // them in, and clicking one opens the drawer rather than a page: the whole
  // point of the board is comparison, and a navigation costs you the thing you
  // were comparing against.
  import { api, type Board, type BubbleHeat, type ThreadHeat, type Workspace } from '../lib/api';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import BubbleBoard, { type BoardBubble } from './BubbleBoard.svelte';
  import BubbleDrawer from './BubbleDrawer.svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
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
  // Opens the drawer with its "+ thread" field already asking, so the orb's
  // menu item lands where the drawer's button would have taken you rather than
  // opening a second way to name a thread.
  let naming = $state(false);
  let renaming = $state<BubbleHeat | null>(null);
  let fresh = $state('');
  let doom = $state<Doom>(null);

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

  /** Every write here reloads: the server classifies, and a board patched in
   *  the browser is a second opinion about which band something is in. */
  async function write(run: () => Promise<unknown>) {
    try {
      await run();
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function removeThread(seq: number) {
    const t = threadsOf(open?.id ?? '').find((x) => x.seq === seq);
    if (!t) return;
    doom = {
      title: `¿Borrar «${t.name}»?`,
      body: 'El thread #' + t.seq + ' sale de la burbuja y del planeador. Su documento sigue en la historia de git, que es de donde se recupera si hacía falta.',
      go: () => write(() => api.deleteThread(t.id)),
    };
  }

  /** The orb's right-click. The same four things the drawer does, reached from
   *  where the eye already is — the bubble itself — instead of after opening
   *  it. Nothing new happens here: this is the drawer's verbs, wired. */
  function act(what: string, b: BoardBubble) {
    const bubble = board?.bubbles.find((x) => x.id === b.id) ?? null;
    if (!bubble) return;
    if (what === 'open' || what === 'thread') {
      open = bubble;
      naming = what === 'thread';
      drawer = true;
      return;
    }
    if (what === 'rename') {
      renaming = bubble;
      fresh = bubble.name;
      return;
    }
    if (what === 'close') {
      doom = {
        title: `¿Cerrar «${bubble.name}»?`,
        // Closing is not deleting, and saying so is the difference between a
        // decision and a scare: it drops to the closed band with its threads,
        // and reopening it is one write away.
        body: 'Baja a la banda de cerradas con todo lo que tiene dentro. No se borra nada: es la forma de decir que este trabajo ya no compite por atención.',
        verb: 'Cerrar',
        go: () =>
          write(() =>
            api.update('bubbles', bubble.id, {
              closed_at: new Date().toISOString().replace('T', ' '),
            }),
          ),
      };
    }
  }

  function rename() {
    const b = renaming;
    const name = fresh.trim();
    renaming = null;
    if (!b || !name || name === b.name) return;
    write(() => api.update('bubbles', b.id, { name }));
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
    onaction={act}
    onopen={(b) => {
      open = board?.bubbles.find((x) => x.id === b.id) ?? null;
      naming = false;
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
    ondeletethread={removeThread}
    bind:naming />
{/if}

<Confirm bind:ask={doom} />

{#if renaming}
  <!-- A bubble is renamed in place: its outcome, its threads and its band are
       the same work under a better name. -->
  <Dialog open onOpenChange={() => (renaming = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-sm space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">Renombrar la burbuja</Dialog.Title>
          <form onsubmit={(e) => { e.preventDefault(); rename(); }}>
            <input
              class="input"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={fresh}
              {@attach (el: HTMLInputElement) => el.select()} />
            <div class="mt-4 flex justify-end gap-2">
              <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (renaming = null)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-primary-500" disabled={!fresh.trim()}>Renombrar</button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}
