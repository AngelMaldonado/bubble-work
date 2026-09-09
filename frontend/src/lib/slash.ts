// Finding the "/" token a slash menu is filtering on
// (docs/journal/ARTIFACT-EDITING.md Phase 5).
//
// Split out from the editor because it is pure and because the boundary rule is
// the part that regresses: a URL is full of slashes and must never open a menu.

export interface SlashToken {
  /** Index of the "/" itself, so a command can replace the whole token. */
  at: number;
  /** What has been typed after it, for filtering. */
  query: string;
}

// A slash only counts at the start of a line or after whitespace, and the query
// runs to the caret with no whitespace or second slash in between.
const TOKEN = /(?:^|\s)\/([^\s/]*)$/;

export function findSlash(text: string, caret: number): SlashToken | null {
  const upto = text.slice(0, caret);
  const lineStart = upto.lastIndexOf('\n') + 1;
  const line = upto.slice(lineStart);
  const m = TOKEN.exec(line);
  if (!m) return null;
  return { at: lineStart + m.index + m[0].indexOf('/'), query: m[1] };
}
