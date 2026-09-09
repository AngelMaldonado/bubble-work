<script lang="ts">
  // A document, read or written — the pane a thread has always had, extracted so
  // the wiki has the SAME one.
  //
  // They are the same kind of thing: markdown in a workspace's repository,
  // rendered by the server, written with a base hash. Two panes would be two
  // renderers, two slash menus and two sets of keys to learn, and they would
  // drift the week after somebody improved one of them.
  //
  // What it owns: the renderizado/markdown switch, the vim preference, the
  // editor itself, the slash menu, and saving. What it does NOT own is where
  // the text comes from or where it goes — that is the caller's, because one
  // writes a thread's document and the other a page.
  import { untrack } from 'svelte';
  import Prose from './Prose.svelte';
  import SlashMenu from './SlashMenu.svelte';
  import ThreadToc from './ThreadToc.svelte';
  import { SlashMenu as SlashMenuState } from '../lib/slashmenu.svelte';
  import { vimPref } from '../lib/vim.svelte';
  import type { MarkdownEditor } from '../lib/editor';
  import type { Heading } from '../lib/prose';

  let {
    markdown = '',
    html = '',
    editing = $bindable(false),
    /** somebody else wrote while this was open */
    elsewhere = false,
    onsave,
    onreload,
    onheadings,
  }: {
    markdown?: string;
    html?: string;
    editing?: boolean;
    elsewhere?: boolean;
    onsave?: (markdown: string) => void;
    onreload?: () => void;
    onheadings?: (h: Heading[]) => void;
  } = $props();

  let editor = $state<MarkdownEditor | null>(null);
  let editorBox = $state<HTMLElement | null>(null);
  let caretAt = 0;
  let draft = $state('');
  const slash = new SlashMenuState();

  $effect(() => {
    draft = markdown;
    // And into the editor, if one is open. The editor is built ONCE (it reads
    // `draft` untracked, or every keystroke would rebuild it), so without this
    // a reload after a conflict changed the document underneath and left the
    // old text on screen — the reload appeared to do nothing.
    editor?.setDoc(markdown);
  });

  /** Saving is explicit and cheap to trigger: ⌘S, `:w`, and leaving the editor.
   *  A document that only saves on a button is a document somebody loses. */
  export function save() {
    if (draft !== markdown) onsave?.(draft);
  }

  function mountEditor(el: HTMLElement) {
    // What this attachment depends on, spelled out. `vimPref.on` is READ here,
    // synchronously, because toggling vim has to rebuild the editor — read
    // inside the dynamic import's callback it is outside the reactive context
    // and the toggle does nothing. `draft` is read UNTRACKED for the opposite
    // reason: it changes on every keystroke, and tracking it would tear the
    // editor down and build a new one per character.
    const useVim = vimPref.on;
    const doc = untrack(() => draft);

    let live = true;
    let made: MarkdownEditor | null = null;
    // Imported HERE, not at the top of the file. A static import puts
    // CodeMirror and its markdown grammar in the main bundle, which everyone
    // downloads to look at a board they may never edit — measured at +240 kB
    // gzip before this line was a function call.
    import('../lib/editor')
      .then(({ createMarkdownEditor }) =>
        createMarkdownEditor({
          parent: el,
          doc,
          dark: document.documentElement.getAttribute('data-mode') === 'dark',
          vim: useVim,
          cursor: caretAt,
          onChange: (next) => {
            draft = next;
            slash.detect(made, next);
          },
          // The menu owns these keys while it is open — that is the whole
          // reason `onKey` exists in the editor.
          onKey: (key) => slash.key(key, made, untrack(() => draft)),
          onSave: save,
          onEscape: () => {
            save();
            editing = false;
          },
          onBlur: save,
        }),
      )
      .then((made_) => {
        // The mode can change while the dynamic import is in flight; without
        // this the editor lands in a box that is no longer on the page.
        if (!live) return made_.destroy();
        made = made_;
        editor = made_;
        made_.focus();
      });
    return () => {
      live = false;
      caretAt = made?.cursor() ?? caretAt;
      slash.open = null;
      made?.destroy();
      if (editor === made) editor = null;
    };
  }
</script>

<!-- Only the view switch lives in the document's own bar: it changes how you
     READ this page, so it belongs to the page. What you can DO to the thread or
     the page is in the HUD, where the board keeps its verbs. -->
<div class="edit-bar">
  <div class="seg" role="group" aria-label="modo de vista">
    <button
      class:on={!editing}
      aria-pressed={!editing}
      onclick={() => {
        save();
        editing = false;
      }}>renderizado</button>
    <button class:on={editing} aria-pressed={editing} onclick={() => (editing = true)}>markdown</button>
  </div>
  {#if editing}
    <!-- Only while there is an editor to apply it to. A preference for how to
         type, shown where you chose to type. -->
    <button
      class="vim"
      class:on={vimPref.on}
      aria-pressed={vimPref.on}
      title="teclas de vim ({vimPref.on ? 'activadas' : 'desactivadas'})"
      onclick={() => vimPref.toggle()}>vim</button>
  {/if}
</div>

{#if elsewhere}
  <!-- Not an error and not a refusal: the base hash says so, and reloading is a
       choice offered rather than a save silently lost. -->
  <p class="elsewhere">
    Este documento cambió en otro lado, así que tu escritura no se guardó. Lo que escribiste sigue
    en el editor.
    <button type="button" class="link" onclick={onreload}>recargar</button>
  </p>
{/if}

{#if editing}
  <!-- The menu is a SIBLING of the editor, in a box that positions it: the
       editor's own box clips its overflow (that is what keeps CodeMirror inside
       its rounded corner), and a menu inside it would be cut off the moment the
       caret was near an edge. -->
  <div class="editors-wrap">
    <div class="editors" bind:this={editorBox} {@attach mountEditor}></div>
    <SlashMenu menu={slash} field={editorBox} onpick={(c) => slash.run(editor, draft, c)} />
  </div>
{:else}
  <Prose {html} {onheadings} />
{/if}

<style>
  .edit-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem; margin-bottom: 1rem; }
  .seg { margin-left: auto; display: flex; border: 1px solid var(--line); border-radius: 999px; overflow: hidden; }
  .seg button { border: none; border-radius: 0; padding: 0.25rem 0.75rem; }
  .seg button.on { background: var(--hover); color: var(--text); }

  .vim {
    padding: 0.28rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: transparent;
    color: var(--faint);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.76rem;
  }
  .vim:hover { color: var(--text); background: var(--hover); }
  .vim.on { color: var(--accent); border-color: color-mix(in oklab, var(--accent) 45%, transparent); }

  .elsewhere {
    margin: 0 0 1rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid color-mix(in oklab, var(--warm) 40%, transparent);
    border-radius: 10px;
    background: color-mix(in oklab, var(--warm) 16%, transparent);
    color: var(--text);
    font-size: 0.82rem;
  }

  /* The positioning box IS the editor's box: the caret's coordinates come back
     relative to CodeMirror's own element, so a wider wrapper would put the slash
     menu one margin to the left. */
  .editors-wrap {
    position: relative;
    max-width: 940px;
    margin-inline: auto;
  }
  .editors {
    height: calc(100dvh - var(--topbar-h) - 8rem);
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface-solid);
    overflow: hidden;
  }
  .editors :global(.cm-vim-panel) {
    padding: 0.2rem 0.6rem;
    border-top: 1px solid var(--line);
    background: var(--surface);
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.78rem;
  }
  .editors :global(.cm-vim-panel input) { color: var(--text); background: transparent; }
</style>
