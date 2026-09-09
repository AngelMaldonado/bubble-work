<script lang="ts">
  import { api, type Board as BoardData, type ThreadHeat, type Workspace } from './lib/api';
  import { boardUrl, parse, plannerUrl, threadUrl, wikiUrl } from './lib/routes';
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

  // The address IS the screen. `lib/routes.ts` says what each one looks like;
  // here it is only read, and every navigation goes through `go`.
  let path = $state(location.pathname);
  addEventListener('popstate', () => (path = location.pathname));
  function go(to: string, replace = false) {
    if (to === location.pathname) return;
    history[replace ? 'replaceState' : 'pushState']({}, '', to);
    path = to;
  }
  const route = $derived(parse(path));

  let ready = $state(false);
  let signedIn = $state(false);
  let workspaces = $state<Workspace[]>([]);

  // Which workspace is on screen is a question the address answers. Landing on
  // `/` with no slug picks the first one and REPLACES the entry, so the back
  // button does not walk through a redirect nobody typed.
  const current = $derived(
    ('slug' in route ? workspaces.find((w) => w.slug === route.slug) : null) ?? workspaces[0] ?? null,
  );
  // Only a BOARD with no slug gets filled in. Asking "does this route carry a
  // slug?" caught `/planeador` and `/theme` too, and sent them straight back to
  // the board — the planner button looked like it did nothing, because the
  // address it set was rewritten before the screen could draw.
  $effect(() => {
    if (ready && signedIn && current && route.kind === 'board' && !route.slug) {
      go(boardUrl(current.slug), true);
    }
  });
  // The box that scrolls. The minimap needs it: the shell holds still and the
  // pane moves, so a spy listening to the window sees a page that never scrolls.
  let paneEl = $state<HTMLElement | null>(null);

  async function boot() {
    const me = await api.refresh();
    signedIn = !!me;
    if (signedIn) workspaces = await api.workspaces();
    ready = true;
  }
  boot();

  // ---- the board, fetched HERE ---------------------------------------------
  //
  // One copy, above every screen that needs it: the board draws it, the omnibar
  // searches it, and a link straight to `/w/alpha/t/14` has to resolve `#14`
  // into a thread before the board component is even on the page. Refetched
  // rather than recomputed — heat is a pure function of evidence and TIME, and
  // the server holds both.
  let board = $state<BoardData | null>(null);
  let loadingBoard = $state(false);
  async function loadBoard() {
    const ws = current;
    if (!ws) return;
    loadingBoard = true;
    try {
      board = await api.board(ws.id);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loadingBoard = false;
    }
  }
  $effect(() => {
    current?.id;
    board = null;
    loadBoard();
  });

  // The thread being read, resolved from the seq in the address. Null while the
  // board is still on its way — which is what makes a pasted link work.
  const open = $derived(
    route.kind === 'thread' ? (board?.threads.find((t) => t.seq === route.seq) ?? null) : null,
  );

  /** Back to the board, and ask the server what it says now: writing in a
   *  thread warms its bubble, so the board that was true when it opened is not
   *  any more. */
  function back() {
    if (current) go(boardUrl(current.slug));
    loadBoard();
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
      // Go to it. Which workspace is on screen is the address's answer now, so
      // a new one is somewhere you navigate rather than a variable you set.
      go(boardUrl(made.slug));
      return made.id; // the column names it in place
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function renameWorkspace(id: string, name: string) {
    try {
      const out = await api.renameWorkspace(id, name);
      // The slug does not change with the name, so the address stays valid.
      workspaces = workspaces.map((w) => (w.id === id ? out : w));
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
      const gone = current?.id === id;
      workspaces = workspaces.filter((w) => w.id !== id);
      if (gone && workspaces[0]) go(boardUrl(workspaces[0].slug), true);
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
  const isLead = $derived(api.me?.role === 'lead');
  const openPlanner = () => go(plannerUrl);

  const openWiki = (page: string) => current && go(wikiUrl(current.slug, page));
  const openThread = (t: ThreadHeat) => current && go(threadUrl(current.slug, t.seq));

  // ---- the omnibar -----------------------------------------------------------
  //
  // What the browser already holds answers instantly; what only the repository
  // knows is asked for after a pause. Both land in one list because "find the
  // thread called X" and "find where it says X" are the same question asked
  // with different confidence.
  let omni = $state(false);
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
{:else if route.kind === 'planner'}
  <!-- The strategic layer gets a screen, not a tab inside the operative one —
       and no workspace: the plan is the department's, not this project's. -->
  <Planner onback={back} onsearch={() => (omni = true)} />
{:else if route.kind === 'thread'}
  <!-- Full screen: the thread carries its own bar, and the board behind it is
       noise while reading. It is resolved from the address, so this is also
       what a pasted link lands on — and while the board is on its way there is
       nothing to draw but the wait. -->
  {#if open}
    <Thread thread={open} onback={back} onsearch={() => (omni = true)} />
  {:else if loadingBoard || !board}
    <p class="faint p-6 text-sm">…</p>
  {:else}
    <p class="card glass m-6 p-4 text-sm">
      No hay un thread #{route.seq} en {current?.name ?? 'este workspace'}.
      <button class="link" onclick={back}>volver al board</button>
    </p>
  {/if}
{:else if route.kind === 'wiki' && current}
  <!-- The wiki wears the thread's shell: same bar, same way back. -->
  <Wiki
    workspace={current}
    path={route.page}
    onback={back}
    onopen={(page) => openWiki(page)}
    onsearch={() => (omni = true)} />
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
    onselect={(id) => {
      const w = workspaces.find((x) => x.id === id);
      if (w) go(boardUrl(w.slug));
    }}
    onrename={renameWorkspace}
    ondelete={deleteWorkspace}
    oncreate={createWorkspace}>
    {#if current}
      <Board
        workspace={current}
        {board}
        onOpen={openThread}
        onreload={loadBoard}
        scroller={paneEl} />
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
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
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
