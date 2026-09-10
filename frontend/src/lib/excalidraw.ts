// Excalidraw scenes, drawn to SVG.
//
// Ours rather than Excalidraw's own: the official renderer needs React and
// ReactDOM as peer dependencies, and putting a second framework in a Svelte app
// to convert JSON into an <svg> is a large, permanent cost for one file type.
// `@excalidraw/utils` is the same code without React, but its latest npm
// release is tagged `0.1.3-test32`, and a dependency on a test build is one
// that can break or vanish without notice.
//
// So: roughjs — the library Excalidraw itself draws with, 170 KB — plus the
// element types that actually appear. Anything we do not know how to draw says
// so, in place, instead of being silently dropped: a diagram that is quietly
// missing a shape is worse than one that admits it.
import rough from 'roughjs';

type Point = [number, number];

interface Element {
  type: string;
  x: number;
  y: number;
  width: number;
  height: number;
  angle?: number;
  strokeColor?: string;
  backgroundColor?: string;
  fillStyle?: string;
  strokeWidth?: number;
  strokeStyle?: string;
  roughness?: number;
  seed?: number;
  opacity?: number;
  points?: Point[];
  text?: string;
  fontSize?: number;
  fontFamily?: number;
  textAlign?: string;
  isDeleted?: boolean;
}

export interface Scene {
  type?: string;
  elements?: Element[];
  appState?: { viewBackgroundColor?: string };
}

/** The three faces Excalidraw ships, by its own numeric ids. */
const FONTS: Record<number, string> = {
  1: "'Excalifont', 'Comic Sans MS', ui-rounded, cursive",
  2: 'ui-sans-serif, system-ui, sans-serif',
  3: 'ui-monospace, SFMono-Regular, Menlo, monospace',
};

const PAD = 16;

function bounds(els: Element[]) {
  let x1 = Infinity;
  let y1 = Infinity;
  let x2 = -Infinity;
  let y2 = -Infinity;
  for (const e of els) {
    // A line's points are relative to its x/y and can run negative, so the box
    // is not simply x..x+width.
    const xs = [e.x, e.x + (e.width || 0)];
    const ys = [e.y, e.y + (e.height || 0)];
    for (const [px, py] of e.points ?? []) {
      xs.push(e.x + px);
      ys.push(e.y + py);
    }
    x1 = Math.min(x1, ...xs);
    y1 = Math.min(y1, ...ys);
    x2 = Math.max(x2, ...xs);
    y2 = Math.max(y2, ...ys);
  }
  if (!Number.isFinite(x1)) return { x1: 0, y1: 0, x2: 100, y2: 100 };
  return { x1, y1, x2, y2 };
}

function options(e: Element) {
  return {
    stroke: e.strokeColor ?? '#1e1e1e',
    strokeWidth: e.strokeWidth ?? 1,
    fill: e.backgroundColor && e.backgroundColor !== 'transparent' ? e.backgroundColor : undefined,
    fillStyle: e.fillStyle ?? 'hachure',
    roughness: e.roughness ?? 1,
    // The seed is what makes a drawing look the SAME every time it is opened.
    // Without it roughjs re-randomises the wobble on every render and the
    // diagram appears to move between visits.
    seed: e.seed ?? 1,
    strokeLineDash:
      e.strokeStyle === 'dashed' ? [8, 8] : e.strokeStyle === 'dotted' ? [2, 6] : undefined,
  };
}

/** Renders a scene into a fresh <svg>. Throws nothing: a bad element is drawn
 *  as a note about itself. */
export function sceneToSvg(scene: Scene): SVGSVGElement {
  const els = (scene.elements ?? []).filter((e) => !e.isDeleted);
  const { x1, y1, x2, y2 } = bounds(els);
  const w = Math.max(1, x2 - x1) + PAD * 2;
  const h = Math.max(1, y2 - y1) + PAD * 2;

  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', `0 0 ${w} ${h}`);
  svg.setAttribute('width', String(w));
  svg.setAttribute('height', String(h));
  svg.style.maxWidth = '100%';
  svg.style.height = 'auto';

  const root = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  root.setAttribute('transform', `translate(${PAD - x1} ${PAD - y1})`);
  svg.append(root);

  const rc = rough.svg(svg);

  for (const e of els) {
    const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
    if (e.opacity != null && e.opacity !== 100) g.setAttribute('opacity', String(e.opacity / 100));
    if (e.angle) {
      const cx = e.x + (e.width || 0) / 2;
      const cy = e.y + (e.height || 0) / 2;
      g.setAttribute('transform', `rotate(${(e.angle * 180) / Math.PI} ${cx} ${cy})`);
    }

    try {
      g.append(...draw(rc, e));
    } catch (err) {
      g.append(note(e, String(err)));
    }
    root.append(g);
  }

  return svg;
}

function draw(rc: ReturnType<typeof rough.svg>, e: Element): Node[] {
  const o = options(e);
  switch (e.type) {
    case 'rectangle':
      return [rc.rectangle(e.x, e.y, e.width, e.height, o)];
    case 'ellipse':
      return [rc.ellipse(e.x + e.width / 2, e.y + e.height / 2, e.width, e.height, o)];
    case 'diamond': {
      const cx = e.x + e.width / 2;
      const cy = e.y + e.height / 2;
      return [
        rc.polygon(
          [
            [cx, e.y],
            [e.x + e.width, cy],
            [cx, e.y + e.height],
            [e.x, cy],
          ],
          o,
        ),
      ];
    }
    case 'line':
    case 'arrow': {
      const pts = (e.points ?? []).map(([px, py]) => [e.x + px, e.y + py] as Point);
      if (pts.length < 2) return [];
      const out: Node[] = [rc.linearPath(pts, o)];
      if (e.type === 'arrow') out.push(head(rc, pts, o));
      return out;
    }
    case 'freedraw': {
      const pts = (e.points ?? []).map(([px, py]) => [e.x + px, e.y + py] as Point);
      // A hand-drawn stroke is already irregular; roughening it again turns it
      // into noise.
      return pts.length < 2 ? [] : [rc.linearPath(pts, { ...o, roughness: 0 })];
    }
    case 'text':
      return [text(e)];
    case 'image':
      // The bytes live in the workspace's assets, not in the scene.
      return [note(e, 'imagen')];
    default:
      return [note(e, e.type)];
  }
}

/** The arrowhead, as two short lines off the last segment. */
function head(rc: ReturnType<typeof rough.svg>, pts: Point[], o: ReturnType<typeof options>): Node {
  const [x2, y2] = pts[pts.length - 1];
  const [x1, y1] = pts[pts.length - 2];
  const a = Math.atan2(y2 - y1, x2 - x1);
  const len = 14;
  const spread = Math.PI / 7;
  const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  for (const s of [a + Math.PI - spread, a + Math.PI + spread]) {
    g.append(rc.line(x2, y2, x2 + Math.cos(s) * len, y2 + Math.sin(s) * len, { ...o, roughness: 0 }));
  }
  return g;
}

function text(e: Element): Node {
  const size = e.fontSize ?? 20;
  const t = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  t.setAttribute('x', String(e.textAlign === 'center' ? e.x + e.width / 2 : e.x));
  t.setAttribute('y', String(e.y));
  t.setAttribute('fill', e.strokeColor ?? '#1e1e1e');
  t.setAttribute('font-size', String(size));
  t.setAttribute('font-family', FONTS[e.fontFamily ?? 1] ?? FONTS[1]);
  t.setAttribute('dominant-baseline', 'hanging');
  if (e.textAlign === 'center') t.setAttribute('text-anchor', 'middle');
  // SVG has no line wrapping: every line is its own <tspan>.
  const lines = (e.text ?? '').split('\n');
  lines.forEach((line, i) => {
    const span = document.createElementNS('http://www.w3.org/2000/svg', 'tspan');
    span.setAttribute('x', t.getAttribute('x')!);
    span.setAttribute('dy', i === 0 ? '0' : `${size * 1.25}`);
    span.textContent = line;
    t.append(span);
  });
  return t;
}

/** What an element we cannot draw looks like: a dashed box saying what it is. */
function note(e: Element, what: string): Node {
  const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  const box = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
  box.setAttribute('x', String(e.x));
  box.setAttribute('y', String(e.y));
  box.setAttribute('width', String(Math.max(40, e.width || 40)));
  box.setAttribute('height', String(Math.max(24, e.height || 24)));
  box.setAttribute('fill', 'none');
  box.setAttribute('stroke', 'currentColor');
  box.setAttribute('stroke-dasharray', '4 4');
  box.setAttribute('opacity', '0.5');
  const t = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  t.setAttribute('x', String(e.x + 6));
  t.setAttribute('y', String(e.y + 16));
  t.setAttribute('font-size', '12');
  t.setAttribute('fill', 'currentColor');
  t.setAttribute('opacity', '0.6');
  t.textContent = what;
  g.append(box, t);
  return g;
}
