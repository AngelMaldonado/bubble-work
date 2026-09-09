<script lang="ts" module>
  /** One person who is around. `idle` is a tab left open — true and less
   *  interesting than being here, so it is drawn dimmer rather than dropped. */
  export type Around = { id: string; name: string; idle?: boolean };
</script>

<script lang="ts">
  // Who is here, top right.
  //
  // Drawn from a list rather than from the server directly, so the mock and the
  // real app draw the SAME row — the component moved out of `MockPage` the
  // moment there was presence behind it, because a second copy of this CSS is
  // how the drawing and the thing drift apart.
  //
  // Fixed rather than in the pane: presence is about now, and it should not
  // scroll away with the board.
  import { Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';

  let { people = [], me = '' }: { people?: Around[]; me?: string } = $props();

  // The colour is DERIVED from the id, not stored: a hue column is a column
  // somebody has to pick, migrate and keep unique, to answer a question that a
  // hash answers for free and identically on every screen.
  const hue = (id: string) => {
    let h = 0;
    for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) % 360;
    return h;
  };
  const initial = (name: string) => (name[0] ?? '?').toUpperCase();
</script>

{#if people.length}
  <div class="presence" aria-label="en línea">
    {#each people as who, i (who.id)}
      <!-- A real tooltip rather than `title`: the browser's takes a second to
           appear, is drawn by the OS where no stylesheet reaches it, and cannot
           be themed. The trigger is passed as the `element` snippet so it stays
           OUR span — Skeleton's own trigger is a <button>, and wrapping the
           avatar in one would put a box around it and break the overlap. -->
      <Tooltip openDelay={120} closeDelay={60}>
        <Tooltip.Trigger>
          {#snippet element(attributes: Record<string, unknown>)}
            <span
              class="who"
              class:me={who.id === me}
              class:idle={who.idle}
              style="--hue: {hue(who.id)}; --i: {i}"
              {...attributes}>
              {initial(who.name)}
            </span>
          {/snippet}
        </Tooltip.Trigger>
        <Portal>
          <Tooltip.Positioner>
            <Tooltip.Content>
              {who.name}{who.id === me ? ' (tú)' : ''}{who.idle ? ' · pestaña abierta' : ''}
            </Tooltip.Content>
          </Tooltip.Positioner>
        </Portal>
      </Tooltip>
    {/each}
  </div>
{/if}

<style>
  .presence {
    position: fixed;
    top: 1rem;
    right: 1rem;
    z-index: var(--z-chrome);
    display: flex;
  }
  .who {
    /* Overlapped, and stacked so the leftmost — you — sits on top. */
    margin-left: -8px;
    z-index: calc(20 - var(--i));
    width: 30px;
    height: 30px;
    display: grid;
    place-content: center;
    border-radius: 999px;
    border: 2px solid var(--bg);
    background: oklch(0.72 0.13 var(--hue));
    color: oklch(0.22 0.05 var(--hue));
    font-size: 0.78rem;
    font-weight: 700;
    line-height: 1;
    transition: transform 0.14s ease;
  }
  .who:first-child { margin-left: 0; }
  /* Hover lifts one out of the stack: it grows and comes to the front, which is
     how you read a name in a row of initials without a tooltip getting there
     first. */
  .who:hover {
    transform: scale(1.25);
    z-index: 30;
  }
  /* Present but not at the keyboard. Dimmed rather than greyed: the colour is
     what makes a person recognisable at this size. */
  .idle { opacity: 0.45; }

  @media (prefers-reduced-motion: reduce) {
    .who { transition: none; }
  }
  /* You get the ring rather than a label: it is the only one you never need
     named. */
  .me { box-shadow: 0 0 0 2px var(--accent, oklch(0.68 0.2 40)); }
</style>
