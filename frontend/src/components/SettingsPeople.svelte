<script lang="ts">
  // Usuarios: el lead global da de alta, cambia el rol, borra y restaura.
  //
  // Borrar es MARCAR: la persona deja de poder entrar, sus sesiones y sus
  // agentes se cortan, y lo que escribió se queda firmado con su nombre. Sus
  // membresías se conservan, así que restaurarla la deja como estaba. Los
  // candados —no borrarse a uno mismo, no dejar el departamento sin lead
  // global— están en el servidor; aquí sólo se evita ofrecer lo que va a
  // rechazar.
  import { api, avatarUrl, type Person, type PersonAdmin } from '../lib/api';
  import { ago } from '../lib/when';
  import Confirm, { type Doom } from './Confirm.svelte';

  let { me }: { me: Person } = $props();

  let people = $state<PersonAdmin[]>([]);
  let projects = $state<Record<string, string[]>>({});
  // Cuándo latió cada una por última vez: lo más parecido a «último acceso»
  // que el servidor sabe sin guardar nada nuevo.
  let beats = $state<Record<string, string>>({});
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

  const nameOf = (p: PersonAdmin) => p.display_name || p.email;

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
  let email = $state('');
  let fullName = $state('');
  let role = $state<'member' | 'lead'>('member');
  let password = $state('');
  let creating = $state(false);
  let created = $state<{ email: string; password: string } | null>(null);
  let copied = $state(false);

  function generate() {
    const bytes = crypto.getRandomValues(new Uint8Array(12));
    password = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 16);
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    creating = true;
    error = '';
    try {
      await api.createPerson({ email: email.trim(), name: fullName.trim(), password, role });
      created = { email: email.trim(), password };
      email = fullName = password = '';
      role = 'member';
      await load();
    } catch (err) {
      error = (err as Error).message;
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
      error = 'El navegador no dejó copiar al portapapeles.';
    }
  }
</script>

<section class="set-section">
  <p class="set-lead">
    Quién tiene cuenta. Borrar a alguien no borra lo que hizo: deja de entrar, y lo que escribió
    sigue firmado con su nombre. Para darle acceso a un proyecto, invítala desde 👥 en ese proyecto.
  </p>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  <form class="set-card" onsubmit={create}>
    <h3>Dar de alta</h3>
    <div class="form">
      <input class="input" type="email" required placeholder="correo" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" bind:value={email} />
      <input class="input" placeholder="nombre visible (opcional)" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" maxlength="100" bind:value={fullName} />
      <div class="set-row">
        <input class="input grow" type="text" required minlength="8" placeholder="contraseña (mín. 8)" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" bind:value={password} />
        <button type="button" class="btn btn-sm preset-tonal-surface" onclick={generate}>generar</button>
      </div>
      <div class="set-row">
        <select class="select w-auto" bind:value={role} aria-label="rol">
          <option value="member">member</option>
          <option value="lead">lead global</option>
        </select>
        <button class="btn btn-sm preset-filled-primary-500" type="submit" disabled={creating || !email.trim() || password.length < 8}>
          {creating ? '…' : 'Dar de alta'}
        </button>
      </div>
    </div>
  </form>

  {#if created}
    <div class="set-card" role="status">
      <h3>✓ {created.email} ya puede entrar</h3>
      <p class="help">
        Pásale la dirección, su correo y esta contraseña por un canal privado. No se vuelve a
        enseñar; puede cambiarla desde Ajustes → Mi usuario.
      </p>
      <code class="secret">{created.password}</code>
      <div class="set-row">
        <button class="btn btn-sm preset-filled-primary-500" onclick={copyCreds}>
          {copied ? '✓ copiado' : 'Copiar dirección, correo y contraseña'}
        </button>
        <button class="btn btn-sm preset-tonal-surface ml-auto" onclick={() => (created = null)}>Listo</button>
      </div>
    </div>
  {/if}

  <div class="set-card">
    <div class="set-row">
      <h3 class="m-0">Personas</h3>
      {#if deletedCount}
        <label class="faint ml-auto text-xs">
          <input type="checkbox" bind:checked={showDeleted} /> ver las borradas ({deletedCount})
        </label>
      {/if}
    </div>

    {#if !loaded}
      <p class="faint text-sm">leyendo…</p>
    {:else}
      <ul class="list">
        {#each shown as p (p.id)}
          {@const self = p.id === me.id}
          {@const lastLead = p.role === 'lead' && liveLeads <= 1}
          <li class="person" class:dead={!!p.deleted_at}>
            <span class="av" aria-hidden="true">
              {#if p.avatar}<img src={avatarUrl(p, '64x64')} alt="" />{:else}{nameOf(p)[0]?.toUpperCase()}{/if}
            </span>
            <div class="min-w-0 flex-1">
              <p class="pname">{nameOf(p)}{#if self}<span class="faint"> · tú</span>{/if}</p>
              <p class="meta">
                {p.display_name ? p.email : ''}
                {#if p.deleted_at}
                  · borrada {ago(p.deleted_at)}
                {:else}
                  {p.display_name ? '·' : ''}
                  {(projects[p.id] ?? []).length
                    ? `${projects[p.id].length} ${projects[p.id].length === 1 ? 'proyecto' : 'proyectos'}`
                    : 'sin proyectos'}
                  {#if seen(p.id)}· visto {ago(seen(p.id))}{/if}
                {/if}
              </p>
            </div>
            {#if p.deleted_at}
              <button class="btn btn-sm preset-tonal-surface" onclick={() => run(api.restorePerson(p.id))}>Restaurar</button>
            {:else}
              <select
                class="select w-auto role"
                value={p.role}
                disabled={self || lastLead}
                title={self ? 'tu propio rol lo cambia otro lead' : lastLead ? 'es el último lead global' : ''}
                onchange={(e) => setRole(p, e.currentTarget.value as 'lead' | 'member')}>
                <option value="member">member</option>
                <option value="lead">lead global</option>
              </select>
              <button
                class="btn btn-sm preset-tonal-surface"
                onclick={() => remove(p)}
                disabled={self || lastLead}
                title={self ? 'no puedes borrarte a ti mismo' : lastLead ? 'es el último lead global' : ''}>
                Borrar
              </button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</section>

<Confirm bind:ask={doom} />

<style>
  .form { display: flex; flex-direction: column; gap: 0.6rem; max-width: 30rem; }
  .grow { flex: 1; min-width: 10rem; }
  .help { margin: 0 0 0.6rem; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
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
  .person {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.55rem 0;
    border-top: 1px solid var(--line);
  }
  .person.dead { opacity: 0.6; }
  .av {
    display: grid;
    flex: none;
    place-content: center;
    width: 32px;
    height: 32px;
    overflow: hidden;
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 0.85rem;
    font-weight: 700;
  }
  .av img { width: 100%; height: 100%; object-fit: cover; }
  .pname { margin: 0; font-size: 0.9rem; font-weight: 600; }
  .meta { margin: 0.1rem 0 0; color: var(--faint); font-size: 0.76rem; }
  .role { font-size: 0.8rem; }
</style>
