/**
 * Una repetición, desplegada en los días que caen dentro de una ventana.
 *
 * Se guarda UNA fila con su primera fecha y cómo se repite; las siguientes se
 * calculan aquí. Guardar una fila por ocurrencia sería una tabla que crece sola
 * y que alguien tiene que podar, y mover la junta de los lunes obligaría a
 * reescribir todas las filas futuras en vez de una.
 *
 * Todo en días, sin horas y sin zonas horarias: lo que el calendario muestra es
 * «el 30», no «el 30 a las 9». Con horas habría que decidir la zona de quién, y
 * eso es otra decisión y otro campo.
 */

/** Cómo se repite. Menú cerrado; ver la migración `calendar_events`. */
export type Repeat = '' | 'daily' | 'weekly' | 'biweekly' | 'monthly';

/** Cuántas ocurrencias se devuelven como mucho por evento.
 *
 *  Un tope y no una ventana infinita: una repetición no tiene fin de serie, así
 *  que sin esto una diaria llenaría la memoria en cuanto alguien navegara a
 *  2050. Con la ventana que usa el planeador, 600 son más de año y medio de una
 *  diaria. */
const MAX = 600;

const DAY = 86_400_000;

/** `2026-09-21` desde una fecha, en hora local: `toISOString` pasa por UTC y en
 *  México eso mueve el día al anterior durante seis horas cada noche. */
export function isoDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Un `YYYY-MM-DD` como fecha local, sin que la zona horaria lo corra un día. */
export function fromDay(day: string): Date {
  const [y, m, d] = day.slice(0, 10).split('-').map(Number);
  return new Date(y, (m ?? 1) - 1, d ?? 1);
}

/**
 * Los días en que cae `start` repetido, dentro de `[from, to]` inclusive.
 *
 * Sin repetición devuelve el propio día si está dentro, y nada si no: una fecha
 * suelta es una repetición de una.
 */
export function occurrences(start: string, repeat: Repeat, from: string, to: string): string[] {
  const first = fromDay(start);
  const end = fromDay(to);
  const begin = fromDay(from);
  if (Number.isNaN(first.getTime()) || Number.isNaN(end.getTime())) return [];
  if (!repeat) {
    const day = isoDay(first);
    return day >= from.slice(0, 10) && day <= to.slice(0, 10) ? [day] : [];
  }

  const out: string[] = [];
  if (repeat === 'monthly') {
    // El mismo día del mes. Si el mes no lo tiene —un 31 en febrero— se usa el
    // último día: la alternativa es saltarse el mes entero, y una junta que
    // desaparece de febrero sin decir nada es peor que una que cae el 28.
    const dom = first.getDate();
    let y = first.getFullYear();
    let mo = first.getMonth();
    for (let n = 0; n < MAX; n++) {
      const last = new Date(y, mo + 1, 0).getDate();
      const d = new Date(y, mo, Math.min(dom, last));
      if (d > end) break;
      if (d >= begin) out.push(isoDay(d));
      mo++;
      if (mo > 11) {
        mo = 0;
        y++;
      }
    }
    return out;
  }

  const step = { daily: 1, weekly: 7, biweekly: 14 }[repeat] * DAY;
  // Arrancar en la primera ocurrencia que alcanza la ventana en vez de contar
  // desde el principio: una junta semanal de hace tres años son 150 vueltas
  // antes de la primera que se va a dibujar.
  let t = first.getTime();
  if (t < begin.getTime()) {
    t += Math.ceil((begin.getTime() - t) / step) * step;
  }
  for (let n = 0; n < MAX && t <= end.getTime(); n++, t += step) {
    out.push(isoDay(new Date(t)));
  }
  return out;
}
