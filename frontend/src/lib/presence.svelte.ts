// Who is here right now, as a store the whole app reads.
//
// Three moving parts, and they are not the same thing:
//
//   · this browser BEATS while its tab is visible. Beating from a hidden tab
//     would report a laptop, not a person.
//   · everybody else's beats arrive over PocketBase's realtime stream, so a
//     person appears the moment they open the app rather than up to half a
//     minute later.
//   · a TICKER re-derives who is still fresh, with no network at all. This is
//     the part a subscription cannot do for us: nobody emits an event when
//     somebody STOPS beating, and going quiet is exactly what presence has to
//     notice.
//
// The stream is best-effort. It reconnects on its own, a full read follows every
// (re)connect, and a slow read underneath covers the case where it never comes
// back at all — a row of avatars is not worth a reconnection state machine.
import { api } from './api';
import { when } from './when';

/** How fresh a beat has to be to count. Two bands, because a tab left open in
 *  another window is a true and useless statement about a person. */
const HERE = 2 * 60 * 1000;
const OPEN = 15 * 60 * 1000;

const BEAT = 60_000;
/** The backstop read. Not the primary path any more — that is the stream. */
const READ = 5 * 60_000;
/** How often "is she still here?" is asked of the rows we already have. */
const TICK = 20_000;

export type Around = { id: string; name: string; idle: boolean };

type Row = { id: string; name: string; at: string };

class Presence {
  /** Who is around, this person first — a row of initials is read left to
   *  right, and the one avatar you already know is yours. */
  around = $state<Around[]>([]);

  private rows: Row[] = [];
  private timers: ReturnType<typeof setInterval>[] = [];
  private stream: EventSource | null = null;
  private soon: ReturnType<typeof setTimeout> | null = null;

  /** Called once, after signing in. Idempotent: starting twice would double
   *  every request for the rest of the session. */
  start() {
    if (this.timers.length) return;
    this.beat();
    this.read();
    this.listen();
    this.timers = [
      setInterval(() => this.beat(), BEAT),
      setInterval(() => this.read(), READ),
      setInterval(() => this.derive(), TICK),
    ];
    // A tab coming back to the front says "still here" immediately rather than
    // up to a minute later, which is the difference between presence and a
    // rumour.
    document.addEventListener('visibilitychange', this.awake);
  }

  stop() {
    this.timers.forEach(clearInterval);
    this.timers = [];
    this.stream?.close();
    this.stream = null;
    this.rows = [];
    this.around = [];
    document.removeEventListener('visibilitychange', this.awake);
  }

  private awake = () => {
    if (document.visibilityState === 'visible') {
      this.beat();
      this.read();
    }
  };

  private async beat() {
    if (document.visibilityState !== 'visible') return;
    try {
      await api.beat();
    } catch {
      // A missed beat is not an error worth showing anybody: the next one is a
      // minute away, and presence that interrupts you is worse than no presence.
    }
  }

  private async read() {
    try {
      this.rows = await api.presence();
      this.derive();
    } catch {
      this.rows = [];
      this.derive();
    }
  }

  /** Who is around, from the rows already in hand. Pure, and called on a timer:
   *  the bands are a question about TIME, and time moves with no events. */
  private derive() {
    const now = Date.now();
    const live = this.rows
      .map((r) => ({ ...r, since: now - when(r.at) }))
      .filter((r) => r.since >= -60_000 && r.since < OPEN)
      .map((r) => ({ id: r.id, name: r.name, idle: r.since >= HERE }));
    const me = api.me?.id;
    this.around = [...live.filter((p) => p.id === me), ...live.filter((p) => p.id !== me)];
  }

  /** PocketBase's realtime stream.
   *
   *  Two steps, and the second is the one that is easy to miss: the EventSource
   *  is anonymous — it cannot carry an Authorization header — so it earns
   *  nothing until the client id it is handed is POSTed BACK with the token and
   *  the topics. The id changes on every reconnect, so that POST belongs to the
   *  connect message rather than to `start`. */
  private listen() {
    if (this.stream) return;
    const es = new EventSource('/api/realtime');
    this.stream = es;
    es.addEventListener('PB_CONNECT', (e) => {
      const { clientId } = JSON.parse((e as MessageEvent).data ?? '{}');
      if (!clientId) return;
      api
        .subscribe(clientId, ['presence'])
        // Whatever happened while we were away is in the collection, not in the
        // stream: a subscription starts at now.
        .then(() => this.read())
        .catch(() => {});
    });
    // Every beat by anybody lands here. The message carries the record, but not
    // the person's name — an expand costs a subscription option and a shape to
    // parse — so this only says "something moved", and the read that follows is
    // one small request.
    es.addEventListener('presence', () => this.nudge());
  }

  /** Coalesce: five people beating in the same second are one read. */
  private nudge() {
    if (this.soon) return;
    this.soon = setTimeout(() => {
      this.soon = null;
      this.read();
    }, 400);
  }
}

export const presence = new Presence();
