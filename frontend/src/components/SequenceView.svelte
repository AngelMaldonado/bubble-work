<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, avatarUrl, type Board, type Person } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { lastOpenBubble } from '../lib/filters.svelte';
  import SequenceCanvas from './SequenceCanvas.svelte';
  import type { SeqThread } from './SequencePane.svelte';
  import { teams } from '../lib/teams.svelte';

  let { me, onopen, onopenboard }: { me: Person; onopen?: (slug: string, seq: number) => void; onopenboard?: (slug: string) => void } = $props();
  const isLead = $derived(me.role === 'lead');
  // svelte-ignore state_referenced_locally
  let scope = $state<'all' | 'mine'>(me.role === 'lead' ? 'all' : 'mine');
  let mode = $state<'tasks' | 'bubbles'>('tasks');
  $effect(() => { if (!isLead) scope = 'mine'; });
  let board = $state<Board | null>(null);
  let people = $state<Record<string, { name: string; avatar: string }>>({});
  let error = $state('');
  let generation = 0;
  let stopped = false;
  async function load() {
    const turn=++generation, owner=me.id, lead=isLead;
    try {
      // En «Todos los equipos» lo personal ni viaja: lo recorta el servidor.
      const [b,ps] = await Promise.all([lead ? api.allBoard(teams.viewing ? 'shared' : '') : api.readPlanner().then((d) => d.board),api.people()]);
      if (stopped || turn!==generation || owner!==me.id || lead!==isLead) return;
      board=b; people=Object.fromEntries(ps.map((p) => [p.id,{name:p.display_name || p.email,avatar:avatarUrl(p,'64x64')}])); error='';
    } catch (e) { if (!stopped && turn===generation) error=String(e); }
  }
  $effect(() => { me.id; isLead; teams.viewing; board=null; people={}; error=''; void load(); });
  let timer: ReturnType<typeof setTimeout> | undefined;
  const offs=['threads','bubbles','memberships'].map((c) => live.watch(c,() => {clearTimeout(timer);timer=setTimeout(load,180);}));
  onDestroy(() => {stopped=true;generation++;clearTimeout(timer);offs.forEach((off) => off());});
  // La secuencia es de UN equipo: sus proyectos (los de un equipo ajeno al
  // que te invitaron no entran a ninguna línea tuya).
  const teamOf = (ws: string) => board?.workspaces.find((w) => w.id === ws)?.team;
  const inTeam = (ws: string) => !teams.current || teamOf(ws) === teams.current;
  const own = (id?: string) => !!board?.bubbles.find((b) => b.id===id)?.owners?.includes(me.id);
  const mine = (t: { assignees?: string[]; bubble?: string }) => !!t.assignees?.includes(me.id) || own(t.bubble);
  function persons(ids: string[]) { return ids.map((id) => ({id,name:people[id]?.name ?? 'Persona no disponible',avatar:people[id]?.avatar || undefined})); }
  // «Todos los equipos»: la secuencia de UN equipo a la vez, en el lienzo
  // entero; el título es el menú para cambiar de equipo. No hay una línea
  // global que las mezcle, y reordenar es de quien entra al equipo. Cuál se
  // está viendo es del navegador: no toca `prefs.team`.
  const pickKey = $derived(`bubble.sequence.team.${me.id}`);
  let picked = $state('');
  $effect(() => { try { picked = localStorage.getItem(pickKey) ?? ''; } catch { picked = ''; } });
  function pick(id: string) { picked = id; try { localStorage.setItem(pickKey, id); } catch { /* sin almacenamiento: sólo esta visita */ } }
  const shown = $derived(teams.viewing
    ? (teams.sharedTeams.find((t) => t.id === picked) ?? teams.sharedTeams[0] ?? null)
    : null);
  const shownRows = $derived(shown ? rowsWhere((ws) => teamOf(ws) === shown.id) : []);
  const rows = $derived<SeqThread[]>(rowsWhere(inTeam));
  function rowsWhere(inTeam: (ws: string) => boolean): SeqThread[] { return mode==='tasks'
    ? (board?.threads ?? []).filter((t) => inTeam(t.workspace) && (scope==='all' || mine(t))).map((t) => ({id:t.id,seq:t.seq,name:t.name,project:board?.workspaces.find((w) => w.id===t.workspace)?.name ?? '',workspace:t.workspace,bubble:board?.bubbles.find((b) => b.id===t.bubble)?.name ?? '',priority:t.priority,sequence:t.sequence ?? 0,step:t.step,done:t.heat.lifecycle==='closed',due:t.due_date?.slice(0,10),assignees:persons(t.assignees ?? [])}))
    : (board?.bubbles ?? []).filter((b) => inTeam(b.workspace)).filter((b) => scope==='all' || b.owners?.includes(me.id) || board?.threads.some((t) => t.bubble===b.id && t.assignees?.includes(me.id))).map((b) => ({id:b.id,name:b.name,project:board?.workspaces.find((w) => w.id===b.workspace)?.name ?? '',workspace:b.workspace,bubble:'',priority:b.priority,sequence:b.sequence ?? 0,done:b.closed,due:b.due_date?.slice(0,10),assignees:persons(b.owners ?? [])})); }
  function open(row: SeqThread) {
    if (mode==='tasks') { const t=board?.threads.find((t) => t.id===row.id);const ws=board?.workspaces.find((w) => w.id===t?.workspace);if(t&&ws) onopen?.(ws.slug,t.seq); }
    else { const b=board?.bubbles.find((b) => b.id===row.id);const ws=board?.workspaces.find((w) => w.id===b?.workspace);if(b&&ws) {lastOpenBubble.set(b.id);onopenboard?.(ws.slug);} }
  }
</script>
{#snippet modes()}
  <!-- En la píldora de abajo al centro. El equipo es el de la columna; en
       «Todos los equipos» se elige en el título. Quien no es lead sólo ve lo
       suyo, y se dice. -->
  <div class="reader-hud">
    {#if !isLead}<span class="reader-state">Lo mío</span>{/if}
    {#if isLead}<div class="seg" role="group" aria-label="alcance">
      <button class:on={scope==='all'} aria-pressed={scope==='all'} onclick={() => scope='all'}>Equipo</button>
      <button class:on={scope==='mine'} aria-pressed={scope==='mine'} onclick={() => scope='mine'}>Lo mío</button>
    </div>{/if}
    <div class="seg" role="group" aria-label="Contenido de la secuencia">
      <button class:on={mode==='tasks'} aria-pressed={mode==='tasks'} onclick={() => mode='tasks'}>Tareas</button>
      <button class:on={mode==='bubbles'} aria-pressed={mode==='bubbles'} onclick={() => mode='bubbles'}>Módulos</button>
    </div>
    <!-- Aquí sólo se lee: para reordenarla se entra al equipo. -->
    {#if shown}<button class="enter" onclick={() => teams.set(shown.id)}>Entrar a {shown.name}</button>{/if}
    {#if error}<span class="reader-error" role="alert">{error} <button onclick={load}>Reintentar</button></span>{/if}
  </div>
{/snippet}
<div class="sequence-reader">
  {#if shown}
    <section class="panel" aria-label={`Secuencia de ${shown.name}`}>
      {#key `${mode}.${scope}.${shown.id}`}<SequenceCanvas readonly edgeToEdge modesBottom loading={!board && !error} threads={shownRows} onopen={open} {modes}
        title={`🧭 ${shown.name}`}
        titleMenu={{ label: 'Secuencia del equipo', value: shown.id, options: teams.sharedTeams.map((t) => ({ value: t.id, label: t.name })), onselect: pick }}
        what={mode==='tasks' ? 'hilos' : 'módulos'} />{/key}
    </section>
  {:else}
    <div class="panel">{#key `${mode}.${scope}.${teams.current}`}<SequenceCanvas readonly edgeToEdge modesBottom loading={!board && !error} threads={rows} onopen={open} {modes} what={mode==='tasks' ? 'hilos' : 'módulos'} />{/key}</div>
  {/if}
</div>
<style>
 .sequence-reader {height:100%;min-height:0;display:flex;flex-direction:column;}
 .reader-hud {display:contents;}
 .reader-state {font-size:0.8rem;font-weight:700;color:var(--muted);padding:0.3rem 0.7rem;border-radius:10px;border:1px solid var(--line);background:var(--surface-solid);}
 .reader-error {font-size:0.8rem;color:var(--color-error-500);padding:0.5rem;background:var(--surface-solid);border-radius:7px;max-width:100%;overflow-wrap:anywhere;}
 .reader-error button {text-decoration:underline;}
 .panel {flex:1;min-height:0;}
 .enter {padding:0.3rem 0.8rem;border-radius:10px;border:1px solid var(--line);background:var(--surface-solid);font-size:0.85rem;font-weight:700;}
 .enter:hover {background:var(--hover);}
 .seg {display:flex;flex-wrap:wrap;max-width:100%;padding:3px;border:1px solid var(--line);border-radius:10px;background:var(--surface-solid);}
 .seg button {padding:0.3rem 0.7rem;border-radius:7px;font-size:0.85rem;}
 .seg button.on {background:var(--hover);font-weight:700;}
</style>
