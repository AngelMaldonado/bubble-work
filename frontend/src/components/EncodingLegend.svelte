<script lang="ts">
  // Qué significan el tamaño y el color ahora mismo, y cuánto pasa el filtro.
  import type { Legend } from '../lib/encoding';

  let { legend, colorLabel, filtering }: { legend: Legend; colorLabel: string; filtering: boolean } = $props();
</script>

<aside class="legend" aria-label="Leyenda de tamaño y color">
  <p class="title">Color: {colorLabel}</p>
  <ul>
    {#each legend.colors as c (c.key)}
      <li><span class="dot" style:background={`linear-gradient(150deg, ${c.g1} 8%, ${c.g2} 52%, ${c.g3} 96%)`}></span>{c.label}</li>
    {/each}
  </ul>
  {#if legend.size}<p class="size"><span class="sz s1"></span><span class="sz s2"></span><span class="sz s3"></span>{legend.size}</p>{/if}
  {#if filtering}<p class="count">{legend.filtered} de {legend.total} pasan el filtro</p>{/if}
</aside>

<style>
  .legend { position: absolute; z-index: 3; top: 4.75rem; right: 1rem; max-width: 18rem; padding: 0.6rem 0.75rem; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in oklab, var(--surface-solid) 92%, transparent); box-shadow: 0 4px 16px rgb(0 0 0 / 0.16); font-size: 0.75rem; pointer-events: none; }
  .title { font-weight: 700; margin-bottom: 0.3rem; }
  ul { display: grid; gap: 0.2rem; }
  li { display: flex; align-items: center; gap: 0.45rem; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .dot { width: 0.75rem; height: 0.75rem; border-radius: 999px; flex: none; box-shadow: inset -1px -2px 3px rgb(0 0 0 / 0.3); }
  .size { display: flex; align-items: center; gap: 0.3rem; margin-top: 0.45rem; color: var(--muted); }
  .sz { display: inline-block; border-radius: 999px; background: var(--faint); flex: none; }
  .s1 { width: 0.4rem; height: 0.4rem; } .s2 { width: 0.6rem; height: 0.6rem; } .s3 { width: 0.85rem; height: 0.85rem; margin-right: 0.2rem; }
  .count { margin-top: 0.4rem; color: var(--text); font-weight: 600; }
  @media (max-width: 650px) { .legend { top: auto; bottom: calc(4.75rem + 42px + 2.2rem); right: 0.5rem; max-width: 11rem; } }
</style>
