<script lang="ts">
  import { boardMenu } from '../lib/contextmenu.svelte';
  import { store } from '../lib/store.svelte';
  import { theme } from '../lib/theme.svelte';
  import { LEVELS } from '../lib/types';
  import { ApiError } from '../lib/api';

  // Both open forms are owned by Workspace, so the menu just asks for them.
  let { onnewbubble, onsearch }: { onnewbubble: () => void; onsearch: () => void } = $props();

  const MENU_W = 230;
  const MENU_H = 330;
  const px = $derived(Math.max(8, Math.min(boardMenu.x, window.innerWidth - MENU_W - 8)));
  const py = $derived(Math.max(8, Math.min(boardMenu.y, window.innerHeight - MENU_H - 8)));

  const kiosk = $derived(store.kiosk);
  // Only the bands that actually hold something — a jump target for an empty
  // band is just noise.
  const bands = $derived(LEVELS.map((l) => ({ ...l, n: store.byLevel(l.key).length })).filter((b) => b.n > 0));
  const mine = $derived(store.scope === 'mine');

  let busy = $state(false);

  function pick(fn: () => void): void {
    boardMenu.hide();
    fn();
  }

  function goBand(level: string): void {
    boardMenu.hide();
    document.getElementById('band-' + level)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  async function refresh(): Promise<void> {
    boardMenu.hide();
    if (busy) return;
    busy = true;
    try {
      await store.refresh();
      store.flash = 'board refreshed';
    } catch (e) {
      store.error = e instanceof ApiError ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape') boardMenu.hide();
  }
</script>

<svelte:window onkeydown={onKey} onresize={() => boardMenu.hide()} onscroll={() => boardMenu.hide()} />

{#if boardMenu.open}
  <div
    class="ctxmenu-scrim"
    role="presentation"
    onclick={() => boardMenu.hide()}
    oncontextmenu={(e) => {
      e.preventDefault();
      boardMenu.hide();
    }}
  ></div>

  <div class="ctxmenu" style="left: {px}px; top: {py}px" role="menu">
    {#if bands.length}
      <div class="ctxmenu-head">Jump to band</div>
      <div class="jump">
        {#each bands as b (b.key)}
          <button class="band" onclick={() => goBand(b.key)} title="{b.n} {b.label}">
            <span class="bico">{b.icon}</span>
            <span class="bn">{b.n}</span>
          </button>
        {/each}
      </div>
      <div class="ctxmenu-sep"></div>
    {/if}

    {#if !kiosk}
      <button class="ctxmenu-item" role="menuitem" onclick={() => pick(onnewbubble)}>
        <span class="ctxmenu-ic">🫧</span> New bubble…
      </button>
      <button class="ctxmenu-item" role="menuitem" onclick={() => pick(onsearch)}>
        <span class="ctxmenu-ic">🔍</span> Search &amp; commands
        <span class="ctxmenu-hint">⌘K</span>
      </button>
      <div class="ctxmenu-sep"></div>
    {/if}

    <button class="ctxmenu-item" role="menuitem" onclick={refresh}>
      <span class="ctxmenu-ic">↻</span> Refresh now
      <span class="ctxmenu-hint">{busy ? '…' : ''}</span>
    </button>

    {#if !kiosk}
      <button
        class="ctxmenu-item"
        role="menuitem"
        onclick={() => pick(() => store.setScope(mine ? 'workspace' : 'mine'))}
      >
        <span class="ctxmenu-ic">{mine ? '👥' : '👤'}</span>
        {mine ? 'Show everyone' : 'Only my bubbles'}
      </button>
    {/if}

    <button class="ctxmenu-item" role="menuitem" onclick={() => pick(() => theme.cycle())}>
      <span class="ctxmenu-ic">{theme.icon}</span> Theme
      <span class="ctxmenu-hint">{theme.choice}</span>
    </button>

    {#if store.godmode || kiosk}
      <div class="ctxmenu-sep"></div>
    {/if}
    {#if store.godmode}
      <button class="ctxmenu-item" role="menuitem" onclick={() => pick(() => store.openGodMode())}>
        <span class="ctxmenu-ic">⚡</span> God Mode
      </button>
    {/if}
    {#if kiosk}
      <button class="ctxmenu-item" role="menuitem" onclick={() => pick(() => store.exitKiosk())}>
        <span class="ctxmenu-ic">⎋</span> Exit kiosk
      </button>
    {/if}
  </div>
{/if}

<style>
  /* the band jump strip: one compact row instead of five menu rows */
  .jump {
    display: flex;
    gap: 2px;
    padding: 0 0.15rem 0.15rem;
  }
  .band {
    flex: 1;
    display: grid;
    justify-items: center;
    gap: 0.1rem;
    padding: 0.35rem 0.2rem;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    font-family: inherit;
  }
  .band:hover {
    background: var(--hover);
    color: var(--text);
  }
  .bico {
    font-size: 0.95rem;
    line-height: 1;
  }
  .bn {
    font-size: 0.68rem;
    font-weight: 700;
  }
</style>
