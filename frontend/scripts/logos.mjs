// Los logos de marca, escritos a disco desde `simple-icons`.
//
// Se GENERAN y no se versionan, que es la diferencia honesta con los renders de
// 3dicons: aquellos se bajaron una vez de un CDN ajeno y viven aquí porque ese
// CDN puede mudarse; estos salen de una dependencia fijada en package.json, y
// tres mil quinientos archivos en el repositorio ensucian cada diff a cambio de
// nada que no se pueda reproducir con una línea.
//
// Un archivo por marca, monocromo. El color lo pone la pantalla: la baldosa
// dibuja el SVG como MÁSCARA y le pinta el fondo — gris en reposo, el color de
// la marca al pasar por encima. Con una imagen normal harían falta dos archivos
// por logo para el mismo efecto.
//
// La licencia de los SVG es CC0; la MARCA sigue siendo de su dueño. Mostrar el
// logo de Cloudflare para decir «esto está en Cloudflare» es uso nominativo, que
// es justo para lo que sirve.
import { mkdirSync, writeFileSync, rmSync } from 'node:fs';
import * as si from 'simple-icons';

const OUT = new URL('../public/logos/', import.meta.url);
rmSync(OUT, { recursive: true, force: true });
mkdirSync(OUT, { recursive: true });

const index = [];
for (const key of Object.keys(si)) {
  const icon = si[key];
  if (!icon?.slug || !icon?.path) continue;
  writeFileSync(
    new URL(`${icon.slug}.svg`, OUT),
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="${icon.path}"/></svg>`,
  );
  index.push({ slug: icon.slug, title: icon.title, hex: icon.hex });
}
// El índice se carga entero para poder buscar; por eso lleva sólo lo que la
// búsqueda necesita, y el dibujo se pide por separado cuando hay que dibujarlo.
writeFileSync(new URL('index.json', OUT), JSON.stringify(index));
console.log(`${index.length} logos en public/logos`);
