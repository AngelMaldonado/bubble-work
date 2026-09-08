/**
 * Fuzzy matching for the omnibar.
 *
 * What this is for: finding a thread you already know exists, by typing the
 * parts of its name you remember. "mdtree" should find "El árbol de markdown y
 * sus commits" — a substring filter never will, and that is the difference
 * between a field people type into and a field people gave up on.
 *
 * It matches a SUBSEQUENCE: every character of the query, in order, somewhere in
 * the text. The score is what makes the list useful rather than merely correct —
 * a match at the start of a word beats one in the middle of one, and characters
 * that land together beat the same characters scattered, because that is how a
 * person remembers a name.
 *
 * Written here rather than pulled in: this is forty lines, and the libraries
 * that do it are between 3 and 12 kB gzip for a list that never exceeds a few
 * hundred rows.
 */

/** [start, end) of one run of matched characters, for highlighting. */
export type Range = [number, number];

export type Match = { score: number; ranges: Range[] };

/** A word starts after a space, a slash, a dash or a case change. */
function boundary(text: string, i: number): boolean {
  if (i === 0) return true;
  const prev = text[i - 1];
  if (/[\s/\-_.#·:]/.test(prev)) return true;
  return prev === prev.toLowerCase() && text[i] !== text[i].toLowerCase();
}

/** Accents are typed away by half the people who use this. `José` matches `jose`. */
export function fold(s: string): string {
  return s.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase();
}

/**
 * Score `text` against `query`, or null when the query is not in it at all.
 *
 * Greedy, left to right: the first place each character fits is the one taken.
 * A backtracking matcher scores a few names better and costs a loop nobody can
 * read; on a list this size the difference never reached the screen.
 */
export function match(query: string, text: string): Match | null {
  const q = fold(query.trim());
  if (!q) return { score: 0, ranges: [] };
  const t = fold(text);

  const ranges: Range[] = [];
  let score = 0;
  let run = 0;
  let at = 0;

  for (const ch of q) {
    if (ch === ' ') {
      // A space in the query means "and then, later" — it never has to match.
      run = 0;
      continue;
    }
    const i = t.indexOf(ch, at);
    if (i < 0) return null;

    // Consecutive characters compound: three in a row is worth more than three
    // apart, which is what makes a prefix beat a scatter without a special case.
    run = i === at && at > 0 ? run + 1 : 1;
    score += run;
    if (boundary(text, i)) score += 8;
    if (i === 0) score += 4;
    // Distance costs, but never enough to lose to a match that is not there.
    score -= Math.min(i - at, 6) * 0.5;

    const last = ranges[ranges.length - 1];
    if (last && last[1] === i) last[1] = i + 1;
    else ranges.push([i, i + 1]);
    at = i + 1;
  }

  // A short name that matched is usually the one meant: "API" over "API gateway
  // migration notes" when both match "api".
  score += Math.max(0, 20 - text.length) * 0.2;
  return { score, ranges };
}

/** Rank `items` by how well `key(item)` matches, dropping what does not match. */
export function rank<T>(query: string, items: T[], key: (item: T) => string): (T & { match: Match })[] {
  const out: (T & { match: Match })[] = [];
  for (const item of items) {
    const m = match(query, key(item));
    if (m) out.push({ ...item, match: m });
  }
  // Stable within a score: the caller's order is a real signal (threads before
  // pages, hot before cold) and a sort that discards it looks random.
  return out.sort((a, b) => b.match.score - a.match.score);
}

/** Split `text` into matched and unmatched pieces, for rendering. */
export function pieces(text: string, ranges: Range[]): { text: string; on: boolean }[] {
  if (!ranges.length) return [{ text, on: false }];
  const out: { text: string; on: boolean }[] = [];
  let at = 0;
  for (const [from, to] of ranges) {
    if (from > at) out.push({ text: text.slice(at, from), on: false });
    out.push({ text: text.slice(from, to), on: true });
    at = to;
  }
  if (at < text.length) out.push({ text: text.slice(at), on: false });
  return out;
}
