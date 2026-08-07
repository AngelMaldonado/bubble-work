<script lang="ts">
  import { t } from '../lib/i18n.svelte';
  import {
    Combobox,
    Portal,
    useListCollection,
    type ComboboxRootProps,
  } from '@skeletonlabs/skeleton-svelte';
  import { store } from '../lib/store.svelte';

  // Reuses ProjectCombobox's global .proj-cb* styles so it looks identical.
  type Opt = { value: string; label: string };

  const options = $derived<Opt[]>([
    { value: 'workspace', label: 'Everyone' },
    ...(store.kiosk ? [] : [{ value: 'mine', label: 'Mine' }]),
    ...store.members.map((m) => ({ value: 'm:' + m, label: m })),
  ]);

  const current = $derived(
    store.scope === 'mine' ? 'mine' : store.scope === 'member' ? 'm:' + store.viewMember : 'workspace',
  );

  let visible = $state<Opt[]>([]);
  let inputValue = $state('');

  const collection = $derived(
    useListCollection({
      items: visible,
      itemToString: (i) => i.label,
      itemToValue: (i) => i.value,
    }),
  );
  const value = $derived([current]);

  $effect(() => {
    visible = options;
    inputValue = options.find((o) => o.value === current)?.label ?? 'Everyone';
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
    const v = d.value[0] ?? 'workspace';
    if (v === 'workspace') store.setScope('workspace');
    else if (v === 'mine') store.setScope('mine');
    else store.setViewMember(v.slice(2));
  };
</script>

<Combobox
  class="proj-cb"
  aria-label={t('combo.viewScope')}
  placeholder={t('combo.view')}
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
    <Combobox.Input class="proj-cb-input" aria-label={t('combo.viewScope')} />
    <Combobox.Trigger class="proj-cb-trigger" aria-label={t('combo.openViews')}>
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
