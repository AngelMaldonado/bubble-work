<script lang="ts">
  import { teams } from '../lib/teams.svelte';
  import Icon from './Icon.svelte';
  // The planner — the strategic layer, in one screen.
  //
  // Una vista a la vez: inbox, calendario o canvas de secuencia, elegidos
  // desde el HUD. Objetivos y prioridades son modales: se acuerdan y consultan,
  // no compiten por el espacio de la vista de trabajo.
  //
  // Focused, like a thread or the wiki: no project column, its own way back.
  import { Dialog, Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import XIcon from '@lucide/svelte/icons/x';
  import type { Card } from '../lib/cards';
  import InboxPool from './InboxPool.svelte';
  import type { SheetThread } from './CardSheet.svelte';
  import { plannerReturn } from '../lib/filters.svelte';
  import CardSheet from './CardSheet.svelte';
  import InboxSheet, { type Note } from './InboxSheet.svelte';
  import type { Snippet } from 'svelte';
  import PlannerCalendar, { type CalEvent } from './PlannerCalendar.svelte';
  import ThemeToggle from './ThemeToggle.svelte';
  import { limited } from '../lib/limits.svelte';

  // Skeleton ships no CSS for Dialog; its parts are styled with utilities, and
  // this is the shape its own documentation uses.
  const anim =
    'transition transition-discrete opacity-0 translate-y-[100px] ' +
    'starting:data-[state=open]:opacity-0 starting:data-[state=open]:translate-y-[100px] ' +
    'data-[state=open]:opacity-100 data-[state=open]:translate-y-0';

  export type Objective = { n: number; name: string; why: string; share: number };
  export type Priority = [string, string, string, string];

  let {
    workspace = '',
    cards = $bindable([]),
    inbox = $bindable([]),
    peek = null,
    events = [],
    objectives = $bindable([]),
    priorities = $bindable([]),
    priorityMap,
    render,
    onattachcard,
    onattachnote,
    onback,
    onsearch,
    onopencard,
    onmoveevent,
    oncalrange,
    ondeletecard,
    oncapture,
    onpromote,
    onsavenote,
    ondeletenote,
    onaddobjective,
    ondeleteobjective,
    onpatchobjective,
    onpatchcard,
    loadPeople,
    threadsOf,
    canPrioritize = false,
    onthreadpriority,
    onnewthread,
    onopenthread,
    choosePriority = true,
    readonly = false,
    inboxOnly = false,
    onopenartifact,
    title = 'Planeador',
    onpickevent,
    filterBar,
    cardsAreTasks = false,
    onopendates,
    boardPane,
  }: {
    /** Qué va en el panel del tablero. Con la función de secuencia encendida,
     *  la LÍNEA de ejecución; sin ella —o con el inbox elegido— este hueco se
     *  queda vacío y se dibuja el pool de lo capturado. Un snippet y no una
     *  bandera porque lo que va dentro lo arma quien tiene los datos. */
    boardPane?: Snippet;
    /** Abrir el editor de fechas del departamento (juntas, cierres). Lo dibuja
     *  quien tiene los datos; esta pantalla sólo pone el botón. */
    onopendates?: () => void;
    /** Las tarjetas del tablero son TAREAS, no módulos. Cambia una sola cosa
     *  aquí: abrir una tarjeta lleva a su hilo en vez de abrir la hoja de la
     *  burbuja, que es de otra fila y editaría los campos equivocados. */
    cardsAreTasks?: boolean;
    /** La barra de filtros, encima del tablero. La dibuja quien tiene los datos
     *  —los proyectos, la gente, los objetivos— en vez de pasarle cuatro listas
     *  a esta pantalla para que los dibuje ella. Sin ella, no hay barra: la
     *  agenda sólo mira. */
    filterBar?: Snippet;
    /** Whose plan this is, shown top right. Empty in the real planner: the plan
     *  is the department's, and a project's name up there says it is that
     *  project's — which is exactly what it is not. */
    workspace?: string;
    /** lo que se puede abrir: módulos o tareas (calendario y secuencia) */
    cards?: Card[];
    inbox?: Note[];
    /** Abrir esta nota en su hoja, pedido desde fuera de la vista Inbox. */
    peek?: { note: Note; at: number } | null;
    events?: CalEvent[];
    objectives?: Objective[];
    priorities?: Priority[];
    priorityMap?: { cols: string[]; rows: string[][] };
    /** markdown → html; the server in the real app */
    /** markdown → html. Con la tarjeta, cuando la hay: sus imágenes viven en
     *  el workspace de la burbuja y sólo allí se resuelven. */
    render?: (md: string, card?: Card | null) => string | Promise<string>;
    /** guardar una imagen pegada en una tarjeta — al `assets/` de su workspace */
    onattachcard?: (card: Card, file: File) => Promise<{ path: string } | null | void>;
    /** guardar una imagen pegada en una nota — en la propia nota, que no tiene
     *  workspace */
    onattachnote?: (noteId: string, file: File) => Promise<{ path: string } | null | void>;
    onback?: () => void;
    onsearch?: () => void;
    onopencard?: (card: Card) => void;
    /** an event dragged to another day — the thread's due date */
    onmoveevent?: (id: string, day: string) => void;
    oncalrange?: (from: string, to: string) => void;
    ondeletecard?: (cardId: string) => void;
    oncapture?: (text: string) => void;
    onpromote?: (note: Note) => void;
    /** guardar lo escrito en una nota del inbox. Sin esto, el panel edita el
     *  objeto de la lista y nadie lo manda al servidor. */
    onsavenote?: (id: string, patch: { text: string; body: string }) => void;
    ondeletenote?: (id: string) => void;
    onaddobjective?: (name: string) => void;
    ondeleteobjective?: (n: number) => void;
    /** an objective edited in place — its name, or the outcome it claims */
    onpatchobjective?: (n: number, fields: { name?: string; outcome?: string }) => void;
    onpatchcard?: (id: string, fields: Record<string, unknown>) => void;
    /** el roster del proyecto de una tarjeta, para ponerle responsables */
    loadPeople?: (card: Card) => Promise<{ id: string; name: string; avatar?: string }[]>;
    /** los hilos de una burbuja, para la otra cara de su tarjeta */
    threadsOf?: (card: Card) => SheetThread[];
    canPrioritize?: boolean;
    onthreadpriority?: (threadId: string, priority: string) => void;
    onnewthread?: (card: Card, name: string) => void | Promise<void>;
    /** abrir un hilo a pantalla completa. Antes, esta pantalla anota a dónde
     *  volver: la tarjeta, por su cara de hilos, y el scroll de la lista. */
    onopenthread?: (threadId: string) => void;
    /** passed through to the card sheet — see there */
    choosePriority?: boolean;
    /** The same screen for somebody who does not run the plan: only the
     *  calendar, nothing draggable, and no verbs that write. The objectives,
     *  the inbox and the kanban of the whole department are the lead's; the
     *  DATES are somebody's week, and the person whose week it is should be
     *  able to look at it in the same place. */
    readonly?: boolean;
    inboxOnly?: boolean;
    onopenartifact?: (id: string) => void;
    title?: string;
    /** where a click on an event goes when nothing here may be edited */
    onpickevent?: (id: string) => void;
  } = $props();

  // UNA vista a la vez, elegida abajo a la derecha: el inbox (lo capturado,
  // flotando), el calendario o la secuencia. Eran paneles que se encendían a la
  // vez y se repartían el ancho; con el kanban fuera, cada uno pide la pantalla
  // entera y tenerlos juntos era repartir poco espacio entre tres cosas que no
  // se miran a la vez.
  //
  // Recordada por navegador: es una costumbre de trabajo, no un ajuste.
  type View = 'inbox' | 'cal' | 'sequence';
  const VIEW = 'bubble.planner.view';
  // svelte-ignore state_referenced_locally
  let view = $state<View>(
    readonly
      ? 'inbox'
      : (() => {
          try {
            const v = localStorage.getItem(VIEW);
            return v === 'cal' || v === 'sequence' ? v : 'inbox';
          } catch {
            return 'inbox';
          }
        })(),
  );
  // La secuencia sólo existe si el departamento la encendió.
  const shownView = $derived<View>(inboxOnly || (view === 'sequence' && !boardPane) ? 'inbox' : view);
  $effect(() => {
    if (readonly) return;
    try {
      localStorage.setItem(VIEW, view);
    } catch {
      // no poder recordarlo no impide el cambio
    }
  });

  // The card being read. It is the SAME object the board holds, not a copy, so
  // editing the sheet moves the board underneath it — which is the point.
  let open = $state<Card | null>(null);
  let cardOpen = $state(false);
  let cardFace = $state<'plan' | 'hilos'>('plan');
  let backScroll = $state(0);

  // Abrir un hilo desde la tarjeta lleva a otra pantalla; volver de ella tiene
  // que traer de vuelta la misma tarjeta, por la cara de hilos y con la lista
  // donde se quedó. Se anota antes de irse y se consume al volver.
  function openThread(card: Card, t: SheetThread, scroll: number) {
    plannerReturn.set({ card: card.id, face: 'hilos', scroll });
    onopenthread?.(t.id);
  }
  $effect(() => {
    const back = plannerReturn.peek();
    if (!back || !cards.some((x) => x.id === back.card)) return;
    plannerReturn.clear();
    openById(back.card, back.face, back.scroll);
  });

  // Keep the open card in step with the server.
  //
  // Every edit here goes out and comes back: the write lands, the board is
  // asked again, and `columns` is rebuilt from the answer. The sheet is holding
  // the OLD object, so without this a priority you just set showed up on the
  // board behind and not in the card in front of you.
  //
  // What is not refreshed is text being typed. Replacing the whole card mid-word
  // put the server's title back into the field and the rename that followed
  // saved the old name — measured, and the reason this is a merge rather than an
  // assignment. `writing` is the sheet saying so.
  let writing = $state(false);
  $effect(() => {
    if (!open) return;
    const fresh = cards.find((c) => c.id === open!.id);
    if (!fresh) return;
    for (const k of ['obj', 'prio', 'due', 'impact', 'urgency'] as const) {
      if (open[k] !== fresh[k]) (open as Card)[k] = fresh[k] as never;
    }
    if (writing) return;
    if (open.title !== fresh.title) open.title = fresh.title;
    if (fresh.notes !== undefined && open.notes !== fresh.notes) open.notes = fresh.notes;
  });

  /** Open a card by id — what a click on a calendar event means.
   *
   *  The sheet is opened HERE because this is where it lives: the owner of the
   *  data can fetch a document but it cannot open a dialog that belongs to this
   *  component, which is why the click did nothing at all. */
  function openById(id: string, face: 'plan' | 'hilos' = 'plan', scroll = 0) {
    const card = cards.find((c) => c.id === id);
    if (!card) return;
    cardFace = face;
    backScroll = scroll;
    open = card;
    cardOpen = true;
    onopencard?.(card);
  }

  function dropCard(id: string) {
    if (ondeletecard) return ondeletecard(id);
    cards = cards.filter((c) => c.id !== id);
  }

  // The note being read. Same object the list holds, so editing moves the list.
  let note = $state<Note | null>(null);
  let noteOpen = $state(false);
  // Una nota pedida desde fuera (la barra de Secuencia): la misma hoja.
  $effect(() => {
    if (peek) { note = peek.note; noteOpen = true; }
  });

  function promote(n: Note) {
    if (onpromote) {
      noteOpen = false;
      return onpromote(n);
    }
    // Sin servidor (el mock): la nota pasa a ser una tarjeta, con lo escrito.
    const c: Card = { id: 'k' + Date.now(), title: n.text, notes: n.body };
    cards = [c, ...cards];
    inbox = inbox.filter((x) => x.id !== n.id);
    open = c;
    cardOpen = true;
  }

  let objOpen = $state(false);
  let prioOpen = $state(false);


  function capture(raw: string) {
    const text = raw.trim();
    if (!text) return;
    if (oncapture) return oncapture(text);
    // Straight to the top: what you just typed is what you are still thinking
    // about, and burying it under a week of older notes is how an inbox stops
    // being used.
    inbox = [{ id: 'i' + Date.now(), text, from: 'tú', when: 'ahora' }, ...inbox];
  }

  const views = $derived(
    inboxOnly
      ? []
      : [
          { k: 'inbox' as const, face: '📥', label: 'Inbox' },
          { k: 'cal' as const, face: '🗓', label: 'Calendario' },
          ...(boardPane ? [{ k: 'sequence' as const, face: '🧭', label: 'Secuencia' }] : []),
        ],
  );
  const opens = $derived(
    readonly
      ? [{ k: 'search', face: '🔍', label: 'Buscar · ⌘K', go: onsearch }]
      : [
          { k: 'obj', face: '🎯', label: 'Objetivos', go: () => (objOpen = true) },
          { k: 'prio', face: '🔢', label: 'Prioridades', go: () => (prioOpen = true) },
          // Lo que pasa en una fecha y no es trabajo. Sólo si quien contiene
          // esta pantalla sabe escribirlo: la agenda mira y no escribe.
          ...(onopendates ? [{ k: 'dates', face: '📅', label: 'Fechas', go: onopendates }] : []),
          { k: 'search', face: '🔍', label: 'Buscar · ⌘K', go: onsearch },
        ],
  );

  function addObjective() {
    if (onaddobjective) return onaddobjective('Objetivo nuevo');
    objectives = [
      ...objectives,
      { n: objectives.length + 1, name: 'Objetivo nuevo', why: '', share: 0 },
    ];
  }
  function dropObjective(n: number) {
    if (ondeleteobjective) return ondeleteobjective(n);
    // Renumbered on removal: the number IS the priority order, so a gap in it
    // would be a claim nobody made.
    objectives = objectives.filter((o) => o.n !== n).map((o, i) => ({ ...o, n: i + 1 }));
  }
  function addPriority() {
    priorities = [...priorities, ['P' + (priorities.length + 1), '', '', '']];
  }
</script>

<div class="screen" class:full-canvas={shownView === 'sequence'} aria-label="planeador">
  <div class="topbar">
    <!-- Un enlace de verdad, interceptado: el clic normal lo maneja la
         aplicación, ⌘/ctrl/medio lo deja al navegador, y si el manejador no
         corriera, el `href` navega igual. -->
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <h2 class="ttl">{title}</h2>

    {#if workspace}<span class="ws">{workspace}</span>{/if}
    <!-- El planeador es de UN equipo. Aquí no hay barra lateral, así que el
         equipo se dice —y se cambia— en la barra de arriba. -->
    {#if teams.list.length > 1}
      <label class="team">
        <span class="sr-only">Equipo</span>
        <select class="select" value={teams.current} aria-label="Equipo del planeador"
          onchange={(e) => teams.set((e.currentTarget as HTMLSelectElement).value)}>
          {#each teams.list as t (t.id)}
            <option value={t.id}>{t.personal ? '🔒 ' : ''}{t.name}</option>
          {/each}
        </select>
      </label>
    {/if}
  </div>

  <div class="panes">
    {#if shownView === 'inbox'}
      <section class="pane board">
        <div class="board-wrap">
          <InboxPool notes={inbox} {readonly} onopen={(n) => { note = n; noteOpen = true; }} />
        </div>
      </section>
    {:else if shownView === 'cal'}
      <section class="pane cal">
        <PlannerCalendar
          {events}
          onrange={oncalrange}
          {readonly}
          onmove={readonly ? undefined : onmoveevent}
          onpick={readonly ? onpickevent : openById} />
      </section>
    {:else if boardPane}
      <section class="pane board">{@render boardPane()}</section>
    {/if}
  </div>
</div>

<!-- The HUD: the panes first, then what opens over them, then the theme. -->
<div class="hud">
  {#if views.length}
    <!-- Selección ÚNICA: una vista a la vez. -->
    <div class="views" role="radiogroup" aria-label="Qué mirar">
      {#each views as t (t.k)}
        <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
          <Tooltip.Trigger>
            {#snippet element(attributes: Record<string, unknown>)}
              <button class="hud-btn" class:on={shownView === t.k} role="radio"
                aria-checked={shownView === t.k} aria-label={t.label} {...attributes} onclick={() => (view = t.k)}>
                <Icon of={t.face} size="1.35rem" tint />
              </button>
            {/snippet}
          </Tooltip.Trigger>
          <Portal>
            <Tooltip.Positioner>
              <Tooltip.Content>{t.label}</Tooltip.Content>
            </Tooltip.Positioner>
          </Portal>
        </Tooltip>
      {/each}
    </div>
    <span class="sep" aria-hidden="true"></span>
  {/if}

  {#each opens as a (a.k)}
    <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
      <Tooltip.Trigger>
        {#snippet element(attributes: Record<string, unknown>)}
          <button class="hud-btn" {...attributes} aria-label={a.label} onclick={a.go}>
            <Icon of={a.face} size="1.35rem" tint />
          </button>
        {/snippet}
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content>{a.label}</Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip>
  {/each}

  <ThemeToggle floating={false} />
</div>

<!-- Montados sólo cuando se usan, como en el resto de la aplicación.
     Permanentes eran cuatro máquinas de Zag vivas en una pantalla que casi
     siempre usa cero, y cuatro desmontajes simultáneos al salir de ella. -->
{#if cardOpen && open}
<CardSheet
  bind:open={cardOpen}
  bind:card={open}
  objectives={objectives}
  {priorityMap}
  {render}
  onattach={onattachcard}
  ondelete={dropCard}
  onpatch={onpatchcard}
  {loadPeople}
  {threadsOf}
  {canPrioritize}
  {onthreadpriority}
  {onnewthread}
  onopenthread={openThread}
  bind:face={cardFace}
  {backScroll}
  bind:writing
  {choosePriority} />
{/if}

{#if noteOpen && note}
<InboxSheet
  bind:open={noteOpen}
  bind:note
  {readonly}
  {onopenartifact}
  render={render ? (md) => render(md) : undefined}
  onattach={onattachnote}
  onpromote={promote}
  onsave={(patch) => note && onsavenote?.(note.id, patch)}
  ondelete={(id) => (ondeletenote ? ondeletenote(id) : (inbox = inbox.filter((x) => x.id !== id)))} />
{/if}

<!-- ── objectives ──────────────────────────────────────────────────────── -->
{#if objOpen}
<Dialog open={objOpen} onOpenChange={(e: { open: boolean }) => (objOpen = e.open)}>
  <Portal>
    <Dialog.Backdrop
      class="scrim"
      style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-start justify-center overflow-y-auto p-4 pt-[8vh]"
      style="z-index: var(--z-drawer)">
      <Dialog.Content
        class="card bg-surface-100-900 w-full max-w-2xl space-y-4 p-4 shadow-xl {anim}">
        <header class="flex items-center justify-between gap-3">
          <Dialog.Title class="text-lg font-bold">Objetivos</Dialog.Title>
          <Dialog.CloseTrigger class="btn-icon hover:preset-tonal">
            <XIcon class="size-4" />
          </Dialog.CloseTrigger>
        </header>
        <p class="lead">
          El orden es la prioridad. Cada módulo apunta a uno de estos, y
          una que no apunta a ninguno es trabajo que nadie pidió.
        </p>
        <ul class="rows">
          {#each objectives as o (o.n)}
            <li>
              <span class="n">{o.n}</span>
              <!-- Edited in place, and saved when the field is left rather than
                   on every keystroke: an objective is a sentence somebody
                   composes, and a write per character is a write per character.
                   `onchange` is the browser saying "they moved on". -->
              <span class="fields">
                <input
                  autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                  bind:value={o.name} {@attach limited('objectives.name')}
                  onchange={() => onpatchobjective?.(o.n, { name: o.name })}
                  placeholder="nombre" />
                <input
                  autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                  class="why"
                  bind:value={o.why} {@attach limited('objectives.outcome')}
                  onchange={() => onpatchobjective?.(o.n, { outcome: o.why })}
                  placeholder="qué es verdad cuando esté cumplido" />
              </span>
              <button class="x" onclick={() => dropObjective(o.n)} aria-label="eliminar">×</button>
            </li>
          {/each}
        </ul>
        <button class="addrow" onclick={addObjective}><PlusIcon class="size-4" /> objetivo</button>
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>
{/if}

<!-- ── priorities ──────────────────────────────────────────────────────── -->
{#if prioOpen}
<Dialog open={prioOpen} onOpenChange={(e: { open: boolean }) => (prioOpen = e.open)}>
  <Portal>
    <Dialog.Backdrop
      class="scrim"
      style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-start justify-center overflow-y-auto p-4 pt-[8vh]"
      style="z-index: var(--z-drawer)">
      <Dialog.Content
        class="card bg-surface-100-900 w-full max-w-2xl space-y-4 p-4 shadow-xl {anim}">
        <header class="flex items-center justify-between gap-3">
          <Dialog.Title class="text-lg font-bold">Prioridades</Dialog.Title>
          <Dialog.CloseTrigger class="btn-icon hover:preset-tonal">
            <XIcon class="size-4" />
          </Dialog.CloseTrigger>
        </header>
        <p class="lead">
          Qué significa cada una y qué pasa cuando llega. La prioridad de un hilo
          no se teclea: sale del mapa de abajo.
        </p>
        <ul class="rows">
          {#each priorities as p, i (p[0])}
            <li>
              <span class="prio-chip prio-{p[0]}">{p[0]}</span>
              <span class="fields">
                <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" bind:value={priorities[i][1]} placeholder="nombre" />
                <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" class="why" bind:value={priorities[i][2]} placeholder="qué significa" />
                <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" class="why" bind:value={priorities[i][3]} placeholder="qué sucede" />
              </span>
              <button
                class="x"
                onclick={() => (priorities = priorities.filter((_, j) => j !== i))}
                aria-label="eliminar">×</button>
            </li>
          {/each}
        </ul>
        <button class="addrow" onclick={addPriority}><PlusIcon class="size-4" /> prioridad</button>

        {#if priorityMap}
          <h3>El mapa</h3>
          <p class="lead">
            Impacto contra urgencia. «Lo necesito urgente» se responde con «¿pasa algo
            si no se hace hoy?».
          </p>
          <table>
            <thead>
              <tr><th></th>{#each priorityMap.cols as c (c)}<th>{c}</th>{/each}</tr>
            </thead>
            <tbody>
              {#each priorityMap.rows as row (row[0])}
                <tr>
                  <th>{row[0]}</th>
                  {#each row.slice(1) as cell, i (i)}<td><span class="prio-chip prio-{cell}">{cell}</span></td>{/each}
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>
{/if}

<style>
  .screen {
    --topbar-h: 46px;
    display: flex;
    flex-direction: column;
    height: 100dvh;
    overflow: hidden;
  }
  .topbar {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    /* Lo que no cabe se encoge; lo que no se puede encoger se recorta. Sin esto
       el último control se sale por la derecha y no hay forma de alcanzarlo. */
    overflow: hidden;
    height: var(--topbar-h);
    padding: 0 0.9rem;
    border-bottom: 1px solid var(--line);
    background: color-mix(in oklab, var(--bg) 82%, transparent);
    backdrop-filter: blur(8px);
  }
  /* Se conserva la cabecera principal. Sólo el cuerpo es el canvas: los
     controles se superponen dentro de él y no descuentan otra franja. */
  .topbar { flex: none; }
  .full-canvas .pane.board { padding-bottom: 0; }
  .back {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.82rem;
  }
  .back:hover { color: var(--text); background: var(--hover); }
  .ttl { flex: none; margin: 0; font-size: 0.95rem; font-weight: 700; }
  .back { flex: none; }
  .ws { margin-left: auto; color: var(--faint); font-size: 0.82rem; }

  /* No cards. Three panes of one screen are regions of a whole, and wrapping
     each in its own bordered box says they are three things sitting next to
     each other. A hairline between them says the same thing with one pixel and
     gives the calendar back the width the borders and gaps were taking. */
  .panes { flex: 1; min-height: 0; display: flex; }
  .pane {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    border-right: 1px solid var(--line);
  }
  .pane:last-child { border-right: none; }

  /* El borde entre dos paneles, agarrable. Se monta ENCIMA de la línea de un
     pixel que ya los separa —márgenes negativos— para no robarle ancho a nadie;
     al pasar o arrastrar, la línea se ilumina y dice que se puede mover. */
  .resizer {
    position: relative;
    z-index: 2;
    flex: none;
    width: 9px;
    margin: 0 -5px 0 -4px;
    cursor: col-resize;
    touch-action: none;
  }
  .resizer::after {
    content: '';
    position: absolute;
    top: 0;
    bottom: 0;
    left: 3px;
    width: 3px;
    border-radius: 3px;
    background: transparent;
    transition: background 0.12s ease;
  }
  .resizer:hover::after,
  .resizer:focus-visible::after,
  :global(body.resizing-cols) .resizer::after { background: var(--accent, var(--line)); }
  .resizer:focus-visible { outline: none; }
  /* Mientras se arrastra, ni se selecciona texto ni el cursor parpadea al
     pasar por encima de otra cosa. */
  :global(body.resizing-cols) { cursor: col-resize; user-select: none; }

  /* The inbox is a column you scan; the other two are surfaces you work on, and
     the calendar takes whatever the kanban does not need. */
  .cal, .board { flex: 1 1 0; }
  /* La barra de botones flota abajo a la derecha: el calendario y la secuencia
     dejan su alto libre, o la última fila queda debajo de los botones. */
  .pane.board { padding-bottom: 72px; }
  .pane.cal { overflow-y: auto; padding-bottom: 0; }
  .board-wrap { flex: 1; min-height: 0; }
  .empty { margin: auto; color: var(--faint); font-size: 0.85rem; }

  .hud {
    position: fixed;
    right: 1rem;
    bottom: 1rem;
    z-index: var(--z-chrome);
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .hud .sep { width: 1px; height: 26px; background: var(--line); }
  .hud-btn {
    width: 42px;
    height: 42px;
    display: grid;
    place-content: center;
    font-size: 1.15rem;
    line-height: 1;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    transition: transform 0.15s ease, background 0.15s ease;
  }
  .hud-btn:hover,
  .hud :global(.theme-toggle:hover) { background: var(--hover); transform: translateY(-1px); }
  /* A pane that is ON says so on its own button, which is the only place the
     answer is useful. */
  .hud-btn.on {
    border-color: color-mix(in oklab, var(--accent) 55%, transparent);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18), inset 0 0 0 2px color-mix(in oklab, var(--accent) 35%, transparent);
  }

  /* El selector de vista: una píldora con tres botones, uno encendido. Que se
     lea como UNA elección y no como tres interruptores sueltos. */
  .views {
    display: flex;
    gap: 0.2rem;
    padding: 0.22rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
  }
  .views .hud-btn {
    width: 38px;
    height: 38px;
    border-color: transparent;
    background: transparent;
    box-shadow: none;
  }
  .views .hud-btn.on {
    border-color: color-mix(in oklab, var(--accent) 60%, transparent);
    background: color-mix(in oklab, var(--accent) 20%, var(--surface-solid));
    box-shadow: none;
  }

  .lead { margin: 0; color: var(--faint); font-size: 0.82rem; line-height: 1.45; }
  .rows { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 1rem; }
  .rows li { display: flex; align-items: center; gap: 0.6rem; }
  .n { flex: none; color: var(--faint); font-size: 0.8rem; font-weight: 700; }
  .fields { flex: 1; min-width: 0; display: flex; gap: 0.65rem; }
  @media (max-width: 640px) {
    .fields { flex-direction: column; }
    .rows li { align-items: flex-start; }
  }
  .fields input {
    flex: 1;
    min-width: 0;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
    font-size: 0.85rem;
  }
  .fields .why { flex: 2; }
  .x { flex: none; padding: 0 0.4rem; color: var(--faint); font-size: 1rem; }
  .x:hover { color: var(--text); }
  .addrow {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin-top: 0.6rem;
    padding: 0.4rem 0.5rem;
    border-radius: 8px;
    color: var(--faint);
    font-size: 0.82rem;
  }
  .addrow:hover { background: var(--hover); color: var(--text); }

  h3 { margin: 1.4rem 0 0.3rem; font-size: 0.9rem; }
  table { width: 100%; border-collapse: collapse; text-align: center; font-size: 0.82rem; }
  th { color: var(--faint); font-weight: 400; padding: 0.3rem; }
  tbody th { text-align: right; }
  td { padding: 0.25rem; }
  .team { margin-left: auto; flex: none; }
  .team .select { padding-block: 0.2rem; font-size: 0.82rem; font-weight: 600; max-width: 14rem; }
</style>
