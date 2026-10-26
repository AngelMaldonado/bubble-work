<script lang="ts">
  // El nivel que produce una regla de furia, como un desplegable de cabezas:
  // blanco, con la cabeza y el nombre. Sólo «Desatada» lleva tema: fuego
  // animado. Sin permiso para editar se dibuja igual, sin menú (como
  // PriorityBadge).
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import Icon from './Icon.svelte';
  import FuryFlames from './FuryFlames.svelte';
  import { furyIcon } from '../lib/icons';
  import type { FuryLevel } from '../lib/api';

  let {
    value,
    levels,
    label,
    canEdit = false,
    onchange,
  }: {
    value: number;
    levels: FuryLevel[];
    label: string;
    canEdit?: boolean;
    onchange?: (level: number) => void;
  } = $props();

  // Lo que el semáforo de la oficina enciende con cada nivel.
  const LIGHT = ['luz verde', 'luz amarilla', 'luz roja', 'luz roja parpadeando'];
  const name = (n: number) => levels.find((l) => l.level === n)?.name ?? '';
</script>


{#if canEdit}
  <Menu onSelect={(e: { value: string }) => onchange?.(Number(e.value))}>
    <Menu.Trigger>
      {#snippet element(attributes: Record<string, unknown>)}
        <button {...attributes} class="pick" class:fury-fire={value === 3} aria-label={`${label}: ${name(value)}`}>
          {#if value === 3}<FuryFlames from={30} to={92} />{/if}
          <Icon of={furyIcon(value)} tint size="2.4rem" /><b>{name(value)}</b><span class="caret" aria-hidden="true">▾</span>
        </button>
      {/snippet}
    </Menu.Trigger>
    <Portal>
      <Menu.Positioner>
        <Menu.Content>
          {#each levels as l (l.level)}
            <Menu.Item value={String(l.level)}>
              <Menu.ItemText>
                <span class="fury-opt" class:on={l.level === value} class:fury-fire={l.level === 3}>
                  {#if l.level === 3}<FuryFlames from={28} to={96} place="inside" size={30} opacity={0.55} />{/if}
                  <Icon of={furyIcon(l.level)} tint size="3.25rem" />
                  <span class="txt"><b>{l.name}</b><span class="light">{LIGHT[l.level]}</span></span>
                </span>
              </Menu.ItemText>
            </Menu.Item>
          {/each}
        </Menu.Content>
      </Menu.Positioner>
    </Portal>
  </Menu>
{:else}
  <span class="pick ro" class:fury-fire={value === 3} aria-label={`${label}: ${name(value)}`}>
    {#if value === 3}<FuryFlames from={30} to={92} />{/if}
    <Icon of={furyIcon(value)} tint size="2.4rem" /><b>{name(value)}</b>
  </span>
{/if}

<style>
  .pick {
    display: inline-flex; flex: none; align-items: center; gap: 0.4rem;
    padding: 0.15rem 0.7rem 0.15rem 0.35rem; border-radius: 999px;
    border: 1px solid var(--line); background: #fff; color: #1b1d24;
    font-size: 0.9rem; cursor: pointer;
  }
  .pick.ro { cursor: default; }
  .pick:not(.ro):hover { border-color: var(--muted); }
  .caret { color: #6b7080; font-size: 0.75rem; }
  /* El menú se dibuja en un portal. */
  :global(.fury-opt) { display: flex; align-items: center; gap: 0.7rem; padding: 0.2rem 0.6rem 0.2rem 0.25rem; min-width: 14rem; border-radius: 12px; }
  :global(.fury-opt .txt) { display: grid; line-height: 1.25; }
  :global(.fury-opt .light) { color: var(--faint); font-size: 0.78rem; }
  :global(.fury-opt.on b) { text-decoration: underline; text-underline-offset: 3px; }

  /* Desatada: el fondo es `.fury-fire` (app.css) y las llamas, FuryFlames. */
  .pick:global(.fury-fire) .caret { color: #fff; }
  :global(.fury-opt.fury-fire) { overflow: hidden; }
  :global(.fury-opt.fury-fire .light) { color: rgb(255 255 255 / 0.85); }
</style>
