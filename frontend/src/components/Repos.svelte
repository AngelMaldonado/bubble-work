<script lang="ts">
  // La caja del código, en la esquina de abajo a la izquierda.
  //
  // Un workspace tiene un árbol de markdown —el que este servidor escribe— y
  // tiene, aparte, el sitio donde vive el código. Lo segundo no estaba en
  // ninguna parte: se preguntaba en el chat cada vez que alguien nuevo entraba.
  //
  // Enfrente de la HUD a propósito. Los botones de la derecha son VERBOS —
  // buscar, invitar, conectar un agente— y esto no es un verbo: es un lugar. Un
  // sitio al que se va tiene su propia esquina, y la que quedaba libre es ésta.
  //
  // Cerrada es una caja; al pasar por encima se abre y saca una marca por
  // repositorio. El mismo barro → color de las piezas del inventario, porque es
  // el mismo gesto: en reposo no compite con la pantalla, y bajo el cursor dice
  // que está viva.
  import { api, type Repo } from '../lib/api';
  import Confirm, { type Doom } from './Confirm.svelte';

  let {
    workspace,
    canWrite = false,
  }: {
    workspace: { id: string; name: string };
    /** el lead del workspace (o el global) es quien enlaza y quita */
    canWrite?: boolean;
  } = $props();

  let repos = $state<Repo[]>([]);
  let open = $state(false);
  let adding = $state(false);
  let draft = $state('');
  let error = $state('');
  let doom = $state<Doom>(null);

  $effect(() => {
    const id = workspace.id;
    api
      .repos(id)
      .then((rows) => (repos = rows))
      .catch(() => (repos = []));
  });

  /** Cómo se llama un repositorio en voz alta: `owner/repo`. El nombre guardado
   *  gana, pero casi nunca hace falta escribirlo — la URL ya lo dice. */
  function label(r: Repo) {
    if (r.name) return r.name;
    try {
      const path = new URL(r.url).pathname.replace(/^\/+|\/+$|\.git$/g, '');
      return path || r.url;
    } catch {
      return r.url;
    }
  }

  async function add() {
    const url = draft.trim();
    if (!url) return;
    error = '';
    try {
      // Sin esquema no es una URL para el servidor, y `github.com/a/b` es como
      // la gente la copia. Se completa aquí en vez de rechazarla.
      const full = /^https?:\/\//.test(url) ? url : `https://${url}`;
      const row = await api.addRepo(workspace.id, full);
      repos = [...repos, row];
      draft = '';
      adding = false;
    } catch (e) {
      error = (e as Error).message || 'no se pudo enlazar';
    }
  }

  function drop(r: Repo) {
    doom = {
      title: `¿Quitar «${label(r)}»?`,
      body: 'Deja de aparecer en esta caja. El repositorio no se toca.',
      verb: 'Quitar',
      go: async () => {
        await api.removeRepo(r.id);
        repos = repos.filter((x) => x.id !== r.id);
      },
    };
  }
</script>

<!-- Una caja que se abre sobre nada es ruido. Si no hay repositorios y quien
     mira no puede enlazar ninguno, no hay caja. -->
{#if repos.length || canWrite}
  <div
    class="repos"
    class:open
    role="group"
    aria-label="Repositorios de {workspace.name}"
    onmouseenter={() => (open = true)}
    onmouseleave={() => {
      if (!adding) open = false;
    }}
    onfocusin={() => (open = true)}
    onfocusout={(e) => {
      // El foco sale del grupo entero, no de un botón a otro dentro de él.
      if (!adding && !e.currentTarget.contains(e.relatedTarget as Node)) open = false;
    }}>
    <div class="fan">
      {#each repos as r (r.id)}
        <div class="row">
          <a class="mark" href={r.url} target="_blank" rel="noopener noreferrer">
            <img src="/marks/github.png" alt="" />
            <span class="tag">{label(r)}</span>
          </a>
          {#if canWrite}
            <button class="drop" title="quitar" onclick={() => drop(r)} aria-label="Quitar">✕</button>
          {/if}
        </div>
      {/each}

      {#if canWrite}
        {#if adding}
          <form
            class="row"
            onsubmit={(e) => {
              e.preventDefault();
              add();
            }}>
            <!-- svelte-ignore a11y_autofocus -->
            <input
              class="input url"
              autofocus
              bind:value={draft}
              placeholder="github.com/owner/repo"
              onkeydown={(e) => {
                if (e.key === 'Escape') {
                  adding = false;
                  draft = '';
                  error = '';
                }
              }} />
          </form>
        {:else}
          <button class="row link" onclick={() => (adding = true)}>
            <span class="plus">+</span>
            <span class="tag">Enlazar un repositorio</span>
          </button>
        {/if}
        {#if error}<p class="oops">{error}</p>{/if}
      {/if}
    </div>

    <!-- El cubo. Literalmente una caja: lo que hay dentro son enlaces, y una
         carpeta habría prometido archivos que aquí no existen. -->
    <button
      class="box"
      aria-expanded={open}
      onclick={() => (open = !open)}
      title="Repositorios de {workspace.name}">
      <img class="still" src="/3dicons/clay/cube.png" alt="" />
      <img class="lit" src="/3dicons/color/cube.png" alt="" aria-hidden="true" />
      {#if repos.length}<span class="count">{repos.length}</span>{/if}
    </button>
  </div>
{/if}

<Confirm bind:ask={doom} />

<style>
  /* Contra el PANE, no contra la ventana: `fixed` mide desde el borde de la
     pantalla y ahí a la izquierda está la columna de proyectos. El contenedor
     lo pone el shell, que sabe dónde empieza el pane. */
  .repos {
    position: absolute;
    left: 1rem;
    bottom: 1rem;
    z-index: var(--z-chrome);
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
  }

  /* Sale hacia ARRIBA y en columna: abajo está el borde de la ventana, y a la
     derecha está la pantalla que se está mirando.

     FUERA del flujo: cerrado, el abanico seguía ocupando su alto en la columna
     —invisible pero presente— y el área que abría la caja era esa columna
     entera, media pantalla de alto. Anclado sobre la caja, lo que se puede
     señalar es la caja.

     El hueco hasta la caja es PADDING, no margen. Con margen, entre la caja y
     la primera fila quedaba una banda muerta donde no se está sobre ninguna de
     las dos: el cursor la cruzaba, el hover se apagaba y el abanico se cerraba
     justo cuando ibas a pulsar. El padding pertenece al abanico, así que esa
     banda ya es parte de él. El de la derecha hace lo mismo para el movimiento
     en diagonal, que es como se mueve una mano de verdad. */
  .fan {
    position: absolute;
    left: 0;
    bottom: 100%;
    padding: 0.35rem 3rem 0.5rem 0;
    display: flex;
    flex-direction: column-reverse;
    align-items: flex-start;
    gap: 0.35rem;
    opacity: 0;
    pointer-events: none;
    transform: translateY(6px);
    transition: opacity 0.16s ease, transform 0.16s cubic-bezier(0.2, 0.8, 0.3, 1);
  }
  .repos.open .fan {
    opacity: 1;
    pointer-events: auto;
    transform: none;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .mark,
  .link {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.28rem 0.6rem 0.28rem 0.3rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    color: inherit;
    text-decoration: none;
    cursor: pointer;
    transition: transform 0.15s ease, background 0.15s ease;
  }
  .mark:hover,
  .link:hover {
    background: var(--hover);
    transform: translateX(2px);
  }
  .mark img {
    width: 26px;
    height: 26px;
    object-fit: contain;
  }
  .plus {
    width: 26px;
    display: grid;
    place-content: center;
    font-size: 1.05rem;
    line-height: 1;
    color: var(--muted);
  }
  .tag {
    font-size: 0.8rem;
    max-width: 15rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .drop {
    width: 22px;
    height: 22px;
    display: grid;
    place-content: center;
    border: none;
    border-radius: 999px;
    background: transparent;
    color: var(--muted);
    font-size: 0.7rem;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.15s ease, color 0.15s ease;
  }
  /* Quitar aparece cuando ya se está mirando esa fila. Visible siempre, es un
     aspa por repositorio compitiendo con el enlace que sí se va a pulsar. */
  .row:hover .drop,
  .drop:focus-visible {
    opacity: 1;
  }
  .drop:hover { color: var(--p1, tomato); }

  .url {
    width: 15rem;
    font-size: 0.8rem;
  }
  .oops {
    max-width: 16rem;
    color: var(--p1, tomato);
    font-size: 0.75rem;
  }

  /* Y la caja lleva su propio margen de puntería: 56px de dibujo dentro de un
     blanco algo mayor, para que acercarse no exija precisión de pixel. */
  .box {
    position: relative;
    margin: -0.4rem;
    padding: 0.4rem;
    display: grid;
    grid-template-areas: 'art';
    place-items: center;
    width: 56px;
    height: 56px;
    box-sizing: content-box;
    border: none;
    background: transparent;
    cursor: pointer;
  }
  .box img {
    grid-area: art;
    width: 52px;
    height: 52px;
    object-fit: contain;
    filter: drop-shadow(0 8px 12px rgb(11 18 32 / 0.22));
    transition: transform 0.18s cubic-bezier(0.2, 0.8, 0.3, 1), opacity 0.18s ease;
  }
  .lit { opacity: 0; }
  .repos.open .still { opacity: 0; transform: scale(1.12) translateY(-2px); }
  .repos.open .lit { opacity: 1; transform: scale(1.12) translateY(-2px); }
  .box:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: 14px; }

  /* Cuántos hay, sin abrir la caja. */
  .count {
    position: absolute;
    top: -2px;
    right: -2px;
    min-width: 18px;
    height: 18px;
    padding: 0 4px;
    display: grid;
    place-content: center;
    border-radius: 999px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    font-size: 0.66rem;
    font-weight: 600;
  }

  @media (prefers-reduced-motion: reduce) {
    .fan, .box img, .mark, .link { transition: opacity 0.16s ease; }
    .repos.open .still, .repos.open .lit { transform: none; }
    .mark:hover, .link:hover { transform: none; }
  }
</style>
