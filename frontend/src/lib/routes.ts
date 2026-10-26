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
 *     /secuencia                   the execution sequence, as the viewer sees it
 *     /prompts/agents/operators    what the AI is told: AGENTS.md, the system prompts, the tools
 *     /ajustes/tokens              settings, one tab per address
 *     /artefacto/<id>              an ephemeral artifact (a grill), where it is answered
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
  | { kind: 'planner'; inboxOnly?: boolean }
  | { kind: 'inventory' }
  | { kind: 'sequence' }
  | { kind: 'roadmap' }
  | { kind: 'prompts'; key: string }
  | { kind: 'settings'; tab: string }
  | { kind: 'artifact'; id: string }
  | { kind: 'space' }
  | { kind: 'theme' }
  | { kind: 'mock' };

export function parse(pathname: string): Route {
  const p = pathname.replace(/\/+$/, '') || '/';
  if (p === '/theme') return { kind: 'theme' };
  if (p === '/theme/mock') return { kind: 'mock' };
  if (p === '/planeador') return { kind: 'planner' };
  if (p === '/planeador/inbox') return { kind: 'planner', inboxOnly: true };
  if (p === '/inventario') return { kind: 'inventory' };
  if (p === '/secuencia') return { kind: 'sequence' };
  if (p === '/roadmap') return { kind: 'roadmap' };
  if (p === '/mi-espacio') return { kind: 'space' };
  // Un artefacto efímero por su id: no tiene número ni nombre estable, y es el
  // enlace que el agente le da a la persona para contestar.
  const art = p.match(/^\/artefacto\/([a-z0-9]+)$/);
  if (art) return { kind: 'artifact', id: art[1] };
  // Los dos AGENTS.md del departamento, uno por rol.
  // Todo lo que se le dice a la IA. `/agentes/<rol>` era la pantalla de un
  // AGENTS.md; sigue abriendo el mismo documento, ahora dentro de Prompts.
  const agents = p.match(/^\/agentes\/(planner|operators)$/);
  if (agents) return { kind: 'prompts', key: `agents/${agents[1]}` };
  const pr = p.match(/^\/prompts(?:\/(.+))?$/);
  if (pr) return { kind: 'prompts', key: pr[1] ? decodeURIComponent(pr[1]) : '' };
  // La tab va en la dirección: «mira tus tokens» es un enlace, no unas
  // instrucciones. Vacía, la pantalla elige la primera que te corresponde.
  const set = p.match(/^\/ajustes(?:\/([a-z-]+))?$/);
  if (set) return { kind: 'settings', tab: set[1] ?? '' };
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
  if (what === 'wiki') return { kind: 'wiki', slug, page: decodeURIComponent(rest) };
  return { kind: 'board', slug };
}

export const boardUrl = (slug: string) => `/w/${slug}`;
export const allUrl = '/todos';
export const threadUrl = (slug: string, seq: number) => `/w/${slug}/t/${seq}`;
export const wikiUrl = (slug: string, page = '') =>
  `/w/${slug}/wiki${page ? '/' + page.split('/').map(encodeURIComponent).join('/') : ''}`;
export const plannerUrl = '/planeador';
export const readonlyInboxUrl = '/planeador/inbox';
export const inventoryUrl = '/inventario';
export const sequenceUrl = '/secuencia';
/** El calendario como un camino: el pasado arriba, el futuro abajo. */
export const roadmapUrl = '/roadmap';
export const spaceUrl = '/mi-espacio';
export const artifactUrl = (id: string) => `/artefacto/${id}`;
export const promptsUrl = (key = '') => (key ? `/prompts/${key}` : '/prompts');
export const settingsUrl = (tab = '') => (tab ? `/ajustes/${tab}` : '/ajustes');
