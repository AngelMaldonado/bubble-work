// ¿Se recortó el texto de un elemento? Medido EN LOTE.
//
// Leer `scrollHeight` obliga al navegador a calcular el layout. Si cada burbuja
// lo lee al montarse, justo después de que otra cambió el DOM, son tantos
// layouts de la página entera como burbujas: con 410 hilos en Flotabilidad,
// ~13 s congelada (medido). Aquí se juntan todas las peticiones de un cuadro:
// primero se lee todo (un solo layout) y después se avisa, que es cuando se
// escribe.
type Job = { el: HTMLElement; done: (clipped: boolean) => void };
let queue: Job[] = [];
let scheduled = false;

function flush() {
  scheduled = false;
  const jobs = queue;
  queue = [];
  const results = jobs.map((j) => (j.el.isConnected ? j.el.scrollHeight - j.el.clientHeight > 1 : null));
  jobs.forEach((j, i) => { if (results[i] !== null) j.done(results[i]!); });
}

export function measureClip(el: HTMLElement, done: (clipped: boolean) => void) {
  queue.push({ el, done });
  if (!scheduled) {
    scheduled = true;
    requestAnimationFrame(flush);
  }
}
