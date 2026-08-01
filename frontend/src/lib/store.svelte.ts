// Central reactive state (Svelte 5 runes). One polling loop keeps bubbles fresh;
// components read from here and call the action helpers.
import { api, ApiError, getToken, setToken, clearToken } from './api';
import type { Actor, BubbleView, Inbox, Level } from './types';

const POLL_MS = 20_000;
const FOCUS_CAP = 5; // soft warning threshold for the In-progress band

class Store {
  actor = $state<Actor | null>(null);
  bubbles = $state<BubbleView[]>([]);
  inbox = $state<Inbox | null>(null);

  loading = $state(true);
  error = $state<string | null>(null);
  authed = $state<boolean>(!!getToken());

  // active instance filter ("" = all the caller's instances)
  instance = $state<string>('');
  polling = $state(false);

  private timer: ReturnType<typeof setInterval> | null = null;

  get instances(): string[] {
    return this.actor?.instances ?? [];
  }

  get visible(): BubbleView[] {
    return this.instance ? this.bubbles.filter((b) => b.instance === this.instance) : this.bubbles;
  }

  byLevel(level: Level): BubbleView[] {
    return this.visible
      .filter((b) => b.level === level)
      .sort((a, b) => b.score - a.score);
  }

  get focusCount(): number {
    return this.byLevel('in_progress').length;
  }

  get focusCap(): number {
    return FOCUS_CAP;
  }

  get overFocus(): boolean {
    return this.focusCount > FOCUS_CAP;
  }

  async boot(): Promise<void> {
    if (!getToken()) {
      this.authed = false;
      this.loading = false;
      return;
    }
    this.loading = true;
    try {
      this.actor = await api.whoami();
      this.authed = true;
      await this.refresh();
      this.startPolling();
    } catch (e) {
      if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
        this.signOut();
      } else {
        this.error = e instanceof Error ? e.message : String(e);
      }
    } finally {
      this.loading = false;
    }
  }

  async refresh(): Promise<void> {
    this.polling = true;
    try {
      const [bubbles, inbox] = await Promise.all([
        api.bubbles(),
        api.inbox().catch(() => null),
      ]);
      this.bubbles = bubbles;
      this.inbox = inbox;
      this.error = null;
    } catch (e) {
      if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
        this.signOut();
        return;
      }
      this.error = e instanceof Error ? e.message : String(e);
    } finally {
      this.polling = false;
    }
  }

  startPolling(): void {
    this.stopPolling();
    this.timer = setInterval(() => void this.refresh(), POLL_MS);
  }

  stopPolling(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
  }

  async signIn(token: string): Promise<void> {
    setToken(token);
    this.authed = true;
    await this.boot();
  }

  signOut(): void {
    this.stopPolling();
    clearToken();
    this.actor = null;
    this.bubbles = [];
    this.inbox = null;
    this.authed = false;
  }

  bubble(id: string): BubbleView | undefined {
    return this.bubbles.find((b) => b.id === id);
  }
}

export const store = new Store();
