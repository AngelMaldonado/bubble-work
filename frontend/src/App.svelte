<script lang="ts">
  import { onMount } from 'svelte';
  import { store } from './lib/store.svelte';
  import { tour } from './lib/tour.svelte';
  import AuthGate from './components/AuthGate.svelte';
  import Workspace from './components/Workspace.svelte';
  import ThreadView from './components/ThreadView.svelte';
  import PagesView from './components/PagesView.svelte';
  import GodMode from './components/GodMode.svelte';

  // The tour runs itself once, and only once the board is real.
  //
  // Gating on store.authed alone is not enough: it is seeded from token
  // PRESENCE, not validity, so an unreachable server leaves it optimistically
  // true — and the tour would open over a board that failed to load, whose
  // empty-workspace branch would helpfully suggest creating a bubble. store.actor
  // is only set once whoami has actually answered, so that is the real gate.
  $effect(() => {
    if (store.actor && !store.loading && !store.error && !store.threadId && !store.pagesWorkspace) {
      tour.maybeAutoStart();
    }
  });

  onMount(() => {
    void store.boot();
    store.syncFromLocation();
    const onNav = () => store.syncFromLocation();
    window.addEventListener('hashchange', onNav);
    window.addEventListener('popstate', onNav);
    return () => {
      window.removeEventListener('hashchange', onNav);
      window.removeEventListener('popstate', onNav);
      store.stopStream();
      store.stopSecondary();
    };
  });
</script>

{#if !store.authed}
  <AuthGate />
{:else if store.godView}
  <GodMode />
{:else if store.threadId}
  <ThreadView />
{:else if store.pagesWorkspace}
  <PagesView
    workspaceId={store.pagesWorkspace}
    workspaceName={store.workspaces.find((w) => w.id === store.pagesWorkspace)?.name ??
      store.pagesWorkspace}
    onclose={() => store.closePages()}
  />
{:else if store.loading && store.bubbles.length === 0}
  <div class="splash">
    <span class="orb"></span>
    <p>surfacing your bubbles…</p>
  </div>
{:else}
  <Workspace />
{/if}

<style>
  .splash {
    height: 100vh;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: 1rem;
    color: var(--muted);
  }
  .orb {
    width: 42px;
    height: 42px;
    border-radius: 999px;
    background: radial-gradient(circle at 35% 30%, white 0%, var(--wip) 55%, transparent 75%);
    animation: bob 2.4s ease-in-out infinite;
    filter: drop-shadow(0 8px 24px oklch(0.72 0.19 40 / 0.4));
  }
</style>
