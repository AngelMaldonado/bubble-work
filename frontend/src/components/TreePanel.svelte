<script lang="ts">
  // Marco compartido por todos los filetrees: el árbol no sabe cuánto ancho
  // tiene, y el borde recuerda el ancho por pantalla y por persona.
  import type { Snippet } from 'svelte';
  import { api } from '../lib/api';
  let { name, initial = 320, collapsed = false, children }: {
    name: string; initial?: number; collapsed?: boolean; children: Snippet;
  } = $props();
  const min = 220;
  const storageKey = () => `bubble.tree.width.${api.me?.id ?? 'local'}.${name}`;
  function remembered() {
    try {
      const n = Number(localStorage.getItem(storageKey()));
      return Number.isFinite(n) && n >= min ? n : initial;
    } catch { return initial; }
  }
  let width = $state(remembered());
  let panel = $state<HTMLElement>();
  let dragging = $state(false);
  let startX = 0;
  let startWidth = 0;
  const max = () => Math.max(min, Math.min(720, innerWidth * 0.55));
  function update(n: number) { width = Math.round(Math.max(min, Math.min(max(), n))); }
  function save() {
    try { localStorage.setItem(storageKey(), String(width)); } catch { /* sólo no se recuerda */ }
  }
  function down(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    startX = e.clientX;
    startWidth = panel!.getBoundingClientRect().width;
    dragging = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function finish() { dragging = false; save(); }
  function keys(e: KeyboardEvent) {
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      e.preventDefault(); update(width + (e.key === 'ArrowLeft' ? -20 : 20)); save();
    } else if (e.key === 'Home') { e.preventDefault(); update(initial); save(); }
  }
</script>

<div class="tree-panel" class:collapsed class:dragging bind:this={panel}
  style:--tree-width="{width}px">
  {@render children()}
  {#if !collapsed}
    <!-- Separador ajustable con teclado, según el patrón Window Splitter de ARIA. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div class="edge" data-cursor={dragging ? 'grabbing' : 'resize'} role="separator" tabindex="0" aria-label="Ancho del árbol"
      aria-orientation="vertical" aria-valuemin={min} aria-valuemax={720} aria-valuenow={width}
      onpointerdown={down}
      onpointermove={(e) => dragging && update(startWidth + e.clientX - startX)}
      onpointerup={finish} onpointercancel={finish} onlostpointercapture={() => (dragging = false)}
      onkeydown={keys} ondblclick={() => { update(initial); save(); }}></div>
  {/if}
</div>

<style>
  .tree-panel { position: relative; flex: none; min-width: 0; width: min(var(--tree-width), 55vw); height: 100%; display: flex; }
  .tree-panel.collapsed { width: auto; }
  .tree-panel:not(.collapsed) :global([data-scope='navigation'][data-part='root']) { width: 100%; max-width: none; }
  .edge { position: absolute; top: 0; right: -4px; bottom: 0; width: 8px; z-index: 2; cursor: col-resize; touch-action: none; }
  .edge::after { content: ''; position: absolute; left: 3px; top: 0; bottom: 0; width: 2px; background: transparent; transition: background 0.15s; }
  .edge:hover::after, .edge:focus-visible::after, .dragging .edge::after { background: var(--accent); }
  .edge:focus-visible { outline: 1px solid var(--accent); }
  .dragging { user-select: none; }
  @media (max-width: 760px) { .tree-panel { display: none; } }
</style>
