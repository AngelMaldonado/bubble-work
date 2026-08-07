// Mirrors the server's JSON contracts (internal/domain/domain.go).
import { i18n, type MsgKey } from './i18n.svelte';

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
  reason: string; // English, from the server
  reason_code?: string;
  reason_args?: Record<string, string>;
  outcome?: string;
  owner?: string;
  members?: string[]; // thread assignees + contract owner (per-assignee boards)
  threads: number;
  /** per-thread levels rolled up, e.g. {in_progress: 2, zzzz: 1} (THREAD-LIFECYCLE.md) */
  thread_levels?: Partial<Record<Level, number>>;
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

/** A thread's own derived lifecycle (THREAD-LIFECYCLE.md Phase A), flattened
 *  into the thread DTOs by the server. */
export interface Buoyancy {
  lifecycle: Lifecycle;
  level: Level;
  score: number;
  reason: string; // English, from the server
  /** stable key + params so the client renders the sentence in its own
   *  language (internal/heat: Reason* codes) */
  reason_code?: string;
  reason_args?: Record<string, string>;
}

export interface ThreadNode extends Buoyancy {
  id: string; // namespaced slug:project:workitem
  seq: number;
  title: string;
  active: boolean;
  owner?: string;
  parent?: string;
  state?: string; // Plane state name (localized — display only)
  state_group?: string; // backlog|unstarted|started|completed|cancelled
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

export interface ThreadDetail extends Buoyancy {
  id: string;
  seq: number;
  title: string;
  kind: 'simple' | 'phased';
  active: boolean;
  priority?: string;
  assignees?: string[];
  state?: string; // Plane state name (localized — display only)
  state_group?: string; // backlog|unstarted|started|completed|cancelled
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
  /** an unsent draft kept after a failed post (PLANE-SYNC.md Phase 5). It holds
   *  no credential, so only its author can re-send it — with their live key,
   *  which is also the only way Plane records the right author. */
  pending?: boolean;
  draft_id?: number;
  error?: string;
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

/** One instance's live Plane rate-limit state. Plane allows 60 req/min per API
 *  key and reports the remaining allowance on every response (PLANE-SYNC.md). */
export interface RateBudget {
  instance: string;
  known: boolean; // false until a response carried the headers
  remaining: number; // -1 when unknown
  limit: number; // inferred ceiling (largest remaining seen)
  reset_in: number; // seconds until the window rolls over
  throttled: number; // 429s observed since start
  waits: number; // times background work yielded to the floor
  spent: number; // requests issued since start
  floor: number; // allowance reserved for interactive work
}

export interface AdminStats {
  instances: number;
  cached_instances: number;
  cached_identities: number;
  revision: string;
  built: string;
  started_at: string;
  rate_budgets?: RateBudget[];
}

export interface AdminInstance {
  slug: string;
  name: string;
  base_url: string;
  workspace: string;
  project: string;
  has_webhook: boolean;
  cached: boolean;
  auto_state: boolean; // writes derived levels back to Plane (Phase B)
}

// ---- Plane mirror (docs/PLANE-SYNC.md) ----

/** One instance's mirror census and cursor. Cheap: no Plane calls. */
/** One instance's sync freshness. */
export interface InstanceStatus {
  instance: string;
  stale: boolean;
  last_ok?: string;
  behind_seconds?: number;
  last_error?: string;
}

/** Whether what you are looking at can be trusted to be current
 *  (PLANE-SYNC.md Phase 7). Reads come from a local mirror, so a stopped sync
 *  would otherwise leave the board rendering confidently from ageing data.
 *  Being behind is fine; being behind silently is not. */
export interface ServiceStatus {
  stale: boolean;
  reason?: string;
  instances?: InstanceStatus[];
  unsent_drafts?: number;
}

/** One write that has not reached Plane. */
export interface OutboxItem {
  id: number;
  instance: string;
  kind: string; // state | comment
  target_id: string;
  author?: string;
  status: string; // pending | abandoned
  attempts: number;
  field_lock?: string;
  last_error?: string;
  created_at?: string;
  next_at?: string;
  summary: string;
}

/** The queue of unsent writes. An abandoned entry stays visible on purpose:
 *  silently dropping a write is the one thing an outbox must never do. */
export interface OutboxView {
  pending: number;
  abandoned: number;
  entries?: OutboxItem[];
}

export interface SyncStatus {
  instance: string;
  projects: number;
  modules: number;
  items: number;
  states: number;
  members: number;
  comments: number;
  watermark?: string; // newest updated_at applied
  last_full?: string; // last complete reconcile
  last_ok?: string; // last successful pass
  last_error?: string; // why the mirror may be stale
}

export interface SyncResult {
  instance: string;
  full: boolean;
  projects: number;
  modules: number;
  items: number;
  comments: number;
  pruned: number;
  watermark?: string;
  took_ms: number;
  /** some projects could not be walked completely (usually a 429); what
   *  arrived is kept, prunes and the watermark are held back */
  partial?: boolean;
  errors?: string[];
}

export interface SyncFinding {
  kind: string; // missing-in-mirror | missing-in-plane | field
  scope: string; // module | membership | item
  id: string;
  label: string;
  field?: string;
  plane?: string;
  local?: string;
  text: string; // pre-rendered one-liner, so every surface agrees
}

/** The mirror compared against a live fetch — the Phase 1 acceptance gate.
 *  Expensive: it runs the very calls the mirror exists to remove. */
export interface SyncDiff {
  instance: string;
  projects: number;
  modules: number;
  items: number;
  clean: boolean;
  findings?: SyncFinding[];
  watermark?: string;
  last_full?: string;
  last_ok?: string;
  last_error?: string;
  took_ms: number;
}

export interface KioskToken {
  token: string;
  instance: string;
  name: string;
  created_at: string;
}

export interface Member {
  id: string;
  name: string;
  email: string;
  role: number;
  admin: boolean;
}

export interface InstanceMembers {
  instance: string;
  name: string;
  members: Member[];
  error?: string;
}

/** The buoyancy calibration — every threshold the lifecycle rules use, at both
 *  grains. Keys match the server's JSON tags, so a partial patch is just
 *  `{key: value}` (internal/domain/domain.go). */
export interface Tuning {
  cycle_hours: number;
  dormant_cycles: number;
  decay_cycles: number;
  ownerless_is_dormant: boolean;
  bubble_rip_needs_owner: boolean;
  bubble_level_rollup: boolean;
  thread_birth_heats: boolean;
  thread_grace_cycles: number;
  thread_rip_needs_owner: boolean;
  pulse_cycles: number;
  thread_terminal_state_wins: boolean;
}

export type TuningKey = keyof Tuning;

/** Self-describing schema for one knob — the server owns the labels and help so
 *  the CLI and God Mode never drift. */
export interface TuningField {
  key: TuningKey;
  label: string;
  help: string;
  kind: 'number' | 'toggle';
  group: 'pulse' | 'bubble' | 'thread';
  min?: number;
  max?: number;
  step?: number;
}

export interface TuningView {
  tuning: Tuning;
  defaults: Tuning;
  fields: TuningField[];
}

export const TUNING_GROUPS: { key: TuningField['group']; label: string; hint: string }[] = [
  { key: 'pulse', label: 'Pulse', hint: 'the rhythm both grains are measured against' },
  { key: 'bubble', label: 'Bubbles', hint: 'how a bubble reaches 🪦 vs 😴' },
  { key: 'thread', label: 'Threads', hint: 'how a single work item moves between bands' },
];

export const LEVELS: { key: Level; label: string; icon: string }[] = [
  { key: 'in_progress', label: 'In progress', icon: '🔥' },
  { key: 'reviewed', label: 'Reviewed', icon: '👀' },
  { key: 'zzzz', label: 'Zzzz', icon: '😴' },
  { key: 'rip', label: 'RIP', icon: '🪦' },
  { key: 'done', label: 'Done', icon: '🏆' },
];

const BY_LEVEL = new Map(LEVELS.map((l) => [l.key as string, l]));

/** Band icon for a level — used for bubbles AND for a single thread's own
 *  buoyancy (THREAD-LIFECYCLE.md). Unknown/absent → a neutral dot. */
export function levelIcon(level?: string): string {
  return BY_LEVEL.get(level ?? '')?.icon ?? '•';
}

/** Band name in the reader's language. LEVELS keeps the English label as the
 *  fallback so a level this build does not know still renders as something. */
export function levelLabel(level?: string): string {
  const l = BY_LEVEL.get(level ?? '');
  if (!l) return '—';
  const key = `band.${l.key}` as MsgKey;
  return i18n.t(key);
}

/** Ordered [level, count] pairs from a thread-level roll-up, hottest first. */
export function rollup(levels?: Partial<Record<Level, number>>): [Level, number][] {
  if (!levels) return [];
  return LEVELS.map((l) => [l.key, levels[l.key] ?? 0] as [Level, number]).filter(([, n]) => n > 0);
}
