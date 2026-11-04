<script lang="ts">
  import Loading from './Loading.svelte';
  import FormatBar from './FormatBar.svelte';
  import Icon from './Icon.svelte';
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
  import { droppedFile, insertAttachment, pastedImage } from '../lib/attach';
  import { annotate } from '../lib/annotate';
  import { acceptUploads } from '../lib/limits.svelte';
  import { onDestroy, untrack } from 'svelte';
  import Prose from './Prose.svelte';
  import SlashMenu from './SlashMenu.svelte';
  import ThreadToc from './ThreadToc.svelte';
  import { SlashMenu as SlashMenuState } from '../lib/slashmenu.svelte';
  import { vimPref } from '../lib/vim.svelte';
  import type { MarkdownEditor } from '../lib/editor';
  import type { Heading } from '../lib/prose';
  import { toggleMarkdownTodo } from '../lib/markdownTodo';
  import { rememberMarkdownScroll, scrollParent } from '../lib/markdownScroll';
  import { blocksOf, lineAt, topOf } from '../lib/docPosition';

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
    ontodo,
    scrollKey,
    autosave = true,
  }: {
    markdown?: string;
    scrollKey?: string;
    ontodo?: (index: number, done: boolean) => void | Promise<void>;
    html?: string;
    editing?: boolean;
    elsewhere?: boolean;
    /** Guardar. `false` (o un error) es «no se guardó»: el indicador lo dice
     *  en vez de enseñar un ✓ que no es verdad. */
    onsave?: (markdown: string) => void | boolean | Promise<void | boolean>;
    /** Guardar solo mientras se escribe, con el indicador. Falso donde guardar
     *  es un paso aparte (Prompts tiene su propio botón). */
    autosave?: boolean;
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

  // Lo último que se mandó a guardar. Cuando el servidor lo devuelve, es el eco
  // de este mismo editor y NO se vuelve a escribir en él: con autoguardado se
  // sigue tecleando mientras viaja, y reescribir el documento con lo que se
  // mandó borraba lo tecleado entre tanto.
  let sentText: string | null = null;
  $effect(() => {
    const m = markdown;
    untrack(() => {
      if (m === sentText) return;
      draft = m;
      // And into the editor, if one is open. The editor is built ONCE (it reads
      // `draft` untracked, or every keystroke would rebuild it), so without this
      // a reload after a conflict changed the document underneath and left the
      // old text on screen — the reload appeared to do nothing.
      editor?.setDoc(m);
      if (!inflight) saveState = 'saved';
    });
  });

  async function tickFromMarkdown(index: number, done: boolean) {
    await onsave?.(toggleMarkdownTodo(markdown, index, done));
  }

  // ---- guardar ----
  //
  // Solo, un momento después de dejar de teclear, y además con ⌘S, `:w`, al
  // salir del editor y al cambiar de vista. Uno a la vez: dos escrituras con la
  // misma base son un conflicto contra uno mismo. Lo que se escribe mientras
  // una viaja sale en la siguiente.
  //
  // El indicador dice lo que de verdad pasó: «Guardado» sólo cuando el dueño
  // contestó que sí. Antes, un guardado que no llegaba —en una tableta, por
  // ejemplo— no lo decía nadie, y el texto desaparecía al ir al renderizado.
  const AUTOSAVE_MS = 1200;
  const RETRY_MS = 5000;
  let saveState = $state<'saved' | 'dirty' | 'saving' | 'failed'>('saved');
  let inflight: Promise<void> | null = null;
  let again = false;
  let autoTimer: ReturnType<typeof setTimeout> | undefined;

  function schedule() {
    if (!autosave) return;
    clearTimeout(autoTimer);
    autoTimer = setTimeout(() => void save(), saveState === 'failed' ? RETRY_MS : AUTOSAVE_MS);
  }

  export function save(): Promise<void> {
    clearTimeout(autoTimer);
    // Lo que tiene el editor, no lo último que avisó: un teclado táctil puede
    // tener la palabra a medias todavía sin entregar.
    if (editor) draft = editor.value();
    if (inflight) { again = true; return inflight; }
    if (draft === markdown) { saveState = 'saved'; return Promise.resolve(); }
    const sent = draft;
    sentText = sent;
    saveState = 'saving';
    const run = (async () => {
      let ok = false;
      try { ok = (await onsave?.(sent)) !== false; } catch { ok = false; }
      inflight = null;
      if (!ok) { saveState = 'failed'; again = false; if (draft !== markdown) schedule(); return; }
      if (draft === sent) { saveState = 'saved'; again = false; return; }
      saveState = 'dirty';
      if (again) { again = false; void save(); } else schedule();
    })();
    inflight = run;
    return run;
  }
  onDestroy(() => {
    clearTimeout(autoTimer);
    // Irse a otra pantalla también es dejar de escribir.
    if (draft !== markdown && !inflight) void save();
  });

  // ---- el mismo sitio en las dos vistas ----
  //
  // Al pasar a renderizado se lleva la línea que estaba arriba del editor; al
  // volver, la que está arriba de lo leído. El cursor vuelve donde estaba si
  // no se movió la lectura; si se leyó otra parte, va al principio de lo que
  // se estaba leyendo — es ahí donde se quiere escribir.
  let bar = $state<HTMLElement | null>(null);
  let readBox = $state<HTMLElement | null>(null);
  let switched = $state(false);
  let editorLine: number | null = null; // arriba del editor al salir de él
  let readTarget: number | null = null; // a dónde llevar el renderizado
  let readShown: number | null = null; // dónde quedó al llevarlo
  let writeTarget: { line: number; caret: boolean } | null = null;

  /** El borde de arriba de lo que se lee: bajo la barra, que va fija. */
  function viewTop(of: HTMLElement): { scroller: HTMLElement; top: number } {
    const scroller = scrollParent(of);
    const top = scroller === document.scrollingElement ? 0 : scroller.getBoundingClientRect().top;
    return { scroller, top: Math.max(top, bar?.getBoundingClientRect().bottom ?? top) };
  }

  function placeRead() {
    if (readTarget === null || !readBox) return;
    const line = readTarget;
    readTarget = null;
    const { scroller, top } = viewTop(readBox);
    const y = topOf(blocksOf(readBox), line);
    if (y === null) return;
    scroller.scrollTop += y - top;
    readShown = lineAt(blocksOf(readBox), viewTop(readBox).top);
  }

  function toRead() {
    if (editor) editorLine = editor.topLine();
    void save();
    if (mode === 'write' && editorLine !== null) { readTarget = editorLine; switched = true; }
    mode = 'read';
  }

  function toWrite() {
    if (mode === 'read' && readBox) {
      const at = lineAt(blocksOf(readBox), viewTop(readBox).top);
      const still = at !== null && readShown !== null && Math.abs(at - readShown) < 0.5;
      if (at !== null && !still) writeTarget = { line: Math.floor(at), caret: true };
      else if (editorLine !== null) writeTarget = { line: editorLine, caret: false };
      else writeTarget = null;
    } else {
      writeTarget = editorLine !== null ? { line: editorLine, caret: false } : null;
    }
    if (writeTarget) switched = true;
    mode = 'write';
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
      // La misma inserción que el brief de una burbuja y el cuerpo de una nota:
      // una sola definición de «qué es una imagen pegada».
      const next = await insertAttachment(editor, file, onattach);
      if (next !== null) draft = next;
    } finally {
      attaching = false;
    }
  }

  /** Pasting a screenshot is the fastest path there is, and the one people try
   *  first. The clipboard carries the image as a file with no name worth using;
   *  the caller's slug decides what it is called on disk. */
  function pasted(e: ClipboardEvent) {
    const file = onattach ? pastedImage(e) : null;
    if (file) {
      // Antes que CodeMirror, y sin dejarle seguir: si la imagen viene como URL
      // base64, pegarla como texto es escribir la imagen entera dentro del
      // documento.
      e.preventDefault();
      e.stopPropagation();
      // Se pega después de anotarla, o no se pega si se canceló.
      annotate(file).then((out) => out && attach(out));
    }
  }

  function dropped(e: DragEvent) {
    const file = droppedFile(e);
    if (file) {
      e.preventDefault();
      // Sólo una imagen pasa por el modal de anotar. Un PDF no se anota ni se
      // recorta: se sube tal cual y se enlaza.
      if (file.type.startsWith('image/')) annotate(file).then((out) => out && attach(out));
      else attach(file);
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
    const identity = scrollKey || location.pathname;
    const doc = untrack(() => draft);
    let releaseScroll: (() => void) | undefined;

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
            if (next !== draft) {
              draft = next;
              if (!inflight && saveState !== 'failed') saveState = next === markdown ? 'saved' : 'dirty';
              if (next !== markdown) schedule();
            }
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
        const target = writeTarget;
        writeTarget = null;
        if (target) {
          // La página conserva el scroll de la lectura, y el editor (de alto
          // fijo) quedaría con su borde de arriba bajo la barra fija.
          const { scroller, top } = viewTop(el);
          scroller.scrollTop += el.getBoundingClientRect().top - top - 8;
          made_.revealLine(target.line, target.caret);
        }
        const content = el.querySelector<HTMLElement>('.cm-content');
        if (content) releaseScroll = rememberMarkdownScroll(content, identity + '.markdown', { restore: !target });
      });
    return () => {
      live = false;
      releaseScroll?.();
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
<!-- Fija arriba al hacer scroll: cambiar de vista, el indicador de guardado y
     el formato se alcanzan desde cualquier parte de un documento largo. -->
<div class="doc-bar" bind:this={bar}>
<div class="edit-bar">
  {#if autosave && (mode === 'write' || saveState !== 'saved')}
    <span class="autosave" data-state={saveState} role="status" aria-live="polite">
      {#if saveState === 'saving'}<Loading compact label="Guardando" /> Guardando…
      {:else if saveState === 'saved'}<Icon of="check" /> Guardado
      {:else if saveState === 'dirty'}<span class="dot" aria-hidden="true"></span> Sin guardar
      {:else}⚠ No se guardó <button type="button" class="link" onclick={() => save()}>reintentar</button>{/if}
    </span>
  {/if}
  <div class="seg" role="group" aria-label="modo de vista">
    <button
      class:on={mode === 'read'}
      aria-pressed={mode === 'read'}
      onclick={() => mode !== 'read' && toRead()}>renderizado</button>
    <button
      class:on={mode === 'write'}
      aria-pressed={mode === 'write'}
      onclick={() => mode !== 'write' && toWrite()}>markdown</button>
    {#if ondiff}
      <button
        class:on={mode === 'diff'}
        aria-pressed={mode === 'diff'}
        onclick={() => {
          if (editor) editorLine = editor.topLine();
          save();
          mode = 'diff';
        }}>cambios</button>
    {/if}
  </div>
  {#if editing && onattach}
    <!-- Adjuntar vive donde se escribe, porque lo que produce es una línea de
         markdown en el documento. Arrastrar y pegar hacen lo mismo; el botón
         está para quien no sabe que puede. -->
    <button class="vim" disabled={attaching} onclick={() => picker?.click()} title="adjuntar un archivo">
      {#if attaching}<Loading compact label="Subiendo archivo" />{:else}<Icon of="paperclip" />{/if}
    </button>
    <input
      class="hidden-file"
      type="file"
      accept={acceptUploads()}
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
{#if editing && mode === 'write'}<FormatBar {editor} />{/if}
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
      <Loading label="Leyendo cambios" />
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
      onpastecapture={pasted}
      ondrop={dropped}
      ondragover={(e) => onattach && e.preventDefault()}
      {@attach mountEditor}></div>
    <SlashMenu menu={slash} field={editorBox} onpick={(c) => slash.run(editor, draft, c)} />
  </div>
{:else}
  <div bind:this={readBox}>
    <Prose {html} {onheadings} {scrollKey} restore={!switched} onrendered={placeRead}
      ontodo={ontodo || (onsave ? tickFromMarkdown : undefined)} />
  </div>
{/if}

<style>
  .doc-bar {
    position: sticky;
    top: 0;
    z-index: 6;
    margin-bottom: 1rem;
    padding-block: 0.4rem;
  }
  .edit-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem; }
  .doc-bar > :global(* + *) { margin-top: 0.4rem; }
  /* El formato conserva su renglón, alineado a la derecha como la vista. */
  .doc-bar > :global(.format-bar) { justify-content: flex-end; }
  /* A la derecha, junto a las pestañas: el `margin-left: auto` lo empuja él y
     las pestañas van pegadas detrás. */
  .autosave { margin-left: auto; display: inline-flex; align-items: center; gap: 0.35rem; font-size: 0.76rem; color: var(--faint); }
  .autosave + .seg { margin-left: 0.4rem; }
  .autosave[data-state='saved'] { color: var(--muted); }
  .autosave[data-state='dirty'] { color: var(--muted); }
  .autosave[data-state='failed'] { color: var(--color-error-500); }
  .autosave .dot { width: 0.45rem; height: 0.45rem; border-radius: 999px; background: var(--warm, var(--accent)); }
  .autosave .link { border: 0; background: none; padding: 0; color: inherit; text-decoration: underline; cursor: pointer; }
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
    max-width: 1200px;
    margin-inline: auto;
  }
  .editors {
    height: calc(100dvh - var(--topbar-h) - 8rem);
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface-solid);
    overflow: hidden;
    /* CodeMirror numera sus capas para sí mismo —paneles 300, gutters 200,
       tooltips 500— y sin un contexto propio esos números compiten con los de
       la página: la barra `--INSERT--` salía por encima del modal de anotar
       una imagen. Aislado, cuentan sólo dentro del editor. */
    isolation: isolate;
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
