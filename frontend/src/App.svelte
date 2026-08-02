<script lang="ts">
  import { onMount } from 'svelte';
  import { store } from './lib/store.svelte';
  import AuthGate from './components/AuthGate.svelte';
  import Workspace from './components/Workspace.svelte';

  onMount(() => {
    void store.boot();
    return () => {
      store.stopStream();
      store.stopSecondary();
    };
  });
</script>

{#if !store.authed}
  <AuthGate />
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
