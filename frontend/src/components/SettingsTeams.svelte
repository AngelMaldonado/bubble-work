<script lang="ts">
  import FilterPick from './FilterPick.svelte';
  // Equipos: los CONTEXTOS de trabajo.
  //
  // Un equipo tiene sus proyectos, objetivos, secuencia, calendario, inbox y
  // canal; lo de uno no se mezcla con lo de otro. Un lead global es lead de
  // todo equipo que no sea personal. Un equipo personal es el contexto privado
  // de una persona: sólo lo ve ella. A otra persona se la invita a un PROYECTO
  // del equipo, no al equipo.
  import { api, type Person, type Team } from '../lib/api';
  import { teams } from '../lib/teams.svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
  import Loading from './Loading.svelte';
  import Icon from './Icon.svelte';
  import PersonTile from './PersonTile.svelte';

  let { me }: { me: Person } = $props();
  const isAdmin = $derived(me.role === 'lead');

  let people = $state<Person[]>([]);
  let error = $state('');
  let busy = $state('');
  let doom = $state<Doom>(null);
  let newName = $state('');
  let adding = $state<Record<string, string>>({});
  let renaming = $state<Record<string, string>>({});

  const hasPersonal = $derived(teams.list.some((t) => t.personal && t.owner === me.id));

  async function load() {
    try {
      const [ps] = await Promise.all([api.people(), teams.load()]);
      people = ps.filter((p) => !('deleted_at' in p && (p as { deleted_at?: string }).deleted_at));
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  async function run(what: string, p: () => Promise<unknown>) {
    busy = what;
    error = '';
    try {
      await p();
      await teams.load();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = '';
    }
  }

  const nameOf = (p: Person) => p.display_name || p.email;
  const byId = $derived(new Map(people.map((p) => [p.id, p])));

  // Renombrar: el nombre ES el campo. Un clic lo abre, Enter o salir guarda,
  // Escape lo deja como estaba.
  function commitRename(t: Team) {
    const n = (renaming[t.id] ?? '').trim();
    delete renaming[t.id];
    if (n && n !== t.name) run(`ren:${t.id}`, () => api.renameTeam(t.id, n));
  }
  const ROLES = [{ value: 'member', label: 'member' }, { value: 'lead', label: 'lead' }];
  const outside = (t: Team) => people.filter((p) => !t.members?.some((m) => m.user === p.id));

  function createShared() {
    const name = newName.trim();
    if (!name) return;
    run('new', async () => {
      await api.createTeam(name);
      newName = '';
    });
  }

  function askDelete(t: Team) {
    doom = {
      title: `¿Borrar el equipo «${t.name}»?`,
      body: 'Sólo se puede borrar un equipo sin proyectos. Sus objetivos, fechas, ideas y canal se van con él.',
      verb: 'Borrar',
      go: () => run(`del:${t.id}`, () => api.deleteTeam(t.id)),
    };
  }
</script>

<section class="set-section">

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  {#if isAdmin}
    <div class="set-card create">
      <h3>Nuevo equipo</h3>
      <form class="set-row" onsubmit={(e) => { e.preventDefault(); createShared(); }}>
        <input class="input grow" placeholder="Nombre del equipo (p. ej. Operaciones)" bind:value={newName}
          autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
        <button class="btn btn-sm preset-filled-primary-500" disabled={!newName.trim() || !!busy}>
          {#if busy === 'new'}<Loading compact label="Creando" />{:else}Crear equipo{/if}
        </button>
      </form>
      {#if !hasPersonal}
        <div class="set-row personal">
          <span class="faint text-sm">Tu contexto privado, para tus propios proyectos.</span>
          <button class="btn btn-sm preset-tonal-primary" disabled={!!busy}
            onclick={() => run('personal', () => api.createPersonalTeam())}>
            {#if busy === 'personal'}<Loading compact label="Creando" />{:else}🔒 Crear mi equipo personal{/if}
          </button>
        </div>
      {/if}
    </div>
  {/if}

  {#each teams.list as t (t.id)}
    {@const lead = t.role === 'lead'}
    <div class="set-card team">
      <header class="head">
        {#if lead && renaming[t.id] !== undefined}
          <input class="input name-edit" bind:value={renaming[t.id]} aria-label="Nombre del equipo"
            {@attach (el: HTMLInputElement) => { el.focus(); el.select(); }}
            onblur={() => commitRename(t)}
            onkeydown={(e) => { if (e.key === 'Enter') commitRename(t); if (e.key === 'Escape') delete renaming[t.id]; }}
            autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
        {:else if lead}
          <!-- El nombre se edita pulsándolo, como el de un proyecto en la columna. -->
          <button class="name editable" title="Renombrar" onclick={() => (renaming[t.id] = t.name)}>
            {t.personal ? '🔒 ' : ''}{t.name}
          </button>
        {:else}
          <h3 class="name">{t.personal ? '🔒 ' : ''}{t.name}</h3>
        {/if}
        <span class="badge">{t.personal ? 'personal' : 'compartido'}</span>
        <span class="badge">{t.implicit ? 'lead (global)' : t.role}</span>
        {#if t.id === teams.current && !teams.viewing}<span class="badge here">trabajando aquí</span>{:else}
          <button class="btn btn-sm preset-tonal-surface" onclick={() => teams.set(t.id)}>Trabajar aquí</button>{/if}
        {#if lead && (t.personal || isAdmin)}
          <button class="btn btn-sm preset-tonal-error ml-auto" aria-label={`Borrar el equipo ${t.name}`} onclick={() => askDelete(t)}>
            <Icon of="trash" danger /> Borrar
          </button>
        {/if}
      </header>

      {#if t.personal}
        <p class="faint text-sm">Sólo tú. Para trabajar con alguien aquí, invítalo a un proyecto del equipo.</p>
      {:else}
        <ul class="members">
          {#each t.members ?? [] as m (m.user)}
            {@const p = byId.get(m.user)}
            <PersonTile person={p} name={m.name || 'Persona'} you={m.user === me.id}>
              {#snippet meta()}{p?.display_name ? p.email : ''}{/snippet}
              {#snippet actions()}
                {#if m.global}
                  <!-- Un lead global lo es de todo equipo compartido: su fila no
                       cambia eso, así que no se ofrece cambiarla. -->
                  <span class="role-fixed" title="Lead de todo equipo compartido">lead global</span>
                {:else if lead}
                  <span class="role-pick">
                    <FilterPick items={ROLES} value={m.role} allowEmpty={false} label={`Papel de ${m.name} en ${t.name}`}
                      onchange={(r) => r && r !== m.role && run(`role:${t.id}`, () => api.setTeamMember(t.id, m.user, r as 'lead' | 'member'))} />
                  </span>
                  <button class="btn btn-sm preset-tonal-surface" aria-label={`Quitar a ${m.name} de ${t.name}`}
                    onclick={() => run(`rm:${t.id}`, () => api.removeTeamMember(t.id, m.user))}>Quitar</button>
                {:else}
                  <span class="role-fixed">{m.role}</span>
                {/if}
              {/snippet}
            </PersonTile>
          {:else}
            <li class="faint text-sm">Nadie todavía.</li>
          {/each}
        </ul>
        {#if lead && outside(t).length}
          <form class="set-row add" onsubmit={(e) => { e.preventDefault(); const u = adding[t.id]; if (u) run(`add:${t.id}`, () => api.setTeamMember(t.id, u, 'member')); adding[t.id] = ''; }}>
            <span class="grow">
              <!-- Function binding, no `adding[t.id]` a secas: un equipo recién creado
                   aún no tiene clave, y enlazar `undefined` a un prop con valor por
                   defecto (`$bindable('')`) es un error de Svelte que tumba Ajustes. -->
              <FilterPick items={outside(t).map((p) => ({ value: p.id, label: nameOf(p) }))}
                bind:value={() => adding[t.id] ?? '', (v) => (adding[t.id] = v)}
                any="Agregar a alguien…" label={`Agregar a ${t.name}`} />
            </span>
            <button class="btn btn-sm preset-tonal-primary" disabled={!adding[t.id] || !!busy}>Agregar</button>
          </form>
        {/if}
      {/if}
    </div>
  {/each}
</section>

<Confirm bind:ask={doom} />

<style>
  .create { display: grid; gap: 0.6rem; }
  .personal { justify-content: space-between; flex-wrap: wrap; gap: 0.5rem; }
  .team { display: grid; gap: 0.6rem; }
  .head { display: flex; align-items: center; flex-wrap: wrap; gap: 0.5rem; }
  .name { margin: 0; font-size: 1.1rem; font-weight: 700; }
  .name.editable { padding: 0.1rem 0.35rem; margin-left: -0.35rem; border-radius: 8px; cursor: text; }
  .name.editable:hover { background: var(--hover); box-shadow: inset 0 0 0 1px var(--line); }
  .name-edit { width: min(18rem, 100%); font-size: 1rem; font-weight: 700; }
  /* Más grandes que antes: dicen qué es el equipo y qué eres en él. */
  .badge { padding: 0.2rem 0.65rem; border-radius: 999px; font-size: 0.8rem; font-weight: 700; background: var(--hover); color: var(--muted); }
  .badge.here { background: color-mix(in oklab, var(--accent) 18%, transparent); color: var(--text); }
  .members { margin: 0; padding: 0; list-style: none; }
  .role-pick { width: 9rem; flex: 0 1 9rem; }
  .role-fixed { padding: 0.2rem 0.6rem; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.8rem; font-weight: 700; }
  .grow { flex: 1; min-width: 0; }
</style>
