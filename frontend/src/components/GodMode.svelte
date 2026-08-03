<script lang="ts">
  import { onMount } from 'svelte';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import type { AdminStats, AdminInstance, InstanceMembers, KioskToken, Member } from '../lib/types';

  function roleLabel(m: Member): string {
    if (m.admin) return 'admin';
    return m.role >= 15 ? 'member' : m.role >= 5 ? 'guest' : String(m.role);
  }

  let stats = $state<AdminStats | null>(null);
  let instances = $state<AdminInstance[]>([]);
  let memberGroups = $state<InstanceMembers[]>([]);
  let tokens = $state<KioskToken[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let toast = $state<string | null>(null);
  let busy = $state<string | null>(null); // id of the action in flight

  // kiosk mint form
  let mintInstance = $state('');
  let mintName = $state('');

  function flash(msg: string): void {
    toast = msg;
    setTimeout(() => (toast = null), 4000);
  }

  function fail(e: unknown): void {
    error = e instanceof ApiError ? e.message : String(e);
  }

  async function loadAll(): Promise<void> {
    loading = true;
    error = null;
    try {
      [stats, instances, memberGroups, tokens] = await Promise.all([
        api.adminStats(),
        api.adminInstances(),
        api.adminMembers(),
        api.adminKiosk(),
      ]);
      if (!mintInstance && instances.length) mintInstance = instances[0].slug;
    } catch (e) {
      fail(e);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (store.godmode) void loadAll();
    else loading = false;
  });

  async function run(id: string, fn: () => Promise<void>): Promise<void> {
    if (busy) return;
    busy = id;
    error = null;
    try {
      await fn();
    } catch (e) {
      fail(e);
    } finally {
      busy = null;
    }
  }

  const refreshCaches = () =>
    run('refresh', async () => {
      await api.adminRefresh();
      await store.refresh();
      await loadAll();
      flash('caches refreshed');
    });

  const tick = () =>
    run('tick', async () => {
      await api.adminTick();
      flash('cooling sweep triggered');
    });

  const toggleAllOrgs = () =>
    run('allorgs', async () => {
      store.allOrgs = !store.allOrgs;
      await store.refresh();
      flash(store.allOrgs ? 'cross-org board on' : 'cross-org board off');
    });

  const mintKiosk = () =>
    run('mint', async () => {
      if (!mintInstance) {
        error = 'pick an instance';
        return;
      }
      const k = await api.adminKioskCreate(mintInstance, mintName.trim());
      tokens = [...tokens, k];
      mintName = '';
      flash('kiosk token minted');
    });

  const revokeKiosk = (t: KioskToken) =>
    run('revoke:' + t.token, async () => {
      await api.adminKioskRevoke(t.token);
      tokens = tokens.filter((x) => x.token !== t.token);
      flash('kiosk token revoked');
    });

  function kioskUrl(t: KioskToken): string {
    return `${location.origin}/?kiosk=${encodeURIComponent(t.token)}`;
  }

  async function copy(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      flash('copied to clipboard');
    } catch {
      flash('copy failed — select and copy manually');
    }
  }
</script>

<div class="screen" aria-label="god mode">
  <div class="topbar">
    <button class="back" onclick={() => store.closeGodMode()} aria-label="back to board">
      <span aria-hidden="true">←</span> board
    </button>
    <span class="bolt" aria-hidden="true">⚡</span>
    <h2 class="ttl">God Mode</h2>
    {#if store.allOrgs}<span class="chip on">all orgs</span>{/if}
    {#if toast}<span class="toast" role="status">{toast}</span>{/if}
  </div>

  {#if !store.godmode}
    <p class="pad dim">Service admin only.</p>
  {:else if loading}
    <p class="pad dim">Loading…</p>
  {:else}
    {#if error}<p class="err pad">{error}</p>{/if}

    <div class="grid">
      <!-- stats -->
      <section class="card">
        <h3>Server</h3>
        {#if stats}
          <dl class="kv">
            <dt>revision</dt>
            <dd>{stats.revision ? stats.revision.slice(0, 10) : '—'}</dd>
            <dt>built</dt>
            <dd>{stats.built || '—'}</dd>
            <dt>started</dt>
            <dd>{new Date(stats.started_at).toLocaleString()}</dd>
            <dt>instances</dt>
            <dd>{stats.instances} ({stats.cached_instances} cached)</dd>
            <dt>identities</dt>
            <dd>{stats.cached_identities} cached</dd>
          </dl>
        {/if}
      </section>

      <!-- maintenance -->
      <section class="card">
        <h3>Maintenance</h3>
        <div class="actions">
          <button onclick={refreshCaches} disabled={busy === 'refresh'}>
            {busy === 'refresh' ? '…' : 'Refresh caches'}
          </button>
          <button onclick={tick} disabled={busy === 'tick'}>
            {busy === 'tick' ? '…' : 'Tick (cooling sweep)'}
          </button>
          <button class:on={store.allOrgs} onclick={toggleAllOrgs} disabled={busy === 'allorgs'}>
            {store.allOrgs ? 'Cross-org board: on' : 'Cross-org board: off'}
          </button>
        </div>
        <p class="hint">
          Refresh flushes server caches and re-polls. Tick forces a cooling sweep now. Cross-org
          shows bubbles across every instance on the board.
        </p>
      </section>

      <!-- instances -->
      <section class="card wide">
        <h3>Instances ({instances.length})</h3>
        <div class="tablewrap">
          <table>
            <thead>
              <tr><th>slug</th><th>base url</th><th>workspace</th><th>project</th><th>webhook</th><th>cached</th></tr>
            </thead>
            <tbody>
              {#each instances as i (i.slug)}
                <tr>
                  <td class="mono">{i.slug}</td>
                  <td class="mono dim">{i.base_url}</td>
                  <td>{i.workspace}</td>
                  <td class="mono">{i.project || '—'}</td>
                  <td>{i.has_webhook ? '✓' : '—'}</td>
                  <td>{i.cached ? '✓' : '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <!-- members (read-only; Plane owns membership) -->
      <section class="card wide">
        <h3>Members</h3>
        <p class="hint" style="margin-top:0">
          Membership is managed in Plane — this is a read-only view of who has access.
        </p>
        {#each memberGroups as g (g.instance)}
          <div class="mgroup">
            <div class="mgroup-head">
              <span class="mono">{g.instance}</span>
              <span class="dim">{g.name}</span>
              <span class="dim tok-when">{g.members.length} members</span>
            </div>
            {#if g.error}
              <p class="err">could not load: {g.error}</p>
            {:else}
              <div class="tablewrap">
                <table>
                  <thead>
                    <tr><th>name</th><th>email</th><th>role</th></tr>
                  </thead>
                  <tbody>
                    {#each g.members as m (m.id)}
                      <tr>
                        <td>{m.name}</td>
                        <td class="mono dim">{m.email}</td>
                        <td>
                          {#if m.admin}<span class="chip on">{roleLabel(m)}</span>
                          {:else}<span class="chip">{roleLabel(m)}</span>{/if}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}
          </div>
        {/each}
      </section>

      <!-- kiosk tokens -->
      <section class="card wide">
        <h3>Kiosk display tokens ({tokens.length})</h3>
        <div class="mint">
          <select bind:value={mintInstance} aria-label="instance">
            {#each instances as i (i.slug)}
              <option value={i.slug}>{i.slug}</option>
            {/each}
          </select>
          <input placeholder="label (e.g. lobby screen)" bind:value={mintName} />
          <button onclick={mintKiosk} disabled={busy === 'mint'}>
            {busy === 'mint' ? '…' : 'Mint token'}
          </button>
        </div>

        {#if tokens.length === 0}
          <p class="hint">No kiosk tokens. Mint one to boot a read-only display board.</p>
        {:else}
          <ul class="tokens">
            {#each tokens as t (t.token)}
              <li>
                <div class="tok-main">
                  <span class="tok-name">{t.name || '(unnamed)'}</span>
                  <span class="chip">{t.instance}</span>
                  <span class="dim tok-when">{new Date(t.created_at).toLocaleDateString()}</span>
                </div>
                <div class="tok-url">
                  <code>{kioskUrl(t)}</code>
                  <button class="mini" onclick={() => copy(kioskUrl(t))} title="copy kiosk URL">copy</button>
                  <button
                    class="mini danger"
                    onclick={() => revokeKiosk(t)}
                    disabled={busy === 'revoke:' + t.token}
                    title="revoke">revoke</button
                  >
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .screen {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: transparent;
  }
  .topbar {
    position: sticky;
    top: 0;
    z-index: 25;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.55rem 1.25rem;
    font-size: 0.75rem;
    color: var(--faint);
    background: color-mix(in oklab, var(--bg) 72%, transparent);
    backdrop-filter: blur(14px);
    border-bottom: 1px solid var(--line);
  }
  .back {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.7rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.75rem;
  }
  .back:hover {
    color: var(--text);
    background: var(--hover);
  }
  .bolt {
    color: var(--wip);
  }
  .ttl {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 800;
    letter-spacing: -0.01em;
    color: var(--text);
  }
  .toast {
    margin-left: auto;
    color: var(--text);
    background: color-mix(in oklab, var(--reviewed) 20%, transparent);
    padding: 0.15rem 0.6rem;
    border-radius: 999px;
  }
  .chip {
    color: var(--muted);
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 0.1rem 0.5rem;
    font-size: 0.72rem;
  }
  .chip.on {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 45%, transparent);
    font-weight: 700;
  }
  .pad {
    padding: 2rem 1.5rem;
  }
  .err {
    color: oklch(0.68 0.19 25);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
    padding: clamp(1rem, 3vw, 2rem);
    max-width: 1100px;
    width: 100%;
    margin: 0 auto;
  }
  .card {
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 14px;
    padding: 1rem 1.15rem;
    backdrop-filter: blur(12px);
  }
  .card.wide {
    grid-column: 1 / -1;
  }
  .card h3 {
    margin: 0 0 0.75rem;
    font-size: 0.82rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--faint);
  }

  .kv {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.35rem 1rem;
    margin: 0;
    font-size: 0.85rem;
  }
  .kv dt {
    color: var(--faint);
  }
  .kv dd {
    margin: 0;
    color: var(--text);
    text-align: right;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  button {
    padding: 0.4rem 0.8rem;
    border-radius: 9px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.82rem;
    cursor: pointer;
  }
  button:hover:not(:disabled) {
    background: var(--hover);
  }
  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  button.on {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 40%, transparent);
    font-weight: 700;
  }
  .hint {
    margin: 0.7rem 0 0;
    font-size: 0.75rem;
    color: var(--faint);
    line-height: 1.4;
  }

  .tablewrap {
    overflow-x: auto;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.8rem;
  }
  th,
  td {
    text-align: left;
    padding: 0.4rem 0.6rem;
    border-bottom: 1px solid var(--line);
    white-space: nowrap;
  }
  th {
    color: var(--faint);
    font-weight: 600;
    text-transform: uppercase;
    font-size: 0.66rem;
    letter-spacing: 0.05em;
  }
  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.76rem;
  }
  .dim {
    color: var(--faint);
  }

  .mgroup {
    margin-top: 0.85rem;
  }
  .mgroup-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.3rem;
    font-size: 0.82rem;
  }

  .mint {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-bottom: 0.85rem;
  }
  .mint select,
  .mint input {
    padding: 0.4rem 0.6rem;
    border-radius: 9px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    color: var(--text);
    font-size: 0.82rem;
  }
  .mint input {
    flex: 1;
    min-width: 12rem;
  }

  .tokens {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.6rem;
  }
  .tokens li {
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface-solid);
  }
  .tok-main {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.4rem;
  }
  .tok-name {
    font-weight: 700;
    color: var(--text);
    font-size: 0.85rem;
  }
  .tok-when {
    margin-left: auto;
    font-size: 0.72rem;
  }
  .tok-url {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .tok-url code {
    flex: 1;
    min-width: 0;
    overflow-x: auto;
    white-space: nowrap;
    font-size: 0.74rem;
    color: var(--muted);
    background: var(--hover);
    padding: 0.3rem 0.5rem;
    border-radius: 7px;
  }
  .mini {
    flex: none;
    padding: 0.3rem 0.6rem;
    font-size: 0.74rem;
  }
  .mini.danger {
    color: oklch(0.62 0.2 25);
    border-color: color-mix(in oklab, oklch(0.62 0.2 25) 40%, transparent);
  }

  @media (max-width: 720px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
