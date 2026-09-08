<script lang="ts">
  // A thread, against the real server.
  //
  // `ThreadView` draws; this holds the document, the hash it was read at, and
  // what happens when the write comes back 409. Keeping those apart is what
  // lets the view be looked at in /theme/mock with invented data and be the
  // same component here.
  //
  // Every write carries the hash it read. That is the only thing standing
  // between two editors and a lost paragraph, and it is why a conflict is a
  // banner offering a reload rather than a save that quietly wins.
  import { ApiError, api, type Doc, type ThreadHeat } from '../lib/api';
  import { bandName } from '../lib/bands';
  import ThreadView from './ThreadView.svelte';

  let {
    thread,
    onback,
    onsearch,
  }: { thread: ThreadHeat; onback?: () => void; onsearch?: () => void } = $props();

  let doc = $state<Doc | null>(null);
  let error = $state('');
  let elsewhere = $state(false);
  let saving = $state(false);

  async function load() {
    try {
      doc = await api.readThread(thread.id);
      error = '';
      elsewhere = false;
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => {
    thread.id;
    load();
  });

  async function save(markdown: string) {
    if (!doc || saving) return;
    // With a conflict standing, the base we hold is old and every write is
    // another 409 — including the blur-save that fires when the pointer leaves
    // the editor to press "recargar", which raced the reload it was answering.
    if (elsewhere) return;
    saving = true;
    try {
      // The base is the hash we READ, not the one we hold: they differ exactly
      // when somebody else wrote, which is the case this exists for.
      doc = await api.patchThread(thread.id, { base: doc.hash, content: markdown });
      elsewhere = false;
      error = '';
    } catch (e) {
      if (e instanceof ApiError && e.conflict) {
        // Not an error to apologise for. Somebody wrote first; the edit is
        // still in the editor, and reloading is a choice offered rather than a
        // save silently lost.
        elsewhere = true;
      } else {
        error = (e as Error).message;
      }
    } finally {
      saving = false;
    }
  }
</script>

{#if error}
  <p class="card glass m-6 p-4 text-sm text-error-500">{error}</p>
{:else if !doc}
  <p class="faint p-6 text-sm">…</p>
{:else}
  <ThreadView
    seq={thread.seq}
    title={thread.name}
    lifecycle={thread.heat.lifecycle}
    reason={thread.heat.reason}
    priority={thread.priority ?? ''}
    threadState={bandName(thread.heat.lifecycle)}
    markdown={doc.content}
    html={doc.html}
    {elsewhere}
    {onback}
    {onsearch}
    onsave={save}
    onreload={load} />
{/if}
