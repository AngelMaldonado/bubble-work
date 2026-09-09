<script lang="ts">
  // The calendar, for somebody who does not run the plan.
  //
  // The same SCREEN as the planner — its bar, its way back, its pane and its
  // HUD — with one pane in it and nothing that writes. Not a second layout:
  // `PlannerView` in `readonly`, because two screens showing the same dates in
  // two shapes is how they start disagreeing about what a date looks like.
  //
  // The objectives, the inbox and the kanban of the whole department stay the
  // lead's. The DATES are not strategy: a thread with a due date is somebody's
  // week, and the person whose week it is should be able to look at it.
  import { api, type ThreadRecord, type Workspace } from '../lib/api';
  import PlannerView from './PlannerView.svelte';
  import type { CalEvent } from './PlannerCalendar.svelte';

  let {
    onback,
    onsearch,
    onopen,
  }: {
    onback?: () => void;
    onsearch?: () => void;
    /** go to the thread behind an event, by workspace slug and seq */
    onopen?: (workspace: string, seq: number) => void;
  } = $props();

  let threads = $state<ThreadRecord[]>([]);
  let places = $state<Workspace[]>([]);
  let error = $state('');

  async function load() {
    try {
      // The same call the planner makes. What comes back is what the RULES
      // allow: a member sees the threads of the workspaces they are in, so this
      // is their agenda without a single filter written here.
      const [th, ws] = await Promise.all([api.allThreads(), api.workspaces()]);
      threads = th;
      places = ws;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  const events = $derived<CalEvent[]>(
    threads
      .filter((t) => t.due_date)
      .map((t) => ({
        id: t.id,
        title: t.name,
        start: t.due_date!.slice(0, 10),
        allDay: true,
      })),
  );

  /** A day is a thread. Clicking one goes there, which is where the work is —
   *  the card sheet is the planner's, and it writes. */
  function open(id: string) {
    const t = threads.find((x) => x.id === id);
    const ws = places.find((w) => w.id === t?.workspace);
    if (t && ws) onopen?.(ws.slug, t.seq);
  }
</script>

{#if error}
  <p class="failed" role="alert">{error}</p>
{/if}

<PlannerView readonly title="Calendario" {events} {onback} {onsearch} onpickevent={open} />

<style>
  .failed {
    position: fixed;
    top: 1rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: var(--z-toast);
    margin: 0;
    padding: 0.6rem 0.8rem;
    border-radius: 10px;
    background: color-mix(in oklab, var(--p1, tomato) 16%, var(--surface-solid));
    color: var(--text);
    font-size: 0.85rem;
  }
</style>
