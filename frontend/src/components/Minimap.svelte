<script lang="ts">
  import { onMount } from 'svelte';
  import { store } from '../lib/store.svelte';
  import { LEVELS } from '../lib/types';

  let activeLevel = $state('');

  const groups = $derived(
    LEVELS.map((lv) => ({ ...lv, bubbles: store.byLevel(lv.key) })).filter(
      (g) => g.bubbles.length > 0,
    ),
  );

  function go(id: string) {
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
  function goBand(level: string) {
    document.getElementById('band-' + level)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  // scroll-spy: highlight the band nearest the top of the viewport
  function sync() {
    const line = 140;
    let active = groups[0]?.key ?? '';
    for (const g of groups) {
      const el = document.getElementById('band-' + g.key);
      if (!el) continue;
      if (el.getBoundingClientRect().top > line) break;
      active = g.key;
    }
    if (window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 2) {
      active = groups.at(-1)?.key ?? active;
    }
    activeLevel = active;
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
    window.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener('scroll', schedule);
      window.removeEventListener('resize', schedule);
    };
  });
</script>

<aside class="minimap" aria-label="Navigate bubbles">
  <div class="inner">
    {#each groups as g (g.key)}
      <div class="sec lvl-{g.key}" class:active={activeLevel === g.key}>
        <button class="sechead" onclick={() => goBand(g.key)} title="{g.label} ({g.bubbles.length})">
          <span class="ico">{g.icon}</span>
          <span class="lbl">{g.label}</span>
          <span class="cnt">{g.bubbles.length}</span>
        </button>
        <div class="items">
          {#each g.bubbles as b (b.id)}
            <button class="item" onclick={() => go('bw-' + b.id)} title={b.name}>
              <span class="dash"></span>
              <span class="iname">{b.name}</span>
            </button>
          {/each}
        </div>
      </div>
    {/each}
  </div>
</aside>

<style>
  .lvl-in_progress {
    --dot: var(--wip);
  }
  .lvl-reviewed {
    --dot: var(--reviewed);
  }
  .lvl-zzzz {
    --dot: var(--zzzz);
  }
  .lvl-rip {
    --dot: var(--rip);
  }
  .lvl-done {
    --dot: var(--done);
  }

  .minimap {
    position: fixed;
    z-index: 28;
    right: 0;
    top: 50%;
    transform: translateY(-50%);
    max-height: 82vh;
    display: flex;
    align-items: center;
    padding: 0.5rem 0.6rem;
  }
  .inner {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    max-height: 82vh;
    overflow: hidden;
    padding: 0.6rem 0.5rem;
    border-radius: 16px;
    border: 1px solid transparent;
    background: transparent;
    transition:
      background 0.18s ease,
      border-color 0.18s ease,
      box-shadow 0.18s ease,
      padding 0.18s ease;
  }
  /* collapsed → a slim minimap of coloured dashes */
  .sec {
    display: flex;
    flex-direction: column;
    gap: 3px;
    align-items: flex-end;
  }
  .sechead {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    width: 100%;
    justify-content: flex-end;
    background: transparent;
    border: none;
    cursor: pointer;
    color: var(--muted);
    padding: 0.1rem 0.15rem;
  }
  .ico {
    font-size: 0.82rem;
    filter: grayscale(0.2);
    opacity: 0.85;
    transition:
      opacity 0.18s ease,
      filter 0.18s ease;
  }
  .sec.active .ico {
    opacity: 1;
    filter: none;
  }
  .lbl {
    font-size: 0.74rem;
    font-weight: 700;
    letter-spacing: 0.02em;
    white-space: nowrap;
  }
  .cnt {
    font-size: 0.66rem;
    color: var(--faint);
    background: var(--hover);
    padding: 0 0.4rem;
    border-radius: 999px;
  }
  .items {
    display: flex;
    flex-direction: column;
    gap: 3px;
    align-items: flex-end;
    width: 100%;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    justify-content: flex-end;
    width: 100%;
    background: transparent;
    border: none;
    cursor: pointer;
    padding: 2px 0.15rem;
    color: var(--muted);
    border-radius: 7px;
  }
  .dash {
    display: block;
    width: 18px;
    height: 3px;
    border-radius: 999px;
    background: var(--dot);
    opacity: 0.55;
    flex: none;
    transition:
      width 0.18s ease,
      opacity 0.18s ease;
  }
  .sec.active .dash {
    opacity: 0.95;
  }
  .item:hover .dash {
    opacity: 1;
    width: 24px;
  }

  /* labels + names only when expanded */
  .lbl,
  .cnt,
  .iname {
    display: none;
  }
  .iname {
    font-size: 0.78rem;
    line-height: 1.2;
    max-width: 210px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .minimap:hover .inner,
  .minimap:focus-within .inner {
    background: color-mix(in oklab, var(--surface-solid) 74%, transparent);
    backdrop-filter: blur(16px);
    -webkit-backdrop-filter: blur(16px);
    border-color: var(--line);
    box-shadow: 0 20px 50px var(--shadow-strong);
    overflow-y: auto;
    padding: 0.8rem 0.8rem;
    gap: 0.7rem;
  }
  .minimap:hover .sec,
  .minimap:focus-within .sec {
    align-items: stretch;
    gap: 1px;
  }
  .minimap:hover .sechead,
  .minimap:focus-within .sechead {
    justify-content: flex-start;
    border-bottom: 1px solid var(--line);
    padding-bottom: 0.3rem;
    margin-bottom: 0.15rem;
  }
  .minimap:hover .items,
  .minimap:focus-within .items {
    align-items: stretch;
  }
  .minimap:hover .item,
  .minimap:focus-within .item {
    justify-content: flex-start;
  }
  .minimap:hover .item:hover,
  .minimap:focus-within .item:hover {
    background: var(--hover);
    color: var(--text);
  }
  .minimap:hover .lbl,
  .minimap:hover .cnt,
  .minimap:hover .iname,
  .minimap:focus-within .lbl,
  .minimap:focus-within .cnt,
  .minimap:focus-within .iname {
    display: block;
  }
  .minimap:hover .dash,
  .minimap:focus-within .dash {
    width: 8px;
    height: 8px;
    border-radius: 999px;
  }
  .minimap:hover .item:hover .dash,
  .minimap:focus-within .item:hover .dash {
    width: 8px;
  }
  .sec.active .lbl {
    color: var(--text);
  }

  @media (max-width: 720px) {
    .minimap {
      display: none;
    }
  }
</style>
