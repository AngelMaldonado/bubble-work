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
        onChange: (next) => (value = next),
        onSave: () => (editing = false),
        onEscape: () => (editing = false),
        onBlur: () => {},
      }),
    ).then((m) => {
      if (!live) return m.destroy();
      made = m;
      m.focus();
    });
    return () => {
      live = false;
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
    <div class="editor" {@attach mount}></div>
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
  .editor { overflow: hidden; }
  /* CodeMirror sizes itself to its content, so `min-height` on the box around it
     left a tall box with a short editor inside and a dead strip underneath. The
     minimum belongs to the editor itself. */
  .editor :global(.cm-editor) { min-height: var(--min); }
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
