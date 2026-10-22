// Del tablero que manda el servidor a los puntos de la vista Blobs.
//
// Puro, para fijarlo con pruebas. Los blobs son TAGS: los del frontmatter del
// documento de cada hilo, que escriben la persona y su IA. No hay catálogo que
// mantener.
// - Hilos: un punto por hilo, en el blob de su PRIMER tag (el que su autor
//   puso primero). Sin tags, «Sin tags».
// - Burbujas: un punto por burbuja, en el blob que más se repite como primer
//   tag entre sus hilos (empate: el primero por orden alfabético). Sin hilos
//   con tags, «Sin tags». La burbuja no tiene documento propio en git.
// - Un tag es una palabra, no un registro con dueño: en «Todos los equipos»
//   el mismo tag de dos equipos es el mismo blob. Las aristas siguen sin
//   cruzar equipos.
// - Furia: no hay evaluador todavía, así que todo punto es `calm`. Nunca se
//   deduce del calor.
import type { Board } from './api';
import type { GraphGroup, GraphLink, MapKind } from './graph';
import type { Encodable } from './encoding';

export type BoardGraph = {
  bubbles: Encodable[];
  threads: Encodable[];
  /** las relaciones del servidor, como aristas entre hilos */
  relations: GraphLink[];
  /** los blobs, del más poblado al menos, y «Sin tags» al final */
  groups: GraphGroup[];
};

/** El blob de una burbuja: el primer tag más común entre sus hilos. */
export function bubbleGroup(firstTags: readonly string[]): string {
  const count = new Map<string, number>();
  for (const t of firstTags) if (t) count.set(t, (count.get(t) ?? 0) + 1);
  let best = '';
  for (const [tag, n] of count) {
    const m = count.get(best) ?? 0;
    if (n > m || (n === m && tag < best)) best = tag;
  }
  return best;
}

/** El segundo tag más común (para colorear una burbuja por «tag secundario»). */
function commonSecond(lists: readonly (readonly string[])[]): string | undefined {
  const count = new Map<string, number>();
  for (const l of lists) if (l[1]) count.set(l[1], (count.get(l[1]) ?? 0) + 1);
  return [...count].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))[0]?.[0];
}

export function boardGraph(
  board: Board,
  projectOf: (workspace: string) => string,
  { me = '', teamName = (t: string) => t }: { me?: string; teamName?: (team: string) => string } = {},
): BoardGraph {
  const teamOf = new Map((board.workspaces ?? []).map((w) => [w.id, w.team ?? '']));
  const bubbleName = new Map(board.bubbles.map((b) => [b.id, b.name]));
  const firsts = new Map<string, string[]>();
  const tagLists = new Map<string, string[][]>();
  const threadCount = new Map<string, number>();
  const threads: Encodable[] = board.threads.map((t) => {
    const group = t.tags?.[0] ?? '';
    const team = teamOf.get(t.workspace) ?? '';
    if (t.bubble) {
      firsts.set(t.bubble, [...(firsts.get(t.bubble) ?? []), group]);
      tagLists.set(t.bubble, [...(tagLists.get(t.bubble) ?? []), t.tags ?? []]);
      threadCount.set(t.bubble, (threadCount.get(t.bubble) ?? 0) + 1);
    }
    return {
      id: t.id, name: t.name, team, teamName: team ? teamName(team) : '', project: projectOf(t.workspace),
      group, life: t.heat.lifecycle, level: 'calm', priority: t.priority ?? '', kind: 'threads',
      bubbleId: t.bubble || undefined, bubbleName: t.bubble ? bubbleName.get(t.bubble) : undefined,
      score: t.heat.score, due: t.due_date?.slice(0, 10) || undefined, people: t.assignees?.length ?? 0,
      mine: !!me && !!t.assignees?.includes(me), tags: t.tags ?? [], words: t.words ?? 0, files: t.files ?? 0,
    };
  });
  const bubbles: Encodable[] = board.bubbles.map((b) => {
    const team = teamOf.get(b.workspace) ?? '';
    const group = bubbleGroup(firsts.get(b.id) ?? []);
    const second = commonSecond(tagLists.get(b.id) ?? []);
    return {
      id: b.id, name: b.name, team, teamName: team ? teamName(team) : '', project: projectOf(b.workspace),
      group, life: b.heat.lifecycle, level: 'calm', priority: b.priority ?? '', kind: 'bubbles',
      score: b.heat.score, due: b.due_date?.slice(0, 10) || undefined, people: b.owners?.length ?? 0,
      mine: !!me && !!b.owners?.includes(me), tags: group ? [group, ...(second ? [second] : [])] : [],
      threads: threadCount.get(b.id) ?? 0,
    };
  });
  const relations: GraphLink[] = (board.links ?? []).map((l) => ({ source: l.from, target: l.to, kind: l.kind, count: 1 }));
  // Orden de los blobs: los más poblados primero (contando hilos), luego por
  // nombre. Estable para la disposición: no depende del filtro.
  const size = new Map<string, number>();
  for (const t of threads) if (t.group) size.set(t.group, (size.get(t.group) ?? 0) + 1);
  for (const b of bubbles) if (b.group && !size.has(b.group)) size.set(b.group, 0);
  const groups: GraphGroup[] = [...size.keys()]
    .sort((a, b) => (size.get(b)! - size.get(a)!) || a.localeCompare(b))
    .map((tag) => ({ id: tag, name: tag }));
  groups.push({ id: '', name: 'Sin tags' });
  return { bubbles, threads, relations, groups };
}

/** Lo que se ve de un nivel: sin cerradas, salvo que se pidan. */
export function visibleNodes(g: BoardGraph, kind: MapKind, closed: boolean): Encodable[] {
  return (kind === 'bubbles' ? g.bubbles : g.threads).filter((n) => closed || n.life !== 'closed');
}
