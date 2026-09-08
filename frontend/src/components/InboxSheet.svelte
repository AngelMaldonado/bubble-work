<script lang="ts">
  // An inbox item, opened.
  //
  // Deliberately thinner than a card: an inbox item has no objective, no impact
  // and no urgency, and that absence IS what makes it an inbox item. Asking for
  // them here would turn capture into triage, which is the thing an inbox exists
  // to keep apart.
  //
  // What it does get is room to think — the same markdown field a card has, so
  // "el cliente pide X" can grow into what X actually is before anybody decides
  // where it goes.
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import XIcon from '@lucide/svelte/icons/x';
  import MarkdownField from './MarkdownField.svelte';

  export type Note = { id: string; text: string; from: string; when: string; body?: string };

  let {
    open = $bindable(false),
    note = $bindable(null),
    render,
    onpromote,
    ondelete,
  }: {
    open?: boolean;
    note?: Note | null;
    render?: (md: string) => string | Promise<string>;
    /** send it to the board, where it acquires an objective and a priority */
    onpromote?: (note: Note) => void;
    ondelete?: (id: string) => void;
  } = $props();

  const anim =
    'transition transition-discrete opacity-0 translate-y-[100px] ' +
    'starting:data-[state=open]:opacity-0 starting:data-[state=open]:translate-y-[100px] ' +
    'data-[state=open]:opacity-100 data-[state=open]:translate-y-0';

  let body = $state('');
  let seeded = $state<string | null>(null);
  $effect(() => {
    if (note && note.id !== seeded) {
      seeded = note.id;
      body = note.body ?? '';
    }
  });
  $effect(() => {
    if (note) note.body = body;
  });
</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Portal>
    <Dialog.Backdrop
      class="fixed inset-0 bg-surface-50-950/50"
      style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-center justify-center p-4"
      style="z-index: var(--z-drawer)">
      <Dialog.Content class="card bg-surface-100-900 w-full max-w-xl space-y-4 p-4 shadow-xl {anim}">
        {#if note}
          <header class="flex items-center justify-between gap-3">
            <Dialog.Title class="min-w-0 flex-1 text-lg font-bold">
              <input class="ttl" bind:value={note.text} aria-label="qué llegó" />
            </Dialog.Title>
            <Dialog.CloseTrigger class="btn-icon hover:preset-tonal">
              <XIcon class="size-4" />
            </Dialog.CloseTrigger>
          </header>

          <p class="who">{note.from} · {note.when}</p>

          <MarkdownField
            bind:value={body}
            {render}
            minHeight="10rem"
            placeholder="¿qué pidió exactamente? ¿a quién afecta? lo que sepas, aunque esté a medias…" />

          <footer>
            <button class="promote" onclick={() => { onpromote?.(note); open = false; }}>
              → al kanban
            </button>
            <button class="danger" onclick={() => { ondelete?.(note.id); open = false; }}>
              🗑 descartar
            </button>
          </footer>
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  .ttl {
    width: 100%;
    padding: 0.2rem 0;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 1.05rem;
    font-weight: 700;
  }
  .who { margin: 0; color: var(--faint); font-size: 0.75rem; }
  footer { display: flex; align-items: center; gap: 0.5rem; }
  .promote {
    padding: 0.35rem 0.7rem;
    border: 1px solid color-mix(in oklab, var(--accent) 45%, transparent);
    border-radius: 8px;
    color: var(--accent);
    font-size: 0.82rem;
  }
  .promote:hover { background: color-mix(in oklab, var(--accent) 12%, transparent); }
  .danger {
    margin-left: auto;
    padding: 0.35rem 0.6rem;
    border-radius: 8px;
    color: var(--faint);
    font-size: 0.82rem;
  }
  .danger:hover {
    color: var(--color-error-500);
    background: color-mix(in oklab, var(--color-error-500) 12%, transparent);
  }
</style>
