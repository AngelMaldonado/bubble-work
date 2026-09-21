/**
 * Pegar o soltar un archivo dentro de un editor de markdown.
 *
 * Una sola vez, para todos los editores: el del documento de un thread, el del
 * brief de una burbuja y el del cuerpo de una nota del inbox. Tres copias de
 * «qué es una imagen pegada» se separan, y la que se queda atrás es la que un
 * día sube un PDF o escribe la referencia en el sitio equivocado.
 *
 * Lo que NO decide es dónde se guarda: eso depende de quién es dueño de lo que
 * se está escribiendo. Un thread y una burbuja tienen workspace, así que la
 * imagen va a su `assets/`, y se cita por la ruta corta. Una nota no tiene
 * workspace —no tenerlo es lo que la hace una nota— así que se guarda en el
 * propio registro, y se cita por su URL. Quien llama da `upload`; esto sólo
 * pone la referencia donde está el cursor.
 */
import type { MarkdownEditor } from './editor';

/** La imagen que viene en un pegado, o nada.
 *
 *  Tres formas en que un portapapeles trae una imagen, y hay que mirar las tres:
 *
 *  - como ARCHIVO (`files`): una captura de pantalla del sistema;
 *  - como ÍTEM de tipo archivo (`items`): algunos navegadores la ponen ahí y no
 *    en `files`;
 *  - como TEXTO, una URL `data:image/…;base64,…`: lo que copian varias
 *    herramientas de capturas y el «copiar imagen» de algunos navegadores.
 *
 *  La tercera es la que rompía: no llegaba como archivo, así que nadie la
 *  interceptaba, y el editor pegaba el base64 tal cual — cientos de miles de
 *  caracteres dentro del texto, que el servidor rechazaba por largo. Una imagen
 *  nunca debe acabar escrita DENTRO del markdown: se sube y se cita. */
export function pastedImage(e: ClipboardEvent): File | null {
  const data = e.clipboardData;
  if (!data) return null;

  const file = [...(data.files ?? [])].find((f) => f.type.startsWith('image/'));
  if (file) return file;

  for (const item of [...(data.items ?? [])]) {
    if (item.kind === 'file' && item.type.startsWith('image/')) {
      const f = item.getAsFile();
      if (f) return f;
    }
  }

  const text = data.getData('text/plain') || '';
  const html = data.getData('text/html') || '';
  const url =
    text.trim().match(/^data:image\/[a-z0-9.+-]+;base64,[a-z0-9+/=\s]+$/i)?.[0] ??
    html.match(/src=["'](data:image\/[a-z0-9.+-]+;base64,[a-z0-9+/=\s]+)["']/i)?.[1];
  return url ? fromDataUrl(url) : null;
}

/** Una URL `data:` convertida en un archivo de verdad, para subirlo como
 *  cualquier otro. */
function fromDataUrl(url: string): File | null {
  const m = url.match(/^data:(image\/[a-z0-9.+-]+);base64,(.*)$/is);
  if (!m) return null;
  try {
    const bin = atob(m[2].replace(/\s+/g, ''));
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    const ext = m[1].split('/')[1].replace('jpeg', 'jpg').replace(/\+.*/, '');
    return new File([bytes], `imagen.${ext}`, { type: m[1] });
  } catch {
    return null;
  }
}

/** El archivo que se soltó encima, o nada. */
export function droppedFile(e: DragEvent): File | null {
  return [...(e.dataTransfer?.files ?? [])][0] ?? null;
}

/** Sube y escribe la referencia donde está el cursor. Devuelve el markdown
 *  resultante, o nada si no se escribió.
 *
 *  Una imagen se EMBEBE —`![nombre](ruta)`, se ve dentro del documento— y un
 *  documento adjunto se ENLAZA —`[informe.pdf](ruta)`, se abre—. Quien lo dice
 *  es el servidor, en `image`: la lista de qué extensión es una imagen vive en
 *  `internal/tree/layout.go` y una copia aquí sería una segunda lista que
 *  mantener.
 *
 *  El nombre completo, con extensión, para un adjunto: `informe` no dice que
 *  sea un PDF, y es lo único que va a leer quien pase por ahí. */
export async function insertAttachment(
  editor: MarkdownEditor | null,
  file: File,
  upload: (file: File) => Promise<{ path: string; image?: boolean } | null | void>,
): Promise<string | null> {
  const out = await upload(file);
  const path = out?.path;
  if (!path || !editor) return null;
  const image = out?.image ?? file.type.startsWith('image/');
  const label = image ? file.name.replace(/\.[^.]+$/, '') || 'imagen' : file.name || 'adjunto';
  const at = editor.cursor();
  const text = image ? `![${label}](${path})` : `[${label}](${path})`;
  editor.replace(at, at, text, text.length);
  return editor.value();
}
