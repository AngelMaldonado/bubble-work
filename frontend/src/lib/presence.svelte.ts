// Who is here right now, as a store the whole app reads.
//
// Two halves, and they are not symmetric: this browser BEATS while its tab is
// visible, and READS everybody's beats on a slower cadence. Beating from a
// hidden tab would report a laptop, not a person — the mock's note about grey
// avatars applies here too: the question is who is around, not who left a tab
// open in another window.
//
// Polling rather than PocketBase's realtime stream, for now. The collection is
// one row per person and the read is one request every half minute, which for a
// department is noise; the SSE subscription is a swap behind this same store
// when it earns itself, and nothing above here would change.
import { api } from './api';
import { when } from './when';

/** How fresh a beat has to be to count. Two bands, because a tab left open in
 *  another window is a true and useless statement about a person. */
const HERE = 2 * 60 * 1000;
const OPEN = 15 * 60 * 1000;

const BEAT = 60_000;
const READ = 30_000;

export type Around = { id: string; name: string; idle: boolean };

class Presence {
  /** Who is around, this person first — a row of initials is read left to
   *  right, and the one avatar you already know is yours. */
  around = $state<Around[]>([]);
  private timers: ReturnType<typeof setInterval>[] = [];

  /** Called once, after signing in. Idempotent: starting twice would double
   *  every request for the rest of the session. */
  start() {
    if (this.timers.length) return;
    this.beat();
    this.read();
    this.timers = [
      setInterval(() => this.beat(), BEAT),
      setInterval(() => this.read(), READ),
    ];
    // A tab coming back to the front says "still here" immediately rather than
    // up to a minute later, which is the difference between presence and a
    // rumour.
    document.addEventListener('visibilitychange', this.awake);
  }

  stop() {
    this.timers.forEach(clearInterval);
    this.timers = [];
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
      // A missed beat is not an error worth showing anybody: the next one is
      // thirty seconds away, and presence that interrupts you is worse than no
      // presence.
    }
  }

  private async read() {
    try {
      const rows = await api.presence();
      const now = Date.now();
      const live = rows
        .map((r) => ({ ...r, since: now - when(r.at) }))
        .filter((r) => r.since >= 0 && r.since < OPEN)
        .map((r) => ({ id: r.id, name: r.name, idle: r.since >= HERE }));
      const me = api.me?.id;
      this.around = [...live.filter((p) => p.id === me), ...live.filter((p) => p.id !== me)];
    } catch {
      this.around = [];
    }
  }
}

export const presence = new Presence();
