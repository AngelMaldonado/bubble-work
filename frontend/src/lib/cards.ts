// Lo que el planeador abre: un módulo (burbuja) o una tarea (hilo), con lo
// justo para dibujarlo en el calendario, la secuencia y su hoja.
//
// Vivía en Kanban.svelte; el kanban se quitó y la tarjeta se quedó, porque la
// hoja del módulo y el calendario siguen abriéndola.
export type Card = {
  id: string;
  title: string;
  obj?: number;
  /** el NOMBRE del objetivo; `obj` es sólo su posición en la lista */
  objName?: string;
  /** DERIVED from impact × urgency — never typed in by hand */
  prio?: string;
  due?: string;
  impact?: string;
  urgency?: string;
  notes?: string;
  /** cuántos threads lleva dentro */
  pieces?: number;
  /** el NOMBRE del proyecto donde vive */
  where?: string;
  /** cuándo nació, ISO */
  born?: string;
  /** el id del workspace: lo que resuelve sus imágenes */
  ws?: string;
  /** quién está a cargo: ids de persona (`bubbles.owners`) */
  owners?: string[];
};
