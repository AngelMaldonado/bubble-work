<script lang="ts">
  import { api, type Board as BoardData, type ThreadHeat, type Workspace } from './lib/api';
  import SignIn from './components/SignIn.svelte';
  import Board from './components/Board.svelte';
  import Thread from './components/Thread.svelte';
  import Wiki from './components/Wiki.svelte';
  import Omnibar, { type Command, type Hit } from './components/Omnibar.svelte';
  import { bandFace } from './lib/bands';
  import { theme } from './lib/theme.svelte';
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

  // The thread being read. A view rather than a route for now: the board is
  // still underneath it, and coming back has to cost nothing.
  let open = $state<ThreadHeat | null>(null);
  // Bumped when a thread closes, to remount the board. Writing warms a bubble,
  // so the board that was true when the thread opened is stale by then — and
  // heat is the server's answer, never one the browser recomputes.
  let visit = $state(0);
  function back() {
    open = null;
    visit += 1;
  }

  // The wiki, by path. `null` is "not looking at it" rather than a separate
  // flag, so there is one place that says which page is on screen.
  let wiki = $state<string | null>(null);

  // ---- the omnibar -----------------------------------------------------------
  //
  // What the browser already holds answers instantly; what only the repository
  // knows is asked for after a pause. Both land in one list because "find the
  // thread called X" and "find where it says X" are the same question asked
  // with different confidence.
  let omni = $state(false);
  let board = $state<BoardData | null>(null);
  let found = $state<Hit[]>([]);
  let searching = $state(false);

  const items = $derived<Hit[]>([
    ...(board?.threads ?? []).map((t) => ({
      id: 't:' + t.id,
      kind: 'thread',
      icon: bandFace(t.heat.lifecycle),
      title: t.name,
      hint: '#' + t.seq,
      group: 'Threads',
    })),
    ...(board?.bubbles ?? []).map((b) => ({
      id: 'b:' + b.id,
      kind: 'burbuja',
      icon: bandFace(b.heat.lifecycle),
      title: b.name,
      hint: b.outcome,
      group: 'Burbujas',
    })),
  ]);

  const commands = $derived<Command[]>([
    { id: 'wiki', icon: '📖', title: 'Abrir la wiki', hint: 'docs/', run: () => openWiki('README.md') },
    { id: 'board', icon: '🫧', title: 'Volver al board', run: back },
    // The same three states the floating control cycles, named so they can be
    // reached directly: from the keyboard, picking is faster than cycling.
    { id: 'light', icon: '☀️', title: 'Tema claro', run: () => theme.set('light') },
    { id: 'dark', icon: '🌙', title: 'Tema oscuro', run: () => theme.set('dark') },
    { id: 'system', icon: '🌗', title: 'Tema automático', hint: 'como el sistema', run: () => theme.set('system') },
    { id: 'signout', icon: '🚪', title: 'Salir de la sesión', run: () => { api.signOut(); signedIn = false; } },
  ]);

  // One request per pause, and the last one wins: an answer that arrives after
  // the query moved on is an answer to a question nobody is asking any more.
  let asked = 0;
  async function query(q: string) {
    const mine = ++asked;
    if (!current || q.length < 2) {
      found = [];
      searching = false;
      return;
    }
    searching = true;
    try {
      const out = await api.search(current.id, q);
      if (mine !== asked) return;
      found = out.hits.map((h: { path: string; title: string; line: number; text: string }) => ({
        id: `p:${h.path}:${h.line}`,
        kind: h.path.startsWith('threads/') ? 'thread' : 'página',
        icon: '¶',
        title: h.text,
        hint: `${h.path}:${h.line}`,
      }));
    } catch {
      if (mine === asked) found = [];
    } finally {
      if (mine === asked) searching = false;
    }
  }

  function openWiki(page: string) {
    open = null;
    wiki = page;
  }

  function openThread(t: ThreadHeat) {
    wiki = null;
    open = t;
  }

  function pick(hit: Hit) {
    const [what, ...rest] = hit.id.split(':');
    const rest0 = rest.join(':');
    if (what === 't') {
      const t = board?.threads.find((x) => x.id === rest0);
      if (t) openThread(t);
    } else if (what === 'b') {
      // A bubble is not a screen: it is a place ON the board, and the board is
      // what shows whether it is floating or sunk.
      back();
      requestAnimationFrame(() =>
        document.getElementById('bw-' + rest0)?.scrollIntoView({ behavior: 'smooth', block: 'center' }),
      );
    } else if (what === 'p') {
      const file = rest0.slice(0, rest0.lastIndexOf(':'));
      // A hit inside a thread's document belongs to the thread, not to the wiki.
      // The path carries the seq the server derived it from, which is how the
      // one maps back to the other without a second round trip.
      const seq = Number(file.match(/^threads\/(\d+)-/)?.[1]);
      const t = seq ? board?.threads.find((x) => x.seq === seq) : undefined;
      if (t) openThread(t);
      else openWiki(file);
    }
  }

  // ⌘K from anywhere, including inside a thread. Not bound in the editor: there
  // ⌘K is CodeMirror's, and stealing a key from the thing that has focus is how
  // a shortcut becomes a surprise.
  function hotkeys(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      omni = true;
    }
  }
</script>

<svelte:window onkeydown={hotkeys} />

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
{:else if open}
  <!-- Full screen: the thread carries its own bar, and the board behind it is
       noise while reading. -->
  <Thread thread={open} onback={back} onsearch={() => (omni = true)} />
{:else if wiki && current}
  <!-- The wiki wears the thread's shell: same bar, same way back. -->
  <Wiki workspace={current} bind:path={wiki} onback={back} onsearch={() => (omni = true)} />
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
        <button class="faint hover:[color:var(--text)]" onclick={() => (omni = true)}>buscar · ⌘K</button>
        <button class="faint hover:[color:var(--text)]" onclick={() => openWiki('README.md')}>wiki</button>
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
      {#key visit}
        <!-- The board's data is lifted here as it lands: the omnibar searches
             what is on screen, and two fetches of the same board could disagree
             about what is on it. -->
        <Board workspace={current} onOpen={openThread} onload={(b) => (board = b)} />
      {/key}
    {/if}
  </div>
{/if}

<!-- Outside the branches on purpose: ⌘K has to answer on the board, inside a
     thread and inside the wiki, and a field that only exists on one screen is a
     field people stop reaching for. -->
{#if signedIn && current}
  <Omnibar bind:open={omni} {items} {found} {commands} {searching} onquery={query} onpick={pick} />
{/if}
