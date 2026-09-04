<script lang="ts">
  import { api } from '../lib/api';

  let { onDone }: { onDone: () => void } = $props();
  let identity = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: Event) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      await api.signIn(identity, password);
      onDone();
    } catch (err) {
      error = (err as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="grid min-h-screen place-items-center p-6">
  <form class="card w-full max-w-sm p-6" onsubmit={submit}>
    <h1 class="mb-1 text-lg font-semibold">bubble.work</h1>
    <p class="faint mb-5 text-sm">Attention behaves like buoyancy.</p>

    <label class="mb-3 block text-sm">
      <span class="muted">Email</span>
      <input
        class="mt-1 w-full rounded-lg border px-3 py-2"
        style="border-color: var(--line); background: var(--surface-solid); color: var(--text)"
        type="email" bind:value={identity} autocomplete="username" required />
    </label>
    <label class="mb-4 block text-sm">
      <span class="muted">Password</span>
      <input
        class="mt-1 w-full rounded-lg border px-3 py-2"
        style="border-color: var(--line); background: var(--surface-solid); color: var(--text)"
        type="password" bind:value={password} autocomplete="current-password" required />
    </label>

    {#if error}
      <p class="mb-3 text-sm" style="color: var(--hot)">{error}</p>
    {/if}

    <button
      class="w-full rounded-lg px-3 py-2 font-medium"
      style="background: var(--text); color: var(--surface-solid)"
      disabled={busy}>{busy ? '…' : 'Entrar'}</button>
  </form>
</div>
