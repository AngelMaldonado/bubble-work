// Una conexión para toda la aplicación, y muchos temas encima.
//
// PocketBase abre un stream por `EventSource` y le cuelga las suscripciones que
// se le manden: cada pantalla que quiera enterarse de algo no necesita su propia
// conexión, necesita decir de qué tema quiere saber. Con una por pantalla
// tendríamos dos manejos de reconexión, dos ventanas de "me perdí lo que pasó
// mientras tanto", y el navegador contando conexiones contra el mismo origen.
//
// Dos cosas que este cliente hace y que son fáciles de olvidar:
//
//   · La suscripción se manda con el TOKEN, y el `EventSource` es anónimo. El
//     stream no vale nada hasta que se devuelve el `clientId` por POST — y ese
//     id cambia en cada reconexión, así que el envío cuelga del mensaje de
//     conexión y no del arranque.
//   · Una suscripción empieza en AHORA. Lo que pasó mientras estabas
//     desconectado no viaja por el stream, así que cada (re)conexión avisa a
//     todos los interesados para que relean.
//
// Las reglas siguen siendo del servidor: PocketBase filtra cada evento por la
// regla de lectura de su colección, así que abrir un stream no abre una puerta.
import { api } from './api';

type Watcher = (data: unknown) => void;

class Live {
  private stream: EventSource | null = null;
  /** temas que YA tienen su oyente en este stream. Sin esto, cada `sync`
   *  agregaría otro y un evento se entregaría dos, tres, cuatro veces. */
  private attached = new Set<string>();
  /** tema → quiénes lo escuchan */
  private watchers = new Map<string, Set<Watcher>>();
  private clientId = '';

  /** Escucha un tema — una colección, o `colección/id`. Devuelve su baja. */
  watch(topic: string, fn: Watcher): () => void {
    const set = this.watchers.get(topic) ?? new Set<Watcher>();
    set.add(fn);
    this.watchers.set(topic, set);
    this.open();
    this.sync();
    return () => {
      set.delete(fn);
      if (!set.size) this.watchers.delete(topic);
      this.sync();
      // La conexión se cierra cuando ya no queda nadie escuchando: un stream
      // abierto sin temas es una conexión que el servidor mantiene para nada.
      if (!this.watchers.size) this.close();
    };
  }

  close() {
    this.stream?.close();
    this.stream = null;
    this.clientId = '';
    this.attached.clear();
    this.sent = '';
  }

  private open() {
    if (this.stream) return;
    const es = new EventSource('/api/realtime');
    this.stream = es;

    es.addEventListener('PB_CONNECT', (e) => {
      const { clientId } = JSON.parse((e as MessageEvent).data ?? '{}');
      if (!clientId) return;
      this.clientId = clientId;
      this.sync(true);
    });

    // Un solo oyente por tema, agregado cuando el tema aparece. El SDK manda el
    // nombre del tema como nombre del evento.
    for (const topic of this.watchers.keys()) this.listen(es, topic);
  }

  private listen(es: EventSource, topic: string) {
    if (this.attached.has(topic)) return;
    this.attached.add(topic);
    es.addEventListener(topic, (e) => {
      let data: unknown = null;
      try {
        data = JSON.parse((e as MessageEvent).data ?? 'null');
      } catch {
        data = null;
      }
      for (const fn of this.watchers.get(topic) ?? []) fn(data);
    });
  }

  /** Manda al servidor la lista de temas. Reenviarla la REEMPLAZA, que es el
   *  contrato de PocketBase: se manda entera o no se manda. */
  private sent = '';
  private sync(reconnected = false) {
    if (!this.stream || !this.clientId) return;
    const topics = [...this.watchers.keys()].sort();
    const key = topics.join('|');
    if (key === this.sent && !reconnected) return;
    this.sent = key;
    for (const topic of topics) this.listen(this.stream, topic);
    api
      .subscribe(this.clientId, topics)
      // Cada (re)conexión invita a releer: la suscripción empieza en ahora, y lo
      // que pasó mientras no estábamos no llega por aquí.
      .then(() => {
        if (reconnected) {
          for (const set of this.watchers.values()) for (const fn of set) fn(null);
        }
      })
      .catch(() => {});
  }
}

export const live = new Live();
