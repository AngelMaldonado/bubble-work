<script lang="ts">
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import { levelIcon, levelLabel, rollup } from '../lib/types';
  import { t } from '../lib/i18n.svelte';
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

  // ---- renaming ----
  //
  // A bubble's name is the Plane module's `name`, edited in place here for the
  // same reason a thread's title is edited in ThreadView: it is a property of the
  // object, not content inside it, so it has its own path and no editor.
  //
  // Renaming is not production — a bubble IS its outcome (§4) — so nothing warms.
  let nameEl = $state<HTMLElement | null>(null);
  let nameWas = $state('');

  // Opened from the context menu's Rename: land in the field with the name
  // selected, so the menu item does what it says rather than merely showing you
  // where renaming happens. Cleared immediately — it is a one-shot intention.
  $effect(() => {
    if (!store.detailRename) return;
    const el = nameEl;
    if (!el) return;
    el.focus();
    getSelection()?.selectAllChildren(el);
    store.detailRename = false;
  });

  function onNameKey(e: KeyboardEvent): void {
    // Stops the panel's own Escape-to-close from firing while editing: leaving
    // the field and closing the panel are different intentions.
    e.stopPropagation();
    if (e.key === 'Enter') {
      e.preventDefault();
      nameEl?.blur(); // commits
    } else if (e.key === 'Escape') {
      e.preventDefault();
      if (nameEl) nameEl.textContent = nameWas;
      nameEl?.blur();
    }
  }

  async function commitName(): Promise<void> {
    const el = nameEl;
    if (!el) return;
    const next = (el.textContent ?? '').replace(/\s+/g, ' ').trim();
    if (!next) {
      el.textContent = nameWas; // an empty name is a slip, not a rename
      return;
    }
    if (next === bubble.name) return;
    try {
      await api.renameBubble(bubble.id, next);
      // The board owns the bubble list, so it has to hear about this — the panel
      // is a view onto store.detail, not the owner of the name.
      await store.refresh();
    } catch (e) {
      el.textContent = nameWas;
      error = e instanceof ApiError ? e.message : String(e);
    }
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
      <!-- The name is the Plane module's NAME, a property of the bubble rather
           than content in it, so it is renamed on its own path. -->
      {#if store.kiosk}
        <h2>{bubble.name}</h2>
      {:else}
        <!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
        <h2
          class="edit"
          contenteditable="plaintext-only"
          role="textbox"
          aria-label={t('bubble.renameHint')}
          tabindex="0"
          spellcheck="false"
          title={t('bubble.renameHint')}
          bind:this={nameEl}
          onfocus={() => (nameWas = bubble.name)}
          onblur={commitName}
          onkeydown={onNameKey}
        >{bubble.name}</h2>
      {/if}
      <span class="sub">
        {bubble.instance}{bubble.project_name ? ' · ' + bubble.project_name : ''}
      </span>
    </div>
    <button class="x" onclick={close} aria-label={t('detail.close')}>✕</button>
  </header>

  {#if loading}
    <p class="dim">{t('board.loading')}</p>
  {:else if error}
    <p class="err">{error}</p>
  {:else if nodes.length === 0}
    <p class="dim">{t('detail.noThreads')}</p>
  {:else}
    <p class="count">
      {nodes.length} thread{nodes.length === 1 ? '' : 's'} · newest first
      {#each rollup(bubble.thread_levels) as [lv, n] (lv)}
        <span class="tally lvl-{lv}" title="{n} {levelLabel(lv)}">{levelIcon(lv)} {n}</span>
      {/each}
    </p>
    <ol class="rail">
      {#each nodes as n (n.id)}
        <li class="node" class:done={!n.active}>
          <span class="dot"></span>
          <button class="body" onclick={() => store.openThread(n.id)} title={t('detail.openThread')}>
            <div class="line1">
              <span class="seq">#{n.seq}</span>
              <span class="title">{n.title}</span>
            </div>
            <div class="line2">
              <!-- the thread's OWN buoyancy, not just open/closed -->
              <span class="lvl lvl-{n.level}" title={n.reason}>
                {levelIcon(n.level)} {levelLabel(n.level)}
              </span>
              {#if n.state}<span class="meta" title={t('detail.planeState')}>· {n.state}</span>{/if}
              {#if n.owner}<span class="meta">· {n.owner}</span>{/if}
              <span class="meta">· {age(n.created_at)}</span>
            </div>
          </button>
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
  .htext {
    /* The close button is flex:none, so without this a long hostname-style name
       would grow the header instead of wrapping — a flex child will not shrink
       below its content width until min-width is cleared. */
    min-width: 0;
  }
  .htext h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 750;
    letter-spacing: -0.01em;
    color: var(--text);
    overflow-wrap: anywhere;
  }
  /* Same affordance as a thread's title in ThreadView: invisible until you go
     near it, so the header still reads as a heading rather than a form. */
  .htext h2.edit {
    border-radius: 7px;
    padding: 0 0.3rem;
    margin-left: -0.3rem;
    outline: none;
    cursor: text;
  }
  .htext h2.edit:hover {
    background: color-mix(in oklab, var(--text) 7%, transparent);
  }
  .htext h2.edit:focus {
    background: color-mix(in oklab, var(--wip) 12%, transparent);
    box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--wip) 50%, transparent);
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
    width: 100%;
    text-align: left;
    border: none;
    background: transparent;
    color: inherit;
    cursor: pointer;
    padding: 0.15rem 0.4rem;
    margin: -0.15rem -0.4rem;
    border-radius: 8px;
    transition: background 0.14s ease;
  }
  .body:hover {
    background: var(--hover);
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
  /* a thread's own buoyancy chip — same palette as the bands */
  .lvl {
    font-weight: 600;
    color: var(--muted);
  }
  .lvl-in_progress {
    color: var(--wip);
  }
  .lvl-reviewed {
    color: var(--reviewed);
  }
  .lvl-zzzz {
    color: var(--zzzz);
  }
  .lvl-rip {
    color: var(--rip);
  }
  .lvl-done {
    color: var(--done);
  }
  .tally {
    margin-left: 0.45rem;
    font-weight: 600;
  }
</style>
