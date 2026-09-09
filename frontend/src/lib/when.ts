// How long ago, in words, and how much of a window is left.
//
// The server sends instants, never phrases: "hace 2 d" is a sentence in a
// language, and the API does not have one. It lives here rather than in the
// component that draws it because the drawer, the board and the thread all say
// the same thing and should say it the same way.

/** PocketBase writes a date as `2026-09-09 15:05:07.654Z` and our own routes as
 *  RFC3339 with a `T`. Both are the same instant; only one of them parses in
 *  every browser, so the space becomes a `T` before anybody reads it. */
export function when(iso: string): number {
  if (!iso) return NaN;
  const s = iso.replace(' ', 'T');
  return Date.parse(/[Zz]|[+-]\d\d:?\d\d$/.test(s) ? s : s + 'Z');
}

/** "ahora", "hace 3 h", "hace 12 d". Coarse on purpose: the question a row
 *  answers is whether this is stale, and minutes past the first hour are noise. */
export function ago(iso: string, now = Date.now()): string {
  if (!iso) return '';
  const then = when(iso);
  if (Number.isNaN(then)) return '';
  const mins = Math.floor((now - then) / 60000);
  if (mins < 2) return 'ahora';
  if (mins < 60) return `hace ${mins} min`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `hace ${hours} h`;
  const days = Math.floor(hours / 24);
  if (days < 60) return `hace ${days} d`;
  return `hace ${Math.floor(days / 30)} meses`;
}

/** What is LEFT of the cycle since something last produced, as 0..100 and as a
 *  phrase. `null` when nothing has produced yet: a bar filled with a number
 *  nobody computed is worse than no bar, which is what the drawer already says.
 */
export function cycleLeft(
  warmAt: string,
  cycleHours: number,
  now = Date.now(),
): { pct: number; words: string } | null {
  if (!warmAt || !cycleHours) return null;
  const then = when(warmAt);
  if (Number.isNaN(then)) return null;
  const spentHours = (now - then) / 3600000;
  const leftHours = cycleHours - spentHours;
  const pct = Math.max(0, Math.min(100, Math.round((leftHours / cycleHours) * 100)));
  if (leftHours <= 0) return { pct: 0, words: 'el ciclo se agotó' };
  if (leftHours < 48) return { pct, words: `quedan ${Math.max(1, Math.round(leftHours))} h` };
  return { pct, words: `quedan ${Math.round(leftHours / 24)} d` };
}
