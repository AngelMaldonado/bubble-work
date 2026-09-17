/**
 * Abrir el modal de anotación sobre una imagen pegada, y esperar su respuesta.
 *
 * Montado aparte, sobre `body`, y no dentro de cada editor: pegar una imagen
 * ocurre en el documento de un thread, en el brief de una burbuja y en el
 * cuerpo de una nota, y el modal tiene que ser el mismo en los tres. Es el
 * mismo argumento que `attach.ts` hace para «qué es una imagen pegada».
 */
import { mount, unmount } from 'svelte';
import AnnotateImage from '../components/AnnotateImage.svelte';

/** La imagen a pegar —la original o la anotada—, o nada si se canceló. */
export function annotate(file: File): Promise<File | null> {
  return new Promise((resolve) => {
    const app = mount(AnnotateImage, {
      target: document.body,
      props: {
        file,
        ondone: (out: File | null) => {
          resolve(out);
          // Después de que el diálogo se cierre: desmontarlo dentro de su
          // propio manejador deja a Zag cerrando algo que ya no existe.
          setTimeout(() => unmount(app), 0);
        },
      },
    });
  });
}
