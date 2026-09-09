<script lang="ts">
  import { api, type Board as BoardData, type ThreadHeat, type Workspace } from './lib/api';
  import SignIn from './components/SignIn.svelte';
  import Board from './components/Board.svelte';
  import Thread from './components/Thread.svelte';
  import Wiki from './components/Wiki.svelte';
  import Planner from './components/Planner.svelte';
  import Omnibar, { type Command, type Hit } from './components/Omnibar.svelte';
  import { bandFace } from './lib/bands';
  import { theme } from './lib/theme.svelte';
  import NewWorkspace from './components/NewWorkspace.svelte';
  import { Dialog, Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import Shell from './components/Shell.svelte';
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
  // The box that scrolls. The minimap needs it: the shell holds still and the
  // pane moves, so a spy listening to the window sees a page that never scrolls.
  let paneEl = $state<HTMLElement | null>(null);

  async function boot() {
    const me = await api.refresh();
    signedIn = !!me;
    if (signedIn) {
      workspaces = await api.workspaces();
      // Keep the one being looked at across a reload of the list.
      current = workspaces.find((w) => w.id === current?.id) ?? workspaces[0] ?? null;
    }
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
    wiki = null;
    planner = false;
    visit += 1;
  }

  // ---- the workspaces in the column ----------------------------------------
  //
  // Founding one takes a name AND a slug: the slug is its address on disk — the
  // git repository's directory — so it is derived once here and never follows a
  // rename. A name changes on a Tuesday; a repository must not move with it.
  async function createWorkspace() {
    const name = 'Workspace nuevo';
    const slug = 'ws-' + Math.random().toString(36).slice(2, 8);
    try {
      const made = await api.createWorkspace(name, slug);
      workspaces = [...workspaces, made];
      current = made;
      return made.id; // the column names it in place
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function renameWorkspace(id: string, name: string) {
    try {
      const out = await api.renameWorkspace(id, name);
      workspaces = workspaces.map((w) => (w.id === id ? out : w));
      if (current?.id === id) current = out;
    } catch (e) {
      error = (e as Error).message;
    }
  }

  // Deleting a workspace takes its rows with it — threads, bubbles, evidence —
  // because the relations cascade. The markdown survives on disk, in its repo,
  // which is the whole point of keeping it there; the board that indexed it does
  // not. That is not a click to honour without asking.
  let doomed = $state<Workspace | null>(null);

  async function deleteWorkspace(id: string) {
    doomed = workspaces.find((w) => w.id === id) ?? null;
  }

  async function reallyDelete() {
    const id = doomed?.id;
    doomed = null;
    if (!id) return;
    try {
      await api.deleteWorkspace(id);
      workspaces = workspaces.filter((w) => w.id !== id);
      if (current?.id === id) current = workspaces[0] ?? null;
    } catch (e) {
      error = (e as Error).message;
    }
  }

  // What the server refused, said out loud. A rename that silently does nothing
  // is worse than one that fails.
  let error = $state('');

  // The planner. A view like the others, and only for a lead: planning is the
  // strategic layer's job, and a screen full of verbs somebody cannot use reads
  // as a broken screen rather than as one that is not theirs.
  let planner = $state(false);
  const isLead = $derived(api.me?.role === 'lead');
  function openPlanner() {
    open = null;
    wiki = null;
    planner = true;
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
    ...(isLead
      ? [{ id: 'planner', icon: '🗓', title: 'Abrir el planeador', hint: 'inbox · calendario · kanban', run: openPlanner }]
      : []),
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
    planner = false;
    wiki = page;
  }

  function openThread(t: ThreadHeat) {
    wiki = null;
    planner = false;
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
{:else if planner && current}
  <!-- The strategic layer gets a screen, not a tab inside the operative one. -->
  <Planner workspace={current} onback={back} onsearch={() => (omni = true)} />
{:else if wiki && current}
  <!-- The wiki wears the thread's shell: same bar, same way back. -->
  <Wiki workspace={current} bind:path={wiki} onback={back} onsearch={() => (omni = true)} />
{:else if !ready}
  <p class="faint p-6 text-sm">…</p>
{:else if !signedIn}
  <SignIn onDone={boot} />
{:else if !workspaces.length}
  <!-- Anybody signed in may found one, and the founder becomes its lead. Saying
       "a lead has to invite you" was simply false, and left a fresh install with
       nothing to do. This is the only screen without the shell: a column of
       workspaces with no workspaces in it is a frame around nothing. -->
  <div class="mx-auto max-w-md p-6 sm:pt-16">
    <p class="muted mb-3 text-sm">Todavía no hay ningún workspace. Crea el primero.</p>
    <NewWorkspace onDone={boot} />
  </div>
{:else}
  <Shell
    items={workspaces.map((w) => ({ id: w.id, name: w.name, hint: w.slug }))}
    current={current?.id ?? ''}
    label="Workspaces"
    newLabel="Nuevo"
    bind:pane={paneEl}
    onselect={(id) => (current = workspaces.find((w) => w.id === id) ?? current)}
    onrename={renameWorkspace}
    ondelete={deleteWorkspace}
    oncreate={createWorkspace}>
    {#if current}
      {#key visit}
        <!-- The board's data is lifted here as it lands: the omnibar searches
             what is on screen, and two fetches of the same board could disagree
             about what is on it. -->
        <Board workspace={current} onOpen={openThread} onload={(b) => (board = b)} scroller={paneEl} />
      {/key}
    {/if}
  </Shell>

  <!-- The verbs that live over the page rather than in it. The theme button
       holds the corner and these pack to its left — a row rather than fixed
       slots, because this screen has no planner and a reserved empty place
       reads as a button that failed to draw. -->
  <div class="floats">
  <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
    <Tooltip.Trigger>
      {#snippet element(attributes: Record<string, unknown>)}
        <button class="float-btn search-btn" {...attributes} onclick={() => (omni = true)}>
          <span aria-hidden="true">🔍</span>
        </button>
      {/snippet}
    </Tooltip.Trigger>
    <Portal>
      <Tooltip.Positioner><Tooltip.Content>Buscar · ⌘K</Tooltip.Content></Tooltip.Positioner>
    </Portal>
  </Tooltip>

  {#if isLead}
    <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
      <Tooltip.Trigger>
        {#snippet element(attributes: Record<string, unknown>)}
          <button class="float-btn" {...attributes} onclick={openPlanner}>
            <span aria-hidden="true">🗓</span>
          </button>
        {/snippet}
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner><Tooltip.Content>Planeador</Tooltip.Content></Tooltip.Positioner>
      </Portal>
    </Tooltip>
  {/if}

  <Tooltip positioning={{ placement: 'top' }} openDelay={120} closeDelay={60}>
    <Tooltip.Trigger>
      {#snippet element(attributes: Record<string, unknown>)}
        <button class="float-btn wiki-btn" {...attributes} onclick={() => openWiki('README.md')}>
          <span aria-hidden="true">📖</span>
        </button>
      {/snippet}
    </Tooltip.Trigger>
    <Portal>
      <Tooltip.Positioner>
        <Tooltip.Content>Wiki de {current?.name ?? ''}</Tooltip.Content>
      </Tooltip.Positioner>
    </Portal>
  </Tooltip>
  </div>

  <!-- Signing out is not a floating button: it is rare, and a rare verb next to
       the two you press all day is the one you press by accident. It lives in
       the omnibar, behind `/`. -->
{/if}

{#if doomed}
  <Dialog open onOpenChange={() => (doomed = null)}>
    <Portal>
      <Dialog.Backdrop class="fixed inset-0 bg-surface-50-950/50" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-md space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">¿Eliminar «{doomed.name}»?</Dialog.Title>
          <Dialog.Description class="muted text-sm">
            Se van con él sus burbujas, sus threads y su evidencia. Los documentos
            siguen en el repositorio en disco — para eso están ahí —, pero el
            tablero que los ordenaba no vuelve.
          </Dialog.Description>
          <div class="flex justify-end gap-2">
            <button class="btn btn-sm preset-tonal-surface" onclick={() => (doomed = null)}>Cancelar</button>
            <button class="btn btn-sm preset-filled-error-500" onclick={reallyDelete}>Eliminar</button>
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

{#if error}
  <!-- What the server refused. Fixed at the top rather than in the pane: the
       refusal belongs to the action, and the pane may have scrolled since. -->
  <div class="err" role="alert">
    <span>{error}</span>
    <button onclick={() => (error = '')} aria-label="cerrar">×</button>
  </div>
{/if}

<!-- Outside the branches on purpose: ⌘K has to answer on the board, inside a
     thread and inside the wiki, and a field that only exists on one screen is a
     field people stop reaching for. -->
{#if signedIn && current}
  <Omnibar bind:open={omni} {items} {found} {commands} {searching} onquery={query} onpick={pick} />
{/if}

<style>
  .err {
    position: fixed;
    top: 1rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: var(--z-toast);
    display: flex;
    align-items: center;
    gap: 0.75rem;
    max-width: min(92vw, 520px);
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--color-error-500);
    border-radius: 12px;
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.85rem;
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.22);
  }
  .err button { color: var(--faint); font-size: 1rem; line-height: 1; }
</style>
