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
  <p class="set-lead">
    Un equipo es un <b>contexto</b>: sus proyectos, objetivos, secuencia, calendario, inbox de ideas y
    canal de WhatsApp son suyos y no se mezclan con los de otro. El equipo en el que trabajas se elige
    arriba de la columna de proyectos, y es también a dónde va lo que tu IA crea sin decir equipo.
  </p>
  <ul class="faint text-sm notes">
    <li>Un lead global es lead de todo equipo que no sea personal.</li>
    <li>
      Un <b>equipo personal</b> es tu contexto privado: sólo lo ves tú, ni siquiera otro lead global. Para
      trabajar con alguien ahí, invítalo a un <b>proyecto</b> del equipo: verá ese proyecto y nada más.
    </li>
  </ul>

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
    <div class="set-card team" class:mine={t.id === teams.current}>
      <header class="head">
        {#if lead && renaming[t.id] !== undefined}
          <form class="set-row grow" onsubmit={(e) => { e.preventDefault(); const n = renaming[t.id].trim(); if (n) run(`ren:${t.id}`, () => api.renameTeam(t.id, n)); delete renaming[t.id]; }}>
            <input class="input grow" bind:value={renaming[t.id]} aria-label="Nombre del equipo"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
            <button class="btn btn-sm preset-filled-primary-500">Guardar</button>
            <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => delete renaming[t.id]}>Cancelar</button>
          </form>
        {:else}
          <h3 class="name">{t.personal ? '🔒 ' : ''}{t.name}</h3>
          <span class="badge">{t.personal ? 'personal' : 'compartido'}</span>
          <span class="role">{t.implicit ? 'lead (global)' : t.role}</span>
          {#if t.id === teams.current}<span class="here">trabajando aquí</span>{:else}
            <button class="link" onclick={() => teams.set(t.id)}>Trabajar aquí</button>{/if}
          {#if lead}
            <button class="link" onclick={() => (renaming[t.id] = t.name)}>Renombrar</button>
            {#if t.personal || isAdmin}<button class="link danger" onclick={() => askDelete(t)}>Borrar</button>{/if}
          {/if}
        {/if}
      </header>

      {#if t.personal}
        <p class="faint text-sm">Sólo tú. Para trabajar con alguien aquí, invítalo a un proyecto del equipo.</p>
      {:else}
        <ul class="members">
          {#each t.members ?? [] as m (m.user)}
            <li>
              <span class="who">{m.name || 'Persona'}</span>
              {#if m.global}
                <!-- Un lead global lo es de todo equipo compartido: su fila no
                     cambia eso, así que no se ofrece cambiarla. -->
                <span class="faint" title="Lead de todo equipo compartido">lead global</span>
              {:else if lead}
                <span class="role-pick">
                  <FilterPick items={ROLES} value={m.role} allowEmpty={false} label={`Papel de ${m.name} en ${t.name}`}
                    onchange={(r) => r && r !== m.role && run(`role:${t.id}`, () => api.setTeamMember(t.id, m.user, r as 'lead' | 'member'))} />
                </span>
                <button class="link danger" aria-label={`Quitar a ${m.name} de ${t.name}`}
                  onclick={() => run(`rm:${t.id}`, () => api.removeTeamMember(t.id, m.user))}>quitar</button>
              {:else}
                <span class="faint">{m.role}</span>
              {/if}
            </li>
          {:else}
            <li class="faint">Nadie todavía.</li>
          {/each}
        </ul>
        {#if lead && outside(t).length}
          <form class="set-row add" onsubmit={(e) => { e.preventDefault(); const u = adding[t.id]; if (u) run(`add:${t.id}`, () => api.setTeamMember(t.id, u, 'member')); adding[t.id] = ''; }}>
            <span class="grow">
              <FilterPick items={outside(t).map((p) => ({ value: p.id, label: nameOf(p) }))} bind:value={adding[t.id]}
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
  .notes { margin: 0 0 1rem; padding-left: 1.1rem; list-style: disc; display: grid; gap: 0.25rem; }
  .create { display: grid; gap: 0.6rem; }
  .personal { justify-content: space-between; flex-wrap: wrap; gap: 0.5rem; }
  .team { display: grid; gap: 0.6rem; }
  .team.mine { box-shadow: inset 3px 0 0 var(--accent); }
  .head { display: flex; align-items: center; flex-wrap: wrap; gap: 0.5rem; }
  .name { margin: 0; font-size: 1rem; font-weight: 700; }
  .badge, .role, .here { padding: 0.05rem 0.5rem; border-radius: 999px; font-size: 0.72rem; font-weight: 700; background: var(--hover); color: var(--muted); }
  .here { background: color-mix(in oklab, var(--accent) 18%, transparent); color: var(--text); }
  .link { font-size: 0.82rem; text-decoration: underline; color: var(--muted); }
  .link:hover { color: var(--text); }
  .link.danger { color: var(--color-error-500); }
  .members { display: grid; gap: 0.3rem; margin: 0; padding: 0; list-style: none; }
  .members li { display: flex; align-items: center; gap: 0.6rem; }
  .who { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .role-pick { width: 9rem; flex: none; }
  .grow { flex: 1; min-width: 0; }
</style>
