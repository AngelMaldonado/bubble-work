<script lang="ts">
  import { store } from '../lib/store.svelte';
  import { LEVELS } from '../lib/types';
  import type { BubbleView } from '../lib/types';
  import Band from './Band.svelte';
  import Omnibar from './Omnibar.svelte';
  import BirthForm from './BirthForm.svelte';

  let omni = $state(false);
  let birthTarget = $state<BubbleView | null>(null);

  function onKey(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      omni = true;
    }
  }

  function startBirth(b: BubbleView) {
    omni = false;
    birthTarget = b;
  }

  const unread = $derived(store.inbox?.unread_count ?? 0);
</script>

<svelte:window onkeydown={onKey} />

<div class="app">
  <header class="topbar">
    <div class="brand">
      <span class="orb"></span>
      <span class="wordmark">bubble.work</span>
    </div>

    <div class="right">
      {#if store.instances.length > 1}
        <select class="picker" bind:value={store.instance} aria-label="instance filter">
          <option value="">all instances</option>
          {#each store.instances as slug (slug)}
            <option value={slug}>{slug}</option>
          {/each}
        </select>
      {:else if store.instances.length === 1}
        <span class="chip">{store.instances[0]}</span>
      {/if}

      {#if unread > 0}
        <span class="chip alert" title="unread notices">✉ {unread}</span>
      {/if}

      <span class="who" title={store.actor?.email}>
        {store.actor?.name ?? store.actor?.email ?? '—'}
      </span>

      <button class="kbtn" onclick={() => (omni = true)}>⌘K</button>
    </div>
  </header>

  {#if store.error}
    <div class="banner">{store.error}</div>
  {/if}

  <main class="canvas">
    {#each LEVELS as lv (lv.key)}
      <Band
        level={lv.key}
        label={lv.label}
        icon={lv.icon}
        bubbles={store.byLevel(lv.key)}
        collapsible={lv.key === 'rip' || lv.key === 'done'}
        startCollapsed={lv.key === 'rip' || lv.key === 'done'}
      />
    {/each}

    {#if store.visible.length === 0}
      <div class="hollow">
        <p>No bubbles here yet.</p>
        <p class="dim">Press <b>⌘K</b>, type <b>&gt;</b>, and birth a thread — or create a bubble from the CLI.</p>
      </div>
    {/if}
  </main>

  <footer class="statusbar">
    <span>⌘K search · type <b>&gt;</b> for commands</span>
    <span class="dot" class:live={store.polling}>● {store.polling ? 'syncing' : 'polling'}</span>
  </footer>
</div>

<Omnibar bind:open={omni} onbirth={startBirth} />
{#if birthTarget}
  <BirthForm bubble={birthTarget} onclose={() => (birthTarget = null)} />
{/if}

<style>
  .app {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }
  .topbar {
    position: sticky;
    top: 0;
    z-index: 20;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.7rem 1.25rem;
    background: color-mix(in oklab, var(--bg) 70%, transparent);
    backdrop-filter: blur(14px);
    border-bottom: 1px solid var(--line);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .orb {
    width: 22px;
    height: 22px;
    border-radius: 999px;
    background: radial-gradient(circle at 35% 30%, white 0%, var(--wip) 55%, transparent 78%);
    animation: bob 3s ease-in-out infinite;
  }
  .wordmark {
    font-weight: 750;
    letter-spacing: -0.01em;
  }
  .right {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .picker,
  .chip,
  .who,
  .kbtn {
    font-size: 0.8rem;
  }
  .picker {
    background: var(--surface-solid);
    color: var(--text);
    border: 1px solid var(--line);
    border-radius: 9px;
    padding: 0.3rem 0.5rem;
  }
  .chip {
    color: var(--muted);
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 0.2rem 0.6rem;
  }
  .chip.alert {
    color: oklch(0.82 0.16 95);
    border-color: color-mix(in oklab, var(--done) 40%, transparent);
  }
  .who {
    color: var(--muted);
  }
  .kbtn {
    background: var(--surface-solid);
    color: var(--muted);
    border: 1px solid var(--line);
    border-radius: 9px;
    padding: 0.3rem 0.55rem;
    cursor: pointer;
  }
  .kbtn:hover {
    color: var(--text);
  }
  .banner {
    padding: 0.6rem 1.25rem;
    background: oklch(0.35 0.12 25 / 0.35);
    color: oklch(0.9 0.08 25);
    font-size: 0.82rem;
    border-bottom: 1px solid var(--line);
  }
  .canvas {
    flex: 1;
    width: min(1100px, 94vw);
    margin: 0 auto;
    padding: 1.25rem 0 4rem;
  }
  .hollow {
    text-align: center;
    color: var(--muted);
    margin-top: 3rem;
  }
  .hollow .dim {
    color: var(--faint);
    font-size: 0.85rem;
  }
  .hollow b {
    color: var(--text);
  }
  .statusbar {
    position: sticky;
    bottom: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 1.25rem;
    font-size: 0.72rem;
    color: var(--faint);
    background: color-mix(in oklab, var(--bg) 70%, transparent);
    backdrop-filter: blur(14px);
    border-top: 1px solid var(--line);
  }
  .statusbar b {
    color: var(--muted);
  }
  .dot {
    transition: color 0.3s ease;
  }
  .dot.live {
    color: oklch(0.8 0.16 145);
  }
</style>
