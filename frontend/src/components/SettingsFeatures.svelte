<script lang="ts">
  // Funciones: qué partes del producto tiene encendidas el departamento.
  //
  // Encender o apagar es del lead global, y lo nota todo el departamento a la
  // vez. Apagar no borra nada: los datos se quedan y volver a encender trae las
  // cosas como estaban.
  import { Switch } from '@skeletonlabs/skeleton-svelte';
  import { api, type Features } from '../lib/api';

  let { features, onfeatures }: { features: Features; onfeatures?: (f: Features) => void } = $props();

  let saving = $state(false);
  let error = $state('');

  async function set(fields: Partial<Omit<Features, 'id' | 'updated'>>) {
    if (!features.id) return;
    saving = true;
    error = '';
    try {
      onfeatures?.(await api.setFeatures(features.id, fields));
    } catch (e) {
      error = (e as Error).message;
    } finally {
      saving = false;
    }
  }
</script>

<section class="set-section">
  <p class="set-lead">
    Partes del producto que el departamento puede encender o apagar. Apagar no borra nada: al
    volver a encenderla, todo está como se dejó.
  </p>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}
  {#if !features.id}
    <p class="set-err">No hay fila de funciones en el servidor: falta aplicar la migración.</p>
  {/if}

  <div class="set-card">
    <div class="feature">
      <Switch
        checked={features.sequence}
        disabled={saving || !features.id}
        onCheckedChange={(e: { checked: boolean }) => set({ sequence: e.checked })}>
        <Switch.Control><Switch.Thumb /></Switch.Control>
        <Switch.Label>🧭 Secuencia de ejecución</Switch.Label>
        <Switch.HiddenInput />
      </Switch>
      <p class="help">
        Una sola línea con el orden en que el departamento ejecuta los hilos, con pasos en
        paralelo, que se recorre sola al terminar hilos. Encendida aparece como panel del planeador,
        como «Secuencia» en la columna, en el cajón de cada burbuja y en la tool <code>next</code>
        del MCP.
      </p>
    </div>
  </div>
</section>

<style>
  .feature { display: flex; flex-direction: column; gap: 0.4rem; }
  .help { margin: 0; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
</style>
