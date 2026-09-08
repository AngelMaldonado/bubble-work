<script lang="ts" module>
  export type Hit = {
    id: string;
    kind: string;
    icon: string;
    title: string;
    hint?: string;
    /** where this row goes in the list. Rows keep the caller's order inside it. */
    group?: string;
  };

  /** A verb typed rather than clicked. Reached with `/`. */
  export type Command = {
    id: string;
    icon: string;
    title: string;
    hint?: string;
    run: () => void;
  };
</script>

<script lang="ts">
  // One field that finds a thread, a bubble or a page — and, when something is
  // waiting on a pick, hands it back instead of navigating.
  //
  // It is the same field in both cases on purpose: "relate this thread to
  // another" and "take me to that thread" are the same act of finding, and
  // giving each its own control is how a picker ends up nailed to the bottom of
  // a sidebar answering the second half of the question first.
  //
  // Three things share it, in this order, because that is the order of how sure
  // the answer is:
  //   · what the browser already holds — threads and bubbles, matched fuzzily,
  //     with no round trip, so the list moves with the keys;
  //   · what only the server can answer — the text INSIDE the documents, asked
  //     for after a pause, because a request per keystroke is a request per
  //     keystroke;
  //   · commands, which are not found but named, and so live behind `/`.
  import { Dialog, Portal } from '@skeletonlabs/skeleton-svelte';
  import SearchIcon from '@lucide/svelte/icons/search';
  import { pieces, rank, type Range } from '../lib/fuzzy';

  let {
    open = $bindable(false),
    items = [],
    /** hits from the server's search over the document text */
    found = [],
    /** the verbs behind `/` */
    commands = [],
    /** the server is still answering the query being typed */
    searching = false,
    /** shown in place of the placeholder while something is waiting on a pick */
    prompt = '',
    /** ask the server. Called after a pause, never per keystroke. */
    onquery,
    onpick,
  }: {
    open?: boolean;
    items?: Hit[];
    found?: Hit[];
    commands?: Command[];
    searching?: boolean;
    prompt?: string;
    onquery?: (q: string) => void;
    onpick?: (hit: Hit) => void;
  } = $props();

  let q = $state('');
  let cursor = $state(0);

  /** `/` at the start is the mode switch, and the rest is the command's name. */
  const slash = $derived(q.startsWith('/'));
  const term = $derived(slash ? q.slice(1) : q.trim());

  type Row =
    | { kind: 'head'; key: string; label: string }
    | { kind: 'hit'; key: string; hit: Hit; ranges: Range[] }
    | { kind: 'cmd'; key: string; cmd: Command; ranges: Range[] };

  const rows = $derived.by((): Row[] => {
    const out: Row[] = [];
    if (slash) {
      for (const c of rank(term, commands, (c) => c.title)) {
        out.push({ kind: 'cmd', key: 'c:' + c.id, cmd: c, ranges: c.match.ranges });
      }
      return out;
    }

    // Grouped, with the caller's order preserved inside each group: threads
    // before bubbles is a real signal, and a sort that flattens it looks random.
    const ranked = term ? rank(term, items, (i) => i.title + ' ' + (i.hint ?? '')) : items.map((i) => ({ ...i, match: { score: 0, ranges: [] as Range[] } }));
    const groups = new Map<string, typeof ranked>();
    for (const r of ranked) {
      const g = r.group ?? '';
      let list = groups.get(g);
      if (!list) groups.set(g, (list = []));
      list.push(r);
    }
    for (const [label, list] of groups) {
      if (label) out.push({ kind: 'head', key: 'h:' + label, label });
      for (const r of list) out.push({ kind: 'hit', key: 'i:' + r.id, hit: r, ranges: r.match.ranges });
    }

    // The server's hits are already the answer to this exact query; ranking them
    // again against the same string only reorders somebody else's ordering.
    if (found.length) {
      out.push({ kind: 'head', key: 'h:texto', label: 'En el texto' });
      for (const f of found) out.push({ kind: 'hit', key: 'f:' + f.id, hit: f, ranges: [] });
    }
    return out;
  });

  /** Only these can be picked; headings are furniture. */
  const pickable = $derived(rows.filter((r) => r.kind !== 'head'));

  // The cursor is an index into a list that changes as you type; clamping it
  // here is what stops Enter from picking whatever happens to be at position 7.
  $effect(() => {
    if (cursor > pickable.length - 1) cursor = Math.max(0, pickable.length - 1);
  });
  $effect(() => {
    if (open) {
      q = '';
      cursor = 0;
    }
  });

  // The pause before asking the server. Long enough that typing a word is one
  // request and not six; short enough that stopping to think shows the answer.
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const ask = slash ? '' : term;
    clearTimeout(timer);
    if (!onquery) return;
    timer = setTimeout(() => onquery(ask), 180);
    return () => clearTimeout(timer);
  });

  function keys(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' || (e.key === 'n' && e.ctrlKey)) {
      e.preventDefault();
      cursor = Math.min(cursor + 1, pickable.length - 1);
    } else if (e.key === 'ArrowUp' || (e.key === 'p' && e.ctrlKey)) {
      e.preventDefault();
      cursor = Math.max(cursor - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      run(pickable[cursor]);
    }
  }

  function run(row?: Row) {
    if (!row || row.kind === 'head') return;
    open = false;
    if (row.kind === 'cmd') row.cmd.run();
    else onpick?.(row.hit);
  }

  /** The index a row has among the pickable ones — headings do not take a turn. */
  function index(row: Row): number {
    return pickable.indexOf(row as (typeof pickable)[number]);
  }
</script>

<Dialog {open} onOpenChange={(e: { open: boolean }) => (open = e.open)}>
  <Portal>
    <Dialog.Backdrop
      class="fixed inset-0 bg-surface-50-950/50"
      style="z-index: var(--z-drawer-scrim)" />
    <!-- Near the top, not centred: the list grows downward, and a box that
         grows from the middle of the screen moves the field you are typing in. -->
    <Dialog.Positioner
      class="fixed inset-0 flex items-start justify-center p-4 pt-[12vh]"
      style="z-index: var(--z-drawer)">
      <Dialog.Content class="omni card bg-surface-100-900 w-full max-w-xl p-0 shadow-xl">
        <label class="field">
          <SearchIcon class="size-4 shrink-0" />
          <!-- svelte-ignore a11y_autofocus -->
          <input
            bind:value={q}
            onkeydown={keys}
            placeholder={prompt || 'Buscar, o «/» para un comando…'}
            {@attach (el: HTMLInputElement) => el.focus()} />
          {#if searching}<span class="wait" aria-label="buscando">…</span>{/if}
          <kbd>esc</kbd>
        </label>

        <ul class="hits">
          {#each rows as r (r.key)}
            {#if r.kind === 'head'}
              <li class="head">{r.label}</li>
            {:else if r.kind === 'cmd'}
              {@const i = index(r)}
              <li>
                <button class="hit" class:on={i === cursor} onmouseenter={() => (cursor = i)} onclick={() => run(r)}>
                  <span class="ico" aria-hidden="true">{r.cmd.icon}</span>
                  <span class="t">
                    {#each pieces(r.cmd.title, r.ranges) as p}<span class:hi={p.on}>{p.text}</span>{/each}
                  </span>
                  {#if r.cmd.hint}<span class="hint">{r.cmd.hint}</span>{/if}
                  <span class="kind">comando</span>
                </button>
              </li>
            {:else}
              {@const i = index(r)}
              <li>
                <button class="hit" class:on={i === cursor} onmouseenter={() => (cursor = i)} onclick={() => run(r)}>
                  <span class="ico" aria-hidden="true">{r.hit.icon}</span>
                  <span class="t">
                    {#each pieces(r.hit.title, r.ranges) as p}<span class:hi={p.on}>{p.text}</span>{/each}
                  </span>
                  {#if r.hit.hint}<span class="hint">{r.hit.hint}</span>{/if}
                  <span class="kind">{r.hit.kind}</span>
                </button>
              </li>
            {/if}
          {/each}
          {#if !pickable.length}
            <li class="none">
              {#if slash}Ningún comando con «{term}»
              {:else if searching}Buscando «{term}»…
              {:else}Nada con «{term}»{/if}
            </li>
          {/if}
        </ul>
      </Dialog.Content>
    </Dialog.Positioner>
  </Portal>
</Dialog>

<style>
  /* Skeleton ships no CSS for Dialog; the surface comes from its own utilities
     (`card bg-surface-100-900 shadow-xl`) and only the two things it cannot
     know are ours: the list must be able to scroll inside the panel, and the
     panel must clip its own rounded corners. */
  :global(.omni) {
    display: flex;
    flex-direction: column;
    max-height: 70vh;
    overflow: hidden;
  }

  .field {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.85rem 1rem;
    border-bottom: 1px solid var(--line);
    color: var(--faint);
  }
  .field input {
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 1rem;
    outline: none;
  }
  .wait { flex: none; font-size: 0.8rem; }
  kbd {
    flex: none;
    padding: 0 0.35rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    font-size: 0.7rem;
  }

  .hits {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    margin: 0;
    padding: 0.35rem;
    list-style: none;
  }
  .head {
    padding: 0.55rem 0.6rem 0.25rem;
    color: var(--faint);
    font-size: 0.68rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .hit {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    padding: 0.5rem 0.6rem;
    border: none;
    border-radius: 9px;
    background: transparent;
    color: var(--muted);
    text-align: left;
  }
  /* One highlight, driven by the cursor — the pointer moves it rather than
     drawing a second one, so the keyboard and the mouse never disagree about
     what Enter will open. */
  .hit.on { background: var(--hover); color: var(--text); }
  .ico { flex: none; }
  .t { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  /* The characters that earned the row its place. Weight and colour, never a
     background: a highlight that boxes every other letter is unreadable. */
  .hi { color: var(--text); font-weight: 650; }
  .hint, .kind { flex: none; font-size: 0.72rem; color: var(--faint); }
  .kind {
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--hover);
  }
  .none { padding: 1rem; color: var(--faint); font-size: 0.85rem; }
</style>
