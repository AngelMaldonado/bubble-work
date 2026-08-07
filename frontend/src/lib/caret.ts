// Where is the caret, in pixels?
//
// A <textarea> will not tell you. The standard answer is to build a hidden div
// with the same typography and box metrics, put the text up to the caret inside
// it, and measure where a marker span lands. That is what this does.
//
// It exists so the slash menu can open AT the caret instead of at a corner —
// which is the whole difference between a command palette and a Notion-style
// `/` menu (docs/ARTIFACT-EDITING.md Phase 5).

// Everything that changes where a glyph lands. Miss one and the mirror drifts
// from the real text, subtly and only on some lines.
const MIRRORED = [
  'box-sizing',
  'width',
  'font-family',
  'font-size',
  'font-weight',
  'font-style',
  'font-variant',
  'letter-spacing',
  'line-height',
  'text-indent',
  'text-transform',
  'word-spacing',
  'white-space',
  'word-break',
  'overflow-wrap',
  'padding-top',
  'padding-right',
  'padding-bottom',
  'padding-left',
  'border-top-width',
  'border-right-width',
  'border-bottom-width',
  'border-left-width',
  'tab-size',
] as const;

export interface CaretPoint {
  /** Offsets from the textarea's own top-left, already adjusted for scroll. */
  top: number;
  left: number;
  /** One line's height, so a caller can place something just below the caret. */
  line: number;
}

export function caretPoint(el: HTMLTextAreaElement, index: number): CaretPoint {
  const style = getComputedStyle(el);
  const mirror = document.createElement('div');

  for (const prop of MIRRORED) mirror.style.setProperty(prop, style.getPropertyValue(prop));
  mirror.style.position = 'absolute';
  mirror.style.top = '0';
  mirror.style.left = '-9999px';
  mirror.style.visibility = 'hidden';
  mirror.style.whiteSpace = 'pre-wrap';
  mirror.style.overflowWrap = 'break-word';
  mirror.style.height = 'auto';

  mirror.textContent = el.value.slice(0, index);
  const marker = document.createElement('span');
  // A caret at the very end has nothing after it to measure, and an empty span
  // has no box — so give it something with width.
  marker.textContent = el.value.slice(index) || '.';
  mirror.appendChild(marker);

  document.body.appendChild(mirror);
  const top = marker.offsetTop;
  const left = marker.offsetLeft;
  document.body.removeChild(mirror);

  const line = parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.4 || 16;
  return { top: top - el.scrollTop, left: left - el.scrollLeft, line };
}
