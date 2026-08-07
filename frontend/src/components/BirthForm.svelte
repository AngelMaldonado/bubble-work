<script lang="ts">
  import { t } from '../lib/i18n.svelte';
  import type { BubbleView } from '../lib/types';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';

  let { bubble, onclose }: { bubble: BubbleView; onclose: () => void } = $props();

  let name = $state('');
  let problem = $state('');
  let outcome = $state('');
  let dod = $state('');
  let logbook = $state('');
  let small = $state(false);
  let busy = $state(false);
  let err = $state<string | null>(null);

  const ready = $derived(
    name.trim() !== '' &&
      problem.trim() !== '' &&
      outcome.trim() !== '' &&
      dod.trim() !== '' &&
      (small || logbook.trim() !== ''),
  );

  function buildBrief(): string {
    // The §3 birth artifact: problem/outcome/constraints + a required DoD section.
    return [
      `## Problem / opportunity`,
      problem.trim(),
      ``,
      `## Intended outcome`,
      outcome.trim(),
      ``,
      `## Definition of Done`,
      dod.trim(),
    ].join('\n');
  }

  async function submit(e: Event) {
    e.preventDefault();
    if (!ready) return;
    busy = true;
    err = null;
    try {
      await api.birth({
        instance: bubble.instance,
        bubble_id: bubble.id,
        name: name.trim(),
        brief: buildBrief(),
        logbook: small ? '' : logbook.trim(),
        small_thread: small,
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

<div class="scrim" role="button" tabindex="-1" onclick={onclose} onkeydown={(e) => e.key === 'Escape' && onclose()}></div>
<form class="modal" onsubmit={submit}>
  <header>
    <h2>{t('form.birthThread')}</h2>
    <span class="into">{t('form.into')} <b>{bubble.name}</b> · {bubble.instance}</span>
  </header>

  <label for="bf-name">{t('form.title')}</label>
  <input id="bf-name" bind:value={name} placeholder={t('form.titlePlaceholder')} disabled={busy} />

  <div class="two">
    <div>
      <label for="bf-problem">{t('form.problem')}</label>
      <textarea id="bf-problem" bind:value={problem} rows="3" placeholder={t('form.problemPlaceholder')} disabled={busy}></textarea>
    </div>
    <div>
      <label for="bf-outcome">{t('form.intended')}</label>
      <textarea id="bf-outcome" bind:value={outcome} rows="3" placeholder={t('form.intendedPlaceholder')} disabled={busy}></textarea>
    </div>
  </div>

  <label for="bf-dod">{t('form.dod')} <span class="req">{t('form.required')}</span></label>
  <textarea id="bf-dod" bind:value={dod} rows="3" placeholder={t('form.dodPlaceholder')} disabled={busy}></textarea>

  <label class="check">
    <input type="checkbox" bind:checked={small} disabled={busy} />
    {t('form.smallThread')}
  </label>

  {#if !small}
    <label for="bf-logbook">{t('form.logbook')} <span class="req">{t('form.logbookHint')}</span></label>
    <textarea id="bf-logbook" bind:value={logbook} rows="4" placeholder={'## Plan\n- [ ] first actionable todo\n\nOwner: …\nState: planning'} disabled={busy}></textarea>
  {/if}

  {#if err}<p class="err">{err}</p>{/if}

  <div class="actions">
    <button type="button" class="ghost" onclick={onclose} disabled={busy}>{t('form.cancel')}</button>
    <button type="submit" class="go" disabled={!ready || busy}>{busy ? t('form.birthing') : t('form.birth')}</button>
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
    top: 8vh;
    left: 50%;
    transform: translateX(-50%);
    width: min(94vw, 640px);
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
  }
  .into b {
    color: var(--muted);
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
  textarea {
    padding: 0.55rem 0.7rem;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 6%, transparent);
    color: var(--text);
    outline: none;
    width: 100%;
    resize: vertical;
    font-family: inherit;
  }
  input:focus,
  textarea:focus {
    border-color: var(--wip);
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }
  @media (max-width: 520px) {
    .two {
      grid-template-columns: 1fr;
    }
  }
  .check {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.8rem;
    color: var(--muted);
    font-weight: 400;
    margin-top: 0.5rem;
  }
  .check input {
    width: auto;
  }
  .err {
    margin: 0.3rem 0 0;
    color: oklch(0.72 0.19 25);
    font-size: 0.8rem;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.6rem;
    margin-top: 0.75rem;
  }
  .go,
  .ghost {
    padding: 0.6rem 1.1rem;
    border-radius: 11px;
    border: none;
    cursor: pointer;
    font-weight: 700;
  }
  .ghost {
    background: transparent;
    color: var(--muted);
    border: 1px solid var(--line);
  }
  .go {
    color: oklch(0.15 0.02 265);
    background: linear-gradient(180deg, oklch(0.8 0.19 45), var(--wip));
  }
  .go:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
