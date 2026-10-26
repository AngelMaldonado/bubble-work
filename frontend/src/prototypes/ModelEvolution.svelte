<script lang="ts">
  // Sólo fixtures: /theme/model en DEV. No se monta App ni se consulta su API.
  import { untrack } from 'svelte';
  import { Dialog, Portal, Tabs } from '@skeletonlabs/skeleton-svelte';
  import FilterPick from '../components/FilterPick.svelte';
  import Icon from '../components/Icon.svelte';
  import BlobGraph from '../components/BlobGraph.svelte';
  import { bandFace, bandName } from '../lib/bands';
  import type { Lifecycle } from '../lib/api';
  import { bubbles, threads, groups, teamItems, levels, visibleLinks, type MapKind, type ModelNode } from './modelFixtures';

  type Scope = 'a' | 'b' | 'all';
  let { globalLead = true, initialScope = 'a', viewer = 'fixture' }: {
    globalLead?: boolean; initialScope?: Scope; viewer?: string;
  } = $props();
  const heatRank: Record<Lifecycle, number> = { hot: 3, dormant: 2, rip: 1, closed: 0 };
  const bands: Lifecycle[] = ['hot', 'dormant', 'rip', 'closed'];
  const kinds: MapKind[] = ['bubbles', 'threads'];
  let scope = $state<Scope>(untrack(() => initialScope === 'all' && !globalLead ? 'a' : initialScope));
  const key = untrack(() => `bubble.prototype.model.${viewer}.view`);
  let view = $state<'blobs' | 'flotation'>(untrack(() => {
    try { return localStorage.getItem(key) === 'blobs' ? 'blobs' : 'flotation'; }
    catch { return 'flotation'; }
  }));
  let kind = $state<MapKind>(untrack(() => {
    try { return localStorage.getItem(key + '.kind') === 'threads' ? 'threads' : 'bubbles'; }
    catch { return 'bubbles'; }
  }));
  let query = $state('');
  let onlyMine = $state(false);
  let visibility = $state('open');
  let selected = $state<ModelNode | null>(null);
  let creating = $state(false);
  let destination = $state('');
  let notice = $state('');
  const teamName = (id: string) => teamItems.find((t) => t.value === id)?.label ?? '';
  const scopeItems = $derived(globalLead ? [...teamItems, { value: 'all', label: 'Todos los equipos' }] : teamItems.slice(0, 1));
  $effect(() => { if (!globalLead && scope !== 'a') scope = 'a'; });
  $effect(() => { try { localStorage.setItem(key, view); localStorage.setItem(key + '.kind', kind); } catch { /* storage opcional */ } });
  const universe = $derived(kind === 'bubbles' ? bubbles : threads);
  const rows = $derived(universe.filter((p) =>
    (scope === 'all' || p.team === scope) &&
    (visibility === 'all' || p.life !== 'closed') &&
    (!onlyMine || p.mine) && `${p.name} ${p.bubbleName ?? ''}`.toLowerCase().includes(query.trim().toLowerCase()),
  ).sort((a, b) => heatRank[b.life] - heatRank[a.life] || a.name.localeCompare(b.name)));
  const links = $derived(visibleLinks(kind, rows));
  const shownTeams = $derived(scope === 'all' ? teamItems : teamItems.filter((t) => t.value === scope));
  function createPreview() { destination = scope === 'all' ? '' : scope; creating = true; }
</script>

<div class="prototype" data-scope={scope} data-view={view} data-kind={kind}>
  <header>
    <div><span class="eyebrow">Prototipo · datos inventados</span><h1>Todos los proyectos</h1></div>
    <div class="scope-pick"><FilterPick items={scopeItems} value={scope} allowEmpty={false} label="Equipo del prototipo" onchange={(v) => scope = v as Scope} /></div>
  </header>
  <p class="intro">Blobs por tag, como constelaciones, no cajas. Calor y furia siguen siendo señales distintas.</p>
  <Tabs value={kind} onValueChange={(e) => { if (e.value === 'bubbles' || e.value === 'threads') { kind = e.value; selected = null; } }}>
    <Tabs.List aria-label="Qué mostrar en el mapa">
      <Tabs.Trigger value="bubbles">Burbujas</Tabs.Trigger>
      <Tabs.Trigger value="threads">Hilos</Tabs.Trigger>
    </Tabs.List>
    <div class="filters">
      <input class="input" type="search" aria-label={`Buscar ${kind === 'bubbles' ? 'burbujas' : 'hilos'} del prototipo`} placeholder={kind === 'bubbles' ? 'Buscar una burbuja…' : 'Buscar un hilo o su burbuja…'} bind:value={query} />
      <button class="btn preset-tonal-surface" class:chosen={onlyMine} aria-pressed={onlyMine} onclick={() => onlyMine = !onlyMine}>Sólo mías</button>
      <div class="state-pick"><FilterPick items={[{ value: 'open', label: 'Sin cerrar' }, { value: 'all', label: 'Incluir cerrados' }]} bind:value={visibility} allowEmpty={false} label="Estado del prototipo" /></div>
      <button class="btn preset-filled-primary-500" onclick={createPreview}>Crear…</button>
    </div>
    {#if notice}<p class="notice" role="status">{notice}</p>{/if}
    <div class="map-summary" aria-live="polite"><span>{rows.length} {kind === 'bubbles' ? 'burbujas' : 'hilos'} visibles</span><span>{kind === 'bubbles' ? 'Relaciones proyectadas desde sus hilos' : 'Blob por su primer tag; compromisos propios'}</span></div>
    {#each kinds as tab (tab)}
      <Tabs.Content value={tab}>
        {#if kind === tab}{@render map()}{/if}
      </Tabs.Content>
    {/each}
  </Tabs>

  {#snippet piece(p: ModelNode)}
    <div class="piece band-{p.life}" data-piece={p.id}>
      <button class="open-piece" aria-label={`Abrir ${p.name}`} onclick={() => selected = p}>
        <Icon of={bandFace(p.life)} size="2.8rem" tint />
        <span class="identity"><strong>{p.name}</strong><small>{p.bubbleName ? `${p.bubbleName} · ` : ''}{scope === 'all' ? `${teamName(p.team)} · ` : ''}{p.project}</small><span class="heat">{bandName(p.life)} · {p.priority}</span></span>
      </button>
      <button class="pressure {p.level}" aria-label={`Furia de ${p.name}: ${levels[p.level]}`} onclick={() => selected = p}>{levels[p.level]}{p.id === 'billing' ? ' · vencida 1 d' : ''}</button>
    </div>
  {/snippet}
  {#snippet map()}
    {#if !rows.length}<p class="empty" role="status">No hay {kind === 'bubbles' ? 'burbujas' : 'hilos'} con estos filtros.</p>
    {:else if view === 'blobs'}
      <BlobGraph nodes={rows} {universe} {groups} {links} {kind} resetKey={`${kind}/${scope}`} onopen={(node) => selected = node} />
    {:else}
      <div class="bands" aria-label="Mapa por flotabilidad">
        {#each bands as band (band)}
          {@const items = rows.filter((p) => p.life === band)}
          {#if items.length}
            <section class="lane" data-band={band}>
              <h2><Icon of={bandFace(band)} tint /> {bandName(band)} <span>{items.length}</span></h2>
              <div class="group-pieces">{#each items as p (p.id)}{@render piece(p)}{/each}</div>
            </section>
          {/if}
        {/each}
      </div>
    {/if}
  {/snippet}

  <section class="sequences" aria-label="Secuencias paralelas por equipo">
    <div class="section-head"><h2>Qué toca por equipo</h2><span>Lectura: no existe una secuencia global para reordenar.</span></div>
    <div class="team-plans">
      {#each shownTeams as team (team.value)}
        <article><h3>{team.label}</h3><p><span>Ahora · {team.value === 'a' ? 'Integración' : 'Incorporación'}</span><b aria-hidden="true">→</b><span>Después · {team.value === 'a' ? 'Respaldo' : 'Red'}</span></p>
          {#if scope === 'all'}<button class="btn btn-sm preset-tonal-surface" onclick={() => scope = team.value as Scope}>Entrar a {team.label}</button>{:else}<small>Se ordenaría desde el planeador de este equipo.</small>{/if}
        </article>
      {/each}
    </div>
  </section>
  <footer><span>Fixtures · 19 oct 2026, 12:00 UTC · ciclo 24 h · sin llamadas a la API</span><span>{globalLead ? 'Simulación: lead global' : 'Simulación: miembro de Equipo A'}</span></footer>
  <nav class="view-toggle" aria-label="Vista de proyectos"><button class:active={view === 'blobs'} aria-pressed={view === 'blobs'} onclick={() => view = 'blobs'}>Blobs</button><button class:active={view === 'flotation'} aria-pressed={view === 'flotation'} onclick={() => view = 'flotation'}>Flotabilidad</button></nav>
</div>

<Dialog open={!!selected || creating} onOpenChange={(e) => { if (!e.open) { selected = null; creating = false; } }}>
  <Portal>
    <Dialog.Backdrop class="prototype-scrim" />
    <Dialog.Positioner class="prototype-pos">
      <Dialog.Content class="prototype-detail card bg-surface-100-900">
        <div class="detail-head"><Dialog.Title>{selected?.name ?? 'Elige un equipo concreto'}</Dialog.Title><Dialog.CloseTrigger class="btn btn-sm preset-tonal-surface" aria-label="Cerrar detalle del prototipo">✕</Dialog.CloseTrigger></div>
        {#if selected}
          <Dialog.Description>{selected.kind === 'threads' ? `Hilo de ${selected.bubbleName} · ` : 'Burbuja · '}{teamName(selected.team)} · {selected.project} · {selected.priority}</Dialog.Description>
          <p class="heat"><Icon of={bandFace(selected.life)} tint /> {bandName(selected.life)} — evidencia, no urgencia.</p>
          <p class="pressure {selected.level}">{levels[selected.level]} — máximo de las causas, no su suma.</p>
          <dl><dt>Compromiso</dt><dd>{selected.deadline}</dd><dt>Estancamiento</dt><dd>{selected.stagnation}</dd><dt>Prioridad</dt><dd>En este fixture, {selected.priority}{selected.kind === 'threads' ? ' se hereda de su burbuja' : ' viene de las entradas de la burbuja'}. Los tags no la modifican.</dd></dl>
          <p class="faint">El producto real reutilizaría su detalle. Este prototipo sólo explica fixtures: no recalcula políticas ni cambia asignaciones o fechas.</p>
        {:else}
          <Dialog.Description>«Todos los equipos» es una vista, no un destino. El prototipo sólo selecciona un equipo; no crea ningún registro.</Dialog.Description>
          <FilterPick items={teamItems} bind:value={destination} label="Equipo de destino del prototipo" any="Elige equipo…" portal={false} />
          <button class="btn preset-filled-primary-500" disabled={!destination} onclick={() => { scope = destination as Scope; creating = false; notice = `Equipo seleccionado: ${teamName(destination)}. No se creó ningún registro.`; }}>Usar este equipo</button>
        {/if}
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  .prototype { max-width: 1380px; margin: auto; padding: 2rem 2.4rem 6rem; }
  header, .filters, .section-head, footer, .detail-head, .map-summary { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
  h1, h2, h3 { font-family: var(--font-display); }
  h1 { font-size: clamp(1.8rem, 3vw, 2.6rem); line-height: 1.2; margin-top: 0.3rem; }
  .prototype :global([data-scope='tabs'][data-part='trigger']) { color: var(--muted); }
  .prototype :global([data-scope='tabs'][data-part='trigger'][aria-selected='true']) { color: var(--text); box-shadow: inset 0 -2px 0 var(--color-primary-500); }
  .eyebrow { font-size: 0.72rem; text-transform: uppercase; letter-spacing: 0.14em; color: var(--faint); }
  .scope-pick { width: 15rem; }
  .intro { color: var(--faint); font-size: 0.9rem; margin: 1.25rem 0; }
  .filters { justify-content: flex-start; flex-wrap: wrap; margin: 1.2rem 0 1rem; }
  .filters input { max-width: 23rem; flex: 1 1 12rem; }
  .state-pick { width: 12rem; }
  .chosen { outline: 1px solid var(--accent); }
  .map-summary { color: var(--faint); font-size: 0.72rem; flex-wrap: wrap; margin-bottom: 0.7rem; }
  .group-pieces { display: flex; gap: 0.8rem; flex-wrap: wrap; }
  .piece { display: flex; flex-direction: column; gap: 0.65rem; min-width: min(100%, 15rem); flex: 1 1 15rem; max-width: 24rem; padding: 0.95rem; border-radius: 1.35rem; background: var(--surface-solid); border: 1px solid var(--line); }
  .open-piece { display: flex; align-items: center; gap: 0.7rem; text-align: left; padding: 0.1rem; cursor: pointer; }
  .identity { display: flex; flex-direction: column; min-width: 0; gap: 0.15rem; }
  .identity strong { font-size: 1.08rem; }
  .identity small { color: var(--faint); font-size: 0.73rem; }
  .heat { font-size: 0.78rem; color: var(--accent); display: flex; gap: 0.4rem; align-items: center; }
  .pressure { align-self: start; font-size: 0.72rem; border-radius: 0.5rem; padding: 0.25rem 0.55rem; background: var(--surface); color: var(--faint); border: 1px solid var(--line); text-align: left; }
  button.pressure { cursor: pointer; }
  .pressure.warning { color: light-dark(#805100, #ffd68b); background: light-dark(#fff2d9, #493a24); border-color: light-dark(#efd6a5, #71562c); }
  .pressure.angry { color: light-dark(#94411c, #ffba99); background: light-dark(#fff0e8, #482e26); border-color: light-dark(#ebc4b3, #744939); }
  .pressure.furious { color: light-dark(#a02539, #ffadba); background: light-dark(#ffeaf0, #472732); border-color: light-dark(#eebcc7, #824050); }
  .bands { display: flex; flex-direction: column; gap: 1.4rem; }
  .lane { border-top: 1px solid var(--line); padding-top: 1rem; }
  .lane h2 { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1rem; font-size: 1.2rem; }
  .lane h2 span { color: var(--faint); font: 0.8rem var(--typo-base--font-family); }
  .sequences { margin-top: 2rem; padding-top: 1.5rem; border-top: 1px solid var(--line); }
  .section-head { margin-bottom: 1rem; flex-wrap: wrap; gap: 0.4rem; }
  .section-head h2 { font-size: 1.2rem; }
  .section-head > span, footer, article small { color: var(--faint); font-size: 0.76rem; }
  .team-plans { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr)); gap: 1rem; }
  article { border-left: 2px solid var(--line); padding-left: 1rem; }
  article h3 { font-size: 1.05rem; }
  article p { display: flex; align-items: center; gap: 0.6rem; font-size: 0.8rem; flex-wrap: wrap; margin: 0.4rem 0 0.7rem; }
  article p span { padding: 0.3rem 0.5rem; border-radius: 0.4rem; background: var(--surface); }
  article p b { color: var(--faint); }
  footer { flex-wrap: wrap; margin-top: 2rem; gap: 0.4rem; }
  .view-toggle { position: fixed; bottom: 1rem; left: 50%; transform: translateX(-50%); display: flex; gap: 0.4rem; z-index: 10; }
  .view-toggle button { border: 1px solid var(--line); padding: 0.6rem 1rem; border-radius: 0.6rem; background: var(--surface-solid); cursor: pointer; font-size: 0.85rem; }
  .view-toggle button.active { color: var(--color-primary-500); border-color: var(--color-primary-500); }
  .empty, .notice { padding: 1rem; color: var(--faint); }
  :global(.prototype-scrim) { position: fixed; inset: 0; background: rgb(0 0 0 / 0.35); z-index: 100; }
  :global(.prototype-pos) { position: fixed; inset: 0; display: flex; align-items: center; justify-content: center; padding: 1rem; z-index: 101; }
  :global(.prototype-detail) { width: min(36rem, 100%); padding: 1.5rem; display: flex; flex-direction: column; gap: 1rem; max-height: calc(100dvh - 2rem); overflow-y: auto; }
  .detail-head :global([data-part='title']) { font-family: var(--font-display); font-size: 1.5rem; }
  dl { font-size: 0.9rem; }
  dt { font-weight: 600; margin: 0.8rem 0 0.2rem; }
  dd { color: var(--faint); }
  .faint { font-size: 0.78rem; }
  button:focus-visible { outline: 2px solid var(--color-primary-500); outline-offset: 3px; }
  @media (max-width: 650px) {
    .prototype { padding: 1.1rem 0.9rem 6rem; }
    header { align-items: flex-start; flex-direction: column; gap: 0.9rem; }
    .scope-pick { width: 100%; }
    .filters { gap: 0.6rem; }
    .filters input { max-width: none; flex-basis: 100%; }
    .state-pick { flex: 1; min-width: 8rem; }
    .map-summary { font-size: 0.67rem; gap: 0.3rem; }
    .piece { max-width: none; }
  }
</style>
