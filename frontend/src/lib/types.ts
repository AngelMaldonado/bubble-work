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
  read_only?: boolean; // kiosk display token
  instances: string[];
}

export interface BubbleView {
  id: string;
  name: string;
  instance: string;
  project: string;
  project_name?: string;
  lifecycle: Lifecycle;
  level: Level;
  score: number;
  reason: string;
  outcome?: string;
  owner?: string;
  members?: string[]; // thread assignees + contract owner (per-assignee boards)
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

// ---- Thread interior (INTERIOR-PLAN.md) ----

export interface ThreadNode {
  id: string; // namespaced slug:project:workitem
  seq: number;
  title: string;
  active: boolean;
  owner?: string;
  parent?: string;
  created_at: string;
  completed_at?: string;
}

export interface TOCEntry {
  level: number;
  title: string;
  slug: string;
}

export interface Todo {
  text: string;
  done: boolean;
}

export interface Artifact {
  title: string;
  toc: TOCEntry[];
  markdown: string;
  html: string;
}

export interface Logbook {
  markdown: string;
  html: string;
  todos: Todo[];
  dod?: Todo[];
  dod_html?: string;
  phased: boolean;
}

export interface ThreadDetail {
  id: string;
  seq: number;
  title: string;
  kind: 'simple' | 'phased';
  active: boolean;
  priority?: string;
  assignees?: string[];
  artifacts: Artifact[];
  logbook?: Logbook;
  revisions: Artifact[];
  created_at: string;
  completed_at?: string;
}

export interface Reader {
  id: string;
  name: string;
}

export interface Comment {
  id: string;
  author: string;
  author_id?: string;
  markdown: string;
  html: string;
  mine: boolean;
  readers?: Reader[];
  created_at: string;
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

export interface AdminStats {
  instances: number;
  cached_instances: number;
  cached_identities: number;
  revision: string;
  built: string;
  started_at: string;
}

export interface AdminInstance {
  slug: string;
  name: string;
  base_url: string;
  workspace: string;
  project: string;
  has_webhook: boolean;
  cached: boolean;
}

export const LEVELS: { key: Level; label: string; icon: string }[] = [
  { key: 'in_progress', label: 'In progress', icon: '🔥' },
  { key: 'reviewed', label: 'Reviewed', icon: '👀' },
  { key: 'zzzz', label: 'Zzzz', icon: '😴' },
  { key: 'rip', label: 'RIP', icon: '🪦' },
  { key: 'done', label: 'Done', icon: '🏆' },
];
