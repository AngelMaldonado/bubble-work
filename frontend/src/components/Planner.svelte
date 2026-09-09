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
  // There is no card collection, and that is the point: a planner with cards of
  // its own is a second inventory of the work, and two lists of the same work
  // disagree by Thursday.
  import {
    api,
    type InboxItem,
    type Objective as ObjectiveRecord,
    type State,
    type ThreadRecord,
    type Workspace,
  } from '../lib/api';
  import PlannerView, { type Objective } from './PlannerView.svelte';
  import type { Card, Column } from './Kanban.svelte';
  import type { CalEvent } from './PlannerCalendar.svelte';
  import type { Note } from './InboxSheet.svelte';
  import { priorityMap, priorityMeaning } from '../lib/priority';

  let {
    workspace,
    onback,
    onsearch,
  }: {
    workspace: Workspace;
    onback?: () => void;
    onsearch?: () => void;
  } = $props();

  let states = $state<State[]>([]);
  let threads = $state<ThreadRecord[]>([]);
  let objectiveRows = $state<ObjectiveRecord[]>([]);
  let notes = $state<InboxItem[]>([]);
  // Priority is DERIVED, so it is read like any other server answer rather than
  // recomputed here from the same square the server used.
  let prios = $state<Record<string, string>>({});
  // The documents of the cards that have been opened, with the hash each was
  // READ at. A card's description IS the thread's document — the same markdown
  // file the thread view edits — so it is fetched when a card is opened rather
  // than for every thread on the board, and written with its base hash like
  // every other write to that file.
  let docs = $state<Record<string, { content: string; hash: string }>>({});
  let error = $state('');

  async function load() {
    try {
      const [st, th, ob, inb, pr] = await Promise.all([
        api.states(workspace.id),
        api.threads(workspace.id),
        api.objectives(workspace.id),
        api.inbox(workspace.id),
        api.priorities(workspace.id),
      ]);
      states = st;
      threads = th;
      objectiveRows = ob;
      notes = inb;
      prios = Object.fromEntries(pr.map((x) => [x.id, x.priority]));
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => {
    workspace.id;
    load();
  });

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

  const columns = $derived<Column[]>(
    states.map((s) => ({
      id: s.id,
      name: s.name,
      cards: threads
        .filter((t) => (t.state || unfiledState()) === s.id)
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
          }),
        ),
    })),
  );

  /** Where a thread with no state sits: the workspace's default column, or the
   *  first one. A thread that belongs to no column would simply not be drawn,
   *  which is how work disappears from a board that claims to show all of it. */
  function unfiledState() {
    return (states.find((s) => s.is_default) ?? states[0])?.id ?? '';
  }

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

  const moveCard = (id: string, column: string) =>
    write(() => api.update('threads', id, { state: column }));

  const addCard = (column: string, title: string) =>
    write(() => api.create('threads', { workspace: workspace.id, name: title, state: column }));

  const deleteCard = (id: string) => write(() => api.deleteThread(id));

  const capture = (text: string) =>
    write(() => api.create('inbox_items', { workspace: workspace.id, note: text }));

  const deleteNote = (id: string) => write(() => api.remove('inbox_items', id));

  /** Triage: the note becomes a thread and keeps a pointer to what it became.
   *  Two writes, in this order — a note pointing at a thread that failed to be
   *  created is worse than a thread nobody linked. */
  const promote = (note: Note) =>
    write(async () => {
      const thread = await api.create<ThreadRecord>('threads', {
        workspace: workspace.id,
        name: note.text,
        state: unfiledState(),
      });
      await api.update('inbox_items', note.id, { thread: thread.id });
    });

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
    if ('due' in fields) out.due_date = fields.due || null;
    if ('impact' in fields) out.impact = fields.impact || '';
    if ('urgency' in fields) out.urgency = fields.urgency || '';
    if ('obj' in fields) {
      const n = fields.obj as number | undefined;
      out.objective = n ? (objectiveAt(n)?.id ?? '') : '';
    }
    if (Object.keys(out).length) write(() => api.update('threads', id, out));
  }

  const addObjective = (name: string) =>
    write(() =>
      api.create('objectives', {
        workspace: workspace.id,
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
  <p class="card glass m-6 p-4 text-sm text-error-500">{error}</p>
{/if}

<PlannerView
  workspace={workspace.name}
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
  ondeleteobjective={deleteObjective} />
