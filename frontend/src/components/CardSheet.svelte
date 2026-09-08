<script lang="ts">
  // A planner card, opened, in Trello's shape: the list above the title, the
  // facts in one row, then sections with their own Edit.
  //
  // Priority is CHOSEN and the map is CONSULTED — which is how INITIAL.md
  // describes the map, "solo para consulta o modificación". An earlier cut
  // derived the priority from impact × urgency and offered no way to set it;
  // the map is still here, one section down, and clicking a cell in it sets the
  // priority, so consulting and choosing are the same gesture.
  import {
    Combobox,
    DatePicker,
    Dialog,
    Menu,
    Portal,
    parseDate,
    useListCollection,
    type ComboboxRootProps,
  } from '@skeletonlabs/skeleton-svelte';
  import XIcon from '@lucide/svelte/icons/x';
  import ChevronIcon from '@lucide/svelte/icons/chevron-down';
  import AlignLeftIcon from '@lucide/svelte/icons/align-left';
  import ListIcon from '@lucide/svelte/icons/list';
  import MarkdownField from './MarkdownField.svelte';
  import type { Card } from './Kanban.svelte';

  // Skeleton ships NO css for Dialog — its parts are styled with utilities, and
  // the shape below is the one its documentation uses. Writing our own scrim and
  // panel produced a dialog that worked and did not look like the rest of the
  // system. The z-indexes come from our scale rather than the docs' `z-50`,
  // because ours already says what sits above what.
  const anim =
    'transition transition-discrete opacity-0 translate-y-[100px] ' +
    'starting:data-[state=open]:opacity-0 starting:data-[state=open]:translate-y-[100px] ' +
    'data-[state=open]:opacity-100 data-[state=open]:translate-y-0';

  export type Objective = { n: number; name: string };

  let {
    open = $bindable(false),
    card = $bindable(null),
    objectives = [],
    priorityMap,
    render,
    columns = [],
    columnId = '',
    onmove,
    ondelete,
  }: {
    open?: boolean;
    card?: Card | null;
    objectives?: Objective[];
    priorityMap?: { cols: string[]; rows: string[][] };
    /** markdown → html; the server in the real app */
    render?: (md: string) => string | Promise<string>;
    /** where the card can go, and where it is */
    columns?: { id: string; name: string }[];
    columnId?: string;
    onmove?: (cardId: string, columnId: string) => void;
    ondelete?: (id: string) => void;
  } = $props();

  // Priority is CHOSEN here, and the map is what you consult before choosing —
  // which is how INITIAL.md describes it: "solo para consulta o modificación".
  // An earlier cut derived it from impact × urgency and offered no way to set
  // it; this one asks, and keeps the map in view so the answer is not a guess.
  const PRIOS = ['P1', 'P2', 'P3', 'P4'] as const;
  let notes = $state('');

  // Seeded when a different card opens, not on every render: typing here must
  // not be undone by the parent re-rendering.
  let seeded = $state<string | null>(null);
  $effect(() => {
    if (card && card.id !== seeded) {
      seeded = card.id;
      notes = card.notes ?? '';
    }
  });

  // Written back so the board shows what the sheet decided.
  $effect(() => {
    if (card) card.notes = notes;
  });


  // The description edits in place, driven from its section header the way
  // Trello does it: reading is the default and "Editar" is a decision.
  let editingNotes = $state(false);

  const column = $derived(columns.find((c) => c.id === columnId));

  /** What each priority means, so the list says more than four letters. */
  const meaning: Record<string, string> = {
    P1: 'crítica — operación detenida',
    P2: 'alta — hay workaround',
    P3: 'normal — entra en planeación',
    P4: 'baja — backlog',
  };

  // The objective is a Combobox rather than a <select>: the list grows with the
  // department, the popup of a native select is drawn by the operating system
  // where no stylesheet reaches it, and typing to find one is the point.
  const asItem = (o: Objective) => ({ label: `${o.n}. ${o.name}`, value: String(o.n) });
  let objItems = $state<{ label: string; value: string }[]>([]);
  $effect(() => {
    objItems = objectives.map(asItem);
  });
  const objCollection = $derived(
    useListCollection({
      items: objItems,
      itemToString: (i) => i.label,
      itemToValue: (i) => i.value,
    }),
  );
  const prioCollection = useListCollection({
    items: PRIOS.map((p) => ({ label: p, value: p })),
    itemToString: (i) => i.label,
    itemToValue: (i) => i.value,
  });

  const onObjInput: ComboboxRootProps['onInputValueChange'] = (e) => {
    const q = e.inputValue.toLowerCase();
    const hit = objectives.map(asItem).filter((i) => i.label.toLowerCase().includes(q));
    objItems = hit.length ? hit : objectives.map(asItem);
  };

  // The due date is a real date, picked from a calendar and stored as an ISO
  // string. Typing "12 sep" into a text box is how a date becomes a label
  // nobody can sort, filter or put on a calendar.
  const due = $derived.by(() => {
    if (!card?.due) return [];
    try {
      return [parseDate(card.due)];
    } catch {
      // The mock's older cards carry "12 sep", which is not a date. Showing an
      // empty picker is better than refusing to open the card.
      return [];
    }
  });
</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Portal>
    <Dialog.Backdrop
      class="fixed inset-0 bg-surface-50-950/50"
      style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-center justify-center p-4"
      style="z-index: var(--z-drawer)">
      <Dialog.Content
        class="card bg-surface-100-900 w-full max-w-xl space-y-4 p-4 shadow-xl {anim}">
        {#if card}
          <!-- Where it is, first — Trello puts the list above the title because
               a card's column is the loudest thing about it. -->
          {#if column}
            <Menu onSelect={(e: { value: string }) => onmove?.(card.id, e.value)}>
              <Menu.Trigger>
                <span class="lista">🗂 {column.name} <ChevronIcon class="size-3.5" /></span>
              </Menu.Trigger>
              <Portal>
                <Menu.Positioner>
                  <Menu.Content>
                    {#each columns as c (c.id)}
                      <Menu.Item value={c.id}>
                        <Menu.ItemText>{c.id === columnId ? '· ' : ''}{c.name}</Menu.ItemText>
                      </Menu.Item>
                    {/each}
                  </Menu.Content>
                </Menu.Positioner>
              </Portal>
            </Menu>
          {/if}

          <header class="head">
            <Dialog.Title class="min-w-0 flex-1 text-lg font-bold">
              <input class="ttl" bind:value={card.title} aria-label="título" />
            </Dialog.Title>
            <Dialog.CloseTrigger class="btn-icon hover:preset-tonal">
              <XIcon class="size-4" />
            </Dialog.CloseTrigger>
          </header>

          <!-- The card's facts, in one row of chips. Priority has no "+" and no
               dropdown on purpose: it is the only one here that is DERIVED. -->
          <div class="facts">
            <div class="fact pick">
              <Combobox
                positioning={{ sameWidth: false }}
                collection={objCollection}
                value={card.obj ? [String(card.obj)] : []}
                onValueChange={(e: { value: string[] }) =>
                  (card.obj = e.value[0] ? Number(e.value[0]) : undefined)}
                onOpenChange={() => (objItems = objectives.map(asItem))}
                onInputValueChange={onObjInput}
                placeholder="sin objetivo">
                <Combobox.Label class="cap">Objetivo</Combobox.Label>
                <Combobox.Control>
                  <Combobox.Input />
                  <Combobox.Trigger />
                </Combobox.Control>
                <Portal>
                  <Combobox.Positioner>
                    <Combobox.Content>
                      {#each objItems as item (item.value)}
                        <Combobox.Item {item}>
                          <Combobox.ItemText>{item.label}</Combobox.ItemText>
                          <Combobox.ItemIndicator />
                        </Combobox.Item>
                      {/each}
                    </Combobox.Content>
                  </Combobox.Positioner>
                </Portal>
              </Combobox>
            </div>
            <div class="fact prio-field">
              <Combobox
                positioning={{ sameWidth: false }}
                collection={prioCollection}
                value={card.prio ? [card.prio] : []}
                onValueChange={(e: { value: string[] }) => (card.prio = e.value[0] ?? '')}
                placeholder="sin prioridad">
                <Combobox.Label class="cap">Prioridad</Combobox.Label>
                <Combobox.Control class="prio-{card.prio ?? ''}">
                  <Combobox.Input />
                  <Combobox.Trigger />
                </Combobox.Control>
                <Portal>
                  <Combobox.Positioner>
                    <Combobox.Content>
                      {#each PRIOS as pr (pr)}
                        <Combobox.Item item={{ label: pr, value: pr }}>
                          <Combobox.ItemText>
                            <span class="prio-chip prio-{pr}">{pr}</span>
                            {meaning[pr] ?? ''}
                          </Combobox.ItemText>
                          <Combobox.ItemIndicator />
                        </Combobox.Item>
                      {/each}
                    </Combobox.Content>
                  </Combobox.Positioner>
                </Portal>
              </Combobox>
            </div>
            <div class="fact">
              <DatePicker
                value={due}
                onValueChange={(e: { valueAsString: string[] }) => (card.due = e.valueAsString[0] ?? '')}
                locale="es-MX"
                startOfWeek={1}>
                <DatePicker.Label class="cap">Entrega</DatePicker.Label>
                <DatePicker.Control class="dp-control">
                  <DatePicker.Input placeholder="sin fecha" />
                  <DatePicker.Trigger>🗓</DatePicker.Trigger>
                </DatePicker.Control>
                <Portal>
                  <DatePicker.Positioner>
                    <DatePicker.Content class="dp-content">
                      <DatePicker.View view="day">
                        <DatePicker.Context>
                          {#snippet children(dp)}
                            <DatePicker.ViewControl class="dp-nav">
                              <DatePicker.PrevTrigger>‹</DatePicker.PrevTrigger>
                              <DatePicker.ViewTrigger>
                                <DatePicker.RangeText />
                              </DatePicker.ViewTrigger>
                              <DatePicker.NextTrigger>›</DatePicker.NextTrigger>
                            </DatePicker.ViewControl>
                            <DatePicker.Table>
                              <DatePicker.TableHead>
                                <DatePicker.TableRow>
                                  {#each dp().weekDays as d, i (i)}
                                    <DatePicker.TableHeader>{d.short}</DatePicker.TableHeader>
                                  {/each}
                                </DatePicker.TableRow>
                              </DatePicker.TableHead>
                              <DatePicker.TableBody>
                                {#each dp().weeks as week, i (i)}
                                  <DatePicker.TableRow>
                                    {#each week as day, j (j)}
                                      <DatePicker.TableCell value={day}>
                                        <DatePicker.TableCellTrigger>{day.day}</DatePicker.TableCellTrigger>
                                      </DatePicker.TableCell>
                                    {/each}
                                  </DatePicker.TableRow>
                                {/each}
                              </DatePicker.TableBody>
                            </DatePicker.Table>
                          {/snippet}
                        </DatePicker.Context>
                      </DatePicker.View>
                    </DatePicker.Content>
                  </DatePicker.Positioner>
                </Portal>
              </DatePicker>
            </div>
          </div>

          <section>
            <div class="sec">
              <AlignLeftIcon class="size-4" />
              <h3>Descripción</h3>
              <button class="edit" onclick={() => (editingNotes = !editingNotes)}>
                {editingNotes ? 'Listo' : 'Editar'}
              </button>
            </div>
            <MarkdownField
              bind:value={notes}
              bind:editing={editingNotes}
              chrome={false}
              {render}
              minHeight="8rem"
              placeholder="por qué existe, qué es verdad cuando esté hecho, el siguiente paso…" />
          </section>

          {#if priorityMap}
            <section>
              <div class="sec">
                <ListIcon class="size-4" />
                <h3>El mapa</h3>
              </div>
              <!-- Read-only, and here for one reason: to be looked at before the
                   priority above is chosen. «Lo necesito urgente» se responde
                   con «¿pasa algo si no se hace hoy?». -->
              <table class="map">
                <thead>
                  <tr><th></th>{#each priorityMap.cols as c (c)}<th>{c}</th>{/each}</tr>
                </thead>
                <tbody>
                  {#each priorityMap.rows as row (row[0])}
                    <tr>
                      <th>{row[0]}</th>
                      {#each row.slice(1) as cell, i (i)}
                        <td>
                          <!-- No "current" mark: three combinations give P3, so
                               outlining every cell that matches the chosen
                               priority points at three places you are not. -->
                          <button
                            class="prio-chip prio-{cell}"
                            title="poner {cell}"
                            onclick={() => (card.prio = cell)}>{cell}</button>
                        </td>
                      {/each}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </section>
          {/if}

          <footer>
            <button class="danger" onclick={() => { ondelete?.(card.id); open = false; }}>
              🗑 borrar
            </button>
          </footer>
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  /* Skeleton draws combobox, date-picker and segmented-control by [data-part];
     what is set here is only the density and the two things it cannot know:
     these live in a row of facts, so they must not stretch, and the calendar is
     portalled, so its surface has to be given to it. */
  /* Every field is the same box and the same height, because they are the same
     kind of thing. `min-height` rather than `height`: the chip inside the
     priority field is taller than bare text, and a fixed height would clip it. */
  .facts :global([data-scope='combobox'][data-part='control']),
  .facts :global([data-scope='date-picker'][data-part='control']) {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 2.1rem;
    padding: 0.15rem 0.45rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
  }
  .facts :global([data-scope='combobox'][data-part='root']),
  .facts :global([data-scope='date-picker'][data-part='root']) { min-width: 0; }
  /* The colour goes on the FIELD, not on a chip inside it: a chip beside the
     input made the box say "P4 P4". A bar down its left edge carries the same
     colour without repeating the word. */
  .prio-field :global([data-scope='combobox'][data-part='control']) {
    border-left-width: 4px;
  }
  /* Matched at the same weight as the rule that draws the box. `.prio-P1`
     alone lost to `[data-scope][data-part]`, whose `border` shorthand carries a
     colour, so the bar stayed the neutral line. */
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P1) { border-left-color: var(--p1); }
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P2) { border-left-color: var(--p2); }
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P3) { border-left-color: var(--p3); }
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P4) { border-left-color: var(--p4); }
  /* The input inside carries Skeleton's own field styling — its own radius and
     padding — which inside our control reads as a box drawn inside another box.
     The control IS the field; the input is only the text in it. */
  .facts :global(input) {
    min-width: 0;
    padding: 0.15rem 0.25rem;
    border: none;
    border-radius: 0;
    background: transparent;
    box-shadow: none;
    color: var(--text);
    font-size: 0.82rem;
    outline: none;
  }
  /* One ring, on the box that is the field. */
  .facts :global([data-scope='combobox'][data-part='control']:focus-within),
  .facts :global([data-scope='date-picker'][data-part='control']:focus-within) {
    border-color: color-mix(in oklab, var(--accent) 60%, transparent);
  }
  .facts :global([data-scope='combobox'][data-part='trigger']),
  .facts :global([data-scope='date-picker'][data-part='trigger']) {
    flex: none;
    color: var(--faint);
    font-size: 0.8rem;
  }

  :global(.dp-content) {
    padding: 0.6rem;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface-solid);
    box-shadow: 0 20px 50px var(--shadow-strong);
    z-index: var(--z-float);
  }
  :global(.dp-nav) {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding-bottom: 0.4rem;
    color: var(--muted);
    font-size: 0.82rem;
    font-weight: 700;
  }
  :global(.dp-content [data-part='table-header']) {
    padding: 0.2rem;
    color: var(--faint);
    font-size: 0.7rem;
    font-weight: 600;
  }
  :global(.dp-content [data-part='table-cell-trigger']) {
    display: grid;
    place-content: center;
    width: 2rem;
    height: 2rem;
    border-radius: 8px;
    font-size: 0.8rem;
  }
  :global(.dp-content [data-part='table-cell-trigger']:hover) { background: var(--hover); }
  :global(.dp-content [data-part='table-cell-trigger'][data-selected]) {
    background: var(--accent);
    color: var(--bg);
    font-weight: 700;
  }
  :global(.dp-content [data-part='table-cell-trigger'][data-outside-range]) { opacity: 0.35; }


  /* Trello's shape: the list above the title, the title with a status mark, one
     row of facts, then sections with an icon, a name and their own Edit. What is
     NOT copied is the row of quick-add buttons — Checklist, Attachment, Location
     are features we do not have, and a button that does nothing is worse than a
     missing one. */
  .lista {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.76rem;
    font-weight: 700;
  }
  .lista:hover { background: var(--hover); color: var(--text); }

  /* No status circle beside the title. Trello's marks a card complete; ours
     would duplicate the list picker directly above it, and a second control for
     one fact is one of them going stale. */
  .head { display: flex; align-items: center; gap: 0.6rem; }
  .ttl {
    width: 100%;
    padding: 0.2rem 0;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 1.15rem;
    font-weight: 700;
  }

  /* A grid, not a flex row: the three fields were sized by their content, so
     the objective was wide, the priority was a chip floating on its own and the
     date sat somewhere between them. Named columns give the row proportions
     that hold whatever is typed into it. */
  .facts {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 1fr) minmax(0, 1.2fr);
    gap: 0.75rem;
    align-items: end;
  }
  @media (max-width: 560px) {
    .facts { grid-template-columns: minmax(0, 1fr); }
  }
  .fact { display: flex; flex-direction: column; gap: 0.25rem; min-width: 0; }
  .cap {
    color: var(--faint);
    font-size: 0.7rem;
    font-weight: 700;
    letter-spacing: 0.03em;
  }
  .chip {
    padding: 0.25rem 0.55rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
    font-size: 0.82rem;
  }
  .pick { max-width: 15rem; }

  .sec { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.4rem; color: var(--faint); }
  .sec h3 { margin: 0; color: var(--text); font-size: 0.9rem; font-weight: 700; }
  .edit {
    margin-left: auto;
    padding: 0.2rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    color: var(--muted);
    font-size: 0.78rem;
  }
  .edit:hover { background: var(--hover); color: var(--text); }


  .ttl {
    width: 100%;
    padding: 0.2rem 0;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 1.05rem;
    font-weight: 700;
  }
  footer { display: flex; }
  .danger {
    padding: 0.35rem 0.6rem;
    border-radius: 8px;
    color: var(--faint);
    font-size: 0.82rem;
  }
  .danger:hover {
    color: var(--color-error-500);
    background: color-mix(in oklab, var(--color-error-500) 12%, transparent);
  }

  /* The map. Its cells are buttons: looking one up and choosing it are the same
     gesture, so the answer you just read is the one you can press. */
  .map { width: 100%; border-collapse: collapse; text-align: center; font-size: 0.8rem; }
  .map th { padding: 0.25rem; color: var(--faint); font-weight: 400; }
  .map tbody th { text-align: right; padding-right: 0.5rem; }
  .map td { padding: 0.2rem; }
  .map button { opacity: 0.55; transition: opacity 0.12s ease, outline-color 0.12s ease; }
  .map button:hover { opacity: 1; }

  /* The list is sized by what it SAYS. Zag anchors a combobox popup to the
     width of its field (`width: var(--reference-width)` on the positioner),
     which is right when the field is wide and absurd when it is not: the
     priority field is 124px, so "P1 crítica — operación detenida" wrapped onto
     four lines. `sameWidth: false` unpins it; the min keeps it from ever being
     narrower than the field it belongs to. */
  :global([data-scope='combobox'][data-part='content']) {
    min-width: max(var(--reference-width, 0px), 15rem);
    max-width: min(24rem, 90vw);
  }
  :global([data-scope='combobox'][data-part='item']) {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.35rem 0.6rem;
    border-radius: 7px;
    font-size: 0.84rem;
    line-height: 1.35;
  }
  :global([data-scope='combobox'][data-part='item'] .prio-chip) { flex: none; }
</style>
