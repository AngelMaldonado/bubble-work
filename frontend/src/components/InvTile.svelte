<script lang="ts">
  // Una pieza del inventario: un grupo o una cosa. La misma baldosa para las
  // dos, porque son la misma pregunta a dos alturas — «¿qué hay aquí?» — y dos
  // baldosas distintas para eso serían dos sitios donde arreglar el mismo
  // hover.
  //
  // El clic derecho usa el `ContextTrigger` de Zag y no un `oncontextmenu`
  // propio: también abre con pulsación larga y con la tecla de menú del
  // teclado, que un manejador de ratón deja fuera en silencio.
  import { Menu, Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import { guess, piece } from '../lib/art';

  export type TileAction = { value: string; label: string; tone?: 'danger' };

  let {
    title,
    sub = '',
    art = '',
    image = '',
    badge = '',
    urgent = false,
    hint = '',
    hex = '',
    actions = [],
    onopen,
    onaction,
  }: {
    title: string;
    sub?: string;
    /** el slug de la pieza; si viene vacío, la palabra decide */
    art?: string;
    /** una imagen subida gana siempre: el dibujo es un buen valor por defecto,
     *  no una opinión sobre lo que hay dentro */
    image?: string;
    /** lo que se ve sólo cuando importa: una renovación vencida o cercana */
    badge?: string;
    urgent?: boolean;
    /** la descripción, que vive en el tooltip: en la baldosa robaba una línea
     *  para decir algo que casi nunca se está buscando */
    hint?: string;
    /** el color de la marca, cuando la pieza es una marca */
    hex?: string;
    /** vacío = sin menú. Un menú con nada dentro es peor que ninguno. */
    actions?: TileAction[];
    onopen?: () => void;
    onaction?: (what: string) => void;
  } = $props();

  const slug = $derived(art || guess(title + ' ' + sub));
  const p = $derived(piece(slug, hex));

  /** Dos juegos de atributos sobre el MISMO botón: los del menú contextual y
   *  los del tooltip. Esparcir uno detrás del otro pisa los manejadores que
   *  comparten nombre —y el que se pierde es siempre el que hace falta— así que
   *  las funciones se encadenan en vez de reemplazarse. */
  function merge(a: Record<string, unknown>, b: Record<string, unknown>) {
    const out: Record<string, unknown> = { ...a };
    for (const [k, v] of Object.entries(b)) {
      const prev = out[k];
      if (typeof prev === 'function' && typeof v === 'function') {
        out[k] = (...args: unknown[]) => {
          (prev as (...a: unknown[]) => void)(...args);
          (v as (...a: unknown[]) => void)(...args);
        };
      } else if (k === 'class' && prev) {
        out[k] = `${prev} ${v}`;
      } else {
        out[k] = v;
      }
    }
    return out;
  }

  // Controlado, y primado al montar. Es la misma lección que el orbe del board:
  // Zag reposiciona un menú contextual desde un observador sobre el punto de
  // anclaje, y ese observador no puede ver el cambio que ESTABLECE el valor que
  // observa — así que el primer clic derecho dibujaba el menú en la esquina
  // superior izquierda y el segundo funcionaba. Darle un valor al montar
  // convierte cada clic derecho real en un cambio.
  let open = $state(false);
  let el = $state<HTMLElement | null>(null);
  let priming = false;
  $effect(() => {
    const node = el;
    if (!node || !actions.length) return;
    priming = true;
    node.dispatchEvent(new MouseEvent('contextmenu', { bubbles: false, clientX: 0, clientY: 0 }));
    priming = false;
  });
</script>

{#snippet face(attributes: Record<string, unknown> = {})}
  <button class="tile" class:held={open} bind:this={el} {...attributes} onclick={onopen}>
    <span class="shot">
      {#if image}
        <img class="still" src={image} alt="" />
      {:else if p.kind === 'render'}
        <!-- Dos capas puestas desde el principio: cambiar el `src` al pasar por
             encima parpadea mientras la segunda carga, y un parpadeo por baldosa
             convierte una galería en un nervio. Aquí sólo cambia la opacidad. -->
        <img class="still" src={p.still} alt="" />
        <img class="lit" src={p.lit} alt="" aria-hidden="true" />
      {:else}
        <!-- Una marca no trae volumen: es una silueta. El volumen lo pone el
             pedestal, y el color lo pone el hover — el SVG se usa como MÁSCARA
             para poder pintarlo, que con una imagen normal habría necesitado dos
             archivos por logo. -->
        <span class="pedestal">
          <span class="brand" style="--mask: url({p.mask}); --brand: #{p.hex}"></span>
        </span>
      {/if}
    </span>
    <span class="label">
      <b>{title}</b>
      {#if sub}<span class="faint">{sub}</span>{/if}
    </span>
    <!-- La alarma se queda a la vista. Lo demás cabe en el tooltip; una fecha
         vencida escondida detrás de un hover no es una alarma. -->
    {#if badge && urgent}<span class="alarm">{badge}</span>{/if}
  </button>
{/snippet}

{#snippet tipped(attributes: Record<string, unknown> = {})}
  {#if hint}
    <Tooltip positioning={{ placement: 'bottom' }} openDelay={220} closeDelay={60}>
      <Tooltip.Trigger>
        {#snippet element(tip: Record<string, unknown>)}
          {@render face(merge(attributes, tip))}
        {/snippet}
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content>{hint}</Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip>
  {:else}
    {@render face(attributes)}
  {/if}
{/snippet}

{#if actions.length}
  <Menu
    {open}
    onSelect={(e: { value: string }) => onaction?.(e.value)}
    onOpenChange={(e: { open: boolean }) => {
      if (priming) return;
      open = e.open;
    }}>
    <Menu.ContextTrigger>
      {#snippet element(attributes: Record<string, unknown>)}
        {@render tipped(attributes)}
      {/snippet}
    </Menu.ContextTrigger>
    <Portal>
      <Menu.Positioner>
        <Menu.Content>
          {#each actions as a (a.value)}
            <Menu.Item value={a.value}>
              <Menu.ItemText>
                <span class:danger={a.tone === 'danger'}>{a.label}</span>
              </Menu.ItemText>
            </Menu.Item>
          {/each}
        </Menu.Content>
      </Menu.Positioner>
    </Portal>
  </Menu>
{:else}
  {@render tipped()}
{/if}

<style>
  /* Sin tarjeta. La imagen es la pieza, y un marco alrededor de cada una
     convierte una galería de objetos en una hoja de cálculo con fotos: cuarenta
     bordes compiten con las cuarenta formas que se están mirando. */
  .tile {
    width: 100%;
    display: grid;
    gap: 0.5rem;
    padding: 0.4rem 0.2rem 0.6rem;
    border: none;
    border-radius: 14px;
    background: transparent;
    text-align: center;
  }
  .tile:hover, .tile.held { background: transparent; }
  .tile:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }

  .shot {
    display: grid;
    grid-template-areas: 'art';
    place-items: center;
    height: 104px;
  }
  .shot img {
    grid-area: art;
    max-height: 100px;
    max-width: 100px;
    object-fit: contain;
    /* La sombra la pone el dibujo; ésta es la que lo apoya en la página. */
    filter: drop-shadow(0 8px 12px rgb(11 18 32 / 0.2));
    transition: transform 0.18s cubic-bezier(0.2, 0.8, 0.3, 1), opacity 0.18s ease;
  }
  .lit { opacity: 0; }

  /* El pedestal: el volumen que la silueta no tiene. Mismo gradiente, mismo
     brillo y misma sombra que los renders, para que las dos clases de pieza se
     vean de la misma familia en la misma cuadrícula. */
  .pedestal {
    grid-area: art;
    width: 88px;
    height: 88px;
    display: grid;
    place-items: center;
    border-radius: 26px;
    background:
      radial-gradient(circle at 32% 24%, rgb(255 255 255 / 0.35), transparent 55%),
      linear-gradient(150deg, color-mix(in oklab, var(--text) 12%, var(--surface)), color-mix(in oklab, var(--text) 22%, var(--surface)));
    box-shadow:
      inset 0 1px 0 rgb(255 255 255 / 0.35),
      0 10px 16px rgb(11 18 32 / 0.22);
    transition: transform 0.18s cubic-bezier(0.2, 0.8, 0.3, 1), background 0.18s ease;
  }
  .brand {
    width: 46px;
    height: 46px;
    background: var(--muted);
    transition: background 0.18s ease;
    -webkit-mask: var(--mask) center / contain no-repeat;
    mask: var(--mask) center / contain no-repeat;
  }
  /* En reposo, gris; al pasar por encima, su color. Es el mismo paso barro →
     color de los renders, dicho con una marca. */
  .tile:hover .brand,
  .tile:focus-visible .brand,
  .tile.held .brand { background: var(--brand); }
  .tile:hover .pedestal,
  .tile:focus-visible .pedestal,
  .tile.held .pedestal { transform: scale(1.08) translateY(-2px); }
  @media (prefers-reduced-motion: reduce) {
    .tile:hover .pedestal, .tile.held .pedestal { transform: none; }
  }
  /* Crece y se pinta: es todo el estado que necesita una pieza sin marco. */
  .tile:hover .still,
  .tile:focus-visible .still,
  .tile.held .still { opacity: 0; transform: scale(1.14) translateY(-2px); }
  .tile:hover .lit,
  .tile:focus-visible .lit,
  .tile.held .lit { opacity: 1; transform: scale(1.14) translateY(-2px); }
  @media (prefers-reduced-motion: reduce) {
    .shot img { transition: opacity 0.18s ease; }
    .tile:hover .still, .tile:hover .lit,
    .tile.held .still, .tile.held .lit { transform: none; }
  }

  /* Centrados bajo la pieza. En los extremos, el nombre y la cuenta se leían
     como dos columnas de una tabla; centrados se leen como el pie de una foto,
     que es lo que son. */
  .label {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.1rem;
    padding: 0 0.2rem;
    text-align: center;
  }
  .label b { font-size: 0.9rem; }
  .label .faint { font-size: 0.76rem; }
  .alarm {
    padding: 0 0.2rem;
    color: var(--p1, tomato);
    font-size: 0.74rem;
    font-weight: 600;
    text-align: center;
  }
  .danger { color: var(--p1, tomato); }
</style>
