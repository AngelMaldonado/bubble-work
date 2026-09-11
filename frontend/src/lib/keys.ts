/**
 * Quién es dueño del teclado ahora mismo.
 *
 * Un atajo global tiene que saber cuándo callarse: dentro de un campo, de un
 * menú, de un diálogo o del editor, las teclas ya significan algo y robárselas
 * es cómo un atajo se convierte en una sorpresa.
 *
 * Vive aquí y no dentro de un componente porque lo preguntan dos —el omnibar y
 * el cambiador de workspace— y dos copias de esta lista se separan: una aprende
 * a respetar el editor y la otra no.
 */
export function isEditable(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el.isContentEditable) return true;
  return (
    ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName) ||
    el.closest('[role="menu"]') !== null ||
    el.closest('[role="dialog"]') !== null ||
    el.closest('.cm-editor') !== null
  );
}
