<script lang="ts">
  import { levelIcon, levelLabel, rollup } from '../lib/types';
  import type { BubbleView } from '../lib/types';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import { bubbleMenu } from '../lib/contextmenu.svelte';

  let { bubble, index = 0 }: { bubble: BubbleView; index?: number } = $props();

  let pinned = $state(false);
  let busy = $state(false);

  const delay = $derived((index % 7) * 0.45);

  async function act(fn: () => Promise<unknown>) {
    busy = true;
    try {
      await fn();
      await store.refresh();
    } catch (e) {
      store.error = e instanceof ApiError ? e.message : String(e);
    } finally {
      busy = false;
      pinned = false;
    }
  }

  const isReviewed = $derived(bubble.level === 'reviewed');
  const isDone = $derived(bubble.level === 'done');
  // Everyone involved in the bubble (contract owner + every thread assignee),
  // owner first. Drives the avatar stack on the orb.
  const people = $derived.by<string[]>(() => {
    const ms = bubble.members ?? [];
    if (!bubble.owner) return ms;
    return [bubble.owner, ...ms.filter((m) => m !== bubble.owner)];
  });
  const MAX_AVATARS = 3;
  const shownPeople = $derived(people.slice(0, MAX_AVATARS));
  const extraPeople = $derived(Math.max(0, people.length - shownPeople.length));
</script>

<div class="wrap lvl-{bubble.level}" class:pinned style="--d: {delay}s" id="bw-{bubble.id}">
  <button
    class="orb"
    class:busy
    onclick={() => store.openDetail(bubble)}
    oncontextmenu={(e) => {
      e.preventDefault();
      e.stopPropagation(); // the bubble's menu wins over the board's
      bubbleMenu.show(bubble, e.clientX, e.clientY);
    }}
    aria-label="open {bubble.name}"
  >
    <span class="sheen"></span>
    {#if bubble.threads > 0}<span class="badge">{bubble.threads}</span>{/if}
    {#if people.length}
      <span class="people">
        {#each shownPeople as p, i (p)}
          <span
            class="person"
            class:owner={p === bubble.owner}
            style="--i: {i}"
            title={p === bubble.owner ? p + ' (owner)' : p}>{p[0]?.toUpperCase()}</span
          >
        {/each}
        {#if extraPeople}
          <span class="person more" style="--i: {shownPeople.length}" title={people.slice(MAX_AVATARS).join(', ')}
            >+{extraPeople}</span
          >
        {/if}
      </span>
    {/if}
  </button>

  <span class="caption">{bubble.name}</span>

  <div class="detail" role="dialog">
    <div class="dhead">
      <span class="dname">{bubble.name}</span>
      <span class="dinstance">{bubble.instance}</span>
    </div>
    <p class="why">{bubble.reason}</p>
    <div class="stats">
      <span>{bubble.threads} thread{bubble.threads === 1 ? '' : 's'}</span>
      <!-- how those threads are doing individually (THREAD-LIFECYCLE.md) -->
      {#each rollup(bubble.thread_levels) as [lv, n] (lv)}
        <span class="tally" title="{n} {levelLabel(lv)}">{levelIcon(lv)}{n}</span>
      {/each}
      {#if bubble.owner}<span title="owner">· 👤 {bubble.owner}</span>{/if}
      {#if people.length}
        <span class="dim" title="everyone involved">· {people.length} involved</span>
      {/if}
    </div>
    {#if bubble.outcome}<p class="outcome">🎯 {bubble.outcome}</p>{/if}

    <div class="acts">
      <button class="open" onclick={() => store.openDetail(bubble)}>🔎 open</button>
      {#if !isDone}
        {#if isReviewed}
          <button onclick={() => act(() => api.unreview(bubble.id))}>↩ un-review</button>
        {:else}
          <button onclick={() => act(() => api.review(bubble.id))}>👀 reviewed</button>
        {/if}
        <button onclick={() => act(() => api.close(bubble.id))}>🏆 done</button>
      {:else}
        <button onclick={() => act(() => api.reopen(bubble.id))}>↩ reopen</button>
      {/if}
    </div>
  </div>
</div>

<style>
  /* Per-band gradient palettes — vivid, opaque spheres (real contrast). */
  .lvl-in_progress {
    --g1: oklch(0.82 0.16 40);
    --g2: oklch(0.64 0.21 15);
    --g3: oklch(0.55 0.2 350);
    --glow: oklch(0.65 0.2 20);
  }
  .lvl-reviewed {
    --g1: oklch(0.86 0.11 200);
    --g2: oklch(0.68 0.15 225);
    --g3: oklch(0.58 0.16 255);
    --glow: oklch(0.68 0.15 225);
  }
  .lvl-zzzz {
    --g1: oklch(0.8 0.11 295);
    --g2: oklch(0.62 0.17 300);
    --g3: oklch(0.54 0.18 268);
    --glow: oklch(0.6 0.16 285);
  }
  .lvl-rip {
    --g1: oklch(0.74 0.02 265);
    --g2: oklch(0.56 0.02 265);
    --g3: oklch(0.44 0.02 265);
    --glow: oklch(0.5 0.02 265);
  }
  .lvl-done {
    --g1: oklch(0.92 0.12 98);
    --g2: oklch(0.81 0.16 85);
    --g3: oklch(0.68 0.15 68);
    --glow: oklch(0.8 0.16 88);
  }

  .wrap {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.55rem;
    width: 108px;
    padding-top: 0.4rem;
    /* bob lives on the wrap so it never fights the orb's hover scale */
    animation: bob 6.5s ease-in-out infinite;
    animation-delay: var(--d);
  }
  .wrap:hover {
    z-index: 6;
    animation-play-state: paused;
  }

  .orb {
    position: relative;
    width: 84px;
    height: 84px;
    border-radius: 50%;
    border: none;
    cursor: pointer;
    padding: 0;
    background:
      radial-gradient(circle at 70% 78%, var(--g3), transparent 60%),
      linear-gradient(150deg, var(--g1) 8%, var(--g2) 52%, var(--g3) 96%);
    box-shadow:
      inset -6px -8px 16px color-mix(in oklab, var(--g3) 70%, black 10%),
      inset 4px 5px 10px color-mix(in oklab, var(--g1) 60%, white 30%);
    transition:
      transform 0.32s cubic-bezier(0.34, 1.56, 0.4, 1),
      filter 0.32s ease;
  }
  /* the glossy catch-light */
  .orb .sheen {
    position: absolute;
    top: 12%;
    left: 16%;
    width: 42%;
    height: 34%;
    border-radius: 50%;
    background: radial-gradient(ellipse at 38% 34%, oklch(1 0 0 / 0.95), transparent 70%);
    filter: blur(1px);
    pointer-events: none;
  }
  .orb.busy {
    opacity: 0.55;
    pointer-events: none;
  }
  .wrap:hover .orb {
    transform: translateY(-8px) scale(1.2);
    filter: brightness(1.06) saturate(1.05);
  }
  .pinned .orb {
    transform: translateY(-6px) scale(1.12);
    box-shadow:
      inset -6px -8px 16px color-mix(in oklab, var(--g3) 70%, black 10%),
      inset 4px 5px 10px color-mix(in oklab, var(--g1) 60%, white 30%),
      0 0 0 3px color-mix(in oklab, var(--glow) 55%, transparent);
  }

  .badge {
    position: absolute;
    top: -3px;
    right: -3px;
    min-width: 20px;
    height: 20px;
    padding: 0 5px;
    border-radius: 999px;
    display: grid;
    place-content: center;
    font-size: 0.66rem;
    font-weight: 800;
    color: oklch(0.98 0 0);
    background: color-mix(in oklab, var(--g2) 80%, black 12%);
    box-shadow: 0 2px 6px color-mix(in oklab, var(--glow) 50%, transparent);
    border: 1.5px solid var(--surface-solid);
  }
  /* avatar stack of everyone involved, bottom-left of the orb */
  .people {
    position: absolute;
    bottom: -2px;
    left: -2px;
    display: flex;
  }
  .person {
    width: 22px;
    height: 22px;
    border-radius: 999px;
    display: grid;
    place-content: center;
    font-size: 0.6rem;
    font-weight: 800;
    border: 1.5px solid var(--surface-solid);
    margin-left: -8px;
    /* owner (first) sits on top, then each subsequent avatar behind */
    z-index: calc(20 - var(--i));
    /* default = assignee: softer than the owner */
    color: oklch(0.32 0.02 265);
    background: color-mix(in oklab, oklch(0.96 0.02 265) 82%, var(--surface-solid));
  }
  .person:first-child {
    margin-left: 0;
  }
  .person.owner {
    color: oklch(0.15 0.02 265);
    background: oklch(0.96 0.02 265);
  }
  .person.more {
    color: oklch(0.32 0.02 265);
    background: color-mix(in oklab, var(--text) 12%, var(--surface-solid));
    font-size: 0.58rem;
  }

  .caption {
    font-size: 0.76rem;
    font-weight: 600;
    line-height: 1.15;
    text-align: center;
    color: var(--text);
    max-width: 108px;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  /* hover / click reveals the detail card */
  .detail {
    position: absolute;
    z-index: 40;
    top: 100%;
    left: 50%;
    transform: translateX(-50%) translateY(6px);
    width: 236px;
    padding: 0.85rem 0.9rem;
    border-radius: 16px;
    background: var(--surface-solid);
    border: 1px solid color-mix(in oklab, var(--glow) 30%, var(--line));
    box-shadow: 0 20px 46px var(--shadow-strong);
    display: grid;
    gap: 0.4rem;
    opacity: 0;
    pointer-events: none;
    transition:
      opacity 0.16s ease,
      transform 0.16s ease;
  }
  /* transparent bridge so the cursor can cross into the card without a dead-zone */
  .detail::before {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    top: -12px;
    height: 12px;
  }
  .wrap:hover .detail,
  .pinned .detail {
    opacity: 1;
    transform: translateX(-50%) translateY(0);
    pointer-events: auto;
  }
  .dhead {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .dname {
    font-weight: 700;
    font-size: 0.86rem;
    color: var(--text);
  }
  .dinstance {
    font-size: 0.68rem;
    color: var(--faint);
    white-space: nowrap;
  }
  .why {
    margin: 0;
    font-size: 0.74rem;
    color: var(--muted);
    line-height: 1.35;
  }
  .stats {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    font-size: 0.72rem;
    color: var(--faint);
  }
  .tally {
    color: var(--muted);
  }
  .outcome {
    margin: 0;
    font-size: 0.72rem;
    color: var(--muted);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin-top: 0.15rem;
  }
  .acts button {
    flex: 1 1 auto;
    min-width: 0;
    padding: 0.4rem 0.4rem;
    border-radius: 9px;
    border: 1px solid var(--line);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    font-size: 0.74rem;
    white-space: nowrap;
  }
  .acts button:hover {
    background: var(--hover);
    border-color: color-mix(in oklab, var(--glow) 40%, var(--line));
  }
  /* primary action: its own full-width row so the pair below never overflows */
  .acts button.open {
    flex-basis: 100%;
    border-color: color-mix(in oklab, var(--glow) 45%, var(--line));
    color: var(--text);
    font-weight: 600;
  }

  @media (prefers-reduced-motion: reduce) {
    .wrap {
      animation: none;
    }
  }
</style>
