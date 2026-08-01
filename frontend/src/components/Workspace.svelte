<script lang="ts">
  import { store } from '../lib/store.svelte';
  import { theme } from '../lib/theme.svelte';
  import { LEVELS } from '../lib/types';
  import type { BubbleView } from '../lib/types';
  import Band from './Band.svelte';
  import Minimap from './Minimap.svelte';
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
  const themeLabel = $derived(
    theme.choice === 'system' ? 'system' : theme.choice === 'dark' ? 'dark' : 'light',
  );
</script>

<svelte:window onkeydown={onKey} />

<div class="app">
  <!-- the one fixed floating bubble, top-left; doubles as the ⌘K launcher -->
  <button class="brand" onclick={() => (omni = true)} title="search · commands (⌘K)">
    <span class="orb"></span>
    <span class="wordmark">bubble.work</span>
  </button>

  {#if store.error}
    <div class="banner">{store.error}</div>
  {/if}

  <main class="canvas">
    {#each LEVELS as lv (lv.key)}
      <div id="band-{lv.key}" class="band-anchor">
        <Band
          level={lv.key}
          label={lv.label}
          icon={lv.icon}
          bubbles={store.byLevel(lv.key)}
          collapsible={lv.key === 'rip' || lv.key === 'done'}
          startCollapsed={false}
        />
      </div>
    {/each}

    {#if store.visible.length === 0}
      <div class="hollow">
        <p>No bubbles here yet.</p>
        <p class="dim">Press <b>⌘K</b>, type <b>&gt;</b>, and birth a thread — or create a bubble from the CLI.</p>
      </div>
    {/if}
  </main>

  <footer class="statusbar">
    <div class="side">
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

      <span class="who" title={store.actor?.email}>
        {store.actor?.name ?? store.actor?.email ?? '—'}
      </span>

      {#if unread > 0}
        <span class="chip alert" title="unread notices">✉ {unread}</span>
      {/if}
    </div>

    <div class="side right">
      <span class="hint">⌘K search · <b>&gt;</b> commands</span>
      <button
        class="theme"
        onclick={() => theme.cycle()}
        title="theme: {themeLabel} (click to change)"
      >
        <span class="tico">{theme.icon}</span>
        <span class="tlabel">{themeLabel}</span>
      </button>
      <span class="dot" class:live={store.polling} title={store.polling ? 'syncing' : 'polling'}
        >●</span
      >
    </div>
  </footer>
</div>

<Minimap />
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
  /* fixed floating brand bubble */
  .brand {
    position: fixed;
    top: 1rem;
    left: 1.15rem;
    z-index: 30;
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.35rem 0.75rem 0.35rem 0.4rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface);
    backdrop-filter: blur(12px);
    box-shadow: 0 8px 24px var(--shadow);
    color: var(--text);
    cursor: pointer;
    transition:
      transform 0.15s ease,
      box-shadow 0.15s ease;
  }
  .brand:hover {
    transform: translateY(-1px);
    box-shadow: 0 12px 30px var(--shadow-strong);
  }
  .orb {
    width: 24px;
    height: 24px;
    border-radius: 999px;
    flex: none;
    background:
      radial-gradient(circle at 34% 30%, var(--sheen), transparent 42%),
      radial-gradient(circle at 65% 80%, color-mix(in oklab, var(--wip) 60%, transparent), transparent 70%),
      var(--wip);
    box-shadow: inset 0 1px 2px var(--sheen);
    animation: bob 3s ease-in-out infinite;
  }
  .wordmark {
    font-weight: 750;
    letter-spacing: -0.01em;
    font-size: 0.88rem;
  }

  .banner {
    margin: 0 0 0 0;
    padding: 0.6rem 1.25rem;
    padding-top: 4.2rem;
    background: color-mix(in oklab, oklch(0.6 0.18 25) 22%, transparent);
    color: oklch(0.45 0.16 25);
    font-size: 0.82rem;
  }
  :root[data-mode='dark'] .banner {
    color: oklch(0.9 0.08 25);
  }

  .canvas {
    flex: 1;
    width: min(1100px, 94vw);
    margin: 0 auto;
    padding: 4.5rem 3.25rem 5rem 0.75rem;
  }
  .band-anchor {
    scroll-margin-top: 5rem;
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
    z-index: 25;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.55rem 1.25rem;
    font-size: 0.75rem;
    color: var(--faint);
    background: color-mix(in oklab, var(--bg) 72%, transparent);
    backdrop-filter: blur(14px);
    border-top: 1px solid var(--line);
  }
  .side {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    min-width: 0;
  }
  .right {
    justify-content: flex-end;
  }
  .picker {
    background: var(--surface-solid);
    color: var(--text);
    border: 1px solid var(--line);
    border-radius: 9px;
    padding: 0.25rem 0.45rem;
    font-size: 0.75rem;
  }
  .chip {
    color: var(--muted);
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 0.15rem 0.55rem;
  }
  .chip.alert {
    color: var(--done);
    border-color: color-mix(in oklab, var(--done) 40%, transparent);
  }
  .who {
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .hint b {
    color: var(--muted);
  }
  .theme {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    background: var(--surface-solid);
    color: var(--muted);
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 0.2rem 0.55rem 0.2rem 0.45rem;
    cursor: pointer;
  }
  .theme:hover {
    color: var(--text);
  }
  .tico {
    font-size: 0.85rem;
    line-height: 1;
  }
  .tlabel {
    font-size: 0.72rem;
  }
  .dot {
    color: var(--faint);
    transition: color 0.3s ease;
  }
  .dot.live {
    color: oklch(0.7 0.16 145);
  }

  @media (max-width: 560px) {
    .hint,
    .tlabel {
      display: none;
    }
  }
</style>
