/**
 * The slash menu's state, ported from v0's `ArtifactEditor`.
 *
 * The behaviour is v0's, unchanged, because it was right: type `/` on a fresh
 * line or after a space, filter as you type, Enter inserts. Every command
 * inserts plain MARKDOWN — the buffer is markdown, there is no hidden document
 * model that could disagree with the text on screen.
 *
 * What is new is the shape: v0 had one editor and could keep this inside it,
 * and here two screens host editors — a thread's document and a card's
 * description. So the state lives in a class both can instantiate and the menu
 * is a component both can render, rather than a second copy of the same twelve
 * commands drifting from the first.
 *
 * `findSlash` came across verbatim in `lib/slash.ts`: the boundary rule is the
 * part that regresses, and a URL is full of slashes that must never open a menu.
 */
import type { MarkdownEditor } from './editor';
import { findSlash } from './slash';

export interface Command {
  id: string;
  label: string;
  glyph: string;
  /** Markdown to insert, and where the caret lands inside it. */
  text: string;
  caret: number;
  /** Extra words to match on, so "checkbox" finds the to-do. */
  alias?: string;
}

export const COMMANDS: Command[] = [
  { id: 'h1', label: 'Título', glyph: 'H1', text: '# ', caret: 2, alias: 'titulo heading h1' },
  { id: 'h2', label: 'Subtítulo', glyph: 'H2', text: '## ', caret: 3, alias: 'subtitulo heading h2' },
  { id: 'h3', label: 'Apartado', glyph: 'H3', text: '### ', caret: 4, alias: 'heading h3' },
  { id: 'bullet', label: 'Lista', glyph: '•', text: '- ', caret: 2, alias: 'list ul viñeta' },
  { id: 'numbered', label: 'Lista numerada', glyph: '1.', text: '1. ', caret: 3, alias: 'list ol ordenada' },
  { id: 'todo', label: 'Casilla', glyph: '☐', text: '- [ ] ', caret: 6, alias: 'task checkbox tarea pendiente' },
  { id: 'quote', label: 'Cita', glyph: '❝', text: '> ', caret: 2, alias: 'blockquote' },
  { id: 'code', label: 'Código', glyph: '</>', text: '```\n\n```\n', caret: 4, alias: 'pre fence bloque' },
  {
    id: 'mermaid',
    label: 'Diagrama',
    glyph: '◇',
    text: '```mermaid\ngraph TD\n  A --> B\n```\n',
    caret: 11,
    alias: 'mermaid diagram grafico flujo',
  },
  {
    id: 'excalidraw',
    label: 'Dibujo',
    glyph: '✎',
    text: '```excalidraw\n{"elements":[]}\n```\n',
    caret: 14,
    alias: 'excalidraw boceto sketch',
  },
  { id: 'table', label: 'Tabla', glyph: '▦', text: '| a | b |\n| --- | --- |\n|  |  |\n', caret: 2, alias: 'grid' },
  { id: 'divider', label: 'Separador', glyph: '—', text: '---\n', caret: 4, alias: 'hr rule linea' },
  { id: 'link', label: 'Enlace', glyph: '🔗', text: '[](url)', caret: 1, alias: 'url href' },
];

export type Anchor = {
  /** index of the `/` itself, so a command replaces the whole token */
  at: number;
  query: string;
  /** just below the caret — where the menu sits when it opens downwards */
  top: number;
  /** the caret's own top — where the menu's BOTTOM goes when it flips up */
  caretTop: number;
  left: number;
};

export class SlashMenu {
  open = $state<Anchor | null>(null);
  picked = $state(0);

  matches = $derived(
    this.open
      ? COMMANDS.filter((c) => {
          const q = this.open!.query.toLowerCase();
          if (!q) return true;
          return (c.label.toLowerCase() + ' ' + c.id + ' ' + (c.alias ?? '')).includes(q);
        })
      : [],
  );

  /** Look at the text under the caret and open, move or close the menu. */
  detect(editor: MarkdownEditor | null, text: string) {
    if (!editor) return;
    const found = findSlash(text, editor.cursor());
    const point = found ? editor.caret() : null;
    if (!found || !point) {
      this.open = null;
      return;
    }
    this.open = { ...found, top: point.top + point.line, caretTop: point.top, left: point.left };
    this.picked = 0;
  }

  run(editor: MarkdownEditor | null, text: string, c: Command) {
    const s = this.open;
    if (!s || !editor) return;
    const end = editor.cursor();
    // Every one of these is a block, so it starts its own line — otherwise
    // "nota /todo" would render as one paragraph rather than a checkbox.
    const before = text.slice(0, s.at);
    const lead = before === '' || before.endsWith('\n') ? '' : '\n';
    editor.replace(s.at, end, lead + c.text, lead.length + c.caret);
    this.open = null;
  }

  /** The keys the menu owns while it is open. True when it consumed one. */
  key(k: string, editor: MarkdownEditor | null, text: string): boolean {
    if (!this.open) return false;
    const n = this.matches.length;
    switch (k) {
      case 'ArrowDown':
        this.picked = n ? (this.picked + 1) % n : 0;
        return true;
      case 'ArrowUp':
        this.picked = n ? (this.picked - 1 + n) % n : 0;
        return true;
      case 'Enter':
      case 'Tab': {
        const c = this.matches[this.picked];
        if (!c) return false;
        this.run(editor, text, c);
        return true;
      }
      case 'Escape':
        this.open = null;
        return true;
    }
    return false;
  }
}
