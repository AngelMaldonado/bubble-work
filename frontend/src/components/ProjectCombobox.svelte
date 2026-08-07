<script lang="ts">
  import { t } from '../lib/i18n.svelte';
  import {
    Combobox,
    Portal,
    useListCollection,
    type ComboboxRootProps,
  } from '@skeletonlabs/skeleton-svelte';
  import { store } from '../lib/store.svelte';

  type Opt = { value: string; label: string };
  const ALL: Opt = $derived({ value: '', label: t('combo.allProjects') });

  const options = $derived<Opt[]>([ALL, ...store.projects.map((p) => ({ value: p.id, label: p.name }))]);

  let visible = $state<Opt[]>([]);
  let inputValue = $state('');

  const collection = $derived(
    useListCollection({
      items: visible,
      itemToString: (i) => i.label,
      itemToValue: (i) => i.value,
    }),
  );
  const value = $derived([store.project]);

  $effect(() => {
    visible = options;
    inputValue = options.find((o) => o.value === store.project)?.label ?? ALL.label;
  });

  const onOpenChange: ComboboxRootProps['onOpenChange'] = (d) => {
    if (d.open) visible = options;
  };
  const onInputValueChange: ComboboxRootProps['onInputValueChange'] = (d) => {
    inputValue = d.inputValue;
    const q = d.inputValue.trim().toLowerCase();
    visible = q ? options.filter((o) => o.label.toLowerCase().includes(q)) : options;
  };
  const onValueChange: ComboboxRootProps['onValueChange'] = (d) => {
    store.selectProject(d.value[0] ?? '');
  };
</script>

<Combobox
  class="proj-cb"
  aria-label={t('combo.projectFilter')}
  placeholder={t('combo.project')}
  {collection}
  {value}
  {inputValue}
  {onOpenChange}
  {onInputValueChange}
  {onValueChange}
  inputBehavior="autohighlight"
  openOnClick
  closeOnSelect
>
  <Combobox.Control class="proj-cb-control">
    <Combobox.Input class="proj-cb-input" aria-label={t('combo.projectFilter')} />
    <Combobox.Trigger class="proj-cb-trigger" aria-label={t('combo.openProjects')}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
    </Combobox.Trigger>
  </Combobox.Control>
  <Portal>
    <Combobox.Positioner class="proj-cb-positioner">
      <Combobox.Content class="proj-cb-content">
        {#each visible as item (item.value)}
          <Combobox.Item class="proj-cb-item" {item}>
            <Combobox.ItemText>{item.label}</Combobox.ItemText>
          </Combobox.Item>
        {/each}
      </Combobox.Content>
    </Combobox.Positioner>
  </Portal>
</Combobox>

<style>
  /* root fills its fixed-width wrapper (see Workspace .projwrap) — Skeleton's
     default would otherwise stretch it and shove siblings to the far edge. */
  :global(.proj-cb) {
    display: flex;
    width: 100%;
    min-width: 0;
  }
  /* the control is the ONE bordered box; the input fills it with no border of
     its own (fixes the mds "input shorter than container" double-border). */
  :global(.proj-cb-control) {
    display: flex;
    align-items: stretch;
    width: 100%;
    height: 26px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    border-radius: 9px;
    overflow: hidden;
  }
  :global(.proj-cb-input) {
    flex: 1 1 auto;
    min-width: 0;
    height: 100%;
    background: transparent !important;
    border: none !important;
    outline: none !important;
    box-shadow: none !important;
    color: var(--text);
    font-size: 0.75rem;
    padding: 0 0.55rem;
  }
  :global(.proj-cb-trigger) {
    flex: 0 0 auto;
    display: grid;
    place-content: center;
    width: 1.6rem;
    background: transparent;
    border: none;
    cursor: pointer;
    color: var(--faint);
  }
  :global(.proj-cb-trigger svg) {
    width: 13px;
    height: 13px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.6;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  :global(.proj-cb-positioner) {
    z-index: 1000;
  }
  /* opaque items box — no see-through, no blur */
  :global(.proj-cb-content) {
    position: relative;
    z-index: 1000;
    min-width: 12rem;
    max-height: 42vh;
    overflow-y: auto;
    padding: 0.35rem;
    border-radius: 12px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 20px 50px var(--shadow-strong);
  }
  :global(.proj-cb-item) {
    padding: 0.4rem 0.55rem;
    border-radius: 8px;
    font-size: 0.82rem;
    color: var(--text);
    background: transparent;
    cursor: pointer;
  }
  /* clear, theme-aware highlight (overrides Skeleton's default dark fill) */
  :global(.proj-cb-item[data-highlighted]) {
    background: color-mix(in oklab, var(--wip) 20%, transparent) !important;
    color: var(--text) !important;
  }
  :global(.proj-cb-item[data-state='checked']) {
    color: var(--wip) !important;
    font-weight: 700;
  }
</style>
