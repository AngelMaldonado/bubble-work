// Regiones visuales, no dependencias nuevas: un paso con varios nodos es
// paralelo incluso si la persona los movió a diferentes alturas del canvas.
export type Point = { x: number; y: number };
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
