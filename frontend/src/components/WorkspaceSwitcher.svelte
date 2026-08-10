<script lang="ts">
  // Alt+Tab, for workspaces.
  //
  // Shift is the HELD key and Tab is the advance, exactly as Windows uses Alt:
  // press Shift+Tab to open and step forward, keep tapping Tab to keep going,
  // release Shift to commit. Arrows move without committing, a letter jumps to
  // the next workspace starting with it, Esc puts back whatever was scoped
  // before.
  //
  // Shift+Tab is also how a keyboard user walks focus BACKWARDS, which is not a
  // shortcut worth breaking. So it is only taken over when focus is on the board
  // itself: inside any field, editor or menu the browser keeps it.
  import { store } from '../lib/store.svelte';
  import { t } from '../lib/i18n.svelte';

  // The board suppresses this while a modal, the omnibar or a context menu owns
  // the keyboard — those have their own idea of what Tab and Escape mean.
  let { enabled = true }: { enabled?: boolean } = $props();

  type Entry = { id: string; name: string; identifier: string; bubbles: number };

  // "" is All projects and belongs in the ring: it is a real scope, and without
  // it there is no way back to the whole board without reaching for the mouse.
  const entries = $derived.by<Entry[]>(() => {
    const all: Entry = {
      id: '',
      name: t('combo.allProjects'),
      identifier: '',
      bubbles: store.bubbles.length,
    };
    const rest = store.projects.map((p) => ({
      id: p.id,
      name: p.name,
      identifier: store.workspaces.find((w) => w.id === p.id)?.identifier ?? '',
      bubbles: store.bubbles.filter((b) => b.project === p.id).length,
    }));
    return [all, ...rest];
  });

  let open = $state(false);
  let idx = $state(0);
  /** what was scoped when the switcher opened — what Escape puts back */
  let before = $state('');

  // Typing a letter jumps to the next match, Windows-style. The buffer resets
  // between openings rather than on a timer: one pass through a list is one
  // interaction.
  let typed = $state('');

  function isEditable(el: EventTarget | null): boolean {
    if (!(el instanceof HTMLElement)) return false;
    if (el.isContentEditable) return true;
    return ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName) || el.closest('[role="menu"]') !== null;
  }

  function show(): void {
    before = store.project;
    idx = Math.max(0, entries.findIndex((e) => e.id === store.project));
    typed = '';
    open = true;
  }

  function step(by: number): void {
    const n = entries.length;
    idx = (idx + by + n) % n;
  }

  function commit(): void {
    open = false;
    const target = entries[idx];
    if (target && target.id !== store.project) store.selectProject(target.id);
  }

  function cancel(): void {
    open = false;
    if (store.project !== before) store.selectProject(before);
  }

  // Jump to the next entry starting with the letter — "next", not "first", so
  // pressing the same letter walks through the workspaces that share it.
  function jump(letter: string): void {
    const n = entries.length;
    for (let i = 1; i <= n; i++) {
      const cand = entries[(idx + i) % n];
      if (cand.name.toLowerCase().startsWith(letter)) {
        idx = (idx + i) % n;
        return;
      }
    }
  }

  // Two letters, always. A Plane identifier is free text and the real ones run
  // to eight characters, which no square badge can hold — and a badge that
  // resizes per workspace stops reading as a grid. Two initials are decoration
  // that anchors the eye; the name underneath is what actually identifies it.
  function initials(e: Entry): string {
    return (e.identifier || e.name).replace(/[^\p{L}\p{N}]/gu, '').slice(0, 2).toUpperCase();
  }

  function onKeyDown(e: KeyboardEvent): void {
    if (!open) {
      if (!enabled || !e.shiftKey || e.key !== 'Tab') return;
      if (isEditable(e.target)) return; // reverse-tabbing still belongs to the browser
      if (entries.length < 2) return; // nothing to switch between
      e.preventDefault();
      show();
      step(1);
      return;
    }

    switch (e.key) {
      // Shift is HELD throughout, so Tab cannot also mean "go back" the way
      // Alt+Shift+Tab does on Windows. The arrows are the way back.
      case 'Tab':
        e.preventDefault();
        step(1);
        return;
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        step(1);
        return;
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        step(-1);
        return;
      case 'Home':
        e.preventDefault();
        idx = 0;
        return;
      case 'End':
        e.preventDefault();
        idx = entries.length - 1;
        return;
      case 'Enter':
        e.preventDefault();
        commit();
        return;
      case 'Escape':
        e.preventDefault();
        cancel();
        return;
    }
    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();
      typed = e.key.toLowerCase();
      jump(typed);
    }
  }

  // Releasing the held key commits — the whole point of the metaphor.
  function onKeyUp(e: KeyboardEvent): void {
    if (open && e.key === 'Shift') commit();
  }

  // A window that loses focus mid-switch must not leave the overlay stuck over
  // the board with no key left to dismiss it.
  function onBlur(): void {
    if (open) cancel();
  }
</script>

<svelte:window onkeydown={onKeyDown} onkeyup={onKeyUp} onblur={onBlur} />

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="scrim" onclick={cancel}>
    <div
      class="panel"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-label={t('switch.title')}
      onclick={(e) => e.stopPropagation()}
    >
      <div class="tiles">
        {#each entries as e, i (e.id)}
          <button
            class="tile"
            class:on={i === idx}
            aria-current={i === idx}
            onmouseenter={() => (idx = i)}
            onclick={commit}
          >
            <span class="mark" class:allmark={e.id === ''}>
              {e.id === '' ? '∗' : initials(e)}
            </span>
            <span class="name">{e.name}</span>
            <span class="count">{t('switch.bubbles', { n: e.bubbles })}</span>
          </button>
        {/each}
      </div>
      <p class="hint">{t('switch.hint')}</p>
    </div>
  </div>
{/if}

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 300;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    background: oklch(0.12 0.02 265 / 0.5);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .panel {
    max-width: min(92vw, 60rem);
    max-height: 80vh;
    overflow-y: auto;
    padding: 1.1rem 1.15rem 0.85rem;
    border-radius: 20px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 30px 80px var(--shadow-strong);
    font-family: var(--sans);
  }
  .tiles {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 0.5rem;
  }
  .tile {
    width: 9.5rem;
    display: grid;
    justify-items: center;
    gap: 0.3rem;
    padding: 0.85rem 0.5rem 0.7rem;
    border-radius: 14px;
    border: 1px solid transparent;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    cursor: pointer;
  }
  /* The selection ring is the whole UI — it has to read at a glance, from the
     corner of the eye, while a key is held down. */
  .tile.on {
    background: color-mix(in oklab, var(--wip) 14%, transparent);
    border-color: color-mix(in oklab, var(--wip) 55%, var(--line));
  }
  .mark {
    display: grid;
    place-items: center;
    width: 2.6rem;
    height: 2.6rem;
    border-radius: 12px;
    background: color-mix(in oklab, var(--text) 8%, transparent);
    border: 1px solid var(--line);
    font-size: 0.9rem;
    font-weight: 800;
    letter-spacing: 0.03em;
    color: var(--muted);
  }
  .tile.on .mark {
    background: var(--wip);
    border-color: transparent;
    color: oklch(0.99 0 0);
  }
  .allmark {
    font-size: 1.1rem;
  }
  /* Workspace names are hostnames here — warehouse.cuby.work does not fit on one
     line at any tile width worth having. Two lines, broken anywhere, with the
     height reserved either way so the counts underneath stay on one baseline. */
  .name {
    width: 100%;
    min-height: 2.5em;
    font-size: 0.78rem;
    font-weight: 700;
    line-height: 1.25;
    text-align: center;
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .count {
    font-size: 0.68rem;
    color: var(--faint);
  }
  .hint {
    margin: 0.75rem 0 0;
    text-align: center;
    font-size: 0.7rem;
    color: var(--faint);
  }
</style>
