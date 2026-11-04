// Pasar de leer a escribir —y de vuelta— sin perder el sitio.
//
// El servidor marca cada bloque de primer nivel del renderizado con
// `data-line`: la línea del documento donde empieza. Con eso la traducción es
// exacta en los bordes de los bloques, y dentro de un bloque se reparte en
// proporción a su alto. Sin marcas (un documento que no las trae) no hay
// traducción: quien llama se queda con lo que tenía.

export type Block = { line: number; top: number; height: number };

/** Los bloques marcados, con su posición vertical relativa al mismo origen
 *  que `viewTop` (la pantalla). */
export function blocksOf(root: HTMLElement): Block[] {
  const out: Block[] = [];
  for (const el of root.querySelectorAll<HTMLElement>('[data-line]')) {
    const line = Number(el.dataset.line);
    if (!Number.isFinite(line)) continue;
    const r = el.getBoundingClientRect();
    out.push({ line, top: r.top, height: r.height });
  }
  return out;
}

/** Qué línea del documento está en `viewTop`. Puede ser fraccionaria: a media
 *  altura de un párrafo de cuatro líneas que empieza en la 10, es la 12. */
export function lineAt(blocks: Block[], viewTop: number): number | null {
  if (!blocks.length) return null;
  for (let i = 0; i < blocks.length; i++) {
    const b = blocks[i];
    if (b.top + b.height <= viewTop) continue;
    if (b.top >= viewTop) return b.line;
    const next = blocks[i + 1]?.line ?? b.line + 1;
    return b.line + ((viewTop - b.top) / Math.max(1, b.height)) * (next - b.line);
  }
  return blocks[blocks.length - 1].line;
}

/** Dónde (en el mismo origen que los bloques) empieza la línea `line`. */
export function topOf(blocks: Block[], line: number): number | null {
  if (!blocks.length) return null;
  let i = 0;
  while (i + 1 < blocks.length && blocks[i + 1].line <= line) i++;
  const b = blocks[i];
  if (line <= b.line) return b.top;
  const next = blocks[i + 1];
  const span = (next?.line ?? b.line + 1) - b.line;
  const into = Math.min(1, (line - b.line) / Math.max(1, span));
  return b.top + into * b.height;
}
