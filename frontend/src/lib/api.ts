// The one place that talks to the server.
//
// Two kinds of call live here and they are not the same thing: PocketBase's own
// record API for structure, and Bubble's routes for everything the rules cannot
// express — the board, documents, the tree, search, assets. Both are the same
// origin (vite proxies in development), so nothing here deals with CORS.

export type Lifecycle = 'hot' | 'dormant' | 'rip' | 'closed';

export type Heat = {
  lifecycle: Lifecycle;
  score: number;
  reason: string;
  code: string;
  args?: Record<string, string>;
};

export type ThreadHeat = {
  id: string;
  seq: number;
  name: string;
  bubble?: string;
  priority?: string;
  heat: Heat;
  pulse: boolean;
};

export type BubbleHeat = {
  id: string;
  name: string;
  owner?: string;
  outcome?: string;
  closed: boolean;
  heat: Heat;
};

export type Board = {
  workspace: string;
  at: string;
  tuning: Record<string, number | boolean>;
  bubbles: BubbleHeat[];
  threads: ThreadHeat[];
};

export type Workspace = { id: string; name: string; slug: string };

/** A column of the planner's board: the workflow the workspace defined. */
export type State = {
  id: string;
  name: string;
  group: 'backlog' | 'unstarted' | 'started' | 'completed' | 'cancelled';
  position: number;
  is_default: boolean;
};

/** A thread as the planner reads it — the record, not the board's heat view. */
export type ThreadRecord = {
  id: string;
  seq: number;
  name: string;
  workspace: string;
  bubble?: string;
  state?: string;
  objective?: string;
  due_date?: string;
  impact?: string;
  urgency?: string;
  completed_at?: string;
};

export type Objective = {
  id: string;
  name: string;
  outcome?: string;
  due_date?: string;
  position?: number;
  closed_at?: string;
};

export type InboxItem = {
  id: string;
  note: string;
  captured_by: string;
  thread?: string;
  created: string;
};
export type Person = { id: string; email: string; display_name?: string; role?: string };

export type Doc = {
  workspace: string;
  path: string;
  thread?: string;
  hash: string;
  /** the markdown — the record, and what an `edits` quote must match */
  content: string;
  /** the same text rendered by the SERVER, so every surface shows one thing */
  html: string;
  done: number;
};

export type Entry = {
  path: string;
  name: string;
  title?: string;
  dir: boolean;
  area?: 'thread' | 'doc' | 'asset';
  size?: number;
};

const TOKEN = 'bubble.token';

/**
 * A failed call, with the status kept.
 *
 * The status is what tells a caller WHICH failure this is: 409 means somebody
 * wrote first and the honest answer is to offer a reload, not to show the
 * sentence and lose the edit. Matching on the message would work until the
 * message is reworded.
 */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }

  get conflict() {
    return this.status === 409;
  }
}

class Api {
  token = localStorage.getItem(TOKEN) ?? '';
  me: Person | null = null;

  private async call<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    if (this.token) headers.set('Authorization', this.token);
    if (init.body && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json');
    }
    const res = await fetch(path, { ...init, headers });
    const text = await res.text();
    let body: any = null;
    try {
      body = text ? JSON.parse(text) : null;
    } catch {
      body = text;
    }
    if (!res.ok) {
      // The server's own sentence is almost always the useful one — "quote more
      // of it", "the document changed since you read it". Do not replace it.
      const err = new ApiError(body?.message || res.statusText || `HTTP ${res.status}`, res.status);
      throw err;
    }
    return body as T;
  }

  get signedIn() {
    return !!this.token;
  }

  async signIn(identity: string, password: string) {
    const out = await this.call<{ token: string; record: Person }>(
      '/api/collections/users/auth-with-password',
      { method: 'POST', body: JSON.stringify({ identity, password }) },
    );
    this.token = out.token;
    this.me = out.record;
    localStorage.setItem(TOKEN, out.token);
    return out.record;
  }

  signOut() {
    this.token = '';
    this.me = null;
    localStorage.removeItem(TOKEN);
  }

  async refresh(): Promise<Person | null> {
    if (!this.token) return null;
    try {
      const out = await this.call<{ token: string; record: Person }>(
        '/api/collections/users/auth-refresh',
        { method: 'POST' },
      );
      this.token = out.token;
      this.me = out.record;
      localStorage.setItem(TOKEN, out.token);
      return out.record;
    } catch {
      this.signOut();
      return null;
    }
  }

  async workspaces() {
    const out = await this.call<{ items: Workspace[] }>(
      '/api/collections/workspaces/records?perPage=200&sort=name',
    );
    return out.items;
  }

  createWorkspace(name: string, slug: string) {
    return this.call<Workspace>('/api/collections/workspaces/records', {
      method: 'POST',
      body: JSON.stringify({ name, slug }),
    });
  }

  renameWorkspace(id: string, name: string) {
    return this.call<Workspace>(`/api/collections/workspaces/records/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    });
  }

  // The slug is NOT renamed with it. It is the workspace's address on disk —
  // the repository's directory — and a name people change on a Tuesday must not
  // move a git repo.
  deleteWorkspace(id: string) {
    return this.call<unknown>(`/api/collections/workspaces/records/${id}`, { method: 'DELETE' });
  }

  createThread(fields: { workspace: string; bubble?: string; name: string }) {
    return this.call<{ id: string; seq: number }>('/api/collections/threads/records', {
      method: 'POST',
      body: JSON.stringify(fields),
    });
  }

  deleteThread(id: string) {
    return this.call<unknown>(`/api/collections/threads/records/${id}`, { method: 'DELETE' });
  }

  createBubble(fields: { workspace: string; name: string; outcome?: string }) {
    return this.call<{ id: string }>('/api/collections/bubbles/records', {
      method: 'POST',
      body: JSON.stringify(fields),
    });
  }

  // ---- the planner ---------------------------------------------------------
  //
  // Plain record calls: the planner reads and writes the same collections the
  // board does, and every rule that guards them is the server's. What is NOT
  // here is a card: the kanban's columns are `states` and what moves across
  // them is a thread.
  states(workspace: string) {
    return this.list<State>('states', workspace, 'position,name');
  }

  threads(workspace: string) {
    return this.list<ThreadRecord>('threads', workspace, '-created');
  }

  /** Every thread this person can see, across every workspace. What the
   *  department's planner is looking at; the rules decide what "can see" means
   *  and the global lead's includes everything. */
  async allThreads() {
    const out = await this.call<{ items: ThreadRecord[] }>(
      '/api/collections/threads/records?perPage=500&sort=-created',
    );
    return out.items;
  }

  /** …and their derived priorities, same scope. */
  async allPriorities() {
    const out = await this.call<{ items: { id: string; priority: string }[] }>(
      '/api/collections/thread_priority/records?perPage=500',
    );
    return out.items;
  }

  // No workspace: the plan belongs to the DEPARTMENT. Everybody signed in reads
  // it, the global lead shapes it, and a thread from any project can hang from
  // any objective — which is what makes the objective worth stating.
  async objectives() {
    const out = await this.call<{ items: Objective[] }>(
      '/api/collections/objectives/records?perPage=500&sort=position,created',
    );
    return out.items;
  }

  bubbles(workspace: string) {
    return this.list<{ id: string; name: string; closed_at?: string }>('bubbles', workspace, 'name');
  }

  /** The derived priorities, by thread. A VIEW collection: the server computes
   *  it from impact × urgency, and nothing writes to it. */
  priorities(workspace: string) {
    return this.list<{ id: string; priority: string }>('thread_priority', workspace, 'id');
  }

  /** The department's inbox: captured before anybody knows whose it is. */
  async inbox() {
    const out = await this.call<{ items: InboxItem[] }>(
      '/api/collections/inbox_items/records?perPage=500&sort=-created',
    );
    return out.items;
  }

  private async list<T>(collection: string, workspace: string, sort: string) {
    const out = await this.call<{ items: T[] }>(
      `/api/collections/${collection}/records?perPage=500&sort=${sort}` +
        `&filter=${encodeURIComponent(`workspace='${workspace}'`)}`,
    );
    return out.items;
  }

  create<T>(collection: string, fields: Record<string, unknown>) {
    return this.call<T>(`/api/collections/${collection}/records`, {
      method: 'POST',
      body: JSON.stringify(fields),
    });
  }

  update<T>(collection: string, id: string, fields: Record<string, unknown>) {
    return this.call<T>(`/api/collections/${collection}/records/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(fields),
    });
  }

  remove(collection: string, id: string) {
    return this.call<unknown>(`/api/collections/${collection}/records/${id}`, { method: 'DELETE' });
  }

  board(workspace: string) {
    return this.call<Board>(`/api/workspaces/${workspace}/board`);
  }

  tree(workspace: string) {
    return this.call<{ entries: Entry[] }>(`/api/workspaces/${workspace}/tree`);
  }

  search(workspace: string, q: string) {
    return this.call<{ hits: any[] }>(
      `/api/workspaces/${workspace}/search?q=${encodeURIComponent(q)}`,
    );
  }

  /** Markdown → HTML by the server's renderer. The browser has none, and a
   *  second one is how two screens show the same document differently. */
  async renderMarkdown(content: string) {
    const out = await this.call<{ html: string }>('/api/markdown', {
      method: 'POST',
      body: JSON.stringify({ content }),
    });
    return out.html;
  }

  readThread(thread: string) {
    return this.call<Doc>(`/api/threads/${thread}/document`);
  }

  readPath(workspace: string, path: string) {
    return this.call<Doc>(
      `/api/workspaces/${workspace}/document?path=${encodeURIComponent(path)}`,
    );
  }

  // Every write goes through here, and every write carries the hash it read.
  // That is the only thing standing between two editors and a lost paragraph.
  patchThread(thread: string, patch: Record<string, unknown>) {
    return this.call<Doc>(`/api/threads/${thread}/document`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    });
  }

  patchPath(workspace: string, path: string, patch: Record<string, unknown>) {
    return this.call<Doc>(`/api/workspaces/${workspace}/document`, {
      method: 'PATCH',
      body: JSON.stringify({ path, ...patch }),
    });
  }
}

export const api = new Api();
