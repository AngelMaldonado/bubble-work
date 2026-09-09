/**
 * The URL is the state, not a copy of it.
 *
 * Until now a view was a variable: opening a thread set `open`, and reloading
 * put you back on the board because the variable was gone and the address had
 * never said otherwise. That is fine for a mock and wrong for a product — a
 * screen you cannot link to is a screen you cannot send anybody.
 *
 * The shape reads as what it is:
 *
 *     /todos                       every workspace's bubbles on one board
 *     /w/alpha                     the board of a workspace
 *     /w/alpha/t/14                thread #14 of that workspace
 *     /w/alpha/wiki/docs/adr.md    a page in its wiki
 *     /planeador                   the department's planner — no workspace
 *     /theme, /theme/mock          the design system, and the whole product mocked
 *
 * The slug and the seq, not ids: both are what people already say out loud
 * ("alpha, catorce"), both are stable, and an id in an address is a string
 * nobody can check against what they are looking at.
 */
export type Route =
  | { kind: 'board'; slug?: string }
  | { kind: 'all' }
  | { kind: 'thread'; slug: string; seq: number }
  | { kind: 'wiki'; slug: string; page: string }
  | { kind: 'planner' }
  | { kind: 'theme' }
  | { kind: 'mock' };

export function parse(pathname: string): Route {
  const p = pathname.replace(/\/+$/, '') || '/';
  if (p === '/theme') return { kind: 'theme' };
  if (p === '/theme/mock') return { kind: 'mock' };
  if (p === '/planeador') return { kind: 'planner' };
  // Todos es una DIRECCIÓN y no un filtro guardado: mirar todos los proyectos a
  // la vez es una pantalla, y una pantalla que no se puede enlazar es una
  // pantalla que no se puede mandar a nadie.
  if (p === '/todos') return { kind: 'all' };

  const m = p.match(/^\/w\/([^/]+)(?:\/(t|wiki)(?:\/(.*))?)?$/);
  if (!m) return { kind: 'board' };
  const [, slug, what, rest = ''] = m;
  if (what === 't' && /^\d+$/.test(rest)) return { kind: 'thread', slug, seq: Number(rest) };
  // The wiki's rest IS a path, slashes and all, so it is decoded whole rather
  // than split: `docs/arquitectura/overview.md` is one thing, not three.
  if (what === 'wiki') return { kind: 'wiki', slug, page: decodeURIComponent(rest) || 'README.md' };
  return { kind: 'board', slug };
}

export const boardUrl = (slug: string) => `/w/${slug}`;
export const allUrl = '/todos';
export const threadUrl = (slug: string, seq: number) => `/w/${slug}/t/${seq}`;
export const wikiUrl = (slug: string, page: string) =>
  `/w/${slug}/wiki/${page.split('/').map(encodeURIComponent).join('/')}`;
export const plannerUrl = '/planeador';
