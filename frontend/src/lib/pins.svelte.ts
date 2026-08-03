// Browser-pinned artifacts: a personal, per-browser shortcut rail to specific
// thread artifacts (a Brief, a revision, the Logbook). Stored in localStorage so
// pins persist across sessions on this device; not synced server-side.
import type { ArtKind } from './store.svelte';

export interface Pin {
  threadId: string;
  threadTitle: string;
  kind: ArtKind;
  idx: number;
  title: string;
}

const KEY = 'bubble.pins';

function load(): Pin[] {
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as Pin[]) : [];
  } catch {
    return [];
  }
}

function pinId(p: Pin): string {
  return `${p.threadId}|${p.kind}${p.idx}`;
}

class Pins {
  items = $state<Pin[]>(load());

  private persist(): void {
    try {
      localStorage.setItem(KEY, JSON.stringify(this.items));
    } catch {
      /* storage full / unavailable — pins are best-effort */
    }
  }

  has(p: Pin): boolean {
    const id = pinId(p);
    return this.items.some((x) => pinId(x) === id);
  }

  toggle(p: Pin): void {
    const id = pinId(p);
    this.items = this.has(p) ? this.items.filter((x) => pinId(x) !== id) : [...this.items, p];
    this.persist();
  }

  remove(p: Pin): void {
    const id = pinId(p);
    this.items = this.items.filter((x) => pinId(x) !== id);
    this.persist();
  }
}

export const pins = new Pins();
