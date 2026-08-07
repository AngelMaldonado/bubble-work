<script lang="ts">
  import { t } from '../lib/i18n.svelte';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import Combo from './Combo.svelte';

  let { onclose }: { onclose: () => void } = $props();

  let fInstance = $state(store.instance || store.instances[0] || '');
  let fProject = $state('');
  let name = $state('');
  let outcome = $state('');
  let owner = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  // Projects available for the chosen instance, derived from existing bubbles
  // (a bubble is a module inside a project). New bubbles land in active projects.
  const projects = $derived.by<{ id: string; name: string }[]>(() => {
    const seen = new Map<string, string>();
    for (const b of store.bubbles) {
      if (b.instance !== fInstance) continue;
      if (!seen.has(b.project)) seen.set(b.project, b.project_name || b.project);
    }
    return [...seen.entries()]
      .map(([id, n]) => ({ id, name: n }))
      .sort((a, b) => a.name.localeCompare(b.name));
  });

  const projectOpts = $derived(projects.map((p) => ({ value: p.id, label: p.name })));

  // members active in the chosen instance — options for the owner combobox
  // (free-text still allowed, so an owner outside this set can be typed).
  const memberOpts = $derived.by<{ value: string; label: string }[]>(() => {
    const seen = new Set<string>();
    for (const b of store.bubbles) {
      if (b.instance !== fInstance) continue;
      for (const m of b.members ?? []) seen.add(m);
    }
    return [...seen].sort((a, b) => a.localeCompare(b)).map((m) => ({ value: m, label: m }));
  });

  // keep the project selection valid as the instance changes
  $effect(() => {
    if (!projects.some((p) => p.id === fProject)) {
      fProject = store.project && projects.some((p) => p.id === store.project) ? store.project : (projects[0]?.id ?? '');
    }
  });

  const ready = $derived(fInstance !== '' && fProject !== '' && name.trim() !== '');

  async function submit(e: Event): Promise<void> {
    e.preventDefault();
    if (!ready || busy) return;
    busy = true;
    err = null;
    try {
      await api.createBubble({
        instance: fInstance,
        project: fProject,
        name: name.trim(),
        outcome: outcome.trim() || undefined,
        owner: owner.trim() || undefined,
      });
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
    <h2>{t('form.newBubble')}</h2>
    <span class="into">{t('form.newBubbleHint')}</span>
  </header>

  <div class="two">
    {#if store.instances.length > 1}
      <div>
        <label for="cb-inst">{t('form.instance')}</label>
        <select id="cb-inst" bind:value={fInstance} disabled={busy}>
          {#each store.instances as slug (slug)}
            <option value={slug}>{slug}</option>
          {/each}
        </select>
      </div>
    {/if}
    <div>
      <label for="cb-proj">{t('form.project')}</label>
      {#if projects.length}
        <Combo options={projectOpts} bind:value={fProject} ariaLabel="project" placeholder={t('form.pickProject')} />
      {:else}
        <p class="empty">{t('form.noProjects')} <code>bubble workspace</code>.</p>
      {/if}
    </div>
  </div>

  <label for="cb-name">{t('form.name')}</label>
  <input id="cb-name" bind:value={name} placeholder={t('form.namePlaceholder')} disabled={busy} />

  <label for="cb-outcome">{t('form.outcome')} <span class="req">{t('form.optionalDone')}</span></label>
  <input id="cb-outcome" bind:value={outcome} placeholder={t('form.outcomePlaceholder')} disabled={busy} />

  <label for="cb-owner">{t('form.owner')} <span class="req">{t('form.optionalOwner')}</span></label>
  <Combo options={memberOpts} bind:value={owner} ariaLabel="owner" placeholder={t('form.ownerPlaceholder')} allowCustom />


  {#if err}<p class="err">{err}</p>{/if}

  <div class="actions">
    <button type="button" class="ghost" onclick={onclose} disabled={busy}>{t('form.cancel')}</button>
    <button type="submit" class="go" disabled={!ready || busy}>
      {busy ? t('form.creating') : t('form.createBubble')}
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
    top: 12vh;
    left: 50%;
    transform: translateX(-50%);
    width: min(94vw, 560px);
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
    font-size: 0.78rem;
    color: var(--faint);
    text-align: right;
  }
  label {
    font-size: 0.76rem;
    color: var(--muted);
    font-weight: 600;
    margin-top: 0.3rem;
  }
  .req {
    color: var(--faint);
    font-weight: 400;
    font-style: italic;
    margin-left: 0.3rem;
  }
  input,
  select {
    width: 100%;
    padding: 0.55rem 0.7rem;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 6%, transparent);
    color: var(--text);
    outline: none;
    font-size: 0.9rem;
  }
  input:focus,
  select:focus {
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }
  .two > div {
    display: grid;
    gap: 0.3rem;
    align-content: start;
  }
  .empty {
    margin: 0.3rem 0 0;
    font-size: 0.78rem;
    color: var(--faint);
  }
  .err {
    margin: 0.3rem 0 0;
    color: oklch(0.62 0.2 25);
    font-size: 0.82rem;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.6rem;
    margin-top: 0.8rem;
  }
  .ghost,
  .go {
    padding: 0.5rem 1rem;
    border-radius: 10px;
    border: 1px solid var(--line);
    cursor: pointer;
    font-size: 0.85rem;
  }
  .ghost {
    background: transparent;
    color: var(--muted);
  }
  .go {
    background: var(--wip);
    border-color: transparent;
    color: white;
    font-weight: 700;
  }
  .go:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
