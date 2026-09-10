// La pieza que representa a una cosa del inventario.
//
// Un solo juego: los renders de 3dicons, en sus dos temas. Hubo un segundo —SVG
// dibujados a mano para servidor, nube, base de datos, dominio y red, que ese
// set no cubre— y se quitó: dos familias visuales en la misma galería se ven
// como dos aplicaciones pegadas, y tener el concepto exacto no compensa que la
// cuadrícula se rompa a la mitad. Donde falta el objeto literal, gana el que
// alguien reconoce: una computadora por un VPS, un candado por una credencial.
//
// Se guarda el NOMBRE de la pieza, nunca la imagen: la imagen ya está en el
// disco de todos, y guardarla por fila sería una copia por grupo de algo que no
// cambia.

/** Cuando nadie eligió nada: una caja. Que exista un genérico es lo que evita
 *  que la galería se vea rota el día que alguien inventa un grupo llamado
 *  «otros». */
const ANY = 'cube';

/** Qué se dibuja en una baldosa.
 *
 *  Dos clases, y no una con excepciones: un RENDER es una imagen que ya trae su
 *  volumen y su color, y una MARCA es una silueta monocroma a la que la pantalla
 *  le pone las dos cosas. Mezclarlas en un solo tipo obligaba a la baldosa a
 *  adivinar cuál tenía delante mirando la ruta. */
export type Piece =
  | { kind: 'render'; still: string; lit: string }
  | { kind: 'brand'; mask: string; hex: string };

/** `si:cloudflare` es una marca; cualquier otra cosa, una pieza de 3dicons. El
 *  prefijo va en el mismo campo a propósito: son la misma decisión —qué
 *  representa esto— y dos columnas para una decisión son dos sitios donde
 *  quedarse a medias. */
export function piece(art: string, hex = ''): Piece {
  if (art.startsWith('si:')) {
    const slug = art.slice(3);
    return { kind: 'brand', mask: `/logos/${slug}.svg`, hex: hex || '888888' };
  }
  return { kind: 'render', still: still(art), lit: lit(art) };
}

/** Reposo: el volumen sin color. Cuarenta piezas pintadas compiten entre ellas
 *  y no se lee ninguna; en barro, lo que se lee es la forma y el nombre. */
export const still = (art: string) => `/3dicons/clay/${art || ANY}.png`;

/** Y al pasar por encima, la misma pieza pintada — sobre la que estás mirando y
 *  sólo esa. */
export const lit = (art: string) => `/3dicons/color/${art || ANY}.png`;

/** Cuando nadie eligió: la palabra decide. Un grupo llamado «Dominios» no
 *  debería tener que elegir además un icono, y el orden de esta lista es parte
 *  de la respuesta — «servidores de correo» gana la primera que coincida. */
const GUESS: [RegExp, string][] = [
  [/dominio|dns|domain|url/i, 'link'],
  [/vps|servidor|server|host|maquina|máquina|computad|equipo|laptop/i, 'computer'],
  [/nube|cloud|aws|azure|gcp|sphere/i, 'sphere'],
  [/base de datos|database|postgres|mysql|sqlite|almacen|storage|bucket|s3|disco/i, 'locker'],
  [/red|network|cdn|proxy|balance|wifi|internet/i, 'wifi'],
  [/licencia|licen|clave|llave|token|credencial|suscrip/i, 'key'],
  [/correo|mail|smtp|buzon|buzón/i, 'mail'],
  [/certificad|ssl|tls|seguridad|firewall/i, 'sheild'],
  [/respaldo|backup|copia|archivo|carpeta/i, 'folder'],
  [/servicio|saas|api|integrac|herramienta/i, 'setting'],
  [/pago|cobro|tarjeta|factura|costo/i, 'card'],
  [/monitor|metric|grafica|gráfica|reporte/i, 'chart'],
  [/tiempo|renovac|vence|calendario/i, 'calender'],
];

export const guess = (text: string) =>
  (GUESS.find(([re]) => re.test(text)) ?? [null, ANY])[1] as string;
