<script lang="ts">
  // El ánimo de la IA respecto a TU trabajo, junto al cerebro: la cabeza del
  // robot en el nivel más grave de lo tuyo (hilos asignados a ti y burbujas de
  // las que respondes). Es lo mismo que enseñará el semáforo de tu escritorio.
  //
  // Al pasar por encima se abre hacia arriba, como el cerebro: lo tuyo que está
  // en problemas, con sus razones; clic abre el hilo o el tablero. Con la furia
  // apagada en el departamento no se dibuja.
  //
  // Se recalcula cuando cambia un hilo o una burbuja (el stream de PocketBase) y
  // cada minuto, porque la furia también cambia sola con el tiempo: vencer.
  import { onDestroy } from 'svelte';
  import Icon from './Icon.svelte';
  import FuryFlames from './FuryFlames.svelte';
  import { api, type FuryLevel, type FuryPreview } from '../lib/api';
  import { furyIcon } from '../lib/icons';
  import { live } from '../lib/live.svelte';

  let { onopen }: { onopen: (it: FuryPreview['items'][number]) => void } = $props();

  let on = $state(false);
  let level = $state<FuryLevel | null>(null);
  let items = $state<FuryPreview['items']>([]);
  let open = $state(false);
  let stopped = false;
  let gen = 0;

  async function load() {
    if (stopped) return;
    const turn = ++gen;
    try {
      const r = await api.myFury();
      if (stopped || turn !== gen) return;
      on = r.on;
      level = r.level;
      items = r.items;
    } catch {
      // Sin red: se queda como estaba.
    }
  }
  load();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const soon = () => { clearTimeout(timer); timer = setTimeout(load, 400); };
  const offs = ['threads', 'bubbles', 'features', 'fury_rules'].map((c) => live.watch(c, soon));
  const tick = setInterval(load, 60_000);
  onDestroy(() => { stopped = true; clearTimeout(timer); clearInterval(tick); offs.forEach((off) => off()); });

  const LIGHT = ['luz verde', 'luz amarilla', 'luz roja', 'luz roja parpadeando'];
  const lvl = $derived(level?.level ?? 0);
  const say = $derived(
    `IA ${level?.name ?? 'Zen'} · ${LIGHT[lvl]}` + (items.length ? ` · ${items.length} en problemas` : ' · nada tuyo en problemas'),
  );
</script>

{#if on}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="orb" class:open onmouseenter={() => (open = true)} onmouseleave={() => (open = false)}
    onfocusin={() => (open = true)} onfocusout={(e) => { if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) open = false; }}>
    <div class="fan" aria-label="Lo tuyo con furia">
      <div class="head-row" class:fury-fire={lvl === 3}>
        {#if lvl === 3}<FuryFlames from={10} to={92} place="inside" size={26} opacity={0.5} />{/if}
        <b>IA {level?.name ?? 'Zen'}</b><span class="faint">· {LIGHT[lvl]}</span>
      </div>
      {#each items.slice(0, 8) as it (it.kind + it.id)}
        <button class="row" class:fury-fire={it.fury.level === 3} onclick={() => { open = false; onopen(it); }} title={it.name}>
          {#if it.fury.level === 3}<FuryFlames from={20} to={94} place="inside" size={26} opacity={0.5} />{/if}
          <Icon of={furyIcon(it.fury.level)} tint size="1.6rem" />
          <span class="what">
            <span class="title">{it.name}</span>
            <span class="why">{it.kind === 'bubble' ? 'burbuja' : `#${it.seq}`} · {it.fury.reasons[0]?.text ?? ''}</span>
          </span>
        </button>
      {:else}
        <p class="empty">Nada tuyo en problemas. La IA está tranquila.</p>
      {/each}
      {#if items.length > 8}<p class="empty">y {items.length - 8} más</p>{/if}
    </div>

    <button class="box lv{lvl}" aria-expanded={open} aria-label={say} title={say} onclick={() => (open = !open)}>
      {#if lvl === 3}<FuryFlames from={8} to={92} place="around" size={20} />{/if}
      <span class="head"><Icon of={furyIcon(lvl)} tint size="44px" /></span>
      {#if items.length}<span class="count lv{lvl}">{items.length}</span>{/if}
    </button>
  </div>
{/if}

<style>
  .orb { position: relative; display: flex; flex-direction: column; align-items: flex-start; }
  .fan {
    position: absolute; left: 0; bottom: 100%;
    display: flex; flex-direction: column-reverse; align-items: flex-start; gap: 0.35rem;
    padding: 0.35rem 3rem 0.5rem 0;
    opacity: 0; pointer-events: none; transform: translateY(6px);
    transition: opacity 0.16s ease, transform 0.16s cubic-bezier(0.2, 0.8, 0.3, 1);
  }
  .orb.open .fan { opacity: 1; pointer-events: auto; transform: none; }
  .head-row, .row, .empty {
    display: flex; align-items: center; gap: 0.5rem;
    max-width: min(24rem, calc(100dvw - 8rem));
    padding: 0.35rem 0.75rem; border: 1px solid var(--line); border-radius: 999px;
    background: var(--surface-solid); box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    color: inherit; font-size: 0.8rem; white-space: nowrap;
  }
  .empty { white-space: normal; border-radius: 14px; color: var(--muted); margin: 0; }
  .row, .head-row { position: relative; overflow: hidden; }
  .row:global(.fury-fire) .why, .head-row:global(.fury-fire) .faint { color: rgb(255 255 255 / 0.85); }
  .row { padding: 0.25rem 0.85rem 0.25rem 0.4rem; text-align: left; cursor: pointer; transition: transform 0.15s ease, background 0.15s ease; }
  .row:hover { background: color-mix(in oklab, var(--accent) 12%, var(--surface-solid)); transform: translateX(2px); }
  .what { display: grid; min-width: 0; line-height: 1.2; }
  .title { overflow: hidden; text-overflow: ellipsis; font-weight: 600; }
  .why { overflow: hidden; text-overflow: ellipsis; color: var(--muted); font-size: 0.74rem; }

  .box {
    position: relative; display: grid; place-items: center;
    width: 56px; height: 56px; border: none; border-radius: 999px;
    background: transparent; cursor: pointer;
  }
  .head { display: inline-grid; filter: drop-shadow(0 8px 12px rgb(11 18 32 / 0.22)); transition: transform 0.18s cubic-bezier(0.2, 0.8, 0.3, 1); }
  .orb.open .head { transform: scale(1.1) translateY(-2px); }
  /* Desatada: llamas (FuryFlames) alrededor de la cabeza, sin moverla. */
  .head { position: relative; z-index: 1; }
  .box:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .count {
    position: absolute; z-index: 2; top: 0; right: 0;
    min-width: 18px; height: 18px; padding: 0 3px; box-sizing: border-box;
    display: grid; place-content: center; border-radius: 9999px;
    color: #fff; font-size: 0.66rem; font-weight: 700; line-height: 1;
  }
  .count.lv1 { background: #d97706; }
  .count.lv2, .count.lv3 { background: #dc2626; }
  @media (prefers-reduced-motion: reduce) {
    .fan, .head, .row { transition: opacity 0.16s ease; }
    .orb.open .head { transform: none; }
  }
</style>
