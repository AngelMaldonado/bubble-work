import { api } from './api';

/** Browser-local reading position. The root is the actual reader scroller,
 * not the article (thread/wiki/Prompts reuse their scroll container). */
function parent(node: HTMLElement): HTMLElement {
  for (let at = node.parentElement; at; at = at.parentElement) {
    if (/auto|scroll|overlay/.test(getComputedStyle(at).overflowY)) return at;
  }
  return (document.scrollingElement ?? document.documentElement) as HTMLElement;
}
type Owner = {
  key: string;
  refs: number;
  add: (node: HTMLElement) => void;
  remove: (node: HTMLElement) => void;
  dispose: () => void;
};
const owners = new WeakMap<HTMLElement, Owner>();

export function rememberMarkdownScroll(node: HTMLElement, identity: string): () => void {
  const element = parent(node);
  const scroller = element === document.scrollingElement ? 'document-scroll' : (element.id || element.classList.item(0) || element.tagName.toLowerCase());
  const key = `bubble.md.scroll.${api.me?.id ?? 'local'}.${identity}.${scroller}`;
  const prior = owners.get(element);
  const release = (record: Owner) => () => {
    record.remove(node);
    if (--record.refs > 0) return;
    record.dispose();
    if (owners.get(element) === record) owners.delete(element);
  };
  if (prior?.key === key) { prior.refs++; prior.add(node); return release(prior); }
  prior?.dispose();

  let saved = 0;
  try { const value = Number(localStorage.getItem(key)); if (Number.isFinite(value) && value >= 0) saved = value; } catch { /* browser policy */ }
  let restoring = !location.hash;
  let last = restoring ? saved : element.scrollTop;
  let closed = false;
  const nodes = new Map<HTMLElement, number>();
  const connected = () => [...nodes.keys()].some((n) => n.isConnected);
  const persist = () => { try { localStorage.setItem(key, String(last)); } catch { /* quota/privacy mode */ } };
  const save = () => {
    if (closed || restoring || !connected()) return;
    last = element.scrollTop;
    persist();
  };
  const restore = () => {
    if (closed || !restoring || !connected()) return;
    element.scrollTop = Math.min(saved, Math.max(0, element.scrollHeight - element.clientHeight));
  };
  const observer = new ResizeObserver(restore);
  const interact = () => {
    if (!restoring) return;
    restoring = false;
    observer.disconnect();
    save();
  };
  // Viewport scroll events go to window/document, not the HTML element.
  const target: EventTarget = element === document.scrollingElement ? window : element;
  target.addEventListener('scroll', save, { passive: true });
  for (const event of ['wheel', 'pointerdown', 'touchstart', 'keydown']) element.addEventListener(event, interact, { passive: true });
  window.addEventListener('pagehide', save);

  const record: Owner = {
    key, refs: 1,
    add(n) { nodes.set(n, (nodes.get(n) ?? 0) + 1); if (restoring) observer.observe(n); },
    remove(n) { const count = nodes.get(n) ?? 0; if (count > 1) nodes.set(n, count - 1); else { nodes.delete(n); observer.unobserve(n); } },
    dispose() {
      if (closed) return;
      // The DOM may already contain the next document, or be clamped by its
      // shorter height. Keep the last observed position, never that teardown 0.
      persist();
      closed = true;
      observer.disconnect();
      target.removeEventListener('scroll', save);
      for (const event of ['wheel', 'pointerdown', 'touchstart', 'keydown']) element.removeEventListener(event, interact);
      window.removeEventListener('pagehide', save);
    },
  };
  record.add(node);
  owners.set(element, record);
  observer.observe(element);
  requestAnimationFrame(restore);
  return release(record);
}
