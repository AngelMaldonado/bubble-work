<script lang="ts">
  // El selector de equipo de las barras (planeador, secuencia, roadmap,
  // canales): el mismo menú del tema que el de la columna, no un `<select>`
  // nativo —el sistema dibuja ese menú y no le llega el tema—. Una píldora con
  // el icono y el nombre del equipo, y una lista que marca el actual y los
  // privados.
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
  import CheckIcon from '@lucide/svelte/icons/check';
  import Icon from './Icon.svelte';
  import type { Team } from '../lib/api';

  let {
    teams,
    value,
    onchange,
    label = 'Equipo',
  }: {
    teams: Pick<Team, 'id' | 'name' | 'personal'>[];
    value: string;
    onchange?: (id: string) => void;
    /** lo que dice el botón a un lector de pantalla */
    label?: string;
  } = $props();

  const now = $derived(teams.find((t) => t.id === value) ?? teams[0]);
</script>

<Menu positioning={{ placement: 'bottom-start' }} onSelect={(e: { value: string }) => onchange?.(e.value)}>
  <Menu.Trigger>
    {#snippet element(attributes: Record<string, unknown>)}
      <button {...attributes} class="team-pick" aria-label={`${label}: ${now?.name ?? ''}`} title={`${label}: ${now?.name ?? ''}`}>
        <Icon of={now?.personal ? 'key' : 'people'} size="20px" tint />
        <span class="name">{now?.name ?? ''}</span>
        <ChevronsUpDownIcon class="size-4 chev" />
      </button>
    {/snippet}
  </Menu.Trigger>
  <Portal>
    <Menu.Positioner>
      <Menu.Content class="team-pick-menu">
        {#each teams as t (t.id)}
          <Menu.Item value={t.id}>
            <span class="face"><Icon of={t.personal ? 'key' : 'people'} tint /></span>
            <Menu.ItemText>{t.name}</Menu.ItemText>
            {#if t.personal}<span class="tag">privado</span>{/if}
            {#if t.id === now?.id}<CheckIcon class="size-4 check" aria-label="actual" />{/if}
          </Menu.Item>
        {/each}
      </Menu.Content>
    </Menu.Positioner>
  </Portal>
</Menu>

<style>
  .team-pick {
    display: inline-flex; align-items: center; gap: 0.45rem; max-width: 16rem;
    padding: 0.25rem 0.6rem 0.25rem 0.45rem; border-radius: 999px;
    border: 1px solid var(--line); background: var(--surface-solid); color: var(--text);
    font-size: 0.85rem; font-weight: 700;
  }
  .team-pick:hover { background: var(--hover); }
  .team-pick:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .team-pick :global(.chev) { flex: none; color: var(--muted); }
  :global(.team-pick-menu) { min-width: 13rem; }
  :global(.team-pick-menu [data-part='item']) { display: flex; align-items: center; gap: 0.55rem; justify-content: flex-start; }
  :global(.team-pick-menu .face) { width: 1rem; text-align: center; }
  :global(.team-pick-menu .tag) { margin-left: auto; padding: 0 0.4rem; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.68rem; font-weight: 700; }
  :global(.team-pick-menu .check) { margin-left: auto; color: var(--accent); }
  :global(.team-pick-menu .tag + .check) { margin-left: 0.3rem; }
</style>
