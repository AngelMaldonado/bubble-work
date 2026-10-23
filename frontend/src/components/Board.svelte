<script lang="ts">
  import Loading from './Loading.svelte';
  // The board, against the real server: the same component the mock draws, fed
  // by `/board` instead of by fixtures.
  //
  // Buoyancy is spatial. Bubbles float in the band their hottest thread put
  // them in, and clicking one opens the drawer rather than a page: the whole
  // point of the board is comparison, and a navigation costs you the thing you
  // were comparing against.
  import { api, avatarUrl, type Board, type BubbleHeat, type ThreadHeat, type Workspace } from '../lib/api';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import BlobGraph from './BlobGraph.svelte';
  import Graph3D from './Graph3D.svelte';
  import EncodingPanel from './EncodingPanel.svelte';
  import EncodingLegend from './EncodingLegend.svelte';
  import { encode, readEncoding, COLOR_LABELS, DEFAULT_ENCODING, type Encoding } from '../lib/encoding';
  import { boardGraph, visibleNodes } from '../lib/boardGraph';
  import { groupsFor, projectLinks, type GraphNode, type MapKind } from '../lib/graph';
  import { live } from '../lib/live.svelte';
  import BubbleBoard, { type BoardBubble } from './BubbleBoard.svelte';
  import BubbleDrawer from './BubbleDrawer.svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
  import Minimap, { type MapItem } from './Minimap.svelte';
  import Crumbs from './Crumbs.svelte';
  import { ago, cycleLeft } from '../lib/when';
  import { lastOpenBubble } from '../lib/filters.svelte';
  import { named } from '../lib/people';
  import { teams } from '../lib/teams.svelte';
  import { limited } from '../lib/limits.svelte';

  let {
    workspace,
    board,
    onOpen,
    onreload,
    onnew,
    scroller = null,
  }: {
    /** El workspace en pantalla, o `null` cuando el board es TODOS.
     *
     *  Null y no un workspace inventado: los verbos de esta pantalla —crear una
     *  burbuja, un thread, renombrar, cerrar— necesitan saber DÓNDE aterrizan, y
     *  una vista de varios proyectos no puede contestar cuál. Así que en Todos
     *  se mira y se abre, y lo que se escribe se escribe desde su proyecto. */
    workspace: Workspace | null;
    /** The board, fetched by whoever owns the screen.
     *
     *  Not fetched here any more: a link straight to a thread has to resolve
     *  `#14` before this component is on the page at all, so the one copy of
     *  the board lives above and everybody reads THAT one. Two fetches of the
     *  same board are two answers that can disagree. */
    board: Board | null;
    onOpen: (t: ThreadHeat) => void;
    /** ask for it again — writing here changes what the server would say */
    onreload?: () => void;
    /** right-click on the empty board: make a bubble, or a thread */
    onnew?: (what: 'bubble' | 'thread') => void;
    /** the box that actually scrolls, for the minimap's spy */
    scroller?: HTMLElement | null;
  } = $props();

  let error = $state('');
  const load = () => onreload?.();

  /** Todos: más de un proyecto en la misma pantalla. Es lo que decide si esto es
   *  un board donde se trabaja o un mirador donde se compara. */
  const all = $derived(!workspace);
  // En «Todos los equipos» dos equipos pueden tener un «Soporte»: el
  // proyecto se nombra con su equipo delante.
  const projectOf = (id: string) => {
    const w = (board?.workspaces ?? []).find((x) => x.id === id);
    if (!w) return '';
    return teams.viewing && w.team_name ? `${w.team_name} · ${w.name}` : w.name;
  };

  const threadsOf = (bubble: string) => (board?.threads ?? []).filter((t) => t.bubble === bubble);
  const boardTitle = $derived(workspace?.name ?? (teams.viewing ? 'Todos los equipos' : 'Todos los proyectos'));

  // ---- nivel y disposición ----------------------------------------------------
  //
  // Dos ejes independientes (§10 del diseño): QUÉ es cada punto —Burbujas o
  // Hilos— y CÓMO se acomoda —Flotabilidad, por evidencia, o Blobs, por los
  // tags de sus documentos—, en la barra de abajo. Se recuerdan por persona en
  // este navegador. Flotabilidad es la de siempre y la de partida.
  type Layout = 'flotation' | 'blobs' | '3d';
  const viewKey = () => `bubble.board.view.${api.me?.id ?? 'local'}`;
  function readView(): { level: MapKind; layout: Layout; closed: boolean } {
    try {
      const v = JSON.parse(localStorage.getItem(viewKey()) ?? '{}');
      // «contexts» era el nombre anterior de la misma disposición.
      return { level: v.level === 'threads' ? 'threads' : 'bubbles',
        layout: v.layout === 'blobs' || v.layout === 'contexts' ? 'blobs' : v.layout === '3d' ? '3d' : 'flotation', closed: v.closed === true };
    } catch {
      return { level: 'bubbles', layout: 'flotation', closed: false };
    }
  }
  const savedView = readView();
  let level = $state<MapKind>(savedView.level);
  let layout = $state<Layout>(savedView.layout);
  /** las cerradas sólo si se piden: no dominan el inicio */
  let showClosed = $state(savedView.closed);
  $effect(() => {
    const v = JSON.stringify({ level, layout, closed: showClosed });
    try { localStorage.setItem(viewKey(), v); } catch { /* sin almacenamiento, dura la pestaña */ }
  });

  // Los blobs salen de los tags que trae el tablero: no hay nada más que leer.
  const graph = $derived(board ? boardGraph(board, projectOf, { me: api.me?.id ?? '', teamName: (t) => teams.name(t) }) : null);
  // La disposición se calcula sobre TODO el nivel, cerradas incluidas: mostrar u
  // ocultar cerradas no mueve a nadie de su sitio.
  const universe = $derived(graph ? visibleNodes(graph, level, true) : []);
  const levelNodes = $derived(graph ? visibleNodes(graph, level, showClosed) : []);

  // Qué dicen el tamaño y el color, y qué se resalta (lib/encoding). De cada
  // persona, en este navegador, y lo mismo en Blobs y en 3D.
  const encKey = () => `bubble.board.encoding.${api.me?.id ?? 'local'}`;
  let encoding = $state<Encoding>((() => { try { return readEncoding(JSON.parse(localStorage.getItem(encKey()) ?? 'null')); } catch { return { ...DEFAULT_ENCODING }; } })());
  $effect(() => { const v = JSON.stringify(encoding); try { localStorage.setItem(encKey(), v); } catch { /* dura la pestaña */ } });
  let encOpen = $state(false);
  const levelLinks = $derived(graph ? projectLinks(level, levelNodes, graph.threads, graph.relations) : []);
  const encoded = $derived(encode(levelNodes, levelLinks, encoding));
  const filtering = $derived(encoding.presets.length > 0 || encoding.text.trim() !== '');
  // «Ocultar» quita del dibujo lo que no pasa; la disposición sigue calculada
  // sobre todo el universo, así que nada se reacomoda.
  const shownNodes = $derived(encoding.mode === 'hide' && filtering ? levelNodes.filter((n) => encoded.looks.get(n.id)?.match) : levelNodes);
  const links = $derived(graph ? projectLinks(level, shownNodes, graph.threads, graph.relations) : []);
  const graphGroups = $derived(graph ? groupsFor(universe, graph.groups) : []);

  function changeLevel(next: string) {
    if (next !== 'bubbles' && next !== 'threads') return;
    if (next === level) return;
    level = next;
    // Un punto de otro nivel es otra entidad: no arrastra un cajón abierto.
    drawer = false;
    open = null;
  }

  function openNode(n: GraphNode) {
    if (n.kind === 'bubbles') {
      open = board?.bubbles.find((b) => b.id === n.id) ?? null;
      naming = false;
      drawer = true;
      return;
    }
    const t = board?.threads.find((x) => x.id === n.id);
    if (t) onOpen(t);
  }

  // Who works here. Read once per workspace, for the one field on this screen
  // that needs a person: who is accountable for a bubble.
  //
  // En Todos el orbe lleva el proyecto, pero su drawer necesita nombres.
  // Una lectura del directorio autorizado evita N peticiones de roster.
  let roster = $state<{ id: string; name: string; avatar?: string }[]>([]);
  $effect(() => {
    const id = workspace?.id;
    roster = [];
    let active = true;
    const reading = id
      ? api.roster(id)
      : api.people().then((ps) => ps.map((p) => ({
          id: p.id,
          name: named(p),
          avatar: avatarUrl(p, '64x64'),
        })));
    reading.then((r) => { if (active) roster=r; }).catch(() => { if (active) error='No se pudieron cargar los nombres de los encargados.'; });
    return () => { active=false; };
  });
  const nameOf = (id: string) => roster.find((p) => p.id === id)?.name ?? '';

  // Sin filtro por proyecto. Lo hubo —unas fichas sobre las bandas para
  // limitarse a unos cuantos— y se quitó: en Todos, lo que se está pidiendo es
  // ver TODO junto, y limitarlo es volver a mirar un proyecto, que es lo que
  // hace la columna de la izquierda con un clic y sin esconder nada.
  const orbs = $derived<BoardBubble[]>(
    (board?.bubbles ?? []).map((b) => ({
      id: b.id,
      name: b.name,
      life: b.heat.lifecycle,
      // The orb carries the initials of whoever is accountable. A band tells
      // you a bubble went quiet; the initials tell you who to ask, which is the
      // other half of what 😴 means.
      // Sólo los que ya tienen nombre: el roster llega por su cuenta, y un
      // avatar sin nombre no dice nada que valga la pena dibujar.
      owner: all ? '' : nameOf((b.owners ?? [])[0] ?? ''),
      people: all ? [] : (b.owners ?? []).map(nameOf).filter(Boolean),
      faces: all
        ? {}
        : Object.fromEntries(
            roster.filter((p) => p.avatar && (b.owners ?? []).includes(p.id)).map((p) => [p.name, p.avatar!]),
          ),
      project: all ? projectOf(b.workspace) : '',
      // The flame counts what is PRODUCING inside it, which is the one number
      // an orb can carry without becoming a card.
      burning: threadsOf(b.id).filter((t) => t.heat.lifecycle === 'hot').length,
      priority: b.priority ?? '',
      canDelete: mayDelete(b.workspace),
    })),
  );

  // Flotabilidad en el nivel Hilos: el mismo tablero de orbes, uno por hilo,
  // con su burbuja dicha debajo. Aquí sí van los sueltos: es el nivel que los
  // pide, y su origen dice «sin burbuja».
  const threadOrbs = $derived<BoardBubble[]>(
    (board?.threads ?? []).map((t) => ({
      id: t.id,
      name: t.name,
      life: t.heat.lifecycle,
      priority: t.priority ?? '',
      owner: all ? '' : nameOf((t.assignees ?? [])[0] ?? ''),
      people: all ? [] : (t.assignees ?? []).map(nameOf).filter(Boolean),
      project: [all ? projectOf(t.workspace) : '', t.bubble ? (board?.bubbles.find((b) => b.id === t.bubble)?.name ?? '') : 'sin burbuja']
        .filter(Boolean).join(' · '),
    })),
  );

  const visibleOrbs = $derived((level === 'bubbles' ? orbs : threadOrbs).filter((n) => showClosed || n.life !== 'closed'));
  // El minimapa lee lo que dibuja Flotabilidad. En Blobs se navega con la cámara.
  const mapItems = $derived<MapItem[]>(visibleOrbs.map((b) => ({ id: b.id, name: b.name, life: b.life })));

  let open = $state<BubbleHeat | null>(null);

  // Volver de un thread devuelve el cajón donde estaba.
  //
  // Un thread se abre DESDE una burbuja, así que volver al board y encontrarlo
  // cerrado obliga a buscar otra vez la burbuja de la que acababas de salir. Se
  // guarda al irse y se consume al volver — una vez, no en cada dibujado, o
  // cerrar el cajón a mano lo reabriría en el siguiente cambio del board.
  let restored = $state(false);
  $effect(() => {
    if (restored || !board) return;
    restored = true;
    const id = lastOpenBubble.get();
    const b = id ? (board.bubbles.find((x) => x.id === id) ?? null) : null;
    if (!b) return;
    open = b;
    drawer = true;
    // Y se ve dónde está: el cajón tapa media pantalla, y abrirlo sobre una
    // burbuja que quedó fuera de vista cuenta la mitad de la historia.
    requestAnimationFrame(() =>
      document.getElementById('bw-' + b.id)?.scrollIntoView({ block: 'center' }),
    );
  });

  // De ahí en adelante, lo guardado ES el estado del cajón: abierto se recuerda,
  // cerrado se olvida. Una sola regla en un solo sitio, en vez de anotarlo en
  // cada puerta por la que se puede salir — y las puertas son cuatro: el thread,
  // la ×, Escape y el clic fuera.
  $effect(() => {
    if (!restored) return;
    lastOpenBubble.set(drawer && open ? open.id : '');
  });
  // The drawer reads the bubble from the BOARD, not from the copy taken when it
  // was opened: renaming, handing it over or closing it reloads the board, and
  // a panel showing the snapshot would keep displaying what you just changed.
  const shown = $derived(board?.bubbles.find((b) => b.id === open?.id) ?? open);
  let drawer = $state(false);
  // La barra del HUD se monta en la columna del pane (ver el marcado).
  let hudAnchor = $state<HTMLElement>();
  const hudTarget = $derived((hudAnchor?.closest('.pane')?.parentElement as HTMLElement | null | undefined) ?? undefined);
  let deleteRoles = $state<Record<string, boolean>>({});
  const mayDelete = (ws: string) => api.me?.role === 'lead' || !!deleteRoles[ws];
  $effect(() => {
    const ids=board?.workspaces.map((w) => w.id) ?? [];
    deleteRoles={};
    let active=true;
    if (api.me?.role !== 'lead') Promise.all(ids.map(async (id) => [id,await api.myRole(id)==='lead'] as const))
      .then((roles) => {if(active) deleteRoles=Object.fromEntries(roles);});
    return () => {active=false;};
  });
  const canDelete = $derived(!!shown && mayDelete(shown.workspace));
  function removeBubble(b: BubbleHeat | null) {
    if (!b || !mayDelete(b.workspace)) return;
    doom={title:`¿Eliminar «${b.name}»?`,body:'Se elimina la burbuja, no sus hilos ni sus documentos. Sus hilos quedan sueltos y pueden asignarse a otra burbuja.',crumbs:[projectOf(b.workspace)],go:async () => {
      try { await api.deleteBubble(b.id); if(open?.id===b.id) {drawer=false;open=null;lastOpenBubble.set('');} await load(); }
      catch(e) {error=String(e);}
    }};
  }
  // How much of the cycle this bubble has left since it last produced. The
  // window is the workspace's calibration, so the bar means the same thing the
  // bands do — recalibrating moves both, and neither is stored.
  const cycle = $derived(
    shown && !shown.closed
      ? cycleLeft(shown.warm_at ?? '', Number(board?.tuning?.cycle_hours ?? 0))
      : null,
  );
  // Opens the drawer with its "+ thread" field already asking, so the orb's
  // menu item lands where the drawer's button would have taken you rather than
  // opening a second way to name a thread.
  let naming = $state(false);
  let renaming = $state<BubbleHeat | null>(null);
  let fresh = $state('');
  let doom = $state<Doom>(null);
  // Closing asks for one line — how it ended, in the words of whoever closed
  // it. It is optional and it is the reason this is not a plain confirmation:
  // a bubble that dies without saying why teaches nobody anything.
  let closing = $state<BubbleHeat | null>(null);
  let closure = $state('');

  async function newThread(name: string) {
    if (!open || !workspace) return;
    try {
      await api.createThread({ workspace: workspace.id, bubble: open.id, name });
      // Creating a thread IS evidence, so the band this bubble sits in can have
      // changed by the time the drawer closes. Ask the server rather than
      // guessing: it is the one that classifies.
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  /** Every write here reloads: the server classifies, and a board patched in
   *  the browser is a second opinion about which band something is in. */
  async function write(run: () => Promise<unknown>) {
    try {
      await run();
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function removeThread(seq: number) {
    const t = threadsOf(open?.id ?? '').find((x) => x.seq === seq);
    if (!t) return;
    doom = {
      title: `¿Borrar «${t.name}»?`,
      body: 'El hilo #' + t.seq + ' sale de la burbuja y del planeador. Su documento sigue en la historia de git, que es de donde se recupera si hacía falta.',
      crumbs: [projectOf(open?.workspace ?? '') || workspace?.name || '', open?.name],
      go: () => write(() => api.deleteThread(t.id)),
    };
  }

  /** The orb's right-click. The same four things the drawer does, reached from
   *  where the eye already is — the bubble itself — instead of after opening
   *  it. Nothing new happens here: this is the drawer's verbs, wired. */
  function act(what: string, b: BoardBubble) {
    const bubble = board?.bubbles.find((x) => x.id === b.id) ?? null;
    if (!bubble) return;
    if (what === 'delete') { removeBubble(bubble); return; }
    // En Todos el menú del orbe abre, y nada más. Renombrar o cerrar una burbuja
    // desde una pantalla que no es la de su proyecto es escribir a ciegas: lo
    // que cambia no está delante.
    if (all && what !== 'open') return;
    if (what === 'open' || what === 'thread') {
      open = bubble;
      naming = what === 'thread';
      drawer = true;
      return;
    }
    if (what === 'rename') {
      renaming = bubble;
      fresh = bubble.name;
      return;
    }
    if (what === 'close') {
      closing = bubble;
      closure = bubble.closure ?? '';
    }
  }

  function closeBubble() {
    const b = closing;
    closing = null;
    if (!b) return;
    write(() =>
      api.update('bubbles', b.id, {
        closed_at: new Date().toISOString().replace('T', ' '),
        closure: closure.trim(),
      }),
    );
  }

  /** Reopening clears both: `closed_at` because it is not closed any more, and
   *  the closure because it described an ending that no longer holds. */
  const reopen = (b: BubbleHeat) =>
    write(() => api.update('bubbles', b.id, { closed_at: '', closure: '' }));

  function rename() {
    const b = renaming;
    const name = fresh.trim();
    renaming = null;
    if (!b || !name || name === b.name) return;
    write(() => api.update('bubbles', b.id, { name }));
  }
</script>


{#if layout === 'flotation'}<Minimap items={mapItems} {scroller} />{/if}

{#if error}
  <p class="card glass m-6 p-4 text-sm text-error-500">{error}</p>
{/if}

{#if !board}
  <Loading />
{:else}
  <div class="model-board" class:canvas-mode={layout !== 'flotation'}>
    {#key level}
      {#if layout === 'flotation'}
        <BubbleBoard title={boardTitle} bubbles={visibleOrbs}
          onnew={all ? undefined : onnew}
          onaction={level === 'bubbles' ? act : undefined}
          onopen={(b) => {
            const node = universe.find((n) => n.id === b.id);
            if (node) openNode(node);
          }} />
      {:else if layout === '3d'}
        <div class="blob-board">
          <Graph3D nodes={shownNodes} {universe} groups={graphGroups} {links} kind={level} title={boardTitle}
            looks={encoded.looks}
            empty={`No hay ${level === 'bubbles' ? 'burbujas' : 'hilos'} en este alcance.`}
            resetKey={`${level}/${workspace?.id ?? 'all'}/${teams.current}/${teams.viewing}`}
            onopen={openNode} />
          <EncodingLegend legend={encoded.legend} colorLabel={COLOR_LABELS[encoding.color]} {filtering} />
        </div>
      {:else}
        <!-- Un lienzo que llena el pane, como la secuencia: el título flota arriba. -->
        <div class="blob-board">
          <BlobGraph nodes={shownNodes} {universe} groups={graphGroups} {links}
            kind={level} fury={false} canvas title={boardTitle} looks={encoded.looks}
            empty={`No hay ${level === 'bubbles' ? 'burbujas' : 'hilos'} en este alcance.`}
            resetKey={`${level}/${workspace?.id ?? 'all'}/${teams.current}/${teams.viewing}`}
            onopen={openNode} />
          <EncodingLegend legend={encoded.legend} colorLabel={COLOR_LABELS[encoding.color]} {filtering} />
        </div>
      {/if}
    {/key}
  </div>

  <!-- Los controles del tablero, abajo al centro y con la forma de los de la
       secuencia: qué se ve, cómo se acomoda y si entran las cerradas. Van en la
       franja del HUD, a la altura de las esquinas: montados en la columna del
       pane (como el cerebro), no dentro de él, para que no se muevan con el
       scroll ni los desvanezca la máscara del borde del pane. -->
  <span class="hud-anchor" aria-hidden="true" bind:this={hudAnchor}></span>
  {#snippet hud()}
  <div class="board-hud">
    <div class="seg" role="group" aria-label="Qué mostrar en el tablero">
      <button class:on={level === 'bubbles'} aria-pressed={level === 'bubbles'} onclick={() => changeLevel('bubbles')}>Burbujas</button>
      <button class:on={level === 'threads'} aria-pressed={level === 'threads'} onclick={() => changeLevel('threads')}>Hilos</button>
    </div>
    <div class="seg" role="group" aria-label="Disposición del tablero">
      <button class:on={layout === 'blobs'} aria-pressed={layout === 'blobs'} onclick={() => (layout = 'blobs')}>Blobs</button>
      <button class:on={layout === '3d'} aria-pressed={layout === '3d'} onclick={() => (layout = '3d')}>3D</button>
      <button class:on={layout === 'flotation'} aria-pressed={layout === 'flotation'} onclick={() => (layout = 'flotation')}>Flotabilidad</button>
    </div>
    <div class="seg">
      <button class:on={showClosed} aria-pressed={showClosed} onclick={() => (showClosed = !showClosed)}>Incluir cerradas</button>
    </div>
    {#if layout !== 'flotation'}
      <div class="seg">
        <button class:on={encOpen || filtering || !encoding.size.includes('uniform') || encoding.color !== 'state'} aria-expanded={encOpen}
          onclick={() => (encOpen = !encOpen)}>Tamaño y color{filtering ? ` · ${encoded.legend.filtered}` : ''}</button>
      </div>
    {/if}
    {#if encOpen && layout !== 'flotation'}
      <div class="enc-pop"><EncodingPanel bind:value={encoding} {level} onclose={() => (encOpen = false)} /></div>
    {/if}
  </div>
  {/snippet}
  {#if hudTarget}<Portal target={hudTarget}>{@render hud()}</Portal>{:else}{@render hud()}{/if}

  <BubbleDrawer
    bind:open={drawer}
    name={shown?.name ?? ''}
    project={projectOf(shown?.workspace ?? '') || workspace?.name || ''}
    lifecycle={shown?.heat.lifecycle ?? 'hot'}
    reason={shown?.heat.reason ?? ''}
    owners={shown?.owners ?? []}
    closed={shown?.closed ?? false}
    closure={shown?.closure ?? ''}
    people={roster}
    ondelete={canDelete ? () => removeBubble(shown) : undefined}
    threads={threadsOf(shown?.id ?? '').map((t) => ({
      seq: t.seq,
      title: t.name,
      lifecycle: t.heat.lifecycle,
      // The second line of the row, which is what the mock drew and the real
      // drawer had nothing to fill: who has it, and how long since anything
      // happened to it.
      owner: (t.assignees ?? []).map(nameOf).filter(Boolean).join(', '),
      age: ago(t.at ?? ''),
      priority: t.priority ?? '',
      step: t.step ?? 0,
    }))}
    cyclePct={cycle?.pct ?? null}
    cycleLeft={cycle?.words ?? ''}
    onopenthread={(seq) => {
      const t = threadsOf(open?.id ?? '').find((x) => x.seq === seq);
      // El cajón NO se cierra al salir: esta pantalla se va entera, y dejarlo
      // abierto es lo que hace que «dónde estaba» siga siendo verdad cuando se
      // vuelva.
      if (t) onOpen(t);
    }}
    onnewthread={all ? undefined : newThread}
    ondeletethread={all ? undefined : removeThread}
    onpriority={api.me?.role === 'lead'
      ? (seq, p) => {
          const t = threadsOf(shown?.id ?? '').find((x) => x.seq === seq);
          if (t) write(() => api.setThreadPriority(t.id, p));
        }
      : undefined}
    onowners={all
      ? undefined
      : (ids: string[]) => shown && write(() => api.update('bubbles', shown.id, { owners: ids }))}
    onclose={all
      ? undefined
      : () => shown && act('close', { id: shown.id, name: shown.name, life: shown.heat.lifecycle })}
    onreopen={all ? undefined : () => shown && reopen(shown)}
    bubble={shown?.id ?? ''}
    loadBrief={(id) => api.bubbleBrief(id)}
    // Con su workspace, para que las imágenes de `assets/` se resuelvan.
    renderBrief={(md) => api.renderMarkdown(md, shown?.workspace ?? '')}
    // Editar el plan es del lead global: es quien da forma a esa capa, y el
    // planeador ya se lo reserva a él.
    saveBrief={api.me?.role === 'lead'
      ? async (id, md) => {
          await api.update('bubbles', id, { brief: md });
        }
      : undefined}
    attachBrief={shown?.workspace
      ? (file) => api.uploadAsset(shown!.workspace, file)
      : undefined}
    bind:naming />
{/if}

<Confirm bind:ask={doom} />

{#if closing}
  <!-- Cerrar no borra: baja a la banda de cerradas con todo lo que tiene
       dentro, y reabrirla es una escritura. Lo que sí se pide es la frase. -->
  <Dialog open onOpenChange={() => (closing = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-md space-y-4 p-5 shadow-xl">
          <div>
            <Crumbs parts={[projectOf(closing.workspace) || workspace?.name || '']} />
            <Dialog.Title class="text-lg font-bold">¿Cerrar «{closing.name}»?</Dialog.Title>
          </div>
          <Dialog.Description class="muted text-sm">
            Baja a la banda de cerradas con sus hilos. No se borra nada, y se
            puede reabrir desde el mismo cajón.
          </Dialog.Description>
          <form onsubmit={(e) => { e.preventDefault(); closeBubble(); }}>
            <input
              class="input"
              placeholder="¿Cómo terminó? (opcional)"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={closure} {@attach limited('bubbles.closure')}
              {@attach (el: HTMLInputElement) => el.focus()} />
            <div class="mt-4 flex justify-end gap-2">
              <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (closing = null)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-error-500">Cerrar</button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

{#if renaming}
  <!-- A bubble is renamed in place: its outcome, its threads and its band are
       the same work under a better name. -->
  <Dialog open onOpenChange={() => (renaming = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-sm space-y-4 p-5 shadow-xl">
          <div>
            <Crumbs parts={[projectOf(renaming.workspace) || workspace?.name || '', renaming.name]} />
            <Dialog.Title class="text-lg font-bold">Renombrar la burbuja</Dialog.Title>
          </div>
          <form onsubmit={(e) => { e.preventDefault(); rename(); }}>
            <input
              class="input"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={fresh} {@attach limited('bubbles.name')}
              {@attach (el: HTMLInputElement) => el.select()} />
            <div class="mt-4 flex justify-end gap-2">
              <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (renaming = null)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-primary-500" disabled={!fresh.trim()}>Renombrar</button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

<style>
  .model-board { padding-bottom: 4.5rem; }
  /* En Blobs no hay página que desplazar: el lienzo es el pane entero. */
  .model-board.canvas-mode { height: 100%; padding-bottom: 0; }
  .blob-board { position: relative; height: 100%; }
  .enc-pop { position: absolute; bottom: calc(100% + 0.6rem); left: 50%; transform: translateX(-50%); }
  .hud-anchor { display: none; }
  /* La fila de las esquinas: `bottom: 1rem` y 42 px de alto, como .float-btn y
     .corner-dock; los grupos quedan centrados en ella. */
  .board-hud { position: absolute; left: 50%; bottom: 1rem; transform: translateX(-50%); min-height: 42px; display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 0.5rem; width: max-content; max-width: calc(100% - 2rem); z-index: var(--z-chrome); }
  /* Sin lugar al centro entre las esquinas: sube sobre ellas (igual que la secuencia). 1320 px: con el cerebro, la cabeza de la furia y el pin, las esquinas ya no dejan sitio a 1280. */
  @media (max-width: 1320px) { .board-hud { bottom: 4.75rem; } }
  .seg { display: flex; flex-wrap: wrap; max-width: 100%; padding: 3px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface-solid); }
  .seg button { padding: 0.3rem 0.7rem; border-radius: 7px; font-size: 0.85rem; }
  .seg button.on { background: var(--hover); font-weight: 700; }
  .seg button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
