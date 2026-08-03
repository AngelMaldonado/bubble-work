// Central reactive state (Svelte 5 runes). One polling loop keeps bubbles fresh;
// components read from here and call the action helpers.
import { api, ApiError, getToken, setToken, clearToken, setKioskToken } from './api';
import type { Actor, BubbleView, Inbox, Level } from './types';

const SEC_MS = 30_000; // secondary timer: inbox freshness (+ cross-org when godmode)
const RECONNECT_MS = 3_000; // SSE reconnect backoff
const FOCUS_CAP = 5; // soft warning threshold for the In-progress band

class Store {
  actor = $state<Actor | null>(null);
  bubbles = $state<BubbleView[]>([]);
  inbox = $state<Inbox | null>(null);

  loading = $state(true);
  error = $state<string | null>(null);
  flash = $state<string | null>(null); // transient info notice (e.g. godmode readouts)
  authed = $state<boolean>(!!getToken());

  // godmode: view bubbles across ALL orgs (service-admin only).
  allOrgs = $state(false);

  // active filters ("" = all). project holds a Plane project id.
  instance = $state<string>('');
  project = $state<string>('');
  polling = $state(false);

  // view scope (Phase 9): whose bubbles to show. "workspace" = everyone in
  // scope; "mine" = bubbles I own; "member" = a chosen owner (viewMember).
  scope = $state<'mine' | 'workspace' | 'member'>('workspace');
  viewMember = $state<string>('');

  // the bubble whose timeline detail panel is open (null = closed).
  detail = $state<BubbleView | null>(null);
  // the namespaced thread id whose interior is open (null = closed).
  threadId = $state<string | null>(null);

  private secTimer: ReturnType<typeof setInterval> | null = null;
  private stream: AbortController | null = null;

  get instances(): string[] {
    return this.actor?.instances ?? [];
  }

  get visible(): BubbleView[] {
    const me = this.actor?.name ?? '';
    return this.bubbles.filter(
      (b) =>
        (!this.instance || b.instance === this.instance) &&
        (!this.project || b.project === this.project) &&
        (this.scope === 'workspace' ||
          (this.scope === 'mine' && (b.members ?? []).includes(me)) ||
          (this.scope === 'member' && (b.members ?? []).includes(this.viewMember))),
    );
  }

  // read-only kiosk display (no personal identity, no writes).
  get kiosk(): boolean {
    return this.actor?.kind === 'kiosk' || !!this.actor?.read_only;
  }

  // distinct assignees within the current instance/project filter — the options
  // for the per-member view scope.
  get members(): string[] {
    const seen = new Set<string>();
    for (const b of this.bubbles) {
      if (this.instance && b.instance !== this.instance) continue;
      if (this.project && b.project !== this.project) continue;
      for (const m of b.members ?? []) seen.add(m);
    }
    return [...seen].sort((a, b) => a.localeCompare(b));
  }

  setScope(s: 'mine' | 'workspace' | 'member'): void {
    this.scope = s;
    if (s !== 'member') this.viewMember = '';
  }

  setViewMember(m: string): void {
    this.viewMember = m;
    this.scope = m ? 'member' : 'workspace';
  }

  // projects present within the current instance scope (id + display name)
  get projects(): { id: string; name: string }[] {
    const seen = new Map<string, string>();
    for (const b of this.bubbles) {
      if (this.instance && b.instance !== this.instance) continue;
      if (!seen.has(b.project)) seen.set(b.project, b.project_name || b.project);
    }
    return [...seen.entries()]
      .map(([id, name]) => ({ id, name }))
      .sort((a, b) => a.name.localeCompare(b.name));
  }

  selectInstance(v: string): void {
    this.instance = v;
    // drop the project filter if it no longer exists in the new scope
    if (this.project && !this.projects.some((p) => p.id === this.project)) {
      this.project = '';
    }
  }

  selectProject(v: string): void {
    this.project = v;
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
    // A ?kiosk=<token> URL boots a read-only display: adopt the token (in
    // sessionStorage, so it never clobbers a personal login) and strip it from
    // the visible URL so the credential isn't left in the address bar.
    const params = new URLSearchParams(window.location.search);
    const kioskTok = params.get('kiosk');
    if (kioskTok) {
      setKioskToken(kioskTok);
      params.delete('kiosk');
      const qs = params.toString();
      const url = window.location.pathname + (qs ? '?' + qs : '') + window.location.hash;
      window.history.replaceState(null, '', url);
      this.authed = true;
    }
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
      this.startStream();
      this.startSecondary();
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

  get godmode(): boolean {
    return !!this.actor?.service_admin;
  }

  async refresh(): Promise<void> {
    this.polling = true;
    const crossOrg = this.allOrgs && this.godmode;
    try {
      const [bubbles, inbox] = await Promise.all([
        crossOrg ? api.adminBubbles() : api.bubbles(),
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

  // ---- live updates (F4): SSE stream for bubbles, light timer for the rest ----

  private get crossOrg(): boolean {
    return this.allOrgs && this.godmode;
  }

  async refreshInbox(): Promise<void> {
    this.inbox = await api.inbox().catch(() => this.inbox);
  }

  // The stream is actor-scoped; the cross-org (godmode) board and the inbox are
  // refreshed on a slow timer instead of polling bubbles every few seconds.
  startSecondary(): void {
    this.stopSecondary();
    this.secTimer = setInterval(() => {
      if (this.crossOrg) void this.refresh();
      else void this.refreshInbox();
    }, SEC_MS);
  }

  stopSecondary(): void {
    if (this.secTimer) clearInterval(this.secTimer);
    this.secTimer = null;
  }

  startStream(): void {
    this.stopStream();
    const ac = new AbortController();
    this.stream = ac;
    void this.consumeStream(ac);
  }

  stopStream(): void {
    this.stream?.abort();
    this.stream = null;
  }

  private async consumeStream(ac: AbortController): Promise<void> {
    while (this.authed && this.stream === ac && !ac.signal.aborted) {
      try {
        const res = await fetch('/api/stream', {
          headers: { Authorization: `Bearer ${getToken()}` },
          signal: ac.signal,
        });
        if (res.status === 401 || res.status === 403) {
          this.signOut();
          return;
        }
        if (!res.ok || !res.body) throw new Error(`stream ${res.status}`);

        this.error = null;
        const reader = res.body.getReader();
        const dec = new TextDecoder();
        let buf = '';
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          buf += dec.decode(value, { stream: true });
          let i: number;
          while ((i = buf.indexOf('\n\n')) >= 0) {
            this.handleFrame(buf.slice(0, i));
            buf = buf.slice(i + 2);
          }
        }
      } catch {
        if (ac.signal.aborted) return;
      }
      // dropped connection — back off, then the while-loop reconnects
      await new Promise((r) => setTimeout(r, RECONNECT_MS));
    }
  }

  private handleFrame(frame: string): void {
    let event = 'message';
    let data = '';
    for (const line of frame.split('\n')) {
      if (line.startsWith(':')) return; // heartbeat/comment
      if (line.startsWith('event:')) event = line.slice(6).trim();
      else if (line.startsWith('data:')) data += line.slice(5).trim();
    }
    if (event !== 'bubbles' || !data) return;
    try {
      const vs = JSON.parse(data) as BubbleView[];
      // in cross-org (godmode) mode the board comes from the admin poll, not this
      // actor-scoped stream.
      if (!this.crossOrg) {
        this.bubbles = vs;
        this.error = null;
      }
    } catch {
      /* ignore malformed frame */
    }
  }

  async signIn(token: string): Promise<void> {
    setToken(token);
    this.authed = true;
    await this.boot();
  }

  signOut(): void {
    this.stopStream();
    this.stopSecondary();
    clearToken();
    this.actor = null;
    this.bubbles = [];
    this.inbox = null;
    this.authed = false;
  }

  bubble(id: string): BubbleView | undefined {
    return this.bubbles.find((b) => b.id === id);
  }

  openDetail(b: BubbleView): void {
    this.detail = b;
  }

  closeDetail(): void {
    this.detail = null;
  }

  openThread(id: string): void {
    this.threadId = id;
    this.setHash(`thread/${encodeURIComponent(id)}`);
  }

  closeThread(): void {
    this.threadId = null;
    this.setHash('');
  }

  // ---- hash routing for the dedicated thread screen ----

  private setHash(h: string): void {
    if (typeof location === 'undefined') return;
    if (location.hash.replace(/^#/, '') !== h) location.hash = h;
  }

  // syncFromHash makes the URL the source of truth (back/forward, refresh,
  // deep links). Called on boot and on every hashchange.
  syncFromHash(): void {
    if (typeof location === 'undefined') return;
    const h = location.hash.replace(/^#/, '');
    this.threadId = h.startsWith('thread/') ? decodeURIComponent(h.slice('thread/'.length)) : null;
  }
}

export const store = new Store();
