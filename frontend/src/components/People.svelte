<script lang="ts">
  // Who is in this workspace, and how somebody else gets in.
  //
  // Belonging was a row nobody could write from the product: the rules have
  // supported inviting since phase 1 — a workspace's lead adds a membership, and
  // the last lead can neither leave nor demote themselves — and the only way to
  // exercise any of it was the PocketBase dashboard. So a fresh account had one
  // move available, founding a workspace of its own, which is the opposite of
  // what a workspace is for.
  //
  // Reading the roster is every member's; writing it is the lead's. The panel
  // shows the same list to both and simply stops offering the verbs, because a
  // button that always answers 404 is worse than no button.
  import { Combobox, Dialog, Portal, useListCollection } from '@skeletonlabs/skeleton-svelte';
  import { api, type Member, type Person, type Workspace } from '../lib/api';
  import Confirm, { type Doom } from './Confirm.svelte';

  let {
    open = $bindable(false),
    workspace,
    onchanged,
  }: {
    open?: boolean;
    workspace: Workspace;
    /** somebody joined or left — the roster the board reads is stale */
    onchanged?: () => void;
  } = $props();

  let members = $state<Member[]>([]);
  let people = $state<Person[]>([]);
  let error = $state('');
  let doom = $state<Doom>(null);
  let picked = $state('');
  let hunting = $state('');
  // Tu propio nombre, editable donde se lee. `display_name` es opcional y una
  // cuenta creada desde el dashboard casi nunca lo trae, así que sin esto la
  // única forma de dejar de ser un correo era el dashboard otra vez.
  let renamingMe = $state(false);
  let myName = $state('');

  const nameOf = (p: Person) => p.display_name || p.email || 'sin nombre';

  // May I write here? The server is the authority — every rule below reads
  // `memberships.role = 'lead'`, plus the global lead's bypass — and this is
  // only what the panel offers.
  const mine = $derived(members.find((m) => m.user === api.me?.id));
  const canInvite = $derived(mine?.role === 'lead' || api.me?.role === 'lead');
  // The last lead is not removable and not demotable, and the server says so
  // with a 400. Saying it here too is the difference between a rule and a
  // surprise.
  const leads = $derived(members.filter((m) => m.role === 'lead').length);

  // Who is NOT in yet. Inviting somebody who already belongs is not an error
  // worth explaining — it is a name that should not have been on the list.
  const outsiders = $derived(
    people
      .filter((p) => !members.some((m) => m.user === p.id))
      .filter((p) => nameOf(p).toLowerCase().includes(hunting.trim().toLowerCase())),
  );
  const collection = $derived(
    useListCollection({
      items: outsiders,
      itemToString: nameOf,
      itemToValue: (p: Person) => p.id,
    }),
  );

  async function load() {
    try {
      [members, people] = await Promise.all([api.members(workspace.id), api.people()]);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }

  $effect(() => {
    if (open) {
      workspace.id;
      load();
    }
  });

  async function write(run: () => Promise<unknown>) {
    try {
      await run();
      await load();
      onchanged?.();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function invite() {
    const who = picked;
    if (!who) return;
    picked = '';
    hunting = '';
    write(() => api.invite(workspace.id, who));
  }

  /** Tu nombre, y la copia que el resto de la app lleva de ti. Sin el refresh
   *  el panel se actualiza y el saludo de arriba sigue diciendo el anterior. */
  const renameMe = (name: string) =>
    write(async () => {
      await api.update('users', api.me!.id, { display_name: name });
      await api.refresh();
    });

  function drop(m: Member) {
    doom = {
      title: `¿Sacar a ${m.name} de ${workspace.name}?`,
      body: 'Deja de ver este proyecto: su board, sus threads y su wiki. Lo que escribió se queda, y volver a entrar es otra invitación.',
      verb: 'Sacar',
      go: () => write(() => api.remove('memberships', m.id)),
    };
  }
</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Portal>
    <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-center justify-center p-4"
      style="z-index: var(--z-drawer)">
      <Dialog.Content class="card bg-surface-100-900 w-full max-w-lg space-y-4 p-5 shadow-xl">
        <header class="flex items-start gap-3">
          <Dialog.Title class="min-w-0 flex-1 text-lg font-bold">
            Personas de {workspace.name}
          </Dialog.Title>
          <Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface">✕</Dialog.CloseTrigger>
        </header>

        {#if error}
          <p class="err" role="alert">{error}</p>
        {/if}

        <ul class="roster">
          {#each members as m (m.id)}
            <li class="row">
              {#if m.user === api.me?.id && renamingMe}
                <input
                  class="mine"
                  placeholder="¿Cómo te llamas?"
                  autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                  bind:value={myName}
                  {@attach (el: HTMLInputElement) => { el.focus(); el.select(); }}
                  onblur={() => { renamingMe = false; }}
                  onkeydown={(e) => {
                    if (e.key === 'Escape') renamingMe = false;
                    if (e.key === 'Enter' && myName.trim()) {
                      renamingMe = false;
                      renameMe(myName.trim());
                    }
                  }} />
              {:else if m.user === api.me?.id}
                <button
                  class="min-w-0 flex-1 truncate text-left"
                  title="cambiar tu nombre"
                  onclick={() => { myName = api.me?.display_name ?? ''; renamingMe = true; }}>
                  {m.name} <span class="faint text-xs">(tú)</span>
                </button>
              {:else}
                <span class="min-w-0 flex-1 truncate">{m.name}</span>
              {/if}
              {#if canInvite}
                <!-- El rol se cambia aquí mismo. `lead` en un workspace es quien
                     invita y quien configura; no es el lead global, que es un
                     campo de la persona y vive en otra parte. -->
                <select
                  value={m.role}
                  disabled={m.role === 'lead' && leads === 1}
                  title={m.role === 'lead' && leads === 1 ? 'es el último lead' : ''}
                  onchange={(e) =>
                    write(() =>
                      api.setRole(m.id, (e.currentTarget as HTMLSelectElement).value as 'lead' | 'member'),
                    )}>
                  <option value="member">miembro</option>
                  <option value="lead">lead</option>
                </select>
                <button
                  class="x"
                  aria-label="sacar a {m.name}"
                  disabled={m.role === 'lead' && leads === 1}
                  title={m.role === 'lead' && leads === 1 ? 'es el último lead' : 'sacar'}
                  onclick={() => drop(m)}>×</button>
              {:else}
                <span class="faint text-xs">{m.role === 'lead' ? 'lead' : 'miembro'}</span>
              {/if}
            </li>
          {:else}
            <li class="faint text-sm">Nadie todavía.</li>
          {/each}
        </ul>

        {#if canInvite}
          <div class="invite">
            <Combobox
              class="min-w-0 flex-1"
              placeholder="¿A quién invitas?"
              {collection}
              value={picked ? [picked] : []}
              inputValue={hunting}
              onInputValueChange={(e: { inputValue: string }) => (hunting = e.inputValue)}
              onValueChange={(e: { value: string[] }) => (picked = e.value[0] ?? '')}>
              <Combobox.Control>
                <Combobox.Input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
                <Combobox.Trigger />
              </Combobox.Control>
              <Portal>
                <Combobox.Positioner>
                  <Combobox.Content class="who-list">
                    {#each outsiders as p (p.id)}
                      <Combobox.Item item={p}>
                        <Combobox.ItemText>{nameOf(p)}</Combobox.ItemText>
                        <Combobox.ItemIndicator />
                      </Combobox.Item>
                    {:else}
                      <p class="faint px-2 py-1 text-xs">ya está todo el mundo aquí</p>
                    {/each}
                  </Combobox.Content>
                </Combobox.Positioner>
              </Portal>
            </Combobox>
            <button class="btn btn-sm preset-filled-primary-500" disabled={!picked} onclick={invite}>
              Invitar
            </button>
          </div>
          <p class="faint text-xs">
            Entra como miembro: ve el board, escribe y crea trabajo. Un lead además invita y
            configura el flujo.
          </p>
        {:else}
          <p class="faint text-xs">
            Solo un lead de este workspace puede invitar o sacar a alguien.
          </p>
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<Confirm bind:ask={doom} />

<style>
  .roster { display: flex; flex-direction: column; gap: 0.15rem; margin: 0; padding: 0; list-style: none; }
  .row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.3rem 0.4rem;
    border-radius: 8px;
    font-size: 0.88rem;
  }
  .row:hover { background: var(--hover); }
  .mine {
    flex: 1;
    min-width: 0;
    padding: 0.15rem 0.35rem;
    border: 1px solid var(--accent, var(--line));
    border-radius: 7px;
    background: var(--surface);
    color: var(--text);
    font-size: 0.85rem;
  }
  .row select {
    padding: 0.15rem 0.35rem;
    border: 1px solid var(--line);
    border-radius: 7px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.76rem;
  }
  .row select:disabled { opacity: 0.5; }
  .x { width: 24px; color: var(--faint); font-size: 1rem; line-height: 1; }
  .x:hover:not(:disabled) { color: var(--text); }
  .x:disabled { opacity: 0.3; }

  .invite { display: flex; align-items: center; gap: 0.5rem; }
  .invite :global([data-scope='combobox'][data-part='root']) { min-width: 0; width: 100%; }
  .invite :global([data-scope='combobox'][data-part='control']) {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    min-height: 2.1rem;
    padding: 0.1rem 0.45rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
  }
  .invite :global([data-scope='combobox'][data-part='control']:focus-within) {
    border-color: color-mix(in oklab, var(--accent) 60%, transparent);
  }
  .invite :global(input) {
    min-width: 0;
    padding: 0.15rem 0.25rem;
    border: none;
    border-radius: 0;
    background: transparent;
    box-shadow: none;
    color: var(--text);
    font-size: 0.85rem;
    outline: none;
  }
  .invite :global([data-scope='combobox'][data-part='trigger']) {
    flex: none;
    color: var(--faint);
    font-size: 0.8rem;
  }

  .err {
    margin: 0;
    padding: 0.5rem 0.7rem;
    border-radius: 9px;
    background: color-mix(in oklab, var(--p1, tomato) 14%, transparent);
    color: var(--text);
    font-size: 0.82rem;
  }
</style>
