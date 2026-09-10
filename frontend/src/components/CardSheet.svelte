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
    onpatch,
    choosePriority = true,
    writing = $bindable(false),
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
    /** Where an edited field GOES, when the card is a row on a server. Without
     *  it the sheet only edits the object it was handed, which is exactly right
     *  for the mock and quietly loses the edit anywhere else. */
    onpatch?: (id: string, fields: Record<string, unknown>) => void;
    /** Whether priority is TYPED IN here. It is, in the mock — that is what was
     *  asked for and what the map beside it is for. Against the real server it
     *  is not: there a thread's priority is derived from impact × urgency, and
     *  a dropdown that writes nowhere is a control that lies. */
    choosePriority?: boolean;
    /** Whether somebody is typing in here right now. The owner of the data
     *  reads it before refreshing the card from the server: everything else can
     *  be replaced under the sheet safely, text being typed cannot. */
    writing?: boolean;
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
  // Whether anybody has started writing in this description. It is what stops
  // an arriving document from overwriting what is being typed.
  let touched = $state(false);
  $effect(() => {
    if (!card) return;
    if (card.id !== seeded) {
      seeded = card.id;
      notes = card.notes ?? '';
      touched = false;
      return;
    }
    // The description can arrive AFTER the sheet opened: in the real planner it
    // is the thread's DOCUMENT, fetched when the card is opened. Take it when
    // it lands, unless somebody is already writing over it.
    if (!touched && !editingNotes && (card.notes ?? '') !== notes) notes = card.notes ?? '';
  });

  // Written back so the board shows what the sheet decided — only when nobody
  // else owns the write. With `onpatch` the owner holds the copy that counts,
  // and mirroring into this one would immediately look like an arrived
  // document and stop the real one from ever seeding.
  $effect(() => {
    if (card && !onpatch) card.notes = notes;
  });

  /** Set a field on the card AND, if somebody owns the data, on the server. */
  function set(fields: Partial<Card>) {
    if (!card) return;
    Object.assign(card, fields);
    onpatch?.(card.id, fields as Record<string, unknown>);
  }


  // The description edits in place, driven from its section header the way
  // Trello does it: reading is the default and "Editar" is a decision.
  let editingNotes = $state(false);

  let title = $state<HTMLInputElement | null>(null);
  let titleFocused = $state(false);
  $effect(() => {
    writing = editingNotes || titleFocused;
  });

  /** Put focus back inside the sheet after the editor is torn down.
   *
   *  Leaving edit mode DESTROYS the CodeMirror instance, and with it whatever
   *  had focus. The dialog watches for focus leaving itself and closes when it
   *  does, so ending an edit shut the card — which reads as the card refusing
   *  to be edited. */
  function keepFocus() {
    requestAnimationFrame(() => title?.focus({ preventScroll: true }));
  }
  // Whichever way the edit ended — the button, or Escape inside the field —
  // focus comes back here. Watching the flag rather than the button is what
  // covers the second one.
  let wasEditing = false;
  $effect(() => {
    if (wasEditing && !editingNotes) keepFocus();
    wasEditing = editingNotes;
  });

  const column = $derived(columns.find((c) => c.id === columnId));

  /** The map's labels, in the two words the server stores. It reads the axes
   *  from `impact` and `urgency`; the table says them in Spanish and at three
   *  levels, and this is the one place the two vocabularies meet. */
  const AXIS: Record<string, string> = {
    'Impacto alto': 'high',
    'Impacto medio': 'mid',
    'Impacto bajo': 'low',
    'Urgencia alta': 'high',
    Media: 'mid',
    Baja: 'low',
  };

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

  /** The picked day as `2026-09-15`.
   *
   *  NOT `valueAsString`, which is what this used and why nothing was ever
   *  saved: Zag formats that field for the LOCALE, so with `es-MX` it hands
   *  back `15/09/2026` — read out of its machine, where the default `format`
   *  builds an `Intl.DateTimeFormat` with 2-digit day and month. The server
   *  refused it, quietly, and the field went back to "sin fecha" on reload.
   *
   *  The value itself is a calendar date with three numbers on it, which is the
   *  same date in any language. */
  function isoDate(d?: { year: number; month: number; day: number }) {
    if (!d) return '';
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.year}-${pad(d.month)}-${pad(d.day)}`;
  }

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

<!-- Escape never closes this one. It hosts a text editor, and in vim that key
     means "leave insert mode"; a modal that also takes it is a modal that
     throws away what was being written. Closing is the ✕, a click outside, or
     the button that says so.

     Safe to set precisely because `lib/escape.ts` gets there first when the
     editor has focus: Zag answers `closeOnEscape: false` by calling
     `preventDefault()`, and CodeMirror skips its handlers on an event that
     carries that — so this flag alone would break vim, and the window-level
     handler alone would not stop a click-free Escape from closing the card. -->
<Dialog
  {open}
  closeOnEscape={false}
  onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Portal>
    <Dialog.Backdrop
      class="scrim"
      style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-center justify-center p-4"
      style="z-index: var(--z-drawer)">
      <Dialog.Content
        class="card bg-surface-100-900 w-full max-w-3xl space-y-4 p-4 shadow-xl {anim}">
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
              <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                bind:this={title}
                onfocus={() => (titleFocused = true)}
                onblur={() => (titleFocused = false)}
                class="ttl"
                value={card.title}
                oninput={(e) => (card.title = e.currentTarget.value)}
                onchange={(e) => set({ title: e.currentTarget.value })}
                aria-label="título" />
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
                  set({ obj: e.value[0] ? Number(e.value[0]) : undefined })}
                onOpenChange={() => (objItems = objectives.map(asItem))}
                onInputValueChange={onObjInput}
                placeholder="sin objetivo">
                <Combobox.Label class="cap">Objetivo</Combobox.Label>
                <Combobox.Control>
                  <Combobox.Input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
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
              {#if !choosePriority}
                <!-- Read-only, and it says where it comes from: the map below is
                     no longer something to consult before choosing, it is the
                     rule that produced this. -->
                <span class="cap">Prioridad</span>
                <p class="prio-read prio-{card.prio ?? ''}" title={card.prio ? meaning[card.prio] : ''}>
                  {#if card.prio}
                    <span class="prio-chip prio-{card.prio}">{card.prio}</span>
                    <span class="what">{meaning[card.prio] ?? ''}</span>
                  {:else}
                    <span class="what">sin impacto ni urgencia</span>
                  {/if}
                </p>
              {:else}
              <Combobox
                positioning={{ sameWidth: false }}
                collection={prioCollection}
                value={card.prio ? [card.prio] : []}
                onValueChange={(e: { value: string[] }) => set({ prio: e.value[0] ?? '' })}
                placeholder="sin prioridad">
                <Combobox.Label class="cap">Prioridad</Combobox.Label>
                <Combobox.Control class="prio-{card.prio ?? ''}">
                  <Combobox.Input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
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
              {/if}
            </div>
            <div class="fact">
              <!-- Keyed by the card, and UNCONTROLLED inside it.
                   Controlled, the picker only moves when the value it was
                   handed moves — so a click on a day was a no-op unless the
                   round trip through the server came back first, which is
                   exactly what it looked like: the calendar opened, a day did
                   nothing. Zag owns the selection while the sheet is open; the
                   key re-seeds it when a different card is opened. -->
              {#key card.id}
              <DatePicker
                defaultValue={due}
                onValueChange={(e: { value: { year: number; month: number; day: number }[] }) =>
                  set({ due: isoDate(e.value?.[0]) })}
                locale="es-MX"
                startOfWeek={1}>
                <DatePicker.Label class="cap">Entrega</DatePicker.Label>
                <DatePicker.Control class="dp-control">
                  <DatePicker.Input placeholder="sin fecha" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
                  <DatePicker.Trigger>🗓</DatePicker.Trigger>
                </DatePicker.Control>
                <!-- NOT portalled, unlike every other popup here. A modal
                     dialog turns off pointer events outside itself and hands
                     them back layer by layer; a calendar that lands on the body
                     is outside, so it drew fine and ignored every click. Kept
                     inside the dialog it is part of the layer that is active.
                     Zag positions it fixed anyway, so nothing clips it. -->
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
              </DatePicker>
              {/key}
            </div>
          </div>

          <section>
            <div class="sec">
              <AlignLeftIcon class="size-4" />
              <h3>Descripción</h3>
              <!-- "Listo" is the save. The description is a document on a server
                   in the real planner, and a field that writes on every
                   keystroke writes a commit per keystroke. -->
              <button
                class="edit"
                onclick={() => {
                  if (editingNotes) set({ notes });
                  else touched = true;
                  editingNotes = !editingNotes;
                  if (!editingNotes) keepFocus();
                }}>
                {editingNotes ? 'Listo' : 'Editar'}
              </button>
            </div>
            <MarkdownField
              bind:value={notes}
              bind:editing={editingNotes}
              chrome={false}
              {render}
              minHeight="22rem"
              placeholder="por qué existe, qué es verdad cuando esté hecho, el siguiente paso…" />
          </section>

          {#if priorityMap}
            <section>
              <div class="sec">
                <ListIcon class="size-4" />
                <h3>El mapa</h3>
              </div>
              <!-- What impact × urgency produces. Where the priority is chosen it
                   is what you consult first: «lo necesito urgente» se responde
                   con «¿pasa algo si no se hace hoy?». Where it is derived, it
                   is the rule that produced the chip above. -->
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
                          <!-- Where the priority is derived, a cell is not a
                               shortcut to a letter: it IS the pair — impact
                               across, urgency down — and picking one is how the
                               priority above changes at all. There was no other
                               way to set the two axes, so the chip never moved
                               off "sin impacto ni urgencia".

                               Marked only in that mode, and for the same
                               reason: the current cell is one pair, while three
                               different pairs give P3, so highlighting by
                               letter would point at two places you are not. -->
                          <button
                            class="prio-chip prio-{cell}"
                            class:here={!choosePriority &&
                              card.impact === AXIS[row[0]] &&
                              card.urgency === AXIS[priorityMap.cols[i]]}
                            title={choosePriority
                              ? `poner ${cell}`
                              : `${row[0]} × ${priorityMap.cols[i]} → ${cell}`}
                            onclick={() =>
                              set(
                                choosePriority
                                  ? { prio: cell }
                                  : { impact: AXIS[row[0]], urgency: AXIS[priorityMap.cols[i]] },
                              )}>{cell}</button>
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
  /* The same box as the two fields beside it. It is read-only, not absent: a
     bare line of text next to two boxed fields reads as something that failed
     to render, and the wrapping made the row three different heights. */
  /* The pair this card is on. A ring rather than a fill: the chip already
     carries the priority's colour, and filling it again would say the same
     thing twice. */
  .map .here {
    outline: 2px solid var(--text);
    outline-offset: 2px;
  }
  .prio-read {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 2.1rem;
    margin: 0;
    padding: 0.15rem 0.45rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    font-size: 0.8rem;
  }

  /* One line, cut with an ellipsis. What it means is a hint, not a paragraph:
     the sentence wrapped to two lines and pushed the row out of alignment. */
  .prio-read .what {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--faint);
  }

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
  /* Fill the column. Without a width the control is as wide as its text, so a
     2fr column held a narrow box with a stretch of nothing beside it — which
     read as a gap between the fields rather than as one field being wider. */
  .facts :global([data-scope='combobox'][data-part='root']),
  .facts :global([data-scope='date-picker'][data-part='root']) {
    min-width: 0;
    width: 100%;
  }
  .facts :global([data-scope='combobox'][data-part='control']),
  .facts :global([data-scope='date-picker'][data-part='control']) { width: 100%; }

  /* One label, whatever draws it. Two of these are Skeleton's `<label>` parts
     and one is our own span; left alone they sat at different heights and
     different sizes, which made the row look misaligned rather than deliberate. */
  .facts :global([data-part='label']),
  .facts .cap {
    display: block;
    margin: 0;
    color: var(--faint);
    font-size: 0.7rem;
    font-weight: 700;
    line-height: 1.3;
    letter-spacing: 0.03em;
  }
  /* The whole field takes the priority's colour — not a stripe down one edge.
     A 4px bar is a decoration you have to be told to read; a field tinted with
     the colour IS the priority, at a glance and from across the desk. Matched at
     the same weight as the rule that draws the box, because `.prio-P1` alone
     loses to `[data-scope][data-part]` and the tint never landed. */
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P1),
  .prio-read.prio-P1 { --p: var(--p1); }
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P2),
  .prio-read.prio-P2 { --p: var(--p2); }
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P3),
  .prio-read.prio-P3 { --p: var(--p3); }
  .prio-field :global([data-scope='combobox'][data-part='control'].prio-P4),
  .prio-read.prio-P4 { --p: var(--p4); }
  .prio-field :global([data-scope='combobox'][data-part='control'][class*='prio-P']),
  .prio-read[class*='prio-P'] {
    border-color: color-mix(in oklab, var(--p) 55%, transparent);
    background: color-mix(in oklab, var(--p) 12%, var(--surface));
  }
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
    /* Three equal columns. They are three facts of the same standing, and a row
       of three different widths reads as three different KINDS of thing. */
    grid-template-columns: repeat(3, minmax(0, 1fr));
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
  /* The objective used to be capped at 15rem, from back when this row was a
     flex line and the field would otherwise eat it. In a grid the column
     already says how wide it is, and the cap only left a strip of nothing
     between this field and the next — which reads as a broken layout, not as a
     narrow field. */

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
