// Thin typed client over the server REST surface (§9). The browser presents the
// same credential as the CLI — a Plane API key — as a Bearer token.
import type {
  Actor,
  BubbleView,
  ThreadHit,
  Inbox,
  AdminStats,
  AdminInstance,
  ThreadNode,
  ThreadDetail,
  Comment,
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

  // interior read model (INTERIOR-PLAN.md)
  timeline: (bubbleId: string) =>
    req<ThreadNode[]>('GET', `/api/bubbles/${encodeURIComponent(bubbleId)}/threads`),
  thread: (id: string) => req<ThreadDetail>('GET', `/api/threads/${encodeURIComponent(id)}`),
  comments: (id: string) =>
    req<Comment[]>('GET', `/api/threads/${encodeURIComponent(id)}/comments`),
  postComment: (id: string, body: string) =>
    req<Comment>('POST', `/api/threads/${encodeURIComponent(id)}/comments`, { body }),
  markCommentsRead: (id: string, commentIds: string[]) =>
    req<{ ok: boolean }>('POST', `/api/threads/${encodeURIComponent(id)}/comments/read`, {
      comment_ids: commentIds,
    }),

  createBubble: (instance: string, workspace: string, name: string) =>
    req<{ id: string }>('POST', '/api/bubbles', { instance, workspace, name }),
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
};
