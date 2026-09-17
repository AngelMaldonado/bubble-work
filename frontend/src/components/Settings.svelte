<script lang="ts">
  // Ajustes: una pantalla con tabs, como el planeador — su propia barra y su
  // propia vuelta, sin la columna de proyectos.
  //
  // La tab va en la DIRECCIÓN (`/ajustes/tokens`): «mira tus tokens» se manda
  // como enlace, y recargar no te devuelve a la primera tab. Por eso las tabs
  // de Skeleton van controladas: el valor lo dice la ruta y cambiar de tab es
  // navegar.
  import { Tabs } from '@skeletonlabs/skeleton-svelte';
  import type { Features, Person } from '../lib/api';
  import SettingsFeatures from './SettingsFeatures.svelte';
  import SettingsChannels from './SettingsChannels.svelte';
  import SettingsBuoyancy from './SettingsBuoyancy.svelte';
  import SettingsPeople from './SettingsPeople.svelte';
  import SettingsMe from './SettingsMe.svelte';
  import SettingsTokens from './SettingsTokens.svelte';

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

  // En el orden en que se pidieron. Flotabilidad y Usuarios sólo existen para
  // el lead global: son sus decisiones, y una tab que un member sólo puede
  // mirar sin tocar es ruido en su pantalla.
  const tabs = $derived([
    ...(isLead
      ? [
          { id: 'flotabilidad', label: 'Flotabilidad' },
          { id: 'usuarios', label: 'Usuarios' },
          { id: 'funciones', label: 'Funciones' },
          { id: 'canales', label: 'Canales' },
        ]
      : []),
    { id: 'mi-usuario', label: 'Mi usuario' },
    { id: 'tokens', label: 'Tokens' },
  ]);

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
    <Tabs value={current} onValueChange={(e: { value: string }) => ontab?.(e.value)}>
      <Tabs.List>
        {#each tabs as t (t.id)}
          <Tabs.Trigger value={t.id}>{t.label}</Tabs.Trigger>
        {/each}
        <Tabs.Indicator />
      </Tabs.List>

      {#if isLead}
        <Tabs.Content value="flotabilidad">
          <SettingsBuoyancy canEdit />
        </Tabs.Content>
        <Tabs.Content value="usuarios">
          <SettingsPeople {me} />
        </Tabs.Content>
        <Tabs.Content value="funciones">
          <SettingsFeatures {features} {onfeatures} />
        </Tabs.Content>
        <Tabs.Content value="canales">
          <SettingsChannels />
        </Tabs.Content>
      {/if}
      <Tabs.Content value="mi-usuario">
        <SettingsMe {me} {onme} />
      </Tabs.Content>
      <Tabs.Content value="tokens">
        <SettingsTokens {me} />
      </Tabs.Content>
    </Tabs>
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

  /* Una columna legible y centrada, que desplaza entera: son formularios, y un
     formulario a todo el ancho de un monitor se lee de lado a lado. */
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    width: 100%;
    max-width: 860px;
    margin: 0 auto;
    padding: 1.25rem 1rem 4rem;
  }
  .body :global([data-scope='tabs'][data-part='content']) { padding-top: 1.25rem; }

  /* Lo común a las cuatro secciones, una vez aquí. */
  .body :global(.set-section) { display: flex; flex-direction: column; gap: 1rem; }
  .body :global(.set-lead) { margin: 0; color: var(--muted); font-size: 0.88rem; line-height: 1.5; }
  .body :global(.set-card) {
    padding: 1rem;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface);
  }
  .body :global(.set-card h3) { margin: 0 0 0.6rem; font-size: 0.95rem; font-weight: 700; }
  .body :global(.set-row) { display: flex; flex-wrap: wrap; align-items: center; gap: 0.6rem; }
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
