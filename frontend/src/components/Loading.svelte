<script lang="ts">
  import BrandOrb from './BrandOrb.svelte';
  let { compact = false, viewport = false, size, label = 'Cargando' }: { compact?: boolean; viewport?: boolean; size?: number; label?: string } = $props();
  const orbSize = $derived(size ?? (compact ? 18 : 56));
</script>

<span class="loading-logo" class:compact class:viewport role="status" aria-label={label} aria-busy="true">
  <BrandOrb size={orbSize} />
  <span class="sr-only">{label}</span>
</span>

<style>
  .loading-logo { display: grid; place-items: center; width: 100%; height: 100%; min-height: 10rem; flex: 1; }
  .loading-logo.viewport { height: 100dvh; min-height: 100dvh; }
  .loading-logo.compact { display: inline-grid; width: auto; height: auto; min-height: 0; flex: none; vertical-align: middle; }
</style>
