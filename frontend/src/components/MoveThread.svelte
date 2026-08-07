<script lang="ts">
  // Re-home a thread into another bubble.
  //
  // A bubble is a Plane module and membership is just a link, so this is a genuine
  // move: the work item, its Brief, its Logbook, its comments and its id are all
  // untouched. It is not evidence of production and warms nothing — filing work
  // somewhere else is not the same as doing any.
  import { t } from '../lib/i18n.svelte';
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import Combo from './Combo.svelte';

  let {
    threadId,
    title,
    onclose,
    onmoved,
  }: {
    /** namespaced "<instance>:<project>:<work item>" */
    threadId: string;
    title: string;
    onclose: () => void;
    onmoved: () => void;
  } = $props();

  // Plane groups work items only within their own project, so the destinations
  // are the bubbles of this thread's own workspace. Reading them off the id
  // rather than asking the server keeps this to one round trip.
  const [instance, project] = $derived(threadId.split(':'));

  const targets = $derived(
    store.bubbles
      .filter((b) => b.instance === instance && b.project === project)
      .map((b) => ({ value: b.id, label: b.name }))
      .sort((a, b) => a.label.localeCompare(b.label)),
  );

  let target = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  const ready = $derived(target !== '' && !busy);

  async function submit(e: Event): Promise<void> {
    e.preventDefault();
    if (!ready) return;
    busy = true;
    err = null;
    try {
      await api.moveThread(threadId, target);
      store.flash = t('move.done', {
        name: targets.find((x) => x.value === target)?.label ?? target,
      });
      await store.refresh();
      onmoved();
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
  <h2>{t('move.title', { what: title })}</h2>
  <p class="explain">{t('move.explain')}</p>

  {#if targets.length}
    <label for="mv-target">{t('move.pick')}</label>
    <Combo
      options={targets}
      bind:value={target}
      ariaLabel="destination bubble"
      placeholder={t('move.pick')}
    />
    <p class="fine">{t('move.sameWorkspace')}</p>
  {:else}
    <p class="fine">{t('move.none')}</p>
  {/if}

  {#if err}<p class="err">{err}</p>{/if}

  <div class="actions">
    <button type="button" class="ghost" onclick={onclose} disabled={busy}>{t('form.cancel')}</button>
    <button type="submit" class="go" disabled={!ready}>
      {busy ? t('move.moving') : t('move.go')}
    </button>
  </div>
</form>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 200;
    background: oklch(0.08 0.02 265 / 0.6);
    backdrop-filter: blur(4px);
    border: none;
  }
  .modal {
    position: fixed;
    z-index: 210;
    top: 16vh;
    left: 50%;
    transform: translateX(-50%);
    width: min(94vw, 440px);
    padding: 1.4rem 1.45rem;
    border-radius: 18px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 30px 80px var(--shadow-strong);
    display: grid;
    gap: 0.5rem;
    font-family: var(--sans);
  }
  h2 {
    margin: 0;
    font-size: 1.02rem;
    font-weight: 800;
    color: var(--text);
  }
  .explain {
    margin: 0;
    font-size: 0.82rem;
    line-height: 1.5;
    color: var(--muted);
  }
  label {
    margin-top: 0.35rem;
    font-size: 0.76rem;
    font-weight: 600;
    color: var(--muted);
  }
  .fine {
    margin: 0;
    font-size: 0.73rem;
    color: var(--faint);
    line-height: 1.45;
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
