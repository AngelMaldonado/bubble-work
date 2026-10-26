<script lang="ts">
  // La bienvenida: lo que alguien nuevo tiene que entender antes de su primer
  // tablero, en pantallas de dos líneas. Una pantalla propia y no un modal sobre
  // el tablero: es un momento aparte, con el clima de la puerta (SignIn).
  // Explica y no bloquea: se salta con un clic y no exige fundar nada —
  // pertenecer a nada todavía es un estado normal (ver el estado vacío de App),
  // y el último paso ofrece las mismas dos salidas que ese estado.
  //
  // El orden es el del modelo: dónde se ve el trabajo (equipos) y quién decide
  // qué en él, luego proyecto → burbuja → hilo, cómo se desinfla una
  // burbuja, y para qué es todo eso — terminar lo importante: la prioridad y la
  // furia. Después, dónde se sueltan las ideas; para un lead, lo que decide.
  //
  // Cada ícono entra en barro y se enciende a su color al llegar a su paso: el
  // mismo gesto que hace `Icon` bajo el cursor, aquí sin cursor.
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import { fly } from 'svelte/transition';
  import { iconSrc, type IconName } from '../lib/icons';
  import { onboarding } from '../lib/onboarding.svelte';
  import { api } from '../lib/api';
  import { setupPrompt } from '../lib/connect';
  import Loading from './Loading.svelte';
  import BrandOrb from './BrandOrb.svelte';
  import RisingBubbles from './RisingBubbles.svelte';

  let {
    lead = false,
    role = 'member',
    fury = false,
    onconnect,
  }: {
    /** lead global o de algún equipo: recibe los pasos de planear */
    lead?: boolean;
    /** su papel más alto, para marcarlo con «tú» en el organigrama */
    role?: 'global' | 'team' | 'member';
    /** la furia de IA está encendida */
    fury?: boolean;
    /** ir a conectar su IA (Ajustes → Tokens) */
    onconnect: () => void;
  } = $props();

  type Scene = 'teams' | 'roles' | 'project' | 'bubble' | 'thread' | 'deflate' | 'finish' | 'fury' | 'ideas' | 'plan' | 'triage' | 'connect';
  type Step = { scene: Scene; title: string; body: string; lead?: boolean };

  const steps = $derived(
    ([
      {
        scene: 'teams',
        title: 'Trabajas en equipos',
        body: 'Cada equipo tiene sus proyectos, sus metas y sus ideas, y lo de uno no se mezcla con otro. Un equipo personal es sólo tuyo: nadie más lo ve.',
      },
      {
        scene: 'roles',
        title: 'Quién ve y quién decide',
        body: 'El lead global ve todos los equipos. Un lead de equipo decide qué va primero e invita a otros. Un miembro hace su parte. Un invitado ve sólo su proyecto.',
      },
      {
        scene: 'project',
        title: 'Un proyecto reúne un mismo trabajo',
        body: 'Un proyecto reúne todo lo de un mismo trabajo: sus burbujas, sus hilos y sus documentos. Ves los proyectos a los que te invitaron.',
      },
      {
        scene: 'bubble',
        title: 'Una burbuja agrupa tareas con un mismo fin',
        body: 'Dice qué es cierto cuando termine, quién responde por ella y para cuándo. Cuando se cumple, se cierra.',
      },
      {
        scene: 'thread',
        title: 'Un hilo es una tarea concreta',
        body: 'Algo que se hace en horas o pocos días. Tiene su propia página con el plan, los pasos por marcar y las pruebas de lo hecho.',
      },
      {
        scene: 'deflate',
        title: 'Una burbuja sin avances se desinfla',
        body: 'Avanzar un hilo, marcar un paso o agregar una prueba la infla. Si nadie avanza, se enfría: caliente, dormida, tumba. Comentar no cuenta como avance.',
      },
      {
        scene: 'finish',
        title: 'El objetivo es terminar lo importante',
        body: 'No se trata de estar ocupado, sino de cerrar. Cada burbuja tiene prioridad y fecha, y el tablero pone arriba lo que hay que terminar primero.',
      },
      ...(fury
        ? [{
            scene: 'fury' as const,
            title: 'La Furia señala lo importante que se rompe',
            body: 'Una burbuja que crece sin terminarse, se vence, se olvida o se queda sin responsable se enoja. Se calma terminándola o dándole un responsable, no moviendo la fecha.',
          }]
        : []),
      {
        scene: 'ideas',
        title: 'Y las ideas, suéltalas aquí',
        body: 'En Mi espacio, en la bandeja de ideas del equipo o por WhatsApp. Una idea no es un pendiente: nadie la debe hasta que se decide hacerla.',
      },
      {
        scene: 'plan',
        lead: true,
        title: 'Tú pones la prioridad y el orden',
        body: 'Cada burbuja sirve a una meta del equipo. Tú decides qué tan importante y urgente es, para cuándo, y en qué orden se hace.',
      },
      {
        scene: 'triage',
        lead: true,
        title: 'Las ideas del equipo llegan a ti',
        body: 'En el Planeador ves la bandeja de ideas y decides cuáles se vuelven trabajo y cuáles se descartan.',
      },
      {
        scene: 'connect',
        title: 'Configura tu IA',
        body: 'Copia el prompt y pégalo en tu chat de IA (Claude, Codex o Antigravity): se configura solo y trabaja aquí contigo. Lo que haga queda a tu nombre.',
      },
    ] satisfies Step[]).filter((s) => !('lead' in s) || lead),
  );

  let at = $state(0);
  let dir = $state(1);
  const step = $derived(steps[Math.min(at, steps.length - 1)]);
  const last = $derived(at >= steps.length - 1);

  // Sin movimiento: los pasos aparecen y ya. El CSS apaga lo suyo aparte.
  const still = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

  function go(to: number) {
    if (to < 0 || to >= steps.length || to === at) return;
    dir = to > at ? 1 : -1;
    at = to;
  }
  // La primera vez, la aplicación se abre al copiar el prompt: configurar la IA
  // es la puerta. Saltar y Esc llevan a ese paso en vez de cerrar. Vista otra
  // vez desde Ajustes, no se bloquea nada.
  function skip() {
    if (gated) go(steps.length - 1);
    else onboarding.done();
  }
  function keys(e: KeyboardEvent) {
    if (!onboarding.open) return;
    if (e.key === 'ArrowRight') go(at + 1);
    else if (e.key === 'ArrowLeft') go(at - 1);
    else if (e.key === 'Escape' && gated) skip();
  }
  function settings() {
    onboarding.done();
    onconnect();
  }

  // Configurar la IA sin salir de aquí: lo mismo que Ajustes → Agente (un
  // token y el prompt que lo configura), sin la palabra «token». La primera vez
  // el token se crea solo al llegar al paso; vista otra vez, sólo si se pide,
  // para no dejar un token nuevo cada vez que alguien la repasa. Copiar es un
  // clic aparte porque el portapapeles sólo se deja escribir dentro del clic.
  //
  // `failed`: no hubo token o no hubo portapapeles. Se abre igual — dejar a
  // alguien encerrado por un fallo nuestro es peor que entrar sin IA.
  let link = $state<'idle' | 'busy' | 'ready' | 'copied' | 'failed'>('idle');
  let linkErr = $state('');
  let prompt = $state('');
  const gated = $derived(!onboarding.replaying && link !== 'copied' && link !== 'failed');
  async function makeLink() {
    link = 'busy';
    try {
      const t = await api.createToken('Mi asistente', 90);
      prompt = await setupPrompt(`${location.origin}/mcp`, t.token ?? '');
      link = 'ready';
    } catch (e) {
      linkErr = `No se pudo preparar tu prompt (${(e as Error).message}). Puedes configurarla después en Ajustes → Agente.`;
      link = 'failed';
    }
  }
  $effect(() => {
    if (step.scene === 'connect' && link === 'idle' && !onboarding.replaying) makeLink();
  });
  async function copyLink() {
    try {
      await navigator.clipboard.writeText(prompt);
      link = 'copied';
    } catch {
      linkErr = 'El navegador no dejó copiar. Selecciona el prompt de abajo, cópialo a mano y pégalo en tu chat de IA.';
      link = 'failed';
    }
  }

  // Equipos de ejemplo, no los de quien mira: una persona nueva suele estar en
  // uno solo, y con uno no se ve que son contextos aparte.
  const sampleTeams = ['TI', 'Diseño', 'RH'];
  // El organigrama de PAPELES, no de quién le reporta a quién: un equipo es
  // dónde se ve el trabajo (README → Team).
  const chart: { key: 'global' | 'team' | 'member' | 'guest'; icon: IconName; name: string; n: number }[] = [
    { key: 'global', icon: 'key', name: 'Lead global', n: 1 },
    { key: 'team', icon: 'compass', name: 'Lead de equipo', n: 2 },
    { key: 'member', icon: 'people', name: 'Miembro', n: 3 },
    { key: 'guest', icon: 'door', name: 'Invitado a un proyecto', n: 1 },
  ];
  const sources: IconName[] = ['pin', 'chat', 'robot'];
  // La misma burbuja, cada vez más chica y más hundida.
  const deflate: { band: string; icon: IconName; name: string; s: number; y: number }[] = [
    { band: 'hot', icon: 'fire', name: 'Caliente', s: 92, y: -18 },
    { band: 'dormant', icon: 'dormant', name: 'Dormida', s: 64, y: 4 },
    { band: 'rip', icon: 'tombstone', name: 'Tumba', s: 40, y: 26 },
  ];
  const moods: { icon: IconName; name: string; s: number }[] = [
    { icon: 'zen', name: 'En calma', s: 48 },
    { icon: 'irritated', name: 'Irritada', s: 56 },
    { icon: 'furious', name: 'Furiosa', s: 66 },
    { icon: 'unleashed', name: 'Desatada', s: 78 },
  ];
</script>

<svelte:window onkeydown={keys} />

{#snippet art(name: IconName, size: number, delay = 0, cls = '')}
  <span class="art {cls}" style="--s: {size}px; --d: {delay}ms">
    <img class="clay" src={iconSrc(name, 'clay')} alt="" />
    <img class="lit" src={iconSrc(name, 'tint')} alt="" />
  </span>
{/snippet}

<Dialog
  open={onboarding.open}
  onOpenChange={(e) => { if (!e.open) onboarding.done(); }}
  closeOnEscape={!gated}
  closeOnInteractOutside={false}>
  <Portal>
    <Dialog.Positioner class="fixed inset-0" style="z-index: var(--z-drawer)">
      <Dialog.Content class="welcome-screen">
        <RisingBubbles />
        <div class="top">
          <div class="brand"><BrandOrb size={26} /><span class="display">bubble.work</span></div>
          <div class="dots" role="tablist" aria-label="Pasos">
            {#each steps as s, i (s.scene)}
              <button
                class="step-dot"
                class:on={i === at}
                class:past={i < at}
                role="tab"
                aria-selected={i === at}
                aria-label="Paso {i + 1} de {steps.length}"
                onclick={() => go(i)}></button>
            {/each}
          </div>
          <span class="skip-slot">
            {#if !last}
              <button class="skip" onclick={skip}>Saltar</button>
            {/if}
          </span>
        </div>

        <div class="frame">
          {#key at}
            <div class="slide" in:fly={{ x: still ? 0 : dir * 48, duration: still ? 0 : 320 }}>
              <div class="scene" aria-hidden="true">
                {#if step.scene === 'teams'}
                  <div class="teams">
                    {#each sampleTeams as name, i (name)}
                      <div class="team-card" style="--d: {150 + i * 220}ms">
                        {@render art('people', 44, 300 + i * 220)}
                        <span class="team-name">{name}</span>
                        <span class="team-stuff">
                          {@render art('board', 22, 500 + i * 220)}
                          {@render art('target', 22, 600 + i * 220)}
                          {@render art('inbox', 22, 700 + i * 220)}
                        </span>
                      </div>
                    {/each}
                  </div>
                {:else if step.scene === 'roles'}
                  <div class="chart">
                    {#each chart as c, i (c.key)}
                      <div class="tier" class:guest={c.key === 'guest'} style="--d: {150 + i * 280}ms">
                        {#each Array(c.n) as _, j (j)}
                          <div class="node" class:me={c.key === role && j === 0}>
                            {@render art(c.icon, c.key === 'global' ? 40 : 30, 200 + i * 280)}
                            {#if c.key === role && j === 0}<span class="you">tú</span>{/if}
                          </div>
                        {/each}
                        <span class="tier-name">{c.name}</span>
                      </div>
                    {/each}
                  </div>
                {:else if step.scene === 'project'}
                  <div class="trio">
                    <div class="tag">{@render art('bubble', 56, 450)}<span>burbujas</span></div>
                    <div class="tag">{@render art('board', 104, 150, 'bob')}<span>proyecto</span></div>
                    <div class="tag">{@render art('book', 56, 750)}<span>documentos</span></div>
                  </div>
                {:else if step.scene === 'bubble'}
                  <div class="trio">
                    <div class="tag">{@render art('target', 56, 600)}<span>resultado</span></div>
                    <div class="tag">{@render art('bubble', 112, 150, 'bob')}<span>burbuja</span></div>
                    <div class="tag">{@render art('people', 56, 900)}<span>responsable</span></div>
                  </div>
                {:else if step.scene === 'thread'}
                  <div class="flow">
                    {@render art('bubble', 64, 100)}
                    {@render art('arrow', 40, 300, 'nudge')}
                    <div class="doc">
                      <span class="doc-icon">{@render art('thread', 44, 500, 'bob')}</span>
                      <span class="line w70"></span>
                      {#each [0, 1, 2] as i (i)}
                        <span class="todo">
                          <span class="box" style="--d: {900 + i * 650}ms"></span>
                          <span class="line" style="width: {[64, 48, 56][i]}%"></span>
                        </span>
                      {/each}
                    </div>
                  </div>
                {:else if step.scene === 'deflate'}
                  <div class="bands">
                    {#each deflate as b, i (b.band)}
                      <div class="band" style="--y: {b.y}px; --d: {200 + i * 450}ms">
                        <span class="orb band-{b.band}" style="--s: {b.s}px">
                          {@render art(b.icon, Math.round(b.s * 0.45), 200 + i * 450)}
                        </span>
                        <span class="band-name">{b.name}</span>
                      </div>
                    {/each}
                  </div>
                {:else if step.scene === 'finish'}
                  <div class="trio">
                    <div class="tag">{@render art('numbers', 56, 450)}<span>prioridad</span></div>
                    <div class="tag">{@render art('trophy', 108, 150, 'bob')}<span>terminada</span></div>
                    <div class="tag">{@render art('calendar', 56, 750)}<span>fecha</span></div>
                  </div>
                {:else if step.scene === 'fury'}
                  <div class="bands moods">
                    {#each moods as m, i (m.icon)}
                      <div class="band">
                        {@render art(m.icon, m.s, 150 + i * 380, i === 3 ? 'shake' : '')}
                        <span class="band-name">{m.name}</span>
                      </div>
                    {/each}
                  </div>
                {:else if step.scene === 'ideas'}
                  <div class="ideas">
                    <div class="sources">
                      {#each sources as n, i (n)}
                        <div class="source">
                          {@render art(n, 52, 120 + i * 140, 'bob')}
                          <span class="drop band-hot" style="--d: {i * 600}ms"></span>
                        </div>
                      {/each}
                    </div>
                    {@render art('inbox', 104, 600)}
                  </div>
                {:else if step.scene === 'plan'}
                  <div class="trio">
                    <div class="tag">{@render art('numbers', 56, 450)}<span>importancia</span></div>
                    <div class="tag">{@render art('target', 96, 150, 'bob')}<span>objetivo</span></div>
                    <div class="tag">{@render art('compass', 56, 750)}<span>orden</span></div>
                  </div>
                {:else if step.scene === 'triage'}
                  <div class="flow">
                    {@render art('inbox', 84, 100)}
                    {@render art('arrow', 40, 300, 'nudge')}
                    <div class="fork">
                      {@render art('check', 56, 550, 'bob')}
                      {@render art('trash', 44, 800)}
                    </div>
                  </div>
                {:else}
                  <div class="flow">
                    {@render art('robot', 104, 150, 'bob')}
                    {@render art('arrow', 40, 400, 'nudge')}
                    {@render art('bubble', 76, 650)}
                  </div>
                {/if}
              </div>
              <Dialog.Title class="display title">{step.title}</Dialog.Title>
              <Dialog.Description class="body">{step.body}</Dialog.Description>
              {#if step.scene === 'connect' && (link === 'copied' || link === 'failed')}
                <p class="link-note" class:bad={link === 'failed'} role="status">
                  {#if link === 'copied'}
                    ✓ Copiado. Pégalo en tu chat de IA y sigue lo que te diga. Esta conexión la ves o la quitas en Ajustes → Agente.
                  {:else}
                    {linkErr}
                  {/if}
                </p>
                {#if link === 'failed' && prompt}
                  <textarea class="prompt-box" readonly rows="4" onfocus={(e) => e.currentTarget.select()}>{prompt}</textarea>
                {/if}
              {/if}
            </div>
          {/key}
        </div>

        <div class="actions">
          {#if at > 0}
            <button class="btn preset-tonal-surface" onclick={() => go(at - 1)}>Atrás</button>
          {/if}
          <span class="grow"></span>
          {#if last}
            {#if link === 'copied'}
              <button class="btn preset-tonal-surface" onclick={copyLink}>Copiar otra vez</button>
              <button class="go" {@attach (el: HTMLButtonElement) => el.focus()} onclick={() => onboarding.done()}>Entrar a Bubble Work</button>
            {:else if link === 'failed'}
              <button class="btn preset-tonal-surface" onclick={settings}>Ir a Ajustes</button>
              <button class="go" onclick={() => onboarding.done()}>Entrar a Bubble Work</button>
            {:else if link === 'idle'}
              <!-- Vista otra vez: el token sólo si se pide. -->
              <button class="btn preset-tonal-surface" onclick={() => onboarding.done()}>Cerrar</button>
              <button class="go" onclick={makeLink}>Crear mi prompt</button>
            {:else}
              {#if onboarding.replaying}
                <button class="btn preset-tonal-surface" onclick={() => onboarding.done()}>Cerrar</button>
              {/if}
              <button class="go" {@attach (el: HTMLButtonElement) => el.focus()} disabled={link === 'busy'} onclick={copyLink}>
                {#if link === 'busy'}<Loading compact label="Preparando tu prompt" />{:else}Copiar prompt{/if}
              </button>
            {/if}
          {:else}
            {#key at}
              <button class="go" {@attach (el: HTMLButtonElement) => el.focus()} onclick={() => go(at + 1)}>Siguiente</button>
            {/key}
          {/if}
        </div>
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  /* La pantalla entera, con el fondo de la página: encima del tablero, que
     no se ve detrás. Tres filas: la marca y los pasos, el escenario, los botones. */
  :global(.welcome-screen) {
    position: fixed;
    inset: 0;
    display: grid;
    grid-template-rows: auto 1fr auto;
    overflow: hidden;
    background: var(--bg);
  }

  .top {
    position: relative;
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    padding: 1.25rem 1.75rem;
  }
  .brand { display: flex; align-items: center; gap: 0.55rem; font-size: 1.15rem; }
  .skip-slot { justify-self: end; }
  .dots { display: flex; gap: 0.5rem; }
  .step-dot {
    width: 0.6rem;
    height: 0.6rem;
    border-radius: 999px;
    background: var(--line);
    transition: width 0.25s ease, background 0.25s ease;
  }
  .step-dot.past { background: color-mix(in oklab, var(--hot) 45%, var(--line)); }
  .step-dot.on { width: 1.9rem; background: var(--hot); }
  .skip { color: var(--muted); font-size: 0.9rem; }
  .skip:hover { color: var(--text); text-decoration: underline; }

  .frame { position: relative; display: grid; place-items: center; overflow: hidden; padding: 0 1.5rem; }
  .slide { width: 100%; max-width: 44rem; text-align: center; }
  /* El escenario, ampliado entero: cada escena está dibujada a tamaño de
     tarjeta, y `zoom` la agranda con sus distancias y no sólo sus íconos. */
  .scene {
    display: grid;
    place-items: center;
    height: 13rem;
    margin-bottom: 1.25rem;
    zoom: 1.6;
  }
  .slide :global(.title) { font-size: 2.5rem; line-height: 1.1; margin-bottom: 0.75rem; }
  .slide :global(.body) {
    max-width: 34rem;
    /* Tres líneas siempre: con dos, el texto saltaba de altura entre pasos. */
    min-height: 4.5em;
    margin: 0 auto;
    color: var(--muted);
    font-size: 1.08rem;
    line-height: 1.55;
  }

  .actions {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    max-width: 44rem;
    margin: 0 auto;
    padding: 1.25rem 1.5rem 2rem;
  }
  .grow { flex: 1; }
  /* El acento de la app, como el botón de la puerta (SignIn). */
  .go {
    padding: 0.6rem 1.6rem;
    border-radius: 12px;
    border: 1px solid color-mix(in oklab, var(--hot) 70%, black);
    background: var(--hot);
    color: oklch(0.99 0 0);
    font-weight: 650;
    font-size: 1rem;
    white-space: nowrap;
    transition: filter 0.15s ease, transform 0.1s ease;
  }
  .go:hover:not(:disabled) { filter: brightness(1.06); }
  .go:disabled { opacity: 0.7; }
  .link-note {
    max-width: 34rem;
    margin: 0.75rem auto 0;
    padding: 0.6rem 0.9rem;
    border-radius: 10px;
    background: color-mix(in oklab, var(--hot) 12%, transparent);
    font-size: 0.92rem;
    line-height: 1.45;
  }
  .prompt-box {
    display: block;
    width: 100%;
    max-width: 34rem;
    margin: 0.6rem auto 0;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--hover);
    font-size: 0.75rem;
    resize: none;
  }
  .link-note.bad { background: color-mix(in oklab, var(--color-error-500) 14%, transparent); }
  .go:active { transform: translateY(1px); }

  @media (max-width: 640px), (max-height: 680px) {
    .scene { zoom: 1.1; margin-bottom: 0.5rem; }
    .slide :global(.title) { font-size: 1.8rem; }
    .slide :global(.body) { font-size: 0.95rem; }
    .brand span { display: none; }
    .go { padding: 0.55rem 1.1rem; }
    .actions { gap: 0.4rem; padding-inline: 1rem; }
  }

  /* ---- el ícono: entra en barro y se enciende ---------------------------- */
  .art {
    position: relative;
    display: inline-block;
    width: var(--s);
    height: var(--s);
    flex: none;
    animation: pop 0.45s cubic-bezier(0.34, 1.56, 0.64, 1) both;
    animation-delay: calc(var(--d) * 0.4);
  }
  .art img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: contain; }
  .art .lit { opacity: 0; animation: light 0.6s ease forwards; animation-delay: var(--d); }
  @keyframes pop {
    from { transform: scale(0.6); opacity: 0; }
    to { transform: scale(1); opacity: 1; }
  }
  @keyframes light { to { opacity: 1; } }

  .bob { animation: pop 0.45s cubic-bezier(0.34, 1.56, 0.64, 1) both, bob 3.6s ease-in-out 0.5s infinite alternate; }
  @keyframes bob { from { translate: 0 0; } to { translate: 0 -8px; } }
  .sink { animation: pop 0.45s ease both, sink 2.4s ease-in-out 0.8s forwards; }
  @keyframes sink { to { translate: 0 6px; filter: saturate(0.4); } }
  .nudge { animation: pop 0.45s ease both, nudge 1.4s ease-in-out 0.6s infinite; }
  @keyframes nudge { 50% { translate: 5px 0; } }
  .shake { animation: pop 0.45s ease both, shake 0.5s ease-in-out 1.6s 3; }
  @keyframes shake { 25% { rotate: -8deg; } 75% { rotate: 8deg; } }

  /* ---- equipos: cada uno su tarjeta ----------------------------------- */
  .teams { display: flex; gap: 1rem; }
  .team-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.2rem;
    width: 7.5rem;
    padding: 0.75rem 0.5rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--hover);
    animation: pop 0.45s cubic-bezier(0.34, 1.56, 0.64, 1) var(--d) both;
  }
  .team-name { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.8rem; font-weight: 700; }
  .team-stuff { display: flex; gap: 0.3rem; margin-top: 0.35rem; }

  /* ---- organigrama de papeles ------------------------------------------- */
  .chart { display: flex; flex-direction: column; align-items: center; gap: 0.45rem; }
  .tier {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    animation: pop 0.45s ease var(--d) both;
  }
  /* El nombre del nivel a la derecha, fuera del centro: los nodos quedan
     alineados como un árbol. */
  .tier-name {
    position: absolute;
    left: calc(100% + 0.7rem);
    color: var(--muted);
    font-size: 0.68rem;
    font-weight: 600;
    white-space: nowrap;
  }
  .tier + .tier::before {
    content: '';
    position: absolute;
    top: -0.45rem;
    left: 50%;
    height: 0.45rem;
    border-left: 1.5px solid var(--line);
  }
  .tier.guest::before { border-left-style: dashed; }
  .tier.guest .node { border-style: dashed; }
  .node {
    position: relative;
    display: grid;
    place-items: center;
    padding: 0.2rem;
    border: 1.5px solid var(--line);
    border-radius: 999px;
    background: var(--hover);
  }
  .node.me { border-color: var(--hot); border-width: 2px; }
  .you {
    position: absolute;
    top: -0.55rem;
    right: -0.7rem;
    padding: 0 0.3rem;
    border-radius: 999px;
    background: var(--hot);
    color: white;
    font-size: 0.58rem;
    font-weight: 700;
  }

  /* ---- desinflarse: la burbuja encoge y se hunde ---------------------- */
  .orb {
    display: grid;
    place-items: center;
    width: var(--s);
    height: var(--s);
    border-radius: 999px;
    /* Los mismos degradados que las burbujas del tablero y de la puerta. */
    background:
      radial-gradient(circle at 34% 30%, oklch(1 0 0 / 0.65), transparent 45%),
      radial-gradient(circle at 65% 80%, color-mix(in oklab, var(--accent) 60%, transparent), transparent 70%),
      color-mix(in oklab, var(--accent) 38%, transparent);
    animation: inflate 0.6s cubic-bezier(0.34, 1.56, 0.64, 1) both;
    animation-delay: var(--d);
  }
  @keyframes inflate {
    from { transform: scale(0.3); opacity: 0; }
    to { transform: scale(1); opacity: 1; }
  }
  /* La caliente respira: sigue viva. */
  .orb.band-hot { animation: inflate 0.6s cubic-bezier(0.34, 1.56, 0.64, 1) var(--d) both, breathe 2.8s ease-in-out 1s infinite alternate; }
  @keyframes breathe { to { transform: scale(1.06); } }
  .moods { align-items: flex-end; }

  /* ---- ideas: caen al inbox -------------------------------------------- */
  .ideas { display: flex; flex-direction: column; align-items: center; gap: 1.6rem; }
  .sources { display: flex; gap: 2.5rem; }
  .source { position: relative; display: grid; place-items: center; }
  .drop {
    position: absolute;
    top: 100%;
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: var(--accent);
    opacity: 0;
    animation: fall 1.8s ease-in var(--d) infinite;
  }
  @keyframes fall {
    0% { transform: translateY(0); opacity: 0; }
    20% { opacity: 1; }
    100% { transform: translateY(3.2rem); opacity: 0; }
  }

  /* ---- trabajo: el documento se va marcando ----------------------------- */
  .flow { display: flex; align-items: center; justify-content: center; gap: 0.9rem; }
  .doc {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
    width: 11rem;
    padding: 1rem 0.9rem 0.9rem;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--hover);
    text-align: left;
    animation: pop 0.45s ease both 0.25s;
  }
  .doc-icon { position: absolute; top: -1.3rem; right: -1.1rem; }
  .line { display: block; height: 0.45rem; border-radius: 999px; background: var(--line); }
  .w70 { width: 70%; height: 0.6rem; margin-bottom: 0.2rem; }
  .todo { display: flex; align-items: center; gap: 0.45rem; }
  .box {
    position: relative;
    flex: none;
    width: 0.9rem;
    height: 0.9rem;
    border: 1.5px solid var(--muted);
    border-radius: 4px;
    animation: tick 0.3s ease var(--d) forwards;
  }
  .box::after {
    content: '';
    position: absolute;
    left: 0.22rem;
    top: 0.05rem;
    width: 0.3rem;
    height: 0.55rem;
    border: solid white;
    border-width: 0 2px 2px 0;
    rotate: 45deg;
    opacity: 0;
    animation: light 0.2s ease var(--d) forwards;
  }
  @keyframes tick { to { background: var(--hot); border-color: var(--hot); } }

  /* ---- calor: cada banda flota a su altura ------------------------------ */
  .bands { display: flex; align-items: center; gap: 1.4rem; }
  .band { display: flex; flex-direction: column; align-items: center; gap: 0.35rem; translate: 0 var(--y); }
  .band-name, .tag span { color: var(--muted); font-size: 0.72rem; font-weight: 600; }

  .trio { display: flex; align-items: center; justify-content: center; gap: 1.6rem; }
  .tag { display: flex; flex-direction: column; align-items: center; gap: 0.35rem; }
  .fork { display: flex; flex-direction: column; align-items: center; gap: 0.5rem; }

  @media (prefers-reduced-motion: reduce) {
    .art, .art .lit, .bob, .sink, .nudge, .shake, .doc, .box, .box::after, .orb, .team-card, .tier { animation: none; opacity: 1; }
    .art .lit { opacity: 1; }
    .box { background: var(--hot); border-color: var(--hot); }
    .drop { display: none; }
  }
</style>
