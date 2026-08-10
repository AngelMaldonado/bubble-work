<script lang="ts">
  // Whose bubbles you are looking at, on the name in the status bar.
  //
  // That name used to be a plain label — it told you who you were signed in as
  // and did nothing. It is now the owner switch: pick a person and the board
  // narrows to the bubbles they are on, pick "everyone" to widen again.
  //
  // Reuses ProjectCombobox's global .proj-cb* styles, so the three controls in
  // the status bar are one control repeated rather than three that nearly match.
  import { t } from '../lib/i18n.svelte';
  import {
    Combobox,
    Portal,
    useListCollection,
    type ComboboxRootProps,
  } from '@skeletonlabs/skeleton-svelte';
  import { store } from '../lib/store.svelte';

  type Opt = { value: string; label: string };

  const me = $derived(store.actor?.name ?? store.actor?.email ?? '');

  // Signed-in identity first and marked, then everyone else. The board's member
  // list is derived from the bubbles in scope, so somebody with nothing assigned
  // does not appear — which is correct: there is nothing of theirs to show.
  const options = $derived<Opt[]>([
    { value: '', label: t('combo.everyone') },
    ...(me && !store.kiosk ? [{ value: 'mine', label: t('owner.me', { name: me }) }] : []),
    ...store.members.filter((m) => m !== me).map((m) => ({ value: 'm:' + m, label: m })),
  ]);

  const current = $derived(
    store.scope === 'mine' ? 'mine' : store.scope === 'member' ? 'm:' + store.viewMember : '',
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
    inputValue = options.find((o) => o.value === current)?.label ?? options[0].label;
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
    const v = d.value[0] ?? '';
    if (v === 'mine') store.setScope('mine');
    else if (v.startsWith('m:')) store.setViewMember(v.slice(2));
    else store.setScope('workspace');
  };
</script>

<Combobox
  class="proj-cb"
  aria-label={t('owner.filter')}
  placeholder={t('owner.placeholder')}
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
    <Combobox.Input class="proj-cb-input" aria-label={t('owner.filter')} title={store.actor?.email} />
    <Combobox.Trigger class="proj-cb-trigger" aria-label={t('owner.open')}>
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
