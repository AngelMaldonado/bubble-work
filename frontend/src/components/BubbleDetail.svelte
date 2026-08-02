<script lang="ts">
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import type { ThreadNode } from '../lib/types';

  // store.detail is guaranteed non-null while this component is mounted.
  const bubble = $derived(store.detail!);

  let nodes = $state<ThreadNode[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // (re)load the timeline whenever the opened bubble changes.
  $effect(() => {
    const b = store.detail;
    if (!b) return;
    loading = true;
    error = null;
    api
      .timeline(b.id)
      .then((n) => (nodes = n))
      .catch((e) => (error = e instanceof ApiError ? e.message : String(e)))
      .finally(() => (loading = false));
  });

  function close() {
    store.closeDetail();
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }

  function age(iso: string): string {
    if (!iso) return '';
    const d = Date.now() - new Date(iso).getTime();
    const m = Math.floor(d / 60000);
    if (m < 1) return 'just now';
    if (m < 60) return `${m}m ago`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ago`;
    return `${Math.floor(h / 24)}d ago`;
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="scrim" onclick={close} role="presentation"></div>

<div class="panel" role="dialog" aria-modal="true" aria-label="{bubble.name} timeline">
  <header>
    <div class="htext">
      <h2>{bubble.name}</h2>
      <span class="sub">
        {bubble.instance}{bubble.project_name ? ' · ' + bubble.project_name : ''}
      </span>
    </div>
    <button class="x" onclick={close} aria-label="close">✕</button>
  </header>

  {#if loading}
    <p class="dim">Loading timeline…</p>
  {:else if error}
    <p class="err">{error}</p>
  {:else if nodes.length === 0}
    <p class="dim">No threads in this bubble yet.</p>
  {:else}
    <p class="count">{nodes.length} thread{nodes.length === 1 ? '' : 's'} · newest first</p>
    <ol class="rail">
      {#each nodes as n (n.id)}
        <li class="node" class:done={!n.active}>
          <span class="dot"></span>
          <div class="body">
            <div class="line1">
              <span class="seq">#{n.seq}</span>
              <span class="title">{n.title}</span>
            </div>
            <div class="line2">
              <span class="state">{n.active ? 'open' : 'done'}</span>
              {#if n.owner}<span class="meta">· {n.owner}</span>{/if}
              <span class="meta">· {age(n.created_at)}</span>
            </div>
          </div>
        </li>
      {/each}
    </ol>
  {/if}
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 50;
    background: color-mix(in oklab, var(--bg) 40%, transparent);
    backdrop-filter: blur(2px);
  }
  .panel {
    position: fixed;
    z-index: 51;
    top: 0;
    right: 0;
    height: 100vh;
    width: min(460px, 92vw);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 1.15rem 1.25rem 1.5rem;
    background: var(--surface-solid);
    border-left: 1px solid var(--line);
    box-shadow: -24px 0 60px var(--shadow-strong);
    overflow-y: auto;
    animation: slide 0.22s cubic-bezier(0.2, 0.8, 0.3, 1);
  }
  @keyframes slide {
    from {
      transform: translateX(24px);
      opacity: 0.4;
    }
  }
  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
  }
  .htext h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 750;
    letter-spacing: -0.01em;
    color: var(--text);
  }
  .sub {
    font-size: 0.74rem;
    color: var(--faint);
  }
  .x {
    flex: none;
    width: 28px;
    height: 28px;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.78rem;
  }
  .x:hover {
    color: var(--text);
    background: var(--hover);
  }
  .dim {
    color: var(--faint);
    font-size: 0.85rem;
  }
  .err {
    color: oklch(0.68 0.19 25);
    font-size: 0.82rem;
  }
  .count {
    margin: 0.1rem 0 0.35rem;
    font-size: 0.72rem;
    color: var(--faint);
  }

  /* the commit rail: a gradient spine with glowing nodes */
  .rail {
    list-style: none;
    margin: 0;
    padding: 0 0 0 0.35rem;
    position: relative;
  }
  .node {
    position: relative;
    padding: 0 0 1.15rem 1.5rem;
  }
  /* connecting line segment (skip below the last node) */
  .node::before {
    content: '';
    position: absolute;
    left: 5px;
    top: 0.55rem;
    bottom: -0.1rem;
    width: 2px;
    background: linear-gradient(
      to bottom,
      color-mix(in oklab, var(--wip) 55%, transparent),
      color-mix(in oklab, var(--wip) 18%, transparent)
    );
  }
  .node:last-child::before {
    display: none;
  }
  .dot {
    position: absolute;
    left: 0;
    top: 0.3rem;
    width: 12px;
    height: 12px;
    border-radius: 999px;
    background: var(--wip);
    box-shadow:
      0 0 0 3px color-mix(in oklab, var(--wip) 20%, transparent),
      0 0 10px color-mix(in oklab, var(--wip) 60%, transparent);
  }
  .node.done .dot {
    background: var(--done);
    box-shadow:
      0 0 0 3px color-mix(in oklab, var(--done) 20%, transparent),
      0 0 10px color-mix(in oklab, var(--done) 55%, transparent);
  }
  .body {
    display: grid;
    gap: 0.15rem;
  }
  .line1 {
    display: flex;
    align-items: baseline;
    gap: 0.45rem;
  }
  .seq {
    font-size: 0.7rem;
    font-weight: 700;
    color: var(--faint);
    font-variant-numeric: tabular-nums;
  }
  .title {
    font-size: 0.86rem;
    font-weight: 600;
    color: var(--text);
    line-height: 1.25;
  }
  .line2 {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    font-size: 0.72rem;
    color: var(--faint);
  }
  .state {
    color: var(--wip);
    font-weight: 600;
  }
  .node.done .state {
    color: var(--done);
  }
</style>
