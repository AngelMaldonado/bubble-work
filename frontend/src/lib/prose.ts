// Post-processing for rendered markdown, shared by everything that shows a
// document: a thread's artifacts, a workspace's pages, a chat message.
//
// The server renders markdown to HTML (internal/md, goldmark), so the browser's
// job is only what HTML cannot express on its own: diagrams, the headings a
// table of contents needs, and making a checkbox a real control. It lives here
// rather than in a component because "the same render" has to mean the same code,
// not two copies that agree today.

export interface Heading {
  id: string;
  title: string;
  depth: number;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mermaidAPI: any = null;
let mmdSeq = 0;

/**
 * Replace ```mermaid code blocks with rendered SVG (mds-style), lazy-loading
 * mermaid only when a diagram is actually present — it is the single heaviest
 * dependency in the bundle and most documents have none.
 */
export async function renderMermaid(el: HTMLElement): Promise<void> {
  const blocks = [...el.querySelectorAll<HTMLElement>('pre > code.language-mermaid')];
  if (!blocks.length) return;
  mermaidAPI ||= (await import('mermaid')).default;
  const dark = document.documentElement.dataset.mode === 'dark';
  mermaidAPI.initialize({ startOnLoad: false, theme: dark ? 'dark' : 'default', securityLevel: 'strict' });
  for (const code of blocks) {
    const pre = code.parentElement;
    if (!pre) continue;
    const holder = document.createElement('div');
    holder.className = 'mermaid-diagram';
    try {
      const { svg } = await mermaidAPI.render(`mmd-${++mmdSeq}`, code.textContent ?? '');
      holder.innerHTML = svg;
    } catch (err) {
      holder.innerHTML = `<div class="mermaid-error">${String(err)}</div>`;
    }
    pre.replaceWith(holder);
  }
}

/** Extract the rendered headings, for a table of contents or a minimap. */
export function extractHeadings(el: HTMLElement): Heading[] {
  const hs = [...el.querySelectorAll<HTMLElement>('h1, h2, h3')].filter((h) => h.id);
  const levels = hs.map((h) => Number(h.tagName[1]));
  const min = levels.length ? Math.min(...levels) : 1;
  return hs.map((h) => ({
    id: h.id,
    // strip a leading emoji/symbol/checkbox (e.g. ⬜) so the TOC reads cleanly
    title: (h.textContent ?? '').replace(/^[^\p{L}\p{N}]+/u, '').trim(),
    depth: Number(h.tagName[1]) - min,
  }));
}

/**
 * goldmark writes checkboxes as `<input type=checkbox disabled>`, and a disabled
 * input receives no mouse events at all — so an inline todo would look like a
 * control and be dead. Plane's own taskList shape is not disabled, hence "if
 * present". Re-run on every render: {@html} replaces the nodes.
 *
 * Scoped to `[data-region]` because that is what marks a todo list the server can
 * actually be asked to tick. A page's checkboxes stay decorative: there is no
 * per-page toggle endpoint, and a control that silently does nothing is worse
 * than one that plainly cannot be clicked.
 */
export function enableCheckboxes(el: HTMLElement): void {
  for (const box of el.querySelectorAll<HTMLInputElement>(
    '[data-region] input[type=checkbox][disabled]',
  )) {
    box.disabled = false;
  }
}
