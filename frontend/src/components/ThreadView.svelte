<script lang="ts">
  import { slide } from 'svelte/transition';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import type { ThreadDetail } from '../lib/types';
  import ThreadToc, { type Heading } from './ThreadToc.svelte';

  type Sel = { kind: 'artifact' | 'logbook' | 'revision'; idx: number };

  let detail = $state<ThreadDetail | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let sel = $state<Sel>({ kind: 'artifact', idx: 0 });

  let contentEl = $state<HTMLElement | null>(null);
  let headings = $state<Heading[]>([]);
  let sideCollapsed = $state(false);

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let mermaidAPI: any = null;
  let mmdSeq = 0;

  // Extract the rendered headings for the minimap.
  function extractHeadings(el: HTMLElement): void {
    const hs = [...el.querySelectorAll<HTMLElement>('.prose h1, .prose h2, .prose h3')].filter(
      (h) => h.id,
    );
    const levels = hs.map((h) => Number(h.tagName[1]));
    const min = levels.length ? Math.min(...levels) : 1;
    headings = hs.map((h) => ({
      id: h.id,
      // strip a leading emoji/symbol/checkbox (e.g. ⬜) so the TOC reads cleanly
      title: (h.textContent ?? '').replace(/^[^\p{L}\p{N}]+/u, '').trim(),
      depth: Number(h.tagName[1]) - min,
    }));
  }

  // Replace ```mermaid code blocks with rendered SVG diagrams (mds-style),
  // lazy-loading mermaid only when a diagram is present.
  async function renderMermaid(el: HTMLElement): Promise<void> {
    const blocks = [...el.querySelectorAll<HTMLElement>('pre > code.language-mermaid')];
    if (!blocks.length) return;
    mermaidAPI ||= (await import('mermaid')).default;
    const dark = document.documentElement.dataset.mode === 'dark';
    mermaidAPI.initialize({ startOnLoad: false, theme: dark ? 'dark' : 'default', securityLevel: 'strict' });
    for (const code of blocks) {
      const pre = code.parentElement;
      if (!pre) continue;
      const holder = document.createElement('div');
      holder.className = 'mermaid-diagram';
      try {
        const { svg } = await mermaidAPI.render(`mmd-${++mmdSeq}`, code.textContent ?? '');
        holder.innerHTML = svg;
      } catch (err) {
        holder.innerHTML = `<div class="mermaid-error">${String(err)}</div>`;
      }
      pre.replaceWith(holder);
    }
  }

  function firstSel(d: ThreadDetail): Sel {
    if (d.artifacts?.length) return { kind: 'artifact', idx: 0 };
    if (d.logbook) return { kind: 'logbook', idx: 0 };
    if (d.revisions?.length) return { kind: 'revision', idx: 0 };
    return { kind: 'artifact', idx: 0 };
  }

  $effect(() => {
    const tid = store.threadId;
    if (!tid) return;
    loading = true;
    error = null;
    detail = null;
    api
      .thread(tid)
      .then((d) => {
        detail = d;
        sel = firstSel(d);
      })
      .catch((e) => (error = e instanceof ApiError ? e.message : String(e)))
      .finally(() => (loading = false));
  });

  const current = $derived.by(() => {
    if (!detail) return null;
    if (sel.kind === 'artifact') return detail.artifacts?.[sel.idx] ?? null;
    if (sel.kind === 'revision') return detail.revisions?.[sel.idx] ?? null;
    return null;
  });

  // On content switch: reset scroll, render mermaid, rebuild the minimap headings.
  $effect(() => {
    const html = current?.html; // dependency: re-run when the shown doc changes
    const el = contentEl;
    if (!el) return;
    requestAnimationFrame(() => {
      if (!html) {
        headings = [];
        return;
      }
      window.scrollTo({ top: 0 });
      void renderMermaid(el);
      extractHeadings(el);
    });
  });

  function close() {
    store.closeThread();
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="screen" aria-label="thread">
  <div class="topbar">
    <button class="back" onclick={close} aria-label="back to board">
      <span aria-hidden="true">←</span> board
    </button>
    {#if detail}
      <span class="seq">#{detail.seq}</span>
      <h2 class="ttl">{detail.title}</h2>
      <div class="chips">
        <span class="chip {detail.kind}">{detail.kind}</span>
        <span class="chip {detail.active ? 'open' : 'done'}">{detail.active ? 'open' : 'done'}</span>
        {#if detail.priority && detail.priority !== 'none'}
          <span class="chip">{detail.priority}</span>
        {/if}
        {#if detail.assignees?.length}
          <span class="chip who">{detail.assignees.join(', ')}</span>
        {/if}
      </div>
    {/if}
  </div>

  {#if loading}
    <p class="dim pad">Loading thread…</p>
  {:else if error}
    <p class="err pad">{error}</p>
  {:else if detail}
    <div class="body">
      <nav class="side" class:collapsed={sideCollapsed}>
        <div class="side-head">
          {#if !sideCollapsed}<span class="side-title">Contents</span>{/if}
          <button
            class="side-toggle"
            onclick={() => (sideCollapsed = !sideCollapsed)}
            aria-label={sideCollapsed ? 'expand sidebar' : 'collapse sidebar'}
            title={sideCollapsed ? 'expand' : 'collapse'}
          >
            {sideCollapsed ? '☰' : '‹'}
          </button>
        </div>

        {#if !sideCollapsed}
          <div class="side-scroll" transition:slide={{ duration: 220, axis: 'y' }}>
            {#if detail.artifacts?.length}
              <div class="sect">Work</div>
              {#each detail.artifacts as a, i (i)}
                <button
                  class="item"
                  class:active={sel.kind === 'artifact' && sel.idx === i}
                  onclick={() => (sel = { kind: 'artifact', idx: i })}
                >
                  <span class="ico">📄</span>{a.title}
                </button>
              {/each}
            {/if}

            {#if detail.logbook}
              <div class="sect">Logbook</div>
              <button
                class="item"
                class:active={sel.kind === 'logbook'}
                onclick={() => (sel = { kind: 'logbook', idx: 0 })}
              >
                <span class="ico">✅</span>Tasks{detail.kind === 'phased' ? ' · phased' : ''}
              </button>
            {/if}

            {#if detail.revisions?.length}
              <div class="sect">Revisions</div>
              {#each detail.revisions as r, i (i)}
                <button
                  class="item"
                  class:active={sel.kind === 'revision' && sel.idx === i}
                  onclick={() => (sel = { kind: 'revision', idx: i })}
                >
                  <span class="ico">📝</span>{r.title}
                </button>
              {/each}
            {/if}
          </div>
        {/if}
      </nav>

      <main class="content" bind:this={contentEl}>
        {#if sel.kind === 'logbook' && detail.logbook}
          <!-- rendered through the same goldmark/prose pipeline as the rest -->
          <article class="prose">
            <h1>Logbook</h1>
            {#if detail.logbook.html}
              {@html detail.logbook.html}
            {:else}
              <p class="dim">No tasks recorded.</p>
            {/if}
            {#if detail.logbook.dod_html}
              <h2>Definition of Done</h2>
              {@html detail.logbook.dod_html}
            {/if}
          </article>
        {:else if current}
          <article class="prose">
            <!-- server-rendered goldmark HTML (safe: raw HTML is escaped) -->
            {@html current.html}
          </article>
        {:else}
          <p class="dim">Nothing to show.</p>
        {/if}
      </main>

      {#if headings.length}
        <ThreadToc {headings} />
      {/if}
    </div>
  {/if}
</div>

<style>
  /* the page (window) scrolls — like the board — so the sticky top bar reliably
     blurs the content passing under it. */
  .screen {
    --topbar-h: 46px;
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    background: transparent; /* let the body's noise + gradient show through */
  }
  /* identical to the board's .statusbar (which blurs correctly), just at the top */
  .topbar {
    position: sticky;
    top: 0;
    z-index: 25;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.55rem 1.25rem;
    font-size: 0.75rem;
    color: var(--faint);
    background: color-mix(in oklab, var(--bg) 72%, transparent);
    backdrop-filter: blur(14px);
    border-bottom: 1px solid var(--line);
  }
  .back {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.7rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.75rem;
  }
  .back:hover {
    color: var(--text);
    background: var(--hover);
  }
  .pad {
    padding: 2rem 1.5rem;
  }
  /* title lives inline in the top bar, next to the back button */
  .seq {
    flex: none;
    font-size: 0.78rem;
    font-weight: 700;
    color: var(--faint);
    font-variant-numeric: tabular-nums;
  }
  .ttl {
    flex: 0 1 auto;
    min-width: 0;
    margin: 0;
    font-size: 0.95rem;
    font-weight: 700;
    letter-spacing: -0.01em;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chips {
    display: flex;
    flex-wrap: nowrap;
    gap: 0.35rem;
    margin-left: auto;
    flex: none;
  }
  .chip {
    font-size: 0.68rem;
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    color: var(--muted);
    text-transform: lowercase;
  }
  .chip.phased {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 45%, transparent);
  }
  .chip.open {
    color: var(--wip);
  }
  .chip.done {
    color: var(--done);
    border-color: color-mix(in oklab, var(--done) 45%, transparent);
  }
  .chip.who {
    text-transform: none;
  }

  .body {
    flex: 1;
    display: flex;
    align-items: flex-start;
  }
  /* file tree: a padded rounded card, kept in view (sticky) while the page scrolls */
  .side {
    position: sticky;
    top: calc(var(--topbar-h) + 0.8rem);
    flex: 0 0 236px;
    margin: 0.8rem 0 1rem 1rem;
    padding: 0.5rem;
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - var(--topbar-h) - 1.6rem);
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: color-mix(in oklab, var(--surface-solid) 60%, transparent);
    transition:
      flex-basis 0.24s ease,
      padding 0.24s ease;
  }
  /* collapsed → shrinks to just the toggle (width via flex-basis, height because
     the scroll slides out and the card no longer stretches to full height) */
  .side.collapsed {
    flex-basis: 46px;
    align-self: flex-start;
    padding: 0.4rem;
  }
  .side-head {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.15rem 0.4rem 0.35rem;
  }
  .side.collapsed .side-head {
    justify-content: center;
    padding: 0;
  }
  .side-title {
    flex: 1;
    min-width: 0;
    font-size: 0.66rem;
    font-weight: 800;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--faint);
    white-space: nowrap;
    overflow: hidden;
  }
  .side-toggle {
    flex: none;
    width: 28px;
    height: 28px;
    display: grid;
    place-content: center;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.9rem;
    line-height: 1;
  }
  .side-toggle:hover {
    color: var(--text);
    background: var(--hover);
  }
  /* fixed width so items don't reflow while the card slides — they just clip */
  .side-scroll {
    flex: 1 1 auto;
    min-height: 0;
    width: 216px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0 0.1rem 0.25rem;
  }
  .sect {
    font-size: 0.66rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--faint);
    margin: 0.6rem 0.4rem 0.25rem;
  }
  .sect:first-child {
    margin-top: 0;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    width: 100%;
    text-align: left;
    padding: 0.4rem 0.5rem;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    font-size: 0.82rem;
    line-height: 1.2;
  }
  .item:hover {
    background: var(--hover);
    color: var(--text);
  }
  .item.active {
    background: color-mix(in oklab, var(--wip) 16%, transparent);
    color: var(--text);
    font-weight: 600;
  }
  .ico {
    font-size: 0.8rem;
    flex: none;
  }

  .content {
    flex: 1;
    min-width: 0;
    padding: 2.4rem clamp(2.75rem, 5vw, 4rem) 6rem clamp(1.5rem, 4vw, 3.5rem);
  }
  .dim {
    color: var(--faint);
  }
  .err {
    color: oklch(0.68 0.19 25);
  }

  /* ---- prose: matches the mds tool's render (serif body, sans headings) ---- */
  .prose {
    --code-bg: color-mix(in oklab, var(--text) 7%, transparent);
    --sans:
      Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    max-width: 940px;
    margin: 0 auto;
    font-family: 'Iowan Old Style', 'Palatino Linotype', Palatino, ui-serif, Georgia, Cambria,
      'Times New Roman', serif;
    font-size: 18px;
    line-height: 1.72;
    color: var(--text);
    overflow-wrap: break-word;
  }
  .prose :global(> :first-child) {
    margin-top: 0;
  }
  .prose :global(h1),
  .prose :global(h2),
  .prose :global(h3),
  .prose :global(h4),
  .prose :global(h5),
  .prose :global(h6) {
    color: var(--text);
    font-family: var(--sans);
    font-weight: 750;
    line-height: 1.2;
    scroll-margin-top: 80px;
  }
  .prose :global(h1) {
    margin: 0 0 28px;
    font-size: clamp(2rem, 4vw, 3.1rem);
    letter-spacing: -0.04em;
  }
  .prose :global(h2) {
    margin: 2.3em 0 0.7em;
    padding-bottom: 0.35em;
    font-size: 1.75rem;
    border-bottom: 1px solid var(--line);
  }
  .prose :global(h3) {
    margin: 1.8em 0 0.6em;
    font-size: 1.35rem;
  }
  .prose :global(h4),
  .prose :global(h5),
  .prose :global(h6) {
    margin: 1.5em 0 0.5em;
  }
  .prose :global(p),
  .prose :global(ul),
  .prose :global(ol),
  .prose :global(blockquote),
  .prose :global(table) {
    margin: 1em 0;
  }
  .prose :global(ul),
  .prose :global(ol) {
    padding-left: 1.5em;
  }
  .prose :global(li) {
    margin: 0.3em 0;
  }
  .prose :global(a) {
    color: var(--wip);
    text-decoration: underline;
    text-decoration-thickness: 0.08em;
    text-underline-offset: 0.18em;
  }
  .prose :global(img) {
    display: block;
    max-width: 100%;
    height: auto;
    margin: 26px auto;
    border-radius: 10px;
    box-shadow: 0 18px 48px var(--shadow);
  }
  /* Plane description images can't be fetched via the API — link to Plane instead */
  .prose :global(a.plane-img) {
    display: inline-flex;
    align-items: center;
    gap: 0.4em;
    margin: 0.3em 0.4em 0.3em 0;
    padding: 0.25em 0.7em;
    border: 1px dashed var(--line);
    border-radius: 9px;
    color: var(--muted);
    text-decoration: none;
    font-family: var(--sans);
    font-size: 0.82em;
  }
  .prose :global(a.plane-img:hover) {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
    background: color-mix(in oklab, var(--wip) 8%, transparent);
  }
  .prose :global(blockquote) {
    margin-left: 0;
    padding: 0.25em 1.15em;
    color: var(--muted);
    border-left: 4px solid var(--wip);
    background: color-mix(in oklab, var(--text) 4%, transparent);
    border-radius: 0 10px 10px 0;
  }
  .prose :global(code) {
    padding: 0.15em 0.38em;
    border-radius: 5px;
    background: var(--code-bg);
    font: 0.86em ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .prose :global(pre) {
    overflow: auto;
    padding: 18px 20px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--code-bg);
    box-shadow: 0 18px 48px var(--shadow);
  }
  .prose :global(pre code) {
    padding: 0;
    background: transparent;
    font-size: 14px;
  }
  .prose :global(table) {
    display: block;
    width: max-content;
    max-width: 100%;
    overflow: auto;
    border-collapse: collapse;
    font-family: var(--sans);
    font-size: 15px;
  }
  .prose :global(th),
  .prose :global(td) {
    padding: 9px 13px;
    border: 1px solid var(--line);
  }
  .prose :global(th) {
    background: color-mix(in oklab, var(--text) 5%, transparent);
    text-align: left;
  }
  .prose :global(hr) {
    height: 1px;
    margin: 2.5em 0;
    background: var(--line);
    border: 0;
  }
  /* task-list items: flex row so the checkbox sits on the first line and wrapped
     text hangs under the text (not back under the checkbox) */
  .prose :global(li:has(> input[type='checkbox'])) {
    list-style: none;
    display: flex;
    align-items: flex-start;
  }
  .prose :global(li:has(> input[type='checkbox']) > input[type='checkbox']) {
    flex: none;
    margin-top: 0.34em; /* drop the box onto the first text line */
  }
  /* custom task-list checkboxes (ported from mds) */
  .prose :global(input[type='checkbox']) {
    display: inline-grid;
    width: 1.05em;
    height: 1.05em;
    margin: 0 0.5em 0 0;
    appearance: none;
    vertical-align: -0.12em;
    border: 1.5px solid color-mix(in oklab, var(--faint) 82%, var(--text));
    border-radius: 0.28em;
    background: color-mix(in oklab, var(--text) 6%, transparent);
    place-content: center;
  }
  .prose :global(input[type='checkbox']:checked) {
    border-color: var(--wip);
    background: var(--wip);
  }
  .prose :global(input[type='checkbox']:checked::before) {
    width: 0.5em;
    height: 0.28em;
    border-bottom: 0.14em solid white;
    border-left: 0.14em solid white;
    content: '';
    transform: translate(0, -0.06em) rotate(-45deg);
  }
  /* mermaid diagrams */
  .prose :global(.mermaid-diagram) {
    margin: 28px 0;
    padding: 20px;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--surface-solid);
    box-shadow: 0 18px 48px var(--shadow);
    text-align: center;
  }
  .prose :global(.mermaid-diagram svg) {
    max-width: 100%;
    height: auto;
  }
  .prose :global(.mermaid-error) {
    color: oklch(0.62 0.2 20);
    font-family: var(--sans);
  }

  @media (max-width: 640px) {
    .side {
      flex-basis: 150px;
    }
    .content {
      padding: 1.4rem 1.1rem 3rem;
    }
    .prose {
      font-size: 16px;
    }
  }
</style>
