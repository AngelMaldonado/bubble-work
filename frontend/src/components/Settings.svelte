<script lang="ts">
  // Ajustes: su propia barra y su propia vuelta, y una columna con las
  // categorías —la `Navigation` de Skeleton, como la del hilo o la del toolbox
  // de la secuencia—, que se encoge a rail igual que la de proyectos.
  //
  // La categoría va en la DIRECCIÓN (`/ajustes/tokens`): «mira tus tokens» se
  // manda como enlace, y recargar no te devuelve a la primera. Por eso cada fila
  // es un ancla con su dirección, y elegirla es navegar.
  import { Navigation, Portal, Tooltip } from '@skeletonlabs/skeleton-svelte';
  import InfoIcon from '@lucide/svelte/icons/info';
  import PanelLeftIcon from '@lucide/svelte/icons/panel-left';
  import { avatarUrl, type Features, type Person } from '../lib/api';
  import { settingsUrl } from '../lib/routes';
  import { rail } from '../lib/filters.svelte';
  import { narrow } from '../lib/viewport.svelte';
  import Icon from './Icon.svelte';
  import { edgeFade } from '../lib/fade.svelte';
  import SettingsFeatures from './SettingsFeatures.svelte';
  import SettingsChannels from './SettingsChannels.svelte';
  import SettingsBuoyancy from './SettingsBuoyancy.svelte';
  import SettingsPeople from './SettingsPeople.svelte';
  import SettingsMe from './SettingsMe.svelte';
  import SettingsTokens from './SettingsTokens.svelte';
  import SettingsTeams from './SettingsTeams.svelte';
  import { teams } from '../lib/teams.svelte';

  let {
    me,
    tab = '',
    onback,
    ontab,
    onme,
    features,
    onfeatures,
  }: {
    me: Person;
    /** la de la dirección; vacía elige la primera que corresponde */
    tab?: string;
    onback?: () => void;
    ontab?: (tab: string) => void;
    /** lo mío cambió: el resto de la aplicación lee a la persona de arriba */
    onme?: (p: Person) => void;
    features: Features;
    onfeatures?: (f: Features) => void;
  } = $props();

  const isLead = $derived(me.role === 'lead');
  /** Quien lidera un equipo conecta su canal, aunque no sea lead global. */
  const leadsATeam = $derived(teams.list.some((t) => t.role === 'lead'));

  // En el orden en que se pidieron. Flotabilidad y Usuarios sólo existen para
  // el lead global: son sus decisiones, y una tab que un member sólo puede
  // mirar sin tocar es ruido en su pantalla.
  const tabs = $derived([
    ...(isLead
      ? [
          { id: 'flotabilidad', label: 'Flotabilidad', face: '🫧' },
          { id: 'usuarios', label: 'Usuarios', face: '🚪' },
          { id: 'funciones', label: 'Funciones', face: '🛠' },
        ]
      : []),
    { id: 'equipos', label: 'Equipos', face: '👥' },
    ...(isLead || leadsATeam ? [{ id: 'canales', label: 'Canales', face: '💬' }] : []),
    { id: 'mi-usuario', label: 'Mi usuario', face: '' },
    // La dirección sigue siendo `tokens` (la enlaza «Conectar tu IA»); lo que se
    // lee es lo que conecta: tu agente.
    { id: 'tokens', label: 'Agente', face: '🤖' },
  ]);

  // Encogida o abierta, como la columna de proyectos (la misma preferencia); en
  // una pantalla angosta, siempre rail.
  let railed = $state(rail.get());
  const shut = $derived(narrow.on || railed);
  const layout = $derived(shut ? 'rail' : 'sidebar');
  // La lista se desvanece en el borde que aún tiene filas detrás, como la de proyectos.
  const listFade = edgeFade();

  // Una tab que no te corresponde —un enlace a `/ajustes/flotabilidad` pegado
  // a un member— cae en la primera que sí.
  const current = $derived(
    tabs.some((t) => t.id === tab) ? tab : isLead ? 'flotabilidad' : 'mi-usuario',
  );
</script>

<div class="screen" aria-label="ajustes">
  <div class="topbar">
    <button class="back" onclick={onback} aria-label="volver al board">
      <span aria-hidden="true">←</span> board
    </button>
    <h2 class="ttl">Ajustes</h2>
  </div>

  <div class="body">
    <Navigation {layout} class="set-nav">
      <Navigation.Content>
        <Navigation.Header>
          <Navigation.Trigger onclick={() => { railed = !railed; rail.set(railed); }} title={shut ? 'expandir' : 'colapsar'}>
            <PanelLeftIcon class={shut ? 'size-5' : 'size-4'} />
            {#if !shut}<span class="side-title">Categorías</span>{/if}
          </Navigation.Trigger>
        </Navigation.Header>
        <Navigation.Group class="cats" style={listFade.style} {@attach listFade.attach}>
          <Navigation.Menu>
            {#each tabs as t (t.id)}
              <div class="row" class:on={current === t.id}>
                <!-- Un ancla y no un `Navigation.Trigger`: es un LUGAR, con
                     dirección. Lleva a mano los `data-part` de Skeleton para
                     verse como sus filas (lo mismo que las fijadas de la
                     columna de proyectos). -->
                <a class="cat" data-scope="navigation" data-part="trigger" data-layout={layout}
                  href={settingsUrl(t.id)} title={t.label} aria-current={current === t.id ? 'page' : undefined}
                  onclick={(e) => {
                    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0 || !ontab) return;
                    e.preventDefault();
                    ontab(t.id);
                  }}>
                  {#if t.face}
                    <Icon of={t.face} size="22px" tint />
                  {:else}
                    <span class="me" aria-hidden="true">
                      {#if me.avatar}<img src={avatarUrl(me, '64x64')} alt="" />{:else}{(me.display_name || me.email || '?')[0].toUpperCase()}{/if}
                    </span>
                  {/if}
                  <span data-scope="navigation" data-part="trigger-text" data-layout={layout}>{t.label}</span>
                </a>
              </div>
            {/each}
          </Navigation.Menu>
        </Navigation.Group>
      </Navigation.Content>
    </Navigation>

    <main class="pane">
      <div class="column">
        <!-- El título de la sección, como el de cualquier pantalla: en display. -->
        <header class="sec-head">
          <h1 class="display sec-title">{tabs.find((t) => t.id === current)?.label}</h1>
          {#if current === 'equipos'}
            <!-- Qué es un equipo, a un clic: quien ya lo leyó no lo vuelve a
                 cargar encima de la lista cada vez que entra. -->
            <Tooltip positioning={{ placement: 'bottom-start' }} openDelay={80} closeDelay={80} interactive>
              <Tooltip.Trigger class="info" aria-label="Qué es un equipo">
                <InfoIcon class="size-4" />
              </Tooltip.Trigger>
              <Portal>
                <Tooltip.Positioner>
                  <Tooltip.Content class="team-help">
                    <p>
                      Un equipo es un <b>contexto</b>: sus proyectos, objetivos, secuencia, calendario, inbox de ideas y
                      canal de WhatsApp son suyos y no se mezclan con los de otro. El equipo en el que trabajas se elige
                      arriba de la columna de proyectos, y es también a dónde va lo que tu IA crea sin decir equipo.
                    </p>
                    <p>Un lead global es lead de todo equipo que no sea personal.</p>
                    <p>
                      Un <b>equipo personal</b> es tu contexto privado: sólo lo ves tú, ni siquiera otro lead global.
                      Para trabajar con alguien ahí, invítalo a un <b>proyecto</b> del equipo: verá ese proyecto y nada más.
                    </p>
                  </Tooltip.Content>
                </Tooltip.Positioner>
              </Portal>
            </Tooltip>
          {/if}
        </header>
        {#if current === 'flotabilidad'}<SettingsBuoyancy canEdit />
        {:else if current === 'usuarios'}<SettingsPeople {me} />
        {:else if current === 'funciones'}<SettingsFeatures {features} {onfeatures} />
        {:else if current === 'equipos'}<SettingsTeams {me} {features} />
        {:else if current === 'canales'}<SettingsChannels />
        {:else if current === 'mi-usuario'}<SettingsMe {me} {onme} />
        {:else}<SettingsTokens {me} />{/if}
      </div>
    </main>
  </div>
</div>

<style>
  .screen {
    --topbar-h: 46px;
    display: flex;
    flex-direction: column;
    height: 100dvh;
    overflow: hidden;
  }
  .topbar {
    display: flex;
    flex: none;
    align-items: center;
    gap: 0.75rem;
    height: var(--topbar-h);
    padding: 0 0.9rem;
    border-bottom: 1px solid var(--line);
    background: color-mix(in oklab, var(--bg) 82%, transparent);
    backdrop-filter: blur(8px);
  }
  .back {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted);
    font-size: 0.82rem;
  }
  .back:hover { color: var(--text); background: var(--hover); }
  .ttl { margin: 0; font-size: 0.95rem; font-weight: 700; }

  .body { flex: 1; min-height: 0; display: flex; align-items: stretch; }

  /* La columna: sin fondo, una línea a la derecha, todo el alto (las mismas
     reglas que la del hilo). `class="set-nav"` va a un componente, así que se
     ancla en `.body`, que sí es nuestro. */
  .body :global(.set-nav) { flex: none; height: 100%; display: flex; flex-direction: column; overflow: hidden; border-right: 1px solid var(--line); background: transparent; }
  .body :global(.set-nav [data-part='content'][data-layout='sidebar']) { flex: 1 1 auto; min-height: 0; display: flex; flex-direction: column; align-items: stretch; gap: 0.75rem; }
  .body :global(.set-nav [data-part='group']) { align-items: stretch; min-width: 0; }
  /* La LISTA desplaza, no la columna: las mismas reglas que la de proyectos
     (Shell.svelte). En rail el grupo es `display: contents` y no tiene caja que
     desplazar, así que se le da una. La barra va sobre el borde de la columna. */
  .body :global(.set-nav[data-layout='rail'] .cats) { display: flex; flex-direction: column; gap: 1rem; }
  .body :global(.set-nav .cats) {
    flex: 1 1 auto; min-height: 0; overflow-y: auto;
    width: auto; margin-inline: -1rem; padding-inline: 1rem 1.5rem;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in oklab, var(--muted) 35%, transparent) transparent;
  }
  .body :global(.set-nav[data-layout='rail'] .cats) { scrollbar-width: none; }
  .body :global(.set-nav[data-layout='rail'] .cats::-webkit-scrollbar) { display: none; }
  .side-title { font-size: 0.7rem; font-weight: 800; letter-spacing: 0.09em; text-transform: uppercase; color: var(--faint); }
  .row { display: flex; width: 100%; }
  .cat, .cat:visited { text-decoration: none; color: inherit; }
  /* La elegida lleva fondo y borde, como el proyecto elegido en la columna. */
  .row.on .cat { background: var(--hover); box-shadow: inset 0 0 0 1px var(--line); font-weight: 600; }
  .me { flex: none; display: grid; place-content: center; width: 22px; height: 22px; overflow: hidden; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.7rem; font-weight: 700; }
  .me img { width: 100%; height: 100%; object-fit: cover; }

  /* El contenido desplaza solo, en una columna legible y centrada: son
     formularios, y uno a todo el ancho de un monitor se lee de lado a lado. */
  /* `position: relative`: el pane es el bloque contenedor de lo que lleva dentro.
     Sin él, un hijo `position: absolute` (el `label.sr-only` de cada FilterPick)
     se posicionaba contra <body>, escapaba del recorte del pane y alargaba el
     documento: dos barras de scroll, la del pane y la de la página, en cuanto
     la lista de Usuarios pasaba del pliegue. */
  .pane { position: relative; flex: 1; min-width: 0; overflow-y: auto; }
  .sec-head { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1rem; }
  .sec-title { margin: 0; font-size: 1.9rem; line-height: 1.1; }
  .sec-head :global(.info) { display: grid; place-content: center; width: 26px; height: 26px; border-radius: 999px; color: var(--muted); }
  .sec-head :global(.info:hover), .sec-head :global(.info[data-state='open']) { background: var(--hover); color: var(--text); }
  /* El tooltip del tema es de una línea; éste es un párrafo. */
  :global([data-scope='tooltip'][data-part='content'].team-help) { display: grid; gap: 0.5rem; width: max-content; max-width: min(26rem, calc(100vw - 2rem)); white-space: normal; padding: 0.65rem 0.8rem; font-size: 0.8rem; font-weight: 500; line-height: 1.45; }
  :global(.team-help p) { margin: 0; }
  .column { width: 100%; max-width: 860px; margin: 0 auto; padding: 1.25rem 1rem 4rem; }

  /* Lo común a las cuatro secciones, una vez aquí. */
  .body :global(.set-section) { display: flex; flex-direction: column; gap: 1.25rem; }
  .body :global(.set-lead) { margin: 0; color: var(--muted); font-size: 0.88rem; line-height: 1.5; }
  /* Las secciones van PLANAS sobre el fondo, sin caja: una línea fina las
     separa. Una caja por bloque era ruido en una pantalla que sólo tiene
     bloques. */
  .body :global(.set-card) { padding: 0; border: 0; border-radius: 0; background: none; }
  .body :global(.set-card + .set-card),
  .body :global(.set-err + .set-card) { padding-top: 1.25rem; border-top: 1px solid var(--line); }
  .body :global(.set-card h3) { margin: 0 0 0.6rem; font-size: 0.95rem; font-weight: 700; }
  .body :global(.set-row) { display: flex; flex-wrap: wrap; align-items: center; gap: 0.6rem; }
  /* Un título dentro de una fila se centra con lo que lleva al lado (el logo, un
     badge): sin el margen inferior del título de una sección. */
  .body :global(.set-row > h3) { margin: 0; line-height: 1.2; }
  .body :global(.set-err) {
    margin: 0;
    padding: 0.45rem 0.7rem;
    border-radius: 8px;
    background: color-mix(in oklab, var(--color-error-500) 12%, transparent);
    color: var(--text);
    font-size: 0.82rem;
  }
  .body :global(.set-ok) { color: var(--muted); font-size: 0.82rem; }
</style>
