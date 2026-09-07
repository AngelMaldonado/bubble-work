<script lang="ts">
  // One field that finds a thread, a bubble or a page — and, when something is
  // waiting on a pick, hands it back instead of navigating.
  //
  // It is the same field in both cases on purpose: "relate this thread to
  // another" and "take me to that thread" are the same act of finding, and
  // giving each its own control is how a picker ends up nailed to the bottom of
  // a sidebar answering the second half of the question first.
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import SearchIcon from '@lucide/svelte/icons/search';

  export type Hit = { id: string; kind: string; icon: string; title: string; hint?: string };

  let {
    open = $bindable(false),
    items = [],
    /** shown in place of the placeholder while something is waiting on a pick */
    prompt = '',
    onpick,
  }: {
    open?: boolean;
    items?: Hit[];
    prompt?: string;
    onpick?: (hit: Hit) => void;
  } = $props();

  let q = $state('');
  let cursor = $state(0);

  const hits = $derived(
    q.trim()
      ? items.filter((i) => (i.title + ' ' + (i.hint ?? '')).toLowerCase().includes(q.trim().toLowerCase()))
      : items,
  );

  // The cursor is an index into a list that changes as you type; clamping it
  // here is what stops Enter from picking whatever happens to be at position 7.
  $effect(() => {
    if (cursor > hits.length - 1) cursor = Math.max(0, hits.length - 1);
  });
  $effect(() => {
    if (open) {
      q = '';
      cursor = 0;
    }
  });

  function keys(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      cursor = Math.min(cursor + 1, hits.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      cursor = Math.max(cursor - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      pick(hits[cursor]);
    }
  }

  function pick(hit?: Hit) {
    if (!hit) return;
    open = false;
    onpick?.(hit);
  }
</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Dialog.Backdrop class="omni-scrim" />
  <Portal>
    <Dialog.Positioner class="omni-pos">
      <Dialog.Content class="omni">
        <label class="field">
          <SearchIcon class="size-4 shrink-0" />
          <!-- svelte-ignore a11y_autofocus -->
          <input
            bind:value={q}
            onkeydown={keys}
            placeholder={prompt || 'Buscar un thread, una burbuja o una página…'}
            {@attach (el: HTMLInputElement) => el.focus()} />
          <kbd>esc</kbd>
        </label>

        <ul class="hits">
          {#each hits as h, i (h.id)}
            <li>
              <button class="hit" class:on={i === cursor} onmouseenter={() => (cursor = i)} onclick={() => pick(h)}>
                <span class="ico" aria-hidden="true">{h.icon}</span>
                <span class="t">{h.title}</span>
                {#if h.hint}<span class="hint">{h.hint}</span>{/if}
                <span class="kind">{h.kind}</span>
              </button>
            </li>
          {/each}
          {#if !hits.length}
            <li class="none">Nada con «{q}»</li>
          {/if}
        </ul>
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  :global(.omni-scrim) {
    position: fixed;
    inset: 0;
    z-index: var(--z-drawer-scrim);
    background: color-mix(in oklab, var(--bg) 45%, transparent);
    backdrop-filter: blur(2px);
  }
  :global(.omni-pos) {
    position: fixed;
    inset: 0;
    z-index: var(--z-drawer);
    display: flex;
    justify-content: center;
    /* Near the top, not centred: the list grows downward, and a box that grows
       from the middle of the screen moves the field you are typing in. */
    align-items: flex-start;
    padding-top: 12vh;
    pointer-events: none;
  }
  :global(.omni) {
    pointer-events: auto;
    width: min(560px, 92vw);
    max-height: 70vh;
    display: flex;
    flex-direction: column;
    border: 1px solid var(--line);
    border-radius: 16px;
    background: var(--surface-solid);
    box-shadow: 0 30px 80px var(--shadow-strong);
    overflow: hidden;
  }

  .field {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.85rem 1rem;
    border-bottom: 1px solid var(--line);
    color: var(--faint);
  }
  .field input {
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 1rem;
    outline: none;
  }
  kbd {
    flex: none;
    padding: 0 0.35rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    font-size: 0.7rem;
  }

  .hits {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    margin: 0;
    padding: 0.35rem;
    list-style: none;
  }
  .hit {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    padding: 0.5rem 0.6rem;
    border: none;
    border-radius: 9px;
    background: transparent;
    color: var(--muted);
    text-align: left;
  }
  /* One highlight, driven by the cursor — the pointer moves it rather than
     drawing a second one, so the keyboard and the mouse never disagree about
     what Enter will open. */
  .hit.on { background: var(--hover); color: var(--text); }
  .ico { flex: none; }
  .t { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hint, .kind { flex: none; font-size: 0.72rem; color: var(--faint); }
  .kind {
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--hover);
  }
  .none { padding: 1rem; color: var(--faint); font-size: 0.85rem; }
</style>
