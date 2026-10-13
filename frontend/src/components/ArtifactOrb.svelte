<script lang="ts">
  import Icon from './Icon.svelte';
  // Lo que se está pensando ahora, en la esquina de abajo a la derecha.
  //
  // Encima del botón del tema, que tiene la esquina en todas las pantallas, y
  // en TODAS las pantallas: un artefacto es del departamento, no de un
  // workspace, así que no hay pantalla en la que no venga a cuento.
  //
  // Cerrado es una burbuja de chat con cuántos hay abiertos; un anillo verde
  // dice que tu agente está conectado (llamó al MCP hace poco). Al pasar por
  // encima se abre hacia arriba: el estado del agente y tus artefactos, con su
  // progreso. Clic en uno abre su pantalla, que es donde se contesta.
  //
  // Se entera solo: el stream de PocketBase avisa de cada entrada nueva. El
  // estado del agente no viaja por el stream (el sello del token se escribe
  // directo, sin eventos), así que ése se vuelve a pedir cada 30 s.
  import { onDestroy, tick as settled } from 'svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
  import Loading from './Loading.svelte';
  import { api, type AgentStatus, type Artifact } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { ago, when } from '../lib/when';

  let {
    onopen,
    onmore,
    ondeleted,
    docked = false,
  }: {
    onopen: (id: string) => void;
    onmore: () => void;
    ondeleted?: (id: string) => void;
    /** junto a la caja de repositorios, en la esquina del pane, en vez de flotar */
    docked?: boolean;
  } = $props();

  // Un cerebro: lo que guarda es lo que se está pensando. Si el dibujo todavía
  // no está en `public/brain/`, la burbuja de chat ocupa su sitio en vez de una
  // imagen rota.
  let still = $state('/brain/clay.png');
  let lit = $state('/brain/tint.png');

  let artifacts = $state<Artifact[]>([]);
  let agent = $state<AgentStatus>({ live: false, last_used_at: '' });
  let open = $state(false);
  let doom = $state<Doom>(null);
  let deleting = $state('');
  let failed = $state('');
  let brain: HTMLButtonElement | undefined;
  let readGeneration = 0;
  let stopped = false;

  const openOnes = $derived(artifacts.filter((a) => a.status === 'open'));
  const waiting = $derived(openOnes.reduce((n, a) => n + a.waiting, 0));

  async function load() {
    if (stopped) return;
    const owner=api.me?.id, turn=++readGeneration;
    try {
      const out = await api.artifacts();
      if (stopped || owner!==api.me?.id || turn!==readGeneration) return;
      artifacts = out.artifacts;
      agent = out.agent;
    } catch {
      // Sin sesión o sin red: la caja se queda como estaba en vez de vaciarse.
    }
  }

  function mayDelete(a: Artifact) {
    return a.creator===api.me?.id || api.me?.role==='lead';
  }

  function remove(a: Artifact) {
    if (deleting || !mayDelete(a)) return;
    const owner=api.me?.id;
    open=false;
    doom={
      title: `¿Eliminar «${a.title}»?`,
      crumbs: ['Artefactos efímeros'],
      body: 'Se eliminará el artefacto y todas sus publicaciones o preguntas y respuestas, sin recuperación. La nota del Inbox y los hilos y documentos duraderos se conservan. También se descartan las respuestas aún no enviadas de este artefacto.',
      go: async () => {
        if (stopped || owner!==api.me?.id) return;
        deleting=a.id; failed=''; ++readGeneration;
        try {
          await api.deleteArtifact(a.id);
          if (stopped || owner!==api.me?.id) return;
          ++readGeneration;
          artifacts=artifacts.filter((x)=>x.id!==a.id);
          ondeleted?.(a.id);
          await settled();
          if (!stopped) { brain?.focus(); void load(); }
        } catch (e) {
          if (!stopped && owner===api.me?.id) { failed=(e as Error).message || 'No se pudo eliminar'; open=true; }
        } finally {
          if (!stopped && owner===api.me?.id) deleting='';
        }
      },
    };
  }

  // Varias entradas llegan juntas (una ronda entera); una sola relectura basta.
  let soon: ReturnType<typeof setTimeout> | undefined;
  function nudge() {
    clearTimeout(soon);
    soon = setTimeout(load, 250);
  }

  load();
  const tick = setInterval(load, 30_000);
  const off1 = live.watch('artifacts', nudge);
  const off2 = live.watch('artifact_entries', nudge);
  onDestroy(() => {
    stopped=true; ++readGeneration;
    clearInterval(tick);
    clearTimeout(soon);
    off1();
    off2();
  });

  function left(a: Artifact) {
    if (a.pinned) return 'fijado';
    if (!a.expires_at) return '';
    const days = Math.ceil((when(a.expires_at) - Date.now()) / 86_400_000);
    return days <= 0 ? 'caduca hoy' : `${days} d`;
  }
  function soonGone(a: Artifact) {
    if (a.pinned || !a.expires_at) return false;
    return when(a.expires_at) - Date.now() < 3 * 86_400_000;
  }
</script>

<div
  class="orb"
  class:docked
  class:open
  role="group"
  aria-label="Artefactos"
  onmouseenter={() => (open = true)}
  onmouseleave={() => (open = false)}
  onfocusin={() => (open = true)}
  onfocusout={(e) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node)) open = false;
  }}>
  <div class="fan">
    <div class="agent" class:on={agent.live}>
      <span class="dot" aria-hidden="true"></span>
      {#if agent.live}
        MCP en vivo · {ago(agent.last_used_at)}
      {:else if agent.last_used_at}
        Tu agente no está conectado · {ago(agent.last_used_at)}
      {:else}
        Tu agente nunca se ha conectado
      {/if}
    </div>

    {#if failed}<p class="empty delete-error" role="alert">{failed}</p>{/if}
    {#each artifacts.slice(0, 8) as a (a.id)}
      <div class="row" class:done={a.status !== 'open'}>
      <button class="row-open" title={a.title} aria-label={a.title} aria-describedby={`artifact-status-${a.id}`} disabled={deleting===a.id} onclick={() => { open = false; onopen(a.id); }}>
        <span class="face">{#if a.live && !a.watched}<span class="live-dot"></span>{:else}<Icon of={a.watched ? 'eyes' : a.status === 'open' ? 'fire' : 'check'} tint />{/if}</span>
        <span class="title">{a.title}</span>
        <span class="meta" id={`artifact-status-${a.id}`}>
          {#if a.watched}<b class="ok">tu agente espera</b> ·{/if}
          {#if a.status === 'open' && a.waiting}<b>{a.waiting} por contestar</b> ·{/if}
          {#if a.kind === 'markdown'}<span class="filename" title={a.document?.filename}>{a.document?.filename ?? 'documento .md'}</span>{:else}{a.answered}/{a.total}{/if} ·
          {#if a.pinned}<Icon of="pin" tint />{:else}<span class:warn={soonGone(a)}>{left(a)}</span>{/if}
        </span>
      </button>
      {#if mayDelete(a)}<button class="row-remove" aria-label={`Eliminar «${a.title}»`} title="Eliminar artefacto" disabled={!!deleting} onclick={() => remove(a)}>
        {#if deleting===a.id}<Loading compact label="Eliminando artefacto" />{:else}<Icon of="trash" danger />{/if}
      </button>{/if}
      </div>
    {:else}
      <p class="empty">
        Nada pensándose ahora. Pídele a tu agente: <i>«grill me on …»</i>
      </p>
    {/each}
    {#if artifacts.length > 8}
      <button class="more" onclick={() => { open = false; onmore(); }} aria-label={`Ver ${artifacts.length - 8} artefactos más en el Inbox de sólo lectura`}>+{artifacts.length - 8}</button>
    {/if}
  </div>

  <button
    bind:this={brain}
    class="box"
    class:live={agent.live}
    class:has={artifacts.length > 0}
    aria-expanded={open}
    onclick={() => { open = false; onmore(); }}
    aria-label="Abrir Inbox"
    title="Inbox · artefactos{agent.live ? ' · agente conectado' : ''}">
    <img class="still" src={still} alt="" onerror={() => (still = '/3dicons/clay/chat-bubble.png')} />
    <img class="lit" src={lit} alt="" aria-hidden="true"
      onerror={() => (lit = '/i3d/chat-tint.png')} />
    <!-- Cuántos ARTEFACTOS abiertos. Encendido si alguno tiene preguntas
         esperándote; cuántas, lo dice cada fila. -->
    {#if openOnes.length}
      <span class="count" class:hot={waiting > 0}
        title="{openOnes.length} abierto{openOnes.length === 1 ? '' : 's'}{waiting ? ` · ${waiting} preguntas por contestar` : ''}">
        {openOnes.length}
      </span>
    {/if}
  </button>
</div>

<Confirm bind:ask={doom} />

<style>
  .more { width: 100%; padding: 0.65rem; border: 1px solid var(--border); border-radius: 10px; color: var(--accent); background: var(--surface); font-weight: 700; }
  .more:hover { background: var(--surface-hover); }
  /* Encima del botón del tema (42px en bottom: 1rem), en la misma columna. */
  .orb {
    position: fixed;
    right: calc(1rem - 7px);
    bottom: 4.1rem;
    z-index: var(--z-chrome);
    display: flex;
    flex-direction: column;
    align-items: flex-end;
  }

  /* Al lado del cubo: el contenedor de la esquina lo coloca, y el abanico se
     abre hacia arriba pegado a la IZQUIERDA, hacia la pantalla. */
  .orb.docked {
    position: relative;
    right: auto;
    bottom: auto;
    align-items: flex-start;
  }
  .orb.docked .fan {
    right: auto;
    left: 0;
    padding: 0.35rem 3rem 0.5rem 0;
    align-items: flex-start;
  }
  .orb.docked .box img { width: 52px; height: 52px; }
  .orb.docked .row:hover { transform: translateX(2px); }

  /* Hacia ARRIBA y pegado a la derecha. El hueco hasta la burbuja es padding y
     no margen, por la misma razón que en la caja de repositorios: una banda
     muerta entre los dos apagaba el hover a medio camino. */
  .fan {
    position: absolute;
    right: 0;
    bottom: 100%;
    padding: 0.35rem 0 0.5rem 3rem;
    display: flex;
    flex-direction: column-reverse;
    align-items: flex-end;
    gap: 0.35rem;
    opacity: 0;
    pointer-events: none;
    transform: translateY(6px);
    transition: opacity 0.16s ease, transform 0.16s cubic-bezier(0.2, 0.8, 0.3, 1);
  }
  .orb.open .fan {
    opacity: 1;
    pointer-events: auto;
    transform: none;
  }

  .agent,
  .row,
  .empty {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    max-width: 22rem;
    padding: 0.35rem 0.75rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    color: inherit;
    font-size: 0.8rem;
    white-space: nowrap;
  }
  .empty { white-space: normal; border-radius: 14px; color: var(--muted); }
  .agent { color: var(--muted); }
  .agent.on { color: inherit; }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 999px;
    background: var(--faint, #888);
  }
  .agent.on .dot {
    background: #22c55e;
    box-shadow: 0 0 0 3px rgb(34 197 94 / 0.25);
  }

  .row {
    padding: 0 0.25rem 0 0;
    gap: 0.3rem;
    max-width: min(22rem, calc(100dvw - 8rem));
    transition: transform 0.15s ease, background 0.15s ease;
  }
  /* Sólido: `--hover` es translúcido, y sobre el board se veían las burbujas
     a través de la fila. */
  .row:hover {
    background: color-mix(in oklab, var(--accent) 12%, var(--surface-solid));
    transform: translateX(-2px);
  }
  .row.done .row-open { opacity: 0.7; }
  .row-open { display:flex; align-items:center; gap:0.5rem; flex:1; min-width:0; padding:0.35rem 0.2rem 0.35rem 0.75rem; border:0; border-radius:999px; background:transparent; color:inherit; text-align:left; }
  .row-remove { display:grid; place-items:center; flex:0 0 auto; width:1.5rem; height:1.5rem; padding:0; border:0; border-radius:999px; background:transparent; color:var(--color-error-500); }
  .row-remove:hover, .row-remove:focus-visible { background:color-mix(in oklab, var(--color-error-500) 12%, transparent); }
  .row-remove:disabled { opacity:0.5; }
  .delete-error { color:var(--color-error-500); }
  .title {
    max-width: 12rem;
    min-width: 2rem;
    flex: 1 1 12rem;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 600;
  }
  .meta { display:inline-flex; align-items:center; gap:0.15rem; color: var(--muted); font-size: 0.74rem; flex-shrink:0; min-width:0; max-width:min(13rem, calc(100% - 5.25rem)); overflow:hidden; }
  .filename { min-width:0; max-width:8rem; overflow:hidden; text-overflow:ellipsis; }
  .meta > span:not(.filename) { flex-shrink:0; }
  .meta b { min-width:0; overflow:hidden; text-overflow:ellipsis; }
  .face { display: inline-grid; place-items: center; width: 1.3em; }
  .live-dot, :global(.chip .dot) {
    width: 9px; height: 9px; border-radius: 999px; display: inline-block;
    background: tomato; box-shadow: 0 0 0 3px rgb(255 99 71 / 0.25);
  }
  .meta b { color: var(--accent); font-weight: 650; }
  .meta b.ok { color: #4ade80; }
  .warn { color: var(--p1, tomato); }

  .box {
    position: relative;
    display: grid;
    grid-template-areas: 'art';
    isolation: isolate;
    place-items: center;
    width: 56px;
    height: 56px;
    border: none;
    border-radius: 999px;
    background: transparent;
    cursor: pointer;
  }
  .box img {
    position: relative;
    z-index: 1;
    grid-area: art;
    width: 46px;
    height: 46px;
    object-fit: contain;
    filter: drop-shadow(0 8px 12px rgb(11 18 32 / 0.22));
    transition: transform 0.18s cubic-bezier(0.2, 0.8, 0.3, 1), opacity 0.18s ease;
  }
  .lit { opacity: 0; }
  .box.has .still { opacity: 0; }
  .box.has .lit { opacity: 1; }
  .orb.open .box img { transform: scale(1.1) translateY(-2px); }
  /* El agente conectado: un anillo, no un color de la burbuja — el dibujo
     sigue diciendo qué es, y el anillo dice que hay alguien al otro lado. */
  .box.live::after {
    content: '';
    z-index: 0;
    position: absolute;
    inset: 3px;
    border-radius: 999px;
    border: 2px solid #22c55e;
    box-shadow: 0 0 12px rgb(34 197 94 / 0.45);
    pointer-events: none;
  }
  .box:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }

  .count {
    position: absolute;
    z-index: 2;
    top: 0;
    right: 0;
    min-width: 18px;
    height: 18px;
    padding: 0 3px;
    box-sizing: border-box;
    line-height: 1;
    display: grid;
    place-content: center;
    border-radius: 9999px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    font-size: 0.66rem;
    font-weight: 600;
  }
  .count.hot {
    background: var(--accent);
    border-color: var(--accent);
    color: white;
  }

  @media (prefers-reduced-motion: reduce) {
    .fan, .box img, .row { transition: opacity 0.16s ease; }
    .orb.open .box img { transform: none; }
    .row:hover { transform: none; }
  }
</style>
