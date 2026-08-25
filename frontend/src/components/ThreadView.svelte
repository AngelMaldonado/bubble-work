<script lang="ts">
  import { untrack } from 'svelte';
  import { slide } from 'svelte/transition';
  import { store, type ArtSel } from '../lib/store.svelte';
  import { pins, type Pin } from '../lib/pins.svelte';
  import { api, ApiError } from '../lib/api';
  import { t } from '../lib/i18n.svelte';
  import { levelIcon, levelLabel } from '../lib/types';
  import type { ThreadDetail, Comment, RegionName, ThreadHit } from '../lib/types';
  import ThreadToc from './ThreadToc.svelte';
  import Prose from './Prose.svelte';
  import type { Heading } from '../lib/prose';
  import ArtifactEditor from './ArtifactEditor.svelte';
  import ConfirmDelete from './ConfirmDelete.svelte';
  import MoveThread from './MoveThread.svelte';

  type Sel = ArtSel;

  let detail = $state<ThreadDetail | null>(null);
  // re-homing this thread into another bubble
  let moving = $state(false);
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
    editing = false; // a different artifact is a different buffer
    store.setThreadSel(s);
  }

  // ---- editing (docs/journal/ARTIFACT-EDITING.md Phase 4) ----
  //
  // What the board calls an artifact and what the write path calls a region are
  // not the same shape: artifacts[0] is everything that is not the Logbook or
  // the DoD, which the API names "brief". Revisions are separate work items and
  // have no region at all, so they are read-only here.
  let editing = $state(false);
  const editRegion = $derived<RegionName | null>(
    sel.kind === 'artifact' && sel.idx === 0 ? 'brief' : sel.kind === 'logbook' ? 'logbook' : null,
  );
  const canEdit = $derived(!!editRegion && !!detail?.regions?.[editRegion]);

  // The open thread must never be repainted out from under a dirty buffer, so
  // reloadDetail asks the editors first (Phase 7).
  let editors = $state<Record<string, { isDirty: () => boolean } | null>>({});
  function anyDirty(): boolean {
    return Object.values(editors).some((e) => e?.isDirty());
  }

  // Set when a change arrived that we refused to apply because a buffer was
  // dirty. Cleared by taking theirs, or by the next clean reload.
  let elsewhere = $state(false);

  async function takeTheirs(): Promise<void> {
    elsewhere = false;
    editors = {};
    await reloadDetail();
  }

  // ---- inline checkboxes (docs/journal/ARTIFACT-EDITING.md Phase 6) ----
  //
  // Tick a todo straight from the rendered view. The server primitive shipped
  // with the write path; all this has to do is work out WHICH item was clicked
  // and hand over its text, because the server refuses an index whose text no
  // longer matches rather than ticking the wrong box.
  let ticking = $state<string | null>(null);

  async function onProseClick(e: MouseEvent): Promise<void> {
    const el = e.target as HTMLInputElement | null;
    if (!el || el.tagName !== 'INPUT' || el.type !== 'checkbox') return;
    if (store.kiosk || !detail || ticking) return;

    // Which list owns this box? The Logbook, the DoD and the document are
    // numbered independently — and a REVISION is a different work item
    // altogether, so the wrapper names the thread as well as the region.
    const wrap = el.closest('[data-region]') as HTMLElement | null;
    const region = wrap?.dataset.region as RegionName | undefined;
    if (!wrap || !region) return;
    const target = wrap.dataset.thread || detail.id;

    const boxes = [...wrap.querySelectorAll('input[type=checkbox]')];
    const index = boxes.indexOf(el);
    if (index < 0) return;

    // The item's own text, read off the DOM the same way a person reads it.
    const text = (el.closest('li')?.textContent ?? '').trim();

    // The browser flips `checked` BEFORE dispatching click, so this is already
    // the state being asked for — negating it sent the exact opposite, which is
    // why ticking a box appeared to do nothing. preventDefault then puts the
    // box back, so the server's answer is what actually lands.
    const done = el.checked;
    e.preventDefault();

    ticking = `${target}:${region}:${index}`;
    try {
      const fresh = await api.toggleTodo(target, region, index, text, done);
      // A revision's detail is its own, not this thread's — adopting it would
      // navigate the reader into the revision they just ticked a box in.
      if (target === detail.id) {
        detail = fresh;
      } else {
        await reloadDetail();
      }
    } catch (err) {
      // A 409 means the list moved under us — the honest answer is to show what
      // is actually there rather than guess which item was meant.
      error = err instanceof ApiError && err.status === 409 ? t('editor.todoMoved') : String(err);
      await reloadDetail();
    } finally {
      ticking = null;
    }
  }

  // ---- deleting (docs/journal/ARTIFACT-EDITING.md) ----
  //
  // Irreversible, and it deletes from Plane. The menu only ARMS it; the dialog
  // is the act, and it says what goes and what stays rather than "are you sure".
  type Pending = { kind: 'thread' } | { kind: 'region'; region: RegionName };
  let pending = $state<Pending | null>(null);
  let deleting = $state(false);

  const pendingWhat = $derived(
    !pending || !detail
      ? ''
      : pending.kind === 'thread'
        ? detail.title
        : pending.region === 'logbook'
          ? t('thread.logbook')
          : pending.region === 'dod'
            ? t('thread.dod')
            : detail.title,
  );

  async function confirmDelete(): Promise<void> {
    const p = pending;
    if (!p || !detail) return;
    deleting = true;
    try {
      if (p.kind === 'thread') {
        await api.deleteThread(detail.id);
        pending = null;
        close(); // the thread is gone — there is nothing left to look at
        await store.refresh();
      } else {
        detail = await api.deleteRegion(detail.id, p.region);
        pending = null;
        editing = false;
      }
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e);
    } finally {
      deleting = false;
    }
  }

  // ---- renaming ----
  //
  // A title is the work item's Plane `name`, which is why it is edited here
  // rather than in the markdown editor: it is not part of the body at all, and
  // splicing has nothing to do with it.
  let titleEl = $state<HTMLElement | null>(null);
  let titleWas = $state('');

  function onTitleKey(e: KeyboardEvent): void {
    if (e.key === 'Enter') {
      e.preventDefault();
      titleEl?.blur(); // commits
    } else if (e.key === 'Escape') {
      e.preventDefault();
      if (titleEl) titleEl.textContent = titleWas; // put it back, then leave
      titleEl?.blur();
    }
  }

  async function commitTitle(): Promise<void> {
    const el = titleEl;
    if (!el || !detail) return;
    const next = (el.textContent ?? '').replace(/\s+/g, ' ').trim();
    if (!next) {
      el.textContent = titleWas; // an empty title is not a rename, it is a slip
      return;
    }
    if (next === detail.title) return;
    try {
      detail = await api.updateThread(detail.id, { title: next });
    } catch (err) {
      el.textContent = titleWas;
      error = err instanceof ApiError ? err.message : String(err);
    }
  }

  // ---- finishing ----
  //
  // 🏆 is Plane's state, so this asks the server to move the work item rather than
  // recording anything of its own. The Definition of Done no longer gates it
  // (docs/decisions/0005): finishing is a position somebody takes. What comes back
  // is a REPORT — the items still open when they took it — which is what dodNote
  // shows, once, after the fact.
  let completing = $state(false);
  let dodNote = $state<string[] | null>(null);

  async function completeThread(): Promise<void> {
    if (!detail || completing) return;
    completing = true;
    try {
      detail = await api.completeThread(detail.id);
      dodNote = detail.unmet_dod?.length ? detail.unmet_dod : null;
      // The bubble's band is derived from its threads, so the board behind this
      // is now stale.
      await store.refresh();
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e);
    } finally {
      completing = false;
    }
  }

  // ---- links (docs/decisions/0006) ----
  //
  // External evidence lives in Plane as a link on the work item, not as a line of
  // prose somebody typed into the document. Landing one is production, so the board
  // behind this goes stale the moment it lands.
  let addingLink = $state(false);
  let linkURL = $state('');
  let linkTitle = $state('');

  async function submitLink(e: Event): Promise<void> {
    e.preventDefault();
    if (!detail || linkURL.trim() === '') return;
    try {
      detail = await api.addLink(detail.id, linkURL.trim(), linkTitle.trim());
      linkURL = '';
      linkTitle = '';
      addingLink = false;
      await store.refresh();
    } catch (err) {
      error = err instanceof ApiError ? err.message : String(err);
    }
  }

  async function dropLink(linkId: string): Promise<void> {
    if (!detail) return;
    try {
      detail = await api.removeLink(detail.id, linkId);
    } catch (err) {
      error = err instanceof ApiError ? err.message : String(err);
    }
  }

  // ---- labels (docs/decisions/0006) ----
  //
  // Plane's labels, edited as a comma-separated list because that is how somebody
  // thinks about them ("bug, infra") — the server resolves names to ids and creates
  // what the project does not have yet.
  let editingLabels = $state(false);
  let labelText = $state('');

  function startLabels(): void {
    labelText = (detail?.labels ?? []).map((l) => l.name).join(', ');
    editingLabels = true;
  }

  async function submitLabels(e: Event): Promise<void> {
    e.preventDefault();
    if (!detail) return;
    const names = labelText
      .split(',')
      .map((n) => n.trim())
      .filter((n) => n !== '');
    try {
      detail = await api.setLabels(detail.id, names);
      editingLabels = false;
    } catch (err) {
      error = err instanceof ApiError ? err.message : String(err);
    }
  }

  // ---- relations (docs/decisions/0006) ----
  //
  // The other end is chosen by SEARCH rather than by pasting an id: a namespaced id
  // is not something anyone has in their head, and the ids that are easy to paste
  // are the ones that are easy to get wrong.
  const RELATION_TYPES = ['relates_to', 'duplicate', 'blocking', 'blocked_by'];
  let addingRelation = $state(false);
  let relQuery = $state('');
  let relType = $state('relates_to');
  let relHits = $state<ThreadHit[]>([]);

  async function searchRelated(): Promise<void> {
    const q = relQuery.trim();
    if (q === '') {
      relHits = [];
      return;
    }
    try {
      const hits = await api.threads(q);
      // A thread cannot relate to itself, and the server relates only within one
      // project — so filtering here turns two refusals into two fewer round trips.
      relHits = hits.filter((h) => h.id !== detail?.id).slice(0, 6);
    } catch {
      relHits = [];
    }
  }

  async function relate(other: string): Promise<void> {
    if (!detail) return;
    try {
      detail = await api.relateThreads(detail.id, other, relType);
      relQuery = '';
      relHits = [];
      addingRelation = false;
    } catch (err) {
      error = err instanceof ApiError ? err.message : String(err);
    }
  }

  async function unrelate(other: string): Promise<void> {
    if (!detail) return;
    try {
      detail = await api.unrelateThreads(detail.id, other);
    } catch (err) {
      error = err instanceof ApiError ? err.message : String(err);
    }
  }

  async function reopenThread(): Promise<void> {
    if (!detail || completing) return;
    completing = true;
    try {
      detail = await api.reopenThread(detail.id);
      dodNote = null;
      await store.refresh();
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e);
    } finally {
      completing = false;
    }
  }

  function onSaved(d: ThreadDetail): void {
    elsewhere = false;
    // The write already returned the fresh thread, so adopt it rather than
    // spending another round trip re-fetching what we were just handed.
    detail = d;
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
  // Repaint when an agent edits the thread that is on screen. Watching a
  // Logbook change as it happens is the whole point of the MCP surface
  // (docs/journal/MCP-ACCESS.md); before this, ThreadView loaded once on open and
  // nothing short of navigating away would refresh it.
  $effect(() => {
    const changed = store.threadChanged;
    if (!changed) return;
    const open = store.threadId;
    if (!open || changed.id !== open) return;
    // untrack the reload so re-entering this effect is driven only by a NEW
    // change event, never by the state the reload itself writes.
    untrack(() => {
      // Never repaint over unsaved work. An agent editing the Logbook while
      // someone has the Brief open is exactly the case this whole surface
      // exists for, and losing their typing to it would be the worst possible
      // answer. Say so instead; the editor keeps saving on its own schedule.
      if (anyDirty()) {
        elsewhere = true;
        return;
      }
      void reloadDetail();
    });
  });

  async function reloadDetail(): Promise<void> {
    const tid = store.threadId;
    if (!tid) return;
    try {
      const d = await api.thread(tid);
      detail = d;
      if (commentsLoaded) comments = await api.comments(tid);
    } catch {
      // a transient failure just leaves what is on screen; the next change
      // event or a manual reopen recovers it
    }
  }

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
  // because it deliberately carries no credential (docs/journal/PLANE-SYNC.md Phase 5).
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

  // The renderer — diagrams, heading ids, live checkboxes and the prose styling —
  // lives in Prose.svelte / lib/prose.ts, so a page renders exactly like a thread
  // artifact. Headings come back from it, for the minimap.

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

  // On content switch, remember where to land. The restore itself waits for
  // Prose to report the document settled — diagrams change the page height, so
  // scrolling before they render scrolls to the wrong place.
  let pendingScroll = $state<number | null>(null);
  $effect(() => {
    void store.threadId;
    void sel.kind;
    void sel.idx;
    pendingScroll = savedScroll();
  });

  function onRendered(): void {
    const y = pendingScroll;
    pendingScroll = null;
    if (y !== null) window.scrollTo({ top: y });
  }

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
      <!-- The title is the Plane work item's NAME, not part of the body, so it
           is renamed on its own path rather than through the splice. -->
      {#if store.kiosk}
        <h2 class="ttl">{detail.title}</h2>
      {:else}
        <!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
        <h2
          class="ttl edit"
          contenteditable="plaintext-only"
          role="textbox"
          aria-label={t('thread.renameHint')}
          tabindex="0"
          spellcheck="false"
          title={t('thread.renameHint')}
          bind:this={titleEl}
          onfocus={() => (titleWas = detail?.title ?? '')}
          onblur={commitTitle}
          onkeydown={onTitleKey}
        >{detail.title}</h2>
      {/if}
      <div class="chips">
        <!-- Plane's own labels: what kind of work this is (docs/decisions/0006) -->
        {#if editingLabels}
          <form class="labelform" onsubmit={submitLabels}>
            <!-- svelte-ignore a11y_autofocus -->
            <input bind:value={labelText} placeholder={t('thread.labelsPlaceholder')} autofocus />
            <button type="submit">{t('thread.save')}</button>
            <button type="button" onclick={() => (editingLabels = false)}>{t('form.cancel')}</button>
          </form>
        {:else}
          {#each detail.labels ?? [] as l (l.id)}
            <span class="chip label">{l.name}</span>
          {/each}
          {#if !store.kiosk}
            <button class="chip label add" onclick={startLabels} title={t('thread.editLabels')}>
              {(detail.labels ?? []).length ? '✎' : '+ ' + t('thread.labels')}
            </button>
          {/if}
        {/if}
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
        <!-- written in Plane and imported (docs/decisions/0001). Deliberately a
             quiet chip, not a banner: editing in Plane is allowed. -->
        {#if detail.from_plane}
          <span class="chip who" title={t('thread.fromPlaneHelp')}>{t('thread.fromPlane')}</span>
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
      <nav class="side" class:collapsed={sideCollapsed} data-tour="artifacts">
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

            {#if detail.links?.length || (canEdit && !store.kiosk)}
              <div class="sect">
                {t('thread.links')}
                {#if canEdit && !store.kiosk}
                  <button class="sect-add" onclick={() => (addingLink = !addingLink)} title={t('thread.addLink')}>+</button>
                {/if}
              </div>
              {#if addingLink}
                <form class="linkform" onsubmit={submitLink}>
                  <input bind:value={linkURL} placeholder="https://…" />
                  <input bind:value={linkTitle} placeholder={t('thread.linkTitle')} />
                  <button type="submit" disabled={linkURL.trim() === ''}>{t('thread.addLink')}</button>
                </form>
              {/if}
              {#each detail.links ?? [] as l (l.id)}
                <div class="pin-row">
                  <a class="item" href={l.url} target="_blank" rel="noreferrer noopener" title={l.url}>
                    <span class="ico">🔗</span>{l.title || l.url}
                  </a>
                  {#if canEdit && !store.kiosk}
                    <button class="pin-x" onclick={() => dropLink(l.id)} aria-label="remove link">×</button>
                  {/if}
                </div>
              {/each}
            {/if}

            {#if detail.related?.length || !store.kiosk}
              <div class="sect">
                {t('thread.related')}
                {#if !store.kiosk}
                  <button class="sect-add" onclick={() => (addingRelation = !addingRelation)} title={t('thread.addRelation')}>+</button>
                {/if}
              </div>
              {#if addingRelation}
                <div class="linkform">
                  <select bind:value={relType}>
                    {#each RELATION_TYPES as rt (rt)}
                      <option value={rt}>{rt.replace('_', ' ')}</option>
                    {/each}
                  </select>
                  <input bind:value={relQuery} oninput={searchRelated} placeholder={t('thread.findThread')} />
                  {#each relHits as h (h.id)}
                    <button type="button" class="hit" onclick={() => relate(h.id)} title={h.bubble_name}>
                      {h.name}
                    </button>
                  {/each}
                </div>
              {/if}
              {#each detail.related ?? [] as r (r.id + r.type)}
                <div class="pin-row">
                  <button class="item" onclick={() => store.openThread(r.id)} title={r.type}>
                    <span class="ico">🔀</span>{r.title || r.id}
                    <span class="reltype">{r.type.replace('_', ' ')}</span>
                  </button>
                  {#if !store.kiosk}
                    <button class="pin-x" onclick={() => unrelate(r.id)} aria-label="remove relation">×</button>
                  {/if}
                </div>
              {/each}
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
        {#if canEdit && !store.kiosk}
          <div class="edit-bar">
            <!-- Finishing leads the bar: it is the act the whole thread exists to
                 reach, and it used to be the one thing you had to leave for
                 Plane's UI (docs/journal/THREAD-LIFECYCLE.md). -->
            {#if detail.level === 'done'}
              <button onclick={reopenThread} disabled={completing} title={t('thread.reopenThread')}>
                ↩ {t('thread.reopenThread')}
              </button>
            {:else}
              <button
                class="finish"
                onclick={() => completeThread()}
                disabled={completing}
                title={t('thread.doneHint')}
              >
                🏆 {completing ? t('thread.completing') : t('thread.done')}
              </button>
            {/if}
            {#if editRegion && detail.regions?.[editRegion]}
              <button
                class="danger"
                onclick={() => (pending = { kind: 'region', region: editRegion })}
                title={t('thread.deleteRegion')}>🗑</button
              >
            {/if}
            <button onclick={() => (moving = true)} title={t('thread.move')}>↔</button>
            <button
              class="danger"
              onclick={() => (pending = { kind: 'thread' })}
              title={t('thread.delete')}>🗑 {t('del.threadWord')}</button
            >
            <div class="seg" role="group" aria-label={t('thread.viewMode')} data-tour="viewmode">
              <button
                class:on={!editing}
                aria-pressed={!editing}
                onclick={() => (editing = false)}>{t('thread.rendered')}</button
              >
              <button
                class:on={editing}
                aria-pressed={editing}
                onclick={() => (editing = true)}>{t('thread.markdown')}</button
              >
            </div>
          </div>
        {/if}

        <!-- Finished over an open Definition of Done. Not an error and not a
             refusal — the record simply says what was left, so it is a decision
             somebody can see rather than a shortcut nobody notices. -->
        {#if dodNote}
          <p class="dodblock">
            <b>{t('thread.dodLeftOpen')}</b>
            {dodNote.join(' · ')}
          </p>
        {/if}

        {#if elsewhere}
          <p class="elsewhere">
            {t('editor.changedElsewhere')}
            <button type="button" class="link" onclick={takeTheirs}>{t('editor.reload')}</button>
          </p>
        {/if}

        {#if editing && editRegion && detail.regions?.[editRegion]}
          <!-- Markdown source, autosaved. Two editors when the Logbook is open:
               the DoD is its own region and is written separately. -->
          <div class="editors">
            <h1 class="edit-title">
              {editRegion === 'logbook' ? t('thread.logbook') : detail.title}
            </h1>
            {#key `${detail.id}:${editRegion}`}
              <ArtifactEditor
                bind:this={editors[editRegion]}
                threadId={detail.id}
                region={editRegion}
                initial={detail.regions[editRegion].markdown}
                hash={detail.regions[editRegion].hash}
                onsaved={onSaved}
                onreload={reloadDetail}
                ondone={() => (editing = false)}
              />
            {/key}
            {#if editRegion === 'logbook' && detail.regions?.dod}
              <h2 class="edit-title">{t('thread.dod')}</h2>
              {#key `${detail.id}:dod`}
                <ArtifactEditor
                  bind:this={editors.dod}
                  threadId={detail.id}
                  region="dod"
                  initial={detail.regions.dod.markdown}
                  hash={detail.regions.dod.hash}
                  onsaved={onSaved}
                  onreload={reloadDetail}
                  ondone={() => (editing = false)}
                />
              {/key}
            {/if}
          </div>
        {:else if sel.kind === 'logbook' && detail.logbook}
          <!-- rendered through the same goldmark/prose pipeline as the rest.
               Each section carries its own data-region because their todos are
               numbered independently, and the click handler needs to know which
               list it just counted. -->
          <Prose
            ticking={ticking !== null}
            onclick={onProseClick}
            onheadings={(h) => (headings = h)}
            onrendered={onRendered}
            key={`l:${detail.logbook.html}:${detail.logbook.dod_html}`}
          >
            <h1>{t('thread.logbook')}</h1>
            <div data-region="logbook" data-tour="logbook">
              {#if detail.logbook.html}
                {@html detail.logbook.html}
              {:else}
                <p class="dim">{t('thread.noTasks')}</p>
              {/if}
            </div>
            {#if detail.logbook.dod_html}
              <h2>{t('thread.dod')}</h2>
              <div data-region="dod">
                {@html detail.logbook.dod_html}
              </div>
            {/if}
          </Prose>
        {:else if current}
          <!-- Todos are NOT confined to the Logbook: 59 of 96 real bodies keep
               them here, in the document. Without this wrapper they rendered as
               checkboxes that did nothing at all. -->
          {@const owner =
            sel.kind === 'revision' ? (detail.revisions?.[sel.idx]?.id ?? detail.id) : detail.id}
          <Prose
            ticking={ticking !== null}
            onclick={onProseClick}
            onheadings={(h) => (headings = h)}
            onrendered={onRendered}
            key={`a:${current.html}`}
          >
            <div data-region="brief" data-thread={owner}>
              <!-- server-rendered goldmark HTML (safe: raw HTML is escaped) -->
              {@html current.html}
            </div>
          </Prose>
        {:else}
          <p class="dim">{t('thread.nothing')}</p>
        {/if}
      </main>

      {#if headings.length}
        <ThreadToc {headings} />
      {/if}
    </div>

    {#if pending}
      <ConfirmDelete
        what={pendingWhat}
        detail={pending.kind === 'thread' ? t('del.thread') : t('del.region')}
        busy={deleting}
        oncancel={() => (pending = null)}
        onconfirm={confirmDelete}
      />
    {/if}

    {#if moving && detail}
      <MoveThread
        threadId={store.threadId ?? detail.id}
        title={detail.title}
        onclose={() => (moving = false)}
        onmoved={() => reloadDetail()}
      />
    {/if}

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
                  <div class="msg-body"><Prose html={c.html} compact /></div>
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
  /* Plane's labels. Neutral rather than colour-coded: a label's own colour comes
     from Plane and is not guaranteed to be legible against either theme. */
  .sect-add {
    float: right;
    border: none;
    background: none;
    color: var(--faint);
    cursor: pointer;
    font: inherit;
    line-height: 1;
    padding: 0 0.2rem;
  }
  .sect-add:hover {
    color: var(--wip);
  }
  .linkform {
    display: grid;
    gap: 0.25rem;
    padding: 0.25rem 0.35rem 0.5rem;
  }
  .linkform select,
  .linkform input {
    font: inherit;
    font-size: 0.75rem;
    padding: 0.3rem 0.4rem;
    border-radius: 7px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 6%, transparent);
    color: var(--text);
  }
  .linkform button {
    font: inherit;
    font-size: 0.75rem;
    padding: 0.3rem;
    border-radius: 7px;
    border: 1px solid var(--line);
    background: transparent;
    color: var(--muted);
    cursor: pointer;
  }
  .linkform button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .labelform {
    display: flex;
    gap: 0.3rem;
    align-items: center;
    flex: 1 1 16rem;
  }
  .labelform input {
    font: inherit;
    font-size: 0.72rem;
    padding: 0.15rem 0.45rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 6%, transparent);
    color: var(--text);
    flex: 1;
  }
  .labelform button {
    font: inherit;
    font-size: 0.68rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: transparent;
    color: var(--muted);
    padding: 0.12rem 0.5rem;
    cursor: pointer;
  }
  .chip.label.add {
    cursor: pointer;
    border-style: dashed;
    background: transparent;
    font: inherit;
    font-size: 0.68rem;
    color: var(--faint);
  }
  .hit {
    font: inherit;
    font-size: 0.75rem;
    text-align: left;
    padding: 0.25rem 0.4rem;
    border-radius: 7px;
    border: 1px solid transparent;
    background: color-mix(in oklab, var(--text) 5%, transparent);
    color: var(--muted);
    cursor: pointer;
  }
  .hit:hover {
    border-color: var(--line);
    color: var(--text);
  }
  .reltype {
    margin-left: auto;
    font-size: 0.65rem;
    color: var(--faint);
  }
  .chip.label {
    text-transform: none;
    background: color-mix(in oklab, var(--text) 7%, transparent);
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

  .ttl.edit {
    border-radius: 7px;
    padding: 0 0.3rem;
    margin-left: -0.3rem;
    outline: none;
    cursor: text;
  }
  .ttl.edit:hover {
    background: color-mix(in oklab, var(--text) 7%, transparent);
  }
  .ttl.edit:focus {
    background: color-mix(in oklab, var(--wip) 12%, transparent);
    box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--wip) 50%, transparent);
  }


  /* ---- editing ---- */
  .elsewhere {
    margin: 0 0 0.7rem;
    padding: 0.5rem 0.75rem;
    border-radius: 10px;
    border: 1px solid color-mix(in oklab, oklch(0.72 0.19 25) 40%, var(--line));
    background: color-mix(in oklab, oklch(0.72 0.19 25) 8%, transparent);
    font-family: var(--sans);
    font-size: 0.78rem;
    color: var(--text);
  }
  .elsewhere .link {
    border: none;
    background: none;
    padding: 0;
    margin-left: 0.4rem;
    color: var(--wip);
    text-decoration: underline;
    cursor: pointer;
    font: inherit;
  }
  .edit-bar {
    position: sticky;
    top: 0.75rem;
    z-index: 5;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    /* The bar had no gap, so anything without its own pill padding collided with
       its neighbour. */
    gap: 0.4rem;
    margin-bottom: 0.6rem;
    pointer-events: none; /* only the controls themselves catch clicks */
  }
  .edit-bar > * {
    pointer-events: auto;
  }
  /* The pill, for every direct button in the bar.
     It used to be declared only on .danger, so a button without that class
     inherited the reading column's 18px serif and drew no border at all — it
     looked like a stray line of prose sitting in the toolbar. `>` keeps the
     segmented control's own buttons out of it, since they are nested in .seg and
     have their own look. */
  .edit-bar > button {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.32rem 0.7rem;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 5%, transparent);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    color: var(--muted);
    font-family: var(--sans);
    font-size: 0.74rem;
    font-weight: 600;
    line-height: 1.2;
    white-space: nowrap;
    cursor: pointer;
  }
  .edit-bar > button:hover:not(:disabled) {
    color: var(--text);
    border-color: color-mix(in oklab, var(--text) 25%, var(--line));
  }
  /* Filled, and the only one in the bar that is: finishing is the goal the thread
     exists to reach, and everything else here is a tool. */
  .edit-bar .finish {
    background: var(--done);
    border-color: transparent;
    color: oklch(0.24 0.03 90);
    font-weight: 700;
  }
  .edit-bar .finish:hover:not(:disabled) {
    background: color-mix(in oklab, var(--done) 88%, white);
    border-color: transparent;
    color: oklch(0.24 0.03 90);
  }
  .edit-bar button:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .dodblock {
    margin: 0 0 0.7rem;
    padding: 0.5rem 0.75rem;
    border-radius: 10px;
    border: 1px solid color-mix(in oklab, var(--done) 45%, var(--line));
    background: color-mix(in oklab, var(--done) 8%, transparent);
    font-size: 0.8rem;
    color: var(--muted);
    line-height: 1.45;
  }
  .edit-bar .danger:hover {
    color: oklch(0.98 0 0);
    background: oklch(0.55 0.2 25);
    border-color: transparent;
  }
  .seg {
    display: inline-flex;
    padding: 2px;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 5%, transparent);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
  }
  .seg button {
    padding: 0.32rem 0.85rem;
    border: none;
    border-radius: 999px;
    background: none;
    color: var(--muted);
    font-family: var(--sans);
    font-size: 0.76rem;
    font-weight: 600;
    cursor: pointer;
    transition:
      background 120ms ease,
      color 120ms ease;
  }
  .seg button:hover {
    color: var(--text);
  }
  .seg button.on {
    color: oklch(0.16 0.02 265);
    background: var(--wip);
  }
  .editors {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .edit-title {
    margin: 0;
    font-family: var(--sans);
    font-size: 1.05rem;
    font-weight: 700;
    color: var(--muted);
  }

  @media (max-width: 640px) {
    .side {
      flex-basis: 150px;
    }
    .content {
      padding: 1.4rem 1.1rem 3rem;
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
    align-items: flex-start;
    gap: 0.15rem;
  }
  .msg.mine {
    align-self: flex-end;
    align-items: flex-end;
  }
  .msg.mine .msg-meta {
    flex-direction: row-reverse;
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
    /* The bubble around a message. The prose inside is rendered by Prose in its
       `compact` mode, which drops the page column a document gets — in a chat
       bubble an auto margin beats align-items, and every message floated to the
       middle with neither side aligned. */
    margin: 0;
    max-width: 100%;
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
