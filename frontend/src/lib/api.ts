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
      throw new Error(body?.message || res.statusText || `HTTP ${res.status}`);
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
