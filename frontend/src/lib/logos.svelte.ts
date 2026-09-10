// Las marcas, para cuando lo que se documenta es de alguien.
//
// El índice —slug, título y color de marca— se carga una vez y entero, porque
// se busca sobre él; el dibujo de cada logo se pide sólo cuando hay que
// dibujarlo. Al revés (todos los dibujos por adelantado) serían cinco megas
// para enseñar tres baldosas.
export type Brand = { slug: string; title: string; hex: string };

class Logos {
  all = $state<Brand[]>([]);
  private asked = false;

  /** Idempotente: la galería y el selector la llaman por su cuenta, y dos
   *  peticiones del mismo índice son una de más. */
  async load() {
    if (this.asked) return;
    this.asked = true;
    try {
      this.all = await fetch('/logos/index.json').then((r) => r.json());
    } catch {
      this.all = [];
    }
  }

  /** ¿Este nombre es una marca que conocemos?
   *
   *  Comparación laxa —sin espacios, sin puntos, sin mayúsculas— porque nadie
   *  escribe «DigitalOcean» igual dos veces, y el proveedor se teclea a mano.
   *  Exacta y no por contenido: «Amazon Web Services» debe encontrar a AWS, pero
   *  «mi vps» no debería encontrar «VPS Server Co» y pintar un logo ajeno. */
  find(name: string): Brand | null {
    const key = flat(name);
    if (!key) return null;
    return this.all.find((b) => flat(b.title) === key || flat(b.slug) === key) ?? null;
  }
}

const flat = (s: string) => s.toLowerCase().replace(/[^a-z0-9]/g, '');

export const logos = new Logos();
