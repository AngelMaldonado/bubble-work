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
  Comment,
  TuningView,
  SyncStatus,
  SyncDiff,
  SyncFidelity,
  SyncResult,
  OutboxView,
  ServiceStatus,
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
