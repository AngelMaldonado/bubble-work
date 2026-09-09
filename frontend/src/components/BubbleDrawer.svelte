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
  import { Dialog, Portal, Progress } from '@skeletonlabs/skeleton-svelte';
  import { edgeFade } from '../lib/fade.svelte';
  import type { Lifecycle } from '../lib/api';

  let {
    open = $bindable(false),
    name,
    outcome = '',
    lifecycle,
    reason = '',
    owner = '',
    cycleLeft = '',
    cyclePct = null,
    threads = [],
    onopenthread,
    onnewthread,
    ondeletethread,
  }: {
    open?: boolean;
    name: string;
    outcome?: string;
    lifecycle: Lifecycle;
    reason?: string;
    owner?: string;
    /** what is left of the cycle, in words */
    cycleLeft?: string;
    /** and the same thing as 0..100, for the bar. `null` draws no bar: a bar
     *  filled with a number nobody computed is worse than no bar. */
    cyclePct?: number | null;
    onopenthread?: (seq: number) => void;
    /** create one with the name typed here */
    onnewthread?: (name: string) => void;
    ondeletethread?: (seq: number) => void;
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

  let naming = $state(false);
  let fresh = $state('');

</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Dialog.Backdrop class="scrim" />
  <Portal>
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
      {#if outcome}
        <Dialog.Description class="faint mt-1 text-sm">{outcome}</Dialog.Description>
      {/if}

      <!-- The cycle, as a bar. It used to be a sentence about the last thing
           that happened, which answers "what" when the question a bubble raises
           is "how much of the window is left". -->
      <!-- The number sits BESIDE the bar, not over it: on its own line it cost
           a whole row of height to say what the bar is already showing, and it
           pushed the bar away from the subtitle it belongs to. "ciclo · quedan
           6 d" and the owner are gone for the same reason. -->
      {#if cyclePct !== null}
        <div class="mt-3 flex items-center gap-3">
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
                 mis-aimed attempt to open it. -->
            <button
              class="tile-x"
              aria-label="eliminar thread #{t.seq}"
              onclick={() => ondeletethread?.(t.seq)}>×</button>

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
        {#if naming}
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
