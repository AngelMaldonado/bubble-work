/** File ids only, scoped to the signed-in reader and the tree's owner.
 * Available ids always come from the current server response, not storage. */
export function readTreeSelection(user: string, scope: string, available: readonly string[]): string {
  const first = available[0] ?? '';
  try {
    const saved = localStorage.getItem(key(user, scope));
    return saved && available.includes(saved) ? saved : first;
  } catch { return first; }
}

export function saveTreeSelection(user: string, scope: string, selected: string): void {
  if (!selected) return;
  try { localStorage.setItem(key(user, scope), selected); } catch { /* quota/privacy mode */ }
}

function key(user: string, scope: string): string {
  return `bubble.tree.selected.${user}.${scope}`;
}
