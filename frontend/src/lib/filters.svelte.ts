/**
 * Lo que elegiste ver, y que sigue elegido cuando vuelves.
 *
 * Una sola cosa: en Todos, a qué proyectos limitarte. Hubo tres ejes —de quién,
 * qué banda, qué proyecto— y los otros dos se quitaron: la banda ES el board (se
 * mira, no se filtra) y «de quién» respondía una pregunta que el orbe ya
 * contesta con las iniciales de quien responde por él.
 *
 * En `localStorage` y NO en la dirección, que es la misma decisión que tomó v0 y
 * por la misma razón: la dirección lleva QUÉ estás mirando —un board, un thread,
 * una página— y por eso se puede mandar a alguien. A qué proyectos te limitaste
 * es una preferencia tuya, y un enlace que arrastra la de quien lo mandó enseña
 * al que lo abre un board que no es el suyo.
 *
 * Y es exactamente lo que hace que volver de un thread devuelva el board como lo
 * dejaste: nunca estuvo en la dirección, así que navegar no lo toca.
 */
const KEY = 'bw.filters';

export type Filters = {
  /** en Todos, a qué workspaces limitarse. Vacío es TODOS — el estado normal, y
   *  el que hay que saber leer cuando la caja llega de una versión anterior */
  projects: string[];
};

const EMPTY: Filters = { projects: [] };

function read(): Filters {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? 'null');
    if (!raw || typeof raw !== 'object') return { ...EMPTY };
    // Con el tipo comprobado: una caja escrita por una versión anterior, o a
    // mano, no puede dejar el board sin dibujar.
    return {
      projects: Array.isArray(raw.projects)
        ? raw.projects.filter((p: unknown) => typeof p === 'string')
        : [],
    };
  } catch {
    return { ...EMPTY }; // una entrada corrupta no vale un board roto
  }
}

class Store {
  current = $state<Filters>(read());

  /** Si hay algo puesto. Lo que decide si el board se anuncia como limitado: uno
   *  sin limitar no debería llevar un cartel diciéndolo. */
  get on(): boolean {
    return this.current.projects.length > 0;
  }

  clear(): void {
    this.current = { ...EMPTY };
    this.save();
  }

  /** Enciende o apaga un proyecto. Vacío significa todos, así que quitar el
   *  último equivale a no limitar — que es lo que una persona espera al
   *  desmarcar la última casilla. */
  toggle(id: string): void {
    const list = this.current.projects;
    this.current = {
      projects: list.includes(id) ? list.filter((x) => x !== id) : [...list, id],
    };
    this.save();
  }

  private save(): void {
    try {
      localStorage.setItem(KEY, JSON.stringify(this.current));
    } catch {
      // Un navegador que no deja guardar deja la elección en memoria, que sirve
      // durante la sesión. No es motivo para no poder elegir.
    }
  }
}

export const filters = new Store();

/**
 * Qué burbuja estaba abierta cuando te fuiste a un thread.
 *
 * En `sessionStorage` y no junto a lo anterior: esto NO es una preferencia, es
 * dónde ibas — vale para esta pestaña y para este rato, y encontrártelo mañana
 * al abrir el board sería una ventana que nadie pidió.
 */
const OPEN_KEY = 'bw.open-bubble';

export const lastOpenBubble = {
  get(): string {
    try {
      return sessionStorage.getItem(OPEN_KEY) ?? '';
    } catch {
      return '';
    }
  },
  set(id: string): void {
    try {
      if (id) sessionStorage.setItem(OPEN_KEY, id);
      else sessionStorage.removeItem(OPEN_KEY);
    } catch {
      // igual que arriba: sin memoria, el board abre sin cajón
    }
  },
};
