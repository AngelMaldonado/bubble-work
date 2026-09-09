// A real markdown editor, built on CodeMirror 6.
//
// This replaced a bare <textarea>, which was the honest first cut and a poor
// one: no syntax highlighting, no list continuation, no undo grouping, and a
// hand-rolled hidden-div measurement to find the caret. CodeMirror gives all of
// that, and `@codemirror/lang-markdown` in particular ships the two commands
// that make markdown editing feel like markdown rather than like typing into a
// box — Enter continues a list or checkbox, Backspace unwinds the marker.
//
// It lives behind a dynamic import (see ThreadView) so none of it lands in the
// main bundle: CodeMirror and the vim keymap only exist once somebody switches
// to Markdown, and vim only once they ask for vim.
//
// Ported from v0 unchanged apart from the palette — `--wip` is `--accent` here,
// which means the caret and the selection take the BAND's colour and the editor
// agrees with the page it is inside.

import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { EditorState, Prec, type Extension } from '@codemirror/state';
import { drawSelection, EditorView, keymap, placeholder as cmPlaceholder } from '@codemirror/view';
import { tags } from '@lezer/highlight';
// What to do with an Escape that was taken away from this editor, keyed by the
// editor's element. See `lib/escape.ts` for why the key is caught at the window.
import { escapeHandlers } from './escape';

export interface CaretPoint {
  /** Relative to the editor's own box, so a menu can be positioned inside it. */
  top: number;
  left: number;
  line: number;
}

export interface MarkdownEditor {
  destroy(): void;
  value(): string;
  /** Replace the whole document — for when the SERVER's copy changed. */
  setDoc(text: string): void;
  focus(): void;
  cursor(): number;
  caret(): CaretPoint | null;
  /** Replace a range and place the caret at an offset inside the new text. */
  replace(from: number, to: number, text: string, caretOffset: number): void;
  /** Wrap the selection (or a placeholder) in a marker, e.g. ** for bold. */
  wrap(marker: string, fallback: string): void;
  /** Prefix every selected line, e.g. "- " for a bullet. */
  prefix(text: string): void;
}

export interface EditorOptions {
  parent: HTMLElement;
  doc: string;
  dark: boolean;
  /** vim keybindings. Loaded on demand, so non-vim users never download it. */
  vim?: boolean;
  /** Where to put the caret on creation — used to survive a mode switch. */
  cursor?: number;
  placeholder?: string;
  onChange: (doc: string) => void;
  /** Return true to swallow the key — used while the slash menu is open. */
  onKey?: (key: string) => boolean;
  onSave: () => void;
  onEscape: () => void;
  onBlur: () => void;
}

// Markdown's own vocabulary, in the app's palette. Deliberately restrained: the
// point is to make structure legible at a glance, not to turn a Brief into a
// christmas tree.
function highlight(): Extension {
  const style = HighlightStyle.define([
    { tag: tags.heading1, fontWeight: '800', fontSize: '1.25em' },
    { tag: tags.heading2, fontWeight: '750', fontSize: '1.14em' },
    {
      tag: [tags.heading3, tags.heading4, tags.heading5, tags.heading6],
      fontWeight: '700',
    },
    { tag: tags.strong, fontWeight: '750' },
    { tag: tags.emphasis, fontStyle: 'italic' },
    { tag: tags.strikethrough, textDecoration: 'line-through' },
    { tag: tags.link, color: 'var(--accent)', textDecoration: 'underline' },
    { tag: tags.url, color: 'var(--muted)' },
    { tag: tags.monospace, color: 'var(--accent)' },
    { tag: tags.quote, color: 'var(--muted)', fontStyle: 'italic' },
    // The marker characters themselves — "##", "-", "*" — recede so the words
    // they decorate stay the thing you read.
    { tag: tags.processingInstruction, color: 'var(--faint)' },
    { tag: tags.contentSeparator, color: 'var(--faint)' },
    { tag: tags.list, color: 'var(--text)' },
  ]);
  return syntaxHighlighting(style);
}

function theme(dark: boolean): Extension {
  return EditorView.theme(
    {
      '&': {
        color: 'var(--text)',
        backgroundColor: 'transparent',
        fontSize: '0.88rem',
        height: '100%',
      },
      // The scroller is CodeMirror's own, and it is the one that should move.
      // overflow is explicit because the host element hides its overflow: with
      // both set to auto you get an outer scrollbar that scrolls nothing.
      '.cm-scroller': {
        fontFamily: 'var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace)',
        lineHeight: '1.7',
        padding: '0.9rem 0',
        overflow: 'auto',
      },
      '.cm-content': { padding: '0 1rem', caretColor: 'var(--accent)' },
      '&.cm-focused': { outline: 'none' },
      '.cm-cursor, .cm-dropCursor': {
        borderLeftColor: 'var(--accent)',
        borderLeftWidth: '2px',
      },
      '.cm-activeLine': {
        backgroundColor: 'color-mix(in oklab, var(--accent) 6%, transparent)',
      },
      '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
        backgroundColor: 'color-mix(in oklab, var(--accent) 24%, transparent)',
      },
      '.cm-placeholder': { color: 'var(--faint)' },
    },
    { dark },
  );
}

// :w and :q have to reach THIS editor, but `Vim.defineEx` is global and there
// can be more than one editor on a screen. So the commands are defined once and
// dispatch through a per-view registry.
const vimHooks = new WeakMap<EditorView, { save: () => void; done: () => void }>();
let vimExDefined = false;

async function vimExtensions(opts: EditorOptions) {
  const { vim, Vim, getCM } = await import('@replit/codemirror-vim');
  if (!vimExDefined) {
    vimExDefined = true;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const hooks = (cm: any) => vimHooks.get(cm?.cm6);
    Vim.defineEx('write', 'w', (cm: unknown) => hooks(cm)?.save());
    Vim.defineEx('quit', 'q', (cm: unknown) => hooks(cm)?.done());
    const writeQuit = (cm: unknown) => {
      const h = hooks(cm);
      h?.save();
      h?.done();
    };
    Vim.defineEx('wq', 'wq', writeQuit);
    Vim.defineEx('xit', 'x', writeQuit);
  }
  // vim() must come before every other keymap, and drawSelection is what makes
  // visual mode render correctly when you are not using basicSetup.
  return { exts: [vim({ status: true }), drawSelection()], Vim, getCM };
}

export async function createMarkdownEditor(opts: EditorOptions): Promise<MarkdownEditor> {
  const loaded = opts.vim ? await vimExtensions(opts) : null;
  const vimExt = loaded?.exts ?? [];
  // The slash menu owns these keys while it is open, so they bind ABOVE the
  // default keymap — otherwise Enter would insert a newline before the menu
  // ever saw it.
  const intercept = Prec.highest(
    keymap.of(
      ['ArrowDown', 'ArrowUp', 'Enter', 'Tab', 'Escape'].map((key) => ({
        key,
        run: () => (opts.onKey ? opts.onKey(key) : false),
      })),
    ),
  );

  const commands = keymap.of([
    { key: 'Mod-s', preventDefault: true, run: () => (opts.onSave(), true) },
    // In vim, Escape means "leave insert mode" and belongs to vim. Leaving the
    // editor is `:q` there, which is what a vim user reaches for anyway.
    ...(opts.vim ? [] : [{ key: 'Escape', run: () => (opts.onEscape(), true) }]),
  ]);

  const view = new EditorView({
    parent: opts.parent,
    state: EditorState.create({
      doc: opts.doc,
      selection:
        opts.cursor != null ? { anchor: Math.min(opts.cursor, opts.doc.length) } : undefined,
      extensions: [
        ...vimExt,
        history(),
        intercept,
        // markdown() brings its own keymap: Enter continues a list or task
        // item, Backspace unwinds the marker. That is the single biggest
        // difference between this and a textarea.
        markdown({ base: markdownLanguage }),
        highlight(),
        theme(opts.dark),
        EditorView.lineWrapping,
        keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
        commands,
        opts.placeholder ? cmPlaceholder(opts.placeholder) : [],
        EditorView.updateListener.of((u) => {
          if (u.docChanged || u.selectionSet) opts.onChange(u.state.doc.toString());
        }),
        EditorView.domEventHandlers({ blur: () => (opts.onBlur(), false) }),
      ],
    }),
  });

  // The Escape this editor never receives, handed to it by name.
  //
  // A dialog swallows the key before it can arrive (`lib/escape.ts`), so the
  // interceptor calls this instead of trying to fake a keypress. In vim it is
  // vim's own `<Esc>` — leave insert mode, cancel a pending command. Without
  // vim it is what Escape has always meant here: stop editing.
  escapeHandlers.set(view.dom, () => {
    const cm = loaded?.getCM(view);
    if (cm) return void loaded!.Vim.handleKey(cm, '<Esc>', 'user');
    opts.onEscape();
  });

  if (opts.vim) {
    vimHooks.set(view, { save: opts.onSave, done: opts.onEscape });
  }

  // Password managers attach to anything that looks like a field, and
  // CodeMirror's editing surface is a `contenteditable` — which 1Password reads
  // as one, and then offers to save what is being typed as a login. These
  // attributes are how each of them is told to leave an element alone; the only
  // credential fields in this product are the two on the sign-in screen.
  view.contentDOM.setAttribute('data-1p-ignore', '');
  view.contentDOM.setAttribute('data-lpignore', 'true');
  view.contentDOM.setAttribute('data-bwignore', '');
  view.contentDOM.setAttribute('data-form-type', 'other');
  view.contentDOM.setAttribute('autocomplete', 'off');
  view.contentDOM.setAttribute('autocorrect', 'off');
  view.contentDOM.setAttribute('spellcheck', 'false');

  const api: MarkdownEditor = {
    destroy: () => {
      escapeHandlers.delete(view.dom);
      view.destroy();
    },
    value: () => view.state.doc.toString(),
    setDoc(text) {
      if (view.state.doc.toString() === text) return;
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
        selection: { anchor: Math.min(view.state.selection.main.head, text.length) },
      });
    },
    focus: () => view.focus(),
    cursor: () => view.state.selection.main.head,

    caret() {
      const pos = view.state.selection.main.head;
      const rect = view.coordsAtPos(pos);
      if (!rect) return null;
      // coordsAtPos is viewport-relative; the menu is positioned inside the
      // editor's box, so subtract the box.
      const box = view.dom.getBoundingClientRect();
      return {
        top: rect.top - box.top,
        left: rect.left - box.left,
        line: rect.bottom - rect.top,
      };
    },

    replace(from, to, text, caretOffset) {
      view.dispatch({
        changes: { from, to, insert: text },
        selection: { anchor: from + caretOffset },
        scrollIntoView: true,
      });
      view.focus();
    },

    wrap(marker, fallback) {
      const { from, to } = view.state.selection.main;
      const chosen = view.state.sliceDoc(from, to) || fallback;
      view.dispatch({
        changes: { from, to, insert: marker + chosen + marker },
        selection: {
          anchor: from + marker.length,
          head: from + marker.length + chosen.length,
        },
      });
      view.focus();
    },

    prefix(text) {
      const { from, to } = view.state.selection.main;
      const first = view.state.doc.lineAt(from);
      const last = view.state.doc.lineAt(to);
      const changes = [];
      for (let n = first.number; n <= last.number; n++) {
        const line = view.state.doc.line(n);
        changes.push({ from: line.from, insert: text });
      }
      view.dispatch({ changes });
      view.focus();
    },
  };
  return api;
}
