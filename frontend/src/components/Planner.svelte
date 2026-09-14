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
    type BubbleRecord,
    type InboxItem,
    type Objective as ObjectiveRecord,
    type Stage,
    type ThreadRecord,
  } from '../lib/api';
  import PlannerView, { type Objective } from './PlannerView.svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
  import type { Card, Column } from './Kanban.svelte';
  import type { CalEvent } from './PlannerCalendar.svelte';
  import type { Note } from './InboxSheet.svelte';
  import { priorityMap, priorityMeaning } from '../lib/priority';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import { tooLong } from '../lib/limits.svelte';

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
  let born = $state<{ title: string; stage: string; note?: Note } | null>(null);
  let bornIn = $state('');
  // No bubble is asked for here, and that is the difference between the two
  // screens rather than an oversight. The board is the OPERATIVE surface: work
  // on it lives in a bubble because the board draws bubbles. The planner is the
  // strategic one, where the sentence is "this is the work and it belongs to
  // this project" — which bubble carries it is a decision for the board, later,
  // by whoever runs it.

  // Lo que este tablero organiza son BURBUJAS.
  //
  // Un thread es ejecución: lo escribe quien opera, y cómo se organiza por
  // dentro es suyo. Un lead plantea actividades a corto, mediano y largo plazo,
  // y eso es una burbuja — un cuerpo de trabajo con un outcome. Los threads
  // siguen aquí para una sola cosa: sus fechas, que es lo que el calendario
  // muestra. Las fechas son de la ejecución.
  let bubbles = $state<BubbleRecord[]>([]);
  let stageRows = $state<Stage[]>([]);
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
  let error = $state('');
  // Nothing is destroyed without being asked first, and the question is asked
  // HERE — the writer knows the name of what is about to go and what goes with
  // it. See `Confirm.svelte`.
  let doom = $state<Doom>(null);

  async function load() {
    try {
      const [bu, st, th, ob, inb, pr, ws] = await Promise.all([
        api.allBubbles(),
        api.stages(),
        api.allThreads(),
        api.objectives(),
        api.inbox(),
        api.allPriorities(),
        api.workspaces(),
      ]);
      bubbles = bu;
      stageRows = st;
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
      // Qué parte del trabajo cuelga de él. Contado sobre las BURBUJAS, que es
      // lo que un objetivo agrupa — contar threads decía «catorce» donde lo que
      // hay son tres cosas en marcha. Y contado, no guardado: un número que vive
      // en dos sitios es un número que deriva.
      share: bubbles.length
        ? Math.round((bubbles.filter((b) => b.objective === o.id).length / bubbles.length) * 100)
        : 0,
    })),
  );

  /** La primera columna: lo que nadie ha colocado en el plan todavía. Con
   *  nombre y no escondida — una burbuja que existe y no está planeada es
   *  justamente una de las dos cosas que esta pantalla sirve para ver. */
  const UNSTAGED = '';

  /** Cómo se ordena una columna.
   *
   *  Por defecto, sola: por la prioridad que el servidor deriva de impacto ×
   *  urgencia, que es lo que el modelo dice — el orden se deriva, no se
   *  administra. Empatan por fecha de vencimiento, y al final por nombre, para
   *  que dos cargas iguales no bailen entre recargas.
   *
   *  Si alguien arrastró aquí, manda su orden. No es un segundo sistema: es una
   *  excepción declarada, y la columna lo dice con un botón para deshacerla.
   *  «Esto va primero porque el cliente llama el martes» no cabe en impacto ×
   *  urgencia, y obligar a falsear la urgencia para colocar una tarjeta sería
   *  peor — corrompe el dato con el que se calcula todo lo demás. */
  function sortColumn(rows: BubbleRecord[]): BubbleRecord[] {
    const manual = rows.some((t) => (t.rank ?? 0) > 0);
    return [...rows].sort((a, b) => {
      if (manual) {
        // Sin rango va al final: una tarjeta recién llegada a una columna
        // ordenada a mano no tiene sitio asignado, y ponerla arriba sería
        // inventarle uno.
        const ra = a.rank || Number.MAX_SAFE_INTEGER;
        const rb = b.rank || Number.MAX_SAFE_INTEGER;
        if (ra !== rb) return ra - rb;
      }
      const pa = prios[a.id] || 'P9';
      const pb = prios[b.id] || 'P9';
      if (pa !== pb) return pa < pb ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  }

  /** Cuántos threads tiene cada burbuja: lo que una tarjeta dice sin abrirse.
   *
   *  No es el detalle —eso vive en los threads y es de quien opera— sino su
   *  tamaño: «esto son ocho piezas» y «esto es una» se planean distinto. */
  const pieces = $derived.by(() => {
    const n: Record<string, number> = {};
    for (const t of threads) if (t.bubble) n[t.bubble] = (n[t.bubble] ?? 0) + 1;
    return n;
  });

  const columns = $derived<Column[]>(
    [
      { id: UNSTAGED, name: 'Sin planear', locked: true },
      ...stageRows.map((st) => ({ id: st.id, name: st.name })),
    ].map(
      (col) => ({
        ...col,
        manual: bubbles.some((b) => (b.stage ?? '') === col.id && (b.rank ?? 0) > 0),
        cards: sortColumn(bubbles.filter((b) => (b.stage ?? '') === col.id)).map(
          (b): Card => ({
            id: b.id,
            title: b.name,
            obj: b.objective ? objectiveRows.findIndex((o) => o.id === b.objective) + 1 : undefined,
            objName: b.objective ? objectiveRows.find((o) => o.id === b.objective)?.name : undefined,
            impact: b.impact,
            urgency: b.urgency,
            prio: prios[b.id] || undefined,
            // El BRIEF: lo que el lead escribe al planear, con diagramas e
            // imágenes. No el outcome, que es el contrato de una frase — estirado
            // hasta ser el cuaderno del plan dejaría de leerse de un vistazo en
            // el board.
            notes: b.brief,
            pieces: pieces[b.id] ?? 0,
            where: places[b.workspace],
            ws: b.workspace,
            born: b.created,
          }),
        ),
      }),
    ),
  );

  const inbox = $derived<Note[]>(
    notes
      // Fuera las triadas, sea a un thread o a una burbuja: ya son de algún
      // sitio, y el inbox es lo que todavía no lo es.
      .filter((n) => !n.thread && !n.bubble)
      .map((n) => ({
        id: n.id,
        text: n.note,
        body: n.body ?? '',
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
  /** Soltar una tarjeta: a qué columna, y en qué sitio.
   *
   *  La columna es el objetivo. El sitio se guarda con `rank`, y se escribe
   *  para TODA la columna de destino: mezclar tarjetas con rango y sin él
   *  dentro de la misma columna es tener dos órdenes a la vez y no saber cuál
   *  gana. Con el primer arrastre, la columna entera pasa a ser suya — lo que
   *  se ve antes de soltar es exactamente lo que queda después.
   *
   *  Los rangos van de diez en diez sin más motivo que dejar hueco a la vista
   *  cuando alguien mire la base a mano. */
  function moveCard(id: string, column: string, before: string | null = null) {
    const col = columns.find((c) => c.id === column);
    if (!col) return;
    const order = col.cards.map((c) => c.id).filter((x) => x !== id);
    const at = before ? order.indexOf(before) : -1;
    if (at < 0) order.push(id);
    else order.splice(at, 0, id);

    write(async () => {
      // La etapa primero: si algo falla después, la burbuja ya está donde se la
      // soltó y sólo queda mal el orden — el revés dejaría una tarjeta con sitio
      // en una columna a la que no pertenece.
      const was = bubbles.find((b) => b.id === id)?.stage ?? '';
      if (was !== column) await api.update('bubbles', id, { stage: column });
      await Promise.all(
        order.map((bid, i) => api.update('bubbles', bid, { rank: (i + 1) * 10 })),
      );
    });
  }

  /** Devolver una columna a que se ordene sola. Cero es «ninguno»: la prioridad
   *  derivada vuelve a mandar, que es el valor por defecto de todo esto. */
  function autoOrder(column: string) {
    const col = columns.find((c) => c.id === column);
    if (!col) return;
    write(() => Promise.all(col.cards.map((c) => api.update('bubbles', c.id, { rank: 0 }))));
  }

  /** Ask where it is born — unless there is only one place it could be. */
  function ask(title: string, stage: string, note?: Note) {
    const only = Object.keys(places);
    bornIn = only.length === 1 ? only[0] : bornIn || only[0] || '';
    born = { title, stage, note };
    // With a single project there is nothing to choose, so nothing is asked.
    if (only.length === 1) create();
  }

  async function create() {
    const b = born;
    if (!b || !bornIn) return;
    // El título de la nota pasa a ser el NOMBRE de la burbuja, que cabe menos
    // que una nota. Se dice antes de intentarlo, no con un rechazo después.
    const long = tooLong('bubbles.name', b.title);
    if (long) {
      born = null;
      error = `No pasó al kanban: el título se vuelve el nombre de la burbuja. ${long}`;
      return;
    }
    born = null;
    await write(async () => {
      // Nace una BURBUJA: lo que se planea es un cuerpo de trabajo. Sus threads
      // los abre quien la ejecute, y ahí es donde se documenta el detalle.
      // Lo que se entendió de la nota viaja a la burbuja como su brief — con sus
      // imágenes, que siguen citando la nota por URL y siguen cargando porque
      // la nota NO se borra.
      const bub = await api.create<BubbleRecord>('bubbles', {
        workspace: bornIn,
        name: b.title,
        stage: b.stage || '',
        brief: b.note?.body ?? '',
      });
      // Una nota triada guarda a qué se convirtió, y sale del inbox por eso —
      // no porque se borre. Antes se borraba, y con ella se iban su cuerpo y
      // sus imágenes: lo único que se había entendido de la captura. Dos
      // escrituras en este orden: una nota que apunta a algo que no llegó a
      // crearse es peor que algo que nadie enlazó.
      if (b.note) await api.update('inbox_items', b.note.id, { bubble: bub.id });
    });
  }

  const addCard = (column: string, title: string) => ask(title, column);

  const deleteCard = (id: string) => {
    const b = bubbles.find((x) => x.id === id);
    const n = pieces[id] ?? 0;
    doom = {
      title: `¿Borrar la burbuja «${b?.name ?? id}»?`,
      body: n
        ? `Sus ${n} threads NO se borran: quedan sin burbuja en el board de su proyecto, que es exactamente lo que pasó.`
        : 'Sale del plan y del board de su proyecto.',
      go: () => write(() => api.remove('bubbles', id)),
    };
  };

  const capture = (text: string) => write(() => api.create('inbox_items', { note: text }));

  /** Guardar lo escrito en una nota.
   *
   *  Faltaba entero: el panel dejaba editar el título y el texto, los cambiaba
   *  en el objeto de la lista y nadie los mandaba al servidor — así que
   *  recargar los borraba. Es de las peores formas de perder algo, porque
   *  mientras la pantalla está abierta parece guardado. */
  function saveNote(id: string, patch: { text: string; body: string }) {
    const n = notes.find((x) => x.id === id);
    if (!n) return;
    const note = patch.text.trim();
    if (!note || (note === n.note && patch.body === (n.body ?? ''))) return;
    write(() => api.update('inbox_items', id, { note, body: patch.body }));
  }

  const deleteNote = (id: string) => {
    const n = notes.find((x) => x.id === id);
    doom = {
      title: '¿Borrar esta nota?',
      body: n?.note
        ? `«${n.note}» deja de existir: una nota no es trabajo y no queda en ningún otro lado.`
        : 'Una nota no es trabajo y no queda en ningún otro lado.',
      go: () => write(() => api.remove('inbox_items', id)),
    };
  };

  /** Triage: the note becomes a thread, and that is where its project is
   *  decided — which is the question an inbox exists to defer. */
  const promote = (note: Note) => ask(note.text, '', note);

  /** Abrir una tarjeta ya no pide nada: la descripción de una burbuja es su
   *  outcome, y viene con la fila. Antes había que ir a buscar el documento del
   *  thread — que es ejecución, y no es lo que se planea. */
  function openCard(_card: Card) {}

  /** Un campo editado en el panel, dicho en palabras del planeador y traducido
   *  a las de la burbuja.
   *
   *  `prio` no está: se DERIVA de impacto × urgencia en el servidor, y
   *  escribirla aquí sería el único sitio del producto donde las dos se
   *  contradicen a propósito. */
  function patchCard(id: string, fields: Record<string, unknown>) {
    const out: Record<string, unknown> = {};
    if ('title' in fields) out.name = fields.title;
    // El brief de la burbuja: el cuaderno del plan. No el outcome —ése es una
    // frase, el contrato— ni un documento de git, que son de los threads y son
    // la ejecución.
    if ('notes' in fields) out.brief = String(fields.notes ?? '');
    if ('impact' in fields) out.impact = fields.impact || '';
    if ('urgency' in fields) out.urgency = fields.urgency || '';
    if ('obj' in fields) {
      const n = fields.obj as number | undefined;
      out.objective = n ? (objectiveAt(n)?.id ?? '') : '';
    }
    if (Object.keys(out).length) write(() => api.update('bubbles', id, out));
  }

  // Las columnas del kanban son ETAPAS. Estos tres verbos seguían escribiendo
  // en `objectives` desde cuando las columnas eran objetivos: renombrar una
  // etapa pedía un objetivo con el id de la etapa, y el servidor contestaba que
  // no existía.
  //
  // «Sin planear» no es una fila (es `UNSTAGED`, la burbuja sin etapa), así que
  // no se renombra ni se borra.

  /** Una etapa nueva entra ANTES de la de terminado: «Hecho» es el final del
   *  tablero, y una columna añadida detrás de él sería trabajo después de
   *  acabado. Sin columna de terminado, al final. */
  const addStage = () =>
    write(async () => {
      const rows = [...stageRows].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
      const end = rows.findIndex((r) => r.done);
      const at = end < 0 ? rows.length : end;
      // Los de detrás se corren uno, del último al primero.
      for (const r of rows.slice(at).reverse()) {
        await api.update('stages', r.id, { position: (r.position ?? 0) + 1 });
      }
      await api.create('stages', {
        name: 'Etapa nueva',
        position: at < rows.length ? (rows[at].position ?? at) : (rows.at(-1)?.position ?? -1) + 1,
      });
    });

  /** Arrastrar una columna es reordenar las etapas, y se guarda: antes el
   *  tablero movía su propia copia y al recargar volvía todo a su sitio. Se
   *  reescriben las posiciones que cambiaron, de 0 en adelante. */
  const moveStage = (id: string, before: string | null) => {
    if (id === UNSTAGED) return;
    const rows = [...stageRows].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    const moving = rows.find((r) => r.id === id);
    if (!moving) return;
    const rest = rows.filter((r) => r.id !== id);
    const at = before && before !== UNSTAGED ? rest.findIndex((r) => r.id === before) : -1;
    // Delante de «Sin planear» es lo mismo que el principio: esa columna no es
    // una etapa y siempre va primero.
    if (before === UNSTAGED) rest.unshift(moving);
    else if (at < 0) rest.push(moving);
    else rest.splice(at, 0, moving);
    write(async () => {
      for (const [i, r] of rest.entries()) {
        if ((r.position ?? -1) !== i) await api.update('stages', r.id, { position: i });
      }
    });
  };

  const renameStage = (id: string, name: string) => {
    if (id === UNSTAGED) return;
    write(() => api.update('stages', id, { name }));
  };

  const deleteStage = (id: string) => {
    if (id === UNSTAGED) return;
    const st = stageRows.find((x) => x.id === id);
    const n = bubbles.filter((b) => b.stage === id).length;
    doom = {
      title: `¿Borrar la etapa «${st?.name ?? id}»?`,
      body: n
        ? `Sus ${n} burbujas NO se borran: vuelven a «Sin planear», que es exactamente lo que pasó.`
        : 'Sale del tablero. No hay burbujas en ella.',
      go: () => write(() => api.remove('stages', id)),
    };
  };

  const deleteObjectiveById = (id: string) => {
    if (!id) return; // "Sin objetivo" is not a row and cannot be deleted
    const o = objectiveRows.find((x) => x.id === id);
    doom = {
      title: `¿Borrar el objetivo «${o?.name ?? id}»?`,
      body: 'Las burbujas que colgaban de él no se borran: quedan sin objetivo, que es exactamente lo que pasó.',
      go: () => write(() => api.remove('objectives', id)),
    };
  };

  const addObjective = (name: string) =>
    write(() =>
      api.create('objectives', {
        name: `${name} ${objectiveRows.length + 1}`,
        position: objectiveRows.length,
      }),
    );

  // The 🎯 dialog numbers its objectives; the kanban knows their ids. Both end
  // in the same place, so both ask the same question.
  const deleteObjective = (n: number) => deleteObjectiveById(objectiveAt(n)?.id ?? '');
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
            <button
              class="btn btn-sm preset-filled-primary-500"
              onclick={create}
              disabled={!bornIn}>Crear</button>
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

<Confirm bind:ask={doom} />

<PlannerView
  {columns}
  {inbox}
  {events}
  {objectives}
  priorities={priorityMeaning.map((p) => [...p] as [string, string, string, string])}
  {priorityMap}
  render={(md, card) => api.renderMarkdown(md, card?.ws ?? '')}
  onattachcard={(card, file) =>
    // Al `assets/` del workspace de la burbuja, como una imagen de thread: la
    // burbuja tiene workspace, así que su imagen tiene dónde vivir y se cita por
    // la ruta corta, la que sobrevive a una mudanza.
    card.ws ? api.uploadAsset(card.ws, file) : Promise.resolve(null)}
  onattachnote={(id, file) => api.attachToNote(id, file)}
  choosePriority={false}
  {onback}
  {onsearch}
  onopencard={openCard}
  onpatchcard={patchCard}
  onmoveevent={(id, day) => patchCard(id, { due: day })}
  onmovecard={moveCard}
  onautoorder={autoOrder}
  onaddcard={addCard}
  ondeletecard={deleteCard}
  oncapture={capture}
  onpromote={promote}
  onsavenote={saveNote}
  ondeletenote={deleteNote}
  onpatchobjective={(n, fields) => {
    const row = objectiveAt(n);
    if (row) write(() => api.update('objectives', row.id, fields));
  }}
  onaddobjective={addObjective}
  ondeleteobjective={deleteObjective}
  onaddcolumn={addStage}
  onrenamecolumn={renameStage}
  ondeletecolumn={deleteStage}
  onmovecolumn={moveStage} />

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
