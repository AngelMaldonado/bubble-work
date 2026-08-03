<script lang="ts">
  import type { BubbleView, Level } from '../lib/types';
  import BubbleCard from './BubbleCard.svelte';
  import { store } from '../lib/store.svelte';

  let {
    level,
    label,
    icon,
    bubbles,
    collapsible = false,
    startCollapsed = false,
  }: {
    level: Level;
    label: string;
    icon: string;
    bubbles: BubbleView[];
    collapsible?: boolean;
    startCollapsed?: boolean;
  } = $props();

  // Deliberately a one-shot seed: startCollapsed decides how the band OPENS,
  // and from then on it is the reader's toggle to own.
  // svelte-ignore state_referenced_locally
  let expanded = $state(!startCollapsed);

  const isFocus = $derived(level === 'in_progress');
</script>

<section class="band lvl-{level}" class:empty={bubbles.length === 0}>
  <header>
    <button
      class="title"
      onclick={() => collapsible && (expanded = !expanded)}
      class:static={!collapsible}
    >
      <span class="icon">{icon}</span>
      <span class="label">{label}</span>
      <span class="count">{bubbles.length}</span>
      {#if collapsible}<span class="chev">{expanded ? '▾' : '▸'}</span>{/if}
    </button>

    {#if isFocus}
      <span class="focus" class:warn={store.overFocus}>
        {store.focusCount} / {store.focusCap} focus{store.overFocus ? ' ⚠' : ''}
      </span>
    {/if}
  </header>

  {#if expanded}
    {#if bubbles.length === 0}
      <p class="none">— nothing here —</p>
    {:else}
      <div class="cards">
        {#each bubbles as b, i (b.id)}
          <BubbleCard bubble={b} index={i} />
        {/each}
      </div>
    {/if}
  {/if}
</section>

<style>
  .band {
    padding: 0.5rem 0 1rem;
    border-bottom: 1px solid color-mix(in oklab, white 6%, transparent);
  }
  .band.empty {
    opacity: 0.72;
  }
  header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.75rem;
  }
  .title {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: transparent;
    border: none;
    color: var(--text);
    cursor: pointer;
    padding: 0.2rem 0;
  }
  .title.static {
    cursor: default;
  }
  .icon {
    font-size: 1.05rem;
  }
  .label {
    font-weight: 750;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    font-size: 0.82rem;
    color: var(--muted);
  }
  .count {
    font-size: 0.72rem;
    color: var(--muted);
    background: var(--hover);
    padding: 0.05rem 0.45rem;
    border-radius: 999px;
  }
  .chev {
    color: var(--faint);
    font-size: 0.75rem;
  }
  .focus {
    margin-left: auto;
    font-size: 0.75rem;
    color: var(--faint);
  }
  .focus.warn {
    color: oklch(0.78 0.17 60);
    font-weight: 600;
  }
  .cards {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 0.9rem;
  }
  .none {
    margin: 0;
    color: var(--faint);
    font-size: 0.8rem;
    font-style: italic;
  }
</style>
