// The product tour (docs/journal/ARTIFACT-EDITING.md is not its subject — the MODEL is).
//
// Deliberately not a UI walkthrough. Where the buttons are is guessable; what is
// not guessable is that heat means evidence of changed reality rather than
// activity, that the bands are COMPUTED and cannot be set, that a bubble is its
// threads, and that a bubble dying is the system working. So the UI is the
// illustration and the model is the content.
//
// driver.js is lazy-loaded: it costs nothing until somebody asks for the tour.

import { api } from './api';
import { store } from './store.svelte';
import { t } from './i18n.svelte';

const SEEN = 'bubble.tour.seen';

/** Anchors the tour points at. Kept here so the markup and the tour agree. */
export const TOUR = {
  brand: 'brand',
  band: 'band',
  orb: 'orb',
  artifacts: 'artifacts',
  logbook: 'logbook',
  viewmode: 'viewmode',
} as const;

const sel = (name: string) => `[data-tour="${name}"]`;

/** Waits for an element the tour is about to point at. */
function waitFor(name: string, ms = 4000): Promise<Element | null> {
  const found = document.querySelector(sel(name));
  if (found) return Promise.resolve(found);
  return new Promise((resolve) => {
    const started = Date.now();
    const tick = () => {
      const el = document.querySelector(sel(name));
      if (el) return resolve(el);
      if (Date.now() - started > ms) return resolve(null);
      requestAnimationFrame(tick);
    };
    tick();
  });
}

/** A real thread for the tour to open, preferring one with a Logbook.
 *
 * The Logbook is where the model is most visible — a plan, its todos, and the
 * fact that ticking one is evidence — so the tour would rather show a thread
 * that has one. All of these are local mirror reads, so looking is cheap. */
async function sampleThread(): Promise<{ id: string; hasLogbook: boolean } | null> {
  const bubble = store.bubbles.find((b) => b.threads > 0);
  if (!bubble) return null;
  let first: string | null = null;
  try {
    const threads = await api.timeline(bubble.id);
    for (const th of threads.slice(0, 3)) {
      first ??= th.id;
      const d = await api.thread(th.id);
      if (d.logbook) return { id: th.id, hasLogbook: true };
    }
  } catch {
    // A tour is not worth an error message; fall through to whatever we have.
  }
  return first ? { id: first, hasLogbook: false } : null;
}

class Tour {
  running = $state(false);
  private obj: { destroy: () => void; moveNext: () => void; drive: () => void } | null = null;

  /** True once the tour has run itself, so it never ambushes the same person twice. */
  get seen(): boolean {
    try {
      return localStorage.getItem(SEEN) === '1';
    } catch {
      return true; // no storage: treat as seen rather than nag on every load
    }
  }

  private markSeen(): void {
    try {
      localStorage.setItem(SEEN, '1');
    } catch {
      // not remembering is not a reason to refuse
    }
  }

  /** Runs once, the first time somebody signs in. */
  maybeAutoStart(): void {
    // store.actor, not store.authed: authed is seeded from token presence and
    // stays true when the server is simply unreachable. An actor means whoami
    // answered, so we know who this is and the board is genuinely loaded.
    if (this.seen || store.kiosk || !store.actor) return;
    this.markSeen();
    // Let the board paint and settle before pointing at anything on it.
    setTimeout(() => void this.start(), 900);
  }

  async start(): Promise<void> {
    if (this.running) return;
    this.running = true;
    this.markSeen();

    const { driver } = await import('driver.js');
    await import('driver.js/dist/driver.css');

    // The orbs bob (a 6.5s loop), and driver.js cuts a STATIC hole in its
    // overlay — a moving target drifts out of its own highlight. The board
    // already knows how to hold still for prefers-reduced-motion; this reuses
    // that switch rather than inventing a second one.
    document.documentElement.setAttribute('data-tour', 'on');

    const sample = await sampleThread();
    const steps = this.steps(sample);

    const obj = driver({
      showProgress: true,
      allowClose: true,
      overlayOpacity: 0.68,
      popoverClass: 'bw-tour',
      nextBtnText: t('tour.next'),
      prevBtnText: t('tour.prev'),
      doneBtnText: t('tour.done'),
      progressText: '{{current}} / {{total}}',
      steps,
      onDestroyed: () => this.finish(),
    });
    this.obj = obj as unknown as typeof this.obj;
    obj.drive();
  }

  private finish(): void {
    document.documentElement.removeAttribute('data-tour');
    this.running = false;
    this.obj = null;
  }

  stop(): void {
    this.obj?.destroy();
    this.finish();
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private steps(sample: { id: string; hasLogbook: boolean } | null): any[] {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const steps: any[] = [
      {
        popover: {
          title: t('tour.1.title'),
          description: t('tour.1.body'),
        },
      },
      {
        element: sel(TOUR.band),
        popover: {
          title: t('tour.2.title'),
          description: t('tour.2.body'),
          side: 'bottom',
          align: 'start',
        },
      },
    ];

    // An empty workspace has no orb to point at, and "here is your hottest
    // bubble" is a broken promise on an empty board. The tour says what to do
    // instead and keeps the model steps, rather than highlighting nothing.
    if (!sample) {
      steps.push({
        element: sel(TOUR.brand),
        popover: {
          title: t('tour.empty.title'),
          description: t('tour.empty.body'),
          side: 'bottom',
          align: 'start',
        },
      });
      steps.push({ popover: { title: t('tour.7.title'), description: t('tour.7.body') } });
      return steps;
    }

    steps.push({
      element: sel(TOUR.orb),
      popover: {
        title: t('tour.3.title'),
        description: t('tour.3.body'),
        side: 'bottom',
        align: 'center',
        // Crossing into the interior is the one place the tour navigates. Do it
        // on Next, wait for the DOM the next step points at, and only then
        // advance — otherwise driver.js highlights an element that is not there
        // yet and the tour silently degrades to a modal.
        onNextClick: async () => {
          store.openThread(sample.id, sample.hasLogbook ? { kind: 'logbook', idx: 0 } : undefined);
          await waitFor(TOUR.artifacts);
          this.obj?.moveNext();
        },
      },
    });

    steps.push({
      element: sel(TOUR.artifacts),
      popover: {
        title: t('tour.4.title'),
        description: t('tour.4.body'),
        side: 'right',
        align: 'start',
      },
    });

    // Only when the thread actually has one: pointing at an absent Logbook
    // would highlight nothing and make the step read as a bug.
    if (sample.hasLogbook) {
      steps.push({
        element: sel(TOUR.logbook),
        popover: {
          title: t('tour.5.title'),
          description: t('tour.5.body'),
          side: 'left',
          align: 'start',
        },
      });
    }

    steps.push({
      element: sel(TOUR.viewmode),
      popover: {
        title: t('tour.6.title'),
        description: t('tour.6.body'),
        side: 'left',
        align: 'start',
      },
    });

    steps.push({
      popover: {
        title: t('tour.7.title'),
        description: t('tour.7.body'),
        // Put people back where they started, not deep inside a thread they
        // did not choose to open.
        onNextClick: () => {
          store.closeThread();
          this.stop();
        },
      },
    });
    return steps;
  }
}

export const tour = new Tour();
