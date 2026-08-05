<script lang="ts">
  import { store } from '../lib/store.svelte';
  import { theme } from '../lib/theme.svelte';
  import { LEVELS } from '../lib/types';
  import type { BubbleView } from '../lib/types';
  import Band from './Band.svelte';
  import Minimap from './Minimap.svelte';
  import Omnibar from './Omnibar.svelte';
  import BirthForm from './BirthForm.svelte';
  import CreateBubbleForm from './CreateBubbleForm.svelte';
  import BubbleContextMenu from './BubbleContextMenu.svelte';
  import BoardContextMenu from './BoardContextMenu.svelte';
  import BubbleDetail from './BubbleDetail.svelte';
  import ProjectCombobox from './ProjectCombobox.svelte';
  import ViewCombobox from './ViewCombobox.svelte';
  import { boardMenu } from '../lib/contextmenu.svelte';

  let omni = $state(false);
  let birthTarget = $state<BubbleView | null>(null);
  let showCreate = $state(false);

  // a kiosk display is a passive read-only screen: no ⌘K, no commands, no birth.
  const kiosk = $derived(store.kiosk);

  function onKey(e: KeyboardEvent) {
    if (kiosk) return;
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

  // auto-dismiss transient godmode/info notices
  $effect(() => {
    if (store.flash) {
      const t = setTimeout(() => (store.flash = null), 4500);
      return () => clearTimeout(t);
    }
  });

  // "3m" reads better than "180 seconds" in a banner someone glances at.
  function fmtBehind(sec: number): string {
    if (sec < 90) return `${Math.max(1, Math.round(sec))}s`;
    if (sec < 5400) return `${Math.round(sec / 60)}m`;
    return `${Math.round(sec / 3600)}h`;
  }
</script>

<svelte:window onkeydown={onKey} />

<div
  class="app"
  oncontextmenu={(e) => {
    // The board's own menu. A bubble stops propagation, so its menu wins.
    e.preventDefault();
    boardMenu.show(e.clientX, e.clientY);
  }}
  role="presentation"
>
  <!-- the one fixed floating bubble, top-left; doubles as the ⌘K launcher
       (a kiosk display is passive, so it's just a mark there) -->
  {#if kiosk}
    <div class="brand" aria-label="bubble.work kiosk">
      <span class="orb"></span>
      <span class="wordmark">bubble.work</span>
      <span class="kiosk-tag">kiosk</span>
    </div>
  {:else}
    <button class="brand" onclick={() => (omni = true)} title="search · commands (⌘K)">
      <span class="orb"></span>
      <span class="wordmark">bubble.work</span>
    </button>
  {/if}

  {#if store.status?.stale}
    <!-- The board renders from a local mirror, so a stopped sync would leave it
         looking perfectly healthy while quietly ageing. Being behind is fine;
         being behind silently is not (PLANE-SYNC.md Phase 7). -->
    <div class="banner stale" role="status">
      ⧗ {store.status.reason || 'Plane sync is behind — this may be older data'}
      {#each store.status.instances ?? [] as i (i.instance)}
        {#if i.stale}
          <span class="stale-inst">
            {i.instance}: {i.last_ok ? `last synced ${fmtBehind(i.behind_seconds ?? 0)} ago` : 'never synced'}
          </span>
        {/if}
      {/each}
    </div>
  {/if}
  {#if store.status?.unsent_drafts}
    <div class="banner unsent" role="status">
      ⧗ {store.status.unsent_drafts} unsent comment{store.status.unsent_drafts === 1 ? '' : 's'} —
      open the thread to retry or discard.
    </div>
  {/if}
  {#if store.error}
    <div class="banner">{store.error}</div>
  {/if}
  {#if store.flash}
    <div class="flash" role="status">{store.flash}</div>
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
        <p class="dim">Press <b>⌘K</b>, type <b>&gt;</b>, and choose <b>new bubble</b> — then birth threads into it.</p>
      </div>
    {/if}
  </main>

  <footer class="statusbar">
    <div class="side">
      {#if store.instances.length > 1}
        <select
          class="picker"
          value={store.instance}
          onchange={(e) => store.selectInstance(e.currentTarget.value)}
          aria-label="instance filter"
        >
          <option value="">all instances</option>
          {#each store.instances as slug (slug)}
            <option value={slug}>{slug}</option>
          {/each}
        </select>
      {:else if store.instances.length === 1}
        <span class="chip">{store.instances[0]}</span>
      {/if}

      {#if store.projects.length > 1}
        <span class="projwrap"><ProjectCombobox /></span>
      {/if}

      <!-- view scope: Everyone · Mine · <assignee> (Phase 9), same combobox as
           the project filter. "Mine" is hidden on a kiosk (no personal identity). -->
      <span class="projwrap"><ViewCombobox /></span>

      <span class="who" title={store.actor?.email}>
        {store.actor?.name ?? store.actor?.email ?? '—'}
      </span>

      {#if unread > 0}
        <span class="chip alert" title="unread notices">✉ {unread}</span>
      {/if}

      {#if store.godmode}
        <button
          class="god-enter"
          onclick={() => store.openGodMode()}
          title="open God Mode (/god-mode)"
        >
          ⚡ God Mode{store.allOrgs ? ' · all orgs' : ''}
        </button>
      {/if}
    </div>

    <div class="side right">
      {#if kiosk}
        <span class="hint">read-only display</span>
        <button class="theme" onclick={() => store.exitKiosk()} title="exit kiosk mode">
          <span class="tico">⎋</span>
          <span class="tlabel">exit kiosk</span>
        </button>
      {:else}
        <span class="hint">⌘K search · <b>&gt;</b> commands</span>
      {/if}
      <button
        class="theme"
        onclick={() => theme.cycle()}
        title="theme: {themeLabel} (click to change)"
      >
        <span class="tico">{theme.icon}</span>
        <span class="tlabel">{themeLabel}</span>
      </button>
      <span
        class="state"
        class:live={!store.error}
        class:err={!!store.error}
        title={store.error ? 'cannot reach the server' : store.polling ? 'syncing…' : 'connected'}
      >
        <span class="pip"></span>
        {store.error ? 'offline' : store.polling ? 'syncing' : 'live'}
      </span>
    </div>
  </footer>
</div>

<Minimap />
{#if !kiosk}
  <Omnibar
    bind:open={omni}
    onbirth={startBirth}
    onnewbubble={() => {
      omni = false;
      showCreate = true;
    }}
  />
  {#if birthTarget}
    <BirthForm bubble={birthTarget} onclose={() => (birthTarget = null)} />
  {/if}
  {#if showCreate}
    <CreateBubbleForm onclose={() => (showCreate = false)} />
  {/if}
{/if}
{#if store.detail}
  <BubbleDetail />
{/if}

<BubbleContextMenu onbirth={startBirth} />
<BoardContextMenu
  onnewbubble={() => (showCreate = true)}
  onsearch={() => (omni = true)}
/>

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
  .kiosk-tag {
    font-size: 0.62rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--wip);
    border: 1px solid color-mix(in oklab, var(--wip) 40%, transparent);
    border-radius: 999px;
    padding: 0.05rem 0.4rem;
  }

  /* stale: the board is real but ageing. Distinct from .banner (an error),
     because "slightly behind" and "broken" deserve different alarm. */
  .banner.stale,
  .banner.unsent {
    background: var(--warn-bg, rgba(217, 119, 6, 0.12));
    color: var(--warn, #b45309);
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: baseline;
  }
  .stale-inst {
    font-size: 0.72rem;
    opacity: 0.85;
    font-variant-numeric: tabular-nums;
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
  .projwrap {
    display: inline-flex;
    flex: 0 0 auto;
    width: 9.5rem;
    max-width: 28vw;
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
  .god-enter {
    color: var(--wip);
    border: 1px solid color-mix(in oklab, var(--wip) 45%, transparent);
    border-radius: 999px;
    padding: 0.15rem 0.6rem;
    background: transparent;
    font-weight: 700;
    font-size: inherit;
    cursor: pointer;
  }
  .god-enter:hover {
    background: color-mix(in oklab, var(--wip) 16%, transparent);
  }
  .flash {
    padding: 0.55rem 1.25rem;
    padding-top: 4.2rem;
    background: color-mix(in oklab, var(--reviewed) 18%, transparent);
    color: var(--text);
    font-size: 0.82rem;
  }
  .who {
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 0 0 auto; /* never collapse to zero — always show the name */
    max-width: 16rem;
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
  .state {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--faint);
    transition: color 0.3s ease;
  }
  .state .pip {
    width: 7px;
    height: 7px;
    border-radius: 999px;
    background: currentColor;
    box-shadow: 0 0 0 0 currentColor;
  }
  .state.live {
    color: oklch(0.7 0.16 145);
  }
  .state.live .pip {
    animation: pulse 2.4s ease-in-out infinite;
  }
  .state.err {
    color: oklch(0.68 0.19 25);
  }
  @keyframes pulse {
    0%,
    100% {
      box-shadow: 0 0 0 0 color-mix(in oklab, currentColor 60%, transparent);
    }
    50% {
      box-shadow: 0 0 0 4px transparent;
    }
  }

  @media (max-width: 560px) {
    .hint,
    .tlabel {
      display: none;
    }
  }
</style>
