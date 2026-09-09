<script lang="ts">
  // The wiki, against the real repository.
  //
  // Same split as `Thread.svelte`: `WikiView` draws, this holds the tree, the
  // page and what a click on a file means. The two containers exist so the
  // views stay drawable with invented data in /theme/mock.
  import { ApiError, api, type Entry, type Workspace } from '../lib/api';
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import type { TreeNode } from './SideTree.svelte';
  import FileTextIcon from '@lucide/svelte/icons/file-text';
  import FolderIcon from '@lucide/svelte/icons/folder';
  import WikiView from './WikiView.svelte';

  let {
    workspace,
    path = 'README.md',
    onback,
    onopen,
    onsearch,
  }: {
    workspace: Workspace;
    path?: string;
    onback?: () => void;
    /** which page to go to. The caller owns it, because the page is in the URL */
    onopen?: (page: string) => void;
    onsearch?: () => void;
  } = $props();

  let entries = $state<Entry[]>([]);
  let html = $state('');
  let error = $state('');
  // The page's markdown and the hash it was READ at. Same contract as a
  // thread's document, because it is the same file in the same repository:
  // every write carries the hash it read, and a 409 means somebody else wrote
  // first rather than a save that quietly wins.
  let content = $state('');
  let hash = $state('');
  let naming = $state(false);
  let fresh = $state('');
  let doomed = $state(false);

  // The tree comes back FLAT — a list of paths, which is what the filesystem
  // has. The nesting is derived here rather than on the server for the same
  // reason the heading rail nests in the browser: it is a way of drawing a list,
  // not a fact about it.
  const nodes = $derived.by((): TreeNode[] => {
    const roots: TreeNode[] = [];
    const dirs = new Map<string, TreeNode>();
    // Only what a person reads HERE. Images live in the repo too, and a wiki
    // that lists its own attachments is a file browser wearing a wiki's
    // clothes; a thread's document lives there too, and it is reached through
    // the thread — where its band, its evidence and its verbs are.
    const pages = entries.filter((e) => !e.dir && e.area !== 'asset' && e.area !== 'thread');
    for (const e of pages) {
      const parts = e.path.split('/');
      let list = roots;
      let at = '';
      for (const dir of parts.slice(0, -1)) {
        at = at ? at + '/' + dir : dir;
        let node = dirs.get(at);
        if (!node) {
          node = { id: 'd:' + at, name: dir, icon: FolderIcon, children: [] };
          dirs.set(at, node);
          list.push(node);
        }
        list = node.children!;
      }
      list.push({ id: e.path, name: e.title || e.name, icon: FileTextIcon });
    }
    return roots;
  });

  async function loadTree() {
    try {
      entries = (await api.tree(workspace.id)).entries;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function loadPage() {
    try {
      const doc = await api.readPath(workspace.id, path);
      html = doc.html;
      content = doc.content;
      hash = doc.hash;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function save(markdown: string) {
    try {
      const out = await api.patchPath(workspace.id, path, { base: hash, content: markdown });
      html = out.html;
      content = out.content;
      hash = out.hash;
      error = '';
      await loadTree(); // a page that did not exist does now
    } catch (e) {
      error =
        e instanceof ApiError && e.conflict
          ? 'Esta página cambió en otro lado, así que tu escritura no se guardó. Recárgala para ver lo que hay.'
          : (e as Error).message;
    }
  }

  /** A new page is a WRITE to a path that does not exist yet — there is no
   *  "create" anywhere in this model. The hash of nothing is what says "I know
   *  it is not there", and the server derives it. */
  async function create() {
    const name = fresh.trim();
    naming = false;
    fresh = '';
    if (!name) return;
    const file = (name.endsWith('.md') ? name : `${name}.md`).replace(/^\/+/, '');
    const page = file.startsWith('docs/') || file === 'README.md' ? file : `docs/${file}`;
    try {
      const empty = await api.readPath(workspace.id, page);
      await api.patchPath(workspace.id, page, {
        base: empty.hash,
        content: `# ${name.replace(/\.md$/, '')}\n\n`,
        message: `born: ${page}`,
      });
      await loadTree();
      onopen?.(page);
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function remove() {
    doomed = false;
    try {
      await api.removePath(workspace.id, path);
      await loadTree();
      // Back to the README: the page that was on screen is gone, and leaving
      // its address up would show an error where a document used to be.
      onopen?.('README.md');
    } catch (e) {
      error = (e as Error).message;
    }
  }

  $effect(() => {
    workspace.id;
    loadTree();
  });
  $effect(() => {
    workspace.id;
    path;
    loadPage();
  });
</script>

{#if error}
  <p class="failed" role="alert">
    {error}
    <button onclick={() => (error = '')} aria-label="cerrar">×</button>
  </p>
{/if}

<WikiView
  workspace={workspace.name}
  {path}
  {html}
  markdown={content}
  tree={nodes}
  {onback}
  {onsearch}
  onsave={save}
  onnew={() => (naming = true)}
  ondelete={() => (doomed = true)}
  onopen={(id) => {
    // A folder's row is not a page. Zag reports the selection either way, so
    // the filter lives here rather than in the tree.
    if (!id.startsWith('d:')) onopen?.(id);
  }} />

{#if naming}
  <Dialog open onOpenChange={() => (naming = false)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-sm space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">Nueva página</Dialog.Title>
          <Dialog.Description class="muted text-sm">
            Va bajo <code>docs/</code>. Puedes anidarla escribiendo la ruta:
            <code>arquitectura/decisiones.md</code>.
          </Dialog.Description>
          <form onsubmit={(e) => { e.preventDefault(); create(); }}>
            <input
              class="input"
              placeholder="¿Cómo se llama?"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={fresh}
              {@attach (el: HTMLInputElement) => el.focus()} />
            <div class="mt-4 flex justify-end gap-2">
              <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (naming = false)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-primary-500" disabled={!fresh.trim()}>Crear</button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

{#if doomed}
  <Dialog open onOpenChange={() => (doomed = false)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-center justify-center p-4"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-md space-y-4 p-5 shadow-xl">
          <Dialog.Title class="text-lg font-bold">¿Borrar «{path}»?</Dialog.Title>
          <Dialog.Description class="muted text-sm">
            Sale del árbol y del repositorio. Lo escrito sigue en la historia de
            git, que es de donde se recupera si hacía falta.
          </Dialog.Description>
          <div class="flex justify-end gap-2">
            <button class="btn btn-sm preset-tonal-surface" onclick={() => (doomed = false)}>Cancelar</button>
            <button class="btn btn-sm preset-filled-error-500" onclick={remove}>Borrar</button>
          </div>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

<style>
  .failed {
    position: fixed;
    top: 1rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: var(--z-toast);
    display: flex;
    align-items: center;
    gap: 0.75rem;
    max-width: min(92vw, 560px);
    margin: 0;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--color-error-500);
    border-radius: 12px;
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.85rem;
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.22);
  }
  .failed button { color: var(--faint); font-size: 1rem; line-height: 1; }
</style>
