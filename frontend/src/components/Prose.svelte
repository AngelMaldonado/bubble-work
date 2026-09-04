<script lang="ts">
  // ONE renderer for every document in the app.
  //
  // A thread's document and a wiki page show the same thing — markdown the
  // server rendered — and in v0 they each owned a copy of the styling and the
  // post-processing. Copies agree until they don't: pages never got mermaid
  // diagrams or heading ids at all, because that code lived inside ThreadView.
  //
  // Content arrives as `html` — the server's render. Svelte's scoping puts the
  // class on the .prose element here and every rule below reaches into it with
  // :global, so nothing about the styling depends on who is calling.
  import type { Snippet } from 'svelte';
  import { enableCheckboxes, extractHeadings, renderMermaid, type Heading } from '../lib/prose';

  let {
    html = '',
    children,
    ticking = false,
    compact = false,
    onheadings,
    onrendered,
    onclick,
    key,
  }: {
    /** rendered HTML from the server. Ignored when `children` is given. */
    html?: string;
    children?: Snippet;
    /** a todo is being written: checkboxes show as busy rather than clickable */
    ticking?: boolean;
    /** drop the page-column sizing, for prose inside something small (a chat bubble) */
    compact?: boolean;
    onheadings?: (h: Heading[]) => void;
    /**
     * Fired once the document has actually settled — diagrams rendered, headings
     * read. A caller restoring a scroll position has to wait for this: a diagram
     * changes the page height, so restoring before it lands lands somewhere else.
     */
    onrendered?: () => void;
    onclick?: (e: MouseEvent) => void;
    /**
     * Re-run the pipeline when this changes. With `html` the content is its own
     * trigger, but a snippet is opaque from here, so a caller passing children
     * has to say when what is inside them changed.
     */
    key?: unknown;
  } = $props();

  let el = $state<HTMLElement | null>(null);

  // Diagrams and heading extraction both need the DOM {@html} just produced, so
  // this waits a frame rather than reading during the update.
  $effect(() => {
    void html;
    void key;
    const node = el;
    if (!node) return;
    requestAnimationFrame(() => {
      enableCheckboxes(node);
      void (async () => {
        await renderMermaid(node);
        onheadings?.(extractHeadings(node));
        onrendered?.();
      })();
    });
  });
</script>

<!-- The click is for the todo checkboxes inside, which are themselves real
     focusable controls — the handler on the article is delegation, not a
     keyboard-inaccessible affordance of its own. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<article class="prose" class:ticking class:compact bind:this={el} {onclick}>
  {#if children}{@render children()}{:else}{@html html}{/if}
</article>

<style>
  /* ---- prose: matches the mds tool's render (serif body, sans headings) ---- */
  .prose {
    --code-bg: color-mix(in oklab, var(--text) 7%, transparent);
    --sans:
      Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    max-width: 940px;
    margin: 0 auto;
    font-family: 'Iowan Old Style', 'Palatino Linotype', Palatino, ui-serif, Georgia, Cambria,
      'Times New Roman', serif;
    font-size: 18px;
    line-height: 1.72;
    color: var(--text);
    overflow-wrap: break-word;
  }
  .prose :global(> :first-child) {
    margin-top: 0;
  }
  .prose :global(h1),
  .prose :global(h2),
  .prose :global(h3),
  .prose :global(h4),
  .prose :global(h5),
  .prose :global(h6) {
    color: var(--text);
    font-family: var(--sans);
    font-weight: 750;
    line-height: 1.2;
    scroll-margin-top: 80px;
  }
  .prose :global(h1) {
    margin: 0 0 28px;
    font-size: clamp(2rem, 4vw, 3.1rem);
    letter-spacing: -0.04em;
  }
  .prose :global(h2) {
    margin: 2.3em 0 0.7em;
    padding-bottom: 0.35em;
    font-size: 1.75rem;
    border-bottom: 1px solid var(--line);
  }
  .prose :global(h3) {
    margin: 1.8em 0 0.6em;
    font-size: 1.35rem;
  }
  .prose :global(h4),
  .prose :global(h5),
  .prose :global(h6) {
    margin: 1.5em 0 0.5em;
  }
  .prose :global(p),
  .prose :global(ul),
  .prose :global(ol),
  .prose :global(blockquote),
  .prose :global(table) {
    margin: 1em 0;
  }
  .prose :global(ul),
  .prose :global(ol) {
    padding-left: 1.5em;
  }
  .prose :global(li) {
    margin: 0.3em 0;
  }
  .prose :global(a) {
    color: var(--wip);
    text-decoration: underline;
    text-decoration-thickness: 0.08em;
    text-underline-offset: 0.18em;
  }
  .prose :global(img) {
    display: block;
    max-width: 100%;
    height: auto;
    margin: 26px auto;
    border-radius: 10px;
    box-shadow: 0 18px 48px var(--shadow);
  }
  /* Plane description images can't be fetched via the API — link to Plane instead */
  .prose :global(a.plane-img) {
    display: inline-flex;
    align-items: center;
    gap: 0.4em;
    margin: 0.3em 0.4em 0.3em 0;
    padding: 0.25em 0.7em;
    border: 1px dashed var(--line);
    border-radius: 9px;
    color: var(--muted);
    text-decoration: none;
    font-family: var(--sans);
    font-size: 0.82em;
  }
  .prose :global(a.plane-img:hover) {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
    background: color-mix(in oklab, var(--wip) 8%, transparent);
  }
  /* Plane @mentions. Invisible until ARTIFACT-EDITING.md Phase 2 — a
     <mention-component> rendered to nothing at all — so this is the first time
     they show up in the interior. */
  .prose :global(span.plane-mention) {
    padding: 0.05em 0.4em;
    border-radius: 6px;
    font-family: var(--sans);
    font-size: 0.88em;
    font-weight: 600;
    color: var(--wip);
    background: color-mix(in oklab, var(--wip) 12%, transparent);
    white-space: nowrap;
  }
  .prose :global(blockquote) {
    margin-left: 0;
    padding: 0.25em 1.15em;
    color: var(--muted);
    border-left: 4px solid var(--wip);
    background: color-mix(in oklab, var(--text) 4%, transparent);
    border-radius: 0 10px 10px 0;
  }
  .prose :global(code) {
    padding: 0.15em 0.38em;
    border-radius: 5px;
    background: var(--code-bg);
    font: 0.86em ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .prose :global(pre) {
    overflow: auto;
    padding: 18px 20px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--code-bg);
    box-shadow: 0 18px 48px var(--shadow);
  }
  .prose :global(pre code) {
    padding: 0;
    background: transparent;
    font-size: 14px;
  }
  .prose :global(table) {
    display: block;
    width: max-content;
    max-width: 100%;
    overflow: auto;
    border-collapse: collapse;
    font-family: var(--sans);
    font-size: 15px;
  }
  .prose :global(th),
  .prose :global(td) {
    padding: 9px 13px;
    border: 1px solid var(--line);
  }
  .prose :global(th) {
    background: color-mix(in oklab, var(--text) 5%, transparent);
    text-align: left;
  }
  .prose :global(hr) {
    height: 1px;
    margin: 2.5em 0;
    background: var(--line);
    border: 0;
  }
  /* Task-list items: a HANGING INDENT, not a flex row.
     Flex was the obvious way to keep wrapped text from tucking back under the
     checkbox, and it was wrong: it makes every child of the <li> a flex item, so
     each text run, `code` span and **bold** run became its own box and the item
     stopped flowing as a sentence. Padding plus a negative margin gets the same
     hanging indent while leaving the content as ordinary inline text.
     1.55em = the checkbox's 1.05em width + its 0.5em right margin. */
  .prose :global(li:has(> input[type='checkbox'])) {
    list-style: none;
    padding-left: 1.55em;
  }
  .prose :global(li:has(> input[type='checkbox']) > input[type='checkbox']) {
    margin-left: -1.55em; /* pull the box out into the gutter it just made */
  }
  /* custom task-list checkboxes (ported from mds) */
  .prose :global(input[type='checkbox']) {
    display: inline-grid;
    width: 1.05em;
    height: 1.05em;
    margin: 0 0.5em 0 0;
    appearance: none;
    vertical-align: -0.12em;
    border: 1.5px solid color-mix(in oklab, var(--faint) 82%, var(--text));
    border-radius: 0.28em;
    background: color-mix(in oklab, var(--text) 6%, transparent);
    place-content: center;
  }
  .prose :global(input[type='checkbox']:checked) {
    border-color: var(--wip);
    background: var(--wip);
  }
  .prose :global(input[type='checkbox']:checked::before) {
    width: 0.5em;
    height: 0.28em;
    border-bottom: 0.14em solid white;
    border-left: 0.14em solid white;
    content: '';
    transform: translate(0, -0.06em) rotate(-45deg);
  }
  /* mermaid diagrams */
  .prose :global(.mermaid-diagram) {
    margin: 28px 0;
    padding: 20px;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--surface-solid);
    box-shadow: 0 18px 48px var(--shadow);
    text-align: center;
  }
  .prose :global(.mermaid-diagram svg) {
    max-width: 100%;
    height: auto;
  }
  .prose :global(.mermaid-error) {
    color: oklch(0.62 0.2 20);
    font-family: var(--sans);
  }

  /* a rendered todo is a control, not decoration (Phase 6) */
  .prose :global(input[type='checkbox']) {
    cursor: pointer;
    accent-color: var(--wip);
    width: 0.95em;
    height: 0.95em;
    margin-right: 0.35em;
    vertical-align: -0.08em;
  }
  .prose.ticking :global(input[type='checkbox']) {
    cursor: progress;
    opacity: 0.6;
  }

  /* inside a chat bubble: no page column, no serif display size */
  .prose.compact {
    max-width: none;
    margin: 0;
    font-size: 0.86rem;
    line-height: 1.5;
  }
  .prose.compact :global(p) {
    margin: 0.25rem 0;
  }
  .prose.compact :global(p:first-child) {
    margin-top: 0;
  }
  .prose.compact :global(p:last-child) {
    margin-bottom: 0;
  }

  @media (max-width: 640px) {
    .prose {
      font-size: 16px;
    }
  }
</style>
