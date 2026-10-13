<script lang="ts">
  import Loading from './Loading.svelte';
  import { tick, untrack } from 'svelte';
  import Icon from './Icon.svelte';
  import { rowSquare } from '../lib/rowSquare';
  import { threadPins } from '../lib/threadPins.svelte';
  import { threadUrl } from '../lib/routes';
  let { thread, left = false, onopen }: {
    thread?: { id: string; name: string };
    left?: boolean;
    onopen?: (url: string) => void;
  } = $props();
  let open = $state(false);
  let root = $state<HTMLElement>();
  let shift = $state(0);
  function fitPopup() {
    const box = root?.querySelector('.panel')?.getBoundingClientRect();
    if (!box) return;
    const base = box.left - shift;
    shift = Math.max(16, Math.min(base, window.innerWidth - box.width - 16)) - base;
  }
  $effect(() => { if (open) void tick().then(fitPopup); });
  const pinned = $derived(!!thread && threadPins.ids.includes(thread.id));
  $effect(() => {
    void thread?.id;
    untrack(() => threadPins.load());
  });
  function navigate(e: MouseEvent, url: string) {
    if (onopen && !e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey && e.button === 0) {
      e.preventDefault(); open = false; onopen(url);
    }
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') open = false; }} onresize={() => { if (open) fitPopup(); }} />
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="pins" class:left bind:this={root}
  onmouseenter={() => (open = true)} onmouseleave={() => (open = false)}
  onfocusin={() => (open = true)}
  onfocusout={(e) => { if (!root?.contains(e.relatedTarget as Node)) open = false; }}>
  <button class="pin-button" class:on={pinned} aria-pressed={thread ? pinned : undefined}
    aria-label={thread ? pinned ? 'Quitar pin del hilo' : 'Fijar hilo' : 'Hilos fijados'}
    title={thread ? pinned ? 'Quitar pin del hilo' : 'Fijar este hilo' : 'Hilos fijados'}
    aria-expanded={open} disabled={threadPins.saving || (!!thread && threadPins.loading)}
    onclick={() => thread && threadPins.loaded ? threadPins.toggle(thread.id) : (open = true)}>
    <Icon of="pin" size="52px" tint={threadPins.items.length > 0} clay={!threadPins.items.length} />
    {#if threadPins.items.length}<span class="pin-count">{threadPins.items.length}</span>{/if}
  </button>
  {#if open}
    <div class="bridge" style:transform={`translateX(${shift}px)`}>
      <section class="panel" aria-label="Hilos fijados">
        <h3><Icon of="pin" tint /> Hilos fijados</h3>
        {#if threadPins.error}
          <p class="error" role="alert">{threadPins.error}</p>
          <button class="link retry" disabled={threadPins.loading || threadPins.saving} onclick={() => threadPins.load()}>Reintentar</button>
        {/if}
        {#if threadPins.loading}<Loading size={32} label="Cargando fijados" />
        {:else}
          <ul>
            {#each threadPins.items as t (t.id)}
              <li>
                <a href={threadUrl(t.slug, t.seq)} onclick={(e) => navigate(e, threadUrl(t.slug, t.seq))}>
                  <span class="name">{t.name}</span><span class="meta">{t.project} · #{t.seq}</span>
                </a>
                <button class="remove" {@attach (el) => rowSquare(el, 'li')} disabled={threadPins.saving} aria-label="Desfijar {t.name}" onclick={() => threadPins.toggle(t.id)}>×</button>
              </li>
            {:else}
              <li class="empty">Fija un hilo con el pin junto al cerebro para volver a él desde aquí.</li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .pins { position: relative; flex: none; display: flex; align-items: center; }
  .pin-button { position: relative; display: grid; place-items: center; width: 56px; height: 56px; padding: 0; border: none; border-radius: 14px; background: transparent; color: var(--text); cursor: pointer; }
  .pin-button :global(.icon) { filter: drop-shadow(0 8px 12px rgb(11 18 32 / 0.22)); transition: transform 0.18s ease; }
  .pin-button:hover :global(.icon) { transform: scale(1.1) translateY(-2px); }
  .pin-button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .pin-button:disabled { cursor: wait; }
  .pin-count { position: absolute; right: -3px; top: -3px; min-width: 18px; height: 18px; padding: 0 3px; box-sizing: border-box; border-radius: 9999px; display: grid; place-items: center; background: var(--accent); color: white; font-size: 0.65rem; font-weight: 750; line-height: 1; }
  .bridge { position: absolute; right: 0; bottom: 100%; z-index: var(--z-float); padding-bottom: 0.6rem; }
  .left .bridge { right: auto; left: 0; }
  .panel { width: 21rem; max-width: calc(100vw - 2rem); padding: 0.7rem; border: 1px solid var(--line); border-radius: 14px; background: var(--surface-solid); color: var(--text); box-shadow: 0 14px 40px rgb(0 0 0 / 0.23); }
  h3 { display: flex; align-items: center; gap: 0.4rem; margin: 0 0 0.55rem; font-size: 0.85rem; font-weight: 750; }
  ul { max-height: min(55vh, 24rem); overflow-y: auto; margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: center; gap: 0.4rem; }
  a { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.2rem; padding: 0.5rem; border-radius: 8px; color: inherit; }
  a:hover { background: var(--hover); }
  .name { font-size: 0.85rem; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: 0.72rem; color: var(--muted); }
  .remove { flex: none; width: 40px; height: 40px; aspect-ratio: 1; border: none; border-radius: 8px; background: none; color: var(--faint); cursor: pointer; }
  .remove:hover { background: var(--hover); color: var(--text); }
  .empty, .error { margin: 0; padding: 0.4rem; font-size: 0.8rem; color: var(--muted); line-height: 1.45; }
  .error { color: var(--color-error-500); }
  @media (prefers-reduced-motion: reduce) { .pin-button :global(.icon) { transition: none; transform: none; } }
  .retry { margin: 0.25rem 0.4rem 0.5rem; font-size: 0.8rem; }
</style>
