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
import type { Actor, BubbleView, Inbox, Level, ServiceStatus, Workspace } from './types';

// ArtSel identifies one artifact within a thread's interior (work file, the
// logbook, or a revision), used by the route, pins, and scroll restoration.
export type ArtKind = 'artifact' | 'logbook' | 'revision';
export interface ArtSel {
  kind: ArtKind;
  idx: number;
}

// Filters survive a reload. Losing them on every refresh made the board feel
// like it forgot what you were doing — and with the workspace filter especially,
// re-picking it was the first thing you did every single time.
//
// They live in localStorage, not the URL: the hash already carries WHAT you are
// looking at (thread, artifact) and is meant to be shareable. Whose bubbles you
// filtered to is a preference of yours, not part of the address.
const FILTERS_KEY = 'bubble.filters';

interface SavedFilters {
  instance?: string;
  project?: string;
  scope?: 'mine' | 'workspace' | 'member';
  viewMember?: string;
  /** project ids most-recently-scoped first; "" (all projects) is a real entry */
  recentProjects?: string[];
}

// How many visits the switcher remembers. Alt+Tab's value is in its first two or
// three entries; past that you are reading a list, not flicking between places.
const RECENT_CAP = 12;

function readFilters(): SavedFilters {
  try {
    const raw = localStorage.getItem(FILTERS_KEY);
    if (!raw) return {};
    const v = JSON.parse(raw) as SavedFilters;
    return v && typeof v === 'object' ? v : {};
  } catch {
    return {}; // a corrupt or unreadable entry is not worth a broken board
  }
}

// Read once at module load: the filters are seeded into $state fields, and four
// separate reads of the same key would only invite them to disagree.
const savedFilters = readFilters();

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
  /** last thread whose artifacts changed under us, with a timestamp so two
   *  edits to the SAME thread still register as two distinct changes. */
  threadChanged = $state<{ id: string; at: number } | null>(null);
  flash = $state<string | null>(null); // transient info notice (e.g. godmode readouts)
  authed = $state<boolean>(!!getToken());

  // godmode: view bubbles across ALL orgs (service-admin only).
  allOrgs = $state(false);

  // active filters ("" = all). project holds a Plane project id.
  // Seeded from the last session — see FILTERS_KEY.
  instance = $state<string>(savedFilters.instance ?? '');
  project = $state<string>(savedFilters.project ?? '');
  polling = $state(false);

  // Project scopes in the order they were last visited, most recent first. This
  // is what makes Shift+Tab behave like Alt+Tab: the ring is ordered by where you
  // have BEEN, so one tap returns to the project you just came from instead of
  // landing on whatever happens to be next alphabetically.
  //
  // Seeded with the restored scope so the very first Shift+Tab of a session has a
  // current entry to step away from.
  recentProjects = $state<string[]>(
    savedFilters.recentProjects?.length
      ? savedFilters.recentProjects
      : [savedFilters.project ?? ''],
  );

  // view scope (Phase 9): whose bubbles to show. "workspace" = everyone in
  // scope; "mine" = bubbles I own; "member" = a chosen owner (viewMember).
  scope = $state<'mine' | 'workspace' | 'member'>(savedFilters.scope ?? 'workspace');
  viewMember = $state<string>(savedFilters.viewMember ?? '');

  // the bubble whose timeline detail panel is open (null = closed).
  detail = $state<BubbleView | null>(null);
  // the namespaced thread id whose interior is open (null = closed).
  threadId = $state<string | null>(null);
  // the workspace whose documents are open (null = closed). A screen of its own,
  // not a modal: reading a spec is a sitting-down activity, and it deserves a URL
  // you can send someone.
  pagesWorkspace = $state<string | null>(null);
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
    this.saveFilters();
  }

  setViewMember(m: string): void {
    this.viewMember = m;
    this.scope = m ? 'member' : 'workspace';
    this.saveFilters();
  }

  // A kiosk is a shared wall display, not a person: it must not inherit whatever
  // the last human at that browser was filtered to, and must not leave its own
  // filters behind for them either.
  saveFilters(): void {
    if (this.kiosk) return;
    try {
      localStorage.setItem(
        FILTERS_KEY,
        JSON.stringify({
          instance: this.instance,
          project: this.project,
          scope: this.scope,
          viewMember: this.viewMember,
          recentProjects: this.recentProjects,
        } satisfies SavedFilters),
      );
    } catch {
      // storage full or blocked (private mode): filters just stop persisting
    }
  }

  // A saved filter can outlive what it points at — a workspace gets deleted, a
  // person stops owning anything, an instance is un-federated. Restoring it then
  // leaves an empty board with no visible reason, and no way to tell the filter
  // is the cause. So once the real data is in, anything that no longer resolves
  // is dropped.
  private pruneFilters(): void {
    // Identity is only known once whoami has answered, which is after the state
    // was seeded — so this is the first point at which a kiosk can disown the
    // filters it inherited from whoever last used this browser.
    if (this.kiosk) {
      this.instance = '';
      this.project = '';
      this.scope = 'workspace';
      this.viewMember = '';
      // A wall display must not inherit the browsing history of whoever last
      // used this browser either.
      this.recentProjects = [''];
      return;
    }
    if (this.instance && !this.instances.includes(this.instance)) {
      this.instance = '';
      this.project = '';
      this.touchProject('');
    }
    // Only once the workspace list has actually arrived — an empty list during
    // loading is not evidence that the workspace is gone.
    if (this.project && this.workspaces.length && !this.workspaces.some((w) => w.id === this.project)) {
      this.project = '';
      this.touchProject('');
    }
    // The recency ring outlives the workspaces in it, and a deleted project would
    // otherwise sit there forever taking up one of RECENT_CAP slots. Harmless to
    // the switcher, which orders the LIVE projects by this and ignores the rest —
    // but a persisted list of ids that resolve to nothing is a lie worth not
    // keeping. Same guard: an empty workspace list is loading, not deletion.
    if (this.workspaces.length) {
      const live = this.recentProjects.filter(
        (id) => id === '' || this.workspaces.some((w) => w.id === id),
      );
      // Only on an actual change: this runs on every poll, and reassigning an
      // identical array would wake the switcher's ordering for nothing.
      if (live.length !== this.recentProjects.length) {
        this.recentProjects = live.length ? live : [this.project];
      }
    }
    // Deliberately checked against EVERY bubble rather than store.members, which
    // is narrowed by the current instance and project filters. Someone who owns
    // nothing in the workspace you happen to be filtered to is an empty result
    // you asked for, not a stale filter to throw away.
    if (this.scope === 'member' && this.bubbles.length) {
      const known = this.bubbles.some((b) => (b.members ?? []).includes(this.viewMember));
      if (!known) {
        this.viewMember = '';
        this.scope = 'workspace';
      }
    }
    this.saveFilters();
  }

  // The workspaces the server knows about, INCLUDING empty ones. The board is
  // assembled from bubbles and skips a project with no modules, so a workspace
  // nothing has been put in yet cannot be inferred from it — which used to make
  // a newly created one invisible, and therefore impossible to put a first
  // bubble into from here.
  workspaces = $state<Workspace[]>([]);

  // projects present within the current instance scope (id + display name).
  // Falls back to what the bubbles imply while the real list is still loading,
  // so the filter never blinks empty on a cold start.
  get projects(): { id: string; name: string }[] {
    if (this.workspaces.length) {
      return this.workspaces
        .filter((w) => !this.instance || w.instance === this.instance)
        .map((w) => ({ id: w.id, name: w.name }))
        .sort((a, b) => a.name.localeCompare(b.name));
    }
    const seen = new Map<string, string>();
    for (const b of this.bubbles) {
      if (this.instance && b.instance !== this.instance) continue;
      if (!seen.has(b.project)) seen.set(b.project, b.project_name || b.project);
    }
    return [...seen.entries()]
      .map(([id, name]) => ({ id, name }))
      .sort((a, b) => a.name.localeCompare(b.name));
  }

  /** The instance a workspace belongs to — needed to address it as slug:project. */
  instanceOf(projectId: string): string {
    const w = this.workspaces.find((x) => x.id === projectId);
    if (w) return w.instance;
    return this.bubbles.find((b) => b.project === projectId)?.instance ?? this.instance;
  }

  // The workspace the UI is acting ON, which is not quite the filter: when there
  // is only one workspace in scope the filter is hidden entirely, and "no
  // selection" then means that one rather than nothing.
  get activeProject(): { id: string; name: string } | null {
    const ps = this.projects;
    if (this.project) return ps.find((p) => p.id === this.project) ?? null;
    return ps.length === 1 ? ps[0] : null;
  }

  selectInstance(v: string): void {
    this.instance = v;
    // drop the project filter if it no longer exists in the new scope
    if (this.project && !this.projects.some((p) => p.id === this.project)) {
      this.project = '';
      this.touchProject('');
    }
    this.saveFilters();
  }

  selectProject(v: string): void {
    this.project = v;
    this.touchProject(v);
    this.saveFilters();
  }

  /** Record a visit: move the scope to the front of the recency ring. Every path
   *  that changes `project` goes through here, so the ring cannot silently drift
   *  out of step with where the board actually is. */
  private touchProject(id: string): void {
    this.recentProjects = [id, ...this.recentProjects.filter((p) => p !== id)].slice(0, RECENT_CAP);
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
      const [bubbles, inbox, status, workspaces] = await Promise.all([
        crossOrg ? api.adminBubbles() : api.bubbles(),
        api.inbox().catch(() => null),
        // never let a status failure break the board it is describing
        api.status().catch(() => null),
        // nor a workspace-list failure: the filter falls back to the bubbles
        api.workspaces().catch(() => null),
      ]);
      this.bubbles = bubbles;
      // Re-point the open detail panel at the LIVE bubble. It holds a reference
      // into the array we just replaced, so without this it shows whatever was
      // true when it was opened — a renamed bubble keeps its old name, a closed
      // one its old band. Falls back to the stale copy when the bubble is gone
      // (deleted, or filtered out), which the panel already survives.
      if (this.detail) {
        const id = this.detail.id;
        this.detail = bubbles.find((b) => b.id === id) ?? this.detail;
      }
      if (workspaces) this.workspaces = workspaces;
      this.pruneFilters();
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
    if (!data) return;
    // A thread-scoped nudge: an agent edited these artifacts (MCP-ACCESS.md).
    // Bumping a counter rather than fetching here keeps the store ignorant of
    // how a thread is rendered — ThreadView owns that and re-reads when the
    // thread on screen is the one that changed.
    if (event === 'thread') {
      try {
        this.threadChanged = { id: JSON.parse(data) as string, at: Date.now() };
      } catch {
        /* ignore malformed frame */
      }
      return;
    }
    if (event !== 'bubbles') return;
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
    try {
      localStorage.removeItem(FILTERS_KEY);
    } catch {
      // nothing to do — the filters are cosmetic
    }
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

  /** one-shot: open the detail panel with its name focused for renaming. Same
   *  shape as openThread's `sel` — "open this, pointed at that" — and consumed by
   *  the panel so a later reopen is an ordinary read, not another rename. */
  detailRename = $state(false);

  openDetail(b: BubbleView, opts?: { rename?: boolean }): void {
    this.detail = b;
    this.detailRename = opts?.rename === true;
  }

  closeDetail(): void {
    this.detail = null;
    this.detailRename = false;
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

  openPages(workspaceId: string): void {
    this.pagesWorkspace = workspaceId;
    this.setHash(`pages/${encodeURIComponent(workspaceId)}`);
  }

  closePages(): void {
    this.pagesWorkspace = null;
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
    if (h.startsWith('pages/')) {
      this.pagesWorkspace = decodeURIComponent(h.slice('pages/'.length));
      this.threadId = null;
      this.threadSel = null;
      return;
    }
    this.pagesWorkspace = null;
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
