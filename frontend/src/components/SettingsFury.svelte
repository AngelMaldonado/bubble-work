<script lang="ts">
  // Furia de IA: las reglas que deciden cuándo algo importante se está
  // rompiendo (internal/fury). Las edita el lead global, iguales para todos los
  // equipos; los demás las leen para entender por qué algo está furioso.
  //
  // La vista previa calcula con las reglas que se están editando, aunque la
  // función esté apagada: se calibra con datos reales antes de que nada brille.
  import { Switch } from '@skeletonlabs/skeleton-svelte';
  import FuryLevelPick from './FuryLevelPick.svelte';
  import Loading from './Loading.svelte';
  import Icon from './Icon.svelte';
  import { furyIcon, iconSrc } from '../lib/icons';
  import { api, type FuryLevel, type FuryPreview, type FuryRules, type Person } from '../lib/api';

  let { me }: { me: Person } = $props();
  const canEdit = $derived(me.role === 'lead');

  let on = $state(false);
  let rules = $state<FuryRules | null>(null);
  let saved = $state('');
  let levels = $state<FuryLevel[]>([]);
  let error = $state('');
  let saving = $state(false);
  let preview = $state<FuryPreview | null>(null);
  let previewing = $state(false);

  async function load() {
    try {
      const r = await api.furyRules();
      on = r.on;
      rules = r.rules;
      saved = JSON.stringify(r.rules);
      levels = r.levels;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  const dirty = $derived(!!rules && JSON.stringify(rules) !== saved);
  // Lo que enciende el semáforo de la oficina con cada nivel.
  const LIGHT = ['luz verde', 'luz amarilla', 'luz roja', 'luz roja parpadeando'];

  // La vista previa sigue a lo que se edita, con un respiro para no pedirla en
  // cada tecla.
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const snapshot = rules ? JSON.stringify(rules) : '';
    if (!canEdit || !snapshot) return;
    clearTimeout(timer);
    timer = setTimeout(async () => {
      previewing = true;
      try {
        preview = await api.furyPreview(JSON.parse(snapshot));
        error = '';
      } catch (e) {
        error = (e as Error).message;
      } finally {
        previewing = false;
      }
    }, 350);
    return () => clearTimeout(timer);
  });

  async function save(reset = false) {
    if (!rules) return;
    saving = true;
    try {
      const r = await api.saveFuryRules(rules, reset);
      rules = r.rules;
      saved = JSON.stringify(r.rules);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      saving = false;
    }
  }

  const lv = (n: number) => levels[n] ?? { level: n, name: '', emoji: '', color: '', blink: false };
</script>

{#snippet level(get: () => number, set: (v: number) => void, label: string)}
  <FuryLevelPick value={get()} {levels} {label} {canEdit} onchange={set} />
{/snippet}

{#snippet num(get: () => number, set: (v: number) => void, label: string, unit: string)}
  <label class="num">
    <input class="input" type="number" min="1" value={get()} disabled={!canEdit} aria-label={label}
      oninput={(e) => { const v = Number((e.currentTarget as HTMLInputElement).value); if (v >= 1) set(Math.round(v)); }} />
    <span>{unit}</span>
  </label>
{/snippet}

{#snippet toggle(get: () => boolean, set: (v: boolean) => void, title: string)}
  <Switch checked={get()} disabled={!canEdit} onCheckedChange={(e: { checked: boolean }) => set(e.checked)}>
    <Switch.Control><Switch.Thumb /></Switch.Control>
    <Switch.Label><b>{title}</b></Switch.Label>
    <Switch.HiddenInput />
  </Switch>
{/snippet}

<section class="set-section">
  <p class="set-lead">
    La IA se enoja cuando algo importante se está rompiendo, y lo dice con un nivel. Se calcula sola
    con estas reglas, siempre dice por qué, y se apaga cuando se arregla la causa. No cambia el calor
    ni el orden del tablero. «Prioridad alta» es P1 o P2.
  </p>

  <!-- Los cuatro ánimos de la IA, en grande: la cabeza, su nombre y el LED que
       enciende el semáforo de la oficina. -->
  <ul class="gallery" aria-label="Niveles de furia">
    {#each levels as l (l.level)}
      <li class="lvface">
        <img class="big" class:fury-unleashed={l.level === 3} src={iconSrc(furyIcon(l.level), 'tint')} alt="" />
        <b class="display lvname">{l.name}</b>
        <span class="light" title={LIGHT[l.level]}><span class="led {l.color}" class:blink={l.blink}></span>{LIGHT[l.level]}</span>
      </li>
    {/each}
  </ul>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}
  {#if !on}
    <p class="set-note">
      La furia está <b>apagada</b>: nadie la ve. {canEdit ? 'Se enciende en Ajustes → Funciones.' : 'La enciende el lead global.'}
    </p>
  {/if}

  {#if !rules}
    <Loading label="Leyendo las reglas" />
  {:else}
    <div class="set-card rules">
      <div class="rule">
        {@render toggle(() => rules!.overdue.on, (v) => (rules!.overdue.on = v), 'Vencida')}
        <p class="help">Prioridad alta con el plazo pasado y sin terminar.</p>
        <div class="knobs">
          <span>produce</span>{@render level(() => rules!.overdue.level, (v) => (rules!.overdue.level = v), 'Nivel de vencida')}
          <span>y con más de</span>{@render num(() => rules!.overdue.late_days, (v) => (rules!.overdue.late_days = v), 'Días de retraso', 'días de retraso')}
          {@render level(() => rules!.overdue.late_level, (v) => (rules!.overdue.late_level = v), 'Nivel de vencida hace mucho')}
        </div>
      </div>

      <div class="rule">
        {@render toggle(() => rules!.inflated.on, (v) => (rules!.inflated.on = v), 'Burbuja inflada')}
        <p class="help">
          Una burbuja abierta con mucho trabajo encima. Tiene que pasar todos los umbrales; las palabras y
          los hilos cuentan también los cerrados, y los días activos son días distintos con evidencia.
        </p>
        <div class="knobs">
          <span>al menos</span>
          {@render num(() => rules!.inflated.threads, (v) => (rules!.inflated.threads = v), 'Hilos', 'hilos')}
          {@render num(() => rules!.inflated.words, (v) => (rules!.inflated.words = v), 'Palabras', 'palabras')}
          {@render num(() => rules!.inflated.active_days, (v) => (rules!.inflated.active_days = v), 'Días activos', 'días activos')}
          {@render num(() => rules!.inflated.age_days, (v) => (rules!.inflated.age_days = v), 'Días abierta', 'días abierta')}
        </div>
        <div class="knobs">
          <span>produce</span>{@render level(() => rules!.inflated.level, (v) => (rules!.inflated.level = v), 'Nivel de inflada')}
          <span>y con el doble de todo</span>{@render level(() => rules!.inflated.double_level, (v) => (rules!.inflated.double_level = v), 'Nivel de muy inflada')}
        </div>
      </div>

      <div class="rule">
        {@render toggle(() => rules!.forgotten.on, (v) => (rules!.forgotten.on = v), 'Importante y olvidada')}
        <p class="help">Prioridad alta y el calor dice que nadie la toca.</p>
        <div class="knobs">
          <span><Icon of="dormant" tint /> dormida</span>{@render level(() => rules!.forgotten.dormant_level, (v) => (rules!.forgotten.dormant_level = v), 'Nivel de olvidada dormida')}
          <span><Icon of="tombstone" tint /> en la tumba</span>{@render level(() => rules!.forgotten.rip_level, (v) => (rules!.forgotten.rip_level = v), 'Nivel de olvidada en la tumba')}
        </div>
      </div>

      <div class="rule">
        {@render toggle(() => rules!.ownerless.on, (v) => (rules!.ownerless.on = v), 'Importante y sin dueño')}
        <p class="help">Prioridad alta sin asignados (un hilo) o sin responsables (una burbuja).</p>
        <div class="knobs">
          <span>produce</span>{@render level(() => rules!.ownerless.level, (v) => (rules!.ownerless.level = v), 'Nivel de sin dueño')}
        </div>
      </div>

      {#if canEdit}
        <div class="set-row actions">
          <button class="btn btn-sm preset-tonal-surface" disabled={saving} onclick={() => save(true)}>Ajustes default</button>
          <button class="btn btn-sm preset-filled-primary-500 ml-auto" disabled={saving || !dirty} onclick={() => save()}>
            {#if saving}<Loading compact label="Guardando" />{:else}{dirty ? 'Guardar' : 'Guardado'}{/if}
          </button>
        </div>
      {/if}
    </div>

    {#if canEdit}
      <div class="set-card">
        <div class="set-row">
          <h3 class="m-0">Con estas reglas, ahora mismo</h3>
          {#if previewing}<Loading compact label="Calculando" />{/if}
        </div>
        {#if preview}
          <p class="help tallies">
            {#each [3, 2, 1] as n (n)}
              <span class="tally" title={lv(n).name} aria-label={`${preview.levels[n]} ${lv(n).name}`}>
                <Icon of={furyIcon(n)} tint size="2.6rem" /><b>{preview.levels[n]}</b>
              </span>
            {/each}
            <span>{preview.counts.bubble} burbujas y {preview.counts.thread} hilos{on ? '' : ', aunque la furia esté apagada'}.</span>
          </p>
          <ul class="items">
            {#each preview.items.slice(0, 25) as it (it.kind + it.id)}
              <li>
                <span class="head" title={it.fury.name}><Icon of={furyIcon(it.fury.level)} tint size="2.6rem" label={it.fury.name} /></span>
                <span class="what">
                  <span><b>{it.name}</b> <span class="faint">· {it.kind === 'bubble' ? 'burbuja' : `hilo #${it.seq}`} · {it.workspace}</span></span>
                  <span class="why">{it.fury.reasons.map((r) => (r.thread_name ? `«${r.thread_name}»: ` : '') + r.text).join(' · ')}</span>
                </span>
              </li>
            {:else}
              <li class="faint">Nada dispararía: <Icon of="zen" tint /> Zen.</li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
  {/if}
</section>

<style>
  .rules { display: grid; gap: 0; }
  .rule { display: grid; gap: 0.45rem; }
  .rule + .rule { margin-top: 1rem; padding-top: 1rem; border-top: 1px solid var(--line); }
  .help { margin: 0; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  .knobs { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem 0.6rem; font-size: 0.85rem; color: var(--muted); }
  .gallery { display: grid; grid-template-columns: repeat(auto-fill, minmax(9.5rem, 1fr)); gap: 0.5rem 1rem; margin: 0.25rem 0 0.5rem; padding: 0; list-style: none; }
  .lvface { display: grid; justify-items: center; gap: 0.15rem; text-align: center; }
  .lvface .big { width: 128px; height: 128px; }
  .lvname { font-size: 1.6rem; line-height: 1.1; }
  .light { display: inline-flex; align-items: center; gap: 0.4rem; color: var(--muted); font-size: 0.78rem; white-space: nowrap; }
  /* El LED del semáforo: una luz, con su halo. */
  .led { width: 0.75rem; height: 0.75rem; border-radius: 999px; box-shadow: 0 0 0 2px color-mix(in oklab, currentColor 18%, transparent), 0 0 10px 1px currentColor; }
  .led.green { color: #22c55e; background: #22c55e; }
  .led.yellow { color: #facc15; background: #facc15; }
  .led.red { color: #ef4444; background: #ef4444; }
  .led.blink { animation: fury-blink 1s steps(2, start) infinite; }
  @keyframes fury-blink { to { visibility: hidden; } }
  @media (prefers-reduced-motion: reduce) { .led.blink { animation: none; } }
  .tallies { display: flex; flex-wrap: wrap; align-items: center; gap: 0.25rem 0.9rem; }
  .tally { display: inline-flex; align-items: center; gap: 0.15rem; color: var(--text); font-size: 1.05rem; }
  .head { flex: none; display: inline-flex; }
  .num { display: inline-flex; align-items: center; gap: 0.35rem; }
  .num input { width: 6rem; }
  .actions { margin-top: 1rem; padding-top: 1rem; border-top: 1px solid var(--line); }
  .items { margin: 0.6rem 0 0; padding: 0; list-style: none; }
  .items li { display: flex; align-items: flex-start; gap: 0.6rem; padding: 0.45rem 0; border-top: 1px solid var(--line); font-size: 0.85rem; }
  .what { display: grid; min-width: 0; }
  .why { color: var(--muted); font-size: 0.78rem; }
  .set-note { margin: 0; padding: 0.5rem 0.75rem; border-radius: 8px; background: var(--hover); font-size: 0.85rem; }
</style>
