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
  heat: Heat;
  pulse: boolean;
  /** who has it */
  assignees?: string[];
  /** when anything last happened to it, RFC3339 — the row says it in words */
  at?: string;
  /** su PROPIA prioridad (P1…P4), separada de la de su burbuja */
  priority?: string;
  /** su paso en la secuencia del departamento; 0 o ausente, fuera de ella */
  sequence?: number;
  /** su lugar en la línea viva de todo el departamento: 1 = ahora; 0 si no
   *  está en la secuencia o ya terminó */
  step?: number;
  /** en qué columna del tablero de TAREAS está; vacío es «Sin planear» */
  stage?: string;
  /** cuándo vence ESTA pieza, un día. Independiente del plazo de su burbuja:
   *  aquél es el compromiso, éste es cómo se reparte. */
  due_date?: string;
};

/** Lo que pasa en una fecha y no es trabajo: una junta, un cierre, una visita.
 *
 *  No es un hilo a propósito: un hilo se completa, produce evidencia y calienta,
 *  y una junta semanal que renaciera como hilo mantendría una burbuja 🔥 para
 *  siempre sin que nadie trabajara. */
export type CalendarEvent = {
  id: string;
  name: string;
  /** cuándo es la PRIMERA; las siguientes se calculan */
  start: string;
  /** vacío es «no se repite» */
  repeat?: '' | 'daily' | 'weekly' | 'biweekly' | 'monthly';
  notes?: string;
};

/** Una columna del tablero de TAREAS. Del departamento, como `Stage`, pero de
 *  hilos: un módulo y una tarea no están en el mismo sitio del plan. */
export type ThreadStage = {
  id: string;
  name: string;
  position?: number;
  /** la columna donde una tarea se da por terminada */
  done?: boolean;
};

export type BubbleHeat = {
  id: string;
  workspace: string;
  name: string;
  /** en qué punto del plan la puso el lead */
  stage?: string;
  /** para qué sirve este cuerpo de trabajo */
  objective?: string;
  /** derivada de impacto × urgencia. Nunca mezclada con la banda: una dice si
   *  la realidad está cambiando, la otra cuánto importa que cambie. */
  priority?: string;
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

/** Una etapa del plan: en qué punto está una burbuja, según quien orquesta.
 *
 *  No es un `State`: los estados son del workspace y los pone quien ejecuta;
 *  las etapas son del departamento y son el vocabulario común que permite mirar
 *  seis proyectos en el mismo tablero. */
export type Stage = { id: string; name: string; position?: number; done?: boolean };

/** Una burbuja como fila: lo que el planeador organiza. */
export type BubbleRecord = {
  id: string;
  workspace: string;
  name: string;
  outcome?: string;
  owners?: string[];
  stage?: string;
  objective?: string;
  impact?: string;
  urgency?: string;
  /** cuándo tiene que estar este cuerpo de trabajo. Es de la burbuja y no de sus
   *  threads: el plazo lo decide quien orquesta. */
  due_date?: string;
  /** el sitio que el lead le dio a mano dentro de su columna. Cero —o ausente—
   *  es «ninguno»: entonces la columna se ordena sola, por prioridad. */
  rank?: number;
  closed_at?: string;
  closure?: string;
  /** cuándo nació. Lo pone el servidor, y es lo que la tarjeta cuenta como edad */
  created?: string;
  /** el cuaderno del plan, en markdown. El outcome es el contrato de una frase;
   *  esto es lo largo, con sus diagramas e imágenes. */
  brief?: string;
};

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
  /** el sitio que alguien le dio a mano dentro de su columna del planeador.
   *  Cero —o ausente— es «ninguno»: entonces la columna se ordena sola, por la
   *  prioridad que el servidor deriva. */
  rank?: number;
  completed_at?: string;
  /** su propia prioridad, P1…P4 */
  priority?: string;
  /** su paso en la secuencia del departamento */
  sequence?: number;
  assignees?: string[];
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
  /** lo que se entendió de la captura, en markdown. En la base y no en el árbol
   *  de un workspace: una nota todavía no tiene workspace, y no tenerlo es
   *  justo lo que la hace una nota y no trabajo. */
  body?: string;
  captured_by: string;
  thread?: string;
  /** la burbuja en la que se convirtió al triarse. Queda apuntando en vez de
   *  borrarse: su cuerpo y sus imágenes siguen siendo de algún sitio. */
  bubble?: string;
  created: string;
  /** por qué canal entró (`whatsapp`), si no se escribió en la app */
  source?: string;
  /** quién la mandó por ese canal, como lo dice el canal */
  source_from?: string;
};

/** Cómo está un canal del inbox. `extra` es lo propio de cada canal. */
export type ChannelStatus = {
  kind: string;
  enabled: boolean;
  state: 'disabled' | 'unlinked' | 'pairing' | 'connecting' | 'connected' | 'error';
  detail?: string;
  extra?: Record<string, unknown>;
};
/** Qué corre aquí, y si hay algo más nuevo. `boot` dice si esta instancia
 *  puede actualizarse sola: una lanzada con `serve` a mano no tiene quién
 *  deshaga una actualización que no arranca, y por eso no se le ofrece. */
export type Version = { version: string; latest: string; stale: boolean; boot: boolean };

export type Person = {
  id: string;
  email: string;
  display_name?: string;
  role?: string;
  /** nombre del archivo en el propio registro; la URL la arma `avatarUrl` */
  avatar?: string;
  created?: string;
};

/** Qué partes del producto tiene encendidas el departamento. Una fila. */
export type Features = { id: string; sequence: boolean; updated?: string };

/** Lo que una persona eligió y tiene que seguirla entre dispositivos.
 *
 *  Casi todo lo de vista se queda en `localStorage` (ver `lib/filters.svelte`):
 *  esto es sólo para lo que no puede. Un saco, no un esquema — nadie lo
 *  consulta ni lo filtra, sólo su dueña lo lee y lo escribe. */
export type Prefs = {
  /** cómo ve el planeador: por módulos (burbujas) o por tareas (hilos) */
  planner?: 'bubbles' | 'tasks';
};

/** De quién es un AGENTS.md: de quien planea (el lead) o de quien opera. */
export type AgentsRole = 'planner' | 'operators';

/** Una versión de un AGENTS.md. Cada guardado es una fila nueva. */
export type AgentsVersion = {
  id: string;
  role: AgentsRole;
  content: string;
  author: string;
  authorName?: string;
  created: string;
};

/** La calibración de la flotabilidad: una sola fila para todo el departamento. */
export type Tuning = {
  id: string;
  cycle_hours: number;
  dormant_cycles: number;
  decay_cycles: number;
  grace_cycles: number;
  ownerless_is_rip: boolean;
  updated?: string;
};

/** One person's place in one workspace. `id` is the MEMBERSHIP's — that is what
 *  a role change or a removal edits — and `user` is the person's. */
export type Member = {
  id: string;
  user: string;
  name: string;
  role: 'lead' | 'member';
  /** la persona está borrada: su membresía se conserva para restaurarla */
  deleted?: boolean;
  /** la URL de su avatar, o vacío */
  avatar?: string;
};

/** Lo que alguien dijo en un thread. El cuerpo es markdown: lo renderiza el
 *  mismo servidor que el documento, porque dos renderers coinciden hasta que
 *  dejan de hacerlo. */
export type Comment = {
  id: string;
  thread: string;
  author: string;
  name: string;
  /** la URL de su avatar, o vacío */
  avatar: string;
  body: string;
  created: string;
  mine: boolean;
};

/** El inventario: dónde vive lo que hace funcionar todo esto. No es trabajo y no
 *  calienta nada; es lo primero que alguien busca a las tres de la mañana. */
/** Un repositorio de código asociado al workspace. `name` puede venir vacío:
 *  entonces lo dice la URL, que es como se llama un repositorio en voz alta. */
export type Repo = { id: string; workspace: string; url: string; name?: string };

export type InvGroup = {
  id: string;
  name: string;
  note?: string;
  /** el slug de la pieza que lo representa — `computer`, `key`, `servidor`… */
  art?: string;
  image?: string;
  /** cuántas cosas hay dentro — la galería lo dice sin abrir el grupo */
  items?: number;
  position?: number;
};

export type InvItem = {
  id: string;
  group: string;
  name: string;
  art?: string;
  provider?: string;
  url?: string;
  /** el enlace a la bóveda. NUNCA la contraseña. */
  vault?: string;
  notes?: string;
  renews_at?: string;
  cost?: string;
  image?: string;
  position?: number;
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

/** Cómo se nombra a una persona: su nombre, o su correo, o `fallback`. Una
 *  persona borrada lo dice: lo que escribió se queda con su firma, y leerla sin
 *  saber que ya no está es preguntarle algo a quien no va a contestar. */
function named(
  u: { display_name?: string; email?: string; deleted_at?: string } | undefined,
  fallback: string,
) {
  const name = u?.display_name || u?.email || fallback;
  return u?.deleted_at ? `${name} (eliminado)` : name;
}

/** Un token de agente, tal como lo enseña el servidor: nunca la huella. */
export type AgentToken = {
  id: string;
  owner: string;
  name: string;
  prefix: string;
  expires_at: string;
  last_used_at: string;
  revoked_at: string;
  created: string;
  /** sólo en la respuesta de crear: la única vez que sale */
  token?: string;
};

/** Una persona vista desde la gestión de usuarios. */
export type PersonAdmin = Person & { deleted_at?: string };

/** La URL del avatar de una persona, o vacío. `thumb` pide una de las
 *  miniaturas que declara la migración (`64x64`, `160x160`). */
export function avatarUrl(p: Pick<Person, 'id' | 'avatar'> | null | undefined, thumb = '') {
  if (!p?.avatar) return '';
  const q = thumb ? `?thumb=${thumb}` : '';
  return `/api/files/users/${p.id}/${encodeURIComponent(p.avatar)}${q}`;
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
      //
      // Pero en un rechazo de validación esa frase es sólo «Failed to update
      // record.», y lo que dice QUÉ campo y POR QUÉ viene aparte, en `data`. Sin
      // ello, un texto que no cabía se leía como un fallo sin causa.
      const fields = body?.data && typeof body.data === 'object'
        ? Object.entries(body.data as Record<string, { message?: string }>)
            .filter(([, v]) => v && typeof v === 'object' && v.message)
            .map(([k, v]) => `${k}: ${v.message}`)
        : [];
      const said = body?.message || res.statusText || `HTTP ${res.status}`;
      const err = new ApiError(fields.length ? `${said} ${fields.join('; ')}` : said, res.status);
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

  /** …y las prioridades derivadas de sus BURBUJAS, del mismo alcance.
   *
   *  De la burbuja y no del thread: un objetivo y una prioridad describen un
   *  cuerpo de trabajo, y quien los decide es quien orquesta. El operador ya
   *  tiene su forma de ordenarse el día dentro de una burbuja. */
  async allPriorities() {
    const out = await this.call<{ items: { id: string; priority: string }[] }>(
      '/api/collections/bubble_priority/records?perPage=500',
    );
    return out.items;
  }

  /** Las etapas del plan, en su orden. Del departamento: son el vocabulario
   *  común que permite mirar seis proyectos en el mismo tablero. */
  async stages() {
    const out = await this.call<{ items: Stage[] }>(
      '/api/collections/stages/records?perPage=200&sort=position,created',
    );
    return out.items;
  }

  /** El brief de una burbuja: lo que el planeador escribió de qué trata. El
   *  board no lo trae —es largo, con diagramas, y el board se pide entero en
   *  cada cambio— así que se lee cuando alguien lo abre. */
  async bubbleBrief(id: string) {
    const out = await this.call<{ brief?: string }>(
      `/api/collections/bubbles/records/${id}?fields=brief`,
    );
    return out.brief ?? '';
  }

  /** Las burbujas que alcanzas, de todos tus workspaces: es lo que el planeador
   *  organiza. */
  async allBubbles() {
    const out = await this.call<{ items: BubbleRecord[] }>(
      '/api/collections/bubbles/records?perPage=500&sort=name',
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
        expand?: { user?: { display_name?: string; email?: string; deleted_at?: string; avatar?: string } };
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
      name: named(m.expand?.user, 'sin nombre'),
      deleted: !!m.expand?.user?.deleted_at,
      avatar: avatarUrl({ id: m.user, avatar: m.expand?.user?.avatar }, '64x64'),
    }));
  }

  /** The same roster, shaped for a field that only needs to name a person. */
  async roster(workspace: string) {
    const rows = await this.members(workspace);
    // Sin las personas borradas: su membresía se conserva para poder
    // restaurarlas, pero no se pone a cargo a quien ya no puede entrar.
    return rows.filter((m) => !m.deleted).map((m) => ({ id: m.user, name: m.name, avatar: m.avatar ?? '' }));
  }

  // ---- ajustes ------------------------------------------------------------

  /** La calibración. Una fila, sembrada por la migración: la lee cualquiera
   *  con sesión y sólo la cambia el lead global. */
  async tuning(): Promise<Tuning> {
    const out = await this.call<{ items: Tuning[] }>('/api/collections/tuning/records?perPage=1');
    if (!out.items[0]) throw new Error('no hay calibración: falta la fila de tuning');
    return out.items[0];
  }

  setTuning(id: string, fields: Partial<Omit<Tuning, 'id' | 'updated'>>) {
    return this.update<Tuning>('tuning', id, fields);
  }

  // ---- canales del inbox (lead global) ----

  channels() {
    return this.call<ChannelStatus[]>('/api/channels');
  }

  setChannel(kind: string, on: boolean) {
    return this.call<ChannelStatus>(`/api/channels/${kind}/${on ? 'enable' : 'disable'}`, { method: 'POST' });
  }

  /** Lo propio de un canal: vincular, elegir grupo, desvincular… */
  channelAction<T = ChannelStatus>(kind: string, action: string, body: Record<string, unknown> = {}) {
    return this.call<T>(`/api/channels/${kind}/${action}`, { method: 'POST', body: JSON.stringify(body) });
  }

  /** El QR vigente de un canal que se está vinculando, como URL de blob. La
   *  ruta exige sesión y un `<img>` no manda cabeceras. Vacío si no hay. */
  async channelQR(kind: string): Promise<string> {
    const res = await fetch(`/api/channels/${kind}/qr.png?t=${Date.now()}`, {
      headers: this.token ? { Authorization: this.token } : {},
    });
    return res.ok ? URL.createObjectURL(await res.blob()) : '';
  }

  /** Las funciones encendidas. Sin fila, todo apagado: una pantalla que enseña
   *  algo que el servidor no tiene encendido es una promesa rota. */
  async features(): Promise<Features> {
    try {
      const out = await this.call<{ items: Features[] }>('/api/collections/features/records?perPage=1');
      return out.items[0] ?? { id: '', sequence: false };
    } catch {
      return { id: '', sequence: false };
    }
  }

  setFeatures(id: string, fields: Partial<Omit<Features, 'id' | 'updated'>>) {
    return this.update<Features>('features', id, fields);
  }

  // ---- las preferencias de quien mira ----
  //
  // La fila nace la primera vez que se guarda algo, no al entrar: alguien que
  // nunca cambió nada no necesita una fila que diga que no cambió nada. Por eso
  // leer sin fila devuelve `{}` y no es un error.

  /** El id de mi fila, cacheado para no preguntarlo en cada guardado. */
  private prefRow = '';

  /** Lo que pasa en una fecha y no es trabajo. Del departamento, como los
   *  objetivos: lo escribe el lead global y lo ve todo el mundo. */
  async calendarEvents(): Promise<CalendarEvent[]> {
    const out = await this.call<{ items: CalendarEvent[] }>(
      '/api/collections/calendar_events/records?perPage=500&sort=start',
    );
    return out.items;
  }

  /** Las columnas del tablero de tareas, en orden. Vacío es una respuesta: el
   *  lead todavía no ha definido ninguna. */
  async threadStages(): Promise<ThreadStage[]> {
    const out = await this.call<{ items: ThreadStage[] }>(
      '/api/collections/thread_stages/records?perPage=200&sort=position,created',
    );
    return out.items;
  }

  async prefs(): Promise<Prefs> {
    if (!this.me) return {};
    try {
      const out = await this.call<{ items: { id: string; value?: Prefs }[] }>(
        '/api/collections/prefs/records?perPage=1',
      );
      const row = out.items[0];
      this.prefRow = row?.id ?? '';
      return row?.value ?? {};
    } catch {
      // Sin preferencias se ve lo de siempre. Una lista que falla no es motivo
      // para no dejar entrar a nadie.
      return {};
    }
  }

  /** Guarda el saco entero. Quien llame manda lo que quiere que quede, no un
   *  parche: son cuatro campos y fusionarlos en el servidor pediría leer antes
   *  de escribir en cada cambio. */
  async setPrefs(value: Prefs): Promise<void> {
    if (!this.me) return;
    if (this.prefRow) {
      await this.update('prefs', this.prefRow, { value });
      return;
    }
    const row = await this.call<{ id: string }>('/api/collections/prefs/records', {
      method: 'POST',
      body: JSON.stringify({ user: this.me.id, value }),
    });
    this.prefRow = row.id;
  }

  // ---- AGENTS.md del departamento ----

  /** La versión vigente de un rol: la más reciente. `null` si todavía no hay. */
  async agentsDoc(role: AgentsRole): Promise<AgentsVersion | null> {
    const out = await this.call<{ items: (AgentsVersion & { expand?: { author?: Person } })[] }>(
      `/api/collections/agents_md/records?perPage=1&sort=-created&expand=author` +
        `&filter=${encodeURIComponent(`role='${role}'`)}`,
    );
    const r = out.items[0];
    return r ? { ...r, authorName: named(r.expand?.author, '') } : null;
  }

  /** Guardar es crear una versión nueva: las anteriores no se tocan. */
  saveAgentsDoc(role: AgentsRole, content: string) {
    return this.create<AgentsVersion>('agents_md', { role, content, author: this.me?.id });
  }

  // ---- tokens de agente ----

  /** Los míos; el lead global recibe los de todos (lo decide la regla). */
  async tokens(): Promise<(AgentToken & { ownerName: string })[]> {
    const out = await this.call<{
      items: (AgentToken & { expand?: { owner?: { display_name?: string; email?: string; deleted_at?: string } } })[];
    }>('/api/collections/agent_tokens/records?perPage=500&sort=-created&expand=owner');
    return out.items.map((t) => ({ ...t, ownerName: named(t.expand?.owner, '') }));
  }

  createToken(name: string, days: number) {
    return this.call<AgentToken>('/api/tokens', { method: 'POST', body: JSON.stringify({ name, days }) });
  }

  /** `days` desde hoy o desde la caducidad actual, la más tarde; 0 la quita. */
  extendToken(id: string, days: number) {
    return this.call<AgentToken>(`/api/tokens/${id}/extend`, { method: 'POST', body: JSON.stringify({ days }) });
  }

  renameToken(id: string, name: string) {
    return this.call<AgentToken>(`/api/tokens/${id}/rename`, { method: 'POST', body: JSON.stringify({ name }) });
  }

  revokeToken(id: string) {
    return this.call<AgentToken>(`/api/tokens/${id}/revoke`, { method: 'POST', body: '{}' });
  }

  // ---- gestión de personas (lead global) ----

  // ---- prioridad de un hilo y secuencia (lead global) ----

  /** La prioridad PROPIA de un hilo; vacía la quita. */
  setThreadPriority(id: string, priority: string) {
    return this.update<ThreadRecord>('threads', id, { priority });
  }

  /** El paso de un hilo en la secuencia; 0 lo saca de ella. */
  setSequence(id: string, sequence: number) {
    return this.update<ThreadRecord>('threads', id, { sequence });
  }

  /** Terminar un hilo: pasa al estado «completado» de su proyecto. */
  completeThread(id: string) {
    return this.call<{ id: string; state: string }>(`/api/threads/${id}/complete`, { method: 'POST' });
  }

  /** Todas, borradas incluidas: restaurar necesita verlas. */
  async allPeople(): Promise<PersonAdmin[]> {
    const out = await this.call<{ items: PersonAdmin[] }>(
      '/api/collections/users/records?perPage=500&sort=display_name,email',
    );
    return out.items;
  }

  /** Cuántos proyectos tiene cada persona, `user` → nombres. */
  async membershipsByPerson(): Promise<Record<string, string[]>> {
    const out = await this.call<{
      items: { user: string; expand?: { workspace?: { name?: string } } }[];
    }>('/api/collections/memberships/records?perPage=1000&expand=workspace');
    const by: Record<string, string[]> = {};
    for (const m of out.items) (by[m.user] ??= []).push(m.expand?.workspace?.name ?? '?');
    return by;
  }

  createPerson(fields: { email: string; name: string; password: string; role: 'lead' | 'member' }) {
    return this.call<PersonAdmin>('/api/people', { method: 'POST', body: JSON.stringify(fields) });
  }

  /** El rol GLOBAL de una persona. No confundir con `setRole`, que es el de su
   *  membresía en un proyecto. */
  setGlobalRole(id: string, role: 'lead' | 'member') {
    return this.update<PersonAdmin>('users', id, { role });
  }

  deletePerson(id: string) {
    return this.call<PersonAdmin>(`/api/people/${id}/delete`, { method: 'POST', body: '{}' });
  }

  restorePerson(id: string) {
    return this.call<PersonAdmin>(`/api/people/${id}/restore`, { method: 'POST', body: '{}' });
  }

  /** Cambiar lo mío: nombre, o el avatar (con `FormData`). Deja `me` al día,
   *  que es lo que lee el resto de la aplicación. */
  async updateMe(fields: Record<string, unknown> | FormData): Promise<Person> {
    if (!this.me) throw new Error('sin sesión');
    const out = await this.call<Person>(`/api/collections/users/records/${this.me.id}`, {
      method: 'PATCH',
      body: fields instanceof FormData ? fields : JSON.stringify(fields),
    });
    this.me = { ...this.me, ...out };
    return this.me;
  }

  /** Cambiar la contraseña.
   *
   *  PocketBase rota el `tokenKey` de la persona al cambiarla, así que el token
   *  con el que se hizo la petición deja de valer en ese mismo instante —y con él
   *  el de cualquier agente que usara esta sesión—. Se vuelve a entrar con la
   *  nueva para que la pantalla no se quede sin sesión a media frase. */
  async changePassword(oldPassword: string, password: string) {
    if (!this.me) throw new Error('sin sesión');
    const email = this.me.email;
    await this.call(`/api/collections/users/records/${this.me.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ oldPassword, password, passwordConfirm: password }),
    });
    return this.signIn(email, password);
  }

  /** Everybody with an account. Listing people is open to anybody signed in —
   *  that is what makes an invitation possible — and it is deliberately NOT the
   *  same list as a workspace's roster. */
  async people() {
    const out = await this.call<{ items: Person[] }>(
      `/api/collections/users/records?perPage=500&sort=display_name,email` +
        // A quien se invita tiene que poder entrar.
        `&filter=${encodeURIComponent(`deleted_at = ""`)}`,
    );
    return out.items;
  }

  /** Cuánto cabe en cada campo de texto, `colección.campo` → caracteres. Lo
   *  dice el esquema del servidor; ver `lib/limits`. */
  limits(): Promise<Record<string, number>> {
    return this.call<Record<string, number>>('/api/limits');
  }

  /** Qué está corriendo. Sin sesión: es la misma respuesta que mira el
   *  healthcheck del contenedor, y una que necesitara sesión no serviría para
   *  eso. */
  async version(): Promise<Version> {
    try {
      const out = await this.call<Version>('/api/version');
      return { version: out.version ?? '', latest: out.latest ?? '', stale: !!out.stale, boot: !!out.boot };
    } catch {
      return { version: '', latest: '', stale: false, boot: false };
    }
  }

  /** Actualizar esta instancia. Contesta 202 y el servidor se va: quien cambia
   *  la versión y sabe deshacerlo es el arranque, no él. Lo que sigue es
   *  esperar a que `/api/version` diga otro número. */
  selfUpdate(version?: string) {
    return this.call<{ from: string; to: string; backup: string }>('/api/update', {
      method: 'POST',
      body: JSON.stringify(version ? { version } : {}),
    });
  }

  // ---- dónde vive el código ----------------------------------------------
  //
  // Una fila por repositorio. No es el árbol de markdown del workspace —ése lo
  // escribe este servidor y no se elige— sino el enlace a donde está el código,
  // que este servidor no toca.

  async repos(workspace: string): Promise<Repo[]> {
    const out = await this.call<{ items: Repo[] }>(
      `/api/collections/workspace_repos/records?perPage=100&sort=created` +
        `&filter=${encodeURIComponent(`workspace='${workspace}'`)}`,
    );
    return out.items;
  }

  addRepo(workspace: string, url: string, name = '') {
    return this.create<Repo>('workspace_repos', { workspace, url, name });
  }

  removeRepo(id: string) {
    return this.remove('workspace_repos', id);
  }

  /** Mi papel AQUÍ. El lead global escribe en todas partes; los demás, sólo
   *  donde su membresía dice lead. Se pregunta con una fila y no con el roster
   *  entero porque la respuesta es una palabra. */
  async myRole(workspace: string): Promise<'lead' | 'member' | ''> {
    if (this.me?.role === 'lead') return 'lead';
    try {
      const out = await this.call<{ items: { role: 'lead' | 'member' }[] }>(
        `/api/collections/memberships/records?perPage=1&fields=role` +
          `&filter=${encodeURIComponent(
            `workspace='${workspace}' && user='${this.me?.id ?? ''}'`,
          )}`,
      );
      return out.items[0]?.role ?? '';
    } catch {
      return '';
    }
  }

  // ---- el inventario ------------------------------------------------------
  //
  // Verlo se ASIGNA, y se asigna con una fila. Preguntar "¿me toca?" es leer la
  // propia: la regla deja ver la tuya y ninguna más, así que la aplicación puede
  // averiguar si tiene permiso sin pedir permiso para averiguarlo.
  async seesInventory() {
    if (this.me?.role === 'lead') return true;
    try {
      const out = await this.call<{ totalItems: number }>(
        `/api/collections/inventory_access/records?perPage=1` +
          `&filter=${encodeURIComponent(`user='${this.me?.id ?? ''}'`)}`,
      );
      return out.totalItems > 0;
    } catch {
      return false;
    }
  }

  /** Quién puede verlo. Sólo el lead global lo lee entero: repartirlo es su
   *  trabajo. */
  async inventoryAccess() {
    const out = await this.call<{
      items: { id: string; user: string; expand?: { user?: { display_name?: string; email?: string } } }[];
    }>('/api/collections/inventory_access/records?perPage=200&expand=user');
    return out.items.map((r) => ({
      id: r.id,
      user: r.user,
      name: r.expand?.user?.display_name || r.expand?.user?.email || 'sin nombre',
    }));
  }

  grantInventory(user: string) {
    return this.create<{ id: string }>('inventory_access', { user });
  }

  async invGroups(): Promise<InvGroup[]> {
    const out = await this.call<{ items: (InvGroup & { collectionId: string })[] }>(
      '/api/collections/inventory_groups/records?perPage=200&sort=position,name',
    );
    // Las cuentas se piden aparte y en una sola llamada: una galería que cuesta
    // una petición por tarjeta es una galería que se deja de dibujar.
    const items = await this.call<{ items: { group: string }[] }>(
      '/api/collections/inventory_items/records?perPage=500&fields=group',
    );
    const by: Record<string, number> = {};
    for (const i of items.items) by[i.group] = (by[i.group] ?? 0) + 1;
    return out.items.map((g) => ({
      ...g,
      items: by[g.id] ?? 0,
      image: g.image ? this.fileUrl('inventory_groups', g.id, String(g.image)) : '',
    }));
  }

  createInvGroup(fields: { name: string; note?: string; art?: string }) {
    return this.create<{ id: string }>('inventory_groups', fields);
  }

  createInvItem(fields: Record<string, unknown>) {
    return this.create<{ id: string }>('inventory_items', fields);
  }

  async invItems(group: string): Promise<InvItem[]> {
    const out = await this.call<{ items: (InvItem & { collectionId: string })[] }>(
      `/api/collections/inventory_items/records?perPage=500&sort=position,name` +
        `&filter=${encodeURIComponent(`group='${group}'`)}`,
    );
    return out.items.map((i) => ({
      ...i,
      image: i.image ? this.fileUrl('inventory_items', i.id, String(i.image)) : '',
    }));
  }

  /** La url de un archivo de PocketBase. Las imágenes del inventario no van a
   *  `assets/` del workspace a propósito: ese árbol es de un proyecto, y una
   *  factura de dominio no pertenece a ninguno. */
  fileUrl(collection: string, id: string, file: string) {
    return `/api/files/${collection}/${id}/${file}`;
  }

  /** Lo dicho en un thread, del más viejo al más nuevo: una conversación se lee
   *  hacia abajo, y la última línea es la que estabas esperando. */
  async comments(thread: string): Promise<Comment[]> {
    const out = await this.call<{
      items: {
        id: string; thread: string; author: string; body: string; created: string;
        expand?: { author?: { display_name?: string; email?: string; deleted_at?: string; avatar?: string } };
      }[];
    }>(
      `/api/collections/comments/records?perPage=500&sort=created&expand=author` +
        `&filter=${encodeURIComponent(`thread='${thread}'`)}`,
    );
    return out.items.map((c) => ({
      id: c.id,
      thread: c.thread,
      author: c.author,
      name: named(c.expand?.author, 'alguien'),
      avatar: avatarUrl({ id: c.author, avatar: c.expand?.author?.avatar }, '64x64'),
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
      items: { user: string; at: string; expand?: { user?: { display_name?: string; email?: string; deleted_at?: string; avatar?: string } } }[];
    }>('/api/collections/presence/records?perPage=200&expand=user');
    return out.items.filter((r) => !r.expand?.user?.deleted_at).map((r) => ({
      id: r.user,
      at: r.at,
      avatar: avatarUrl({ id: r.user, avatar: r.expand?.user?.avatar }, '64x64'),
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

  /** The derived priorities, by BUBBLE. A VIEW collection: the server computes
   *  it from impact × urgency, and nothing writes to it. */
  priorities(workspace: string) {
    return this.list<{ id: string; priority: string }>('bubble_priority', workspace, 'id');
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

  /** Una imagen para una nota del inbox.
   *
   *  No va a ningún `assets/`: una nota no tiene workspace, que es justo lo que
   *  la hace una nota. Se guarda como archivo del propio registro —como las
   *  imágenes del inventario— y la nota la cita por su URL completa, que en un
   *  registro de la base no tiene el problema que tendría en un documento de
   *  git: aquí nadie la va a leer desde un clon.
   *
   *  `files+` AÑADE al campo en vez de reemplazarlo; sin el `+` cada pegado
   *  borraría la imagen anterior, y la nota citaría archivos que ya no
   *  existen. */
  async attachToNote(note: string, file: File): Promise<{ path: string }> {
    const form = new FormData();
    form.append('files+', file);
    const rec = await this.call<{ files?: string[] }>(
      `/api/collections/inbox_items/records/${note}`,
      { method: 'PATCH', body: form },
    );
    const names = rec.files ?? [];
    const name = names[names.length - 1];
    if (!name) throw new Error('la imagen no se guardó');
    return { path: this.fileUrl('inbox_items', note, name) };
  }

  /** Attach an image. It lands in `assets/`, is committed like every other
   *  write, and comes back as the path a document refers to it by — the SHORT
   *  one, because that is what the layout says and what survives a move. */
  async uploadAsset(workspace: string, file: File, name = '') {
    const form = new FormData();
    form.append('file', file);
    if (name) form.append('name', name);
    return this.call<{ path: string; url: string; bytes: number; image: boolean }>(
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
