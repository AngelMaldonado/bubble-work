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
  import { ago, cycleLeft } from '../lib/when';
  import { filters, lastOpenBubble } from '../lib/filters.svelte';

  let {
    workspace,
    board,
    onOpen,
    onreload,
    onnew,
    scroller = null,
  }: {
    /** El workspace en pantalla, o `null` cuando el board es TODOS.
     *
     *  Null y no un workspace inventado: los verbos de esta pantalla —crear una
     *  burbuja, un thread, renombrar, cerrar— necesitan saber DÓNDE aterrizan, y
     *  una vista de varios proyectos no puede contestar cuál. Así que en Todos
     *  se mira y se abre, y lo que se escribe se escribe desde su proyecto. */
    workspace: Workspace | null;
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

  /** Todos: más de un proyecto en la misma pantalla. Es lo que decide si esto es
   *  un board donde se trabaja o un mirador donde se compara. */
  const all = $derived(!workspace);
  const projectOf = (id: string) =>
    (board?.workspaces ?? []).find((w) => w.id === id)?.name ?? '';

  const threadsOf = (bubble: string) => (board?.threads ?? []).filter((t) => t.bubble === bubble);

  // Who works here. Read once per workspace, for the one field on this screen
  // that needs a person: who is accountable for a bubble.
  //
  //  En Todos no se pide: serían N peticiones para dibujar iniciales, y en un
  //  board de varios proyectos lo que hace falta saber de un orbe no es quién
  //  responde sino de dónde es. La insignia del proyecto ocupa ese sitio.
  let roster = $state<{ id: string; name: string }[]>([]);
  $effect(() => {
    const id = workspace?.id;
    roster = [];
    if (id) api.roster(id).then((r) => (roster = r)).catch(() => (roster = []));
  });
  const nameOf = (id: string) => roster.find((p) => p.id === id)?.name ?? '';

  // A qué proyectos te limitaste, en Todos.
  //
  // Se aplica aquí y no en el servidor: el board que llega es la verdad —qué
  // flota y por qué— y limitar la vista es una elección de quien mira, que
  // cambia al hacer clic y no debería costar una petición. El servidor sigue
  // siendo el que clasifica; esto sólo decide qué se dibuja de lo clasificado.
  const f = $derived(filters.current);

  const visible = $derived(
    (board?.bubbles ?? []).filter((b) => !f.projects.length || f.projects.includes(b.workspace)),
  );

  const orbs = $derived<BoardBubble[]>(
    visible.map((b) => ({
      id: b.id,
      name: b.name,
      life: b.heat.lifecycle,
      // The orb carries the initials of whoever is accountable. A band tells
      // you a bubble went quiet; the initials tell you who to ask, which is the
      // other half of what 😴 means.
      // Sólo los que ya tienen nombre: el roster llega por su cuenta, y un
      // avatar sin nombre no dice nada que valga la pena dibujar.
      owner: all ? '' : nameOf((b.owners ?? [])[0] ?? ''),
      people: all ? [] : (b.owners ?? []).map(nameOf).filter(Boolean),
      project: all ? projectOf(b.workspace) : '',
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

  // Volver de un thread devuelve el cajón donde estaba.
  //
  // Un thread se abre DESDE una burbuja, así que volver al board y encontrarlo
  // cerrado obliga a buscar otra vez la burbuja de la que acababas de salir. Se
  // guarda al irse y se consume al volver — una vez, no en cada dibujado, o
  // cerrar el cajón a mano lo reabriría en el siguiente cambio del board.
  let restored = $state(false);
  $effect(() => {
    if (restored || !board) return;
    restored = true;
    const id = lastOpenBubble.get();
    const b = id ? (board.bubbles.find((x) => x.id === id) ?? null) : null;
    if (!b) return;
    open = b;
    drawer = true;
    // Y se ve dónde está: el cajón tapa media pantalla, y abrirlo sobre una
    // burbuja que quedó fuera de vista cuenta la mitad de la historia.
    requestAnimationFrame(() =>
      document.getElementById('bw-' + b.id)?.scrollIntoView({ block: 'center' }),
    );
  });

  // De ahí en adelante, lo guardado ES el estado del cajón: abierto se recuerda,
  // cerrado se olvida. Una sola regla en un solo sitio, en vez de anotarlo en
  // cada puerta por la que se puede salir — y las puertas son cuatro: el thread,
  // la ×, Escape y el clic fuera.
  $effect(() => {
    if (!restored) return;
    lastOpenBubble.set(drawer && open ? open.id : '');
  });
  // The drawer reads the bubble from the BOARD, not from the copy taken when it
  // was opened: renaming, handing it over or closing it reloads the board, and
  // a panel showing the snapshot would keep displaying what you just changed.
  const shown = $derived(board?.bubbles.find((b) => b.id === open?.id) ?? open);
  let drawer = $state(false);
  // How much of the cycle this bubble has left since it last produced. The
  // window is the workspace's calibration, so the bar means the same thing the
  // bands do — recalibrating moves both, and neither is stored.
  const cycle = $derived(
    shown && !shown.closed
      ? cycleLeft(shown.warm_at ?? '', Number(board?.tuning?.cycle_hours ?? 0))
      : null,
  );
  // Opens the drawer with its "+ thread" field already asking, so the orb's
  // menu item lands where the drawer's button would have taken you rather than
  // opening a second way to name a thread.
  let naming = $state(false);
  let renaming = $state<BubbleHeat | null>(null);
  let fresh = $state('');
  let doom = $state<Doom>(null);
  // Closing asks for one line — how it ended, in the words of whoever closed
  // it. It is optional and it is the reason this is not a plain confirmation:
  // a bubble that dies without saying why teaches nobody anything.
  let closing = $state<BubbleHeat | null>(null);
  let closure = $state('');

  async function newThread(name: string) {
    if (!open || !workspace) return;
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
    // En Todos el menú del orbe abre, y nada más. Renombrar o cerrar una burbuja
    // desde una pantalla que no es la de su proyecto es escribir a ciegas: lo
    // que cambia no está delante.
    if (all && what !== 'open') return;
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
      closing = bubble;
      closure = bubble.closure ?? '';
    }
  }

  function closeBubble() {
    const b = closing;
    closing = null;
    if (!b) return;
    write(() =>
      api.update('bubbles', b.id, {
        closed_at: new Date().toISOString().replace('T', ' '),
        closure: closure.trim(),
      }),
    );
  }

  /** Reopening clears both: `closed_at` because it is not closed any more, and
   *  the closure because it described an ending that no longer holds. */
  const reopen = (b: BubbleHeat) =>
    write(() => api.update('bubbles', b.id, { closed_at: '', closure: '' }));

  function rename() {
    const b = renaming;
    const name = fresh.trim();
    renaming = null;
    if (!b || !name || name === b.name) return;
    write(() => api.update('bubbles', b.id, { name }));
  }
</script>


<!-- Los proyectos, en Todos: a cuáles quieres limitarte.
     Sobre las bandas y no en una barra lateral, porque la respuesta a «¿por qué
     falta esta burbuja?» tiene que estar donde se hizo la pregunta. Lo elegido
     sobrevive irse a un thread y volver: vive en el navegador, no en la
     dirección — un enlace lleva QUÉ miras, no a qué te limitaste.

     Sólo aquí: dentro de un proyecto, elegir proyecto es decir dos veces lo
     mismo. Y sólo las bandas se quedaron fuera de esto a propósito — una banda
     no se filtra, se mira: esconderla cambia la forma del board, que es la única
     cosa que el board tiene que decir. -->
{#snippet controls()}
  {#if all && (board?.workspaces ?? []).length > 1}
    <div class="projects">
      <div class="group" role="group" aria-label="Proyectos">
        {#each board?.workspaces ?? [] as w (w.id)}
          <button
            class="chip"
            class:on={f.projects.includes(w.id)}
            aria-pressed={f.projects.includes(w.id)}
            onclick={() => filters.toggle(w.id)}>
            {w.name}
          </button>
        {/each}
      </div>

      {#if filters.on}
        <!-- Cuántas quedaron fuera, y la salida. Un board limitado que no dice
             que lo está es un board que miente sobre cuánto trabajo hay. -->
        <button class="chip clear" onclick={() => filters.clear()}>
          ✕ ver todos · {(board?.bubbles.length ?? 0) - visible.length} ocultas
        </button>
      {/if}
    </div>
  {/if}
{/snippet}

<Minimap items={mapItems} {scroller} />

{#if error}
  <p class="card glass m-6 p-4 text-sm text-error-500">{error}</p>
{/if}

{#if !board}
  <p class="faint p-6 text-sm">…</p>
{:else}
  <BubbleBoard
    title={workspace?.name ?? 'Todos los workspaces'}
    bubbles={orbs}
    {controls}
    onnew={all ? undefined : onnew}
    onaction={act}
    onopen={(b) => {
      open = board?.bubbles.find((x) => x.id === b.id) ?? null;
      naming = false;
      drawer = true;
    }} />

  <BubbleDrawer
    bind:open={drawer}
    name={shown?.name ?? ''}
    outcome={shown?.outcome ?? ''}
    lifecycle={shown?.heat.lifecycle ?? 'hot'}
    reason={shown?.heat.reason ?? ''}
    owners={shown?.owners ?? []}
    closed={shown?.closed ?? false}
    closure={shown?.closure ?? ''}
    people={roster}
    threads={threadsOf(shown?.id ?? '').map((t) => ({
      seq: t.seq,
      title: t.name,
      lifecycle: t.heat.lifecycle,
      priority: t.priority,
      // The second line of the row, which is what the mock drew and the real
      // drawer had nothing to fill: who has it, and how long since anything
      // happened to it.
      owner: (t.assignees ?? []).map(nameOf).filter(Boolean).join(', '),
      age: ago(t.at ?? ''),
    }))}
    cyclePct={cycle?.pct ?? null}
    cycleLeft={cycle?.words ?? ''}
    onopenthread={(seq) => {
      const t = threadsOf(open?.id ?? '').find((x) => x.seq === seq);
      // El cajón NO se cierra al salir: esta pantalla se va entera, y dejarlo
      // abierto es lo que hace que «dónde estaba» siga siendo verdad cuando se
      // vuelva.
      if (t) onOpen(t);
    }}
    onnewthread={all ? undefined : newThread}
    ondeletethread={all ? undefined : removeThread}
    onoutcome={all
      ? undefined
      : (text) => shown && write(() => api.update('bubbles', shown.id, { outcome: text }))}
    onowners={all
      ? undefined
      : (ids: string[]) => shown && write(() => api.update('bubbles', shown.id, { owners: ids }))}
    onclose={all
      ? undefined
      : () => shown && act('close', { id: shown.id, name: shown.name, life: shown.heat.lifecycle })}
    onreopen={all ? undefined : () => shown && reopen(shown)}
    bind:naming />
{/if}

<Confirm bind:ask={doom} />

{#if closing}
  <!-- Cerrar no borra: baja a la banda de cerradas con todo lo que tiene
       dentro, y reabrirla es una escritura. Lo que sí se pide es la frase. -->
  <Dialog open onOpenChange={() => (closing = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-md space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">¿Cerrar «{closing.name}»?</Dialog.Title>
          <Dialog.Description class="muted text-sm">
            Baja a la banda de cerradas con sus threads. No se borra nada, y se
            puede reabrir desde el mismo cajón.
          </Dialog.Description>
          <form onsubmit={(e) => { e.preventDefault(); closeBubble(); }}>
            <input
              class="input"
              placeholder="¿Cómo terminó? (opcional)"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={closure}
              {@attach (el: HTMLInputElement) => el.focus()} />
            <div class="mt-4 flex justify-end gap-2">
              <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (closing = null)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-error-500">Cerrar</button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

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

<style>
  /* Los proyectos son CROMO, no contenido: por debajo del título en peso, tamaño
     y color, o compiten con el board que están describiendo. Ningún acento
     inventado — el encendido se dice con el texto a plena opacidad sobre el
     mismo `--hover` que usa el resto de la aplicación. */
  .projects {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    align-items: center;
    /* El hueco antes de «ver todos» es varias veces el de dentro del grupo:
       quitar la limitación no es un proyecto más de la lista. */
    gap: 0.4rem 1.4rem;
    margin-top: 1rem;
  }
  .group {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 0.35rem;
  }
  .chip {
    padding: 0.34rem 0.68rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: transparent;
    color: var(--faint);
    font-family: inherit;
    font-size: 0.72rem;
    font-weight: 600;
    line-height: 1;
    cursor: pointer;
  }
  .chip:hover {
    color: var(--text);
  }
  .chip.on {
    background: var(--hover);
    color: var(--text);
  }

  /* Ver todos no es un proyecto más: sin borde hasta que se apunta, para que no
     compita con los que sí dicen qué estás viendo. */
  .clear {
    border-color: transparent;
    font-weight: 500;
  }
  .clear:hover {
    border-color: var(--line);
  }
</style>
