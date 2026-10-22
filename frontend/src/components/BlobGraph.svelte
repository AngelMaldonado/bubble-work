<script lang="ts" generics="N extends GraphNode">
  import { onMount, untrack } from 'svelte';
  // La vista Blobs: un blob por tag, un punto por burbuja o por hilo, y sólo
  // las relaciones declaradas como aristas. La posición es fija
  // (`graphLayout`); arrastrar mueve la lectura, no clasifica nada.
  import { graphLayout, fitCamera, zoomCamera, groupFontSize, levels, rank, type Point, type Camera, type GraphGroup, type GraphLink, type MapKind, type GraphNode, type Level } from '../lib/graph';
  import { bandName } from '../lib/bands';
  import Icon from './Icon.svelte';
  import { STATE_SPHERE, type Look, type Sphere } from '../lib/encoding';

  let { nodes, universe, groups: blobs, links, kind, resetKey, onopen, onfury, fury: showFury = true, height = 'clamp(30rem, 62dvh, 48rem)', canvas = false, title = '', empty = '', looks }: {
    nodes: N[]; universe: N[]; groups: GraphGroup[]; links: GraphLink[];
    kind: MapKind; resetKey: string; onopen: (node: N) => void;
    /** el aro de furia; sin él, abre el nodo */
    onfury?: (node: N) => void;
    /** ¿hay furia que enseñar? Sin evaluador, no: la leyenda no la promete */
    fury?: boolean;
    height?: string;
    /** Lienzo a pantalla completa, como la secuencia: llena su contenedor,
     *  fondo punteado que sigue a la cámara, zoom en una píldora arriba a la
     *  izquierda y el título en una insignia arriba al centro. */
    canvas?: boolean;
    /** el título de la insignia (sólo en `canvas`) */
    title?: string;
    /** qué decir cuando no hay puntos: fijo al centro, no se mueve con el mapa */
    empty?: string;
    /** tamaño, color y filtro de cada punto (lib/encoding); sin esto, el estado */
    looks?: Map<string, Look>;
  } = $props();
  const fury = (node: N) => (onfury ?? onopen)(node);
  // Las esferas que se usan: por omisión los colores de Flotabilidad
  // (Orb.svelte) por banda; con una codificación, las de su color. Un degradado
  // por color, con un índice como id (las claves llevan nombres libres).
  const lookOf = (n: N): Look => looks?.get(n.id) ?? { size: 1, sphere: STATE_SPHERE[n.life] ?? STATE_SPHERE.hot, match: true };
  const spheres = $derived.by(() => {
    const out = new Map<string, { i: number; s: Sphere }>();
    for (const s of Object.values(STATE_SPHERE)) out.set(s.key, { i: out.size, s });
    for (const l of looks?.values() ?? []) if (!out.has(l.sphere.key)) out.set(l.sphere.key, { i: out.size, s: l.sphere });
    return out;
  });
  const sphereId = (n: N) => spheres.get(lookOf(n).sphere.key)?.i ?? 0;
  const layout = $derived(graphLayout(universe, blobs));
  const groups = $derived(blobs.map((blob) => {
    const members = nodes.filter((p) => p.group === blob.id);
    const open = members.filter((p) => p.life !== 'closed');
    const max = open.reduce((level, p) => rank[p.level] > rank[level] ? p.level : level, 'calm' as Level);
    return { ...blob, members, open: open.length, max, anchor: layout.groups.get(blob.id)! };
  }).filter((g) => g.members.length));
  let wrapper: HTMLDivElement;
  let svg: SVGSVGElement;
  let size = $state({ width: 1000, height: 550 });
  let camera = $state<Camera>({ x: 0, y: 0, scale: 1 });
  let hover = $state('');
  const prefix = `model-graph-${Math.random().toString(36).slice(2)}`;
  let gesture: { pointer: number; id?: string; fury: boolean; x: number; y: number; origin: Point; moved: boolean } | undefined;
  let clickCandidate: { id: string; fury: boolean } | undefined;
  // La mano cerrada (familia de cursores de los lienzos, app.css) sólo cuando
  // el gesto ya es un arrastre: un clic para abrir no la hace parpadear.
  let grabbing = $state(false);
  // El punto con foco de teclado: su nombre se ve aunque no quepa.
  let focused = $state('');
  // La posición es la de la disposición: los puntos no se arrastran. Arrastrar
  // mueve la cámara, empiece donde empiece el gesto.
  const point = (id: string) => layout.positions.get(id) ?? { x: 0, y: 0 };
  function fit() {
    const points = [...nodes.map((n) => point(n.id)), ...groups.map((g) => ({ x: g.anchor.x, y: g.anchor.y - g.anchor.radius - 45 }))];
    if (!canvas) { camera = fitCamera(points, size.width, size.height); return; }
    // En el lienzo, arriba flota el título y abajo la leyenda y el HUD: se
    // encuadra en la franja libre entre ellos, no en el alto entero.
    const top = size.width < 650 ? 130 : 80, bottom = size.width < 1150 ? 170 : 115;
    const c = fitCamera(points, size.width, Math.max(120, size.height - top - bottom));
    camera = { ...c, y: c.y - (top - bottom) / 2 / c.scale };
  }
  onMount(() => {
    const observer = new ResizeObserver(([entry]) => { size = { width: entry.contentRect.width, height: entry.contentRect.height }; });
    observer.observe(wrapper);
    // La captura retargetea el click al SVG. Delegarlo permite distinguirlo
    // del arrastre y abrir el detalle sólo cuando el gesto ha terminado.
    svg.addEventListener('click', click);
    return () => { observer.disconnect(); svg.removeEventListener('click', click); cancelAnimationFrame(lensFrame); lensFrame = 0; };
  });
  $effect(() => {
    void resetKey; void size;
    untrack(fit);
  });
  // La retícula del lienzo, en pantalla: un punto cada 40 unidades del mundo,
  // duplicando el paso cuando el zoom los juntaría demasiado.
  const grid = $derived.by(() => {
    let step = 40 * camera.scale;
    while (step < 14) step *= 2;
    const ox = size.width / 2 - camera.x * camera.scale, oy = size.height / 2 - camera.y * camera.scale;
    const mod = (v: number) => ((v % step) + step) % step;
    return { step, x: mod(ox), y: mod(oy) };
  });
  const openCount = $derived(nodes.filter((n) => n.life !== 'closed').length);
  // La lente: donde pasa el cursor, el mapa se agranda como el dock de macOS.
  // Las burbujas cercanas crecen de forma progresiva —más cuanto más cerca— y
  // se abren hacia afuera con un ojo de pez (Sarkar–Brown): una burbuja a
  // distancia relativa x del cursor pasa a g(x) = (k+1)·x / (k·x+1). Lo cercano
  // se separa mucho y en el borde de la lente nada se mueve, así que no hay
  // saltos. Sus títulos crecen con ellas. Es lectura: ninguna posición cambia.
  const LENS_PX = 150; // radio de la lente en pantalla
  const LENS_K = 2.2; // cuánto separa lo cercano
  let lens = $state<Point | null>(null); // el cursor, en el mundo
  function nodeRadius() {
    return kind === 'bubbles' ? Math.max(15, 9 / camera.scale) : Math.max(7, 4.5 / camera.scale);
  }
  // Bajo el cursor: al menos 2.2×, y lo que haga falta para ~34 px de diámetro.
  const peak = $derived(Math.max(2.2, 17 / (nodeRadius() * camera.scale)));
  // La lente sólo existe dentro de un blob (su círculo más un margen): en el
  // fondo vacío no se agranda nada. Y agranda sólo ese blob.
  const BLOB_MARGIN_PX = 30;
  const lensBlob = $derived.by(() => {
    if (!lens) return null;
    let best: string | null = null, bestD = Infinity;
    for (const [id, g] of layout.groups) {
      const d = Math.hypot(g.x - lens.x, g.y - lens.y);
      if (d <= g.radius + BLOB_MARGIN_PX / camera.scale && d < bestD) { best = id; bestD = d; }
    }
    return best;
  });
  const fish = $derived.by(() => {
    const out = new Map<string, { x: number; y: number; s: number }>();
    if (!lens || lensBlob === null) return out;
    for (const n of nodes) {
      if (n.group !== lensBlob) continue;
      const q = layout.positions.get(n.id);
      if (!q) continue;
      const dx = (q.x - lens.x) * camera.scale, dy = (q.y - lens.y) * camera.scale;
      const d = Math.hypot(dx, dy);
      const x = d / LENS_PX;
      if (x >= 1) continue;
      const moved = d === 0 ? 0 : LENS_PX * ((LENS_K + 1) * x) / (LENS_K * x + 1) - d;
      const near = (1 - x) * (1 - x);
      out.set(n.id, {
        x: d === 0 ? 0 : (dx / d) * moved / camera.scale,
        y: d === 0 ? 0 : (dy / d) * moved / camera.scale,
        s: 1 + (peak - 1) * near,
      });
    }
    return out;
  });
  const shifted = (id: string) => {
    const p = point(id), f = fish.get(id);
    return f ? { x: p.x + f.x, y: p.y + f.y } : p;
  };
  // El cursor se lee una vez por cuadro, no en cada evento.
  let lensFrame = 0, lensAt: { x: number; y: number } | null = null;
  function aim(event: PointerEvent) {
    lensAt = { x: event.clientX, y: event.clientY };
    if (lensFrame) return;
    lensFrame = requestAnimationFrame(() => {
      lensFrame = 0;
      if (!lensAt || gesture || !svg) return;
      const rect = svg.getBoundingClientRect();
      lens = {
        x: camera.x + (lensAt.x - rect.left - rect.width / 2) / camera.scale,
        y: camera.y + (lensAt.y - rect.top - rect.height / 2) / camera.scale,
      };
    });
  }
  function release() {
    lensAt = null;
    lens = null;
    if (!gesture) hover = '';
  }
  // El orden del DOM no cambia con el hover: reordenar movía el nodo entre
  // pointerdown y pointerup y el navegador perdía el click. No hace falta
  // subirlo: el empuje aparta a las vecinas más allá de la esfera crecida.
  function zoom(factor: number, dx = 0, dy = 0) { camera = zoomCamera(camera, factor, dx, dy); }
  function wheel(event: WheelEvent) {
    event.preventDefault();
    const rect = svg.getBoundingClientRect();
    const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? rect.height : 1);
    zoom(Math.exp(-Math.max(-200, Math.min(200, delta)) * 0.002), event.clientX - rect.left - rect.width / 2, event.clientY - rect.top - rect.height / 2);
  }
  // El hover lo decide la cercanía, no el círculo exacto: el punto más cercano
  // al cursor dentro de HOVER_PX en pantalla, a cualquier zoom. El que ya está
  // en hover lo conserva hasta HOLD_PX, para no saltar entre vecinos. Se mide
  // contra la disposición, no contra lo empujado: si no, apartar a las vecinas
  // cambiaría quién está más cerca.
  const HOVER_PX = 24;
  const HOLD_PX = 36;
  function nearest(event: PointerEvent | MouseEvent) {
    const rect = svg.getBoundingClientRect();
    const wx = camera.x + (event.clientX - rect.left - rect.width / 2) / camera.scale;
    const wy = camera.y + (event.clientY - rect.top - rect.height / 2) / camera.scale;
    let best = '', bestD = Infinity, current = Infinity;
    for (const n of nodes) {
      const p = layout.positions.get(n.id);
      if (!p) continue;
      const d = Math.hypot(p.x - wx, p.y - wy) * camera.scale;
      if (d < bestD) { bestD = d; best = n.id; }
      if (n.id === hover) current = d;
    }
    if (hover && current <= HOLD_PX) return hover;
    return bestD <= HOVER_PX ? best : '';
  }
  function down(event: PointerEvent) {
    if (event.button !== 0 || gesture) return;
    clickCandidate = undefined;
    const target = (event.target as Element).closest('[data-piece]');
    // Se abre el que se ve resaltado (el más cercano), aunque bajo el cursor haya
    // quedado una vecina empujada; presionar cerca de un punto es presionarlo.
    const id = nearest(event) || target?.getAttribute('data-piece') || undefined;
    const onRing = !!(event.target as Element).closest('[data-fury]');
    gesture = { pointer: event.pointerId, id, fury: onRing, x: event.clientX, y: event.clientY, origin: { x: camera.x, y: camera.y }, moved: false };
    svg.setPointerCapture(event.pointerId);
  }
  function move(event: PointerEvent) {
    if (!gesture) {
      if (event.pointerType !== 'touch') { hover = nearest(event); aim(event); }
      return;
    }
    if (gesture.pointer !== event.pointerId) return;
    const dx = event.clientX - gesture.x, dy = event.clientY - gesture.y;
    if (Math.hypot(dx, dy) > 4) { gesture.moved = true; grabbing = true; lens = null; }
    if (!gesture.moved) return;
    camera = { ...camera, x: gesture.origin.x - dx / camera.scale, y: gesture.origin.y - dy / camera.scale };
  }
  function up(event: PointerEvent) {
    if (!gesture || gesture.pointer !== event.pointerId) return;
    const current = gesture;
    gesture = undefined;
    grabbing = false;
    // Durante el gesto el SVG tiene el puntero capturado y el nodo recibe
    // `pointerleave`: si eso borrara el hover, el nodo cambiaría de orden en el
    // DOM entre pointerdown y pointerup y el navegador no daría el click. Por
    // eso el hover se congela mientras dura el gesto, y un arrastre lo suelta.
    if (current.moved) hover = '';
    if (svg.hasPointerCapture(event.pointerId)) svg.releasePointerCapture(event.pointerId);
    clickCandidate = event.type === 'pointerup' && !current.moved && current.id ? { id: current.id, fury: current.fury } : undefined;
  }
  function click() {
    const hit = clickCandidate;
    clickCandidate = undefined;
    const node = nodes.find((n) => n.id === hit?.id);
    if (node) (hit!.fury ? fury : onopen)(node);
  }
  function keyboard(event: KeyboardEvent, node: N) {
    if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onopen(node); }
  }
  function path(edge: GraphLink) {
    const a = shifted(edge.source), b = shifted(edge.target);
    const distance = Math.hypot(b.x - a.x, b.y - a.y) || 1;
    const end = edge.kind === 'dependency' ? { x: b.x - (b.x - a.x) / distance * 17, y: b.y - (b.y - a.y) / distance * 17 } : b;
    const curve = edge.kind === 'related' ? 18 : -10;
    return `M ${a.x} ${a.y} Q ${(a.x + b.x) / 2 - (b.y - a.y) / distance * curve} ${(a.y + b.y) / 2 + (b.x - a.x) / distance * curve} ${end.x} ${end.y}`;
  }
</script>

<div class="graph" class:canvas bind:this={wrapper} data-map-kind={kind} style:--graph-height={height}
  style:--gs={`${grid.step}px`} style:--gx={`${grid.x}px`} style:--gy={`${grid.y}px`}>
  {#if canvas}
    <!-- Arriba, como en la secuencia: el zoom en el borde izquierdo y el título
         al centro. -->
    <div class="topbar">
      <div class="ctrl" aria-label="Controles del mapa">
        <button aria-label="Alejar mapa" title="Alejar" onclick={() => zoom(1 / 1.25)}>−</button>
        <span aria-live="polite">{Math.round(camera.scale * 100)}%</span>
        <button aria-label="Acercar mapa" title="Acercar" onclick={() => zoom(1.25)}>+</button>
        <button aria-label="Encuadrar" title="Ver todo" onclick={fit}>⤢</button>
      </div>
      {#if title}
        <header class="head">
          <span class="pane-name display">{title}</span>
          <span class="count" title={kind === 'bubbles' ? 'burbujas abiertas' : 'hilos abiertos'}>{openCount}</span>
        </header>
      {/if}
    </div>
  {:else}
    <div class="graph-tools" aria-label="Controles del mapa">
      <button class="btn btn-sm preset-tonal-surface" aria-label="Alejar mapa" onclick={() => zoom(1 / 1.25)}>−</button>
      <span aria-live="polite">{Math.round(camera.scale * 100)}%</span>
      <button class="btn btn-sm preset-tonal-surface" aria-label="Acercar mapa" onclick={() => zoom(1.25)}>+</button>
      <button class="btn btn-sm preset-tonal-surface" onclick={fit}>Encuadrar</button>
    </div>
  {/if}
  <svg bind:this={svg} role="group" aria-label="Mapa por tags" viewBox={`${camera.x - size.width / camera.scale / 2} ${camera.y - size.height / camera.scale / 2} ${size.width / camera.scale} ${size.height / camera.scale}`}
    onwheel={wheel} onpointerdown={down} onpointermove={move} onpointerleave={release} onpointerup={up} onpointercancel={up} onlostpointercapture={() => { gesture = undefined; grabbing = false; }}
    data-cursor={grabbing ? 'grabbing' : 'grab'}>
    <defs>
      <radialGradient id={`${prefix}-halo`}><stop offset="0" stop-color="var(--color-primary-500)" stop-opacity="0.055" /><stop offset="1" stop-color="var(--color-primary-500)" stop-opacity="0" /></radialGradient>
      <!-- Las esferas de Flotabilidad (Orb.svelte), en SVG: el mismo degradado de
           150°, el oscurecido de abajo a la derecha, el volumen del borde y el
           brillo de arriba, con los colores de cada banda. -->
      {#each [...spheres.values()] as { i, s: sp } (sp.key)}
        <linearGradient id={`${prefix}-lin-${i}`} x1="0.25" y1="0.07" x2="0.75" y2="0.93">
          <stop offset="0.08" style={`stop-color:${sp.g1}`} /><stop offset="0.52" style={`stop-color:${sp.g2}`} /><stop offset="0.96" style={`stop-color:${sp.g3}`} />
        </linearGradient>
        <radialGradient id={`${prefix}-low-${i}`} cx="0.7" cy="0.78" r="0.6">
          <stop offset="0" style={`stop-color:${sp.g3}`} /><stop offset="1" style={`stop-color:${sp.g3};stop-opacity:0`} />
        </radialGradient>
      {/each}
      <radialGradient id={`${prefix}-shade`} cx="0.4" cy="0.36" r="0.68">
        <stop offset="0.6" stop-color="black" stop-opacity="0" /><stop offset="1" stop-color="black" stop-opacity="0.32" />
      </radialGradient>
      <radialGradient id={`${prefix}-sheen`} cx="0.4" cy="0.36" r="0.6">
        <stop offset="0" stop-color="white" stop-opacity="0.95" /><stop offset="1" stop-color="white" stop-opacity="0" />
      </radialGradient>
      <marker id={`${prefix}-arrow`} viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 1 1 L 9 5 L 1 9" fill="none" stroke="var(--faint)" /></marker>
    </defs>
    <g class="edges" aria-hidden="true">
      {#each links as edge (`${edge.source}/${edge.target}/${edge.kind}`)}
        <path class:related={edge.kind === 'related'} class:faded={!!looks && !(looks.get(edge.source)?.match && looks.get(edge.target)?.match)} d={path(edge)} marker-end={edge.kind === 'dependency' ? `url(#${prefix}-arrow)` : undefined} data-link={`${edge.source}/${edge.target}`}><title>{edge.kind === 'dependency' ? 'Dependencia' : 'Relación'} · {edge.count} {edge.count === 1 ? 'vínculo' : 'vínculos'}</title></path>
      {/each}
    </g>
    {#each groups as group (group.id)}
      <g data-group={group.id || 'none'} role="group" aria-label={group.name}>
        <circle cx={group.anchor.x} cy={group.anchor.y} r={group.anchor.radius + 80} fill={`url(#${prefix}-halo)`} pointer-events="none" />
        {#each group.members as node (node.id)}
          {@const p = point(node.id)}
          {@const look = lookOf(node)}
          {@const radius = nodeRadius() * look.size}
          {@const f = fish.get(node.id)}
          {@const k = f?.s ?? 1}
          {@const isHover = hover === node.id}
          <g role="group" aria-label={node.name} class="node band-{node.life}" class:emphasized={hover === node.id} class:faded={!look.match} data-piece={node.id} transform={`translate(${p.x} ${p.y})`}
>
            <g class="shift" style:transform={f ? `translate(${f.x}px, ${f.y}px)` : undefined}>
            <circle class="hit" r={radius + 8} role="button" tabindex="0" aria-label={`Abrir ${node.name}`} onfocus={() => focused = node.id} onblur={() => { if (focused === node.id) focused = ''; }} onkeydown={(e) => keyboard(e, node)} onclick={(e) => { if (!e.detail) onopen(node); }}>
              <title>{node.name} · {node.bubbleName ? `${node.bubbleName} · ` : ''}{node.project} · {bandName(node.life)}{showFury ? ` · ${levels[node.level]}` : ''}</title>
            </circle>
            <!-- La lente la agranda; su sitio en la disposición no cambia. -->
            <g class="ball" pointer-events="none" style:transform={k > 1.001 ? `scale(${k})` : undefined}>
              <circle class="base" r={radius} fill={`url(#${prefix}-lin-${sphereId(node)})`} />
              <circle r={radius} fill={`url(#${prefix}-low-${sphereId(node)})`} />
              <circle r={radius} fill={`url(#${prefix}-shade)`} />
              <ellipse cx={-radius * 0.26} cy={-radius * 0.34} rx={radius * 0.44} ry={radius * 0.32} fill={`url(#${prefix}-sheen)`} />
            </g>
            {#if showFury && node.level !== 'calm'}
              <g data-fury role="button" tabindex="0" aria-label={`Furia de ${node.name}: ${levels[node.level]}`} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); fury(node); } }} onclick={(e) => { if (!e.detail) fury(node); }}>
                <title>{levels[node.level]}: ver compromiso y estancamiento</title>
                <circle class="fury {node.level}" r={radius + 5} />
              </g>
            {/if}
            </g>
          </g>
        {/each}
      </g>
    {/each}
    <!-- Los nombres, en su capa, encima de todas las esferas: sólo el de la
         burbuja bajo el cursor (16 px, fijo) o con el foco del teclado. -->
    <g class="labels" aria-hidden="true">
      <!-- Los títulos de los blobs también van arriba: un blob que se infla con
           la lente no tapa su propio nombre. -->
      {#each groups as group (group.id)}
        <text class="group-name" style={`font-size:${groupFontSize(camera.scale)}px`} x={group.anchor.x} y={group.anchor.y - group.anchor.radius - 38 / camera.scale} text-anchor="middle">{group.name}</text>
        <text class="group-meta" style={`font-size:${12 / camera.scale}px`} x={group.anchor.x} y={group.anchor.y - group.anchor.radius - 18 / camera.scale} text-anchor="middle">{group.open} {kind === 'bubbles' ? (group.open === 1 ? 'abierta' : 'abiertas') : (group.open === 1 ? 'abierto' : 'abiertos')}{showFury ? ` · máx. ${levels[group.max]}` : ''}</text>
      {/each}
      {#each groups as group (group.id)}
        {#each group.members as node (node.id)}
          {@const p = point(node.id)}
          {@const f = fish.get(node.id)}
          {@const k = f?.s ?? 1}
          {@const isHover = hover === node.id}
          <!-- Sólo el nombre de la burbuja bajo el cursor (o con el foco): los
               demás no se dibujan. -->
          {#if focused === node.id || isHover}
          <g class="label-slot" transform={`translate(${p.x} ${p.y})`}>
            <text class="node-label shown" class:hovered={isHover}
              style:transform={f ? `translate(${f.x}px, ${f.y}px)` : undefined}
              style={`font-size:${(isHover ? 16 : 11) / camera.scale}px`}
              y={nodeRadius() * lookOf(node).size * k + (isHover ? 22 : 17) / camera.scale} text-anchor="middle">{node.name}</text>
          </g>
          {/if}
        {/each}
      {/each}
    </g>
  </svg>
  {#if empty && !nodes.length}
    <!-- En el origen del mapa, no pegado a la pantalla: se mueve y escala con
         él, como en Mi espacio y en la secuencia. -->
    <div class="empty" style:transform={`translate(${size.width / 2 - camera.x * camera.scale}px, ${size.height / 2 - camera.y * camera.scale}px) translate(-50%, -50%) scale(${camera.scale})`}>
      <Icon of="bubble" size="3.5rem" tint />
      <p>{empty}</p>
    </div>
  {/if}
  <div class="graph-legend"><span>● Color: calor{showFury ? ' · aro: furia' : ''}</span><span>→ Dependencia <i aria-hidden="true">┄</i> Relación</span><span>Arrastra para mover · rueda para acercar</span></div>
</div>

<style>
  .graph { position: relative; width: 100%; height: var(--graph-height); overflow: hidden; isolation: isolate; overscroll-behavior: contain; }
  /* El cursor lo pone `data-cursor` con la familia de app.css, como en la secuencia. */
  svg { width: 100%; height: 100%; display: block; touch-action: none; }
  .graph-tools { position: absolute; top: 0.5rem; right: 0; display: flex; align-items: center; gap: 0.45rem; z-index: 2; }
  .graph-tools span { color: var(--faint); font-size: 0.7rem; min-width: 2.4rem; text-align: center; }
  .edges path { stroke: var(--faint); stroke-width: 1; opacity: 0.3; fill: none; vector-effect: non-scaling-stroke; }
  .edges path.faded, .edges path.related.faded { opacity: 0.06; }
  .node.faded { opacity: 0.16; }
  .edges path.related { stroke-dasharray: 3 5; opacity: 0.22; }
  .group-name { fill: var(--text); font-family: var(--font-display); font-size: 21px; pointer-events: none; paint-order: stroke; stroke: var(--bg); stroke-width: 5px; stroke-linejoin: round; }
  .group-meta { fill: var(--faint); font-size: 9px; pointer-events: none; paint-order: stroke; stroke: var(--bg); stroke-width: 4px; stroke-linejoin: round; }
  .hit { fill: transparent; }
  .fury { fill: none; stroke-width: 1.8; stroke: var(--color-warning-500); }
  .fury.angry { stroke: light-dark(#c35c28, #fca073); }
  .fury.furious { stroke: light-dark(#bf3556, #fc849f); stroke-width: 2.4; }
  .node-label { fill: var(--text); font-size: 10px; opacity: 0; pointer-events: none; paint-order: stroke; stroke: var(--bg); stroke-width: 4px; stroke-linejoin: round; }
  .node-label.shown { opacity: 1; }
  .labels { pointer-events: none; }
  .ball {
    transform-box: fill-box; transform-origin: center;
    transition: transform 0.32s cubic-bezier(0.34, 1.56, 0.4, 1), filter 0.32s ease;
  }
  /* La lente sigue al cursor: una transición corta la suaviza sin arrastrarla. */
  .shift, .ball { transition: transform 0.16s ease-out; }
  .node-label { transition: transform 0.16s ease-out; }
  .node.emphasized .ball { filter: brightness(1.1) saturate(1.06); }
  /* Con teclado no hay cursor: el foco agranda la suya. */
  .node:has(.hit:focus-visible) .ball { transform: scale(2); }
  .node-label.hovered { font-weight: 700; }
  @media (prefers-reduced-motion: reduce) { .ball, .shift, .node-label { transition: none; } }
  .node [role='button']:focus-visible { outline: none; }
  .node .hit:focus-visible ~ .ball .base { stroke: var(--text); stroke-width: 2; }
  .node [role='button']:focus-visible .fury { stroke: var(--text); stroke-width: 3; }
  .graph-legend { position: absolute; bottom: 0.3rem; left: 0; right: 0; display: flex; justify-content: space-between; flex-wrap: wrap; gap: 0.3rem 1rem; color: var(--faint); font-size: 0.68rem; pointer-events: none; }
  .graph-legend i { margin: 0 0.4rem; font-style: normal; }

  /* El lienzo: llena su contenedor y lleva el fondo de la secuencia. */
  .graph.canvas {
    height: 100%;
    background-color: color-mix(in oklab, var(--surface-solid) 55%, transparent);
    background-image: radial-gradient(circle, color-mix(in oklab, var(--text) 22%, transparent) 1.1px, transparent 1.4px);
    background-size: var(--gs) var(--gs);
    background-position: var(--gx) var(--gy);
  }
  .topbar {
    position: absolute; z-index: 2; top: 1rem; left: 1rem; right: 1rem;
    display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 0.6rem;
    pointer-events: none;
  }
  .topbar > * { pointer-events: auto; }
  .ctrl {
    grid-column: 1; justify-self: start;
    display: flex; align-items: center; gap: 0.2rem; padding: 0.25rem;
    border: 1px solid var(--line); border-radius: 999px; background: var(--surface-solid);
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.16); font-size: 0.8rem;
  }
  .ctrl button { width: 26px; height: 26px; border: 0; border-radius: 999px; background: none; color: var(--text); cursor: pointer; }
  .ctrl button:hover { background: var(--hover); }
  .ctrl button:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .ctrl span { min-width: 2.8rem; text-align: center; color: var(--muted); }
  .head {
    grid-column: 2; display: flex; align-items: center; gap: 0.5rem; min-height: 42px; max-width: 100%;
    padding: 0.5rem 0.9rem; border: 1px solid var(--line); border-radius: 26px;
    background: var(--surface-solid); box-shadow: 0 4px 16px rgb(0 0 0 / 0.16);
  }
  .pane-name { font-size: 1.5rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .count { padding: 0 0.45rem; border-radius: 999px; background: var(--hover); font-size: 0.75rem; }
  /* Lo vacío, como en la secuencia: fijo al centro del lienzo, sin estorbar el arrastre. */
  .empty { position: absolute; left: 0; top: 0; z-index: 1; width: 22rem; transform-origin: 50% 50%; display: grid; justify-items: center; gap: 0.7rem; color: var(--muted); text-align: center; pointer-events: none; }
  .empty p { max-width: 22rem; }
  /* La leyenda, justo encima de la franja del HUD (cerebro, controles, lupa). */
  /* Sobre el cerebro, que con su insignia es lo más alto de la franja. */
  .graph.canvas .graph-legend { left: 1rem; right: 1rem; bottom: calc(1rem + 56px + 0.5rem); }
  @media (max-width: 1150px) { .graph.canvas .graph-legend { bottom: calc(4.75rem + 42px + 0.6rem); } }
  @media (max-width: 650px) {
    .graph:not(.canvas) { height: 34rem; }
    .pane-name { font-size: 1.15rem; }
    /* Sin ancho para tres columnas: el zoom en su fila y el título debajo. */
    .topbar { display: flex; flex-direction: column; align-items: center; }
    .topbar > .ctrl { align-self: flex-start; }
    .head { max-width: 100%; min-width: 0; }
    .graph-tools { gap: 0.3rem; }
    .graph-legend { font-size: 0.61rem; }
    .graph-legend span:last-child { display: none; }
  }
</style>
