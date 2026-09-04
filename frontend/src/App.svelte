<script lang="ts">
  import { api, type ThreadHeat, type Workspace } from './lib/api';
  import { theme } from './lib/theme.svelte';
  import SignIn from './components/SignIn.svelte';
  import Board from './components/Board.svelte';

  let ready = $state(false);
  let signedIn = $state(false);
  let workspaces = $state<Workspace[]>([]);
  let current = $state<Workspace | null>(null);

  async function boot() {
    const me = await api.refresh();
    signedIn = !!me;
    if (signedIn) {
      workspaces = await api.workspaces();
      current = workspaces[0] ?? null;
    }
    ready = true;
  }
  boot();

  function openThread(t: ThreadHeat) {
    // Phase 4b: the thread interior. Until then, say so rather than doing
    // nothing — a button that silently ignores a click is worse than no button.
    alert(`#${t.seq} ${t.name}\n\n${t.heat.reason}`);
  }
</script>

{#if !ready}
  <p class="faint p-6 text-sm">…</p>
{:else if !signedIn}
  <SignIn onDone={boot} />
{:else}
  <div class="mx-auto max-w-6xl p-4 sm:p-6">
    <header class="mb-6 flex flex-wrap items-center gap-3">
      <h1 class="font-semibold">bubble.work</h1>

      {#if workspaces.length > 1}
        <select
          class="rounded-lg border px-2 py-1 text-sm"
          style="border-color: var(--line); background: var(--surface-solid); color: var(--text)"
          bind:value={current}>
          {#each workspaces as w (w.id)}
            <option value={w}>{w.name}</option>
          {/each}
        </select>
      {:else if current}
        <span class="muted text-sm">{current.name}</span>
      {/if}

      <div class="ml-auto flex items-center gap-3 text-sm">
        <select
          class="rounded-lg border px-2 py-1"
          style="border-color: var(--line); background: var(--surface-solid); color: var(--text)"
          value={theme.mode}
          onchange={(e) => theme.set((e.currentTarget as HTMLSelectElement).value as any)}>
          <option value="system">auto</option>
          <option value="light">claro</option>
          <option value="dark">oscuro</option>
        </select>
        <button class="faint hover:[color:var(--text)]" onclick={() => { api.signOut(); signedIn = false; }}>
          salir
        </button>
      </div>
    </header>

    {#if current}
      <Board workspace={current} onOpen={openThread} />
    {:else}
      <p class="card p-4 text-sm">
        No perteneces a ningún workspace todavía. Un lead te tiene que invitar.
      </p>
    {/if}
  </div>
{/if}
