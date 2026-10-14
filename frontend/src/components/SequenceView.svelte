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
      const [b,ps] = await Promise.all([lead ? api.allBoard() : api.readPlanner().then((d) => d.board),api.people()]);
      if (stopped || turn!==generation || owner!==me.id || lead!==isLead) return;
      board=b; people=Object.fromEntries(ps.map((p) => [p.id,{name:p.display_name || p.email,avatar:avatarUrl(p,'64x64')}])); error='';
    } catch (e) { if (!stopped && turn===generation) error=String(e); }
  }
  $effect(() => { me.id; isLead; board=null; people={}; error=''; void load(); });
  let timer: ReturnType<typeof setTimeout> | undefined;
  const offs=['threads','bubbles','memberships'].map((c) => live.watch(c,() => {clearTimeout(timer);timer=setTimeout(load,180);}));
  onDestroy(() => {stopped=true;generation++;clearTimeout(timer);offs.forEach((off) => off());});
  // La secuencia es de UN equipo: sus proyectos (los de un equipo ajeno al
  // que te invitaron no entran a ninguna línea tuya).
  const inTeam = (ws: string) => !teams.current || board?.workspaces.find((w) => w.id === ws)?.team === teams.current;
  const own = (id?: string) => !!board?.bubbles.find((b) => b.id===id)?.owners?.includes(me.id);
  const mine = (t: { assignees?: string[]; bubble?: string }) => !!t.assignees?.includes(me.id) || own(t.bubble);
  function persons(ids: string[]) { return ids.map((id) => ({id,name:people[id]?.name ?? 'Persona no disponible',avatar:people[id]?.avatar || undefined})); }
  const rows = $derived<SeqThread[]>(mode==='tasks'
    ? (board?.threads ?? []).filter((t) => inTeam(t.workspace) && (scope==='all' || mine(t))).map((t) => ({id:t.id,seq:t.seq,name:t.name,project:board?.workspaces.find((w) => w.id===t.workspace)?.name ?? '',workspace:t.workspace,bubble:board?.bubbles.find((b) => b.id===t.bubble)?.name ?? '',priority:t.priority,sequence:t.sequence ?? 0,step:t.step,done:t.heat.lifecycle==='closed',due:t.due_date?.slice(0,10),assignees:persons(t.assignees ?? [])}))
    : (board?.bubbles ?? []).filter((b) => inTeam(b.workspace)).filter((b) => scope==='all' || b.owners?.includes(me.id) || board?.threads.some((t) => t.bubble===b.id && t.assignees?.includes(me.id))).map((b) => ({id:b.id,name:b.name,project:board?.workspaces.find((w) => w.id===b.workspace)?.name ?? '',workspace:b.workspace,bubble:'',priority:b.priority,sequence:b.sequence ?? 0,done:b.closed,due:b.due_date?.slice(0,10),assignees:persons(b.owners ?? [])})));
  function open(row: SeqThread) {
    if (mode==='tasks') { const t=board?.threads.find((t) => t.id===row.id);const ws=board?.workspaces.find((w) => w.id===t?.workspace);if(t&&ws) onopen?.(ws.slug,t.seq); }
    else { const b=board?.bubbles.find((b) => b.id===row.id);const ws=board?.workspaces.find((w) => w.id===b?.workspace);if(b&&ws) {lastOpenBubble.set(b.id);onopenboard?.(ws.slug);} }
  }
</script>
{#snippet modes()}
  <div class="reader-hud">
    <span class="reader-state">Sólo lectura · {scope==='all' ? 'Equipo' : 'Mi trabajo'}</span>
    {#if teams.list.length > 1}
      <select class="select team" value={teams.current} aria-label="Equipo de la secuencia"
        onchange={(e) => teams.set((e.currentTarget as HTMLSelectElement).value)}>
        {#each teams.list as t (t.id)}<option value={t.id}>{t.personal ? '🔒 ' : ''}{t.name}</option>{/each}
      </select>
    {/if}
    {#if isLead}<div class="seg" role="group" aria-label="alcance">
      <button class:on={scope==='all'} aria-pressed={scope==='all'} onclick={() => scope='all'}>Equipo</button>
      <button class:on={scope==='mine'} aria-pressed={scope==='mine'} onclick={() => scope='mine'}>Lo mío</button>
    </div>{/if}
    <div class="seg" role="group" aria-label="Contenido de la secuencia">
      <button class:on={mode==='tasks'} aria-pressed={mode==='tasks'} onclick={() => mode='tasks'}>Tareas</button>
      <button class:on={mode==='bubbles'} aria-pressed={mode==='bubbles'} onclick={() => mode='bubbles'}>Módulos</button>
    </div>
    {#if error}<span class="reader-error" role="alert">{error} <button onclick={load}>Reintentar</button></span>{/if}
  </div>
{/snippet}
<div class="sequence-reader">
  <div class="panel">{#key `${mode}.${scope}.${teams.current}`}<SequenceCanvas readonly edgeToEdge loading={!board && !error} threads={rows} onopen={open} {modes} what={mode==='tasks' ? 'hilos' : 'módulos'} />{/key}</div>
</div>
<style>
 .sequence-reader {height:100%;min-height:0;display:flex;flex-direction:column;}
 .reader-hud {display:flex;flex-direction:column;align-items:flex-start;gap:0.5rem;max-width:100%;}
 .reader-state {font-size:0.75rem;color:var(--muted);padding:0.2rem 0.5rem;border-radius:7px;background:var(--surface-solid);}
 .reader-error {font-size:0.8rem;color:var(--color-error-500);padding:0.5rem;background:var(--surface-solid);border-radius:7px;max-width:100%;overflow-wrap:anywhere;}
 .reader-error button {text-decoration:underline;}
 .panel {flex:1;min-height:0;}
 .team {font-size:0.82rem;font-weight:600;max-width:14rem;padding-block:0.2rem;}
 .seg {display:flex;flex-wrap:wrap;max-width:100%;padding:3px;border:1px solid var(--line);border-radius:10px;background:var(--surface-solid);}
 .seg button {padding:0.3rem 0.7rem;border-radius:7px;font-size:0.85rem;}
 .seg button.on {background:var(--hover);font-weight:700;}
</style>
