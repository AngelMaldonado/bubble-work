/** Match a row's inner height, including when fonts or wrapping change. */
export function rowSquare(button: HTMLElement, selector: string): () => void {
  const row = button.parentElement?.closest<HTMLElement>(selector);
  if (!row) return () => {};
  const sync = () => {
    const css = getComputedStyle(row);
    const size = row.getBoundingClientRect().height - parseFloat(css.paddingTop) - parseFloat(css.paddingBottom)
      - parseFloat(css.borderTopWidth) - parseFloat(css.borderBottomWidth);
    if (size > 0) { button.style.width = `${size}px`; button.style.height = `${size}px`; }
  };
  const observer = new ResizeObserver(sync);
  observer.observe(row);
  sync();
  return () => observer.disconnect();
}
