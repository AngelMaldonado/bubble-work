<script lang="ts">
  // What you can do to a WORKSPACE — the boundary for a body of work, which is a
  // Plane project (AGENTS.md), not a Plane workspace.
  //
  // Rename and delete act on the workspace the board is currently scoped to. With
  // "All projects" selected there is no single subject, so they say so and stay
  // disabled rather than guessing which one you meant.
  import { workspaceMenu } from '../lib/contextmenu.svelte';
  import { store } from '../lib/store.svelte';
  import { t } from '../lib/i18n.svelte';

  let {
    onrename,
    oncreate,
    ondelete,
    ondocs,
  }: {
    onrename: () => void;
    oncreate: () => void;
    ondelete: () => void;
    ondocs: () => void;
  } = $props();

  const MENU_W = 232;
  const px = $derived(Math.max(8, Math.min(workspaceMenu.x, window.innerWidth - MENU_W - 8)));

  // Measured, not guessed: the menu's height changes with the language and with
  // whether the "pick one first" hint is showing, and being wrong by a row puts
  // it back off the bottom edge.
  let box = $state<HTMLElement | null>(null);
  let h = $state(0);
  $effect(() => {
    if (workspaceMenu.open && box) h = box.offsetHeight;
  });

  // Open upward when there is no room below — the handle lives in the status bar
  // at the bottom of the window, so upward is in fact the normal case.
  const py = $derived.by(() => {
    const below = workspaceMenu.bottom + 6;
    if (h > 0 && below + h + 8 > window.innerHeight) {
      return Math.max(8, workspaceMenu.top - h - 6);
    }
    return below;
  });

  // The workspace being acted on: the filtered one, or the only one there is.
  const current = $derived(store.activeProject);

  function pick(fn: () => void): void {
    workspaceMenu.hide();
    fn();
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape') workspaceMenu.hide();
  }
</script>

<svelte:window
  onkeydown={onKey}
  onresize={() => workspaceMenu.hide()}
  onscroll={() => workspaceMenu.hide()}
/>

{#if workspaceMenu.open}
  <div
    class="ctxmenu-scrim"
    role="presentation"
    onclick={() => workspaceMenu.hide()}
    oncontextmenu={(e) => {
      e.preventDefault();
      workspaceMenu.hide();
    }}
  ></div>

  <div
    class="ctxmenu"
    bind:this={box}
    style="left: {px}px; top: {py}px; visibility: {h > 0 ? 'visible' : 'hidden'}"
    role="menu"
  >
    <div class="ctxmenu-head">
      {current ? current.name : t('ws.noneScoped')}
    </div>

    <button class="ctxmenu-item" role="menuitem" disabled={!current} onclick={() => pick(ondocs)}>
      <span class="ctxmenu-ic">📄</span> {t('pages.title')}
    </button>

    <div class="ctxmenu-sep"></div>

    <button
      class="ctxmenu-item"
      role="menuitem"
      disabled={!current}
      onclick={() => pick(onrename)}
    >
      <span class="ctxmenu-ic">✏️</span> {t('ws.rename')}
    </button>

    <button class="ctxmenu-item" role="menuitem" onclick={() => pick(oncreate)}>
      <span class="ctxmenu-ic">➕</span> {t('ws.new')}
    </button>

    <div class="ctxmenu-sep"></div>

    <button
      class="ctxmenu-item danger"
      role="menuitem"
      disabled={!current}
      onclick={() => pick(ondelete)}
    >
      <span class="ctxmenu-ic">🗑</span> {t('ws.delete')}
    </button>

    {#if !current}
      <p class="hint">{t('ws.pickFirst')}</p>
    {/if}
  </div>
{/if}

<style>
  .ctxmenu-item:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .ctxmenu-item:disabled:hover {
    background: transparent;
  }
  .hint {
    margin: 0.1rem 0.15rem 0.15rem;
    padding: 0 0.55rem;
    font-size: 0.68rem;
    line-height: 1.4;
    color: var(--faint);
  }
</style>
