/** El correo puede no ser visible y el nombre no es obligatorio. Nunca usamos
 * el id como nombre ni pedimos más permisos para obtener un respaldo. */
export function named(
  person: { display_name?: string | null; email?: string | null; deleted_at?: string | null } | null | undefined,
  fallback = 'sin nombre',
): string {
  const name = person?.display_name?.trim() || person?.email?.trim() || fallback;
  return person?.deleted_at ? `${name} (eliminado)` : name;
}

/** Normaliza antes de filtrar: también Zag necesita un itemToString válido.
 * Conserva la identidad y cualquier dato adicional, sin mutar la respuesta. */
export function matchingPeople<T extends { id: string; name?: string | null }>(
  people: readonly T[],
  query: string,
): (T & { name: string })[] {
  const needle = query.trim().toLowerCase();
  return people
    .map((person) => ({ ...person, name: named({ display_name: person.name }) }))
    .filter((person) => person.name.toLowerCase().includes(needle));
}
