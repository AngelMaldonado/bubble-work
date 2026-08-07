<script lang="ts">
  import { slide } from 'svelte/transition';
  import { store, type ArtSel } from '../lib/store.svelte';
  import { pins, type Pin } from '../lib/pins.svelte';
  import { api, ApiError } from '../lib/api';
  import { t } from '../lib/i18n.svelte';
  import { levelIcon, levelLabel } from '../lib/types';
  import type { ThreadDetail, Comment } from '../lib/types';
  import ThreadToc, { type Heading } from './ThreadToc.svelte';

  type Sel = ArtSel;

  let detail = $state<ThreadDetail | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // The selected artifact is derived from the route (store.threadSel) so it's
  // deep-linkable and pin-able; a missing/stale selection falls back to the
  // thread's default. All changes go through store.setThreadSel.
  const sel = $derived.by<Sel>(() => {
    const d = detail;
    if (!d) return { kind: 'artifact', idx: 0 };
    const want = store.threadSel;
    if (want) {
      if (want.kind === 'artifact' && d.artifacts?.[want.idx]) return want;
      if (want.kind === 'logbook' && d.logbook) return want;
      if (want.kind === 'revision' && d.revisions?.[want.idx]) return want;
    }
    return firstSel(d);
  });

  function select(s: Sel): void {
    saveScroll(); // remember where we were before leaving this artifact
    store.setThreadSel(s);
  }

  let contentEl = $state<HTMLElement | null>(null);
  let headings = $state<Heading[]>([]);
  let sideCollapsed = $state(false);

  // ---- discussion (comments) chat ----
  let chatOpen = $state(false);
  let comments = $state<Comment[]>([]);
  let chatLoading = $state(false);
  let chatErr = $state<string | null>(null);
  let draft = $state('');
  let posting = $state(false);
  let chatScroll = $state<HTMLElement | null>(null);
  let composeEl = $state<HTMLTextAreaElement | null>(null);
  let commentsLoaded = $state(false);

  // ---- lightweight markdown formatting for the composer ----
  function restoreSel(el: HTMLTextAreaElement, start: number, end: number): void {
    requestAnimationFrame(() => {
      el.focus();
      el.selectionStart = start;
      el.selectionEnd = end;
    });
  }
  // wrap the selection (or a placeholder) in a marker, e.g. ** for bold.
  function wrapSel(marker: string, placeholder = 'text'): void {
    const el = composeEl;
    if (!el) return;
    const s = el.selectionStart;
    const e = el.selectionEnd;
    const sel = draft.slice(s, e) || placeholder;
    draft = draft.slice(0, s) + marker + sel + marker + draft.slice(e);
    restoreSel(el, s + marker.length, s + marker.length + sel.length);
  }
  // prefix each selected line, e.g. "- " for a bullet list.
  function prefixLines(prefix: string): void {
    const el = composeEl;
    if (!el) return;
    const s = el.selectionStart;
    const e = el.selectionEnd;
    const lineStart = draft.lastIndexOf('\n', s - 1) + 1;
    const block = draft.slice(lineStart, e);
    const replaced = block
      .split('\n')
      .map((l) => prefix + l)
      .join('\n');
    draft = draft.slice(0, lineStart) + replaced + draft.slice(e);
    restoreSel(el, lineStart, lineStart + replaced.length);
  }
  function insertLink(): void {
    const el = composeEl;
    if (!el) return;
    const s = el.selectionStart;
    const e = el.selectionEnd;
    const label = draft.slice(s, e) || 'link text';
    const snippet = `[${label}](url)`;
    draft = draft.slice(0, s) + snippet + draft.slice(e);
    const urlAt = s + snippet.indexOf('url');
    restoreSel(el, urlAt, urlAt + 3);
  }

  function scrollChatToBottom(): void {
    requestAnimationFrame(() => {
      if (chatScroll) chatScroll.scrollTop = chatScroll.scrollHeight;
    });
  }

  // Fetch only — no read-marking. Runs eagerly on thread open so the unread
  // badge is accurate before the panel is ever opened.
  async function loadComments(): Promise<void> {
    const tid = store.threadId;
    if (!tid) return;
    chatLoading = true;
    chatErr = null;
    try {
      comments = await api.comments(tid);
      commentsLoaded = true;
      if (chatOpen) scrollChatToBottom();
    } catch (e) {
      chatErr = e instanceof ApiError ? e.message : String(e);
    } finally {
      chatLoading = false;
    }
  }

  // Marks everyone else's *unread* comments as read (👀) — only when the panel
  // is opened. Your own comments are never marked. Reflect your eyes locally to
  // avoid a reload, which also clears the unread badge.
  async function markOthersRead(tid: string): Promise<void> {
    if (store.kiosk) return; // a read-only display doesn't leave read-receipts
    const others = comments
      .filter((c) => !isMine(c) && !(c.readers ?? []).some((r) => r.id === myId))
      .map((c) => c.id);
    if (others.length === 0) return;
    try {
      await api.markCommentsRead(tid, others);
    } catch {
      return; // best-effort — a failed receipt shouldn't disrupt reading
    }
    const meName = store.actor?.name ?? 'me';
    const seen = new Set(others);
    comments = comments.map((c) =>
      seen.has(c.id) ? { ...c, readers: [...(c.readers ?? []), { id: myId, name: meName }] } : c,
    );
  }

  async function toggleChat(): Promise<void> {
    chatOpen = !chatOpen;
    if (!chatOpen) return;
    if (!commentsLoaded && !chatLoading) await loadComments();
    scrollChatToBottom();
    const tid = store.threadId;
    if (tid) void markOthersRead(tid);
  }

  // A comment that could not reach Plane comes back as a draft (202) rather
  // than an error, so the words stay put. Only its author can re-send it,
  // because it deliberately carries no credential (docs/PLANE-SYNC.md Phase 5).
  let draftBusy = $state<number | null>(null);

  async function retryDraft(c: Comment): Promise<void> {
    const tid = store.threadId;
    if (!tid || !c.draft_id) return;
    draftBusy = c.draft_id;
    chatErr = null;
    try {
      const sent = await api.retryDraft(tid, c.draft_id);
      comments = comments.map((x) => (x.draft_id === c.draft_id ? sent : x));
      if (sent.pending) chatErr = sent.error || 'still unable to reach Plane';
    } catch (e) {
      chatErr = e instanceof ApiError ? e.message : String(e);
    } finally {
      draftBusy = null;
    }
  }

  async function discardDraft(c: Comment): Promise<void> {
    const tid = store.threadId;
    if (!tid || !c.draft_id) return;
    draftBusy = c.draft_id;
    try {
      await api.discardDraft(tid, c.draft_id);
      comments = comments.filter((x) => x.draft_id !== c.draft_id);
    } catch (e) {
      chatErr = e instanceof ApiError ? e.message : String(e);
    } finally {
      draftBusy = null;
    }
  }

  async function postComment(): Promise<void> {
    const tid = store.threadId;
    const body = draft.trim();
    if (!tid || !body || posting) return;
    posting = true;
    chatErr = null;
    try {
      const c = await api.postComment(tid, body);
      comments = [...comments, c];
      draft = '';
      scrollChatToBottom();
    } catch (e) {
      chatErr = e instanceof ApiError ? e.message : String(e);
    } finally {
      posting = false;
    }
  }

  // Enter sends; Shift+Enter inserts a newline.
  function onDraftKey(e: KeyboardEvent): void {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void postComment();
    }
  }

  // Compute ownership on the client from the logged-in identity so it's
  // consistent for freshly-posted and reloaded comments alike (the server's
  // per-comment flag can lag behind identity resolution).
  const myId = $derived(store.actor?.id ?? '');
  function isMine(c: Comment): boolean {
    return (!!c.author_id && c.author_id === myId) || c.mine;
  }
  // unread = others' comments I haven't read yet; drives the FAB badge and
  // clears to 0 (badge hidden) once opening the chat marks them read.
  const unread = $derived(
    comments.filter((c) => !isMine(c) && !(c.readers ?? []).some((r) => r.id === myId)).length,
  );

  function fmtTime(iso: string): string {
    const t = new Date(iso).getTime();
    if (!t) return '';
    const s = Math.round((Date.now() - t) / 1000);
    if (s < 60) return 'just now';
    const m = Math.round(s / 60);
    if (m < 60) return `${m}m`;
    const h = Math.round(m / 60);
    if (h < 24) return `${h}h`;
    const d = Math.round(h / 24);
    if (d < 7) return `${d}d`;
    return new Date(iso).toLocaleDateString();
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let mermaidAPI: any = null;
  let mmdSeq = 0;

  // Extract the rendered headings for the minimap.
  function extractHeadings(el: HTMLElement): void {
    const hs = [...el.querySelectorAll<HTMLElement>('.prose h1, .prose h2, .prose h3')].filter(
      (h) => h.id,
    );
    const levels = hs.map((h) => Number(h.tagName[1]));
    const min = levels.length ? Math.min(...levels) : 1;
    headings = hs.map((h) => ({
      id: h.id,
      // strip a leading emoji/symbol/checkbox (e.g. ⬜) so the TOC reads cleanly
      title: (h.textContent ?? '').replace(/^[^\p{L}\p{N}]+/u, '').trim(),
      depth: Number(h.tagName[1]) - min,
    }));
  }

  // Replace ```mermaid code blocks with rendered SVG diagrams (mds-style),
  // lazy-loading mermaid only when a diagram is present.
  async function renderMermaid(el: HTMLElement): Promise<void> {
    const blocks = [...el.querySelectorAll<HTMLElement>('pre > code.language-mermaid')];
    if (!blocks.length) return;
    mermaidAPI ||= (await import('mermaid')).default;
    const dark = document.documentElement.dataset.mode === 'dark';
    mermaidAPI.initialize({ startOnLoad: false, theme: dark ? 'dark' : 'default', securityLevel: 'strict' });
    for (const code of blocks) {
      const pre = code.parentElement;
      if (!pre) continue;
      const holder = document.createElement('div');
      holder.className = 'mermaid-diagram';
      try {
        const { svg } = await mermaidAPI.render(`mmd-${++mmdSeq}`, code.textContent ?? '');
        holder.innerHTML = svg;
      } catch (err) {
        holder.innerHTML = `<div class="mermaid-error">${String(err)}</div>`;
      }
      pre.replaceWith(holder);
    }
  }

  function firstSel(d: ThreadDetail): Sel {
    if (d.artifacts?.length) return { kind: 'artifact', idx: 0 };
    if (d.logbook) return { kind: 'logbook', idx: 0 };
    if (d.revisions?.length) return { kind: 'revision', idx: 0 };
    return { kind: 'artifact', idx: 0 };
  }

  $effect(() => {
    const tid = store.threadId;
    if (!tid) return;
    loading = true;
    error = null;
    detail = null;
    // reset the discussion for the new thread and eagerly fetch its comments so
    // the unread badge is correct before the panel is opened. Fetching does NOT
    // mark anything read — that happens only when the user opens the chat.
    comments = [];
    commentsLoaded = false;
    chatErr = null;
    void loadComments();
    api
      .thread(tid)
      .then((d) => {
        detail = d;
      })
      .catch((e) => (error = e instanceof ApiError ? e.message : String(e)))
      .finally(() => (loading = false));
  });

  const current = $derived.by(() => {
    if (!detail) return null;
    if (sel.kind === 'artifact') return detail.artifacts?.[sel.idx] ?? null;
    if (sel.kind === 'revision') return detail.revisions?.[sel.idx] ?? null;
    return null;
  });

  // ---- per-artifact scroll restoration (ported from the mds tool) ----
  // Remember the reading position per (thread, artifact) so switching artifacts
  // or returning to a thread lands where you left off. sessionStorage keeps it
  // across reloads in the same tab. Restored AFTER render settles (mermaid /
  // images) so the target offset is accurate.
  function scrollKey(): string | null {
    if (!store.threadId) return null;
    return `bubble.scroll|${store.threadId}|${sel.kind}${sel.idx}`;
  }
  function saveScroll(): void {
    const k = scrollKey();
    if (!k) return;
    try {
      sessionStorage.setItem(k, String(Math.round(window.scrollY)));
    } catch {
      /* best-effort */
    }
  }
  function savedScroll(): number {
    const k = scrollKey();
    if (!k) return 0;
    const raw = sessionStorage.getItem(k);
    const y = raw ? parseInt(raw, 10) : 0;
    return Number.isFinite(y) ? y : 0;
  }
  let scrollTimer: ReturnType<typeof setTimeout> | null = null;
  function onScroll(): void {
    if (scrollTimer) return; // throttle writes
    scrollTimer = setTimeout(() => {
      scrollTimer = null;
      saveScroll();
    }, 120);
  }

  // On content switch: render mermaid + rebuild the minimap, then restore the
  // saved scroll position for this artifact (default: top).
  $effect(() => {
    const html = current?.html; // dependency: re-run when the shown doc changes
    const el = contentEl;
    if (!el) return;
    const target = savedScroll(); // capture for THIS artifact before async work
    requestAnimationFrame(() => {
      if (!html) {
        headings = [];
        return;
      }
      void (async () => {
        await renderMermaid(el);
        extractHeadings(el);
        // restore now that layout (diagrams/images) has settled
        window.scrollTo({ top: target });
      })();
    });
  });

  // ---- pinned artifacts ----
  const currentPin = $derived.by<Pin | null>(() => {
    if (!detail) return null;
    const title = sel.kind === 'logbook' ? 'Logbook' : (current?.title ?? '');
    return { threadId: detail.id, threadTitle: detail.title, kind: sel.kind, idx: sel.idx, title };
  });
  const isPinned = $derived(currentPin ? pins.has(currentPin) : false);
  function togglePin(): void {
    if (currentPin) pins.toggle(currentPin);
  }
  function openPin(p: Pin): void {
    saveScroll();
    store.openThread(p.threadId, { kind: p.kind, idx: p.idx });
  }

  function close() {
    saveScroll();
    store.closeThread();
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }
</script>

<svelte:window onkeydown={onKey} onscroll={onScroll} />

<div class="screen" aria-label={t('thread.aria')}>
  <div class="topbar">
    <button class="back" onclick={close} aria-label={t('thread.back')}>
      <span aria-hidden="true">←</span> board
    </button>
    {#if detail}
      <span class="seq">#{detail.seq}</span>
      <h2 class="ttl">{detail.title}</h2>
      <div class="chips">
        <span class="chip {detail.kind}">{detail.kind}</span>
        <!-- the thread's own buoyancy (THREAD-LIFECYCLE.md): subsumes open/done -->
        <span class="chip lvl lvl-{detail.level}" title={detail.reason}>
          {levelIcon(detail.level)} {levelLabel(detail.level)}
        </span>
        {#if detail.state}
          <span class="chip who" title="Plane state ({detail.state_group})">{detail.state}</span>
        {/if}
        {#if detail.priority && detail.priority !== 'none'}
          <span class="chip">{detail.priority}</span>
        {/if}
        {#if detail.assignees?.length}
          <span class="chip who">{detail.assignees.join(', ')}</span>
        {/if}
      </div>
      <button
        class="pin-btn"
        class:on={isPinned}
        onclick={togglePin}
        title={isPinned ? 'unpin this artifact' : 'pin this artifact'}
        aria-label={isPinned ? 'unpin this artifact' : 'pin this artifact'}
        aria-pressed={isPinned}
      >
        <span aria-hidden="true">📌</span>
      </button>
    {/if}
  </div>

  {#if loading}
    <p class="dim pad">{t('thread.loading')}</p>
  {:else if error}
    <p class="err pad">{error}</p>
  {:else if detail}
    <div class="body">
      <nav class="side" class:collapsed={sideCollapsed}>
        <div class="side-head">
          {#if !sideCollapsed}<span class="side-title">{t('thread.contents')}</span>{/if}
          <button
            class="side-toggle"
            onclick={() => (sideCollapsed = !sideCollapsed)}
            aria-label={sideCollapsed ? 'expand sidebar' : 'collapse sidebar'}
            title={sideCollapsed ? 'expand' : 'collapse'}
          >
            {sideCollapsed ? '☰' : '‹'}
          </button>
        </div>

        {#if !sideCollapsed}
          <div class="side-scroll" transition:slide={{ duration: 220, axis: 'y' }}>
            {#if pins.items.length}
              <div class="sect">📌 Pinned</div>
              {#each pins.items as p (p.threadId + p.kind + p.idx)}
                <div class="pin-row">
                  <button
                    class="item pin-item"
                    class:active={p.threadId === detail.id &&
                      p.kind === sel.kind &&
                      p.idx === sel.idx}
                    onclick={() => openPin(p)}
                    title={p.threadTitle + ' · ' + p.title}
                  >
                    <span class="ico">{p.kind === 'logbook' ? '✅' : p.kind === 'revision' ? '📝' : '📄'}</span>
                    <span class="pin-label">
                      {p.title || 'Untitled'}
                      {#if p.threadId !== detail.id}<span class="pin-thread">· {p.threadTitle}</span>{/if}
                    </span>
                  </button>
                  <button
                    class="pin-x"
                    onclick={() => pins.remove(p)}
                    title={t('thread.unpin')}
                    aria-label="unpin {p.title}">×</button
                  >
                </div>
              {/each}
            {/if}

            {#if detail.artifacts?.length}
              <div class="sect">{t('thread.work')}</div>
              {#each detail.artifacts as a, i (i)}
                <button
                  class="item"
                  class:active={sel.kind === 'artifact' && sel.idx === i}
                  onclick={() => select({ kind: 'artifact', idx: i })}
                >
                  <span class="ico">📄</span>{a.title}
                </button>
              {/each}
            {/if}

            {#if detail.logbook}
              <div class="sect">{t('thread.logbook')}</div>
              <button
                class="item"
                class:active={sel.kind === 'logbook'}
                onclick={() => select({ kind: 'logbook', idx: 0 })}
              >
                <span class="ico">✅</span>Tasks{detail.kind === 'phased' ? ' · phased' : ''}
              </button>
            {/if}

            {#if detail.revisions?.length}
              <div class="sect">{t('thread.revisions')}</div>
              {#each detail.revisions as r, i (i)}
                <button
                  class="item"
                  class:active={sel.kind === 'revision' && sel.idx === i}
                  onclick={() => select({ kind: 'revision', idx: i })}
                >
                  <span class="ico">📝</span>{r.title}
                </button>
              {/each}
            {/if}
          </div>
        {/if}
      </nav>

      <main class="content" bind:this={contentEl}>
        {#if sel.kind === 'logbook' && detail.logbook}
          <!-- rendered through the same goldmark/prose pipeline as the rest -->
          <article class="prose">
            <h1>{t('thread.logbook')}</h1>
            {#if detail.logbook.html}
              {@html detail.logbook.html}
            {:else}
              <p class="dim">{t('thread.noTasks')}</p>
            {/if}
            {#if detail.logbook.dod_html}
              <h2>{t('thread.dod')}</h2>
              {@html detail.logbook.dod_html}
            {/if}
          </article>
        {:else if current}
          <article class="prose">
            <!-- server-rendered goldmark HTML (safe: raw HTML is escaped) -->
            {@html current.html}
          </article>
        {:else}
          <p class="dim">{t('thread.nothing')}</p>
        {/if}
      </main>

      {#if headings.length}
        <ThreadToc {headings} />
      {/if}
    </div>

    <!-- bottom-right discussion chat (Plane work-item comments) -->
    <div class="chat">
      {#if chatOpen}
        <div class="chat-panel" transition:slide={{ duration: 180, axis: 'y' }}>
          <header class="chat-head">
            <span class="chat-title">{t('chat.title')}</span>
            <button class="chat-x" onclick={toggleChat} aria-label={t('thread.closeChat')}>×</button>
          </header>
          <div class="chat-scroll" bind:this={chatScroll}>
            {#if chatLoading}
              <p class="dim chat-empty">{t('board.loading')}</p>
            {:else if comments.length === 0}
              <p class="dim chat-empty">{t('chat.empty')}</p>
            {:else}
              {#each comments as c (c.id)}
                <div class="msg" class:mine={isMine(c)} class:unsent={c.pending}>
                  <div class="msg-meta">
                    <span class="msg-who">{c.author}</span>
                    {#if c.pending}
                      <span class="msg-unsent" title={c.error}>⧗ {t('chat.unsent')}</span>
                    {:else}
                      <span class="msg-age">{fmtTime(c.created_at)}</span>
                    {/if}
                  </div>
                  <!-- server-rendered goldmark HTML (raw HTML escaped upstream) -->
                  <div class="msg-body prose">{@html c.html}</div>
                  {#if c.pending}
                    <!-- A draft carries no credential, so it can only be re-sent
                         from here, with the live session — which is also what
                         makes Plane record the right author. -->
                    <div class="msg-draft">
                      <span class="draft-why">{c.error || t('chat.unreachable')}</span>
                      <button onclick={() => retryDraft(c)} disabled={draftBusy === c.draft_id}>
                        {draftBusy === c.draft_id ? '…' : t('chat.retry')}
                      </button>
                      <button class="link" onclick={() => discardDraft(c)} disabled={draftBusy === c.draft_id}>
                        {t('chat.discard')}
                      </button>
                    </div>
                  {/if}
                  {#if c.readers && c.readers.length}
                    <div class="msg-seen" title={c.readers.map((r) => r.name).join(', ')}>
                      <span aria-hidden="true">👀</span>
                      {c.readers.map((r) => r.name).join(', ')}
                    </div>
                  {/if}
                </div>
              {/each}
            {/if}
          </div>
          {#if chatErr}<p class="err chat-err">{chatErr}</p>{/if}
          {#if store.kiosk}
            <p class="dim chat-readonly">{t('chat.readonly')}</p>
          {:else}
          <div class="chat-compose">
            <div class="fmt-bar">
              <!-- onmousedown+preventDefault keeps the textarea selection intact -->
              <button
                type="button"
                title={t('fmt.bold')}
                onmousedown={(e) => {
                  e.preventDefault();
                  wrapSel('**');
                }}><b>B</b></button
              >
              <button
                type="button"
                title={t('fmt.italic')}
                onmousedown={(e) => {
                  e.preventDefault();
                  wrapSel('*');
                }}><i>I</i></button
              >
              <button
                type="button"
                title={t('fmt.code')}
                onmousedown={(e) => {
                  e.preventDefault();
                  wrapSel('`', 'code');
                }}>{'</>'}</button
              >
              <button
                type="button"
                title={t('fmt.link')}
                onmousedown={(e) => {
                  e.preventDefault();
                  insertLink();
                }}>🔗</button
              >
              <button
                type="button"
                title={t('fmt.list')}
                onmousedown={(e) => {
                  e.preventDefault();
                  prefixLines('- ');
                }}>≡</button
              >
            </div>
            <div class="compose-row">
              <textarea
                bind:this={composeEl}
                bind:value={draft}
                onkeydown={onDraftKey}
                placeholder={t('chat.placeholder')}
                rows="2"
              ></textarea>
              <button class="chat-send" onclick={postComment} disabled={posting || !draft.trim()}>
                {posting ? '…' : t('chat.send')}
              </button>
            </div>
          </div>
          {/if}
        </div>
      {:else}
        <button class="chat-fab" onclick={toggleChat} aria-label={t('thread.openChat')}>
          <span aria-hidden="true">💬</span>
          {#if unread}<span class="chat-badge">{unread}</span>{/if}
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  /* the page (window) scrolls — like the board — so the sticky top bar reliably
     blurs the content passing under it. */
  .screen {
    --topbar-h: 46px;
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    background: transparent; /* let the body's noise + gradient show through */
  }
  /* identical to the board's .statusbar (which blurs correctly), just at the top */
  .topbar {
    position: sticky;
    top: 0;
    z-index: 25;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.55rem 1.25rem;
    font-size: 0.75rem;
    color: var(--faint);
    background: color-mix(in oklab, var(--bg) 72%, transparent);
    backdrop-filter: blur(14px);
    border-bottom: 1px solid var(--line);
  }
  .back {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.7rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.75rem;
  }
  .back:hover {
    color: var(--text);
    background: var(--hover);
  }
  .pin-btn {
    flex: none;
    display: inline-grid;
    place-content: center;
    width: 1.9rem;
    height: 1.9rem;
    border-radius: 999px;
    border: 1px solid transparent;
    background: transparent;
    cursor: pointer;
    font-size: 0.9rem;
    filter: grayscale(1) opacity(0.55);
  }
  .pin-btn:hover {
    background: var(--hover);
    filter: grayscale(0.4) opacity(0.9);
  }
  .pin-btn.on {
    filter: none;
    background: color-mix(in oklab, var(--wip) 18%, transparent);
    border-color: color-mix(in oklab, var(--wip) 35%, transparent);
  }
  .pad {
    padding: 2rem 1.5rem;
  }
  /* title lives inline in the top bar, next to the back button */
  .seq {
    flex: none;
    font-size: 0.78rem;
    font-weight: 700;
    color: var(--faint);
    font-variant-numeric: tabular-nums;
  }
  .ttl {
    flex: 0 1 auto;
    min-width: 0;
    margin: 0;
    font-size: 0.95rem;
    font-weight: 700;
    letter-spacing: -0.01em;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chips {
    display: flex;
    flex-wrap: nowrap;
    gap: 0.35rem;
    margin-left: auto;
    flex: none;
  }
  .chip {
    font-size: 0.68rem;
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    color: var(--muted);
    text-transform: lowercase;
  }
  .chip.phased {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 45%, transparent);
  }
  .chip.lvl {
    text-transform: none;
    font-weight: 600;
  }
  .chip.lvl-in_progress {
    color: var(--wip);
    border-color: color-mix(in oklab, var(--wip) 45%, transparent);
  }
  .chip.lvl-reviewed {
    color: var(--reviewed);
    border-color: color-mix(in oklab, var(--reviewed) 45%, transparent);
  }
  .chip.lvl-zzzz {
    color: var(--zzzz);
    border-color: color-mix(in oklab, var(--zzzz) 45%, transparent);
  }
  .chip.lvl-rip {
    color: var(--rip);
  }
  .chip.lvl-done {
    color: var(--done);
    border-color: color-mix(in oklab, var(--done) 45%, transparent);
  }
  .chip.who {
    text-transform: none;
  }

  .body {
    flex: 1;
    display: flex;
    align-items: flex-start;
  }
  /* file tree: a padded rounded card, kept in view (sticky) while the page scrolls */
  .side {
    position: sticky;
    top: calc(var(--topbar-h) + 0.8rem);
    flex: 0 0 236px;
    margin: 0.8rem 0 1rem 1rem;
    padding: 0.5rem;
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - var(--topbar-h) - 1.6rem);
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: color-mix(in oklab, var(--surface-solid) 60%, transparent);
    transition:
      flex-basis 0.24s ease,
      padding 0.24s ease;
  }
  /* collapsed → shrinks to just the toggle (width via flex-basis, height because
     the scroll slides out and the card no longer stretches to full height) */
  .side.collapsed {
    flex-basis: 46px;
    align-self: flex-start;
    padding: 0.4rem;
  }
  .side-head {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.15rem 0.4rem 0.35rem;
  }
  .side.collapsed .side-head {
    justify-content: center;
    padding: 0;
  }
  .side-title {
    flex: 1;
    min-width: 0;
    font-size: 0.66rem;
    font-weight: 800;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--faint);
    white-space: nowrap;
    overflow: hidden;
  }
  .side-toggle {
    flex: none;
    width: 28px;
    height: 28px;
    display: grid;
    place-content: center;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--surface);
    color: var(--muted);
    cursor: pointer;
    font-size: 0.9rem;
    line-height: 1;
  }
  .side-toggle:hover {
    color: var(--text);
    background: var(--hover);
  }
  /* fixed width so items don't reflow while the card slides — they just clip */
  .side-scroll {
    flex: 1 1 auto;
    min-height: 0;
    width: 216px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0 0.1rem 0.25rem;
  }
  .sect {
    font-size: 0.66rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--faint);
    margin: 0.6rem 0.4rem 0.25rem;
  }
  .sect:first-child {
    margin-top: 0;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    width: 100%;
    text-align: left;
    padding: 0.4rem 0.5rem;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    font-size: 0.82rem;
    line-height: 1.2;
  }
  .item:hover {
    background: var(--hover);
    color: var(--text);
  }
  .item.active {
    background: color-mix(in oklab, var(--wip) 16%, transparent);
    color: var(--text);
    font-weight: 600;
  }
  .ico {
    font-size: 0.8rem;
    flex: none;
  }

  /* pinned artifacts rail */
  .pin-row {
    display: flex;
    align-items: center;
  }
  .pin-item {
    min-width: 0;
  }
  .pin-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .pin-thread {
    color: var(--faint);
    font-weight: 400;
  }
  .pin-x {
    flex: none;
    border: none;
    background: none;
    color: var(--faint);
    font-size: 1rem;
    line-height: 1;
    cursor: pointer;
    padding: 0 0.35rem;
    opacity: 0;
  }
  .pin-row:hover .pin-x {
    opacity: 1;
  }
  .pin-x:hover {
    color: var(--text);
  }

  .content {
    flex: 1;
    min-width: 0;
    padding: 2.4rem clamp(2.75rem, 5vw, 4rem) 6rem clamp(1.5rem, 4vw, 3.5rem);
  }
  .dim {
    color: var(--faint);
  }
  .err {
    color: oklch(0.68 0.19 25);
  }

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
  /* task-list items: flex row so the checkbox sits on the first line and wrapped
     text hangs under the text (not back under the checkbox) */
  .prose :global(li:has(> input[type='checkbox'])) {
    list-style: none;
    display: flex;
    align-items: flex-start;
  }
  .prose :global(li:has(> input[type='checkbox']) > input[type='checkbox']) {
    flex: none;
    margin-top: 0.34em; /* drop the box onto the first text line */
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

  @media (max-width: 640px) {
    .side {
      flex-basis: 150px;
    }
    .content {
      padding: 1.4rem 1.1rem 3rem;
    }
    .prose {
      font-size: 16px;
    }
  }

  /* ---- discussion chat (bottom-right) ---- */
  .chat {
    position: fixed;
    right: 1.25rem;
    bottom: 1.25rem;
    z-index: 40;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
  }
  .chat-fab {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    width: 3rem;
    height: 3rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--surface-solid);
    box-shadow: var(--shadow-strong, 0 6px 24px rgba(0, 0, 0, 0.18));
    font-size: 1.25rem;
    cursor: pointer;
    position: relative;
  }
  .chat-fab:hover {
    background: var(--hover);
  }
  .chat-badge {
    position: absolute;
    top: -4px;
    right: -4px;
    min-width: 1.1rem;
    height: 1.1rem;
    padding: 0 0.3rem;
    border-radius: 999px;
    background: var(--wip);
    color: white;
    font-size: 0.62rem;
    font-weight: 700;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-variant-numeric: tabular-nums;
  }
  .chat-panel {
    display: flex;
    flex-direction: column;
    width: min(360px, calc(100vw - 2.5rem));
    height: min(60vh, 560px);
    /* solid fill: a fixed panel under the app stacking context can't sample the
       page for backdrop-filter (same limitation as the minimaps) */
    background: var(--surface-solid);
    border: 1px solid var(--line);
    border-radius: 14px;
    box-shadow: var(--shadow-strong, 0 12px 40px rgba(0, 0, 0, 0.28));
    overflow: hidden;
  }
  .chat-head {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.6rem 0.85rem;
    border-bottom: 1px solid var(--line);
  }
  .chat-title {
    font-family: var(--sans);
    font-size: 0.82rem;
    font-weight: 700;
    color: var(--text);
  }
  .chat-x {
    border: none;
    background: none;
    color: var(--faint);
    font-size: 1.2rem;
    line-height: 1;
    cursor: pointer;
    padding: 0 0.2rem;
  }
  .chat-x:hover {
    color: var(--text);
  }
  .chat-scroll {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }
  .chat-empty {
    margin: auto;
    text-align: center;
    font-size: 0.82rem;
  }
  .msg {
    max-width: 85%;
    align-self: flex-start;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .msg.mine {
    align-self: flex-end;
    align-items: flex-end;
  }
  .msg-meta {
    display: flex;
    gap: 0.4rem;
    font-size: 0.66rem;
    color: var(--faint);
  }
  .msg-who {
    font-weight: 700;
    color: var(--muted);
  }
  .msg-body {
    padding: 0.4rem 0.65rem;
    border-radius: 12px;
    background: var(--hover);
    border: 1px solid var(--line);
    font-size: 0.85rem;
    /* break long unbreakable tokens (URLs) instead of overflowing */
    overflow-wrap: anywhere;
    word-break: break-word;
    min-width: 0;
  }
  .msg-body :global(a) {
    overflow-wrap: anywhere;
  }
  .msg-body :global(pre) {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .msg-body :global(pre),
  .msg-body :global(code) {
    overflow-wrap: anywhere;
    word-break: break-word;
  }
  .msg-body :global(img) {
    max-width: 100%;
    height: auto;
  }
  .msg.mine .msg-body {
    background: color-mix(in oklab, var(--wip) 18%, var(--surface-solid));
    border-color: color-mix(in oklab, var(--wip) 30%, transparent);
  }
  /* an unsent draft: present in the thread, visibly not yet real */
  .msg.unsent {
    opacity: 0.75;
    border-left: 2px dashed var(--warn, #d97706);
    padding-left: 0.5rem;
  }
  .msg-unsent {
    color: var(--warn, #d97706);
    font-size: 0.7rem;
    font-weight: 700;
  }
  .msg-draft {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: 0.3rem;
    font-size: 0.7rem;
    color: var(--faint);
  }
  .draft-why {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .msg-draft button.link {
    background: none;
    border: none;
    color: var(--faint);
    text-decoration: underline;
    cursor: pointer;
    padding: 0;
    font-size: inherit;
  }

  .msg-seen {
    display: flex;
    gap: 0.3rem;
    align-items: center;
    max-width: 100%;
    font-size: 0.64rem;
    color: var(--faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* tighten prose inside a chat bubble */
  .msg-body :global(p) {
    margin: 0.25rem 0;
  }
  .msg-body :global(p:first-child) {
    margin-top: 0;
  }
  .msg-body :global(p:last-child) {
    margin-bottom: 0;
  }
  .chat-err {
    flex: none;
    margin: 0;
    padding: 0.3rem 0.85rem;
    font-size: 0.72rem;
  }
  .chat-readonly {
    flex: none;
    margin: 0;
    padding: 0.7rem 0.85rem;
    border-top: 1px solid var(--line);
    font-size: 0.78rem;
    text-align: center;
  }
  .chat-compose {
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.6rem;
    border-top: 1px solid var(--line);
  }
  .fmt-bar {
    display: flex;
    gap: 0.15rem;
  }
  .fmt-bar button {
    min-width: 1.9rem;
    height: 1.9rem;
    padding: 0 0.4rem;
    border-radius: 7px;
    border: 1px solid transparent;
    background: none;
    color: var(--muted);
    font-size: 0.82rem;
    cursor: pointer;
  }
  .fmt-bar button:hover {
    background: var(--hover);
    color: var(--text);
  }
  .compose-row {
    display: flex;
    gap: 0.5rem;
    align-items: flex-end;
  }
  .chat-compose textarea {
    flex: 1;
    resize: none;
    max-height: 9rem;
    min-height: 3.4rem;
    padding: 0.5rem 0.65rem;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text);
    font-family: var(--sans);
    font-size: 0.9rem;
    line-height: 1.4;
  }
  .chat-compose textarea:focus {
    outline: none;
    border-color: color-mix(in oklab, var(--wip) 45%, var(--line));
  }
  .chat-send {
    flex: none;
    padding: 0.45rem 0.9rem;
    border-radius: 10px;
    border: 1px solid transparent;
    background: var(--wip);
    color: white;
    font-weight: 700;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .chat-send:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
