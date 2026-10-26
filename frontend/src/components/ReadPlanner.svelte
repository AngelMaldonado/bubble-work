<script lang="ts">
  import { teams } from '../lib/teams.svelte';
  import Loading from './Loading.svelte';
  import { onDestroy } from 'svelte';
  import { api, type Board, type CalendarEvent } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { isoDay, occurrences } from '../lib/repeat';
  import { lastOpenBubble } from '../lib/filters.svelte';
  import PlannerView from './PlannerView.svelte';
  import type { SeqThread } from './SequencePane.svelte';
  import SequenceCanvas from './SequenceCanvas.svelte';
  import FilterPick from './FilterPick.svelte';
  import type { Note } from './InboxSheet.svelte';
  import type { CalEvent } from './PlannerCalendar.svelte';
  import { projectColors } from '../lib/projectColors.svelte';

  let { inboxOnly = false, sequenceEnabled = false, onback, onsearch, onopenthread, onopenboard, onopenartifact }: {
    inboxOnly?: boolean; sequenceEnabled?: boolean;
    onback?: () => void; onsearch?: () => void;
    onopenthread?: (workspace: string, seq: number) => void;
    onopenboard?: (workspace: string) => void;
    onopenartifact?: (id: string) => void;
  } = $props();
  let board = $state<Board | null>(null);
  let inbox = $state<Note[]>([]);
  let calendar = $state<CalendarEvent[]>([]);
  let failure = $state('');
  let mode = $state('bubbles');
  let window = $state({ from: isoDay(new Date()), to: isoDay(new Date(new Date().getFullYear()+1,0,1)) });
  let generation = 0;
  let stopped = false;
  async function load() {
    const turn = ++generation;
    const owner = api.me?.id;
    try {
      const [data, dates] = await Promise.all([api.readPlanner(), api.calendarEvents()]);
      if (stopped || turn !== generation || owner !== api.me?.id) return;
      // Un equipo a la vez: lo de otro contexto no se mezcla. Sin equipos (una
      // base vieja), todo.
      const T = teams.current;
      const wsIn = new Set(data.board.workspaces.filter((w) => !T || teams.inScope(w)).map((w) => w.id));
      board = { ...data.board, workspaces: data.board.workspaces.filter((w) => wsIn.has(w.id)),
        bubbles: data.board.bubbles.filter((b) => wsIn.has(b.workspace)), threads: data.board.threads.filter((t) => wsIn.has(t.workspace)) };
      calendar = dates.filter((d) => !T || !d.team || teams.inView(d.team)); failure = '';
      inbox = data.inbox.filter((n) => !T || !n.team || teams.inView(n.team)).map((n) => ({ id: n.id, text: n.note, body: n.body,
        from: n.source_from || (n.captured_by === owner ? 'tú' : 'equipo'), when: n.created,
        artifacts: data.artifacts.filter((a) => a.inbox_item === n.id).map((a) => ({ id:a.id,title:a.title,pinned:a.pinned,expires_at:a.expires_at })),
      }));
    } catch (e) { if (!stopped && turn === generation) failure = String(e); }
  }
  let timer: ReturnType<typeof setTimeout> | undefined;
  function changed() { clearTimeout(timer); timer = setTimeout(load, 180); }
  const offs = ['bubbles','threads','inbox_items','artifacts','artifact_entries','calendar_events','memberships'].map((c) => live.watch(c,changed));
  load();
  let shownTeam = `${teams.current}.${teams.viewing}`;
  $effect(() => { const now = `${teams.current}.${teams.viewing}`; if (now !== shownTeam) { shownTeam = now; load(); } });
  onDestroy(() => { stopped = true; generation++; clearTimeout(timer); offs.forEach((off) => off()); });

  function address(workspace: string) { return board?.workspaces.find((w) => w.id === workspace)?.slug ?? ''; }
  function project(workspace: string) { return board?.workspaces.find((w) => w.id === workspace)?.name ?? ''; }
  const bubbles = $derived(board?.bubbles ?? []);
  const threads = $derived(board?.threads ?? []);
  const rows = $derived<SeqThread[]>(mode === 'bubbles'
    ? bubbles.map((b) => ({ id:b.id,name:b.name,project:project(b.workspace),workspace:b.workspace,bubble:'',priority:b.priority,sequence:b.sequence ?? 0,done:b.heat.lifecycle==='closed',due:b.due_date?.slice(0,10),assignees:[] }))
    : threads.map((t) => ({ id:t.id,seq:t.seq,name:t.name,project:project(t.workspace),workspace:t.workspace,bubble:bubbles.find((b) => b.id===t.bubble)?.name ?? '',priority:t.priority,sequence:t.sequence ?? 0,step:t.step,done:t.heat.lifecycle==='closed',due:t.due_date?.slice(0,10),assignees:[] })));
  function openRow(row: SeqThread) {
    if (mode === 'bubbles') { const b=bubbles.find((b) => b.id===row.id); if (b) { lastOpenBubble.set(b.id); onopenboard?.(address(b.workspace)); } }
    else { const t=threads.find((t) => t.id===row.id); if (t) onopenthread?.(address(t.workspace),t.seq); }
  }
  const events = $derived.by(() => {
    const out: CalEvent[] = [];
    for (const b of bubbles) if (b.due_date && b.heat.lifecycle !== 'closed') out.push({ id:'b:'+b.id,title:b.name,start:b.due_date.slice(0,10),allDay:true,color:projectColors.get(b.workspace)||undefined });
    for (const t of threads) if (t.due_date && t.heat.lifecycle !== 'closed') out.push({ id:'t:'+t.id,title:t.name,start:t.due_date.slice(0,10),allDay:true,color:projectColors.get(t.workspace)||undefined });
    for (const c of calendar) for (const day of occurrences(c.start,c.repeat ?? '',window.from,window.to)) out.push({ id:`e:${c.id}:${day}`,title:c.name,start:day,allDay:true });
    return out;
  });
</script>

{#snippet selectMode()}
  <div class="mode"><FilterPick label="Mostrar" bind:value={mode} items={[{value:'bubbles',label:'Módulos'},{value:'tasks',label:'Hilos'}]} allowEmpty={false} /></div>
{/snippet}
{#snippet line()}
  {#key mode}<SequenceCanvas threads={rows} readonly onopen={openRow} modes={selectMode} what={mode==='bubbles' ? 'módulos' : 'hilos'} />{/key}
{/snippet}
{#if failure}<p class="failure" role="alert">{failure} <button onclick={load}>Reintentar</button></p>{/if}
{#if !board && !failure}<Loading viewport label="Cargando planeador" />{:else}
<PlannerView {inbox} {events} readonly {inboxOnly} title="Planeador · sólo lectura" {onback} {onsearch} {onopenartifact}
  render={(md) => api.renderMarkdown(md)} boardPane={sequenceEnabled ? line : undefined}
  oncalrange={(from, to) => { if (from !== window.from || to !== window.to) window={from,to}; }}
  onpickevent={(id) => {
    const t = threads.find((t) => id === 't:'+t.id);
    if (t) onopenthread?.(address(t.workspace),t.seq);
    const b = bubbles.find((b) => id === 'b:'+b.id);
    if (b) { lastOpenBubble.set(b.id); onopenboard?.(address(b.workspace)); }
  }} />
{/if}
<style>
  .mode { max-width: 230px; }
  .failure { position: fixed; top: 3rem; left: 50%; transform: translateX(-50%); z-index: var(--z-chrome); padding: 0.75rem; background: var(--surface); color: var(--color-error-500); border-radius: 10px; }
  .failure button { text-decoration: underline; }
</style>
