// Tiny subsequence fuzzy matcher for client-side filtering (bubbles + commands).
// Returns a score (higher = better) or -1 for no match.
export function fuzzyScore(query: string, target: string): number {
  const q = query.toLowerCase();
  const t = target.toLowerCase();
  if (q === '') return 0;

  let score = 0;
  let ti = 0;
  let prevMatch = -2;
  for (let qi = 0; qi < q.length; qi++) {
    const ch = q[qi];
    const found = t.indexOf(ch, ti);
    if (found === -1) return -1;
    // reward consecutive matches and word-start matches
    if (found === prevMatch + 1) score += 6;
    if (found === 0 || t[found - 1] === ' ' || t[found - 1] === '-') score += 4;
    score += 1;
    prevMatch = found;
    ti = found + 1;
  }
  // prefer shorter targets and earlier first-hit
  score -= t.length * 0.05;
  return score;
}

export function fuzzyFilter<T>(
  query: string,
  items: T[],
  key: (item: T) => string,
): T[] {
  if (query.trim() === '') return items;
  return items
    .map((item) => ({ item, s: fuzzyScore(query, key(item)) }))
    .filter((r) => r.s >= 0)
    .sort((a, b) => b.s - a.s)
    .map((r) => r.item);
}
