<script lang="ts">
  import { api, type ThreadHeat, type Workspace } from './lib/api';
  import SignIn from './components/SignIn.svelte';
  import Board from './components/Board.svelte';
  import NewWorkspace from './components/NewWorkspace.svelte';
  import { Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import ThemePage from './components/ThemePage.svelte';
  import MockPage from './components/MockPage.svelte';
  import ThemeToggle from './components/ThemeToggle.svelte';

  // Routing, such as it is. One extra page does not need a router, and the
  // server already serves index.html for any unknown path. When there are three
  // of these, replace this with something that deserves the name.
  let path = $state(location.pathname);
  addEventListener('popstate', () => (path = location.pathname));
  function go(to: string) {
    history.pushState({}, '', to);
    path = to;
  }

  let ready = $state(false);
  let signedIn = $state(false);
  let workspaces = $state<Workspace[]>([]);
  let current = $state<Workspace | null>(null);
  let creating = $state(false);

  async function boot() {
    const me = await api.refresh();
    signedIn = !!me;
    if (signedIn) {
      workspaces = await api.workspaces();
      // Keep the one being looked at across a reload of the list.
      current = workspaces.find((w) => w.id === current?.id) ?? workspaces[0] ?? null;
    }
    creating = false;
    ready = true;
  }
  boot();

  function openThread(t: ThreadHeat) {
    // Phase 4b: the thread interior. Until then, say so rather than doing
    // nothing — a button that silently ignores a click is worse than no button.
    alert(`#${t.seq} ${t.name}\n\n${t.heat.reason}`);
  }
</script>

<ThemeToggle />

{#if path === '/theme/mock'}
  <!-- The interface as a whole, with invented data. No sign-in for the same
       reason as /theme: there is nothing real behind it. The way back is the
       wordmark in its own bar — a floating button here landed on top of it. -->
  <MockPage />
{:else if path === '/theme'}
  <!-- No sign-in: it shows no data, and looking at the design system should not
       require an account. -->
  <div class="mx-auto max-w-4xl px-6 pt-6">
    <button class="btn btn-sm preset-tonal-surface" onclick={() => go('/')}>← volver</button>
  </div>
  <ThemePage />
{:else if !ready}
  <p class="faint p-6 text-sm">…</p>
{:else if !signedIn}
  <SignIn onDone={boot} />
{:else}
  <div class="mx-auto max-w-6xl p-4 sm:p-6">
    <header class="mb-6 flex flex-wrap items-center gap-3">
      <h1 class="display text-lg">bubble.work</h1>

      {#if workspaces.length > 1}
        <!-- A Menu rather than a native <select>: the popup of a <select> is drawn
             by the operating system, where no stylesheet reaches it, and a
             switcher is something people look at often enough to notice. -->
        <Menu onSelect={(e: { value: string }) => (current = workspaces.find((w) => w.id === e.value) ?? current)}>
          <Menu.Trigger>
            <span class="btn btn-sm preset-tonal-surface">{current?.name ?? 'workspace'} ▾</span>
          </Menu.Trigger>
          <Portal>
            <Menu.Positioner>
            <Menu.Content>
              {#each workspaces as w (w.id)}
                <Menu.Item value={w.id}><Menu.ItemText>{w.name}</Menu.ItemText></Menu.Item>
              {/each}
            </Menu.Content>
            </Menu.Positioner>
          </Portal>
        </Menu>
      {:else if current}
        <span class="muted text-sm">{current.name}</span>
      {/if}

      {#if workspaces.length}
        <button class="faint text-sm hover:[color:var(--text)]" onclick={() => (creating = !creating)}>
          {creating ? 'cancelar' : '+ workspace'}
        </button>
      {/if}

      <div class="ml-auto flex items-center gap-3 text-sm">
        <!-- The theme control is the floating one now: it reaches every screen,
             including the ones without this header. -->
        <button class="faint hover:[color:var(--text)]" onclick={() => go('/theme')}>tema</button>
        <button class="faint hover:[color:var(--text)]" onclick={() => { api.signOut(); signedIn = false; }}>
          salir
        </button>
      </div>
    </header>

    {#if creating || !workspaces.length}
      {#if !workspaces.length}
        <!-- Anybody signed in may found one, and the founder becomes its lead.
             Saying "a lead has to invite you" was simply false, and left a fresh
             install with nothing to do. -->
        <p class="muted mb-3 text-sm">Todavía no hay ningún workspace. Crea el primero.</p>
      {/if}
      <NewWorkspace onDone={boot} />
    {/if}

    {#if current && !creating}
      <Board workspace={current} onOpen={openThread} />
    {/if}
  </div>
{/if}
