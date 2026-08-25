<script lang="ts">
  import { t } from '../lib/i18n.svelte';
  import type { BubbleView } from '../lib/types';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';

  let { bubble, onclose }: { bubble: BubbleView; onclose: () => void } = $props();

  // A name and a document. There is no template mode any more (docs/decisions/0006):
  // the form asked for Context / Outcome / Symptom / Repro / Scope because the server
  // read them, and the server does not read anything now.
  let name = $state('');
  let body = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  const ready = $derived(name.trim() !== '');

  async function submit(e: Event) {
    e.preventDefault();
    if (!ready) return;
    busy = true;
    err = null;
    try {
      await api.createThread({
        instance: bubble.instance,
        bubble_id: bubble.id,
        name: name.trim(),
        body: body.trim(),
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

  <label for="bf-body">{t('form.document')} <span class="req">{t('form.documentHint')}</span></label>
  <textarea
    id="bf-body"
    bind:value={body}
    rows="14"
    placeholder={t('form.documentPlaceholder')}
    disabled={busy}></textarea>

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
