<script lang="ts">
  import { api } from '../lib/api';

  let { onDone }: { onDone: () => void } = $props();
  let name = $state('');
  let slug = $state('');
  let touched = $state(false);
  let error = $state('');
  let busy = $state(false);

  // The slug follows the name until somebody edits it. It is in the URL and in
  // the directory that holds the workspace's markdown, so it is worth seeing —
  // and worth being able to override before it is fixed forever.
  const suggested = $derived(
    name.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '')
      .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 60),
  );
  $effect(() => {
    if (!touched) slug = suggested;
  });

  async function submit(e: Event) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      await api.createWorkspace(name.trim(), slug);
      onDone();
    } catch (err) {
      error = (err as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<form class="card glass p-4" onsubmit={submit}>
  <h2 class="mb-1 font-medium">Nuevo workspace</h2>
  <p class="faint mb-4 text-sm">
    Quien lo crea queda como su <b>lead</b>: puede invitar, definir el flujo de
    trabajo y cerrarlo.
  </p>

  <div class="flex flex-wrap gap-3">
    <label class="min-w-48 flex-1 text-sm">
      <span class="muted">Nombre</span>
      <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
        class="input mt-1"
        bind:value={name} required />
    </label>
    <label class="min-w-48 flex-1 text-sm">
      <span class="muted">Slug</span>
      <input autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
        class="input mt-1 font-mono text-sm"
        bind:value={slug} oninput={() => (touched = true)}
        pattern="[a-z0-9]+(-[a-z0-9]+)*" required />
    </label>
  </div>
  <p class="faint mt-1 text-xs">El slug nombra el directorio de sus markdown y no cambia después.</p>

  {#if error}
    <p class="mt-3 text-sm text-error-500">{error}</p>
  {/if}

  <button
    class="btn btn-sm preset-filled-primary-500 mt-4"
    disabled={busy || !name.trim() || !slug}>{busy ? '…' : 'Crear'}</button>
</form>
