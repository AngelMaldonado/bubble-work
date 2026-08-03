<script lang="ts">
  import { bubbleMenu } from '../lib/contextmenu.svelte';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import type { BubbleView } from '../lib/types';

  // birth uses the existing form flow, owned by Workspace.
  let { onbirth }: { onbirth: (b: BubbleView) => void } = $props();

  const b = $derived(bubbleMenu.bubble);

  const MENU_W = 210;
  const MENU_H = 300;
  const px = $derived(Math.max(8, Math.min(bubbleMenu.x, window.innerWidth - MENU_W - 8)));
  const py = $derived(Math.max(8, Math.min(bubbleMenu.y, window.innerHeight - MENU_H - 8)));

  // run a bubble action, then refresh the board.
  async function act(fn: (b: BubbleView) => Promise<unknown>): Promise<void> {
    const bb = bubbleMenu.bubble;
    bubbleMenu.hide();
    if (!bb) return;
    try {
      await fn(bb);
      await store.refresh();
    } catch (e) {
      store.error = e instanceof ApiError ? e.message : String(e);
    }
  }

  function openTimeline(): void {
    const bb = bubbleMenu.bubble;
    bubbleMenu.hide();
    if (bb) store.openDetail(bb);
  }

  function birth(): void {
    const bb = bubbleMenu.bubble;
    bubbleMenu.hide();
    if (bb) onbirth(bb);
  }

  function copyId(): void {
    const bb = bubbleMenu.bubble;
    bubbleMenu.hide();
    if (bb) navigator.clipboard?.writeText(bb.id).catch(() => {});
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape') bubbleMenu.hide();
  }
</script>

<svelte:window onkeydown={onKey} onresize={() => bubbleMenu.hide()} onscroll={() => bubbleMenu.hide()} />

{#if bubbleMenu.open && b}
  <!-- click/right-click anywhere else dismisses -->
  <div
    class="cm-scrim"
    role="presentation"
    onclick={() => bubbleMenu.hide()}
    oncontextmenu={(e) => {
      e.preventDefault();
      bubbleMenu.hide();
    }}
  ></div>

  <div class="cm" style="left: {px}px; top: {py}px" role="menu">
    <div class="cm-head" title={b.name}>{b.name}</div>
    <button class="cm-item" role="menuitem" onclick={openTimeline}>
      <span class="ic">🫧</span> Open timeline
    </button>

    {#if !store.kiosk}
      <button class="cm-item" role="menuitem" onclick={birth}>
        <span class="ic">➕</span> Birth thread…
      </button>

      <div class="cm-sep"></div>

      {#if b.level === 'reviewed'}
        <button class="cm-item" role="menuitem" onclick={() => act((x) => api.unreview(x.id))}>
          <span class="ic">↩</span> Un-review
        </button>
      {:else}
        <button class="cm-item" role="menuitem" onclick={() => act((x) => api.review(x.id))}>
          <span class="ic">👀</span> Mark reviewed
        </button>
      {/if}

      {#if b.level === 'done'}
        <button class="cm-item" role="menuitem" onclick={() => act((x) => api.reopen(x.id))}>
          <span class="ic">♻️</span> Reopen
        </button>
      {:else}
        <button class="cm-item" role="menuitem" onclick={() => act((x) => api.close(x.id))}>
          <span class="ic">🏆</span> Close (done)
        </button>
      {/if}
    {/if}

    <div class="cm-sep"></div>
    <button class="cm-item" role="menuitem" onclick={copyId}>
      <span class="ic">🔗</span> Copy id
    </button>
  </div>
{/if}

<style>
  .cm-scrim {
    position: fixed;
    inset: 0;
    z-index: 80;
    background: transparent;
    border: none;
  }
  .cm {
    position: fixed;
    z-index: 81;
    min-width: 200px;
    padding: 0.35rem;
    border-radius: 12px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 20px 50px var(--shadow-strong);
    display: grid;
    gap: 1px;
  }
  .cm-head {
    padding: 0.35rem 0.55rem 0.4rem;
    font-size: 0.72rem;
    font-weight: 700;
    color: var(--faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 260px;
  }
  .cm-item {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    width: 100%;
    text-align: left;
    padding: 0.45rem 0.55rem;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    cursor: pointer;
    font-size: 0.85rem;
  }
  .cm-item:hover {
    background: var(--hover);
  }
  .ic {
    width: 1.1rem;
    text-align: center;
    flex: none;
    font-size: 0.85rem;
  }
  .cm-sep {
    height: 1px;
    margin: 0.25rem 0.3rem;
    background: var(--line);
  }
</style>
