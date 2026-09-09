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
  /** de qué workspace es esta fila. Redundante en el board de uno, y lo único
   *  que la hace abrible en el board de todos: la dirección se hace del slug de
   *  su workspace y del seq. */
  workspace: string;
  name: string;
  bubble?: string;
  priority?: string;
  heat: Heat;
  pulse: boolean;
  /** who has it */
  assignees?: string[];
  /** when anything last happened to it, RFC3339 — the row says it in words */
  at?: string;
};

export type BubbleHeat = {
  id: string;
  workspace: string;
  name: string;
  /** who is accountable. Plural: the 🪦 band asks whether ANYBODY is. */
  owners?: string[];
  outcome?: string;
  closed: boolean;
  /** how it ended, if it did */
  closure?: string;
  /** when it last produced anything, RFC3339 — what the cycle bar measures */
  warm_at?: string;
  heat: Heat;
};

export type Board = {
  /** el workspace del board, o «» cuando el board es TODOS */
  workspace: string;
  at: string;
  tuning: Record<string, number | boolean>;
  /** de qué se compuso este board. Siempre viene, y en el board de un
   *  workspace trae ese solo: un cliente que tenga que distinguir «campo
   *  ausente» de «lista vacía» es un cliente con dos formas que atender. */
  workspaces: BoardWorkspace[];
  bubbles: BubbleHeat[];
  threads: ThreadHeat[];
};

/** Lo justo de un workspace para etiquetar una fila y para construir la
 *  dirección que la abre. */
export type BoardWorkspace = { id: string; slug: string; name: string };

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

/** One person's place in one workspace. `id` is the MEMBERSHIP's — that is what
 *  a role change or a removal edits — and `user` is the person's. */
export type Member = {
  id: string;
  user: string;
  name: string;
  role: 'lead' | 'member';
};

/** Lo que alguien dijo en un thread. El cuerpo es markdown: lo renderiza el
 *  mismo servidor que el documento, porque dos renderers coinciden hasta que
 *  dejan de hacerlo. */
export type Comment = {
  id: string;
  thread: string;
  author: string;
  name: string;
  body: string;
  created: string;
  mine: boolean;
};

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

/** Cuánto se escribió en un archivo dentro de una ventana. Sumado y quitado por
 *  separado: «+120 −4» y «+124 −8» son dos tardes distintas, y un neto las
 *  esconde. */
export type Churn = { path: string; added: number; removed: number };

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
    // FormData carries its own multipart boundary in the Content-Type. Setting
    // JSON over it produces a body the server cannot parse and an error that
    // blames the upload.
    if (init.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
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

  /** Who works HERE: the workspace's roster, membership row and all.
   *
   *  Read from `memberships` rather than from `users` on purpose. Everybody
   *  signed in can list people — that is what makes inviting possible — but the
   *  question a workspace asks is not "who exists", it is "who is in this", and
   *  that is a membership. The row's own id comes back because changing a role
   *  or removing somebody edits THE MEMBERSHIP, not the person. */
  async members(workspace: string): Promise<Member[]> {
    const out = await this.call<{
      items: {
        id: string;
        user: string;
        role: 'lead' | 'member';
        expand?: { user?: { display_name?: string; email?: string } };
      }[];
    }>(
      `/api/collections/memberships/records?perPage=200&expand=user` +
        `&filter=${encodeURIComponent(`workspace='${workspace}'`)}`,
    );
    return out.items.map((m) => ({
      id: m.id,
      user: m.user,
      role: m.role,
      // `display_name` no es obligatorio, y PocketBase OCULTA el correo de otra
      // persona salvo que ella lo haya hecho visible — así que el correo es un
      // segundo intento, no el respaldo. Lo último es decirlo, no imprimir un id.
      name: m.expand?.user?.display_name || m.expand?.user?.email || 'sin nombre',
    }));
  }

  /** The same roster, shaped for a field that only needs to name a person. */
  async roster(workspace: string) {
    const rows = await this.members(workspace);
    return rows.map((m) => ({ id: m.user, name: m.name }));
  }

  /** Everybody with an account. Listing people is open to anybody signed in —
   *  that is what makes an invitation possible — and it is deliberately NOT the
   *  same list as a workspace's roster. */
  async people() {
    const out = await this.call<{ items: Person[] }>(
      '/api/collections/users/records?perPage=500&sort=display_name,email',
    );
    return out.items;
  }

  /** Lo dicho en un thread, del más viejo al más nuevo: una conversación se lee
   *  hacia abajo, y la última línea es la que estabas esperando. */
  async comments(thread: string): Promise<Comment[]> {
    const out = await this.call<{
      items: {
        id: string; thread: string; author: string; body: string; created: string;
        expand?: { author?: { display_name?: string; email?: string } };
      }[];
    }>(
      `/api/collections/comments/records?perPage=500&sort=created&expand=author` +
        `&filter=${encodeURIComponent(`thread='${thread}'`)}`,
    );
    return out.items.map((c) => ({
      id: c.id,
      thread: c.thread,
      author: c.author,
      name: c.expand?.author?.display_name || c.expand?.author?.email || 'alguien',
      body: c.body,
      created: c.created,
      mine: c.author === this.me?.id,
    }));
  }

  /** El autor NO se manda: lo estampa el servidor, y por eso nadie firma como
   *  otro. Mandarlo desde aquí sería pedir permiso para algo ya decidido. */
  comment(thread: string, body: string) {
    return this.create<{ id: string }>('comments', { thread, body });
  }

  /** "Sigo aquí." The server stamps the time; this only says who asked. */
  beat() {
    return this.call<{ at: string }>('/api/presence', { method: 'POST' });
  }

  /** Claim a realtime stream as this person, and say what to hear about.
   *
   *  The EventSource that opened the stream is ANONYMOUS — it cannot carry a
   *  header — so this is where the token arrives and where the connection stops
   *  being a stranger's. Sending it again with a different list replaces the
   *  subscriptions rather than adding to them, which is PocketBase's contract
   *  and not ours. */
  subscribe(clientId: string, subscriptions: string[]) {
    return this.call<unknown>('/api/realtime', {
      method: 'POST',
      body: JSON.stringify({ clientId, subscriptions }),
    });
  }

  /** Who has said it lately. One row per person, so this is small by
   *  construction; WHAT counts as online is decided by the reader, not stored. */
  async presence() {
    const out = await this.call<{
      items: { user: string; at: string; expand?: { user?: { display_name?: string; email?: string } } }[];
    }>('/api/collections/presence/records?perPage=200&expand=user');
    return out.items.map((r) => ({
      id: r.user,
      at: r.at,
      name: r.expand?.user?.display_name || r.expand?.user?.email || 'alguien',
    }));
  }

  /** An invitation is a ROW: one person, one workspace, one role. The server
   *  decides who may write it — this workspace's lead, or the global one. */
  invite(workspace: string, user: string, role: 'lead' | 'member' = 'member') {
    return this.create<{ id: string }>('memberships', { workspace, user, role });
  }

  setRole(membership: string, role: 'lead' | 'member') {
    return this.update('memberships', membership, { role });
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

  /** TODOS: las burbujas de cada workspace que alcanzas, en un solo board.
   *
   *  Lo agrega el servidor y no este cliente. Pedir un board por workspace
   *  serían N respuestas calculadas en N instantes distintos —el calor es
   *  función del tiempo— y comparar dos burbujas medidas contra dos «ahora» es
   *  exactamente lo que esta pantalla existe para no hacer. */
  allBoard() {
    return this.call<Board>('/api/board');
  }

  /** Qué se movió, por archivo. Lo calcula git, que ya tiene la respuesta:
   *  cada escritura aquí es un commit. `since` vacío es EL CICLO — la misma
   *  ventana contra la que se mide todo lo demás. */
  async changes(workspace: string, since = '') {
    const out = await this.call<{ changes: Churn[] }>(
      `/api/workspaces/${workspace}/changes${since ? `?since=${encodeURIComponent(since)}` : ''}`,
    );
    return out.changes;
  }

  /** Y lo mismo en palabras: el parche que git escribe, MÁS los dos lados. Un
   *  parche es lo que git imprime; dos documentos es lo que una vista de diff
   *  necesita, y reconstruir un lado a partir del otro en el navegador sería una
   *  segunda implementación de «qué cambió». */
  diff(workspace: string, path: string, since = '') {
    return this.call<{ path: string; diff: string; before: string; after: string }>(
      `/api/workspaces/${workspace}/diff?path=${encodeURIComponent(path)}` +
        (since ? `&since=${encodeURIComponent(since)}` : ''),
    );
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
  async renderMarkdown(content: string, workspace = '') {
    const out = await this.call<{ html: string }>('/api/markdown', {
      method: 'POST',
      body: JSON.stringify({ content, workspace }),
    });
    return out.html;
  }

  /** Attach an image. It lands in `assets/`, is committed like every other
   *  write, and comes back as the path a document refers to it by — the SHORT
   *  one, because that is what the layout says and what survives a move. */
  async uploadAsset(workspace: string, file: File, name = '') {
    const form = new FormData();
    form.append('file', file);
    if (name) form.append('name', name);
    return this.call<{ path: string; url: string; bytes: number }>(
      `/api/workspaces/${workspace}/asset`,
      { method: 'POST', body: form },
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

  /** Remove a page. Only under `docs/` — a thread's document belongs to the
   *  thread, and the server refuses it by that door for the same reason. */
  removePath(workspace: string, path: string) {
    return this.call<unknown>(
      `/api/workspaces/${workspace}/document?path=${encodeURIComponent(path)}`,
      { method: 'DELETE' },
    );
  }

  patchPath(workspace: string, path: string, patch: Record<string, unknown>) {
    return this.call<Doc>(`/api/workspaces/${workspace}/document`, {
      method: 'PATCH',
      body: JSON.stringify({ path, ...patch }),
    });
  }
}

export const api = new Api();
