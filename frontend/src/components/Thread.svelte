<script lang="ts">
  // A thread, against the real server.
  //
  // `ThreadView` draws; this holds the document, the hash it was read at, and
  // what happens when the write comes back 409. Keeping those apart is what
  // lets the view be looked at in /theme/mock with invented data and be the
  // same component here.
  //
  // Every write carries the hash it read. That is the only thing standing
  // between two editors and a lost paragraph, and it is why a conflict is a
  // banner offering a reload rather than a save that quietly wins.
  import { ApiError, api, type Doc, type State, type ThreadHeat, type Workspace } from '../lib/api';
  import { bandName } from '../lib/bands';
  import ThreadView from './ThreadView.svelte';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';

  let {
    thread,
    workspace,
    onback,
    onsearch,
    onchanged,
  }: {
    thread: ThreadHeat;
    workspace: Workspace;
    onback?: () => void;
    onsearch?: () => void;
    /** the thread's row changed — the board is stale now */
    onchanged?: () => void;
  } = $props();

  // The workflow and the bubbles, for the two verbs that need them. Fetched
  // when the thread opens rather than kept in a store: they are small, they are
  // the workspace's configuration, and a copy that lives longer than the screen
  // is a copy that goes out of date.
  let states = $state<State[]>([]);
  let bubbles = $state<{ id: string; name: string }[]>([]);
  $effect(() => {
    workspace.id;
    api.states(workspace.id).then((x) => (states = x)).catch(() => {});
    api.bubbles(workspace.id).then((x) => (bubbles = x)).catch(() => {});
  });

  let moving = $state(false);
  let doomed = $state(false);

  /** Completing is a STATE reaching the completed group, never a field somebody
   *  writes: that is how the server decides it too (`internal/bubble/events.go`),
   *  and a second way of saying "done" is a second answer to it. */
  const completedState = $derived(states.find((s) => s.group === 'completed'));
  const openState = $derived(states.find((s) => s.is_default) ?? states.find((s) => s.group !== 'completed' && s.group !== 'cancelled'));

  async function setState(state?: State) {
    if (!state) {
      error = 'Este workspace no tiene un estado para eso. Defínelo en su flujo de trabajo.';
      return;
    }
    try {
      await api.update('threads', thread.id, { state: state.id });
      onchanged?.();
      onback?.();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function moveTo(bubble: string) {
    moving = false;
    try {
      await api.update('threads', thread.id, { bubble });
      onchanged?.();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function reallyDelete() {
    doomed = false;
    try {
      await api.deleteThread(thread.id);
      onchanged?.();
      onback?.();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  let doc = $state<Doc | null>(null);
  let error = $state('');
  let elsewhere = $state(false);
  let saving = $state(false);

  async function load() {
    try {
      doc = await api.readThread(thread.id);
      error = '';
      elsewhere = false;
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => {
    thread.id;
    load();
  });

  async function save(markdown: string) {
    if (!doc || saving) return;
    // With a conflict standing, the base we hold is old and every write is
    // another 409 — including the blur-save that fires when the pointer leaves
    // the editor to press "recargar", which raced the reload it was answering.
    if (elsewhere) return;
    saving = true;
    try {
      // The base is the hash we READ, not the one we hold: they differ exactly
      // when somebody else wrote, which is the case this exists for.
      doc = await api.patchThread(thread.id, { base: doc.hash, content: markdown });
      elsewhere = false;
      error = '';
    } catch (e) {
      if (e instanceof ApiError && e.conflict) {
        // Not an error to apologise for. Somebody wrote first; the edit is
        // still in the editor, and reloading is a choice offered rather than a
        // save silently lost.
        elsewhere = true;
      } else {
        error = (e as Error).message;
      }
    } finally {
      saving = false;
    }
  }
</script>

{#if error}
  <p class="card glass m-6 p-4 text-sm text-error-500">{error}</p>
{:else if !doc}
  <p class="faint p-6 text-sm">…</p>
{:else}
  <ThreadView
    seq={thread.seq}
    title={thread.name}
    lifecycle={thread.heat.lifecycle}
    reason={thread.heat.reason}
    priority={thread.priority ?? ''}
    threadState={bandName(thread.heat.lifecycle)}
    markdown={doc.content}
    html={doc.html}
    {elsewhere}
    {onback}
    {onsearch}
    onsave={save}
    onreload={load}
    onfinish={thread.heat.lifecycle === 'closed' ? undefined : () => setState(completedState)}
    onreopen={thread.heat.lifecycle === 'closed' ? () => setState(openState) : undefined}
    onmove={() => (moving = true)}
    ondelete={() => (doomed = true)} />

  {#if moving}
    <Dialog open onOpenChange={() => (moving = false)}>
      <Portal>
        <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
        <Dialog.Positioner
          class="fixed inset-0 flex items-center justify-center p-4"
          style="z-index: var(--z-drawer)">
          <Dialog.Content class="card bg-surface-100-900 w-full max-w-sm space-y-4 p-5 shadow-xl">
            <Dialog.Title class="text-lg font-bold">¿A qué burbuja?</Dialog.Title>
            <Dialog.Description class="muted text-sm">
              Una burbuja es la unidad de atención: mover un thread cambia lo que
              flota y lo que se hunde.
            </Dialog.Description>
            <ul class="pick">
              <li>
                <button class="one" class:on={!thread.bubble} onclick={() => moveTo('')}>
                  Sin burbuja
                </button>
              </li>
              {#each bubbles as b (b.id)}
                <li>
                  <button class="one" class:on={thread.bubble === b.id} onclick={() => moveTo(b.id)}>
                    {b.name}
                  </button>
                </li>
              {/each}
            </ul>
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog>
  {/if}

  {#if doomed}
    <Dialog open onOpenChange={() => (doomed = false)}>
      <Portal>
        <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
        <Dialog.Positioner
          class="fixed inset-0 flex items-center justify-center p-4"
          style="z-index: var(--z-drawer)">
          <Dialog.Content class="card bg-surface-100-900 w-full max-w-md space-y-4 p-5 shadow-xl">
            <Dialog.Title class="text-lg font-bold">¿Borrar «{thread.name}»?</Dialog.Title>
            <Dialog.Description class="muted text-sm">
              Se va el thread y su documento del repositorio. Lo escrito sigue en
              la historia de git, que es de donde se recupera si hizo falta.
            </Dialog.Description>
            <div class="flex justify-end gap-2">
              <button class="btn btn-sm preset-tonal-surface" onclick={() => (doomed = false)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-error-500" onclick={reallyDelete}>Borrar</button>
            </div>
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog>
  {/if}
{/if}

<style>
  .pick { display: flex; flex-direction: column; gap: 0.25rem; margin: 0; padding: 0; list-style: none; }
  .one {
    width: 100%;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: transparent;
    color: var(--muted);
    text-align: left;
    font-size: 0.9rem;
  }
  .one:hover { background: var(--hover); color: var(--text); }
  .one.on { border-color: var(--accent, var(--line)); color: var(--text); font-weight: 600; }
</style>
