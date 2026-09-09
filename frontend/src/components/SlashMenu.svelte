<script lang="ts">
  // The slash menu, anchored AT THE CARET — which is the whole difference
  // between this and a command palette: ⌘K asks "what am I looking for", this
  // one answers "what goes here".
  //
  // Ported from v0, including the two pieces of measuring that make it usable:
  // it flips above the caret when there is no room below, and it keeps the
  // highlighted row in view with container maths rather than `scrollIntoView`,
  // which walks every scrollable ancestor and would nudge the page behind it.
  import { untrack } from 'svelte';
  import type { Command, SlashMenu } from '../lib/slashmenu.svelte';

  let {
    menu,
    field,
    onpick,
  }: {
    menu: SlashMenu;
    /** the editor's box, for deciding where the menu fits */
    field: HTMLElement | null;
    onpick: (c: Command) => void;
  } = $props();

  let box = $state<HTMLElement | null>(null);
  let list = $state<HTMLElement | null>(null);
  let items = $state<HTMLElement[]>([]);
  let place = $state<{ top: number; left: number } | null>(null);

  $effect(() => {
    const s = menu.open;
    // The number of matches is a dependency on purpose: filtering changes the
    // height, and the height decides whether it flips.
    void menu.matches.length;
    if (!s) {
      // Cleared, or the next open flashes at the previous caret's position.
      untrack(() => (place = null));
      return;
    }
    if (!box || !field) return;
    const h = box.offsetHeight;
    const w = box.offsetWidth;
    const roomBelow = field.clientHeight - s.top;
    const flip = h > roomBelow && s.caretTop > h;
    const left = Math.max(0, Math.min(s.left, field.clientWidth - w));
    untrack(() => (place = { top: flip ? s.caretTop - h : s.top, left }));
  });

  $effect(() => {
    const el = items[menu.picked];
    if (!menu.open || !list || !el) return;
    const top = el.offsetTop;
    const bottom = top + el.offsetHeight;
    if (top < list.scrollTop) list.scrollTop = top;
    else if (bottom > list.scrollTop + list.clientHeight) list.scrollTop = bottom - list.clientHeight;
  });
</script>

{#if menu.open}
  <div
    class="slash"
    style="top:{place?.top ?? menu.open.top}px; left:{place?.left ?? menu.open.left}px;
           visibility:{place ? 'visible' : 'hidden'}"
    role="listbox"
    tabindex="-1"
    bind:this={box}>
    <div class="slash-list" bind:this={list}>
      {#if menu.matches.length === 0}
        <p class="none">Ningún bloque con «{menu.open.query}»</p>
      {:else}
        {#each menu.matches as c, i (c.id)}
          <button
            type="button"
            class="item"
            class:on={i === menu.picked}
            bind:this={items[i]}
            role="option"
            aria-selected={i === menu.picked}
            onmouseenter={() => (menu.picked = i)}
            onmousedown={(e) => {
              e.preventDefault(); // keep the focus in the editor
              onpick(c);
            }}>
            <span class="glyph">{c.glyph}</span>{c.label}
          </button>
        {/each}
      {/if}
    </div>
    <!-- Outside the scroller, so the keys stay readable while the list moves. -->
    <p class="hint">↑↓ para elegir · ⏎ para insertar · esc para cerrar</p>
  </div>
{/if}

<style>
  .slash {
    position: absolute;
    z-index: var(--z-float);
    display: flex;
    flex-direction: column;
    min-width: 13rem;
    max-height: 15rem;
    padding: 0.3rem;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface-solid);
    box-shadow: 0 18px 40px var(--shadow-strong);
  }
  .slash-list {
    position: relative; /* the offsetParent the scroll maths measures against */
    min-height: 0;
    overflow-y: auto;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    padding: 0.35rem 0.5rem;
    border: none;
    border-radius: 8px;
    background: none;
    color: var(--text);
    font-size: 0.82rem;
    text-align: left;
  }
  .item.on { background: color-mix(in oklab, var(--accent, var(--hot)) 18%, transparent); }
  .glyph {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 1.6rem;
    height: 1.35rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: var(--muted);
    font-size: 0.68rem;
    font-weight: 700;
  }
  .none,
  .hint {
    margin: 0;
    padding: 0.35rem 0.55rem;
    color: var(--faint);
    font-size: 0.72rem;
  }
  .hint { border-top: 1px solid var(--line); }
</style>
