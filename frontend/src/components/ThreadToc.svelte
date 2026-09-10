<script lang="ts">
  // The heading rail for a thread's document, ported from v0.
  //
  // It mirrors the board's Minimap — fixed to the right, vertically centred,
  // expanding in place on hover — because it answers the same question about a
  // different thing: where am I, and what else is there. Collapsed it is a
  // column of dashes whose length falls with the heading's depth, so the SHAPE
  // of the document is readable before a single word is.
  //
  // Two CSS notes carried over from v0, both learned the hard way:
  //   · no `transform` on the rail — a transformed ancestor becomes a backdrop
  //     root and kills any `backdrop-filter` inside it, so the vertical centring
  //     is `margin-block: auto`;
  //   · no `backdrop-filter` either — a fixed element nested under a stacking
  //     context samples an empty backdrop in Chromium. Solid fill.
  import type { Heading } from '../lib/prose';

  let {
    headings,
    /** what scrolls. The window in v0; a pane here, if one is given. */
    scroller = null,
  }: { headings: Heading[]; scroller?: HTMLElement | null } = $props();

  let active = $state('');

  $effect(() => {
    if (!headings.some((h) => h.id === active)) active = headings[0]?.id ?? '';
  });

  function sync() {
    const line = 100; // just below the sticky top bar
    let now = headings[0]?.id ?? '';
    for (const h of headings) {
      const el = document.getElementById(h.id);
      if (!el) continue;
      if (el.getBoundingClientRect().top > line) break;
      now = h.id;
    }
    // The last heading never crosses the line, so at the bottom it would never
    // light up. Say so explicitly rather than leaving it dark.
    const box = scroller;
    const atEnd = box
      ? box.scrollTop + box.clientHeight >= box.scrollHeight - 2
      : innerHeight + scrollY >= document.documentElement.scrollHeight - 2;
    if (atEnd) now = headings.at(-1)?.id ?? now;
    active = now;
  }

  $effect(() => {
    void headings; // re-run when the document changes
    let frame = 0;
    const schedule = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        sync();
      });
    };
    sync();
    // Captured on the document: it catches a pane's scroll and the window's
    // without the rail having to know which one this page uses.
    document.addEventListener('scroll', schedule, { passive: true, capture: true });
    addEventListener('resize', schedule, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener('scroll', schedule, { capture: true });
      removeEventListener('resize', schedule);
    };
  });

  function goTo(id: string) {
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
</script>

{#if headings.length}
  <aside class="rail" aria-label="en esta página">
    <div class="inner">
      <div class="head">En esta página</div>
      <div class="list">
        {#each headings as h, i (h.id)}
          <button
            class="item"
            class:active={h.id === active}
            class:first={i === 0}
            style:--indent="{h.depth * 4}ch"
            style:--dash="{Math.max(20 - h.depth * 5, 8)}px"
            onclick={() => goTo(h.id)}>
            <span class="dash"></span>
            <span class="title">{h.title}</span>
          </button>
        {/each}
      </div>
    </div>
  </aside>
{/if}

<style>
  .rail {
    position: fixed;
    z-index: var(--z-chrome);
    right: 0.6rem;
    top: 0;
    bottom: 0;
    height: fit-content;
    margin-block: auto;
    display: flex;
    align-items: center;
    padding: 0.5rem 0.55rem;
    border: 1px solid transparent;
    border-radius: 16px;
    background: transparent;
    transition:
      background 0.18s ease,
      border-color 0.18s ease,
      box-shadow 0.18s ease,
      padding 0.18s ease;
  }
  .inner { display: flex; flex-direction: column; flex: 1; min-width: 0; max-height: 78vh; }
  .list {
    display: flex; flex-direction: column;
    gap: 3px; min-height: 0; max-height: 74vh;
    overflow: hidden; align-items: flex-end;
    transition: gap 0.18s ease;
  }
  .head {
    display: none;
    font-size: 0.66rem; font-weight: 800; letter-spacing: 0.09em;
    text-transform: uppercase; color: var(--faint);
    padding: 0 0.4rem 0.35rem;
    border-bottom: 1px solid var(--line);
    margin-bottom: 0.2rem;
    align-self: stretch;
  }
  .item {
    display: flex; align-items: center; justify-content: flex-end;
    gap: 0.5rem; width: 100%;
    padding: 2px 0.15rem;
    border: none; border-left: 2px solid transparent; border-radius: 7px;
    background: transparent; color: var(--muted);
  }
  .dash {
    display: block; width: var(--dash); height: 3px;
    border-radius: 999px; background: currentColor;
    opacity: 0.5; flex: none;
    transition: width 0.18s ease, opacity 0.18s ease;
  }
  .item.active .dash { background: var(--accent); opacity: 1; }
  .title { display: none; font-size: 0.78rem; line-height: 1.35; }

  .rail:hover,
  .rail:focus-within {
    min-width: 210px;
    padding: 0.7rem 0.75rem;
    background: var(--surface-solid);
    border-color: var(--line);
    box-shadow: 0 20px 50px var(--shadow-strong);
  }
  .rail:hover .list,
  .rail:focus-within .list { align-items: stretch; gap: 1px; overflow-y: auto; }
  .rail:hover .head,
  .rail:focus-within .head { display: block; }
  .rail:hover .item,
  .rail:focus-within .item {
    justify-content: flex-start;
    padding: 6px 8px 6px calc(8px + var(--indent));
    border-radius: 0 8px 8px 0;
  }
  /* The first heading is the document's own title and sits flush left. */
  .rail:hover .item.first,
  .rail:focus-within .item.first { padding-left: 8px; }
  .rail:hover .item:hover,
  .rail:focus-within .item:hover { color: var(--text); background: var(--hover); }
  .rail:hover .item.active,
  .rail:focus-within .item.active {
    color: var(--accent);
    border-left-color: var(--accent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
  }
  /* No dots once the titles are readable: two indicators for one fact. */
  .rail:hover .dash,
  .rail:focus-within .dash { display: none; }
  .rail:hover .title,
  .rail:focus-within .title {
    display: -webkit-box;
    flex: 1; min-width: 0; overflow: hidden;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    text-align: left;
  }

  /* No room for a rail on a phone, and nothing to navigate on one screen. */
  @media (max-width: 900px) {
    .rail { display: none; }
  }
</style>
