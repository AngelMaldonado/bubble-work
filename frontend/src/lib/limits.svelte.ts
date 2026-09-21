/**
 * Cuánto cabe en cada campo, dicho ANTES de guardar.
 *
 * El servidor rechaza lo que no cabe, y lo hacía bien —con un 400 que nadie
 * veía—. Esta pantalla guarda sola: al dejar de escribir, al salir de un campo,
 * al cerrar un panel. Un rechazo en ese momento no tiene dónde aparecer, y lo
 * que se había escrito se perdía al recargar sin que nada lo hubiera dicho. Lo
 * peor que puede hacer un editor.
 *
 * Así que el tope se enseña mientras se escribe —cerca de él, cuánto queda; al
 * pasarlo, por cuánto— y lo que no cabe no se manda: se queda en el campo, en
 * rojo, hasta que alguien lo recorte.
 *
 * Los topes vienen del esquema del servidor (`/api/limits`), no de una copia
 * aquí: dos listas se separan, y la que se queda atrás dice «cabe» sobre algo
 * que se va a rechazar.
 */
import type { Attachment } from 'svelte/attachments';
import { api } from './api';

let table = $state<Record<string, number>>({});
/** Las extensiones que el servidor admite en `assets/`, para el diálogo de
 *  archivos. Vacío mientras no se sabe: un `accept` vacío deja elegir
 *  cualquier cosa y que conteste el servidor, que es mejor que filtrar por una
 *  lista adivinada. */
let uploads = $state<string[]>([]);
let asking: Promise<void> | null = null;

/** Pide los topes una vez. Sin sesión todavía falla, y se vuelve a pedir en el
 *  siguiente uso; mientras tanto no se avisa de nada y el servidor sigue
 *  siendo quien decide. */
function ready(): Promise<void> {
  asking ??= api
    .limits()
    .then((t) => {
      table = t.fields ?? {};
      uploads = t.accept ?? [];
    })
    .catch(() => {
      asking = null;
    });
  return asking;
}

/** Lo que el diálogo de archivos debe ofrecer: `.png,.pdf,…`. Vacío mientras no
 *  se sabe, que deja elegir cualquier cosa — el servidor sigue decidiendo. */
export function acceptUploads(): string {
  ready();
  return uploads.join(',');
}

/** Cuánto cabe en `colección.campo`, o 0 si no se sabe (aún). */
export function limitOf(key: string): number {
  ready();
  return table[key] ?? 0;
}

export type Fit = { used: number; max: number; over: number; near: boolean };

/** Cómo va un texto contra su tope. `null` si ese campo no tiene tope conocido.
 *
 *  El servidor cuenta CARACTERES (runas), y `length` cuenta unidades UTF-16: un
 *  emoji vale dos. Contar runas en cada tecla sobre dos millones de caracteres
 *  no es gratis, así que se cuenta barato y sólo cerca del tope se cuenta bien —
 *  lejos de él, la diferencia no cambia la respuesta. */
export function fit(key: string, value: string | null | undefined): Fit | null {
  const max = limitOf(key);
  if (!max) return null;
  const text = value ?? '';
  let used = text.length;
  if (used >= max * 0.9) {
    used = 0;
    for (const _ of text) used++;
  }
  return { used, max, over: Math.max(0, used - max), near: used >= max * 0.9 };
}

const n = (x: number) => x.toLocaleString('es');

/** Lo que se le dice a quien se pasó. Qué pasa, por cuánto, y qué hacer. */
export function overText(f: Fit): string {
  return `Se pasa por ${n(f.over)} caracteres (${n(f.used)} de ${n(f.max)}). Así no se guarda: recórtalo.`;
}

/** El mensaje si `value` no cabe en `key`; `null` si cabe. Para quien guarda
 *  sin pasar por un campo con `limited`: un panel que guarda al cerrarse. */
export function tooLong(key: string, value: string | null | undefined): string | null {
  const f = fit(key, value);
  return f && f.over ? overText(f) : null;
}

/**
 * Un campo que sabe su tope: `{@attach limited('bubbles.name')}`.
 *
 * - Cerca del tope enseña cuánto queda; pasado, por cuánto se pasa, debajo del
 *   campo y en rojo — mientras dure, esté el foco donde esté.
 * - Pasado, NO deja que el campo se guarde, sea cual sea el gesto que guarda
 *   aquí: Enter (o ⌘/Ctrl+Enter en un área de texto), salir del campo, el
 *   `change` o enviar el formulario. El texto se queda donde está.
 *
 * Lo segundo se hace deteniendo el evento antes de que llegue a quien guarda,
 * en vez de pedirle a cada campo que compruebe: son treinta campos, y el que se
 * olvide de comprobarlo es el que vuelve a perder algo en silencio.
 */
export function limited(key: string): Attachment<HTMLElement> {
  return (el) => {
    const input = el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement ? el : null;
    const read = () => (input ? input.value : (el.textContent ?? ''));

    // En la capa superior (`popover`), no al lado del campo: los campos viven
    // dentro de flex, de diálogos y de tarjetas con `overflow: hidden`, y un
    // aviso que se corta o descoloca la fila no avisa de nada.
    const hint = document.createElement('div');
    hint.className = 'limit-hint';
    hint.setAttribute('popover', 'manual');
    hint.setAttribute('role', 'status');
    hint.setAttribute('aria-live', 'polite');
    document.body.append(hint);

    let state: Fit | null = null;
    const over = () => !!state?.over;

    const place = () => {
      const r = el.getBoundingClientRect();
      hint.style.left = `${Math.max(8, r.left)}px`;
      hint.style.top = `${r.bottom + 4}px`;
      hint.style.maxWidth = `${Math.max(220, r.width)}px`;
    };

    const update = () => {
      state = fit(key, read());
      const bad = over();
      input?.setCustomValidity(bad ? overText(state!) : '');
      if (bad) el.setAttribute('aria-invalid', 'true');
      else el.removeAttribute('aria-invalid');

      const show = !!state && el.isConnected && (bad || (state.near && document.activeElement === el));
      if (!show) {
        if (hint.matches(':popover-open')) hint.hidePopover();
        return;
      }
      hint.textContent = bad ? overText(state!) : `${n(state!.used)} / ${n(state!.max)}`;
      hint.classList.toggle('over', bad);
      place();
      if (!hint.matches(':popover-open')) hint.showPopover();
    };

    /** Un intento de guardar lo que no cabe: se para, y se dice otra vez. */
    const refuse = (e: Event) => {
      e.preventDefault();
      e.stopImmediatePropagation();
      hint.classList.remove('nudge');
      void hint.offsetWidth; // reinicia la animación
      hint.classList.add('nudge');
    };

    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Enter' || !over()) return;
      // En un área de texto, Enter solo es un salto de línea, no guardar.
      if (el instanceof HTMLTextAreaElement && !(e.metaKey || e.ctrlKey)) return;
      if (e.shiftKey && !(e.metaKey || e.ctrlKey)) return;
      refuse(e);
    };
    const onLeave = (e: Event) => {
      if (over()) refuse(e);
      else update();
    };
    const onSubmit = (e: Event) => {
      update();
      if (over()) refuse(e);
    };
    const onMove = () => {
      if (hint.matches(':popover-open')) place();
    };

    // En CAPTURA: en el propio campo, los de captura corren antes que los
    // `onblur`/`onchange` que guardan, y `stopImmediatePropagation` los salta.
    el.addEventListener('input', update);
    el.addEventListener('focus', update);
    el.addEventListener('keydown', onKey, true);
    el.addEventListener('blur', onLeave, true);
    el.addEventListener('change', onLeave, true);
    const form = input?.form ?? null;
    form?.addEventListener('submit', onSubmit, true);
    window.addEventListener('scroll', onMove, true);
    window.addEventListener('resize', onMove);

    // Un campo que llega ya con demasiado —lo que se trajo del servidor antes
    // de que bajara un tope— se avisa al aparecer, no a la primera tecla.
    ready().then(update);
    // `bind:value` cambia el valor sin evento `input` (al abrir otra tarjeta).
    const watch = new MutationObserver(update);
    if (!input) watch.observe(el, { characterData: true, subtree: true, childList: true });

    return () => {
      el.removeEventListener('input', update);
      el.removeEventListener('focus', update);
      el.removeEventListener('keydown', onKey, true);
      el.removeEventListener('blur', onLeave, true);
      el.removeEventListener('change', onLeave, true);
      form?.removeEventListener('submit', onSubmit, true);
      window.removeEventListener('scroll', onMove, true);
      window.removeEventListener('resize', onMove);
      watch.disconnect();
      hint.remove();
    };
  };
}
