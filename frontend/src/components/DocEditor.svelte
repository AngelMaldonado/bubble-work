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
    ondiff,
    /** somebody else wrote while this was open */
    elsewhere = false,
    onsave,
    onreload,
    onheadings,
    onattach,
  }: {
    markdown?: string;
    html?: string;
    editing?: boolean;
    elsewhere?: boolean;
    onsave?: (markdown: string) => void;
    onreload?: () => void;
    onheadings?: (h: Heading[]) => void;
    /** Los dos lados de un diff, tal como los da el servidor. Sin esto la
     *  pestaña no se dibuja: un botón que abre una pantalla vacía es peor que
     *  no tener el botón. */
    ondiff?: (since: string) => Promise<{ before: string; after: string; diff: string }>;
    /** Attach a file to the workspace and say what the document should call
     *  it. The caller owns the upload because it owns the workspace; this only
     *  puts the reference where the caret is. */
    onattach?: (file: File) => Promise<{ path: string } | null | void>;
  } = $props();

  // Tres modos, no dos: leer, escribir, y ver qué cambió. El tercero no es una
  // variante del segundo — se mira, no se toca — así que vive en el mismo
  // interruptor y no dentro del editor.
  let mode = $state<'read' | 'write' | 'diff'>('read');
  let since = $state('cycle');
  let patch = $state<{ before: string; after: string; diff: string } | null>(null);
  let loadingDiff = $state(false);

  // `editing` sigue siendo la verdad de quién escribe, porque es lo que el
  // resto de la aplicación mira; el modo es la vista.
  $effect(() => {
    editing = mode === 'write';
  });
  $effect(() => {
    if (mode !== 'diff' || !ondiff) return;
    const window = since;
    loadingDiff = true;
    patch = null;
    ondiff(window)
      .then((p) => (patch = p))
      .catch(() => (patch = null))
      .finally(() => (loadingDiff = false));
  });

  let editor = $state<MarkdownEditor | null>(null);
  let attaching = $state(false);
  let picker = $state<HTMLInputElement | null>(null);
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

  /** Upload, then write `![nombre](assets/x.png)` where the caret is.
   *
   *  The SHORT path goes in the document — `assets/x.png`, what the layout says
   *  a picture is called from anywhere — and the server rewrites it to the route
   *  that serves it when it renders. A document that spells out the route is one
   *  that cannot be moved or read from a git clone. */
  async function attach(file: File | null | undefined) {
    if (!file || !onattach || attaching) return;
    attaching = true;
    try {
      const out = await onattach(file);
      const path = out?.path;
      if (!path || !editor) return;
      const label = file.name.replace(/\.[^.]+$/, '');
      const at = editor.cursor();
      const text = `![${label}](${path})`;
      editor.replace(at, at, text, text.length);
      draft = editor.value();
    } finally {
      attaching = false;
    }
  }

  /** Pasting a screenshot is the fastest path there is, and the one people try
   *  first. The clipboard carries the image as a file with no name worth using;
   *  the caller's slug decides what it is called on disk. */
  function pasted(e: ClipboardEvent) {
    const file = [...(e.clipboardData?.files ?? [])][0];
    if (file && file.type.startsWith('image/')) {
      e.preventDefault();
      attach(file);
    }
  }

  function dropped(e: DragEvent) {
    const file = [...(e.dataTransfer?.files ?? [])][0];
    if (file) {
      e.preventDefault();
      attach(file);
    }
  }

  /** El diff, con el mismo editor y el mismo tema que el markdown de al lado. */
  function mountDiff(el: HTMLElement) {
    const p = patch;
    if (!p) return;
    let live = true;
    let made: { destroy(): void } | null = null;
    import('../lib/editor').then(({ createDiffView }) =>
      createDiffView({
        parent: el,
        before: p.before,
        after: p.after,
        dark: document.documentElement.getAttribute('data-mode') === 'dark',
      }).then((v) => {
        if (!live) return v.destroy();
        made = v;
      }),
    );
    return () => {
      live = false;
      made?.destroy();
    };
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
      class:on={mode === 'read'}
      aria-pressed={mode === 'read'}
      onclick={() => {
        save();
        mode = 'read';
      }}>renderizado</button>
    <button
      class:on={mode === 'write'}
      aria-pressed={mode === 'write'}
      onclick={() => (mode = 'write')}>markdown</button>
    {#if ondiff}
      <button
        class:on={mode === 'diff'}
        aria-pressed={mode === 'diff'}
        onclick={() => {
          save();
          mode = 'diff';
        }}>cambios</button>
    {/if}
  </div>
  {#if editing && onattach}
    <!-- Adjuntar vive donde se escribe, porque lo que produce es una línea de
         markdown en el documento. Arrastrar y pegar hacen lo mismo; el botón
         está para quien no sabe que puede. -->
    <button class="vim" disabled={attaching} onclick={() => picker?.click()} title="adjuntar una imagen">
      {attaching ? '…' : '📎'}
    </button>
    <input
      class="hidden-file"
      type="file"
      accept="image/*"
      bind:this={picker}
      onchange={(e) => {
        const el = e.currentTarget as HTMLInputElement;
        attach(el.files?.[0]);
        el.value = '';
      }} />
  {/if}
  {#if mode === 'diff'}
    <!-- Contra qué. El ciclo por defecto, porque es la ventana contra la que se
         mide todo lo demás; "último cambio" está para cuando el ciclo está
         vacío y lo que querías era ver lo último que alguien hizo. -->
    <div class="seg since" role="group" aria-label="ventana del diff">
      <button class:on={since === 'cycle'} onclick={() => (since = 'cycle')}>este ciclo</button>
      <button class:on={since === 'last'} onclick={() => (since = 'last')}>último cambio</button>
    </div>
  {/if}
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

{#if mode === 'diff'}
  <div class="editors-wrap">
    {#if loadingDiff}
      <p class="faint p-4 text-sm">…</p>
    {:else if patch && patch.diff}
      <div class="editors" {@attach mountDiff}></div>
    {:else}
      <p class="faint p-4 text-sm">
        {since === 'cycle'
          ? 'Nada cambió en este ciclo. Prueba «último cambio».'
          : 'Este documento no tiene historia todavía.'}
      </p>
    {/if}
  </div>
{:else if editing}
  <!-- The menu is a SIBLING of the editor, in a box that positions it: the
       editor's own box clips its overflow (that is what keeps CodeMirror inside
       its rounded corner), and a menu inside it would be cut off the moment the
       caret was near an edge. -->
  <div class="editors-wrap">
    <!-- `role="group"`: la caja recibe drops y CodeMirror pone dentro su propio
         textbox, así que el rol que describe la caja es el del grupo, no el del
         campo — que ya lo trae el editor. -->
    <div
      class="editors"
      role="group"
      bind:this={editorBox}
      onpaste={pasted}
      ondrop={dropped}
      ondragover={(e) => onattach && e.preventDefault()}
      {@attach mountEditor}></div>
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
  /* Pegada al interruptor de vista, y los dos al borde derecho. Con un
     `margin-right: auto` aquí y el `margin-left: auto` del otro, los dos
     empujaban en direcciones opuestas y el par acababa centrado. */
  .since { margin-left: 0.4rem; font-size: 0.76rem; }

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
  .vim:disabled { opacity: 0.5; }
  .hidden-file { display: none; }
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
