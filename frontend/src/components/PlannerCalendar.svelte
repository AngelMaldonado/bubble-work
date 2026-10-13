<script lang="ts" module>
  export type CalEvent = {
    id: string;
    title: string;
    /** ISO date, or date + time for something that happens AT an hour */
    start: string;
    end?: string;
    allDay?: boolean;
    prio?: string;
    /** color personal del proyecto, si la persona eligió uno */
    color?: string;
  };
</script>

<script lang="ts">
  import Loading from './Loading.svelte';
  // The planner's calendar, over @event-calendar/core.
  //
  // Chosen because it is written in Svelte 5 — the framework this app already
  // runs on — so month, week and agenda cost 49 kB and no second runtime.
  //
  // Used as a COMPONENT, not through `createCalendar`. The package ships a
  // `svelte` export condition that resolves to a different entry point than the
  // default one: in a Svelte app you get `Calendar`, and the imperative
  // `createCalendar` simply is not there. Its options are a `$state` object and
  // changing a field is how you drive it — there is no method to call.
  //
  // Loaded on demand: a planner opened to look at the kanban should not
  // download a calendar.
  import '@event-calendar/core/index.css';
  import Icon from './Icon.svelte';
  import FilterPick from './FilterPick.svelte';
  import { shiftCalendarDate } from '../lib/calendarDate';

  let {
    events = [],
    view = $bindable('dayGridMonth'),
    readonly = false,
    onpick,
    onmove,
    onrange,
  }: {
    events?: CalEvent[];
    view?: string;
    /** Nobody may reschedule from here. Not a permission check — the server
     *  makes those — but the honest shape of a calendar somebody can only
     *  read: an event that moves under the pointer and springs back is worse
     *  than one that does not move. */
    readonly?: boolean;
    onpick?: (id: string) => void;
    /** Dragging an event is RESCHEDULING it, which is the whole reason a
     *  calendar beats a list of dates — so somebody has to write the new date
     *  down. Without this the event moved on screen and nowhere else. */
    onmove?: (id: string, day: string) => void;
    onrange?: (from: string, to: string) => void;
  } = $props();

  const VIEWS = [
    { k: 'dayGridMonth', label: 'mes' },
    { k: 'timeGridWeek', label: 'semana' },
    { k: 'listWeek', label: 'agenda' },
  ];

  /** A local calendar day as `2026-09-15`. NOT `toISOString`, which converts to
   *  UTC first and moves the date by one across most of the world's evenings. */
  function day(d: Date) {
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  }

  const shape = (list: CalEvent[]) =>
    list.map((e) => ({
      id: e.id,
      title: e.title,
      start: e.start,
      end: e.end ?? e.start,
      allDay: e.allDay ?? true,
      ...(e.color ? { backgroundColor: e.color, textColor: '#1f2430' } : {}),
      extendedProps: { prio: e.prio },
    }));

  // svelte-ignore state_referenced_locally
  const seeded = shape(events);

  let rangeTitle = $state('');
  let options = $state({
    view,
    // Our own header is outside the calendar, so its toolbar is empty rather
    // than hidden: an empty toolbar takes no space and needs no override.
    headerToolbar: { start: '', center: '', end: '' },
    firstDay: 1 as const, // the week starts on Monday here
    locale: 'es',
    height: '100%',
    // Dragging an event is rescheduling it, which is the whole reason a
    // calendar beats a list of dates.
    // svelte-ignore state_referenced_locally
    editable: !readonly,
    date: new Date(),
    events: seeded,
    datesSet: (info: { startStr: string; endStr: string; view: { title: string } }) => {
      rangeTitle = info.view.title.replace(/^./, (c) => c.toUpperCase());
      const end = new Date(info.endStr.slice(0, 10) + 'T00:00:00Z');
      end.setUTCDate(end.getUTCDate() - 1);
      onrange?.(info.startStr.slice(0, 10), end.toISOString().slice(0, 10));
    },
    allDayContent: 'todo el día',
    noEventsContent: 'Sin fechas esta semana',
    // Its ids are `string | number`; ours are strings, so it is coerced at the
    // boundary rather than everywhere downstream.
    eventClick: (info: { event: { id: string | number } }) => onpick?.(String(info.event.id)),
    eventDrop: (info: { event: { id: string | number; start: Date } }) =>
      onmove?.(String(info.event.id), day(info.event.start)),
    eventResize: (info: { event: { id: string | number; start: Date } }) =>
      onmove?.(String(info.event.id), day(info.event.start)),
  });

  // …and kept in step afterwards.
  //
  // This used to read `events` once and never again, which was right for a mock
  // whose only source of change was dragging inside the calendar. Against a
  // server it is wrong in the plainest way: a date set in a card was written,
  // reloaded, and never reached the calendar that exists to show it.
  //
  // Compared as text rather than by identity: the parent rebuilds this list on
  // every reload, so identity always differs and assigning on identity would
  // reset the calendar under the pointer.
  let last = JSON.stringify(seeded);
  $effect(() => {
    const next = shape(events);
    const text = JSON.stringify(next);
    if (text === last) return;
    last = text;
    options.events = next;
  });

  const year = $derived(options.date.getFullYear());
  const month = $derived(options.date.getMonth());
  const months = Array.from({ length: 12 }, (_, i) => ({ value: String(i), label: new Intl.DateTimeFormat('es', { month: 'long' }).format(new Date(2026, i, 1)) }));
  const years = $derived(Array.from({ length: 21 }, (_, i) => ({ value: String(year - 10 + i), label: String(year - 10 + i) })));
  function jump(y: string, m: string) { options.date = new Date(Number(y), Number(m), 1); }

  function go(next: string) {
    view = next;
    options.view = next;
    options.height = next === 'dayGridMonth' ? '100%' : 'auto';
  }

  /** Navigation updates the reactive date option; no component ref needed. */
  function step(dir: -1 | 0 | 1) {
    if (dir === 0) return void (options.date = new Date());
    const at = new Date(options.date);
    if (options.view === 'dayGridMonth') options.date = shiftCalendarDate(at, dir);
    else { at.setDate(at.getDate() + 7 * dir); options.date = at; }
  }
</script>

<div class="desk">
<div class="wrap paper" class:expanded={view !== 'dayGridMonth'}>
  <div class="binding" aria-hidden="true"><i></i><i></i><i></i><i></i><i></i></div>
  <header>
    <h2 class="display period"><Icon of="calendar" size="1.35em" tint /> {rangeTitle || 'Calendario'}</h2>
    <div class="date-picks">
      <FilterPick items={months} value={String(month)} label="Mes del calendario" allowEmpty={false} onchange={(m) => jump(String(year), m)} />
      <FilterPick items={years} value={String(year)} label="Año del calendario" allowEmpty={false} onchange={(y) => jump(y, String(month))} />
    </div>
    <div class="nav">
      <button onclick={() => (options.date = shiftCalendarDate(options.date, -12))} aria-label="Año anterior">«</button>
      <button onclick={() => step(-1)} aria-label="anterior">‹</button>
      <button class="today" onclick={() => step(0)}>hoy</button>
      <button onclick={() => step(1)} aria-label="siguiente">›</button>
      <button onclick={() => (options.date = shiftCalendarDate(options.date, 12))} aria-label="Año siguiente">»</button>
    </div>
    <div class="views" role="group" aria-label="vista del calendario">
      {#each VIEWS as v (v.k)}
        <button class:on={view === v.k} aria-pressed={view === v.k} onclick={() => go(v.k)}>
          {v.label}
        </button>
      {/each}
    </div>
  </header>

  <div class="cal">
    {#await import('@event-calendar/core')}
      <Loading label="Cargando calendario" />
    {:then EC}
      <EC.Calendar plugins={[EC.DayGrid, EC.TimeGrid, EC.List, EC.Interaction]} {options} />
    {/await}
  </div>
</div>
</div>

<style>
  .desk { flex: none; width: 100%; padding: 1.5rem 1.75rem 7rem; }
  .wrap {
    --paper: oklch(0.976 0.018 88);
    --paper-edge: oklch(0.91 0.026 85);
    --paper-ink: oklch(0.32 0.03 65);
    --paper-muted: oklch(0.5 0.025 65);
    --paper-line: oklch(0.57 0.025 70 / 0.2);
    position: relative;
    isolation: isolate;
    display: flex;
    flex-direction: column;
    width: 100%;
    max-width: 1160px;
    min-width: 0;
    aspect-ratio: 1;
    min-height: 680px;
    margin: 0 auto;
    padding: 0.8rem 1rem 1rem;
    color: var(--paper-ink);
    border: 1px solid var(--paper-edge);
    border-radius: 18px 18px 12px 12px;
    background: var(--paper);
    box-shadow: 0 18px 40px rgb(32 23 8 / 0.13), 0 3px 6px rgb(32 23 8 / 0.09);
  }
  :global([data-mode='dark']) .wrap {
    --paper: oklch(0.25 0.012 75);
    --paper-edge: oklch(0.34 0.018 75);
    --paper-ink: oklch(0.91 0.019 85);
    --paper-muted: oklch(0.71 0.023 85);
    --paper-line: oklch(0.74 0.02 85 / 0.19);
    box-shadow: 0 18px 40px rgb(0 0 0 / 0.3), 0 3px 6px rgb(0 0 0 / 0.2);
  }
  .wrap::before { content: ''; position: absolute; inset: 5px -3px -5px; z-index: -1; transform: rotate(-0.3deg); border: 1px solid var(--paper-edge); border-radius: 16px 16px 11px 11px; background: var(--paper); box-shadow: 0 3px 4px rgb(0 0 0 / 0.07); }
  .wrap::after { content: ''; position: absolute; inset: 0; border-radius: inherit; pointer-events: none; opacity: 0.13; background-image: radial-gradient(var(--paper-muted) 0.45px, transparent 0.7px); background-size: 5px 5px; }
  .binding { position: absolute; top: -12px; left: 50%; transform: translateX(-50%); display: flex; gap: 2.3rem; z-index: 2; }
  .binding i { width: 10px; height: 25px; border: 2px solid oklch(0.59 0.02 75); border-radius: 999px; background: linear-gradient(90deg, oklch(0.58 0.02 75), oklch(0.88 0.014 85) 45%, oklch(0.53 0.02 75)); box-shadow: 1px 2px 2px rgb(0 0 0 / 0.18), inset 1px 0 1px rgb(255 255 255 / 0.45); }
  .period { display: flex; align-items: center; gap: 0.5rem; margin: 0; font-size: 2rem; line-height: 1.2; }
  header, .cal { position: relative; z-index: 1; }
  header {
    position: sticky;
    top: 0;
    z-index: 3;
    background: var(--paper);
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    padding: 0.8rem 0.3rem 1rem;
    border-bottom: 1px solid var(--paper-line);
  }
  .date-picks { display: flex; gap: 0.35rem; margin-left: auto; }
  .date-picks :global([data-part='root']) { width: 11rem; }
  .date-picks :global([data-part='root']:last-child) { width: 7.5rem; }
  .date-picks :global([data-part='control']) { display: flex; }
  .date-picks :global([data-part='input']) { width: 100%; min-width: 0; color: var(--paper-ink); font-size: 0.85rem; }
  .date-picks :global([data-part='trigger']) { flex-shrink: 0; }
  .nav { margin-left: 0; display: flex; gap: 0.2rem; }
  .nav button, .views button {
    padding: 0.2rem 0.55rem;
    border: 1px solid transparent;
    border-radius: 8px;
    color: var(--paper-muted);
    font-size: 0.8rem;
  }
  .nav button:hover, .views button:hover { background: color-mix(in oklab, var(--paper-ink) 7%, transparent); color: var(--paper-ink); }
  .views { display: flex; gap: 0.2rem; }
  .views button.on {
    background: color-mix(in oklab, var(--paper-ink) 8%, var(--paper));
    color: var(--paper-ink);
    border-color: var(--paper-line);
    font-weight: 600;
  }
  /* A definite height, not just a flex share: the calendar's own `height: 100%`
     resolves against this box, and against an auto height it falls back to its
     natural size — a month grid squeezed into the top of an empty pane. */
  .cal { flex: 1 1 0; min-height: 0; height: 100%; }
  .cal :global(.ec) { height: 100%; }
  /* `.ec` is a flex column whose main region does not grow on its own, so a
     month grid sat at its natural height with the rest of the pane empty
     beneath it. */
  .cal :global(.ec-main) { flex: 1 1 auto; min-height: 0; }
  .cal :global(.ec-body) { height: 100%; }
  /* And in the month view the weeks share what is left evenly, rather than each
     being as tall as its fullest day. */
  .cal :global(.ec-day-grid .ec-grid) { height: 100%; grid-auto-rows: 1fr; }
  .wrap.expanded { aspect-ratio: auto; min-height: 680px; }
  .expanded .cal { flex: none; height: auto; min-height: 0; }
  .expanded .cal :global(.ec) { height: auto; }
  .expanded .cal :global(.ec-main), .expanded .cal :global(.ec-body) { height: auto; max-height: none; min-height: 0; overflow-y: visible; flex: none; }

  /* The calendar's own palette, replaced with ours. Left alone it brings its
     greys and its blue, and the pane reads as an embedded widget. */
  .cal :global(.ec) {
    --ec-border-color: var(--paper-line);
    --ec-bg-color: var(--paper);
    --ec-text-color: var(--paper-ink);
    --ec-accent-color: var(--accent);
    --ec-button-bg-color: var(--paper);
    --ec-button-text-color: var(--paper-muted);
    --ec-today-bg-color: color-mix(in oklab, var(--accent) 10%, transparent);
    --ec-highlight-color: color-mix(in oklab, var(--accent) 16%, transparent);
    --ec-event-bg-color: color-mix(in oklab, var(--accent) 22%, transparent);
    --ec-event-text-color: var(--paper-ink);
    font-family: inherit;
    font-size: 0.82rem;
  }
  .cal :global(.ec-day-head) { color: var(--paper-muted); font-weight: 700; text-transform: lowercase; }
  .cal :global(.ec-header) { padding-top: 0.35rem; }
  .cal :global(.ec-event) { box-shadow: 0 2px 3px rgb(0 0 0 / 0.06); }
  @media (max-width: 760px) {
    .desk { padding: 1.3rem 0.5rem 7rem; }
    .wrap { padding: 0.6rem; }
    .period { width: 100%; font-size: 1.75rem; }
    .date-picks { margin-left: 0; }
    .nav { margin-left: 0; }
    .views { margin-left: auto; }
  }
  .cal :global(.ec-other-month) { opacity: 0.45; }
  .cal :global(.ec-event) { border-radius: 6px; border: none; }
</style>
