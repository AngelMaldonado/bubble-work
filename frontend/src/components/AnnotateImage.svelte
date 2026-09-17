<script lang="ts">
  // Anotar una imagen antes de pegarla: trazos a mano alzada y texto, en el
  // color que se elija.
  //
  // Lo que se anota se APLANA en la imagen que se sube. Guardar las anotaciones
  // aparte —capas, un formato propio— haría que la imagen dijera una cosa en la
  // app y otra en un clon del repositorio, que es donde también se lee. Un PNG
  // con las flechas dentro dice lo mismo en todas partes.
  //
  // Sin anotaciones se sube el archivo ORIGINAL, sin pasar por el canvas: mismos
  // bytes, mismo formato, y pegar dos veces la misma captura sigue reusando la
  // misma ruta en `assets/`.
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import { onMount, tick } from 'svelte';
  import LineSquiggleIcon from '@lucide/svelte/icons/line-squiggle';

  let { file, ondone }: { file: File; ondone: (out: File | null) => void } = $props();

  type Point = { x: number; y: number };
  type Op =
    | { kind: 'stroke'; color: string; width: number; points: Point[] }
    | { kind: 'text'; color: string; size: number; at: Point; text: string };

  const colors = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#3b82f6', '#a855f7', '#111827', '#ffffff'];
  const weights = [
    { id: 0, label: 'Fino', stroke: 1.5 },
    { id: 1, label: 'Medio', stroke: 2.5 },
    { id: 2, label: 'Grueso', stroke: 4 },
  ];

  let open = $state(true);
  let tool = $state<'pen' | 'text'>('pen');
  let color = $state(colors[0]);
  let weight = $state(1);
  let ops = $state<Op[]>([]);

  let canvas = $state<HTMLCanvasElement | null>(null);
  let stage = $state<HTMLDivElement | null>(null);
  let image: ImageBitmap | null = null;
  // Una captura de una pantalla grande pasa de 5000 px; más que esto no se ve
  // mejor en un documento y sí pesa más en el repositorio.
  const MAX_SIDE = 4096;

  // Grosor y tamaño de letra relativos a la imagen: el mismo «medio» tiene que
  // leerse igual en una captura de un botón que en una de la pantalla entera.
  const side = () => Math.max(canvas?.width ?? 0, canvas?.height ?? 0);
  const penWidth = (w: number) => Math.max(2, side() * [0.0025, 0.005, 0.01][w]);
  const fontSize = (w: number) => Math.max(14, side() * [0.025, 0.04, 0.06][w]);

  let done = false;
  function finish(out: File | null) {
    if (done) return;
    done = true;
    open = false;
    ondone(out);
  }

  onMount(async () => {
    try {
      image = await createImageBitmap(file);
    } catch {
      // Un formato que el navegador no sabe dibujar (un SVG, en algunos) no se
      // puede anotar, pero se puede pegar: se sube tal cual.
      finish(file);
      return;
    }
    const scale = Math.min(1, MAX_SIDE / Math.max(image.width, image.height));
    await tick();
    if (!canvas) return;
    canvas.width = Math.round(image.width * scale);
    canvas.height = Math.round(image.height * scale);
    redraw();
  });

  function ctx() {
    return canvas?.getContext('2d') ?? null;
  }

  function draw(c: CanvasRenderingContext2D, op: Op) {
    if (op.kind === 'stroke') {
      c.strokeStyle = op.color;
      c.lineWidth = op.width;
      c.lineCap = 'round';
      c.lineJoin = 'round';
      c.beginPath();
      const [first, ...rest] = op.points;
      c.moveTo(first.x, first.y);
      // Un clic sin arrastrar es un punto, y un camino de un solo punto no
      // dibuja nada.
      if (!rest.length) c.lineTo(first.x + 0.01, first.y);
      for (const p of rest) c.lineTo(p.x, p.y);
      c.stroke();
      return;
    }
    c.font = `700 ${op.size}px system-ui, sans-serif`;
    c.textBaseline = 'top';
    // Un contorno del color opuesto: un texto rojo encima de una captura roja
    // también tiene que leerse.
    c.lineWidth = Math.max(2, op.size / 7);
    c.lineJoin = 'round';
    c.strokeStyle = isLight(op.color) ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.8)';
    c.strokeText(op.text, op.at.x, op.at.y);
    c.fillStyle = op.color;
    c.fillText(op.text, op.at.x, op.at.y);
  }

  function isLight(hex: string) {
    const n = parseInt(hex.slice(1), 16);
    const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255];
    return 0.299 * r + 0.587 * g + 0.114 * b > 160;
  }

  function redraw() {
    const c = ctx();
    if (!c || !canvas || !image) return;
    c.clearRect(0, 0, canvas.width, canvas.height);
    c.drawImage(image, 0, 0, canvas.width, canvas.height);
    for (const op of ops) draw(c, op);
  }

  /** De la pantalla al canvas: el canvas va a la resolución de la imagen y se
   *  dibuja encogido para caber en el modal. */
  function at(e: { clientX: number; clientY: number }): Point {
    const r = canvas!.getBoundingClientRect();
    return {
      x: ((e.clientX - r.left) * canvas!.width) / r.width,
      y: ((e.clientY - r.top) * canvas!.height) / r.height,
    };
  }

  // ---- mano alzada ----------------------------------------------------------

  let stroke: Extract<Op, { kind: 'stroke' }> | null = null;

  function down(e: PointerEvent) {
    if (!canvas || e.button !== 0) return;
    if (tool === 'text') {
      place(e);
      return;
    }
    canvas.setPointerCapture(e.pointerId);
    stroke = { kind: 'stroke', color, width: penWidth(weight), points: [at(e)] };
    const c = ctx();
    if (c) draw(c, stroke);
  }

  function move(e: PointerEvent) {
    if (!stroke) return;
    const prev = stroke.points[stroke.points.length - 1];
    const p = at(e);
    stroke.points.push(p);
    // Sólo el tramo nuevo: redibujar la imagen entera en cada movimiento del
    // ratón es redibujar una captura de 4K sesenta veces por segundo.
    const c = ctx();
    if (!c) return;
    draw(c, { ...stroke, points: [prev, p] });
  }

  function up() {
    if (!stroke) return;
    ops.push(stroke);
    stroke = null;
  }

  // ---- texto ----------------------------------------------------------------

  let typing = $state<{ at: Point; left: number; top: number; px: number } | null>(null);
  let typed = $state('');
  let field = $state<HTMLInputElement | null>(null);

  async function place(e: PointerEvent) {
    e.preventDefault();
    commitText();
    const r = canvas!.getBoundingClientRect();
    // Medido desde el `stage`, que es donde vive el campo: el canvas va
    // centrado dentro de él y no empieza en su borde.
    const s = stage!.getBoundingClientRect();
    typing = {
      at: at(e),
      left: e.clientX - s.left,
      top: e.clientY - s.top,
      px: (fontSize(weight) * r.width) / canvas!.width,
    };
    typed = '';
    await tick();
    field?.focus();
  }

  function commitText() {
    if (typing && typed.trim()) {
      ops.push({ kind: 'text', color, size: fontSize(weight), at: typing.at, text: typed.trim() });
      redraw();
    }
    typing = null;
    typed = '';
  }

  // ---- lo demás -------------------------------------------------------------

  function undo() {
    if (typing) {
      typing = null;
      return;
    }
    ops.pop();
    redraw();
  }

  function clear() {
    typing = null;
    ops = [];
    redraw();
  }

  async function insert() {
    commitText();
    if (!ops.length || !canvas) {
      finish(file);
      return;
    }
    const blob = await new Promise<Blob | null>((r) => canvas!.toBlob(r, 'image/png'));
    if (!blob) {
      finish(file);
      return;
    }
    const stem = file.name.replace(/\.[^.]+$/, '') || 'imagen';
    finish(new File([blob], `${stem}.png`, { type: 'image/png' }));
  }

  function keys(e: KeyboardEvent) {
    if (e.target === field) return;
    const mod = e.metaKey || e.ctrlKey;
    if (mod && e.key.toLowerCase() === 'z') {
      e.preventDefault();
      undo();
    } else if (mod && e.key === 'Enter') {
      e.preventDefault();
      insert();
    }
  }
</script>

<Dialog
  {open}
  closeOnInteractOutside={false}
  onOpenChange={(e: { open: boolean }) => {
    if (!e.open) finish(null);
  }}>
  <Portal>
    <Dialog.Backdrop class="scrim" style="z-index: 55" />
    <Dialog.Positioner class="fixed inset-0 flex items-center justify-center p-4" style="z-index: 56">
      <Dialog.Content class="card bg-surface-100-900 annotate shadow-xl" onkeydown={keys}>
        <header class="bar">
          <Dialog.Title class="text-base font-bold">Anotar la imagen</Dialog.Title>

          <div class="tools">
            <div class="group" role="radiogroup" aria-label="Herramienta">
              <button class="chip" class:on={tool === 'pen'} onclick={() => ((tool = 'pen'), commitText())}
                title="Mano alzada">✏️ Trazo</button>
              <button class="chip" class:on={tool === 'text'} onclick={() => (tool = 'text')}
                title="Clic en la imagen para escribir">🔤 Texto</button>
            </div>

            <div class="group" role="radiogroup" aria-label="Color">
              {#each colors as c (c)}
                <button
                  class="swatch"
                  class:on={color === c}
                  style="--c: {c}"
                  aria-label={c}
                  onclick={() => (color = c)}></button>
              {/each}
              <label class="swatch custom" class:on={!colors.includes(color)} title="Otro color">
                <input type="color" bind:value={color} aria-label="Otro color" />
              </label>
            </div>

            <div class="group" role="radiogroup" aria-label="Grosor">
              {#each weights as w (w.id)}
                <!-- Una muestra del trazo, no un icono de «grosor»: el mismo
                     garabato dibujado cada vez más grueso, como en Excalidraw.
                     Se lee sin palabras porque es lo que se va a pintar. -->
                <button
                  class="chip stroke"
                  class:on={weight === w.id}
                  onclick={() => (weight = w.id)}
                  title={w.label}
                  aria-label={w.label}>
                  <LineSquiggleIcon size={18} strokeWidth={w.stroke} aria-hidden="true" />
                </button>
              {/each}
            </div>

            <div class="group">
              <button class="chip" onclick={undo} disabled={!ops.length && !typing} title="⌘Z">↶ Deshacer</button>
              <button class="chip" onclick={clear} disabled={!ops.length}>Borrar todo</button>
            </div>
          </div>
        </header>

        <div class="stage" bind:this={stage}>
          <canvas
            bind:this={canvas}
            class:text={tool === 'text'}
            onpointerdown={down}
            onpointermove={move}
            onpointerup={up}
            onpointercancel={up}></canvas>
          {#if typing}
            <input
              bind:this={field}
              bind:value={typed}
              class="typing"
              style="left: {typing.left}px; top: {typing.top}px; font-size: {typing.px}px; color: {color}"
              placeholder="Escribe…"
              onkeydown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  commitText();
                } else if (e.key === 'Escape') {
                  // Suelta el texto, no el modal.
                  e.preventDefault();
                  e.stopPropagation();
                  typing = null;
                  typed = '';
                }
              }}
              onblur={commitText} />
          {/if}
        </div>

        <footer class="bar end">
          <span class="faint text-xs">
            {ops.length ? 'Se pega con las anotaciones.' : 'Sin anotaciones se pega la imagen original.'}
          </span>
          <button class="btn btn-sm preset-tonal-surface" onclick={() => finish(null)}>Cancelar</button>
          <button class="btn btn-sm preset-filled-primary-500" onclick={insert} title="⌘↵">Pegar</button>
        </footer>
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  :global(.annotate) {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    width: min(96vw, 1100px);
    padding: 1rem;
  }
  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: 0.5rem 1rem; }
  .bar.end { justify-content: flex-end; gap: 0.5rem; }
  .bar.end span { margin-right: auto; }
  .group { display: flex; align-items: center; gap: 0.3rem; }
  /* Las herramientas, a la derecha del título. */
  .tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem 1rem;
    margin-left: auto;
  }

  .chip {
    padding: 0.2rem 0.55rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font-size: 0.78rem;
  }
  .chip.on { background: var(--hover); border-color: currentColor; font-weight: 600; }
  .chip:disabled { opacity: 0.4; }
  .chip.stroke { display: grid; place-content: center; padding: 0.15rem 0.4rem; }

  .swatch {
    position: relative;
    width: 20px;
    height: 20px;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--c);
  }
  .swatch.on { outline: 2px solid var(--text); outline-offset: 2px; }
  .swatch.custom {
    overflow: hidden;
    background: conic-gradient(red, yellow, lime, aqua, blue, magenta, red);
    cursor: pointer;
  }
  .swatch.custom input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }

  /* Alto fijo y no «lo que sobre»: en una columna flexible el stage se encogía
     para caber, y con `overflow: hidden` la imagen salía recortada en vez de
     escalada. Con un alto definido, el canvas cabe entero dentro. */
  .stage {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    height: 80vh;
    overflow: hidden;
  }
  canvas {
    max-width: 100%;
    max-height: 100%;
    border-radius: 8px;
    background: repeating-conic-gradient(var(--hover) 0 25%, transparent 0 50%) 0 0 / 16px 16px;
    cursor: crosshair;
    touch-action: none;
  }
  canvas.text { cursor: text; }

  /* Encima del canvas, donde se hizo clic, con la letra al tamaño en que va a
     quedar dibujada. */
  .typing {
    position: absolute;
    min-width: 6rem;
    padding: 0;
    border: none;
    outline: 1px dashed currentColor;
    background: transparent;
    font-weight: 700;
    font-family: system-ui, sans-serif;
    line-height: 1;
  }
</style>
