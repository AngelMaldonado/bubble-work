<script lang="ts">
  // El tablero de /theme/mock con el modelo propuesto encima: pestañas Burbujas /
  // Hilos y disposición Blobs / Flotabilidad. Sólo en desarrollo (MockPage lo
  // carga bajo import.meta.env.DEV); los datos son los inventados de lib/mock.ts.
  import { Dialog, Portal, Tabs } from '@skeletonlabs/skeleton-svelte';
  import BubbleBoard, { type BoardBubble } from '../components/BubbleBoard.svelte';
  import Icon from '../components/Icon.svelte';
  import BlobGraph from '../components/BlobGraph.svelte';
  import { bandFace, bandName } from '../lib/bands';
  import { levels, type MapKind, type ModelNode } from './modelFixtures';
  import { mockBubbleNodes, mockGroups, mockLinks, mockThreadNodes } from './mockModel';

  type Layout = 'blobs' | 'flotation';
  let { title, orbs, layout = $bindable<Layout>('flotation'), level = $bindable<MapKind>('bubbles'), onopenbubble, onopenthread }: {
    title: string; orbs: BoardBubble[]; layout?: Layout; level?: MapKind;
    onopenbubble: (name: string) => void; onopenthread: (thread: ModelNode) => void;
  } = $props();

  const KEY = 'bubble.mock.model.';
  try {
    if (localStorage.getItem(KEY + 'layout') === 'blobs') layout = 'blobs';
    if (localStorage.getItem(KEY + 'level') === 'threads') level = 'threads';
  } catch { /* storage opcional */ }
  $effect(() => { try { localStorage.setItem(KEY + 'layout', layout); localStorage.setItem(KEY + 'level', level); } catch { /* storage opcional */ } });

  let explained = $state<ModelNode | null>(null);
  const universe = $derived(level === 'bubbles' ? mockBubbleNodes : mockThreadNodes);
  const visible = $derived(universe.filter((n) => n.life !== 'closed'));
  const links = $derived(mockLinks(level, visible));
  // Flotabilidad de hilos: el mismo tablero de orbes, con la burbuja como origen.
  const threadOrbs = $derived<BoardBubble[]>(mockThreadNodes.map((t) => ({
    id: t.id, name: t.name, life: t.life, priority: t.priority, project: t.bubbleName,
  })));
  function open(node: ModelNode) {
    if (node.kind === 'bubbles') onopenbubble(node.name);
    else onopenthread(node);
  }
</script>

<div class="mock-model">
  <Tabs value={level} onValueChange={(e) => { if (e.value === 'bubbles' || e.value === 'threads') level = e.value; }}>
    <Tabs.List aria-label="Qué mostrar en el tablero">
      <Tabs.Trigger value="bubbles">Burbujas</Tabs.Trigger>
      <Tabs.Trigger value="threads">Hilos</Tabs.Trigger>
    </Tabs.List>
    {#each ['bubbles', 'threads'] as tab (tab)}
      <Tabs.Content value={tab}>
        {#if level === tab}
          {#if layout === 'flotation'}
            <BubbleBoard {title} bubbles={level === 'bubbles' ? orbs : threadOrbs}
              onopen={(b) => level === 'bubbles' ? onopenbubble(b.name) : open(mockThreadNodes.find((t) => t.id === b.id)!)} />
          {:else}
            <div class="blobs">
              <header class="board-head"><h1 class="display text-2xl">{title}</h1></header>
              <BlobGraph nodes={visible} {universe} groups={mockGroups} {links} kind={level}
                resetKey={level} height="max(28rem, calc(100dvh - 15rem))" onopen={open} onfury={(n) => explained = n} />
            </div>
          {/if}
        {/if}
      </Tabs.Content>
    {/each}
  </Tabs>
</div>

<nav class="view-toggle" aria-label="Disposición del tablero">
  <button class:active={layout === 'blobs'} aria-pressed={layout === 'blobs'} onclick={() => layout = 'blobs'}>Blobs</button>
  <button class:active={layout === 'flotation'} aria-pressed={layout === 'flotation'} onclick={() => layout = 'flotation'}>Flotabilidad</button>
</nav>

<Dialog open={!!explained} onOpenChange={(e) => { if (!e.open) explained = null; }}>
  <Portal>
    <Dialog.Backdrop class="mock-model-scrim" />
    <Dialog.Positioner class="mock-model-pos">
      <Dialog.Content class="mock-model-detail card bg-surface-100-900">
        <div class="detail-head"><Dialog.Title>{explained?.name}</Dialog.Title><Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface" aria-label="Cerrar explicación">✕</Dialog.CloseTrigger></div>
        {#if explained}
          <Dialog.Description>{explained.kind === 'threads' ? `Hilo de ${explained.bubbleName}` : 'Burbuja'} · {explained.priority}</Dialog.Description>
          <p class="heat"><Icon of={bandFace(explained.life)} tint /> {bandName(explained.life)} — evidencia, no urgencia.</p>
          <p><strong>{levels[explained.level]}</strong> — el máximo de las causas, no su suma.</p>
          <dl><dt>Compromiso</dt><dd>{explained.deadline}</dd><dt>Estancamiento</dt><dd>{explained.stagnation}</dd></dl>
          <button class="btn preset-filled-primary-500" onclick={() => { const n = explained!; explained = null; open(n); }}>Abrir {explained.kind === 'threads' ? 'hilo' : 'burbuja'}</button>
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  .mock-model { max-width: 1400px; margin: 0 auto; }
  .mock-model :global([data-scope='tabs'][data-part='list']) { justify-content: center; padding-top: 1rem; }
  .mock-model :global([data-scope='tabs'][data-part='trigger']) { color: var(--muted); }
  .mock-model :global([data-scope='tabs'][data-part='trigger'][aria-selected='true']) { color: var(--text); box-shadow: inset 0 -2px 0 var(--color-primary-500); }
  .blobs { padding: 2rem 1.5rem 5rem; }
  .board-head { text-align: center; margin-bottom: 0.5rem; }
  .view-toggle { position: fixed; bottom: 1rem; left: 50%; transform: translateX(-50%); display: flex; gap: 0.4rem; z-index: var(--z-chrome); }
  .view-toggle button { border: 1px solid var(--line); padding: 0.6rem 1rem; border-radius: 0.6rem; background: var(--surface-solid); cursor: pointer; font-size: 0.85rem; }
  .view-toggle button.active { color: var(--color-primary-500); border-color: var(--color-primary-500); }
  .view-toggle button:focus-visible { outline: 2px solid var(--color-primary-500); outline-offset: 3px; }
  :global(.mock-model-scrim) { position: fixed; inset: 0; background: var(--scrim); z-index: 100; }
  :global(.mock-model-pos) { position: fixed; inset: 0; display: flex; align-items: center; justify-content: center; padding: 1rem; z-index: 101; }
  :global(.mock-model-detail) { width: min(32rem, 100%); padding: 1.5rem; display: flex; flex-direction: column; gap: 0.9rem; }
  .detail-head { display: flex; justify-content: space-between; gap: 1rem; align-items: center; }
  .detail-head :global([data-part='title']) { font-family: var(--font-display); font-size: 1.5rem; }
  .heat { display: flex; gap: 0.4rem; align-items: center; font-size: 0.85rem; }
  dt { font-weight: 600; margin-top: 0.6rem; }
  dd { color: var(--faint); font-size: 0.9rem; }
</style>
