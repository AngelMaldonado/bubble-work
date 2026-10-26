<script lang="ts">
  // Una persona como fila: su cara, su nombre, una línea de detalle y, a la
  // derecha, lo que se puede hacer con ella. La misma en Ajustes → Usuarios y
  // en Ajustes → Equipos, para que una persona se vea igual en los dos lados.
  import type { Snippet } from 'svelte';
  import { avatarUrl } from '../lib/api';

  let {
    person,
    name,
    you = false,
    dead = false,
    meta,
    actions,
  }: {
    /** lo que hace falta para la cara; sin avatar, la inicial del nombre */
    person: { id: string; avatar?: string } | null | undefined;
    name: string;
    you?: boolean;
    dead?: boolean;
    meta?: Snippet;
    actions?: Snippet;
  } = $props();
</script>

<li class="person" class:dead>
  <span class="av" aria-hidden="true">
    {#if person?.avatar}<img src={avatarUrl({ id: person.id, avatar: person.avatar }, '64x64')} alt="" />{:else}{name[0]?.toUpperCase()}{/if}
  </span>
  <div class="who">
    <p class="pname">{name}{#if you}<span class="faint">{' · tú'}</span>{/if}</p>
    {#if meta}<p class="meta">{@render meta()}</p>{/if}
  </div>
  {#if actions}<div class="acts">{@render actions()}</div>{/if}
</li>

<style>
  .person { display: flex; align-items: center; gap: 0.75rem; padding: 0.55rem 0; border-top: 1px solid var(--line); }
  .person.dead { opacity: 0.6; }
  .av {
    display: grid; flex: none; place-content: center; width: 32px; height: 32px; overflow: hidden;
    border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.85rem; font-weight: 700;
  }
  .av img { width: 100%; height: 100%; object-fit: cover; }
  .who { flex: 1; min-width: 0; }
  .pname { margin: 0; font-size: 0.9rem; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { margin: 0.1rem 0 0; color: var(--faint); font-size: 0.76rem; }
  .acts { display: flex; flex: none; align-items: center; gap: 0.5rem; }
  /* En una pantalla angosta (con la columna en rail) las acciones bajan a su
     propia fila, bajo el nombre. */
  @media (max-width: 640px) {
    .person { flex-wrap: wrap; }
    .who { flex-basis: calc(100% - 44px); }
    .acts { flex: 1 1 100%; margin-left: 44px; flex-wrap: wrap; }
  }
</style>
