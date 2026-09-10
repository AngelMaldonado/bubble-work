<script lang="ts">
  // The bubble. The star of the interface, and the reason the model is called
  // what it is.
  //
  // A sphere rather than a card because the claim is PHYSICAL: attention behaves
  // like a fluid, hot rises, cold sinks. Cards in a grid say "list with colours";
  // orbs floating in a band say what the model actually means, before anybody
  // reads a word. The bob, the sheen and the hover lift are not decoration —
  // they are what makes it read as buoyant rather than as a coloured circle.
  //
  // The five palettes carry the same hues as the flat accents in app.css, so an
  // orb and a dot never disagree about what "dormant" looks like.
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import type { Lifecycle } from '../lib/api';

  let {
    name,
    lifecycle,
    burning = 0,
    people = [],
    owner = '',
    project = '',
    index = 0,
    onclick,
    onaction,
  }: {
    name: string;
    lifecycle: Lifecycle;
    burning?: number;
    people?: string[];
    owner?: string;
    /** de qué workspace es, cuando el board es de más de uno. Vacío en el board
     *  de un proyecto: ahí decirlo en cada orbe es repetir la pantalla entera. */
    project?: string;
    index?: number;
    onclick?: () => void;
    onaction?: (what: string) => void;
  } = $props();

  // Staggered so a band of orbs breathes instead of pulsing in unison. Seven
  // steps: enough that a row never lines up, small enough to stay a rhythm.
  const delay = $derived(`${(index % 7) * 0.45}s`);

  const MAX = 3;
  // Sin vacíos y sin repetidos, y en ese orden: el dueño primero.
  //
  // Esta lista es la CLAVE del `{#each}` de abajo, y una clave repetida no es un
  // dibujo feo — Svelte lanza `each_key_duplicate` y ABORTA la actualización, lo
  // que deja la pantalla anterior en su sitio con el estado ya cambiado. Pasó:
  // dos personas a cargo cuyos nombres aún no habían cargado llegaban como dos
  // cadenas vacías, y el board no volvía a dibujarse nunca.
  const ordered = $derived(
    [...new Set([owner, ...people].filter(Boolean))],
  );
  const shown = $derived(ordered.slice(0, MAX));
  const extra = $derived(Math.max(0, ordered.length - shown.length));
  const initial = (s: string) => (s[0] ?? '?').toUpperCase();

  // The menu is CONTROLLED, and that is not a style choice — it is the only
  // right-click path that positions itself.
  //
  // Zag's uncontrolled `CONTEXT_MENU` transition opens with
  // `[setAnchorPoint, setTriggerValue, invokeOnOpen]` and NO `reposition`; it
  // leans on a watcher over the anchor point, which cannot see the first
  // null → point change because the tracker is primed in the same cycle. So the
  // first right-click left `--x`/`--y` unset, `transform: translate3d(var(--x),
  // var(--y), 0)` invalid, and the menu in the top-left corner; the second one
  // worked. Passing `open` takes the controlled branch, whose `CONTROLLED.OPEN`
  // carries `reposition` explicitly. Verified in a browser, not reasoned about:
  // Skeleton's own documented example fails the same way without it.
  //
  // It doubles as what v0 had: the orb HOLDS its hover while its menu is open.
  // Otherwise the pointer moves onto the menu, the orb drops back, and the menu
  // floats with nothing saying which bubble it belongs to.
  let menuOpen = $state(false);
  let orbEl = $state<HTMLButtonElement | null>(null);
  // True only while the priming event below is in flight, so the open it
  // provokes is swallowed instead of shown.
  let priming = false;

  // Prime the anchor point once, at mount.
  //
  // Zag repositions a context menu from a watcher over the anchor point, and a
  // watcher cannot see the change that establishes the value it is watching:
  // the tracker is primed in the same cycle that sets it. So the FIRST
  // right-click positioned nothing and the menu drew at the top-left corner;
  // the second one worked, because by then the point was changing from one
  // value to another. Giving it a value at mount makes every real right-click a
  // change. Verified in a browser both ways — Skeleton's own documented example
  // fails identically without this, and @zag-js/menu 1.43.3 does not fix it.
  $effect(() => {
    const el = orbEl;
    if (!el) return;
    priming = true;
    el.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, clientX: 0, clientY: 0 }));
    priming = false;
  });
</script>

<!-- Right-click opens the actions. Zag's ContextTrigger, not a hand-rolled
     `oncontextmenu`: it also opens on a long press and on the keyboard's own
     context key, which a mouse-only handler silently drops.
     ContextTrigger renders a <button> of its own, so the orb is passed to it as
     the `element` snippet rather than nested inside it. A button inside a
     button is not markup the parser keeps: it closes the outer one and
     reparents, and Zag ends up measuring an element that no longer holds the
     orb — which is how the menu opened at the top-left corner. -->
<Menu
  open={menuOpen}
  onSelect={(e: { value: string }) => onaction?.(e.value)}
  onOpenChange={(e: { open: boolean }) => {
    if (priming) return;
    menuOpen = e.open;
  }}>
<div class="wrap lvl-{lifecycle}" class:held={menuOpen} style="--d: {delay}">
  <Menu.ContextTrigger>
  {#snippet element(attributes: Record<string, unknown>)}
  <!-- Zag's own handler runs, and then the event stops here.
       The board behind this orb has a context menu of its own — "nueva burbuja"
       on the empty space — and a right-click on an orb was reaching both: the
       orb's menu is on the orb, the board's is on an ancestor, and an event
       that nobody stops visits every element on the way up. Composed rather
       than replaced: spreading `attributes` after our own handler would drop
       Zag's, and the orb's menu would be the one that never opened. -->
  <button
    class="orb"
    bind:this={orbEl}
    {...attributes}
    oncontextmenu={(e: MouseEvent) => {
      (attributes.oncontextmenu as ((e: MouseEvent) => void) | undefined)?.(e);
      e.stopPropagation();
    }}
    {onclick}
    aria-label={name}>
    <span class="sheen"></span>
    <!-- Counts what is BURNING, not the inventory. A bubble carrying twelve
         finished threads is not a "12": that number reads as workload and
         competes with the band the orb already sits in. -->
    {#if burning > 0}
      <span class="badge" title="{burning} en curso">{burning}</span>
    {/if}
    {#if ordered.length}
      <span class="people">
        <!-- La posición forma parte de la clave: si dos personas comparten
             nombre visible, se dibujan dos avatares, no se cae la aplicación. -->
        {#each shown as p, i (p + ':' + i)}
          <span class="person" class:owner={p === owner} style="--i: {i}" title={p}>{initial(p)}</span>
        {/each}
        {#if extra}
          <span class="person more" style="--i: {shown.length}" title={ordered.slice(MAX).join(', ')}>+{extra}</span>
        {/if}
      </span>
    {/if}
  </button>
  {/snippet}
  </Menu.ContextTrigger>
  <span class="caption">
    <!-- El proyecto ENCIMA del nombre y en pequeño: en el board de todos hay que
         poder barrer la columna y saber de dónde es cada burbuja sin leer, y un
         nombre de proyecto del mismo tamaño que el de la burbuja compite con lo
         que sí se está leyendo. -->
    {#if project}<span class="project">{project}</span>{/if}
    {name}
  </span>
</div>
  <Portal>
    <Menu.Positioner>
    <Menu.Content>
      <Menu.Item value="open"><Menu.ItemText>Abrir</Menu.ItemText></Menu.Item>
      <Menu.Item value="thread"><Menu.ItemText>+ thread</Menu.ItemText></Menu.Item>
      <Menu.Separator />
      <Menu.Item value="rename"><Menu.ItemText>Renombrar</Menu.ItemText></Menu.Item>
      <Menu.Separator />
      <Menu.Item value="close"><Menu.ItemText>Cerrar la burbuja</Menu.ItemText></Menu.Item>
    </Menu.Content>
    </Menu.Positioner>
  </Portal>
</Menu>

<style>
  /* Vivid, opaque spheres — real contrast, not a tint of the page. */
  .lvl-hot {
    --g1: oklch(0.82 0.16 40); --g2: oklch(0.64 0.21 15);
    --g3: oklch(0.55 0.2 350); --glow: oklch(0.65 0.2 20);
  }
  .lvl-dormant {
    --g1: oklch(0.8 0.11 295); --g2: oklch(0.62 0.17 300);
    --g3: oklch(0.54 0.18 268); --glow: oklch(0.6 0.16 285);
  }
  /* Grey, and the only band that is: a grave should not be a colour. */
  .lvl-rip {
    --g1: oklch(0.74 0.02 265); --g2: oklch(0.56 0.02 265);
    --g3: oklch(0.44 0.02 265); --glow: oklch(0.5 0.02 265);
  }
  /* Gold, v0's `done`. Finishing is the point. */
  .lvl-closed {
    --g1: oklch(0.92 0.12 98); --g2: oklch(0.81 0.16 85);
    --g3: oklch(0.68 0.15 68); --glow: oklch(0.8 0.16 88);
  }

  .wrap {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.55rem;
    width: 108px;
    padding-top: 0.4rem;
    /* The bob lives on the WRAP so it never fights the orb's hover scale. */
    animation: bob 6.5s ease-in-out infinite;
    animation-delay: var(--d);
  }
  .wrap:hover,
  .wrap.held {
    z-index: 6;
    animation-play-state: paused;
  }

  .orb {
    position: relative;
    width: 84px;
    height: 84px;
    border-radius: 50%;
    border: none;
    padding: 0;
    cursor: pointer;
    background:
      radial-gradient(circle at 70% 78%, var(--g3), transparent 60%),
      linear-gradient(150deg, var(--g1) 8%, var(--g2) 52%, var(--g3) 96%);
    box-shadow:
      inset -6px -8px 16px color-mix(in oklab, var(--g3) 70%, black 10%),
      inset 4px 5px 10px color-mix(in oklab, var(--g1) 60%, white 30%);
    transition:
      transform 0.32s cubic-bezier(0.34, 1.56, 0.4, 1),
      filter 0.32s ease;
  }
  .wrap:hover .orb,
  .wrap.held .orb {
    transform: translateY(-8px) scale(1.2);
    filter: brightness(1.06) saturate(1.05);
  }
  .orb:focus-visible {
    outline: var(--default-outline-width) solid color-mix(in oklab, var(--glow) 70%, transparent);
    outline-offset: 3px;
  }

  /* The glossy catch-light: what makes it a sphere and not a disc. */
  .sheen {
    position: absolute;
    top: 12%;
    left: 16%;
    width: 42%;
    height: 34%;
    border-radius: 50%;
    background: radial-gradient(ellipse at 38% 34%, oklch(1 0 0 / 0.95), transparent 70%);
    filter: blur(1px);
    pointer-events: none;
  }

  .badge {
    position: absolute;
    top: -3px;
    right: -3px;
    min-width: 20px;
    height: 20px;
    padding: 0 5px;
    border-radius: 999px;
    display: grid;
    place-content: center;
    font-size: 0.66rem;
    font-weight: 800;
    color: oklch(0.98 0 0);
    background: color-mix(in oklab, var(--g2) 80%, black 12%);
    box-shadow: 0 2px 6px color-mix(in oklab, var(--glow) 50%, transparent);
    border: 1.5px solid var(--surface-solid);
  }

  .people {
    position: absolute;
    bottom: -2px;
    left: -2px;
    display: flex;
  }
  .person {
    width: 22px;
    height: 22px;
    border-radius: 999px;
    display: grid;
    place-content: center;
    font-size: 0.6rem;
    font-weight: 800;
    border: 1.5px solid var(--surface-solid);
    margin-left: -8px;
    /* The owner sits on top, everyone else behind, in order. */
    z-index: calc(20 - var(--i));
    color: var(--color-surface-900);
    background: color-mix(in oklab, var(--color-surface-100) 82%, var(--surface-solid));
  }
  .person:first-child { margin-left: 0; }
  .person.owner {
    color: var(--color-surface-950);
    background: var(--color-surface-100);
  }
  .person.more {
    background: color-mix(in oklab, var(--text) 12%, var(--surface-solid));
    font-size: 0.58rem;
  }

  .caption {
    /* NOT the display face. A bubble's name is DATA — often a hostname, a
       ticket number, somebody's shorthand — and a display face turns data into
       decoration. Boogaloo names the sections; the bubbles name themselves. */
    font-size: 0.84rem;
    font-weight: 600;
    line-height: 1.15;
    text-align: center;
    color: var(--text);
    max-width: 108px;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    /* A hostname has no break opportunity, so without this it filled one line
       and clipped — wasting half the space it had already reserved. */
    overflow-wrap: anywhere;
  }
  .project {
    display: block;
    font-size: 0.62rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--faint);
  }

  /* Somebody who asked not to be moved should not be. */
  @media (prefers-reduced-motion: reduce) {
    .wrap { animation: none; }
    .orb { transition: none; }
    .wrap:hover .orb,
  .wrap.held .orb { transform: none; }
  }

  @keyframes bob {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-6px); }
  }
</style>
