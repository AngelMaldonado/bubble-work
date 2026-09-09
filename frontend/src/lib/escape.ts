/**
 * Escape, inside a text editor, belongs to the editor.
 *
 * The problem, read out of Zag rather than guessed at. A dialog tracks the key
 * like this:
 *
 *     addDomEvent(getDocument(node), "keydown", handleKeyDown, { capture: true })
 *     …
 *     if (!event.defaultPrevented && onDismiss) { event.preventDefault(); onDismiss() }
 *
 * Document, CAPTURE phase — so it runs before the keypress ever reaches what has
 * focus — and whichever way it decides, it calls `preventDefault()`: to dismiss,
 * or, with `closeOnEscape` off, to refuse. CodeMirror then skips every handler
 * it has (`if (event.defaultPrevented) break`), so vim never leaves insert mode.
 *
 * That is why the two obvious fixes both failed. Stopping the event on the way
 * up is too late; turning `closeOnEscape` off still poisons the event.
 *
 * What actually works is being EARLIER. The capture path starts at the window,
 * one step above the document, so a listener here sees the key before any
 * dialog can — no registration-order race, just the shape of event propagation.
 * When the key belongs to an editor we stop it there and CALL that editor's own
 * handler — vim's `<Esc>`, or "stop editing" when vim is off. Faking a keypress
 * was the first attempt and it depends on too much: which element CodeMirror
 * listens on, whether a synthetic event is treated like a real one. A function
 * the editor registered when it was built depends on nothing.
 *
 * Everywhere else Escape is untouched: a dialog with nothing being typed in it
 * still closes, which is what everyone expects of it.
 *
 * The registry lives HERE rather than in `lib/editor.ts` for a boring reason:
 * `main.ts` imports this at startup, and importing the editor from it would
 * drag CodeMirror into the main bundle — which is exactly what the dynamic
 * import over there exists to avoid.
 */
export const escapeHandlers = new WeakMap<Element, () => void>();

export function letEditorsKeepEscape() {
  addEventListener(
    'keydown',
    (e) => {
      if (e.key !== 'Escape') return;
      const el = e.target as HTMLElement | null;
      const editor = el?.closest?.('.cm-editor');
      if (!editor) return;

      const handle = escapeHandlers.get(editor);
      if (!handle) return;

      // Stop first, act second. The dialog below is listening on the document
      // and would take the key away; the editor is handed it by name instead of
      // through a keypress nobody can guarantee arrives.
      e.stopPropagation();
      handle();
    },
    { capture: true },
  );
}
