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
