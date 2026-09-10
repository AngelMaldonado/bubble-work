<script lang="ts">
  // El inventario: dónde vive lo que hace funcionar todo esto.
  //
  // VPS, dominios, servicios contratados, licencias. No es trabajo —documentar
  // un servidor no es evidencia de que la realidad cambió, y esto no calienta
  // nada— pero es lo primero que alguien busca a las tres de la mañana, y hasta
  // hoy vivía en la cabeza de una persona o en un chat que nadie encuentra.
  //
  // Dos niveles y un detalle: la galería de grupos, sus cosas dentro, y el modal
  // con lo que hay que saber de una. Dos niveles porque «¿dónde está el DNS?» y
  // «¿cuándo se renueva ESTE dominio?» son dos preguntas distintas, y una lista
  // plana de cuarenta filas no contesta bien ninguna.
  import { DatePicker, Dialog, Portal, parseDate } from '@skeletonlabs/skeleton-svelte';
  import { api, type InvGroup, type InvItem } from '../lib/api';
  import { guess, lit } from '../lib/art';
  import { logos } from '../lib/logos.svelte';
  import InvTile from './InvTile.svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
  import { ago } from '../lib/when';
  import Prose from './Prose.svelte';
  import ThemeToggle from './ThemeToggle.svelte';

  let {
    onback,
    onsearch,
    canWrite = false,
  }: {
    onback?: () => void;
    onsearch?: () => void;
    /** el lead global escribe; los demás leen — y sólo si les fue asignado */
    canWrite?: boolean;
  } = $props();

  let groups = $state<InvGroup[]>([]);
  let items = $state<InvItem[]>([]);
  let inGroup = $state<InvGroup | null>(null);
  let open = $state<InvItem | null>(null);
  let html = $state('');
  let error = $state('');

  // El alta de un grupo, con su pieza. Vive en la pantalla y no en el panel de
  // PocketBase porque elegir el dibujo ES parte de crear la categoría: se ve
  // cómo va a quedar la galería mientras se decide.
  let making = $state(false);
  /** el grupo que se está editando; `null` mientras se crea uno nuevo */
  let editingGroup = $state<InvGroup | null>(null);
  let fresh = $state({ name: '', note: '', art: '' });
  let doom = $state<Doom>(null);

  // El alta y la edición de una COSA. Un solo formulario para las dos: son los
  // mismos campos, y dos formularios serían dos sitios donde olvidar uno.
  let thing = $state<Partial<InvItem> | null>(null);
  let pickingFor = $state<'group' | 'item'>('group');
  let hunting = $state('');
  let catalog = $state<{ slug: string; title: string }[]>([]);

  async function loadCatalog() {
    if (catalog.length) return;
    try {
      catalog = await fetch('/3dicons/index.json').then((r) => r.json());
    } catch {
      catalog = [];
    }
  }

  const q = $derived(hunting.trim().toLowerCase());
  const shortlist = $derived(catalog.filter((c) => c.title.toLowerCase().includes(q)).slice(0, 60));

  /** Las que de verdad aparecen en un inventario de infraestructura. Tres mil
   *  quinientas marcas sin filtrar no son un selector, son un directorio — así
   *  que sin búsqueda se ven éstas, y buscando se ve todo lo demás. */
  const USUAL = [
    'cloudflare', 'hetzner', 'digitalocean', 'amazonwebservices', 'googlecloud',
    'microsoftazure', 'vercel', 'netlify', 'railway', 'render', 'fly', 'linode',
    'ovh', 'namecheap', 'godaddy', 'porkbun', 'github', 'gitlab', 'docker',
    'postgresql', 'mysql', 'sqlite', 'redis', 'mongodb', 'supabase', 'firebase',
    'stripe', 'sentry', 'grafana', 'cloudinary', 'letsencrypt', 'nginx',
    'ubuntu', 'debian', 'proxmox', 'tailscale', 'wireguard', 'zoho', 'protonmail',
  ];

  const brands = $derived(
    q.length < 2
      ? USUAL.map((slug) => logos.all.find((b) => b.slug === slug)).filter(Boolean).slice(0, 40)
      : logos.all.filter((b) => b.title.toLowerCase().includes(q)).slice(0, 40),
  ) as { slug: string; title: string; hex: string }[];

  async function saveGroup() {
    const name = fresh.name.trim();
    if (!name) return;
    // Sin elección explícita, la palabra decide: crear «Dominios» no debería
    // obligar a elegir además un icono.
    const fields = { name, note: fresh.note.trim(), art: fresh.art || guess(name) };
    try {
      if (editingGroup) await api.update('inventory_groups', editingGroup.id, fields);
      else await api.createInvGroup(fields);
      making = false;
      editingGroup = null;
      fresh = { name: '', note: '', art: '' };
      await loadGroups();
      if (inGroup) {
        inGroup = groups.find((g) => g.id === inGroup?.id) ?? null;
      }
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function groupAction(what: string, g: InvGroup) {
    if (what === 'open') return enter(g);
    if (what === 'add') {
      inGroup = g;
      api.invItems(g.id).then((rows) => (items = rows));
      thing = { group: g.id };
      loadCatalog();
      return;
    }
    if (what === 'edit') {
      editingGroup = g;
      fresh = { name: g.name, note: g.note ?? '', art: g.art ?? '' };
      pickingFor = 'group';
      making = true;
      loadCatalog();
      return;
    }
    if (what === 'drop') {
      doom = {
        title: `¿Borrar el grupo «${g.name}»?`,
        body: `Se lleva las ${g.items} cosas que tiene dentro. Lo que documentaban —dónde vivía cada cosa, cuándo se renovaba— no está en ningún otro lado.`,
        go: async () => {
          try {
            await api.remove('inventory_groups', g.id);
            if (inGroup?.id === g.id) {
              inGroup = null;
              items = [];
            }
            await loadGroups();
          } catch (e) {
            error = (e as Error).message;
          }
        },
      };
    }
  }

  function itemAction(what: string, i: InvItem) {
    if (what === 'open') return show(i);
    if (what === 'panel' && i.url) return void window.open(i.url, '_blank', 'noreferrer');
    if (what === 'vault' && i.vault) return void window.open(i.vault, '_blank', 'noreferrer');
    if (what === 'edit') {
      thing = { ...i };
      pickingFor = 'item';
      loadCatalog();
      return;
    }
    if (what === 'drop') {
      doom = {
        title: `¿Borrar «${i.name}»?`,
        body: 'Sale del inventario. Si la cosa sigue existiendo y pagándose, borrarla de aquí sólo hace que nadie se acuerde de ella.',
        go: async () => {
          try {
            await api.remove('inventory_items', i.id);
            if (inGroup) items = await api.invItems(inGroup.id);
            await loadGroups();
          } catch (e) {
            error = (e as Error).message;
          }
        },
      };
    }
  }

  /** El día elegido como `2026-09-15`.
   *
   *  NO `valueAsString`: Zag lo formatea para el LOCALE, así que con es-MX
   *  entrega «15/09/2026» y el servidor lo rechaza en silencio — es el mismo
   *  error que costó una tarde en el planeador, y por eso aquí se arma a mano
   *  desde los tres números, que son la misma fecha en cualquier idioma. */
  function isoDay(d?: { year: number; month: number; day: number }) {
    if (!d) return '';
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.year}-${pad(d.month)}-${pad(d.day)}`;
  }

  const renewValue = $derived.by(() => {
    const day = (thing?.renews_at ?? '').slice(0, 10);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) return [];
    try {
      return [parseDate(day)];
    } catch {
      return [];
    }
  });

  /** Guardar una cosa: la misma función para la que nace y la que cambia. */
  async function saveThing() {
    const t = thing;
    if (!t?.name?.trim()) return;
    const fields: Record<string, unknown> = {
      group: t.group,
      name: t.name.trim(),
      provider: t.provider ?? '',
      url: t.url ?? '',
      vault: t.vault ?? '',
      cost: t.cost ?? '',
      notes: t.notes ?? '',
      art: t.art ?? '',
    };
    // Una fecha o el vacío que la borra; nunca a medias.
    const day = (t.renews_at ?? '').slice(0, 10);
    fields.renews_at = /^\d{4}-\d{2}-\d{2}$/.test(day) ? `${day} 00:00:00.000Z` : '';
    try {
      if (t.id) await api.update('inventory_items', t.id, fields);
      else await api.createInvItem(fields);
      thing = null;
      if (inGroup) items = await api.invItems(inGroup.id);
      await loadGroups();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  // Las marcas: se cargan una vez, y con eso una cosa cuyo proveedor se llama
  // «Cloudflare» toma su logo sin que nadie lo elija.
  logos.load();

  /** La pieza de una cosa, en orden de quién sabe más: lo que se eligió a mano,
   *  la marca de su proveedor, y si no, la del grupo. */
  function pieceOf(i: InvItem): { art: string; hex: string } {
    if (i.art) {
      const b = i.art.startsWith('si:') ? logos.find(i.art.slice(3)) : null;
      return { art: i.art, hex: b?.hex ?? '' };
    }
    const brand = logos.find(i.provider ?? '') ?? logos.find(i.name);
    if (brand) return { art: `si:${brand.slug}`, hex: brand.hex };
    return { art: inGroup?.art ?? '', hex: '' };
  }

  async function loadGroups() {
    try {
      groups = await api.invGroups();
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  loadGroups();

  async function enter(g: InvGroup) {
    inGroup = g;
    try {
      items = await api.invItems(g.id);
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function show(i: InvItem) {
    open = i;
    html = '';
    if (i.notes) {
      try {
        html = await api.renderMarkdown(i.notes);
      } catch {
        html = '';
      }
    }
  }

  /** Cuánto falta para que se renueve, en palabras. Lo que hace que esta
   *  pantalla se gane su sitio: un dominio que expira es el clásico «nadie se
   *  dio cuenta». */
  function renewal(at?: string) {
    if (!at) return null;
    const days = Math.round((Date.parse(at.replace(' ', 'T')) - Date.now()) / 86400000);
    if (Number.isNaN(days)) return null;
    if (days < 0) return { words: `venció ${ago(at)}`, urgent: true };
    if (days === 0) return { words: 'se renueva hoy', urgent: true };
    if (days <= 30) return { words: `se renueva en ${days} d`, urgent: days <= 7 };
    return { words: `se renueva en ${Math.round(days / 30)} meses`, urgent: false };
  }

  const soon = $derived(
    items
      .map((i) => ({ item: i, r: renewal(i.renews_at) }))
      .filter((x) => x.r?.urgent).length,
  );
</script>

<div class="screen" aria-label="inventario">
  <div class="topbar">
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <h2 class="ttl">Inventario</h2>
    {#if inGroup}
      <span class="faint">·</span>
      <button class="crumb" onclick={() => { inGroup = null; items = []; }}>todos los grupos</button>
      <span class="faint">›</span>
      <span class="here">{inGroup.name}</span>
    {:else}
      <span class="faint text-xs">dónde vive lo que hace funcionar todo esto</span>
    {/if}
    {#if canWrite && !inGroup}
      <button class="new" onclick={() => { editingGroup = null; fresh = { name: '', note: '', art: '' }; pickingFor = 'group'; making = true; loadCatalog(); }}>
        + grupo
      </button>
    {:else if canWrite && inGroup}
      <button class="new" onclick={() => { thing = { group: inGroup?.id }; pickingFor = 'item'; loadCatalog(); }}>
        + cosa
      </button>
    {/if}
  </div>

  {#if error}
    <p class="failed" role="alert">{error}</p>
  {/if}

  <div class="body">
    {#if !inGroup}
      <!-- La galería. Los grupos son datos y no una lista en el código: qué
           clases de cosas contrata un departamento cambia sin que nadie
           recompile. -->
      <ul class="gallery">
        {#each groups as g (g.id)}
          <li>
            <InvTile
              title={g.name}
              sub="{g.items} {g.items === 1 ? 'cosa' : 'cosas'}"
              art={g.art ?? ''}
              image={g.image ?? ''}
              hint={g.note ?? ''}
              actions={canWrite
                ? [
                    { value: 'open', label: 'Abrir' },
                    { value: 'add', label: '+ cosa aquí' },
                    { value: 'edit', label: 'Renombrar o cambiar la pieza' },
                    { value: 'drop', label: 'Borrar el grupo', tone: 'danger' as const },
                  ]
                : []}
              onopen={() => enter(g)}
              onaction={(what) => groupAction(what, g)} />
          </li>
        {:else}
          <li class="empty">
            <p class="faint">
              Todavía no hay nada inventariado.
              {#if canWrite}
                Los grupos y sus cosas se crean desde el panel de PocketBase por ahora.
              {/if}
            </p>
          </li>
        {/each}
      </ul>
    {:else}
      {#if soon}
        <p class="renew-warn">
          {soon} {soon === 1 ? 'cosa se renueva' : 'cosas se renuevan'} pronto, o ya venció.
        </p>
      {/if}
      <ul class="gallery">
        {#each items as i (i.id)}
          {@const r = renewal(i.renews_at)}
          <li>
            <InvTile
              title={i.name}
              sub={i.provider ?? ''}
              art={pieceOf(i).art}
              hex={pieceOf(i).hex}
              image={i.image ?? ''}
              badge={r?.words ?? ''}
              urgent={r?.urgent ?? false}
              hint={[i.provider, r?.words, i.cost].filter(Boolean).join(' · ')}
              actions={canWrite
                ? [
                    { value: 'open', label: 'Ver el detalle' },
                    { value: 'edit', label: 'Editar' },
                    ...(i.url ? [{ value: 'panel', label: 'Abrir el panel ↗' }] : []),
                    ...(i.vault ? [{ value: 'vault', label: 'Abrir la bóveda ↗' }] : []),
                    { value: 'drop', label: 'Borrar', tone: 'danger' as const },
                  ]
                : []}
              onopen={() => show(i)}
              onaction={(what) => itemAction(what, i)} />
          </li>
        {:else}
          <li class="empty"><p class="faint">Este grupo está vacío.</p></li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<div class="hud">
  <button class="hud-btn" onclick={onsearch} title="Buscar · ⌘K"><span aria-hidden="true">🔍</span></button>
  <ThemeToggle floating={false} />
</div>

{#if making}
  <Dialog open onOpenChange={() => (making = false)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-start justify-center overflow-y-auto p-4 pt-[7vh]"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-2xl space-y-4 p-5 shadow-xl">
          <header class="flex items-start gap-3">
            <Dialog.Title class="min-w-0 flex-1 text-lg font-bold">
              {editingGroup ? 'Editar el grupo' : 'Nuevo grupo'}
            </Dialog.Title>
            <Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface">✕</Dialog.CloseTrigger>
          </header>

          <form onsubmit={(e) => { e.preventDefault(); saveGroup(); }} class="space-y-3">
            <input
              class="input"
              placeholder="¿Cómo se llama? (Dominios, VPS, Licencias…)"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={fresh.name}
              {@attach (el: HTMLInputElement) => el.focus()} />
            <input
              class="input"
              placeholder="Una línea sobre qué guarda (opcional)"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={fresh.note} />

            <div class="picker">
              <div class="picker-head">
                <span class="lbl">La pieza</span>
                <input
                  class="input find"
                  placeholder="buscar…"
                  autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                  bind:value={hunting} />
                <span class="faint text-xs">
                  {fresh.art ? fresh.art.replace(/-/g, ' ') : `por el nombre: ${guess(fresh.name || '')}`}
                </span>
              </div>
              <!-- En COLOR aquí: al elegir se mira la pieza, y para eso hay que
                   verla como es. En la galería vive en barro, que es otra
                   pregunta — allá se comparan cuarenta, aquí se elige una.
                   Y sin marcas: un GRUPO es una clase de cosa —dominios, VPS—
                   y no es de nadie. La marca es de lo que hay dentro. -->
              <ul class="picks">
                {#each shortlist as c (c.slug)}
                  <li>
                    <button
                      type="button"
                      class="pick"
                      class:on={fresh.art === c.slug}
                      title={c.title}
                      onclick={() => (fresh.art = c.slug)}>
                      <img src={lit(c.slug)} alt={c.title} loading="lazy" />
                    </button>
                  </li>
                {/each}
              </ul>
            </div>

            <div class="flex justify-end gap-2">
              <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (making = false)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-primary-500" disabled={!fresh.name.trim()}>Crear</button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

{#if thing}
  <!-- El alta y la edición de una cosa, en el mismo formulario: son los mismos
       campos, y dos formularios serían dos sitios donde olvidar uno. -->
  <Dialog open onOpenChange={() => (thing = null)}>
    <Portal>
      <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
      <Dialog.Positioner
        class="fixed inset-0 flex items-start justify-center overflow-y-auto p-4 pt-[7vh]"
        style="z-index: var(--z-drawer)">
        <Dialog.Content class="card bg-surface-100-900 w-full max-w-2xl space-y-4 p-5 shadow-xl">
          <header class="flex items-start gap-3">
            <Dialog.Title class="min-w-0 flex-1 text-lg font-bold">
              {thing.id ? 'Editar' : 'Nueva cosa'}{inGroup ? ` · ${inGroup.name}` : ''}
            </Dialog.Title>
            <Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface">✕</Dialog.CloseTrigger>
          </header>

          <form onsubmit={(e) => { e.preventDefault(); saveThing(); }} class="space-y-3">
            <div class="pair">
              <input class="input" placeholder="¿Cómo se llama? (vps-01, cuby.mx…)"
                autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                bind:value={thing.name} {@attach (el: HTMLInputElement) => el.focus()} />
              <input class="input" placeholder="Proveedor (Hetzner, Cloudflare…)"
                autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                bind:value={thing.provider} />
            </div>
            <div class="pair">
              <input class="input" placeholder="Costo (6 EUR/mes)"
                autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                bind:value={thing.cost} />
              <!-- El mismo calendario que el planeador. Un `<input type=date>`
                   se dibuja distinto en cada navegador y en ninguno se parece a
                   esta aplicación; y sobre todo, dos formas de elegir una fecha
                   en el mismo producto son dos formas de equivocarse. -->
              <DatePicker
                defaultValue={renewValue}
                onValueChange={(e: { value: { year: number; month: number; day: number }[] }) =>
                  (thing = { ...thing, renews_at: isoDay(e.value?.[0]) })}
                locale="es-MX"
                startOfWeek={1}>
                <!-- Sin etiqueta encima: el resto de esta hoja pregunta desde
                     dentro del campo, y una etiqueta suelta desalineaba la fila. -->
                <DatePicker.Control class="input dp-control">
                  <DatePicker.Input placeholder="Se renueva" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
                  <DatePicker.Trigger>🗓</DatePicker.Trigger>
                </DatePicker.Control>
                <!-- Sin portal, como en la tarjeta del planeador: un diálogo
                     modal apaga los eventos fuera de sí y los devuelve capa por
                     capa, así que un calendario que aterriza en el <body> se
                     dibuja bien y no recibe un solo clic. -->
                <DatePicker.Positioner>
                  <DatePicker.Content class="dp-content">
                    <DatePicker.View view="day">
                      <DatePicker.Context>
                        {#snippet children(dp)}
                          <DatePicker.ViewControl class="dp-nav">
                            <DatePicker.PrevTrigger>‹</DatePicker.PrevTrigger>
                            <DatePicker.ViewTrigger><DatePicker.RangeText /></DatePicker.ViewTrigger>
                            <DatePicker.NextTrigger>›</DatePicker.NextTrigger>
                          </DatePicker.ViewControl>
                          <DatePicker.Table>
                            <DatePicker.TableHead>
                              <DatePicker.TableRow>
                                {#each dp().weekDays as d, i (i)}
                                  <DatePicker.TableHeader>{d.short}</DatePicker.TableHeader>
                                {/each}
                              </DatePicker.TableRow>
                            </DatePicker.TableHead>
                            <DatePicker.TableBody>
                              {#each dp().weeks as week, i (i)}
                                <DatePicker.TableRow>
                                  {#each week as day, j (j)}
                                    <DatePicker.TableCell value={day}>
                                      <DatePicker.TableCellTrigger>{day.day}</DatePicker.TableCellTrigger>
                                    </DatePicker.TableCell>
                                  {/each}
                                </DatePicker.TableRow>
                              {/each}
                            </DatePicker.TableBody>
                          </DatePicker.Table>
                        {/snippet}
                      </DatePicker.Context>
                    </DatePicker.View>
                  </DatePicker.Content>
                </DatePicker.Positioner>
              </DatePicker>
            </div>
            <input class="input" placeholder="URL del panel"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={thing.url} />
            <input class="input" placeholder="Enlace a la bóveda — NUNCA la contraseña"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              bind:value={thing.vault} />
            <textarea class="input" rows="3" placeholder="Notas (markdown): qué corre aquí, qué depende de esto…"
              bind:value={thing.notes}></textarea>

            <!-- Sólo marcas. Una COSA es de alguien —un VPS está en Hetzner, un
                 dominio en Namecheap— y su logo dice eso más rápido que
                 cualquier objeto. Las piezas 3D son para los grupos, que son
                 clases de cosa y no son de nadie. -->
            <div class="picker">
              <div class="picker-head">
                <span class="lbl">La marca</span>
                <input class="input find" placeholder="buscar entre 3459…"
                  autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
                  bind:value={hunting} />
                <span class="faint text-xs">
                  {thing.art
                    ? thing.art.replace(/^si:/, '')
                    : logos.find(thing.provider ?? '')
                      ? 'la de su proveedor'
                      : 'la pieza de su grupo'}
                </span>
              </div>
              <ul class="picks">
                <li>
                  <!-- Ninguna: hereda la pieza del grupo. Sin esto, elegir una
                       marca por error no tenía vuelta atrás. -->
                  <button type="button" class="pick none" class:on={!thing.art}
                    title="ninguna — la del grupo" onclick={() => (thing = { ...thing, art: '' })}>
                    <span>—</span>
                  </button>
                </li>
                {#each brands as b (b.slug)}
                  <li>
                    <button type="button" class="pick brand-pick" class:on={thing.art === `si:${b.slug}`}
                      title={b.title} onclick={() => (thing = { ...thing, art: `si:${b.slug}` })}>
                      <span style="--mask: url(/logos/{b.slug}.svg); --brand: #{b.hex}"></span>
                    </button>
                  </li>
                {/each}
              </ul>
            </div>

            <div class="flex justify-end gap-2">
              <button type="button" class="btn btn-sm preset-tonal-surface" onclick={() => (thing = null)}>
                Cancelar
              </button>
              <button class="btn btn-sm preset-filled-primary-500" disabled={!thing.name?.trim()}>
                Guardar
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Positioner>
    </Portal>
  </Dialog>
{/if}

<Confirm bind:ask={doom} />

<Dialog open={!!open} onOpenChange={() => (open = null)}>
  <Portal>
    <Dialog.Backdrop class="scrim" style="z-index: var(--z-drawer-scrim)" />
    <Dialog.Positioner
      class="fixed inset-0 flex items-start justify-center overflow-y-auto p-4 pt-[8vh]"
      style="z-index: var(--z-drawer)">
      <Dialog.Content class="card bg-surface-100-900 w-full max-w-xl space-y-4 p-5 shadow-xl">
        {#if open}
          {@const r = renewal(open.renews_at)}
          <header class="flex items-start gap-3">
            <Dialog.Title class="min-w-0 flex-1 text-lg font-bold">{open.name}</Dialog.Title>
            <Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface">✕</Dialog.CloseTrigger>
          </header>

          {#if open.image}
            <img class="hero" src={open.image} alt="" />
          {/if}

          <dl class="facts">
            {#if open.provider}
              <dt>Proveedor</dt>
              <dd>{open.provider}</dd>
            {/if}
            {#if open.cost}
              <dt>Costo</dt>
              <dd>{open.cost}</dd>
            {/if}
            {#if r}
              <dt>Renovación</dt>
              <dd class:urgent={r.urgent}>{r.words}</dd>
            {/if}
            {#if open.url}
              <dt>Panel</dt>
              <dd><a href={open.url} target="_blank" rel="noreferrer">{open.url}</a></dd>
            {/if}
            {#if open.vault}
              <dt>Credenciales</dt>
              <dd>
                <a href={open.vault} target="_blank" rel="noreferrer">en la bóveda ↗</a>
                <span class="faint">— aquí no se guardan contraseñas</span>
              </dd>
            {/if}
          </dl>

          {#if html}
            <!-- Las notas son otra cosa que los datos: una línea las separa, o
                 el primer párrafo se lee como el valor del último campo. -->
            <div class="notes"><Prose compact {html} /></div>
          {/if}
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  .screen {
    display: flex;
    flex-direction: column;
    height: 100dvh;
    overflow: hidden;
  }
  .topbar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    height: 46px;
    padding: 0 0.9rem;
    border-bottom: 1px solid var(--line);
    background: color-mix(in oklab, var(--bg) 82%, transparent);
    backdrop-filter: blur(8px);
  }
  .back {
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.82rem;
  }
  .back:hover { color: var(--text); background: var(--hover); }
  .ttl { margin: 0; font-size: 0.95rem; font-weight: 700; }
  .crumb { color: var(--muted); font-size: 0.82rem; }
  .crumb:hover { color: var(--text); text-decoration: underline; }
  .here { font-size: 0.82rem; font-weight: 600; }

  .body { flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 1.2rem 1.4rem 5rem; }

  .renew-warn {
    max-width: 1100px;
    margin: 0 auto 1rem;
    padding: 0.55rem 0.8rem;
    border-radius: 10px;
    background: color-mix(in oklab, var(--warm, orange) 18%, transparent);
    color: var(--text);
    font-size: 0.84rem;
  }

  /* Una galería, no una tabla: lo que se busca aquí se reconoce por su forma
     antes que por su nombre. La baldosa y su hover viven en `InvTile`. */
  .gallery {
    max-width: 1100px;
    margin: 0 auto;
    padding: 0;
    list-style: none;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
    gap: 1rem;
  }
  dd.urgent { color: var(--p1, tomato); font-weight: 600; }
  .empty { grid-column: 1 / -1; }

  .new {
    margin-left: auto;
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.8rem;
  }
  .new:hover { color: var(--text); background: var(--hover); }

  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 0.6rem; }
  @media (max-width: 620px) { .pair { grid-template-columns: 1fr; } }

  .picker { display: grid; gap: 0.5rem; }
  .picker-head { display: flex; align-items: center; gap: 0.6rem; }
  .lbl { color: var(--faint); font-size: 0.7rem; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; }
  .find { flex: 1; max-width: 14rem; }
  .picks {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(64px, 1fr));
    gap: 0.4rem;
    max-height: 15rem;
    overflow-y: auto;
    margin: 0;
    padding: 0.2rem;
    list-style: none;
    border: 1px solid var(--line);
    border-radius: 12px;
  }
  .pick {
    width: 100%;
    aspect-ratio: 1;
    display: grid;
    place-items: center;
    border-radius: 10px;
    background: transparent;
    padding: 0.25rem;
  }
  .pick img { max-width: 100%; max-height: 100%; object-fit: contain; }
  /* La marca se pinta con su color desde el principio en el SELECTOR: aquí se
     está eligiendo, y para eso hay que verla como es. En la galería vive en
     gris, que es otra pregunta. */
  .pick.none span { color: var(--faint); font-size: 1.1rem; }
  .brand-pick span {
    width: 60%;
    height: 60%;
    background: var(--brand);
    -webkit-mask: var(--mask) center / contain no-repeat;
    mask: var(--mask) center / contain no-repeat;
  }
  .pick:hover { background: var(--hover); }
  .pick.on { background: color-mix(in oklab, var(--accent) 18%, transparent); outline: 2px solid var(--accent); }

  .facts {
    display: grid;
    grid-template-columns: auto 1fr;
    /* Baseline, no `start`: la etiqueta es tipografía pequeña en mayúsculas y el
       valor es texto normal, así que alinearlos por el borde de arriba los deja
       flotando a alturas distintas — que es lo que se veía. Se alinean por la
       línea sobre la que se escriben, como en una ficha. */
    align-items: baseline;
    gap: 0.55rem 1rem;
    margin: 0;
    font-size: 0.88rem;
  }
  .facts dt {
    color: var(--faint);
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    /* A la derecha: la columna de etiquetas tiene anchos muy distintos
       —PANEL, CREDENCIALES— y alineadas a la izquierda dejan un desfase
       aleatorio contra los valores. Pegadas a su columna, todos los valores
       arrancan en la misma vertical. */
    text-align: right;
    white-space: nowrap;
  }
  .facts dd { margin: 0; min-width: 0; overflow-wrap: anywhere; line-height: 1.5; }
  .notes { padding-top: 0.9rem; border-top: 1px solid var(--line); }
  .hero { width: 100%; max-height: 220px; object-fit: contain; border-radius: 10px; background: var(--hover); }

  .hud {
    position: fixed;
    right: 1rem;
    bottom: 1rem;
    z-index: var(--z-chrome);
    display: flex;
    gap: 0.5rem;
  }
  .hud-btn {
    width: 40px;
    height: 40px;
    display: grid;
    place-content: center;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    font-size: 1rem;
  }
  .hud-btn:hover { background: var(--hover); }

  .failed {
    margin: 0.6rem auto;
    max-width: 1100px;
    padding: 0.5rem 0.75rem;
    border-radius: 10px;
    background: color-mix(in oklab, var(--p1, tomato) 14%, transparent);
    color: var(--text);
    font-size: 0.85rem;
  }
</style>
