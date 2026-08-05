// Central reactive state (Svelte 5 runes). One polling loop keeps bubbles fresh;
// components read from here and call the action helpers.
import {
  api,
  ApiError,
  getToken,
  setToken,
  clearToken,
  setKioskToken,
  clearKioskToken,
} from './api';
import type { Actor, BubbleView, Inbox, Level, ServiceStatus } from './types';

// ArtSel identifies one artifact within a thread's interior (work file, the
// logbook, or a revision), used by the route, pins, and scroll restoration.
export type ArtKind = 'artifact' | 'logbook' | 'revision';
export interface ArtSel {
  kind: ArtKind;
  idx: number;
}

const SEC_MS = 30_000; // secondary timer: inbox freshness (+ cross-org when godmode)
const RECONNECT_MS = 3_000; // SSE reconnect backoff
const FOCUS_CAP = 5; // soft warning threshold for the In-progress band

class Store {
  actor = $state<Actor | null>(null);
  bubbles = $state<BubbleView[]>([]);
  inbox = $state<Inbox | null>(null);

  loading = $state(true);
  error = $state<string | null>(null);
  /** whether the board is being served from an ageing mirror, and how far
   *  behind (PLANE-SYNC.md Phase 7). Fetched alongside the board because a
   *  stale board that says nothing is the failure mode the mirror introduces. */
  status = $state<ServiceStatus | null>(null);
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
  // which artifact within the open thread is selected (from the route, so it's
  // deep-linkable and pin-able). null → the thread view picks its default.
  threadSel = $state<ArtSel | null>(null);
  // top-level route: the board (default) or the /god-mode admin view.
  route = $state<'board' | 'god'>('board');

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
      const [bubbles, inbox, status] = await Promise.all([
        crossOrg ? api.adminBubbles() : api.bubbles(),
        api.inbox().catch(() => null),
        // never let a status failure break the board it is describing
        api.status().catch(() => null),
      ]);
      this.bubbles = bubbles;
      this.inbox = inbox;
      this.status = status;
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

  // exitKiosk drops the kiosk display token (sessionStorage) and reloads, so the
  // browser falls back to a personal login (or the auth gate on a dedicated
  // device). A full reload is cleanest — it re-boots identity from scratch.
  exitKiosk(): void {
    clearKioskToken();
    if (typeof location !== 'undefined') location.reload();
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

  openThread(id: string, sel?: ArtSel): void {
    this.threadId = id;
    this.threadSel = sel ?? null;
    this.setHash(threadHash(id, sel));
  }

  closeThread(): void {
    this.threadId = null;
    this.threadSel = null;
    this.setHash('');
  }

  // setThreadSel updates the selected artifact WITHOUT a history push (so
  // flipping between artifacts doesn't spam back/forward), keeping the route
  // deep-linkable and pin-able.
  setThreadSel(sel: ArtSel): void {
    this.threadSel = sel;
    if (this.threadId && typeof location !== 'undefined') {
      const h = '#' + threadHash(this.threadId, sel);
      history.replaceState(null, '', location.pathname + location.search + h);
    }
  }

  // ---- routing ----

  get godView(): boolean {
    return this.route === 'god';
  }

  // /god-mode is a real path so it's linkable and bookmarkable; the board and
  // thread screens keep using the hash.
  openGodMode(): void {
    this.route = 'god';
    if (typeof history !== 'undefined') history.pushState(null, '', '/god-mode');
  }

  closeGodMode(): void {
    this.route = 'board';
    if (typeof history !== 'undefined') {
      history.pushState(null, '', '/' + (location.hash || ''));
    }
  }

  private setHash(h: string): void {
    if (typeof location === 'undefined') return;
    if (location.hash.replace(/^#/, '') !== h) location.hash = h;
  }

  // syncFromLocation makes the URL the source of truth (back/forward, refresh,
  // deep links). Called on boot, hashchange, and popstate.
  syncFromLocation(): void {
    if (typeof location === 'undefined') return;
    if (location.pathname.replace(/\/+$/, '') === '/god-mode') {
      this.route = 'god';
      return;
    }
    this.route = 'board';
    const h = location.hash.replace(/^#/, '');
    if (!h.startsWith('thread/')) {
      this.threadId = null;
      this.threadSel = null;
      return;
    }
    // "thread/<encId>" or "thread/<encId>/<kindChar><idx>" (a0 | l0 | r1)
    const rest = h.slice('thread/'.length);
    const slash = rest.indexOf('/');
    if (slash < 0) {
      this.threadId = decodeURIComponent(rest);
      this.threadSel = null;
      return;
    }
    this.threadId = decodeURIComponent(rest.slice(0, slash));
    this.threadSel = parseArtSel(rest.slice(slash + 1));
  }
}

function threadHash(id: string, sel?: ArtSel | null): string {
  let h = `thread/${encodeURIComponent(id)}`;
  if (sel) h += `/${sel.kind[0]}${sel.idx}`; // a0 | l0 | r1
  return h;
}

function parseArtSel(s: string): ArtSel | null {
  const m = /^([alr])(\d+)$/.exec(s);
  if (!m) return null;
  const kind = m[1] === 'l' ? 'logbook' : m[1] === 'r' ? 'revision' : 'artifact';
  return { kind, idx: Number(m[2]) };
}

export const store = new Store();
