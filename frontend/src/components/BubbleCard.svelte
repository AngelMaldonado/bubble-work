<script lang="ts">
  import { bandFace, bandName } from '../lib/bands';
  import type { BubbleHeat, ThreadHeat } from '../lib/api';

  let {
    bubble,
    threads,
    onOpen,
  }: {
    bubble: BubbleHeat;
    threads: ThreadHeat[];
    onOpen: (t: ThreadHeat) => void;
  } = $props();

  const open = $derived(threads.filter((t) => t.heat.lifecycle !== 'closed'));
</script>

<article class="card glass band-{bubble.heat.lifecycle} p-4" class:sunk={bubble.heat.lifecycle === 'dormant' || bubble.closed}>
  <header class="flex items-start gap-2">
    <div class="min-w-0 flex-1">
      <h3 class="truncate text-base font-semibold">{bubble.name}</h3>
      {#if bubble.outcome}
        <p class="faint mt-0.5 line-clamp-2 text-sm">{bubble.outcome}</p>
      {/if}
    </div>
    <span class="text-lg" title={bubble.heat.reason}>{bandFace(bubble.heat.lifecycle)}</span>
  </header>

  <!-- The reason, not just the verdict: a band nobody can explain is a band
       nobody trusts. -->
  <p class="faint mt-2 text-xs">{bubble.heat.reason}</p>

  {#if open.length}
    <ul class="mt-3 space-y-1">
      {#each open as t (t.id)}
        <li>
          <button
            class="band-{t.heat.lifecycle} flex w-full items-center gap-2 rounded-lg px-2 py-1 text-left text-sm hover:[background:var(--hover)]"
            class:sunk={t.heat.lifecycle === 'dormant'}
            onclick={() => onOpen(t)}>
            <span class="dot"></span>
            <span class="faint tabular-nums">#{t.seq}</span>
            <span class="min-w-0 flex-1 truncate">{t.name}</span>
            {#if t.priority}
              <span class="faint text-xs">{t.priority}</span>
            {/if}
            {#if t.pulse}<span class="faint text-xs" title="somebody commented recently">💬</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="faint mt-3 text-xs">sin threads abiertos</p>
  {/if}
</article>
