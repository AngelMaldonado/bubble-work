<script lang="ts">
  import { bandFace, bandName } from '../lib/bands';
  // The bubble's interior, as a drawer from the right.
  //
  // A drawer rather than a page because opening a bubble should not lose the
  // board: the whole point of the board is comparison, and a full-page
  // navigation costs you the thing you were comparing against. v0 learned this
  // and used the same shape.
  //
  // Skeleton's Dialog underneath, so the focus trap, the escape key and the aria
  // wiring are somebody else's problem — the part that is easy to do almost
  // right and hard to do correctly.
  import { untrack } from 'svelte';
  import { Combobox, Dialog, Portal, Progress, useListCollection } from '@skeletonlabs/skeleton-svelte';
  import { ago } from '../lib/when';
  import { edgeFade } from '../lib/fade.svelte';
  import type { Lifecycle } from '../lib/api';
  import { limited } from '../lib/limits.svelte';
  import Prose from './Prose.svelte';
  import PriorityBadge from './PriorityBadge.svelte';
  import MarkdownField from './MarkdownField.svelte';

  let {
    open = $bindable(false),
    // Bindable, because "+ thread" is reachable from OUTSIDE the drawer — the
    // orb's right-click opens the panel already asking for the name. One field,
    // two ways in, rather than a second place to name a thread.
    naming = $bindable(false),
    name,
    bubble = '',
    lifecycle,
    reason = '',
    owners = [],
    cycleLeft = '',
    cyclePct = null,
    threads = [],
    closed = false,
    closure = '',
    people = [],
    onopenthread,
    onnewthread,
    ondeletethread,
    onpriority,
    onowners,
    onclose,
    onreopen,
    loadBrief,
    renderBrief,
    saveBrief,
    attachBrief,
  }: {
    open?: boolean;
    naming?: boolean;
    name: string;
    /** su id: a qué burbuja pertenece lo que se lee y se guarda aquí */
    bubble?: string;
    lifecycle: Lifecycle;
    reason?: string;
    /** who is accountable, by id. Plural because the work is. */
    owners?: string[];
    /** what is left of the cycle, in words */
    cycleLeft?: string;
    /** and the same thing as 0..100, for the bar. `null` draws no bar: a bar
     *  filled with a number nobody computed is worse than no bar. */
    cyclePct?: number | null;
    /** whether it has already been closed. The band says so too, but the panel
     *  offers a different verb depending on the answer. */
    closed?: boolean;
    /** how it ended, if it did — la frase de quien la cerró */
    closure?: string;
    /** who could be accountable for this: the workspace's roster */
    people?: { id: string; name: string; avatar?: string }[];
    onopenthread?: (seq: number) => void;
    /** create one with the name typed here */
    onnewthread?: (name: string) => void;
    ondeletethread?: (seq: number) => void;
    /** cambiar la prioridad PROPIA de un hilo. Sólo se pasa al lead global. */
    onpriority?: (seq: number, priority: string) => void;
    /** An empty list hands it back to nobody, which is a real answer and has a
     *  cost: a quiet bubble with nobody accountable is 🪦, not 😴. */
    onowners?: (ids: string[]) => void;
    onclose?: () => void;
    onreopen?: () => void;
    /** El brief que el planeador escribió, en markdown; vacío si no hay. Se
     *  pide al abrirlo y no antes: es largo y casi nunca se lee. */
    loadBrief?: (bubble: string) => Promise<string>;
    /** markdown → html, con el workspace de la burbuja para sus imágenes */
    renderBrief?: (md: string) => Promise<string>;
    /** guardarlo. Sólo se pasa a quien puede editar el plan: el lead global */
    saveBrief?: (bubble: string, md: string) => Promise<void>;
    /** subir una imagen pegada mientras se edita */
    attachBrief?: (file: File) => Promise<{ path: string }>;
    threads?: {
      seq: number;
      title: string;
      lifecycle: Lifecycle;
      priority?: string;
      state?: string;
      owner?: string;
      age?: string;
      /** su lugar en la línea viva del departamento: 1 = ahora; 0, fuera */
      step?: number;
    }[];
  } = $props();

  // Dos órdenes para la misma lista. «Recientes» contesta qué se movió; «En
  // secuencia», qué toca hacer de esta burbuja y en qué paso de la línea de
  // TODO el departamento va cada pieza. Lo que no está en la secuencia va al
  // final, en su orden de siempre.
  let order = $state<'recent' | 'sequence'>('recent');
  const inLine = $derived(threads.some((t) => (t.step ?? 0) > 0));
  const shownThreads = $derived(
    order === 'recent'
      ? threads
      : [
          ...threads.filter((t) => (t.step ?? 0) > 0).sort((a, b) => (a.step ?? 0) - (b.step ?? 0)),
          ...threads.filter((t) => !(t.step ?? 0)),
        ],
  );
  const stepText = (n: number) => (n === 1 ? 'ahora' : n === 2 ? 'siguiente' : `paso ${n}`);

  // The thread list fades at whichever edge still has list behind it, the same
  // as the project column and the board.
  const fade = edgeFade();

  let fresh = $state('');
  const nameOf = (id: string) => people.find((p) => p.id === id)?.name ?? id;

  // Skeleton's Combobox, multiple. Not a native `<select multiple>`, which on a
  // Mac is a scrolling box you ⌘-click into and on every platform is a control
  // nobody recognises as "add a person". The collection is Zag's, filtered as
  // you type; who is already on it is drawn as chips underneath, because the
  // input shows what you are SEARCHING and the chips show what is true.
  let hunting = $state('');
  const shortlist = $derived(
    people.filter((p) => p.name.toLowerCase().includes(hunting.trim().toLowerCase())),
  );
  // «¿De qué trata?»: el brief del planeador, leído aquí mismo.
  //
  // El brief es lo que el planeador escribe —por qué, qué incluye, lo que se
  // sabe— y hasta ahora sólo se veía desde el planeador, así que quien ejecuta
  // no leía lo que se planeó. Sustituye al outcome que iba bajo el nombre.
  // Ocupa el sitio de la lista de threads en vez de abrir otra pantalla, y se
  // vuelve con ←.
  let reading = $state(false);
  /** el brief como está guardado; `null` mientras no se ha leído */
  let md = $state<string | null>(null);
  let html = $state('');
  let failed = $state('');

  // Editar el plan: sólo el lead global, que es quien da forma a esa capa.
  // Termina guardando por cualquier salida —«Listo», «← threads» o cerrar el
  // cajón—; sólo «Cancelar» descarta. Es la misma regla que la tarjeta del
  // planeador: lo escrito no se pierde por la forma de salir.
  let editingPlan = $state(false);
  // De QUIÉN es lo que se está escribiendo, fijado al empezar. Al cambiar de
  // burbuja se guarda lo pendiente, y guardarlo en la burbuja que acaba de
  // abrirse sería escribir un plan en otra.
  let editingOf = '';
  let draft = $state('');
  let writing = $state(false);
  let saving = $state(false);

  async function fetchBrief() {
    if (!loadBrief) return;
    failed = '';
    md = null;
    const of = bubble;
    try {
      const got = await loadBrief(of);
      const out = got.trim() && renderBrief ? await renderBrief(got) : '';
      // Llegó tarde, para una burbuja que ya no está abierta.
      if (of !== bubble) return;
      html = out;
      md = got;
    } catch (e) {
      failed = (e as Error).message;
    }
  }

  async function readBrief() {
    reading = true;
    editingPlan = false;
    await fetchBrief();
  }

  async function editBrief() {
    reading = true;
    if (md === null) await fetchBrief();
    if (md === null) return;
    draft = md;
    editingOf = bubble;
    writing = true;
    editingPlan = true;
  }

  /** Guarda si cambió y vuelve a leerlo. `false` si no se pudo guardar. */
  async function finishEdit(): Promise<boolean> {
    if (!editingPlan) return true;
    if (md !== null && draft !== md && saveBrief) {
      saving = true;
      try {
        await saveBrief(editingOf, draft);
        md = draft;
        html = draft.trim() && renderBrief ? await renderBrief(draft) : '';
      } catch (e) {
        failed = `No se guardó: ${(e as Error).message}`;
        return false;
      } finally {
        saving = false;
      }
    }
    editingPlan = false;
    return true;
  }

  async function backToThreads() {
    if (await finishEdit()) reading = false;
  }

  // Otra burbuja, o el cajón cerrado: se vuelve a la lista, guardando antes lo
  // que se estuviera escribiendo. Abrir una burbuja y encontrarse leyendo el
  // plan de la anterior sería leer lo que no es.
  $effect(() => {
    void bubble;
    void open;
    return () => {
      untrack(() => void finishEdit());
      reading = false;
      md = null;
    };
  });

  const roster = $derived(
    useListCollection({
      items: shortlist,
      itemToString: (p: { id: string; name: string }) => p.name,
      itemToValue: (p: { id: string; name: string }) => p.id,
    }),
  );

</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Portal>
    <!-- INSIDE the portal, like the panel it dims.
         Left outside, the backdrop renders where the drawer was declared —
         inside the scrolling pane — and that pane carries a `mask-image` for
         its edge fade. A mask establishes a containing block for fixed
         positioning, so `position: fixed; inset: 0` stopped meaning "the
         viewport" and started meaning "the pane": the board went dim and the
         sidebar beside it stayed lit, above a scrim that could not reach it. -->
    <Dialog.Backdrop class="scrim" />
    <Dialog.Positioner class="drawer-pos">
    <Dialog.Content class="drawer band-{lifecycle}">
      <header class="flex items-start gap-3">
        <!-- The band, first: it is the thing you came to find out, and it reads
             before the name does. -->
        <span class="mt-0.5 text-xl" title={reason}>{bandFace(lifecycle)}</span>
        <Dialog.Title class="min-w-0 flex-1 truncate text-lg font-bold">{name}</Dialog.Title>
        <Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface">✕</Dialog.CloseTrigger>
      </header>

      {#if closed && closure}
        <p class="closed-note mt-2">🏆 {closure}</p>
      {/if}

      {#if reading}
        <div class="mt-3 flex items-center gap-2">
          <button class="back" onclick={backToThreads} disabled={saving}>← hilos</button>
          <span class="flex-1"></span>
          {#if editingPlan}
            <button class="back" onclick={() => (editingPlan = false)} disabled={saving}>Cancelar</button>
            <button class="btn btn-sm preset-filled-primary-500" onclick={finishEdit} disabled={saving}>
              {saving ? 'guardando…' : 'Listo'}
            </button>
          {:else if saveBrief && md !== null}
            <button class="back" onclick={editBrief}>editar plan</button>
          {/if}
        </div>
        {#if failed}<p class="failed mt-2" role="alert">{failed}</p>{/if}
        <div class="brief mt-2" class:editing={editingPlan}>
          {#if editingPlan}
            <MarkdownField
              bind:value={draft}
              bind:editing={writing}
              limit="bubbles.brief"
              render={renderBrief}
              onattach={attachBrief}
              fill
              placeholder="el plan de esto: por qué, qué incluye, lo que se sabe — pega imágenes o escribe /mermaid" />
          {:else if md === null && !failed}
            <p class="faint text-sm">leyendo…</p>
          {:else if html}
            <Prose compact {html} />
          {:else if md !== null}
            <p class="faint text-sm">
              {saveBrief
                ? 'Todavía no hay plan escrito. «editar plan» lo empieza aquí mismo.'
                : 'El planeador todavía no escribió de qué trata. Se escribe en la tarjeta de esta burbuja, en «Descripción».'}
            </p>
          {/if}
        </div>
      {:else}

      {#if loadBrief}
        <!-- Juntos, sin hueco en medio: la pregunta y sus respuestas se leen
             como una frase. -->
        <div class="about mt-2">
          <span class="faint">¿de qué trata?</span>
          <button class="about-go" onclick={readBrief}>leer el plan →</button>
          {#if saveBrief}
            <span class="faint" aria-hidden="true">·</span>
            <button class="about-go" onclick={editBrief}>editar plan</button>
          {/if}
        </div>
      {/if}

      {#if onowners}
        <!-- Quién está a cargo no es decoración: sin nadie, una burbuja que se
             enfría cae a 🪦 en vez de a 😴, porque no hay a quién preguntarle.
             Por eso se dice aquí lo que cuesta dejarlo vacío. -->
        <div class="owners mt-2">
          <span class="faint shrink-0">a cargo</span>
          <Combobox
            openOnClick
            class="min-w-0 flex-1"
            multiple
            placeholder={owners.length ? 'agregar a alguien…' : 'nadie'}
            collection={roster}
            value={owners}
            inputValue={hunting}
            onInputValueChange={(e: { inputValue: string }) => (hunting = e.inputValue)}
            onOpenChange={() => (hunting = '')}
            onValueChange={(e: { value: string[] }) => onowners?.(e.value)}>
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
                    <p class="faint px-2 py-1 text-xs">nadie más en este proyecto</p>
                  {/each}
                </Combobox.Content>
              </Combobox.Positioner>
            </Portal>
          </Combobox>
        </div>
        {#if owners.length}
          <!-- Las fichas dicen lo que ES; el campo de arriba dice lo que estás
               buscando. Quitar a alguien se hace aquí, en la ficha. -->
          <ul class="chips">
            {#each owners as id (id)}
              <li class="chip">
                {#if people.find((p) => p.id === id)?.avatar}
                  <img class="chip-face" src={people.find((p) => p.id === id)?.avatar} alt="" />
                {/if}
                {nameOf(id)}
                <button
                  aria-label="quitar a {nameOf(id)}"
                  onclick={() => onowners?.(owners.filter((x) => x !== id))}>×</button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="faint mt-1 text-xs">nadie a cargo · si se enfría, cae a 🪦</p>
        {/if}
      {:else if owners.length}
        <p class="faint mt-2 text-sm">a cargo: {owners.map(nameOf).join(', ')}</p>
      {/if}

      <!-- The cycle, as a bar. It used to be a sentence about the last thing
           that happened, which answers "what" when the question a bubble raises
           is "how much of the window is left". -->
      <!-- The number sits BESIDE the bar, not over it: on its own line it cost
           a whole row of height to say what the bar is already showing, and it
           pushed the bar away from the subtitle it belongs to. "ciclo · quedan
           6 d" and the owner are gone for the same reason. -->
      {#if cyclePct !== null}
        <div class="mt-3 flex items-center gap-3" title={cycleLeft}>
          <Progress value={cyclePct} class="flex-1">
            <Progress.Track><Progress.Range /></Progress.Track>
          </Progress>
          <span class="faint shrink-0 text-xs tabular-nums">{cyclePct}%</span>
        </div>
      {/if}

      <div class="listhead mt-4">
        <p class="faint text-xs">
          {threads.length} hilos · {order === 'recent' ? 'el más reciente primero' : 'en el orden de la secuencia'}
        </p>
        {#if inLine || order === 'sequence'}
          <div class="seg" role="group" aria-label="orden">
            <button class:on={order === 'recent'} aria-pressed={order === 'recent'} onclick={() => (order = 'recent')}>Recientes</button>
            <button class:on={order === 'sequence'} aria-pressed={order === 'sequence'} onclick={() => (order = 'sequence')}>🧭 Secuencia</button>
          </div>
        {/if}
      </div>
      <ul class="threads mt-1 space-y-0.5" style={fade.style} {@attach fade.attach}>
        {#each shownThreads as t (t.seq)}
          <!-- Two lines, because one was hiding the half that answers "should I
               open this?": its own band and why, who has it, and how old it is.
               The title alone only answers "what is it called". -->
          <li class="tile band-{t.lifecycle}" class:sunk={t.lifecycle === 'dormant'}>
            <button
              class="tile-open rounded-lg px-2 py-2 text-left"
              onclick={() => onopenthread?.(t.seq)}>
              <!-- The first line keeps 28px clear on the right: that is where
                   the ✕ appears, and without the gap it landed on top of the
                   priority. Reserved rather than shifted on hover, so nothing
                   moves under the pointer. -->
              <span class="flex items-center gap-2 pr-7">
                <span class="faint shrink-0 tabular-nums">#{t.seq}</span>
                <span class="min-w-0 flex-1 truncate text-sm">{t.title}</span>
                {#if t.step}
                  <span class="step" class:now={t.step === 1} title="su lugar en la secuencia del departamento">🧭 {stepText(t.step)}</span>
                {/if}
                <PriorityBadge
                  size="xs"
                  value={t.priority ?? ''}
                  canEdit={!!onpriority}
                  onchange={(p) => onpriority?.(t.seq, p)} />
              </span>
              <!-- The band, who has it, and how long since. The column's own
                   state — Backlog, En curso — is not on here: it is a place a
                   card sits in some other tool, and the band already answers
                   whether this is moving. -->
              <span class="faint mt-0.5 block truncate text-xs">
                {bandFace(t.lifecycle)} {bandName(t.lifecycle)}{t.owner
                  ? ` · ${t.owner}`
                  : ''}{t.age ? ` · ${t.age}` : ''}
              </span>
            </button>
            <!-- Outside the tile's own button, so removing a thread is never a
                 mis-aimed attempt to open it. Y sólo si hay a quién pedírselo:
                 sin manejador esta × era un botón que no hacía nada. -->
            {#if ondeletethread}
              <button
                class="tile-x"
                aria-label="eliminar hilo #{t.seq}"
                onclick={() => ondeletethread?.(t.seq)}>×</button>
            {/if}

          </li>
        {/each}
      </ul>

      <!-- Pinned, like the sidebar's: creating a thread is the one action the
           panel always offers, so it sits where the panel ends rather than
           drifting down as threads are added. -->
      <div class="new-thread">
        <!-- A thread needs a NAME, and it is the only thing it needs. Asking
             for it here — rather than creating "Thread nuevo" and hoping
             somebody renames it — is the difference between a list of work and
             a list of placeholders. -->
        {#if !onnewthread}
          <!-- Nada: en el board de TODOS no hay proyecto al que mandar un thread
               nuevo, y un campo que no puede contestar «dónde» es un campo que
               falla al enviarse. Se crea desde el board de su workspace. -->
        {:else if naming}
          <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
            class="new-name"
            placeholder="¿Cómo se llama?"
            {@attach (el: HTMLInputElement) => el.focus()}
            bind:value={fresh} {@attach limited('threads.name')}
            onblur={() => (naming = false)}
            onkeydown={(e) => {
              if (e.key === 'Enter' && fresh.trim()) {
                onnewthread?.(fresh.trim());
                fresh = '';
                naming = false;
              }
              if (e.key === 'Escape') naming = false;
            }} />
        {:else}
          <button class="btn btn-sm w-full preset-tonal-surface" onclick={() => (naming = true)}>
            + hilo
          </button>
        {/if}
        <!-- Debajo de "+ thread": crear es lo de todos los días y va primero;
             cerrar o reabrir la burbuja es una decisión de vez en cuando. -->
        {#if closed && onreopen}
          <!-- Una burbuja cerrada se reabre: cerrarla fue una decisión, no un
               borrado, y el modelo dice que se puede redefinir. -->
          <button class="verb mt-2" onclick={() => onreopen?.()}>Reabrir la burbuja</button>
        {:else if onclose}
          <button class="verb mt-2" onclick={() => onclose?.()}>Cerrar la burbuja</button>
        {/if}
      </div>
      {/if}
    </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>

  .closed-note {
    margin: 0;
    padding: 0.35rem 0.55rem;
    border-radius: 9px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.82rem;
  }

  .owners { display: flex; align-items: center; gap: 0.5rem; font-size: 0.82rem; }
  /* La misma densidad que los campos del planeador: un solo tipo de caja en
     todo el producto, para que un combobox aquí no parezca de otra aplicación.
     El input trae el estilo de campo de Skeleton — su propio radio y padding —
     que dentro del control se lee como una caja dibujada dentro de otra. */
  .owners :global([data-scope='combobox'][data-part='root']) { min-width: 0; width: 100%; }
  .owners :global([data-scope='combobox'][data-part='control']) {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    min-height: 2rem;
    padding: 0.1rem 0.4rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
  }
  .owners :global([data-scope='combobox'][data-part='control']:focus-within) {
    border-color: color-mix(in oklab, var(--accent) 60%, transparent);
  }
  .owners :global(input) {
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
  .owners :global([data-scope='combobox'][data-part='trigger']) {
    flex: none;
    color: var(--faint);
    font-size: 0.8rem;
  }
  /* La lista está portada a <body>, fuera del alcance de un selector con
     ancestro: se estiliza por su clase, que es lo único que viaja con ella. */
  :global(.who-list) { min-width: max(var(--reference-width, 0px), 13rem); }
  :global(.who-list [data-part='item']) {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.3rem 0.6rem;
    border-radius: 7px;
    font-size: 0.84rem;
  }

  /* El margen vive AQUÍ, no en una utilidad: `.chips { margin: 0 }` con el
     atributo de scope de Svelte le gana en especificidad a `mt-2.5`, así que la
     clase de Tailwind se aplicaba y no movía nada. */
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin: 0.7rem 0 0;
    padding: 0;
    list-style: none;
  }
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

  /* El verbo destructivo del panel, tono bajo: está siempre a la vista y no
     debe competir con "+ thread", que es lo que se pulsa todos los días. */
  .verb {
    width: 100%;
    padding: 0.35rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: transparent;
    color: var(--muted);
    font-size: 0.82rem;
  }
  .verb:hover { background: var(--hover); color: var(--text); }

  .new-name {
    width: 100%;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--accent, var(--line));
    border-radius: 9px;
    background: var(--surface);
    color: var(--text);
    font-size: 0.9rem;
  }

  /* The panel is a column: header and the new-thread button hold still, the
     list is the only part that moves. */
  :global(.drawer) {
    display: flex;
    flex-direction: column;
  }
  /* Una fila como la de «a cargo»: etiqueta tenue a la izquierda, lo que se
     puede hacer a la derecha. */
  /* Una fila como la de «a cargo»: la pregunta, y justo a su lado lo que se
     puede hacer con ella. */
  .listhead { display: flex; flex: none; align-items: center; justify-content: space-between; gap: 0.5rem; }
  .listhead p { margin: 0; }
  .seg { display: inline-flex; padding: 2px; border: 1px solid var(--line); border-radius: 8px; background: var(--surface); }
  .seg button { padding: 0.05rem 0.45rem; border-radius: 6px; color: var(--faint); font-size: 0.7rem; }
  .seg button.on { background: var(--hover); color: var(--text); font-weight: 600; }
  .step {
    flex: none;
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.66rem;
    white-space: nowrap;
  }
  .step.now { background: color-mix(in oklab, var(--accent) 22%, transparent); color: var(--text); font-weight: 600; }
  .about {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    width: 100%;
    padding: 0.3rem 0;
    font-size: 0.85rem;
  }
  .about-go { color: var(--muted); }
  .about-go:hover { color: var(--text); text-decoration: underline; }
  .failed {
    margin: 0;
    padding: 0.4rem 0.6rem;
    border-radius: 8px;
    background: color-mix(in oklab, var(--color-error-500) 12%, transparent);
    color: var(--text);
    font-size: 0.8rem;
  }
  .back {
    align-self: flex-start;
    padding: 0.2rem 0.5rem;
    border-radius: 8px;
    color: var(--muted);
    font-size: 0.82rem;
  }
  .back:hover { background: var(--hover); color: var(--text); }
  /* Editando, el campo se estira hasta abajo y desplaza por dentro: el
     contenedor no desplaza, sólo le da el alto. */
  .brief.editing {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    padding-bottom: 0.25rem;
  }
  /* El brief ocupa lo que ocupaba la lista, y se desplaza igual que ella. */
  .brief {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    margin-inline: -1.25rem;
    padding-inline: 1.25rem;
    scrollbar-width: thin;
  }

  .threads {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    /* The bar rides the panel's own edge rather than floating inside the
       padding, as it does in the sidebar. */
    margin-inline: -1.25rem;
    padding-inline: 1.25rem;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  /* The ✕ is positioned ON the tile rather than beside it: taking a column of
     its own narrowed every title by 26px to serve an action used once. */
  .tile { position: relative; }
  .tile-open { display: block; width: 100%; }
  /* Driven from the TILE, not from the button. With the hover on the button
     itself, moving onto the ✕ — which is a sibling, not a child — dropped the
     highlight, so the row you were about to act on stopped looking like the row
     you were about to act on. */
  .tile:hover .tile-open,
  .tile:focus-within .tile-open { background: var(--hover); }
  .tile-x {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 24px;
    height: 24px;
    display: grid;
    place-content: center;
    border-radius: 7px;
    background: var(--surface-solid);
    color: var(--faint);
    font-size: 1rem;
    line-height: 1;
    opacity: 0;
    transition: opacity 0.12s ease;
  }
  /* Only on the tile being pointed at. A column of permanent ✕ is a column of
     invitations to delete something. Focus counts as pointing: otherwise the
     button exists for a mouse and for nobody else. */
  .tile:hover .tile-x,
  .tile:focus-within .tile-x { opacity: 1; }
  .tile-x:hover { background: var(--hover); color: var(--text); }

  .new-thread {
    flex: none;
    margin-top: auto;
    padding-top: 0.6rem;
    border-top: 1px solid var(--line);
  }
  .new-thread button { transition: background 0.14s ease, color 0.14s ease; }
  .new-thread button:hover {
    background: var(--hover);
    color: var(--text);
  }

  /* The bar takes the BAND's accent: the drawer already says which band it is,
     and a black bar in a hot bubble is a second colour saying nothing. */
  :global(.drawer [data-scope='progress'][data-part='range']) {
    background: var(--accent);
  }

  /* Skeleton positions a dialog centred. A drawer is the same component pinned
     to one edge, which is a placement decision rather than a different widget. */
  /* `.scrim` itself lives in app.css now — one dimming for every modal. What is
     left here is only where this one sits in the stack. */
  :global(.scrim) { z-index: var(--z-drawer-scrim); }
  :global(.drawer-pos) {
    position: fixed;
    inset: 0;
    z-index: var(--z-drawer);
    display: flex;
    justify-content: flex-end;
    pointer-events: none;
  }
  :global(.drawer) {
    pointer-events: auto;
    height: 100vh;
    /* Wide enough for two lines of thread without either wrapping. */
    width: min(560px, 96vw);
    overflow: hidden;
    /* Bottom equal to the sides. It was 1.5rem against 1.25rem, which reads as
       the button sitting slightly low rather than as deliberate breathing
       room. */
    padding: 1.15rem 1.25rem 1.25rem;
    background: var(--surface-solid);
    border-left: 1px solid var(--line);
    box-shadow: -24px 0 60px var(--shadow-strong);
    animation: slide 0.22s cubic-bezier(0.2, 0.8, 0.3, 1);
  }
  @keyframes slide {
    from {
      transform: translateX(24px);
      opacity: 0.4;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    :global(.drawer) { animation: none; }
  }
</style>
