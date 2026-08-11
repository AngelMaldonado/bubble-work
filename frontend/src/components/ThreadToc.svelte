<script lang="ts">
  import { t } from '../lib/i18n.svelte';
  // Heading rail / minimap for the thread interior — mirrors the board's Minimap
  // (fixed to the right, vertically centered, expands IN PLACE on hover), but
  // observes a scroll CONTAINER instead of the window.
  // The type belongs to the producer (lib/prose extractHeadings), re-exported here
  // so existing importers keep working and there is still only one definition.
  import type { Heading } from '../lib/prose';
  export type { Heading };

  let { headings }: { headings: Heading[] } = $props();

  let activeID = $state('');

  $effect(() => {
    if (!headings.some((h) => h.id === activeID)) activeID = headings[0]?.id ?? '';
  });

  // scroll-spy on the window (the page scrolls) — same as the board minimap
  function sync() {
    const line = 100; // below the sticky top bar
    let active = headings[0]?.id ?? '';
    for (const h of headings) {
      const el = document.getElementById(h.id);
      if (!el) continue;
      if (el.getBoundingClientRect().top > line) break;
      active = h.id;
    }
    if (window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 2) {
      active = headings.at(-1)?.id ?? active;
    }
    activeID = active;
  }

  $effect(() => {
    void headings; // re-run when the doc changes
    let frame = 0;
    const schedule = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        sync();
      });
    };
    sync();
    window.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener('scroll', schedule);
      window.removeEventListener('resize', schedule);
    };
  });

  function goTo(e: MouseEvent, id: string) {
    e.preventDefault();
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
</script>

<aside class="rail" aria-label={t('thread.onThisPage')}>
  <div class="inner">
    <div class="head">{t('thread.onThisPage')}</div>
    <div class="list">
      {#each headings as h, i (h.id)}
        <button
          class="item"
          class:active={h.id === activeID}
          class:first={i === 0}
          style:--indent={`${h.depth * 4}ch`}
          style:--dash={`${Math.max(20 - h.depth * 5, 8)}px`}
          onclick={(e) => goTo(e, h.id)}
        >
          <span class="dash"></span>
          <span class="title">{h.title}</span>
        </button>
      {/each}
    </div>
  </div>
</aside>

<style>
  /* full-height fixed rail, flex-centered — NO transform (a transformed ancestor
     becomes a backdrop root and would kill the child's backdrop-filter) */
  /* The frost lives on the FIXED rail itself (like the board's brand bubble) —
     backdrop-filter on a child of a fixed element samples an empty backdrop. */
  .rail {
    position: fixed;
    z-index: 8;
    right: 0;
    top: 0;
    bottom: 0;
    height: fit-content;
    margin-block: auto; /* vertical center without transform (transform kills the blur) */
    display: flex;
    align-items: center;
    padding: 0.5rem 0.55rem;
    border: 1px solid transparent;
    border-radius: 16px;
    background: transparent;
    /* Note: a frosted backdrop-filter here is impossible — a position:fixed element
       nested under #app's stacking context samples an empty backdrop (Chromium
       limitation), so the expanded panel uses a solid fill instead. */
    transition:
      background 0.18s ease,
      border-color 0.18s ease,
      box-shadow 0.18s ease,
      padding 0.18s ease;
  }
  .inner {
    display: flex;
    flex-direction: column;
    flex: 1; /* fill the rail's width so the expanded TOC uses the full panel */
    min-width: 0;
    max-height: 78vh;
  }
  /* scroll layer — clipping lives here, not on the frosted .inner */
  .list {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-height: 0;
    max-height: 74vh;
    overflow: hidden;
    align-items: flex-end;
    transition: gap 0.18s ease;
  }
  .head {
    display: none;
    font-size: 0.66rem;
    font-weight: 800;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--faint);
    padding: 0 0.4rem 0.35rem;
    border-bottom: 1px solid var(--line);
    margin-bottom: 0.2rem;
    align-self: stretch;
  }
  .item {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    width: 100%;
    padding: 2px 0.15rem;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    border-left: 2px solid transparent;
  }
  .dash {
    display: block;
    width: var(--dash);
    height: 3px;
    border-radius: 999px;
    background: currentColor;
    opacity: 0.5;
    flex: none;
    transition:
      width 0.18s ease,
      opacity 0.18s ease;
  }
  .item.active .dash {
    background: var(--wip);
    opacity: 1;
  }
  .title {
    display: none;
    font-size: 0.78rem;
    line-height: 1.35;
  }

  /* hover/focus → reveal the tint + border (blur is already on from the base) */
  .rail:hover,
  .rail:focus-within {
    min-width: 210px;
    padding: 0.7rem 0.75rem;
    background: var(--surface-solid);
    border-color: var(--line);
    box-shadow: 0 20px 50px var(--shadow-strong);
  }
  .rail:hover .list,
  .rail:focus-within .list {
    align-items: stretch;
    gap: 1px;
    overflow-y: auto;
  }
  .rail:hover .head,
  .rail:focus-within .head {
    display: block;
  }
  .rail:hover .item,
  .rail:focus-within .item {
    justify-content: flex-start;
    padding: 6px 8px 6px calc(8px + var(--indent));
    border-radius: 0 8px 8px 0;
  }
  /* first heading (doc title) sits flush-left, like the mds section TOC */
  .rail:hover .item.first,
  .rail:focus-within .item.first {
    padding-left: 8px;
  }
  .rail:hover .item:hover,
  .rail:focus-within .item:hover {
    color: var(--text);
    background: var(--hover);
  }
  .rail:hover .item.active,
  .rail:focus-within .item.active {
    color: var(--wip);
    border-left-color: var(--wip);
    background: color-mix(in oklab, var(--wip) 14%, transparent);
  }
  /* no bullet dots in the expanded TOC — just indented titles */
  .rail:hover .dash,
  .rail:focus-within .dash {
    display: none;
  }
  .rail:hover .title,
  .rail:focus-within .title {
    display: -webkit-box;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    text-align: left;
  }

  @media (max-width: 900px) {
    .rail {
      display: none;
    }
  }
</style>
