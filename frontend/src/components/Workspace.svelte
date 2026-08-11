<script lang="ts">
  import { store } from '../lib/store.svelte';
  import { i18n, t } from '../lib/i18n.svelte';
  import { theme } from '../lib/theme.svelte';
  import { tour } from '../lib/tour.svelte';
  import McpConnect from './McpConnect.svelte';
  import { LEVELS } from '../lib/types';
  import type { BubbleView } from '../lib/types';
  import Band from './Band.svelte';
  import Minimap from './Minimap.svelte';
  import Omnibar from './Omnibar.svelte';
  import BirthForm from './BirthForm.svelte';
  import CreateBubbleForm from './CreateBubbleForm.svelte';
  import BubbleContextMenu from './BubbleContextMenu.svelte';
  import BoardContextMenu from './BoardContextMenu.svelte';
  import BubbleDetail from './BubbleDetail.svelte';
  import ProjectCombobox from './ProjectCombobox.svelte';
  import WorkspaceMenu from './WorkspaceMenu.svelte';
  import WorkspaceSwitcher from './WorkspaceSwitcher.svelte';
  import WorkspaceForm from './WorkspaceForm.svelte';
  import ConfirmDelete from './ConfirmDelete.svelte';
  import ViewCombobox from './ViewCombobox.svelte';
  import { boardMenu, workspaceMenu } from '../lib/contextmenu.svelte';
  import { api, ApiError } from '../lib/api';

  let omni = $state(false);
  let mcpOpen = $state(false);
  let birthTarget = $state<BubbleView | null>(null);
  let showCreate = $state(false);

  // The workspace tier: create, rename, delete (docs/ARTIFACT-EDITING.md).
  let wsForm = $state<{ mode: 'create' | 'rename'; id: string; current: string } | null>(null);
  let wsPendingDelete = $state<{ id: string; name: string; bubbles: number; threads: number } | null>(
    null,
  );
  let wsDeleting = $state(false);

  // The workspace being acted on — the filtered one, or the only one there is.
  const scopedWorkspace = $derived(store.activeProject);

  // What a delete would cost, counted from what we can already see. The server
  // counts again for real before it deletes; this is only so the dialog can say
  // what goes without a round trip.
  function wsCost(projectId: string): { bubbles: number; threads: number } {
    let bubbles = 0;
    let threads = 0;
    for (const b of store.bubbles) {
      if (b.project !== projectId) continue;
      bubbles++;
      threads += b.threads ?? 0;
    }
    return { bubbles, threads };
  }

  function wsIdOf(projectId: string): string {
    return `${store.instanceOf(projectId)}:${projectId}`;
  }

  function armWorkspaceDelete(): void {
    const p = scopedWorkspace;
    if (!p) return;
    wsPendingDelete = { id: wsIdOf(p.id), name: p.name, ...wsCost(p.id) };
  }

  async function confirmWorkspaceDelete(): Promise<void> {
    if (!wsPendingDelete || wsDeleting) return;
    wsDeleting = true;
    try {
      const res = await api.deleteWorkspace(wsPendingDelete.id);
      store.flash = t('ws.deleted', {
        name: wsPendingDelete.name,
        b: res.deleted_bubbles,
        n: res.deleted_threads,
      });
      // The filter now points at a workspace that no longer exists.
      store.selectProject('');
      wsPendingDelete = null;
      await store.refresh();
    } catch (e) {
      store.error = e instanceof ApiError ? e.message : String(e);
    } finally {
      wsDeleting = false;
    }
  }

  // a kiosk display is a passive read-only screen: no ⌘K, no commands, no birth.
  const kiosk = $derived(store.kiosk);

  function onKey(e: KeyboardEvent) {
    if (kiosk) return;
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      omni = true;
    }
  }

  function startBirth(b: BubbleView) {
    omni = false;
    birthTarget = b;
  }

  const unread = $derived(store.inbox?.unread_count ?? 0);
  const themeLabel = $derived(
    theme.choice === 'system' ? 'system' : theme.choice === 'dark' ? 'dark' : 'light',
  );

  // auto-dismiss transient godmode/info notices
  $effect(() => {
    if (store.flash) {
      const t = setTimeout(() => (store.flash = null), 4500);
      return () => clearTimeout(t);
    }
  });

  // "3m" reads better than "180 seconds" in a banner someone glances at.
  function fmtBehind(sec: number): string {
    if (sec < 90) return `${Math.max(1, Math.round(sec))}s`;
    if (sec < 5400) return `${Math.round(sec / 60)}m`;
    return `${Math.round(sec / 3600)}h`;
  }
</script>

<svelte:window onkeydown={onKey} />

<div
  class="app"
  oncontextmenu={(e) => {
    // The board's own menu. A bubble stops propagation, so its menu wins.
    e.preventDefault();
    boardMenu.show(e.clientX, e.clientY);
  }}
  role="presentation"
>
  <!-- the one fixed floating bubble, top-left; doubles as the ⌘K launcher
       (a kiosk display is passive, so it's just a mark there) -->
  {#if kiosk}
    <div class="brand" aria-label={t('chrome.kiosk')}>
      <span class="orb"></span>
      <span class="wordmark">bubble.work</span>
      <span class="kiosk-tag">{t('chrome.kioskBadge')}</span>
    </div>
  {:else}
    <button
      class="brand"
      data-tour="brand"
      onclick={() => (omni = true)}
      title={t('chrome.searchCommands')}
    >
      <span class="orb"></span>
      <span class="wordmark">bubble.work</span>
    </button>
  {/if}

  {#if store.status?.stale}
    <!-- The board renders from a local mirror, so a stopped sync would leave it
         looking perfectly healthy while quietly ageing. Being behind is fine;
         being behind silently is not (PLANE-SYNC.md Phase 7). -->
    <div class="banner stale" role="status">
      ⧗ {t('status.stale')}
      {#each store.status.instances ?? [] as i (i.instance)}
        {#if i.stale}
          <span class="stale-inst">
            {i.instance}: {i.last_ok
              ? t('status.lastSynced', { ago: fmtBehind(i.behind_seconds ?? 0) })
              : t('status.neverSynced')}
          </span>
        {/if}
      {/each}
    </div>
  {/if}
  {#if store.status?.unsent_drafts}
    <div class="banner unsent" role="status">
      ⧗ {i18n.plural('status.unsent', store.status.unsent_drafts)}
    </div>
  {/if}
  {#if store.error}
    <div class="banner">{store.error}</div>
  {/if}
  {#if store.flash}
    <div class="flash" role="status">{store.flash}</div>
  {/if}

  <main class="canvas">
    {#each LEVELS as lv (lv.key)}
      <div id="band-{lv.key}" class="band-anchor">
        <Band
          level={lv.key}
          label={lv.label}
          icon={lv.icon}
          bubbles={store.byLevel(lv.key)}
          collapsible={lv.key === 'rip' || lv.key === 'done'}
          startCollapsed={false}
        />
      </div>
    {/each}

    {#if store.visible.length === 0}
      <div class="hollow">
        <p>{t('chrome.noBubbles')}</p>
        <p class="dim">{t('chrome.press')} <b>⌘K</b>, {t('chrome.emptyHint', { cmd: '>', new: t('chrome.newBubbleCmd') })}</p>
      </div>
    {/if}
  </main>

  <footer class="statusbar">
    <div class="side">
      {#if store.instances.length > 1}
        <select
          class="picker"
          value={store.instance}
          onchange={(e) => store.selectInstance(e.currentTarget.value)}
          aria-label={t('combo.instanceFilter')}
        >
          <option value="">{t('combo.allInstances')}</option>
          {#each store.instances as slug (slug)}
            <option value={slug}>{slug}</option>
          {/each}
        </select>
      {:else if store.instances.length === 1}
        <span class="chip">{store.instances[0]}</span>
      {/if}

      {#if store.projects.length > 1}
        <span class="projwrap"><ProjectCombobox /></span>
      {/if}
      {#if !kiosk && store.instances.length > 0}
        <button
          class="wsdots"
          aria-haspopup="menu"
          aria-expanded={workspaceMenu.open}
          title={t('ws.menuTitle')}
          onclick={(e) => workspaceMenu.showAt(e.currentTarget)}
        >⋯</button>
      {/if}

      <!-- view scope: Everyone · Mine · <assignee> (Phase 9), same combobox as
           the project filter. "Mine" is hidden on a kiosk (no personal identity). -->
      <span class="projwrap"><ViewCombobox /></span>

      <span class="who" title={store.actor?.email}>
        {store.actor?.name ?? store.actor?.email ?? '—'}
      </span>

      {#if unread > 0}
        <span class="chip alert" title={t('board.unreadNotices')}>✉ {unread}</span>
      {/if}

      {#if store.godmode}
        <button
          class="god-enter"
          onclick={() => store.openGodMode()}
          title={t('chrome.openGodMode')}
        >
          ⚡ God Mode{store.allOrgs ? ' · all orgs' : ''}
        </button>
      {/if}
    </div>

    <div class="side right">
      {#if kiosk}
        <span class="hint">{t('chrome.readonly')}</span>
        <button class="theme" onclick={() => store.exitKiosk()} title={t('chrome.exitKiosk')}>
          <span class="tico">⎋</span>
          <span class="tlabel">{t('chrome.exitKioskShort')}</span>
        </button>
      {:else}
        <span class="hint">{t('chrome.searchHint')} · <b>&gt;</b> {t('chrome.commandsHint')}</span>
      {/if}
      <!-- the tour lives with the display badges: it is help, not an action -->
      {#if !kiosk}
        <button class="theme tour" onclick={() => tour.start()} title={t('tour.start')}>
          <span class="tico">?</span>
        </button>
      {/if}
      <!-- connecting an agent sits left of the language pair: it is per-person
           setup, and it carries a credential, so it wants its own affordance -->
      {#if !kiosk}
        <button class="theme mcp" onclick={() => (mcpOpen = true)} title={t('mcp.title')}>
          <span class="tico">🔌</span>
          <span class="tlabel">{t('mcp.badge')}</span>
        </button>
      {/if}
      <!-- language sits immediately left of the theme badge and shares its
           shape: two adjacent display preferences should read as one pair. -->
      <button
        class="theme lang"
        onclick={() => i18n.toggle()}
        title="{i18n.label} — {i18n.lang === 'es' ? 'switch to English' : 'cambiar a español'}"
        aria-label={i18n.label}
      >
        <span class="tico">{i18n.flag}</span>
        <span class="tlabel">{i18n.code}</span>
      </button>
      <button
        class="theme"
        onclick={() => theme.cycle()}
        title="theme: {themeLabel} (click to change)"
      >
        <span class="tico">{theme.icon}</span>
        <span class="tlabel">{themeLabel}</span>
      </button>
      <span
        class="state"
        class:live={!store.error}
        class:err={!!store.error}
        title={store.error ? 'cannot reach the server' : store.polling ? 'syncing…' : 'connected'}
      >
        <span class="pip"></span>
        {store.error ? 'offline' : store.polling ? 'syncing' : 'live'}
      </span>
    </div>
  </footer>
</div>

<Minimap />
{#if !kiosk}
  <Omnibar
    bind:open={omni}
    onbirth={startBirth}
    onnewbubble={() => {
      omni = false;
      showCreate = true;
    }}
  />
  {#if birthTarget}
    <BirthForm bubble={birthTarget} onclose={() => (birthTarget = null)} />
  {/if}
  {#if mcpOpen}
    <McpConnect onclose={() => (mcpOpen = false)} />
  {/if}
  {#if showCreate}
    <CreateBubbleForm onclose={() => (showCreate = false)} />
  {/if}
{/if}
{#if store.detail}
  <BubbleDetail />
{/if}

<BubbleContextMenu onbirth={startBirth} />
<BoardContextMenu
  onnewbubble={() => (showCreate = true)}
  onsearch={() => (omni = true)}
/>
{#if !kiosk}
  <WorkspaceSwitcher
    enabled={!omni &&
      !showCreate &&
      !mcpOpen &&
      !birthTarget &&
      !wsForm &&
      !wsPendingDelete &&
      !store.detail &&
      !boardMenu.open &&
      !workspaceMenu.open}
  />
  <WorkspaceMenu
    onrename={() =>
      scopedWorkspace &&
      (wsForm = { mode: 'rename', id: wsIdOf(scopedWorkspace.id), current: scopedWorkspace.name })}
    oncreate={() => (wsForm = { mode: 'create', id: '', current: '' })}
    ondelete={armWorkspaceDelete}
    ondocs={() => scopedWorkspace && store.openPages(wsIdOf(scopedWorkspace.id))}
  />
  {#if wsForm}
    <WorkspaceForm
      mode={wsForm.mode}
      id={wsForm.id}
      current={wsForm.current}
      onclose={() => (wsForm = null)}
    />
  {/if}
  {#if wsPendingDelete}
    <ConfirmDelete
      what={wsPendingDelete.name}
      detail={t('del.workspace', { b: wsPendingDelete.bubbles, n: wsPendingDelete.threads })}
      prefer={t('del.workspacePrefer')}
      busy={wsDeleting}
      oncancel={() => (wsPendingDelete = null)}
      onconfirm={confirmWorkspaceDelete}
    />
  {/if}
{/if}

<style>
  /* the workspace menu handle: quiet until you go looking for it */
  .wsdots {
    flex: none;
    width: 1.6rem;
    height: 1.6rem;
    border-radius: 8px;
    border: 1px solid transparent;
    background: transparent;
    color: var(--faint);
    font-family: inherit;
    font-size: 0.95rem;
    line-height: 1;
    cursor: pointer;
  }
  .wsdots:hover {
    background: var(--hover);
    color: var(--text);
    border-color: var(--line);
  }
  .wsdots:focus-visible {
    outline: 2px solid var(--wip);
    outline-offset: 1px;
  }
  .app {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }
  /* fixed floating brand bubble */
  .brand {
    position: fixed;
    top: 1rem;
    left: 1.15rem;
    z-index: 30;
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.35rem 0.75rem 0.35rem 0.4rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface);
    backdrop-filter: blur(12px);
    box-shadow: 0 8px 24px var(--shadow);
    color: var(--text);
    cursor: pointer;
    transition:
      transform 0.15s ease,
      box-shadow 0.15s ease;
  }
  .brand:hover {
    transform: translateY(-1px);
    box-shadow: 0 12px 30px var(--shadow-strong);
  }
  .orb {
    width: 24px;
    height: 24px;
    border-radius: 999px;
    flex: none;
    background:
      radial-gradient(circle at 34% 30%, var(--sheen), transparent 42%),
      radial-gradient(circle at 65% 80%, color-mix(in oklab, var(--wip) 60%, transparent), transparent 70%),
      var(--wip);
    box-shadow: inset 0 1px 2px var(--sheen);
    animation: bob 3s ease-in-out infinite;
  }
  .wordmark {
    font-weight: 750;
    letter-spacing: -0.01em;
    font-size: 0.88rem;
  }
  .kiosk-tag {
    font-size: 0.62rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--wip);
    border: 1px solid color-mix(in oklab, var(--wip) 40%, transparent);
    border-radius: 999px;
    padding: 0.05rem 0.4rem;
  }

  /* stale: the board is real but ageing. Distinct from .banner (an error),
     because "slightly behind" and "broken" deserve different alarm. */
  .banner.stale,
  .banner.unsent {
    background: var(--warn-bg, rgba(217, 119, 6, 0.12));
    color: var(--warn, #b45309);
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: baseline;
  }
  .stale-inst {
    font-size: 0.72rem;
    opacity: 0.85;
    font-variant-numeric: tabular-nums;
  }

  .banner {
    margin: 0 0 0 0;
    padding: 0.6rem 1.25rem;
    padding-top: 4.2rem;
    background: color-mix(in oklab, oklch(0.6 0.18 25) 22%, transparent);
    color: oklch(0.45 0.16 25);
    font-size: 0.82rem;
  }
  :root[data-mode='dark'] .banner {
    color: oklch(0.9 0.08 25);
  }

  .canvas {
    flex: 1;
    width: min(1100px, 94vw);
    margin: 0 auto;
    padding: 4.5rem 3.25rem 5rem 0.75rem;
  }
  .band-anchor {
    scroll-margin-top: 5rem;
  }
  .hollow {
    text-align: center;
    color: var(--muted);
    margin-top: 3rem;
  }
  .hollow .dim {
    color: var(--faint);
    font-size: 0.85rem;
  }
  .hollow b {
    color: var(--text);
  }

  .statusbar {
    position: sticky;
    bottom: 0;
    z-index: 25;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.55rem 1.25rem;
    font-size: 0.75rem;
    color: var(--faint);
    background: color-mix(in oklab, var(--bg) 72%, transparent);
    backdrop-filter: blur(14px);
    border-top: 1px solid var(--line);
  }
  .side {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    min-width: 0;
  }
  .right {
    justify-content: flex-end;
  }
  .projwrap {
    display: inline-flex;
    flex: 0 0 auto;
    width: 9.5rem;
    max-width: 28vw;
  }
  .picker {
    background: var(--surface-solid);
    color: var(--text);
    border: 1px solid var(--line);
    border-radius: 9px;
    padding: 0.25rem 0.45rem;
    font-size: 0.75rem;
  }
  .chip {
    color: var(--muted);
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 0.15rem 0.55rem;
  }
  .chip.alert {
    color: var(--done);
    border-color: color-mix(in oklab, var(--done) 40%, transparent);
  }
  .god-enter {
    color: var(--wip);
    border: 1px solid color-mix(in oklab, var(--wip) 45%, transparent);
    border-radius: 999px;
    padding: 0.15rem 0.6rem;
    background: transparent;
    font-weight: 700;
    font-size: inherit;
    cursor: pointer;
  }
  .god-enter:hover {
    background: color-mix(in oklab, var(--wip) 16%, transparent);
  }
  .flash {
    padding: 0.55rem 1.25rem;
    padding-top: 4.2rem;
    background: color-mix(in oklab, var(--reviewed) 18%, transparent);
    color: var(--text);
    font-size: 0.82rem;
  }
  .who {
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 0 0 auto; /* never collapse to zero — always show the name */
    max-width: 16rem;
  }
  .hint b {
    color: var(--muted);
  }
  .theme {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    background: var(--surface-solid);
    color: var(--muted);
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 0.2rem 0.55rem 0.2rem 0.45rem;
    cursor: pointer;
  }
  .theme:hover {
    color: var(--text);
  }
  .tico {
    font-size: 0.85rem;
    line-height: 1;
  }
  /* The flag glyph renders taller than the theme icons, so it is nudged down to
     sit on the same optical baseline as ☀️/🌙 rather than the text baseline. */
  .lang .tico {
    font-size: 0.95rem;
    transform: translateY(0.5px);
  }
  .lang .tlabel {
    font-variant-numeric: tabular-nums;
    letter-spacing: 0.02em;
  }
  .tlabel {
    font-size: 0.72rem;
  }
  .state {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--faint);
    transition: color 0.3s ease;
  }
  .state .pip {
    width: 7px;
    height: 7px;
    border-radius: 999px;
    background: currentColor;
    box-shadow: 0 0 0 0 currentColor;
  }
  .state.live {
    color: oklch(0.7 0.16 145);
  }
  .state.live .pip {
    animation: pulse 2.4s ease-in-out infinite;
  }
  .state.err {
    color: oklch(0.68 0.19 25);
  }
  @keyframes pulse {
    0%,
    100% {
      box-shadow: 0 0 0 0 color-mix(in oklab, currentColor 60%, transparent);
    }
    50% {
      box-shadow: 0 0 0 4px transparent;
    }
  }

  @media (max-width: 560px) {
    .hint,
    .tlabel {
      display: none;
    }
  }
</style>
