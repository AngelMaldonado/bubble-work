/**
 * Lo que eligió quien mira: una preferencia y un «dónde ibas».
 *
 * Aquí vivió también el filtro por proyecto del board de Todos —unas fichas
 * sobre las bandas— y se quitó entero: en Todos, lo que se está pidiendo es ver
 * TODO junto, y limitarlo a unos cuantos proyectos es volver a mirar un
 * proyecto, que la columna de la izquierda hace con un clic y sin esconder
 * nada.
 *
 * Ninguna de las dos cosas va en la dirección, que es la misma decisión que
 * tomó v0 y por la misma razón: la dirección lleva QUÉ estás mirando —un board,
 * un thread, una página— y por eso se puede mandar a alguien. Un enlace que
 * arrastrara la columna colapsada de quien lo mandó enseñaría al que lo abre una
 * aplicación que no es la suya.
 */

/**
 * Si la columna de la izquierda está colapsada.
 *
 * En `localStorage` porque es una PREFERENCIA: quien trabaja con la columna
 * estrecha la quiere estrecha mañana, y volver a colapsarla en cada recarga es
 * pedirle que repita una decisión que ya tomó.
 */
const RAIL_KEY = 'bw.rail';

export const rail = {
  get(): boolean {
    try {
      return localStorage.getItem(RAIL_KEY) === '1';
    } catch {
      // Una pestaña privada, o un navegador que bloquea el almacenamiento, abre
      // con la columna ancha — que es el valor por defecto, no un error.
      return false;
    }
  },
  set(on: boolean): void {
    try {
      localStorage.setItem(RAIL_KEY, on ? '1' : '0');
    } catch {
      // Sin memoria, la elección vale para esta sesión. No es motivo para no
      // poder colapsarla.
    }
  },
};

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

/**
 * A dónde volver en el planeador después de abrir un hilo desde una tarjeta.
 *
 * En `sessionStorage`, como la burbuja abierta del board: es «dónde ibas» en
 * esta pestaña, no una preferencia. Se escribe al irse y se CONSUME al volver —
 * leerla dos veces reabriría la tarjeta cada vez que alguien entra al planeador.
 */
const RETURN_KEY = 'bw.planner-return';

export type PlannerReturn = { card: string; face: 'plan' | 'hilos'; scroll: number };

export const plannerReturn = {
  set(v: PlannerReturn): void {
    try {
      sessionStorage.setItem(RETURN_KEY, JSON.stringify(v));
    } catch {
      // sin memoria de sesión, volver deja el planeador como estaba
    }
  },
  peek(): PlannerReturn | null {
    try {
      return JSON.parse(sessionStorage.getItem(RETURN_KEY) ?? 'null');
    } catch {
      return null;
    }
  },
  clear(): void {
    try {
      sessionStorage.removeItem(RETURN_KEY);
    } catch {
      // nada que limpiar
    }
  },
};

/**
 * Lo que el planeador está escondiendo ahora mismo.
 *
 * FILTRAR, no buscar: ⌘K navega —te lleva a una tarjeta, a un hilo, a una
 * página— y esto esconde del tablero lo que no coincide, sin moverte de sitio.
 * Dos superficies porque son dos preguntas distintas: «llévame a esto» y
 * «enséñame sólo esto».
 *
 * En `localStorage`, como el orden de las columnas y los paneles, y por la
 * misma razón: acomodar mi tablero no es acomodar el de los demás. Y en el
 * navegador, no en la dirección: un enlace que arrastrara los filtros de quien
 * lo mandó enseñaría al que lo abre un tablero con cosas escondidas y ninguna
 * pista de por qué.
 */
const PLANNER_KEY = 'bw.planner-filters';

export type PlannerFilters = {
  /** texto libre, contra el nombre de la tarjeta */
  q: string;
  /** id del workspace */
  project: string;
  /** id de la persona */
  owner: string;
  /** id del objetivo; `none` es «sin objetivo», que es una respuesta */
  objective: string;
  /** la banda de calor: hot, dormant, rip, closed */
  band: string;
};

export const NO_FILTERS: PlannerFilters = {
  q: '',
  project: '',
  owner: '',
  objective: '',
  band: '',
};

export const plannerFilters = {
  get(): PlannerFilters {
    try {
      const raw = JSON.parse(localStorage.getItem(PLANNER_KEY) ?? 'null');
      return raw && typeof raw === 'object' ? { ...NO_FILTERS, ...raw } : { ...NO_FILTERS };
    } catch {
      // Sin memoria, el tablero abre sin esconder nada — que es el valor por
      // defecto y no un error.
      return { ...NO_FILTERS };
    }
  },
  set(v: PlannerFilters): void {
    try {
      localStorage.setItem(PLANNER_KEY, JSON.stringify(v));
    } catch {
      // vale para esta sesión
    }
  },
};
