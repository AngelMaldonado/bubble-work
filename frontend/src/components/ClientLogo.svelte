<script lang="ts">
  // El logo del cliente de un agente (Claude Code, Codex, Cursor…), o su
  // inicial si la marca no tiene logo en simple-icons. Es el CLIENTE, no el
  // modelo: MCP no dice qué modelo hay detrás. Lo mismo en Usuarios, en Agente
  // y en los comandos por harness, para que una marca se vea igual en todos.
  //
  // `tip` es lo que dice el tooltip; sin él, el nombre de la marca.
  import { Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import type { ClientBrand } from '../lib/agentClients';

  let { brand, tip = '', size = 26 }: { brand: ClientBrand; tip?: string; size?: number } = $props();
  const say = $derived(tip || brand.name);
</script>

<Tooltip openDelay={120} closeDelay={60} positioning={{ placement: 'top' }}>
  <Tooltip.Trigger>
    {#snippet element(attributes: Record<string, unknown>)}
      <span class="client" style="--s: {size}px" role="img" aria-label={say} {...attributes}>
        {#if brand.slug}
          <!-- Máscara, como Inventario: las marcas negras toman el color del
               texto y se ven en el tema oscuro. -->
          <span class="logo" style="--mask: url(/logos/{brand.slug}.svg); --brand: {brand.hex ? '#' + brand.hex : 'var(--text)'}"></span>
        {:else}
          <span class="mono">{brand.mono}</span>
        {/if}
      </span>
    {/snippet}
  </Tooltip.Trigger>
  <Portal>
    <Tooltip.Positioner>
      <Tooltip.Content>{say}</Tooltip.Content>
    </Tooltip.Positioner>
  </Portal>
</Tooltip>

<style>
  .client {
    display: inline-grid; flex: none; place-content: center;
    width: var(--s); height: var(--s); border-radius: calc(var(--s) * 0.27);
    background: var(--hover);
  }
  .logo {
    width: calc(var(--s) * 0.62); height: calc(var(--s) * 0.62);
    background: var(--brand);
    -webkit-mask: var(--mask) center / contain no-repeat;
    mask: var(--mask) center / contain no-repeat;
  }
  .mono { color: var(--muted); font-size: calc(var(--s) * 0.27); font-weight: 800; letter-spacing: -0.02em; }
</style>
