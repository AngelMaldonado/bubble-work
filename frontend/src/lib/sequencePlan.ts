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

/** B va después de A, junto a lo que ya ocupe ese paso. No muta las filas. */
export function connectSequence(layers: string[][], a: string, b: string): string[][] {
  if (a === b) return layers.map((r) => [...r]);
  const rows = layers.map((r) => r.filter((id) => id !== b)).filter((r) => r.length);
  let ia = rows.findIndex((r) => r.includes(a));
  if (ia < 0) { rows.push([a]); ia = rows.length - 1; }
  if (ia + 1 < rows.length) rows[ia + 1].push(b);
  else rows.push([b]);
  return rows;
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
