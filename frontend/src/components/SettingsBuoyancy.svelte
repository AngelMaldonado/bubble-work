<script lang="ts">
  import Loading from './Loading.svelte';
  import Icon from './Icon.svelte';
  // Flotabilidad: la calibración con la que el servidor decide qué flota.
  //
  // Son cinco números en una fila de `tuning`, y cambiarlos recalibra el board
  // de TODO el departamento en el siguiente cálculo — sin migración ni backfill,
  // porque nada guarda una banda. Por eso se dicen en palabras de lo que
  // producen, y por eso lo edita sólo el lead global.
  import { Slider, Switch } from '@skeletonlabs/skeleton-svelte';
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
  // Los artefactos temporales: viven en la misma fila de calibración.
  let artDays = $state(30);

  function seed(t: Tuning) {
    saved = t;
    // Enteros: el slider se mueve de uno en uno. Un valor viejo con decimales
    // se redondea al leerlo (y se guarda así sólo si se pulsa «Guardar»).
    cycleDays = Math.max(1, Math.round(t.cycle_hours / 24));
    dormant = Math.max(1, Math.round(t.dormant_cycles));
    decay = Math.max(1, Math.round(t.decay_cycles));
    grace = Math.max(0, Math.round(t.grace_cycles));
    ownerlessRip = !!t.ownerless_is_rip;
    artDays = t.artifact_days && t.artifact_days > 0 ? Math.round(t.artifact_days) : 30;
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
        ownerlessRip !== !!saved.ownerless_is_rip ||
        artDays !== (saved.artifact_days && saved.artifact_days > 0 ? saved.artifact_days : 30)),
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
          artifact_days: artDays,
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

{#snippet cycleHelp()}La ventana contra la que se mide todo: lo que produjo evidencia dentro del ciclo está <Icon of="fire" tint />.{/snippet}
{#snippet dormantHelp()}Cuánto silencio hace falta para pasar de <Icon of="fire" tint /> a <Icon of="dormant" tint />.{/snippet}
{#snippet decayHelp()}Ordena dentro de cada banda: cuanto mayor, más tiempo sigue arriba algo que ya no produce. Hoy flota al {afterOne}% tras un ciclo y al {afterTwo}% tras dos.{/snippet}
{#snippet artHelp()}Un grill o un borrador que nadie toca ni fija se borra solo a los {artDays} {artDays === 1 ? 'día' : 'días'} de su último movimiento: escribir o contestar en él, traspasarlo o quitarle el fijado reinician la cuenta, y fijado no caduca. Bajarlo borra en la siguiente hora los que ya pasaron del nuevo plazo.{/snippet}
{#snippet graceHelp()}Un hilo recién creado sin evidencia se lee como nuevo, no como abandonado, durante este tiempo.{/snippet}

<!-- Un número de la calibración: su nombre y su valor en palabras arriba, el
     slider, y qué produce. -->
{#snippet tune(label: string, value: number, min: number, max: number, marks: number[], set: (v: number) => void, said: string, aside: string, help: import('svelte').Snippet)}
  <div class="field">
    <!-- El Slider de Skeleton (`@skeletonlabs/skeleton-svelte`), de uno en uno. -->
    <Slider value={[value]} {min} max={Math.max(max, value)} step={1} disabled={!canEdit || saving}
      onValueChange={(e: { value: number[] }) => set(e.value[0])}>
      <div class="head">
        <Slider.Label class="name">{label}</Slider.Label>
        <span class="said">{said}{#if aside}<span class="aside">{aside}</span>{/if}</span>
      </div>
      <Slider.Control>
        <Slider.Track><Slider.Range /></Slider.Track>
        <Slider.Thumb index={0} aria-label={label}><Slider.HiddenInput /></Slider.Thumb>
      </Slider.Control>
      <Slider.MarkerGroup>
        {#each marks as m (m)}<Slider.Marker value={m}>{m}</Slider.Marker>{/each}
      </Slider.MarkerGroup>
    </Slider>
    <span class="help">{@render help()}</span>
  </div>
{/snippet}

<section class="set-section">
  <p class="set-lead">
    Cómo decide el servidor qué flota. Un cambio aquí recalibra el board de todo el
    departamento a la vez: nada guarda una banda, así que no hay nada que migrar.
    {#if !canEdit}<br /><b>Sólo el lead global la cambia.</b>{/if}
  </p>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  {#if saved}
    <!-- Planos sobre el fondo, y un slider por número: se calibra arrastrando y
         mirando qué produce, no tecleando cifras. Los rangos cubren lo sensato;
         lo que el servidor rechaza, igual se dice abajo. -->
    <fieldset class="grid" disabled={!canEdit || saving}>
      {@render tune('Duración de un ciclo', cycleDays, 1, 30, [1, 7, 14, 21, 30], (v) => (cycleDays = v), `${cycleDays} ${cycleDays === 1 ? 'día' : 'días'}`, `= ${hours} horas`, cycleHelp)}
      {@render tune('Se duerme tras', dormant, 1, 12, [1, 2, 3, 4, 6, 8, 10, 12], (v) => (dormant = v), `${dormant} ${dormant === 1 ? 'ciclo' : 'ciclos'} sin evidencia`, `= ${days(dormant)}`, dormantHelp)}
      {@render tune('Enfriamiento', decay, 1, 10, [1, 2, 3, 4, 5, 6, 7, 8, 9, 10], (v) => (decay = v), `${decay} ${decay === 1 ? 'ciclo' : 'ciclos'}`, '', decayHelp)}
      {@render tune('Gracia al nacer', grace, 0, 6, [0, 1, 2, 3, 4, 5, 6], (v) => (grace = v), `${grace} ${grace === 1 ? 'ciclo' : 'ciclos'}`, `= ${days(grace)}`, graceHelp)}

      <div class="field">
        <Switch
          checked={ownerlessRip}
          disabled={!canEdit || saving}
          onCheckedChange={(e: { checked: boolean }) => (ownerlessRip = e.checked)}>
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Label>Sin nadie a cargo, lo dormido cae a <Icon of="tombstone" tint /></Switch.Label>
          <Switch.HiddenInput />
        </Switch>
        <span class="help">Algo callado sin responsable no es una siesta: es una decisión pendiente. Lo que sigue produciendo se queda <Icon of="fire" tint /> igual.</span>
      </div>

      <!-- No es flotabilidad, pero vive en la misma fila de calibración y se
           guarda con el mismo botón. -->
      <h3 class="sub">Artefactos temporales</h3>
      {@render tune('Se borran tras', artDays, 1, 180, [1, 30, 60, 90, 120, 150, 180], (v) => (artDays = v), `${artDays} ${artDays === 1 ? 'día' : 'días'} sin movimiento`, '', artHelp)}
    </fieldset>

    {#if canEdit}
      <div class="set-row">
        <button class="btn btn-sm preset-filled-primary-500" onclick={save} disabled={!dirty || !!invalid || saving}>
          {#if saving}<Loading compact label="Guardando ajustes" />{:else}Guardar{/if}
        </button>
        <button class="btn btn-sm preset-tonal-surface" onclick={() => saved && seed(saved)} disabled={!dirty || saving}>
          Descartar
        </button>
        {#if invalid}<span class="set-err">{invalid}</span>{:else if done}<span class="set-ok">{done}</span>{/if}
        {#if saved.updated}<span class="faint ml-auto text-xs">cambiada {ago(saved.updated)}</span>{/if}
      </div>
    {/if}
  {:else if !error}
    <Loading label="Leyendo ajustes" />
  {/if}
</section>

<style>
  .grid { display: flex; flex-direction: column; gap: 1.6rem; margin: 0; padding: 0; border: 0; min-width: 0; }
  .field { display: flex; flex-direction: column; gap: 0.45rem; max-width: 36rem; }
  /* Las marcas de Zag van en absoluto y su grupo no ocupa alto: se le da el de
     una línea, para que no caigan sobre la ayuda. */
  .field :global([data-scope='slider'][data-part='marker-group']) { position: relative; height: 1.1rem; margin-top: 0.15rem; }
  .head { display: flex; align-items: baseline; gap: 0.75rem; margin-bottom: 0.35rem; }
  .field :global(.name) { font-size: 0.88rem; font-weight: 600; }
  .said { margin-left: auto; font-size: 0.85rem; font-weight: 700; font-variant-numeric: tabular-nums; }
  .aside { margin-left: 0.4rem; color: var(--faint); font-size: 0.75rem; font-weight: 500; }
  .sub { margin: 0.4rem 0 -0.6rem; padding-top: 1.2rem; border-top: 1px solid var(--line); font-size: 0.95rem; font-weight: 700; }
  .help { color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
</style>
