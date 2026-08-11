<script lang="ts">
  import { bubbleMenu } from '../lib/contextmenu.svelte';
  import { store } from '../lib/store.svelte';
  import { t } from '../lib/i18n.svelte';
  import ConfirmDelete from './ConfirmDelete.svelte';
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

  // Deleting is irreversible, so the menu only ARMS it — the dialog is the act.
  let pendingDelete = $state<BubbleView | null>(null);
  let deleting = $state(false);

  function armDelete(): void {
    pendingDelete = bubbleMenu.bubble ?? null;
    bubbleMenu.hide();
  }

  async function confirmDelete(): Promise<void> {
    const b = pendingDelete;
    if (!b) return;
    deleting = true;
    try {
      await api.deleteBubble(b.id);
      pendingDelete = null;
      await store.refresh();
    } catch (e) {
      store.error = e instanceof ApiError ? e.message : String(e);
    } finally {
      deleting = false;
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

  // A menu cannot hold a text field, so renaming opens the panel that owns the
  // name and lands in it — the same place a rename would have taken you anyway.
  function rename(): void {
    const bb = bubbleMenu.bubble;
    bubbleMenu.hide();
    if (bb) store.openDetail(bb, { rename: true });
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
      <span class="ctxmenu-ic">🫧</span> {t('bubble.openTimeline')}
    </button>

    {#if !store.kiosk}
      <button class="ctxmenu-item" role="menuitem" onclick={birth}>
        <span class="ctxmenu-ic">➕</span> {t('bubble.birthThread')}
      </button>

      <button class="ctxmenu-item" role="menuitem" onclick={rename}>
        <span class="ctxmenu-ic">✏️</span> {t('bubble.rename')}
      </button>

      <div class="ctxmenu-sep"></div>

      {#if b.level === 'reviewed'}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.unreview(x.id))}>
          <span class="ctxmenu-ic">↩</span> {t('bubble.unreview')}
        </button>
      {:else}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.review(x.id))}>
          <span class="ctxmenu-ic">👀</span> {t('bubble.markReviewed')}
        </button>
      {/if}

      {#if b.level === 'done'}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.reopen(x.id))}>
          <span class="ctxmenu-ic">♻️</span> {t('bubble.reopen')}
        </button>
      {:else}
        <button class="ctxmenu-item" role="menuitem" onclick={() => act((x) => api.close(x.id))}>
          <span class="ctxmenu-ic">🏆</span> {t('bubble.done')}
        </button>
      {/if}
    {/if}

    <div class="ctxmenu-sep"></div>
    <button class="ctxmenu-item" role="menuitem" onclick={copyId}>
      <span class="ctxmenu-ic">🔗</span> Copy id
    </button>
    {#if !store.kiosk}
      <button class="ctxmenu-item danger" role="menuitem" onclick={armDelete}>
        <span class="ctxmenu-ic">🗑</span> {t('bubble.delete')}
      </button>
    {/if}
  </div>
{/if}

{#if pendingDelete}
  <ConfirmDelete
    what={pendingDelete.name}
    detail={pendingDelete.threads > 0
      ? t('del.bubble', { n: pendingDelete.threads })
      : t('del.bubbleEmpty')}
    prefer={t('del.prefer')}
    busy={deleting}
    oncancel={() => (pendingDelete = null)}
    onconfirm={confirmDelete}
  />
{/if}
