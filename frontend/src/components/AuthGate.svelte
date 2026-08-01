<script lang="ts">
  import { store } from '../lib/store.svelte';

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
    // keep the field cleared regardless
    key = '';
  }
</script>

<div class="gate">
  <form class="card" onsubmit={submit}>
    <div class="brand">
      <span class="orb"></span>
      <div>
        <h1>bubble.work</h1>
        <p class="sub">your attention, floating</p>
      </div>
    </div>

    <label for="key">Plane API key</label>
    <input
      id="key"
      type="password"
      autocomplete="off"
      placeholder="plane_api_…"
      bind:value={key}
      disabled={busy}
    />
    <p class="hint">
      The same key the CLI uses. Stored only in this browser; sent as a bearer
      token. Identity, scope and inbox are derived server-side.
    </p>

    {#if err}<p class="err">{err}</p>{/if}

    <button type="submit" class="go" disabled={busy || !key.trim()}>
      {busy ? 'checking…' : 'enter'}
    </button>
  </form>
</div>

<style>
  .gate {
    height: 100vh;
    display: grid;
    place-content: center;
  }
  .card {
    width: min(92vw, 380px);
    padding: 1.75rem;
    border-radius: 20px;
    background: var(--surface);
    border: 1px solid var(--line);
    backdrop-filter: blur(18px);
    box-shadow: 0 24px 60px oklch(0.1 0.03 265 / 0.55);
    display: grid;
    gap: 0.6rem;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    margin-bottom: 0.6rem;
  }
  .orb {
    width: 40px;
    height: 40px;
    border-radius: 999px;
    background: radial-gradient(circle at 35% 30%, white 0%, var(--wip) 55%, transparent 78%);
    animation: bob 2.6s ease-in-out infinite;
    flex: none;
  }
  h1 {
    margin: 0;
    font-size: 1.25rem;
    font-weight: 750;
  }
  .sub {
    margin: 0;
    color: var(--faint);
    font-size: 0.8rem;
  }
  label {
    font-size: 0.78rem;
    color: var(--muted);
    font-weight: 600;
  }
  input {
    padding: 0.65rem 0.8rem;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: oklch(0.12 0.02 265 / 0.6);
    color: var(--text);
    outline: none;
  }
  input:focus {
    border-color: var(--wip);
  }
  .hint {
    margin: 0;
    font-size: 0.72rem;
    color: var(--faint);
    line-height: 1.4;
  }
  .err {
    margin: 0;
    font-size: 0.78rem;
    color: oklch(0.72 0.19 25);
  }
  .go {
    margin-top: 0.4rem;
    padding: 0.7rem;
    border-radius: 12px;
    border: none;
    font-weight: 700;
    cursor: pointer;
    color: oklch(0.15 0.02 265);
    background: linear-gradient(180deg, oklch(0.8 0.19 45), var(--wip));
  }
  .go:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
