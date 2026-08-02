<script lang="ts">
  import { store } from '../lib/store.svelte';
  import BubblePool from './BubblePool.svelte';

  let key = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  async function submit(e: Event) {
    e.preventDefault();
    if (!key.trim()) return;
    busy = true;
    err = null;
    await store.signIn(key.trim());
    busy = false;
    if (!store.authed) {
      err = store.error || 'That key was not accepted.';
    }
    key = '';
  }
</script>

<BubblePool />

<div class="gate">
  <h1 class="title">bubble.work</h1>

  <form class="entry" onsubmit={submit}>
    <label for="key">Plane API key</label>
    <div class="row">
      <input
        id="key"
        type="password"
        autocomplete="off"
        placeholder="plane_api_…"
        bind:value={key}
        disabled={busy}
      />
      <button type="submit" class="go" disabled={busy || !key.trim()}>
        {busy ? '…' : 'log in'}
      </button>
    </div>
    {#if err}<p class="err">{err}</p>{/if}
  </form>
</div>

<style>
  .gate {
    position: relative;
    z-index: 1;
    height: 100vh;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: 1.6rem;
    padding: 1.5rem;
    pointer-events: none; /* let the pool receive hover except on the form */
  }
  .title {
    margin: 0;
    font-size: clamp(2.8rem, 11vw, 6.5rem);
    font-weight: 800;
    letter-spacing: -0.03em;
    line-height: 1;
    background: linear-gradient(
      120deg,
      oklch(0.72 0.19 40),
      oklch(0.7 0.18 330),
      oklch(0.72 0.16 250)
    );
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
    text-shadow: 0 2px 30px oklch(0.5 0.1 300 / 0.25);
  }
  .entry {
    pointer-events: auto;
    width: min(92vw, 400px);
    display: grid;
    gap: 0.5rem;
    padding: 1.1rem 1.15rem;
    border-radius: 18px;
    background: color-mix(in oklab, var(--surface-solid) 55%, transparent);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    border: 1px solid var(--line);
    box-shadow: 0 24px 60px var(--shadow-strong);
  }
  label {
    font-size: 0.76rem;
    color: var(--muted);
    font-weight: 600;
  }
  .row {
    display: flex;
    gap: 0.5rem;
  }
  input {
    flex: 1;
    min-width: 0;
    padding: 0.65rem 0.8rem;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 6%, transparent);
    color: var(--text);
    outline: none;
  }
  input:focus {
    border-color: var(--wip);
  }
  .go {
    padding: 0.65rem 1.1rem;
    border-radius: 12px;
    border: none;
    font-weight: 700;
    cursor: pointer;
    white-space: nowrap;
    color: oklch(0.16 0.02 265);
    background: linear-gradient(180deg, oklch(0.8 0.19 45), var(--wip));
  }
  .go:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .err {
    margin: 0.1rem 0 0;
    font-size: 0.78rem;
    color: oklch(0.72 0.19 25);
  }
</style>
