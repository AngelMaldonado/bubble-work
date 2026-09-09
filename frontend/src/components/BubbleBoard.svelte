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
    /** su workspace, cuando el board es de varios */
    project?: string;
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
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import { bandOrder, bandFace, bandName } from '../lib/bands';

  let {
    title = '',
    bubbles = [],
    onopen,
    onaction,
    onnew,
    controls,
    children,
  }: {
    title?: string;
    bubbles?: BoardBubble[];
    /** los controles del board, sobre las bandas: a qué proyectos limitarse */
    controls?: import('svelte').Snippet;
    onopen?: (bubble: BoardBubble) => void;
    onaction?: (what: string, bubble: BoardBubble) => void;
    /** right-click on the empty part of the board: what to make here */
    onnew?: (what: 'bubble' | 'thread') => void;
    /** anything that belongs under the bands — the footer saying what heat was
     *  computed against, the threads with no bubble */
    children?: import('svelte').Snippet;
  } = $props();

  let newOpen = $state(false);
  let boardEl = $state<HTMLElement | null>(null);
  // True only while the priming event below is in flight, so the open it
  // provokes is swallowed instead of shown.
  let priming = false;
  $effect(() => {
    const el = boardEl;
    if (!el || !onnew) return;
    priming = true;
    el.dispatchEvent(new MouseEvent('contextmenu', { bubbles: false, clientX: 0, clientY: 0 }));
    priming = false;
  });
</script>

<!-- Right-click on the board itself — the empty space between the orbs — is how
     something new is made. It is where the eye already is when the thought
     arrives ("this needs a bubble"), and it costs no button in a corner.

     Controlled and primed at mount for the same reason the orb's menu is: Zag
     repositions a context menu from a watcher over the anchor point, and that
     watcher cannot see the change that establishes the value — so the first
     right-click drew the menu in the top-left corner. Fixed once, here as
     there, rather than explained twice. -->
<Menu
  open={newOpen}
  onSelect={(e: { value: string }) => onnew?.(e.value as 'bubble' | 'thread')}
  onOpenChange={(e: { open: boolean }) => {
    if (priming) return;
    newOpen = e.open;
  }}>
  <Menu.ContextTrigger>
    {#snippet element(attributes: Record<string, unknown>)}
      <div class="board" bind:this={boardEl} {...attributes}>
  {#if title || controls}
    <header class="board-head">
      {#if title}<h1 class="display text-2xl">{title}</h1>{/if}
      {#if controls}{@render controls()}{/if}
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
                project={b.project ?? ''}
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
    {/snippet}
  </Menu.ContextTrigger>
  {#if onnew}
    <Portal>
      <Menu.Positioner>
        <Menu.Content>
          <Menu.Item value="bubble"><Menu.ItemText>Nueva burbuja</Menu.ItemText></Menu.Item>
          <Menu.Item value="thread"><Menu.ItemText>Nuevo thread</Menu.ItemText></Menu.Item>
        </Menu.Content>
      </Menu.Positioner>
    </Portal>
  {/if}
</Menu>

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
