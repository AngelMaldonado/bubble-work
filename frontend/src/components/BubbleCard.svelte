<script lang="ts">
  import type { BubbleView } from '../lib/types';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';

  let { bubble, index = 0 }: { bubble: BubbleView; index?: number } = $props();

  let open = $state(false);
  let busy = $state(false);

  const delay = $derived((index % 6) * 0.4);

  async function act(fn: () => Promise<unknown>) {
    busy = true;
    try {
      await fn();
      await store.refresh();
    } catch (e) {
      store.error = e instanceof ApiError ? e.message : String(e);
    } finally {
      busy = false;
      open = false;
    }
  }

  const isReviewed = $derived(bubble.level === 'reviewed');
  const isDone = $derived(bubble.level === 'done');
</script>

<div class="wrap" style="--d: {delay}s">
  <button
    class="bubble lvl-{bubble.level}"
    class:busy
    onclick={() => (open = !open)}
    title={bubble.reason}
  >
    <span class="name">{bubble.name}</span>
    <span class="meta">
      {#if bubble.threads > 0}
        <span class="threads">{bubble.threads} thread{bubble.threads === 1 ? '' : 's'}</span>
      {:else}
        <span class="threads faint">no threads</span>
      {/if}
      {#if bubble.owner}<span class="owner" title={bubble.owner}>{bubble.owner[0]?.toUpperCase()}</span>{/if}
    </span>
  </button>

  {#if open}
    <div class="menu" role="menu">
      <div class="why">{bubble.reason}</div>
      {#if !isDone}
        {#if isReviewed}
          <button onclick={() => act(() => api.unreview(bubble.id))}>↩ un-review</button>
        {:else}
          <button onclick={() => act(() => api.review(bubble.id))}>👀 mark reviewed</button>
        {/if}
        <button onclick={() => act(() => api.close(bubble.id))}>🏆 close (done)</button>
      {:else}
        <button onclick={() => act(() => api.reopen(bubble.id))}>↩ reopen</button>
      {/if}
      <button class="ghost" onclick={() => (open = false)}>close menu</button>
    </div>
  {/if}
</div>

<style>
  .wrap {
    position: relative;
  }
  .bubble {
    position: relative;
    min-width: 150px;
    max-width: 230px;
    text-align: left;
    padding: 0.85rem 1rem;
    border-radius: 18px;
    cursor: pointer;
    color: var(--text);
    background: var(--surface);
    border: 1px solid var(--line);
    backdrop-filter: blur(10px);
    box-shadow: 0 10px 30px oklch(0.1 0.03 265 / 0.4);
    animation: bob 6s ease-in-out infinite;
    animation-delay: var(--d);
    display: grid;
    gap: 0.5rem;
    transition:
      transform 0.15s ease,
      box-shadow 0.15s ease;
  }
  .bubble:hover {
    transform: translateY(-3px) scale(1.02);
    box-shadow: 0 16px 40px oklch(0.1 0.03 265 / 0.55);
  }
  .bubble.busy {
    opacity: 0.6;
    pointer-events: none;
  }
  /* per-band left glow */
  .lvl-in_progress {
    border-left: 3px solid var(--wip);
  }
  .lvl-reviewed {
    border-left: 3px solid var(--reviewed);
  }
  .lvl-zzzz {
    border-left: 3px solid var(--zzzz);
    opacity: 0.9;
  }
  .lvl-rip {
    border-left: 3px solid var(--rip);
    opacity: 0.78;
  }
  .lvl-done {
    border-left: 3px solid var(--done);
  }
  .name {
    font-weight: 650;
    font-size: 0.92rem;
    line-height: 1.2;
  }
  .meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.72rem;
    color: var(--muted);
  }
  .faint {
    color: var(--faint);
  }
  .owner {
    width: 20px;
    height: 20px;
    border-radius: 999px;
    display: grid;
    place-content: center;
    font-size: 0.65rem;
    font-weight: 700;
    color: oklch(0.15 0.02 265);
    background: var(--reviewed);
  }
  .menu {
    position: absolute;
    z-index: 30;
    top: calc(100% + 6px);
    left: 0;
    min-width: 200px;
    padding: 0.4rem;
    border-radius: 14px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 20px 50px oklch(0.08 0.03 265 / 0.6);
    display: grid;
    gap: 0.15rem;
  }
  .why {
    padding: 0.4rem 0.5rem;
    font-size: 0.72rem;
    color: var(--faint);
    border-bottom: 1px solid var(--line);
    margin-bottom: 0.2rem;
  }
  .menu button {
    text-align: left;
    padding: 0.5rem 0.55rem;
    border-radius: 9px;
    border: none;
    background: transparent;
    color: var(--text);
    cursor: pointer;
    font-size: 0.82rem;
  }
  .menu button:hover {
    background: oklch(0.32 0.03 265 / 0.7);
  }
  .menu .ghost {
    color: var(--faint);
    font-size: 0.75rem;
  }
</style>
