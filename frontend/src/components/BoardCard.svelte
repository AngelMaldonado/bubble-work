<script lang="ts" module>
  /** Quién tiene esto, para la fila de avatares. */
  export type CardPerson = { id: string; name: string; avatar?: string };
</script>

<script lang="ts">
  // UNA tarjeta, para el kanban y para la secuencia.
  //
  // Vivía dentro de `Kanban.svelte` y la secuencia dibujaba la suya, parecida
  // pero no igual: distinta tipografía, distinto pie, y la prioridad en otro
  // sitio. Dos tarjetas que representan la misma cosa acaban divergiendo, y la
  // que se queda atrás es la que nadie mira — así que es un componente, y las
  // dos pantallas piden la misma.
  //
  // Lo que cambia entre una y otra no es la tarjeta: es qué datos tiene la fila
  // detrás. Un módulo sabe cuántas piezas lleva dentro y un hilo no; un hilo
  // sabe quién lo tiene. Cada campo se dibuja si viene y se calla si no, que es
  // lo que permite que sea una sola.
  import SpoolIcon from '@lucide/svelte/icons/spool';
  import SproutIcon from '@lucide/svelte/icons/sprout';
  import type { Snippet } from 'svelte';
  import { ago, when } from '../lib/when';
  import type { Card } from './Kanban.svelte';

  let {
    card,
    people = [],
    lifting = false,
    onopen,
    ondelete,
    footer,
    attach,
  }: {
    card: Card;
    /** quién la tiene. El kanban no los pasa; la secuencia sí — en una línea de
     *  ejecución, «de quién es esto» se pregunta en cada fila. */
    people?: CardPerson[];
    /** se está arrastrando: se apaga en su sitio mientras viaja */
    lifting?: boolean;
    onopen?: () => void;
    ondelete?: () => void;
    /** Verbos propios de quien la dibuja —terminar, dar por revisado—, DENTRO
     *  de la tarjeta y bajo su contenido. Dentro porque son de ella: sueltos
     *  debajo se leían como si fueran de la lista. Fuera del botón que la abre,
     *  porque un botón no puede contener otro. */
    footer?: Snippet;
    /** el enganche de arrastre, que lo pone quien sabe a dónde puede ir */
    attach?: (el: HTMLElement) => void | (() => void);
  } = $props();

  /** La tarjeta dice la fecha como la dice una persona; el valor sigue ISO. */
  const dayFmt = new Intl.DateTimeFormat('es-MX', { day: 'numeric', month: 'short' });
  function shortDate(iso: string): string {
    const d = new Date(iso + 'T00:00:00');
    return Number.isNaN(d.getTime()) ? iso : dayFmt.format(d);
  }

  /** El día en que nació, para el tooltip — donde «hace 3 meses» ya no basta. */
  const bornFmt = new Intl.DateTimeFormat('es-MX', { day: 'numeric', month: 'short', year: 'numeric' });
  function bornOn(iso: string): string {
    const t = when(iso);
    return Number.isNaN(t) ? iso : bornFmt.format(new Date(t));
  }
</script>

<article class="card" class:lifting {@attach (el) => attach?.(el)}>
  <button class="open" onclick={onopen}>
    <!-- El proyecto y el título en la MISMA línea, el proyecto delante: en un
         tablero que cruza el departamento es lo que sitúa la tarjeta, y se lee
         antes que el nombre porque es el orden en que se usa. En su propio
         renglón se comía un tercio del alto de cada tarjeta.
         La fila deja 24px libres a la derecha: ahí aparece la ×, y sin el hueco
         cae encima de las palabras. Reservado y no desplazado al pasar el
         ratón, para que nada se mueva bajo el puntero. -->
    <span class="head">
      {#if card.where}<span class="where">{card.where}</span>{/if}
      <!-- Una línea y punto, con el nombre entero en el tooltip: una tarjeta
           mide lo mismo que todas las demás, y una lista de tarjetas de altos
           distintos deja de poder recorrerse de un vistazo. -->
      <span class="ttl" title={card.title}>{card.title}</span>
    </span>
    <span class="meta">
      {#if card.prio}<span class="prio-chip prio-{card.prio}">{card.prio}</span>{/if}
      {#if card.objName}
        <span class="obj" title="Objetivo: {card.objName}">{card.objName}</span>
      {:else if card.obj}
        <span class="obj">O{card.obj}</span>
      {/if}
      {#if card.due}<span class="due">{shortDate(card.due)}</span>{/if}
    </span>

    {#if people.length}
      <span class="people">
        {#each people as p (p.id)}
          <span class="avatar" title={p.name}>
            {#if p.avatar}
              <img src={p.avatar} alt={p.name} />
            {:else}
              {(p.name || '?').slice(0, 1).toUpperCase()}
            {/if}
          </span>
        {/each}
      </span>
    {/if}

    <!-- El pie: el tamaño de esto — cuánto abarca y cuánto lleva. -->
    {#if card.pieces || card.born}
      <span class="foot">
        {#if card.pieces}
          <span class="pieces" title="{card.pieces} {card.pieces === 1 ? 'hilo' : 'hilos'} dentro">
            <SpoolIcon class="size-3" aria-hidden="true" />{card.pieces}
          </span>
        {/if}
        {#if card.born}
          <!-- Desde que nació, no desde la última evidencia: eso es el calor del
               board, y aquí se pregunta otra cosa — cuánto lleva esto en el
               plan. -->
          <span class="age" title="Creada el {bornOn(card.born)}">
            <SproutIcon class="size-3" aria-hidden="true" />{ago(card.born)}
          </span>
        {/if}
      </span>
    {/if}
  </button>

  {#if footer}
    <div class="actions">{@render footer()}</div>
  {/if}

  {#if ondelete}
    <!-- FUERA del botón de la tarjeta: un borrado que se puede pulsar mientras
         se apunta a abrir algo es un borrado en el que nadie confía. -->
    <button
      class="x"
      aria-label="eliminar «{card.title}»"
      title="eliminar"
      onclick={(e) => {
        e.stopPropagation();
        ondelete();
      }}>×</button>
  {/if}
</article>

<style>
  .card {
    position: relative;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface-solid);
    box-shadow: 0 1px 2px var(--shadow);
  }
  /* Se enseña cuando la tarjeta se está usando —ratón encima o foco dentro—.
     Una × por tarjeta, siempre visible, es una columna de invitaciones a tirar
     trabajo. */
  .x {
    position: absolute;
    top: 0.25rem;
    right: 0.25rem;
    display: grid;
    place-content: center;
    width: 20px;
    height: 20px;
    border-radius: 6px;
    color: var(--faint);
    font-size: 0.95rem;
    line-height: 1;
    opacity: 0;
    transition: opacity 0.12s ease;
  }
  .card:hover .x,
  .card:focus-within .x { opacity: 1; }
  .x:hover { background: var(--hover); color: var(--color-error-500); }
  .card.lifting { opacity: 0.4; }
  .open {
    display: block;
    width: 100%;
    padding: 0.55rem 0.6rem;
    text-align: left;
    background: transparent;
  }
  .card:hover { border-color: color-mix(in oklab, var(--accent) 40%, transparent); }
  /* Los verbos de la tarjeta: alineados a la derecha, bajo su contenido y con
     el mismo margen que él. */
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.3rem;
    padding: 0 0.6rem 0.5rem;
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
    min-width: 0;
    /* El hueco de la × lo reserva la FILA, no el título: con el badge delante,
       el que llega al borde derecho puede ser cualquiera de los dos. */
    padding-right: 1.4rem;
  }
  .ttl {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.86rem;
    line-height: 1.3;
    color: var(--text);
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    min-width: 0;
    margin-top: 0.35rem;
    font-size: 0.7rem;
  }
  .obj, .due { color: var(--faint); }
  .due { margin-left: auto; }
  /* El objetivo se recorta y no empuja: es texto libre y los de verdad son
     frases, así que sin `min-width: 0` en el flex se comía la fila y echaba
     fuera la prioridad. Completo en el tooltip. */
  .obj {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pieces {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    color: var(--faint);
  }

  .people { display: flex; margin-top: 0.4rem; }
  .avatar {
    display: grid;
    place-content: center;
    width: 20px;
    height: 20px;
    margin-left: -5px;
    border: 1px solid var(--surface-solid);
    border-radius: 999px;
    overflow: hidden;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.6rem;
    font-weight: 700;
  }
  .avatar:first-child { margin-left: 0; }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }

  /* El pie: edad a la izquierda, proyecto a la derecha, en la misma línea. */
  .foot {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.4rem;
    font-size: 0.64rem;
  }
  .age {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    color: var(--faint);
  }
  /* De qué proyecto es: una insignia delante del título, en su misma línea.
     Nunca se lleva más de un tercio de la fila — sitúa la tarjeta, no es lo que
     la tarjeta dice—, y un proyecto de nombre largo se recorta antes que
     empujar el nombre fuera. */
  .where {
    flex: 0 1 auto;
    min-width: 2.5rem;
    max-width: 33%;
    padding: 0.05rem 0.45rem;
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.64rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
