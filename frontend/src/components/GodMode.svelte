<script lang="ts">
  import { i18n, t } from '../lib/i18n.svelte';
  import { onMount } from 'svelte';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import { TUNING_GROUPS } from '../lib/types';
  import type {
    AdminStats,
    AdminInstance,
    InstanceMembers,
    KioskToken,
    Member,
    Tuning,
    TuningField,
    TuningKey,
    TuningView,
    SyncStatus,
    SyncDiff,
    SyncFidelity,
    OutboxView,
  } from '../lib/types';

  function roleLabel(m: Member): string {
    if (m.admin) return 'admin';
    return m.role >= 15 ? 'member' : m.role >= 5 ? 'guest' : String(m.role);
  }

  let stats = $state<AdminStats | null>(null);
  let instances = $state<AdminInstance[]>([]);
  let memberGroups = $state<InstanceMembers[]>([]);
  let tokens = $state<KioskToken[]>([]);
  let tuning = $state<TuningView | null>(null);
  // the form's working copy: edits stay local until Apply, so a half-typed
  // number never reaches the board.
  let draft = $state<Tuning | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let toast = $state<string | null>(null);
  let busy = $state<string | null>(null); // id of the action in flight

  // kiosk mint form
  let mintInstance = $state('');
  let mintName = $state('');

  // Plane mirror (PLANE-SYNC.md). Census loads with the page; diff and backfill
  // are explicit because both walk Plane completely and cost real rate budget.
  let syncStatus = $state<Record<string, SyncStatus>>({});
  let syncDiffs = $state<Record<string, SyncDiff>>({});
  let syncFidelity = $state<Record<string, SyncFidelity>>({});
  // Writes that have not reached Plane (PLANE-SYNC.md Phase 5).
  let outbox = $state<OutboxView | null>(null);

  function flash(msg: string): void {
    toast = msg;
    setTimeout(() => (toast = null), 4000);
  }

  function fail(e: unknown): void {
    error = e instanceof ApiError ? e.message : String(e);
  }

  // Both of these walk Plane completely and are rate-budgeted server-side, so
  // they can legitimately take a while. The button stays disabled meanwhile
  // rather than letting an impatient click queue a second full walk.
  async function dropOutbox(id: number): Promise<void> {
    busy = 'outbox:' + id;
    error = null;
    try {
      await api.adminOutboxDrop(id);
      outbox = await api.adminOutbox();
      flash(t('toast.outboxDropped', { id }));
    } catch (e) {
      fail(e);
    } finally {
      busy = null;
    }
  }

  async function runSyncDiff(slug: string): Promise<void> {
    busy = 'diff:' + slug;
    error = null;
    try {
      syncDiffs[slug] = await api.adminSyncDiff(slug);
      syncStatus[slug] = await api.adminSync(slug);
      flash(
        syncDiffs[slug].clean
          ? t('toast.mirrorMatches', { slug })
          : t('toast.mirrorFindings', { slug, n: syncDiffs[slug].findings?.length ?? 0 }),
      );
    } catch (e) {
      fail(e);
    } finally {
      busy = null;
    }
  }

  // Mirror-only, so unlike diff and backfill this costs no Plane calls and can
  // be re-run freely after any change to the markdown bridge.
  async function runSyncFidelity(slug: string): Promise<void> {
    busy = 'fidelity:' + slug;
    error = null;
    try {
      const f = await api.adminSyncFidelity(slug);
      syncFidelity[slug] = f;
      flash(t('god.fidelitySurvive', { stable: f.stable, bodies: f.bodies }));
    } catch (e) {
      fail(e);
    } finally {
      busy = null;
    }
  }

  async function runSyncBackfill(slug: string): Promise<void> {
    busy = 'backfill:' + slug;
    error = null;
    try {
      const r = await api.adminSyncBackfill(slug);
      syncStatus[slug] = await api.adminSync(slug);
      flash(
        r.partial
          ? `${slug}: partial — ${r.items} item(s), ${r.errors?.length ?? 0} project(s) incomplete`
          : `${slug}: ${r.items} item(s), ${r.modules} module(s), ${r.pruned} pruned`,
      );
    } catch (e) {
      fail(e);
    } finally {
      busy = null;
    }
  }

  async function loadAll(): Promise<void> {
    loading = true;
    error = null;
    try {
      [stats, instances, memberGroups, tokens, tuning] = await Promise.all([
        api.adminStats(),
        api.adminInstances(),
        api.adminMembers(),
        api.adminKiosk(),
        api.adminTuning(),
      ]);
      draft = { ...tuning.tuning };
      if (!mintInstance && instances.length) mintInstance = instances[0].slug;
      // Census only — cheap, and it is what tells you whether the mirror is
      // keeping up. A failure here must not blank the rest of God Mode.
      for (const i of instances) {
        try {
          syncStatus[i.slug] = await api.adminSync(i.slug);
        } catch {
          // an instance without a mirror simply has no card body
        }
      }
      try {
        outbox = await api.adminOutbox();
      } catch {
        // the queue is a diagnostic; failing to read it must not blank God Mode
      }
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
      flash(t('toast.cachesRefreshed'));
    });

  const tick = () =>
    run('tick', async () => {
      await api.adminTick();
      flash(t('toast.tickTriggered'));
    });

  const toggleAllOrgs = () =>
    run('allorgs', async () => {
      store.allOrgs = !store.allOrgs;
      await store.refresh();
      flash(t(store.allOrgs ? 'toast.crossOrgOn' : 'toast.crossOrgOff'));
    });

  const mintKiosk = () =>
    run('mint', async () => {
      if (!mintInstance) {
        error = t('toast.pickInstance');
        return;
      }
      const k = await api.adminKioskCreate(mintInstance, mintName.trim());
      tokens = [...tokens, k];
      mintName = '';
      flash(t('toast.kioskMinted'));
    });

  const revokeKiosk = (tok: KioskToken) =>
    run('revoke:' + tok.token, async () => {
      await api.adminKioskRevoke(tok.token);
      tokens = tokens.filter((x) => x.token !== tok.token);
      flash(t('toast.kioskRevoked'));
    });

  // Turning this on lets the cooling sweep move cards in Plane: 😴 → Backlog,
  // 🪦 → Cancelled, 🔥 → In Progress. A thread a human moves is handed off and
  // never auto-managed again (THREAD-LIFECYCLE.md Phase B).
  const toggleAutoState = (i: AdminInstance) =>
    run('autostate:' + i.slug, async () => {
      if (!i.auto_state) {
        const ok = confirm(
          `Let Bubble move cards in Plane for "${i.slug}"?\n\n` +
            `The cooling sweep will move work items between Backlog, In Progress and Cancelled ` +
            `to match their derived level. Threads someone moves by hand are left alone from then on.`,
        );
        if (!ok) return;
      }
      const res = await api.adminAutoState(i.slug, !i.auto_state);
      instances = instances.map((x) => (x.slug === i.slug ? { ...x, auto_state: res.auto_state } : x));
      flash(t(res.auto_state ? 'toast.autoStateOn' : 'toast.autoStateOff', { slug: i.slug }));
    });

  // ---- buoyancy tuning (THREAD-LIFECYCLE.md) ----

  const fieldsIn = (group: TuningField['group']): TuningField[] =>
    tuning?.fields.filter((f) => f.group === group) ?? [];

  // only the knobs that actually differ get sent, so an Apply is a real diff.
  const changed = $derived.by<TuningKey[]>(() => {
    if (!tuning || !draft) return [];
    return (Object.keys(draft) as TuningKey[]).filter((k) => draft![k] !== tuning!.tuning[k]);
  });

  function isDefault(k: TuningKey): boolean {
    return !!tuning && !!draft && draft[k] === tuning.defaults[k];
  }

  function setKnob(k: TuningKey, v: number | boolean): void {
    if (draft) draft = { ...draft, [k]: v };
  }

  function revertDraft(): void {
    if (tuning) draft = { ...tuning.tuning };
  }

  const applyTuning = () =>
    run('tuning', async () => {
      if (!draft || changed.length === 0) return;
      const patch: Record<string, number | boolean> = {};
      for (const k of changed) patch[k] = draft[k];
      tuning = await api.adminTuningSet(patch);
      draft = { ...tuning.tuning }; // the server may have clamped a value
      await store.refresh();
      flash(t('toast.applied', { n: changed.length }));
    });

  const resetTuning = () =>
    run('tuning-reset', async () => {
      if (!tuning) return;
      tuning = await api.adminTuningSet({ ...tuning.defaults });
      draft = { ...tuning.tuning };
      await store.refresh();
      flash(t('toast.calibrationReset'));
    });

  function kioskUrl(tok: KioskToken): string {
    return `${location.origin}/?kiosk=${encodeURIComponent(tok.token)}`;
  }

  async function copy(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      flash(t('toast.copied'));
    } catch {
      flash(t('toast.copyFailed'));
    }
  }
</script>

<div class="screen" aria-label={t('god.aria')}>
  <div class="topbar">
    <button class="back" onclick={() => store.closeGodMode()} aria-label={t('thread.back')}>
      <span aria-hidden="true">←</span> board
    </button>
    <span class="bolt" aria-hidden="true">⚡</span>
    <h2 class="ttl">{t('god.title')}</h2>
    {#if store.allOrgs}<span class="chip on">{t('god.allOrgs')}</span>{/if}
    {#if toast}<span class="toast" role="status">{toast}</span>{/if}
  </div>

  {#if !store.godmode}
    <p class="pad dim">{t('god.adminOnly')}</p>
  {:else if loading}
    <p class="pad dim">{t('board.loading')}</p>
  {:else}
    {#if error}<p class="err pad">{error}</p>{/if}

    <div class="grid">
      <!-- stats -->
      <section class="card">
        <h3>{t('god.server')}</h3>
        {#if stats}
          <dl class="kv">
            <dt>{t('god.revision')}</dt>
            <dd>{stats.revision ? stats.revision.slice(0, 10) : '—'}</dd>
            <dt>{t('god.built')}</dt>
            <dd>{stats.built || '—'}</dd>
            <dt>{t('god.started')}</dt>
            <dd>{new Date(stats.started_at).toLocaleString()}</dd>
            <dt>{t('god.instances')}</dt>
            <dd>{stats.instances} ({stats.cached_instances} cached)</dd>
            <dt>{t('god.identities')}</dt>
            <dd>{stats.cached_identities} cached</dd>
          </dl>
        {/if}
      </section>

      <!-- plane rate budget -->
      <section class="card">
        <h3>{t('god.rateBudget')}</h3>
        {#if stats?.rate_budgets?.length}
          {#each stats.rate_budgets as b (b.instance)}
            <div class="budget">
              <div class="bhead">
                <strong>{b.instance}</strong>
                {#if b.known}
                  <span class="bnum" class:low={b.remaining <= b.floor}>
                    {b.remaining}{b.limit > 0 ? ` / ${b.limit}` : ''}
                  </span>
                {:else}
                  <span class="bnum unknown">—</span>
                {/if}
              </div>
              {#if b.known}
                <!-- the reserved slice is drawn as a distinct zone, so "thin" is
                     visible at a glance rather than requiring arithmetic -->
                <div
                  class="bar"
                  title="{b.remaining} left, {b.floor} reserved for interactive calls"
                >
                  <div
                    class="fill"
                    class:low={b.remaining <= b.floor}
                    style="width: {b.limit > 0
                      ? Math.min(100, Math.max(2, (b.remaining / b.limit) * 100))
                      : 0}%"
                  ></div>
                  {#if b.limit > 0}
                    <div class="floor" style="width: {Math.min(100, (b.floor / b.limit) * 100)}%"></div>
                  {/if}
                </div>
                <p class="bmeta">
                  resets in {b.reset_in}s · {b.spent} spent · {b.throttled} × 429 · {b.waits} bg
                  {b.waits === 1 ? 'yield' : 'yields'}
                </p>
                {#if b.remaining <= b.floor}
                  <p class="bwarn">{t('god.atFloor')}</p>
                {/if}
              {:else}
                <p class="bmeta">
                  Nothing observed yet — no calls made, or this Plane omits the rate headers.
                </p>
              {/if}
            </div>
          {/each}
        {:else}
          <p class="bmeta">{t('god.noInstances')}</p>
        {/if}
      </section>

      <!-- outbox: writes that have not reached Plane (PLANE-SYNC.md Phase 5) -->
      <section class="card wide">
        <h3>
          Outbox
          {#if outbox && (outbox.pending || outbox.abandoned)}
            <span class="bnum" class:low={!!outbox.abandoned}>
              {outbox.pending} pending{outbox.abandoned ? ` · ${outbox.abandoned} abandoned` : ''}
            </span>
          {/if}
        </h3>
        {#if !outbox || !outbox.entries?.length}
          <p class="bmeta">{t('god.outboxEmpty')}</p>
        {:else}
          <p class="bmeta">
            Auto-state moves retry on their own. Comment drafts hold no credential by
            design, so only their author can re-send one — from the thread it was
            written in.
          </p>
          {#each outbox.entries as e (e.id)}
            <div class="budget">
              <div class="bhead">
                <strong>{e.status === 'abandoned' ? '✖' : '⧗'} {e.summary}</strong>
                <span class="bnum">{e.instance}</span>
              </div>
              <p class="bmeta">
                {e.kind} · attempts {e.attempts}{e.next_at ? ` · next ${new Date(e.next_at).toLocaleTimeString()}` : ''}
                {e.field_lock ? ` · holding "${e.field_lock}"` : ''}
              </p>
              {#if e.last_error}<p class="bwarn">{e.last_error}</p>{/if}
              <div class="actions tight">
                <button onclick={() => dropOutbox(e.id)} disabled={busy !== null}>
                  {busy === 'outbox:' + e.id ? '…' : 'Drop'}
                </button>
              </div>
            </div>
          {/each}
        {/if}
      </section>

      <!-- plane mirror (PLANE-SYNC.md) -->
      <section class="card wide">
        <h3>{t('god.mirror')}</h3>
        <p class="bmeta">
          A local SQLite copy of Plane, kept current by one background worker. In
          shadow mode nothing reads from it yet — <em>sync-diff</em> is the gate that
          proves it agrees with Plane before anything does.
        </p>
        {#each instances as i (i.slug)}
          {@const st = syncStatus[i.slug]}
          {@const d = syncDiffs[i.slug]}
          <div class="budget">
            <div class="bhead">
              <strong>{i.slug}</strong>
              {#if st}
                <span class="bnum">{st.items} items · {st.modules} bubbles</span>
              {:else}
                <span class="bnum unknown">{t('god.noMirror')}</span>
              {/if}
            </div>
            {#if st}
              <p class="bmeta">
                {st.projects} projects · {st.states} states · {st.members} members ·
                {st.comments} comments
              </p>
              <p class="bmeta">
                last full: {st.last_full ? new Date(st.last_full).toLocaleString() : '—'} ·
                watermark: {st.watermark ? new Date(st.watermark).toLocaleString() : '—'}
              </p>
              {#if st.last_error}
                <!-- A stale mirror that says nothing is the failure mode this
                     whole refactor must avoid, so it is surfaced loudly. -->
                <p class="bwarn">stale: {st.last_error}</p>
              {/if}
              <div class="actions tight">
                <button onclick={() => runSyncDiff(i.slug)} disabled={busy !== null}>
                  {busy === 'diff:' + i.slug ? 'comparing…' : 'Compare with Plane'}
                </button>
                <button onclick={() => runSyncBackfill(i.slug)} disabled={busy !== null}>
                  {busy === 'backfill:' + i.slug ? 'walking…' : 'Backfill'}
                </button>
                <button onclick={() => runSyncFidelity(i.slug)} disabled={busy !== null}>
                  {busy === 'fidelity:' + i.slug
                    ? t('god.fidelityRunning')
                    : t('god.fidelity')}
                </button>
              </div>
              {@const fid = syncFidelity[i.slug]}
              {#if fid}
                {#if fid.stable === fid.bodies && fid.mentions === 0 && fid.assets === 0}
                  <p class="bok">{t('god.fidelityOk', { n: fid.bodies })}</p>
                {:else}
                  <p class="bwarn">
                    {t('god.fidelitySurvive', { stable: fid.stable, bodies: fid.bodies })}
                  </p>
                  {#if fid.mentions > 0 || fid.assets > 0}
                    <p class="bwarn">
                      {t('god.fidelityLost', { mentions: fid.mentions, assets: fid.assets })}
                    </p>
                    <p class="bmeta">{t('god.fidelityHint')}</p>
                  {/if}
                  <ul class="findings">
                    {#each fid.unstable ?? [] as u (u.thread_id)}
                      <li>{u.title} — line {u.line}</li>
                    {/each}
                  </ul>
                  {#if (fid.elided ?? 0) > 0}
                    <p class="bmeta">…and {fid.elided} more</p>
                  {/if}
                {/if}
              {/if}
              {#if d}
                {#if d.clean}
                  <p class="bok">✓ mirror matches Plane ({d.items} items compared)</p>
                {:else}
                  <p class="bwarn">{d.findings?.length ?? 0} finding(s):</p>
                  <ul class="findings">
                    {#each (d.findings ?? []).slice(0, 12) as f (f.id + f.field)}
                      <li>{f.text}</li>
                    {/each}
                  </ul>
                  {#if (d.findings?.length ?? 0) > 12}
                    <p class="bmeta">…and {(d.findings?.length ?? 0) - 12} more</p>
                  {/if}
                {/if}
              {/if}
            {/if}
          </div>
        {/each}
      </section>

      <!-- maintenance -->
      <section class="card">
        <h3>{t('god.maintenance')}</h3>
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
              <tr><th>{t('god.slug')}</th><th>{t('god.baseUrl')}</th><th>{t('god.workspace')}</th><th>{t('god.projectCol')}</th><th>{t('god.webhook')}</th><th>{t('god.cached')}</th><th>{t('god.writesToPlane')}</th></tr>
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
                  <td>
                    <button
                      class="mini"
                      class:on={i.auto_state}
                      onclick={() => toggleAutoState(i)}
                      disabled={busy === 'autostate:' + i.slug}
                      title={i.auto_state
                        ? 'the cooling sweep moves cards in Plane — click to stop'
                        : 'Bubble only reads Plane — click to let it move cards'}
                    >
                      {busy === 'autostate:' + i.slug ? '…' : i.auto_state ? 'auto-state on' : 'read-only'}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <!-- members (read-only; Plane owns membership) -->
      <section class="card wide">
        <h3>{t('god.members')}</h3>
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
                    <tr><th>{t('god.nameCol')}</th><th>{t('god.emailCol')}</th><th>{t('god.roleCol')}</th></tr>
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

      <!-- buoyancy calibration: the thresholds behind every band -->
      <section class="card wide">
        <h3>{t('god.calibration')}</h3>
        <p class="hint" style="margin-top:0">
          The thresholds that decide how bubbles <em>and</em> individual threads move between 🔥 😴 🪦 🏆.
          Everything is derived at read time, so a change lands on the board immediately — nothing is
          recomputed or migrated. Values are clamped server-side.
        </p>

        {#if tuning && draft}
          {#each TUNING_GROUPS as g (g.key)}
            <div class="tgroup">
              <div class="tgroup-head">
                <span class="tgroup-name">{i18n.tuning(`group.${g.key}`, g.label)}</span>
                <span class="dim">{i18n.tuning(`hint.${g.key}`, g.hint)}</span>
              </div>
              {#each fieldsIn(g.key) as f (f.key)}
                <div class="knob" class:dirty={draft[f.key] !== tuning.tuning[f.key]}>
                  <div class="knob-main">
                    {#if f.kind === 'toggle'}
                      <label class="tgl">
                        <input
                          type="checkbox"
                          checked={draft[f.key] as boolean}
                          onchange={(e) => setKnob(f.key, e.currentTarget.checked)}
                        />
                        <span>{i18n.tuning(f.key, f.label)}</span>
                      </label>
                    {:else}
                      <label class="num">
                        <span>{i18n.tuning(f.key, f.label)}</span>
                        <input
                          type="number"
                          min={f.min}
                          max={f.max}
                          step={f.step}
                          value={draft[f.key] as number}
                          onchange={(e) => setKnob(f.key, Number(e.currentTarget.value))}
                        />
                      </label>
                    {/if}
                    {#if !isDefault(f.key)}
                      <span class="chip" title={t('god.differsDefault')}>{t('god.modified')}</span>
                    {/if}
                  </div>
                  <p class="knob-help">{i18n.tuning(`help.${f.key}`, f.help)}</p>
                  <code class="knob-key">{f.key}</code>
                </div>
              {/each}
            </div>
          {/each}

          <div class="actions tune-actions">
            <button onclick={applyTuning} disabled={busy === 'tuning' || changed.length === 0}>
              {busy === 'tuning' ? '…' : changed.length ? t('god.applyChanges', { n: changed.length }) : t('god.noChanges')}
            </button>
            <button onclick={revertDraft} disabled={changed.length === 0}>{t('god.discardEdits')}</button>
            <button class="mini danger" onclick={resetTuning} disabled={busy === 'tuning-reset'}>
              {busy === 'tuning-reset' ? '…' : t('god.resetDefaults')}
            </button>
          </div>
        {/if}
      </section>

      <!-- kiosk tokens -->
      <section class="card wide">
        <h3>Kiosk display tokens ({tokens.length})</h3>
        <div class="mint">
          <select bind:value={mintInstance} aria-label={t('god.instance')}>
            {#each instances as i (i.slug)}
              <option value={i.slug}>{i.slug}</option>
            {/each}
          </select>
          <input placeholder={t('god.kioskLabel')} bind:value={mintName} />
          <button onclick={mintKiosk} disabled={busy === 'mint'}>
            {busy === 'mint' ? '…' : t('god.mintToken')}
          </button>
        </div>

        {#if tokens.length === 0}
          <p class="hint">{t('god.noKiosk')}</p>
        {:else}
          <ul class="tokens">
            {#each tokens as tok (tok.token)}
              <li>
                <div class="tok-main">
                  <span class="tok-name">{tok.name || '(unnamed)'}</span>
                  <span class="chip">{tok.instance}</span>
                  <span class="dim tok-when">{new Date(tok.created_at).toLocaleDateString()}</span>
                </div>
                <div class="tok-url">
                  <code>{kioskUrl(tok)}</code>
                  <button class="mini" onclick={() => copy(kioskUrl(tok))} title={t('god.copyKiosk')}>{t('god.copyKiosk')}</button>
                  <button
                    class="mini danger"
                    onclick={() => revokeKiosk(tok)}
                    disabled={busy === 'revoke:' + tok.token}
                    title={t('god.revoke')}>{t('god.revoke')}</button
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

  /* plane rate budget */
  .budget + .budget {
    margin-top: 0.9rem;
    padding-top: 0.9rem;
    border-top: 1px solid var(--line);
  }
  .bhead {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .bnum {
    font-variant-numeric: tabular-nums;
    font-size: 0.85rem;
    color: var(--text);
  }
  .bnum.low {
    color: var(--warn, #d97706);
    font-weight: 700;
  }
  .bnum.unknown {
    color: var(--faint);
  }
  .bar {
    position: relative;
    height: 6px;
    margin: 0.4rem 0 0.35rem;
    border-radius: 999px;
    background: var(--hover);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    border-radius: 999px;
    background: var(--accent, #16a34a);
    transition: width 0.3s ease;
  }
  .fill.low {
    background: var(--warn, #d97706);
  }
  /* the reserved slice, drawn under the fill so it reads as a boundary */
  .floor {
    position: absolute;
    inset: 0 auto 0 0;
    border-right: 1px dashed var(--faint);
    pointer-events: none;
  }
  .bmeta {
    margin: 0;
    font-size: 0.72rem;
    color: var(--faint);
    font-variant-numeric: tabular-nums;
  }
  .bwarn {
    margin: 0.3rem 0 0;
    font-size: 0.72rem;
    color: var(--warn, #d97706);
  }
  .bok {
    margin: 0.3rem 0 0;
    font-size: 0.72rem;
    color: var(--accent, #16a34a);
  }
  .actions.tight {
    margin-top: 0.5rem;
  }
  /* findings are the point of the card when non-empty, so give them room to be
     read rather than truncating each line */
  .findings {
    margin: 0.35rem 0 0;
    padding-left: 1.1rem;
    font-size: 0.72rem;
    color: var(--muted);
    max-height: 12rem;
    overflow-y: auto;
  }
  .findings li {
    margin-bottom: 0.2rem;
    word-break: break-word;
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

  /* ---- buoyancy calibration ---- */
  .tgroup {
    margin-top: 1rem;
  }
  .tgroup-head {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    flex-wrap: wrap;
    padding-bottom: 0.3rem;
    border-bottom: 1px solid var(--line);
    font-size: 0.82rem;
  }
  .tgroup-name {
    font-weight: 700;
    color: var(--text);
  }
  .knob {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 0.15rem 0.75rem;
    padding: 0.6rem 0.6rem 0.6rem 0.7rem;
    border-left: 2px solid transparent;
    border-radius: 0 9px 9px 0;
  }
  .knob:hover {
    background: var(--hover);
  }
  /* an edit that hasn't been applied yet */
  .knob.dirty {
    border-left-color: var(--wip);
    background: color-mix(in oklab, var(--wip) 8%, transparent);
  }
  .knob-main {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
    font-size: 0.86rem;
    color: var(--text);
  }
  .knob-help {
    grid-column: 1 / -1;
    margin: 0;
    font-size: 0.76rem;
    line-height: 1.4;
    color: var(--faint);
    max-width: 62ch;
  }
  .knob-key {
    grid-row: 1;
    grid-column: 2;
    align-self: center;
    font-size: 0.68rem;
    color: var(--faint);
    white-space: nowrap;
  }
  .tgl,
  .num {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }
  .num input {
    width: 6.5rem;
    padding: 0.3rem 0.5rem;
    border-radius: 8px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    color: var(--text);
    font: inherit;
    font-size: 0.82rem;
  }
  .tgl input {
    width: 1rem;
    height: 1rem;
    accent-color: var(--wip);
    cursor: pointer;
  }
  .tune-actions {
    margin-top: 1rem;
    padding-top: 0.85rem;
    border-top: 1px solid var(--line);
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
  .mini.on {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 50%, transparent);
    font-weight: 700;
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
