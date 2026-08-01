<script lang="ts">
  import { store } from '../lib/store.svelte';
  import { api, ApiError } from '../lib/api';
  import { fuzzyFilter } from '../lib/fuzzy';
  import type { BubbleView, ThreadHit } from '../lib/types';

  let {
    open = $bindable(false),
    onbirth,
  }: { open?: boolean; onbirth: (b: BubbleView) => void } = $props();

  type Cmd = {
    id: string;
    label: string;
    hint?: string;
    needsBubble?: boolean;
    needsInput?: string; // placeholder → run(bubble, value)
    run: (b?: BubbleView, value?: string) => Promise<void> | void;
  };

  let query = $state('');
  let sel = $state(0);
  let threads = $state<ThreadHit[]>([]);
  let searching = $state(false);
  let inputEl = $state<HTMLInputElement | null>(null);
  let resultsEl = $state<HTMLDivElement | null>(null);

  // two-stage state for commands that target a bubble / need text
  let stage = $state<'root' | 'pick' | 'input'>('root');
  let pending = $state<Cmd | null>(null);
  let pendingBubble = $state<BubbleView | null>(null);
  let argValue = $state('');

  const isCommand = $derived(query.startsWith('>'));
  const cmdQuery = $derived(isCommand ? query.slice(1).trim() : '');

  const COMMANDS: Cmd[] = [
    { id: 'refresh', label: 'refresh', hint: 're-poll the server', run: () => store.refresh() },
    {
      id: 'birth',
      label: 'birth thread…',
      hint: 'enforces Brief + Definition of Done',
      needsBubble: true,
      run: (b) => b && onbirth(b),
    },
    {
      id: 'review',
      label: 'mark reviewed…',
      hint: '→ 👀',
      needsBubble: true,
      run: (b) => b && api.review(b.id).then(() => store.refresh()),
    },
    {
      id: 'unreview',
      label: 'un-review…',
      needsBubble: true,
      run: (b) => b && api.unreview(b.id).then(() => store.refresh()),
    },
    {
      id: 'close',
      label: 'close bubble (done)…',
      hint: '→ 🏆',
      needsBubble: true,
      run: (b) => b && api.close(b.id).then(() => store.refresh()),
    },
    {
      id: 'owner',
      label: 'set owner…',
      needsBubble: true,
      needsInput: 'owner name / email',
      run: (b, v) => b && v !== undefined && api.contract(b.id, { owner: v }).then(() => store.refresh()),
    },
    {
      id: 'outcome',
      label: 'set outcome…',
      needsBubble: true,
      needsInput: 'the intended outcome',
      run: (b, v) => b && v !== undefined && api.contract(b.id, { outcome: v }).then(() => store.refresh()),
    },
    {
      id: 'signout',
      label: 'sign out',
      run: () => store.signOut(),
    },
  ];

  // instance switch commands (one per instance + "all")
  const instanceCmds = $derived<Cmd[]>([
    {
      id: 'inst:all',
      label: 'view all instances',
      hint: store.instance === '' ? 'active' : '',
      run: () => void (store.instance = ''),
    },
    ...store.instances.map((slug) => ({
      id: `inst:${slug}`,
      label: `switch to ${slug}`,
      hint: store.instance === slug ? 'active' : '',
      run: () => void (store.instance = slug),
    })),
  ]);

  // service-admin (godmode) commands — only present when the caller is elevated.
  const adminCmds = $derived<Cmd[]>(
    store.godmode
      ? [
          {
            id: 'god:allorgs',
            label: store.allOrgs ? 'godmode: my orgs only' : 'godmode: all orgs',
            hint: 'cross-org bubble view',
            run: () => {
              store.allOrgs = !store.allOrgs;
              return store.refresh();
            },
          },
          {
            id: 'god:stats',
            label: 'godmode: stats',
            run: () =>
              api.adminStats().then((s) => {
                store.flash = `godmode · ${s.instances} instances · ${s.cached_instances} cached · ${s.cached_identities} ids · rev ${s.revision.slice(0, 7)}`;
              }),
          },
          {
            id: 'god:instances',
            label: 'godmode: instances',
            run: () =>
              api.adminInstances().then((list) => {
                store.flash =
                  'instances: ' +
                  list.map((i) => i.slug + (i.cached ? '·cached' : '')).join(', ');
              }),
          },
          {
            id: 'god:refresh',
            label: 'godmode: refresh caches',
            run: () =>
              api
                .adminRefresh()
                .then(() => store.refresh())
                .then(() => {
                  store.flash = 'caches refreshed';
                }),
          },
          {
            id: 'god:tick',
            label: 'godmode: tick now',
            run: () =>
              api.adminTick().then(() => {
                store.flash = 'cooling sweep triggered';
              }),
          },
        ]
      : [],
  );

  const allCommands = $derived([...COMMANDS, ...adminCmds, ...instanceCmds]);
  const filteredCmds = $derived(fuzzyFilter(cmdQuery, allCommands, (c) => c.label));
  const filteredBubbles = $derived(fuzzyFilter(query, store.visible, (b) => b.name).slice(0, 8));
  const pickBubbles = $derived(fuzzyFilter(argValue, store.visible, (b) => b.name).slice(0, 8));

  // debounced thread search
  let debounce: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const q = query;
    if (isCommand || stage !== 'root' || q.trim().length < 2) {
      threads = [];
      return;
    }
    searching = true;
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(async () => {
      try {
        threads = await api.threads(q.trim());
      } catch {
        threads = [];
      } finally {
        searching = false;
      }
    }, 180);
  });

  // reset selection whenever the visible list changes shape
  $effect(() => {
    void [query, stage, threads.length];
    sel = 0;
  });

  $effect(() => {
    if (open && inputEl) inputEl.focus();
  });

  // keep the highlighted row visible as you arrow through the list
  $effect(() => {
    void sel;
    const el = resultsEl?.querySelector('.row.active');
    if (el) (el as HTMLElement).scrollIntoView({ block: 'nearest' });
  });

  type Row =
    | { kind: 'thread'; t: ThreadHit }
    | { kind: 'bubble'; b: BubbleView }
    | { kind: 'cmd'; c: Cmd };

  const rows = $derived.by<Row[]>(() => {
    if (stage === 'pick') return pickBubbles.map((b) => ({ kind: 'bubble', b }) as Row);
    if (stage === 'input') return [];
    if (isCommand) return filteredCmds.map((c) => ({ kind: 'cmd', c }) as Row);
    return [
      ...threads.map((t) => ({ kind: 'thread', t }) as Row),
      ...filteredBubbles.map((b) => ({ kind: 'bubble', b }) as Row),
    ];
  });

  function reset() {
    query = '';
    argValue = '';
    stage = 'root';
    pending = null;
    pendingBubble = null;
    threads = [];
    sel = 0;
  }

  function close() {
    open = false;
    reset();
  }

  async function runCmd(c: Cmd) {
    if (c.needsBubble) {
      pending = c;
      stage = 'pick';
      argValue = '';
      sel = 0;
      return;
    }
    await Promise.resolve(c.run());
    close();
  }

  async function pickBubble(b: BubbleView) {
    pendingBubble = b;
    if (pending?.needsInput) {
      stage = 'input';
      argValue = '';
      return;
    }
    await Promise.resolve(pending?.run(b));
    // birth keeps its own modal open; everything else closes
    close();
  }

  async function submitInput() {
    if (!pending || !pendingBubble) return;
    try {
      await Promise.resolve(pending.run(pendingBubble, argValue));
    } catch (e) {
      store.error = e instanceof ApiError ? e.message : String(e);
    }
    close();
  }

  function activate(row: Row) {
    if (row.kind === 'cmd') return runCmd(row.c);
    if (row.kind === 'bubble' && stage === 'pick') return pickBubble(row.b);
    // thread / bubble in search mode → open in Plane not yet wired; just close.
    close();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      if (stage !== 'root') {
        reset();
      } else {
        close();
      }
      return;
    }
    if (stage === 'input') {
      if (e.key === 'Enter') {
        e.preventDefault();
        void submitInput();
      }
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      sel = Math.min(sel + 1, Math.max(0, rows.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      sel = Math.max(sel - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const row = rows[sel];
      if (row) void activate(row);
    }
  }
</script>

{#if open}
  <div
    class="scrim"
    role="button"
    tabindex="-1"
    onclick={close}
    onkeydown={(e) => e.key === 'Escape' && close()}
  ></div>
  <div class="palette" role="dialog" aria-modal="true">
    <div class="input">
      <span class="glyph">{isCommand || stage !== 'root' ? '›' : '⌘K'}</span>
      {#if stage === 'pick'}
        <input
          bind:this={inputEl}
          bind:value={argValue}
          onkeydown={onKey}
          placeholder="pick a bubble for “{pending?.label.replace('…', '')}”…"
        />
      {:else if stage === 'input'}
        <input
          bind:this={inputEl}
          bind:value={argValue}
          onkeydown={onKey}
          placeholder={pending?.needsInput}
        />
      {:else}
        <input
          bind:this={inputEl}
          bind:value={query}
          onkeydown={onKey}
          placeholder="search threads · type > for commands"
        />
      {/if}
    </div>

    <div class="results" bind:this={resultsEl}>
      {#if stage === 'input'}
        <div class="ctx">
          {pending?.label.replace('…', '')} → <b>{pendingBubble?.name}</b> · ⏎ to apply
        </div>
      {:else if rows.length === 0}
        <div class="empty">
          {#if searching}searching…{:else}no matches{/if}
        </div>
      {:else}
        {#each rows as row, i (row.kind + ':' + (row.kind === 'cmd' ? row.c.id : row.kind === 'thread' ? row.t.id : row.b.id))}
          <button
            class="row"
            class:active={i === sel}
            onmouseenter={() => (sel = i)}
            onclick={() => activate(row)}
          >
            {#if row.kind === 'thread'}
              <span class="lead">›</span>
              <span class="main">{row.t.name}</span>
              <span class="tail">{row.t.bubble_name} · {row.t.instance} · {row.t.open ? '🔥 open' : '🏆 done'}</span>
            {:else if row.kind === 'bubble'}
              <span class="lead">◯</span>
              <span class="main">{row.b.name}</span>
              <span class="tail">{row.b.instance} · {row.b.level.replace('_', ' ')}</span>
            {:else}
              <span class="lead">›</span>
              <span class="main">{row.c.label}</span>
              {#if row.c.hint}<span class="tail">{row.c.hint}</span>{/if}
            {/if}
          </button>
        {/each}
      {/if}
    </div>

    <div class="foot">
      {#if stage === 'pick'}
        <span>↑↓ move · ⏎ pick bubble · esc back</span>
      {:else if stage === 'input'}
        <span>⏎ apply · esc back</span>
      {:else}
        <span>↑↓ move · ⏎ open · esc close · type <b>&gt;</b> for commands</span>
      {/if}
    </div>
  </div>
{/if}

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 40;
    background: oklch(0.08 0.02 265 / 0.55);
    backdrop-filter: blur(3px);
    border: none;
  }
  .palette {
    position: fixed;
    z-index: 50;
    top: 14vh;
    left: 50%;
    transform: translateX(-50%);
    width: min(94vw, 620px);
    border-radius: 18px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 30px 80px var(--shadow-strong);
  }
  /* the input is its own inset field (margin, not padding) so its focus glow is
     fully rounded and never clipped by the palette's overflow. */
  .input {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin: 0.7rem;
    padding: 0.8rem 0.95rem;
    border-radius: 12px;
    background: color-mix(in oklab, var(--text) 5%, transparent);
    border: 1px solid var(--line);
    transition:
      border-color 0.15s ease,
      box-shadow 0.15s ease;
  }
  .input:focus-within {
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--wip) 20%, transparent);
  }
  .glyph {
    font-size: 0.8rem;
    color: var(--faint);
    font-weight: 700;
    min-width: 1.7rem;
    padding-left: 0.15rem;
  }
  .input input {
    flex: 1;
    min-width: 0;
    background: transparent;
    border: none;
    outline: none;
    box-shadow: none;
    color: var(--text);
    font-size: 1rem;
  }
  /* defeat any framework focus ring on the bare input (the field glows instead) */
  .input input:focus,
  .input input:focus-visible {
    outline: none;
    box-shadow: none;
    border: none;
  }
  .results {
    max-height: 46vh;
    overflow-y: auto;
    padding: 0.5rem;
    scroll-padding-block: 0.5rem;
    border-top: 1px solid var(--line);
  }
  .row {
    width: 100%;
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    padding: 0.55rem 0.7rem;
    border-radius: 10px;
    border: none;
    background: transparent;
    color: var(--text);
    cursor: pointer;
    text-align: left;
  }
  .row.active {
    background: var(--hover);
  }
  .lead {
    color: var(--faint);
    font-size: 0.85rem;
  }
  .main {
    font-size: 0.9rem;
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tail {
    font-size: 0.72rem;
    color: var(--faint);
    white-space: nowrap;
  }
  .empty,
  .ctx {
    padding: 1rem;
    color: var(--faint);
    font-size: 0.85rem;
  }
  .ctx b {
    color: var(--text);
  }
  .foot {
    padding: 0.5rem 1rem;
    border-top: 1px solid var(--line);
    font-size: 0.72rem;
    color: var(--faint);
  }
  .foot b {
    color: var(--muted);
  }
</style>
