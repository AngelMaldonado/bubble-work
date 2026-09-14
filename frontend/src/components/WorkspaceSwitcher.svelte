<script lang="ts">
  // Alt+Tab, para workspaces.
  //
  // Shift es la tecla que se MANTIENE y Tab la que avanza, igual que Windows usa
  // Alt: Shift+Tab abre y da un paso, seguir tecleando Tab sigue avanzando,
  // soltar Shift confirma. Las flechas mueven sin confirmar, una letra salta al
  // siguiente workspace que empieza por ella, y Esc devuelve dónde estabas.
  //
  // Shift+Tab es TAMBIÉN como un usuario de teclado camina el foco hacia atrás, y
  // ese no es un atajo que valga la pena romper. Por eso sólo se toma cuando el
  // foco está en el board: dentro de un campo, un editor o un menú se lo queda el
  // navegador.
  import { api } from '../lib/api';
  import { isEditable } from '../lib/keys';
  import { allUrl, boardUrl } from '../lib/routes';

  let {
    workspaces,
    current,
    enabled = true,
    onpick,
  }: {
    workspaces: { id: string; slug: string; name: string }[];
    /** el slug en pantalla, o «» cuando lo que se mira es Todos */
    current: string;
    /** el board lo suprime mientras un modal, el omnibar o un menú son dueños
     *  del teclado: esos tienen su propia idea de qué significan Tab y Escape */
    enabled?: boolean;
    onpick: (url: string) => void;
  } = $props();

  type Entry = { slug: string; name: string; count: number | null };

  const RECENT_CAP = 12;
  const KEY = 'bw.recent-workspaces';

  // El anillo de recencia vive en el navegador porque es de ESTE navegador: en
  // cuál de tus proyectos estuviste hace un minuto no es un hecho del equipo.
  let recent = $state<string[]>(read());
  function read(): string[] {
    try {
      const raw = JSON.parse(localStorage.getItem(KEY) ?? '[]');
      return Array.isArray(raw) ? raw.filter((x) => typeof x === 'string') : [];
    } catch {
      return [];
    }
  }
  /** Anota la visita: mueve el destino al frente del anillo.
   *
   *  Colgado de `current` y no de este panel a propósito: se cambia de workspace
   *  desde la columna, desde ⌘K y desde aquí, y un anillo que sólo cuenta sus
   *  propios saltos deriva en silencio hasta señalar a donde ya no estás. */
  function touch(slug: string): void {
    if (recent[0] === slug) return;
    recent = [slug, ...recent.filter((s) => s !== slug)].slice(0, RECENT_CAP);
    try {
      localStorage.setItem(KEY, JSON.stringify(recent));
    } catch {
      // Un navegador que no deja guardar deja el anillo en memoria, que sirve
      // durante la sesión. No es motivo para no cambiar de workspace.
    }
  }

  $effect(() => touch(current));

  // Cuántas burbujas tiene cada uno. Se pregunta al ABRIR el panel y una sola
  // vez: estando dentro de un workspace, el board en pantalla sólo sabe de ese,
  // y una cifra que dice 0 para los demás miente peor que una casilla sin cifra.
  let counts = $state<Record<string, number> | null>(null);
  let asked = false;
  async function countBubbles(): Promise<void> {
    if (asked) return;
    asked = true;
    try {
      const b = await api.allBoard();
      const n: Record<string, number> = {};
      for (const x of b.bubbles) n[x.workspace] = (n[x.workspace] ?? 0) + 1;
      counts = n;
    } catch {
      counts = null; // sin cifras, con casillas: el panel sigue sirviendo
    }
  }

  // «Todos» está EN el anillo, y no aparte: es un destino real, y sin él no hay
  // forma de volver al board entero sin ir por el ratón.
  //
  // ORDENADO POR RECENCIA, el actual primero — la propiedad que hace de esto un
  // Alt+Tab y no una lista. Por orden alfabético, el vecino que nunca visitas
  // queda junto al que usas todo el día, así que un Shift+Tab rápido aterrizaba
  // en cualquier parte y había que leer el panel para volver. Ahora la segunda
  // casilla es siempre de donde vienes: un toque para salir, uno para volver.
  //
  // Lo nunca visitado en este navegador va detrás, conservando su propio orden —
  // un sitio estable donde buscar, en vez de uno arbitrario.
  const entries = $derived.by<Entry[]>(() => {
    const c = counts;
    const all: Entry = {
      slug: '',
      name: 'Todos',
      count: c ? Object.values(c).reduce((n, x) => n + x, 0) : null,
    };
    const live: Entry[] = [
      all,
      ...workspaces.map((w) => ({
        slug: w.slug,
        name: w.name,
        count: c ? (c[w.id] ?? 0) : null,
      })),
    ];
    const rank = new Map(recent.map((s, i) => [s, i]));
    const seen = rank.size;
    return live
      // Lo no visitado indexa en `seen + i`, que está pasado todo rango visitado,
      // así que cae detrás conservando su orden. Cada clave es distinta, así que
      // el orden es total y no puede rebarajarse entre dibujados.
      .map((e, i) => ({ e, key: rank.get(e.slug) ?? seen + i }))
      .sort((a, b) => a.key - b.key)
      .map((x) => x.e);
  });

  let open = $state(false);
  let idx = $state(0);
  /** qué se miraba cuando esto se abrió — lo que Escape devuelve */
  let before = $state('');

  function show(): void {
    before = current;
    countBubbles();
    // El anillo va con el actual primero, así que esto es 0 en el caso normal —
    // pero se deriva en vez de suponerse, porque un workspace borrado bajo los
    // pies puede dejar el foco en otro sitio, y dar un paso desde la casilla
    // equivocada te manda a un proyecto en el que nunca estuviste.
    idx = Math.max(
      0,
      entries.findIndex((e) => e.slug === current),
    );
    open = true;
  }

  function step(by: number): void {
    const n = entries.length;
    idx = (idx + by + n) % n;
  }

  const urlOf = (slug: string) => (slug ? boardUrl(slug) : allUrl);

  function commit(): void {
    open = false;
    const target = entries[idx];
    if (!target) return;
    touch(target.slug);
    if (target.slug !== current) onpick(urlOf(target.slug));
  }

  function cancel(): void {
    open = false;
    if (current !== before) onpick(urlOf(before));
  }

  // Salta a la SIGUIENTE entrada que empieza por esa letra — «siguiente» y no
  // «primera», para que repetir la tecla camine entre las que la comparten.
  function jump(letter: string): void {
    const n = entries.length;
    for (let i = 1; i <= n; i++) {
      const cand = entries[(idx + i) % n];
      if (cand.name.toLowerCase().startsWith(letter)) {
        idx = (idx + i) % n;
        return;
      }
    }
  }

  // Dos letras, siempre. Un nombre de workspace es texto libre y los de verdad
  // llegan a ser hostnames, que no caben en ninguna casilla cuadrada — y una
  // casilla que cambia de tamaño por workspace deja de leerse como una rejilla.
  // Las iniciales son decoración que ancla el ojo; el nombre debajo es lo que
  // identifica.
  const initials = (e: Entry) =>
    e.name
      .replace(/[^\p{L}\p{N}]/gu, '')
      .slice(0, 2)
      .toUpperCase();

  function onKeyDown(e: KeyboardEvent): void {
    if (!open) {
      if (!enabled || !e.shiftKey || e.key !== 'Tab') return;
      if (isEditable(e.target)) return; // tabular hacia atrás sigue siendo del navegador
      if (entries.length < 2) return; // no hay entre qué cambiar
      e.preventDefault();
      show();
      step(1);
      return;
    }

    switch (e.key) {
      // Shift se MANTIENE todo el rato, así que Tab no puede significar también
      // «hacia atrás» como hace Alt+Shift+Tab en Windows. Las flechas son la
      // vuelta.
      case 'Tab':
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        step(1);
        return;
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        step(-1);
        return;
      case 'Home':
        e.preventDefault();
        idx = 0;
        return;
      case 'End':
        e.preventDefault();
        idx = entries.length - 1;
        return;
      case 'Enter':
        e.preventDefault();
        commit();
        return;
      case 'Escape':
        e.preventDefault();
        cancel();
        return;
    }
    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();
      jump(e.key.toLowerCase());
    }
  }

  // Soltar la tecla que se mantenía confirma — el sentido entero de la metáfora.
  function onKeyUp(e: KeyboardEvent): void {
    if (open && e.key === 'Shift') commit();
  }

  // Una ventana que pierde el foco a media conmutación no puede dejar el panel
  // clavado sobre el board sin una tecla con la que quitarlo.
  function onBlur(): void {
    if (open) cancel();
  }
</script>

<!-- En fase de CAPTURA.

     En la de burbuja, cualquiera que esté en medio y llame a `stopPropagation`
     —un menú de Zag que quedó escuchando, una capa de diálogo que no se
     desmontó— apaga el atajo en toda la aplicación sin dejar rastro: deja de
     funcionar y no hay error que mirar. En captura llega antes que nadie, y
     quien de verdad es dueño del teclado ya está cubierto por `isEditable`. -->
<svelte:window onkeydowncapture={onKeyDown} onkeyup={onKeyUp} onblur={onBlur} />

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="scrim" onclick={cancel}>
    <div
      class="panel"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-label="Cambiar de workspace"
      onclick={(e) => e.stopPropagation()}>
      <div class="tiles">
        {#each entries as e, i (e.slug)}
          <button
            class="tile"
            class:on={i === idx}
            aria-current={i === idx}
            onmouseenter={() => (idx = i)}
            onclick={commit}>
            <span class="mark" class:allmark={e.slug === ''}>
              {e.slug === '' ? '∗' : initials(e)}
            </span>
            <span class="name">{e.name}</span>
            <span class="count">
              {#if e.count !== null}{e.count} {e.count === 1 ? 'burbuja' : 'burbujas'}{/if}
            </span>
          </button>
        {/each}
      </div>
      <p class="hint">Tab avanza · ← → mueven · suelta Shift para ir · Esc cancela</p>
    </div>
  </div>
{/if}

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 300;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    background: oklch(0.12 0.02 265 / 0.5);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .panel {
    max-width: min(92vw, 60rem);
    max-height: 80vh;
    overflow-y: auto;
    padding: 1.1rem 1.15rem 0.85rem;
    border-radius: 20px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 30px 80px var(--shadow-strong);
    font-family: var(--sans);
  }
  .tiles {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 0.5rem;
  }
  .tile {
    width: 9.5rem;
    display: grid;
    justify-items: center;
    gap: 0.3rem;
    padding: 0.85rem 0.5rem 0.7rem;
    border-radius: 14px;
    border: 1px solid transparent;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    cursor: pointer;
  }
  /* El anillo de selección ES la interfaz entera: tiene que leerse de un
     vistazo, con el rabillo del ojo, mientras se mantiene una tecla.
     Con `--hot` porque es el acento más vivo del sistema y el único que ya
     significa «esto es lo que está pasando»; v0 usaba un `--wip` que aquí no
     existe, y un color-mix sobre una variable que no existe es una declaración
     inválida: la casilla elegida no se distinguía de las demás. */
  .tile.on {
    background: color-mix(in oklab, var(--hot) 14%, transparent);
    border-color: color-mix(in oklab, var(--hot) 55%, var(--line));
  }
  .mark {
    display: grid;
    place-items: center;
    width: 2.6rem;
    height: 2.6rem;
    border-radius: 12px;
    background: color-mix(in oklab, var(--text) 8%, transparent);
    border: 1px solid var(--line);
    font-size: 0.9rem;
    font-weight: 800;
    letter-spacing: 0.03em;
    color: var(--muted);
  }
  .tile.on .mark {
    background: var(--hot);
    border-color: transparent;
    color: oklch(0.99 0 0);
  }
  .allmark {
    font-size: 1.1rem;
  }
  /* Dos líneas, partidas donde sea, con la altura reservada de todos modos para
     que las cifras de abajo queden en la misma línea base. */
  .name {
    width: 100%;
    min-height: 2.5em;
    font-size: 0.78rem;
    font-weight: 700;
    line-height: 1.25;
    text-align: center;
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .count {
    font-size: 0.68rem;
    color: var(--faint);
  }
  .hint {
    margin: 0.75rem 0 0;
    text-align: center;
    font-size: 0.7rem;
    color: var(--faint);
  }
</style>
