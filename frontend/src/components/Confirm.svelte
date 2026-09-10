<script lang="ts" module>
  /** What is about to be destroyed, and what to run if the answer is yes.
   *
   *  `null` is the closed state, so asking is an assignment and answering is
   *  the dialog setting it back — there is no second boolean to keep in step
   *  with the question. */
  export type Doom = {
    /** the question, in the shape the product already asks it: «¿Borrar «X»?» */
    title: string;
    /** what is actually lost, said plainly. Never omitted for a destructive
     *  action: "are you sure" without it is a speed bump, not a warning. */
    body?: string;
    /** the affirmative button. "Borrar" unless this destroys something that is
     *  not deleted — closing a bubble, say. */
    verb?: string;
    go: () => void;
  } | null;
</script>

<script lang="ts">
  // One confirmation, asked the same way everywhere.
  //
  // Every destructive action in the product goes through this: a card's ✕, a
  // thread's, a note's, an objective's, a page's, closing a bubble. It lives in
  // one file because five hand-written dialogs are five chances for one of them
  // to say "Eliminar" where the others say "Borrar", to lose its description,
  // or — the one that matters — to ship without asking at all.
  //
  // It is deliberately owned by whoever performs the WRITE rather than by the
  // view that draws the ✕: the writer is the one that knows the name of the
  // thing and what disappears with it, and a view that confirms its own deletes
  // would ask again in the mock, where nothing is deleted.
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';

  let { ask = $bindable(null) }: { ask?: Doom } = $props();

  function yes() {
    const go = ask?.go;
    ask = null;
    go?.();
  }
</script>

{#if ask}
  <Dialog open onOpenChange={() => (ask = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-md space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">{ask.title}</Dialog.Title>
          {#if ask.body}
            <Dialog.Description class="muted text-sm">{ask.body}</Dialog.Description>
          {/if}
          <div class="flex justify-end gap-2">
            <button class="btn btn-sm preset-tonal-surface" onclick={() => (ask = null)}>
              Cancelar
            </button>
            <!-- Focused, because the keyboard path to a confirmation should end
                 somewhere: Enter answers the question that was asked, Escape
                 closes it, and neither is a guess. -->
            <button
              class="btn btn-sm preset-filled-error-500"
              {@attach (el: HTMLButtonElement) => el.focus()}
              onclick={yes}>{ask.verb ?? 'Borrar'}</button>
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}
