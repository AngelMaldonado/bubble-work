<script lang="ts">
  // The app's frame: the column of workspaces on the left, and one scrolling
  // pane on the right that every screen is drawn into.
  //
  // Extracted from the mock rather than rewritten for it. The mock was where
  // this shape was worked out — the rail, the pinned "Nuevo", the list that
  // scrolls while the column does not — and a second copy of that CSS is how
  // the real screen and the drawing of it start disagreeing.
  //
  // The shell does NOT scroll; the pane does. The sidebar and the floating
  // buttons stay put because they are outside the scrolling box, not because
  // they are pinned on top of one.
  import { Navigation, Portal, Menu, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import MenuIcon from '@lucide/svelte/icons/menu';
  import MoreVerticalIcon from '@lucide/svelte/icons/more-vertical';
  import BrandOrb from './BrandOrb.svelte';
  import { rail } from '../lib/filters.svelte';
  import { narrow } from '../lib/viewport.svelte';
  import { edgeFade } from '../lib/fade.svelte';
  import type { Snippet } from 'svelte';
  import { limited } from '../lib/limits.svelte';

  export type ShellItem = {
    id: string;
    name: string;
    hint?: string;
    /** cuántas burbujas suyas están produciendo. Lo que hace que un proyecto
     *  flote en esta columna, y lo único que la tiñe. */
    hot?: number;
  };

  /** Something in the column that is NOT a workspace: a screen the whole app
   *  has, reached the same way you reach a project. */
  export type ShellPin = {
    id: string;
    name: string;
    face: string;
    href?: string;
    /** arriba del listado en vez de al pie. «Todos» es el board ENTERO y los
     *  proyectos de abajo son sus partes, así que se lee como el primero de
     *  ellos y no como un lugar aparte. */
    top?: boolean;
  };

  let {
    items = [],
    pins = [],
    pinned = '',
    current = '',
    label = 'Proyectos',
    version = '',
    latest = '',
    updateNote = '',
    onversion,
    newLabel = 'Nuevo',
    pane = $bindable<HTMLElement | null>(null),
    onselect,
    onpin,
    onsignout,
    onrename,
    ondelete,
    /** create one and say which it is, so it can be named straight away */
    oncreate,
    children,
    corner,
  }: {
    items?: ShellItem[];
    /** screens pinned above the list — the planner, and whatever joins it */
    pins?: ShellPin[];
    /** which pin is on screen, if any */
    pinned?: string;
    current?: string;
    label?: string;
    /** qué versión corre el servidor, o el entorno cuando no es una versión */
    version?: string;
    /** la versión más nueva, cuando hay una más nueva que la que corre */
    latest?: string;
    /** por qué pulsar no actualiza, cuando no puede */
    updateNote?: string;
    /** pulsar la etiqueta. Sólo se pasa cuando hay algo que hacer al pulsarla */
    onversion?: () => void;
    newLabel?: string;
    pane?: HTMLElement | null;
    onselect?: (id: string) => void;
    onpin?: (id: string) => void;
    /** salir de la sesión. Un verbo raro, al pie y separado de los de todos los
     *  días — pero VISIBLE: escondido en el omnibar, sólo lo encontraba quien
     *  ya sabía que estaba ahí. */
    onsignout?: () => void;
    onrename?: (id: string, name: string) => void;
    ondelete?: (id: string) => void;
    oncreate?: () => Promise<string | void> | string | void;
    children?: Snippet;
    /** lo que flota sobre el pane sin ir DENTRO de él: la esquina de abajo a la
     *  izquierda. Va aquí y no en la aplicación porque `position: fixed` mide
     *  contra la ventana, y la ventana incluye la columna de proyectos — un
     *  flotante fijo a la izquierda se dibuja encima del sidebar. */
    corner?: Snippet;
  } = $props();

  /** Los fijados se reparten: arriba del listado o al pie de la columna. */
  const head = $derived(pins.filter((p) => p.top));
  const foot = $derived(pins.filter((p) => !p.top));

  // Colapsada o no, como se dejó la última vez. La preferencia se lee al montar
  // en vez de arrancar siempre ancha: quien trabaja con la columna estrecha no
  // tendría por qué volver a colapsarla en cada recarga.
  let railed = $state(rail.get());
  function toggleRail() {
    railed = !railed;
    rail.set(railed);
  }

  /** En un teléfono la columna ancha y el contenido no caben a la vez, así que
   *  la columna se colapsa sola.
   *
   *  Y NO se guarda: la preferencia es de quien la tomó, y sobreescribirla
   *  porque alguien abrió la aplicación en el móvil le dejaría el escritorio
   *  colapsado mañana sin haber pedido nada. Al volver a una pantalla ancha,
   *  vuelve lo que eligió. */
  const shutByWidth = $derived(narrow.on || railed);
  let editing = $state<string | null>(null);
  let draft = $state('');

  // Both scrolling boxes fade at the edge that still has content behind it.
  const listFade = edgeFade();
  const paneFade = edgeFade();

  function startRename(id: string, name: string) {
    editing = id;
    draft = name;
  }
  function commitRename() {
    // An empty name is not a rename, it is a deletion nobody asked for.
    if (editing && draft.trim()) onrename?.(editing, draft.trim());
    editing = null;
  }
  async function create() {
    const id = await oncreate?.();
    if (typeof id === 'string' && id) startRename(id, '');
  }
</script>

<!-- Una fila fijada: mismo alto, mismo resaltado y misma anatomía que la de un
     proyecto, arriba del listado o al pie según a qué conteste. -->
{#snippet pinRow(p: ShellPin)}
  <div class="proj" class:on={pinned === p.id}>
    <!-- Un ancla nuestra, no un `Navigation.Trigger`: es un LUGAR y tiene
         dirección — se abre en otra pestaña, se copia, y si el manejador de la
         aplicación no corriera, el navegador navega igual. Lleva a mano los
         `data-part` de Skeleton para que la fila se vea exactamente como la de
         un proyecto, que es lo único que se estaba pidiendo prestado del
         componente. -->
    <a
      class="pin-row"
      data-scope="navigation"
      data-part="trigger"
      data-layout={shutByWidth ? 'rail' : 'sidebar'}
      href={p.href ?? '#'}
      title={p.name}
      onclick={(e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        // Sin manejador no se cancela: anular el enlace y no navegar en su lugar
        // es cómo se construye un control que no hace nada.
        if (!onpin) return;
        e.preventDefault();
        onpin(p.id);
      }}>
      <span class="pin" aria-hidden="true">{p.face}</span>
      <!-- `data-layout` también en el TEXTO, no sólo en la fila: Skeleton pone
           el tamaño de fuente en `[data-part='trigger-text'][data-layout=…]`, y
           sin el atributo la regla no aplicaba — el texto de una fila fijada
           salía al tamaño del documento y el de un proyecto al del componente,
           dos tamaños en la misma columna. -->
      <span data-scope="navigation" data-part="trigger-text" data-layout={shutByWidth ? 'rail' : 'sidebar'}>
        {p.name}
      </span>
    </a>
  </div>
{/snippet}

{#snippet versionBadge()}
  {#if latest}
    <Tooltip positioning={{ placement: shutByWidth ? 'right' : 'bottom' }} openDelay={120} closeDelay={60}>
      <!-- Pulsable SÓLO cuando esta instancia puede ponérsela. Si no, el
           tooltip dice por qué, y el clic no promete nada. -->
      <Tooltip.Trigger class={onversion ? 'ver new live' : 'ver new'} onclick={onversion}>
        <span class="dot" aria-hidden="true"></span>{version}
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content>
            {latest} disponible — {onversion ? 'pulsa para actualizar' : updateNote}
          </Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip>
  {:else}
    <span class="ver" title={version}>{version}</span>
  {/if}
{/snippet}

<div class="shell">
  <!-- The workspaces live here. A workspace is the boundary for a body of work
       and the thing you switch between all day, so it gets the persistent
       column; the views get the floating buttons at the bottom right. -->
  <Navigation layout={shutByWidth ? 'rail' : 'sidebar'} class="shell-nav">
    <Navigation.Content>
      <Navigation.Header>
        <!-- The mark reads alone, which is exactly what the rail needs; the
             wordmark is what gets dropped when the column narrows, not the
             logo. -->
        <div class="brand" class:railed={shutByWidth}>
          <Navigation.Trigger onclick={toggleRail} title={shutByWidth ? 'expandir' : 'colapsar'}>
            <BrandOrb size={shutByWidth ? 26 : 22} />
            {#if !shutByWidth}<span class="display text-base">bubble.work</span>{/if}
          </Navigation.Trigger>
          <!-- Qué versión es esto, pegado al nombre: es de qué producto se
               habla. Chico y apagado mientras no hay nada que decir; con color
               cuando hay una más nueva, y el tooltip dice cuál y qué hacer.
               También en el rail, debajo del orbe: `v1.4.3` cabe en esa
               anchura, y quien trabaja con la columna estrecha es quien más
               tiempo pasa mirándola. -->
          {#if version}{@render versionBadge()}{/if}
        </div>
      </Navigation.Header>

      <!-- The label sits OUTSIDE the scrolling list: a heading that scrolls
           away is a heading that stops labelling anything. -->
      {#if !shutByWidth}<Navigation.Label>{label}</Navigation.Label>{/if}

      <Navigation.Group class="projects" style={listFade.style} {@attach listFade.attach}>
        <Navigation.Menu>
          <!-- Los de arriba, antes que ningún proyecto. «Todos» es el board
               entero y lo de abajo son sus partes: leerlo como el primero de la
               lista dice esa relación; al pie decía que era otra cosa. -->
          {#each head as p (p.id)}
            {@render pinRow(p)}
          {/each}
          {#each items as p (p.id)}
            {#if editing === p.id && !shutByWidth}
              <!-- Renaming happens IN PLACE. A dialog for one field is a dialog
                   asking you to confirm you meant to type.
                   Focused explicitly, not with `autofocus`: the attribute only
                   acts on a page's first parse, so an input that appears later
                   never gets it — the typing went to the page instead. -->
              <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                class="rename"
                {@attach (el: HTMLInputElement) => { el.focus(); el.select(); }}
                bind:value={draft} {@attach limited('workspaces.name')}
                onblur={commitRename}
                onkeydown={(e) => {
                  if (e.key === 'Enter') commitRename();
                  if (e.key === 'Escape') editing = null;
                }} />
            {:else}
              <div class="proj" class:on={current === p.id} class:burning={!!p.hot}>
                <Navigation.Trigger
                  onclick={() => onselect?.(p.id)}
                  ondblclick={() => startRename(p.id, p.name)}
                  title={p.hot
                    ? `${p.name} · ${p.hot} ${p.hot === 1 ? 'burbuja produciendo' : 'burbujas produciendo'}`
                    : p.hint
                      ? `${p.name} · ${p.hint}`
                      : p.name}>
                  <span class="pin" aria-hidden="true">{p.name.slice(0, 1)}</span>
                  <Navigation.TriggerText>{p.name}</Navigation.TriggerText>
                </Navigation.Trigger>
                {#if !shutByWidth && (onrename || ondelete)}
                  <Menu onSelect={(e: { value: string }) => {
                    if (e.value === 'rename') startRename(p.id, p.name);
                    if (e.value === 'delete') ondelete?.(p.id);
                  }}>
                    <Menu.Trigger>
                      <span class="proj-more" title="acciones"><MoreVerticalIcon class="size-4" /></span>
                    </Menu.Trigger>
                    <Portal>
                      <Menu.Positioner>
                        <Menu.Content>
                          {#if onrename}<Menu.Item value="rename"><Menu.ItemText>Renombrar</Menu.ItemText></Menu.Item>{/if}
                          {#if ondelete}<Menu.Item value="delete"><Menu.ItemText>Eliminar</Menu.ItemText></Menu.Item>{/if}
                        </Menu.Content>
                      </Menu.Positioner>
                    </Portal>
                  </Menu>
                {/if}
              </div>
            {/if}
          {/each}
        </Navigation.Menu>
      </Navigation.Group>

      <!-- Pinned to the bottom in both layouts. Crear un proyecto, los lugares
           que no son un proyecto (planeador, inventario) y salir van juntos en
           UN menú: cuatro filas sueltas le robaban a la lista de proyectos el
           alto que es suyo, y ninguna de ellas se pulsa todos los días. -->
      {#if oncreate || foot.length || onsignout}
        <Navigation.Footer class="new-foot">
          <Navigation.Menu>
            <Menu
              positioning={{ placement: shutByWidth ? 'right-end' : 'top-start' }}
              onSelect={(e: { value: string }) => {
                if (e.value === 'create') create();
                else if (e.value === 'signout') onsignout?.();
                else onpin?.(e.value);
              }}>
              <div class="proj" class:on={foot.some((p) => p.id === pinned)}>
              <Menu.Trigger>
                {#snippet element(attributes: Record<string, unknown>)}
                  <!-- La fila se ve como las demás de la columna: lleva a mano
                       los `data-part` de Skeleton, como las fijadas. Marcada
                       cuando lo que hay en pantalla es uno de sus lugares, para
                       que la columna siga diciendo dónde estás. -->
                  <button
                    {...attributes}
                    class="pin-row menu-row"
                    data-scope="navigation"
                    data-part="trigger"
                    data-layout={shutByWidth ? 'rail' : 'sidebar'}
                    title="Menú">
                    <MenuIcon class={shutByWidth ? 'size-5' : 'size-4'} />
                    <span data-scope="navigation" data-part="trigger-text" data-layout={shutByWidth ? 'rail' : 'sidebar'}>
                      {foot.find((p) => p.id === pinned)?.name ?? 'Menú'}
                    </span>
                  </button>
                {/snippet}
              </Menu.Trigger>
              </div>
              <Portal>
                <Menu.Positioner>
                  <Menu.Content class="shell-menu">
                    {#if oncreate}
                      <Menu.Item value="create">
                        <PlusIcon class="size-4" />
                        <Menu.ItemText>{newLabel}</Menu.ItemText>
                      </Menu.Item>
                    {/if}
                    {#each foot as p (p.id)}
                      <Menu.Item value={p.id}>
                        <span class="face" aria-hidden="true">{p.face}</span>
                        <Menu.ItemText>{p.name}</Menu.ItemText>
                      </Menu.Item>
                    {/each}
                    {#if onsignout}
                      <!-- Al final y separado: es de la SESIÓN, no un lugar. -->
                      <Menu.Separator />
                      <Menu.Item value="signout">
                        <span class="face" aria-hidden="true">🚪</span>
                        <Menu.ItemText>Salir</Menu.ItemText>
                      </Menu.Item>
                    {/if}
                  </Menu.Content>
                </Menu.Positioner>
              </Portal>
            </Menu>
          </Navigation.Menu>
        </Navigation.Footer>
      {/if}
    </Navigation.Content>
  </Navigation>

  <div class="col">
    <main class="pane" bind:this={pane} style={paneFade.style} {@attach paneFade.attach}>
      {@render children?.()}
    </main>
    {@render corner?.()}
  </div>
</div>

<style>
  /* Nav column, then everything else. `align-items: stretch` is what lets
     Navigation's own `height: 100%` mean something. */
  .shell {
    /* `dvh` y no `vh`: en un móvil la barra del navegador aparece y desaparece,
       y `100vh` cuenta la pantalla como si nunca estuviera — el pie de la
       aplicación queda debajo de ella y no se alcanza. */
    height: 100dvh;
    overflow: hidden;
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: stretch;
  }
  .shell :global(.shell-nav) {
    border-right: 1px solid var(--line);
    background: transparent;
  }
  /* Header, projects and footer hang off `content`, not off the root, so the
     root's rows never reached them — the button pinned in the rail (a flex
     column) and floated mid-way in the sidebar. Make `content` the full-height
     column instead. Only in the sidebar layout: the rail's content is
     `display: contents`, and giving that a height would undo the rail. */
  /* A flex chain rather than a percentage: `height: 100%` resolves against the
     root's own height, which comes from `align-items: stretch` and is not
     definite, so the percentage falls back to auto and the column grows to its
     contents — taking the pinned footer past the bottom edge with it. */
  .shell :global(.shell-nav) { display: flex; flex-direction: column; overflow: hidden; }
  .shell :global(.shell-nav [data-part='content'][data-layout='sidebar']) {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
    /* Skeleton aligns this column to `start`, which makes every child as wide as
       its text. The rows need the column. */
    align-items: stretch;
    gap: 0.75rem;
  }
  /* The rail's group is `display: contents`, which has no box and therefore
     cannot scroll. It needs to be one. */
  .shell :global(.shell-nav[data-layout='rail'] .projects) {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  /* The LIST scrolls, not the column. With the overflow on the root, a long
     list pushed the footer past the bottom edge and the button went with it —
     the point of pinning it was that it never moves. */
  .shell :global(.shell-nav .projects) {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    /* The scrolling box spans the whole column, so its bar rides the sidebar's
       own edge instead of floating in the middle of the padding. The padding
       comes back inside, which leaves the rows exactly where they were. */
    /* `width: auto` matters: Skeleton sets `width: 100%` on the group, and a
       fixed width plus negative margins SHIFTS the box instead of widening it —
       the bar ended up further from the edge, not nearer. */
    width: auto;
    margin-inline: -1rem;
    /* More on the right than the left: the scrollbar lives there. */
    padding-inline: 1rem 1.5rem;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  /* Collapsed, the thumb is a second vertical line in a column 100px wide, next
     to the one that already separates the rail from the board. It still
     scrolls; it just stops drawing. */
  .shell :global(.shell-nav[data-layout='rail'] .projects) { scrollbar-width: none; }
  .shell :global(.shell-nav[data-layout='rail'] .projects::-webkit-scrollbar) { display: none; }

  .menu-row { border: none; background: transparent; text-align: left; cursor: pointer; }
  /* El menú está portado a <body>: se estiliza por su clase. */
  :global(.shell-menu) { min-width: 13rem; }
  :global(.shell-menu [data-part='item']) { display: flex; align-items: center; gap: 0.55rem; justify-content: flex-start; }
  :global(.shell-menu .face) { width: 1rem; text-align: center; }

  /* `margin-top: auto` is what pins it in both layouts. */
  .shell :global(.shell-nav [data-part='footer']) {
    margin-top: auto;
    /* `align-items: start` on the column would leave the rule as wide as the
       word, and a divider that stops mid-column reads as a mistake. */
    align-self: stretch;
    width: 100%;
    padding-top: 0.4rem;
    border-top: 1px solid var(--line);
  }
  /* `position: relative` para que la esquina mida contra el pane y no contra la
     ventana. La columna no scrollea —lo hace `.pane`— así que lo que se ancla
     aquí se queda quieto. */
  .col { position: relative; display: flex; flex-direction: column; min-width: 0; min-height: 0; }

  /* A row. Every selector here is scoped to `[data-scope='navigation']` on
     purpose: the ⋮ is a `Menu.Trigger`, which carries `data-part='trigger'`
     too, so an unscoped rule painted the menu button with the row's highlight
     and gave it a block of its own. Same part name, different component. */
  /* Los fijados usan la misma fila que un proyecto — misma altura, mismo
     resaltado — y se separan del listado con una línea, no con espacio: son
     otra clase de cosa, y el espacio solo dice "hay un hueco". */

  .proj {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.15rem;
    width: 100%;
  }
  /* Skeleton's sidebar trigger is `width: 100%`, which in a flex row means the
     WHOLE row: the name ran under the ⋮ and the highlight spilled past both.
     `flex: 1` gives it everything that is actually left over, which is what the
     row is for — the name is the content, the ⋮ is furniture. */
  /* Only in the sidebar layout: the rail's trigger is a square with its own
     width, and taking that away collapses it. */
  :global(.shell-nav[data-layout='sidebar']) .proj :global([data-scope='navigation'][data-part='trigger']) {
    flex: 1 1 auto;
    width: auto;
    min-width: 0;
  }
  /* The name is one line that ends in an ellipsis, in BOTH layouts. In the rail
     it was running out of the square and across the divider — a name too long
     for the column is a fact to state, not a layout to break. */
  .proj :global([data-part='trigger-text']) {
    display: block;
    max-width: 100%;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* La elegida lleva borde. El fondo dice «ésta es distinta» y el borde dice
     dónde EMPIEZA y dónde acaba — que en una columna de filas sin separación es
     la diferencia entre una fila resaltada y una mancha. Va por dentro
     (`inset`), así que no mueve nada de sitio. */
  .proj.on :global([data-scope='navigation'][data-part='trigger']) {
    background: var(--hover);
    box-shadow: inset 0 0 0 1px var(--line);
    font-weight: 600;
  }
  /* El ancla es la fila: sin subrayado y con el color de la columna, porque el
     estilo de la aplicación manda sobre el del navegador. */
  .pin-row,
  .pin-row:visited { text-decoration: none; color: inherit; }

  .proj-more {
    flex: none;
    display: grid;
    place-content: center;
    width: 24px;
    height: 28px;
    border-radius: 7px;
    color: var(--faint);
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.12s ease;
  }
  /* Shown when the row is being used — hovered, focused, or the current one.
     Three permanent ⋮ down the column is three invitations to a menu nobody
     opens; the affordance belongs to the row you are actually on. */
  .proj:hover .proj-more,
  .proj:focus-within .proj-more,
  .proj.on .proj-more {
    opacity: 1;
  }
  .proj-more:hover {
    background: var(--hover);
    color: var(--text);
  }
  .pin {
    flex: none; display: grid; place-content: center;
    width: 22px; height: 22px; border-radius: 7px;
    background: var(--hover); color: var(--muted);
    font-size: 0.72rem; font-weight: 700;
  }
  .rename {
    width: 100%; padding: 0.35rem 0.5rem;
    border: 1px solid var(--accent, var(--line)); border-radius: 8px;
    background: var(--surface); color: var(--text); font-size: 0.88rem;
  }

  .pane { min-width: 0; flex: 1; overflow-y: auto; }

  /* El nombre y la versión, en la misma fila. El disparador no se estira: si
     ocupara toda la fila, la versión quedaría en el borde y no junto al nombre. */
  .brand { display: flex; align-items: center; gap: 0.4rem; min-width: 0; }
  .brand.railed { flex-direction: column; gap: 0.2rem; }
  .brand > :global(button:first-child) { flex: 0 1 auto; width: auto; min-width: 0; }

  .shell :global(.ver) {
    flex: none;
    max-width: 9rem;
    overflow: hidden;
    padding: 0.05rem 0.4rem;
    border: none;
    border-radius: 999px;
    background: var(--hover);
    color: var(--faint);
    font-size: 0.66rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    line-height: 1.5;
    text-overflow: ellipsis;
    white-space: nowrap;
    user-select: text;
  }

  /* Con algo nuevo esperando: el mismo sitio, el mismo tamaño, otro color. No
     un banner — nadie necesita que le interrumpan para decirle que hay una
     versión nueva; necesita poder verlo cuando mire. */
  .shell :global(.ver.new) {
    background: color-mix(in oklab, var(--accent, var(--text)) 18%, transparent);
    color: var(--accent, var(--text));
    cursor: default;
  }
  .shell :global(.ver.new.live) { cursor: pointer; }
  .shell :global(.ver.new.live:hover) {
    background: color-mix(in oklab, var(--accent, var(--text)) 30%, transparent);
  }
  .dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    margin-right: 0.3rem;
    vertical-align: middle;
    border-radius: 999px;
    background: currentColor;
  }

  /* Lo que arde, en la columna.

     El MISMO color que la banda caliente del board —`--hot`— y no un rojo
     nuevo: quien ya aprendió que ese naranja significa «está produciendo» no
     tiene que aprenderlo otra vez aquí.

     TRANSLÚCIDO sobre la columna, y ahí estuvo el error que costó tres vueltas.
     Mezclarlo contra `--surface-solid` —que es casi blanco en claro— convertía
     la fila en una TARJETA BLANCA sobre el degradado lila de la columna: parecía
     elevada, no caliente, y el hover la dejaba del todo en blanco. Un baño
     translúcido deja ver la columna debajo, así que lo que cambia es la
     temperatura y no el material.

     Y la selección se dice con MÁS del mismo baño, no con el gris de siempre:
     dos señales en el mismo canal se estropean, y aquí el canal es el color de
     la fila. Una fila caliente y elegida es la más cargada de todas.

     Sin número. La columna contesta «¿dónde está pasando algo?», y cuántas
     burbujas arden es una pregunta del board, que está a un clic. */
  .proj.burning :global([data-scope='navigation'][data-part='trigger']) {
    background: color-mix(in oklab, var(--hot) 12%, transparent);
  }
  /* El hover sube el mismo baño. El de Skeleton, en claro, es casi blanco, y
     borraba la fila justo al señalarla. */
  .proj.burning :global([data-scope='navigation'][data-part='trigger']:hover) {
    background: color-mix(in oklab, var(--hot) 18%, transparent);
  }
  .proj.burning.on :global([data-scope='navigation'][data-part='trigger']) {
    background: color-mix(in oklab, var(--hot) 26%, transparent);
    /* El borde, del mismo naranja: un gris sobre el baño cálido se lee como
       suciedad y no como un borde. */
    box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--hot) 50%, transparent);
  }
  .proj.burning.on :global([data-scope='navigation'][data-part='trigger']:hover) {
    background: color-mix(in oklab, var(--hot) 32%, transparent);
  }
  .proj.burning :global([data-scope='navigation'][data-part='trigger'] .pin) {
    color: var(--hot);
    background: color-mix(in oklab, var(--hot) 14%, transparent);
  }
</style>
