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
    class="ctxmenu-scrim"
    role="presentation"
    onclick={() => bubbleMenu.hide()}
    oncontextmenu={(e) => {
      e.preventDefault();
      bubbleMenu.hide();
    }}
  ></div>

  <div class="ctxmenu" style="left: {px}px; top: {py}px" role="menu">
    <div class="ctxmenu-head" title={b.name}>{b.name}</div>
    <button class="ctxmenu-item" role="menuitem" onclick={openTimeline}>
      <span class="ctxmenu-ic">🫧</span> Open timeline
    </button>

    {#if !store.kiosk}
      <button class="ctxmenu-item" role="menuitem" onclick={birth}>
        <span class="ctxmenu-ic">➕</span> Birth thread…
      </button>

      <div class="ctxmenu-sep"></div>

      {#if b.level === 'reviewed'}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.unreview(x.id))}>
          <span class="ctxmenu-ic">↩</span> Un-review
        </button>
      {:else}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.review(x.id))}>
          <span class="ctxmenu-ic">👀</span> Mark reviewed
        </button>
      {/if}

      {#if b.level === 'done'}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.reopen(x.id))}>
          <span class="ctxmenu-ic">♻️</span> Reopen
        </button>
      {:else}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.close(x.id))}>
          <span class="ctxmenu-ic">🏆</span> Close (done)
        </button>
      {/if}
    {/if}

    <div class="ctxmenu-sep"></div>
    <button class="ctxmenu-item" role="menuitem" onclick={copyId}>
      <span class="ctxmenu-ic">🔗</span> Copy id
    </button>
  </div>
{/if}
