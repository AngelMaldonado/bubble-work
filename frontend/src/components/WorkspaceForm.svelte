<script lang="ts">
  import { untrack } from 'svelte';
  // Create or rename a WORKSPACE — a Plane project (AGENTS.md), the boundary for
  // a body of work. One form for both, because they ask the same question.
  //
  // Renaming is not evidence of production: what a body of work is called is not
  // what has been done, so nothing warms.
  import { t } from '../lib/i18n.svelte';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';

  let {
    mode,
    id = '',
    current = '',
    onclose,
  }: {
    mode: 'create' | 'rename';
    /** namespaced "<instance>:<project>" — rename only */
    id?: string;
    /** the name it has now — rename only */
    current?: string;
    onclose: () => void;
  } = $props();

  let fInstance = $state(store.instance || store.instances[0] || '');
  // Seeded once on purpose: this is an editable draft of the name, not a mirror
  // of the prop — later keystrokes must not be overwritten by a re-render.
  let name = $state(untrack(() => current));
  let identifier = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);
  let field = $state<HTMLInputElement | null>(null);

  // Renaming starts with the current name selected: the common case is editing
  // it, and the second most common is replacing it outright.
  $effect(() => {
    field?.focus();
    if (mode === 'rename') field?.select();
  });

  const trimmed = $derived(name.trim());
  const ready = $derived(
    trimmed !== '' &&
      (mode === 'create' ? fInstance !== '' : trimmed !== current.trim()),
  );

  async function submit(e: Event): Promise<void> {
    e.preventDefault();
    if (!ready || busy) return;
    busy = true;
    err = null;
    try {
      if (mode === 'create') {
        const ws = await api.createWorkspace({
          instance: fInstance,
          name: trimmed,
          identifier: identifier.trim() || undefined,
        });
        // A brand-new workspace holds no bubbles, and the board derives its
        // project list from bubbles — so it will not appear until it has one.
        // Say that, instead of leaving someone hunting for it.
        store.flash = t('ws.createdNeedsBubble', { name: ws.name });
      } else {
        await api.renameWorkspace(id, trimmed);
        store.flash = t('ws.renamed', { name: trimmed });
      }
      await store.refresh();
      onclose();
    } catch (e2) {
      err = e2 instanceof ApiError ? e2.message : String(e2);
    } finally {
      busy = false;
    }
  }
</script>

<div
  class="scrim"
  role="button"
  tabindex="-1"
  onclick={onclose}
  onkeydown={(e) => e.key === 'Escape' && onclose()}
></div>
<form class="modal" onsubmit={submit}>
  <header>
    <h2>{mode === 'create' ? t('ws.formNew') : t('ws.formRename')}</h2>
    <span class="into">{t('ws.isAProject')}</span>
  </header>

  {#if mode === 'create' && store.instances.length > 1}
    <label for="ws-inst">{t('form.instance')}</label>
    <select id="ws-inst" bind:value={fInstance} disabled={busy}>
      {#each store.instances as slug (slug)}
        <option value={slug}>{slug}</option>
      {/each}
    </select>
  {/if}

  <label for="ws-name">{t('form.name')}</label>
  <input
    id="ws-name"
    bind:this={field}
    bind:value={name}
    placeholder={t('ws.namePlaceholder')}
    disabled={busy}
  />

  {#if mode === 'create'}
    <label for="ws-ident">{t('ws.identifier')} <span class="req">{t('ws.identifierHint')}</span></label>
    <input
      id="ws-ident"
      bind:value={identifier}
      placeholder="KIOSK"
      disabled={busy}
      maxlength="12"
    />
  {/if}

  {#if err}<p class="err">{err}</p>{/if}

  <div class="actions">
    <button type="button" class="ghost" onclick={onclose} disabled={busy}>{t('form.cancel')}</button>
    <button type="submit" class="go" disabled={!ready || busy}>
      {busy
        ? t('ws.saving')
        : mode === 'create'
          ? t('ws.createBtn')
          : t('ws.renameBtn')}
    </button>
  </div>
</form>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 60;
    background: oklch(0.08 0.02 265 / 0.6);
    backdrop-filter: blur(4px);
    border: none;
  }
  .modal {
    position: fixed;
    z-index: 70;
    top: 14vh;
    left: 50%;
    transform: translateX(-50%);
    width: min(94vw, 460px);
    max-height: 84vh;
    overflow-y: auto;
    padding: 1.5rem;
    border-radius: 18px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 30px 80px var(--shadow-strong);
    display: grid;
    gap: 0.5rem;
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 0.4rem;
  }
  h2 {
    margin: 0;
    font-size: 1.15rem;
    font-weight: 750;
  }
  .into {
    font-size: 0.74rem;
    color: var(--faint);
    text-align: right;
  }
  label {
    font-size: 0.76rem;
    color: var(--muted);
    font-weight: 600;
  }
  .req {
    color: var(--faint);
    font-weight: 500;
  }
  input,
  select {
    width: 100%;
    padding: 0.5rem 0.65rem;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--surface);
    color: var(--text);
    font-family: inherit;
    font-size: 0.88rem;
  }
  input:focus-visible,
  select:focus-visible {
    outline: 2px solid var(--wip);
    outline-offset: 1px;
  }
  .err {
    margin: 0.2rem 0 0;
    font-size: 0.78rem;
    color: oklch(0.7 0.18 25);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 0.6rem;
  }
  .actions button {
    padding: 0.45rem 1rem;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 5%, transparent);
    color: var(--text);
    font-family: inherit;
    font-size: 0.82rem;
    font-weight: 700;
    cursor: pointer;
  }
  .actions .go {
    color: oklch(0.98 0 0);
    background: var(--wip);
    border-color: transparent;
  }
  .actions button:disabled {
    opacity: 0.55;
    cursor: default;
  }
</style>
