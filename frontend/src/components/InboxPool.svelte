<script lang="ts">
  // Lo capturado y sin decidir, como un pool de esferas.
  //
  // Ocupa el sitio del kanban, que se quitó: una nota no tiene etapa, ni
  // columna, ni orden — está ahí hasta que alguien decide qué es.
  //
  // Las esferas son de la MISMA familia que las del board (Orb.svelte): el
  // mismo degradado de tres paradas, la misma sombra interior y el mismo brillo.
  // Lo que cambia es el tono, que sale del id de la nota en vez de la banda:
  // una nota no tiene calor todavía, y pintarla de 🔥 o de 😴 diría algo falso.
  // El tamaño dice cuánto se escribió; la posición y el vaivén también salen
  // del id, así que una nota no cambia de sitio al recargar.
  import type { Note } from './InboxSheet.svelte';
  import Icon from './Icon.svelte';

  import { onDestroy } from 'svelte';
  let { notes = [], onopen, readonly = false }: { notes?: Note[]; onopen?: (n: Note) => void; readonly?: boolean } = $props();
  let now = $state(Date.now());
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => clearInterval(tick));
  function ttl(expires?: string) {
    if (!expires) return 'fijado';
    const ms = new Date(expires).getTime() - now;
    if (ms <= 0) return 'por caducar';
    if (ms >= 86_400_000) return `${Math.ceil(ms / 86_400_000)} d`;
    if (ms >= 3_600_000) return `${Math.ceil(ms / 3_600_000)} h`;
    return `${Math.ceil(ms / 60_000)} min`;
  }
  function expiry(n: Note) {
    return n.artifacts?.filter((a) => !a.pinned && a.expires_at)
      .sort((a, b) => a.expires_at!.localeCompare(b.expires_at!))[0]?.expires_at;
  }

  /** Un número estable por nota: 0..1, del id. */
  function seed(id: string, salt = 0) {
    let h = 2166136261 ^ salt;
    for (const c of id) h = Math.imul(h ^ c.charCodeAt(0), 16777619);
    return ((h >>> 0) % 10000) / 10000;
  }

  // Tonos de la paleta de los orbes (oklch): naranja, violeta, dorado, y los
  // que la completan sin caer en el gris de la tumba.
  const HUES = [40, 300, 85, 200, 150, 340, 260];

  const bubbles = $derived(
    notes.map((n) => {
      return {
        n,
        size: 84,
        dx: Math.round((seed(n.id, 2) - 0.5) * 26),
        dy: Math.round(10 + seed(n.id, 3) * 14),
        dur: (7 + seed(n.id, 4) * 6).toFixed(2),
        delay: (-seed(n.id, 5) * 8).toFixed(2),
        hue: HUES[Math.floor(seed(n.id, 6) * HUES.length)],
        lift: Math.round(seed(n.id, 7) * 40),
      };
    }),
  );

  // La ficha del hover es UNA, fija contra la ventana y no dentro del pool: el
  // pool hace scroll, así que todo lo que sale de él se recorta — y una burbuja
  // en la orilla dejaba la mitad de su ficha fuera. Se coloca debajo de la
  // burbuja y se empuja hacia dentro si no cabe.
  let hover = $state<{ n: Note; x: number; y: number; above: boolean } | null>(null);
  let cardW = $state(0);
  let cardH = $state(0);
  function show(n: Note, el: HTMLElement) {
    const r = el.getBoundingClientRect();
    const below = r.bottom + 12;
    hover = { n, x: r.left + r.width / 2, y: below, above: false };
    // Si no cabe abajo, arriba.
    requestAnimationFrame(() => {
      if (hover && hover.n.id === n.id && below + cardH > innerHeight - 8) {
        hover = { ...hover, y: r.top - 12 - cardH, above: true };
      }
    });
  }
  const cardLeft = $derived(
    hover ? Math.max(8, Math.min(hover.x - cardW / 2, innerWidth - cardW - 8)) : 0,
  );
</script>

<div class="pool">
  <header class="pool-head">
    <h1 class="display text-2xl">
      <Icon of="inbox" size="1.1em" tint /> Inbox <span class="count">{notes.length}</span>
    </h1>
    <p class="hint">{readonly ? 'Capturas y artefactos · clic en una esfera para leerlos' : 'lo capturado y sin decidir · clic en una esfera para abrirla y triarla'}</p>
  </header>

  {#if notes.length}
    <div class="water" onscroll={() => (hover = null)}>
      {#each bubbles as b (b.n.id)}
        <button
          class="bub"
          style:--s="{b.size}px"
          style:--dx="{b.dx}px"
          style:--dy="{b.dy}px"
          style:--dur="{b.dur}s"
          style:--delay="{b.delay}s"
          style:--h={b.hue}
          style:--lift="{b.lift}px"
          onclick={() => onopen?.(b.n)}
          onmouseenter={(e) => show(b.n, e.currentTarget)}
          onfocus={(e) => show(b.n, e.currentTarget)}
          onmouseleave={() => (hover = null)}
          onblur={() => (hover = null)}
          aria-label={b.n.text}>
          <span class="sphere">
            <span class="sheen"></span>
          </span>
          <span class="label">{b.n.text}</span>
          {#if b.n.artifacts?.length}
            <span class="ttl" title="Caduca el artefacto; la nota permanece">
              <Icon of="brain" size="1em" tint /> {expiry(b.n) ? ttl(expiry(b.n)) : 'fijado'}
            </span>
          {/if}
        </button>
      {/each}
    </div>
  {:else}
    <div class="empty">
      <Icon of="bubble" size="4rem" tint />
      <p>Nada capturado. Lo que entre al inbox flota aquí hasta que alguien decida qué es.</p>
    </div>
  {/if}
</div>

{#if hover}
  <div class="card" role="tooltip" bind:clientWidth={cardW} bind:clientHeight={cardH}
    style:left="{cardLeft}px" style:top="{hover.y}px">
    <span class="card-head">
      <Icon of={hover.n.body ? 'document' : 'chat'} tint />
      <b>{hover.n.text}</b>
    </span>
    <span class="meta"><Icon of="people" size="1.05em" tint /> {hover.n.from} · <Icon of="calendar" size="1.05em" tint /> {hover.n.when}</span>
    {#each hover.n.artifacts ?? [] as a (a.id)}
      <span class="meta"><Icon of="brain" size="1.05em" tint /> {a.title} · {a.pinned ? 'fijado · sin caducidad' : `caduca en ${ttl(a.expires_at)}`}</span>
    {/each}
    {#if hover.n.artifacts?.length}<span class="meta">La nota permanece al caducar sus artefactos.</span>{/if}
  </div>
{/if}

<style>
  .pool { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  /* Como el título del board de burbujas: centrado, en la fuente display. */
  .pool-head { text-align: center; padding: 1.4rem 1rem 0.2rem; }
  .pool-head h1 { display: inline-flex; align-items: center; gap: 0.5rem; }
  .count {
    padding: 0 0.55rem;
    border-radius: 999px;
    background: var(--hover);
    font-family: var(--sans, inherit);
    font-size: 0.9rem;
    font-weight: 700;
  }
  .hint { margin: 0.3rem 0 0; color: var(--faint); font-size: 0.8rem; }

  .water {
    flex: 1;
    overflow: auto;
    display: flex;
    flex-wrap: wrap;
    align-content: flex-start;
    justify-content: center;
    gap: 1.6rem 2.2rem;
    /* abajo, el aire de la barra de botones */
    padding: 2rem 1.5rem 6rem;
  }

  .bub {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.55rem;
    width: 108px;
    align-self: flex-start;
    margin-top: var(--lift);
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
    animation: drift var(--dur) ease-in-out var(--delay) infinite alternate;
  }
  .bub:hover,
  .bub:focus-visible { z-index: 5; animation-play-state: paused; }
  .bub:focus-visible { outline: none; }

  /* La esfera del board (Orb.svelte), con el tono de la nota. */
  .sphere {
    --g1: oklch(0.82 0.14 var(--h));
    --g2: oklch(0.64 0.19 calc(var(--h) - 20));
    --g3: oklch(0.52 0.18 calc(var(--h) - 45));
    position: relative;
    display: grid;
    place-items: center;
    width: var(--s);
    height: var(--s);
    border-radius: 50%;
    background:
      radial-gradient(circle at 70% 78%, var(--g3), transparent 60%),
      linear-gradient(150deg, var(--g1) 8%, var(--g2) 52%, var(--g3) 96%);
    box-shadow:
      inset -6px -8px 16px color-mix(in oklab, var(--g3) 70%, black 10%),
      inset 4px 5px 10px color-mix(in oklab, var(--g1) 60%, white 30%);
    transition: transform 0.32s cubic-bezier(0.34, 1.56, 0.4, 1), filter 0.32s ease;
  }
  .bub:hover .sphere,
  .bub:focus-visible .sphere {
    transform: translateY(-8px) scale(1.14);
    filter: brightness(1.06) saturate(1.05);
  }
  .bub:focus-visible .sphere { outline: 2px solid var(--accent); outline-offset: 4px; }
  .sheen {
    position: absolute;
    top: 12%;
    left: 16%;
    width: 42%;
    height: 34%;
    border-radius: 50%;
    background: radial-gradient(ellipse at 38% 34%, oklch(1 0 0 / 0.9), transparent 70%);
    filter: blur(1px);
    pointer-events: none;
  }
  .label {
    position: relative;
    width: 108px;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    color: var(--text);
    font-size: 0.84rem;
    font-weight: 600;
    line-height: 1.15;
    text-align: center;
    overflow-wrap: anywhere;
  }

  .ttl { display: inline-flex; align-items: center; gap: 0.25rem; color: var(--muted); font-size: 0.72rem; }

  .card {
    position: fixed;
    z-index: var(--z-float);
    width: max-content;
    max-width: 18rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface-solid);
    box-shadow: 0 12px 30px rgb(0 0 0 / 0.35);
    pointer-events: none;
  }
  .card-head { display: flex; gap: 0.45rem; align-items: flex-start; font-size: 0.88rem; }
  .meta { display: inline-flex; align-items: center; gap: 0.3rem; color: var(--muted); font-size: 0.76rem; }

  .empty {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: 0.8rem;
    color: var(--muted);
    text-align: center;
    max-width: 24rem;
    margin: 0 auto;
  }

  @keyframes drift {
    from { transform: translate(0, 0); }
    to { transform: translate(var(--dx), calc(var(--dy) * -1)); }
  }
  @media (prefers-reduced-motion: reduce) {
    .bub { animation: none; }
    .sphere { transition: none; }
  }
</style>
