// Language: English / Spanish. Detected from the browser on first visit,
// overridable from the board context menu, persisted like the theme.
//
// Deliberately no library. The catalogue is a plain object, the lookup is a
// function, and TypeScript checks that Spanish covers every English key — which
// is the only guarantee an i18n dependency would have bought us here.

export type Lang = 'en' | 'es';

const KEY = 'bubble.lang';

// en is the source of truth: its keys ARE the message ids, so a missing Spanish
// string is a type error rather than a blank label discovered in production.
const en = {
  // board / bands
  'band.in_progress': 'In progress',
  'band.reviewed': 'Reviewed',
  'band.zzzz': 'Zzzz',
  'band.rip': 'RIP',
  'band.done': 'Done',
  'board.refreshed': 'board refreshed',
  'board.unreadNotices': 'unread notices',
  'board.loading': 'Loading…',

  // bubble card + detail
  'bubble.threads': '{n} thread',
  'bubble.threads_plural': '{n} threads',
  'bubble.involved': '{n} involved',
  'bubble.owner': 'owner',
  'bubble.open': 'open',
  'bubble.markReviewed': 'reviewed',
  'bubble.unreview': 'un-review',
  'bubble.done': 'done',
  'bubble.reopen': 'reopen',
  'bubble.openTimeline': 'Open timeline',
  'bubble.birthThread': 'Birth thread…',
  'bubble.newBubble': 'New bubble…',

  // the heat sentence under a bubble — keys match heat.Reason* codes
  'reason.hot_current': 'meaningful output in the current cycle',
  'reason.warm_previous': 'output last cycle; active threads remain',
  'reason.cooling': 'no meaningful output this cycle or last',
  'reason.dormant_ownerless': 'no active owner',
  'reason.dormant_never': 'no meaningful output ever recorded',
  'reason.dormant_silent': 'silent for {cycles}+ cycles',
  'reason.thread_warm_open': 'progress last cycle; still open',
  'reason.thread_newborn': 'born recently; nothing produced yet',
  'reason.closed_bubble': 'outcome reached or explicitly abandoned',
  'reason.closed_thread': 'thread completed',
  'reason.all_threads_done': 'every thread is finished — close or redefine this bubble',
  'reason.no_threads': 'no threads yet',

  // discussion
  'chat.title': 'Discussion',
  'chat.empty': 'No comments yet.',
  'chat.send': 'Send',
  'chat.placeholder': 'Write a comment… (Markdown; Enter to send, Shift+Enter for newline)',
  'chat.readonly': 'Read-only display — sign in to comment.',
  'chat.unsent': 'unsent',
  'chat.retry': 'Retry',
  'chat.discard': 'Discard',
  'chat.unreachable': 'could not reach Plane',

  // degraded mode
  'status.stale': 'Plane sync is behind — this may be older data',
  'status.neverSynced': 'never synced',
  'status.lastSynced': 'last synced {ago} ago',
  'status.unsent': '{n} unsent comment — open the thread to retry or discard.',
  'status.unsent_plural': '{n} unsent comments — open the thread to retry or discard.',

  // context menus / commands
  'menu.jumpToBand': 'Jump to band',
  'menu.search': 'Search & commands',
  'menu.refreshNow': 'Refresh now',
  'menu.onlyMine': 'Only my bubbles',
  'menu.showEveryone': 'Show everyone',
  'menu.theme': 'Theme',
  'menu.language': 'Language',
  'menu.godMode': 'God Mode',
  'menu.exitKiosk': 'Exit kiosk',

  // thread interior
  'thread.aria': 'thread',
  'thread.back': 'back to board',
  'thread.loading': 'Loading thread…',
  'thread.contents': 'Contents',
  'thread.unpin': 'unpin',
  'thread.work': 'Work',
  'thread.logbook': 'Logbook',
  'thread.revisions': 'Revisions',
  'thread.noTasks': 'No tasks recorded.',
  'thread.dod': 'Definition of Done',
  'thread.nothing': 'Nothing to show.',
  'thread.closeChat': 'close discussion',
  'thread.openChat': 'open discussion',
  'thread.onThisPage': 'On this page',
  'fmt.bold': 'Bold (**)',
  'fmt.italic': 'Italic (*)',
  'fmt.code': 'Inline code (`)',
  'fmt.link': 'Link',
  'fmt.list': 'Bullet list',

  // bubble detail
  'detail.close': 'close',
  'detail.noThreads': 'No threads in this bubble yet.',
  'detail.openThread': 'open thread',
  'detail.planeState': 'Plane state',

  // comboboxes
  'combo.open': 'open options',
  'combo.viewScope': 'View scope',
  'combo.view': 'View',
  'combo.openViews': 'Open view scopes',
  'combo.projectFilter': 'Project filter',
  'combo.project': 'Project',
  'combo.openProjects': 'Open projects',
  'combo.instanceFilter': 'instance filter',

  // forms
  'form.instance': 'Instance',
  'form.project': 'Project',
  'form.pickProject': 'pick a project',
  'form.noProjects': 'No projects here yet — create one with',
  'form.name': 'Name',
  'form.namePlaceholder': 'Onboarding revamp',
  'form.outcome': 'Outcome',
  'form.outcomePlaceholder': 'signups convert 20% better',
  'form.owner': 'Owner',
  'form.ownerPlaceholder': 'search members…',
  'form.title': 'Title',
  'form.titlePlaceholder': 'Design the signup screen',
  'form.problem': 'Problem / opportunity',
  'form.problemPlaceholder': "what's the pull?",
  'form.intended': 'Intended outcome',
  'form.intendedPlaceholder': 'what changes when done?',
  'form.dodPlaceholder': "the checklist that says it's finished",
  'form.apiKey': 'Plane API key',

  // chrome
  'chrome.kiosk': 'bubble.work kiosk',
  'chrome.readonly': 'read-only display',
  'chrome.exitKiosk': 'exit kiosk mode',
  'chrome.searchCommands': 'search · commands (⌘K)',
  'chrome.searchHint': '⌘K search',
  'chrome.commandsHint': 'commands',
  'chrome.commandMode': 'command mode',
  'chrome.openGodMode': 'open God Mode (/god-mode)',
  'chrome.navigate': 'Navigate bubbles',
  'chrome.noBubbles': 'No bubbles here yet.',
  'chrome.press': 'Press',

  'form.cancel': 'cancel',
  'form.optionalDone': 'optional · what “done” looks like',
  'form.optionalOwner': "optional · who's accountable",
  'form.required': 'required',
  'form.newBubble': 'new bubble',
  'form.newBubbleHint': 'a durable grouping of work · maps to a Plane module',
  'form.creating': 'creating…',
  'form.createBubble': 'create bubble',
  'form.birthThread': 'birth a thread',
  'form.into': 'into',
  'form.birthing': 'birthing…',
  'form.birth': 'birth thread',
  'form.smallThread': 'small thread — skip the logbook, close with a short paragraph',
  'form.logbookHint': 'plan · ≥1 todo · owner · state',
  'form.logbook': 'Logbook',
  'form.dod': 'Definition of Done',
  'form.login': 'log in',

  'chrome.emptyHint': 'type {cmd}, and choose {new} — then birth threads into it.',
  'chrome.exitKioskShort': 'exit kiosk',
  'chrome.newBubbleCmd': 'new bubble',

  // god mode (admin only)
  'god.aria': 'god mode',
  'god.title': 'God Mode',
  'god.adminOnly': 'Service admin only.',
  'god.server': 'Server',
  'god.rateBudget': 'Plane rate budget',
  'god.atFloor': 'At the floor — background work is yielding to keep pages fast.',
  'god.noInstances': 'No instances configured.',
  'god.outboxEmpty': 'Empty — every write has reached Plane.',
  'god.mirror': 'Plane mirror',
  'god.maintenance': 'Maintenance',
  'god.members': 'Members',
  'god.calibration': 'Buoyancy calibration',
  'god.differsDefault': 'differs from the stock default',
  'god.discardEdits': 'Discard edits',
  'god.noChanges': 'No changes',
  'god.applyChanges': 'Apply {n} change(s)',
  'god.resetDefaults': 'Reset to defaults',
  'god.instance': 'instance',
  'god.kioskLabel': 'label (e.g. lobby screen)',
  'god.mintToken': 'Mint token',
  'god.noKiosk': 'No kiosk tokens. Mint one to boot a read-only display board.',
  'god.copyKiosk': 'copy kiosk URL',
  'god.revoke': 'revoke',

  // tuning groups + knobs — keyed by the server's stable field key, falling back
  // to the server's own English when a key is unknown (same contract as reasons)
  'tune.group.pulse': 'Pulse',
  'tune.group.bubble': 'Bubbles',
  'tune.group.thread': 'Threads',
  'tune.cycle_hours': 'Cycle length (hours)',
  'tune.dormant_cycles': 'Cycles before dormant',
  'tune.decay_cycles': 'Score decay (cycles)',
  'tune.ownerless_is_dormant': 'No owner sinks to dormant',
  'tune.bubble_rip_needs_owner': 'Dormant + ownerless reads as RIP',
  'tune.bubble_level_rollup': 'Bubble band = hottest thread',
  'tune.thread_birth_heats': 'Creating a thread heats it',
  'tune.thread_grace_cycles': 'Newborn grace (cycles)',
  'tune.thread_rip_needs_owner': 'Unassigned + dormant reads as RIP',
  'tune.pulse_cycles': 'Comment keeps a thread alive (cycles)',
  'tune.thread_terminal_state_wins': "Plane's done/cancelled columns win",

  'god.allOrgs': 'all orgs',
  'god.revision': 'revision',
  'god.built': 'built',
  'god.started': 'started',
  'god.instances': 'instances',
  'god.identities': 'identities',
  'god.noMirror': 'no mirror',
  'god.slug': 'slug',
  'god.baseUrl': 'base url',
  'god.workspace': 'workspace',
  'god.projectCol': 'project',
  'god.webhook': 'webhook',
  'god.cached': 'cached',
  'god.nameCol': 'name',
  'god.emailCol': 'email',
  'god.roleCol': 'role',
  'god.modified': 'modified',
  'chrome.kioskBadge': 'kiosk',
  'combo.allInstances': 'all instances',

  'god.writesToPlane': 'writes to Plane',
  'omni.applyHint': '⏎ apply · esc back',
  'omni.navHint': '↑↓ move · ⏎ open · esc close · type',
  'omni.forCommands': 'for commands',
} as const;

export type MsgKey = keyof typeof en;

// Two families are looked up by COMPUTED key and so have no literal call site:
//   band.*   — levelLabel() builds `band.${level}`
//   reason.* — i18n.reason() builds `reason.${server code}`
// Grepping for a literal will find nothing and make them look dead. They are
// not. Delete one and a band goes nameless.

// Record<MsgKey, string> is the point: adding an English key and forgetting the
// Spanish one fails the build.
const es: Record<MsgKey, string> = {
  'band.in_progress': 'En curso',
  'band.reviewed': 'Revisado',
  'band.zzzz': 'Dormido',
  'band.rip': 'Abandonado',
  'band.done': 'Terminado',
  'board.refreshed': 'tablero actualizado',
  'board.unreadNotices': 'avisos sin leer',
  'board.loading': 'Cargando…',

  'bubble.threads': '{n} hilo',
  'bubble.threads_plural': '{n} hilos',
  'bubble.involved': '{n} participantes',
  'bubble.owner': 'responsable',
  'bubble.open': 'abrir',
  'bubble.markReviewed': 'revisado',
  'bubble.unreview': 'quitar revisión',
  'bubble.done': 'terminado',
  'bubble.reopen': 'reabrir',
  'bubble.openTimeline': 'Abrir línea de tiempo',
  'bubble.birthThread': 'Crear hilo…',
  'bubble.newBubble': 'Nueva burbuja…',

  // "salida real" rather than a literal "producción": the model means evidence
  // that something CHANGED, not activity, and that distinction is the whole
  // point of the vocabulary (AGENTS.md).
  'reason.hot_current': 'salida real en el ciclo actual',
  'reason.warm_previous': 'salida el ciclo pasado; quedan hilos activos',
  'reason.cooling': 'sin salida real este ciclo ni el anterior',
  'reason.dormant_ownerless': 'sin responsable activo',
  'reason.dormant_never': 'nunca registró salida real',
  'reason.dormant_silent': 'en silencio {cycles}+ ciclos',
  'reason.thread_warm_open': 'avanzó el ciclo pasado; sigue abierto',
  'reason.thread_newborn': 'recién creado; todavía sin producir',
  'reason.closed_bubble': 'resultado alcanzado o abandonado a propósito',
  'reason.closed_thread': 'hilo completado',
  'reason.all_threads_done': 'todos los hilos terminaron — cierra o redefine esta burbuja',
  'reason.no_threads': 'aún sin hilos',

  'chat.title': 'Discusión',
  'chat.empty': 'Aún no hay comentarios.',
  'chat.send': 'Enviar',
  'chat.placeholder': 'Escribe un comentario… (Markdown; Enter envía, Shift+Enter salto de línea)',
  'chat.readonly': 'Pantalla de solo lectura — inicia sesión para comentar.',
  'chat.unsent': 'sin enviar',
  'chat.retry': 'Reintentar',
  'chat.discard': 'Descartar',
  'chat.unreachable': 'no se pudo contactar a Plane',

  'status.stale': 'La sincronización con Plane está atrasada — estos datos pueden no estar al día',
  'status.neverSynced': 'nunca sincronizado',
  'status.lastSynced': 'sincronizado hace {ago}',
  'status.unsent': '{n} comentario sin enviar — abre el hilo para reintentar o descartar.',
  'status.unsent_plural': '{n} comentarios sin enviar — abre el hilo para reintentar o descartar.',

  'menu.jumpToBand': 'Ir a la banda',
  'menu.search': 'Buscar y comandos',
  'menu.refreshNow': 'Actualizar ahora',
  'menu.onlyMine': 'Solo mis burbujas',
  'menu.showEveryone': 'Ver todas',
  'menu.theme': 'Tema',
  'menu.language': 'Idioma',
  'menu.godMode': 'Modo Dios',
  'menu.exitKiosk': 'Salir de kiosco',

  'thread.aria': 'hilo',
  'thread.back': 'volver al tablero',
  'thread.loading': 'Cargando hilo…',
  'thread.contents': 'Contenido',
  'thread.unpin': 'quitar fijado',
  'thread.work': 'Trabajo',
  'thread.logbook': 'Bitácora',
  'thread.revisions': 'Revisiones',
  'thread.noTasks': 'Sin tareas registradas.',
  'thread.dod': 'Definición de Terminado',
  'thread.nothing': 'Nada que mostrar.',
  'thread.closeChat': 'cerrar discusión',
  'thread.openChat': 'abrir discusión',
  'thread.onThisPage': 'En esta página',
  'fmt.bold': 'Negrita (**)',
  'fmt.italic': 'Cursiva (*)',
  'fmt.code': 'Código en línea (`)',
  'fmt.link': 'Enlace',
  'fmt.list': 'Lista con viñetas',

  'detail.close': 'cerrar',
  'detail.noThreads': 'Esta burbuja aún no tiene hilos.',
  'detail.openThread': 'abrir hilo',
  'detail.planeState': 'estado en Plane',

  'combo.open': 'abrir opciones',
  'combo.viewScope': 'Ámbito de vista',
  'combo.view': 'Vista',
  'combo.openViews': 'Abrir ámbitos de vista',
  'combo.projectFilter': 'Filtro de proyecto',
  'combo.project': 'Proyecto',
  'combo.openProjects': 'Abrir proyectos',
  'combo.instanceFilter': 'filtro de instancia',

  'form.instance': 'Instancia',
  'form.project': 'Proyecto',
  'form.pickProject': 'elige un proyecto',
  'form.noProjects': 'Aquí aún no hay proyectos — crea uno con',
  'form.name': 'Nombre',
  'form.namePlaceholder': 'Rediseño de onboarding',
  'form.outcome': 'Resultado',
  'form.outcomePlaceholder': 'los registros convierten 20% mejor',
  'form.owner': 'Responsable',
  'form.ownerPlaceholder': 'buscar miembros…',
  'form.title': 'Título',
  'form.titlePlaceholder': 'Diseñar la pantalla de registro',
  'form.problem': 'Problema / oportunidad',
  'form.problemPlaceholder': '¿qué lo impulsa?',
  'form.intended': 'Resultado esperado',
  'form.intendedPlaceholder': '¿qué cambia cuando esté listo?',
  'form.dodPlaceholder': 'la lista que dice que ya está terminado',
  'form.apiKey': 'Clave API de Plane',

  'chrome.kiosk': 'kiosco bubble.work',
  'chrome.readonly': 'pantalla de solo lectura',
  'chrome.exitKiosk': 'salir del modo kiosco',
  'chrome.searchCommands': 'buscar · comandos (⌘K)',
  'chrome.searchHint': '⌘K buscar',
  'chrome.commandsHint': 'comandos',
  'chrome.commandMode': 'modo comando',
  'chrome.openGodMode': 'abrir Modo Dios (/god-mode)',
  'chrome.navigate': 'Navegar burbujas',
  'chrome.noBubbles': 'Aquí aún no hay burbujas.',
  'chrome.press': 'Pulsa',

  'form.cancel': 'cancelar',
  'form.optionalDone': 'opcional · cómo se ve “terminado”',
  'form.optionalOwner': 'opcional · quién responde por esto',
  'form.required': 'obligatorio',
  'form.newBubble': 'nueva burbuja',
  'form.newBubbleHint': 'una agrupación duradera de trabajo · equivale a un módulo de Plane',
  'form.creating': 'creando…',
  'form.createBubble': 'crear burbuja',
  'form.birthThread': 'crear un hilo',
  'form.into': 'en',
  'form.birthing': 'creando…',
  'form.birth': 'crear hilo',
  'form.smallThread': 'hilo pequeño — omite la bitácora, ciérralo con un párrafo breve',
  'form.logbookHint': 'plan · ≥1 tarea · responsable · estado',
  'form.logbook': 'Bitácora',
  'form.dod': 'Definición de Terminado',
  'form.login': 'entrar',

  'chrome.emptyHint': 'escribe {cmd} y elige {new} — luego crea hilos dentro.',
  'chrome.exitKioskShort': 'salir del kiosco',
  'chrome.newBubbleCmd': 'nueva burbuja',

  'god.aria': 'modo dios',
  'god.title': 'Modo Dios',
  'god.adminOnly': 'Solo para administradores del servicio.',
  'god.server': 'Servidor',
  'god.rateBudget': 'Cuota de peticiones a Plane',
  'god.atFloor': 'En el límite — el trabajo en segundo plano cede para mantener las páginas rápidas.',
  'god.noInstances': 'No hay instancias configuradas.',
  'god.outboxEmpty': 'Vacía — todas las escrituras llegaron a Plane.',
  'god.mirror': 'Espejo de Plane',
  'god.maintenance': 'Mantenimiento',
  'god.members': 'Miembros',
  'god.calibration': 'Calibración de flotabilidad',
  'god.differsDefault': 'difiere del valor de fábrica',
  'god.discardEdits': 'Descartar cambios',
  'god.noChanges': 'Sin cambios',
  'god.applyChanges': 'Aplicar {n} cambio(s)',
  'god.resetDefaults': 'Restaurar valores de fábrica',
  'god.instance': 'instancia',
  'god.kioskLabel': 'etiqueta (p. ej. pantalla de recepción)',
  'god.mintToken': 'Generar token',
  'god.noKiosk': 'No hay tokens de kiosco. Genera uno para arrancar un tablero de solo lectura.',
  'god.copyKiosk': 'copiar URL de kiosco',
  'god.revoke': 'revocar',

  'tune.group.pulse': 'Pulso',
  'tune.group.bubble': 'Burbujas',
  'tune.group.thread': 'Hilos',
  'tune.cycle_hours': 'Duración del ciclo (horas)',
  'tune.dormant_cycles': 'Ciclos antes de quedar dormida',
  'tune.decay_cycles': 'Decaimiento del puntaje (ciclos)',
  'tune.ownerless_is_dormant': 'Sin responsable pasa a dormida',
  'tune.bubble_rip_needs_owner': 'Dormida y sin responsable se lee como abandonada',
  'tune.bubble_level_rollup': 'La banda de la burbuja = su hilo más activo',
  'tune.thread_birth_heats': 'Crear un hilo lo calienta',
  'tune.thread_grace_cycles': 'Gracia para recién creados (ciclos)',
  'tune.thread_rip_needs_owner': 'Sin asignar y dormido se lee como abandonado',
  'tune.pulse_cycles': 'Un comentario mantiene vivo el hilo (ciclos)',
  'tune.thread_terminal_state_wins': 'Las columnas terminadas/canceladas de Plane mandan',

  'god.allOrgs': 'todas las orgs',
  'god.revision': 'revisión',
  'god.built': 'compilado',
  'god.started': 'iniciado',
  'god.instances': 'instancias',
  'god.identities': 'identidades',
  'god.noMirror': 'sin espejo',
  'god.slug': 'slug',
  'god.baseUrl': 'url base',
  'god.workspace': 'espacio de trabajo',
  'god.projectCol': 'proyecto',
  'god.webhook': 'webhook',
  'god.cached': 'en caché',
  'god.nameCol': 'nombre',
  'god.emailCol': 'correo',
  'god.roleCol': 'rol',
  'god.modified': 'modificado',
  'chrome.kioskBadge': 'kiosco',
  'combo.allInstances': 'todas las instancias',

  'god.writesToPlane': 'escribe en Plane',
  'omni.applyHint': '⏎ aplicar · esc volver',
  'omni.navHint': '↑↓ mover · ⏎ abrir · esc cerrar · escribe',
  'omni.forCommands': 'para comandos',
};

const catalogues: Record<Lang, Record<MsgKey, string>> = { en, es };

function detect(): Lang {
  const stored = localStorage.getItem(KEY);
  if (stored === 'en' || stored === 'es') return stored;
  // navigator.language is "es-MX", "es", "en-GB"… — only the primary subtag
  // matters, and anything we don't speak falls back to English.
  return (navigator.language || '').toLowerCase().startsWith('es') ? 'es' : 'en';
}

class I18n {
  lang = $state<Lang>(detect());

  constructor() {
    this.apply();
  }

  set(l: Lang): void {
    this.lang = l;
    localStorage.setItem(KEY, l);
    this.apply();
  }

  toggle(): void {
    this.set(this.lang === 'en' ? 'es' : 'en');
  }

  /** <html lang> so screen readers and hyphenation follow the choice too. */
  private apply(): void {
    document.documentElement.setAttribute('lang', this.lang);
  }

  get label(): string {
    return this.lang === 'es' ? 'Español' : 'English';
  }

  /** Region flag for the badge. Spanish here is Mexican Spanish, so MX rather
   *  than ES — the wording ("¿qué onda?" register, "responsable") is written for
   *  that audience, and a Spain flag would misstate it. */
  get flag(): string {
    return this.lang === 'es' ? '🇲🇽' : '🇺🇸';
  }

  /** Short code for the badge label, matching the theme badge's terseness. */
  get code(): string {
    return this.lang === 'es' ? 'MX' : 'US';
  }

  /**
   * Translate a key, substituting {placeholders}.
   *
   * A missing key returns the key itself rather than an empty string: a label
   * reading "bubble.owner" is an obvious bug, whereas a blank one looks like a
   * design choice and survives to production.
   */
  t(key: MsgKey, args?: Record<string, string | number>): string {
    const s = catalogues[this.lang][key] ?? en[key] ?? key;
    if (!args) return s;
    return s.replace(/\{(\w+)\}/g, (m, k) => (k in args ? String(args[k]) : m));
  }

  /** English-style plurals (one vs other), which both languages share here. */
  plural(key: MsgKey, n: number, args?: Record<string, string | number>): string {
    const k = (n === 1 ? key : `${key}_plural`) as MsgKey;
    return this.t(k in en ? k : key, { n, ...args });
  }

  /**
   * The heat sentence under a bubble. The server sends a stable code plus the
   * English sentence; we translate the code and fall back to the server's own
   * words when it sends a code this build has never heard of — a newer server
   * then degrades to English rather than to a raw identifier.
   */
  /** A calibration knob's label. Same contract as reason(): the server owns the
   *  English (so the CLI and God Mode cannot drift), we translate by its stable
   *  key, and a knob this build has not heard of shows the server's own words. */
  tuning(key: string, fallback: string): string {
    const k = `tune.${key}` as MsgKey;
    return k in en ? this.t(k) : fallback;
  }

  reason(code: string | undefined, fallback: string, args?: Record<string, string>): string {
    if (!code) return fallback;
    const key = `reason.${code}` as MsgKey;
    return key in en ? this.t(key, args) : fallback;
  }
}

export const i18n = new I18n();

/** Shorthand so markup reads `{t('bubble.open')}`. Reactive: reading i18n.lang
 *  inside makes every call site re-render on a language change. */
export function t(key: MsgKey, args?: Record<string, string | number>): string {
  return i18n.t(key, args);
}
