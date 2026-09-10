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
  import { Combobox, Dialog, Portal, Progress, useListCollection } from '@skeletonlabs/skeleton-svelte';
  import { ago } from '../lib/when';
  import { edgeFade } from '../lib/fade.svelte';
  import type { Lifecycle } from '../lib/api';

  let {
    open = $bindable(false),
    // Bindable, because "+ thread" is reachable from OUTSIDE the drawer — the
    // orb's right-click opens the panel already asking for the name. One field,
    // two ways in, rather than a second place to name a thread.
    naming = $bindable(false),
    name,
    outcome = '',
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
    onoutcome,
    onowners,
    onclose,
    onreopen,
  }: {
    open?: boolean;
    naming?: boolean;
    name: string;
    outcome?: string;
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
    people?: { id: string; name: string }[];
    onopenthread?: (seq: number) => void;
    /** create one with the name typed here */
    onnewthread?: (name: string) => void;
    ondeletethread?: (seq: number) => void;
    /** what is true when this is done, rewritten */
    onoutcome?: (text: string) => void;
    /** An empty list hands it back to nobody, which is a real answer and has a
     *  cost: a quiet bubble with nobody accountable is 🪦, not 😴. */
    onowners?: (ids: string[]) => void;
    onclose?: () => void;
    onreopen?: () => void;
    threads?: {
      seq: number;
      title: string;
      lifecycle: Lifecycle;
      priority?: string;
      state?: string;
      owner?: string;
      age?: string;
    }[];
  } = $props();

  // The thread list fades at whichever edge still has list behind it, the same
  // as the project column and the board.
  const fade = edgeFade();

  let fresh = $state('');
  // The outcome is read most of the time and written rarely, so it is text
  // until you click it. An always-on textarea in the header would turn the
  // first thing you read about a bubble into a form.
  let saying = $state(false);
  let said = $state('');
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

      <!-- The outcome starts where the EMOJI starts, not where the name does.
           Indenting it under the title made it look like a caption on the name;
           it is a claim about the bubble, and it reads as one from the margin. -->
      {#if saying}
        <!-- ⌘/Ctrl+Enter guarda y Escape cancela; salir del campo también
             guarda, como en el editor del documento. -->
        <textarea
          class="outcome-edit mt-1"
          rows="2"
          placeholder="¿Qué es cierto cuando esto termine?"
          bind:value={said}
          {@attach (el: HTMLTextAreaElement) => el.focus()}
          onblur={() => { saying = false; if (said.trim() !== outcome) onoutcome?.(said.trim()); }}
          onkeydown={(e) => {
            if (e.key === 'Escape') { said = outcome; saying = false; }
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) (e.currentTarget as HTMLTextAreaElement).blur();
          }}></textarea>
      {:else if onoutcome}
        <!-- Editable en su sitio, no en un diálogo aparte: el outcome ES la
             burbuja — sin él es una carpeta con nombre bonito — y mandarlo a
             otra pantalla es cómo se queda vacío para siempre. -->
        <button class="outcome mt-1" onclick={() => { said = outcome; saying = true; }}>
          {#if outcome}{outcome}{:else}<span class="faint">+ ¿qué es cierto cuando esto termine?</span>{/if}
        </button>
      {:else if outcome}
        <Dialog.Description class="faint mt-1 text-sm">{outcome}</Dialog.Description>
      {/if}

      {#if closed && closure}
        <p class="closed-note mt-2">🏆 {closure}</p>
      {/if}

      {#if onowners}
        <!-- Quién está a cargo no es decoración: sin nadie, una burbuja que se
             enfría cae a 🪦 en vez de a 😴, porque no hay a quién preguntarle.
             Por eso se dice aquí lo que cuesta dejarlo vacío. -->
        <div class="owners mt-2">
          <span class="faint shrink-0">a cargo</span>
          <Combobox
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

      <p class="faint mt-4 shrink-0 text-xs">{threads.length} threads · el más reciente primero</p>
      <ul class="threads mt-1 space-y-0.5" style={fade.style} {@attach fade.attach}>
        {#each threads as t (t.seq)}
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
                {#if t.priority}<span class="faint shrink-0 text-xs">{t.priority}</span>{/if}
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
                aria-label="eliminar thread #{t.seq}"
                onclick={() => ondeletethread?.(t.seq)}>×</button>
            {/if}

          </li>
        {/each}
      </ul>

      <!-- Pinned, like the sidebar's: creating a thread is the one action the
           panel always offers, so it sits where the panel ends rather than
           drifting down as threads are added. -->
      <div class="new-thread">
        {#if closed && onreopen}
          <!-- Una burbuja cerrada se reabre: cerrarla fue una decisión, no un
               borrado, y el modelo dice que se puede redefinir. -->
          <button class="verb mb-2" onclick={() => onreopen?.()}>Reabrir la burbuja</button>
        {:else if onclose}
          <button class="verb mb-2" onclick={() => onclose?.()}>Cerrar la burbuja</button>
        {/if}
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
            bind:value={fresh}
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
            + thread
          </button>
        {/if}
      </div>
    </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  /* El outcome se lee como texto y se escribe donde se lee: mismo tamaño y
     mismo color en los dos estados, para que entrar a editarlo no mueva nada. */
  .outcome {
    display: block;
    width: 100%;
    padding: 0.15rem 0.3rem;
    margin-left: -0.3rem;
    border-radius: 7px;
    background: transparent;
    color: var(--muted);
    text-align: left;
    font-size: 0.875rem;
    line-height: 1.4;
  }
  .outcome:hover { background: var(--hover); }
  .outcome-edit {
    display: block;
    width: 100%;
    padding: 0.3rem 0.4rem;
    border: 1px solid var(--accent, var(--line));
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.875rem;
    resize: vertical;
  }

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
