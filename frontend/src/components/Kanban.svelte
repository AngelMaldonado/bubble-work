<script lang="ts" module>
  export type Card = {
    id: string;
    title: string;
    obj?: number;
    /** el NOMBRE del objetivo. El número (`obj`) es su posición en la lista, y
     *  cambia si alguien la reordena: una tarjeta que pasa a decir «O2» sin que
     *  nada de ella haya cambiado no dice qué objetivo es. */
    objName?: string;
    /** DERIVED from impact × urgency — see CardSheet. Stored so the board can
     *  show it without recomputing, never typed in by hand. */
    prio?: string;
    due?: string;
    impact?: string;
    urgency?: string;
    notes?: string;
    /** cuántos threads lleva dentro. No el detalle —eso es de quien opera— sino
     *  el tamaño: «esto son ocho piezas» y «esto es una» se planean distinto. */
    pieces?: number;
    /** where this work lives — the project, when the board spans several */
    where?: string;
    /** cuándo nació, ISO. La tarjeta lo dice como edad: «hace 3 meses» */
    born?: string;
    /** el id del workspace de la tarjeta. `where` es su NOMBRE, para leerlo;
     *  esto es lo que hace falta para guardar y resolver sus imágenes. */
    ws?: string;
    /** quién está a cargo: ids de persona (`bubbles.owners`) */
    owners?: string[];
  };
  export type Column = {
    id: string;
    name: string;
    cards: Card[];
    /** alguien ordenó esta columna a mano. Se dice en la pantalla, y con una
     *  salida: un orden que no se sabe que está puesto es un orden que no se
     *  puede quitar. */
    manual?: boolean;
    /** no es una fila, sino lo que queda sin ninguna (en el planeador, «Sin
     *  planear»): no se renombra, no se borra y no se mueve. Ofrecerlo sería un
     *  menú que no hace nada, o un arrastre que al recargar vuelve atrás. */
    locked?: boolean;
    /** nace estrecha. Para la columna de terminadas: lo hecho ya no pide nada,
     *  y abierto le roba ancho a lo que falta. Es sólo el valor INICIAL —
     *  quien la abra la deja abierta mientras mire este tablero. */
    collapsed?: boolean;
  };
</script>

<script lang="ts">
  // The orchestration board: high-level cards, one column per stage.
  //
  // Drag and drop is Atlassian's pragmatic-drag-and-drop, which is what Trello
  // itself runs on: 7 kB, no peer dependencies, and it owns the parts that are
  // genuinely hard — pointer capture, touch, autoscroll, and telling you WHICH
  // edge of a card you are nearest. What it deliberately does not own is the
  // state: moving a card is our reducer, not its.
  //
  // It loads on demand. A planner that is only being read should not download a
  // drag engine.
  import PlusIcon from '@lucide/svelte/icons/plus';
  import MoreIcon from '@lucide/svelte/icons/more-horizontal';
  import BoardCard from './BoardCard.svelte';
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import { limited } from '../lib/limits.svelte';
  import { tick } from 'svelte';

  let {
    columns = $bindable([]),
    onopen,
    onadd,
    ondeletecard,
    onaddcolumn,
    onrenamecolumn,
    ondeletecolumn,
    onmovecard,
    onautoorder,
    onmovecolumn,
    orderKey = '',
  }: {
    columns?: Column[];
    onopen?: (card: Card) => void;
    onadd?: (columnId: string) => void;
    /** throw one away from the board itself */
    ondeletecard?: (id: string) => void;
    /** Where a change to the COLUMNS goes, when they are rows on a server. In
     *  the planner they are the department's objectives; without these the
     *  board rearranges its own copy and nobody else ever sees it. */
    onaddcolumn?: () => void;
    onrenamecolumn?: (id: string, name: string) => void;
    ondeletecolumn?: (id: string) => void;
    /** una columna arrastrada a otro sitio: delante de `before`, o al final */
    onmovecolumn?: (id: string, before: string | null) => void;
    /** Where a DRAGGED card lands, when the columns are rows on a server.
     *  Without it the drop only rearranges this component's copy, which is
     *  right for the mock and, against a server, a move that looked like it
     *  happened until the next reload undid it. */
    /** una tarjeta soltada: a qué columna, y delante de cuál (null = al final).
     *  El `before` importa tanto como la columna — dentro de la misma, es lo
     *  ÚNICO que cambia. */
    onmovecard?: (cardId: string, columnId: string, before: string | null) => void;
    /** devolver una columna al orden automático, el que deriva la prioridad */
    onautoorder?: (columnId: string) => void;
    /** dónde recordar, en este navegador, el orden en que se ven las columnas.
     *  Vacío: el orden es el de `columns`. */
    orderKey?: string;
  } = $props();

  // ---- el orden de las columnas, por navegador ----
  //
  // Cada quien acomoda el tablero a su manera: mover una columna —también «Sin
  // planear» y la de la secuencia, que no se renombran ni se borran— cambia el
  // orden sólo en este navegador. No toca el orden de las etapas en el servidor,
  // que es el de todo el departamento: acomodar mi vista no es reordenar la de
  // los demás. Una columna nueva aparece donde le toca por su orden natural.
  /** ¿Lo que se arrastra es una columna? Por su marca, no por su id: «Sin
   *  planear» se llama `''`, y un id vacío leído como falso la dejaba sin
   *  poder soltarse en ningún sitio. */
  const isCol = (source: { data: Record<string | symbol, unknown> }) => source.data.isCol === true;
  // svelte-ignore state_referenced_locally
  let order = $state<string[]>(
    (() => {
      if (!orderKey) return [];
      try {
        const got = JSON.parse(localStorage.getItem(orderKey) ?? '[]');
        return Array.isArray(got) ? got.filter((x) => typeof x === 'string') : [];
      } catch {
        return [];
      }
    })(),
  );
  function saveOrder(ids: string[]) {
    order = ids;
    if (!orderKey) return;
    try {
      localStorage.setItem(orderKey, JSON.stringify(ids));
    } catch {
      // sin memoria, el orden dura lo que la pestaña
    }
  }
  /** El orden natural: las columnas como llegan. */
  const natural = $derived(columns.map((c) => c.id));
  /** Lo guardado primero; lo que no estaba, detrás de su vecino natural anterior. */
  const slots = $derived.by(() => {
    const exists = new Set(natural);
    const out = order.filter((id) => exists.has(id));
    for (let i = 0; i < natural.length; i++) {
      const id = natural[i];
      if (out.includes(id)) continue;
      let at = 0;
      for (let j = i - 1; j >= 0; j--) {
        const k = out.indexOf(natural[j]);
        if (k >= 0) {
          at = k + 1;
          break;
        }
      }
      out.splice(at, 0, id);
    }
    return out;
  });
  const colById = (id: string) => columns.find((c) => c.id === id);

  // Columns are the board's own shape, and it belongs to whoever runs the
  // board. "Por decidir / Planeado / En curso" is a good default and a terrible
  // law: every team's stages are the argument they had about how work moves.
  let renaming = $state<string | null>(null);
  let draft = $state('');

  function startRename(c: Column) {
    if (c.locked) return;
    renaming = c.id;
    draft = c.name;
  }
  function commitRename() {
    const col = columns.find((c) => c.id === renaming);
    // An empty name is not a rename; it is a deletion nobody asked for.
    if (col && draft.trim()) {
      if (onrenamecolumn) onrenamecolumn(col.id, draft.trim());
      else col.name = draft.trim();
    }
    renaming = null;
  }
  function addColumn() {
    if (onaddcolumn) return onaddcolumn();
    const id = 'col' + Date.now();
    columns = [...columns, { id, name: 'Columna nueva', cards: [] }];
    renaming = id;
    draft = 'Columna nueva';
  }
  function dropColumn(id: string) {
    if (ondeletecolumn) return ondeletecolumn(id);
    columns = columns.filter((c) => c.id !== id);
  }

  /** La columna que se arrastra, y el orden que tendría el tablero si se
   *  soltara ahora. La columna levantada se queda en ese sitio como hueco del
   *  mismo ancho: el tablero se abre para recibirla en vez de pedir que se
   *  apunte a una raya de 3px. */
  let liftingCol = $state<string | null>(null);

  /** Qué columnas ha abierto o cerrado quien mira, por encima de lo que la
   *  columna trae. No se guarda: es una decisión de este rato, no una
   *  preferencia — el orden de las columnas sí lo es, y por eso ese sí. */
  let toggled = $state<Record<string, boolean>>({});
  const shut = (col: Column) => toggled[col.id] ?? !!col.collapsed;
  let preview = $state<string[] | null>(null);
  const shown = $derived(preview ?? slots);

  /** Where a dragged card would land right now: column id, and the card it goes
   *  before (null = at the end). Drawn as a gap, so the board shows the result
   *  rather than asking you to imagine it. */
  let over = $state<{ col: string; before: string | null } | null>(null);
  let dragging = $state<string | null>(null);

  function move(cardId: string, toCol: string, before: string | null) {
    if (onmovecard) {
      // El dueño decide, y también dentro de la misma columna: ahí no cambia el
      // objetivo pero sí el sitio, y ese sitio se guarda. Antes esta rama se
      // iba sin hacer nada mientras la pantalla dibujaba el hueco — un
      // reordenado que se prometía y no ocurría.
      onmovecard(cardId, toCol, before);
      return;
    }
    let card: Card | undefined;
    const next = columns.map((c) => {
      const keep = c.cards.filter((x) => {
        if (x.id !== cardId) return true;
        card = x;
        return false;
      });
      return { ...c, cards: keep };
    });
    if (!card) return;
    const target = next.find((c) => c.id === toCol);
    if (!target) return;
    const at = before ? target.cards.findIndex((x) => x.id === before) : -1;
    if (at < 0) target.cards.push(card);
    else target.cards.splice(at, 0, card);
    columns = next;
  }

  /** Wires one card as both a drag source and a drop target. */
  function card(el: HTMLElement, c: Card, colId: string) {
    let stop: (() => void) | undefined;
    let live = true;
    Promise.all([
      import('@atlaskit/pragmatic-drag-and-drop/element/adapter'),
      import('@atlaskit/pragmatic-drag-and-drop/combine'),
      import('@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge'),
    ]).then(([{ draggable, dropTargetForElements }, { combine }, hitbox]) => {
      if (!live) return;
      stop = combine(
        draggable({
          element: el,
          getInitialData: () => ({ cardId: c.id, from: colId }),
          onDragStart: () => (dragging = c.id),
          onDrop: () => {
            dragging = null;
            over = null;
          },
        }),
        dropTargetForElements({
          element: el,
          // Sólo tarjetas: una COLUMNA arrastrada encima tiene que pasar de
          // largo hasta la columna, que es quien la recibe.
          canDrop: ({ source }) => !!source.data.cardId && source.data.cardId !== c.id,
          getData: ({ input, element }) =>
            // The hitbox says which edge the pointer is nearest, which is what
            // turns "over this card" into "before it" or "after it".
            hitbox.attachClosestEdge({ cardId: c.id, col: colId }, {
              input,
              element,
              allowedEdges: ['top', 'bottom'],
            }),
          onDrag: ({ self }) => {
            const edge = hitbox.extractClosestEdge(self.data);
            over = { col: colId, before: edge === 'bottom' ? nextOf(colId, c.id) : c.id };
          },
          onDragLeave: () => (over = null),
        }),
      );
    });
    return () => {
      live = false;
      stop?.();
    };
  }

  function nextOf(colId: string, cardId: string): string | null {
    const col = columns.find((x) => x.id === colId);
    const i = col?.cards.findIndex((x) => x.id === cardId) ?? -1;
    return col && i >= 0 ? (col.cards[i + 1]?.id ?? null) : null;
  }

  /** The header is the column's handle: dragging a column by its whole body
   *  would fight the cards inside it for the pointer. */
  function head(el: HTMLElement, id: string) {
    let stop: (() => void) | undefined;
    let live = true;
    import('@atlaskit/pragmatic-drag-and-drop/element/adapter').then(({ draggable }) => {
      if (!live) return;
      stop = draggable({
        element: el,
        getInitialData: () => ({ colId: id, isCol: true }),
      });
    });
    return () => {
      live = false;
      stop?.();
    };
  }

  /** El tablero entero recibe columnas: el hueco entre dos columnas no es de
   *  ninguna, y soltar justo ahí —donde se dibuja la barra— no hacía nada. */
  function board(el: HTMLElement) {
    let stop: (() => void) | undefined;
    let live = true;
    Promise.all([
      import('@atlaskit/pragmatic-drag-and-drop/element/adapter'),
      import('@atlaskit/pragmatic-drag-and-drop/combine'),
    ]).then(([{ dropTargetForElements, monitorForElements }, { combine }]) => {
      if (!live) return;
      stop = combine(
        dropTargetForElements({
          element: el,
          canDrop: ({ source }) => isCol(source),
          getData: () => ({ board: true }),
        }),
        monitorForElements({
          canMonitor: ({ source }) => isCol(source),
          onDragStart: ({ source }) => {
            liftingCol = source.data.colId as string;
            preview = [...slots];
          },
          onDrag: ({ location }) => aim(el, location.current.input.clientX),
          onDrop: ({ location }) => {
            // Fuera del tablero no cuenta: la columna vuelve a su sitio.
            if (preview && location.current.dropTargets.some((t) => t.data.board)) saveOrder(preview);
            reflow(el, null);
            liftingCol = null;
          },
        }),
      );
    });
    return () => {
      live = false;
      stop?.();
    };
  }

  /** Coloca el hueco según el puntero: va detrás de cada columna cuyo centro ya
   *  quedó a la izquierda. Se mide con `offsetLeft`, que no lleva la animación
   *  en curso, y sobre la disposición CON el hueco puesto: al cruzar el centro
   *  de una vecina, ésta salta al otro lado y su centro queda más allá del
   *  puntero, así que el hueco no tiembla entre dos sitios. */
  function aim(el: HTMLElement, x: number) {
    // `''` es «Sin planear»: sólo `null` es «nada levantado».
    const lifted = liftingCol;
    if (lifted === null || !preview) return;
    const base = el.getBoundingClientRect().left - el.scrollLeft;
    const others = preview.filter((id) => id !== liftingCol);
    let at = 0;
    for (const id of others) {
      const k = el.querySelector<HTMLElement>(`:scope > [data-slot="${CSS.escape(id)}"]`);
      if (k && base + k.offsetLeft + k.offsetWidth / 2 < x) at++;
    }
    const next = [...others.slice(0, at), lifted, ...others.slice(at)];
    if (next.some((id, i) => id !== preview![i])) reflow(el, next);
  }

  /** Cambia el orden visible y anima el salto (FLIP): cada columna parte de
   *  donde se veía y se desliza a su sitio nuevo. */
  async function reflow(el: HTMLElement, next: string[] | null) {
    const kids = [...el.querySelectorAll<HTMLElement>(':scope > [data-slot]')];
    const from = new Map(kids.map((k) => [k, k.getBoundingClientRect().left]));
    preview = next;
    await tick();
    const base = el.getBoundingClientRect().left - el.scrollLeft;
    for (const k of kids) {
      const dx = from.get(k)! - (base + k.offsetLeft);
      if (Math.abs(dx) < 1) continue;
      k.animate([{ transform: `translateX(${dx}px)` }, { transform: 'none' }], {
        duration: 200,
        easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
      });
    }
  }

  /** The column itself takes a drop, which is what makes an EMPTY column
   *  reachable and what catches the space below the last card. */
  function column(el: HTMLElement, colId: string) {
    let stop: (() => void) | undefined;
    let live = true;
    Promise.all([
      import('@atlaskit/pragmatic-drag-and-drop/element/adapter'),
      import('@atlaskit/pragmatic-drag-and-drop/combine'),
      import('@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge'),
    ]).then(([{ dropTargetForElements, monitorForElements }, { combine }, hitbox]) => {
      if (!live) return;
      stop = combine(
        dropTargetForElements({
          element: el,
          // Una columna arrastrada la recibe el tablero, no ésta.
          canDrop: ({ source }) => !!source.data.cardId,
          getData: () => ({ col: colId }),
          // A card sitting over a CARD has already said where it goes; the
          // column only answers for the empty space around them.
          onDragEnter: ({ location, source }) => {
            if (source.data.cardId && location.current.dropTargets.length === 1) over = { col: colId, before: null };
          },
          onDragLeave: () => (over = null),
        }),
        monitorForElements({
          // A column drag passes through here too; only a card is ours.
          canMonitor: ({ source }) => !!source.data.cardId,
          onDrop: ({ source, location }) => {
            const target = location.current.dropTargets[0];
            if (!target) return;
            // Soltada en la columna de la secuencia: no es una etapa, y la
            const to = (target.data.col as string) ?? colId;
            if (to !== colId) return; // one monitor acts, not five

            // La posición sale del destino sobre el que se SOLTÓ, no del último
            // `onDrag` que vimos pasar. Es el mismo bicho que ya estaba
            // arreglado para las columnas y que a las tarjetas nunca se le
            // aplicó: el puntero puede salir de una tarjeta camino del botón del
            // ratón, `onDragLeave` deja `over` en nulo, y la tarjeta aterrizaba
            // al final de la columna — o en ningún sitio— en vez de donde se
            // apuntaba.
            let before: string | null = null;
            const onCard = target.data.cardId as string | undefined;
            if (onCard) {
              const edge = hitbox.extractClosestEdge(target.data);
              before = edge === 'bottom' ? nextOf(to, onCard) : onCard;
            }
            move(source.data.cardId as string, to, before);
            over = null;
            dragging = null;
          },
        }),
      );
    });
    return () => {
      live = false;
      stop?.();
    };
  }
</script>

<div class="board" {@attach board}>
  {#each slots as slot (slot)}
    {#if colById(slot)}
    {@const col = colById(slot)!}
    <section
      class="col"
      class:over={over?.col === col.id}
      class:lifting={liftingCol === col.id}
      class:shut={shut(col)}
      data-slot={col.id}
      style:order={shown.indexOf(col.id)}
      {@attach (el) => column(el, col.id)}>
      {#if shut(col)}
        <!-- Plegada sigue siendo destino de caída: soltar algo aquí es darlo por
             terminado, que es el gesto que más se va a hacer con esta columna. -->
        <button
          class="shut-head"
          title="{col.name} ({col.cards.length}) — clic para abrir"
          onclick={() => (toggled[col.id] = false)}>
          <span class="shut-name">{col.name}</span>
          <span class="count">{col.cards.length}</span>
        </button>
      {:else}
      <header {@attach (el) => head(el, col.id)}>
        {#if renaming === col.id}
          <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
            class="rename"
            bind:value={draft} {@attach limited('stages.name')}
            onblur={commitRename}
            onkeydown={(e) => {
              if (e.key === 'Enter') commitRename();
              if (e.key === 'Escape') renaming = null;
            }}
            {@attach (el: HTMLInputElement) => { el.focus(); el.select(); }} />
        {:else}
          <button class="name" ondblclick={() => startRename(col)} title={col.locked ? col.name : 'doble clic para renombrar'}>
            {col.name}
          </button>
          <span class="count">{col.cards.length}</span>
          {#if col.collapsed}
            <button class="fold" title="Plegar la columna" onclick={() => (toggled[col.id] = true)}>
              ›
            </button>
          {/if}
          {#if col.manual}
            <!-- Que se VEA que el orden es de alguien. La columna ordenada sola
                 no dice nada: lo normal no necesita etiqueta. -->
            <span class="by-hand" title="Ordenada a mano. Deshazlo desde el menú.">a mano</span>
          {/if}
          {#if !col.locked || (col.manual && onautoorder)}
          <Menu
            onSelect={(e: { value: string }) => {
              if (e.value === 'rename') startRename(col);
              if (e.value === 'auto') onautoorder?.(col.id);
              if (e.value === 'delete') dropColumn(col.id);
            }}>
            <Menu.Trigger>
              <span class="more" title="acciones de la columna"><MoreIcon class="size-4" /></span>
            </Menu.Trigger>
            <Portal>
              <Menu.Positioner>
                <Menu.Content>
                  {#if !col.locked}
                    <Menu.Item value="rename"><Menu.ItemText>Renombrar</Menu.ItemText></Menu.Item>
                  {/if}
                  {#if col.manual && onautoorder}
                    <Menu.Item value="auto">
                      <Menu.ItemText>Volver al orden automático</Menu.ItemText>
                    </Menu.Item>
                  {/if}
                  {#if !col.locked}
                    <Menu.Item value="delete"><Menu.ItemText>Eliminar la columna</Menu.ItemText></Menu.Item>
                  {/if}
                </Menu.Content>
              </Menu.Positioner>
            </Portal>
          </Menu>
          {/if}
        {/if}
      </header>

      <div class="cards">
        {#each col.cards as c (c.id)}
          {#if over?.col === col.id && over.before === c.id}
            <div class="gap" aria-hidden="true"></div>
          {/if}
          <BoardCard
            card={c}
            lifting={dragging === c.id}
            attach={(el) => card(el, c, col.id)}
            onopen={() => onopen?.(c)}
            ondelete={ondeletecard ? () => ondeletecard?.(c.id) : undefined} />
        {/each}
        {#if over?.col === col.id && over.before === null}
          <div class="gap" aria-hidden="true"></div>
        {/if}
      </div>

      <button class="add" onclick={() => onadd?.(col.id)}>
        <PlusIcon class="size-4" /> tarjeta
      </button>
      {/if}
    </section>
    {/if}
  {/each}


  <!-- Adding a column is the one action the board always offers, so it sits at
       the end of the columns rather than hiding in a menu. -->
  <button class="add-col" onclick={addColumn}>
    <PlusIcon class="size-4" /> columna
  </button>
</div>

<style>
  .board {
    /* referencia de `offsetLeft` para medir dónde cae una columna */
    position: relative;
    display: flex;
    gap: 0.85rem;
    height: 100%;
    padding: 0.9rem;
    overflow-x: auto;
    align-items: flex-start;
  }
  /* Plegada: lo que mide su botón, no una columna vacía. Sigue siendo destino
     de caída — soltar algo aquí es darlo por terminado. */
  .col.shut {
    flex: 0 0 auto;
    min-height: 0;
    padding: 0.35rem;
  }
  .shut-head {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.3rem;
    color: var(--muted);
    font-size: 0.78rem;
    font-weight: 700;
    white-space: nowrap;
  }
  .shut-head:hover { color: var(--text); }
  .fold {
    padding: 0 0.3rem;
    border-radius: 6px;
    color: var(--faint);
    font-size: 0.9rem;
    line-height: 1;
  }
  .fold:hover { background: var(--hover); color: var(--text); }

  .col {
    flex: 0 0 272px;
    display: flex;
    flex-direction: column;
    max-height: 100%;
    padding: 0.6rem;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--surface);
  }
  /* The column you are over lights up as a whole, so the target is legible even
     when the gap is off screen. */
  .col.over { border-color: color-mix(in oklab, var(--accent) 55%, transparent); }
  .col > header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.1rem 0.35rem 0.55rem;
    cursor: grab;
  }
  .col > header .name { text-align: left; }
  .more {
    display: grid;
    place-content: center;
    width: 22px;
    height: 22px;
    border-radius: 6px;
    color: var(--faint);
    opacity: 0;
    transition: opacity 0.12s ease;
  }
  .col:hover .more,
  .col:focus-within .more { opacity: 1; }
  .more:hover { background: var(--hover); color: var(--text); }
  .rename {
    width: 100%;
    padding: 0.2rem 0.35rem;
    border: 1px solid var(--accent);
    border-radius: 7px;
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.78rem;
  }

  /* La columna levantada ES el hueco donde caerá: del mismo ancho, con borde
     de trazos y su contenido apagado. Moverse con `order` en vez de mover el
     DOM deja en paz al elemento que el navegador está arrastrando. */
  .add-col { order: 9999; }
  .col.lifting {
    border: 2px dashed color-mix(in oklab, var(--accent) 70%, transparent);
    background: color-mix(in oklab, var(--accent) 8%, transparent);
  }
  .col.lifting > :global(*) { opacity: 0.2; }

  .add-col {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    align-self: flex-start;
    padding: 0.55rem 0.7rem;
    border: 1px dashed var(--line);
    border-radius: 12px;
    color: var(--faint);
    font-size: 0.82rem;
  }
  .add-col:hover { background: var(--hover); color: var(--text); }
  .name {
    font-size: 0.72rem;
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .count {
    margin-left: auto;
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--faint);
    font-size: 0.68rem;
  }

  .cards {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-height: 0.5rem;
    overflow-y: auto;
  }
  /* The landing place, drawn: the board shows the result instead of asking you
     to imagine it. */
  .gap {
    height: 3px;
    border-radius: 999px;
    background: var(--accent);
  }

  .add {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin-top: 0.45rem;
    padding: 0.4rem 0.5rem;
    border-radius: 8px;
    color: var(--faint);
    font-size: 0.8rem;
  }
  .add:hover { background: var(--hover); color: var(--text); }

  /* La etiqueta de «a mano». Discreta: es una nota al pie del nombre de la
     columna, no un aviso — nada va mal por haber ordenado uno mismo. */
  .by-hand {
    padding: 0 0.3rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--faint);
    font-size: 0.62rem;
    letter-spacing: 0.02em;
  }

</style>
