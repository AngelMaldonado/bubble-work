<script lang="ts">
  // Flotabilidad: la calibración con la que el servidor decide qué flota.
  //
  // Son cinco números en una fila de `tuning`, y cambiarlos recalibra el board
  // de TODO el departamento en el siguiente cálculo — sin migración ni backfill,
  // porque nada guarda una banda. Por eso se dicen en palabras de lo que
  // producen, y por eso lo edita sólo el lead global.
  import { Switch } from '@skeletonlabs/skeleton-svelte';
  import { api, type Tuning } from '../lib/api';
  import { ago } from '../lib/when';

  let { canEdit = false }: { canEdit?: boolean } = $props();

  let saved = $state<Tuning | null>(null);
  let error = $state('');
  let saving = $state(false);
  let done = $state('');

  // El formulario, en las unidades en que se piensa: días, no horas.
  let cycleDays = $state(7);
  let dormant = $state(2);
  let decay = $state(2);
  let grace = $state(1);
  let ownerlessRip = $state(true);

  function seed(t: Tuning) {
    saved = t;
    cycleDays = Math.round((t.cycle_hours / 24) * 100) / 100;
    dormant = t.dormant_cycles;
    decay = t.decay_cycles;
    grace = t.grace_cycles;
    ownerlessRip = !!t.ownerless_is_rip;
  }

  $effect(() => {
    api.tuning().then(seed).catch((e) => (error = (e as Error).message));
  });

  const hours = $derived(Math.round(cycleDays * 24));
  const dirty = $derived(
    !!saved &&
      (hours !== saved.cycle_hours ||
        dormant !== saved.dormant_cycles ||
        decay !== saved.decay_cycles ||
        grace !== saved.grace_cycles ||
        ownerlessRip !== !!saved.ownerless_is_rip),
  );

  // Lo que el servidor rechazaría o lo que dejaría el board sin sentido, dicho
  // antes de guardar.
  const invalid = $derived(
    !(hours >= 1)
      ? 'el ciclo tiene que durar al menos una hora'
      : !(dormant >= 1)
        ? 'hace falta al menos un ciclo para dormirse'
        : !(decay > 0)
          ? 'el enfriamiento tiene que ser mayor que cero'
          : !(grace >= 0)
            ? 'la gracia no puede ser negativa'
            : '',
  );

  // Cuánto flota algo según pasa el tiempo sin evidencia: e^(−ciclos / enfriamiento).
  const afterOne = $derived(decay > 0 ? Math.round(Math.exp(-1 / decay) * 100) : 0);
  const afterTwo = $derived(decay > 0 ? Math.round(Math.exp(-2 / decay) * 100) : 0);
  const days = (n: number) => {
    const d = Math.round(n * cycleDays * 10) / 10;
    return d === 1 ? '1 día' : `${d} días`;
  };

  async function save() {
    if (!saved || invalid) return;
    saving = true;
    error = '';
    done = '';
    try {
      seed(
        await api.setTuning(saved.id, {
          cycle_hours: hours,
          dormant_cycles: dormant,
          decay_cycles: decay,
          grace_cycles: grace,
          ownerless_is_rip: ownerlessRip,
        }),
      );
      done = 'Guardado. El board se recalibra al volver a pedirlo.';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      saving = false;
    }
  }
</script>

<section class="set-section">
  <p class="set-lead">
    Cómo decide el servidor qué flota. Un cambio aquí recalibra el board de todo el
    departamento a la vez: nada guarda una banda, así que no hay nada que migrar.
    {#if !canEdit}<br /><b>Sólo el lead global la cambia.</b>{/if}
  </p>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  {#if saved}
    <fieldset class="set-card grid" disabled={!canEdit || saving}>
      <label class="field">
        <span class="name">Duración de un ciclo</span>
        <span class="set-row">
          <input class="input num" type="number" min="0.05" step="0.5" bind:value={cycleDays} /> días
          <span class="faint text-xs">= {hours} horas</span>
        </span>
        <span class="help">La ventana contra la que se mide todo: lo que produjo evidencia dentro del ciclo está 🔥.</span>
      </label>

      <label class="field">
        <span class="name">Se duerme tras</span>
        <span class="set-row">
          <input class="input num" type="number" min="1" step="1" bind:value={dormant} /> ciclos sin evidencia
          <span class="faint text-xs">= {days(dormant)}</span>
        </span>
        <span class="help">Cuánto silencio hace falta para pasar de 🔥 a 😴.</span>
      </label>

      <label class="field">
        <span class="name">Enfriamiento</span>
        <span class="set-row">
          <input class="input num" type="number" min="0.1" step="0.5" bind:value={decay} /> ciclos
        </span>
        <span class="help">
          Ordena dentro de cada banda: cuanto mayor, más tiempo sigue arriba algo que ya no produce.
          Hoy flota al {afterOne}% tras un ciclo y al {afterTwo}% tras dos.
        </span>
      </label>

      <label class="field">
        <span class="name">Gracia al nacer</span>
        <span class="set-row">
          <input class="input num" type="number" min="0" step="0.5" bind:value={grace} /> ciclos
          <span class="faint text-xs">= {days(grace)}</span>
        </span>
        <span class="help">Un hilo recién creado sin evidencia se lee como nuevo, no como abandonado, durante este tiempo.</span>
      </label>

      <div class="field">
        <Switch
          checked={ownerlessRip}
          disabled={!canEdit || saving}
          onCheckedChange={(e: { checked: boolean }) => (ownerlessRip = e.checked)}>
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Label>Sin nadie a cargo, lo dormido cae a 🪦</Switch.Label>
          <Switch.HiddenInput />
        </Switch>
        <span class="help">Algo callado sin responsable no es una siesta: es una decisión pendiente. Lo que sigue produciendo se queda 🔥 igual.</span>
      </div>
    </fieldset>

    {#if canEdit}
      <div class="set-row">
        <button class="btn btn-sm preset-filled-primary-500" onclick={save} disabled={!dirty || !!invalid || saving}>
          {saving ? 'guardando…' : 'Guardar'}
        </button>
        <button class="btn btn-sm preset-tonal-surface" onclick={() => saved && seed(saved)} disabled={!dirty || saving}>
          Descartar
        </button>
        {#if invalid}<span class="set-err">{invalid}</span>{:else if done}<span class="set-ok">{done}</span>{/if}
        {#if saved.updated}<span class="faint ml-auto text-xs">cambiada {ago(saved.updated)}</span>{/if}
      </div>
    {/if}
  {:else if !error}
    <p class="faint text-sm">leyendo…</p>
  {/if}
</section>

<style>
  .grid { display: flex; flex-direction: column; gap: 1.1rem; margin: 0; }
  .field { display: flex; flex-direction: column; gap: 0.3rem; }
  .name { font-size: 0.88rem; font-weight: 600; }
  .help { color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  .num { width: 6rem; }
</style>
