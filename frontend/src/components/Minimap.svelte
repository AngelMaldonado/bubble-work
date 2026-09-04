<script lang="ts">
  // The minimap, ported from v0.
  //
  // Collapsed it is a column of coloured dashes: the SHAPE of the board at a
  // glance — how much is burning, how much is buried — without reading a word.
  // Hovering turns the same column into the names, grouped by band, each one a
  // jump. It is the board's table of contents, and the reason a long board
  // stays navigable.
  //
  // Two CSS notes carried over, both learned the hard way:
  //   · no `transform` on the rail — a transformed ancestor becomes a backdrop
  //     root and kills any `backdrop-filter` inside it;
  //   · no `backdrop-filter` on the rail either — a fixed element nested under a
  //     stacking context samples an empty backdrop in Chromium. Solid fill.
  import { onMount } from 'svelte';
  import type { Lifecycle } from '../lib/api';
  import { bandFace, bandName, bandOrder } from '../lib/bands';

  export type MapItem = { id: string; name: string; life: Lifecycle };

  let {
    items,
    // What actually scrolls. The board used to move the window; now the shell is
    // fixed and only the pane scrolls, and a spy listening to the wrong box sees
    // nothing move at all.
    scroller = null,
  }: { items: MapItem[]; scroller?: HTMLElement | null } = $props();

  // Empty bands are dropped HERE and nowhere else: the board draws them because
  // an empty band is a fact about the work, but a dash column with a gap in it
  // just looks broken.
  const groups = $derived(
    bandOrder
      .map((band) => ({ band, items: items.filter((i) => i.life === band) }))
      .filter((g) => g.items.length > 0),
  );

  let active = $state<string>('');

  function go(id: string) {
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
  function goBand(band: string) {
    document.getElementById('band-' + band)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  // Scroll-spy: whichever band last crossed the line near the top of the
  // viewport is the one being read.
  function sync() {
    const line = 140;
    let now = groups[0]?.band ?? '';
    for (const g of groups) {
      const el = document.getElementById('band-' + g.band);
      if (!el) continue;
      if (el.getBoundingClientRect().top > line) break;
      now = g.band;
    }
    // At the very bottom the last band never crosses the line, so it would
    // never light up. Say so explicitly rather than leaving it dark.
    const box = scroller;
    const atEnd = box
      ? box.scrollTop + box.clientHeight >= box.scrollHeight - 2
      : innerHeight + scrollY >= document.documentElement.scrollHeight - 2;
    if (atEnd) now = groups.at(-1)?.band ?? now;
    active = now;
  }

  onMount(() => {
    let frame = 0;
    const schedule = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        sync();
      });
    };
    sync();
    // Captured on the document: it catches the pane's scroll and the window's
    // without the minimap having to know which one this page uses.
    document.addEventListener('scroll', schedule, { passive: true, capture: true });
    addEventListener('resize', schedule, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener('scroll', schedule, { capture: true });
      removeEventListener('resize', schedule);
    };
  });
</script>

<aside class="minimap" aria-label="navegar el board">
  <div class="inner">
    <div class="list">
      {#each groups as g (g.band)}
        <div class="sec lvl-{g.band}" class:active={active === g.band}>
          <button
            class="sechead"
            onclick={() => goBand(g.band)}
            title="{bandName(g.band)} ({g.items.length})">
            <span class="ico">{bandFace(g.band)}</span>
            <span class="lbl">{bandName(g.band)}</span>
            <span class="cnt">{g.items.length}</span>
          </button>
          <div class="items">
            {#each g.items as b (b.id)}
              <button class="item" onclick={() => go('bw-' + b.id)} title={b.name}>
                <span class="dash"></span>
                <span class="iname">{b.name}</span>
              </button>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
</aside>

<style>
  .lvl-hot { --dot: var(--hot); }
  .lvl-dormant { --dot: var(--dormant); }
  .lvl-rip { --dot: var(--rip); }
  .lvl-closed { --dot: var(--closed); }

  .minimap {
    position: fixed;
    z-index: var(--z-chrome);
    /* Off the edge. Flush against the window the dashes read as a scrollbar,
       which is the one thing this is not. */
    right: 0.6rem;
    top: 0;
    bottom: 0;
    height: fit-content;
    margin-block: auto;
    display: flex;
    align-items: center;
    padding: 0.5rem 0.6rem;
    border: 1px solid transparent;
    border-radius: 16px;
    background: transparent;
    transition:
      background 0.18s ease,
      border-color 0.18s ease,
      box-shadow 0.18s ease,
      padding 0.18s ease;
  }
  .inner { display: flex; flex-direction: column; max-height: 82vh; }
  /* The scroll layer is here, not on the panel — clipping the panel would clip
     its own rounded corners. */
  .list {
    display: flex; flex-direction: column;
    gap: 0.5rem; min-height: 0; max-height: 78vh;
    overflow: hidden;
    transition: gap 0.18s ease;
  }

  .sec { display: flex; flex-direction: column; gap: 3px; align-items: flex-end; }
  .sechead {
    display: flex; align-items: center; gap: 0.45rem;
    width: 100%; justify-content: flex-end;
    background: transparent; border: none; cursor: pointer;
    color: var(--muted); padding: 0.1rem 0.15rem;
  }
  .ico {
    font-size: 0.82rem; opacity: 0.85; filter: grayscale(0.2);
    transition: opacity 0.18s ease, filter 0.18s ease;
  }
  .sec.active .ico { opacity: 1; filter: none; }
  .lbl { font-size: 0.74rem; font-weight: 700; letter-spacing: 0.02em; white-space: nowrap; }
  .cnt {
    font-size: 0.66rem; color: var(--faint);
    background: var(--hover); padding: 0 0.4rem; border-radius: 999px;
  }
  .items { display: flex; flex-direction: column; gap: 3px; align-items: flex-end; width: 100%; }
  .item {
    display: flex; align-items: center; gap: 0.5rem; justify-content: flex-end;
    width: 100%; background: transparent; border: none; cursor: pointer;
    padding: 2px 0.15rem; color: var(--muted); border-radius: 7px;
  }
  .dash {
    display: block; width: 18px; height: 3px; border-radius: 999px;
    background: var(--dot); opacity: 0.55; flex: none;
    transition: width 0.18s ease, opacity 0.18s ease;
  }
  .sec.active .dash { opacity: 0.95; }
  .item:hover .dash { opacity: 1; width: 24px; }

  /* Collapsed, it is only dashes. The words appear on hover. */
  .lbl, .cnt, .iname { display: none; }
  .iname {
    font-size: 0.78rem; line-height: 1.2; max-width: 210px;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }

  .minimap:hover, .minimap:focus-within {
    background: var(--surface-solid);
    border-color: var(--line);
    box-shadow: 0 20px 50px var(--shadow-strong);
    padding: 0.8rem;
  }
  .minimap:hover .list, .minimap:focus-within .list { overflow-y: auto; gap: 0.7rem; }
  .minimap:hover .sec, .minimap:focus-within .sec { align-items: stretch; gap: 1px; }
  .minimap:hover .sechead, .minimap:focus-within .sechead {
    justify-content: flex-start;
    border-bottom: 1px solid var(--line);
    padding-bottom: 0.3rem; margin-bottom: 0.15rem;
  }
  .minimap:hover .items, .minimap:focus-within .items { align-items: stretch; }
  .minimap:hover .item, .minimap:focus-within .item { justify-content: flex-start; }
  .minimap:hover .item:hover, .minimap:focus-within .item:hover {
    background: var(--hover); color: var(--text);
  }
  .minimap:hover .lbl, .minimap:hover .cnt, .minimap:hover .iname,
  .minimap:focus-within .lbl, .minimap:focus-within .cnt, .minimap:focus-within .iname {
    display: block;
  }
  .minimap:hover .dash, .minimap:focus-within .dash {
    width: 8px; height: 8px; border-radius: 999px;
  }
  .minimap:hover .item:hover .dash, .minimap:focus-within .item:hover .dash { width: 8px; }
  .sec.active .lbl { color: var(--text); }

  /* No room for a rail on a phone, and nothing to navigate on one screen. */
  @media (max-width: 720px) {
    .minimap { display: none; }
  }
</style>
