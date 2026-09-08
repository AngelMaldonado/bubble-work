<script lang="ts" module>
  export type CalEvent = {
    id: string;
    title: string;
    /** ISO date, or date + time for something that happens AT an hour */
    start: string;
    end?: string;
    allDay?: boolean;
    prio?: string;
  };
</script>

<script lang="ts">
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

  let {
    events = [],
    view = $bindable('dayGridMonth'),
    onpick,
  }: {
    events?: CalEvent[];
    view?: string;
    onpick?: (id: string) => void;
  } = $props();

  const VIEWS = [
    { k: 'dayGridMonth', label: 'mes' },
    { k: 'timeGridWeek', label: 'semana' },
    { k: 'listWeek', label: 'agenda' },
  ];

  // `events` is read once, on purpose: the calendar owns its copy from then on,
  // because dragging an event edits it in place and a re-derive would undo the
  // move the moment the parent re-rendered.
  // svelte-ignore state_referenced_locally
  const seeded = events.map((e) => ({
    id: e.id,
    title: e.title,
    start: e.start,
    end: e.end ?? e.start,
    allDay: e.allDay ?? true,
    extendedProps: { prio: e.prio },
  }));

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
    editable: true,
    date: new Date(),
    events: seeded,
    // Its ids are `string | number`; ours are strings, so it is coerced at the
    // boundary rather than everywhere downstream.
    eventClick: (info: { event: { id: string | number } }) => onpick?.(String(info.event.id)),
  });

  function go(next: string) {
    view = next;
    options.view = next;
  }

  /** Moving through time is a change to `date`, because that is the only handle
   *  the component gives — and it is enough. */
  function step(dir: -1 | 0 | 1) {
    if (dir === 0) return void (options.date = new Date());
    const at = new Date(options.date);
    if (options.view === 'dayGridMonth') at.setMonth(at.getMonth() + dir);
    else at.setDate(at.getDate() + 7 * dir);
    options.date = at;
  }
</script>

<div class="wrap">
  <header>
    <div class="nav">
      <button onclick={() => step(-1)} aria-label="anterior">‹</button>
      <button class="today" onclick={() => step(0)}>hoy</button>
      <button onclick={() => step(1)} aria-label="siguiente">›</button>
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
      <p class="loading">…</p>
    {:then EC}
      <EC.Calendar plugins={[EC.DayGrid, EC.TimeGrid, EC.List, EC.Interaction]} {options} />
    {/await}
  </div>
</div>

<style>
  .wrap { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.6rem;
    border-bottom: 1px solid var(--line);
  }
  .nav { display: flex; gap: 0.2rem; }
  .nav button, .views button {
    padding: 0.2rem 0.55rem;
    border: 1px solid transparent;
    border-radius: 8px;
    color: var(--muted);
    font-size: 0.8rem;
  }
  .nav button:hover, .views button:hover { background: var(--hover); color: var(--text); }
  .views { margin-left: auto; display: flex; gap: 0.2rem; }
  .views button.on {
    background: var(--hover);
    color: var(--text);
    border-color: var(--line);
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
  .loading { padding: 1rem; color: var(--faint); }

  /* The calendar's own palette, replaced with ours. Left alone it brings its
     greys and its blue, and the pane reads as an embedded widget. */
  .cal :global(.ec) {
    --ec-border-color: var(--line);
    --ec-bg-color: transparent;
    --ec-text-color: var(--text);
    --ec-accent-color: var(--accent);
    --ec-button-bg-color: var(--surface);
    --ec-button-text-color: var(--muted);
    --ec-today-bg-color: color-mix(in oklab, var(--accent) 10%, transparent);
    --ec-highlight-color: color-mix(in oklab, var(--accent) 16%, transparent);
    --ec-event-bg-color: color-mix(in oklab, var(--accent) 22%, transparent);
    --ec-event-text-color: var(--text);
    font-family: inherit;
    font-size: 0.82rem;
  }
  .cal :global(.ec-day-head) { color: var(--faint); font-weight: 700; text-transform: lowercase; }
  .cal :global(.ec-other-month) { opacity: 0.45; }
  .cal :global(.ec-event) { border-radius: 6px; border: none; }
</style>
