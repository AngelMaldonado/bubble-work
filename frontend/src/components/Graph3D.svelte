<script lang="ts" generics="N extends GraphNode">
  // La vista 3D de los blobs, para leer las conexiones. Un <canvas> con
  // proyección en perspectiva hecha a mano (lib/graph3d), sin dependencias:
  // cada blob es una esfera de puntos y sus centros, otra esfera mayor.
  //
  // Arrastrar rota, la rueda acerca, «Ver todo» encuadra. Gira despacio sola
  // hasta que la tocas (no, si el sistema pide menos movimiento). Con el
  // cursor sobre una burbuja se iluminan sus aristas y sus vecinas; sobre un
  // blob, las aristas que entran y salen de él. Un clic abre, como en Blobs.
  //
  // Un canvas no se navega con teclado: para eso está la vista Blobs, y la
  // etiqueta del lienzo lo dice.
  import { onMount, untrack } from 'svelte';
  import Icon from './Icon.svelte';
  import { layout3d, project, fitOrbit, rotate, AXIS_VIEWS, nearestYaw, type Orbit, type Projected } from '../lib/graph3d';
  import type { GraphGroup, GraphLink, GraphNode, MapKind } from '../lib/graph';
  import { STATE_SPHERE, type Look, type Sphere } from '../lib/encoding';
  import { api } from '../lib/api';

  let { nodes, universe, groups, links, kind, title = '', empty = '', resetKey, looks, onopen }: {
    nodes: N[]; universe: N[]; groups: GraphGroup[]; links: GraphLink[]; kind: MapKind;
    title?: string; empty?: string; resetKey: string; looks?: Map<string, Look>; onopen: (node: N) => void;
  } = $props();

  let wrap: HTMLDivElement;
  let cv: HTMLCanvasElement;
  let size = $state({ w: 1000, h: 600 });
  const layout = $derived(layout3d(universe, groups));
  const focal = $derived(Math.min(size.w, size.h) * 1.1);
  let orbit = $state<Orbit>({ yaw: 0.6, pitch: -0.32, dist: 1200 });
  // Preferencias de la vista 3D, por persona en este navegador: invertir el
  // arrastre y si gira sola. Se leen sin confiar en lo guardado.
  const prefKey = () => `bubble.board.3d.${api.me?.id ?? 'local'}`;
  // `spin`: 'auto' gira sola desde que se abre la vista y se pausa PAUSE_MS tras
  // cada interacción; 'off', nunca.
  // Lo guardado antes como booleano se lee: verdadero era «gira sola».
  const saved = (() => {
    try {
      const v = JSON.parse(localStorage.getItem(prefKey()) ?? '{}');
      return { invert: v.invert === true, spin: (v.spin === 'off' || v.spin === false ? 'off' : 'auto') as 'auto' | 'off' };
    } catch { return { invert: false, spin: 'auto' as const }; }
  })();
  let invert = $state(saved.invert);
  let spinMode = $state<'auto' | 'off'>(saved.spin);
  let spinning = $state(false);
  $effect(() => { const v = JSON.stringify({ invert, spin: spinMode }); try { localStorage.setItem(prefKey(), v); } catch { /* dura la pestaña */ } });
  // Automático: gira desde que se abre la vista (al cargar, al volver desde
  // otra disposición o nivel, que la montan de nuevo, y al volver a la pestaña
  // del navegador). Arrastrar la pausa PAUSE_MS; el hover, mientras dura y
  // HOVER_RESUME_MS después.
  const PAUSE_MS = 10_000;
  let idle: ReturnType<typeof setTimeout> | undefined;
  function spin() {
    clearTimeout(idle);
    spinning = spinMode === 'auto' && !reduced;
    if (spinning) kick();
  }
  function touch() {
    spinning = false;
    clearTimeout(idle);
    if (spinMode === 'auto' && !reduced) idle = setTimeout(spin, PAUSE_MS);
  }
  function toggleSpin() {
    spinMode = spinMode === 'auto' ? 'off' : 'auto';
    spin();
  }
  let hover = $state('');
  let hoverBlob = $state<string | null>(null);
  let grabbing = $state(false);
  const reduced = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? (id || 'Sin tags');
  const lookOf = (n: N): Look => looks?.get(n.id) ?? { size: 1, sphere: STATE_SPHERE[n.life] ?? STATE_SPHERE.hot, match: true };

  function fit() {
    orbit = { ...orbit, dist: fitOrbit(layout.extent, Math.min(size.w, size.h - 160), focal) };
  }
  // Encuadra al cambiar de alcance o de tamaño; `fit` lee y escribe `orbit`.
  $effect(() => { void resetKey; void size.w; void size.h; untrack(fit); });

  // Una esfera pre-dibujada por color (los degradados de Orb.svelte), para que
  // cada cuadro sólo copie imágenes.
  const sprites = new Map<string, HTMLCanvasElement>();
  function sprite(s: Sphere) {
    let c = sprites.get(s.key);
    if (c) return c;
    c = document.createElement('canvas');
    c.width = c.height = 96;
    const g = c.getContext('2d')!, R = 46, m = 48;
    const lin = g.createLinearGradient(m - R * 0.5, m - R, m + R * 0.5, m + R);
    lin.addColorStop(0.08, s.g1); lin.addColorStop(0.52, s.g2); lin.addColorStop(0.96, s.g3);
    g.fillStyle = lin; g.beginPath(); g.arc(m, m, R, 0, Math.PI * 2); g.fill();
    const low = g.createRadialGradient(m + R * 0.4, m + R * 0.56, 0, m + R * 0.4, m + R * 0.56, R * 1.2);
    low.addColorStop(0, s.g3); low.addColorStop(1, 'transparent');
    g.fillStyle = low; g.beginPath(); g.arc(m, m, R, 0, Math.PI * 2); g.fill();
    const shade = g.createRadialGradient(m - R * 0.2, m - R * 0.28, R * 0.6, m - R * 0.2, m - R * 0.28, R * 1.4);
    shade.addColorStop(0, 'rgba(0,0,0,0)'); shade.addColorStop(1, 'rgba(0,0,0,0.34)');
    g.fillStyle = shade; g.beginPath(); g.arc(m, m, R, 0, Math.PI * 2); g.fill();
    const sheen = g.createRadialGradient(m - R * 0.28, m - R * 0.36, 0, m - R * 0.28, m - R * 0.36, R * 0.5);
    sheen.addColorStop(0, 'rgba(255,255,255,0.95)'); sheen.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = sheen; g.beginPath(); g.ellipse(m - R * 0.26, m - R * 0.34, R * 0.44, R * 0.32, 0, 0, Math.PI * 2); g.fill();
    sprites.set(s.key, c);
    return c;
  }

  // Lo de un cuadro: posiciones proyectadas, para dibujar y para el hover.
  let shot: { id: string; p: Projected; r: number }[] = [];
  const shown = $derived(new Set(nodes.map((n) => n.id)));
  const byId = $derived(new Map(nodes.map((n) => [n.id, n])));
  const neighbors = $derived.by(() => {
    const m = new Map<string, Set<string>>();
    for (const l of links) {
      if (!m.has(l.source)) m.set(l.source, new Set());
      if (!m.has(l.target)) m.set(l.target, new Set());
      m.get(l.source)!.add(l.target); m.get(l.target)!.add(l.source);
    }
    return m;
  });

  // Los colores del tema, leídos una vez y otra sólo si cambia el tema: leer
  // estilos en cada cuadro del giro obligaba a recalcularlos 60 veces por segundo.
  let palette: { text: string; faint: string; bg: string; accent: string } | null = null;
  function readPalette() {
    const st = getComputedStyle(wrap), v = (n: string, f: string) => st.getPropertyValue(n).trim() || f;
    palette = { text: v('--text', '#e5e7eb'), faint: v('--faint', '#94a3b8'), bg: v('--bg', '#0b0f17'), accent: v('--accent', '#22d3ee') };
  }

  const FLOOR_LINES = 8; // por lado, desde el centro
  const FLOOR_STEPS = 16; // tramos por línea: la perspectiva curva y recorta
  function floor(g: CanvasRenderingContext2D, cx: number, cy: number, faint: string, accent: string) {
    const E = layout.extent, y = E * 1.08, L = E * 1.6, step = L / FLOOR_LINES;
    const seg = (ax: number, az: number, bx: number, bz: number, axis: boolean) => {
      for (let i = 0; i < FLOOR_STEPS; i++) {
        const t0 = i / FLOOR_STEPS, t1 = (i + 1) / FLOOR_STEPS;
        const x0 = ax + (bx - ax) * t0, z0 = az + (bz - az) * t0, x1 = ax + (bx - ax) * t1, z1 = az + (bz - az) * t1;
        const a = project({ x: x0, y, z: z0 }, orbit, focal, cx, cy), b = project({ x: x1, y, z: z1 }, orbit, focal, cx, cy);
        if (!a || !b) continue;
        // Un disco, no un cuadrado: se apaga con la distancia al centro.
        const r = Math.hypot((x0 + x1) / 2, (z0 + z1) / 2) / L;
        if (r >= 1) continue;
        g.globalAlpha = (axis ? 0.5 : 0.22) * (1 - r) * (1 - r);
        g.beginPath(); g.moveTo(a.x, a.y); g.lineTo(b.x, b.y); g.stroke();
      }
    };
    g.setLineDash([]);
    g.lineWidth = 1;
    for (let k = -FLOOR_LINES; k <= FLOOR_LINES; k++) {
      const axis = k === 0;
      g.strokeStyle = axis ? accent : faint;
      seg(k * step, -L, k * step, L, axis);
      seg(-L, k * step, L, k * step, axis);
    }
    g.globalAlpha = 1;
  }

  function draw() {
    if (!cv) return;
    const dpr = devicePixelRatio || 1;
    const g = cv.getContext('2d')!;
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.clearRect(0, 0, size.w, size.h);
    const cx = size.w / 2, cy = size.h / 2 + 10;
    if (!palette) readPalette();
    const { text, faint, bg, accent } = palette!;
    // Tamaño de esfera relativo a la figura: se lee igual con 20 puntos que con 400.
    const base = layout.extent * (kind === 'bubbles' ? 0.026 : 0.014);
    // El piso: una retícula en un plano horizontal bajo la figura, que gira con
    // ella y se desvanece hacia el borde, con los dos ejes centrales marcados.
    // Va primero, así queda debajo de todo.
    floor(g, cx, cy, faint, accent);
    shot = [];
    const at = new Map<string, Projected>();
    for (const n of nodes) {
      const q = layout.positions.get(n.id);
      if (!q) continue;
      const p = project(q, orbit, focal, cx, cy);
      if (!p) continue;
      at.set(n.id, p);
      shot.push({ id: n.id, p, r: Math.max(3, base * lookOf(n).size * p.scale) });
    }
    const near = hover ? neighbors.get(hover) ?? new Set<string>() : null;
    const touches = (id: string) => (hover ? id === hover || near!.has(id) : hoverBlob !== null ? byId.get(id)?.group === hoverBlob : true);
    // Aristas: lo de atrás más tenue; con foco, lo que lo toca se ilumina.
    const depths = shot.map((s) => s.p.depth);
    const dMin = Math.min(...depths), dMax = Math.max(...depths);
    const fade = (d: number) => (dMax === dMin ? 1 : 0.35 + 0.65 * (d - dMin) / (dMax - dMin));
    for (const l of links) {
      const a = at.get(l.source), b = at.get(l.target);
      if (!a || !b) continue;
      const lit = hover ? l.source === hover || l.target === hover : hoverBlob !== null ? byId.get(l.source)?.group === hoverBlob || byId.get(l.target)?.group === hoverBlob : false;
      const focusOn = hover || hoverBlob !== null;
      const okLook = lookOf(byId.get(l.source)!).match && lookOf(byId.get(l.target)!).match;
      g.globalAlpha = (lit ? 0.95 : focusOn ? 0.05 : 0.28) * fade((a.depth + b.depth) / 2) * (okLook ? 1 : 0.25);
      g.strokeStyle = lit ? accent : faint;
      g.lineWidth = lit ? 1.6 : 1;
      if (l.kind === 'related') g.setLineDash([3, 5]); else g.setLineDash([]);
      g.beginPath(); g.moveTo(a.x, a.y); g.lineTo(b.x, b.y); g.stroke();
      if (l.kind === 'dependency') {
        const ang = Math.atan2(b.y - a.y, b.x - a.x), tip = 7 + 4 * b.scale;
        const ex = b.x - Math.cos(ang) * (base * b.scale + 2), ey = b.y - Math.sin(ang) * (base * b.scale + 2);
        g.setLineDash([]);
        g.beginPath(); g.moveTo(ex, ey);
        g.lineTo(ex - Math.cos(ang - 0.4) * tip, ey - Math.sin(ang - 0.4) * tip);
        g.moveTo(ex, ey);
        g.lineTo(ex - Math.cos(ang + 0.4) * tip, ey - Math.sin(ang + 0.4) * tip);
        g.stroke();
      }
    }
    g.setLineDash([]);
    // Esferas, de atrás hacia adelante.
    for (const s of [...shot].sort((a, b) => a.p.depth - b.p.depth)) {
      const n = byId.get(s.id)!, look = lookOf(n);
      const dim = (hover || hoverBlob !== null) && !touches(s.id);
      g.globalAlpha = (look.match ? 1 : 0.16) * (dim ? 0.18 : 1) * fade(s.p.depth);
      const r = s.id === hover ? s.r * 1.7 : s.r;
      g.drawImage(sprite(look.sphere), s.p.x - r, s.p.y - r, r * 2, r * 2);
    }
    // Títulos de los blobs, en su centro proyectado, más grandes cuanto más cerca.
    g.globalAlpha = 1;
    g.textAlign = 'center';
    g.lineJoin = 'round';
    for (const [id, b] of layout.blobs) {
      if (!nodes.some((n) => n.group === id)) continue;
      const p = project({ x: b.x, y: b.y - b.radius - 18, z: b.z }, orbit, focal, cx, cy);
      if (!p) continue;
      const px = Math.max(13, Math.min(26, 30 * p.scale));
      g.font = `600 ${px}px var(--font-display), system-ui, sans-serif`;
      g.globalAlpha = (hoverBlob === null && !hover) || hoverBlob === id ? fade(p.depth) : 0.3;
      g.strokeStyle = bg; g.lineWidth = 5; g.strokeText(groupName(id), p.x, p.y);
      g.fillStyle = text; g.fillText(groupName(id), p.x, p.y);
    }
    // El nombre de la burbuja bajo el cursor, y nada más.
    const h = hover ? shot.find((s) => s.id === hover) : undefined;
    if (h) {
      g.globalAlpha = 1;
      g.font = `700 16px system-ui, sans-serif`;
      const label = byId.get(h.id)!.name;
      g.strokeStyle = bg; g.lineWidth = 5; g.strokeText(label, h.p.x, h.p.y + h.r * 1.7 + 20);
      g.fillStyle = text; g.fillText(label, h.p.x, h.p.y + h.r * 1.7 + 20);
    }
    g.globalAlpha = 1;
  }

  // Un cuadro cuando algo cambió; seguido sólo mientras gira sola.
  let frame = 0;
  function kick() { if (!frame) frame = requestAnimationFrame(tick); }
  // El hover pausa el giro; al quitarlo, sigue HOVER_RESUME_MS después.
  const HOVER_RESUME_MS = 300;
  let hoverRelease = 0, releaseTimer: ReturnType<typeof setTimeout> | undefined, wasHovering = false;
  $effect(() => {
    const now = !!hover || hoverBlob !== null;
    if (wasHovering && !now) {
      hoverRelease = performance.now() + HOVER_RESUME_MS;
      clearTimeout(releaseTimer);
      releaseTimer = setTimeout(kick, HOVER_RESUME_MS + 16);
    }
    wasHovering = now;
  });
  function tick() {
    frame = 0;
    const hovering = !!hover || hoverBlob !== null;
    // Un gesto sólo frena el giro cuando ya es arrastre (`moved`, a partir de
    // 4 px): un clic sin moverse no lo detiene, ni mientras el botón está abajo.
    const turning = spinning && !reduced && !gesture?.moved && !hovering && performance.now() >= hoverRelease;
    if (turning) orbit = { ...orbit, yaw: orbit.yaw + 0.0016 };
    draw();
    // Lo que gira puede pasar bajo el cursor quieto: se vuelve a buscar, y si
    // es una burbuja, el giro se pausa como con un hover.
    if (turning && pointerAt) {
      const h = pick(pointerAt);
      if (h.node !== hover || (h.node ? null : h.blob) !== hoverBlob) { hover = h.node; hoverBlob = h.node ? null : h.blob; }
    }
    // Parado (hover, espera o arrastre), no se redibuja en vano: un cambio de
    // estado o el temporizador lo despiertan.
    if (turning) kick();
  }
  $effect(() => { void orbit; void nodes; void links; void looks; void hover; void hoverBlob; void size.w; kick(); });

  onMount(() => {
    const ro = new ResizeObserver(([e]) => {
      size = { w: e.contentRect.width, h: e.contentRect.height };
      const dpr = devicePixelRatio || 1;
      cv.width = Math.round(size.w * dpr); cv.height = Math.round(size.h * dpr);
      kick();
    });
    ro.observe(wrap);
    // Rueda: no pasiva, para que el gesto no desplace la página.
    const wheel = (e: WheelEvent) => {
      e.preventDefault();
      orbit = { ...orbit, dist: Math.max(layout.extent * 0.4, Math.min(layout.extent * 12, orbit.dist * Math.exp(e.deltaY * 0.0015))) };
    };
    cv.addEventListener('wheel', wheel, { passive: false });
    const theme = new MutationObserver(() => { readPalette(); kick(); });
    theme.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'data-mode', 'data-theme', 'style'] });
    spin();
    const visible = () => { if (document.visibilityState === 'visible') spin(); };
    document.addEventListener('visibilitychange', visible);
    return () => { theme.disconnect(); ro.disconnect(); cv.removeEventListener('wheel', wheel); document.removeEventListener('visibilitychange', visible); cancelAnimationFrame(frame); cancelAnimationFrame(tween); clearTimeout(idle); clearTimeout(releaseTimer); frame = 0; };
  });

  // Arrastrar rota; un clic sin moverse abre lo que hay bajo el cursor.
  let gesture: { x: number; y: number; yaw: number; pitch: number; moved: boolean; id: string } | null = null;
  let pointerAt: { clientX: number; clientY: number } | null = null;
  function pick(ev: { clientX: number; clientY: number }): { node: string; blob: string | null } {
    const r = cv.getBoundingClientRect(), x = ev.clientX - r.left, y = ev.clientY - r.top;
    let best = '', bestD = Infinity, bestDepth = -Infinity;
    for (const s of shot) {
      const d = Math.hypot(s.p.x - x, s.p.y - y);
      // El de adelante gana si dos se tocan bajo el cursor.
      if (d <= Math.max(s.r, 10) + 6 && (d < bestD - 4 || (Math.abs(d - bestD) <= 4 && s.p.depth > bestDepth))) { best = s.id; bestD = d; bestDepth = s.p.depth; }
    }
    if (best) return { node: best, blob: byId.get(best)?.group ?? null };
    // Sin punto: ¿dentro de la silueta de algún blob?
    const cx = size.w / 2, cy = size.h / 2 + 10;
    let blob: string | null = null, blobDepth = -Infinity;
    for (const [id, b] of layout.blobs) {
      if (!nodes.some((n) => n.group === id)) continue;
      const p = project(b, orbit, focal, cx, cy);
      if (!p) continue;
      if (Math.hypot(p.x - x, p.y - y) <= Math.max(18, b.radius * p.scale + 14) && p.depth > blobDepth) { blob = id; blobDepth = p.depth; }
    }
    return { node: '', blob };
  }
  function down(ev: PointerEvent) {
    if (ev.button !== 0) return;
    gesture = { x: ev.clientX, y: ev.clientY, yaw: orbit.yaw, pitch: orbit.pitch, moved: false, id: pick(ev).node };
    cv.setPointerCapture(ev.pointerId);
  }
  function move(ev: PointerEvent) {
    if (!gesture) {
      pointerAt = { clientX: ev.clientX, clientY: ev.clientY };
      const h = pick(ev);
      hover = h.node; hoverBlob = h.node ? null : h.blob;
      return;
    }
    const dx = ev.clientX - gesture.x, dy = ev.clientY - gesture.y;
    // Sólo el arrastre pausa el giro (10 s desde que se suelta).
    if (!gesture.moved && Math.hypot(dx, dy) > 4) { gesture.moved = true; grabbing = true; hover = ''; hoverBlob = null; touch(); }
    if (!gesture.moved) return;
    const sign = invert ? -1 : 1;
    orbit = { ...orbit, yaw: gesture.yaw + sign * dx * 0.006, pitch: Math.max(-1.35, Math.min(1.35, gesture.pitch + sign * dy * 0.006)) };
  }
  function up(ev: PointerEvent) {
    const g = gesture;
    gesture = null; grabbing = false;
    if (g?.moved) touch();
    if (cv.hasPointerCapture(ev.pointerId)) cv.releasePointerCapture(ev.pointerId);
    if (g && !g.moved && g.id) { const n = byId.get(g.id); if (n) onopen(n); }
  }
  // El gizmo: los tres ejes, girados como la cámara (sin perspectiva). El de
  // adelante se dibuja encima; el extremo negativo, como un punto tenue. Pulsar
  // un eje lleva la vista a mirar a lo largo de él.
  const GIZMO = 112; // lado en px
  const GIZMO_R = 37; // largo de un eje en px
  const AXES = [
    { key: 'x', label: 'X', color: 'oklch(0.68 0.19 25)', v: { x: 1, y: 0, z: 0 }, view: 'de lado' },
    { key: 'y', label: 'Y', color: 'oklch(0.74 0.17 145)', v: { x: 0, y: -1, z: 0 }, view: 'desde arriba' },
    { key: 'z', label: 'Z', color: 'oklch(0.68 0.16 250)', v: { x: 0, y: 0, z: 1 }, view: 'de frente' },
  ] as const;
  const gizmo = $derived(AXES.map((a) => {
    const r = rotate(a.v, orbit);
    return { ...a, x: r.x * GIZMO_R, y: r.y * GIZMO_R, depth: r.depth };
  }).sort((a, b) => a.depth - b.depth));
  let tween = 0;
  function look(axis: keyof typeof AXIS_VIEWS) {
    cancelAnimationFrame(tween);
    const target = AXIS_VIEWS[axis];
    const from = { yaw: orbit.yaw, pitch: orbit.pitch };
    const to = { yaw: nearestYaw(from.yaw, target.yaw), pitch: target.pitch };
    if (reduced) { orbit = { ...orbit, ...to }; return; }
    const t0 = performance.now(), ms = 320;
    const step = (now: number) => {
      const t = Math.min(1, (now - t0) / ms), e = 1 - (1 - t) ** 3;
      orbit = { ...orbit, yaw: from.yaw + (to.yaw - from.yaw) * e, pitch: from.pitch + (to.pitch - from.pitch) * e };
      if (t < 1) tween = requestAnimationFrame(step);
    };
    tween = requestAnimationFrame(step);
  }
  function zoom(f: number) { orbit = { ...orbit, dist: Math.max(layout.extent * 0.4, Math.min(layout.extent * 12, orbit.dist / f)) }; }
  const label = $derived(`Vista 3D de ${nodes.length} ${kind === 'bubbles' ? 'burbujas' : 'hilos'} en ${groups.length} blobs y ${links.length} conexiones. Para recorrerla con teclado, usa la vista Blobs.`);
</script>

<div class="g3d" bind:this={wrap} data-hover={hover} data-hover-blob={hoverBlob ?? ''}>
  <div class="topbar">
    <div class="ctrl" aria-label="Controles de la vista 3D">
      <button aria-label="Alejar 3D" title="Alejar" onclick={() => zoom(1 / 1.25)}>−</button>
      <button aria-label="Acercar 3D" title="Acercar" onclick={() => zoom(1.25)}>+</button>
      <button aria-label="Encuadrar 3D" title="Ver todo" onclick={fit}>⤢</button>
      <button class:auto={spinMode === 'auto'} aria-label={spinMode === 'auto' ? 'Giro automático: activado' : 'Giro automático: desactivado'} aria-pressed={spinMode === 'auto'}
        title={spinMode === 'auto' ? 'Gira sola; se pausa con el cursor encima y 10 s tras arrastrarla · pulsa para desactivar' : 'Sin giro automático · pulsa para activarlo'} onclick={toggleSpin}>⟳</button>
      <button aria-label={invert ? 'Arrastre invertido: volver al normal' : 'Invertir el arrastre'} aria-pressed={invert} title={invert ? 'Arrastre invertido' : 'Invertir el arrastre'} onclick={() => (invert = !invert)}>⇄</button>
    </div>
    {#if title}<header class="head"><span class="pane-name display">{title}</span><span class="count">3D</span></header>{/if}
  </div>
  <div class="gizmo" role="group" aria-label="Orientación: pulsa un eje para mirar a lo largo de él">
    <svg viewBox={`${-GIZMO / 2} ${-GIZMO / 2} ${GIZMO} ${GIZMO}`} width={GIZMO} height={GIZMO} aria-hidden="true">
      <circle r={GIZMO / 2 - 5} class="ring" />
      {#each gizmo as a (a.key)}
        <circle cx={-a.x} cy={-a.y} r="4" fill={a.color} opacity={0.35 + 0.25 * (-a.depth + 1) / 2} />
      {/each}
      {#each gizmo as a (a.key)}
        <line x1="0" y1="0" x2={a.x} y2={a.y} stroke={a.color} stroke-width="3" stroke-linecap="round" opacity={0.55 + 0.45 * (a.depth + 1) / 2} />
        <circle cx={a.x} cy={a.y} r="11" fill={a.color} />
        <text x={a.x} y={a.y} dy="4.4" text-anchor="middle" class="axis-label">{a.label}</text>
      {/each}
    </svg>
    {#each gizmo as a (a.key)}
      <button class="axis-hit" style:left={`${GIZMO / 2 + a.x - 14}px`} style:top={`${GIZMO / 2 + a.y - 14}px`}
        aria-label={`Ver ${a.view} (eje ${a.label})`} title={`Ver ${a.view}`} onclick={() => look(a.key)}></button>
    {/each}
  </div>
  <p class="sr-only">{label}</p>
  <canvas bind:this={cv} aria-hidden="true" data-cursor={grabbing ? 'grabbing' : hover ? 'pointer' : 'grab'}
    onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up}
    onpointerleave={() => { pointerAt = null; if (!gesture) { hover = ''; hoverBlob = null; } }}></canvas>
  {#if empty && !nodes.length}
    <div class="empty"><Icon of="bubble" size="3.5rem" tint /><p>{empty}</p></div>
  {/if}
  <div class="legend3d">Arrastra para girar · rueda para acercar · cursor sobre una burbuja: sus conexiones</div>
</div>

<style>
  .g3d { position: relative; width: 100%; height: 100%; overflow: hidden; overscroll-behavior: contain;
    background: radial-gradient(ellipse at 50% 45%, color-mix(in oklab, var(--surface-solid) 70%, transparent), transparent 75%); }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  canvas { width: 100%; height: 100%; display: block; touch-action: none; }
  canvas[data-cursor='pointer'] { cursor: var(--cur-pointer) !important; }
  .topbar { position: absolute; z-index: 2; top: 1rem; left: 1rem; right: 1rem; display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 0.6rem; pointer-events: none; }
  .topbar > * { pointer-events: auto; }
  .ctrl { grid-column: 1; justify-self: start; display: flex; align-items: center; gap: 0.2rem; padding: 0.25rem; border: 1px solid var(--line); border-radius: 999px; background: var(--surface-solid); box-shadow: 0 4px 16px rgb(0 0 0 / 0.16); }
  .ctrl button { width: 28px; height: 28px; border-radius: 999px; color: var(--text); }
  .ctrl button:hover, .ctrl button[aria-pressed='true'] { background: var(--hover); }
  .ctrl button.auto { color: var(--accent); }
  /* Debajo de la píldora de zoom, a la izquierda. */
  .gizmo { position: absolute; z-index: 2; top: calc(1rem + 36px + 1.5rem); left: 1rem; width: 112px; height: 112px; border-radius: 999px; background: color-mix(in oklab, var(--surface-solid) 85%, transparent); border: 1px solid var(--line); box-shadow: 0 4px 16px rgb(0 0 0 / 0.16); }
  .gizmo svg { display: block; }
  .gizmo .ring { fill: none; stroke: var(--line); stroke-dasharray: 2 4; }
  .axis-label { fill: white; font-size: 12px; font-weight: 800; pointer-events: none; }
  .axis-hit { position: absolute; width: 28px; height: 28px; border-radius: 999px; }
  .axis-hit:hover { box-shadow: 0 0 0 2px var(--text); }
  .axis-hit:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .head { grid-column: 2; display: flex; align-items: center; gap: 0.5rem; min-height: 42px; padding: 0.5rem 0.9rem; border: 1px solid var(--line); border-radius: 26px; background: var(--surface-solid); box-shadow: 0 4px 16px rgb(0 0 0 / 0.16); }
  .pane-name { font-size: 1.5rem; white-space: nowrap; }
  .count { padding: 0 0.45rem; border-radius: 999px; background: var(--hover); font-size: 0.75rem; }
  .empty { position: absolute; inset: 0; display: grid; place-content: center; justify-items: center; gap: 0.7rem; color: var(--muted); text-align: center; pointer-events: none; }
  .legend3d { position: absolute; left: 1rem; right: 1rem; bottom: calc(1rem + 56px + 0.5rem); text-align: right; color: var(--faint); font-size: 0.68rem; pointer-events: none; }
  @media (max-width: 1320px) { .legend3d { bottom: calc(4.75rem + 42px + 0.6rem); } }
  @media (max-width: 650px) { .gizmo { top: 8rem; } .topbar { display: flex; flex-direction: column; align-items: center; } .topbar > .ctrl { align-self: flex-start; } .pane-name { font-size: 1.15rem; } }
</style>
