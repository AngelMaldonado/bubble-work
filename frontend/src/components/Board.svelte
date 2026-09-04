<script lang="ts">
  import { bandFace, bandName, bandOrder } from '../lib/bands';
  import { api, type Board, type BubbleHeat, type ThreadHeat, type Workspace } from '../lib/api';
  import BubbleCard from './BubbleCard.svelte';
  import Minimap, { type MapItem } from './Minimap.svelte';

  let {
    workspace,
    onOpen,
  }: { workspace: Workspace; onOpen: (t: ThreadHeat) => void } = $props();

  let board = $state<Board | null>(null);
  let error = $state('');

  // Refetched rather than recomputed on the client: heat is a pure function of
  // evidence and TIME, and the server is the one holding both. A board that
  // ages in the browser would drift from the one everyone else sees.
  async function load() {
    try {
      board = await api.board(workspace.id);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => {
    workspace.id;
    load();
  });


  const byBand = $derived(() => {
    const out: Record<string, BubbleHeat[]> = {};
    for (const b of board?.bubbles ?? []) (out[b.heat.lifecycle] ??= []).push(b);
    return out;
  });
  const unfiled = $derived((board?.threads ?? []).filter((t) => !t.bubble && t.heat.lifecycle !== 'closed'));

  // The minimap reads the same list the board draws, so the two cannot disagree
  // about what is on the page.
  const mapItems = $derived<MapItem[]>(
    (board?.bubbles ?? []).map((b) => ({ id: b.id, name: b.name, life: b.heat.lifecycle })),
  );
</script>

<Minimap items={mapItems} />

{#if error}
  <p class="card glass p-4 text-sm text-error-500">{error}</p>
{:else if !board}
  <p class="faint p-4 text-sm">…</p>
{:else}
  <div class="space-y-8">
    {#each bandOrder as band}
      {@const items = byBand()[band] ?? []}
      {#if items.length}
        <section id="band-{band}">
          <h2 class="band-{band} display mb-3 flex items-center gap-2 text-sm tracking-widest uppercase">
            <span>{bandFace(band)}</span>{bandName(band)}
            <span class="faint font-normal normal-case">{items.length}</span>
          </h2>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {#each items as b (b.id)}
              <div id="bw-{b.id}">
                <BubbleCard
                  bubble={b}
                  threads={board.threads.filter((t) => t.bubble === b.id)}
                  {onOpen} />
              </div>
            {/each}
          </div>
        </section>
      {/if}
    {/each}

    {#if unfiled.length}
      <section>
        <h2 class="faint display mb-3 text-sm tracking-widest uppercase">Sin burbuja</h2>
        <ul class="card glass divide-y-[1px] divide-[var(--line)] p-2">
          {#each unfiled as t (t.id)}
            <li>
              <button
                class="band-{t.heat.lifecycle} flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:[background:var(--hover)]"
                onclick={() => onOpen(t)}>
                <span class="dot"></span>
                <span class="faint tabular-nums">#{t.seq}</span>
                <span class="min-w-0 flex-1 truncate">{t.name}</span>
                {#if t.priority}<span class="faint text-xs">{t.priority}</span>{/if}
              </button>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <!-- Say what it was measured against. A cold board and a short cycle look
         identical until somebody can see the calibration. -->
    <p class="faint text-xs">
      ciclo de {board.tuning.cycle_hours}h · dormido tras {board.tuning.dormant_cycles} ciclos ·
      calculado {new Date(board.at).toLocaleString()}
    </p>
  </div>
{/if}
