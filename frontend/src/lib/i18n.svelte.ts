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
