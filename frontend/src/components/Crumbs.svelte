<script lang="ts">
  // Dónde estás, encima del título de un modal.
  //
  // Encima y no EN el título porque dos de las hojas —la de la burbuja y la de
  // la nota— tienen por título un `<input>` que se edita en vivo, y un
  // breadcrumb no se edita.
  //
  // Sólo los niveles que existen: una burbuja es `Proyecto > Módulo`, una tarea
  // `Proyecto > Módulo > Tarea`, y un diálogo del departamento no tiene ninguno
  // y no pinta nada. Un guion en el hueco de un nivel que no existe es una
  // posición que el ojo aprende a buscar para encontrar siempre lo mismo.
  //
  // Es texto, no navegación: un breadcrumb que navega desde dentro de un modal
  // tendría que decidir si lo cierra, y esa es otra decisión.
  let { parts = [] }: { parts?: (string | null | undefined)[] } = $props();

  const shown = $derived(parts.filter((p): p is string => !!p && !!p.trim()));
</script>

{#if shown.length}
  <p class="crumbs" aria-label="dónde estás">
    {#each shown as part, i}
      {#if i > 0}<span class="sep" aria-hidden="true">›</span>{/if}<span class="part">{part}</span>
    {/each}
  </p>
{/if}

<style>
  .crumbs {
    display: flex;
    align-items: baseline;
    gap: 0.3rem;
    min-width: 0;
    margin-bottom: 0.15rem;
    font-size: 0.72rem;
    line-height: 1.2;
    color: var(--faint);
  }

  /* El último nivel es el que dice dónde estás; los de arriba son contexto. */
  .part {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .part:last-child {
    flex-shrink: 0;
    max-width: 60%;
    color: var(--muted);
  }

  .sep {
    flex-shrink: 0;
    opacity: 0.6;
  }
</style>
