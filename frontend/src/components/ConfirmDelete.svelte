<script lang="ts">
  // The one guard on an irreversible act (docs/ARTIFACT-EDITING.md).
  //
  // Deleting removes things from Plane, which is the system of record, so this
  // says exactly what goes and what stays rather than asking "are you sure?".
  // For a bubble it also says how many threads it will unbubble, because that
  // is the consequence people do not expect.
  import { t } from '../lib/i18n.svelte';

  let {
    what,
    detail,
    prefer,
    busy = false,
    oncancel,
    onconfirm,
  }: {
    /** The thing's own name, shown in the heading. */
    what: string;
    /** What this delete actually costs, in one sentence. */
    detail: string;
    /** The gentler alternative, when there is one. */
    prefer?: string;
    busy?: boolean;
    oncancel: () => void;
    onconfirm: () => void;
  } = $props();

  let box = $state<HTMLElement | null>(null);

  // Focus lands on Cancel, never on Delete: an errant Enter must not destroy
  // anything.
  $effect(() => {
    box?.querySelector<HTMLButtonElement>('.cancel')?.focus();
  });

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      e.preventDefault();
      oncancel();
    }
  }
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="scrim" onclick={oncancel}>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="box"
    bind:this={box}
    role="alertdialog"
    aria-modal="true"
    tabindex="-1"
    onclick={(e) => e.stopPropagation()}
  >
    <h2>{t('del.title', { what })}</h2>
    <p class="warn">{t('del.irreversible')}</p>
    <p class="what">{detail}</p>
    {#if prefer}<p class="prefer">{prefer}</p>{/if}
    <div class="acts">
      <button class="cancel" onclick={oncancel} disabled={busy}>{t('del.cancel')}</button>
      <button class="go" onclick={onconfirm} disabled={busy}>
        {busy ? t('del.deleting') : t('del.confirm')}
      </button>
    </div>
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    background: oklch(0.12 0.02 265 / 0.55);
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
  }
  .box {
    width: min(92vw, 27rem);
    display: grid;
    gap: 0.55rem;
    padding: 1.25rem 1.35rem;
    border-radius: 18px;
    background: var(--surface-solid);
    border: 1px solid color-mix(in oklab, oklch(0.68 0.19 25) 35%, var(--line));
    box-shadow: 0 30px 70px var(--shadow-strong);
    font-family: var(--sans);
  }
  h2 {
    margin: 0;
    font-size: 1rem;
    font-weight: 800;
    color: var(--text);
  }
  p {
    margin: 0;
    font-size: 0.84rem;
    line-height: 1.5;
  }
  .warn {
    color: oklch(0.72 0.19 25);
    font-weight: 700;
  }
  .what {
    color: var(--text);
  }
  .prefer {
    color: var(--muted);
    font-size: 0.78rem;
  }
  .acts {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 0.4rem;
  }
  .acts button {
    padding: 0.45rem 1rem;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 5%, transparent);
    color: var(--text);
    font-family: var(--sans);
    font-size: 0.82rem;
    font-weight: 700;
    cursor: pointer;
  }
  .acts .cancel:focus-visible {
    outline: 2px solid var(--wip);
    outline-offset: 2px;
  }
  .acts .go {
    color: oklch(0.98 0 0);
    background: oklch(0.55 0.2 25);
    border-color: transparent;
  }
  .acts .go:hover:not(:disabled) {
    background: oklch(0.6 0.22 25);
  }
  .acts button:disabled {
    opacity: 0.55;
    cursor: default;
  }
</style>
