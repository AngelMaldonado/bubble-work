// The edge fade for a scrolling box.
//
// It fades only the edge that still has content behind it. Fading both edges
// permanently would dim the first and last row for no reason, which reads as a
// rendering fault rather than as "there is more".
//
// A mask, not a blurred overlay: the content fades to nothing against whatever
// is behind the box, so it stays right in both themes and over the page's
// gradient — an overlay would need to know the colour underneath it.

/** How many pixels of fade at full strength. */
const SIZE = 28;

export function edgeFade(size = SIZE) {
  let top = $state(0);
  let end = $state(0);

  return {
    /** The mask, as an inline style. */
    get style() {
      const t = (top * size).toFixed(1);
      const e = (end * size).toFixed(1);
      return (
        `mask-image: linear-gradient(to bottom, transparent 0, #000 ${t}px,` +
        ` #000 calc(100% - ${e}px), transparent 100%)`
      );
    },

    /** Attach to the scrolling element: `{@attach fade.attach}`. */
    attach(el: HTMLElement) {
      const sync = () => {
        const max = el.scrollHeight - el.clientHeight;
        // Interpolated over the first `size` pixels so the edge fades in rather
        // than switching on.
        top = Math.min(1, el.scrollTop / size);
        end = Math.min(1, Math.max(0, max - el.scrollTop) / size);
      };
      sync();
      el.addEventListener('scroll', sync, { passive: true });
      // The box also changes length when its contents do, and a scroll listener
      // never hears about that.
      const ro = new ResizeObserver(sync);
      ro.observe(el);
      for (const child of el.children) ro.observe(child);
      return () => {
        el.removeEventListener('scroll', sync);
        ro.disconnect();
      };
    },
  };
}
