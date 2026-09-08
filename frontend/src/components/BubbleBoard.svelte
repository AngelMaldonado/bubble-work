<script lang="ts" module>
  import type { Lifecycle } from '../lib/api';

  /** One bubble, as the board needs it. Whatever produced it — the server or
   *  the mock's fixtures — hands over the same five things. */
  export type BoardBubble = {
    id: string;
    name: string;
    life: Lifecycle;
    /** how many of its threads are producing: the flame on the orb */
    burning?: number;
    owner?: string;
    people?: string[];
  };
</script>

<script lang="ts">
  // v0's board, restyled. Bands stacked down the page, orbs CENTRED inside each
  // one, a hairline between bands, and an empty band still drawn — a band with
  // nothing in it is a fact about the work, and hiding it makes the board's
  // shape change under you. Closed lives at the bottom: what is done is worth
  // seeing and is never what you look at first.
  //
  // Drawn from a list rather than from the server's board directly, so the same
  // component renders the mock's fixtures and the real thing. Everything that
  // decides WHAT is on it — heat, membership, the cycle — stays on the server.
  import Orb from './Orb.svelte';
  import { bandOrder, bandFace, bandName } from '../lib/bands';

  let {
    title = '',
    bubbles = [],
    onopen,
    onaction,
    children,
  }: {
    title?: string;
    bubbles?: BoardBubble[];
    onopen?: (bubble: BoardBubble) => void;
    onaction?: (what: string, bubble: BoardBubble) => void;
    /** anything that belongs under the bands — the footer saying what heat was
     *  computed against, the threads with no bubble */
    children?: import('svelte').Snippet;
  } = $props();
</script>

<div class="board">
  {#if title}
    <header class="board-head">
      <h1 class="display text-2xl">{title}</h1>
    </header>
  {/if}

  {#each bandOrder as band (band)}
    {@const items = bubbles.filter((b) => b.life === band)}
    <section id="band-{band}" class="band band-{band}" class:empty={items.length === 0}>
      <header>
        <span class="ico">{bandFace(band)}</span>
        <span class="band-name">{bandName(band)}</span>
        <span class="count">{items.length}</span>
      </header>
      {#if items.length === 0}
        <p class="none">— nada aquí —</p>
      {:else}
        <div class="orbs">
          {#each items as b, i (b.id)}
            <!-- The anchor the minimap scrolls to. It wraps the orb rather than
                 sitting on it: `scrollIntoView` on a floating element lands
                 wherever the bob left it. -->
            <div id="bw-{b.id}">
              <Orb
                name={b.name}
                lifecycle={b.life}
                burning={b.burning ?? 0}
                owner={b.owner ?? ''}
                people={b.people ?? []}
                index={i}
                onclick={() => onopen?.(b)}
                onaction={onaction ? (what) => onaction(what, b) : undefined} />
            </div>
          {/each}
        </div>
      {/if}
    </section>
  {/each}

  {@render children?.()}
</div>

<style>
  /* The board, from v0: one column, centred, read down. */
  .board { max-width: 1100px; margin: 0 auto; padding: 2rem 1.5rem 6rem; }
  .board-head { text-align: center; margin-bottom: 2rem; }

  /* Room around the band. The hairline is a divider between groups of work,
     and with the orbs close to it the page read as one list with lines drawn
     through it rather than as bands. */
  .band { padding: 2rem 0 2.4rem; border-bottom: 1px solid var(--line); }
  .band:last-of-type { border-bottom: none; }
  /* An empty band is still drawn, just quieter. */
  .band.empty { opacity: 0.72; }
  .band > header {
    display: flex; align-items: center; justify-content: center;
    gap: 0.55rem; margin-bottom: 1.5rem;
  }
  .band .ico { font-size: 1.05rem; }
  /* `band-name`, not `label`: Skeleton owns `.label` (`width: 100%; display:
     block`), so a span of ours by that name stretched to the full row and threw
     the count against the right edge. Same collision as `.card`, which is why
     ours is `.glass`. */
  .band .band-name {
    font-weight: 750; letter-spacing: 0.04em; text-transform: uppercase;
    font-size: 0.82rem; color: var(--muted);
  }
  .band .count {
    font-size: 0.72rem; color: var(--muted);
    background: var(--hover); padding: 0.05rem 0.45rem; border-radius: 999px;
  }
  .orbs { display: flex; flex-wrap: wrap; justify-content: center; gap: 0.9rem; }
  .none { margin: 0; text-align: center; color: var(--faint); font-size: 0.8rem; font-style: italic; }
</style>
