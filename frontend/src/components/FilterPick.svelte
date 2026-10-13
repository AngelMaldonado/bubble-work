<script lang="ts">
  import Glyphs from './Glyphs.svelte';
  import { untrack } from 'svelte';
  // Un filtro del tablero: el Combobox de Skeleton, no un `<select>` nativo.
  //
  // El menú de un `<select>` lo dibuja el sistema operativo, donde no llega
  // ninguna hoja de estilo: en un tablero con el tema oscuro puesto aparecía una
  // lista blanca del sistema. Y con veinte proyectos o veinte personas, escribir
  // para encontrar uno es justamente el gesto que hace falta, y un `<select>` no
  // lo tiene.
  //
  // Un componente y no cuatro copias: cuatro filtros con el mismo
  // comportamiento escrito cuatro veces son cuatro sitios donde arreglar lo
  // mismo, y el que se quede atrás será el que nadie mire.
  import { Combobox, Portal, useListCollection } from '@skeletonlabs/skeleton-svelte';

  type Item = { label: string; value: string };

  let {
    items = [],
    value = $bindable(''),
    /** Lo que se lee cuando no hay nada elegido. Es una opción de verdad —la
     *  primera de la lista— y no sólo un texto en gris: quitar el filtro tiene
     *  que ser tan fácil como ponerlo. */
    any = 'cualquiera',
    label = '',
    portal = true,
    onchange,
    allowEmpty = true,
    menuMaxHeight = 'min(20rem, calc(100dvh - 6rem))',
  }: {
    items?: Item[];
    value?: string;
    any?: string;
    label?: string;
    /** Dentro de un modal, el menú pertenece a su capa, no al body. */
    portal?: boolean;
    onchange?: (value: string) => void;
    allowEmpty?: boolean;
    menuMaxHeight?: string;
  } = $props();

  const all = $derived<Item[]>(allowEmpty ? [{ label: any, value: '' }, ...items] : items);

  // Lo que la lista enseña AHORA: todo, o lo que coincide con lo tecleado.
  // The combobox resolves its initial label from the initial collection.
  // An empty first collection leaves a preselected value showing the placeholder.
  let shown = $state<Item[]>(untrack(() => all));
  $effect(() => {
    shown = all;
  });

  const collection = $derived(
    useListCollection({
      items: shown,
      itemToString: (i: Item) => i.label,
      itemToValue: (i: Item) => i.value,
    }),
  );
</script>

{#snippet menu()}
  <Combobox.Positioner>
    <Combobox.Content style={`max-height: ${menuMaxHeight}; overflow-y: auto;`}>
      {#each shown as item (item.value)}
        <Combobox.Item {item}>
          <Combobox.ItemText><Glyphs text={item.label} /></Combobox.ItemText>
          <Combobox.ItemIndicator />
        </Combobox.Item>
      {/each}
    </Combobox.Content>
  </Combobox.Positioner>
{/snippet}

<Combobox
  openOnClick
  positioning={{ sameWidth: false }}
  {collection}
  value={value ? [value] : ['']}
  onValueChange={(e: { value: string[] }) => {
    value = e.value[0] ?? '';
    onchange?.(value);
  }}
  onOpenChange={() => (shown = all)}
  onInputValueChange={(e: { inputValue: string }) => {
    const q = e.inputValue.toLowerCase();
    const hit = all.filter((i) => i.label.toLowerCase().includes(q));
    // Sin coincidencias se enseña todo, no una lista vacía: un menú en blanco
    // parece roto, y lo que pasa es que lo tecleado no está.
    shown = hit.length ? hit : all;
  }}
  placeholder={any}>
  {#if label}<Combobox.Label class="sr-only">{label}</Combobox.Label>{/if}
  <Combobox.Control>
    <Combobox.Input
      aria-label={label}
      autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
    <Combobox.Trigger />
  </Combobox.Control>
  {#if portal}<Portal>{@render menu()}</Portal>{:else}{@render menu()}{/if}
</Combobox>
