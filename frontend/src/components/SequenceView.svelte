<script lang="ts">
  // La secuencia, desde la columna: qué va ahora y qué sigue.
  //
  // El planeador la ARMA; esta pantalla sólo la enseña. El lead global ve la de
  // toda la organización y puede quedarse con lo suyo. Un member ve sólo «Lo
  // mío»: viene a saber qué le toca hacer, no a supervisar la línea entera.
  // Los números de paso son los de la línea completa, no los de lo filtrado:
  // «ahora» es ahora para todos.
  //
  // «Lo mío» es un hilo asignado a quien mira o, si el hilo no tiene a nadie
  // asignado, uno cuya burbuja está a su cargo: una pieza sin dueño propio es
  // de quien responde por el cuerpo de trabajo.
  import { api, avatarUrl, type Board, type Person } from '../lib/api';
  import SequencePane, { type SeqThread } from './SequencePane.svelte';

  let {
    me,
    onopen,
  }: {
    me: Person;
    /** abrir un hilo, por su proyecto y su número */
    onopen?: (slug: string, seq: number) => void;
  } = $props();

  const isLead = $derived(me.role === 'lead');
  // svelte-ignore state_referenced_locally
  let scope = $state<'all' | 'mine'>(me.role === 'lead' ? 'all' : 'mine');
  // Un member no elige: sólo hay «Lo mío».
  $effect(() => {
    if (!isLead) scope = 'mine';
  });

  let board = $state<Board | null>(null);
  let people = $state<Record<string, { name: string; avatar: string }>>({});
  let error = $state('');

  $effect(() => {
    Promise.all([api.allBoard(), api.people()])
      .then(([b, ps]) => {
        board = b;
        people = Object.fromEntries(
          ps.map((p) => [p.id, { name: p.display_name || p.email, avatar: avatarUrl(p, '64x64') }]),
        );
      })
      .catch((e) => (error = (e as Error).message));
  });

  const threads = $derived<SeqThread[]>(
    (board?.threads ?? [])
      .filter((t) => (t.sequence ?? 0) > 0)
      .filter((t) => scope === 'all' || isMine(t))
      .map((t) => ({
        id: t.id,
        seq: t.seq,
        name: t.name,
        project: board?.workspaces.find((w) => w.id === t.workspace)?.name ?? '',
        bubble: board?.bubbles.find((b) => b.id === t.bubble)?.name ?? '',
        priority: t.priority ?? '',
        sequence: t.sequence ?? 0,
        step: t.step || undefined,
        done: t.heat.lifecycle === 'closed',
        assignees: (t.assignees ?? []).map((id) => ({
          id,
          name: people[id]?.name ?? '',
          avatar: people[id]?.avatar || undefined,
        })),
      })),
  );

  function isMine(t: { assignees?: string[]; bubble?: string }) {
    const assigned = t.assignees ?? [];
    if (assigned.length) return assigned.includes(me.id);
    return !!board?.bubbles.find((b) => b.id === t.bubble)?.owners?.includes(me.id);
  }

  function open(t: SeqThread) {
    const th = board?.threads.find((x) => x.id === t.id);
    const slug = board?.workspaces.find((w) => w.id === th?.workspace)?.slug;
    if (th && slug) onopen?.(slug, th.seq);
  }
</script>

<div class="wrap">
  <header class="head">
    <h1 class="display text-2xl">🧭 Secuencia</h1>
    <p class="faint text-sm">
      {scope === 'all'
        ? 'El orden de ejecución de toda la organización. Se arma en el planeador.'
        : 'Lo que te toca, en el orden en que va: los hilos asignados a ti, y los sin asignar de las burbujas a tu cargo.'}
    </p>
    {#if isLead}
      <div class="seg" role="group" aria-label="alcance">
        <button class:on={scope === 'all'} aria-pressed={scope === 'all'} onclick={() => (scope = 'all')}>
          Organización
        </button>
        <button class:on={scope === 'mine'} aria-pressed={scope === 'mine'} onclick={() => (scope = 'mine')}>
          Lo mío
        </button>
      </div>
    {/if}
  </header>

  {#if error}<p class="err" role="alert">{error}</p>{/if}

  {#if board}
    <div class="panel">
      <SequencePane readonly title="Pasos" {threads} onopen={open} />
    </div>
  {:else if !error}
    <p class="faint text-sm">leyendo…</p>
  {/if}
</div>

<style>
  .wrap { max-width: 760px; margin: 0 auto; padding: 2rem 1.5rem 6rem; }
  .head { display: flex; flex-direction: column; align-items: center; gap: 0.5rem; margin-bottom: 1.5rem; text-align: center; }
  .head p { margin: 0; }
  .seg {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--surface);
  }
  .seg button { padding: 0.2rem 0.7rem; border-radius: 7px; color: var(--faint); font-size: 0.8rem; }
  .seg button.on { background: var(--hover); color: var(--text); font-weight: 600; }
  .panel { min-height: 60vh; }
  .err {
    padding: 0.45rem 0.7rem;
    border-radius: 8px;
    background: color-mix(in oklab, var(--color-error-500) 12%, transparent);
    font-size: 0.82rem;
  }
</style>
