// Los íconos de la aplicación: renders 3D generados con Codex en el estilo del
// cerebro de los artefactos — barro en reposo, color al pasar por encima.
// Viven en `public/i3d/<nombre>-clay.png` y `-color.png`.
//
// Un emoji en una cadena era cómo se dibujaba todo, y muchas de esas cadenas
// son DATOS (la banda de una burbuja, la cara de un fijado del sidebar). Por eso
// `Icon` acepta un emoji o un nombre, y este mapa es la única traducción: un
// dato que dice 🔥 se dibuja con el ícono `fire` sin que nadie cambie el modelo.
//
// Lo que no está aquí no es un olvido: los glifos tipográficos (← ⌘ ✕ ↵ …) son
// texto de interfaz, y un emoji dentro de un `title`, un `<option>` o una
// etiqueta de mermaid sólo puede ser texto.
export const ICONS = [
  'fire', 'dormant', 'tombstone', 'trophy', 'thread', 'bubble', 'compass', 'search',
  'calendar', 'check', 'people', 'trash', 'book', 'board', 'inbox', 'chat', 'pin',
  'eyes', 'pencil', 'cabinet', 'sun', 'moon', 'theme', 'robot', 'gear', 'door',
  'paperclip', 'map', 'tools', 'document', 'key', 'warning', 'question', 'arrow',
  'target', 'numbers', 'image', 'link', 'brain', 'prompt',
] as const;
export type IconName = (typeof ICONS)[number];

const byEmoji: Record<string, IconName> = {
  '🔥': 'fire', '😴': 'dormant', '🪦': 'tombstone', '🏆': 'trophy', '🧵': 'thread',
  '🫧': 'bubble', '🧭': 'compass', '🔍': 'search', '🗓': 'calendar', '📅': 'calendar',
  '✅': 'check', '👥': 'people', '🗑': 'trash', '📖': 'book', '🗂': 'board',
  '📥': 'inbox', '💬': 'chat', '📌': 'pin', '👀': 'eyes', '✏': 'pencil', '🗄': 'cabinet',
  '☀': 'sun', '🌙': 'moon', '🌗': 'theme', '🤖': 'robot', '⚙': 'gear', '🚪': 'door',
  '📎': 'paperclip', '🗺': 'map', '🛠': 'tools', '📄': 'document', '🔑': 'key',
  '⚠': 'warning', '❓': 'question', '➡': 'arrow', '🎯': 'target', '🔢': 'numbers',
  '🖼': 'image', '🔗': 'link', '🧠': 'brain', '📜': 'prompt',
};

/** El ícono de un emoji o de un nombre; null si no hay ninguno (se dibuja el texto). */
export function iconFor(of: string): IconName | null {
  if ((ICONS as readonly string[]).includes(of)) return of as IconName;
  return byEmoji[of.replace(/\uFE0F/g, '')] ?? null;
}

/** clay: barro (reposo) · color: el degradado de la marca (hover) · tint: los
 *  colores naturales del objeto — fuego naranja, trofeo dorado — para lo que
 *  se muestra siempre encendido (menú, bandas, botones flotantes). */
export type IconVariant = 'clay' | 'color' | 'tint';
export const iconSrc = (n: IconName, v: IconVariant) =>
  n === 'brain' ? `/brain/${v === 'clay' ? 'clay' : 'color'}.png` : `/i3d/${n}-${v}.png`;
