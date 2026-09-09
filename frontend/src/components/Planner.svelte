<script lang="ts">
  // The planner, against the real server.
  //
  // `PlannerView` draws; this owns the data and every write. The mapping is the
  // whole of it, and it is worth stating because the planner's vocabulary and
  // the model's are not the same words:
  //
  //     column  →  a `state` the workspace defined
  //     card    →  a THREAD, the model's one executable unit
  //     due     →  threads.due_date, which the calendar reads
  //     inbox   →  inbox_items, what is captured and not yet work
  //
  // Two of those are the DEPARTMENT's and not this workspace's: the objectives
  // and the inbox. The board below them is this workspace's, so the screen is
  // the department's plan with one project's work under it — which is what a
  // department head is looking at when they open it.
  //
  // There is no card collection, and that is the point: a planner with cards of
  // its own is a second inventory of the work, and two lists of the same work
  // disagree by Thursday.
  import {
    api,
    type InboxItem,
    type Objective as ObjectiveRecord,
    type ThreadRecord,
  } from '../lib/api';
  import PlannerView, { type Objective } from './PlannerView.svelte';
  import type { Card, Column } from './Kanban.svelte';
  import type { CalEvent } from './PlannerCalendar.svelte';
  import type { Note } from './InboxSheet.svelte';
  import { priorityMap, priorityMeaning } from '../lib/priority';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';

  let {
    onback,
    onsearch,
  }: {
    onback?: () => void;
    onsearch?: () => void;
  } = $props();

  // The planner has NO workspace. Not a default one, not the one you came from:
  // this is the department's screen, and a hidden project underneath it is the
  // thing that made "which board am I actually editing" a question.
  //
  // What still needs one is BIRTH. A thread's document is a file in a
  // workspace's git repository, so something new has to be told where it lives —
  // and being asked once, out loud, is the honest version of a screen that used
  // to decide it for you.
  let born = $state<{ title: string; objective: string; note?: Note } | null>(null);
  let bornIn = $state('');

  let threads = $state<ThreadRecord[]>([]);
  let objectiveRows = $state<ObjectiveRecord[]>([]);
  let notes = $state<InboxItem[]>([]);
  // Priority is DERIVED, so it is read like any other server answer rather than
  // recomputed here from the same square the server used.
  let prios = $state<Record<string, string>>({});
  // Workspace id → name, so a card can say which project it is from. On a board
  // that spans the department, two cards called "Facturación" are two different
  // pieces of work.
  let places = $state<Record<string, string>>({});
  // The documents of the cards that have been opened, with the hash each was
  // READ at. A card's description IS the thread's document — the same markdown
  // file the thread view edits — so it is fetched when a card is opened rather
  // than for every thread on the board, and written with its base hash like
  // every other write to that file.
  let docs = $state<Record<string, { content: string; hash: string }>>({});
  let error = $state('');

  async function load() {
    try {
      const [th, ob, inb, pr, ws] = await Promise.all([
        api.allThreads(),
        api.objectives(),
        api.inbox(),
        api.allPriorities(),
        api.workspaces(),
      ]);
      threads = th;
      places = Object.fromEntries(ws.map((w) => [w.id, w.name]));
      objectiveRows = ob;
      notes = inb;
      prios = Object.fromEntries(pr.map((x) => [x.id, x.priority]));
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  /** The objective's ROW id, from the number the view shows. The view numbers
   *  them 1..n because the number is the priority order; the server does not. */
  const objectiveAt = (n: number) => objectiveRows[n - 1];

  const objectives = $derived<Objective[]>(
    objectiveRows.map((o, i) => ({
      n: i + 1,
      name: o.name,
      why: o.outcome ?? '',
      // What share of the work is under it. Counted from the threads rather
      // than stored: a number kept in two places is a number that drifts.
      share: threads.length
        ? Math.round((threads.filter((t) => t.objective === o.id).length / threads.length) * 100)
        : 0,
    })),
  );

  /** The first column: work nobody said what it is for. Named rather than
   *  hidden — an objective with nothing under it and work under no objective
   *  are the two things this screen exists to show. */
  const UNFILED = '';

  const columns = $derived<Column[]>(
    [{ id: UNFILED, name: 'Sin objetivo' }, ...objectiveRows.map((o) => ({ id: o.id, name: o.name }))].map(
      (col) => ({
        ...col,
        cards: threads
          .filter((t) => (t.objective ?? '') === col.id)
          .map(
            (t): Card => ({
              id: t.id,
              title: t.name,
              obj: t.objective ? objectiveRows.findIndex((o) => o.id === t.objective) + 1 : undefined,
              due: t.due_date ? t.due_date.slice(0, 10) : undefined,
              impact: t.impact,
              urgency: t.urgency,
              prio: prios[t.id] || undefined,
              notes: docs[t.id]?.content,
              where: places[t.workspace],
            }),
          ),
      }),
    ),
  );

  const inbox = $derived<Note[]>(
    notes
      .filter((n) => !n.thread)
      .map((n) => ({
        id: n.id,
        text: n.note,
        from: n.captured_by === api.me?.id ? 'tú' : 'alguien',
        when: new Date(n.created).toLocaleDateString(),
      })),
  );

  // The calendar reads the same threads the board does — anything with a date.
  const events = $derived<CalEvent[]>(
    threads
      .filter((t) => t.due_date)
      .map((t) => ({
        id: t.id,
        title: t.name,
        start: t.due_date!.slice(0, 10),
        allDay: true,
      })),
  );

  // Every write reloads rather than patching the local copy. The server decides
  // the seq, the default state and what a rule refuses; guessing all three in
  // the browser is how two views of the same board start disagreeing.
  async function write(run: () => Promise<unknown>) {
    try {
      await run();
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  // Moving a card says what the work is FOR. Not what state it is in: that is
  // the operative board's answer, and it belongs to the workspace that owns it.
  const moveCard = (id: string, column: string) =>
    write(() => api.update('threads', id, { objective: column }));

  /** Ask where it is born — unless there is only one place it could be. */
  function ask(title: string, objective: string, note?: Note) {
    const only = Object.keys(places);
    bornIn = only.length === 1 ? only[0] : bornIn || only[0] || '';
    born = { title, objective, note };
    if (only.length === 1) create();
  }

  async function create() {
    const b = born;
    born = null;
    if (!b || !bornIn) return;
    await write(async () => {
      const thread = await api.create<ThreadRecord>('threads', {
        workspace: bornIn,
        name: b.title,
        objective: b.objective || '',
      });
      // A triaged note keeps a pointer to what it became. Two writes in this
      // order: a note pointing at a thread that failed to be created is worse
      // than a thread nobody linked.
      if (b.note) await api.update('inbox_items', b.note.id, { thread: thread.id });
    });
  }

  const addCard = (column: string, title: string) => ask(title, column);

  const deleteCard = (id: string) => write(() => api.deleteThread(id));

  const capture = (text: string) => write(() => api.create('inbox_items', { note: text }));

  const deleteNote = (id: string) => write(() => api.remove('inbox_items', id));

  /** Triage: the note becomes a thread, and that is where its project is
   *  decided — which is the question an inbox exists to defer. */
  const promote = (note: Note) => ask(note.text, '', note);

  /** Fetch the card's document the first time it is opened. */
  async function openCard(card: Card) {
    if (docs[card.id]) return;
    try {
      const d = await api.readThread(card.id);
      docs = { ...docs, [card.id]: { content: d.content, hash: d.hash } };
    } catch (e) {
      error = (e as Error).message;
    }
  }

  /** A field edited in the card sheet, in the planner's words, translated into
   *  the thread's. `prio` is not among them: a thread's priority is DERIVED
   *  from impact × urgency by the server, and writing it here would be the one
   *  place in the product where the two disagree on purpose. */
  function patchCard(id: string, fields: Record<string, unknown>) {
    // The description is not a field on the record: it is the thread's
    // document, and it goes through the same base-hash write the thread view
    // uses. A 409 here means somebody else wrote it while the card was open.
    if ('notes' in fields) {
      const doc = docs[id];
      const content = String(fields.notes ?? '');
      if (doc && doc.content !== content) {
        write(async () => {
          const out = await api.patchThread(id, { base: doc.hash, content });
          docs = { ...docs, [id]: { content: out.content, hash: out.hash } };
        });
      }
    }
    const out: Record<string, unknown> = {};
    if ('title' in fields) out.name = fields.title;
    // A date, in the shape PocketBase stores: the picker hands back a bare
    // `2026-09-15`, and the field is a timestamp. Clearing it is an empty
    // string, not null — null is "no opinion" and leaves the old date in place.
    if ('due' in fields) {
      const day = String(fields.due ?? '');
      // Guarded, because this is where a wrongly formatted date used to leave
      // silently and come back as a refusal nobody could see.
      if (day && !/^\d{4}-\d{2}-\d{2}$/.test(day)) {
        error = `Fecha en un formato que el servidor no acepta: «${day}»`;
        return;
      }
      out.due_date = day ? `${day} 00:00:00.000Z` : '';
    }
    if ('impact' in fields) out.impact = fields.impact || '';
    if ('urgency' in fields) out.urgency = fields.urgency || '';
    if ('obj' in fields) {
      const n = fields.obj as number | undefined;
      out.objective = n ? (objectiveAt(n)?.id ?? '') : '';
    }
    if (Object.keys(out).length) write(() => api.update('threads', id, out));
  }

  const renameObjective = (id: string, name: string) => {
    if (!id) return; // "Sin objetivo" is not a row and cannot be renamed
    write(() => api.update('objectives', id, { name }));
  };

  const deleteObjectiveById = (id: string) => {
    if (!id) return;
    write(() => api.remove('objectives', id));
  };

  const addObjective = (name: string) =>
    write(() =>
      api.create('objectives', {
        name: `${name} ${objectiveRows.length + 1}`,
        position: objectiveRows.length,
      }),
    );

  const deleteObjective = (n: number) => {
    const row = objectiveAt(n);
    if (row) write(() => api.remove('objectives', row.id));
  };
</script>

{#if error}
  <!-- Above the view, not behind it. The planner is a fixed full-screen layer,
       so a banner rendered before it was painted underneath — every refusal the
       server made was invisible, and a write that failed looked exactly like a
       write that did nothing. -->
  <p class="failed" role="alert">
    {error}
    <button onclick={() => (error = '')} aria-label="cerrar">×</button>
  </p>
{/if}

{#if born}
  <!-- The one question the department's screen cannot answer by itself. -->
  <Dialog open onOpenChange={() => (born = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-sm space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">¿En qué proyecto nace?</Dialog.Title>
          <Dialog.Description class="muted text-sm">
            «{born.title}» — su documento vive en el repositorio de ese proyecto.
          </Dialog.Description>
          <ul class="places">
            {#each Object.entries(places) as [id, name] (id)}
              <li>
                <button class="place" class:on={bornIn === id} onclick={() => (bornIn = id)}>
                  {name}
                </button>
              </li>
            {/each}
          </ul>
          <div class="flex justify-end gap-2">
            <button class="btn btn-sm preset-tonal-surface" onclick={() => (born = null)}>Cancelar</button>
            <button class="btn btn-sm preset-filled-primary-500" onclick={create} disabled={!bornIn}>
              Crear
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

<PlannerView
  {columns}
  {inbox}
  {events}
  {objectives}
  priorities={priorityMeaning.map((p) => [...p] as [string, string, string, string])}
  {priorityMap}
  render={(md) => api.renderMarkdown(md)}
  choosePriority={false}
  {onback}
  {onsearch}
  onopencard={openCard}
  onpatchcard={patchCard}
  onmovecard={moveCard}
  onaddcard={addCard}
  ondeletecard={deleteCard}
  oncapture={capture}
  onpromote={promote}
  ondeletenote={deleteNote}
  onaddobjective={addObjective}
  ondeleteobjective={deleteObjective}
  onaddcolumn={() => addObjective('Objetivo nuevo')}
  onrenamecolumn={renameObjective}
  ondeletecolumn={deleteObjectiveById} />

<style>
  .places {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    /* Room before the verbs. The list ended where the buttons began, which put
       "Crear" a pixel under the last project — close enough to read as part of
       the list rather than as what happens to it. */
    margin: 0 0 0.75rem;
    padding: 0;
    list-style: none;
  }
  .place {
    width: 100%;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: transparent;
    color: var(--muted);
    text-align: left;
    font-size: 0.9rem;
  }
  .place:hover { background: var(--hover); color: var(--text); }
  .place.on { border-color: var(--accent, var(--line)); color: var(--text); font-weight: 600; }

  .failed {
    position: fixed;
    top: 1rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: var(--z-toast);
    display: flex;
    align-items: center;
    gap: 0.75rem;
    max-width: min(92vw, 560px);
    margin: 0;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--color-error-500);
    border-radius: 12px;
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.85rem;
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.22);
  }
  .failed button { color: var(--faint); font-size: 1rem; line-height: 1; }
</style>
