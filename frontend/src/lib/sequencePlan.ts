// La vista puede ser un canvas; el registro sigue siendo pasos ordenados.
// Mismo número = paralelo. No se guardan aristas independientes.
export type SequenceRow = { id: string; sequence: number; done: boolean };

export function sequenceLayers<T extends SequenceRow>(items: T[]): T[][] {
  const by = new Map<number, T[]>();
  for (const t of items) {
    if (!t.sequence || t.done) continue;
    by.set(t.sequence, [...(by.get(t.sequence) ?? []), t]);
  }
  return [...by.entries()].sort((a, b) => a[0] - b[0]).map(([, ts]) => ts);
}

/** B va después de A, junto a lo que ya ocupe ese paso. No muta las filas.
 *
 *  Si A todavía no está en la línea y B sí, B se queda donde está y A entra
 *  como un paso propio justo antes. Antes A entraba al final y B se iba detrás
 *  de él: enlazar una tarea nueva con la primera la mandaba al final. */
export function connectSequence(layers: string[][], a: string, b: string): string[][] {
  if (a === b) return layers.map((r) => [...r]);
  const ib = layers.findIndex((r) => r.includes(b));
  if (ib >= 0 && !layers.some((r) => r.includes(a))) {
    const rows = layers.map((r) => [...r]);
    rows.splice(ib, 0, [a]);
    return rows;
  }
  const rows = layers.map((r) => r.filter((id) => id !== b)).filter((r) => r.length);
  let ia = rows.findIndex((r) => r.includes(a));
  if (ia < 0) { rows.push([a]); ia = rows.length - 1; }
  if (ia + 1 < rows.length) rows[ia + 1].push(b);
  else rows.push([b]);
  return rows;
}

/** `id` como un paso propio en el hueco `at` (0 = antes del primero, n = al
 *  final), contado sobre las filas que se pasan. Si ya estaba en la línea, sale
 *  de su paso; si ese paso queda vacío, desaparece. No muta las filas. */
export function insertStep(layers: string[][], id: string, at: number): string[][] {
  const marked: (string[] | null)[] = layers.map((r) => [...r]);
  marked.splice(Math.max(0, Math.min(at, layers.length)), 0, null);
  return marked
    .map((r) => (r === null ? [id] : r.filter((x) => x !== id)))
    .filter((r) => r.length);
}

/** Compacta huecos y emite sólo cambios; lo terminado conserva su historial. */
export function sequenceChanges(items: SequenceRow[], layers: string[][]) {
  const now = new Map(items.map((t) => [t.id, t.sequence]));
  const want = new Map<string, number>();
  layers.filter((r) => r.length).forEach((r, i) => r.forEach((id) => want.set(id, (i + 1) * 10)));
  const changes: { id: string; sequence: number }[] = [];
  for (const [id, seq] of want) if ((now.get(id) ?? 0) !== seq) changes.push({ id, sequence: seq });
  for (const t of items) {
    if (t.sequence && !t.done && !want.has(t.id)) changes.push({ id: t.id, sequence: 0 });
  }
  return changes;
}
