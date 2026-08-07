<script lang="ts">
  // Editing a thread's artifacts from the board (docs/ARTIFACT-EDITING.md).
  //
  // The editor is MARKDOWN SOURCE, not WYSIWYG, and that is a decision rather
  // than a shortcut: everything downstream already speaks markdown (the TOC, the
  // minimap, the todo parser, the logbook fingerprint), and Plane serves its
  // description images only to a web session — so a WYSIWYG here would be a
  // WYSIWYG that cannot show you the pictures.
  //
  // The surface underneath is CodeMirror 6, loaded on demand. A <textarea> was
  // the honest first cut and a poor one: no highlighting, no list continuation,
  // no undo grouping, and a hand-rolled hidden-div measurement to find the
  // caret. @codemirror/lang-markdown ships the two commands that make markdown
  // editing feel like markdown — Enter continues a list or checkbox, Backspace
  // unwinds the marker — and coordsAtPos retires the measurement hack.
  //
  // Saving is autosave, which forces two rules the server enforces too: a region
  // submitted unchanged writes nothing at all, and every write carries the hash
  // of what was read so a lost update becomes a 409 instead of a silent
  // overwrite.
  import { onDestroy, untrack } from 'svelte';
  import { api, ApiError } from '../lib/api';
  import type { MarkdownEditor } from '../lib/editor';
  import { t } from '../lib/i18n.svelte';
  import { findSlash } from '../lib/slash';
  import { theme } from '../lib/theme.svelte';
  import { vimPref } from '../lib/vim.svelte';
  import type { RegionName, ThreadDetail } from '../lib/types';

  let {
    threadId,
    region,
    initial,
    hash,
    onsaved,
    onreload,
    ondone,
  }: {
    threadId: string;
    region: RegionName;
    initial: string;
    hash: string;
    /** Hands back the fresh detail so the caller can repaint without refetching. */
    onsaved: (d: ThreadDetail) => void;
    /** Asked for after a conflict: take whatever the server holds. */
    onreload: () => void;
    /** Esc: commit and hand the reader back their rendered view. */
    ondone: () => void;
  } = $props();

  // Idle before we consider a write, and the floor between two writes. Plane
  // allows 60 requests a minute per key and the sync worker already reserves a
  // background floor, so a long typing session must not turn into a request per
  // keystroke batch. Worst case here is ~6 writes a minute.
  const IDLE_MS = 1500;
  const FLOOR_MS = 10_000;

  type State = 'clean' | 'dirty' | 'saving' | 'saved' | 'conflict' | 'error';

  // The buffer is SEEDED from the props and then owned here. untrack says that
  // outright: a later `initial` must not silently overwrite what someone is
  // typing — only an explicit reset does.
  let text = $state(untrack(() => initial));
  let base = $state(untrack(() => hash));
  let saved = $state(untrack(() => initial)); // what the server last confirmed it holds
  let status = $state<State>('clean');
  let message = $state<string | null>(null);
  // House-standard violations this write introduced (spec §3.1, §3.2).
  let lint = $state<{ rule: string; message: string; line?: number }[]>([]);

  let host = $state<HTMLElement | null>(null);
  let cm: MarkdownEditor | null = null;
  let ready = $state(false);

  let idle: ReturnType<typeof setTimeout> | null = null;
  let lastWrite = 0;
  let inflight = false;

  // Dynamic import so none of CodeMirror lands in the main bundle: the editor
  // only exists once somebody switches to Markdown, the same way mermaid only
  // loads for a document that actually has a diagram.
  // Extensions are fixed at construction, so flipping vim means rebuilding the
  // editor. Reading vimPref.on HERE is what makes that happen; the caret is
  // carried across so the switch is not also a jump to the top of the document.
  $effect(() => {
    const el = host;
    const wantVim = vimPref.on;
    if (!el) return;
    const at = cm?.cursor();
    cm?.destroy();
    cm = null;
    ready = false;
    let cancelled = false;
    void (async () => {
      const { createMarkdownEditor } = await import('../lib/editor');
      if (cancelled || !host) return;
      cm = await createMarkdownEditor({
        parent: el,
        doc: untrack(() => text),
        vim: wantVim,
        cursor: at,
        // untracked: this effect must not re-run on a theme switch. Most of
        // the editor's colour comes from CSS vars and follows the theme on its
        // own; the flag only picks CodeMirror's internal defaults.
        dark: untrack(() => theme.resolved === 'dark'),
        placeholder: t('editor.placeholder'),
        onChange: onDocChanged,
        onKey: onEditorKey,
        onSave: () => void save(),
        onEscape,
        onBlur: () => {
          slash = null;
          flush();
        },
      });
      ready = true;
      cm.focus();
    })();
    return () => {
      cancelled = true;
      cm?.destroy();
      cm = null;
      ready = false;
    };
  });

  function cancel(): void {
    if (idle) clearTimeout(idle);
    idle = null;
  }

  function schedule(): void {
    cancel();
    // Never sooner than the floor allows, so holding a key down cannot outrun it.
    const wait = Math.max(IDLE_MS, FLOOR_MS - (Date.now() - lastWrite));
    idle = setTimeout(() => void save(), wait);
  }

  function onDocChanged(doc: string): void {
    text = doc;
    detectSlash();
    if (status === 'conflict') return; // resolve it before typing over it again
    status = text === saved ? 'clean' : 'dirty';
    message = null;
    if (status === 'dirty') schedule();
  }

  function onEscape(): void {
    // Esc means "I am done here" — but only once the menu is out of the way, so
    // the first press never costs you the block you were inserting.
    if (slash) {
      slash = null;
      return;
    }
    void save();
    ondone();
  }

  export async function save(): Promise<void> {
    cancel();
    // Unchanged costs nothing — the server agrees, but not sending it at all is
    // what keeps focus and blur free.
    if (inflight || text === saved) {
      if (text === saved && status === 'dirty') status = 'clean';
      return;
    }
    inflight = true;
    status = 'saving';
    // Capture what we are sending: typing continues during the round trip, and
    // treating the post-await `text` as "what the server has" would mark those
    // keystrokes saved and schedule nothing to write them.
    const sending = text;
    try {
      const d = await api.updateThread(threadId, {
        [region]: sending,
        base: { [region]: base },
        // Warn, do not refuse: someone half-way through typing a heading is not
        // yet in violation of anything, and a save that starts failing mid-
        // sentence teaches people to distrust autosave.
        lenient: true,
      } as Parameters<typeof api.updateThread>[1]);
      lastWrite = Date.now();
      saved = sending;
      lint = d.warnings ?? [];
      // Adopt the server's hash: it is the authority on what the body now is,
      // and the next write has to be based on THAT, not on what we sent.
      base = d.regions?.[region]?.hash ?? base;
      onsaved(d);
      if (text !== saved) {
        status = 'dirty';
        schedule(); // they kept typing — write the rest
      } else {
        status = 'saved';
        setTimeout(() => status === 'saved' && (status = 'clean'), 1600);
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        status = 'conflict';
        message = t('editor.conflict');
      } else {
        status = 'error';
        message = e instanceof Error ? e.message : String(e);
        schedule(); // a transient failure should not lose the work
      }
    } finally {
      inflight = false;
    }
  }

  // Leaving without flushing is how autosave loses work, so blur, navigation and
  // closing the tab all force one.
  function flush(): void {
    if (text !== saved && status !== 'conflict') void save();
  }
  $effect(() => {
    const onLeave = () => flush();
    window.addEventListener('beforeunload', onLeave);
    return () => window.removeEventListener('beforeunload', onLeave);
  });
  onDestroy(() => {
    cancel();
    flush();
  });

  /** True while there is unsaved work — the caller uses it to refuse a clobber. */
  export function isDirty(): boolean {
    return text !== saved;
  }

  // ---- the slash menu (docs/ARTIFACT-EDITING.md Phase 5) ----
  //
  // Notion-style: type "/" on a fresh line, filter, hit enter. Each command
  // inserts plain markdown, because markdown is what the buffer IS — there is no
  // hidden document model here that could disagree with the text on screen.
  //
  // Labels go through t() with LITERAL keys rather than `t('cmd.' + id)`: the
  // catalogue is typed, and a computed key is exactly the shape that silently
  // stops being translated.
  interface Command {
    id: string;
    label: () => string;
    glyph: string;
    /** Markdown to insert, and where the caret lands inside it. */
    text: string;
    caret: number;
    /** Extra words to match on, so "checkbox" finds the to-do. */
    alias?: string;
  }

  const COMMANDS: Command[] = [
    { id: 'h1', label: () => t('cmd.h1'), glyph: 'H1', text: '# ', caret: 2, alias: 'title heading' },
    { id: 'h2', label: () => t('cmd.h2'), glyph: 'H2', text: '## ', caret: 3, alias: 'heading' },
    { id: 'h3', label: () => t('cmd.h3'), glyph: 'H3', text: '### ', caret: 4, alias: 'heading' },
    { id: 'bullet', label: () => t('cmd.bullet'), glyph: '•', text: '- ', caret: 2, alias: 'list ul' },
    {
      id: 'numbered',
      label: () => t('cmd.numbered'),
      glyph: '1.',
      text: '1. ',
      caret: 3,
      alias: 'list ol ordered',
    },
    {
      id: 'todo',
      label: () => t('cmd.todo'),
      glyph: '☐',
      text: '- [ ] ',
      caret: 6,
      alias: 'task checkbox tick',
    },
    { id: 'quote', label: () => t('cmd.quote'), glyph: '❝', text: '> ', caret: 2, alias: 'blockquote' },
    {
      id: 'code',
      label: () => t('cmd.code'),
      glyph: '</>',
      text: '```\n\n```\n',
      caret: 4,
      alias: 'pre fence',
    },
    {
      id: 'mermaid',
      label: () => t('cmd.mermaid'),
      glyph: '◇',
      text: '```mermaid\ngraph TD\n  A --> B\n```\n',
      caret: 11,
      alias: 'diagram graph chart',
    },
    {
      id: 'table',
      label: () => t('cmd.table'),
      glyph: '▦',
      text: '| a | b |\n| --- | --- |\n|  |  |\n',
      caret: 2,
      alias: 'grid',
    },
    { id: 'divider', label: () => t('cmd.divider'), glyph: '—', text: '---\n', caret: 4, alias: 'hr rule' },
    { id: 'link', label: () => t('cmd.link'), glyph: '🔗', text: '[](url)', caret: 1, alias: 'url href' },
  ];

  // at is the index of the "/" itself, so running a command can replace the
  // whole token rather than leaving it behind.
  let slash = $state<{
    at: number;
    query: string;
    /** just below the caret — where the menu sits when it opens downwards */
    top: number;
    /** the caret's own top — where the menu's BOTTOM goes when it flips up */
    caretTop: number;
    left: number;
  } | null>(null);
  let picked = $state(0);
  let menuEl = $state<HTMLElement | null>(null);
  let menuBox = $state<HTMLElement | null>(null);
  let fieldEl = $state<HTMLElement | null>(null);
  let itemEls = $state<HTMLElement[]>([]);

  // Where the menu actually lands. A caret near the bottom of the editor has no
  // room beneath it, so the menu flips above the caret rather than being cut off
  // by the editor's own box; a caret near the right edge shifts left. Measured
  // rather than assumed, because the menu's height depends on how many commands
  // survived the filter.
  let place = $state<{ top: number; left: number } | null>(null);

  $effect(() => {
    const s = slash;
    const box = menuBox;
    const field = fieldEl;
    // matches is a dependency on purpose: filtering changes the height.
    void matches.length;
    if (!s) {
      // Clear it, or the next open flashes at the previous caret's position.
      untrack(() => (place = null));
      return;
    }
    if (!box || !field) return;
    const h = box.offsetHeight;
    const w = box.offsetWidth;
    const roomBelow = field.clientHeight - s.top;
    const flip = h > roomBelow && s.caretTop > h;
    const left = Math.max(0, Math.min(s.left, field.clientWidth - w));
    untrack(() => {
      place = { top: flip ? s.caretTop - h : s.top, left };
    });
  });

  // Keep the highlighted command visible. Deliberately container maths rather
  // than scrollIntoView: that walks every scrollable ancestor and would nudge
  // the page behind the menu.
  $effect(() => {
    const list = menuEl;
    const el = itemEls[picked];
    if (!slash || !list || !el) return;
    const top = el.offsetTop;
    const bottom = top + el.offsetHeight;
    if (top < list.scrollTop) {
      list.scrollTop = top;
    } else if (bottom > list.scrollTop + list.clientHeight) {
      list.scrollTop = bottom - list.clientHeight;
    }
  });

  const matches = $derived(
    slash
      ? COMMANDS.filter((c) => {
          const q = slash!.query.toLowerCase();
          if (!q) return true;
          return (c.label().toLowerCase() + ' ' + c.id + ' ' + (c.alias ?? '')).includes(q);
        })
      : [],
  );

  function detectSlash(): void {
    if (!cm) return;
    const found = findSlash(text, cm.cursor());
    const point = found ? cm.caret() : null;
    if (!found || !point) {
      slash = null;
      return;
    }
    slash = {
      ...found,
      top: point.top + point.line,
      caretTop: point.top,
      left: point.left,
    };
    picked = 0;
  }

  function runCommand(c: Command): void {
    if (!slash || !cm) return;
    const end = cm.cursor();
    // Every one of these is a block, so it starts its own line — otherwise
    // "note /todo" would render as one paragraph rather than a checkbox.
    const before = text.slice(0, slash.at);
    const lead = before === '' || before.endsWith('\n') ? '' : '\n';
    cm.replace(slash.at, end, lead + c.text, lead.length + c.caret);
    slash = null;
  }

  /** Keys the menu owns while it is open. Returns true when it consumed one. */
  function onEditorKey(key: string): boolean {
    if (!slash) return false;
    switch (key) {
      case 'ArrowDown':
        picked = matches.length ? (picked + 1) % matches.length : 0;
        return true;
      case 'ArrowUp':
        picked = matches.length ? (picked - 1 + matches.length) % matches.length : 0;
        return true;
      case 'Enter':
      case 'Tab':
        if (!matches[picked]) return false;
        runCommand(matches[picked]);
        return true;
      case 'Escape':
        slash = null;
        return true;
    }
    return false;
  }

  // ---- toolbar ----
  function wrapSel(marker: string, placeholder: string): void {
    cm?.wrap(marker, placeholder);
  }
  function prefixLines(prefix: string): void {
    cm?.prefix(prefix);
  }
  function insertLink(): void {
    if (!cm) return;
    const at = cm.cursor();
    cm.replace(at, at, `[${t('editor.linkText')}](url)`, 1);
  }

  const label = $derived(
    status === 'saving'
      ? t('editor.saving')
      : status === 'saved'
        ? t('editor.saved')
        : status === 'dirty'
          ? t('editor.unsaved')
          : status === 'conflict'
            ? t('editor.conflictShort')
            : status === 'error'
              ? t('editor.failed')
              : t('editor.upToDate'),
  );
</script>

<div class="editor">
  <div class="bar">
    <button type="button" onclick={() => prefixLines('## ')} title={t('editor.heading')}>H</button>
    <button
      type="button"
      onclick={() => wrapSel('**', t('editor.boldText'))}
      title={t('editor.bold')}><b>B</b></button
    >
    <button
      type="button"
      onclick={() => wrapSel('*', t('editor.italicText'))}
      title={t('editor.italic')}><i>I</i></button
    >
    <button type="button" onclick={() => prefixLines('- ')} title={t('editor.bullet')}>•</button>
    <button type="button" onclick={() => prefixLines('- [ ] ')} title={t('editor.todo')}>☐</button>
    <button type="button" onclick={() => prefixLines('> ')} title={t('editor.quote')}>❝</button>
    <button type="button" onclick={() => wrapSel('`', 'code')} title={t('editor.code')}
      >{'</>'}</button
    >
    <button type="button" onclick={insertLink} title={t('editor.link')}>🔗</button>

    <span class="grow"></span>
    <button
      type="button"
      class="vim"
      class:on={vimPref.on}
      onclick={() => vimPref.toggle()}
      title={t('editor.vimTitle')}>vim</button
    >
    <span class="esc">{vimPref.on ? t('editor.vimHint') : t('editor.escHint')}</span>
    <span class="status" class:warn={status === 'conflict' || status === 'error'}>{label}</span>
  </div>

  {#if lint.length}
    <ul class="lint">
      {#each lint as f (f.rule)}
        <li>{f.message}</li>
      {/each}
    </ul>
  {/if}

  {#if message}
    <p class="msg" class:warn={status === 'conflict' || status === 'error'}>
      {message}
      {#if status === 'conflict'}
        <button type="button" class="link" onclick={onreload}>{t('editor.reload')}</button>
      {/if}
    </p>
  {/if}

  <div class="field" bind:this={fieldEl}>
    <div class="cm" bind:this={host}></div>
    {#if !ready}
      <p class="loading">{t('board.loading')}</p>
    {/if}

    {#if slash}
      <!-- Anchored AT the caret, which is the whole difference between this and
           a command palette. -->
      <div
        class="slash"
        style="top:{place?.top ?? slash.top}px; left:{place?.left ?? slash.left}px; visibility:{place
          ? 'visible'
          : 'hidden'}"
        role="listbox"
        tabindex="-1"
        bind:this={menuBox}
      >
        <div class="slash-list" bind:this={menuEl}>
          {#if matches.length === 0}
            <p class="slash-none">{t('cmd.none')}</p>
          {:else}
            {#each matches as c, i (c.id)}
            <button
              type="button"
              class="slash-item"
              class:on={i === picked}
              bind:this={itemEls[i]}
              role="option"
              aria-selected={i === picked}
              onmouseenter={() => (picked = i)}
              onmousedown={(e) => {
                e.preventDefault(); // keep focus in the editor
                runCommand(c);
              }}
            >
                <span class="slash-glyph">{c.glyph}</span>{c.label()}
              </button>
            {/each}
          {/if}
        </div>
        <!-- outside the scroller, so the keys stay readable while you scroll -->
        <p class="slash-hint">{t('cmd.hint')}</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    min-height: 0;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-wrap: wrap;
  }
  .bar button {
    min-width: 2rem;
    padding: 0.25rem 0.45rem;
    border-radius: 8px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 4%, transparent);
    color: var(--muted);
    font-size: 0.82rem;
    line-height: 1.2;
    cursor: pointer;
  }
  .bar button:hover {
    color: var(--text);
    border-color: color-mix(in oklab, var(--wip) 45%, var(--line));
  }
  .grow {
    flex: 1;
  }
  .vim {
    min-width: auto !important;
    font-family: var(--sans);
    font-size: 0.68rem !important;
    font-weight: 700;
    letter-spacing: 0.04em;
  }
  .vim.on {
    color: oklch(0.16 0.02 265);
    background: var(--wip);
    border-color: transparent;
  }
  .esc {
    font-family: var(--sans);
    font-size: 0.68rem;
    color: var(--faint);
    white-space: nowrap;
  }
  .status {
    font-size: 0.72rem;
    color: var(--muted);
    font-family: var(--sans);
    white-space: nowrap;
  }
  .status.warn {
    color: oklch(0.72 0.19 25);
  }
  .lint {
    margin: 0;
    padding: 0.45rem 0.7rem 0.45rem 1.6rem;
    border-radius: 10px;
    border: 1px solid color-mix(in oklab, oklch(0.78 0.16 85) 40%, var(--line));
    background: color-mix(in oklab, oklch(0.78 0.16 85) 9%, transparent);
    font-family: var(--sans);
    font-size: 0.76rem;
    line-height: 1.5;
    color: var(--text);
  }
  .msg {
    margin: 0;
    font-size: 0.78rem;
    color: var(--muted);
  }
  .msg.warn {
    color: oklch(0.72 0.19 25);
  }
  .link {
    border: none;
    background: none;
    padding: 0;
    margin-left: 0.4rem;
    color: var(--wip);
    text-decoration: underline;
    cursor: pointer;
    font: inherit;
  }
  .field {
    position: relative;
    flex: 1;
    /* A DEFINITE height, not min-height.
       CodeMirror sizes itself with `height: 100%`, which resolves against its
       parent — and every ancestor here is auto-height, so it resolved to
       "as tall as the content". The editor grew without bound, its internal
       scroller never engaged, and the toolbar and save status scrolled off the
       top of a long Logbook. Bounding the box is what gives CodeMirror
       something to scroll inside.
       Two editors stack when a Logbook has a DoD, so this is deliberately not
       a full viewport each. */
    height: clamp(18rem, calc(100dvh - 21rem), 46rem);
    display: flex;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 4%, transparent);
    /* NOT hidden: this box is the slash menu's positioning parent, and clipping
       it cropped the menu the moment the caret neared an edge. */
    overflow: visible;
  }
  .field:focus-within {
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
  }
  .cm {
    flex: 1;
    min-width: 0;
    /* hidden, not auto: CodeMirror has its own scroller (.cm-scroller) and a
       second one here would fight it — you would get an outer scrollbar that
       moves nothing. */
    overflow: hidden;
    border-radius: 12px; /* what .field's overflow used to do */
  }
  .loading {
    position: absolute;
    top: 0.9rem;
    left: 1rem;
    margin: 0;
    font-size: 0.82rem;
    color: var(--faint);
  }
  .slash {
    position: absolute;
    z-index: 20;
    display: flex;
    flex-direction: column;
    min-width: 13rem;
    max-height: 15rem;
    padding: 0.3rem;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    box-shadow: 0 18px 40px var(--shadow-strong);
  }
  .slash-list {
    position: relative; /* the offsetParent the scroll maths measures against */
    overflow-y: auto;
    min-height: 0;
  }
  .slash-item {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    padding: 0.35rem 0.5rem;
    border: none;
    border-radius: 8px;
    background: none;
    color: var(--text);
    font-family: var(--sans);
    font-size: 0.82rem;
    text-align: left;
    cursor: pointer;
  }
  .slash-item.on {
    background: color-mix(in oklab, var(--wip) 18%, transparent);
  }
  .slash-glyph {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 1.6rem;
    height: 1.35rem;
    border-radius: 6px;
    border: 1px solid var(--line);
    color: var(--muted);
    font-size: 0.68rem;
    font-weight: 700;
  }
  .slash-none,
  .slash-hint {
    margin: 0;
    padding: 0.35rem 0.55rem;
    font-family: var(--sans);
    font-size: 0.68rem;
    color: var(--muted);
  }
  .slash-hint {
    flex: none;
    border-top: 1px solid var(--line);
    margin-top: 0.25rem;
  }
</style>
