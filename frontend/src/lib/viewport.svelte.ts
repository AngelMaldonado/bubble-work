/**
 * Cuánto ancho hay, como estado reactivo.
 *
 * Una `matchMedia` y no un `resize`: el navegador ya sabe cuándo se cruza el
 * umbral y avisa una sola vez, mientras que escuchar `resize` es recalcular en
 * cada pixel de un arrastre para contestar una pregunta que sólo tiene dos
 * respuestas.
 *
 * El umbral es uno, a propósito. «Estrecho» aquí significa una cosa concreta —
 * no caben una columna de navegación y un panel a la vez — y no una escala de
 * tamaños. Cuando haga falta un segundo, será porque hay una segunda decisión
 * que tomar, y entonces tendrá su propio nombre.
 */

/** Por debajo de esto, la columna de navegación y el contenido no caben juntos. */
const NARROW = '(max-width: 720px)';

function media(query: string) {
  // Sin `window` —durante una prueba, o si esto se renderizara en un servidor—
  // la respuesta es «no es estrecho»: es el caso que la aplicación ya sabe
  // dibujar, y fallar aquí dejaría la pantalla en blanco por una media query.
  const mq = typeof window === 'undefined' ? null : window.matchMedia(query);
  let on = $state(mq?.matches ?? false);
  mq?.addEventListener('change', (e) => (on = e.matches));
  return {
    get on() {
      return on;
    },
  };
}

export const narrow = media(NARROW);
