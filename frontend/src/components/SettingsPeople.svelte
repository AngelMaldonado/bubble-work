<script lang="ts">
  import FilterPick from './FilterPick.svelte';
  import Loading from './Loading.svelte';
  import Icon from './Icon.svelte';
  // Usuarios: el lead global da de alta, cambia el rol, borra y restaura.
  //
  // Borrar es MARCAR: la persona deja de poder entrar, sus sesiones y sus
  // agentes se cortan, y lo que escribió se queda firmado con su nombre. Sus
  // membresías se conservan, así que restaurarla la deja como estaba. Los
  // candados —no borrarse a uno mismo, no dejar el departamento sin lead
  // global— están en el servidor; aquí sólo se evita ofrecer lo que va a
  // rechazar.
  import { api, avatarUrl, type AgentReleases, type Person, type PersonAdmin } from '../lib/api';
  import { agentsByPerson, type PersonAgents } from '../lib/agentClients';
  import { ago } from '../lib/when';
  import Confirm, { type Doom } from './Confirm.svelte';
  import PersonTile from './PersonTile.svelte';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import ClientLogo from './ClientLogo.svelte';
  import AgentsMdBadge from './AgentsMdBadge.svelte';

  let { me }: { me: Person } = $props();

  let people = $state<PersonAdmin[]>([]);
  let projects = $state<Record<string, string[]>>({});
  // Cuándo latió cada una por última vez: lo más parecido a «último acceso»
  // que el servidor sabe sin guardar nada nuevo.
  let beats = $state<Record<string, string>>({});
  // Con qué agentes trabaja cada una y qué AGENTS.md les llegó: sale de sus
  // tokens, que el lead global ve todos. Si falla (un servidor anterior), la
  // lista sale como antes.
  let agents = $state<Record<string, PersonAgents>>({});
  let releases = $state<AgentReleases | null>(null);
  let loaded = $state(false);
  let error = $state('');
  let doom = $state<Doom>(null);
  let showDeleted = $state(false);

  async function load() {
    try {
      const [all, by, pulse] = await Promise.all([api.allPeople(), api.membershipsByPerson(), api.presence()]);
      people = all;
      projects = by;
      beats = Object.fromEntries(pulse.map((r) => [r.id, r.at]));
      error = '';
      Promise.all([api.tokens(), api.agentReleases()]).then(
        ([tokens, cur]) => {
          releases = cur;
          agents = agentsByPerson(tokens, cur);
        },
        () => (agents = {}),
      );
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }
  $effect(() => {
    load();
  });

  const liveLeads = $derived(people.filter((p) => p.role === 'lead' && !p.deleted_at).length);
  const deletedCount = $derived(people.filter((p) => p.deleted_at).length);
  const shown = $derived(showDeleted ? people : people.filter((p) => !p.deleted_at));
  const seen = (id: string) => beats[id] ?? '';

  async function run(p: Promise<unknown>) {
    try {
      await p;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
    await load();
  }

  // `email` puede no venir: PocketBase lo oculta si la cuenta no lo hizo visible.
  const nameOf = (p: PersonAdmin) => p.display_name || p.email || 'sin nombre';

  function setRole(p: PersonAdmin, role: 'lead' | 'member') {
    if (role === p.role) return;
    run(api.setGlobalRole(p.id, role));
  }

  function remove(p: PersonAdmin) {
    doom = {
      title: `¿Borrar a «${nameOf(p)}»?`,
      body:
        'Deja de poder entrar y se cierran sus sesiones y sus agentes. Lo que escribió se queda firmado con su nombre, y sus proyectos se conservan: restaurarla la deja como estaba.',
      verb: 'Borrar',
      go: () => run(api.deletePerson(p.id)),
    };
  }

  // ---- alta ----
  // En un diálogo, abierto desde la tabla: dar de alta es una acción sobre la
  // lista, no una sección fija que ocupa la pantalla cada vez que se entra.
  let adding = $state(false);
  let addErr = $state('');
  let email = $state('');
  let fullName = $state('');
  let role = $state<'member' | 'lead'>('member');
  const ROLES = [{ value: 'member', label: 'member' }, { value: 'lead', label: 'lead global' }];
  let password = $state('');
  let creating = $state(false);
  let created = $state<{ email: string; password: string } | null>(null);
  let copied = $state(false);

  function generate() {
    const bytes = crypto.getRandomValues(new Uint8Array(12));
    password = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 16);
  }

  function openAdd() {
    email = fullName = password = '';
    role = 'member';
    created = null;
    addErr = '';
    adding = true;
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    creating = true;
    addErr = '';
    try {
      await api.createPerson({ email: email.trim(), name: fullName.trim(), password, role });
      // El diálogo pasa a enseñar la contraseña: es la única vez que se ve.
      created = { email: email.trim(), password };
      email = fullName = password = '';
      role = 'member';
      await load();
    } catch (err) {
      addErr = (err as Error).message;
    } finally {
      creating = false;
    }
  }

  async function copyCreds() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(`${location.origin}\n${created.email}\n${created.password}`);
      copied = true;
      setTimeout(() => (copied = false), 1800);
    } catch {
      addErr = 'El navegador no dejó copiar al portapapeles.';
    }
  }
</script>

<section class="set-section">
  <p class="set-lead">
    Quién tiene cuenta. Borrar a alguien no borra lo que hizo: deja de entrar, y lo que escribió
    sigue firmado con su nombre. Para darle acceso a un proyecto, invítala desde <Icon of="people" /> en ese proyecto.
  </p>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  <div class="set-card">
    <div class="set-row">
      <h3 class="m-0">Personas</h3>
      {#if deletedCount}
        <label class="faint ml-auto text-xs">
          <input type="checkbox" bind:checked={showDeleted} /> ver las borradas ({deletedCount})
        </label>
      {/if}
      <button class="btn btn-sm preset-filled-primary-500" class:ml-auto={!deletedCount} onclick={openAdd}>+ Dar de alta</button>
    </div>

    {#if !loaded}
      <Loading label="Leyendo personas" />
    {:else}
      <ul class="list">
        {#each shown as p (p.id)}
          {@const self = p.id === me.id}
          {@const lastLead = p.role === 'lead' && liveLeads <= 1}
          <PersonTile person={p} name={nameOf(p)} you={self} dead={!!p.deleted_at}>
            {#snippet meta()}
              {p.display_name ? p.email : ''}
              {#if p.deleted_at}
                · borrada {ago(p.deleted_at)}
                {#if p.purged_at}· datos purgados{:else}· sus datos se purgan a los 30 días{/if}
              {:else}
                {p.display_name ? '·' : ''}
                {(projects[p.id] ?? []).length
                  ? `${projects[p.id].length} ${projects[p.id].length === 1 ? 'proyecto' : 'proyectos'}`
                  : 'sin proyectos'}
                {#if seen(p.id)}· visto {ago(seen(p.id))}{/if}
              {/if}
            {/snippet}
            {#snippet actions()}
              {@const ag = p.deleted_at ? undefined : agents[p.id]}
              {#if ag}
                <!-- El CLIENTE de cada agente (MCP no dice el modelo) y la última
                     release de agents_md que le llegó a alguno. -->
                <span class="clients">
                  {#each ag.clients as c (c.key)}
                    <ClientLogo brand={c}
                      tip={`${c.name}${c.version ? ' ' + c.version : ''} · ${c.tokens === 1 ? '1 token' : `${c.tokens} tokens`} · usado ${ago(c.last)}`} />
                  {/each}
                </span>
                {#if ag.release || ag.neverRead}
                  {@const role = ag.release?.role ?? (p.role === 'lead' ? 'planner' : 'operators')}
                  <AgentsMdBadge read={ag.release?.id} at={ag.release?.at} {role}
                    want={(role === 'planner' ? releases?.planner : releases?.operators) ?? ''} />
                {/if}
              {/if}
              {#if p.deleted_at}
                <button class="btn btn-sm preset-tonal-surface" onclick={() => run(api.restorePerson(p.id))}
                  title={p.purged_at ? 'Vuelve sin sus proyectos, equipos ni tokens: se purgaron' : ''}>Restaurar</button>
              {:else}
                {#if self || lastLead}
                  <!-- Bloqueado: se dice por qué en vez de ofrecer un menú inerte. -->
                  <span class="role-fixed" title={self ? 'tu propio rol lo cambia otro lead' : 'es el último lead global'}>
                    {p.role === 'lead' ? 'lead global' : 'member'}
                  </span>
                {:else}
                  <span class="role-pick"><FilterPick items={ROLES} value={p.role} allowEmpty={false} label={`rol de ${p.display_name || p.email}`}
                    onchange={(r) => r && r !== p.role && setRole(p, r as 'lead' | 'member')} /></span>
                {/if}
                <button
                  class="btn btn-sm preset-tonal-error"
                  onclick={() => remove(p)}
                  disabled={self || lastLead}
                  title={self ? 'no puedes borrarte a ti mismo' : lastLead ? 'es el último lead global' : ''}>
                  <Icon of="trash" danger /> Borrar
                </button>
              {/if}
            {/snippet}
          </PersonTile>
        {/each}
      </ul>
    {/if}
  </div>
</section>

<Confirm bind:ask={doom} />

{#if adding}
  <Dialog open onOpenChange={(e: { open: boolean }) => { if (!e.open) adding = false; }}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner class="fixed inset-0 flex items-center justify-center p-4" style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-md space-y-4 p-5 shadow-xl">
          {#if created}
            <Dialog.Title class="text-lg font-bold">✓ {created.email} ya puede entrar</Dialog.Title>
            <Dialog.Description class="muted text-sm">
              Pásale la dirección, su correo y esta contraseña por un canal privado. No se vuelve a
              enseñar; puede cambiarla desde Ajustes → Mi usuario.
            </Dialog.Description>
            <code class="secret">{created.password}</code>
            {#if addErr}<p class="dlg-err" role="alert">{addErr}</p>{/if}
            <div class="flex flex-wrap justify-end gap-2">
              <button class="btn btn-sm preset-tonal-surface" onclick={openAdd}>Dar de alta a otra</button>
              <button class="btn btn-sm preset-filled-primary-500" onclick={copyCreds}>
                {copied ? '✓ copiado' : 'Copiar dirección, correo y contraseña'}
              </button>
              <button class="btn btn-sm preset-tonal-surface" onclick={() => (adding = false)}>Listo</button>
            </div>
          {:else}
            <Dialog.Title class="text-lg font-bold">Dar de alta</Dialog.Title>
            <form class="form" onsubmit={create}>
              <input class="input" type="email" required placeholder="correo" aria-label="Correo"
                {@attach (el: HTMLInputElement) => el.focus()}
                autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" bind:value={email} />
              <input class="input" placeholder="nombre visible (opcional)" aria-label="Nombre visible" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" maxlength="100" bind:value={fullName} />
              <div class="pw">
                <input class="input grow" type="text" required minlength="8" placeholder="contraseña (mín. 8)" aria-label="Contraseña" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" bind:value={password} />
                <button type="button" class="btn btn-sm preset-tonal-surface" onclick={generate}>generar</button>
              </div>
              <span class="role-pick"><FilterPick items={ROLES} bind:value={role} allowEmpty={false} label="rol" /></span>
              {#if addErr}<p class="dlg-err" role="alert">{addErr}</p>{/if}
              <div class="flex justify-end gap-2">
                <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (adding = false)}>Cancelar</button>
                <button class="btn btn-sm preset-filled-primary-500" type="submit" disabled={creating || !email.trim() || password.length < 8}>
                  {#if creating}<Loading compact label="Creando persona" />{:else}Dar de alta{/if}
                </button>
              </div>
            </form>
          {/if}
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

<style>
  .form { display: flex; flex-direction: column; gap: 0.6rem; }
  .grow { flex: 1; min-width: 0; }
  /* El diálogo se dibuja en un portal, fuera de `.body` de Ajustes, así que no
     le llegan sus `.set-row` ni `.set-err`: los suyos, aquí. */
  .pw { display: flex; align-items: center; gap: 0.6rem; }
  .dlg-err { margin: 0; padding: 0.45rem 0.7rem; border-radius: 8px; background: color-mix(in oklab, var(--color-error-500) 12%, transparent); font-size: 0.82rem; }
  .secret {
    display: block;
    margin: 0 0 0.75rem;
    padding: 0.5rem 0.7rem;
    border-radius: 8px;
    background: var(--hover);
    font-size: 0.85rem;
    user-select: all;
  }
  .list { margin: 0.6rem 0 0; padding: 0; list-style: none; }
  .role-pick { width: 10rem; flex: 0 1 10rem; }
  /* Sus agentes: un ClientLogo por cliente (logo o inicial). */
  .clients { display: inline-flex; align-items: center; gap: 0.3rem; }
  .role-fixed { padding: 0.2rem 0.6rem; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.8rem; font-weight: 700; }
</style>
