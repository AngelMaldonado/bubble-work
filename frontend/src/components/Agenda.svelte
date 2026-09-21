<script lang="ts">
  // The calendar, for somebody who does not run the plan.
  //
  // The same SCREEN as the planner — its bar, its way back, its pane and its
  // HUD — with one pane in it and nothing that writes. Not a second layout:
  // `PlannerView` in `readonly`, because two screens showing the same dates in
  // two shapes is how they start disagreeing about what a date looks like.
  //
  // The objectives, the inbox and the kanban of the whole department stay the
  // lead's. The DATES are not strategy: a bubble due on the 30th is somebody's
  // month, and the people doing it should be able to look at it.
  import { api, type BubbleRecord, type CalendarEvent, type Workspace } from '../lib/api';
  import { isoDay, occurrences } from '../lib/repeat';
  import { lastOpenBubble } from '../lib/filters.svelte';
  import PlannerView from './PlannerView.svelte';
  import type { CalEvent } from './PlannerCalendar.svelte';

  let {
    onback,
    onsearch,
    onopen,
  }: {
    onback?: () => void;
    onsearch?: () => void;
    /** go to the bubble behind an event: its workspace's board, with it open */
    onopen?: (workspace: string) => void;
  } = $props();

  let bubbles = $state<BubbleRecord[]>([]);
  let places = $state<Workspace[]>([]);
  let dates = $state<CalendarEvent[]>([]);
  let error = $state('');

  async function load() {
    try {
      // The same call the planner makes. What comes back is what the RULES
      // allow: a member sees the bubbles of the workspaces they are in, so this
      // is their agenda without a single filter written here.
      // Las juntas del departamento las ve todo el mundo: no son trabajo de
      // nadie en particular y son justo lo que alguien que no lleva el plan
      // necesita saber de una fecha.
      const [bs, ws, ce] = await Promise.all([
        api.allBubbles(),
        api.workspaces(),
        api.calendarEvents(),
      ]);
      bubbles = bs;
      places = ws;
      dates = ce;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  /** La misma ventana que el planeador, y por la misma razón: el componente del
   *  calendario no dice hacia fuera qué mes está mirando. */
  const window = $derived.by(() => {
    const now = new Date();
    return {
      from: isoDay(new Date(now.getFullYear(), now.getMonth() - 2, 1)),
      to: isoDay(new Date(now.getFullYear() + 1, now.getMonth() + 2, 0)),
    };
  });

  const events = $derived<CalEvent[]>([
    ...dates.flatMap((e) =>
      occurrences(e.start, e.repeat ?? '', window.from, window.to).map((day) => ({
        id: `cal:${e.id}:${day}`,
        title: `📅 ${e.name}`,
        start: day,
        allDay: true,
      })),
    ),
    ...bubbles
      .filter((b) => b.due_date && !b.closed_at)
      .map((b) => ({
        id: b.id,
        title: b.name,
        start: b.due_date!.slice(0, 10),
        allDay: true,
      })),
  ]);

  /** A day is a bubble. Clicking one goes to its board with it open — the
   *  card sheet is the planner's, and it writes. The board opens whatever
   *  bubble was left open, so that is how this one is handed over. */
  function open(id: string) {
    const b = bubbles.find((x) => x.id === id);
    const ws = places.find((w) => w.id === b?.workspace);
    if (!b || !ws) return;
    lastOpenBubble.set(b.id);
    onopen?.(ws.slug);
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
