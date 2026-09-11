<script lang="ts">
  // The planner — the strategic layer, in one screen.
  //
  // Three panes side by side, each one shown or hidden from the HUD, the way
  // Trello does it: the kanban is the view, and the inbox and the calendar are
  // things you pull alongside it while you decide. Objectives and priorities are
  // MODALS rather than panes, because they are not worked in — they are agreed
  // on once and consulted after.
  //
  // Focused, like a thread or the wiki: no project column, its own way back.
  import { Dialog, Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import XIcon from '@lucide/svelte/icons/x';
  import Kanban, { type Card, type Column } from './Kanban.svelte';
  import CardSheet from './CardSheet.svelte';
  import InboxSheet, { type Note } from './InboxSheet.svelte';
  import PlannerCalendar, { type CalEvent } from './PlannerCalendar.svelte';
  import ThemeToggle from './ThemeToggle.svelte';
  import { edgeFade } from '../lib/fade.svelte';

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
    columns = $bindable([]),
    inbox = $bindable([]),
    events = [],
    objectives = $bindable([]),
    priorities = $bindable([]),
    priorityMap,
    render,
    onback,
    onsearch,
    onopencard,
    onmovecard,
    onautoorder,
    onmoveevent,
    onaddcard,
    ondeletecard,
    oncapture,
    onpromote,
    onsavenote,
    ondeletenote,
    onaddobjective,
    ondeleteobjective,
    onpatchobjective,
    onpatchcard,
    onaddcolumn,
    onrenamecolumn,
    ondeletecolumn,
    choosePriority = true,
    readonly = false,
    title = 'Planeador',
    onpickevent,
  }: {
    /** Whose plan this is, shown top right. Empty in the real planner: the plan
     *  is the department's, and a project's name up there says it is that
     *  project's — which is exactly what it is not. */
    workspace?: string;
    /** One board, whose columns are yours to define. More than one board is a
     *  question for when a single one is actually in the way. */
    columns?: Column[];
    inbox?: Note[];
    events?: CalEvent[];
    objectives?: Objective[];
    priorities?: Priority[];
    priorityMap?: { cols: string[]; rows: string[][] };
    /** markdown → html; the server in the real app */
    render?: (md: string) => string | Promise<string>;
    onback?: () => void;
    onsearch?: () => void;
    onopencard?: (card: Card) => void;
    /** Where a change GOES, when there is a server behind this screen.
     *
     *  Given one, this view stops moving its own arrays and lets whoever owns
     *  the data do the write and hand the result back. Two copies of the same
     *  list —one optimistic here, one authoritative there— is the bug this
     *  avoids: the mock keeps its local behaviour precisely because it passes
     *  none of these. */
    onmovecard?: (cardId: string, columnId: string, before: string | null) => void;
    /** devolver una columna al orden que deriva la prioridad */
    onautoorder?: (columnId: string) => void;
    /** an event dragged to another day — the thread's due date */
    onmoveevent?: (id: string, day: string) => void;
    onaddcard?: (columnId: string, title: string) => void;
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
    /** the columns, when they are rows somebody owns — see Kanban */
    onaddcolumn?: () => void;
    onrenamecolumn?: (id: string, name: string) => void;
    ondeletecolumn?: (id: string) => void;
    /** passed through to the card sheet — see there */
    choosePriority?: boolean;
    /** The same screen for somebody who does not run the plan: only the
     *  calendar, nothing draggable, and no verbs that write. The objectives,
     *  the inbox and the kanban of the whole department are the lead's; the
     *  DATES are somebody's week, and the person whose week it is should be
     *  able to look at it in the same place. */
    readonly?: boolean;
    title?: string;
    /** where a click on an event goes when nothing here may be edited */
    onpickevent?: (id: string) => void;
  } = $props();

  // The kanban is on by default and cannot be the only thing turned off: it is
  // the view, not a panel of it.
  //
  // Remembered per browser, because which panes you keep open is a working
  // habit rather than a setting: somebody who plans with the inbox beside the
  // board should not turn it on every morning. Local, not on the server — it is
  // about this screen on this machine, and it is not worth a round trip.
  const PANES = 'bubble.planner.panes';
  const kept = (() => {
    try {
      return JSON.parse(localStorage.getItem(PANES) ?? 'null') as
        | { inbox: boolean; cal: boolean; board: boolean }
        | null;
    } catch {
      return null; // private mode, or something else wrote nonsense there
    }
  })();
  // Read-only, the panes are not a choice: there is one thing to show. Leído
  // una sola vez a propósito — el modo no cambia mientras la pantalla vive, y
  // un `$derived` aquí impediría encender y apagar paneles después.
  // svelte-ignore state_referenced_locally
  let showInbox = $state(readonly ? false : (kept?.inbox ?? false));
  // svelte-ignore state_referenced_locally
  let showCal = $state(readonly ? true : (kept?.cal ?? false));
  // svelte-ignore state_referenced_locally
  let showBoard = $state(readonly ? false : (kept?.board ?? true));
  $effect(() => {
    if (readonly) return; // no es una preferencia, es la única vista
    try {
      localStorage.setItem(
        PANES,
        JSON.stringify({ inbox: showInbox, cal: showCal, board: showBoard }),
      );
    } catch {
      // not being able to remember it is not a reason to refuse the change
    }
  });

  // The card being read. It is the SAME object the board holds, not a copy, so
  // editing the sheet moves the board underneath it — which is the point.
  let open = $state<Card | null>(null);
  // Which column it is in. Kept beside the card because a card does not know:
  // its column is the board's arrangement of it, not a property of the work.
  let openIn = $state('');
  let cardOpen = $state(false);

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
    const fresh = columns.flatMap((c) => c.cards).find((c) => c.id === open!.id);
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
  function openById(id: string) {
    const card = columns.flatMap((c) => c.cards).find((c) => c.id === id);
    if (!card) return;
    open = card;
    openIn = columns.find((c) => c.cards.includes(card))?.id ?? '';
    cardOpen = true;
    onopencard?.(card);
  }

  function moveCard(cardId: string, to: string, before: string | null = null) {
    if (onmovecard) {
      openIn = to;
      return onmovecard(cardId, to, before);
    }
    let card: Card | undefined;
    const next = columns.map((c) => ({
      ...c,
      cards: c.cards.filter((x) => {
        if (x.id !== cardId) return true;
        card = x;
        return false;
      }),
    }));
    if (!card) return;
    next.find((c) => c.id === to)?.cards.push(card);
    columns = next;
    openIn = to;
  }

  function addCard(columnId: string) {
    // A thread needs a name and nothing else; asking for it here rather than
    // creating "Tarjeta nueva" is the difference between a board of work and a
    // board of placeholders.
    if (onaddcard) return onaddcard(columnId, 'Trabajo nuevo');
    const c: Card = { id: 'k' + Date.now(), title: 'Tarjeta nueva' };
    openIn = columnId;
    columns = columns.map((x) => (x.id === columnId ? { ...x, cards: [...x.cards, c] } : x));
    open = c;
    cardOpen = true;
  }
  function dropCard(id: string) {
    if (ondeletecard) return ondeletecard(id);
    columns = columns.map((x) => ({ ...x, cards: x.cards.filter((c) => c.id !== id) }));
  }

  // The note being read. Same object the list holds, so editing moves the list.
  let note = $state<Note | null>(null);
  let noteOpen = $state(false);

  function promote(n: Note) {
    if (onpromote) {
      noteOpen = false;
      return onpromote(n);
    }
    // An inbox item becomes a card in the FIRST column — the one where things
    // are still undecided — carrying whatever was written about it.
    const c: Card = { id: 'k' + Date.now(), title: n.text, notes: n.body };
    openIn = columns[0]?.id ?? '';
    columns = columns.map((x, i) => (i === 0 ? { ...x, cards: [c, ...x.cards] } : x));
    inbox = inbox.filter((x) => x.id !== n.id);
    open = c;
    cardOpen = true;
  }

  let objOpen = $state(false);
  let prioOpen = $state(false);

  let draft = $state('');
  const inboxFade = edgeFade();

  function capture(e: SubmitEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!text) return;
    if (oncapture) {
      draft = '';
      return oncapture(text);
    }
    // Straight to the top: what you just typed is what you are still thinking
    // about, and burying it under a week of older notes is how an inbox stops
    // being used.
    inbox = [{ id: 'i' + Date.now(), text, from: 'tú', when: 'ahora' }, ...inbox];
    draft = '';
  }

  const panes = $derived(
    readonly
      ? []
      : [
          { k: 'inbox', face: '📥', label: 'Inbox', on: showInbox, go: () => (showInbox = !showInbox) },
          { k: 'cal', face: '🗓', label: 'Calendario', on: showCal, go: () => (showCal = !showCal) },
          { k: 'board', face: '🗂', label: 'Kanban', on: showBoard, go: () => (showBoard = !showBoard) },
        ],
  );
  const opens = $derived(
    readonly
      ? [{ k: 'search', face: '🔍', label: 'Buscar · ⌘K', go: onsearch }]
      : [
          { k: 'obj', face: '🎯', label: 'Objetivos', go: () => (objOpen = true) },
          { k: 'prio', face: '🔢', label: 'Prioridades', go: () => (prioOpen = true) },
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

<div class="screen" aria-label="planeador">
  <div class="topbar">
    <!-- Un enlace de verdad, interceptado: el clic normal lo maneja la
         aplicación, ⌘/ctrl/medio lo deja al navegador, y si el manejador no
         corriera, el `href` navega igual. -->
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <h2 class="ttl">{title}</h2>

    {#if workspace}<span class="ws">{workspace}</span>{/if}
  </div>

  <div class="panes">
    {#if showInbox}
      <section class="pane inbox">
        <header><span class="pane-name">📥 Inbox</span><span class="count">{inbox.length}</span></header>
        <!-- The field is FIRST and always there. An inbox exists to be dumped
             into; asking somebody to press "+" before they can type is asking
             them to decide they are capturing something. -->
        <form onsubmit={capture}>
          <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" bind:value={draft} placeholder="Escribe y Enter…" />
        </form>
        <ul class="items" style={inboxFade.style} {@attach inboxFade.attach}>
          {#each inbox as it (it.id)}
            <li>
              <button onclick={() => { note = it; noteOpen = true; }}>
                <span class="what">{it.text}</span>
                <span class="from">{it.from} · {it.when}</span>
              </button>
            </li>
          {/each}
        </ul>
        <p class="hint">Entra vago. Sale con objetivo, impacto y urgencia — y de ahí sale su prioridad.</p>
      </section>
    {/if}

    {#if showCal}
      <section class="pane cal">
        <PlannerCalendar
          {events}
          {readonly}
          onmove={readonly ? undefined : onmoveevent}
          onpick={readonly ? onpickevent : openById} />
      </section>
    {/if}

    {#if showBoard}
      <section class="pane board">
        <Kanban
          bind:columns
          onopen={(c) => {
            open = c;
            openIn = columns.find((x) => x.cards.includes(c))?.id ?? '';
            cardOpen = true;
            onopencard?.(c);
          }}
          onadd={addCard}
          ondeletecard={dropCard}
          onmovecard={moveCard}
          {onautoorder}
          {onaddcolumn}
          {onrenamecolumn}
          {ondeletecolumn} />
      </section>
    {/if}

    {#if !showInbox && !showCal && !showBoard}
      <p class="empty">Todos los paneles están ocultos. Enciende uno abajo a la derecha.</p>
    {/if}
  </div>
</div>

<!-- The HUD: the panes first, then what opens over them, then the theme. -->
<div class="hud">
  {#each panes as t (t.k)}
    <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
      <Tooltip.Trigger>
        {#snippet element(attributes: Record<string, unknown>)}
          <button class="hud-btn" class:on={t.on} aria-pressed={t.on} {...attributes} onclick={t.go}>
            <span aria-hidden="true">{t.face}</span>
          </button>
        {/snippet}
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content>{t.on ? `Ocultar ${t.label}` : `Mostrar ${t.label}`}</Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip>
  {/each}

  {#if panes.length}<span class="sep" aria-hidden="true"></span>{/if}

  {#each opens as a (a.k)}
    <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
      <Tooltip.Trigger>
        {#snippet element(attributes: Record<string, unknown>)}
          <button class="hud-btn" {...attributes} onclick={a.go}>
            <span aria-hidden="true">{a.face}</span>
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
  columns={columns.map((c) => ({ id: c.id, name: c.name }))}
  columnId={openIn}
  onmove={moveCard}
  ondelete={dropCard}
  onpatch={onpatchcard}
  bind:writing
  {choosePriority} />
{/if}

{#if noteOpen && note}
<InboxSheet
  bind:open={noteOpen}
  bind:note
  {render}
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
          El orden es la prioridad. Cada tarjeta del kanban apunta a uno de estos, y
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
                  bind:value={o.name}
                  onchange={() => onpatchobjective?.(o.n, { name: o.name })}
                  placeholder="nombre" />
                <input
                  autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                  class="why"
                  bind:value={o.why}
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
          Qué significa cada una y qué pasa cuando llega. La prioridad de un thread
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
    gap: 0.75rem;
    height: var(--topbar-h);
    padding: 0 0.9rem;
    border-bottom: 1px solid var(--line);
    background: color-mix(in oklab, var(--bg) 82%, transparent);
    backdrop-filter: blur(8px);
  }
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
  .ttl { margin: 0; font-size: 0.95rem; font-weight: 700; }
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

  /* The inbox is a column you scan; the other two are surfaces you work on, and
     the calendar takes whatever the kanban does not need. */
  .inbox { flex: 0 0 288px; padding: 0.75rem; }
  .cal { flex: 3 1 0; }
  .board { flex: 2 1 0; }
  .empty { margin: auto; color: var(--faint); font-size: 0.85rem; }

  .inbox > header { display: flex; align-items: center; gap: 0.5rem; padding-bottom: 0.5rem; }
  .pane-name { font-size: 0.78rem; font-weight: 700; color: var(--muted); }
  .count {
    margin-left: auto;
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--faint);
    font-size: 0.68rem;
  }
  .inbox input {
    width: 100%;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--surface);
    color: var(--text);
    font-size: 0.85rem;
  }
  .items {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    margin: 0.5rem 0 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .items li { border-radius: 9px; background: var(--hover); }
  .items button {
    display: block;
    width: 100%;
    padding: 0.5rem 0.6rem;
    border-radius: 9px;
    text-align: left;
  }
  .items button:hover { background: var(--surface-solid); }
  .what { display: block; font-size: 0.85rem; line-height: 1.35; color: var(--text); }
  .from { display: block; margin-top: 0.25rem; color: var(--faint); font-size: 0.72rem; }
  .hint { margin: 0.6rem 0 0; color: var(--faint); font-size: 0.72rem; line-height: 1.4; }

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

  .lead { margin: 0; color: var(--faint); font-size: 0.82rem; line-height: 1.45; }
  .rows { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 0.45rem; }
  .rows li { display: flex; align-items: center; gap: 0.6rem; }
  .n { flex: none; color: var(--faint); font-size: 0.8rem; font-weight: 700; }
  .fields { flex: 1; min-width: 0; display: flex; gap: 0.4rem; }
  .fields input {
    flex: 1;
    min-width: 0;
    padding: 0.35rem 0.55rem;
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
</style>
