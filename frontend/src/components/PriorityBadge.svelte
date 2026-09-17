<script lang="ts">
  // La prioridad de un hilo, como badge que se despliega.
  //
  // Es la PROPIA del hilo (P1…P4), guardada y separada de la de su burbuja. La
  // cambia sólo el lead global; para los demás es un dato y se dibuja igual, sin
  // menú. Un menú que abre para decir «no puedes» es un control que miente.
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import { priorityMeaning } from '../lib/priority';

  let {
    value = '',
    canEdit = false,
    onchange,
    size = 'sm',
  }: {
    value?: string;
    canEdit?: boolean;
    onchange?: (priority: string) => void;
    /** `xs` para una fila apretada, como la de un hilo en el cajón */
    size?: 'xs' | 'sm';
  } = $props();

  const label = $derived(value || 'P–');
  const hint = (code: string) => {
    const row = priorityMeaning.find(([c]) => c === code);
    return row ? `${row[1]} · ${row[2].toLowerCase()}` : '';
  };
</script>

{#if canEdit}
  <Menu onSelect={(e: { value: string }) => onchange?.(e.value === 'none' ? '' : e.value)}>
    <Menu.Trigger>
      {#snippet element(attributes: Record<string, unknown>)}
        <!-- `stopPropagation`: vive dentro de filas que se abren al hacer clic, y
             elegir una prioridad no es abrir el hilo. -->
        <button
          {...attributes}
          class="pb {size} prio-chip {value ? `prio-${value}` : 'unset'}"
          title={value ? `Prioridad ${value} · ${hint(value)} — cambiar` : 'Sin prioridad — poner una'}
          onclick={(e) => {
            e.stopPropagation();
            (attributes.onclick as ((e: MouseEvent) => void) | undefined)?.(e);
          }}>{label}<span class="caret" aria-hidden="true">▾</span></button>
      {/snippet}
    </Menu.Trigger>
    <Portal>
      <Menu.Positioner>
        <Menu.Content>
          {#each priorityMeaning as [code] (code)}
            <Menu.Item value={code}>
              <Menu.ItemText>
                <span class="prio-chip prio-{code}">{code}</span>
                <span class="mean">{hint(code)}</span>
              </Menu.ItemText>
            </Menu.Item>
          {/each}
          <Menu.Separator />
          <Menu.Item value="none"><Menu.ItemText><span class="mean">Sin prioridad</span></Menu.ItemText></Menu.Item>
        </Menu.Content>
      </Menu.Positioner>
    </Portal>
  </Menu>
{:else if value}
  <span class="pb {size} prio-chip prio-{value}" title="Prioridad {value} · {hint(value)}">{value}</span>
{/if}

<style>
  .pb { flex: none; display: inline-flex; align-items: center; gap: 0.2rem; line-height: 1.4; }
  .pb.xs { padding: 0 0.35rem; font-size: 0.64rem; }
  button.pb { cursor: pointer; }
  button.pb:hover { filter: brightness(1.12); }
  .unset { background: var(--hover); color: var(--faint); }
  .caret { font-size: 0.55rem; opacity: 0.7; }
  .mean { margin-left: 0.4rem; color: var(--muted); font-size: 0.8rem; }
</style>
