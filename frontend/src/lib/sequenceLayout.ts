// Regiones visuales, no dependencias nuevas: un paso con varios nodos es
// paralelo incluso si la persona los movió a diferentes alturas del canvas.
export type Point = { x: number; y: number };

/** Un tramo de flecha. `cut` es la tarea en su punta: cortar el tramo la saca
 *  de la secuencia. Los tramos que salen de una tarea hacia un nudo no tienen
 *  punta, así que no se cortan. */
export type Link = { key: string; from: Point; to: Point; arrow: boolean; cut?: string };
/** Entre el paso `at - 1` y el `at`: soltar ahí crea un paso nuevo en medio. */
export type Gap = { at: number; x: number; y: number };

/** Las flechas entre pasos consecutivos.
 *
 *  El registro dice «todo lo del paso i va antes que todo lo del i+1». Dibujarlo
 *  de cada tarea a cada tarea son m×n flechas que se cruzan; con un nudo en
 *  medio son m+n, y dicen lo mismo. Entre dos pasos de una sola tarea, una
 *  flecha directa. El nudo (o el centro de la flecha) es también el hueco donde
 *  se inserta un paso. */
export function sequenceLinks<T extends { id: string }>(
  layers: T[][], at: (t: T) => Point, width: number, height: number,
): { links: Link[]; gaps: Gap[] } {
  const links: Link[] = [];
  const gaps: Gap[] = [];
  const out = (t: T) => { const p = at(t); return { x: p.x + width / 2, y: p.y + height + 14 }; };
  const into = (t: T) => { const p = at(t); return { x: p.x + width / 2, y: p.y - 6 }; };
  for (let i = 0; i + 1 < layers.length; i++) {
    const up = layers[i], down = layers[i + 1];
    const tails = up.map(out), heads = down.map(into);
    const all = [...tails, ...heads];
    // Un paso en paralelo va dentro de su marco (ver `parallelFrames`: 44 px
    // arriba para el rótulo, 36 abajo). El nudo va entre los marcos, no dentro
    // de uno, para no caer encima del rótulo «En paralelo».
    const below = Math.max(...tails.map((p) => p.y)) + (up.length > 1 ? 22 : 0);
    const above = Math.min(...heads.map((p) => p.y)) - (down.length > 1 ? 44 : 0);
    const j = { x: all.reduce((s, p) => s + p.x, 0) / all.length, y: (below + above) / 2 };
    gaps.push({ at: i + 1, x: j.x, y: j.y });
    if (up.length === 1 && down.length === 1) {
      links.push({ key: `${up[0].id}>${down[0].id}`, from: tails[0], to: heads[0], arrow: true, cut: down[0].id });
      continue;
    }
    up.forEach((t, k) => links.push({ key: `${t.id}>@${i}`, from: tails[k], to: j, arrow: false }));
    down.forEach((t, k) => links.push({ key: `@${i}>${t.id}`, from: j, to: heads[k], arrow: true, cut: t.id }));
  }
  return { links, gaps };
}
export function parallelFrames<T>(layers: T[][], at: (t: T) => Point, width: number, height: number) {
  return layers.flatMap((row, step) => {
    if (row.length < 2) return [];
    const ps = row.map(at);
    const left = Math.min(...ps.map((p) => p.x));
    const top = Math.min(...ps.map((p) => p.y));
    const right = Math.max(...ps.map((p) => p.x + width));
    const bottom = Math.max(...ps.map((p) => p.y + height));
    return [{ step, count: row.length, x: left - 20, y: top - 44,
      width: right - left + 40, height: bottom - top + 80 }];
  });
}
