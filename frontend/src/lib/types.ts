// Mirrors the server's JSON contracts (internal/domain/domain.go).

export type Level = 'in_progress' | 'reviewed' | 'zzzz' | 'rip' | 'done';

export type Lifecycle = 'hot' | 'warm' | 'cooling' | 'dormant' | 'closed';

export interface Actor {
  id: string;
  name: string;
  kind: string; // human | agent | admin
  email?: string;
  admin: boolean;
  service_admin?: boolean;
  instances: string[];
}

export interface BubbleView {
  id: string;
  name: string;
  instance: string;
  lifecycle: Lifecycle;
  level: Level;
  score: number;
  reason: string;
  outcome?: string;
  owner?: string;
  threads: number;
}

export interface ThreadHit {
  id: string;
  name: string;
  bubble_id: string;
  bubble_name: string;
  instance: string;
  open: boolean;
}

export interface Notification {
  id: number;
  at: string;
  instance: string;
  bubble_id: string;
  bubble_name: string;
  kind: string; // cooling | dormant
  message: string;
  unread: boolean;
}

export interface Inbox {
  enabled: boolean;
  unread_count: number;
  notifications: Notification[];
}

export const LEVELS: { key: Level; label: string; icon: string }[] = [
  { key: 'in_progress', label: 'In progress', icon: '🔥' },
  { key: 'reviewed', label: 'Reviewed', icon: '👀' },
  { key: 'zzzz', label: 'Zzzz', icon: '😴' },
  { key: 'rip', label: 'RIP', icon: '🪦' },
  { key: 'done', label: 'Done', icon: '🏆' },
];
