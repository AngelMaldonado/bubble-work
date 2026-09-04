<script lang="ts">
  import { api } from '../lib/api';

  let { onDone }: { onDone: () => void } = $props();
  let identity = $state('');
  // Whether anybody can sign in at all. A superuser is not a person: it operates
  // the box and has no row in `users`.
  let empty = $state(false);
  (async () => {
    try {
      const res = await fetch('/api/collections/users/records?perPage=1');
      if (res.ok) empty = (await res.json()).totalItems === 0;
    } catch {
      /* the server will say so when the form is submitted */
    }
  })();
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
      // PocketBase answers "Failed to authenticate." whether the person does not
      // exist or the password is wrong. When NOBODY exists, that reads as a bug in
      // your typing rather than an empty database — so say which it is.
      const msg = (err as Error).message;
      error = empty
        ? 'Todavía no hay ninguna persona en este servidor. Créala con `just person <correo> <contraseña> lead` — un superuser opera la caja, pero no trabaja aquí.'
        : msg;
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
