<script lang="ts">
  // The design system, on one page: the tokens and the controls, nothing else.
  //
  // Not decoration: a theme is a claim that every control looks like it belongs
  // to the same thing, and the only way to check that claim is to put them beside
  // each other. Each component says WHERE it is going, so this is also the plan
  // for the interface rather than a catalogue of what a library happens to ship.
  //
  // The interface ITSELF — the board, a bubble, a thread, the wiki, the planner —
  // is at /theme/mock. Two pages because they answer different questions: this
  // one asks whether a control is right, that one whether the product is.
  import {
    Switch, Slider, Tabs, Tooltip, Avatar, Progress, Accordion,
    SegmentedControl,
    Combobox,
    Menu, Portal } from '@skeletonlabs/skeleton-svelte';
  import { collection } from '@zag-js/combobox';
  import Orb from './Orb.svelte';
  import SideTree from './SideTree.svelte';
  import { wiki, bandWhy } from '../lib/mock';
  import { bandFace, bandName, bandOrder } from '../lib/bands';

  // A dropdown that has to LOOK like the rest of the app cannot be a native
  // <select>: the utility styles the closed control, and the popup is drawn by
  // the operating system where no stylesheet reaches it. Anywhere the open list
  // matters, this is the component.
  const priorities = ['P1 · crítica', 'P2 · alta', 'P3 · normal', 'P4 · baja'];
  const prioCollection = collection({ items: priorities });
  let prio = $state<string[]>([]);

  const palettes = ['primary', 'secondary', 'tertiary', 'success', 'warning', 'error', 'surface'];
  const steps = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950];
  const bands = bandOrder.map((k) => ({ k, face: bandFace(k), why: bandWhy[k] }));

  // What is not on this page yet, and where each one lands.
  const planned = [
    ['dialog', 'el conflicto de escritura, y confirmar un borrado'],
    ['toast', 'errores que no deben robar el foco'],
    ['date-picker', 'el calendario del planeador, sobre due_date'],
    ['tags-input', 'etiquetas de un thread'],
    ['file-upload', 'subir una imagen a assets/'],
    ['steps', 'las fases de un onboarding'],
    ['pagination', 'la historia de un documento'],
  ];

  let sw = $state(true);
  let sl = $state([2]);
  let picked = $state('—');
</script>

<div class="mx-auto max-w-4xl space-y-10 p-6">
  <header>
    <h1 class="display text-3xl">Sistema de diseño</h1>
    <p class="faint mt-1 text-sm">
      Los primitivos de Skeleton, tematizados con <code>bubble.css</code>. Cambiar
      un token ahí mueve todo lo de esta página a la vez.
    </p>
    <p class="mt-3 text-sm">
      <a class="btn btn-sm preset-tonal-primary" href="/theme/mock">Ver la interfaz completa →</a>
      <span class="faint ml-2">el board, una burbuja, un thread, la wiki y el planeador</span>
    </p>
  </header>

  <!-- ── the scales ─────────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">Escalas</h2>
    <p class="faint text-sm">
      Una sola rampa de luminosidad para las siete: un <code>-500</code> pesa lo
      mismo en cualquier paleta.
    </p>
    {#each palettes as p}
      <div class="flex items-center gap-2">
        <span class="muted w-20 shrink-0 text-xs">{p}</span>
        <div class="flex flex-1 overflow-hidden rounded-lg">
          {#each steps as s}
            <div
              class="h-8 flex-1"
              style="background: var(--color-{p}-{s})"
              title="--color-{p}-{s}"></div>

          {/each}
        </div>
      </div>
    {/each}
  </section>

  <!-- ── the bands ──────────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">Las cuatro bandas</h2>
    <p class="faint text-sm">
      <b>Fuera del tema, a propósito.</b> Son el dominio hablando, no semántica de
      UI: no se mapean a primary/success/error, o la banda de una burbuja cambiaría
      porque alguien restilizó un botón.
    </p>
    <div class="grid gap-2 sm:grid-cols-2">
      {#each bands as b}
        <div class="card glass band-{b.k} flex items-center gap-3 p-3">
          <span class="dot"></span>
          <span class="text-lg">{b.face}</span>
          <div class="min-w-0">
            <div class="text-sm font-medium">{bandName(b.k)}</div>
            <div class="faint text-xs">{b.why}</div>
          </div>
        </div>
      {/each}
    </div>
    <div class="card glass band-dormant sunk flex items-center gap-3 p-3">
      <span class="dot"></span>
      <span class="text-sm">…y lo hundido se dessatura, para leerse sin etiqueta</span>
    </div>
    <p class="faint text-xs">
      Eran cinco: entre 🔥 y 😴 estaban <i>tibio</i> y <i>enfriando</i>, un gradiente
      sobre el que nadie actuaba — «produjo el ciclo pasado» y «no produjo ninguno»
      llevan a la misma mañana. Cada banda que queda nombra una <b>acción distinta</b>,
      y 🪦 es banda propia y no un adorno de 😴: callada con responsable es alguien a
      quien preguntar; callada sin responsable es una decisión que tomar.
    </p>
  </section>

  <!-- ── the orbs ───────────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">Las burbujas</h2>
    <p class="faint text-sm">
      El componente central, y la razón del nombre. <b>Una esfera, no una tarjeta</b>:
      la afirmación del modelo es física — la atención se comporta como un fluido,
      lo caliente sube y lo frío se hunde. Tarjetas en una cuadrícula dicen «lista
      con colores»; orbes flotando en una banda dicen lo que el modelo significa,
      antes de leer una palabra.
    </p>
    <div class="card glass flex flex-wrap items-start justify-center gap-4 p-6">
      <Orb name="Rate limiting" lifecycle="hot" burning={3} owner="Angel" people={['Angel', 'Bea', 'Caro', 'Dani']} index={0} />
      <Orb name="Rediseño del portal" lifecycle="dormant" owner="Caro" people={['Caro', 'Dani']} index={1} />
      <Orb name="Integración con el ERP" lifecycle="rip" index={2} />
      <Orb name="Auditoría ISO" lifecycle="closed" index={3} />
    </div>
    <ul class="faint space-y-1 text-xs">
      <li>· El <b>bob</b> va escalonado — una banda respira en vez de pulsar al unísono.</li>
      <li>· El <b>brillo</b> es lo que la hace esfera y no disco.</li>
      <li>· El <b>badge</b> cuenta lo que está <i>ardiendo</i>, no el inventario: doce
        threads terminados no son un «12», ese número se lee como carga de trabajo.</li>
      <li>· Los <b>avatares</b> ponen al dueño encima, el resto detrás.</li>
      <li>· Con <code>prefers-reduced-motion</code> no se mueve nada.</li>
    </ul>
  </section>

  <!-- ── the pair ───────────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">La tipografía</h2>
    <p class="faint text-sm">
      <b class="display text-base">Boogaloo</b> para lo que se <i>mira</i> — el nombre
      de una burbuja, el de una banda — y <b>DM Sans</b> para todo lo que se
      <i>lee</i>. Una display carga un nombre y arruina un párrafo; separarlas es
      lo que deja que la primera sea alocada sin costo.
    </p>

    <div class="card glass space-y-4 p-5">
      <div>
        <div class="faint text-xs">display · Boogaloo</div>
        <div class="display text-4xl">Migración a PocketBase</div>
        <div class="display text-2xl">Rentabilidad · Clientes · ISO 9001</div>
        <div class="muted display text-sm tracking-widest uppercase">Caliente · Tibio · Dormido</div>
      </div>
      <hr class="hr" />
      <div>
        <div class="faint text-xs">texto · DM Sans</div>
        <p class="max-w-prose text-sm">
          Heat es evidencia de realidad cambiada, no actividad. Una burbuja que se
          calló es información, no un fracaso: revívela con trabajo real,
          redefínela, o ciérrala.
        </p>
      </div>
      <hr class="hr" />
      <div class="flex flex-wrap gap-3">
        <Orb name="Migración a PocketBase" lifecycle="hot" burning={5} owner="Angel" people={['Angel','Bea']} />
        <Orb name="Auditoría ISO 9001" lifecycle="closed" index={2} />
        <Orb name="Deuda técnica del monolito" lifecycle="dormant" index={4} />
      </div>
    </div>

    <ul class="faint space-y-1 text-xs">
      <li>· Las dos vienen de <b>npm (Fontsource)</b> y se sirven desde este servidor:
        <b>ninguna petición sale de la máquina</b>. Ambas OFL — uso comercial, sin comprar nada.</li>
      <li>· <b>Boogaloo tiene un solo peso.</b> Pedirle negrita hace que el navegador la
        falsifique, y una display falseada se lee como un error de render. El peso está
        fijo en <code>.display</code>: el énfasis lo cargan el tamaño y el color.</li>
      <li>· <b>LAMORE</b> quedó fuera: gratuita solo para uso <i>personal</i>, y esto es
        software de empresa.</li>
    </ul>
  </section>

  <!-- ── buttons ────────────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">Botones</h2>
    <div class="flex flex-wrap items-center gap-2">
      <button class="btn preset-filled-primary-500">primario</button>
      <button class="btn preset-tonal-primary">tonal</button>
      <button class="btn preset-outlined-primary-500">contorno</button>
      <button class="btn preset-filled-error-500">destructivo</button>
      <button class="btn preset-tonal-surface" disabled>deshabilitado</button>
      <button class="btn btn-sm preset-filled-primary-500">pequeño</button>
    </div>
  </section>

  <!-- ── forms ──────────────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">Formularios</h2>
    <div class="grid gap-3 sm:grid-cols-2">
      <label class="text-sm"><span class="muted">Texto</span>
        <input class="input mt-1" placeholder="nombre del thread" /></label>
      <div class="text-sm">
        <Combobox collection={prioCollection} value={prio}
          onValueChange={(e: { value: string[] }) => (prio = e.value)}>
          <Combobox.Label class="muted">Selección — <b>Combobox</b></Combobox.Label>
          <Combobox.Control>
            <Combobox.Input placeholder="prioridad…" />
            <Combobox.Trigger>▾</Combobox.Trigger>
          </Combobox.Control>
          <Portal>
            <Combobox.Positioner>
            <Combobox.Content>
              {#each priorities as p (p)}
                <Combobox.Item item={p}>
                  <Combobox.ItemText>{p}</Combobox.ItemText>
                  <Combobox.ItemIndicator>✓</Combobox.ItemIndicator>
                </Combobox.Item>
              {/each}
            </Combobox.Content>
            </Combobox.Positioner>
          </Portal>
        </Combobox>
      </div>
      <label class="text-sm sm:col-span-2"><span class="muted">Área</span>
        <textarea class="textarea mt-1" rows="2" placeholder="¿qué es verdad cuando esto esté hecho?"></textarea></label>
    </div>
    <div class="card glass space-y-2 p-4">
      <div class="text-sm font-medium">…y el <code>&lt;select&gt;</code> nativo, para comparar</div>
      <p class="faint text-xs">
        La utilidad <code>.select</code> estiliza el control <b>cerrado</b>. La lista
        desplegada la dibuja el sistema operativo y ningún CSS la alcanza — por eso
        se ve ajena. Sigue siendo la opción correcta cuando el popup da igual:
        es accesible, nativa en móvil, y cuesta cero JavaScript.
      </p>
      <select class="select"><option>alta</option><option>media</option><option>baja</option></select>
    </div>
  </section>

  <!-- ── components, each with where it goes ────────────────────────────── -->
  <section class="space-y-5">
    <h2 class="display text-base tracking-widest uppercase">Componentes</h2>

    <div class="card glass space-y-2 p-4">
      <div class="text-sm font-medium">SegmentedControl</div>
      <p class="faint text-xs">tema claro/oscuro · timeline / semana / día en el planeador</p>
      <SegmentedControl value="a">
        <SegmentedControl.Control>
          <SegmentedControl.Indicator />
          {#each ['timeline', 'semana', 'día'] as t, i}
            <SegmentedControl.Item value={['a', 'b', 'c'][i]}>
              <SegmentedControl.ItemText>{t}</SegmentedControl.ItemText>
              <SegmentedControl.ItemHiddenInput />
            </SegmentedControl.Item>
          {/each}
        </SegmentedControl.Control>
      </SegmentedControl>
    </div>

    <div class="card glass space-y-2 p-4">
      <div class="text-sm font-medium">Tabs</div>
      <p class="faint text-xs">el interior de un thread: documento · historia · evidencia</p>
      <Tabs defaultValue="doc">
        <Tabs.List>
          <Tabs.Trigger value="doc">documento</Tabs.Trigger>
          <Tabs.Trigger value="hist">historia</Tabs.Trigger>
          <Tabs.Trigger value="ev">evidencia</Tabs.Trigger>
          <Tabs.Indicator />
        </Tabs.List>
        <Tabs.Content value="doc"><p class="faint pt-3 text-sm">el markdown, con un solo renderer</p></Tabs.Content>
        <Tabs.Content value="hist"><p class="faint pt-3 text-sm">los commits de git</p></Tabs.Content>
        <Tabs.Content value="ev"><p class="faint pt-3 text-sm">qué calentó esto y cuándo</p></Tabs.Content>
      </Tabs>
    </div>

    <div class="grid gap-3 sm:grid-cols-2">
      <div class="card glass space-y-2 p-4">
        <div class="text-sm font-medium">Switch</div>
        <p class="faint text-xs">calibración: «sin dueño se duerme»</p>
        <Switch checked={sw} onCheckedChange={(e: { checked: boolean }) => (sw = e.checked)}>
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Label>ownerless_is_dormant</Switch.Label>
          <Switch.HiddenInput />
        </Switch>
      </div>

      <div class="card glass space-y-2 p-4">
        <div class="text-sm font-medium">Slider</div>
        <p class="faint text-xs">calibración: ciclos hasta dormir</p>
        <Slider value={sl} min={1} max={6} onValueChange={(e: { value: number[] }) => (sl = e.value)}>
          <Slider.Control>
            <Slider.Track><Slider.Range /></Slider.Track>
            <Slider.Thumb index={0}><Slider.HiddenInput /></Slider.Thumb>
          </Slider.Control>
          <Slider.ValueText />
        </Slider>
      </div>

      <div class="card glass space-y-2 p-4">
        <div class="text-sm font-medium">Avatar</div>
        <p class="faint text-xs">asignados de un thread · autor de un comentario</p>
        <div class="flex gap-2">
          <Avatar><Avatar.Fallback>AM</Avatar.Fallback></Avatar>
          <Avatar><Avatar.Fallback>BC</Avatar.Fallback></Avatar>
        </div>
      </div>

      <div class="card glass space-y-2 p-4">
        <div class="text-sm font-medium">Progress</div>
        <p class="faint text-xs">casillas marcadas de un documento</p>
        <Progress value={40}>
          <Progress.Track><Progress.Range /></Progress.Track>
        </Progress>
      </div>
    </div>

    <div class="card glass space-y-2 p-4">
      <div class="text-sm font-medium">Accordion</div>
      <p class="faint text-xs">el inbox del planeador · secciones plegables de la wiki</p>
      <Accordion>
        <Accordion.Item value="1">
          <Accordion.ItemTrigger>Por qué esta burbuja está fría<Accordion.ItemIndicator /></Accordion.ItemTrigger>
          <Accordion.ItemContent><p class="faint text-sm">nada este ciclo ni el anterior</p></Accordion.ItemContent>
        </Accordion.Item>
        <Accordion.Item value="2">
          <Accordion.ItemTrigger>Contra qué calibración<Accordion.ItemIndicator /></Accordion.ItemTrigger>
          <Accordion.ItemContent><p class="faint text-sm">ciclo de 168 h, dormido tras 2</p></Accordion.ItemContent>
        </Accordion.Item>
      </Accordion>
    </div>

    <div class="card glass space-y-2 p-4">
      <div class="text-sm font-medium">Menu</div>
      <p class="faint text-xs">selector de workspace · acciones de una burbuja (clic derecho)</p>
      <Menu>
        <Menu.Trigger><span class="btn btn-sm preset-tonal-surface">Software ▾</span></Menu.Trigger>
        <Portal>
          <Menu.Positioner>
          <Menu.Content>
            <Menu.Item value="a"><Menu.ItemText>Software</Menu.ItemText></Menu.Item>
            <Menu.Item value="b"><Menu.ItemText>Infraestructura</Menu.ItemText></Menu.Item>
            <Menu.Item value="c"><Menu.ItemText>Calidad · ISO 9001</Menu.ItemText></Menu.Item>
            <Menu.Item value="d"><Menu.ItemText>Soporte a clientes</Menu.ItemText></Menu.Item>
            <Menu.Separator />
            <Menu.Item value="new"><Menu.ItemText>+ nuevo workspace</Menu.ItemText></Menu.Item>
          </Menu.Content>
          </Menu.Positioner>
        </Portal>
      </Menu>
    </div>

    <div class="card glass space-y-2 p-4">
      <div class="text-sm font-medium">Tooltip</div>
      <p class="faint text-xs">la razón de una banda, sin ocupar la tarjeta</p>
      <Tooltip>
        <Tooltip.Trigger><span class="btn btn-sm preset-tonal-surface">pásame el cursor</span></Tooltip.Trigger>
        <Portal>
          <Tooltip.Positioner>
          <Tooltip.Content>produjo algo en el ciclo actual</Tooltip.Content>
          </Tooltip.Positioner>
        </Portal>
      </Tooltip>
    </div>
  </section>

  <!-- ── the tree ───────────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">El árbol</h2>
    <p class="faint text-sm">
      Skeleton TreeView. Es el mismo componente del sidebar de un thread y el de la
      wiki (<code>docs/</code>): teclado, expandir/colapsar y aria vienen con él.
    </p>
    <div class="card glass max-w-sm p-3">
      <SideTree nodes={wiki} onselect={(n) => (picked = n.id)} />
    </div>
    <p class="faint text-sm">seleccionado: <code>{picked}</code></p>
  </section>

  <!-- ── what is coming ─────────────────────────────────────────────────── -->
  <section class="space-y-3">
    <h2 class="display text-base tracking-widest uppercase">Todavía sin usar</h2>
    <p class="faint text-sm">Disponibles en Skeleton, y dónde va cada uno cuando toque.</p>
    <ul class="card glass divide-y-[1px] divide-[var(--line)] p-2 text-sm">
      {#each planned as [name, where]}
        <li class="flex flex-wrap gap-x-3 px-2 py-1.5">
          <code class="w-32 shrink-0">{name}</code>
          <span class="faint">{where}</span>
        </li>
      {/each}
    </ul>
  </section>
</div>
