<script lang="ts">
  import { t } from '../lib/i18n.svelte';
  import {
    Combobox,
    Portal,
    useListCollection,
    type ComboboxRootProps,
  } from '@skeletonlabs/skeleton-svelte';

  type Opt = { value: string; label: string };

  let {
    options,
    value = $bindable(''),
    placeholder = '',
    ariaLabel = '',
    allowCustom = false,
  }: {
    options: Opt[];
    value?: string;
    placeholder?: string;
    ariaLabel?: string;
    allowCustom?: boolean; // when true, typed text becomes the value (e.g. owner)
  } = $props();

  let visible = $state<Opt[]>([]);
  let inputValue = $state('');

  const collection = $derived(
    useListCollection({
      items: visible,
      itemToString: (i) => i.label,
      itemToValue: (i) => i.value,
    }),
  );
  const selValue = $derived([value]);

  $effect(() => {
    visible = options;
    inputValue = options.find((o) => o.value === value)?.label ?? (allowCustom ? value : '');
  });

  const onOpenChange: ComboboxRootProps['onOpenChange'] = (d) => {
    if (d.open) visible = options;
  };
  const onInputValueChange: ComboboxRootProps['onInputValueChange'] = (d) => {
    inputValue = d.inputValue;
    const q = d.inputValue.trim().toLowerCase();
    visible = q ? options.filter((o) => o.label.toLowerCase().includes(q)) : options;
    if (allowCustom) value = d.inputValue; // free-text value follows the input
  };
  const onValueChange: ComboboxRootProps['onValueChange'] = (d) => {
    value = d.value[0] ?? '';
  };
</script>

<Combobox
  class="cb"
  aria-label={ariaLabel}
  {placeholder}
  {collection}
  value={selValue}
  {inputValue}
  {onOpenChange}
  {onInputValueChange}
  {onValueChange}
  allowCustomValue={allowCustom}
  inputBehavior="autohighlight"
  openOnClick
  closeOnSelect
>
  <Combobox.Control class="cb-control">
    <Combobox.Input class="cb-input" aria-label={ariaLabel} {placeholder} />
    <Combobox.Trigger class="cb-trigger" aria-label={t('combo.open')}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
    </Combobox.Trigger>
  </Combobox.Control>
  <Portal>
    <Combobox.Positioner class="cb-positioner">
      <Combobox.Content class="cb-content">
        {#each visible as item (item.value)}
          <Combobox.Item class="cb-item" {item}>
            <Combobox.ItemText>{item.label}</Combobox.ItemText>
          </Combobox.Item>
        {/each}
      </Combobox.Content>
    </Combobox.Positioner>
  </Portal>
</Combobox>

<style>
  :global(.cb) {
    display: flex;
    width: 100%;
    min-width: 0;
  }
  :global(.cb-control) {
    display: flex;
    align-items: stretch;
    width: 100%;
    height: 2.3rem;
    background: color-mix(in oklab, var(--text) 6%, transparent);
    border: 1px solid var(--line);
    border-radius: 10px;
    overflow: hidden;
  }
  :global(.cb-control:focus-within) {
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
  }
  :global(.cb-input) {
    flex: 1 1 auto;
    min-width: 0;
    height: 100%;
    background: transparent !important;
    border: none !important;
    outline: none !important;
    box-shadow: none !important;
    color: var(--text);
    font-size: 0.9rem;
    padding: 0 0.7rem;
  }
  :global(.cb-trigger) {
    flex: 0 0 auto;
    display: grid;
    place-content: center;
    width: 2rem;
    background: transparent;
    border: none;
    cursor: pointer;
    color: var(--faint);
  }
  :global(.cb-trigger svg) {
    width: 13px;
    height: 13px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.6;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  :global(.cb-positioner) {
    z-index: 1000;
  }
  :global(.cb-content) {
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
  :global(.cb-item) {
    padding: 0.4rem 0.55rem;
    border-radius: 8px;
    font-size: 0.85rem;
    color: var(--text);
    background: transparent;
    cursor: pointer;
  }
  :global(.cb-item[data-highlighted]) {
    background: color-mix(in oklab, var(--wip) 20%, transparent) !important;
    color: var(--text) !important;
  }
  :global(.cb-item[data-state='checked']) {
    color: var(--wip) !important;
    font-weight: 700;
  }
</style>
