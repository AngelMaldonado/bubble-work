<script lang="ts">
  // Editing a thread's artifacts from the board (docs/ARTIFACT-EDITING.md Phase 4).
  //
  // The editor is MARKDOWN SOURCE, not WYSIWYG, and that is a decision rather
  // than a shortcut: everything downstream already speaks markdown (the TOC, the
  // minimap, the todo parser, the logbook fingerprint), and Plane serves its
  // description images only to a web session — so a WYSIWYG here would be a
  // WYSIWYG that cannot show you the pictures.
  //
  // Saving is autosave, which forces two rules the server enforces too: a region
  // submitted unchanged writes nothing at all, and every write carries the hash
  // of what was read so a lost update becomes a 409 instead of a silent
  // overwrite.
  import { onDestroy, untrack } from 'svelte';
  import { api, ApiError } from '../lib/api';
  import { caretPoint } from '../lib/caret';
  import { findSlash } from '../lib/slash';
  import { t } from '../lib/i18n.svelte';
  import type { RegionName, ThreadDetail } from '../lib/types';

  let {
    threadId,
    region,
    initial,
    hash,
    onsaved,
    onreload,
  }: {
    threadId: string;
    region: RegionName;
    initial: string;
    hash: string;
    /** Hands back the fresh detail so the caller can repaint without refetching. */
    onsaved: (d: ThreadDetail) => void;
    /** Asked for after a conflict: take whatever the server holds. */
    onreload: () => void;
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
  // typing — only an explicit reset does, below.
  let text = $state(untrack(() => initial));
  let base = $state(untrack(() => hash));
  let saved = $state(untrack(() => initial)); // what the server last confirmed it holds
  let status = $state<State>('clean');
  let message = $state<string | null>(null);
  let box = $state<HTMLTextAreaElement | null>(null);

  let idle: ReturnType<typeof setTimeout> | null = null;
  let lastWrite = 0;
  let inflight = false;

  // A thread or region switch has to reset everything, or the next one inherits
  // the previous buffer and base hash — and writes it over the wrong thread.
  let loadedFor = $state(untrack(() => `${threadId}:${region}`));
  $effect(() => {
    const key = `${threadId}:${region}`;
    if (key === loadedFor) return;
    untrack(() => {
      cancel();
      loadedFor = key;
      text = initial;
      saved = initial;
      base = hash;
      status = 'clean';
      message = null;
    });
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

  function onInput(): void {
    detectSlash();
    if (status === 'conflict') return; // resolve it before typing over it again
    status = text === saved ? 'clean' : 'dirty';
    message = null;
    if (status === 'dirty') schedule();
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
      } as Parameters<typeof api.updateThread>[1]);
      lastWrite = Date.now();
      saved = sending;
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

  function onBlur(): void {
    slash = null;
    flush();
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

  /** Discard local edits and take whatever the server holds. */
  export function reload(next: string, nextHash: string): void {
    cancel();
    text = next;
    saved = next;
    base = nextHash;
    status = 'clean';
    message = null;
  }

  /** True while there is unsaved work — the caller uses it to refuse a clobber. */
  export function isDirty(): boolean {
    return text !== saved;
  }

  // ---- markdown helpers, the same shape the comment composer already uses ----
  function restoreSel(start: number, end: number): void {
    requestAnimationFrame(() => {
      if (!box) return;
      box.focus();
      box.selectionStart = start;
      box.selectionEnd = end;
    });
  }
  function wrapSel(marker: string, placeholder: string): void {
    if (!box) return;
    const s = box.selectionStart;
    const e = box.selectionEnd;
    const chosen = text.slice(s, e) || placeholder;
    text = text.slice(0, s) + marker + chosen + marker + text.slice(e);
    restoreSel(s + marker.length, s + marker.length + chosen.length);
    onInput();
  }
  function prefixLines(prefix: string): void {
    if (!box) return;
    const s = box.selectionStart;
    const e = box.selectionEnd;
    const lineStart = text.lastIndexOf('\n', s - 1) + 1;
    const replaced = text
      .slice(lineStart, e)
      .split('\n')
      .map((l) => prefix + l)
      .join('\n');
    text = text.slice(0, lineStart) + replaced + text.slice(e);
    restoreSel(lineStart, lineStart + replaced.length);
    onInput();
  }
  function insertLink(): void {
    if (!box) return;
    const s = box.selectionStart;
    const e = box.selectionEnd;
    const label = text.slice(s, e) || t('editor.linkText');
    const snippet = `[${label}](url)`;
    text = text.slice(0, s) + snippet + text.slice(e);
    const at = s + snippet.indexOf('url');
    restoreSel(at, at + 3);
    onInput();
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
    { id: 'numbered', label: () => t('cmd.numbered'), glyph: '1.', text: '1. ', caret: 3, alias: 'list ol ordered' },
    { id: 'todo', label: () => t('cmd.todo'), glyph: '☐', text: '- [ ] ', caret: 6, alias: 'task checkbox tick' },
    { id: 'quote', label: () => t('cmd.quote'), glyph: '❝', text: '> ', caret: 2, alias: 'blockquote' },
    { id: 'code', label: () => t('cmd.code'), glyph: '</>', text: '```\n\n```\n', caret: 4, alias: 'pre fence' },
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
  let slash = $state<{ at: number; query: string; top: number; left: number } | null>(null);
  let picked = $state(0);

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
    if (!box) {
      slash = null;
      return;
    }
    const found = findSlash(text, box.selectionStart);
    if (!found) {
      slash = null;
      return;
    }
    const point = caretPoint(box, found.at);
    slash = { ...found, top: point.top + point.line, left: point.left };
    picked = 0;
  }

  function runCommand(c: Command): void {
    if (!slash || !box) return;
    const end = box.selectionStart;
    const before = text.slice(0, slash.at);
    const after = text.slice(end);
    // Every one of these is a block, so it starts its own line — otherwise
    // "note /todo" would render as one paragraph rather than a checkbox.
    const lead = before === '' || before.endsWith('\n') ? '' : '\n';
    text = before + lead + c.text + after;
    const caret = slash.at + lead.length + c.caret;
    slash = null;
    restoreSel(caret, caret);
    onInput();
  }

  function onKeydown(e: KeyboardEvent): void {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      void save();
      return;
    }
    if (!slash) return;
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        picked = matches.length ? (picked + 1) % matches.length : 0;
        break;
      case 'ArrowUp':
        e.preventDefault();
        picked = matches.length ? (picked - 1 + matches.length) % matches.length : 0;
        break;
      case 'Enter':
      case 'Tab':
        if (matches[picked]) {
          e.preventDefault();
          runCommand(matches[picked]);
        }
        break;
      case 'Escape':
        e.preventDefault();
        slash = null;
        break;
    }
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
    <button type="button" onclick={() => wrapSel('**', t('editor.boldText'))} title={t('editor.bold')}
      ><b>B</b></button
    >
    <button type="button" onclick={() => wrapSel('*', t('editor.italicText'))} title={t('editor.italic')}
      ><i>I</i></button
    >
    <button type="button" onclick={() => prefixLines('- ')} title={t('editor.bullet')}>•</button>
    <button type="button" onclick={() => prefixLines('- [ ] ')} title={t('editor.todo')}>☐</button>
    <button type="button" onclick={() => prefixLines('> ')} title={t('editor.quote')}>❝</button>
    <button type="button" onclick={() => wrapSel('`', 'code')} title={t('editor.code')}>{'</>'}</button>
    <button type="button" onclick={insertLink} title={t('editor.link')}>🔗</button>

    <span class="grow"></span>
    <span class="status" class:warn={status === 'conflict' || status === 'error'}>{label}</span>
  </div>

  {#if message}
    <p class="msg" class:warn={status === 'conflict' || status === 'error'}>
      {message}
      {#if status === 'conflict'}
        <button type="button" class="link" onclick={onreload}>
          {t('editor.reload')}
        </button>
      {/if}
    </p>
  {/if}

  <div class="field">
    <textarea
      bind:this={box}
      bind:value={text}
      oninput={onInput}
      onclick={detectSlash}
      onblur={onBlur}
      onkeydown={onKeydown}
      spellcheck="false"
      aria-label={t('editor.aria')}
    ></textarea>

    {#if slash}
      <!-- Anchored AT the caret, which is the whole difference between this and
           a command palette. -->
      <div class="slash" style="top:{slash.top}px; left:{slash.left}px" role="listbox" tabindex="-1">
        {#if matches.length === 0}
          <p class="slash-none">{t('cmd.none')}</p>
        {:else}
          {#each matches as c, i (c.id)}
            <button
              type="button"
              class="slash-item"
              class:on={i === picked}
              role="option"
              aria-selected={i === picked}
              onmouseenter={() => (picked = i)}
              onmousedown={(e) => {
                e.preventDefault(); // keep focus in the textarea
                runCommand(c);
              }}
            >
              <span class="slash-glyph">{c.glyph}</span>{c.label()}
            </button>
          {/each}
        {/if}
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
    height: 100%;
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
  .status {
    font-size: 0.72rem;
    color: var(--muted);
    font-family: var(--sans);
    white-space: nowrap;
  }
  .status.warn {
    color: oklch(0.72 0.19 25);
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
    display: flex;
    min-height: 0;
  }
  .slash {
    position: absolute;
    z-index: 20;
    min-width: 13rem;
    max-height: 15rem;
    overflow-y: auto;
    padding: 0.3rem;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    box-shadow: 0 18px 40px var(--shadow-strong);
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
    border-top: 1px solid var(--line);
    margin-top: 0.25rem;
  }
  textarea {
    flex: 1;
    min-height: 22rem;
    width: 100%;
    resize: vertical;
    padding: 0.9rem 1rem;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 4%, transparent);
    color: var(--text);
    outline: none;
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 0.86rem;
    line-height: 1.65;
    tab-size: 2;
  }
  textarea:focus {
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
  }
</style>
