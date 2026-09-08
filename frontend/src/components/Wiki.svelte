<script lang="ts">
  // The wiki, against the real repository.
  //
  // Same split as `Thread.svelte`: `WikiView` draws, this holds the tree, the
  // page and what a click on a file means. The two containers exist so the
  // views stay drawable with invented data in /theme/mock.
  import { api, type Entry, type Workspace } from '../lib/api';
  import type { TreeNode } from './SideTree.svelte';
  import FileTextIcon from '@lucide/svelte/icons/file-text';
  import FolderIcon from '@lucide/svelte/icons/folder';
  import WikiView from './WikiView.svelte';

  let {
    workspace,
    path = $bindable('README.md'),
    onback,
    onsearch,
  }: {
    workspace: Workspace;
    path?: string;
    onback?: () => void;
    onsearch?: () => void;
  } = $props();

  let entries = $state<Entry[]>([]);
  let html = $state('');
  let error = $state('');

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
      html = (await api.readPath(workspace.id, path)).html;
      error = '';
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
  <p class="card glass m-6 p-4 text-sm text-error-500">{error}</p>
{:else}
  <WikiView
    workspace={workspace.name}
    {path}
    {html}
    tree={nodes}
    {onback}
    {onsearch}
    onopen={(id) => {
      // A folder's row is not a page. Zag reports the selection either way, so
      // the filter lives here rather than in the tree.
      if (!id.startsWith('d:')) path = id;
    }} />
{/if}
