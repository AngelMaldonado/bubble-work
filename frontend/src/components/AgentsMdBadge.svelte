<script lang="ts">
  // ¿Le llegó al agente el AGENTS.md vigente? Dos estados con un ícono: al día,
  // o necesita actualizarse (una release anterior, o nunca la leyó). La versión
  // va en el tooltip: un hash no le dice nada a quien recorre una lista, y sí a
  // quien pregunta por ese agente. Tooltip de verdad y no `title`, como en
  // Presence: sale al instante y lleva tema.
  import { Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import Icon from './Icon.svelte';
  import { short } from '../lib/agentHealth';
  import { ago } from '../lib/when';

  let {
    read = '',
    at = '',
    want = '',
    role = 'operators',
  }: {
    /** la release que se le entregó; vacía si nunca leyó agents_md */
    read?: string;
    at?: string;
    /** la vigente de su rol; vacía si el servidor no lo dice */
    want?: string;
    role?: string;
  } = $props();

  const ok = $derived(!!read && !!want && read === want);
  const say = $derived(ok ? 'AGENTS.md al día' : 'AGENTS.md necesita actualizarse');
</script>

<Tooltip openDelay={120} closeDelay={60} positioning={{ placement: 'top' }}>
  <Tooltip.Trigger>
    {#snippet element(attributes: Record<string, unknown>)}
      <span class="agents-md" class:current={ok} class:stale={!ok} aria-label={say} {...attributes}>
        AGENTS.md <Icon of={ok ? 'check' : 'warning'} tint size="1.05em" />
      </span>
    {/snippet}
  </Tooltip.Trigger>
  <Portal>
    <Tooltip.Positioner>
      <Tooltip.Content>
        <span class="rel-tip">
          <strong>{ok ? 'Al día' : 'Necesita actualizarse'}</strong>
          {#if read}
            <span>leyó <b class="ver">{short(read)}</b>{at ? ` · ${ago(at)}` : ''}</span>
          {:else}
            <span>nunca ha leído agents_md</span>
          {/if}
          {#if !ok && want}<span>vigente <b class="ver">{short(want)}</b> ({role})</span>{/if}
        </span>
      </Tooltip.Content>
    </Tooltip.Positioner>
  </Portal>
</Tooltip>

<style>
  .agents-md {
    cursor: default; display: inline-flex; flex: none; align-items: center; gap: 0.3rem;
    padding: 0.15rem 0.45rem 0.15rem 0.55rem; border-radius: 999px;
    background: var(--hover); color: var(--muted); font-size: 0.74rem; white-space: nowrap;
  }
  .agents-md.current { background: color-mix(in oklab, var(--color-success-500) 16%, transparent); color: var(--text); }
  .agents-md.stale { background: color-mix(in oklab, var(--color-warning-500) 20%, transparent); color: var(--text); }
  /* El tooltip se dibuja en un portal: :global, como el resto de su estilo (app.css). */
  :global(.rel-tip) { display: flex; flex-direction: column; gap: 0.1rem; }
  /* La versión, en el texto: negrita y subrayada, sin caja de <code>. */
  :global(.rel-tip .ver) { font-weight: 800; text-decoration: underline; text-underline-offset: 2px; font-variant-numeric: tabular-nums; }
</style>
