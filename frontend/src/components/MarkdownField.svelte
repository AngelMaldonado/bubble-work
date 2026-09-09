<script lang="ts">
  // A markdown field that shows the DOCUMENT by default and the source when you
  // ask for it.
  //
  // Preview first is the point: a description is written once and read many
  // times, so the resting state is the reading state. Clicking the preview
  // starts editing, which is the gesture people already expect from a note.
  //
  // The HTML is not produced here. Rendering markdown is the server's job in
  // this system — one renderer, so every surface shows the identical thing —
  // and this component takes a `render` function so the real app can hand it the
  // server and a mock can hand it something local. Two renderers agree until
  // they do not.
  import Prose from './Prose.svelte';
  import { untrack } from 'svelte';
  import type { MarkdownEditor } from '../lib/editor';
  import SlashMenu from './SlashMenu.svelte';
  import { SlashMenu as SlashMenuState } from '../lib/slashmenu.svelte';
  import { vimPref } from '../lib/vim.svelte';

  let {
    value = $bindable(''),
    editing = $bindable(false),
    placeholder = 'Escribe en markdown…',
    minHeight = '9rem',
    /** hide the built-in switch, for a caller that drives editing from its own
        section header — the way a card's "Editar" button does */
    chrome = true,
    render,
  }: {
    value?: string;
    editing?: boolean;
    placeholder?: string;
    minHeight?: string;
    chrome?: boolean;
    /** markdown → html. The server in the real app; anything in a mock. */
    render?: (md: string) => string | Promise<string>;
  } = $props();
  let html = $state('');

  // The slash menu, at the caret. Same class and same list as the thread's
  // editor uses — one catalogue of blocks, not two that drift.
  const slash = new SlashMenuState();
  let editor = $state<MarkdownEditor | null>(null);
  let fieldEl = $state<HTMLElement | null>(null);

  $effect(() => {
    const md = value;
    if (editing || !render) return;
    Promise.resolve(render(md)).then((out) => (html = out));
  });

  function mount(el: HTMLElement) {
    // Read synchronously so toggling vim rebuilds the editor; `value` untracked
    // so typing does not.
    const useVim = vimPref.on;
    const doc = untrack(() => value);
    let live = true;
    let made: MarkdownEditor | null = null;
    import('../lib/editor').then(({ createMarkdownEditor }) =>
      createMarkdownEditor({
        parent: el,
        doc,
        dark: document.documentElement.getAttribute('data-mode') === 'dark',
        vim: useVim,
        placeholder,
        onChange: (next) => {
          value = next;
          slash.detect(made, next);
        },
        onKey: (key) => slash.key(key, made, untrack(() => value)),
        onSave: () => (editing = false),
        onEscape: () => (editing = false),
        onBlur: () => {},
      }),
    ).then((m) => {
      if (!live) return m.destroy();
      made = m;
      editor = m;
      m.focus();
    });
    return () => {
      live = false;
      slash.open = null;
      if (editor === made) editor = null;
      made?.destroy();
    };
  }
</script>

<div class="field" style="--min: {minHeight}">
  {#if chrome}
  <div class="bar">
    <div class="seg" role="group" aria-label="modo">
      <button class:on={!editing} aria-pressed={!editing} onclick={() => (editing = false)}>
        renderizado
      </button>
      <button class:on={editing} aria-pressed={editing} onclick={() => (editing = true)}>
        markdown
      </button>
    </div>
    {#if editing}
      <button
        class="vim"
        class:on={vimPref.on}
        aria-pressed={vimPref.on}
        title="teclas de vim"
        onclick={() => vimPref.toggle()}>vim</button>
    {/if}
  </div>
  {/if}

  {#if editing}
    <!-- No key handling here. Stopping Escape on the way up never kept it from
         the dialog — Zag listens on the DOCUMENT, which runs first — and a
         listener sitting between the editor and the page is exactly the kind of
         thing that swallows a key nobody meant it to. Whoever HOSTS this field
         turns `closeOnEscape` off while it is being written in. -->
    <!-- The menu is positioned against the EDITOR's rectangle, because that is
         what the caret's coordinates are relative to. Against the whole field
         it would be off by the toolbar above it. -->
    <div class="editor-wrap">
      <div class="editor" bind:this={fieldEl} {@attach mount}></div>
      <SlashMenu menu={slash} field={fieldEl} onpick={(c) => slash.run(editor, value, c)} />
    </div>
  {:else if value.trim()}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="preview" ondblclick={() => (editing = true)} title="doble clic para editar">
      <Prose {html} compact />
    </div>
  {:else}
    <button class="empty" onclick={() => (editing = true)}>{placeholder}</button>
  {/if}
</div>

<style>
  .field { display: flex; flex-direction: column; gap: 0.35rem; }
  /* Only a positioning box: it takes the editor's place in the column and the
     editor fills it, so the menu's coordinates and the editor's rectangle are
     the same rectangle. */
  .editor-wrap { position: relative; display: flex; flex-direction: column; }
  .bar { display: flex; align-items: center; gap: 0.4rem; }
  .seg {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: 9px;
    background: var(--surface);
  }
  .seg button {
    padding: 0.15rem 0.5rem;
    border-radius: 7px;
    color: var(--faint);
    font-size: 0.72rem;
  }
  .seg button.on { background: var(--hover); color: var(--text); font-weight: 600; }
  .vim {
    padding: 0.12rem 0.45rem;
    border: 1px solid var(--line);
    border-radius: 7px;
    color: var(--faint);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.7rem;
  }
  .vim.on { color: var(--accent); border-color: color-mix(in oklab, var(--accent) 45%, transparent); }

  .editor,
  .preview,
  .empty {
    min-height: var(--min);
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface);
  }
  /* A DEFINITE height, not a minimum.
     With a minimum the box is as tall as its content, and CodeMirror's panels —
     vim's `--INSERT--` among them — sit right after the last line, which put a
     status bar through the middle of the editor and across the slash menu. A
     definite height lets the editor fill it and the panel land at the bottom,
     which is where it is in a thread and where a status bar belongs. */
  .editor { height: var(--min); overflow: hidden; }
  .editor :global(.cm-editor) { height: 100%; }

  /* The same status bar the thread's editor has. */
  .editor :global(.cm-vim-panel) {
    padding: 0.2rem 0.6rem;
    border-top: 1px solid var(--line);
    background: var(--surface);
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.78rem;
  }
  .editor :global(.cm-vim-panel input) { color: var(--text); background: transparent; }
  .preview {
    padding: 0.6rem 0.8rem;
    overflow: auto;
    /* Reading room. The cap is what stops a long document from pushing the rest
       of a dialog off the screen; it was 40vh, which on a laptop is about eight
       lines. */
    max-height: 60vh;
  }
  .empty {
    display: flex;
    align-items: flex-start;
    padding: 0.6rem 0.8rem;
    color: var(--faint);
    font-size: 0.85rem;
    text-align: left;
  }
  .empty:hover { background: var(--hover); }
</style>
