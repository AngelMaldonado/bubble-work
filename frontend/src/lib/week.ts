// La semana, según el día en que empieza. Cada persona lo elige (Ajustes → Mi
// usuario → Calendario); por defecto, el lunes.
//
// `first` es un día de `Date.getDay()`: 0 domingo, 1 lunes … 6 sábado.

export const WEEKDAYS = ['domingo', 'lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado'];
export const DEFAULT_WEEK_START = 1;

/** un día de semana válido, o el de siempre */
export function validWeekStart(v: unknown): number {
  return typeof v === 'number' && Number.isInteger(v) && v >= 0 && v <= 6 ? v : DEFAULT_WEEK_START;
}

const at = (y: number, m: number, d: number) => new Date(y, m, d);

/** el primer día de la semana que contiene `d` (a medianoche local) */
export function startOfWeek(d: Date, first: number): Date {
  return at(d.getFullYear(), d.getMonth(), d.getDate() - ((d.getDay() - first + 7) % 7));
}

/** qué lugar ocupa `d` en su semana: 0 el primer día, 6 el último */
export function dayInWeek(d: Date, first: number): number {
  return (d.getDay() - first + 7) % 7;
}

const daysBetween = (a: Date, b: Date) => Math.round((b.getTime() - a.getTime()) / 86_400_000);

/** El número de la semana que contiene `d`.
 *  Si empieza en lunes, el de ISO 8601 (la 1 es la del primer jueves del año):
 *  el que se usa en casi todo el mundo que empieza en lunes. Si empieza en otro
 *  día, la 1 es la que contiene el 1 de enero (la regla de EE. UU. y de casi
 *  todos los que empiezan en domingo o sábado). */
export function weekNumber(d: Date, first: number): number {
  if (first === 1) {
    const thu = at(d.getFullYear(), d.getMonth(), d.getDate() + 3 - ((d.getDay() + 6) % 7));
    const jan4 = at(thu.getFullYear(), 0, 4);
    return 1 + Math.round((daysBetween(jan4, thu) - 3 + ((jan4.getDay() + 6) % 7)) / 7);
  }
  const s = startOfWeek(d, first);
  const end = at(s.getFullYear(), s.getMonth(), s.getDate() + 6);
  if (end.getFullYear() > s.getFullYear()) return 1; // contiene el 1 de enero que viene
  return 1 + Math.round(daysBetween(startOfWeek(at(s.getFullYear(), 0, 1), first), s) / 7);
}
