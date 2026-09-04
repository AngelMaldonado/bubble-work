<script lang="ts" module>
  import type { Component } from 'svelte';

  export type TreeNode = {
    id: string;
    name: string;
    /** A lucide component, not an emoji: the emoji alphabet belongs to the
        lifecycle, and spending it on furniture makes the signal harder to see. */
    icon?: Component<{ class?: string }>;
    /** Shown right-aligned on the row: a relation type, a count, a hint. */
    badge?: string;
    href?: string;
    external?: boolean;
    /** Overrides the child count on a branch — a section with an add form still
        reports how many real things are in it. */
    count?: number;
    children?: TreeNode[];
  };
</script>

<script lang="ts">
  // The sidebar's contents, as a real tree.
  //
  // Skeleton's TreeView over Zag: arrow keys, typeahead, expand/collapse state
  // and the aria wiring come with it. A list of hand-rolled buttons looks the
  // same and is not the same — a keyboard reaches every node here, and the same
  // component will render the workspace's docs/ wiki without a second design.
  import { TreeView } from '@skeletonlabs/skeleton-svelte';
  import { collection as treeCollection } from '@zag-js/tree-view';
  import type { Snippet } from 'svelte';

  let {
    nodes,
    onselect,
    branchActions,
    itemActions,
  }: {
    nodes: TreeNode[];
    onselect?: (node: TreeNode) => void;
    /** Rendered at the right of a branch's row — the section's "+" button. */
    branchActions?: Snippet<[TreeNode]>;
    /** Rendered at the right of a leaf's row — the row's "×" button. */
    itemActions?: Snippet<[TreeNode]>;
  } = $props();

  // `nodeToValue` is not optional in practice: the default reads `node.value`,
  // which none of these have, so every node would answer '' and share one
  // expansion and one selection with all the others.
  const coll = $derived(
    treeCollection<TreeNode>({
      nodeToValue: (n) => n.id,
      nodeToString: (n) => n.name,
      rootNode: { id: 'ROOT', name: '', children: nodes },
    }),
  );

  // Every branch open unless it was closed BY HAND. Zag's `defaultExpandedValue`
  // is read once, so branches that appear later — the table of contents, which
  // only exists after the document renders — would arrive collapsed. Holding the
  // closed set instead of the open one keeps a growing tree open.
  let closed = $state<string[]>([]);
  const branches = $derived(collectBranches(nodes));
  const expanded = $derived(branches.filter((id) => !closed.includes(id)));
  function collectBranches(list: TreeNode[]): string[] {
    return list.flatMap((n) => (n.children?.length ? [n.id, ...collectBranches(n.children)] : []));
  }

  function open(node: TreeNode) {
    if (node.href && node.external) window.open(node.href, '_blank', 'noopener');
    else if (node.href) location.hash = node.href.replace(/^#/, '');
    onselect?.(node);
  }
</script>

{#snippet treeNode({ node, indexPath }: { node: TreeNode; indexPath: number[] })}
  <TreeView.NodeProvider value={{ node, indexPath }}>
    {#if node.children?.length}
      <TreeView.Branch>
        <TreeView.BranchControl>
          <TreeView.BranchIndicator>›</TreeView.BranchIndicator>
          <TreeView.BranchText>
            {#if node.icon}{@const Icon = node.icon}<Icon class="size-4 shrink-0" />{/if}
            <span class="node-name">{node.name}</span>
          </TreeView.BranchText>
          <!-- The count lives on the branch, so a collapsed section still says
               how much is inside it. Collapsing hides detail, not information. -->
          <span class="count">{node.count ?? node.children.length}</span>
          {#if branchActions}
            <!-- The section's own button must not toggle the section. -->
            <span class="acts" role="none" onclick={(e) => e.stopPropagation()}>
              {@render branchActions(node)}
            </span>
          {/if}
        </TreeView.BranchControl>
        <TreeView.BranchContent>
          <TreeView.BranchIndentGuide />
          {#each node.children as child, i (child.id)}
            {@render treeNode({ node: child, indexPath: [...indexPath, i] })}
          {/each}
        </TreeView.BranchContent>
      </TreeView.Branch>
    {:else}
      <div class="row">
        <TreeView.Item onclick={() => open(node)} title={node.href ?? node.name}>
          {#if node.icon}{@const Icon = node.icon}<Icon class="size-4 shrink-0" />{/if}
          <span class="node-name">{node.name}</span>
          {#if node.badge}<span class="badge">{node.badge}</span>{/if}
        </TreeView.Item>
        {#if itemActions}
          <span class="acts">{@render itemActions(node)}</span>
        {/if}
      </div>
    {/if}
  </TreeView.NodeProvider>
{/snippet}

<div class="tree">
<TreeView
  collection={coll}
  expandedValue={expanded}
  onExpandedChange={(e: { expandedValue: string[] }) =>
    (closed = branches.filter((id) => !e.expandedValue.includes(id)))}>
  <TreeView.Tree>
    {#each nodes as node, i (node.id)}
      {@render treeNode({ node, indexPath: [i] })}
    {/each}
  </TreeView.Tree>
</TreeView>
</div>

<style>
  /* Skeleton ships the whole tree in `skeleton-common`, keyed on `--depth`,
     which Zag sets per node: indentation, the absolutely-positioned indent
     guide, the chevron's rotation, and hover/selected via preset-tonal /
     preset-filled — which read OUR palette, so there is nothing to re-theme.
     Overriding any of it is how the guide became one long line down the page.
     What is left here is only what Skeleton has no opinion about: the count, a
     row's own buttons, and a long name ending in an ellipsis. */
  /* Every box between the row and the name has to be allowed to shrink, or the
     ellipsis never happens: a flex item's default `min-width: auto` is its
     content, so one un-shrinkable ancestor is enough to push the name past the
     panel's edge and have `overflow-x: hidden` cut it mid-word. */
  .tree :global([data-part='item']),
  .tree :global([data-part='branch-control']),
  .tree :global([data-part='branch-text']) { min-width: 0; }
  .tree :global([data-part='branch-text']) { flex: 1; }
  /* Not `.label`: Skeleton owns that class (`width: 100%; display: block`). */
  .node-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badge, .count {
    margin-left: auto; flex: none;
    font-size: 0.68rem; color: var(--faint);
    background: var(--hover); border-radius: 999px; padding: 0 0.4rem;
  }
  /* The row's buttons sit outside the Item, so a click on them is not a click
     on the node. */
  .row { display: flex; align-items: center; }
  .row :global([data-part='item']) { flex: 1; min-width: 0; }
  .acts { flex: none; display: flex; align-items: center; }
</style>
