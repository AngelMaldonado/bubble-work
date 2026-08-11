// Thin typed client over the server REST surface (§9). The browser presents the
// same credential as the CLI — a Plane API key — as a Bearer token.
import type {
  Actor,
  BubbleView,
  ThreadHit,
  Inbox,
  AdminStats,
  AdminInstance,
  InstanceMembers,
  KioskToken,
  ThreadNode,
  ThreadDetail,
  RegionName,
  Comment,
  TuningView,
  SyncStatus,
  SyncDiff,
  SyncFidelity,
  SyncResult,
  OutboxView,
  ServiceStatus,
  Workspace,
  Page,
  PageDetail,
} from './types';

const TOKEN_KEY = 'bubble.token';
const KIOSK_KEY = 'bubble.kiosk';

// A kiosk display token (sessionStorage) takes precedence over a personal login
// (localStorage) so opening a ?kiosk= URL never clobbers someone's own token and
// is scoped to that browser tab.
export function getToken(): string {
  return sessionStorage.getItem(KIOSK_KEY) || localStorage.getItem(TOKEN_KEY) || '';
}

export function setToken(tok: string): void {
  localStorage.setItem(TOKEN_KEY, tok.trim());
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export function setKioskToken(tok: string): void {
  sessionStorage.setItem(KIOSK_KEY, tok.trim());
}

export function clearKioskToken(): void {
  sessionStorage.removeItem(KIOSK_KEY);
}

export function isKiosk(): boolean {
  return !!sessionStorage.getItem(KIOSK_KEY);
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      Authorization: `Bearer ${getToken()}`,
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = (await res.text()).trim();
    throw new ApiError(res.status, text || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get('content-type') ?? '';
  if (!ct.includes('application/json')) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  whoami: () => req<Actor>('GET', '/api/whoami'),
  bubbles: () => req<BubbleView[]>('GET', '/api/bubbles'),
  threads: (q: string) =>
    req<ThreadHit[]>('GET', `/api/threads?q=${encodeURIComponent(q)}`),
  inbox: () => req<Inbox>('GET', '/api/notifications'),
  // cheap: two indexed sqlite queries, no Plane traffic (PLANE-SYNC.md Phase 7)
  status: () => req<ServiceStatus>('GET', '/api/status'),

  // interior read model (INTERIOR-PLAN.md)
  timeline: (bubbleId: string) =>
    req<ThreadNode[]>('GET', `/api/bubbles/${encodeURIComponent(bubbleId)}/threads`),
  thread: (id: string) => req<ThreadDetail>('GET', `/api/threads/${encodeURIComponent(id)}`),
  comments: (id: string) =>
    req<Comment[]>('GET', `/api/threads/${encodeURIComponent(id)}/comments`),
  postComment: (id: string, body: string) =>
    req<Comment>('POST', `/api/threads/${encodeURIComponent(id)}/comments`, { body }),
  // A failed post returns 202 with the draft; retry re-sends it with the live
  // session credential (PLANE-SYNC.md Phase 5).
  retryDraft: (id: string, draftId: number) =>
    req<Comment>('POST', `/api/threads/${encodeURIComponent(id)}/drafts/${draftId}/retry`),
  discardDraft: (id: string, draftId: number) =>
    req<void>('DELETE', `/api/threads/${encodeURIComponent(id)}/drafts/${draftId}`),
  // Artifact editing (docs/ARTIFACT-EDITING.md). A field you omit is untouched;
  // `base` carries the region hashes read, so a write that lost a race gets a
  // 409 instead of silently clobbering somebody else's edit.
  updateThread: (
    id: string,
    edit: {
      title?: string;
      brief?: string;
      logbook?: string;
      dod?: string;
      base?: Partial<Record<RegionName, string>>;
      /** Downgrade a markdown-standard violation to a warning. The editor sets
       *  it because autosave that stops mid-sentence is its own kind of broken;
       *  every other surface is refused, agents included. */
      lenient?: boolean;
    },
  ) => req<ThreadDetail>('PATCH', `/api/threads/${encodeURIComponent(id)}`, edit),
  // text guards index: the server refuses rather than ticking the wrong box.
  toggleTodo: (id: string, region: RegionName, index: number, text: string, done: boolean) =>
    req<ThreadDetail>('POST', `/api/threads/${encodeURIComponent(id)}/todo`, {
      region,
      index,
      text,
      done,
    }),

  markCommentsRead: (id: string, commentIds: string[]) =>
    req<{ ok: boolean }>('POST', `/api/threads/${encodeURIComponent(id)}/comments/read`, {
      comment_ids: commentIds,
    }),

  createBubble: (input: {
    instance: string;
    project: string;
    name: string;
    outcome?: string;
    owner?: string;
  }) => req<{ id: string; name: string }>('POST', '/api/bubbles', input),
  birth: (input: {
    instance: string;
    bubble_id: string;
    name: string;
    brief: string;
    logbook: string;
    small_thread: boolean;
  }) => req<{ thread_id: string; created: boolean; message: string }>('POST', '/api/threads/birth', input),

  // A Workspace is the boundary for a body of work — a Plane PROJECT, not a
  // Plane workspace. Its id is namespaced "<instance>:<project>".
  //
  // This is the ONLY list that includes an empty workspace. The board is built
  // from bubbles and skips a project with no modules, so a workspace nothing has
  // been put in yet cannot be inferred from it.
  workspaces: () => req<Workspace[]>('GET', '/api/workspaces'),
  createWorkspace: (input: { instance: string; name: string; identifier?: string }) =>
    req<{ id: string; name: string; identifier: string; instance: string }>(
      'POST',
      '/api/workspaces',
      input,
    ),
  renameWorkspace: (id: string, name: string) =>
    req<{ id: string; name: string }>('PATCH', `/api/workspaces/${encodeURIComponent(id)}`, {
      name,
    }),
  // The counts come back from the server, which took them BEFORE deleting —
  // afterwards there is nothing left to count.
  deleteWorkspace: (id: string) =>
    req<{ deleted_bubbles: number; deleted_threads: number }>(
      'DELETE',
      `/api/workspaces/${encodeURIComponent(id)}`,
    ),

  // Project pages: a workspace's documentation. Read live from Plane rather
  // than from the mirror — a page is opened deliberately, one at a time.
  pages: (workspaceId: string) =>
    req<Page[]>('GET', `/api/workspaces/${encodeURIComponent(workspaceId)}/pages`),
  page: (id: string) => req<PageDetail>('GET', `/api/pages/${encodeURIComponent(id)}`),
  createPage: (workspaceId: string, title: string, markdown: string) =>
    req<PageDetail>('POST', `/api/workspaces/${encodeURIComponent(workspaceId)}/pages`, {
      title,
      markdown,
    }),
  updatePage: (id: string, patch: { title?: string; markdown?: string; base_hash?: string }) =>
    req<PageDetail>('PATCH', `/api/pages/${encodeURIComponent(id)}`, patch),
  deletePage: (id: string) => req<{ ok: boolean }>('DELETE', `/api/pages/${encodeURIComponent(id)}`),

  // Re-home a thread. Nothing is lost: the Brief, Logbook, comments, history and
  // id survive, and it leaves every other bubble, so this is a move not a copy.
  moveThread: (id: string, bubbleId: string) =>
    req<ThreadDetail>('POST', `/api/threads/${encodeURIComponent(id)}/move`, {
      bubble_id: bubbleId,
    }),

  // Only the handle changes: the contract, the stage and every thread inside are
  // untouched, and no heat is earned — a name is not what has been done.
  renameBubble: (id: string, name: string) =>
    req<{ id: string; name: string }>('PATCH', `/api/bubbles/${encodeURIComponent(id)}`, { name }),

  // Deleting is IRREVERSIBLE and deletes from Plane. Closing a bubble keeps the
  // record of what was done and is almost always the right verb.
  deleteBubble: (id: string) =>
    req<{ unbubbled_threads: number }>('DELETE', `/api/bubbles/${encodeURIComponent(id)}`),
  deleteThread: (id: string) =>
    req<{ deleted_revisions: number }>('DELETE', `/api/threads/${encodeURIComponent(id)}`),
  deleteRegion: (id: string, region: RegionName) =>
    req<ThreadDetail>('DELETE', `/api/threads/${encodeURIComponent(id)}/regions/${region}`),

  review: (id: string) => req<void>('POST', `/api/bubbles/${id}/review`),
  unreview: (id: string) => req<void>('POST', `/api/bubbles/${id}/unreview`),
  close: (id: string) => req<void>('POST', `/api/bubbles/${id}/close`),
  reopen: (id: string) => req<void>('POST', `/api/bubbles/${id}/reopen`),
  contract: (id: string, patch: { outcome?: string; owner?: string }) =>
    req<void>('POST', `/api/bubbles/${id}/contract`, patch),

  // service-admin (godmode) — server gates these on ServiceAdmin (§ admin).
  adminStats: () => req<AdminStats>('GET', '/api/admin/stats'),
  adminInstances: () => req<AdminInstance[]>('GET', '/api/admin/instances'),
  adminBubbles: () => req<BubbleView[]>('GET', '/api/admin/bubbles'),
  adminRefresh: () => req<void>('POST', '/api/admin/refresh'),
  adminTick: () => req<void>('POST', '/api/admin/tick'),
  adminMembers: () => req<InstanceMembers[]>('GET', '/api/admin/members'),
  adminKiosk: () => req<KioskToken[]>('GET', '/api/admin/kiosk'),
  adminKioskCreate: (instance: string, name: string) =>
    req<KioskToken>('POST', '/api/admin/kiosk', { instance, name }),
  adminKioskRevoke: (token: string) =>
    req<void>('DELETE', `/api/admin/kiosk/${encodeURIComponent(token)}`),
  adminAutoState: (slug: string, enabled: boolean) =>
    req<{ instance: string; auto_state: boolean }>(
      'POST',
      `/api/admin/instances/${encodeURIComponent(slug)}/autostate`,
      { enabled },
    ),
  adminTuning: () => req<TuningView>('GET', '/api/admin/tuning'),
  // a partial body patches only the keys it names; the server clamps and persists
  adminTuningSet: (patch: Record<string, number | boolean>) =>
    req<TuningView>('PUT', '/api/admin/tuning', patch),

  // Plane mirror (PLANE-SYNC.md). Status is a local census — cheap. Diff and
  // backfill both walk Plane completely and can take minutes on a large
  // workspace, hence POST rather than GET: neither should be prefetchable.
  adminSync: (slug: string) =>
    req<SyncStatus>('GET', `/api/admin/sync/${encodeURIComponent(slug)}`),
  adminSyncDiff: (slug: string) =>
    req<SyncDiff>('POST', `/api/admin/sync/${encodeURIComponent(slug)}/diff`),
  // GET, unlike diff: fidelity reads only the mirror, so it spends no rate budget.
  adminSyncFidelity: (slug: string) =>
    req<SyncFidelity>('GET', `/api/admin/sync/${encodeURIComponent(slug)}/fidelity`),
  adminSyncBackfill: (slug: string) =>
    req<SyncResult>('POST', `/api/admin/sync/${encodeURIComponent(slug)}/backfill`),
  adminOutbox: () => req<OutboxView>('GET', '/api/admin/outbox'),
  adminOutboxDrop: (id: number) => req<void>('DELETE', `/api/admin/outbox/${id}`),
};
