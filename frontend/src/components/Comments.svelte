<script lang="ts">
  // Lo que se dijo sobre un thread, en un cajón.
  //
  // Un cajón y no un panel al lado del documento: leer un documento y leer una
  // conversación son dos modos distintos, y un panel que le roba ancho al texto
  // empeora los dos. Es la misma forma que el cajón de la burbuja, que ya se
  // sabe abrir y cerrar.
  //
  // De la receta de chat de Skeleton se toma la mitad que sirve —la lista y el
  // compositor pegado abajo— y se tira la otra: su columna de contactos es
  // nuestro board, y sus burbujas a izquierda y derecha codifican "dos partes
  // hablando", que no es lo que pasa en un thread. Aquí todos están del mismo
  // lado; lo que importa es quién lo dijo y cuándo.
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import { api, type Comment } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { ago } from '../lib/when';
  import Prose from './Prose.svelte';

  let {
    open = $bindable(false),
    thread,
    title = '',
    onposted,
  }: {
    open?: boolean;
    /** el id del thread; vacío mientras no hay ninguno abierto */
    thread: string;
    title?: string;
    /** se dijo algo — el pulso del thread cambió */
    onposted?: () => void;
  } = $props();

  let items = $state<Comment[]>([]);
  let html = $state<Record<string, string>>({});
  let draft = $state('');
  let error = $state('');
  let sending = $state(false);
  let list = $state<HTMLElement | null>(null);

  // El mismo matiz que la presencia, derivado del id: dos pantallas que dibujan
  // a la misma persona la dibujan igual, sin guardar un color en ningún lado.
  const hue = (id: string) => {
    let h = 0;
    for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) % 360;
    return h;
  };

  async function load() {
    if (!thread) return;
    try {
      items = await api.comments(thread);
      error = '';
      // El cuerpo es markdown y lo renderiza el SERVIDOR, igual que el
      // documento: dos renderers coinciden hasta que dejan de hacerlo, y
      // entonces dos pantallas muestran lo mismo distinto.
      const missing = items.filter((c) => html[c.id] === undefined);
      if (missing.length) {
        const done = await Promise.all(missing.map((c) => api.renderMarkdown(c.body)));
        const next = { ...html };
        missing.forEach((c, i) => (next[c.id] = done[i]));
        html = next;
      }
      // Abajo del todo: lo último dicho es lo que viniste a leer.
      requestAnimationFrame(() => list && (list.scrollTop = list.scrollHeight));
    } catch (e) {
      error = (e as Error).message;
    }
  }

  $effect(() => {
    if (!open) return;
    thread;
    load();
    // Mientras el cajón está abierto, lo que otro diga aparece solo. El mensaje
    // trae el registro pero no el nombre de quien lo escribió, así que esto sólo
    // dice "algo se movió" y la lectura que sigue es una petición pequeña —
    // filtrada por el servidor, que no manda lo que este miembro no puede ver.
    //
    // Se escucha SÓLO con el cajón abierto: un stream por pantalla cerrada es
    // una conexión que el servidor mantiene para algo que nadie está mirando.
    return live.watch('comments', (data) => {
      const rec = (data as { record?: { thread?: string } } | null)?.record;
      // Otro thread está hablando; no es esta conversación.
      if (rec?.thread && rec.thread !== thread) return;
      load();
    });
  });

  async function send() {
    const body = draft.trim();
    if (!body || sending || !thread) return;
    sending = true;
    try {
      await api.comment(thread, body);
      draft = '';
      await load();
      onposted?.();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      sending = false;
    }
  }

</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Portal>
    <Dialog.Backdrop class="scrim" />
    <Dialog.Positioner class="drawer-pos">
      <Dialog.Content class="drawer talk">
        <header class="flex items-start gap-3">
          <span class="mt-0.5 text-xl" aria-hidden="true">💬</span>
          <Dialog.Title class="min-w-0 flex-1 truncate text-lg font-bold">
            {title || 'Comentarios'}
          </Dialog.Title>
          <Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface">✕</Dialog.CloseTrigger>
        </header>

        <!-- La frase que evita el malentendido más caro del modelo. Sin ella, lo
             primero que hace cualquiera es comentar para que algo "siga vivo". -->
        <p class="pulse">
          Comentar no calienta el thread — mantiene el pulso: dice que alguien sigue atento, y
          por eso no se le llama abandonado. Lo que lo calienta es escribir el trabajo.
          <br />
          Lo dicho no se edita ni se borra: si algo salió mal, se dice otra cosa debajo.
        </p>

        {#if error}
          <p class="err" role="alert">{error}</p>
        {/if}

        <ul class="stream" bind:this={list}>
          {#each items as c (c.id)}
            <li class="said">
              <span class="who" style="--hue: {hue(c.author)}" title={c.name}>
                {(c.name[0] ?? '?').toUpperCase()}
              </span>
              <div class="min-w-0">
                <p class="meta">
                  <b>{c.mine ? 'tú' : c.name}</b>
                  <span class="faint">{ago(c.created)}</span>
                </p>
                <Prose compact html={html[c.id] ?? ''} />
              </div>
            </li>
          {:else}
            <li class="faint text-sm">Nadie ha dicho nada todavía.</li>
          {/each}
        </ul>

        <!-- Pegado abajo, como el "+ thread" del cajón de la burbuja: escribir
             es la acción que este panel siempre ofrece. -->
        <form class="composer" onsubmit={(e) => { e.preventDefault(); send(); }}>
          <textarea
            rows="2"
            placeholder="Escribe… (⌘↵ para enviar)"
            bind:value={draft}
            onkeydown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                send();
              }
            }}></textarea>
          <button class="btn btn-sm preset-filled-primary-500" disabled={!draft.trim() || sending}>
            {sending ? '…' : 'Comentar'}
          </button>
        </form>
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  /* La columna: cabecera y compositor quietos, la conversación es lo único que
     se mueve. */
  :global(.talk) { display: flex; flex-direction: column; }

  .pulse {
    flex: none;
    margin: 0.6rem 0 0;
    padding: 0.5rem 0.7rem;
    border-radius: 10px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.78rem;
    line-height: 1.45;
  }
  .err {
    flex: none;
    margin: 0.5rem 0 0;
    padding: 0.5rem 0.7rem;
    border-radius: 9px;
    background: color-mix(in oklab, var(--p1, tomato) 14%, transparent);
    color: var(--text);
    font-size: 0.8rem;
  }

  .stream {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    margin: 0.8rem -1.25rem 0;
    padding: 0 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    list-style: none;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  .said { display: grid; grid-template-columns: auto 1fr; gap: 0.6rem; }
  .who {
    width: 28px;
    height: 28px;
    display: grid;
    place-content: center;
    border-radius: 999px;
    background: oklch(0.72 0.13 var(--hue));
    color: oklch(0.22 0.05 var(--hue));
    font-size: 0.76rem;
    font-weight: 700;
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    margin: 0 0 0.15rem;
    font-size: 0.78rem;
  }

  .composer {
    flex: none;
    margin-top: auto;
    padding-top: 0.7rem;
    border-top: 1px solid var(--line);
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
  }
  .composer textarea {
    flex: 1;
    min-width: 0;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
    resize: vertical;
  }
  .composer textarea:focus {
    outline: none;
    border-color: color-mix(in oklab, var(--accent) 60%, transparent);
  }
</style>
