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
    Dialog,
    FloatingPanel,
    Menu,
    Portal,
    useListCollection,
    type ComboboxRootProps,
  } from '@skeletonlabs/skeleton-svelte';
  import XIcon from '@lucide/svelte/icons/x';
  import ChevronIcon from '@lucide/svelte/icons/chevron-down';
  import AlignLeftIcon from '@lucide/svelte/icons/align-left';
  import UsersIcon from '@lucide/svelte/icons/users';
  import MarkdownField from './MarkdownField.svelte';
  import PriorityBadge from './PriorityBadge.svelte';
  import { bandFace, bandName } from '../lib/bands';
  import type { Lifecycle } from '../lib/api';
  import { limited, tooLong } from '../lib/limits.svelte';
  import type { Card } from './Kanban.svelte';
  import Crumbs from './Crumbs.svelte';
  import DueDate from './DueDate.svelte';

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
  export type SheetThread = {
    id: string;
    seq: number;
    name: string;
    lifecycle: Lifecycle;
    priority?: string;
    owner?: string;
    age?: string;
  };

  let {
    open = $bindable(false),
    card = $bindable(null),
    objectives = [],
    priorityMap,
    render,
    onattach,
    columns = [],
    columnId = '',
    onmove,
    ondelete,
    onpatch,
    choosePriority = true,
    writing = $bindable(false),
    loadPeople,
    threadsOf,
    canPrioritize = false,
    onthreadpriority,
    onnewthread,
    onopenthread,
    face = $bindable<'plan' | 'hilos'>('plan'),
    backScroll = 0,
  }: {
    open?: boolean;
    card?: Card | null;
    objectives?: Objective[];
    priorityMap?: { cols: string[]; rows: string[][] };
    /** markdown → html; the server in the real app */
    /** markdown → html. Recibe la tarjeta para que quien llama resuelva sus
     *  imágenes contra el sitio donde viven: `assets/x.png` sólo existe dentro de
     *  un workspace. */
    render?: (md: string, card?: Card | null) => string | Promise<string>;
    /** guardar una imagen pegada en esta tarjeta y decir cómo se la cita */
    onattach?: (card: Card, file: File) => Promise<{ path: string } | null | void>;
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
    /** quién puede estar a cargo de esta tarjeta: el roster de su proyecto.
     *  Sin esto no se ofrece elegir responsables. */
    loadPeople?: (card: Card) => Promise<{ id: string; name: string; avatar?: string }[]>;
    /** los hilos de esta burbuja, para planearlos en la otra cara de la tarjeta */
    threadsOf?: (card: Card) => SheetThread[];
    /** quien mira puede cambiar la prioridad de un hilo (el lead global) */
    canPrioritize?: boolean;
    onthreadpriority?: (threadId: string, priority: string) => void;
    /** crear un hilo en esta burbuja con el nombre escrito aquí */
    onnewthread?: (card: Card, name: string) => void | Promise<void>;
    /** abrir un hilo a pantalla completa; `scroll` es dónde estaba la lista */
    onopenthread?: (card: Card, thread: SheetThread, scroll: number) => void;
    /** qué cara se ve: el plan de la burbuja o sus hilos */
    face?: 'plan' | 'hilos';
    /** al volver de un hilo: el scroll de la lista donde se quedó */
    backScroll?: number;
  } = $props();

  // ---- la otra cara: los hilos ----
  //
  // Planear una burbuja es también planear sus piezas. «Hilos» le da la vuelta a
  // la tarjeta y enseña lo mismo que el cajón de la burbuja en el board, con la
  // prioridad de cada hilo a mano. Voltearla y no abrir otro diálogo: es la
  // misma burbuja vista por detrás, y el plan sigue a un clic.
  const threadRows = $derived(card && threadsOf ? threadsOf(card) : []);
  let contentEl = $state<HTMLElement | null>(null);
  async function flip(to: 'plan' | 'hilos') {
    if (face === to) return;
    const el = contentEl;
    const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    if (!el || reduce || !el.animate) {
      face = to;
      return;
    }
    await el.animate(
      [{ transform: 'perspective(1400px) rotateY(0deg)' }, { transform: 'perspective(1400px) rotateY(90deg)' }],
      { duration: 140, easing: 'ease-in' },
    ).finished;
    face = to;
    el.scrollTop = 0;
    el.animate(
      [{ transform: 'perspective(1400px) rotateY(-90deg)' }, { transform: 'perspective(1400px) rotateY(0deg)' }],
      { duration: 160, easing: 'ease-out' },
    );
  }
  // Volver de un hilo deja la lista donde estaba.
  $effect(() => {
    if (face === 'hilos' && backScroll && contentEl) {
      const el = contentEl;
      requestAnimationFrame(() => (el.scrollTop = backScroll));
    }
  });
  // Otra tarjeta empieza por su plan.
  let faceFor = '';
  $effect(() => {
    const id = card?.id ?? '';
    if (id !== faceFor) {
      if (faceFor) face = 'plan';
      faceFor = id;
    }
  });

  let naming = $state(false);
  let fresh = $state('');
  async function createThread() {
    const name = fresh.trim();
    if (!name || !card) return;
    fresh = '';
    naming = false;
    await onnewthread?.(card, name);
  }

  // ---- responsables ----
  //
  // Los mismos que «a cargo» en el cajón de la burbuja: es el mismo campo
  // (`bubbles.owners`), y quien planea es quien suele decidir a quién le toca.
  // El roster es el del PROYECTO de la tarjeta: el planeador mezcla proyectos,
  // y poner a cargo a alguien que no puede ver la burbuja sería una promesa
  // que no puede cumplir.
  type Someone = { id: string; name: string; avatar?: string };
  let people = $state<Someone[]>([]);
  let peopleFor = '';
  $effect(() => {
    const c = card;
    if (!c || !loadPeople || c.id === peopleFor) return;
    peopleFor = c.id;
    people = [];
    loadPeople(c)
      .then((rows) => {
        if (card?.id === c.id) people = rows;
      })
      .catch(() => (people = []));
  });
  let hunting = $state('');
  const shortlist = $derived(
    people.filter((p) => p.name.toLowerCase().includes(hunting.trim().toLowerCase())),
  );
  const roster = $derived(
    useListCollection({
      items: shortlist,
      itemToString: (p: Someone) => p.name,
      itemToValue: (p: Someone) => p.id,
    }),
  );
  const someone = (id: string) => people.find((p) => p.id === id);

  // ---- el mapa, en un panel flotante ----
  //
  // Estaba debajo de todo, como una sección más, y era lo que empujaba la
  // tarjeta fuera de la pantalla. Se consulta al decidir la prioridad, así que
  // se abre DESDE la prioridad, junto a ella, y se puede arrastrar para ver la
  // tarjeta debajo. Elegir una casilla la aplica y lo cierra.
  let mapOpen = $state(false);
  let mapTrigger = $state<HTMLElement | null>(null);
  // Portado a <body>: fuera del contenido del diálogo. Sin esto, un clic en el
  // panel contaba como un clic FUERA de la tarjeta y la cerraba.
  const inPanel = (target: EventTarget | null) =>
    target instanceof Element && !!target.closest('[data-scope="floating-panel"]');

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
  // Empezar a editar es empezar a escribir, entre por donde entre: «Editar», un
  // clic en el campo vacío o un doble clic en lo renderizado. Sólo lo marcaba
  // «Editar», y entrando por el campo, al terminar, el efecto de arriba tomaba
  // lo escrito por un documento que llegaba tarde y lo pisaba con el vacío del
  // servidor — así que no había nada que guardar.
  $effect(() => {
    if (editingNotes) touched = true;
  });

  // Guardar la descripción es «Listo», pero no SÓLO «Listo»: la edición también
  // termina con Escape dentro del campo o cerrando la tarjeta, y en esos dos
  // caminos lo escrito se quedaba en la pantalla sin llegar nunca al servidor
  // — hasta recargar, que es cuando alguien descubre que no estaba.
  //
  // Lo que no cabe no se manda (el servidor rechazaría la escritura entera), y
  // la tarjeta no se cierra con ello dentro: cerrarla sería perderlo callando.
  const notesBlocked = $derived(tooLong('bubbles.brief', notes));
  const titleBlocked = $derived(card ? tooLong('bubbles.name', card.title) : null);
  const blocked = $derived(titleBlocked ?? notesBlocked);
  let refused = $state(false);
  $effect(() => {
    if (!blocked) refused = false;
  });

  /** Manda la descripción si cambió y cabe. `false` si hay algo sin guardar. */
  function commitNotes(): boolean {
    if (!card) return true;
    if (notes === (card.notes ?? '')) return true;
    if (notesBlocked) return false;
    set({ notes });
    return true;
  }
  /** Cerrar, o decir por qué no. */
  function leave(): boolean {
    if (commitNotes() && !titleBlocked) return true;
    refused = true;
    return false;
  }

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
    if (wasEditing && !editingNotes) {
      commitNotes();
      keepFocus();
    }
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
  onInteractOutside={(e: { detail?: { originalEvent?: Event }; target?: EventTarget | null; preventDefault: () => void }) => {
    if (inPanel(e.detail?.originalEvent?.target ?? e.target ?? null)) e.preventDefault();
  }}
  onFocusOutside={(e: { detail?: { originalEvent?: Event }; target?: EventTarget | null; preventDefault: () => void }) => {
    if (inPanel(e.detail?.originalEvent?.target ?? e.target ?? null)) e.preventDefault();
  }}
  onOpenChange={(e: { open: boolean }) => {
    if (!e.open && !leave()) return;
    open = e.open;
  }}>
  <Portal>
    <Dialog.Backdrop
      class="scrim"
      style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-center justify-center p-4"
      style="z-index: var(--z-drawer)">
      <Dialog.Content
        {@attach (el: HTMLElement) => {
          contentEl = el;
          return () => (contentEl = null);
        }}
        class="card bg-surface-100-900 w-full max-w-3xl max-h-[calc(100dvh-2rem)] space-y-4 overflow-y-auto p-4 shadow-xl {anim}">
        {#if card}
          {#if refused && blocked}
            <p class="unsaved" role="alert">No se guardó ni se cerró: {blocked}</p>
          {/if}
          <!-- Where it is, first — Trello puts the list above the title because
               a card's column is the loudest thing about it. -->
          <div class="toprow">
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
            {#if threadsOf}
              <!-- Le da la vuelta a la tarjeta: detrás del plan, sus hilos. -->
              <button class="flipbtn" class:on={face === 'hilos'} onclick={() => flip(face === 'plan' ? 'hilos' : 'plan')}>
                {face === 'plan' ? `🧵 Hilos (${threadRows.length})` : '← Plan'}
              </button>
            {/if}
          </div>

          <Crumbs parts={[card.where]} />

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
                {@attach limited('bubbles.name')}
                aria-label="título" />
            </Dialog.Title>
            <Dialog.CloseTrigger class="btn-icon hover:preset-tonal">
              <XIcon class="size-4" />
            </Dialog.CloseTrigger>
          </header>

          {#if face === 'hilos' && threadsOf}
            <!-- La otra cara: los hilos de esta burbuja, como en su cajón del
                 board, con la prioridad de cada uno a mano. -->
            <section class="hilos">
              <p class="faint text-xs">{threadRows.length} hilos · el más reciente primero</p>
              <ul class="hlist">
                {#each threadRows as t (t.id)}
                  <li class="htile band-{t.lifecycle}">
                    <button
                      class="hopen"
                      onclick={() => card && onopenthread?.(card, t, contentEl?.scrollTop ?? 0)}>
                      <span class="hline">
                        <span class="faint tabular-nums">#{t.seq}</span>
                        <span class="min-w-0 flex-1 truncate">{t.name}</span>
                      </span>
                      <span class="faint hsub">
                        {bandFace(t.lifecycle)} {bandName(t.lifecycle)}{t.owner ? ` · ${t.owner}` : ''}{t.age ? ` · ${t.age}` : ''}
                      </span>
                    </button>
                    <PriorityBadge
                      size="xs"
                      value={t.priority ?? ''}
                      canEdit={canPrioritize && !!onthreadpriority}
                      onchange={(p) => onthreadpriority?.(t.id, p)} />
                  </li>
                {:else}
                  <li class="faint text-sm">Esta burbuja todavía no tiene hilos.</li>
                {/each}
              </ul>
              {#if onnewthread}
                {#if naming}
                  <input
                    class="input"
                    placeholder="¿Cómo se llama?"
                    autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                    bind:value={fresh}
                    {@attach limited('threads.name')}
                    {@attach (el: HTMLInputElement) => el.focus()}
                    onblur={() => (naming = false)}
                    onkeydown={(e) => {
                      if (e.key === 'Enter') createThread();
                      if (e.key === 'Escape') {
                        e.stopPropagation();
                        naming = false;
                      }
                    }} />
                {:else}
                  <button class="btn btn-sm w-full preset-tonal-surface" onclick={() => (naming = true)}>+ hilo</button>
                {/if}
              {/if}
            </section>
          {:else}
          <!-- The card's facts, in one row of chips. Priority has no "+" and no
               dropdown on purpose: it is the only one here that is DERIVED. -->
          <div class="facts">
            <div class="fact pick">
              <Combobox
                openOnClick
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
                <!-- Se ELIGE en el mapa: la prioridad sale de impacto × urgencia,
                     y el mapa es la única forma de decir los dos a la vez. -->
                <button
                  bind:this={mapTrigger}
                  class="prio-read prio-{card.prio ?? ''}"
                  class:on={mapOpen}
                  title={priorityMap ? 'elegir en el mapa' : card.prio ? meaning[card.prio] : ''}
                  disabled={!priorityMap}
                  onclick={() => (mapOpen = !mapOpen)}>
                  {#if card.prio}
                    <span class="prio-chip prio-{card.prio}">{card.prio}</span>
                    <span class="what">{meaning[card.prio] ?? ''}</span>
                  {:else}
                    <span class="what">sin impacto ni urgencia</span>
                  {/if}
                  {#if priorityMap}<ChevronIcon class="ml-auto size-3.5 shrink-0" />{/if}
                </button>
              {:else}
              <Combobox
                openOnClick
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
              <!-- La fecha es de la BURBUJA: cuándo tiene que estar este cuerpo
                   de trabajo. Cómo se reparte entre sus threads es de quien
                   ejecuta. -->
              {#key card.id}
              <!-- El mismo calendario que la entrega de un hilo. Era cincuenta
                   líneas de marcado copiadas, y dos copias divergen a la primera
                   corrección. -->
              <DueDate
                label="Entrega"
                value={card.due ?? ''}
                onchange={(d) => set({ due: d })} />
              {/key}
            </div>
          </div>

          {#if loadPeople}
            <section>
              <div class="sec">
                <UsersIcon class="size-4" />
                <h3>Responsables</h3>
              </div>
              <div class="owners">
                <Combobox
                  openOnClick
                  class="min-w-0 flex-1"
                  multiple
                  placeholder={(card.owners ?? []).length ? 'agregar a alguien…' : 'nadie a cargo'}
                  collection={roster}
                  value={card.owners ?? []}
                  inputValue={hunting}
                  onInputValueChange={(e: { inputValue: string }) => (hunting = e.inputValue)}
                  onOpenChange={() => (hunting = '')}
                  onValueChange={(e: { value: string[] }) => set({ owners: e.value })}>
                  <Combobox.Control>
                    <Combobox.Input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
                    <Combobox.Trigger />
                  </Combobox.Control>
                  <Portal>
                    <Combobox.Positioner>
                      <Combobox.Content class="who-list">
                        {#each shortlist as p (p.id)}
                          <Combobox.Item item={p}>
                            <Combobox.ItemText>{p.name}</Combobox.ItemText>
                            <Combobox.ItemIndicator />
                          </Combobox.Item>
                        {:else}
                          <p class="faint px-2 py-1 text-xs">
                            {people.length ? 'nadie más con ese nombre' : 'nadie en este proyecto todavía'}
                          </p>
                        {/each}
                      </Combobox.Content>
                    </Combobox.Positioner>
                  </Portal>
                </Combobox>
              </div>
              {#if (card.owners ?? []).length}
                <ul class="chips">
                  {#each card.owners ?? [] as id (id)}
                    <li class="chip">
                      {#if someone(id)?.avatar}<img class="chip-face" src={someone(id)?.avatar} alt="" />{/if}
                      {someone(id)?.name ?? '…'}
                      <button
                        aria-label="quitar a {someone(id)?.name ?? 'esta persona'}"
                        onclick={() => set({ owners: (card!.owners ?? []).filter((x) => x !== id) })}>×</button>
                    </li>
                  {/each}
                </ul>
              {:else}
                <p class="faint mt-1 text-xs">nadie a cargo · si se enfría, cae a 🪦</p>
              {/if}

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
                  // «Listo» con algo que no cabe se queda editando: el aviso
                  // debajo del campo dice por cuánto se pasa.
                  if (editingNotes && notesBlocked) {
                    refused = true;
                    return;
                  }
                  // Terminar la edición guarda (ver el efecto de `wasEditing`).
                  editingNotes = !editingNotes;
                }}>
                {editingNotes ? 'Listo' : 'Editar'}
              </button>
            </div>
            <MarkdownField
              bind:value={notes}
              bind:editing={editingNotes}
              chrome={false}
              limit="bubbles.brief"
              render={render ? (md) => render(md, card) : undefined}
              onattach={onattach && card ? (f) => onattach(card!, f) : undefined}
              minHeight="22rem"
              placeholder="el plan de esto: por qué, qué incluye, lo que se sabe — pega imágenes o escribe /mermaid" />
          </section>

            </section>
          {/if}

          <footer>
            <button class="danger" onclick={() => { ondelete?.(card.id); open = false; }}>
              🗑 borrar
            </button>
          </footer>
          {/if}
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

{#if priorityMap && card}
  <!-- El mapa, flotando junto a la prioridad. Arrastrable para destapar lo que
       tapa; sin redimensionar, porque su tamaño es el de la tabla. -->
  <FloatingPanel
    open={mapOpen}
    onOpenChange={(e: { open: boolean }) => (mapOpen = e.open)}
    resizable={false}
    defaultSize={{ width: 380, height: 188 }}
    getAnchorPosition={({ triggerRect }: { triggerRect: DOMRect | null }) => {
      const r = mapTrigger?.getBoundingClientRect() ?? triggerRect;
      if (!r) return { x: 24, y: 24 };
      const x = Math.min(Math.max(8, r.left), window.innerWidth - 388);
      const below = r.bottom + 6;
      const y = below + 188 > window.innerHeight ? Math.max(8, r.top - 194) : below;
      return { x, y };
    }}>
    <Portal>
      <FloatingPanel.Positioner class="prio-panel">
        <FloatingPanel.Content>
          <FloatingPanel.DragTrigger>
            <FloatingPanel.Header>
              <FloatingPanel.Title>El mapa de prioridad</FloatingPanel.Title>
              <FloatingPanel.Control>
                <FloatingPanel.CloseTrigger aria-label="cerrar el mapa"><XIcon class="size-4" /></FloatingPanel.CloseTrigger>
              </FloatingPanel.Control>
            </FloatingPanel.Header>
          </FloatingPanel.DragTrigger>
          <FloatingPanel.Body>
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
                              onclick={() => {
                                set(
                                  choosePriority
                                    ? { prio: cell }
                                    : { impact: AXIS[row[0]], urgency: AXIS[priorityMap.cols[i]] },
                                );
                                mapOpen = false;
                              }}>{cell}</button>
                          </td>
                        {/each}
                      </tr>
                    {/each}
                  </tbody>
                </table>
          </FloatingPanel.Body>
        </FloatingPanel.Content>
      </FloatingPanel.Positioner>
    </Portal>
  </FloatingPanel>
{/if}

<style>
  .unsaved {
    margin: 0;
    padding: 0.45rem 0.7rem;
    border-radius: 8px;
    font-size: 0.82rem;
    color: var(--color-error-500);
    background: color-mix(in oklab, var(--color-error-500) 10%, transparent);
    border: 1px solid color-mix(in oklab, var(--color-error-500) 40%, transparent);
    animation: limit-nudge 0.32s ease;
  }
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
  .toprow { display: flex; align-items: center; gap: 0.5rem; }
  .flipbtn {
    margin-left: auto;
    padding: 0.2rem 0.65rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.8rem;
  }
  .flipbtn:hover, .flipbtn.on { color: var(--text); background: var(--hover); }
  .hilos { display: flex; flex-direction: column; gap: 0.5rem; }
  .hlist { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 0.15rem; }
  .htile { display: flex; align-items: center; gap: 0.5rem; padding-right: 0.4rem; border-radius: 9px; }
  .htile:hover { background: var(--hover); }
  .hopen { display: flex; flex: 1; min-width: 0; flex-direction: column; padding: 0.45rem 0.5rem; text-align: left; }
  .hline { display: flex; align-items: center; gap: 0.5rem; font-size: 0.88rem; }
  .hsub { display: block; margin-top: 0.1rem; overflow: hidden; font-size: 0.74rem; text-overflow: ellipsis; white-space: nowrap; }

  button.prio-read { width: 100%; text-align: left; cursor: pointer; }
  button.prio-read:disabled { cursor: default; }
  button.prio-read:not(:disabled):hover,
  button.prio-read.on { border-color: color-mix(in oklab, var(--text) 35%, var(--line)); }

  /* Responsables: el mismo control y las mismas fichas que «a cargo» en el
     cajón de la burbuja. */
  .owners :global([data-scope='combobox'][data-part='root']) { min-width: 0; width: 100%; }
  .owners :global([data-scope='combobox'][data-part='control']) {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    min-height: 2.1rem;
    padding: 0.1rem 0.4rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
  }
  .owners :global([data-scope='combobox'][data-part='control']:focus-within) {
    border-color: color-mix(in oklab, var(--accent, var(--text)) 60%, transparent);
  }
  /* El campo de dentro NO es otra caja: Skeleton estiliza el `input` con su
     propio borde y fondo, y dentro de un control que ya es la caja se leía
     como una caja dibujada dentro de otra. */
  .owners :global(input) {
    min-width: 0;
    flex: 1;
    padding: 0.15rem 0.25rem;
    border: none;
    border-radius: 0;
    background: transparent;
    box-shadow: none;
    color: var(--text);
    font-size: 0.85rem;
    outline: none;
  }
  .owners :global([data-scope='combobox'][data-part='trigger']) {
    flex: none;
    color: var(--faint);
    font-size: 0.8rem;
  }
  .chips { display: flex; flex-wrap: wrap; gap: 0.35rem; margin: 0.6rem 0 0; padding: 0; list-style: none; }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.1rem 0.35rem 0.1rem 0.5rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.76rem;
  }
  .chip-face { width: 16px; height: 16px; margin-left: -0.3rem; border-radius: 999px; object-fit: cover; }
  .chip button { color: var(--faint); font-size: 0.9rem; line-height: 1; }
  .chip button:hover { color: var(--text); }

  /* El panel del mapa: por encima de la tarjeta, que ya está por encima del
     board. */
  /* `pointer-events: auto` porque la tarjeta es un diálogo modal, y Zag le
     quita los eventos de puntero a todo `body` mientras está abierta; el panel
     vive portado ahí y, sin esto, se vería y no se podría pulsar. */
  :global(.prio-panel) { z-index: calc(var(--z-drawer) + 2); pointer-events: auto; }
  :global(.prio-panel [data-part='body']) { padding: 0.5rem 0.75rem 0.6rem; overflow: hidden; }
  :global(.prio-panel [data-part='drag-trigger']) { cursor: grab; }

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
