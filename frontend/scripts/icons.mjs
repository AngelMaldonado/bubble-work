// Los iconos PNG del manifiesto, generados del mismo dibujo que el favicon.
//
// Por qué generarlos y no dibujarlos: el favicon es un SVG con dos gradientes
// radiales, y un PNG hecho a mano al lado se separa del SVG en cuanto alguien
// toque uno de los dos. Esto calcula los mismos gradientes pixel a pixel, así
// que el icono instalado y el de la pestaña son literalmente el mismo dibujo.
//
// Por qué PNG y no el SVG en el manifiesto: Chrome acepta un SVG, pero iOS no —
// `apple-touch-icon` tiene que ser un bitmap, y sin él el icono de la
// aplicación instalada en un iPhone es una captura de pantalla.
//
// Sin dependencias: `zlib` viene con node y un PNG sin filtros es una cabecera,
// los datos desinflados y un CRC. Se ejecuta a mano cuando el dibujo cambie
// (`node scripts/icons.mjs`), no en cada build: son cuatro archivos que no
// cambian nunca.
import { deflateSync } from 'node:zlib';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const OUT = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'icons');

/** Los mismos paradores que `favicon.svg`, en el mismo orden. */
const BODY = [
  [0.0, [0xff, 0x8a, 0x5c]],
  [0.55, [0xf2, 0x55, 0x1f]],
  [1.0, [0xc9, 0x3c, 0x12]],
];

/** Interpola un gradiente de paradores en `t` ∈ [0,1]. */
function ramp(stops, t) {
  const x = Math.min(1, Math.max(0, t));
  for (let i = 1; i < stops.length; i++) {
    const [p0, c0] = stops[i - 1];
    const [p1, c1] = stops[i];
    if (x <= p1) {
      const k = p1 === p0 ? 0 : (x - p0) / (p1 - p0);
      return c0.map((v, j) => v + (c1[j] - v) * k);
    }
  }
  return stops.at(-1)[1];
}

/** Un canal sobre otro, con alfa. */
const over = (dst, src, a) => dst.map((v, i) => src[i] * a + v * (1 - a));

/**
 * El dibujo, en `size` píxeles.
 *
 * `pad` deja aire alrededor para un icono «maskable»: Android recorta el icono
 * con la forma que quiera el lanzador, y una esfera que toca el borde sale
 * mordida. `bg` pinta el fondo en vez de dejarlo transparente, que es lo que
 * ese mismo recorte necesita.
 */
function draw(size, { pad = 0, bg = null } = {}) {
  const px = Buffer.alloc(size * size * 4);
  // El SVG dibuja en un lienzo de 64 con la esfera de radio 28 centrada.
  const unit = size * (1 - pad * 2);
  const off = size * pad;
  const cx = off + unit * 0.5;
  const cy = off + unit * 0.5;
  const r = (unit * 28) / 64;
  // Los centros y radios de los dos gradientes, en fracción de la CAJA del
  // círculo, que es contra lo que los resuelve un `radialGradient` de SVG.
  const box = r * 2;
  const g = [
    { cx: cx - r + box * 0.38, cy: cy - r + box * 0.34, r: box * 0.72 },
    { cx: cx - r + box * 0.34, cy: cy - r + box * 0.3, r: box * 0.26 },
  ];

  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const i = (y * size + x) * 4;
      let rgb = bg ? [...bg] : [0, 0, 0];
      let a = bg ? 1 : 0;

      // Antialias por distancia al borde: un círculo de píxeles duros se ve
      // dentado justo en el tamaño en que más se mira, que es el pequeño.
      const d = Math.hypot(x + 0.5 - cx, y + 0.5 - cy);
      const cover = Math.min(1, Math.max(0, r + 0.5 - d));
      if (cover > 0) {
        const body = ramp(BODY, Math.hypot(x + 0.5 - g[0].cx, y + 0.5 - g[0].cy) / g[0].r);
        const sheen = Math.min(
          1,
          Math.max(0, 1 - Math.hypot(x + 0.5 - g[1].cx, y + 0.5 - g[1].cy) / g[1].r),
        );
        const lit = over(body, [255, 255, 255], sheen * 0.85);
        rgb = a ? over(rgb, lit, cover) : lit;
        a = a + (1 - a) * cover;
      }
      px[i] = Math.round(rgb[0]);
      px[i + 1] = Math.round(rgb[1]);
      px[i + 2] = Math.round(rgb[2]);
      px[i + 3] = Math.round(a * 255);
    }
  }
  return px;
}

function crc32(buf) {
  let c = ~0;
  for (const b of buf) {
    c ^= b;
    for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1));
  }
  return ~c >>> 0;
}

function chunk(type, data) {
  const head = Buffer.alloc(8);
  head.writeUInt32BE(data.length, 0);
  head.write(type, 4, 'ascii');
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(Buffer.concat([head.subarray(4), data])), 0);
  return Buffer.concat([head, data, crc]);
}

function png(size, px) {
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(size, 0);
  ihdr.writeUInt32BE(size, 4);
  ihdr[8] = 8; // 8 bits por canal
  ihdr[9] = 6; // RGBA
  // Cada fila lleva delante su byte de filtro, y el nuestro es 0: sin filtro.
  const raw = Buffer.alloc(size * (size * 4 + 1));
  for (let y = 0; y < size; y++) {
    raw[y * (size * 4 + 1)] = 0;
    px.copy(raw, y * (size * 4 + 1) + 1, y * size * 4, (y + 1) * size * 4);
  }
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(raw, { level: 9 })),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}

mkdirSync(OUT, { recursive: true });
const made = [
  ['icon-192.png', 192, {}],
  ['icon-512.png', 512, {}],
  // `maskable` lleva aire y fondo: el lanzador recorta con la forma que quiera.
  ['icon-maskable-512.png', 512, { pad: 0.12, bg: [0xff, 0xff, 0xff] }],
  // iOS no acepta SVG ni transparencia con gracia: fondo sólido y sin aire.
  ['apple-touch-icon.png', 180, { bg: [0xff, 0xff, 0xff] }],
];
for (const [name, size, opts] of made) {
  writeFileSync(join(OUT, name), png(size, draw(size, opts)));
  console.log(`${name}  ${size}x${size}`);
}
